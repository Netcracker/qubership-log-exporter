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

package config

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log_exporter/internal/logger"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestSafeExportConfigCanBeLoggedWithoutCredentials(t *testing.T) {
	cfg := &ExportConfig{
		TLSHostConfig:     TLSHostConfig{Host: "https://metrics.example", Password: "export-secret", DecryptedPassword: "export-plaintext", TlsConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
		LastTimestampHost: &LastTimestampHostConfig{TLSHostConfig: TLSHostConfig{Password: "timestamp-secret", DecryptedPassword: "timestamp-plaintext", TlsConfig: &tls.Config{MinVersion: tls.VersionTLS12}}},
	}
	previousFormat := flag.Lookup("log-format").Value.String()
	previousLevel := flag.Lookup("log-level").Value.String()
	previousOutput, previousFormatter, previousLogLevel := log.StandardLogger().Out, log.StandardLogger().Formatter, log.GetLevel()
	previousCaller := log.StandardLogger().ReportCaller
	t.Cleanup(func() {
		_ = flag.Set("log-format", previousFormat)
		_ = flag.Set("log-level", previousLevel)
		log.SetOutput(previousOutput)
		log.SetFormatter(previousFormatter)
		log.SetLevel(previousLogLevel)
		log.SetReportCaller(previousCaller)
	})
	_ = flag.Set("log-format", "json")
	_ = flag.Set("log-level", "info")
	logger.ConfigureLevelAndFormat()
	var buffer bytes.Buffer
	log.SetOutput(&buffer)
	log.WithField("export_config", cfg.GetSafeCopy()).Info("Export config read")
	output := buffer.Bytes()
	var record map[string]json.RawMessage
	if err := json.Unmarshal(output, &record); err != nil {
		t.Fatalf("safe export config log is not JSON: %v; output=%s", err, output)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(record["export_config"], &fields); err != nil {
		t.Fatalf("export_config is not a JSON object: %v; output=%s", err, output)
	}
	if fields["Host"] != cfg.Host || fields["Password"] != sensitiveDataReplacer || fields["TlsConfig"] != nil {
		t.Errorf("safe export config fields = %v, want the host, a masked password, and no TLS runtime state", fields)
	}
	for _, secret := range []string{"export-secret", "export-plaintext", "timestamp-secret", "timestamp-plaintext"} {
		if bytes.Contains(output, []byte(secret)) {
			t.Errorf("safe export config contains %q: %s", secret, output)
		}
	}
	if cfg.Password != "export-secret" || cfg.LastTimestampHost.Password != "timestamp-secret" || cfg.TlsConfig == nil || cfg.LastTimestampHost.TlsConfig == nil {
		t.Errorf("GetSafeCopy mutated the live configuration: %+v", cfg)
	}
}

func TestExampleConfigs(t *testing.T) {
	log.SetLevel(log.ErrorLevel)
	exampleConfigs := []string{
		"../../examples/config_cloud_promrw.yaml",
		"../../examples/config_cloud_victoria.yaml",
		"../../examples/config_emu.yaml",
		"../../examples/config_nr.yaml",
		"../../examples/unit_test.yaml",
	}
	for i, path := range exampleConfigs {
		t.Run("TestExampleConfig"+fmt.Sprintf("%v", i), func(t *testing.T) {
			testConfig, err := SimpleSilentRead(path)
			if err != nil {
				t.Errorf("Error parsing test config %v : %+v", path, err)
			}
			err = ValidateConfig(testConfig)
			if err != nil {
				t.Errorf("Error validating test config %v : %+v", path, err)
			}
		})
	}
}
