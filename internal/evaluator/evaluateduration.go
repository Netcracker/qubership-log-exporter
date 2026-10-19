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
	"math"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)

func (e *Evaluator) evaluateDurationMetric(data [][]string, metric string, metricCfg *config.MetricsConfig, query string) *MetricEvaluationResult {
	log.WithField("metric", metric).Debug("evaluateDurationMetric")
	seriesMap := make(map[string]*MetricSeries)
	metricState := e.monState.Get(metric)
	isHistogram := (metricCfg.Type == "histogram")

	if metricState == nil {
		log.WithField("metric", metric).Warn("MetricState is empty")
		metricState = CreateMetricState()
		e.monState.Set(metric, metricState)
	}
	result := CreateMetricEvaluationResult(metricState.Size())

	defer func() {
		result = e.performMetricPostEvaluationSteps(result, metricState, seriesMap, metric, metricCfg)
	}()

	intCalls, err := e.evaluateDurationIntCalls(data, metric, query)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_1020).WithFields(log.Fields{"metric": metric, "error": err}).Error("Error evaluating duration metric")
		return result
	}

	var nans, infs int64
	for correlationId, intCall := range intCalls {
		olv := intCall.OrderedLabelValues
		reqTime := intCall.RequestTime
		respTime := intCall.ResponseTime
		log.WithFields(log.Fields{"correlation_id": correlationId, "olv": olv, "req_time": reqTime, "resp_time": respTime}).Debug("Processing int call")
		if reqTime == 0 || respTime == 0 {
			continue
		}
		duration := float64(respTime-reqTime) / 1000

		if math.IsNaN(duration) {
			log.WithField("metric", metric).Warn("Got NaN duration, skipping it")
			continue
		}
		if math.IsInf(duration, 0) {
			log.WithFields(log.Fields{"duration": duration, "metric": metric}).Warn("Got infinite duration, skipping it")
			continue
		}

		ms := seriesMap[olv]
		if ms == nil {
			ms = &MetricSeries{}
			if isHistogram {
				ms.HistValue = CreateHistogramMetricValue(metricCfg.Buckets)
			}
			seriesMap[olv] = ms
		}
		ms.Count++
		ms.Sum += duration
		if isHistogram {
			ms.HistValue.Observe(duration)
		}
	}
	if nans != 0 || infs != 0 {
		log.WithFields(log.Fields{"metric": metric, "nans": nans, "infs": infs}).Warn("Some intCalls were skipped while evaluating the duration metric")
	}

	log.WithFields(log.Fields{"series_map": seriesMap, "metric": metric}).Debug("Evaluated the series map")

	for olv, ms := range seriesMap {
		labels := metricState.Get(olv)
		if labels == nil {
			labels = generateLabelValueMapFromOLV(olv, metricCfg.Labels)
			metricState.Set(olv, labels)
			log.WithFields(log.Fields{"olv": olv, "metric": metric}).Debug("Generate new metricState values")
		}
		ms.Labels = labels
		ms.Average = ms.Sum / float64(ms.Count)
		result.Series = append(result.Series, *ms)
	}

	if metricCfg.HasDurationNoResponseChild {
		log.WithField("metric", metric).Debug("Metric has DurationNoResponseChild")
		result.ChildMetrics = make(map[string]*MetricEvaluationResult)
		for _, childMetric := range metricCfg.ChildMetrics {
			childMetricCfg := e.appConfig.Metrics[childMetric]
			if childMetricCfg.Operation == "duration-no-response" {
				result.ChildMetrics[childMetric] = e.evaluateDurationNoResponseMetric(intCalls, childMetric, childMetricCfg)
			}
		}
	}

	return result
}

