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

type CustomCounter struct {
	sync.RWMutex
	constCounterMap map[string]*prometheus.Metric
	Desc            *prometheus.Desc
	stateMap        map[string]float64
}

func NewCustomCounter(desc *prometheus.Desc) *CustomCounter {
	customCounter := CustomCounter{}
	customCounter.Desc = desc
	customCounter.constCounterMap = make(map[string]*prometheus.Metric)
	customCounter.stateMap = make(map[string]float64)
	return &customCounter
}

func (c *CustomCounter) Describe(ch chan<- *prometheus.Desc) {
	c.RLock()
	defer c.RUnlock()
	for _, constCounter := range c.constCounterMap {
		ch <- (*constCounter).Desc()
	}
}

func (c *CustomCounter) Collect(ch chan<- prometheus.Metric) {
	c.RLock()
	defer c.RUnlock()
	for _, constCounter := range c.constCounterMap {
		ch <- *constCounter
	}
}

func (c *CustomCounter) CollectWithTimestamp(ch chan<- prometheus.Metric, timestamp time.Time) {
	c.RLock()
	defer c.RUnlock()
	for _, constCounter := range c.constCounterMap {
		ch <- prometheus.NewMetricWithTimestamp(timestamp, *constCounter)
	}
}

func (c *CustomCounter) Add(val float64, labels map[string]string, labelKeys []string, timestamp *time.Time) {
	c.Lock()
	defer c.Unlock()
	labelValues := utils.GetOrderedMapValues(labels, labelKeys)
	counterKey := utils.MapToString(labels)
	totalVal := c.stateMap[counterKey] + val
	c.stateMap[counterKey] = totalVal
	constCounter := prometheus.MustNewConstMetric(c.Desc, prometheus.CounterValue, totalVal, labelValues...)
	if timestamp != nil {
		constCounter = prometheus.NewMetricWithTimestamp(*timestamp, constCounter)
	}
	c.constCounterMap[counterKey] = &constCounter
}
