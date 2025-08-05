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

package evaluator

import (
	"sort"
	"time"
)

type MetricEvaluationResult struct {
	Series       []MetricSeries
	ChildMetrics map[string]*MetricEvaluationResult
}

func CreateMetricEvaluationResult(expectedSeriesDimension int64) *MetricEvaluationResult {
	mer := MetricEvaluationResult{}
	mer.Series = make([]MetricSeries, 0, expectedSeriesDimension)
	return &mer
}

type MetricSeries struct {
	Labels    map[string]string
	Average   float64
	Sum       float64
	Count     uint64
	Timestamp *time.Time
	HistValue *HistogramMetricValue
}

func CreateMetricSeries(labels map[string]string) MetricSeries {
	ms := MetricSeries{}
	ms.Labels = labels
	return ms
}

type HistogramMetricValue struct {
	Sum         float64
	Cnt         uint64
	Buckets     map[float64]uint64
	BucketsList []float64
}

func CreateHistogramMetricValue(bucketsList []float64) *HistogramMetricValue {
	histMetricValue := HistogramMetricValue{}
	histMetricValue.Sum = 0
	histMetricValue.Cnt = 0
	sort.Float64s(bucketsList)
	histMetricValue.BucketsList = bucketsList
	histMetricValue.Buckets = make(map[float64]uint64, len(bucketsList))
	for _, v := range bucketsList {
		histMetricValue.Buckets[v] = 0
	}
	return &histMetricValue
}

func (h *HistogramMetricValue) Observe(value float64) {
	h.Sum += value
	h.Cnt++
	for _, v := range h.BucketsList {
		if value <= v {
			h.Buckets[v]++
		}
	}
}
