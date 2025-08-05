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

package config

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var allowedMetricOperations = map[string]bool{
	"count":                true,
	"value":                true,
	"duration":             true,
	"duration-no-response": true,
}
var allowedMetricTypes = map[string]bool{
	"gauge":     true,
	"counter":   true,
	"histogram": true,
}

var countMetricsAllowedParams = map[string]bool{
	"init-value":    true,
	"default-value": true,
}

var valueMetricsAllowedParams = map[string]bool{
	"value-field":   true,
	"init-value":    true,
	"default-value": true,
}

var durationMetricsAllowedParams = map[string]bool{
	"time_field":            true,
	"time_format":           true,
	"message_type_field":    true,
	"message_type_request":  true,
	"message_type_response": true,
	"correlation_id_field":  true,
	"cache":                 true,
	"cache-update":          true,
	"init-value":            true,
	"default-value":         true,
}

var durationNoRespMetricsAllowedParams = map[string]bool{
	"cache_size":    true,
	"init-value":    true,
	"default-value": true,
}

var fieldValueParams = map[string]bool{
	"time_field":           true,
	"message_type_field":   true,
	"correlation_id_field": true,
	"value-field":          true,
}

const currentAPIVersion = int64(1)

var lastTimestampServicesCount int

func SimpleSilentRead(path string) (*Config, error) {
	config := Config{}
	configFile, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening config file %v : %+v", path, err)
	} else {
		if configFile != nil {
			defer func() {
				if err := configFile.Close(); err != nil {
					log.Errorf("error closing config file %v : %+v", path, err)
				}
			}()
		}
	}

	buf := bytes.Buffer{}

	_, err = io.Copy(&buf, configFile)
	if err != nil {
		return nil, fmt.Errorf("error copying config file %v : %+v", path, err)
	}

	err = yaml.Unmarshal(buf.Bytes(), &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling config file %v : %+v", path, err)
	}
	initDSName(&config)

	return &config, nil
}

