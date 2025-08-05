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

package enrichers

import (
	"encoding/json"
	"fmt"
	"log_exporter/internal/config"
	"log_exporter/internal/queues"
	"log_exporter/internal/selfmonitor"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"reflect"
	"regexp"
	"sync"
	"time"

	"github.com/PaesslerAG/jsonpath"
	log "github.com/sirupsen/logrus"
)

const (
	REGEXP_NOT_MATCHED_DEFAULT_VALUE    = "NOT_MATCHED"
	JSON_NOT_PARSED_DEFAULT_VALUE       = "JSON_NOT_PARSED"
	JSONPATH_ERROR_DEFAULT_VALUE        = "JSONPATH_ERROR"
	JSONPATH_UNKNOWN_TYPE_DEFAULT_VALUE = "JSONPATH_UNKNOWN_TYPE"
)

type Enricher struct {
	jsonEnrich       bool
	regexpEnrich     bool
	uriReplaceEnrich []bool
}

func createEnricher(enrichConfig *config.EnrichConfig) *Enricher {
	enricher := Enricher{}

	enricher.jsonEnrich = (enrichConfig.JsonPath != "")
	enricher.regexpEnrich = (enrichConfig.Regexp != "")

	enricher.uriReplaceEnrich = make([]bool, 0, len(enrichConfig.DestFields))
	for _, destFieldConfig := range enrichConfig.DestFields {
		u := destFieldConfig.URIProcessing
		enricher.uriReplaceEnrich = append(enricher.uriReplaceEnrich, u.IDReplacer != "" || u.UUIDReplacer != "" || u.NumberReplacer != "" || u.FSMReplacer != "")
	}
	log.Debugf("Enricher created : jsonEnrich = %v, regexpEnrich = %v, uriReplaceEnrich = %v", enricher.jsonEnrich, enricher.regexpEnrich, enricher.uriReplaceEnrich)

	return &enricher
}

func Enrich(queryName string, graylogData *queues.GraylogData, queryConfig *config.QueryConfig) {
	log.Debug("enrichers.Enrich is called")

	if len(graylogData.Data) == 0 {
		log.Debugf("Nothing to enrich for query %v : No data", queryName)
		now := time.Now()
		for enrichIndex := range queryConfig.Enrich {
			selfMonitorObserveZeroEnrichEvaluationLatency(now, queryName, enrichIndex)
		}
		return
	}

	for enrichIndex, enrichConfig := range queryConfig.Enrich {
		enricher := createEnricher(&enrichConfig)
		enricher.addColumn(queryName, graylogData, enrichConfig, enrichIndex)
	}
}

func (e *Enricher) addColumn(queryName string, graylogData *queues.GraylogData, enrichConfig config.EnrichConfig, enrichIndex int) {
	enrichEvaluationStartTime := time.Now()
	defer func() {
		selfMonitorObserveEnrichEvaluationLatency(enrichEvaluationStartTime, queryName, enrichIndex)
	}()

	log.Debugf("Start processing enrich for query %v, enrich_index %v and source-field %v", queryName, enrichIndex, enrichConfig.SourceField)

	pattern := enrichConfig.RegexpCompiled
	if e.regexpEnrich && pattern == nil {
		log.WithField(ec.FIELD, ec.LME_1010).Errorf("Failed to add columns for query %v, enrich_index %v and source-field %v : for enricher pattern is not compiled", queryName, enrichIndex, enrichConfig.SourceField)
		return
	}
	data := graylogData.Data
	heading := data[0]

	sourceFieldIndex := utils.FindStringIndexInArray(heading, enrichConfig.SourceField)
	if sourceFieldIndex == -1 {
		log.WithField(ec.FIELD, ec.LME_1010).Errorf("Failed to add columns for query %v, enrich_index %v and source-field %v : Source-field is not found", queryName, enrichIndex, enrichConfig.SourceField)
		return
	}

	logLimited(data, fmt.Sprintf("Graylog data BEFORE PROCESSING for query %v, enrich_index %v and source-field %v :", queryName, enrichIndex, enrichConfig.SourceField))
	defer logLimited(data, fmt.Sprintf("Graylog data AFTER PROCESSING for query %v, enrich_index %v and source-field %v :", queryName, enrichIndex, enrichConfig.SourceField))

	for _, destFieldConfig := range enrichConfig.DestFields {
		data[0] = append(data[0], destFieldConfig.FieldName)
	}

	dataSize := len(data)
	threadsNumber := enrichConfig.Threads
	if threadsNumber > dataSize-1 {
		threadsNumber = dataSize - 1
	}
	if threadsNumber <= 1 {
		log.Debugf("Enrich for query %v, enrich_index %v and source-field %v will be executed in the single-thread mode", queryName, enrichIndex, enrichConfig.SourceField)
		e.addColumnTask(queryName, graylogData, enrichConfig, enrichIndex, pattern, sourceFieldIndex, 1, dataSize)
		return
	}

	log.Debugf("Enrich for query %v, enrich_index %v and source-field %v will be executed in the multi-thread mode (threads number is %v)", queryName, enrichIndex, enrichConfig.SourceField, threadsNumber)
	var wg sync.WaitGroup
	wg.Add(threadsNumber)
	for i := 0; i < threadsNumber; i++ {
		i := i
		start := 1 + i*(dataSize-1)/threadsNumber
		end := 1 + (i+1)*(dataSize-1)/threadsNumber
		log.Debugf("Enrich for query %v, enrich_index %v and source-field %v : thread %v , start = %v, end = %v, dataSize = %v", queryName, enrichIndex, enrichConfig.SourceField, i, start, end, dataSize)
		go func() {
			defer wg.Done()
			e.addColumnTask(queryName, graylogData, enrichConfig, enrichIndex, pattern, sourceFieldIndex, start, end)
		}()
	}
	wg.Wait()
}

