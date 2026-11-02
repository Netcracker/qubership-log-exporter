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
	"fmt"
	"log_exporter/internal/config"
	"log_exporter/internal/selfmonitor"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type Evaluator struct {
	appConfig               *config.Config
	monState                *MonitoringState
	rtcRepo                 *RequestTimeCacheRepozitory
	nrcRepo                 *NoResponseCacheRepozitory
	idfcRepo                *IdFieldCacheRepozitory
	metricDefaultValuesRepo *MetricDefaultValuesRepository
}

func CreateEvaluator(appConfig *config.Config) *Evaluator {
	e := Evaluator{}
	e.appConfig = appConfig
	e.monState = CreateMonitoringState()
	for metric, metricCfg := range appConfig.Metrics {
		metricState := CreateMetricState()
		if len(metricCfg.ExpectedLabels) > 0 {
			for itemNum, expectedLabelsItem := range metricCfg.ExpectedLabels {
				cartesian := utils.LabelsCartesian(expectedLabelsItem)
				log.WithFields(log.Fields{"metric": metric, "item_num": itemNum, "cartesian": cartesian}).Info("Expected labels cartesian generated")
				for _, labels := range cartesian {
					metricState.Set(utils.MapToString(labels), labels)
				}
			}
		}
		if len(metricCfg.Labels) == 0 {
			metricState.Set("", make(map[string]string, 0))
		}
		e.monState.Set(metric, metricState)
	}
	e.rtcRepo = CreateRequestTimeCacheRepozitory(appConfig)
	e.nrcRepo = CreateNoResponseCacheRepo(appConfig)
	e.idfcRepo = CreateIdFieldCacheRepo(appConfig)
	e.metricDefaultValuesRepo = CreateMetricDefaultValuesRepository(appConfig.Metrics)
	return &e
}

func (e *Evaluator) EvaluateMetric(data [][]string, metric string, metricCfg *config.MetricsConfig, query string, endTime *time.Time) *MetricEvaluationResult {
	metricEvaluationStartTime := time.Now()
	defer func() {
		selfMonitorObserveMetricEvaluationLatency(metricEvaluationStartTime, metric)
	}()

	if metricCfg == nil {
		log.WithField(ec.FIELD, ec.LME_8102).WithField("metric", metric).Error("The metric is not defined in the metrics section, it cannot be evaluated")
		return nil
	}

	var result *MetricEvaluationResult
	switch metricCfg.Operation {
	case "count":
		result = e.evaluateCountMetric(data, metric, metricCfg)
	case "duration":
		result = e.evaluateDurationMetric(data, metric, metricCfg, query)
	case "value":
		result = e.evaluateValueMetric(data, metric, metricCfg)
	case "duration-no-response":
		log.WithField(ec.FIELD, ec.LME_8102).WithField("metric", metric).Error("The metric has a duration-no-response operation, which can be evaluated only as a child of another duration metric")
		return nil
	default:
		log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"metric": metric, "operation": metricCfg.Operation}).Error("The metric has an unsupported operation")
		return nil
	}

	if !*utils.DisableTimestamp {
		for i := range result.Series {
			result.Series[i].Timestamp = endTime
		}
		for _, childMetric := range result.ChildMetrics {
			for i := range childMetric.Series {
				childMetric.Series[i].Timestamp = endTime
			}
		}
	}
	if log.IsLevelEnabled(log.DebugLevel) {
		logMetricEvaluationResult(metric, result)
	}

	return result
}

func (e *Evaluator) performMetricPostEvaluationSteps(result *MetricEvaluationResult, metricState *MetricState, metricSeriesMap map[string]*MetricSeries, metric string, metricCfg *config.MetricsConfig) *MetricEvaluationResult {
	if metricCfg.Type == "gauge" {
		log.WithField("metric", metric).Debug("Appending NaN values")
		result = e.appendNaNForGauges(result, metricState, metricSeriesMap, metric)
	} else if !*utils.DisableTimestamp {
		log.WithField("metric", metric).Debug("Appending zero values")
		result = appendZeroValuesForNotUpdatedCounters(result, metricState, metricSeriesMap, metricCfg)
	}
	return result
}

