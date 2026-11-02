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

package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"

	ec "log_exporter/internal/utils/errorcodes"

	log "github.com/sirupsen/logrus"
)

func TestJSONFormatterRetainsNonFiniteMetricDiagnostics(t *testing.T) {
	formatter := &utcFormatter{&log.JSONFormatter{}}
	entry := &log.Entry{
		Time: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Level: log.WarnLevel,
		Message: "metric value is not finite", Data: log.Fields{ec.FIELD: "LME_1020", "value": math.NaN(), "count": 2},
	}
	output, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format(NaN) = %v", err)
	}
	var record map[string]interface{}
	if err := json.Unmarshal(output, &record); err != nil {
		t.Fatal(err)
	}
	if got := record["value"]; got != "NaN" {
		t.Errorf("record[value] = %v, want NaN", got)
	}
	if got := record["count"]; got != float64(2) {
		t.Errorf("record[count] = %v, want 2", got)
	}
	if !math.IsNaN(entry.Data["value"].(float64)) {
		t.Errorf("Format mutated entry.Data: %v", entry.Data)
	}
}

func TestJSONFormatterPreservesFieldsOnOnePhysicalLine(t *testing.T) {
	previousFormat := *logFormat
	previousFormatter := log.StandardLogger().Formatter
	previousCaller := log.StandardLogger().ReportCaller
	t.Cleanup(func() {
		*logFormat = previousFormat
		log.SetFormatter(previousFormatter)
		log.SetReportCaller(previousCaller)
	})
	*logFormat = JSON
	setFormatter()
	entry := &log.Entry{
		Logger:  log.StandardLogger(),
		Time:    time.Date(2026, 1, 2, 3, 4, 5, 123000000, time.FixedZone("offset", 3*60*60)),
		Level:   log.ErrorLevel,
		Message: "quoted \"message\"\nsecond line\r\nUnicode: \u041f\u0440\u0438\u0432\u0435\u0442",
		Data:    log.Fields{ec.FIELD: "LME_8101", "error": errors.New("bad \"input\"\nnext"), "count": 7},
		Caller:  &runtime.Frame{File: "/src/main.go", Function: "main.main"},
	}
	output, err := log.StandardLogger().Formatter.Format(entry)
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(output, []byte{'\n'}); got != 1 {
		t.Errorf("physical newline count = %d, want 1: %q", got, output)
	}
	var record map[string]interface{}
	if err := json.Unmarshal(output, &record); err != nil {
		t.Fatalf("decode record: %v; output=%q", err, output)
	}
	for key, want := range map[string]interface{}{
		"time": "2026-01-02T00:04:05.123Z", "level": "error", "message": entry.Message,
		"error": "bad \"input\"\nnext", "count": float64(7), ec.FIELD: "LME_8101", "class": "main.go",
	} {
		if got := record[key]; got != want {
			t.Errorf("record[%q] = %v, want %v", key, got, want)
		}
	}
	if _, exists := record["func"]; exists {
		t.Errorf("record includes function: %v", record)
	}
}

func TestCloudFormatterEscapesMessageAndFieldLineBreaks(t *testing.T) {
	entry := &log.Entry{
		Time: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Level: log.WarnLevel,
		Message: "first\r\nsecond", Data: log.Fields{"z": "last\rline", ec.FIELD: "LME_8101", "a": "first\nline"},
	}
	output, err := (&CloudFormatter{}).Format(entry)
	if err != nil {
		t.Fatal(err)
	}
	want := "[2026-01-02T00:00:00.000] [warning] [x_request_id=0] [tenant_id=-] [thread=-] [class=] [error_code=LME_8101] [a=first\\nline] [z=last\\rline] first\\r\\nsecond\n"
	if string(output) != want {
		t.Errorf("Format() = %q, want %q", output, want)
	}
}

func TestTextFormatterCanReplaceJSONFormatter(t *testing.T) {
	previousFormat := *logFormat
	previousFormatter := log.StandardLogger().Formatter
	previousCaller := log.StandardLogger().ReportCaller
	t.Cleanup(func() {
		*logFormat = previousFormat
		log.SetFormatter(previousFormatter)
		log.SetReportCaller(previousCaller)
	})
	*logFormat = JSON
	setFormatter()
	*logFormat = TEXT
	setFormatter()
	if _, ok := log.StandardLogger().Formatter.(*log.TextFormatter); !ok {
		t.Errorf("formatter = %T, want *log.TextFormatter", log.StandardLogger().Formatter)
	}
	if log.StandardLogger().ReportCaller {
		t.Error("text formatter reports caller, want disabled")
	}
}

func TestLogLevelFlag_Set(t *testing.T) {
	var level logLevelFlag

	// Test valid level
	err := level.Set("info")
	if err != nil {
		t.Errorf("Expected no error for valid level, got: %v", err)
	}
	if log.Level(level) != log.InfoLevel {
		t.Errorf("Expected InfoLevel, got: %v", log.Level(level))
	}

	// Test invalid level
	err = level.Set("invalid")
	if err == nil {
		t.Error("Expected error for invalid level")
	}
}

func TestLogLevelFlag_String(t *testing.T) {
	level := logLevelFlag(log.WarnLevel)
	expected := "warning"
	if level.String() != expected {
		t.Errorf("Expected '%s', got: '%s'", expected, level.String())
	}
}

func TestCloudFormatter_Format(t *testing.T) {
	formatter := &CloudFormatter{}

	entry := &log.Entry{
		Message: "test message",
		Level:   log.InfoLevel,
		Time:    time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		Data:    log.Fields{ec.FIELD: "LME_1234"},
		Caller: &runtime.Frame{
			File: "/path/to/file.go",
		},
	}

	formatted, err := formatter.Format(entry)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	formattedStr := string(formatted)

	// Check basic structure
	if !strings.Contains(formattedStr, "[2023-01-01T12:00:00.000]") {
		t.Error("Expected timestamp in formatted output")
	}
	if !strings.Contains(formattedStr, "[info]") {
		t.Error("Expected log level in formatted output")
	}
	if !strings.Contains(formattedStr, "[error_code=LME_1234]") {
		t.Error("Expected error code in formatted output")
	}
	if !strings.Contains(formattedStr, "test message") {
		t.Error("Expected message in formatted output")
	}
	if !strings.Contains(formattedStr, "[class=file.go]") {
		t.Error("Expected class name in formatted output")
	}
}

func TestCloudFormatter_Format_NoCaller(t *testing.T) {
	formatter := &CloudFormatter{}

	entry := &log.Entry{
		Message: "test message",
		Level:   log.InfoLevel,
		Time:    time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		Data:    log.Fields{},
		Caller:  nil,
	}

	formatted, err := formatter.Format(entry)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	formattedStr := string(formatted)

	// Should not contain error code
	if strings.Contains(formattedStr, "[error_code=") {
		t.Error("Should not contain error code when not present")
	}
}

func TestCreateDirForFile(t *testing.T) {
	// Test with empty path
	createDirForFile("")

	// Test with relative path
	createDirForFile("test.log")

	// Test with path containing directory
	createDirForFile("logs/test.log")

	// These should not panic
	t.Log("createDirForFile tests completed")
}
