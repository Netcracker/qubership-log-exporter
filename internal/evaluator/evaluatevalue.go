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
    log "github.com/sirupsen/logrus"
    "log_exporter/internal/config"
    "log_exporter/internal/utils"
    ec "log_exporter/internal/utils/errorcodes"
    "math"
    "strconv"
)

func (e *Evaluator) evaluateValueMetric(data [][]string, metric string, metricCfg *config.MetricsConfig) *MetricEvaluationResult {
    log.Debugf("evaluateValueMetric %v", metric)
    metricState := e.monState.Get(metric)
    var metricSeriesMap map[string]*MetricSeries

    if metricState == nil {
        log.Warnf("MetricState is empty for %v", metric)
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
            log.Tracef("MetricSeriesMap : for metric %v for olv %v got sum %v and count %v", metric, olv, ms.Sum, ms.Count)
        }
    }

    for olv, ms := range metricSeriesMap {
        labels := metricState.Get(olv)
        if labels == nil {
            labels = generateLabelValueMapFromOLV(olv, metricCfg.Labels)
            metricState.Set(olv, labels)
            log.Debugf("Generate new metricstate values %v for metric %v", olv, metric)
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

func (e *Evaluator) evaluateMetricSeriesMapByOLV(data [][]string, metric string, metricCfg *config.MetricsConfig) (map[string]*MetricSeries) {
    log.Debugf("evaluateMetricSeriesMapByOLV for metric %v", metric)
    dataSize := len(data)
    if dataSize < 2 {
        log.Debugf("DataSize == %v for metric %v, evaluation result is empty", dataSize, metric)
        return make(map[string]*MetricSeries)
    }

    heading := data[0]
    labelIndexes, err := evaluateLabelSourceFieldIndexes(metricCfg, heading)
    if err != nil {
        log.WithField(ec.FIELD, ec.LME_1020).Errorf("Can not evaluate value metric %v : %+v", metric, err)
        return make(map[string]*MetricSeries)
    }
    log.Debugf("labelIndexes = %+v; heading = %v for metric %v", labelIndexes, heading, metric)

    valueField := metricCfg.MetricValue
    if valueField == "" {
        valueField = metricCfg.Parameters["value-field"]
    }
    valueIndex := utils.FindStringIndexInArray(heading, valueField)
    if valueIndex == -1 {
        log.WithField(ec.FIELD, ec.LME_1020).Errorf("Can not evaluate value metric %v : field %v not found in the output", metric, valueField)
        return make(map[string]*MetricSeries)
    }

    var meCondition *MECondition
    if metricCfg.Cond != nil {
        meCondition = CreateMECondition(metric, metricCfg.Cond, heading)
    }
    threadsNumber := metricCfg.Threads
    if threadsNumber > dataSize - 1 {
        threadsNumber = dataSize - 1
    }
    if threadsNumber <= 1 {
        return e.evaluateMetricSeriesMapByOLVTask(data, metric, metricCfg, labelIndexes, valueIndex, meCondition, 1, len(data))
    }
    msChan := make(chan map[string]*MetricSeries)
    for i := 0; i < threadsNumber; i++ {
        start := 1 + i * (dataSize - 1) / threadsNumber
        end := 1 + (i + 1) * (dataSize - 1) / threadsNumber
        go func() {
            msm := e.evaluateMetricSeriesMapByOLVTask(data, metric, metricCfg, labelIndexes, valueIndex, meCondition, start, end)
            msChan <- msm
        }()
    }

    msms := make([]map[string]*MetricSeries, 0, threadsNumber)

    for i := 0; i < threadsNumber; i++ {
        msm := <- msChan
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
                        for k,v := range msHistBalue.Buckets {
                            resHistValue.Buckets[k] += v
                        }
                    }
                }
            }
        }
    }

    return result
}

func (e *Evaluator) evaluateMetricSeriesMapByOLVTask(data [][]string, metric string, metricCfg *config.MetricsConfig, labelIndexes []int, valueIndex int, meCondition *MECondition, start int, end int) (map[string]*MetricSeries) {
    log.Debugf("evaluateMetricSeriesMapByOLVTask %v; start = %v, end = %v", metric, start, end)
    result := make(map[string]*MetricSeries)
    isHistogram := (metricCfg.Type == "histogram")

    if start >= end {
        log.Debugf("start >= end for metric %v; start = %v, end = %v", metric, start, end)
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
            log.Debugf("Error parsing value %v for metric %v : %+v", valStr, metric, err)
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
        log.Warnf("While evaluating metric %v from row %v to row %v datarows were skipped : %v parsing errors, %v nans, %v infs", metric, start, end, parsingErrors, nans, infs)
    }

    return result
}