func (e *Enricher) addColumnTask(queryName string, graylogData *queues.GraylogData, enrichConfig config.EnrichConfig, enrichIndex int, pattern *regexp.Regexp, sourceFieldIndex int, start int, end int) {
	if e.regexpEnrich {
		e.addColumnTaskWithRegexp(queryName, graylogData, enrichConfig, enrichIndex, pattern, sourceFieldIndex, start, end)
	} else {
		e.addColumnTaskWithoutRegexp(queryName, graylogData, enrichConfig, enrichIndex, sourceFieldIndex, start, end)
	}
}

func (e *Enricher) addColumnTaskWithRegexp(queryName string, graylogData *queues.GraylogData, enrichConfig config.EnrichConfig, enrichIndex int, pattern *regexp.Regexp, sourceFieldIndex int, start int, end int) {
	log.Debugf("addColumnTaskWithRegexp is called for query %v, enrichIndex %v, start %v, end %v", queryName, enrichIndex, start, end)
	var matched, notMatched = 0, 0
	now := time.Now()
	data := graylogData.Data
	defer func() {
		selfMonitorRegexps(queryName, enrichIndex, matched, notMatched, now)
	}()
	jsonErrorCountDown := 5
	for i := start; i < end; i++ {
		var content []byte
		if e.jsonEnrich {
			contentString, err := processJson(data[i][sourceFieldIndex], enrichConfig.JsonPath)
			if err != nil && jsonErrorCountDown > 0 {
				jsonErrorCountDown--
				log.WithField(ec.FIELD, ec.LME_1011).Errorf("Error applying jsonpath %v to jsonData %v : %+v", enrichConfig.JsonPath, data[i][sourceFieldIndex], err)
			}
			content = []byte(contentString)
		} else {
			content = []byte(data[i][sourceFieldIndex])
		}
		submatches := pattern.FindSubmatchIndex(content)
		//log.Debugf("### Submatches : %+v, pattern : %+v, content = %+v", submatches, pattern, string(content))
		for destFieldIndex, destFieldConfig := range enrichConfig.DestFields {
			var destFieldValueStr string
			if len(submatches) == 0 {
				notMatched++
				defaultValue := destFieldConfig.DefaultValue
				if defaultValue == "" {
					destFieldValueStr = REGEXP_NOT_MATCHED_DEFAULT_VALUE
				} else {
					destFieldValueStr = defaultValue
				}
			} else {
				matched++
				destFieldValue := []byte{}
				destFieldValue = pattern.Expand(destFieldValue, destFieldConfig.TemplateCompiled, content, submatches)
				if e.uriReplaceEnrich[destFieldIndex] {
					u := destFieldConfig.URIProcessing
					destFieldValueStr = utils.RemoveIDsFromURI(string(destFieldValue), u.UUIDReplacer, u.NumberReplacer, u.IDReplacer, u.IdDigitQuantity, u.FSMReplacer, u.FSMReplacerLimit)
				} else {
					destFieldValueStr = string(destFieldValue)
				}
			}
			data[i] = append(data[i], destFieldValueStr)
		}
	}
}

