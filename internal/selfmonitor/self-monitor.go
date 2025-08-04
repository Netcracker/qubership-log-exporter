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

package selfmonitor

import (
	"fmt"
	"log_exporter/internal/collectors"
	"log_exporter/internal/config"
	"log_exporter/internal/registry"
	"log_exporter/internal/utils"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	coll "github.com/prometheus/client_golang/prometheus/collectors"
)

var (
	queryLatencyBuckets            = []float64{0.01, 0.1, 0.5, 1, 5, 10, 30, 60}
	metricEvaluationLatencyBuckets = []float64{0.01, 0.1, 0.5, 1, 5, 10, 30, 60}
	queryResponseSizeBuckets       = []float64{1, 32, 1024, 32768, 1048576, 33554432, 1073741824}

	dataExporterCacheSize                 *collectors.CustomGauge
	graylogResponseErrorCount             *collectors.CustomCounter
	queryDurationsHistogramVec            *collectors.CustomHistogram
	metricEvaluationDurationsHistogramVec *collectors.CustomHistogram
	enrichEvaluationDurationsHistogramVec *collectors.CustomHistogram
	queryResponseSizeHistogramVec         *collectors.CustomHistogram
	regexMatchedCounterVec                *collectors.CustomCounter
	regexNotMatchedCounterVec             *collectors.CustomCounter
	panicCounterVec                       *collectors.CustomCounter
	queueSizeGaugeVec                     *collectors.CustomGauge

	querySelfLabels        = []string{"query_name"}
	metricSelfLabels       = []string{"metric_name"}
	enrichSelfLabels       = []string{"query_name", "enrich_index"}
	queryProcessSelfLabels = []string{"query_name", "process_name"}
	queryQueueSelfLabels   = []string{"query_name", "queue_name"}
	emptySelfLabels        = []string{}
)

func InitSelfMonitoring(appConfig *config.Config, omnipresentLabels map[string]string, deRegistry *registry.DERegistry) {
	dataExporterCacheSize = collectors.NewCustomGauge(
		prometheus.NewDesc(
			"data_exporter_cache_size",
			"Log-exporter cache size in bytes",
			emptySelfLabels,
			omnipresentLabels,
		),
	)

	graylogResponseErrorCount = collectors.NewCustomCounter(
		prometheus.NewDesc(
			"graylog_response_error_count",
			"Graylog response error count to log-exporter by query",
			querySelfLabels,
			omnipresentLabels,
		),
	)

	queryDurationsHistogramVec = collectors.NewCustomHistogram(
		prometheus.NewDesc(
			"query_latency",
			"Query execution latency in seconds",
			querySelfLabels,
			omnipresentLabels,
		),
	)

	metricEvaluationDurationsHistogramVec = collectors.NewCustomHistogram(
		prometheus.NewDesc(
			"metric_evaluation_latency",
			"Metric evaluation latency in seconds",
			metricSelfLabels,
			omnipresentLabels,
		),
	)

	enrichEvaluationDurationsHistogramVec = collectors.NewCustomHistogram(
		prometheus.NewDesc(
			"enrich_evaluation_latency",
			"Enrich evaluation latency in seconds (enrich_index starts from 0)",
			enrichSelfLabels,
			omnipresentLabels,
		),
	)

	queryResponseSizeHistogramVec = collectors.NewCustomHistogram(
		prometheus.NewDesc(
			"graylog_response_size",
			"Graylog response size to log-exporter in bytes by query",
			querySelfLabels,
			omnipresentLabels,
		),
	)

	regexMatchedCounterVec = collectors.NewCustomCounter(
		prometheus.NewDesc(
			"regex_matched",
			"Count of regexps have been matched per query, enrich_index",
			enrichSelfLabels,
			omnipresentLabels,
		),
	)

	regexNotMatchedCounterVec = collectors.NewCustomCounter(
		prometheus.NewDesc(
			"regex_not_matched",
			"Count of regexps have been not matched per query, enrich_index",
			enrichSelfLabels,
			omnipresentLabels,
		),
	)

	panicCounterVec = collectors.NewCustomCounter(
		prometheus.NewDesc(
			"panic_recovery_count",
			"Count of panics have been recoveried by query, process",
			queryProcessSelfLabels,
			omnipresentLabels,
		),
	)

	queueSizeGaugeVec = collectors.NewCustomGauge(
		prometheus.NewDesc(
			"queue_size",
			"Size of queues inside log-exporter application",
			queryQueueSelfLabels,
			omnipresentLabels,
		),
	)

	initRegexMatchedNotMatched(appConfig)

	deRegistry.MustRegister(utils.SELF_METRICS_REGISTRY_NAME, &SelfmonitorCollector{})
	deRegistry.MustRegister(utils.SELF_METRICS_REGISTRY_NAME, coll.NewGoCollector())
	deRegistry.MustRegister(utils.SELF_METRICS_REGISTRY_NAME, coll.NewProcessCollector(coll.ProcessCollectorOpts{}))

}

func initRegexMatchedNotMatched(appConfig *config.Config) {
	now := time.Now()
	for queryName, queryConfig := range appConfig.Queries {
		for enrichIndex := range queryConfig.Enrich {
			labels := make(map[string]string)
			labels["query_name"] = queryName
			labels["enrich_index"] = fmt.Sprint(enrichIndex)
			AddMatchedRegexpsCount(labels, 0, &now)
			AddNotMatchedRegexpsCount(labels, 0, &now)
		}
	}
}

