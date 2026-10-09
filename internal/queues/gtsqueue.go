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
		log.WithFields(log.Fields{"query": queryName, "cap": cap(gtsq.timestampsByQuery[queryName])}).Info("GTSQueue is created")
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
				log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"query": queryName, "interval_duration": queryConfig.IntervalDuration}).Error("NewGTSQueue : intervalDuration is <= 0. History won't be processed for the query")
				return
			}
			if queryConfig.TimerangeDuration < 0 {
				log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"query": queryName, "interval_duration": queryConfig.IntervalDuration}).Error("NewGTSQueue : timerangeDuration is < 0. History won't be processed for the query")
				return
			}
			if queryConfig.MaxHistoryLookupDuration <= 0 {
				log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"query": queryName, "max_history_lookup_duration": queryConfig.MaxHistoryLookupDuration}).Error("NewGTSQueue : maxHistoryLookupDuration is <= 0. History won't be processed for the query")
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
					log.WithFields(log.Fields{"query": queryName, "attempt": i + 1, "retry_count": retryCount}).Info("NewGTSQueue : Attempt to extract the last timestamp succeeded")
					break
				} else if retryPossible {
					log.WithFields(log.Fields{"query": queryName, "index": i + 1, "retry_count": retryCount, "error": err}).Warn("NewGTSQueue : attempt to extract last timestamp is failed : Error during last timestamp evaluation")
					time.Sleep(gtsq.appConfig.General.LTSRetryPeriodParsed)
				} else {
					log.WithFields(log.Fields{"query": queryName, "index": i + 1, "retry_count": retryCount, "error": err}).Warn("NewGTSQueue : retry is not possible, error occurred")
					break
				}
			}
			if err != nil {
				log.WithField(ec.FIELD, errc).WithFields(log.Fields{"query": queryName, "error": err}).Error("NewGTSQueue : history won't be processed : Error during last timestamp evaluation")
				return
			}
			lastTimestamp := time.Unix(unixTime, 0)
			log.WithFields(log.Fields{"query": queryName, "last_timestamp": lastTimestamp}).Info("NewGTSQueue : last timestamp extracted from Victoria")

			nearestUpcomingCronTime, err := getNearestUpcomingCronTime(queryConfig, croniter)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_1608).WithFields(log.Fields{"query": queryName, "error": err}).Error("NewGTSQueue : History timestamp processing will be skipped : Error evaluating nearestTimestamp")
				return
			} else {
				log.WithFields(log.Fields{"query": queryName, "nearest_upcoming_cron_time": nearestUpcomingCronTime}).Info("NewGTSQueue : nearestUpcomingCronTime evaluated")
			}

			if lastTimestamp.After(time.Now()) {
				log.WithField(ec.FIELD, ec.LME_7130).WithField("query", queryName).Error("NewGTSQueue : history won't be processed : Timestamp extracted from Victoria is after current time")
				return
			}

			nearestUpcomingGraylogTime := nearestUpcomingCronTime.Add(-queryConfig.QueryLagDuration - queryConfig.TimerangeDuration)
			log.WithFields(log.Fields{"query": queryName, "nearest_upcoming_graylog_time": nearestUpcomingGraylogTime}).Info("NewGTSQueue : nearestUpcomingGraylogTime evaluated")
			historyDuration := nearestUpcomingGraylogTime.Sub(lastTimestamp)
			log.WithFields(log.Fields{"query": queryName, "history_duration": historyDuration}).Info("NewGTSQueue : historyDuration evaluated")
			if historyDuration > queryConfig.MaxHistoryLookupDuration {
				log.WithFields(log.Fields{"query": queryName, "history_duration": historyDuration, "max_history_lookup_duration": queryConfig.MaxHistoryLookupDuration}).Info("NewGTSQueue : historyDuration is bigger than MaxHistoryLookupDuration; historyDuration is limited")
				historyDuration = queryConfig.MaxHistoryLookupDuration
			}
			historySize := historyDuration.Nanoseconds() / queryConfig.IntervalDuration.Nanoseconds()
			log.WithFields(log.Fields{"query": queryName, "history_size": historySize}).Info("NewGTSQueue : historySize evaluated")
			if historySize >= int64(queryConfig.GTSQueueSizeParsed) {
				historySize = int64(queryConfig.GTSQueueSizeParsed)
			}
			if historySize <= 0 {
				log.WithFields(log.Fields{"query": queryName, "history_size": historySize}).Info("NewGTSQueue : history won't be processed : historySize is not positive")
				return
			}
			log.WithFields(log.Fields{"query": queryName, "history_size": int(historySize)}).Info("NewGTSQueue : historySize after checks")
			firstGraylogHistoryTime := nearestUpcomingGraylogTime.Add(-queryConfig.IntervalDuration * time.Duration(historySize))
			log.WithFields(log.Fields{"query": queryName, "first_graylog_history_time": firstGraylogHistoryTime}).Info("NewGTSQueue : firstGraylogHistoryTime evaluated")
			result := make([]time.Time, 0, int(historySize))
			for i := 0; i < int(historySize); i++ {
				result = append(result, firstGraylogHistoryTime.Add(queryConfig.IntervalDuration*time.Duration(i)))
			}
			log.WithFields(log.Fields{"query": queryName, "result": result}).Info("NewGTSQueue : History timestamps generated")
			for _, timestamp := range result {
				gtsq.timestampsByQuery[queryName] <- timestamp
			}
		}()
	}
}