func ValidateConfig(config *Config) error {
	startupBlockingErrors := make([]string, 0)
	log.Info("CONFIG VALIDATION STARTED")
	err := checkApiVersion(config.ApiVersion)
	if err != nil {
		startupBlockingErrors = append(startupBlockingErrors, err.Error())
	}

	if len(config.Datasources) != 1 {
		startupBlockingErrors = append(startupBlockingErrors, fmt.Sprintf("Section datasources : Datasources count is %v, but datasources count must be equal to 1", len(config.Datasources)))
	}

	for dsName, dsConfig := range config.Datasources {
		if dsConfig == nil || dsConfig.Host == "" /* || dsConfig.User == "" || dsConfig.Password == "" */ {
			startupBlockingErrors = append(startupBlockingErrors, fmt.Sprintf("Section datasources : Datasource %v must have field host defined", dsName))
		} else {
			_, err := url.ParseRequestURI(dsConfig.Host)
			if err != nil {
				startupBlockingErrors = append(startupBlockingErrors, fmt.Sprintf("Section datasources : Datasource %v must have correct host, current value %v is incorrect : %+v", dsName, dsConfig.Host, err))
			}
		}
	}

	pullExportersCount := 0
	pushExportersCount := 0
	lastTimestampServicesCount = 0
	for exportName, exportConfig := range config.Exports {
		if exportConfig == nil {
			startupBlockingErrors = append(startupBlockingErrors, fmt.Sprintf("Section exports : Export %v has empty configuration", exportName))
			continue
		}
		if exportConfig.Strategy == "" {
			exportConfig.Strategy = "push"
		}
		switch exportConfig.Strategy {
		case "push":
			_, err := url.ParseRequestURI(exportConfig.Host)
			if err != nil {
				startupBlockingErrors = append(startupBlockingErrors, fmt.Sprintf("Section exports : Export %v with 'push' strategy must have correct host, current value %v is incorrect : %+v", exportName, exportConfig.Host, err))
			}

			if exportConfig.LastTimestampHost != nil {
				lastTimestampServicesCount++
				if strings.ToUpper(exportConfig.LastTimestampHost.Host) != "NONE" {
					_, err := url.ParseRequestURI(exportConfig.LastTimestampHost.Host)
					if err != nil {
						startupBlockingErrors = append(startupBlockingErrors, fmt.Sprintf("Section exports : Export %v with 'push' strategy must have correct LastTimestampHost.Host, current value %v is incorrect : %+v", exportName, exportConfig.LastTimestampHost.Host, err))
					}
				}
			}
			pushExportersCount++
		case "pull":
			if exportConfig.Port == "" {
				startupBlockingErrors = append(startupBlockingErrors, fmt.Sprintf("Section exports : Export %v with 'pull' strategy must have field port specified", exportName))
			}
			pullExportersCount++
		default:
			startupBlockingErrors = append(startupBlockingErrors, fmt.Sprintf("Section exports : Unknown strategy %v for export %v", exportConfig.Strategy, exportName))
		}
	}

	if pullExportersCount > 1 {
		startupBlockingErrors = append(startupBlockingErrors, fmt.Sprintf("Section exports : Pull export count is %v, count can not be more than 1", pullExportersCount))
	}
	if pushExportersCount > 1 {
		startupBlockingErrors = append(startupBlockingErrors, fmt.Sprintf("Section exports : Push export count is %v, count can not be more than 1", pullExportersCount))
	}

	if len(config.Metrics) == 0 {
		startupBlockingErrors = append(startupBlockingErrors, "Section metrics : No metrics are specified")
	}

	if len(config.Queries) == 0 {
		startupBlockingErrors = append(startupBlockingErrors, "Section queries : No queries are specified")
	}

	performExportsNonBlockingChecks(config)
	performMetricsNonBlockingChecks(config)
	performQueriesNonBlockingChecks(config)
	performGeneralNonBlockingChecks(config)

	log.Info("CONFIG VALIDATION FINISHED")

	if len(startupBlockingErrors) != 0 {
		log.WithField(ec.FIELD, ec.LME_8101).Error("Log-exporter can not start with the provided configuration, see reasons list below :")
		for i, sbe := range startupBlockingErrors {
			log.WithField(ec.FIELD, ec.LME_8101).Errorf("%v. %+v", i+1, sbe)
		}
		return fmt.Errorf("yaml config is invalid")
	}
	return nil
}

func performExportsNonBlockingChecks(config *Config) {
	for exportName, exportConfig := range config.Exports {
		if exportConfig == nil {
			log.Warnf("Section exports : Export %v has empty configuration", exportName)
			continue
		}
		if exportConfig.Strategy == "push" || exportConfig.Strategy == "" {
			if exportConfig.Host == "" {
				log.Warnf("Section exports : Export %v with 'push' strategy must have field host specified", exportName)
			}
			if exportConfig.Endpoint == "" {
				log.Warnf("Section exports : Export %v with 'push' strategy must have field endpoint specified", exportName)
			}
			if exportConfig.LastTimestampHost != nil && exportConfig.LastTimestampHost.Host == "" {
				log.Warnf("Section exports : Export %v with 'push' strategy must have field host specified for the last-timestamp-host subsection", exportName)
			}
		}
	}
}

