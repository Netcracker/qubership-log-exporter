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
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"strings"

	log "github.com/sirupsen/logrus"
)

func (e *Evaluator) evaluateCountMetric(data [][]string, metric string, metricCfg *config.MetricsConfig) *MetricEvaluationResult {
	log.WithField("metric", metric).Debug("evaluateCountMetric")

	if metricCfg.Type == "histogram" {
		log.WithField(ec.FIELD, ec.LME_8102).WithField("metric", metric).Error("Metric has count operation and doesn't support histogram type")
		return nil
	}
	metricState := e.monState.Get(metric)
	var metricSeriesMap map[string]*MetricSeries

	if metricState == nil {
		log.WithField("metric", metric).Warn("MetricState is empty")
		metricState = CreateMetricState()
		e.monState.Set(metric, metricState)
	}

	dataSize := len(data)
	result := CreateMetricEvaluationResult(metricState.Size())

	defer func() {
		result = e.performMetricPostEvaluationSteps(result, metricState, metricSeriesMap, metric, metricCfg)
	}()

	if dataSize == 0 {
		log.WithField("metric", metric).Debug("DataSize == 0")
		return result
	}

	if len(metricCfg.Labels) == 0 {
		log.WithField("metric", metric).Debug("metricCfg.Labels == 0")
		ms := MetricSeries{}
		ms.Average = float64(dataSize - 1)
		ms.Sum = ms.Average
		ms.Count = uint64(dataSize - 1)
		ms.Labels = make(map[string]string, 0)
		metricSeriesMap = map[string]*MetricSeries{}
		metricSeriesMap[""] = &ms
		result.Series = append(result.Series, ms)
		return result
	}

	metricSeriesMap = e.evaluateCountMetricSeriesMapByOLV(data, metric, metricCfg)
	log.WithFields(log.Fields{"metric_series_map": metricSeriesMap, "metric": metric}).Debug("Evaluated the metric series map")

	for olv, ms := range metricSeriesMap {
		labels := metricState.Get(olv)
		if labels == nil {
			labels = generateLabelValueMapFromOLV(olv, metricCfg.Labels)
			metricState.Set(olv, labels)
			log.WithFields(log.Fields{"olv": olv, "metric": metric}).Debug("Generate new metricstate values")
		}
		ms.Labels = labels
		ms.Average = float64(ms.Count)
		ms.Sum = ms.Average
		result.Series = append(result.Series, *ms)
	}

	return result
}

func (e *Evaluator) evaluateCountMetricSeriesMapByOLV(data [][]string, metric string, metricCfg *config.MetricsConfig) map[string]*MetricSeries {
	log.WithField("metric", metric).Debug("evaluateCountMetricSeriesMapByOLV")
	dataSize := len(data)
	if dataSize < 2 {
		log.WithFields(log.Fields{"data_size": dataSize, "metric": metric}).Debug("Evaluation result is empty")
		return make(map[string]*MetricSeries)
	}

	heading := data[0]
	labelIndexes, err := evaluateLabelSourceFieldIndexes(metricCfg, heading)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_1020).WithFields(log.Fields{"metric": metric, "error": err}).Error("Can not evaluate count metric")
		return make(map[string]*MetricSeries)
	}
	var idFieldIndex = -1
	if metricCfg.IdField != "" {
		idFieldIndex = utils.FindStringIndexInArray(heading, metricCfg.IdField)
		if idFieldIndex >= 0 {
			e.idfcRepo.GetMetricIdFieldCache(metric).IncAge()
		}
	}
	var meCondition *MECondition
	if metricCfg.Cond != nil {
		meCondition = CreateMECondition(metric, metricCfg.Cond, heading)
	}
	log.WithFields(log.Fields{"label_indexes": labelIndexes, "heading": heading, "id_field_index": idFieldIndex, "metric": metric}).Debug("Evaluated the label indexes")

	threadsNumber := metricCfg.Threads
	if threadsNumber > dataSize-1 {
		threadsNumber = dataSize - 1
	}

	if threadsNumber <= 1 {
		return e.evaluateCountMetricSeriesMapByOLVTask(data, metric, metricCfg, labelIndexes, idFieldIndex, meCondition, 1, len(data))
	}
	msChan := make(chan map[string]*MetricSeries)
	for i := 0; i < threadsNumber; i++ {
		start := 1 + i*(dataSize-1)/threadsNumber
		end := 1 + (i+1)*(dataSize-1)/threadsNumber
		go func() {
			msm := e.evaluateCountMetricSeriesMapByOLVTask(data, metric, metricCfg, labelIndexes, idFieldIndex, meCondition, start, end)
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
				//log.WithFields(log.Fields{"olv": olv, "metric_series": metricSeries}).Debug("!!! result[olv] was null, creating new one")
			} else {
				//log.WithFields(log.Fields{"olv": olv, "result_metric_series": resultMetricSeries, "metric_series": metricSeries}).Debug("@@@ result[olv] was not null, adding to the old one")
				resultMetricSeries.Count += metricSeries.Count
				//log.WithFields(log.Fields{"olv": olv, "result_metric_series": resultMetricSeries}).Debug("### Sum result")
			}
		}
	}

	return result
}

