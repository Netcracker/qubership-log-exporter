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
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"time"

	dto "github.com/prometheus/client_model/go"
	log "github.com/sirupsen/logrus"
)

const gmQueueName string = "GMQueue"

type GMQueue struct { // Graylog metrics queue
	appConfig  *config.Config
	mfsByQuery map[string](chan []*dto.MetricFamily)
}

func NewGMQueue(appConfig *config.Config) *GMQueue {
	result := &GMQueue{
		appConfig:  appConfig,
		mfsByQuery: make(map[string](chan []*dto.MetricFamily)),
	}
	for queryName, queryConfig := range appConfig.Queries {
		result.mfsByQuery[queryName] = make(chan []*dto.MetricFamily, queryConfig.GMQueueSizeParsed)
		log.WithFields(log.Fields{"query": queryName, "cap": cap(result.mfsByQuery[queryName])}).Info("GMQueue is created")
	}
	result.mfsByQuery[utils.SELF_METRICS_REGISTRY_NAME] = make(chan []*dto.MetricFamily, appConfig.General.GMQueueSelfMonSizeParsed)
	log.WithField("cap", cap(result.mfsByQuery[utils.SELF_METRICS_REGISTRY_NAME])).Info("For SELF_METRICS GMQueue is created")
	return result
}

func (gmq *GMQueue) Put(queryName string, mfs []*dto.MetricFamily, isBlocking bool) {
	c := gmq.mfsByQuery[queryName]

	if c == nil {
		log.WithField(ec.FIELD, ec.LME_1624).WithField("query", queryName).Error("GMQueue PutGatherer : Attempting to put buffer for execution to channel for non-existent query")
		return
	}

	if isBlocking {
		log.WithFields(log.Fields{"query": queryName, "c_count": len(c)}).Debug("GMQueue PutGatherer (blocking) : put buffer")
		c <- mfs
		size := len(c)
		log.WithFields(log.Fields{"query": queryName, "size": size}).Debug("GMQueue PutGatherer (blocking) : put successfully performed")
		gmq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
	} else {
		select {
		case c <- mfs:
			size := len(c)
			log.WithFields(log.Fields{"query": queryName, "size": size}).Debug("GMQueue PutGatherer (non-blocking) : put buffer")
			gmq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
		default:
			log.WithField(ec.FIELD, ec.LME_1625).WithFields(log.Fields{"query": queryName, "c_count": len(c)}).Error("GMQueue PutGatherer (non-blocking) : Attempting to put buffer for execution to channel, but the channel is full")
		}
	}
}

func (gmq *GMQueue) Get(queryName string) ([]*dto.MetricFamily, bool) {
	c := gmq.mfsByQuery[queryName]
	result, ok := <-c
	size := len(c)
	log.WithFields(log.Fields{"query": queryName, "size": size}).Debug("GMQueue Get : buffer is extracted")
	gmq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
	return result, ok
}

func (gmq *GMQueue) CloseChan(queryName string) {
	c := gmq.mfsByQuery[queryName]
	log.WithField("query", queryName).Info("GMQueue CloseChan : chan is closed")
	close(c)
}

func (gmq *GMQueue) selfMonitorSetQueueSize(value float64, qName string, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["queue_name"] = gmQueueName
	selfmonitor.SetQueueSize(value, labels, &timestamp)
}