func performMetricsNonBlockingChecks(config *Config) {
	queryMetrics := getQueryMetricsCountMap(config)
	childMetrics := getChildMetricsCountMap(config)

	for metricName, metricConfig := range config.Metrics {
		if metricConfig == nil {
			log.Warnf("Section metrics : Metric %v has empty configuration", metricName)
			metricConfig = &MetricsConfig{}
		}
		for metricName := range config.Metrics {
			if queryMetrics[metricName] == 0 && childMetrics[metricName] == 0 {
				log.Warnf("Section metrics : Metric %v is not defined in any query and is not a child of any metric and will never be evaluated", metricName)
			} else if queryMetrics[metricName] > 0 && childMetrics[metricName] > 0 {
				log.Warnf("Section metrics : Metric %v is evaluated by %v query and at the same time is a child of %v metric, it may cause undefined behavior", metricName, queryMetrics[metricName], childMetrics[metricName])
			} else if queryMetrics[metricName] > 1 {
				log.Warnf("Section metrics : Metric %v is evaluated by %v queries at the same time, it may cause undefined behavior", metricName, queryMetrics[metricName])
			} else if childMetrics[metricName] > 1 {
				log.Warnf("Section metrics : Metric %v is a child of %v metrics at the same time, it may cause undefined behavior", metricName, childMetrics[metricName])
			}
		}

		if !allowedMetricOperations[metricConfig.Operation] {
			log.Warnf("Section metrics : Metric %v has unknown operation %v", metricName, metricConfig.Operation)
		}
		if !allowedMetricTypes[metricConfig.Type] {
			log.Warnf("Section metrics : Metric %v has unknown type %v", metricName, metricConfig.Type)
		}
		if metricConfig.Description == "" {
			log.Warnf("Section metrics : Metric %v has empty description", metricName)
		}

		if metricConfig.Type == "histogram" && len(metricConfig.Buckets) == 0 {
			log.Warnf("Section metrics : Metric %v of histogram type doesn't have buckets configured", metricName)
		} else if metricConfig.Type != "histogram" && len(metricConfig.Buckets) > 0 {
			log.Warnf("Section metrics : Metric %v of %v type has buckets configured", metricName, metricConfig.Type)
		} else if metricConfig.Type == "histogram" && len(metricConfig.Buckets) > 0 {
			buckets := make(map[float64]bool, len(metricConfig.Buckets))
			for _, bucketValue := range metricConfig.Buckets {
				if buckets[bucketValue] {
					log.Warnf("Section metrics : Metric %v of histogram type has duplicate bucket configured (bucket value is %v)", metricName, bucketValue)
				}
				buckets[bucketValue] = true
			}
		}

		if len(metricConfig.MultiValueFields) > 0 {
			if metricConfig.Operation != "count" {
				log.Warnf("Section metrics : Metric %v of %v operation has multi-value fields configured, which is supported only for the count operation", metricName, metricConfig.Operation)
			}
		}

		if metricConfig.IdField != "" && metricConfig.Operation != "count" {
			log.Warnf("Section metrics : Metric %v of %v operation has id-field configured, which is supported only for the count operation", metricName, metricConfig.Operation)
		}

		if metricConfig.MetricValue == "" && metricConfig.Operation == "value" {
			log.Warnf("Section metrics : Metric %v of value operation doesn't have metric-value configured", metricName)
		} else if metricConfig.MetricValue != "" && metricConfig.Operation != "value" {
			log.Warnf("Section metrics : Metric %v of %v operation has metric-value configured", metricName, metricConfig.Operation)
		}

		if len(metricConfig.ChildMetrics) > 0 && metricConfig.Operation != "duration" {
			log.Warnf("Section metrics : Metric %v of %v operation has child metrics configured, which is not supported", metricName, metricConfig.Operation)
		}
		if metricConfig.Operation == "count" {
			for paramName := range metricConfig.Parameters {
				if !countMetricsAllowedParams[paramName] {
					log.Warnf("Section metrics : Metric %v has not supported parameter %v for count operation", metricName, paramName)
				}
			}
		}

		if metricConfig.Operation == "value" {
			for paramName := range metricConfig.Parameters {
				if !valueMetricsAllowedParams[paramName] {
					log.Warnf("Section metrics : Metric %v has not supported parameter %v for value operation", metricName, paramName)
				}
			}
		}

		if metricConfig.Operation == "duration" {
			for _, childMetricName := range metricConfig.ChildMetrics {
				childMetricConfig := config.Metrics[childMetricName]
				if childMetricConfig == nil {
					log.Warnf("Section metrics : Metric %v has undefined child metrics %v", metricName, childMetricName)
				} else if childMetricConfig.Operation != "duration-no-response" {
					log.Warnf("Section metrics : Metric %v has child metrics %v with operation %v, which is not supported (Operation must be duration-no-response for the child metric)", metricName, childMetricName, childMetricConfig.Operation)
				}
			}
			if len(metricConfig.ChildMetrics) > 0 {
				childMetrics := make(map[string]bool)
				for _, childMetric := range metricConfig.ChildMetrics {
					if childMetrics[childMetric] {
						log.Warnf("Section metrics : Metric %v has duplicate child metrics %v configured", metricName, childMetric)
					} else {
						childMetrics[childMetric] = true
					}
				}
			}
			for paramName := range metricConfig.Parameters {
				if !durationMetricsAllowedParams[paramName] {
					log.Warnf("Section metrics : Metric %v has not supported parameter %v for duration operation", metricName, paramName)
				}
			}
		}

		if metricConfig.Operation == "duration-no-response" {
			for paramName := range metricConfig.Parameters {
				if !durationNoRespMetricsAllowedParams[paramName] {
					log.Warnf("Section metrics : Metric %v has not supported parameter %v for duration-no-response operation", metricName, paramName)
				}
			}
		}

		if metricConfig.Parameters["init-value"] != "" && metricConfig.Type == "gauge" {
			log.Warnf("Section metrics : Metric %v has not supported parameter init-value for gauge type", metricName)
		}

		if metricConfig.Parameters["default-value"] != "" && metricConfig.Type != "gauge" {
			log.Warnf("Section metrics : Metric %v has not supported parameter default-value for %v type", metricName, metricConfig.Type)
		}

		if metricConfig.Threads < 0 {
			log.Warnf("Section metrics : Metric %v has negative value %v for threads number", metricName, metricConfig.Threads)
		}

		if len(metricConfig.ExpectedLabels) == 0 {
			continue
		}

		totalLabels := make([]string, 0)
		totalLabels = append(totalLabels, metricConfig.LabelsInitial...)

		for label := range metricConfig.LabelFieldMap {
			if utils.FindStringIndexInArray(totalLabels, label) < 0 {
				totalLabels = append(totalLabels, label)
			}
		}
		labelsCount := len(totalLabels)
		for itemNum, expectedLabelsItem := range metricConfig.ExpectedLabels {
			if len(expectedLabelsItem) != labelsCount {
				log.Warnf("Section metrics : Invalid expected labels configuration for metric %v, itemNum %v : Metric has %v labels defined while in expected labels item %v labels defined", metricName, itemNum, labelsCount, len(expectedLabelsItem))
			}
			for _, labelName := range totalLabels {
				if len(expectedLabelsItem[labelName]) == 0 {
					log.Warnf("Section metrics : Invalid expected labels configuration for metric %v, itemNum %v : Metric has label %v defined while in expected labels this label is not defined", metricName, itemNum, labelName)
				}
			}
		}
	}
}