const (
	UNIQ_ID_STRATEGY_NOT_DEFINED = 0
	UNIQ_ID_STRATEGY_METRIC      = 1
	UNIQ_ID_STRATEGY_LABEL       = 2
)

func (e *Evaluator) evaluateCountMetricSeriesMapByOLVTask(data [][]string, metric string, metricCfg *config.MetricsConfig, labelIndexes []int, idFieldIndex int, meCondition *MECondition, start int, end int) map[string]*MetricSeries {
	log.WithFields(log.Fields{"metric": metric, "start": start, "end": end}).Debug("evaluateCountMetricSeriesMapByOLVTask")
	result := make(map[string]*MetricSeries)

	if start >= end {
		log.WithFields(log.Fields{"metric": metric, "start": start, "end": end}).Debug("start >= end")
		return result
	}

	var uniqIdStrategy = UNIQ_ID_STRATEGY_NOT_DEFINED
	var cache *MetricIdFieldCache
	if idFieldIndex > 0 {
		cache = e.idfcRepo.GetMetricIdFieldCache(metric)
		if metricCfg.IdFieldStrategy == "metric" {
			uniqIdStrategy = UNIQ_ID_STRATEGY_METRIC
		} else {
			uniqIdStrategy = UNIQ_ID_STRATEGY_LABEL
		}
	}
	multiValueFieldsEnabled := len(metricCfg.MultiValueFields) > 0

	var multiValueFieldsIndexes []int
	var err error
	if multiValueFieldsEnabled {
		multiValueFieldsIndexes, err = evaluateMultiValueFieldIndexes(metricCfg, data[0])
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_1020).WithFields(log.Fields{"metric": metric, "error": err}).Error("Can not evaluate count metric")
			return result
		}
	}

	log.WithFields(log.Fields{"metric": metric, "uniq_id_strategy": uniqIdStrategy, "multi_value_fields_enabled": multiValueFieldsEnabled, "multi_value_fields_indexes": multiValueFieldsIndexes}).Debug("Count metric task configuration")

	for i := start; i < end; i++ {
		if meCondition != nil && !meCondition.Apply(data[i]) {
			continue
		}
		if uniqIdStrategy == UNIQ_ID_STRATEGY_METRIC {
			if cache.IsUsed(data[i][idFieldIndex]) {
				log.WithFields(log.Fields{"metric": metric, "id_field_value": data[i][idFieldIndex]}).Trace("Id field is used. Skipping metric update")
				continue
			} else {
				log.WithFields(log.Fields{"metric": metric, "id_field_value": data[i][idFieldIndex]}).Trace("Id field is not used yet. Updating the metric")
			}
		}
		if !multiValueFieldsEnabled {
			olv := generateOrderedLabelValuesString(labelIndexes, data[i])
			log.WithFields(log.Fields{"metric": metric, "olv": olv}).Trace("Generated the ordered label values")
			if uniqIdStrategy == UNIQ_ID_STRATEGY_LABEL {
				if cache.IsUsedForOLV(data[i][idFieldIndex], olv) {
					log.WithFields(log.Fields{"metric": metric, "id_field_value": data[i][idFieldIndex], "olv": olv}).Trace("Id field for the olv is used. Skipping metric update")
					continue
				} else {
					log.WithFields(log.Fields{"metric": metric, "id_field_value": data[i][idFieldIndex], "olv": olv}).Trace("Id field for the olv is not used yet. Updating the metric")
				}
			}
			ms := result[olv]
			if result[olv] == nil {
				ms = &MetricSeries{}
				result[olv] = ms
			}
			ms.Count++
		} else {
			olvs := generateOrderedLabelValuesStringList(labelIndexes, data[i], metricCfg, multiValueFieldsIndexes)
			log.WithFields(log.Fields{"metric": metric, "olvs": olvs}).Trace("Generated the ordered label values list")
			for _, olv := range olvs {
				if uniqIdStrategy == UNIQ_ID_STRATEGY_LABEL {
					if cache.IsUsedForOLV(data[i][idFieldIndex], olv) {
						log.WithFields(log.Fields{"metric": metric, "id_field_value": data[i][idFieldIndex], "olv": olv}).Trace("Id field for the olv is used. Skipping metric update")
						continue
					} else {
						log.WithFields(log.Fields{"metric": metric, "id_field_value": data[i][idFieldIndex], "olv": olv}).Trace("Id field for the olv is not used yet. Updating the metric")
					}
				}
				ms := result[olv]
				if result[olv] == nil {
					ms = &MetricSeries{}
					result[olv] = ms
				}
				ms.Count++
			}
		}
	}

	return result
}

