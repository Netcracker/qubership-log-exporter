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
	"reflect"
	"strconv"
	"time"

	"github.com/PaesslerAG/jsonpath"
	log "github.com/sirupsen/logrus"
)

type LastTimestampService struct {
	LastTimestampHost *config.LastTimestampHostConfig
}

func NewLastTimestampService(lastTimestampHost *config.LastTimestampHostConfig) *LastTimestampService {
	return &LastTimestampService{
		LastTimestampHost: lastTimestampHost,
	}
}

func (l *LastTimestampService) GetLastTimestampUnixTime(qName string, queryConfig *config.QueryConfig) (int64, bool, string, error) {
	if l.LastTimestampHost == nil {
		return time.Now().Unix(), false, "", fmt.Errorf("LastTimestampService : Can not evaluate last timestamp, because LastTimestampHost is nil")
	}

	var lastTimestampURL string
	if queryConfig != nil && queryConfig.LastTimestampEndpoint != "" {
		lastTimestampURL = l.LastTimestampHost.Host + queryConfig.LastTimestampEndpoint
	} else {
		lastTimestampURL = l.LastTimestampHost.Host + l.LastTimestampHost.Endpoint
	}
	if lastTimestampURL == "" {
		return time.Now().Unix(), false, ec.LME_8102, fmt.Errorf("LastTimestampService : Can not evaluate last timestamp, because URL and endpoint are not defined, please check query config or last-timestamp-host config")
	}

	var jsonPath string
	if queryConfig != nil && queryConfig.LastTimestampJsonPath != "" {
		jsonPath = queryConfig.LastTimestampJsonPath
	} else {
		jsonPath = l.LastTimestampHost.JsonPath
	}

	if jsonPath == "" {
		return time.Now().Unix(), false, ec.LME_8102, fmt.Errorf("LastTimestampService : Can not evaluate last timestamp, jsonPath is not defined, please check query config or last-timestamp-host config")
	}
	log.Infof("LastTimestampService : For query %v lastTimestampURL : %v , jsonPath : %v ", qName, lastTimestampURL, jsonPath)

	var transport http.RoundTripper = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: l.LastTimestampHost.ConnectionTimeout,
		}).DialContext,
		TLSClientConfig: l.LastTimestampHost.TlsConfig,
	}
	client := http.Client{
		Transport: transport,
		Timeout:   l.LastTimestampHost.ConnectionTimeout,
	}

	req, err := http.NewRequest("GET", lastTimestampURL, nil)
	if err != nil {
		return time.Now().Unix(), true, ec.LME_7130, fmt.Errorf("LastTimestampService : Can not create HTTP request for %v : %+v", lastTimestampURL, err)
	}
	if l.LastTimestampHost.User != "" {
		req.SetBasicAuth(l.LastTimestampHost.User, l.LastTimestampHost.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return time.Now().Unix(), true, ec.LME_7130, fmt.Errorf("LastTimestampService : Last timestamp : Can not evaluate last timestamp, error accessing %v : %+v", lastTimestampURL, err)
	}

	if resp.Body != nil {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Errorf("LastTimestampService : Error closing response body : %+v", err)
			}
		}()
	}

	log.Infof("LastTimestampService : For query %v TSDB Response : %+v", qName, resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return time.Now().Unix(), true, ec.LME_7130, fmt.Errorf("LastTimestampService : Can not evaluate last timestamp, error reading response body : %+v", err)
	}
	data := string(body)
	log.Infof("LastTimestampService : For query %v Response body length : %v", qName, len(data))
	log.Infof("LastTimestampService : For query %v Response body : %v", qName, data)
	if resp.StatusCode >= 400 {
		return time.Now().Unix(), true, ec.LME_7131, fmt.Errorf("LastTimestampService : Can not evaluate last timestamp, error status code received : %v", resp.StatusCode)
	}
	var jsonData interface{}
	if err := json.Unmarshal([]byte(data), &jsonData); err != nil {
		return time.Now().Unix(), true, ec.LME_7133, fmt.Errorf("LastTimestampService : Can not evaluate last timestamp, error unmarshaling JSON response: %+v", err)
	}
	result, err := jsonpath.Get(jsonPath, jsonData)

	log.Infof("LastTimestampService : For query %v for jsonpath %v result = %+v, type = %v", qName, jsonPath, result, reflect.TypeOf(result))

	if err != nil {
		return time.Now().Unix(), true, ec.LME_7133, fmt.Errorf("LastTimestampService : Can not evaluate last timestamp, error during JsonPathLookup : %+v", err)
	}

	var resultFloat float64
	switch res := result.(type) {
	case string:
		if res == "" {
			return time.Now().Unix(), false, ec.LME_7134, fmt.Errorf("LastTimestampService : There is no data (empty string) on the provided jsonpath in TSDB response. Probably metric for the last timestamp evaluation for the query %v is configured for the first time on the environment, if it is the case, ignore this message", qName)
		}
		log.Infof("LastTimestampService : For query %v got type string res = %+v", qName, res)
		resultFloat, err = strconv.ParseFloat(res, 64)
	case []interface{}:
		if len(res) == 0 {
			return time.Now().Unix(), false, ec.LME_7134, fmt.Errorf("LastTimestampService : There is no data (empty slice) on the provided jsonpath in TSDB response. Probably metric for the last timestamp evaluation for the query %v is configured for the first time on the environment, if it is the case, ignore this message", qName)
		}
		log.Infof("LastTimestampService : For query %v got type []interface{} res = %+v", qName, res)
		resultFloat, err = utils.MaxFloat64InSlice(res)
	default:
		err = fmt.Errorf("unknown input type: %+v", reflect.TypeOf(result))
	}

	if err != nil {
		return time.Now().Unix(), true, ec.LME_7134, fmt.Errorf("LastTimestampService : Can not evaluate last timestamp, error parsing to float64 (or []float64) jsonpath extraction %+v from response %+v received from TSDB : %+v", result, data, err)
	}
	return int64(resultFloat), false, "", nil
}
