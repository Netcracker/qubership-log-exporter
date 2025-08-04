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
	defer log.Infof("VictoriaProcessor : Goroutine for query %v is finished", queryName)
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).Errorf("VictoriaProcessor : Panic during pushing for query %v : %+v ; Stacktrace of the panic : %v", queryName, rec, string(debug.Stack()))
			time.Sleep(time.Second * 5)
			log.Infof("VictoriaProcessor : Starting gouroutine for query %v again ...", queryName)
			go vp.startGoroutine(queryName)
			vp.selfMonitorIncPanicRecoveries(queryName, 1.0, time.Now())
		}
	}()
	log.Infof("VictoriaProcessor : Goroutine for query %v is started", queryName)
	victoriaService := vp.victoriaService
	for {
		mfs, ok := vp.gmQueue.Get(queryName)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).Errorf("VictoriaProcessor : Chan is closed for the query %v, stopping goroutine", queryName)
			return
		}
		if len(mfs) == 0 {
			log.Infof("VictoriaProcessor : No metric families received for the query %v", queryName)
			continue
		}
		vp.enrichWithCloudLabels(mfs)
		buffer := mfsToByteBuffer(mfs)
		if victoriaService != nil && buffer != nil {
			errc, err := victoriaService.PushBuffer(buffer, queryName)
			for err != nil {
				log.WithField(ec.FIELD, errc).Errorf("VictoriaProcessor : Error pushing metrics for query %v : %+v", queryName, err)
				if *vp.appConfig.General.PushRetry {
					time.Sleep(vp.appConfig.General.PushRetryPeriodParsed)
					log.Infof("VictoriaProcessor : Retry pushing metrics for query %v", queryName)
					errc, err = victoriaService.PushBuffer(buffer, queryName)
				} else {
					break
				}
			}
		}
	}
}

func (vp *VictoriaProcessor) startGoroutineForSelfMetrics() {
	defer log.Infof("VictoriaProcessor : Goroutine for %v is finished", utils.SELF_METRICS_REGISTRY_NAME)
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).Errorf("VictoriaProcessor : Panic during pushing for %v : %+v ; Stacktrace of the panic : %v", utils.SELF_METRICS_REGISTRY_NAME, rec, string(debug.Stack()))
			time.Sleep(time.Second * 5)
			log.Infof("VictoriaProcessor : Starting gouroutine for %v again ...", utils.SELF_METRICS_REGISTRY_NAME)
			go vp.startGoroutineForSelfMetrics()
			vp.selfMonitorIncPanicRecoveries(utils.SELF_METRICS_REGISTRY_NAME, 1.0, time.Now())
		}
	}()
	log.Infof("VictoriaProcessor : Goroutine for %v is started", utils.SELF_METRICS_REGISTRY_NAME)
	victoriaService := vp.victoriaService
	for {
		mfs, ok := vp.gmQueue.Get(utils.SELF_METRICS_REGISTRY_NAME)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).Errorf("VictoriaProcessor : Chan is closed for %v, stopping goroutine", utils.SELF_METRICS_REGISTRY_NAME)
			return
		}
		if len(mfs) == 0 {
			log.Infof("VictoriaProcessor : No metric families received for %v", utils.SELF_METRICS_REGISTRY_NAME)
			continue
		}
		vp.enrichWithCloudLabels(mfs)
		buffer := mfsToByteBuffer(mfs)
		if victoriaService != nil && buffer != nil {
			errc, err := victoriaService.PushBuffer(buffer, utils.SELF_METRICS_REGISTRY_NAME)
			for err != nil {
				log.WithField(ec.FIELD, errc).Errorf("VictoriaProcessor : Error pushing metrics for %v : %+v", utils.SELF_METRICS_REGISTRY_NAME, err)
				if *vp.appConfig.General.PushRetry {
					time.Sleep(vp.appConfig.General.PushRetryPeriodParsed)
					log.Infof("VictoriaProcessor : Retry pushing metrics for %v", utils.SELF_METRICS_REGISTRY_NAME)
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
			log.WithField(ec.FIELD, ec.LME_1601).Errorf("VictoriaProcessor : Error during formatting metricFamily %v as text : %+v", *mf.Name, err)
		}
		log.Debugf("VictoriaProcessor : Metric family %v processed to text : %v bytes total", *mf.Name, written)
	}

	return buffer
}

func (vp *VictoriaProcessor) selfMonitorIncPanicRecoveries(qName string, value float64, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["process_name"] = "VictoriaProcessor"
	selfmonitor.IncPanicRecoveriesCount(labels, value, &timestamp)
}