func performQueriesNonBlockingChecks(config *Config) {
	isNewRelic := config.Datasources[config.DsName].Type == "newrelic"
	parser := utils.GetCronParser()
	pushExport := getPushExport(config)
	for queryName, queryConfig := range config.Queries {
		if queryConfig == nil {
			log.Warnf("Section queries : For query %v configuration is empty", queryName)
			queryConfig = &QueryConfig{}
		}

		metrics := make(map[string]bool, len(queryConfig.Metrics))
		for _, metricName := range queryConfig.Metrics {
			if config.Metrics[metricName] == nil {
				log.Warnf("Section queries : For query %v metric %v is configured, but this metric is not defined in metrics section", queryName, metricName)
			}
			if metrics[metricName] {
				log.Warnf("Section queries : For query %v metric %v is configured more than once", queryName, metricName)
			} else {
				metrics[metricName] = true
			}
		}

		if len(queryConfig.QueryString) == 0 {
			log.Warnf("Section queries : For query %v query_string is empty", queryName)
		}

		if len(queryConfig.Timerange) == 0 {
			log.Warnf("Section queries : For query %v timerange is empty", queryName)
		} else {
			_, err := time.ParseDuration(queryConfig.Timerange)
			if err != nil {
				log.Warnf("Section queries : For query %v timerange %v can not be parsed as duration : %+v", queryName, queryConfig.Timerange, err)
			}
		}

		if len(queryConfig.FieldsInOrder) == 0 && !isNewRelic {
			log.Warnf("Section queries : For query %v fields_in_order list is empty", queryName)
		}

		if len(queryConfig.Croniter) == 0 {
			log.Warnf("Section queries : For query %v croniter is empty", queryName)
		} else {
			_, err := parser.Parse(queryConfig.Croniter)
			if err != nil {
				log.Warnf("Section queries : For query %v croniter %v is invalid : %+v", queryName, queryConfig.Croniter, err)
			}
		}

		if len(queryConfig.Interval) == 0 {
			log.Warnf("Section queries : For query %v interval is empty", queryName)
		} else {
			_, err := time.ParseDuration(queryConfig.Interval)
			if err != nil {
				log.Warnf("Section queries : For query %v interval %v can not be parsed as duration : %+v", queryName, queryConfig.Interval, err)
			}
		}

		if len(queryConfig.QueryLag) == 0 {
			log.Warnf("Section queries : For query %v query_lag is empty", queryName)
		} else {
			_, err := time.ParseDuration(queryConfig.QueryLag)
			if err != nil {
				log.Warnf("Section queries : For query %v query_lag %v can not be parsed as duration : %+v", queryName, queryConfig.QueryLag, err)
			}
		}

		if queryConfig.GTSQueueSize != "" {
			val, err := strconv.ParseInt(queryConfig.GTSQueueSize, 10, 64)
			if err != nil {
				log.Warnf("Section queries : For query %v gts-queue-size %v can not be parsed as int : %+v", queryName, queryConfig.GTSQueueSize, err)
			} else if val < 0 {
				log.Warnf("Section queries : For query %v gts-queue-size %v is negative", queryName, val)
			}
		}

		if queryConfig.GDQueueSize != "" {
			val, err := strconv.ParseInt(queryConfig.GDQueueSize, 10, 64)
			if err != nil {
				log.Warnf("Section queries : For query %v gd-queue-size %v can not be parsed as int : %+v", queryName, queryConfig.GDQueueSize, err)
			} else if val < 0 {
				log.Warnf("Section queries : For query %v gd-queue-size %v is negative", queryName, val)
			}
		}

		if queryConfig.GMQueueSize != "" {
			val, err := strconv.ParseInt(queryConfig.GMQueueSize, 10, 64)
			if err != nil {
				log.Warnf("Section queries : For query %v gm-queue-size %v can not be parsed as int : %+v", queryName, queryConfig.GMQueueSize, err)
			} else if val < 0 {
				log.Warnf("Section queries : For query %v gm-queue-size %v is negative", queryName, val)
			}
		}

		if queryConfig.MaxHistoryLookup != "" {
			_, err := time.ParseDuration(queryConfig.MaxHistoryLookup)
			if err != nil {
				log.Warnf("Section queries : For query %v max-history-lookup %v can not be parsed as duration : %+v", queryName, queryConfig.QueryLag, err)
			}
		}

		if pushExport != nil && pushExport.LastTimestampHost != nil {
			if pushExport.LastTimestampHost.Endpoint == "" && queryConfig.LastTimestampEndpoint == "" {
				log.Warnf("Section queries : For query %v last-timestamp-endpoint must be set, because push exporter is configured with last-timestamp-host", queryName)
			}
			if pushExport.LastTimestampHost.JsonPath == "" && queryConfig.LastTimestampJsonPath == "" {
				log.Warnf("Section queries : For query %v last-timestamp-json-path must be set, because push exporter is configured with last-timestamp-host", queryName)
			}
		}

		if pushExport == nil || pushExport.LastTimestampHost == nil {
			if queryConfig.LastTimestampEndpoint != "" {
				log.Warnf("Section queries : For query %v last-timestamp-endpoint is set, but push exporter with last-timestamp-host is not configured", queryName)
			}
			if queryConfig.LastTimestampJsonPath != "" {
				log.Warnf("Section queries : For query %v last-timestamp-json-path is set, but push exporter with last-timestamp-host is not configured", queryName)
			}
		}

		if isNewRelic {
			continue
		}

		usedFields := make(map[string]bool)
		availableFields := make(map[string]bool)
		for _, field := range queryConfig.FieldsInOrder {
			if availableFields[field] {
				log.Warnf("Section queries : For query %v fields_in_order list contains duplicate value %v", queryName, field)
			}
			availableFields[field] = true
		}
		for enrichIndex, enrichConfig := range queryConfig.Enrich {
			if enrichConfig.SourceField == "" {
				log.Warnf("Section queries : For query %v enrich %v sourceField is empty", queryName, enrichIndex)
			} else if !availableFields[enrichConfig.SourceField] {
				log.Warnf("Section queries : For query %v enrich %v sourceField %v is referring to not available sourceField", queryName, enrichIndex, enrichConfig.SourceField)
			} else {
				usedFields[enrichConfig.SourceField] = true
			}
			if enrichConfig.Regexp != "" {
				_, err := regexp.Compile(enrichConfig.Regexp)
				if err != nil {
					log.Warnf("Section queries : For query %v enrich %v regexp %v is compiling with errors : %+v", queryName, enrichIndex, enrichConfig.Regexp, err)
				}
				for destFieldIndex, destField := range enrichConfig.DestFields {
					if destField.Template == "" {
						log.Warnf("Section queries : For query %v enrich %v destField %v template is empty, but regexp is specified for enrich", queryName, enrichIndex, destFieldIndex)
					}
				}
			} else {
				for destFieldIndex, destField := range enrichConfig.DestFields {
					if destField.Template != "" {
						log.Warnf("Section queries : For query %v enrich %v destField %v template is set, but regexp is not specified for enrich", queryName, enrichIndex, destFieldIndex)
					}
				}
			}
			for destFieldIndex, destField := range enrichConfig.DestFields {
				availableFields[destField.FieldName] = true
				if destField.URIProcessing.IdDigitQuantity < 0 {
					log.Warnf("Section queries : For query %v enrich %v destField %v uri-processing.id-digit-quantity is %v which is less than 0", queryName, enrichIndex, destFieldIndex, destField.URIProcessing.IdDigitQuantity)
				}
			}
			if enrichConfig.Threads < 0 {
				log.Warnf("Section queries : For query %v enrich %v threads count %v is negative", queryName, enrichIndex, enrichConfig.Threads)
			}
		}
		for _, metricName := range queryConfig.Metrics {
			metricConfig := config.Metrics[metricName]
			if metricConfig == nil {
				continue
			}
			if metricConfig.Parameters != nil {
				for paramName := range fieldValueParams {
					fieldName := metricConfig.Parameters[paramName]
					if fieldName != "" {
						if !availableFields[fieldName] {
							log.Warnf("Section queries : For query %v metric %v requests field with the name %v, but this field is not evaluated by the query", queryName, metricName, fieldName)
						}
						usedFields[fieldName] = true
					}
				}
			}
			if metricConfig.MetricValue != "" {
				if !availableFields[metricConfig.MetricValue] {
					log.Warnf("Section queries : For query %v metric %v requests field with the name %v, but this field is not evaluated by the query", queryName, metricName, metricConfig.MetricValue)
				}
				usedFields[metricConfig.MetricValue] = true
			}
			for _, label := range metricConfig.LabelsInitial {
				if !availableFields[label] {
					log.Warnf("Section queries : For query %v metric %v requests field with the name %v, but this field is not evaluated by the query", queryName, metricName, label)
				}
				usedFields[label] = true
			}
			for _, field := range metricConfig.LabelFieldMap {
				if !availableFields[field] {
					log.Warnf("Section queries : For query %v metric %v requests field with the name %v, but this field is not evaluated by the query", queryName, metricName, field)
				}
				usedFields[field] = true
			}
			for _, mvfc := range metricConfig.MultiValueFields {
				if !availableFields[mvfc.FieldName] {
					log.Warnf("Section queries : For query %v metric %v requests field with the name %v, but this field is not evaluated by the query", queryName, metricName, mvfc.FieldName)
				}
				usedFields[mvfc.FieldName] = true
			}
			if metricConfig.IdField != "" && metricConfig.Operation == "count" {
				usedFields[metricConfig.IdField] = true
			}
		}
		if len(availableFields) > 1 {
			for availableField := range availableFields {
				if !usedFields[availableField] {
					log.Warnf("Section queries : For query %v field %v is evaluated but is never used", queryName, availableField)
				}
			}
		}
	}
}

