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

package collectors

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNewCustomCounter(t *testing.T) {
	desc := prometheus.NewDesc("test_counter", "test counter", []string{"label1", "label2"}, nil)
	counter := NewCustomCounter(desc)

	if counter.Desc == nil {
		t.Error("Expected Desc to be set")
	}
	if counter.constCounterMap == nil {
		t.Error("Expected constCounterMap to be initialized")
	}
	if counter.stateMap == nil {
		t.Error("Expected stateMap to be initialized")
	}
}

func TestCustomCounter_Add(t *testing.T) {
	desc := prometheus.NewDesc("test_counter", "test counter", []string{"label1", "label2"}, nil)
	counter := NewCustomCounter(desc)

	labels := map[string]string{"label1": "value1", "label2": "value2"}
	labelKeys := []string{"label1", "label2"}

	// Add first value
	counter.Add(5.0, labels, labelKeys, nil)

	// Add second value to same labels (should accumulate)
	counter.Add(3.0, labels, labelKeys, nil)

	// Collect metrics
	ch := make(chan prometheus.Metric, 1)
	counter.Collect(ch)
	close(ch)

	metric := <-ch
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if *m.Counter.Value != 8.0 {
		t.Errorf("Expected counter value 8.0, got %f", *m.Counter.Value)
	}
}

func TestCustomCounter_AddWithTimestamp(t *testing.T) {
	desc := prometheus.NewDesc("test_counter", "test counter", []string{"label1"}, nil)
	counter := NewCustomCounter(desc)

	labels := map[string]string{"label1": "value1"}
	labelKeys := []string{"label1"}
	timestamp := time.Now()

	counter.Add(10.0, labels, labelKeys, &timestamp)

	ch := make(chan prometheus.Metric, 1)
	counter.Collect(ch)
	close(ch)

	metric := <-ch
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if m.TimestampMs == nil {
		t.Error("Expected timestamp to be set")
	}
	if *m.Counter.Value != 10.0 {
		t.Errorf("Expected counter value 10.0, got %f", *m.Counter.Value)
	}
}

func TestCustomCounter_Describe(t *testing.T) {
	desc := prometheus.NewDesc("test_counter", "test counter", []string{"label1"}, nil)
	counter := NewCustomCounter(desc)

	labels := map[string]string{"label1": "value1"}
	labelKeys := []string{"label1"}
	counter.Add(1.0, labels, labelKeys, nil)

	ch := make(chan *prometheus.Desc, 1)
	counter.Describe(ch)
	close(ch)

	descFromChan := <-ch
	if descFromChan != desc {
		t.Error("Expected same descriptor")
	}
}

func TestNewCustomGauge(t *testing.T) {
	desc := prometheus.NewDesc("test_gauge", "test gauge", []string{"label1", "label2"}, nil)
	gauge := NewCustomGauge(desc)

	if gauge.Desc == nil {
		t.Error("Expected Desc to be set")
	}
	if gauge.constGaugeMap == nil {
		t.Error("Expected constGaugeMap to be initialized")
	}
}

func TestCustomGauge_Set(t *testing.T) {
	desc := prometheus.NewDesc("test_gauge", "test gauge", []string{"label1", "label2"}, nil)
	gauge := NewCustomGauge(desc)

	labels := map[string]string{"label1": "value1", "label2": "value2"}
	labelKeys := []string{"label1", "label2"}

	// Set value
	gauge.Set(15.5, labels, labelKeys, nil)

	// Collect metrics
	ch := make(chan prometheus.Metric, 1)
	gauge.Collect(ch)
	close(ch)

	metric := <-ch
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if *m.Gauge.Value != 15.5 {
		t.Errorf("Expected gauge value 15.5, got %f", *m.Gauge.Value)
	}
}

func TestCustomGauge_SetWithTimestamp(t *testing.T) {
	desc := prometheus.NewDesc("test_gauge", "test gauge", []string{"label1"}, nil)
	gauge := NewCustomGauge(desc)

	labels := map[string]string{"label1": "value1"}
	labelKeys := []string{"label1"}
	timestamp := time.Now()

	gauge.Set(20.0, labels, labelKeys, &timestamp)

	ch := make(chan prometheus.Metric, 1)
	gauge.Collect(ch)
	close(ch)

	metric := <-ch
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if m.TimestampMs == nil {
		t.Error("Expected timestamp to be set")
	}
	if *m.Gauge.Value != 20.0 {
		t.Errorf("Expected gauge value 20.0, got %f", *m.Gauge.Value)
	}
}