func (e *Enricher) addColumnTaskWithoutRegexp(queryName string, graylogData *queues.GraylogData, enrichConfig config.EnrichConfig, enrichIndex int, sourceFieldIndex int, start int, end int) {
	log.Debugf("addColumnTaskWithoutRegexp is called for query %v, enrichIndex %v, start %v, end %v", queryName, enrichIndex, start, end)

	jsonErrorCountDown := 5
	data := graylogData.Data
	for i := start; i < end; i++ {
		var content string
		var err error
		if e.jsonEnrich {
			content, err = processJson(data[i][sourceFieldIndex], enrichConfig.JsonPath)
			if err != nil && jsonErrorCountDown > 0 {
				jsonErrorCountDown--
				log.WithField(ec.FIELD, ec.LME_1011).Errorf("Error applying jsonpath %v to jsonData %v : %+v", enrichConfig.JsonPath, data[i][sourceFieldIndex], err)
			}
		} else {
			content = data[i][sourceFieldIndex]
		}
		for destFieldIndex, destFieldConfig := range enrichConfig.DestFields {
			var destFieldValue string
			if !e.uriReplaceEnrich[destFieldIndex] {
				destFieldValue = content
			} else {
				u := destFieldConfig.URIProcessing
				destFieldValue = utils.RemoveIDsFromURI(content, u.UUIDReplacer, u.NumberReplacer, u.IDReplacer, u.IdDigitQuantity, u.FSMReplacer, u.FSMReplacerLimit)
			}

			data[i] = append(data[i], destFieldValue)
		}
	}
}

func processJson(data string, jsonPath string) (string, error) {
	//data = "{\"body\":{\"value\":\"qwerty\"}}"
	var jsonData interface{}
	err := json.Unmarshal([]byte(data), &jsonData)
	if err != nil {
		return JSON_NOT_PARSED_DEFAULT_VALUE, err
	}
	result, err := jsonpath.Get(jsonPath, jsonData)
	if err != nil {
		return JSONPATH_ERROR_DEFAULT_VALUE, err
	}

	switch res := result.(type) {
	case string:
		return res, nil
	case []interface{}:
		return fmt.Sprintf("%+v", res), nil
	default:
		return JSONPATH_UNKNOWN_TYPE_DEFAULT_VALUE, fmt.Errorf("unknown type for jsonpath : %+v", reflect.TypeOf(result))
	}
}

func logLimited(data [][]string, message string) {
	const LIMIT = 2

	log.Debugf("%v", message)
	end1 := min(LIMIT, len(data))
	for i := 0; i < end1; i++ {
		log.Debugf("%v : %+v", i, data[i])
	}

	start2 := max(end1, len(data)-LIMIT)
	if start2 != end1 {
		log.Debug("...")
	}

	for i := start2; i < len(data); i++ {
		log.Debugf("%v : %+v", i, data[i])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func selfMonitorRegexps(queryName string, enrichIndex int, matched int, notMatched int, timestamp time.Time) {
	if matched >= 0 {
		labels := make(map[string]string)
		labels["query_name"] = queryName
		labels["enrich_index"] = fmt.Sprint(enrichIndex)
		selfmonitor.AddMatchedRegexpsCount(labels, float64(matched), &timestamp)
	}

	if notMatched >= 0 {
		labels := make(map[string]string)
		labels["query_name"] = queryName
		labels["enrich_index"] = fmt.Sprint(enrichIndex)
		selfmonitor.AddNotMatchedRegexpsCount(labels, float64(notMatched), &timestamp)
	}
}

func selfMonitorObserveEnrichEvaluationLatency(start time.Time, queryName string, enrichIndex int) {
	elapsed := time.Since(start)
	seconds := float64(elapsed) / float64(time.Second)
	labels := make(map[string]string)
	labels["query_name"] = queryName
	labels["enrich_index"] = fmt.Sprint(enrichIndex)
	selfmonitor.ObserveEnrichEvaluationLatency(labels, seconds, &start)
}

func selfMonitorObserveZeroEnrichEvaluationLatency(start time.Time, queryName string, enrichIndex int) {
	labels := make(map[string]string)
	labels["query_name"] = queryName
	labels["enrich_index"] = fmt.Sprint(enrichIndex)
	selfmonitor.ObserveEnrichEvaluationLatency(labels, 0, &start)
}