func (e *Evaluator) evaluateDurationNoResponseMetric(intCalls map[string]*IntCall, metric string, metricCfg *config.MetricsConfig) *MetricEvaluationResult {
	log.WithField("metric", metric).Debug("evaluateDurationNoResponseMetric")
	metricState := e.monState.Get(metric)
	var metricSeriesMap map[string]*MetricSeries

	if log.IsLevelEnabled(log.DebugLevel) {
		log.WithField("metric", metric).Debug("Int calls for the duration-no-response metric")
		for correlationId, intCall := range intCalls {
			log.WithFields(log.Fields{"correlation_id": correlationId, "request_time": intCall.RequestTime, "response_time": intCall.ResponseTime, "ordered_label_values": intCall.OrderedLabelValues}).Debug("### Int call")
		}
	}
	if metricState == nil {
		log.WithField("metric", metric).Warn("MetricState is empty")
		metricState = CreateMetricState()
		e.monState.Set(metric, metricState)
	}
	result := CreateMetricEvaluationResult(metricState.Size())

	defer func() {
		result = e.performMetricPostEvaluationSteps(result, metricState, metricSeriesMap, metric, metricCfg)
	}()

	nrc := e.nrcRepo.GetCache(metric)
	if nrc == nil {
		log.WithField(ec.FIELD, ec.LME_1604).WithField("metric", metric).Error("Duration-no-response metric can not be evaluated, because no-response-cache is nil")
		return nil
	}

	nrCacheBatch := CreateNRCacheBatch()
	for correlationId, intCall := range intCalls {
		if intCall.RequestTime == 0 && intCall.ResponseTime == 0 {
			continue
		}
		if intCall.ResponseTime == 0 {
			nrCacheBatch.PutCachedResult(correlationId, intCall.RequestTime, &intCall.OrderedLabelValues, false)
			continue
		}
		nrc.MarkAsHasResponse(correlationId)
	}
	metricSeriesMap = nrc.CountNoResponseInTheLastBatchByOLV()

	for olv, ms := range metricSeriesMap {
		labels := metricState.Get(olv)
		if labels == nil {
			labels = generateLabelValueMapFromOLV(olv, metricCfg.Labels)
			metricState.Set(olv, labels)
			log.WithFields(log.Fields{"olv": olv, "metric": metric}).Debug("Generate new metricState values")
		}
		ms.Labels = labels
		result.Series = append(result.Series, *ms)
	}

	if log.IsLevelEnabled(log.DebugLevel) {
		log.WithField("metric", metric).Debug("Put new batch to no-response-cache")
		for correlationId, cachedRequest := range nrCacheBatch.cache {
			log.WithFields(log.Fields{"correlation_id": correlationId, "time": cachedRequest.Time, "olv": *cachedRequest.Olv, "has_response": cachedRequest.HasResponse}).Debug("### Cached request")
		}
	}

	nrc.PutBatchToCache(nrCacheBatch)

	return result
}

