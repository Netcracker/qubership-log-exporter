// Copyright 2024 Qubership
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package httpservice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log_exporter/internal/config"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"text/template"
	"time"

	log "github.com/sirupsen/logrus"
)

type NewRelicService struct {
	appConfig *config.Config
	dsConfig  *config.DatasourceConfig
}

type NRResponse struct {
	Results  *[]NRResult
	Facets   []Facet
	Metadata NRMetadata
}

type NRResult struct {
	Events      *[]map[string]interface{}
	UniqueCount *float64
}

type NRMetadata struct {
	Facet *interface{}
}

type Facet struct {
	Name    interface{}
	Results []FacetResult
}

type FacetResult struct {
	Count float64
}

const RESULT_FIELD_NAME string = "_RESULT_"

func CreateNewRelicService(appConfig *config.Config) *NewRelicService {
	g := NewRelicService{}
	g.appConfig = appConfig
	g.dsConfig = appConfig.Datasources[appConfig.DsName]
	return &g
}

func (g *NewRelicService) Query(qName string, startTime time.Time, endTime time.Time) ([][]string, string, error) {
	now := time.Now()
	var err error
	defer func() {
		log.WithFields(log.Fields{"query": qName, "duration": time.Since(now)}).Debug("NewRelicService : Request executed and json processed")
		if err != nil {
			selfMonitorIncErrorCodeCount(qName, now)
		} else {
			selfMonitorRefreshErrorCodeCount(qName, now)
		}
	}()

	stringResult, errc, err := g.queryNewRelic(qName, startTime, endTime)
	selfMonitorObserveQueryLatency(float64(time.Since(now))/float64(time.Second), qName, now)
	selfMonitorObserveQueryResponseSize(float64(len(stringResult)), qName, now)
	if err != nil {
		return make([][]string, 0), errc, err
	}

	result := g.processJson(stringResult, qName)
	return result, "", nil
}

func (g *NewRelicService) queryNewRelic(qName string, startTime time.Time, endTime time.Time) (string, string, error) {
	queryString, err := g.getQueryString(qName, startTime, endTime)
	if err != nil {
		return "", ec.LME_8102, fmt.Errorf("NewRelicService : For query %v error creating queryString : %+v", qName, err)
	}
	log.WithFields(log.Fields{"query": qName, "query_string": queryString}).Debug("NewRelicService : Query string is prepared")

	var transport http.RoundTripper = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: g.dsConfig.ConnectionTimeout,
		}).DialContext,
		TLSClientConfig: g.dsConfig.TlsConfig,
	}
	client := http.Client{
		Transport: transport,
		Timeout:   g.dsConfig.ConnectionTimeout,
	}

	newRelicEndpoint := strings.Trim(g.dsConfig.Host, " /") + "/v1/accounts/" + g.dsConfig.User + "/query?nrql=" + url.QueryEscape(queryString)
	req, err := http.NewRequest("GET", newRelicEndpoint, nil)
	if err != nil {
		return "", ec.LME_7140, fmt.Errorf("NewRelicService : For query %v error creating HTTP request: %+v", qName, err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Query-Key", g.dsConfig.Password)
	log.Debugf("NewRelicService: For query %v request to NewRelic is %+v", qName, req)
	resp, err := client.Do(req)
	if err != nil {
		return "", ec.LME_7140, fmt.Errorf("NewRelicService : For query %v error accessing %v : %+v", qName, newRelicEndpoint, err)
	}

	if resp.Body != nil {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.WithField("error", err).Error("NewRelicService : Error closing response body")
			}
		}()
	}

	log.WithFields(log.Fields{"query": qName, "resp": resp}).Debug("NewRelicService : Received response")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", ec.LME_7140, fmt.Errorf("NewRelicService : For query %v to %v error reading body : %+v", qName, newRelicEndpoint, err)
	}
	result := string(body)
	log.WithFields(log.Fields{"query": qName, "new_relic_endpoint": newRelicEndpoint, "status": resp.Status, "result_count": len(result)}).Info("NewRelicService : Received response from NewRelic")
	if resp.StatusCode != 200 {
		log.WithField(ec.FIELD, ec.LME_7142).WithFields(log.Fields{"query": qName, "status_code": resp.StatusCode, "response_preview": utils.GetLimitedPrefix(result, 10000)}).Error("NewRelicService : Received response with an error status code from graylog")
		if resp.StatusCode >= 400 {
			return "", ec.LME_7141, fmt.Errorf("NewRelicService : For query %v status code is %v", qName, resp.StatusCode)
		}
	}
	log.WithFields(log.Fields{"query": qName, "result": result}).Debug("NewRelicService : Received response body")

	return result, "", nil
}

func (g *NewRelicService) processJson(stringData string, qName string) [][]string {
	log.WithFields(log.Fields{"query": qName, "string_data": stringData}).Debug("NewRelicService : Json processing : NewRelic response is received")
	var nrResponse NRResponse
	err := json.Unmarshal([]byte(stringData), &nrResponse)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_7143).WithFields(log.Fields{"query": qName, "error": err}).Error("NewRelicService : Unmarshaling error")
		return nil
	}

	if nrResponse.Metadata.Facet != nil {
		return g.processFacets(nrResponse, qName)
	} else if nrResponse.Results != nil {
		if len(*nrResponse.Results) == 0 {
			log.WithField("query", qName).Warn("NewRelicService : Json processing : Results list is empty")
			return nil
		}
		events := (*nrResponse.Results)[0].Events
		if events != nil {
			return g.processEvents(*events, qName)
		}
		uniqueCount := (*nrResponse.Results)[0].UniqueCount
		if uniqueCount != nil {
			return g.processUniqueCounts(*uniqueCount, qName)
		}
	}

	log.WithField(ec.FIELD, ec.LME_7144).WithField("query", qName).Error("NewRelicService : Json processing : Got an unknown JSON output case, processing will be skipped")

	return nil
}

