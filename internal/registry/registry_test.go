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

package registry

import (
	"log_exporter/internal/config"
	"log_exporter/internal/utils"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewDERegistry(t *testing.T) {
	appConfig := &config.Config{
		Queries: map[string]*config.QueryConfig{
			"query1": {},
			"query2": {},
		},
	}

	registry := NewDERegistry(appConfig)

	if registry.registries == nil {
		t.Error("Expected registries map to be initialized")
	}
	if len(registry.registries) != 3 { // 2 queries + self metrics
		t.Errorf("Expected 3 registries, got: %d", len(registry.registries))
	}
	if registry.registries["query1"] == nil {
		t.Error("Expected registry for query1")
	}
	if registry.registries["query2"] == nil {
		t.Error("Expected registry for query2")
	}
	if registry.registries[utils.SELF_METRICS_REGISTRY_NAME] == nil {
		t.Error("Expected self metrics registry")
	}
}

func TestDERegistry_MustRegister(t *testing.T) {
	appConfig := &config.Config{
		Queries: map[string]*config.QueryConfig{
			"test_query": {},
		},
	}

	registry := NewDERegistry(appConfig)

	// Create a test counter
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_counter",
		Help: "Test counter",
	})

	// Register it
	registry.MustRegister("test_query", counter)

	// Verify it was registered by gathering
	gathered, err := registry.Gather()
	if err != nil {
		t.Errorf("Expected no error during gather, got: %v", err)
	}
	if len(gathered) == 0 {
		t.Error("Expected at least one metric family")
	}
}

func TestDERegistry_MustRegister_InvalidQuery(t *testing.T) {
	appConfig := &config.Config{
		Queries: map[string]*config.QueryConfig{
			"test_query": {},
		},
	}

	registry := NewDERegistry(appConfig)

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_counter",
		Help: "Test counter",
	})

	// Try to register for non-existent query - should not panic
	registry.MustRegister("non_existent", counter)
}

func TestDERegistry_Gather(t *testing.T) {
	appConfig := &config.Config{
		Queries: map[string]*config.QueryConfig{
			"test_query": {},
		},
	}

	registry := NewDERegistry(appConfig)

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_counter",
		Help: "Test counter",
	})

	registry.MustRegister("test_query", counter)

	gathered, err := registry.Gather()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if gathered == nil {
		t.Error("Expected non-nil result")
	}
}

func TestDERegistry_GetRegistry(t *testing.T) {
	appConfig := &config.Config{
		Queries: map[string]*config.QueryConfig{
			"test_query": {},
		},
	}

	registry := NewDERegistry(appConfig)

	reg := registry.GetRegistry("test_query")
	if reg == nil {
		t.Error("Expected registry to be returned")
	}

	// Test non-existent registry
	reg = registry.GetRegistry("non_existent")
	if reg != nil {
		t.Error("Expected nil for non-existent registry")
	}
}
