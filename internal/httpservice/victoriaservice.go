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
	log.WithFields(log.Fields{"url": victoriaService.url, "export_config": exportConfig.GetSafeCopy()}).Info("VictoriaService : Initialization completed")
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
		log.WithField("url", v.url).Debug("VictoriaService : A response with a nil body is received")
	} else {
		if resp.Body != nil {
			defer func() {
				if err := resp.Body.Close(); err != nil {
					log.WithField("error", err).Error("VictoriaService : Error closing response body")
				}
			}()
		}
	}
	log.WithFields(log.Fields{"url": v.url, "query": queryName, "status": resp.Status}).Info("VictoriaService : Response received")
	if resp.StatusCode >= 400 {
		return ec.LME_7111, fmt.Errorf("VictoriaService : From %v response status code %v is received", v.url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_7113).WithFields(log.Fields{"url": v.url, "error": err}).Error("VictoriaService : Cannot read the victoria response body")
	}
	result := string(body)
	log.WithFields(log.Fields{"url": v.url, "length": len(result)}).Debug("VictoriaService : Body received")
	log.WithFields(log.Fields{"url": v.url, "body": result}).Trace("VictoriaService : Response body received")
	return "", nil
}
