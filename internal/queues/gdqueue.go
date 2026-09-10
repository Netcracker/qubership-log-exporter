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

package queues

import (
	"log_exporter/internal/config"
	"log_exporter/internal/selfmonitor"
	ec "log_exporter/internal/utils/errorcodes"
	"time"

	log "github.com/sirupsen/logrus"
)

const gdQueueName string = "GDQueue"

type GDQueue struct { // Graylog data queue
	appConfig          *config.Config
	graylogDataByQuery map[string](chan *GraylogData)
}

type GraylogData struct {
	Data      [][]string
	StartTime time.Time
	EndTime   time.Time
}

func NewGDQueue(appConfig *config.Config) *GDQueue {
	result := &GDQueue{
		appConfig:          appConfig,
		graylogDataByQuery: make(map[string](chan *GraylogData)),
	}
	for queryName, queryConfig := range appConfig.Queries {
		result.graylogDataByQuery[queryName] = make(chan *GraylogData, queryConfig.GDQueueSizeParsed)
		log.WithFields(log.Fields{"query": queryName, "cap": cap(result.graylogDataByQuery[queryName])}).Info("GDQueue is created")
	}

	return result
}

func (gdq *GDQueue) Put(queryName string, graylogData *GraylogData) {
	c := gdq.graylogDataByQuery[queryName]

	if c == nil {
		log.WithField(ec.FIELD, ec.LME_1624).WithField("query", queryName).Error("GDQueue Put : Attempting to put graylogData to channel for non-existent query")
		return
	}

	log.WithFields(log.Fields{"query": queryName, "c_count": len(c)}).Debug("GDQueue Put (blocking) : put graylogData")
	c <- graylogData
	size := len(c)
	log.WithFields(log.Fields{"query": queryName, "size": size}).Debug("GDQueue Put (blocking) : put successfully performed")
	gdq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
}

func (gdq *GDQueue) Get(queryName string) (*GraylogData, bool) {
	c := gdq.graylogDataByQuery[queryName]
	if c == nil {
		log.WithField(ec.FIELD, ec.LME_1621).WithField("query", queryName).Error("GDQueue Get : Attempting to get graylogData from channel for non-existent query")
		return nil, false
	}
	result, ok := <-c
	size := len(c)
	if !ok {
		log.WithField(ec.FIELD, ec.LME_1621).WithField("query", queryName).Error("GDQueue Get : chan is closed")
	} else if result != nil {
		log.WithFields(log.Fields{"query": queryName, "start_time": result.StartTime, "end_time": result.EndTime, "size": size}).Debug("GDQueue Get : graylogData is extracted")
	} else {
		log.WithField(ec.FIELD, ec.LME_1604).WithFields(log.Fields{"query": queryName, "size": size}).Error("GDQueue Get : nil graylogData is extracted")
	}
	gdq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
	return result, ok
}

func (gdq *GDQueue) CloseChan(queryName string) {
	c := gdq.graylogDataByQuery[queryName]
	if c == nil {
		log.WithField(ec.FIELD, ec.LME_1621).WithField("query", queryName).Error("GDQueue CloseChan : Attempting to close channel for non-existent query")
		return
	}
	log.WithField("query", queryName).Info("GDQueue CloseChan : chan is closed")
	close(c)
}

func (gdq *GDQueue) selfMonitorSetQueueSize(value float64, qName string, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["queue_name"] = gdQueueName
	selfmonitor.SetQueueSize(value, labels, &timestamp)
}
