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
    "log_exporter/internal/selfmonitor"
    "log_exporter/internal/utils"
    ec "log_exporter/internal/utils/errorcodes"
    "fmt"
    "time"
    "bytes"
    "net/http"
    "net"
    "io"
    "strings"
    "encoding/csv"
)

var (
    graylogJsonTemplate = `{
  %v
  "query_string": {
    "type": "elasticsearch",
    "query_string": %v
  },
  "timerange" : {
    "type" : "absolute",
    "from" : "%v",
    "to" : "%v"
  },
  "fields_in_order": %v
}`
)

type GraylogService struct {
    appConfig *config.Config
    dsConfig *config.DatasourceConfig
}

func CreateGraylogService(appConfig *config.Config) (*GraylogService) {
    g := GraylogService{}
    g.appConfig = appConfig
    g.dsConfig = appConfig.Datasources[appConfig.DsName]
    return &g
}

func (g *GraylogService) Query(qName string, startTime time.Time, endTime time.Time) ([][]string, error, string) {
    now := time.Now()
    var err error
    defer func() {
        log.Debugf("GraylogService : For query %v request executed and csv processed in %+v", qName, time.Since(now))
        if err != nil {
            selfMonitorIncErrorCodeCount(qName, now)
        } else {
            selfMonitorRefreshErrorCodeCount(qName, now)
        }
    }()

    stringResult, err, errc := g.queryGraylog(qName, startTime, endTime)
    selfMonitorObserveQueryLatency(float64(time.Since(now)) / float64(time.Second), qName, now)
    selfMonitorObserveQueryResponseSize(float64(len(stringResult)), qName, now)
    if err != nil {
        return make([][]string, 0), err, errc
    }

    result, err, errc := ProcessCsv(stringResult, qName)
    return result, err, errc
}

func (g *GraylogService) queryGraylog(qName string, startTime time.Time, endTime time.Time) (string, error, string) {
    qCfg := g.appConfig.Queries[qName]
    startTimeStr := startTime.Format("2006-01-02T15:04:05Z07:00")
    endTimeStr := endTime.Format("2006-01-02T15:04:05Z07:00")
    requestBody := fmt.Sprintf(graylogJsonTemplate, qCfg.StreamsJson, qCfg.QueryStringJson, startTimeStr, endTimeStr, qCfg.FieldsInOrderJson)
    log.Debugf("GraylogService : For query %v requestBody is %v", qName, requestBody)

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

    graylogEndpoint := strings.Trim(g.dsConfig.Host, " /") + "/api/views/search/messages"
    req, err := http.NewRequest("POST", graylogEndpoint, bytes.NewBufferString(requestBody))
    if g.dsConfig.User != "" {
        req.SetBasicAuth(g.dsConfig.User, g.dsConfig.Password)
    }
    req.Header.Add("X-Requested-By", "*") 
    req.Header.Add("Content-Type", "application/json") 
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("GraylogService : For query %v error acessing %v : %+v", qName, graylogEndpoint, err), ec.LME_7100
    }

    defer resp.Body.Close()
    log.Debugf("GraylogService : For query %v received response : %+v", qName, resp)
    body, err := io.ReadAll(resp.Body)
    if err != nil {
       return "", fmt.Errorf("GraylogService : For query %v to %v error reading body : %+v", qName, graylogEndpoint, err), ec.LME_7100
    }
    result := string(body)
    log.Infof("GraylogService : For query %v to %v response status is %v, body length is %v", qName, graylogEndpoint, resp.Status, len(result))
    if resp.StatusCode != 200 {
        log.WithField(ec.FIELD, ec.LME_7102).Errorf("GraylogService : For query %v received response with status code %v from graylog, response body (limited) : %v", qName, resp.StatusCode, utils.GetLimitedPrefix(result, 10000))
        if resp.StatusCode >= 400 {
            return "", fmt.Errorf("GraylogService : For query %v status code is %v", qName, resp.StatusCode), ec.LME_7101
        }
    }
    log.Tracef("GraylogService : For query %v received response body : %v", qName, result)

    return result, nil, ""
}

func ProcessCsv(stringData string, qName string) ([][]string, error, string) {
    r := csv.NewReader(strings.NewReader(stringData))

    records, err := r.ReadAll()
    if err != nil {
        resError := fmt.Errorf("GraylogService : For query %v got error reading csv : %+v", qName, err)
        return make([][]string, 0), resError, ec.LME_7103
    }
    log.Tracef("GraylogService : For query %v got records : %v", qName, records)
    return records, nil, ""
}

func selfMonitorObserveQueryResponseSize(value float64, qName string, timestamp time.Time) {
    labels := make(map[string]string)
    labels["query_name"] = qName
    selfmonitor.ObserveQueryResponseSize(labels, value, &timestamp)
}

func selfMonitorIncErrorCodeCount(qName string, timestamp time.Time) {
    labels := make(map[string]string)
    labels["query_name"] = qName
    selfmonitor.IncGraylogResponseErrorCount(labels, &timestamp)
}

func selfMonitorRefreshErrorCodeCount(qName string, timestamp time.Time) {
    labels := make(map[string]string)
    labels["query_name"] = qName
    selfmonitor.RefreshGraylogResponseErrorCount(labels, &timestamp)
}

func selfMonitorObserveQueryLatency(value float64, qName string, timestamp time.Time) {
    labels := make(map[string]string)
    labels["query_name"] = qName
    selfmonitor.ObserveQueryLatency(labels, value, &timestamp)
}