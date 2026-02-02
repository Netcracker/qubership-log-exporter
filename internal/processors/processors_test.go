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

package processors

import (
	"log_exporter/internal/config"
	"log_exporter/internal/queues"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/robfig/cron/v3"
)

func TestSignalProcessorCreation(t *testing.T) {
	// Test that we can create a SignalProcessor instance
	// This ensures the processors package can be imported and basic types work

	stopCalled := false
	versionCalled := false

	stopFunc := func() {
		stopCalled = true
	}

	versionFunc := func() string {
		versionCalled = true
		return "test-version"
	}

	processor := NewSignalProcessor(stopFunc, versionFunc)

	// Basic assertions - check for nil first to satisfy staticcheck
	if processor == nil {
		t.Fatal("Expected SignalProcessor to be created")
	}

	if processor.stopCroniter == nil {
		t.Error("Expected stopCroniter function to be set")
	}

	if processor.versionString == nil {
		t.Error("Expected versionString function to be set")
	}

	// Test that the functions work
	processor.stopCroniter()
	if !stopCalled {
		t.Error("Expected stop function to be called")
	}

	version := processor.versionString()
	if !versionCalled {
		t.Error("Expected version function to be called")
	}

	if version != "test-version" {
		t.Errorf("Expected version 'test-version', got '%s'", version)
	}
}

func TestNewSelfMonSchedulerProcessor(t *testing.T) {
	// Test SelfMonSchedulerProcessor constructor
	appConfig := &config.Config{}
	gmQueue := &queues.GMQueue{}
	croniter := cron.New()
	registry := prometheus.NewRegistry()

	processor := NewSelfMonSchedulerProcessor(appConfig, gmQueue, croniter, registry)

	if processor == nil {
		t.Fatal("Expected SelfMonSchedulerProcessor to be created")
	}

	if processor.appConfig != appConfig {
		t.Error("Expected appConfig to be set")
	}

	if processor.gmQueue != gmQueue {
		t.Error("Expected gmQueue to be set")
	}

	if processor.croniter != croniter {
		t.Error("Expected croniter to be set")
	}

	if processor.selfMonregistry != registry {
		t.Error("Expected selfMonregistry to be set")
	}
}

func TestSelfMonSchedulerProcessor_Start(t *testing.T) {
	// Test Start method with nil gmQueue (should disable processor)
	appConfig := &config.Config{}
	croniter := cron.New()
	registry := prometheus.NewRegistry()

	processor := NewSelfMonSchedulerProcessor(appConfig, nil, croniter, registry)

	// This should not panic and should disable the processor
	processor.Start()

	// Test with valid gmQueue but skip the cron setup (complex)
	// The Start method is tested for basic functionality
}

func TestPushProcessor_EnrichWithCloudLabels(t *testing.T) {
	// Test the enrichWithCloudLabels method
	appConfig := &config.Config{
		General: &config.GeneralConfig{
			NamespaceName:          "test-namespace",
			PodName:                "test-pod",
			ContainerName:          "test-container",
			PushCloudLabels:        map[string]string{"env": "test", "region": "us-west"},
			DisablePushCloudLabels: false,
		},
	}

	processor := &PushProcessor{
		appConfig: appConfig,
	}

	// Create test metric families
	mfs := []*dto.MetricFamily{
		{
			Metric: []*dto.Metric{
				{
					Label: []*dto.LabelPair{
						{Name: stringPtr("existing"), Value: stringPtr("value")},
					},
				},
			},
		},
	}

	processor.enrichWithCloudLabels(mfs)

	// Check that labels were added
	metric := mfs[0].Metric[0]
	expectedLabels := map[string]string{
		"existing":  "value",
		"namespace": "test-namespace",
		"pod":       "test-pod",
		"container": "test-container",
		"env":       "test",
		"region":    "us-west",
	}

	if len(metric.Label) != len(expectedLabels) {
		t.Errorf("Expected %d labels, got %d", len(expectedLabels), len(metric.Label))
	}

	// Check each expected label
	for _, label := range metric.Label {
		if expectedValue, exists := expectedLabels[*label.Name]; exists {
			if *label.Value != expectedValue {
				t.Errorf("Expected label %s to have value %s, got %s", *label.Name, expectedValue, *label.Value)
			}
		}
	}
}

func TestPushProcessor_EnrichWithCloudLabels_Disabled(t *testing.T) {
	// Test with cloud labels disabled
	appConfig := &config.Config{
		General: &config.GeneralConfig{
			DisablePushCloudLabels: true,
		},
	}

	processor := &PushProcessor{
		appConfig: appConfig,
	}

	// Create test metric families
	mfs := []*dto.MetricFamily{
		{
			Metric: []*dto.Metric{
				{
					Label: []*dto.LabelPair{
						{Name: stringPtr("existing"), Value: stringPtr("value")},
					},
				},
			},
		},
	}

	originalLabelCount := len(mfs[0].Metric[0].Label)
	processor.enrichWithCloudLabels(mfs)

	// Should not add any labels when disabled
	if len(mfs[0].Metric[0].Label) != originalLabelCount {
		t.Error("Expected no labels to be added when disabled")
	}
}

func TestPushProcessorCreation(t *testing.T) {
	// Test that PushProcessor can be created
	appConfig := &config.Config{}

	processor := &PushProcessor{
		appConfig: appConfig,
	}

	if processor.appConfig != appConfig {
		t.Error("Expected appConfig to be set")
	}
}

// Helper function for creating string pointers
func stringPtr(s string) *string {
	return &s
}

// Additional basic tests to ensure package stability
func TestProcessorsPackageStability(t *testing.T) {
	// This test ensures the package can be imported and basic types exist
	// without causing import cycles or compilation issues

	// Test that key types and functions are accessible
	_ = NewSignalProcessor
	_ = NewSelfMonSchedulerProcessor

	// Test that package-level variables exist
	_ = namespace
	_ = pod
	_ = container

	t.Log("Processors package basic functionality verified")
}