func (g *NewRelicService) processEvents(events []map[string]interface{}, qName string) [][]string {
	log.WithField("query", qName).Debug("NewRelicService : Json processing : processEvents called")
	keyList := make([]string, 0)
	keyListSet := make(map[string]int)
	for _, event := range events {
		for k := range event {
			if _, ok := keyListSet[k]; !ok {
				keyListSet[k] = len(keyList)
				keyList = append(keyList, k)
			}
		}
	}
	log.WithFields(log.Fields{"query": qName, "key_list": keyList}).Debug("NewRelicService : Json processing : Column names were found")
	log.WithFields(log.Fields{"query": qName, "key_list_set": keyListSet}).Debug("NewRelicService : Json processing : Key list set is built")
	records := make([][]string, len(events)+1)
	records[0] = keyList
	rowlen := len(keyList)
	for i, event := range events {
		row := make([]string, rowlen)
		for k, v := range event {
			index, ok := keyListSet[k]
			if !ok {
				log.WithField(ec.FIELD, ec.LME_7143).WithFields(log.Fields{"query": qName, "key": k, "key_list_set": keyListSet, "event": event}).Error("NewRelicService : Json processing : Can not find an index for the key in keyListSet, which is completely unexpected")
			}
			row[index] = fmt.Sprintf("%v", v)
		}
		records[i+1] = row
	}

	log.WithFields(log.Fields{"query": qName, "records": records}).Debug("NewRelicService : Json processing : Records were calculated")
	return records
}

func (g *NewRelicService) processFacets(nrResponse NRResponse, qName string) [][]string {
	log.WithField("query", qName).Debug("NewRelicService : Facets processing : processFacets called")
	labelNames := make([]string, 0)
	switch facet := (*nrResponse.Metadata.Facet).(type) {
	case string:
		labelNames = append(labelNames, facet)
	case []string:
		labelNames = append(labelNames, facet...)
	case []interface{}:
		for _, v := range facet {
			labelNames = append(labelNames, fmt.Sprintf("%v", v))
		}
	case interface{}:
		labelNames = append(labelNames, fmt.Sprintf("%v", facet))
	default:
		log.WithField(ec.FIELD, ec.LME_7144).WithFields(log.Fields{"query": qName, "facet_type": reflect.TypeOf(facet)}).Error("NewRelicService : Facets processing : Got an unknown type for Metadata.Facet in JSON")
	}
	log.WithFields(log.Fields{"query": qName, "label_names": labelNames}).Debug("NewRelicService : Facets processing : Got label names")

	columnNumber := len(labelNames) + 1

	records := make([][]string, 0)
	heading := make([]string, 0, columnNumber)
	heading = append(heading, labelNames...)
	heading = append(heading, RESULT_FIELD_NAME)
	records = append(records, heading)

	for _, facetItem := range nrResponse.Facets {
		row := make([]string, 0, columnNumber)
		switch f := facetItem.Name.(type) {
		case string:
			row = append(row, f)
		case []string:
			row = append(row, f...)
		case []interface{}:
			for _, v := range f {
				row = append(row, fmt.Sprintf("%v", v))
			}
		case interface{}:
			row = append(row, fmt.Sprintf("%v", f))
		default:
			log.WithField(ec.FIELD, ec.LME_7144).WithFields(log.Fields{"query": qName, "facet_name_type": reflect.TypeOf(facetItem.Name)}).Error("NewRelicService : Facets processing : Got an unknown type for Facets.Name in JSON")
		}
		if len(facetItem.Results) != 1 {
			if len(facetItem.Results) == 0 {
				log.WithField("query", qName).Warn("NewRelicService : Facets processing : Facet item has no results, skipping it")
				continue
			}
			log.WithFields(log.Fields{"query": qName, "results_count": len(facetItem.Results)}).Debug("NewRelicService : Facets processing : Facet item has an unexpected number of results")
		}
		row = append(row, fmt.Sprintf("%v", facetItem.Results[0].Count))
		if len(row) != columnNumber {
			log.WithFields(log.Fields{"query": qName, "row_count": len(row), "column_number": columnNumber}).Warn("NewRelicService : Facets processing : Row length does not match the column number")
		}
		records = append(records, row)
	}
	log.WithFields(log.Fields{"query": qName, "records": records}).Debug("NewRelicService : Facets processing : Records were calculated")

	return records
}

func (g *NewRelicService) processUniqueCounts(uniqueCount float64, qName string) [][]string {
	log.WithField("query", qName).Debug("NewRelicService : UniqueCount processing : processUniqueCounts called")
	records := make([][]string, 0)
	records = append(records, []string{RESULT_FIELD_NAME})
	records = append(records, []string{fmt.Sprintf("%v", uniqueCount)})
	log.WithFields(log.Fields{"query": qName, "records": records}).Debug("NewRelicService : UniqueCount processing : Records were calculated")
	return records
}

func (g *NewRelicService) getQueryString(qName string, startTime time.Time, endTime time.Time) (string, error) {
	qCfg := g.appConfig.Queries[qName]
	templateCtx := make(map[string]string)
	templateCtx["StartTime"] = startTime.Format("2006-01-02 15:04:05 MST")
	templateCtx["EndTime"] = endTime.Format("2006-01-02 15:04:05 MST")
	tmpl, err := template.New("query_template").Parse(qCfg.QueryString)
	if err != nil {
		return "", fmt.Errorf("error creating template for query %v : %+v", qName, err)
	}
	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, templateCtx)
	if err != nil {
		return "", fmt.Errorf("error executing template for query %v : %+v", qName, err)
	}
	return buf.String(), nil
}
