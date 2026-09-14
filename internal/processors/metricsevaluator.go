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

package processors

import (
	"log_exporter/internal/collectors"
	"log_exporter/internal/config"
	"log_exporter/internal/evaluator"
	"log_exporter/internal/evaluator/enrichers"
	"log_exporter/internal/queues"
	"log_exporter/internal/registry"
	"log_exporter/internal/selfmonitor"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"math"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

type MetricsEvaluationProcessor struct {
	appConfig       *config.Config
	gdQueue         *queues.GDQueue
	gmQueue         *queues.GMQueue
	deRegistry      *registry.DERegistry
	counterVecs     map[string]*collectors.CustomCounter
	gaugeVecs       map[string]*collectors.CustomGauge
	histogramVecs   map[string]*collectors.CustomHistogram
	metricEvaluator *evaluator.Evaluator
}

var emptyStringMap = make(map[string]string)

func NewMetricsEvaluationProcessor(appConfig *config.Config, gdQueue *queues.GDQueue, gmQueue *queues.GMQueue, deRegistry *registry.DERegistry) *MetricsEvaluationProcessor {
	result := MetricsEvaluationProcessor{
		appConfig:  appConfig,
		gdQueue:    gdQueue,
		gmQueue:    gmQueue,
		deRegistry: deRegistry,
	}

	result.counterVecs = make(map[string]*collectors.CustomCounter)
	result.gaugeVecs = make(map[string]*collectors.CustomGauge)
	result.histogramVecs = make(map[string]*collectors.CustomHistogram)
	result.metricEvaluator = evaluator.CreateEvaluator(appConfig)

	result.initPrometheus()

	return &result
}

func (mep *MetricsEvaluationProcessor) Start() {
	log.Info("MetricsEvaluationProcessor : Start()")
	for queryName := range mep.appConfig.Queries {
		go mep.startGoroutine(queryName)
		mep.selfMonitorIncPanicRecoveries(queryName, 0.0, time.Now())
	}
	log.Info("MetricsEvaluationProcessor : Start() finished")
}

func (mep *MetricsEvaluationProcessor) startGoroutine(queryName string) {
	defer log.WithField("query", queryName).Info("MetricsEvaluationProcessor : Goroutine for the query is finished")
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).WithFields(log.Fields{"query": queryName, "panic": rec, "stacktrace": string(debug.Stack())}).Error("MetricsEvaluationProcessor : Panic during evaluation for the query")
			time.Sleep(time.Second * 5)
			log.WithField("query", queryName).Info("MetricsEvaluationProcessor : Starting goroutine for the query again ...")
			go mep.startGoroutine(queryName)
			mep.selfMonitorIncPanicRecoveries(queryName, 1.0, time.Now())
		}
	}()
	log.WithField("query", queryName).Info("MetricsEvaluationProcessor : Goroutine for the query is started")
	for {
		graylogData, ok := mep.gdQueue.Get(queryName)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).WithField("query", queryName).Error("MetricsEvaluationProcessor : Chan is closed for the query, stopping goroutine")
			return
		}
		if graylogData == nil {
			log.WithField(ec.FIELD, ec.LME_1604).WithField("query", queryName).Error("MetricsEvaluationProcessor : Nil graylogData received for the query")
			continue
		}
		enrichers.Enrich(queryName, graylogData, mep.appConfig.Queries[queryName])
		mep.updateMetrics(queryName, graylogData.Data, graylogData.EndTime)
		if mep.gmQueue != nil {
			metricFamilies := utils.CopyMetricFamiliesFromRegistry(mep.deRegistry.GetRegistry(queryName), queryName)
			if len(metricFamilies) > 0 {
				mep.gmQueue.Put(queryName, metricFamilies, true)
			}
		}
	}
}

func (mep *MetricsEvaluationProcessor) initPrometheus() {
	appConfig := mep.appConfig
	deRegistry := mep.deRegistry

	for queryName, queryCfg := range appConfig.Queries {
		for _, metricName := range queryCfg.Metrics {
			metricCfg := appConfig.Metrics[metricName]
			mep.initMetric(metricName, queryName)
			for _, childMetricName := range metricCfg.ChildMetrics {
				mep.initMetric(childMetricName, queryName)
			}
		}
	}
	prometheus.DefaultGatherer = deRegistry
}