func (e *Evaluator) evaluateDurationIntCalls(data [][]string, metric string, query string) (intCalls map[string]*IntCall, err error) {
	log.WithField("metric", metric).Debug("evaluateDurationIntCalls")
	metricCfg := e.appConfig.Metrics[metric]
	intCalls = make(map[string]*IntCall)

	dataSize := len(data)
	if dataSize == 0 {
		log.WithField("metric", metric).Debug("DataSize == 0")
		return intCalls, nil
	}

	heading := data[0]
	timeField := metricCfg.Parameters["time_field"]
	timeFormat := metricCfg.Parameters["time_format"]
	messageTypeField := metricCfg.Parameters["message_type_field"]
	messageTypeRequest := metricCfg.Parameters["message_type_request"]
	messageTypeResponse := metricCfg.Parameters["message_type_response"]
	correlationIdField := metricCfg.Parameters["correlation_id_field"]
	cacheName := metricCfg.Parameters["cache"]
	isCacheUpdate := (metricCfg.Parameters["cache-update"] == "true")
	if timeField == "" {
		return intCalls, fmt.Errorf("IntCalls for metric %v can't be calculated : parameter time_field not set", metric)
	}
	if messageTypeField == "" {
		return intCalls, fmt.Errorf("IntCalls for metric %v can't be calculated : parameter message_type_field not set", metric)
	}
	if messageTypeRequest == "" {
		messageTypeRequest = "request"
	}
	if messageTypeResponse == "" {
		messageTypeResponse = "response"
	}
	if correlationIdField == "" {
		return intCalls, fmt.Errorf("intCalls for metric %v can't be calculated : parameter correlation_id_field not set", metric)
	}

	timeIndex := utils.FindStringIndexInArray(heading, timeField)
	if timeIndex == -1 {
		return intCalls, fmt.Errorf("can not evaluate duration metric %v : field %v not found in the output", metric, timeField)
	}
	messageTypeIndex := utils.FindStringIndexInArray(heading, messageTypeField)
	if messageTypeIndex == -1 {
		return intCalls, fmt.Errorf("can not evaluate duration metric %v : field %v not found in the output", metric, messageTypeField)
	}
	correlationIdIndex := utils.FindStringIndexInArray(heading, correlationIdField)
	if correlationIdIndex == -1 {
		return intCalls, fmt.Errorf("can not evaluate duration metric %v : field %v not found in the output", metric, correlationIdField)
	}

	labelIndexes, err := evaluateLabelSourceFieldIndexes(metricCfg, heading)
	if err != nil {
		return intCalls, fmt.Errorf("can not evaluate duration metric %v : %+v", metric, err)
	}
	log.WithFields(log.Fields{"metric": metric, "label_indexes": labelIndexes, "heading": heading}).Debug("Evaluated the label indexes")

	for i := 1; i < dataSize; i++ {
		var unixTime int64
		var err error
		if timeFormat == "" {
			unixTime, err = strconv.ParseInt(data[i][timeIndex], 10, 64)
			if err != nil {
				log.WithFields(log.Fields{"metric": metric, "time_field_value": data[i][timeIndex], "error": err}).Debug("Error parsing the time value")
				continue
			}
		} else {
			timestamp, err := time.Parse(timeFormat, data[i][timeIndex])
			if err != nil {
				log.WithFields(log.Fields{"metric": metric, "time_field_value": data[i][timeIndex], "time_format": timeFormat, "error": err}).Debug("Error parsing the time value with the configured format")
				continue
			} else {
				unixTime = timestamp.UnixNano() / 1000000
				log.WithFields(log.Fields{"metric": metric, "time_field_value": data[i][timeIndex], "unix_time": unixTime, "time_format": timeFormat}).Trace("Value parsed successfully")
			}
		}
		messageType := data[i][messageTypeIndex]
		correlationId := data[i][correlationIdIndex]
		switch messageType {
		case messageTypeRequest:
			if intCalls[correlationId] == nil {
				olv := generateOrderedLabelValuesString(labelIndexes, data[i])
				intCalls[correlationId] = CreateIntCall(unixTime, 0, olv)
			} else {
				intCalls[correlationId].RequestTime = unixTime
			}
		case messageTypeResponse:
			olv := generateOrderedLabelValuesString(labelIndexes, data[i])
			if intCalls[correlationId] == nil {
				intCalls[correlationId] = CreateIntCall(0, unixTime, olv)
			} else {
				intCalls[correlationId].ResponseTime = unixTime
				intCalls[correlationId].OrderedLabelValues = olv
			}
		default:
			log.WithField(ec.FIELD, ec.LME_1020).WithFields(log.Fields{"message_type": messageType, "metric": metric}).Error("Wrong messageType")
		}
	}

	cache := e.rtcRepo.GetCache(query, cacheName)
	newBatchToCache := make(map[string]int64)

	for correlationId, intCall := range intCalls {
		olv := intCall.OrderedLabelValues
		reqTime := intCall.RequestTime
		respTime := intCall.ResponseTime
		log.WithFields(log.Fields{"correlation_id": correlationId, "olv": olv, "req_time": reqTime, "resp_time": respTime}).Debug("Processing int call")
		if reqTime == 0 {
			if cache == nil {
				log.WithField("correlation_id", correlationId).Debug("RequestTime is not set. Skipping")
				continue
			} else {
				log.WithField("correlation_id", correlationId).Trace("RequestTime is not set. Trying to find it in cache")
				reqTime = cache.SearchRequestTimeInCache(correlationId)
				if reqTime == 0 {
					log.WithField("correlation_id", correlationId).Debug("RequestTime is not set in cache. Skipping")
					continue
				} else {
					log.WithFields(log.Fields{"req_time": reqTime, "correlation_id": correlationId}).Trace("RequestTime is found in cache")
					intCall.RequestTime = reqTime
				}
			}
		}
		if respTime == 0 {
			if isCacheUpdate && cache != nil {
				log.WithFields(log.Fields{"req_time": reqTime, "correlation_id": correlationId}).Debug("RequestTime put to cache")
				newBatchToCache[correlationId] = reqTime
			}
		}
	}

	if isCacheUpdate && cache != nil {
		log.WithFields(log.Fields{"cache_name": cacheName, "query": query, "metric": metric}).Debug("Updating cache")
		cache.PutBatchToCache(newBatchToCache)
		selfMonitorUpdateCacheSize(query, cacheName, float64(cache.Size()))
	}

	return intCalls, nil
}

func selfMonitorUpdateCacheSize(qName string, cacheName string, value float64) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["cache_name"] = cacheName
	selfmonitor.UpdateDataExporterCacheSize(labels, value)
}
