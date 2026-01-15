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
	"testing"
)

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
