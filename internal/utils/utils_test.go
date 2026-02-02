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

package utils

import (
	"os"
	"testing"
)

func TestMapToString(t *testing.T) {
	m := map[string]string{
		"key2": "value2",
		"key1": "value1",
		"key3": "value3",
	}

	result := MapToString(m)

	// Should be sorted by key
	expected := "key1=\"value1\",key2=\"value2\",key3=\"value3\","
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestGetKeys(t *testing.T) {
	m := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	keys := GetKeys(m)

	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}

	// Check that all expected keys are present
	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}

	if !keyMap["key1"] || !keyMap["key2"] {
		t.Error("Expected keys key1 and key2")
	}
}

func TestGetOrderedMapValues(t *testing.T) {
	m := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	keys := []string{"key2", "key1", "key3"}
	values := GetOrderedMapValues(m, keys)

	expected := []string{"value2", "value1", "value3"}
	for i, val := range values {
		if val != expected[i] {
			t.Errorf("Expected %s at index %d, got %s", expected[i], i, val)
		}
	}
}

func TestFindStringIndexInArray(t *testing.T) {
	arr := []string{"apple", "banana", "cherry"}

	// Test found cases
	if FindStringIndexInArray(arr, "banana") != 1 {
		t.Error("Expected index 1 for 'banana'")
	}

	if FindStringIndexInArray(arr, "apple") != 0 {
		t.Error("Expected index 0 for 'apple'")
	}

	// Test not found case
	if FindStringIndexInArray(arr, "grape") != -1 {
		t.Error("Expected -1 for 'grape'")
	}
}

func TestGetAverage(t *testing.T) {
	// Test normal case
	arr := []float64{1.0, 2.0, 3.0, 4.0}
	avg := GetAverage(arr)
	if avg != 2.5 {
		t.Errorf("Expected 2.5, got %f", avg)
	}

	// Test empty array
	avg = GetAverage([]float64{})
	if avg != 0.0 {
		t.Errorf("Expected 0.0 for empty array, got %f", avg)
	}

	// Test single element
	avg = GetAverage([]float64{5.0})
	if avg != 5.0 {
		t.Errorf("Expected 5.0, got %f", avg)
	}
}

func TestIsID(t *testing.T) {
	// Test valid IDs
	if !IsID("abc123def", 3) {
		t.Error("Expected true for 'abc123def' with 3 digits")
	}

	if !IsID("test456", 3) {
		t.Error("Expected true for 'test456' with 3 digits")
	}

	// Test invalid IDs
	if IsID("abcdef", 3) {
		t.Error("Expected false for 'abcdef' with 3 digits (no digits)")
	}

	if IsID("ab12", 3) {
		t.Error("Expected false for 'ab12' with 3 digits (only 2 digits)")
	}
}

func TestIsIdFSM(t *testing.T) {
	// Test cases based on the FSM logic
	if !IsIdFSM("abc123def456", 10) {
		t.Error("Expected true for complex ID")
	}

	if IsIdFSM("simple", 20) {
		t.Error("Expected false for simple string with high limit")
	}
}

func TestIsUUID(t *testing.T) {
	// Test valid UUID
	validUUID := "12345678-1234-1234-1234-123456789012"
	if !isUUID(validUUID) {
		t.Error("Expected true for valid UUID format")
	}

	// Test invalid UUIDs
	if isUUID("not-a-uuid") {
		t.Error("Expected false for invalid UUID")
	}

	if isUUID("12345678-1234-1234-1234") {
		t.Error("Expected false for too short UUID")
	}
}

func TestIsNumber(t *testing.T) {
	// Test valid numbers
	if !isNumber("123") {
		t.Error("Expected true for '123'")
	}

	if !isNumber("-456") {
		t.Error("Expected true for '-456'")
	}

	if !isNumber("+789") {
		t.Error("Expected true for '+789'")
	}

	// Test invalid numbers
	if isNumber("abc") {
		t.Error("Expected false for 'abc'")
	}

	if isNumber("12a34") {
		t.Error("Expected false for '12a34'")
	}

	if isNumber("") {
		t.Error("Expected false for empty string")
	}
}

func TestGetLimitedPrefix(t *testing.T) {
	longString := "this is a very long string that should be truncated"

	// Test truncation
	result := GetLimitedPrefix(longString, 10)
	if result != "this is a " {
		t.Errorf("Expected 'this is a ', got '%s'", result)
	}

	// Test no truncation needed
	result = GetLimitedPrefix("short", 10)
	if result != "short" {
		t.Errorf("Expected 'short', got '%s'", result)
	}
}

func TestRemoveIDsFromURI(t *testing.T) {
	uri := "/api/v1/users/12345678-1234-1234-1234-123456789012/orders/456/items/789"

	result := RemoveIDsFromURI(uri, "_UUID_", "_NUMBER_", "_ID_", 3, "_FSM_", 10)

	// Should replace UUID and numbers (456 and 789 are both numbers)
	expected := "/api/v1/users/_UUID_/orders/_NUMBER_/items/_NUMBER_"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestMultiplyArrayItems(t *testing.T) {
	arr := []int64{2, 3, 4}
	result := MultiplyArrayItems(arr)
	if result != 24 {
		t.Errorf("Expected 24, got %d", result)
	}

	// Test empty array
	result = MultiplyArrayItems([]int64{})
	if result != 1 {
		t.Errorf("Expected 1 for empty array, got %d", result)
	}
}

func TestIncrementIndexes(t *testing.T) {
	indexes := []int64{0, 0, 0}
	sizes := []int64{2, 3, 2}

	// Test incrementing
	result := IncrementIndexes(indexes, sizes)
	if result {
		t.Error("Expected false for first increment")
	}
	if indexes[0] != 1 || indexes[1] != 0 || indexes[2] != 0 {
		t.Errorf("Expected [1,0,0], got %v", indexes)
	}

	// Test overflow
	indexes = []int64{1, 2, 1} // Last valid combination
	result = IncrementIndexes(indexes, sizes)
	if !result {
		t.Error("Expected true for overflow")
	}
	// Note: function doesn't reset indexes on overflow
}

func TestMaxFloat64InSlice(t *testing.T) {
	input := []interface{}{1.5, "2.7", 3.2, "invalid", 1.8}

	max, err := MaxFloat64InSlice(input)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if max != 3.2 {
		t.Errorf("Expected 3.2, got %f", max)
	}

	// Test with no valid floats
	_, err = MaxFloat64InSlice([]interface{}{"invalid1", "invalid2"})
	if err == nil {
		t.Error("Expected error for no valid floats")
	}
}

func TestGetOctalUintEnvironmentVariable(t *testing.T) {
	// Test with valid octal value
	if err := os.Setenv("TEST_VAR", "755"); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_VAR"); err != nil {
			t.Errorf("Failed to unset environment variable: %v", err)
		}
	}()

	result := GetOctalUintEnvironmentVariable("TEST_VAR", 644)
	if result != 493 { // 755 octal = 493 decimal
		t.Errorf("Expected 493, got %d", result)
	}

	// Test with invalid value (should use default)
	if err := os.Setenv("TEST_VAR", "invalid"); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	result = GetOctalUintEnvironmentVariable("TEST_VAR", 644)
	if result != 644 {
		t.Errorf("Expected default 644, got %d", result)
	}

	// Test with empty value (should use default)
	if err := os.Setenv("TEST_VAR", ""); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	result = GetOctalUintEnvironmentVariable("TEST_VAR", 755)
	if result != 755 {
		t.Errorf("Expected default 755, got %d", result)
	}
}
