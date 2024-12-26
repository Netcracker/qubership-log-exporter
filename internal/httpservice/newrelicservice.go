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
    "log_exporter/internal/config"
    ec "log_exporter/internal/utils/errorcodes"
    "log_exporter/internal/utils"
    "fmt"
    "time"
    "bytes"
    log "github.com/sirupsen/logrus"
    "text/template"
    "encoding/json"
    "io"
    "net"
    "net/http"
    "net/url"
    "strings"
    "reflect"
)

type NewRelicService struct {
    appConfig *config.Config
    dsConfig *config.DatasourceConfig
}

type NRResponse struct {
    Results *[]NRResult
    Facets []Facet
    Metadata NRMetadata
}

type NRResult struct {
    Events *[]map[string]interface{}
    UniqueCount *float64
}

type NRMetadata struct {
    Facet *interface{}
}

type Facet struct {
    Name interface{}
    Results []FacetResult
}

type FacetResult struct {
    Count float64
}

const RESULT_FIELD_NAME string = "_RESULT_"

func CreateNewRelicService(appConfig *config.Config) (*NewRelicService) {
    g := NewRelicService{}
    g.appConfig = appConfig
    g.dsConfig = appConfig.Datasources[appConfig.DsName]
    return &g
}

func (g *NewRelicService) Query(qName string, startTime time.Time, endTime time.Time) ([][]string, error, string) {
    now := time.Now()
    var err error
    defer func() {
        log.Debugf("NewRelicService : For query %v request executed and json processed in %+v", qName, time.Since(now))
        if err != nil {
            selfMonitorIncErrorCodeCount(qName, now)
        } else {
            selfMonitorRefreshErrorCodeCount(qName, now)
        }
    }()

    stringResult, err, errc := g.queryNewRelic(qName, startTime, endTime)
    selfMonitorObserveQueryLatency(float64(time.Since(now)) / float64(time.Second), qName, now)
    selfMonitorObserveQueryResponseSize(float64(len(stringResult)), qName, now)
    if err != nil {
        return make([][]string, 0), err, errc
    }

    result := g.processJson(stringResult, qName)
    return result, nil, ""
}

func (g *NewRelicService) queryNewRelic(qName string, startTime time.Time, endTime time.Time) (string, error, string) {
    queryString, err := g.getQueryString(qName, startTime, endTime)
    if err != nil {
        return "", fmt.Errorf("NewRelicService : For query %v error creating queryString : %+v", qName, err), ec.LME_8102
    }
    log.Debugf("NewRelicService : For query %v queryString is %v", qName, queryString)

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
    req.Header.Add("Content-Type", "application/json")
    req.Header.Add("X-Query-Key", g.dsConfig.Password)
    log.Debugf("NewRelicService: For query %v request to NewRelic is %+v", qName, req)
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("NewRelicService : For query %v error acessing %v : %+v", qName, newRelicEndpoint, err), ec.LME_7140
    }

    defer resp.Body.Close()
    log.Debugf("NewRelicService : For query %v received response : %+v", qName, resp)
    body, err := io.ReadAll(resp.Body)
    if err != nil {
       return "", fmt.Errorf("NewRelicService : For query %v to %v error reading body : %+v", qName, newRelicEndpoint, err), ec.LME_7140
    }
    result := string(body)
    log.Infof("NewRelicService : For query %v to %v response status is %v, body length is %v", qName, newRelicEndpoint, resp.Status, len(result))
    if resp.StatusCode != 200 {
        log.WithField(ec.FIELD, ec.LME_7142).Errorf("NewRelicService : For query %v received response with status code %v from graylog, response body (limited) : %v", qName, resp.StatusCode, utils.GetLimitedPrefix(result, 10000))
        if resp.StatusCode >= 400 {
            return "", fmt.Errorf("NewRelicService : For query %v status code is %v", qName, resp.StatusCode), ec.LME_7141
        }
    }
    log.Debugf("NewRelicService : For query %v received response body : %v", qName, result)

    return result, nil, ""
}

