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
	"encoding/json"
	"fmt"
	"io"
	"log_exporter/internal/config"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type LokiService struct {
	appConfig *config.Config
	dsConfig  *config.DatasourceConfig
}

type LokiResponse struct {
	Status string
	Data   LokiResponseData
}

type LokiResponseData struct {
	ResultType string
	Result     []LokiResponseDataResult
}

type LokiResponseDataResult struct {
	Stream map[string]string
	Values [][]string
}

func CreateLokiService(appConfig *config.Config) *LokiService {
	g := LokiService{}
	g.appConfig = appConfig
	g.dsConfig = appConfig.Datasources[appConfig.DsName]
	return &g
}

func (g *LokiService) Query(qName string, startTime time.Time, endTime time.Time) ([][]string, string, error) {
	now := time.Now()
	var err error
	defer func() {
		log.WithFields(log.Fields{"query": qName, "duration": time.Since(now)}).Debug("LokiService : Request executed and json processed")
		if err != nil {
			selfMonitorIncErrorCodeCount(qName, now)
		} else {
			selfMonitorRefreshErrorCodeCount(qName, now)
		}
	}()

	stringResult, errc, err := g.queryLoki(qName, startTime, endTime)
	selfMonitorObserveQueryLatency(float64(time.Since(now))/float64(time.Second), qName, now)
	selfMonitorObserveQueryResponseSize(float64(len(stringResult)), qName, now)
	if err != nil {
		return make([][]string, 0), errc, err
	}

	result, errc, err := g.processJson(stringResult, qName)
	return result, errc, err
}

func (g *LokiService) queryLoki(qName string, startTime time.Time, endTime time.Time) (string, string, error) {
	qCfg := g.appConfig.Queries[qName]

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

	lokiEndpoint := strings.Trim(g.dsConfig.Host, " /") + "/loki/api/v1/query_range"
	req, err := http.NewRequest("GET", lokiEndpoint, nil)
	if err != nil {
		return "", ec.LME_7160, fmt.Errorf("LokiService : For query %v error creating request to %v : %+v", qName, lokiEndpoint, err)
	}
	if g.dsConfig.User != "" {
		req.SetBasicAuth(g.dsConfig.User, g.dsConfig.Password)
	}
	q := req.URL.Query()
	q.Add("query", qCfg.QueryString)
	q.Add("limit", "5000")
	q.Add("start", startTime.Format(time.RFC3339))
	q.Add("end", endTime.Format(time.RFC3339))
	req.URL.RawQuery = q.Encode()

	log.WithFields(log.Fields{"query": qName, "url": req.URL.String()}).Debug("LokiService : Request is generated")

	resp, err := client.Do(req)
	if err != nil {
		return "", ec.LME_7100, fmt.Errorf("LokiService : For query %v error acessing %v : %+v", qName, lokiEndpoint, err)
	}

	if resp.Body != nil {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.WithField("error", err).Error("LokiService : Error closing response body")
			}
		}()
	}

	log.WithFields(log.Fields{"query": qName, "response": resp}).Debug("LokiService : Received response")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", ec.LME_7100, fmt.Errorf("LokiService : For query %v to %v error reading body : %+v", qName, lokiEndpoint, err)
	}
	result := string(body)
	log.WithFields(log.Fields{"query": qName, "loki_endpoint": lokiEndpoint, "status": resp.Status, "result_count": len(result)}).Info("LokiService : Received response from loki")
	if resp.StatusCode != 200 {
		log.WithField(ec.FIELD, ec.LME_7102).WithFields(log.Fields{"query": qName, "status_code": resp.StatusCode, "response_preview": utils.GetLimitedPrefix(result, 10000)}).Error("LokiService : Received response with an error status code from loki")
		if resp.StatusCode >= 400 {
			return "", ec.LME_7101, fmt.Errorf("LokiService : For query %v status code is %v", qName, resp.StatusCode)
		}
	}
	log.WithFields(log.Fields{"query": qName, "result": result}).Debug("LokiService : Received response body")

	return result, "", nil
}

func (g *LokiService) processJson(stringData string, qName string) ([][]string, string, error) {
	log.WithFields(log.Fields{"query": qName, "string_data": stringData}).Debug("LokiService : Json processing : Loki response is received")
	var lokiResponse LokiResponse
	err := json.Unmarshal([]byte(stringData), &lokiResponse)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_7143).WithFields(log.Fields{"query": qName, "error": err}).Error("LokiService : Unmarshalling error")
		return nil, ec.LME_7143, fmt.Errorf("LokiService : Unmarshalling error for query %v : %+v", qName, err)
	}
	if len(lokiResponse.Data.Result) == 0 {
		return nil, "", nil
	}

	totalLen := 0
	keyList := make([]string, 0)
	keyListSet := make(map[string]int)
	keyList = append(keyList, "message")
	keyListSet["message"] = 0
	fieldsInOrder := g.appConfig.Queries[qName].FieldsInOrder
	for _, field := range fieldsInOrder {
		if field == "message" {
			continue
		}
		keyListSet[field] = len(keyList)
		keyList = append(keyList, field)
	}

	for _, result := range lokiResponse.Data.Result {
		labelsMap := result.Stream
		for k := range labelsMap {
			if _, ok := keyListSet[k]; !ok {
				keyListSet[k] = len(keyList)
				keyList = append(keyList, k)
			}
		}
		totalLen += len(result.Values)
	}
	log.WithFields(log.Fields{"query": qName, "key_list": keyList, "key_list_set": keyListSet, "total_len": totalLen}).Debug("LokiService : Json processing : Key list is built")

	records := make([][]string, 0, totalLen+1)
	records = append(records, keyList)
	rowlen := len(keyList)

	for _, result := range lokiResponse.Data.Result {
		labelsMap := result.Stream
		rowTemplate := make([]string, rowlen)
		for k, v := range labelsMap {
			index, ok := keyListSet[k]
			if !ok {
				log.WithField(ec.FIELD, ec.LME_7143).WithFields(log.Fields{"query": qName, "key": k, "key_list_set": keyListSet, "labels_map": labelsMap}).Error("LokiService : Json processing : Can not find an index for the key in keyListSet, which is completely unexpected")
			}
			rowTemplate[index] = v
		}
		for _, v := range result.Values {
			if len(v) < 2 {
				continue
			}
			row := make([]string, rowlen)
			copy(row, rowTemplate)
			row[0] = v[1]
			records = append(records, row)
		}
	}

	log.WithFields(log.Fields{"query": qName, "records_count": len(records), "records": records}).Debug("LokiService : Json processing : Records are calculated")

	return records, "", nil
}
