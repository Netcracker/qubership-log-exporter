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
	"log_exporter/internal/httpservice"
	"log_exporter/internal/selfmonitor"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"time"

	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
)

const gtsQueueName string = "GTSQueue"

type GTSQueue struct { // Graylog time ranges start queue
	appConfig            *config.Config
	timestampsByQuery    map[string](chan time.Time)
	lastTimestampService *httpservice.LastTimestampService
	croniter             *cron.Cron
}

func NewGTSQueue(appConfig *config.Config, lastTimestampService *httpservice.LastTimestampService, croniter *cron.Cron) *GTSQueue {
	result := &GTSQueue{
		appConfig:            appConfig,
		timestampsByQuery:    make(map[string](chan time.Time)),
		lastTimestampService: lastTimestampService,
		croniter:             croniter,
	}
	result.generateHistoryTimestamps()
	return result
}

func (gtsq *GTSQueue) generateHistoryTimestamps() {
	croniter := utils.GetCron()

	for queryName, queryConfig := range gtsq.appConfig.Queries {
		gtsq.timestampsByQuery[queryName] = make(chan time.Time, queryConfig.GTSQueueSizeParsed)
		log.Infof("For query %v GTSQueue is created, size : %v", queryName, cap(gtsq.timestampsByQuery[queryName]))
	}

	for queryName, queryConfig := range gtsq.appConfig.Queries {
		queryName := queryName
		queryConfig := queryConfig

		go func() {
			defer gtsq.scheduleTimestampGenerationForQuery(queryName)
			if gtsq.lastTimestampService == nil {
				return
			}
			if queryConfig.IntervalDuration <= 0 {
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("NewGTSQueue : For query %v intervalDuration is %v, which is <= 0. History won't be processed for the query", queryName, queryConfig.IntervalDuration)
				return
			}
			if queryConfig.TimerangeDuration < 0 {
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("NewGTSQueue : For query %v timerangeDuration is %v, which is < 0. History won't be processed for the query", queryName, queryConfig.IntervalDuration)
				return
			}
			if queryConfig.MaxHistoryLookupDuration <= 0 {
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("NewGTSQueue : For query %v maxHistoryLookupDuration is %v, which is <= 0. History won't be processed for the query", queryName, queryConfig.MaxHistoryLookupDuration)
				return
			}

			var unixTime int64
			var err error
			var retryPossible bool
			var errc string
			retryCount := gtsq.appConfig.General.LTSRetryCountParsed
			for i := 0; i < retryCount; i++ {
				unixTime, retryPossible, errc, err = gtsq.lastTimestampService.GetLastTimestampUnixTime(queryName, queryConfig)
				if err == nil {
					log.Infof("NewGTSQueue : For query %v attempt %v of %v to extract last timestamp succeeded", queryName, i+1, retryCount)
					break
				} else if retryPossible {
					log.Warnf("NewGTSQueue : For query %v attempt %v of %v to extract last timestamp is failed : Error during last timestamp evaluation : %+v", queryName, i+1, retryCount, err)
					time.Sleep(gtsq.appConfig.General.LTSRetryPeriodParsed)
				} else {
					log.Warnf("NewGTSQueue : For query %v attempt %v of %v : retry is not possible, error occurred : %+v", queryName, i+1, retryCount, err)
					break
				}
			}
			if err != nil {
				log.WithField(ec.FIELD, errc).Errorf("NewGTSQueue : For query %v history won't be processed : Error during last timestamp evaluation : %+v", queryName, err)
				return
			}
			lastTimestamp := time.Unix(unixTime, 0)
			log.Infof("NewGTSQueue : For query %v last timestamp extracted from Victoria : %v", queryName, lastTimestamp)

			nearestUpcomingCronTime, err := getNearestUpcomingCronTime(queryConfig, croniter)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_1608).Errorf("NewGTSQueue : History timestamp processing will be skipped for query %v : Error evaluating nearestTimestamp : %+v", queryName, err)
				return
			} else {
				log.Infof("NewGTSQueue : nearestUpcomingCronTime for query %v : %v", queryName, nearestUpcomingCronTime)
			}

			if lastTimestamp.After(time.Now()) {
				log.WithField(ec.FIELD, ec.LME_7130).Errorf("NewGTSQueue : For query %v history won't be processed : Timestamp extracted from Victoria is after current time", queryName)
				return
			}

			nearestUpcomingGraylogTime := nearestUpcomingCronTime.Add(-queryConfig.QueryLagDuration - queryConfig.TimerangeDuration)
			log.Infof("NewGTSQueue : nearestUpcomingGraylogTime for query %v : %v", queryName, nearestUpcomingGraylogTime)
			historyDuration := nearestUpcomingGraylogTime.Sub(lastTimestamp)
			log.Infof("NewGTSQueue : historyDuration for query %v : %v", queryName, historyDuration)
			if historyDuration > queryConfig.MaxHistoryLookupDuration {
				log.Infof("NewGTSQueue : For query %v historyDuration %v is bigger than MaxHistoryLookupDuration %v; historyDuration will be limited", queryName, historyDuration, queryConfig.MaxHistoryLookupDuration)
				historyDuration = queryConfig.MaxHistoryLookupDuration
			}
			historySize := historyDuration.Nanoseconds() / queryConfig.IntervalDuration.Nanoseconds()
			log.Infof("NewGTSQueue : historySize for query %v : %v", queryName, historySize)
			if historySize >= int64(queryConfig.GTSQueueSizeParsed) {
				historySize = int64(queryConfig.GTSQueueSizeParsed)
			}
			if historySize <= 0 {
				log.Infof("NewGTSQueue : For query %v history won't be processed : historySize is %v", queryName, historySize)
				return
			}
			log.Infof("NewGTSQueue : historySize after checks for query %v : %v", queryName, int(historySize))
			firstGraylogHistoryTime := nearestUpcomingGraylogTime.Add(-queryConfig.IntervalDuration * time.Duration(historySize))
			log.Infof("NewGTSQueue : firstGraylogHistoryTime for query %v : %v", queryName, firstGraylogHistoryTime)
			result := make([]time.Time, 0, int(historySize))
			for i := 0; i < int(historySize); i++ {
				result = append(result, firstGraylogHistoryTime.Add(queryConfig.IntervalDuration*time.Duration(i)))
			}
			log.Infof("NewGTSQueue : History timestamps for query %v are : %+v", queryName, result)
			for _, timestamp := range result {
				gtsq.timestampsByQuery[queryName] <- timestamp
			}
		}()
	}
}

