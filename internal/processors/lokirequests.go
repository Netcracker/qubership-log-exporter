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
	ec "log_exporter/internal/utils/errorcodes"
	"runtime/debug"
	"time"

	log "github.com/sirupsen/logrus"
)

type LokiCallsProcessor struct {
	appConfig   *config.Config
	gtsQueue    *queues.GTSQueue
	gdQueue     *queues.GDQueue
	lokiService *httpservice.LokiService
}

func NewLokiCallsProcessor(appConfig *config.Config, gtsQueue *queues.GTSQueue, gdQueue *queues.GDQueue) *LokiCallsProcessor {
	result := LokiCallsProcessor{
		appConfig: appConfig,
		gtsQueue:  gtsQueue,
		gdQueue:   gdQueue,
	}

	result.lokiService = httpservice.CreateLokiService(appConfig)
	return &result
}

func (gcp *LokiCallsProcessor) Start() {
	log.Info("LokiCallsProcessor : Start()")
	for queryName, queryConfig := range gcp.appConfig.Queries {
		go gcp.startGoroutine(queryName, queryConfig)
		gcp.selfMonitorIncPanicRecoveries(queryName, 0.0, time.Now())
	}
	log.Info("LokiCallsProcessor : Start() finished")
}

func (gcp *LokiCallsProcessor) startGoroutine(queryName string, queryConfig *config.QueryConfig) {
	defer log.Infof("LokiCallsProcessor : Goroutine for query %v is finished", queryName)
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).Errorf("LokiCallsProcessor : Panic during execution of query %v : %+v ; Stacktrace of the panic : %v", queryName, rec, string(debug.Stack()))
			time.Sleep(time.Second * 5)
			log.Infof("LokiCallsProcessor : Starting gouroutine for query %v again ...", queryName)
			go gcp.startGoroutine(queryName, queryConfig)
			gcp.selfMonitorIncPanicRecoveries(queryName, 1.0, time.Now())
		}
	}()
	log.Infof("LokiCallsProcessor : Goroutine for query %v is started", queryName)
	for {
		time, ok := gcp.gtsQueue.Get(queryName)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).Errorf("LokiCallsProcessor : Chan is closed for the query %v, stopping goroutine", queryName)
			return
		}
		if time.IsZero() {
			log.Infof("LokiCallsProcessor : Zero time received for query %v", queryName)
			continue
		}
		gcp.gdQueue.Put(queryName, gcp.executeLokiQuery(queryName, queryConfig, time))
	}
}

func (gcp *LokiCallsProcessor) executeLokiQuery(qName string, queryConfig *config.QueryConfig, startTime time.Time) *queues.GraylogData {
	endTime := startTime.Add(queryConfig.TimerangeDuration)
	log.Debugf("executeLokiQuery for query %v, startTime %v, endTime %v", qName, startTime, endTime)

	queryResult, errc, err := gcp.lokiService.Query(qName, startTime, endTime)

	for err != nil {
		log.WithField(ec.FIELD, errc).Errorf("Error requesting loki for query %v, startTime %v , endTime %v : %+v", qName, startTime, endTime, err)
		if *gcp.appConfig.General.DatasourceRetry {
			time.Sleep(gcp.appConfig.General.DatasourceRetryPeriodParsed)
			log.Infof("Retry requesting loki for query %v, startTime %v , endTime %v", qName, startTime, endTime)
			queryResult, errc, err = gcp.lokiService.Query(qName, startTime, endTime)
		} else {
			break
		}
	}

	return &queues.GraylogData{
		Data:      queryResult,
		StartTime: startTime,
		EndTime:   endTime,
	}
}

func (gcp *LokiCallsProcessor) selfMonitorIncPanicRecoveries(qName string, value float64, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["process_name"] = "LokiCallsProcessor"
	selfmonitor.IncPanicRecoveriesCount(labels, value, &timestamp)
}
