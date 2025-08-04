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
	log.Debugf("evaluateDurationMetric %v", metric)
	seriesMap := make(map[string]*MetricSeries)
	metricState := e.monState.Get(metric)
	isHistogram := (metricCfg.Type == "histogram")

	if metricState == nil {
		log.Warnf("MetricState is empty for %v", metric)
		metricState = CreateMetricState()
		e.monState.Set(metric, metricState)
	}
	result := CreateMetricEvaluationResult(metricState.Size())

	defer func() {
		result = e.performMetricPostEvaluationSteps(result, metricState, seriesMap, metric, metricCfg)
	}()

	intCalls, err := e.evaluateDurationIntCalls(data, metric, query)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_1020).Errorf("Error evaluating duration metric %v : %+v", metric, err)
		return result
	}

	var nans, infs int64
	for correlationId, intCall := range intCalls {
		olv := intCall.OrderedLabelValues
		reqTime := intCall.RequestTime
		respTime := intCall.ResponseTime
		log.Debugf("Processing correlationId %v, olv = %v, reqTime = %v, respTime = %v", correlationId, olv, reqTime, respTime)
		if reqTime == 0 || respTime == 0 {
			continue
		}
		duration := float64(respTime-reqTime) / 1000

		if math.IsNaN(duration) {
			log.Warnf("Got NaN duration for metric %v : skipping it", metric)
			continue
		}
		if math.IsInf(duration, 0) {
			log.Warnf("Got %v duration for metric %v : skipping it", duration, metric)
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
		log.Warnf("While evaluating duration metric %v some intCalls were skipped : %v nans, %v infs", metric, nans, infs)
	}

	log.Debugf("seriesMap = %+v for metric %v", seriesMap, metric)

	for olv, ms := range seriesMap {
		labels := metricState.Get(olv)
		if labels == nil {
			labels = generateLabelValueMapFromOLV(olv, metricCfg.Labels)
			metricState.Set(olv, labels)
			log.Debugf("Generate new metricState values %v for metric %v", olv, metric)
		}
		ms.Labels = labels
		ms.Average = ms.Sum / float64(ms.Count)
		result.Series = append(result.Series, *ms)
	}

	if metricCfg.HasDurationNoResponseChild {
		log.Debugf("Metric %v has DurationNoResponseChild", metric)
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
	log.Debugf("evaluateDurationNoResponseMetric %v", metric)
	metricState := e.monState.Get(metric)
	var metricSeriesMap map[string]*MetricSeries

	if log.IsLevelEnabled(log.DebugLevel) {
		log.Debugf("Int calls for duration-no-response metric %v :", metric)
		for correlationId, intCall := range intCalls {
			log.Debugf("### correlationId %v, reqTime %v, respTime %v, olv %v", correlationId, intCall.RequestTime, intCall.ResponseTime, intCall.OrderedLabelValues)
		}
	}
	if metricState == nil {
		log.Warnf("MetricState is empty for %v", metric)
		metricState = CreateMetricState()
		e.monState.Set(metric, metricState)
	}
	result := CreateMetricEvaluationResult(metricState.Size())

	defer func() {
		result = e.performMetricPostEvaluationSteps(result, metricState, metricSeriesMap, metric, metricCfg)
	}()

	nrc := e.nrcRepo.GetCache(metric)
	if nrc == nil {
		log.WithField(ec.FIELD, ec.LME_1604).Errorf("Duration-no-response metric %v can not be evaluated, because no-response-cache is nil", metric)
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
			log.Debugf("Generate new metricState values %v for metric %v", olv, metric)
		}
		ms.Labels = labels
		result.Series = append(result.Series, *ms)
	}

	if log.IsLevelEnabled(log.DebugLevel) {
		log.Debugf("Put new batch to no-response-cache for metric %v :", metric)
		for correlationId, cachedRequest := range nrCacheBatch.cache {
			log.Debugf("### correlationId %v, time %v, olv %v, hasResponse %v", correlationId, cachedRequest.Time, *cachedRequest.Olv, cachedRequest.HasResponse)
		}
	}

	nrc.PutBatchToCache(nrCacheBatch)

	return result
}

func (e *Evaluator) evaluateDurationIntCalls(data [][]string, metric string, query string) (intCalls map[string]*IntCall, err error) {
	log.Debugf("evaluateDurationIntCalls %v", metric)
	metricCfg := e.appConfig.Metrics[metric]
	intCalls = make(map[string]*IntCall)

	dataSize := len(data)
	if dataSize == 0 {
		log.Debugf("DataSize == 0 for metric %v", metric)
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
	log.Debugf("For metric %v got labelIndexes = %+v; heading = %v", metric, labelIndexes, heading)

	for i := 1; i < dataSize; i++ {
		var unixTime int64
		var err error
		if timeFormat == "" {
			unixTime, err = strconv.ParseInt(data[i][timeIndex], 10, 64)
			if err != nil {
				log.Debugf("For metric %v : Error parsing value %v : %+v", metric, data[i][timeIndex], err)
				continue
			}
		} else {
			timestamp, err := time.Parse(timeFormat, data[i][timeIndex])
			if err != nil {
				log.Debugf("For metric %v : Error parsing value %v with format %v: %+v", metric, data[i][timeIndex], timeFormat, err)
				continue
			} else {
				unixTime = timestamp.UnixNano() / 1000000
				log.Tracef("For metric %v : Value %v parsed successfully to %v with format %v", metric, data[i][timeIndex], unixTime, timeFormat)
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
			log.WithField(ec.FIELD, ec.LME_1020).Errorf("Wrong messageType %v for metric %v", messageType, metric)
		}
	}

	cache := e.rtcRepo.GetCache(query, cacheName)
	newBatchToCache := make(map[string]int64)

	for correlationId, intCall := range intCalls {
		olv := intCall.OrderedLabelValues
		reqTime := intCall.RequestTime
		respTime := intCall.ResponseTime
		log.Debugf("Processing correlationId %v, olv = %v, reqTime = %v, respTime = %v", correlationId, olv, reqTime, respTime)
		if reqTime == 0 {
			if cache == nil {
				log.Debugf("RequestTime is not set for %v. Skipping", correlationId)
				continue
			} else {
				log.Tracef("RequestTime is not set for %v. Trying to find it in cache", correlationId)
				reqTime = cache.SearchRequestTimeInCache(correlationId)
				if reqTime == 0 {
					log.Debugf("RequestTime is not set in cache for %v. Skipping", correlationId)
					continue
				} else {
					log.Tracef("RequestTime %v is found for %v in cache", reqTime, correlationId)
					intCall.RequestTime = reqTime
				}
			}
		}
		if respTime == 0 {
			if isCacheUpdate && cache != nil {
				log.Debugf("RequestTime %v put to cache for %v", reqTime, correlationId)
				newBatchToCache[correlationId] = reqTime
			}
		}
	}

	if isCacheUpdate && cache != nil {
		log.Debugf("Updating cache %v for query %v from metric %v", cacheName, query, metric)
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
