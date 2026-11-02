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
	"bytes"
	"log_exporter/internal/config"
	"log_exporter/internal/httpservice"
	"log_exporter/internal/queues"
	"log_exporter/internal/selfmonitor"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"runtime/debug"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	log "github.com/sirupsen/logrus"
)

type VictoriaProcessor struct {
	PushProcessor
	gmQueue         *queues.GMQueue
	victoriaService *httpservice.VictoriaService
}

func NewVictoriaProcessor(appConfig *config.Config, gmQueue *queues.GMQueue, victoriaService *httpservice.VictoriaService) *VictoriaProcessor {
	vp := &VictoriaProcessor{
		gmQueue:         gmQueue,
		victoriaService: victoriaService,
	}
	vp.appConfig = appConfig
	return vp
}

func (vp *VictoriaProcessor) Start() {
	log.Info("VictoriaProcessor : Start()")
	if vp.gmQueue == nil {
		log.Info("VictoriaProcessor : Start() : vp.gmQueue == nil, VictoriaProcessor will be disabled")
		return
	}
	for queryName := range vp.appConfig.Queries {
		go vp.startGoroutine(queryName)
		vp.selfMonitorIncPanicRecoveries(queryName, 0.0, time.Now())
	}
	go vp.startGoroutineForSelfMetrics()
	vp.selfMonitorIncPanicRecoveries(utils.SELF_METRICS_REGISTRY_NAME, 0.0, time.Now())

	log.Info("VictoriaProcessor : Start() finished")
}

func (vp *VictoriaProcessor) startGoroutine(queryName string) {
	defer log.WithField("query", queryName).Info("VictoriaProcessor : Goroutine for the query is finished")
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).WithFields(log.Fields{"query": queryName, "panic": rec, "stacktrace": string(debug.Stack())}).Error("VictoriaProcessor : Panic during pushing for the query")
			time.Sleep(time.Second * 5)
			log.WithField("query", queryName).Info("VictoriaProcessor : Starting goroutine for the query again ...")
			go vp.startGoroutine(queryName)
			vp.selfMonitorIncPanicRecoveries(queryName, 1.0, time.Now())
		}
	}()
	log.WithField("query", queryName).Info("VictoriaProcessor : Goroutine for the query is started")
	victoriaService := vp.victoriaService
	for {
		mfs, ok := vp.gmQueue.Get(queryName)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).WithField("query", queryName).Error("VictoriaProcessor : Chan is closed for the query, stopping goroutine")
			return
		}
		if len(mfs) == 0 {
			log.WithField("query", queryName).Info("VictoriaProcessor : No metric families received for the query")
			continue
		}
		vp.enrichWithCloudLabels(mfs)
		buffer := mfsToByteBuffer(mfs)
		if victoriaService != nil && buffer != nil {
			errc, err := victoriaService.PushBuffer(buffer, queryName)
			for err != nil {
				log.WithField(ec.FIELD, errc).WithFields(log.Fields{"query": queryName, "error": err}).Error("VictoriaProcessor : Error pushing metrics for the query")
				if *vp.appConfig.General.PushRetry {
					time.Sleep(vp.appConfig.General.PushRetryPeriodParsed)
					log.WithField("query", queryName).Info("VictoriaProcessor : Retry pushing metrics for the query")
					errc, err = victoriaService.PushBuffer(buffer, queryName)
				} else {
					break
				}
			}
		}
	}
}

func (vp *VictoriaProcessor) startGoroutineForSelfMetrics() {
	defer log.WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Info("VictoriaProcessor : Goroutine for the registry is finished")
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).WithFields(log.Fields{"self_metrics_registry_name": utils.SELF_METRICS_REGISTRY_NAME, "panic": rec, "stacktrace": string(debug.Stack())}).Error("VictoriaProcessor : Panic during pushing for the registry")
			time.Sleep(time.Second * 5)
			log.WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Info("VictoriaProcessor : Starting goroutine for the registry again ...")
			go vp.startGoroutineForSelfMetrics()
			vp.selfMonitorIncPanicRecoveries(utils.SELF_METRICS_REGISTRY_NAME, 1.0, time.Now())
		}
	}()
	log.WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Info("VictoriaProcessor : Goroutine for the registry is started")
	victoriaService := vp.victoriaService
	for {
		mfs, ok := vp.gmQueue.Get(utils.SELF_METRICS_REGISTRY_NAME)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Error("VictoriaProcessor : Chan is closed for the registry, stopping goroutine")
			return
		}
		if len(mfs) == 0 {
			log.WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Info("VictoriaProcessor : No metric families received for the registry")
			continue
		}
		vp.enrichWithCloudLabels(mfs)
		buffer := mfsToByteBuffer(mfs)
		if victoriaService != nil && buffer != nil {
			errc, err := victoriaService.PushBuffer(buffer, utils.SELF_METRICS_REGISTRY_NAME)
			for err != nil {
				log.WithField(ec.FIELD, errc).WithFields(log.Fields{"self_metrics_registry_name": utils.SELF_METRICS_REGISTRY_NAME, "error": err}).Error("VictoriaProcessor : Error pushing metrics for the registry")
				if *vp.appConfig.General.PushRetry {
					time.Sleep(vp.appConfig.General.PushRetryPeriodParsed)
					log.WithField("self_metrics_registry_name", utils.SELF_METRICS_REGISTRY_NAME).Info("VictoriaProcessor : Retry pushing metrics for the registry")
					errc, err = victoriaService.PushBuffer(buffer, utils.SELF_METRICS_REGISTRY_NAME)
				} else {
					break
				}
			}
		}
	}
}

func mfsToByteBuffer(mfs []*dto.MetricFamily) *bytes.Buffer {
	buffer := &bytes.Buffer{}
	for _, mf := range mfs {
		written, err := expfmt.MetricFamilyToText(buffer, mf)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_1601).WithFields(log.Fields{"mf_name": *mf.Name, "error": err}).Error("VictoriaProcessor : Error during formatting the metric family as text")
		}
		log.WithFields(log.Fields{"mf_name": *mf.Name, "written": written}).Debug("VictoriaProcessor : Metric family processed to text")
	}

	return buffer
}

func (vp *VictoriaProcessor) selfMonitorIncPanicRecoveries(qName string, value float64, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["process_name"] = "VictoriaProcessor"
	selfmonitor.IncPanicRecoveriesCount(labels, value, &timestamp)
}
