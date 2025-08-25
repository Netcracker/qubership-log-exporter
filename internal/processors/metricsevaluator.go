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
	defer log.Infof("MetricsEvaluationProcessor : Goroutine for query %v is finished", queryName)
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).Errorf("MetricsEvaluationProcessor : Panic during evaluation for query %v : %+v ; Stacktrace of the panic : %v", queryName, rec, string(debug.Stack()))
			time.Sleep(time.Second * 5)
			log.Infof("MetricsEvaluationProcessor : Starting gouroutine for query %v again ...", queryName)
			go mep.startGoroutine(queryName)
			mep.selfMonitorIncPanicRecoveries(queryName, 1.0, time.Now())
		}
	}()
	log.Infof("MetricsEvaluationProcessor : Goroutine for query %v is started", queryName)
	for {
		graylogData, ok := mep.gdQueue.Get(queryName)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).Errorf("MetricsEvaluationProcessor : Chan is closed for the query %v, stopping goroutine", queryName)
			return
		}
		if graylogData == nil {
			log.WithField(ec.FIELD, ec.LME_1604).Errorf("MetricsEvaluationProcessor : Nil graylogData received for query %v", queryName)
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
		log.Errorf("Metric %v doesn't have configuration, but query %v references to the metric. The metric initialization is skipped.", metricName, queryName)
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
		log.Infof("gaugeVec %v registered with labels %+v and constLabels %+v", metricName, labelsList, constLabels)
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
		log.Infof("counterVec %v registered with labels %+v and constLabels %+v", metricName, labelsList, constLabels)
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
		log.Infof("customHistogram %v registered with labels %+v", metricName, labelsList)
	default:
		log.WithField(ec.FIELD, ec.LME_8102).Errorf("Metric %v has not supported type %v", metricName, metricCfg.Type)
	}
}

func initCounterMetric(metricConfig *config.MetricsConfig, metricName string, counterVec *collectors.CustomCounter) {
	initValue := metricConfig.Parameters["init-value"]
	if initValue == "" {
		log.Infof("Parameter init-value is not set for metric %v (counter)", metricName)
		return
	}
	if len(metricConfig.Labels) == 0 {
		if strings.ToUpper(initValue) == "NAN" {
			counterVec.Add(math.NaN(), emptyStringMap, metricConfig.Labels, nil)
			log.Infof("Metric %v is initialized with value NaN", metricName)
		} else {
			initValueFloat, err := strconv.ParseFloat(initValue, 64)
			if err == nil {
				if initValueFloat >= 0 {
					counterVec.Add(initValueFloat, emptyStringMap, metricConfig.Labels, nil)
					log.Infof("Metric %v is initialized with value %v", metricName, initValue)
				} else {
					log.Warnf("Counter metric %v can not be initialized with negative value %v", metricName, initValue)
				}
			} else {
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("Error parsing init-value %v for metric %v : %+v", initValue, metricName, err)
			}
		}
	} else if len(metricConfig.ExpectedLabels) > 0 {
		initValueFloat, err := strconv.ParseFloat(initValue, 64)
		if err == nil {
			if initValueFloat < 0 {
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("Counter metric %v can not be initialized with negative value %v", metricName, initValue)
				return
			}
		} else {
			log.WithField(ec.FIELD, ec.LME_8102).Errorf("Error parsing init-value %v for metric %v : %+v", initValue, metricName, err)
			return
		}
		for itemNum, expectedLabelsItem := range metricConfig.ExpectedLabels {
			cartesian := utils.LabelsCartesian(expectedLabelsItem)
			log.Infof("For metric %v (itemNum %v) expected labels cartesian generated : %+v", metricName, itemNum, cartesian)
			for _, labels := range cartesian {
				counterVec.Add(initValueFloat, labels, metricConfig.Labels, nil)
			}
		}
	} else {
		log.WithField(ec.FIELD, ec.LME_8102).Errorf("Metric %v can not be initialized because it has labels and expected labels are not defined", metricName)
	}
}

func initHistogramMetric(metricConfig *config.MetricsConfig, metricName string, customHistogram *collectors.CustomHistogram) {
	initValue := metricConfig.Parameters["init-value"]
	if initValue == "" {
		log.Infof("Parameter init-value is not set for metric %v (histogram)", metricName)
		return
	}
	buckets := make(map[float64]uint64)
	for _, bucketKey := range metricConfig.Buckets {
		buckets[bucketKey] = 0
	}
	buckets[math.Inf(1.0)] = 0
	if len(metricConfig.Labels) == 0 {
		customHistogram.Observe(0, 0, buckets, emptyStringMap, metricConfig.Labels, nil)
		log.Infof("Histogram %v without labels is initialized", metricName)
	} else if len(metricConfig.ExpectedLabels) > 0 {
		for itemNum, expectedLabelsItem := range metricConfig.ExpectedLabels {
			cartesian := utils.LabelsCartesian(expectedLabelsItem)
			log.Infof("For metric %v (itemNum %v) expected labels cartesian generated : %+v", metricName, itemNum, cartesian)
			for _, labels := range cartesian {
				customHistogram.Observe(0, 0, buckets, labels, metricConfig.Labels, nil)
			}
		}
	} else {
		log.WithField(ec.FIELD, ec.LME_8102).Errorf("Metric %v can not be initialized because it has labels and expected labels are not defined", metricName)
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
				log.WithField(ec.FIELD, ec.LME_1604).Errorf("Error evaluating histogram metric %v for labels %v : histValue is nil", metric, ms.Labels)
			} else {
				histogramVec.Observe(histValue.Sum, histValue.Cnt, histValue.Buckets, ms.Labels, metricCfg.Labels, ms.Timestamp)
			}
		}
	default:
		log.WithField(ec.FIELD, ec.LME_8102).Errorf("Metric %v has not supported type %v", metric, metricCfg.Type)
	}
}

func (mep *MetricsEvaluationProcessor) selfMonitorIncPanicRecoveries(qName string, value float64, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["process_name"] = "MetricsEvaluationProcessor"
	selfmonitor.IncPanicRecoveriesCount(labels, value, &timestamp)
}