func TestCustomGauge_Describe(t *testing.T) {
	desc := prometheus.NewDesc("test_gauge", "test gauge", []string{"label1"}, nil)
	gauge := NewCustomGauge(desc)

	labels := map[string]string{"label1": "value1"}
	labelKeys := []string{"label1"}
	gauge.Set(1.0, labels, labelKeys, nil)

	ch := make(chan *prometheus.Desc, 1)
	gauge.Describe(ch)
	close(ch)

	descFromChan := <-ch
	if descFromChan != desc {
		t.Error("Expected same descriptor")
	}
}

func TestNewCustomHistogram(t *testing.T) {
	desc := prometheus.NewDesc("test_histogram", "test histogram", []string{"label1", "label2"}, nil)
	histogram := NewCustomHistogram(desc)

	if histogram.Desc == nil {
		t.Error("Expected Desc to be set")
	}
	if histogram.stateMap == nil {
		t.Error("Expected stateMap to be initialized")
	}
	if histogram.constHistogramMap == nil {
		t.Error("Expected constHistogramMap to be initialized")
	}
}

func TestCustomHistogram_Observe(t *testing.T) {
	desc := prometheus.NewDesc("test_histogram", "test histogram", []string{"label1", "label2"}, nil)
	histogram := NewCustomHistogram(desc)

	labels := map[string]string{"label1": "value1", "label2": "value2"}
	labelKeys := []string{"label1", "label2"}
	buckets := map[float64]uint64{1.0: 2, 2.5: 1, 5.0: 1}

	// Observe histogram data
	histogram.Observe(10.0, 3, buckets, labels, labelKeys, nil)

	// Collect metrics
	ch := make(chan prometheus.Metric, 1)
	histogram.Collect(ch)
	close(ch)

	metric := <-ch
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if *m.Histogram.SampleCount != 3 {
		t.Errorf("Expected sample count 3, got %d", *m.Histogram.SampleCount)
	}
	if *m.Histogram.SampleSum != 10.0 {
		t.Errorf("Expected sample sum 10.0, got %f", *m.Histogram.SampleSum)
	}
}

func TestCustomHistogram_ObserveSingle(t *testing.T) {
	desc := prometheus.NewDesc("test_histogram", "test histogram", []string{"label1"}, nil)
	histogram := NewCustomHistogram(desc)

	labels := map[string]string{"label1": "value1"}
	labelKeys := []string{"label1"}
	bucketList := []float64{1.0, 2.5, 5.0, 10.0}

	// Observe single values
	histogram.ObserveSingle(0.5, bucketList, labels, labelKeys, nil)
	histogram.ObserveSingle(3.0, bucketList, labels, labelKeys, nil)

	// Collect metrics
	ch := make(chan prometheus.Metric, 1)
	histogram.Collect(ch)
	close(ch)

	metric := <-ch
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if *m.Histogram.SampleCount != 2 {
		t.Errorf("Expected sample count 2, got %d", *m.Histogram.SampleCount)
	}
	if *m.Histogram.SampleSum != 3.5 {
		t.Errorf("Expected sample sum 3.5, got %f", *m.Histogram.SampleSum)
	}
}

func TestCustomHistogram_ObserveWithTimestamp(t *testing.T) {
	desc := prometheus.NewDesc("test_histogram", "test histogram", []string{"label1"}, nil)
	histogram := NewCustomHistogram(desc)

	labels := map[string]string{"label1": "value1"}
	labelKeys := []string{"label1"}
	buckets := map[float64]uint64{1.0: 1}
	timestamp := time.Now()

	histogram.Observe(5.0, 1, buckets, labels, labelKeys, &timestamp)

	ch := make(chan prometheus.Metric, 1)
	histogram.Collect(ch)
	close(ch)

	metric := <-ch
	var m dto.Metric
	if err := metric.Write(&m); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if m.TimestampMs == nil {
		t.Error("Expected timestamp to be set")
	}
}

func TestCustomHistogram_Describe(t *testing.T) {
	desc := prometheus.NewDesc("test_histogram", "test histogram", []string{"label1"}, nil)
	histogram := NewCustomHistogram(desc)

	labels := map[string]string{"label1": "value1"}
	labelKeys := []string{"label1"}
	buckets := map[float64]uint64{1.0: 1}
	histogram.Observe(1.0, 1, buckets, labels, labelKeys, nil)

	ch := make(chan *prometheus.Desc, 1)
	histogram.Describe(ch)
	close(ch)

	descFromChan := <-ch
	if descFromChan != desc {
		t.Error("Expected same descriptor")
	}
}
