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
	"runtime"
	"strings"
	"testing"
	"time"

	ec "log_exporter/internal/utils/errorcodes"

	log "github.com/sirupsen/logrus"
)

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