func (mep *MetricsEvaluationProcessor) initMetric(metricName string, queryName string) {
	appConfig := mep.appConfig
	deRegistry := mep.deRegistry
	metricCfg := appConfig.Metrics[metricName]
	if metricCfg == nil {
		log.WithFields(log.Fields{"metric": metricName, "query": queryName}).Error("The metric doesn't have configuration, but the query references it. The metric initialization is skipped.")
		return
	}
	labelsList := metricCfg.Labels
	constLabels := mep.getConstLabels(metricCfg)
	switch metricCfg.Type {
	case "gauge":
		gaugeVec := collectors.NewCustomGauge(
			prometheus.NewDesc(
				metricName,
				metricCfg.Description,
				labelsList,
				constLabels,
			),
		)
		deRegistry.MustRegister(queryName, gaugeVec)
		mep.gaugeVecs[metricName] = gaugeVec
		log.WithFields(log.Fields{"metric": metricName, "labels_list": labelsList, "const_labels": constLabels}).Info("gaugeVec registered")
	case "counter":
		counterVec := collectors.NewCustomCounter(
			prometheus.NewDesc(
				metricName,
				metricCfg.Description,
				labelsList,
				constLabels,
			),
		)
		deRegistry.MustRegister(queryName, counterVec)
		mep.counterVecs[metricName] = counterVec
		initCounterMetric(metricCfg, metricName, counterVec)
		log.WithFields(log.Fields{"metric": metricName, "labels_list": labelsList, "const_labels": constLabels}).Info("counterVec registered")
	case "histogram":
		customHistogram := collectors.NewCustomHistogram(
			prometheus.NewDesc(
				metricName,
				metricCfg.Description,
				labelsList,
				constLabels,
			),
		)
		deRegistry.MustRegister(queryName, customHistogram)
		mep.histogramVecs[metricName] = customHistogram
		initHistogramMetric(metricCfg, metricName, customHistogram)
		log.WithFields(log.Fields{"metric": metricName, "labels_list": labelsList}).Info("customHistogram registered")
	default:
		log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"metric": metricName, "metric_cfg_type": metricCfg.Type}).Error("The metric has an unsupported type")
	}
}

func initCounterMetric(metricConfig *config.MetricsConfig, metricName string, counterVec *collectors.CustomCounter) {
	initValue := metricConfig.Parameters["init-value"]
	if initValue == "" {
		log.WithField("metric", metricName).Info("Parameter init-value is not set for the counter metric")
		return
	}
	if len(metricConfig.Labels) == 0 {
		if strings.ToUpper(initValue) == "NAN" {
			counterVec.Add(math.NaN(), emptyStringMap, metricConfig.Labels, nil)
			log.WithField("metric", metricName).Info("The metric is initialized with value NaN")
		} else {
			initValueFloat, err := strconv.ParseFloat(initValue, 64)
			if err == nil {
				if initValueFloat >= 0 {
					counterVec.Add(initValueFloat, emptyStringMap, metricConfig.Labels, nil)
					log.WithFields(log.Fields{"metric": metricName, "init_value": initValue}).Info("The metric is initialized with the init value")
				} else {
					log.WithFields(log.Fields{"metric": metricName, "init_value": initValue}).Warn("The counter metric can not be initialized with a negative value")
				}
			} else {
				log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"init_value": initValue, "metric": metricName, "error": err}).Error("Error parsing init-value for the metric")
			}
		}
	} else if len(metricConfig.ExpectedLabels) > 0 {
		initValueFloat, err := strconv.ParseFloat(initValue, 64)
		if err == nil {
			if initValueFloat < 0 {
				log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"metric": metricName, "init_value": initValue}).Error("The counter metric can not be initialized with a negative value")
				return
			}
		} else {
			log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"init_value": initValue, "metric": metricName, "error": err}).Error("Error parsing init-value for the metric")
			return
		}
		for itemNum, expectedLabelsItem := range metricConfig.ExpectedLabels {
			cartesian := utils.LabelsCartesian(expectedLabelsItem)
			log.WithFields(log.Fields{"metric": metricName, "item_num": itemNum, "cartesian": cartesian}).Info("Expected labels cartesian generated for the metric")
			for _, labels := range cartesian {
				counterVec.Add(initValueFloat, labels, metricConfig.Labels, nil)
			}
		}
	} else {
		log.WithField(ec.FIELD, ec.LME_8102).WithField("metric", metricName).Error("The metric can not be initialized because it has labels and expected labels are not defined")
	}
}