func performGeneralNonBlockingChecks(config *Config) {
	if config.General == nil {
		return
	}
	if config.General.GMQueueSelfMonSize != "" {
		_, err := strconv.ParseInt(config.General.GMQueueSelfMonSize, 10, 64)
		if err != nil {
			log.Warnf("Section general : Parameter gm-queue-self-mon-size %v can not be parsed as int : %+v", config.General.GMQueueSelfMonSize, err)
		}
	}
	if config.General.LTSRetryCount != "" {
		_, err := strconv.ParseInt(config.General.LTSRetryCount, 10, 64)
		if err != nil {
			log.Warnf("Section general : Parameter last-timestamp-retry-count %v can not be parsed as int : %+v", config.General.LTSRetryCount, err)
		}
	}
	if config.General.LTSRetryPeriod != "" {
		_, err := time.ParseDuration(config.General.LTSRetryPeriod)
		if err != nil {
			log.Warnf("Section general : Parameter last-timestamp-retry-period %v can not be parsed as duration : %+v", config.General.LTSRetryPeriod, err)
		}
	}

}

func getQueryMetricsCountMap(config *Config) map[string]int {
	result := make(map[string]int)

	for _, queryConfig := range config.Queries {
		if queryConfig == nil {
			continue
		}
		for _, metricName := range queryConfig.Metrics {
			result[metricName]++
		}
	}

	return result
}

