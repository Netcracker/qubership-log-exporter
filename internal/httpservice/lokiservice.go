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
    log "github.com/sirupsen/logrus"
    "log_exporter/internal/config"
    "log_exporter/internal/utils"
    ec "log_exporter/internal/utils/errorcodes"
    "fmt"
    "time"
    "net/http"
    "net"
    "io"
    "strings"
    "encoding/json"
)

type LokiService struct {
    appConfig *config.Config
    dsConfig *config.DatasourceConfig
}

type LokiResponse struct {
    Status string
    Data LokiResponseData
}

type LokiResponseData struct {
    ResultType string
    Result []LokiResponseDataResult
}

type LokiResponseDataResult struct {
    Stream map[string]string
    Values [][]string
}


func CreateLokiService(appConfig *config.Config) (*LokiService) {
    g := LokiService{}
    g.appConfig = appConfig
    g.dsConfig = appConfig.Datasources[appConfig.DsName]
    return &g
}

func (g *LokiService) Query(qName string, startTime time.Time, endTime time.Time) ([][]string, error, string) {
    now := time.Now()
    var err error
    defer func() {
        log.Debugf("LokiService : For query %v request executed and json is processed in %+v", qName, time.Since(now))
        if err != nil {
            selfMonitorIncErrorCodeCount(qName, now)
        } else {
            selfMonitorRefreshErrorCodeCount(qName, now)
        }
    }()

    stringResult, err, errc := g.queryLoki(qName, startTime, endTime)
    selfMonitorObserveQueryLatency(float64(time.Since(now)) / float64(time.Second), qName, now)
    selfMonitorObserveQueryResponseSize(float64(len(stringResult)), qName, now)
    if err != nil {
        return make([][]string, 0), err, errc
    }

    result, err, errc := g.processJson(stringResult, qName)
    return result, err, errc
}

func (g *LokiService) queryLoki(qName string, startTime time.Time, endTime time.Time) (string, error, string) {
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
    if g.dsConfig.User != "" {
        req.SetBasicAuth(g.dsConfig.User, g.dsConfig.Password)
    }
    q := req.URL.Query()
    q.Add("query", qCfg.QueryString)
    q.Add("limit", "5000")
    q.Add("start", startTime.Format(time.RFC3339))
    q.Add("end", endTime.Format(time.RFC3339))
    req.URL.RawQuery = q.Encode()

    log.Debugf("LokiService : For query %v request generated : %+v", qName, req.URL.String())

    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("LokiService : For query %v error acessing %v : %+v", qName, lokiEndpoint, err), ec.LME_7100
    }

    defer resp.Body.Close()
    log.Debugf("LokiService : For query %v received response : %+v", qName, resp)
    body, err := io.ReadAll(resp.Body)
    if err != nil {
       return "", fmt.Errorf("LokiService : For query %v to %v error reading body : %+v", qName, lokiEndpoint, err), ec.LME_7100
    }
    result := string(body)
    log.Infof("LokiService : For query %v to %v response status is %v, body length is %v", qName, lokiEndpoint, resp.Status, len(result))
    if resp.StatusCode != 200 {
        log.WithField(ec.FIELD, ec.LME_7102).Errorf("LokiService : For query %v received response with status code %v from loki, response body (limited) : %v", qName, resp.StatusCode, utils.GetLimitedPrefix(result, 10000))
        if resp.StatusCode >= 400 {
            return "", fmt.Errorf("LokiService : For query %v status code is %v", qName, resp.StatusCode), ec.LME_7101
        }
    }
    log.Debugf("LokiService : For query %v received response body : %v", qName, result)

    return result, nil, ""
}

func (g *LokiService) processJson(stringData string, qName string) ([][]string, error, string) {
    log.Debugf("LokiService : Json processing : For query %v lokiResponse = %v", qName, stringData)
    var lokiResponse LokiResponse
    err := json.Unmarshal([]byte(stringData), &lokiResponse)
    if err != nil {
        log.WithField(ec.FIELD, ec.LME_7143).Errorf("LokiService : Unmarshalling error for query %v : %+v", qName, err)
        return nil, fmt.Errorf("LokiService : Unmarshalling error for query %v : %+v", qName, err), ec.LME_7143
    }
    if len(lokiResponse.Data.Result) == 0 {
        return nil, nil, ""
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
    log.Debugf("LokiService : Json processing : For query %v keyList = %+v, keyListSet = %+v, totalLen = %v", qName, keyList, keyListSet, totalLen)

    records := make([][]string, 0, totalLen + 1)
    records = append(records, keyList)
    rowlen := len(keyList)

    for _, result := range lokiResponse.Data.Result {
        labelsMap := result.Stream
        rowTemplate := make([]string, rowlen)
        for k, v := range labelsMap {
            index, ok := keyListSet[k]
            if !ok {
                log.WithField(ec.FIELD, ec.LME_7143).Errorf("LokiService : Json processing : For query %v can not find index for key %v in keyListSet %+v for labelsMap %+v, which is completely unexpected!", qName, k, keyListSet, labelsMap)
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

    log.Debugf("LokiService : Json processing : For query %v result len = %v, result records = %+v", qName, len(records), records)

    return records, nil, ""
}

