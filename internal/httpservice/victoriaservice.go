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
	"fmt"
	"io"
	"log_exporter/internal/config"
	ec "log_exporter/internal/utils/errorcodes"
	"net"
	"net/http"

	log "github.com/sirupsen/logrus"
)

type VictoriaService struct {
	exportConfig *config.ExportConfig
	url          string
}

func NewVictoriaService(exportConfig *config.ExportConfig) *VictoriaService {
	victoriaService := VictoriaService{}
	victoriaService.exportConfig = exportConfig
	victoriaService.url = exportConfig.Host + exportConfig.Endpoint
	log.Infof("VictoriaService : Initialization completed : url = %v, exportConfig = %+v", victoriaService.url, exportConfig.GetSafeCopy())
	return &victoriaService
}

func (v *VictoriaService) PushBuffer(buffer *bytes.Buffer, queryName string) (string, error) {
	var transport http.RoundTripper = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: v.exportConfig.ConnectionTimeout,
		}).DialContext,
		TLSClientConfig: v.exportConfig.TlsConfig,
	}
	client := http.Client{
		Transport: transport,
		Timeout:   v.exportConfig.ConnectionTimeout,
	}

	req, err := http.NewRequest("POST", v.url, buffer)
	if err != nil {
		return ec.LME_7110, fmt.Errorf("VictoriaService : Error creating POST request to Victoria : %+v", err)
	}
	if v.exportConfig.User != "" {
		req.SetBasicAuth(v.exportConfig.User, v.exportConfig.Password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ec.LME_7110, fmt.Errorf("VictoriaService : Error accessing %v : %+v", v.url, err)
	}

	if resp == nil {
		return ec.LME_7110, fmt.Errorf("VictoriaService : From %v nil response is received", v.url)
	} else if resp.Body == nil {
		log.Debugf("VictoriaService : From %v response with nil body is received", v.url)
	} else {
		if resp.Body != nil {
			defer func() {
				if err := resp.Body.Close(); err != nil {
					log.Errorf("VictoriaService : Error closing response body : %+v", err)
				}
			}()
		}
	}
	log.Infof("VictoriaService : From %v for query %v response received : %v", v.url, queryName, resp.Status)
	if resp.StatusCode >= 400 {
		return ec.LME_7111, fmt.Errorf("VictoriaService : From %v response status code %v is received", v.url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_7113).Errorf("VictoriaService : From %v : Error reading victoria response body : %+v", v.url, err)
	}
	result := string(body)
	log.Debugf("VictoriaService : From %v received body with length : %v", v.url, len(result))
	log.Tracef("VictoriaService : From %v received response body : %v", v.url, result)
	return "", nil
}