func initHistogramMetric(metricConfig *config.MetricsConfig, metricName string, customHistogram *collectors.CustomHistogram) {
	initValue := metricConfig.Parameters["init-value"]
	if initValue == "" {
		log.WithField("metric", metricName).Info("Parameter init-value is not set for the histogram metric")
		return
	}
	buckets := make(map[float64]uint64)
	for _, bucketKey := range metricConfig.Buckets {
		buckets[bucketKey] = 0
	}
	buckets[math.Inf(1.0)] = 0
	if len(metricConfig.Labels) == 0 {
		customHistogram.Observe(0, 0, buckets, emptyStringMap, metricConfig.Labels, nil)
		log.WithField("metric", metricName).Info("The histogram without labels is initialized")
	} else if len(metricConfig.ExpectedLabels) > 0 {
		for itemNum, expectedLabelsItem := range metricConfig.ExpectedLabels {
			cartesian := utils.LabelsCartesian(expectedLabelsItem)
			log.WithFields(log.Fields{"metric": metricName, "item_num": itemNum, "cartesian": cartesian}).Info("Expected labels cartesian generated for the metric")
			for _, labels := range cartesian {
				customHistogram.Observe(0, 0, buckets, labels, metricConfig.Labels, nil)
			}
		}
	} else {
		log.WithField(ec.FIELD, ec.LME_8102).WithField("metric", metricName).Error("The metric can not be initialized because it has labels and expected labels are not defined")
	}
}

func (mep *MetricsEvaluationProcessor) getConstLabels(metricCfg *config.MetricsConfig) map[string]string {
	labels := make(map[string]string)
	for label, labelValue := range mep.appConfig.Datasources[mep.appConfig.DsName].Labels {
		labels[label] = labelValue
	}
	for label, labelValue := range metricCfg.ConstLabels {
		labels[label] = labelValue
	}
	return labels
}

func (mep *MetricsEvaluationProcessor) updateMetrics(qName string, queryResult [][]string, endTime time.Time) {
	qCfg := mep.appConfig.Queries[qName]
	mep.deRegistry.Lock()
	defer mep.deRegistry.Unlock()
	for _, metric := range qCfg.Metrics {
		metricCfg := mep.appConfig.Metrics[metric]
		mer := mep.metricEvaluator.EvaluateMetric(queryResult, metric, metricCfg, qName, &endTime)
		if mer == nil {
			continue // if metric evaluation result is nil, error happened during metric evaluation and it has been already logged.
		}
		mep.updateMetricBySeries(mer.Series, metric, metricCfg)
		for childMetric, cmer := range mer.ChildMetrics {
			childMetricCfg := mep.appConfig.Metrics[childMetric]
			mep.updateMetricBySeries(cmer.Series, childMetric, childMetricCfg)
		}
	}
}

func (mep *MetricsEvaluationProcessor) updateMetricBySeries(metricSeries []evaluator.MetricSeries, metric string, metricCfg *config.MetricsConfig) {
	switch metricCfg.Type {
	case "counter":
		for _, ms := range metricSeries {
			counterVec := mep.counterVecs[metric]
			counterVec.Add(ms.Sum, ms.Labels, metricCfg.Labels, ms.Timestamp)
		}
	case "gauge":
		for _, ms := range metricSeries {
			gaugeVec := mep.gaugeVecs[metric]
			gaugeVec.Set(ms.Average, ms.Labels, metricCfg.Labels, ms.Timestamp)
		}
	case "histogram":
		for _, ms := range metricSeries {
			histogramVec := mep.histogramVecs[metric]
			histValue := ms.HistValue
			if histValue == nil {
				log.WithField(ec.FIELD, ec.LME_1604).WithFields(log.Fields{"metric": metric, "labels": ms.Labels}).Error("Error evaluating the histogram metric, histValue is nil")
			} else {
				histogramVec.Observe(histValue.Sum, histValue.Cnt, histValue.Buckets, ms.Labels, metricCfg.Labels, ms.Timestamp)
			}
		}
	default:
		log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"metric": metric, "metric_cfg_type": metricCfg.Type}).Error("The metric has an unsupported type")
	}
}

func (mep *MetricsEvaluationProcessor) selfMonitorIncPanicRecoveries(qName string, value float64, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["process_name"] = "MetricsEvaluationProcessor"
	selfmonitor.IncPanicRecoveriesCount(labels, value, &timestamp)
}
