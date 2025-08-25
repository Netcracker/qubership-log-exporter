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

type CustomGauge struct {
	sync.RWMutex
	constGaugeMap map[string]*prometheus.Metric
	Desc          *prometheus.Desc
}

func NewCustomGauge(desc *prometheus.Desc) *CustomGauge {
	customGauge := CustomGauge{}
	customGauge.Desc = desc
	customGauge.constGaugeMap = make(map[string]*prometheus.Metric)
	return &customGauge
}

func (g *CustomGauge) Describe(ch chan<- *prometheus.Desc) {
	g.RLock()
	defer g.RUnlock()
	for _, constGauge := range g.constGaugeMap {
		ch <- (*constGauge).Desc()
	}
}

func (g *CustomGauge) Collect(ch chan<- prometheus.Metric) {
	g.RLock()
	defer g.RUnlock()
	for _, constGauge := range g.constGaugeMap {
		ch <- *constGauge
	}
}

func (g *CustomGauge) CollectWithTimestamp(ch chan<- prometheus.Metric, timestamp time.Time) {
	g.RLock()
	defer g.RUnlock()
	for _, constCounter := range g.constGaugeMap {
		ch <- prometheus.NewMetricWithTimestamp(timestamp, *constCounter)
	}
}

func (g *CustomGauge) Set(val float64, labels map[string]string, labelKeys []string, timestamp *time.Time) {
	g.Lock()
	defer g.Unlock()
	labelValues := utils.GetOrderedMapValues(labels, labelKeys)
	gaugeKey := utils.MapToString(labels)
	constGauge := prometheus.MustNewConstMetric(g.Desc, prometheus.GaugeValue, val, labelValues...)
	if timestamp != nil {
		constGauge = prometheus.NewMetricWithTimestamp(*timestamp, constGauge)
	}
	g.constGaugeMap[gaugeKey] = &constGauge
}