func (gtsq *GTSQueue) scheduleTimestampGenerationForQuery(queryName string) {
	queryConfig := gtsq.appConfig.Queries[queryName]
	log.Debugf("queryName : %v, queryConfig: %+v", queryName, queryConfig)
	if queryConfig.QueryLagDuration < 0 || queryConfig.TimerangeDuration < 0 {
		log.WithField(ec.FIELD, ec.LME_8102).Errorf("For query %v QueryLagDuration = %v; TimerangeDuration = %v , which is is incorrect. Query will be skipped", queryName, queryConfig.QueryLagDuration, queryConfig.TimerangeDuration)
		gtsq.CloseChan(queryName)
		return
	}
	res, err := gtsq.croniter.AddFunc(queryConfig.Croniter, func() {
		currentTime := time.Now().UTC().Round(time.Second)
		startTime := currentTime.Add(-queryConfig.QueryLagDuration - queryConfig.TimerangeDuration)
		log.Debugf("For query %v put time %v to gtsQueue (currentTime = %v)", queryName, startTime, currentTime)
		gtsq.Put(queryName, startTime)
	})
	if err != nil {
		gtsq.appConfig.Queries[queryName].CronEntryID = -1
		log.WithField(ec.FIELD, ec.LME_1608).Errorf("During registering query %v in croniter following error occurred : %+v", queryName, err)
	} else {
		gtsq.appConfig.Queries[queryName].CronEntryID = int(res)
		log.Infof("Query %v is registered in croniter with id %v", queryName, int(res))
	}
}

func getNearestUpcomingCronTime(queryConfig *config.QueryConfig, croniter *cron.Cron) (time.Time, error) {
	res, err := croniter.AddFunc(queryConfig.Croniter, func() {})
	if err != nil {
		return time.Time{}, err
	}
	croniter.Start()
	entry := croniter.Entry(res)
	nearestTimestamp := entry.Next
	croniter.Stop()
	croniter.Remove(res)
	return nearestTimestamp, nil
}

func (gtsq *GTSQueue) Put(queryName string, timestamp time.Time) {
	c := gtsq.timestampsByQuery[queryName]

	if c == nil {
		log.WithField(ec.FIELD, ec.LME_1624).Errorf("GTSQueue Put : Attempting to put timestamp for execution to channel for non-existent query %v", queryName)
		return
	}

	select {
	case c <- timestamp:
		size := len(c)
		log.Debugf("GTSQueue Put : For query %v put timestamp %v, queue len is %v", queryName, timestamp, size)
		gtsq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
	default:
		log.WithField(ec.FIELD, ec.LME_1625).Errorf("GTSQueue Put : Attempting to put timestamp for execution to channel for query %v : channel is full, len == %v", queryName, len(c))
	}
}

func (gtsq *GTSQueue) Get(queryName string) (time.Time, bool) {
	c := gtsq.timestampsByQuery[queryName]
	result, ok := <-c
	size := len(c)
	log.Debugf("GTSQueue Get : For query %v timestamp %v is extracted, queue len is %v", queryName, result, size)
	gtsq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
	return result, ok
}

func (gtsq *GTSQueue) CloseChan(queryName string) {
	c := gtsq.timestampsByQuery[queryName]
	log.Infof("GTSQueue CloseChan : For query %v chan is closed", queryName)
	close(c)
}

func (gtsq *GTSQueue) selfMonitorSetQueueSize(value float64, qName string, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["queue_name"] = gtsQueueName
	selfmonitor.SetQueueSize(value, labels, &timestamp)
}