func (g *NewRelicService) processJson(stringData string, qName string) [][]string {
    log.Debugf("NewRelicService : Json processing : For query %v nrResponse = %v", qName, stringData)
    var nrResponse NRResponse
    err := json.Unmarshal([]byte(stringData), &nrResponse)
    if err != nil {
        log.WithField(ec.FIELD, ec.LME_7143).Errorf("NewRelicService : Unmarshalling error for query %v : %+v", qName, err)
        return nil
    }

    if nrResponse.Metadata.Facet != nil {
        return g.processFacets(nrResponse, qName)
    } else if nrResponse.Results != nil {
        if len(*nrResponse.Results) == 0 {
            log.Warnf("NewRelicService : Json processing : For query %v, results list is empty", qName)
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

    log.WithField(ec.FIELD, ec.LME_7144).Errorf("NewRelicService : Json processing : For query %v got Unknown JSON output case, processing will be skipped", qName)

    return nil
}

func (g *NewRelicService) processEvents(events []map[string]interface{}, qName string) [][]string {
    log.Debugf("NewRelicService : Json processing : For query %v processEvents called", qName)
    keyList := make([]string, 0)
    keyListSet := make(map[string]int)
    for _, event := range events {
        for k := range event {
            if _,ok := keyListSet[k];!ok {
                keyListSet[k] = len(keyList)
                keyList = append(keyList, k)
            }
        }
    }
    log.Debugf("NewRelicService : Json processing : For query %v column names were found : %+v", qName, keyList)
    log.Debugf("NewRelicService : Json processing : For query %v keyListSet : %+v", qName, keyListSet)
    records := make([][]string, len(events) + 1)
    records[0] = keyList
    rowlen := len(keyList)
    for i, event := range events {
        row := make([]string, rowlen)
        for k,v := range event {
            index, ok := keyListSet[k]
            if !ok {
                log.WithField(ec.FIELD, ec.LME_7143).Errorf("NewRelicService : Json processing : For query %v can not find index for key %v in keyListSet %+v for event %+v, which is completely unexpected!", qName, k, keyListSet, event)
            }
            row[index] = fmt.Sprintf("%v", v)
        }
        records[i+1] = row
    }

    log.Debugf("NewRelicService : Json processing : For query %v the following records were calculated : %+v", qName, records)
    return records
}

func (g *NewRelicService) processFacets(nrResponse NRResponse, qName string) [][]string {
    log.Debugf("NewRelicService : Facets processing : For query %v processFacets called", qName)
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
        log.WithField(ec.FIELD, ec.LME_7144).Errorf("NewRelicService : Facets processing : For query %v got unknown type for Metadata.Facet in JSON : %+v", qName, reflect.TypeOf(facet))
    }
    log.Debugf("NewRelicService : Facets processing : For query %v got labelNames : %+v", qName, labelNames)

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
            log.WithField(ec.FIELD, ec.LME_7144).Errorf("NewRelicService : Facets processing : For query %v got unknown type for Facets.Name in JSON : %+v", qName, reflect.TypeOf(facetItem.Name))
        }
        if len(facetItem.Results) != 1 {
            if len(facetItem.Results) == 0 {
                log.Warnf("NewRelicService : Facets processing : For query %v len(facetItem.Results) = 0", qName)
                continue
            }
            log.Debugf("NewRelicService : Facets processing : For query %v len(facetItem.Results) = %v", qName, len(facetItem.Results))
        }
        row = append(row, fmt.Sprintf("%v", facetItem.Results[0].Count))
        if len(row) != columnNumber {
            log.Warnf("NewRelicService : Facets processing : For query %v len(row) = %v; columnNumber = %v", qName, len(row), columnNumber)
        }
        records = append(records, row)
    }
    log.Debugf("NewRelicService : Facets processing : For query %v the following records were calculated : %+v", qName, records)

    return records
}

func (g *NewRelicService) processUniqueCounts(uniqueCount float64, qName string) [][]string {
    log.Debugf("NewRelicService : UniqueCount processing : For query %v processUniqueCounts called", qName)
    records := make([][]string, 0)
    records = append(records, []string{RESULT_FIELD_NAME})
    records = append(records, []string{fmt.Sprintf("%v", uniqueCount)})
    log.Debugf("NewRelicService : UniqueCount processing : For query %v the following records were calculated : %+v", qName, records)
    return records
}

func (g *NewRelicService) getQueryString(qName string, startTime time.Time, endTime time.Time) (string, error) {
    qCfg := g.appConfig.Queries[qName]
    templateCtx := make(map[string]string)
    templateCtx["StartTime"] = startTime.Format("2006-01-02 15:04:05 MST")
    templateCtx["EndTime"] = endTime.Format("2006-01-02 15:04:05 MST")
    tmpl, err := template.New("query_template").Parse(qCfg.QueryString)
    if err != nil {
        return "", fmt.Errorf("Error creating template for query %v : %+v", qName, err)
    }
    buf := new(bytes.Buffer)
    err = tmpl.Execute(buf, templateCtx)
    if err != nil {
        return "", fmt.Errorf("Error executing template for query %v : %+v", qName, err)
    }
    return buf.String(), nil
}