func generateOrderedLabelValuesStringList(labelIndexes []int, row []string, metricCfg *config.MetricsConfig, multiValueFieldsIndexes []int) []string {
	startSize := len(labelIndexes)
	var start string
	var startList []string
	if startSize > 0 {
		startList = make([]string, startSize)
		for i := 0; i < startSize; i++ {
			startList[i] = row[labelIndexes[i]]
		}
	}
	start = strings.Join(startList, ";") // first part of olv without multivalues

	multiValueFieldsCount := len(metricCfg.MultiValueFields)
	multiValuesTable := make([][]string, 0, multiValueFieldsCount)

	for i, mvfc := range metricCfg.MultiValueFields {
		data := row[multiValueFieldsIndexes[i]]
		dataSplitted := strings.Split(data, mvfc.Separator)
		multiValuesRow := make([]string, 0, len(dataSplitted))
		for _, split := range dataSplitted {
			multiValuesRow = append(multiValuesRow, strings.TrimSpace(split))
		}
		multiValuesTable = append(multiValuesTable, multiValuesRow)
	}

	multiValuesSizes := make([]int64, 0, multiValueFieldsCount)
	for _, mvr := range multiValuesTable {
		multiValuesSizes = append(multiValuesSizes, int64(len(mvr)))
	}
	currentIndexes := make([]int64, multiValueFieldsCount)

	resultSize := utils.MultiplyArrayItems(multiValuesSizes) // total quantity of multi-label combinations
	result := make([]string, 0, resultSize)
	for {
		endList := make([]string, multiValueFieldsCount)
		for i := 0; i < multiValueFieldsCount; i++ {
			endList[i] = multiValuesTable[i][currentIndexes[i]]
		}
		end := strings.Join(endList, ";")

		result = append(result, start+";"+end)

		if utils.IncrementIndexes(currentIndexes, multiValuesSizes) {
			break
		}
	}

	return result
}

func evaluateMultiValueFieldIndexes(metricCfg *config.MetricsConfig, heading []string) ([]int, error) {
	fieldIndexes := make([]int, len(metricCfg.MultiValueFields))
	for i, mvfc := range metricCfg.MultiValueFields {
		fieldIndexes[i] = utils.FindStringIndexInArray(heading, mvfc.FieldName)
		if fieldIndexes[i] == -1 {
			return nil, fmt.Errorf("field %v not found in the output for multi-value label %v index evaluation", mvfc.FieldName, mvfc.LabelName)
		}
	}
	return fieldIndexes, nil
}
