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
	"log_exporter/internal/config"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"math"
	"strconv"

	log "github.com/sirupsen/logrus"
)

func (e *Evaluator) evaluateValueMetric(data [][]string, metric string, metricCfg *config.MetricsConfig) *MetricEvaluationResult {
	log.WithField("metric", metric).Debug("evaluateValueMetric")
	metricState := e.monState.Get(metric)
	var metricSeriesMap map[string]*MetricSeries

	if metricState == nil {
		log.WithField("metric", metric).Warn("MetricState is empty")
		metricState = CreateMetricState()
		e.monState.Set(metric, metricState)
	}
	result := CreateMetricEvaluationResult(metricState.Size())

	defer func() {
		result = e.performMetricPostEvaluationSteps(result, metricState, metricSeriesMap, metric, metricCfg)
	}()

	metricSeriesMap = e.evaluateMetricSeriesMapByOLV(data, metric, metricCfg)
	if log.IsLevelEnabled(log.TraceLevel) {
		for olv, ms := range metricSeriesMap {
			log.WithFields(log.Fields{"metric": metric, "olv": olv, "sum": ms.Sum, "cnt": ms.Count}).Trace("MetricSeriesMap : sum and count evaluated")
		}
	}

	for olv, ms := range metricSeriesMap {
		labels := metricState.Get(olv)
		if labels == nil {
			labels = generateLabelValueMapFromOLV(olv, metricCfg.Labels)
			metricState.Set(olv, labels)
			log.WithFields(log.Fields{"olv": olv, "metric": metric}).Debug("Generating new metricstate values")
		}
		ms.Labels = labels
		if ms.Count != 0 {
			ms.Average = ms.Sum / float64(ms.Count)
		} else {
			ms.Average = math.NaN()
		}
		result.Series = append(result.Series, *ms)
	}

	return result
}

func (e *Evaluator) evaluateMetricSeriesMapByOLV(data [][]string, metric string, metricCfg *config.MetricsConfig) map[string]*MetricSeries {
	log.WithField("metric", metric).Debug("evaluateMetricSeriesMapByOLV")
	dataSize := len(data)
	if dataSize < 2 {
		log.WithFields(log.Fields{"data_size": dataSize, "metric": metric}).Debug("The evaluation result is empty")
		return make(map[string]*MetricSeries)
	}

	heading := data[0]
	labelIndexes, err := evaluateLabelSourceFieldIndexes(metricCfg, heading)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_1020).WithFields(log.Fields{"metric": metric, "error": err}).Error("Cannot evaluate the value metric")
		return make(map[string]*MetricSeries)
	}
	log.WithFields(log.Fields{"label_indexes": labelIndexes, "heading": heading, "metric": metric}).Debug("Label indexes resolved")

	valueField := metricCfg.MetricValue
	if valueField == "" {
		valueField = metricCfg.Parameters["value-field"]
	}
	valueIndex := utils.FindStringIndexInArray(heading, valueField)
	if valueIndex == -1 {
		log.WithField(ec.FIELD, ec.LME_1020).WithFields(log.Fields{"metric": metric, "field": valueField}).Error("Cannot evaluate the value metric, the field is not found in the output")
		return make(map[string]*MetricSeries)
	}

	var meCondition *MECondition
	if metricCfg.Cond != nil {
		meCondition = CreateMECondition(metric, metricCfg.Cond, heading)
	}
	threadsNumber := metricCfg.Threads
	if threadsNumber > dataSize-1 {
		threadsNumber = dataSize - 1
	}
	if threadsNumber <= 1 {
		return e.evaluateMetricSeriesMapByOLVTask(data, metric, metricCfg, labelIndexes, valueIndex, meCondition, 1, len(data))
	}
	msChan := make(chan map[string]*MetricSeries)
	for i := 0; i < threadsNumber; i++ {
		start := 1 + i*(dataSize-1)/threadsNumber
		end := 1 + (i+1)*(dataSize-1)/threadsNumber
		go func() {
			msm := e.evaluateMetricSeriesMapByOLVTask(data, metric, metricCfg, labelIndexes, valueIndex, meCondition, start, end)
			msChan <- msm
		}()
	}

	msms := make([]map[string]*MetricSeries, 0, threadsNumber)

	for i := 0; i < threadsNumber; i++ {
		msm := <-msChan
		msms = append(msms, msm)
	}

	result := msms[0]

	for i := 1; i < threadsNumber; i++ {
		msm := msms[i]
		for olv, metricSeries := range msm {
			resultMetricSeries := result[olv]
			if resultMetricSeries == nil {
				result[olv] = metricSeries
			} else {
				resultMetricSeries.Sum += metricSeries.Sum
				resultMetricSeries.Count += metricSeries.Count
				if metricSeries.HistValue != nil {
					if resultMetricSeries.HistValue == nil {
						resultMetricSeries.HistValue = metricSeries.HistValue
					} else {
						resHistValue := resultMetricSeries.HistValue
						msHistBalue := metricSeries.HistValue
						resHistValue.Sum += msHistBalue.Sum
						resHistValue.Cnt += msHistBalue.Cnt
						for k, v := range msHistBalue.Buckets {
							resHistValue.Buckets[k] += v
						}
					}
				}
			}
		}
	}

	return result
}

func (e *Evaluator) evaluateMetricSeriesMapByOLVTask(data [][]string, metric string, metricCfg *config.MetricsConfig, labelIndexes []int, valueIndex int, meCondition *MECondition, start int, end int) map[string]*MetricSeries {
	log.WithFields(log.Fields{"metric": metric, "start": start, "end": end}).Debug("evaluateMetricSeriesMapByOLVTask")
	result := make(map[string]*MetricSeries)
	isHistogram := (metricCfg.Type == "histogram")

	if start >= end {
		log.WithFields(log.Fields{"metric": metric, "start": start, "end": end}).Debug("start >= end")
		return result
	}

	var parsingErrors, nans, infs int64
	for i := start; i < end; i++ {
		if meCondition != nil && !meCondition.Apply(data[i]) {
			continue
		}
		olv := generateOrderedLabelValuesString(labelIndexes, data[i])
		valStr := data[i][valueIndex]
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			log.WithFields(log.Fields{"value": valStr, "metric": metric, "error": err}).Debug("Cannot parse the value")
			parsingErrors++
			continue
		}

		if math.IsNaN(val) {
			nans++
			continue
		}
		if math.IsInf(val, 0) {
			infs++
			continue
		}

		ms := result[olv]
		if ms == nil {
			ms = &MetricSeries{}
			if isHistogram {
				ms.HistValue = CreateHistogramMetricValue(metricCfg.Buckets)
			}
			result[olv] = ms
		}
		ms.Sum += val
		ms.Count++
		if isHistogram {
			ms.HistValue.Observe(val)
		}
	}

	if parsingErrors != 0 || nans != 0 || infs != 0 {
		log.WithFields(log.Fields{"metric": metric, "start": start, "end": end, "parsing_errors": parsingErrors, "nans": nans, "infs": infs}).Warn("Datarows were skipped while evaluating the metric")
	}

	return result
}