func (gtsq *GTSQueue) scheduleTimestampGenerationForQuery(queryName string) {
	queryConfig := gtsq.appConfig.Queries[queryName]
	log.WithFields(log.Fields{"query": queryName, "query_config": queryConfig}).Debug("Scheduling timestamp generation for the query")
	if queryConfig.QueryLagDuration < 0 || queryConfig.TimerangeDuration < 0 {
		log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"query": queryName, "query_lag_duration": queryConfig.QueryLagDuration, "timerange_duration": queryConfig.TimerangeDuration}).Error("QueryLagDuration or TimerangeDuration is incorrect. Query will be skipped")
		gtsq.CloseChan(queryName)
		return
	}
	res, err := gtsq.croniter.AddFunc(queryConfig.Croniter, func() {
		currentTime := time.Now().UTC().Round(time.Second)
		startTime := currentTime.Add(-queryConfig.QueryLagDuration - queryConfig.TimerangeDuration)
		log.WithFields(log.Fields{"query": queryName, "start_time": startTime, "current_time": currentTime}).Debug("Put time to gtsQueue")
		gtsq.Put(queryName, startTime)
	})
	if err != nil {
		gtsq.appConfig.Queries[queryName].CronEntryID = -1
		log.WithField(ec.FIELD, ec.LME_1608).WithFields(log.Fields{"query": queryName, "error": err}).Error("During registering the query in croniter the following error occurred")
	} else {
		gtsq.appConfig.Queries[queryName].CronEntryID = int(res)
		log.WithFields(log.Fields{"query": queryName, "cron_entry_id": int(res)}).Info("Query is registered in croniter")
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
		log.WithField(ec.FIELD, ec.LME_1624).WithField("query", queryName).Error("GTSQueue Put : Attempting to put timestamp for execution to channel for non-existent query")
		return
	}

	select {
	case c <- timestamp:
		size := len(c)
		log.WithFields(log.Fields{"query": queryName, "timestamp": timestamp, "size": size}).Debug("GTSQueue Put : put timestamp")
		gtsq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
	default:
		log.WithField(ec.FIELD, ec.LME_1625).WithFields(log.Fields{"query": queryName, "c_count": len(c)}).Error("GTSQueue Put : Attempting to put timestamp for execution to channel, but the channel is full")
	}
}

func (gtsq *GTSQueue) Get(queryName string) (time.Time, bool) {
	c := gtsq.timestampsByQuery[queryName]
	result, ok := <-c
	size := len(c)
	log.WithFields(log.Fields{"query": queryName, "result": result, "size": size}).Debug("GTSQueue Get : timestamp is extracted")
	gtsq.selfMonitorSetQueueSize(float64(size), queryName, time.Now())
	return result, ok
}

func (gtsq *GTSQueue) CloseChan(queryName string) {
	c := gtsq.timestampsByQuery[queryName]
	log.WithField("query", queryName).Info("GTSQueue CloseChan : chan is closed")
	close(c)
}

func (gtsq *GTSQueue) selfMonitorSetQueueSize(value float64, qName string, timestamp time.Time) {
	labels := make(map[string]string)
	labels["query_name"] = qName
	labels["queue_name"] = gtsQueueName
	selfmonitor.SetQueueSize(value, labels, &timestamp)
}