func (e *Evaluator) appendNaNForGauges(result *MetricEvaluationResult, metricState *MetricState, metricSeriesMap map[string]*MetricSeries, metric string) *MetricEvaluationResult {
	metricStateKeys := metricState.GetAllKeys()
	for _, key := range metricStateKeys {
		if metricSeriesMap[key] == nil {
			ms := CreateMetricSeries(metricState.Get(key))
			ms.Average = e.metricDefaultValuesRepo.GetMetricDefaultValue(metric)
			result.Series = append(result.Series, ms)
		}
	}
	return result
}

func appendZeroValuesForNotUpdatedCounters(result *MetricEvaluationResult, metricState *MetricState, metricSeriesMap map[string]*MetricSeries, metricCfg *config.MetricsConfig) *MetricEvaluationResult {
	metricStateKeys := metricState.GetAllKeys()
	switch metricCfg.Type {
	case "counter":
		for _, key := range metricStateKeys {
			if metricSeriesMap[key] == nil {
				ms := CreateMetricSeries(metricState.Get(key))
				result.Series = append(result.Series, ms)
			}
		}
	case "histogram":
		for _, key := range metricStateKeys {
			if metricSeriesMap[key] == nil {
				ms := CreateMetricSeries(metricState.Get(key))
				ms.HistValue = CreateHistogramMetricValue(metricCfg.Buckets)
				result.Series = append(result.Series, ms)
			}
		}
	}
	return result
}

func logMetricEvaluationResult(metric string, result *MetricEvaluationResult) {
	log.WithFields(log.Fields{"metric": metric, "series_len": len(result.Series)}).Debug("Result evaluated")
	for i, ms := range result.Series {
		log.WithFields(log.Fields{"index": i, "labels": ms.Labels, "sum": ms.Sum, "cnt": ms.Count, "avg": ms.Average, "hist": ms.HistValue, "timestamp": ms.Timestamp}).Trace("Evaluated metric series")
	}
}

func generateOrderedLabelValuesString(labelIndexes []int, row []string) string {
	size := len(labelIndexes)
	result := make([]string, size)
	if size == 0 {
		return ""
	}

	for i := 0; i < size; i++ {
		result[i] = row[labelIndexes[i]]
	}

	return strings.Join(result, ";")
}

func evaluateLabelSourceFieldIndexes(metricCfg *config.MetricsConfig, heading []string) ([]int, error) {
	size := len(metricCfg.Labels) - len(metricCfg.MultiValueFields) // Mult-value labels are placed in the end of the labels list and for them there is no need to evaluate source field indexes
	labelIndexes := make([]int, size)
	for i := 0; i < size; i++ {
		label := metricCfg.Labels[i]
		field, ok := metricCfg.LabelFieldMap[label]
		if !ok {
			field = label
		}
		labelIndexes[i] = utils.FindStringIndexInArray(heading, field)
		if labelIndexes[i] == -1 {
			return nil, fmt.Errorf("field %v not found in the output for label %v index evaluation", field, label)
		}
	}
	return labelIndexes, nil
}

func generateLabelValueMapFromOLV(olv string, labels []string) map[string]string {
	labelValues := strings.Split(olv, ";")
	result := make(map[string]string, len(labels))

	if len(labelValues) != len(labels) {
		log.WithField(ec.FIELD, ec.LME_1020).WithFields(log.Fields{"labels_len": len(labels), "label_values_len": len(labelValues), "labels": labels, "label_values": labelValues}).Error("generateLabelValueMapFromOLV : the label and label value counts differ")
	}

	for i, label := range labels {
		result[label] = labelValues[i]
	}

	return result
}

func selfMonitorObserveMetricEvaluationLatency(start time.Time, metricName string) {
	elapsed := time.Since(start)
	seconds := float64(elapsed) / float64(time.Second)
	labels := make(map[string]string)
	labels["metric_name"] = metricName
	selfmonitor.ObserveMetricEvaluationLatency(labels, seconds, &start)
}
