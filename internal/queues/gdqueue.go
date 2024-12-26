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
    "time"
    "log_exporter/internal/config"
    "log_exporter/internal/selfmonitor"
    ec "log_exporter/internal/utils/errorcodes"
    log "github.com/sirupsen/logrus"
)

const gdQueueName string = "GDQueue"

type GDQueue struct { // Graylog data queue
    appConfig *config.Config
    graylogDataByQuery map[string](chan *GraylogData)
}

type GraylogData struct {
    Data [][]string
    StartTime time.Time
    EndTime time.Time
}

func NewGDQueue(appConfig *config.Config) *GDQueue {
    result:= &GDQueue{
        appConfig: appConfig,
        graylogDataByQuery: make(map[string](chan *GraylogData)),
    }
    for queryName, queryConfig := range appConfig.Queries {
        result.graylogDataByQuery[queryName] = make(chan *GraylogData, queryConfig.GDQueueSizeParsed)
        log.Infof("For query %v GDQueue is created, size : %v", queryName, cap(result.graylogDataByQuery[queryName]))
    }

    return result
}

func (gdq *GDQueue) Put(queryName string, graylogData *GraylogData) {
    c := gdq.graylogDataByQuery[queryName]

    if c == nil {
        log.WithField(ec.FIELD, ec.LME_1624).Errorf("GDQueue Put : Attempting to put graylogData to channel for non-existent query %v", queryName)
        return
    }

    log.Debugf("GDQueue Put (blocking) : For query %v put graylogData, queue len is %v", queryName, len(c))
    c <- graylogData
    size := len(c)
    log.Debugf("GDQueue Put (blocking) : For query %v put successfully performed, queue len is %v", queryName, size)
    gdq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
}

func (gdq *GDQueue) Get(queryName string) (*GraylogData, bool) {
    c := gdq.graylogDataByQuery[queryName]
    result, ok := <- c
    size := len(c)
    if !ok {
        log.WithField(ec.FIELD, ec.LME_1621).Errorf("GDQueue Get : For query %v chan is closed", queryName)
    } else if result != nil {
        log.Debugf("GDQueue Get : For query %v graylogData is extracted. Start time : %v ; end time : %v ; queue len : %v", queryName, result.StartTime, result.EndTime, size)
    } else {
        log.WithField(ec.FIELD, ec.LME_1604).Errorf("GDQueue Get : For query %v nil graylogData is extracted. Queue len : %v", queryName, size)
    }
    gdq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
    return result, ok
}

func (gdq *GDQueue) CloseChan(queryName string) {
    c := gdq.graylogDataByQuery[queryName]
    log.Infof("GDQueue CloseChan : For query %v chan is closed", queryName)
    close(c)
}

func (gdq *GDQueue) selfMonitorSetQueueSize(value float64, qName string, timestamp time.Time) {
    labels := make(map[string]string)
    labels["query_name"] = qName
    labels["queue_name"] = gdQueueName
    selfmonitor.SetQueueSize(value, labels, &timestamp)
}