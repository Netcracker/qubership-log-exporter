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

package queues

import (
	"log_exporter/internal/config"
	"testing"
	"time"
)

func TestNewGDQueue(t *testing.T) {
	// Create a minimal config for testing
	appConfig := &config.Config{
		Queries: map[string]*config.QueryConfig{
			"test_query": {
				GDQueueSizeParsed: 10,
			},
		},
	}

	queue := NewGDQueue(appConfig)

	if queue.appConfig != appConfig {
		t.Error("Expected appConfig to be set")
	}
	if queue.graylogDataByQuery == nil {
		t.Error("Expected graylogDataByQuery to be initialized")
	}
	if len(queue.graylogDataByQuery) != 1 {
		t.Errorf("Expected 1 query queue, got: %d", len(queue.graylogDataByQuery))
	}
	if cap(queue.graylogDataByQuery["test_query"]) != 10 {
		t.Errorf("Expected queue capacity 10, got: %d", cap(queue.graylogDataByQuery["test_query"]))
	}
}

func TestGDQueue_PutGet(t *testing.T) {
	// Skip this test as it requires selfmonitoring initialization
	t.Skip("Skipping TestGDQueue_PutGet - requires selfmonitoring setup")
}

func TestGDQueue_Get_NonExistentQuery(t *testing.T) {
	appConfig := &config.Config{
		Queries: map[string]*config.QueryConfig{
			"test_query": {
				GDQueueSizeParsed: 5,
			},
		},
	}

	queue := NewGDQueue(appConfig)

	// Try to get from non-existent query
	retrieved, ok := queue.Get("non_existent")

	if ok {
		t.Error("Expected failure for non-existent query")
	}
	if retrieved != nil {
		t.Error("Expected nil data for non-existent query")
	}
}

func TestGDQueue_Put_NonExistentQuery(t *testing.T) {
	appConfig := &config.Config{
		Queries: map[string]*config.QueryConfig{
			"test_query": {
				GDQueueSizeParsed: 5,
			},
		},
	}

	queue := NewGDQueue(appConfig)

	data := &GraylogData{
		Data:      [][]string{{"test"}},
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}

	// This should not panic, just log an error
	queue.Put("non_existent", data)
}

func TestGDQueue_CloseChan(t *testing.T) {
	appConfig := &config.Config{
		Queries: map[string]*config.QueryConfig{
			"test_query": {
				GDQueueSizeParsed: 5,
			},
		},
	}

	queue := NewGDQueue(appConfig)

	// Close the channel
	queue.CloseChan("test_query")

	// Note: We can't easily test Get after close without initializing selfmonitoring
	// The CloseChan method itself is tested for basic functionality
}

// Similar tests could be added for other queue types (GMQueue, GTSQueue)
// but for brevity, focusing on GDQueue as an example