type SelfmonitorCollector struct {
	sync.RWMutex
}

func (c *SelfmonitorCollector) Describe(ch chan<- *prometheus.Desc) {
	c.RLock()
	defer c.RUnlock()
	dataExporterCacheSize.Describe(ch)
	graylogResponseErrorCount.Describe(ch)
	queryDurationsHistogramVec.Describe(ch)
	metricEvaluationDurationsHistogramVec.Describe(ch)
	enrichEvaluationDurationsHistogramVec.Describe(ch)
	queryResponseSizeHistogramVec.Describe(ch)
	regexMatchedCounterVec.Describe(ch)
	regexNotMatchedCounterVec.Describe(ch)
	panicCounterVec.Describe(ch)
	queueSizeGaugeVec.Describe(ch)
}

func (c *SelfmonitorCollector) Collect(ch chan<- prometheus.Metric) {
	c.RLock()
	defer c.RUnlock()
	if *utils.DisableTimestamp {
		dataExporterCacheSize.Collect(ch)
		graylogResponseErrorCount.Collect(ch)
		queryDurationsHistogramVec.Collect(ch)
		metricEvaluationDurationsHistogramVec.Collect(ch)
		enrichEvaluationDurationsHistogramVec.Collect(ch)
		queryResponseSizeHistogramVec.Collect(ch)
		regexMatchedCounterVec.Collect(ch)
		regexNotMatchedCounterVec.Collect(ch)
		panicCounterVec.Collect(ch)
		queueSizeGaugeVec.Collect(ch)
	} else {
		timestamp := time.Now()
		dataExporterCacheSize.CollectWithTimestamp(ch, timestamp)
		graylogResponseErrorCount.CollectWithTimestamp(ch, timestamp)
		queryDurationsHistogramVec.CollectWithTimestamp(ch, timestamp)
		metricEvaluationDurationsHistogramVec.CollectWithTimestamp(ch, timestamp)
		enrichEvaluationDurationsHistogramVec.CollectWithTimestamp(ch, timestamp)
		queryResponseSizeHistogramVec.CollectWithTimestamp(ch, timestamp)
		regexMatchedCounterVec.CollectWithTimestamp(ch, timestamp)
		regexNotMatchedCounterVec.CollectWithTimestamp(ch, timestamp)
		panicCounterVec.CollectWithTimestamp(ch, timestamp)
		queueSizeGaugeVec.CollectWithTimestamp(ch, timestamp)
	}
}

func UpdateDataExporterCacheSize(labels map[string]string, value float64) {
	dataExporterCacheSize.Set(value, labels, emptySelfLabels, nil)
}

func IncGraylogResponseErrorCount(labels map[string]string, timestamp *time.Time) {
	if *utils.DisableTimestamp {
		timestamp = nil
	}
	graylogResponseErrorCount.Add(1.0, labels, querySelfLabels, timestamp)
}

func RefreshGraylogResponseErrorCount(labels map[string]string, timestamp *time.Time) {
	if *utils.DisableTimestamp {
		return
	}
	graylogResponseErrorCount.Add(0.0, labels, querySelfLabels, timestamp)
}

func ObserveQueryLatency(labels map[string]string, value float64, timestamp *time.Time) {
	if *utils.DisableTimestamp {
		timestamp = nil
	}
	queryDurationsHistogramVec.ObserveSingle(value, queryLatencyBuckets, labels, querySelfLabels, timestamp)
}

func ObserveMetricEvaluationLatency(labels map[string]string, value float64, timestamp *time.Time) {
	if *utils.DisableTimestamp {
		timestamp = nil
	}
	metricEvaluationDurationsHistogramVec.ObserveSingle(value, metricEvaluationLatencyBuckets, labels, metricSelfLabels, timestamp)
}

func ObserveEnrichEvaluationLatency(labels map[string]string, value float64, timestamp *time.Time) {
	if *utils.DisableTimestamp {
		timestamp = nil
	}
	enrichEvaluationDurationsHistogramVec.ObserveSingle(value, metricEvaluationLatencyBuckets, labels, enrichSelfLabels, timestamp)
}

func ObserveQueryResponseSize(labels map[string]string, value float64, timestamp *time.Time) {
	if *utils.DisableTimestamp {
		timestamp = nil
	}
	queryResponseSizeHistogramVec.ObserveSingle(value, queryResponseSizeBuckets, labels, querySelfLabels, timestamp)
}

func AddMatchedRegexpsCount(labels map[string]string, value float64, timestamp *time.Time) {
	if *utils.DisableTimestamp {
		timestamp = nil
	}
	regexMatchedCounterVec.Add(value, labels, enrichSelfLabels, timestamp)
}

func AddNotMatchedRegexpsCount(labels map[string]string, value float64, timestamp *time.Time) {
	if *utils.DisableTimestamp {
		timestamp = nil
	}
	regexNotMatchedCounterVec.Add(value, labels, enrichSelfLabels, timestamp)
}

func IncPanicRecoveriesCount(labels map[string]string, value float64, timestamp *time.Time) {
	if *utils.DisableTimestamp {
		timestamp = nil
	}
	panicCounterVec.Add(value, labels, queryProcessSelfLabels, timestamp)
}

func SetQueueSize(value float64, labels map[string]string, timestamp *time.Time) {
	if *utils.DisableTimestamp {
		timestamp = nil
	}
	queueSizeGaugeVec.Set(value, labels, queryQueueSelfLabels, timestamp)
}
