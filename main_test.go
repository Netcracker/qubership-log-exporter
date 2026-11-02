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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckConfigReturnsFailureForInvalidConfiguration(t *testing.T) {
	malformed := filepath.Join(t.TempDir(), "malformed.yaml")
	if err := os.WriteFile(malformed, []byte("datasources: [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	previousPath := *configPath
	t.Cleanup(func() { *configPath = previousPath })
	for _, test := range []struct {
		name string
		path string
		want int
	}{
		{"valid", "examples/config_emu.yaml", 0},
		{"invalid", "examples/config_incorrect.yaml", 1},
		{"malformed", malformed, 1},
		{"missing", filepath.Join(t.TempDir(), "missing.yaml"), 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			*configPath = test.path
			if got := checkConfigAndExit(); got != test.want {
				t.Errorf("checkConfigAndExit(%q) = %d, want %d", test.path, got, test.want)
			}
		})
	}
}

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
