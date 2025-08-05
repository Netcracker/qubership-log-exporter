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

type NewRelicCallsProcessor struct {
	appConfig       *config.Config
	gtsQueue        *queues.GTSQueue
	gdQueue         *queues.GDQueue
	newRelicService *httpservice.NewRelicService
}

func NewNewRelicCallsProcessor(appConfig *config.Config, gtsQueue *queues.GTSQueue, gdQueue *queues.GDQueue) *NewRelicCallsProcessor {
	result := NewRelicCallsProcessor{
		appConfig: appConfig,
		gtsQueue:  gtsQueue,
		gdQueue:   gdQueue,
	}

	result.newRelicService = httpservice.CreateNewRelicService(appConfig)
	return &result
}

func (nrcp *NewRelicCallsProcessor) Start() {
	log.Info("NewRelicCallsProcessor : Start()")
	for queryName, queryConfig := range nrcp.appConfig.Queries {
		go nrcp.startGoroutine(queryName, queryConfig)
		nrcp.selfMonitorIncPanicRecoveries(queryName, 0.0, time.Now())
	}
	log.Info("NewRelicCallsProcessor : Start() finished")
}

func (nrcp *NewRelicCallsProcessor) startGoroutine(queryName string, queryConfig *config.QueryConfig) {
	defer log.Infof("NewRelicCallsProcessor : Goroutine for query %v is finished", queryName)
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).Errorf("NewRelicCallsProcessor : Panic during execution of query %v : %+v ; Stacktrace of the panic : %v", queryName, rec, string(debug.Stack()))
			time.Sleep(time.Second * 5)
			log.Infof("NewRelicCallsProcessor : Starting gouroutine for query %v again ...", queryName)
			go nrcp.startGoroutine(queryName, queryConfig)
			nrcp.selfMonitorIncPanicRecoveries(queryName, 1.0, time.Now())
		}
	}()
	log.Infof("NewRelicCallsProcessor : Goroutine for query %v is started", queryName)
	for {
		time, ok := nrcp.gtsQueue.Get(queryName)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).Errorf("NewRelicCallsProcessor : Chan is closed for the query %v, stopping goroutine", queryName)
			return
		}
		if time.IsZero() {
			log.Infof("NewRelicCallsProcessor : Zero time received for query %v", queryName)
			continue
		}
		nrcp.gdQueue.Put(queryName, nrcp.executeNewRelicQuery(queryName, queryConfig, time))
	}
}

func (nrcp *NewRelicCallsProcessor) executeNewRelicQuery(qName string, queryConfig *config.QueryConfig, startTime time.Time) *queues.GraylogData {
	endTime := startTime.Add(queryConfig.TimerangeDuration)
	log.Debugf("executeNewRelicQuery for query %v, startTime %v, endTime %v", qName, startTime, endTime)

	queryResult, errc, err := nrcp.newRelicService.Query(qName, startTime, endTime)

	for err != nil {
		log.WithField(ec.FIELD, errc).Errorf("Error requesting newrelic for query %v : %+v", qName, err)
		if *nrcp.appConfig.General.DatasourceRetry {
			time.Sleep(nrcp.appConfig.General.DatasourceRetryPeriodParsed)
			log.Infof("Retry requesting newrelic for query %v, startTime %v , endTime %v", qName, startTime, endTime)
			queryResult, errc, err = nrcp.newRelicService.Query(qName, startTime, endTime)
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

func (nrcp *NewRelicCallsProcessor) selfMonitorIncPanicRecoveries(qName string, value float64, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["process_name"] = "NewRelicCallsProcessor"
	selfmonitor.IncPanicRecoveriesCount(labels, value, &timestamp)
}