func getChildMetricsCountMap(config *Config) map[string]int {
	result := make(map[string]int)

	for _, metricConfig := range config.Metrics {
		if metricConfig == nil {
			continue
		}
		for _, childMetric := range metricConfig.ChildMetrics {
			result[childMetric]++
		}
	}

	return result
}

func getPushExport(config *Config) *ExportConfig {
	for _, exportConfig := range config.Exports {
		if exportConfig != nil && (exportConfig.Strategy == "push" || exportConfig.Strategy == "") {
			return exportConfig
		}
	}
	return nil
}

func checkApiVersion(apiVersion string) error {
	if apiVersion == "" {
		log.Info("apiVersion is not defined in yaml")
		return nil
	}

	if apiVersion[0] == 'v' {
		apiVersion = apiVersion[1:]
	}
	apiVersions := strings.Split(apiVersion, ".")
	version1, err := strconv.ParseInt(apiVersions[0], 10, 64)
	if err != nil {
		return fmt.Errorf("section apiVersion : can not parse apiVersion %v from config file, log-exporter can not start", apiVersions[0])
	}
	if version1 < currentAPIVersion {
		return fmt.Errorf("section apiVersion : minimal supported config version is %v, config file has version %v, log-exporter can not start", currentAPIVersion, version1)
	}
	log.Infof("apiVersion check completed successfully, apiVersion is %v, minimal supported version is %v", version1, currentAPIVersion)
	return nil
}
