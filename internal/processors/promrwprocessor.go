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
	"log_exporter/internal/config"
	"log_exporter/internal/httpservice"
	"log_exporter/internal/queues"
	"log_exporter/internal/selfmonitor"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"runtime/debug"
	"time"

	log "github.com/sirupsen/logrus"
)

type PromRemoteWriteProcessor struct {
	PushProcessor
	gmQueue       *queues.GMQueue
	promRWService *httpservice.PromRWService
}

func NewPromRemoteWriteProcessor(appConfig *config.Config, gmQueue *queues.GMQueue, promRWService *httpservice.PromRWService) *PromRemoteWriteProcessor {
	p := &PromRemoteWriteProcessor{
		gmQueue:       gmQueue,
		promRWService: promRWService,
	}
	p.appConfig = appConfig
	return p
}

func (vp *PromRemoteWriteProcessor) Start() {
	log.Info("PromRemoteWriteProcessor : Start()")
	if vp.gmQueue == nil {
		log.Info("PromRemoteWriteProcessor : Start() : vp.gmQueue == nil, PromRemoteWriteProcessor will be disabled")
		return
	}
	for queryName := range vp.appConfig.Queries {
		go vp.startGoroutine(queryName)
		vp.selfMonitorIncPanicRecoveries(queryName, 0.0, time.Now())
	}
	go vp.startGoroutineForSelfMetrics()
	vp.selfMonitorIncPanicRecoveries(utils.SELF_METRICS_REGISTRY_NAME, 0.0, time.Now())
	log.Info("PromRemoteWriteProcessor : Start() finished")
}

func (vp *PromRemoteWriteProcessor) startGoroutine(queryName string) {
	defer log.WithField("query", queryName).Info("PromRemoteWriteProcessor : Goroutine for the query is finished")
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).WithFields(log.Fields{"query": queryName, "panic": rec, "stacktrace": string(debug.Stack())}).Error("PromRemoteWriteProcessor : Panic during pushing for the query")
			time.Sleep(time.Second * 5)
			log.WithField("query", queryName).Info("PromRemoteWriteProcessor : Starting goroutine for the query again ...")
			go vp.startGoroutine(queryName)
			vp.selfMonitorIncPanicRecoveries(queryName, 1.0, time.Now())
		}
	}()
	log.WithField("query", queryName).Info("PromRemoteWriteProcessor : Goroutine for the query is started")
	promRWService := vp.promRWService
	for {
		mfs, ok := vp.gmQueue.Get(queryName)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).WithField("query", queryName).Error("PromRemoteWriteProcessor : Chan is closed for the query, stopping goroutine")
			return
		}
		if len(mfs) == 0 {
			log.WithField("query", queryName).Info("PromRemoteWriteProcessor : No metric families received for the query")
			continue
		}
		if promRWService != nil {
			vp.enrichWithCloudLabels(mfs)
			errc, err := promRWService.WriteMetrics(mfs, queryName)
			for err != nil {
				log.WithField(ec.FIELD, errc).WithFields(log.Fields{"query": queryName, "error": err}).Error("PromRemoteWriteProcessor : Error pushing metrics for the query")
				if *vp.appConfig.General.PushRetry {
					time.Sleep(vp.appConfig.General.PushRetryPeriodParsed)
					log.WithField("query", queryName).Info("PromRemoteWriteProcessor : Retry pushing metrics for the query")
					errc, err = promRWService.WriteMetrics(mfs, queryName)
				} else {
					break
				}
			}
		}
	}
}

func (vp *PromRemoteWriteProcessor) startGoroutineForSelfMetrics() {
	defer log.WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Info("PromRemoteWriteProcessor : Goroutine for the registry is finished")
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).WithFields(log.Fields{"self_metrics_registry_name": utils.SELF_METRICS_REGISTRY_NAME, "panic": rec, "stacktrace": string(debug.Stack())}).Error("PromRemoteWriteProcessor : Panic during pushing for the registry")
			time.Sleep(time.Second * 5)
			log.WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Info("PromRemoteWriteProcessor : Starting goroutine for the registry again ...")
			go vp.startGoroutineForSelfMetrics()
			vp.selfMonitorIncPanicRecoveries(utils.SELF_METRICS_REGISTRY_NAME, 1.0, time.Now())
		}
	}()
	log.WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Info("PromRemoteWriteProcessor : Goroutine for the registry is started")
	promRWService := vp.promRWService
	for {
		mfs, ok := vp.gmQueue.Get(utils.SELF_METRICS_REGISTRY_NAME)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Error("PromRemoteWriteProcessor : Chan is closed for the registry, stopping goroutine")
			return
		}
		if len(mfs) == 0 {
			log.WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Info("PromRemoteWriteProcessor : No metric families received for the registry")
			continue
		}
		if promRWService != nil {
			vp.enrichWithCloudLabels(mfs)
			errc, err := promRWService.WriteMetrics(mfs, utils.SELF_METRICS_REGISTRY_NAME)
			for err != nil {
				log.WithField(ec.FIELD, errc).WithFields(log.Fields{"self_metrics_registry_name": utils.SELF_METRICS_REGISTRY_NAME, "error": err}).Error("PromRemoteWriteProcessor : Error pushing metrics for the registry")
				if *vp.appConfig.General.PushRetry {
					time.Sleep(vp.appConfig.General.PushRetryPeriodParsed)
					log.WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Info("PromRemoteWriteProcessor : Retry pushing metrics for the registry")
					errc, err = promRWService.WriteMetrics(mfs, utils.SELF_METRICS_REGISTRY_NAME)
				} else {
					break
				}
			}
		}
	}
}

func (vp *PromRemoteWriteProcessor) selfMonitorIncPanicRecoveries(qName string, value float64, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["process_name"] = "PromRemoteWriteProcessor"
	selfmonitor.IncPanicRecoveriesCount(labels, value, &timestamp)
}
