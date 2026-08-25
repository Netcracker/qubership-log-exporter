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
	defer log.WithField("query", queryName).Info("NewRelicCallsProcessor : Goroutine for the query is finished")
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).WithFields(log.Fields{"query": queryName, "panic": rec, "stacktrace": string(debug.Stack())}).Error("NewRelicCallsProcessor : Panic during execution of the query")
			time.Sleep(time.Second * 5)
			log.WithField("query", queryName).Info("NewRelicCallsProcessor : Starting goroutine for the query again ...")
			go nrcp.startGoroutine(queryName, queryConfig)
			nrcp.selfMonitorIncPanicRecoveries(queryName, 1.0, time.Now())
		}
	}()
	log.WithField("query", queryName).Info("NewRelicCallsProcessor : Goroutine for the query is started")
	for {
		time, ok := nrcp.gtsQueue.Get(queryName)
		if !ok {
			log.WithField(ec.FIELD, ec.LME_1621).WithField("query", queryName).Error("NewRelicCallsProcessor : Chan is closed for the query, stopping goroutine")
			return
		}
		if time.IsZero() {
			log.WithField("query", queryName).Info("NewRelicCallsProcessor : Zero time received for the query")
			continue
		}
		nrcp.gdQueue.Put(queryName, nrcp.executeNewRelicQuery(queryName, queryConfig, time))
	}
}

func (nrcp *NewRelicCallsProcessor) executeNewRelicQuery(qName string, queryConfig *config.QueryConfig, startTime time.Time) *queues.GraylogData {
	endTime := startTime.Add(queryConfig.TimerangeDuration)
	log.WithFields(log.Fields{"query": qName, "start_time": startTime, "end_time": endTime}).Debug("executeNewRelicQuery")

	queryResult, errc, err := nrcp.newRelicService.Query(qName, startTime, endTime)

	for err != nil {
		log.WithField(ec.FIELD, errc).WithFields(log.Fields{"query": qName, "error": err}).Error("Error requesting newrelic")
		if *nrcp.appConfig.General.DatasourceRetry {
			time.Sleep(nrcp.appConfig.General.DatasourceRetryPeriodParsed)
			log.WithFields(log.Fields{"query": qName, "start_time": startTime, "end_time": endTime}).Info("Retry requesting newrelic")
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
