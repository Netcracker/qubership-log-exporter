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
	defer log.Infof("PromRemoteWriteProcessor : Goroutine for query %v is finished", queryName)
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).Errorf("PromRemoteWriteProcessor : Panic during pushing for query %v : %+v ; Stacktrace of the panic : %v", queryName, rec, string(debug.Stack()))
			time.Sleep(time.Second * 5)
			log.Infof("PromRemoteWriteProcessor : Starting gouroutine for query %v again ...", queryName)
			go vp.startGoroutine(queryName)
			vp.selfMonitorIncPanicRecoveries(queryName, 1.0, time.Now())
		}
	}()
	log.Infof("PromRemoteWriteProcessor : Goroutine for query %v is started", queryName)
	promRWService := vp.promRWService
	for {
		mfs, ok := vp.gmQueue.Get(queryName)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).Errorf("PromRemoteWriteProcessor : Chan is closed for the query %v, stopping goroutine", queryName)
			return
		}
		if len(mfs) == 0 {
			log.Infof("PromRemoteWriteProcessor : No metric families received for the query %v", queryName)
			continue
		}
		if promRWService != nil {
			vp.enrichWithCloudLabels(mfs)
			errc, err := promRWService.WriteMetrics(mfs, queryName)
			for err != nil {
				log.WithField(ec.FIELD, errc).Errorf("PromRemoteWriteProcessor : Error pushing metrics for query %v : %+v", queryName, err)
				if *vp.appConfig.General.PushRetry {
					time.Sleep(vp.appConfig.General.PushRetryPeriodParsed)
					log.Infof("PromRemoteWriteProcessor : Retry pushing metrics for query %v", queryName)
					errc, err = promRWService.WriteMetrics(mfs, queryName)
				} else {
					break
				}
			}
		}
	}
}

func (vp *PromRemoteWriteProcessor) startGoroutineForSelfMetrics() {
	defer log.Infof("PromRemoteWriteProcessor : Goroutine for %v is finished", utils.SELF_METRICS_REGISTRY_NAME)
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).Errorf("PromRemoteWriteProcessor : Panic during pushing for %v : %+v ; Stacktrace of the panic : %v", utils.SELF_METRICS_REGISTRY_NAME, rec, string(debug.Stack()))
			time.Sleep(time.Second * 5)
			log.Infof("PromRemoteWriteProcessor : Starting gouroutine for %v again ...", utils.SELF_METRICS_REGISTRY_NAME)
			go vp.startGoroutineForSelfMetrics()
			vp.selfMonitorIncPanicRecoveries(utils.SELF_METRICS_REGISTRY_NAME, 1.0, time.Now())
		}
	}()
	log.Infof("PromRemoteWriteProcessor : Goroutine for %v is started", utils.SELF_METRICS_REGISTRY_NAME)
	promRWService := vp.promRWService
	for {
		mfs, ok := vp.gmQueue.Get(utils.SELF_METRICS_REGISTRY_NAME)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).Errorf("PromRemoteWriteProcessor : Chan is closed for %v, stopping goroutine", utils.SELF_METRICS_REGISTRY_NAME)
			return
		}
		if len(mfs) == 0 {
			log.Infof("PromRemoteWriteProcessor : No metric families received for %v", utils.SELF_METRICS_REGISTRY_NAME)
			continue
		}
		if promRWService != nil {
			vp.enrichWithCloudLabels(mfs)
			errc, err := promRWService.WriteMetrics(mfs, utils.SELF_METRICS_REGISTRY_NAME)
			for err != nil {
				log.WithField(ec.FIELD, errc).Errorf("PromRemoteWriteProcessor : Error pushing metrics for %v : %+v", utils.SELF_METRICS_REGISTRY_NAME, err)
				if *vp.appConfig.General.PushRetry {
					time.Sleep(vp.appConfig.General.PushRetryPeriodParsed)
					log.Infof("PromRemoteWriteProcessor : Retry pushing metrics for %v", utils.SELF_METRICS_REGISTRY_NAME)
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
