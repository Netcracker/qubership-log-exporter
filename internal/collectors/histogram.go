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
	"log_exporter/internal/utils"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type CustomHistogram struct {
	sync.RWMutex
	constHistogramMap map[string]*prometheus.Metric
	stateMap          map[string]*CurrentHistogramState
	Desc              *prometheus.Desc
}

type CurrentHistogramState struct {
	Sum     float64
	Cnt     uint64
	Buckets map[float64]uint64
}

func NewCustomHistogram(desc *prometheus.Desc) *CustomHistogram {
	customHistogram := CustomHistogram{}
	customHistogram.Desc = desc
	customHistogram.stateMap = make(map[string]*CurrentHistogramState)
	customHistogram.constHistogramMap = make(map[string]*prometheus.Metric)
	return &customHistogram
}

func (h *CustomHistogram) Describe(ch chan<- *prometheus.Desc) {
	h.RLock()
	defer h.RUnlock()
	for _, constHistogram := range h.constHistogramMap {
		ch <- (*constHistogram).Desc()
	}
}

func (h *CustomHistogram) Collect(ch chan<- prometheus.Metric) {
	h.RLock()
	defer h.RUnlock()
	for _, constHistogram := range h.constHistogramMap {
		ch <- *constHistogram
	}
}

func (h *CustomHistogram) CollectWithTimestamp(ch chan<- prometheus.Metric, timestamp time.Time) {
	h.RLock()
	defer h.RUnlock()
	for _, constCounter := range h.constHistogramMap {
		ch <- prometheus.NewMetricWithTimestamp(timestamp, *constCounter)
	}
}

func (h *CustomHistogram) Observe(sum float64, cnt uint64, buckets map[float64]uint64, labels map[string]string, labelKeys []string, timestamp *time.Time) {
	h.Lock()
	defer h.Unlock()
	labelValues := utils.GetOrderedMapValues(labels, labelKeys)
	histKey := utils.MapToString(labels)
	if h.stateMap[histKey] == nil {
		h.stateMap[histKey] = &CurrentHistogramState{
			Sum:     0,
			Cnt:     0,
			Buckets: make(map[float64]uint64),
		}
	}
	h.stateMap[histKey].Sum += sum
	h.stateMap[histKey].Cnt += cnt
	for key, value := range buckets {
		h.stateMap[histKey].Buckets[key] += value
	}
	constHistogram := prometheus.MustNewConstHistogram(h.Desc, h.stateMap[histKey].Cnt, h.stateMap[histKey].Sum, h.stateMap[histKey].Buckets, labelValues...)
	if timestamp != nil {
		constHistogram = prometheus.NewMetricWithTimestamp(*timestamp, constHistogram)
	}
	h.constHistogramMap[histKey] = &constHistogram
}

func (h *CustomHistogram) ObserveSingle(val float64, bucketList []float64, labels map[string]string, labelKeys []string, timestamp *time.Time) {
	h.Lock()
	defer h.Unlock()
	labelValues := utils.GetOrderedMapValues(labels, labelKeys)
	histKey := utils.MapToString(labels)
	if h.stateMap[histKey] == nil {
		histState := &CurrentHistogramState{
			Sum:     0,
			Cnt:     0,
			Buckets: make(map[float64]uint64),
		}
		for _, b := range bucketList {
			histState.Buckets[b] = 0
		}
		h.stateMap[histKey] = histState
	}

	h.stateMap[histKey].Sum += val
	h.stateMap[histKey].Cnt += 1
	for _, b := range bucketList {
		if val < b {
			h.stateMap[histKey].Buckets[b]++
		}
	}
	constHistogram := prometheus.MustNewConstHistogram(h.Desc, h.stateMap[histKey].Cnt, h.stateMap[histKey].Sum, h.stateMap[histKey].Buckets, labelValues...)
	if timestamp != nil {
		constHistogram = prometheus.NewMetricWithTimestamp(*timestamp, constHistogram)
	}
	h.constHistogramMap[histKey] = &constHistogram
}
