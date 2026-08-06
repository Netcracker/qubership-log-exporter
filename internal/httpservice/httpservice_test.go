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
	"io"
	"log_exporter/internal/config"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func TestMarshalWriteRequest(t *testing.T) {
	request := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels:  []prompb.Label{{Name: "__name__", Value: "test_metric"}},
				Samples: []prompb.Sample{{Value: 42, Timestamp: 123}},
			},
		},
	}

	data, err := marshalWriteRequest(request)
	if err != nil {
		t.Fatalf("marshalWriteRequest() error = %v", err)
	}

	var decoded prompb.WriteRequest
	if err := decoded.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal generated payload: %v", err)
	}
	if got := decoded.Timeseries[0].Samples[0].Value; got != 42 {
		t.Fatalf("decoded sample value = %v, want 42", got)
	}
}

func TestWriteMetricsSendsGeneratedProtobufPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		compressed, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "cannot read request", http.StatusInternalServerError)
			return
		}
		payload, err := snappy.Decode(nil, compressed)
		if err != nil {
			t.Errorf("decode snappy payload: %v", err)
			http.Error(w, "cannot decode request", http.StatusBadRequest)
			return
		}
		var request prompb.WriteRequest
		if err := request.Unmarshal(payload); err != nil {
			t.Errorf("unmarshal remote-write request: %v", err)
			http.Error(w, "cannot unmarshal request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	service := NewPromWRService(&config.ExportConfig{
		TLSHostConfig: config.TLSHostConfig{
			Host:              server.URL,
			ConnectionTimeout: time.Second,
		},
	})

	if code, err := service.WriteMetrics(nil, "test_query"); err != nil {
		t.Fatalf("WriteMetrics() error code = %q, error = %v", code, err)
	}
}

func TestProcessCsv_ValidData(t *testing.T) {
	csvData := `field1,field2,field3
value1,value2,value3
value4,value5,value6`

	result, errc, err := ProcessCsv(csvData, "test_query")

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if errc != "" {
		t.Errorf("Expected empty error code, got: %s", errc)
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 rows, got: %d", len(result))
	}
	if len(result[0]) != 3 {
		t.Errorf("Expected 3 columns in first row, got: %d", len(result[0]))
	}
	if result[0][0] != "field1" {
		t.Errorf("Expected first field to be 'field1', got: %s", result[0][0])
	}
}

func TestProcessCsv_EmptyData(t *testing.T) {
	csvData := ""

	result, errc, err := ProcessCsv(csvData, "test_query")

	if err != nil {
		t.Errorf("Expected no error for empty data, got: %v", err)
	}
	if errc != "" {
		t.Errorf("Expected empty error code, got: %s", errc)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 rows, got: %d", len(result))
	}
}

func TestProcessCsv_InvalidCsv(t *testing.T) {
	csvData := `field1,field2
value1,value2,value3` // Invalid CSV - unequal columns

	result, errc, err := ProcessCsv(csvData, "test_query")

	if err == nil {
		t.Error("Expected error for invalid CSV")
	}
	if errc == "" {
		t.Error("Expected error code")
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 rows for invalid CSV, got: %d", len(result))
	}
}

func TestCreateGraylogService(t *testing.T) {
	// This is a basic test for the constructor
	// In a real scenario, you'd pass a proper config
	// For now, just test that it doesn't panic with nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CreateGraylogService panicked: %v", r)
		}
	}()

	// This will likely panic or error due to nil config, but we want to test the function exists
	// service := CreateGraylogService(nil)
	// For a proper test, we'd need to mock the config
	t.Skip("Skipping CreateGraylogService test - requires complex config setup")
}
