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

package main

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	// Save original values
	originalVersion := Version
	originalBuildDate := BuildDate
	originalBranch := Branch
	originalRevision := Revision

	// Set test values
	Version = "1.2.3"
	BuildDate = "2023-01-01"
	Branch = "main"
	Revision = "abc123"

	defer func() {
		// Restore original values
		Version = originalVersion
		BuildDate = originalBuildDate
		Branch = originalBranch
		Revision = originalRevision
	}()

	result := versionString()

	expectedParts := []string{
		"log-exporter version: 1.2.3",
		"build date: 2023-01-01",
		"branch: main",
		"revision: abc123",
		"go version:",
		"platform:",
	}

	for _, part := range expectedParts {
		if !strings.Contains(result, part) {
			t.Errorf("Expected version string to contain '%s', got: %s", part, result)
		}
	}
}

func TestVersionString_EmptyValues(t *testing.T) {
	// Save original values
	originalVersion := Version
	originalBuildDate := BuildDate
	originalBranch := Branch
	originalRevision := Revision

	// Set empty values
	Version = ""
	BuildDate = ""
	Branch = ""
	Revision = ""

	defer func() {
		// Restore original values
		Version = originalVersion
		BuildDate = originalBuildDate
		Branch = originalBranch
		Revision = originalRevision
	}()

	result := versionString()

	// Should still contain the format
	if !strings.Contains(result, "log-exporter version:") {
		t.Error("Expected version string format even with empty values")
	}
}

// Note: Other functions in main.go (like main(), initExports(), etc.) are difficult to unit test
// because they depend on global state, configuration files, and external services.
// Integration tests would be more appropriate for those functions.
