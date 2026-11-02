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
					log.WithFields(log.Fields{"path": path, "error": err}).Error("Error closing the config file")
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
			log.WithField(ec.FIELD, ec.LME_8101).WithFields(log.Fields{"index": i + 1, "error": sbe}).Error("Startup blocking error")
		}
		return fmt.Errorf("yaml config is invalid")
	}
	return nil
}

func performExportsNonBlockingChecks(config *Config) {
	for exportName, exportConfig := range config.Exports {
		if exportConfig == nil {
			log.WithField("export", exportName).Warn("Section exports : Export has empty configuration")
			continue
		}
		if exportConfig.Strategy == "push" || exportConfig.Strategy == "" {
			if exportConfig.Host == "" {
				log.WithField("export", exportName).Warn("Section exports : Export with 'push' strategy must have field host specified")
			}
			if exportConfig.Endpoint == "" {
				log.WithField("export", exportName).Warn("Section exports : Export with 'push' strategy must have field endpoint specified")
			}
			if exportConfig.LastTimestampHost != nil && exportConfig.LastTimestampHost.Host == "" {
				log.WithField("export", exportName).Warn("Section exports : Export with 'push' strategy must have field host specified for the last-timestamp-host subsection")
			}
		}
	}
}

func performMetricsNonBlockingChecks(config *Config) {
	queryMetrics := getQueryMetricsCountMap(config)
	childMetrics := getChildMetricsCountMap(config)

	for metricName, metricConfig := range config.Metrics {
		if metricConfig == nil {
			log.WithField("metric", metricName).Warn("Section metrics : Metric has empty configuration")
			metricConfig = &MetricsConfig{}
		}
		for metricName := range config.Metrics {
			if queryMetrics[metricName] == 0 && childMetrics[metricName] == 0 {
				log.WithField("metric", metricName).Warn("Section metrics : Metric is not defined in any query and is not a child of any metric and will never be evaluated")
			} else if queryMetrics[metricName] > 0 && childMetrics[metricName] > 0 {
				log.WithFields(log.Fields{"metric": metricName, "query_metric_type": queryMetrics[metricName], "child_metric_type": childMetrics[metricName]}).Warn("Section metrics : Metric is evaluated by a query and at the same time is a child of a metric, it may cause undefined behavior")
			} else if queryMetrics[metricName] > 1 {
				log.WithFields(log.Fields{"metric": metricName, "query_metric_type": queryMetrics[metricName]}).Warn("Section metrics : Metric is evaluated by several queries at the same time, it may cause undefined behavior")
			} else if childMetrics[metricName] > 1 {
				log.WithFields(log.Fields{"metric": metricName, "child_metric_type": childMetrics[metricName]}).Warn("Section metrics : Metric is a child of several metrics at the same time, it may cause undefined behavior")
			}
		}

		if !allowedMetricOperations[metricConfig.Operation] {
			log.WithFields(log.Fields{"metric": metricName, "operation": metricConfig.Operation}).Warn("Section metrics : Metric has an unknown operation")
		}
		if !allowedMetricTypes[metricConfig.Type] {
			log.WithFields(log.Fields{"metric": metricName, "metric_config_type": metricConfig.Type}).Warn("Section metrics : Metric has an unknown type")
		}
		if metricConfig.Description == "" {
			log.WithField("metric", metricName).Warn("Section metrics : Metric has empty description")
		}

		if metricConfig.Type == "histogram" && len(metricConfig.Buckets) == 0 {
			log.WithField("metric", metricName).Warn("Section metrics : Metric of histogram type doesn't have buckets configured")
		} else if metricConfig.Type != "histogram" && len(metricConfig.Buckets) > 0 {
			log.WithFields(log.Fields{"metric": metricName, "metric_config_type": metricConfig.Type}).Warn("Section metrics : Metric of a non-histogram type has buckets configured")
		} else if metricConfig.Type == "histogram" && len(metricConfig.Buckets) > 0 {
			buckets := make(map[float64]bool, len(metricConfig.Buckets))
			for _, bucketValue := range metricConfig.Buckets {
				if buckets[bucketValue] {
					log.WithFields(log.Fields{"metric": metricName, "bucket_value": bucketValue}).Warn("Section metrics : Metric of histogram type has a duplicate bucket configured")
				}
				buckets[bucketValue] = true
			}
		}

		if len(metricConfig.MultiValueFields) > 0 {
			if metricConfig.Operation != "count" {
				log.WithFields(log.Fields{"metric": metricName, "operation": metricConfig.Operation}).Warn("Section metrics : Metric has multi-value fields configured, which is supported only for the count operation")
			}
		}

		if metricConfig.IdField != "" && metricConfig.Operation != "count" {
			log.WithFields(log.Fields{"metric": metricName, "operation": metricConfig.Operation}).Warn("Section metrics : Metric has id-field configured, which is supported only for the count operation")
		}

		if metricConfig.MetricValue == "" && metricConfig.Operation == "value" {
			log.WithField("metric", metricName).Warn("Section metrics : Metric of value operation doesn't have metric-value configured")
		} else if metricConfig.MetricValue != "" && metricConfig.Operation != "value" {
			log.WithFields(log.Fields{"metric": metricName, "operation": metricConfig.Operation}).Warn("Section metrics : Metric of a non-value operation has metric-value configured")
		}

		if len(metricConfig.ChildMetrics) > 0 && metricConfig.Operation != "duration" {
			log.WithFields(log.Fields{"metric": metricName, "operation": metricConfig.Operation}).Warn("Section metrics : Metric has child metrics configured, which is not supported for this operation")
		}
		if metricConfig.Operation == "count" {
			for paramName := range metricConfig.Parameters {
				if !countMetricsAllowedParams[paramName] {
					log.WithFields(log.Fields{"metric": metricName, "param_name": paramName}).Warn("Section metrics : Metric has an unsupported parameter for the count operation")
				}
			}
		}

		if metricConfig.Operation == "value" {
			for paramName := range metricConfig.Parameters {
				if !valueMetricsAllowedParams[paramName] {
					log.WithFields(log.Fields{"metric": metricName, "param_name": paramName}).Warn("Section metrics : Metric has an unsupported parameter for the value operation")
				}
			}
		}

		if metricConfig.Operation == "duration" {
			for _, childMetricName := range metricConfig.ChildMetrics {
				childMetricConfig := config.Metrics[childMetricName]
				if childMetricConfig == nil {
					log.WithFields(log.Fields{"metric": metricName, "child_metric_name": childMetricName}).Warn("Section metrics : Metric has an undefined child metric")
				} else if childMetricConfig.Operation != "duration-no-response" {
					log.WithFields(log.Fields{"metric": metricName, "child_metric_name": childMetricName, "operation": childMetricConfig.Operation}).Warn("Section metrics : Metric has a child metric with an unsupported operation (Operation must be duration-no-response for the child metric)")
				}
			}
			if len(metricConfig.ChildMetrics) > 0 {
				childMetrics := make(map[string]bool)
				for _, childMetric := range metricConfig.ChildMetrics {
					if childMetrics[childMetric] {
						log.WithFields(log.Fields{"metric": metricName, "child_metric": childMetric}).Warn("Section metrics : Metric has a duplicate child metric configured")
					} else {
						childMetrics[childMetric] = true
					}
				}
			}
			for paramName := range metricConfig.Parameters {
				if !durationMetricsAllowedParams[paramName] {
					log.WithFields(log.Fields{"metric": metricName, "param_name": paramName}).Warn("Section metrics : Metric has an unsupported parameter for the duration operation")
				}
			}
		}

		if metricConfig.Operation == "duration-no-response" {
			for paramName := range metricConfig.Parameters {
				if !durationNoRespMetricsAllowedParams[paramName] {
					log.WithFields(log.Fields{"metric": metricName, "param_name": paramName}).Warn("Section metrics : Metric has an unsupported parameter for the duration-no-response operation")
				}
			}
		}

		if metricConfig.Parameters["init-value"] != "" && metricConfig.Type == "gauge" {
			log.WithField("metric", metricName).Warn("Section metrics : Metric has the unsupported parameter init-value for gauge type")
		}

		if metricConfig.Parameters["default-value"] != "" && metricConfig.Type != "gauge" {
			log.WithFields(log.Fields{"metric": metricName, "metric_config_type": metricConfig.Type}).Warn("Section metrics : Metric has the unsupported parameter default-value for a non-gauge type")
		}

		if metricConfig.Threads < 0 {
			log.WithFields(log.Fields{"metric": metricName, "threads": metricConfig.Threads}).Warn("Section metrics : Metric has a negative value for threads number")
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
				log.WithFields(log.Fields{"metric": metricName, "item_num": itemNum, "labels_count": labelsCount, "expected_labels_item_count": len(expectedLabelsItem)}).Warn("Section metrics : Invalid expected labels configuration, the metric label count differs from the expected labels item count")
			}
			for _, labelName := range totalLabels {
				if len(expectedLabelsItem[labelName]) == 0 {
					log.WithFields(log.Fields{"metric": metricName, "item_num": itemNum, "label_name": labelName}).Warn("Section metrics : Invalid expected labels configuration, the metric has a label that is not defined in expected labels")
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
			log.WithField("query", queryName).Warn("Section queries : Query configuration is empty")
			queryConfig = &QueryConfig{}
		}

		metrics := make(map[string]bool, len(queryConfig.Metrics))
		for _, metricName := range queryConfig.Metrics {
			if config.Metrics[metricName] == nil {
				log.WithFields(log.Fields{"query": queryName, "metric": metricName}).Warn("Section queries : Metric is configured for the query, but this metric is not defined in the metrics section")
			}
			if metrics[metricName] {
				log.WithFields(log.Fields{"query": queryName, "metric": metricName}).Warn("Section queries : Metric is configured more than once for the query")
			} else {
				metrics[metricName] = true
			}
		}

		if len(queryConfig.QueryString) == 0 {
			log.WithField("query", queryName).Warn("Section queries : query_string is empty")
		}

		if len(queryConfig.Timerange) == 0 {
			log.WithField("query", queryName).Warn("Section queries : timerange is empty")
		} else {
			_, err := time.ParseDuration(queryConfig.Timerange)
			if err != nil {
				log.WithFields(log.Fields{"query": queryName, "timerange": queryConfig.Timerange, "error": err}).Warn("Section queries : timerange can not be parsed as duration")
			}
		}

		if len(queryConfig.FieldsInOrder) == 0 && !isNewRelic {
			log.WithField("query", queryName).Warn("Section queries : fields_in_order list is empty")
		}

		if len(queryConfig.Croniter) == 0 {
			log.WithField("query", queryName).Warn("Section queries : croniter is empty")
		} else {
			_, err := parser.Parse(queryConfig.Croniter)
			if err != nil {
				log.WithFields(log.Fields{"query": queryName, "croniter": queryConfig.Croniter, "error": err}).Warn("Section queries : croniter is invalid")
			}
		}

		if len(queryConfig.Interval) == 0 {
			log.WithField("query", queryName).Warn("Section queries : interval is empty")
		} else {
			_, err := time.ParseDuration(queryConfig.Interval)
			if err != nil {
				log.WithFields(log.Fields{"query": queryName, "interval": queryConfig.Interval, "error": err}).Warn("Section queries : interval can not be parsed as duration")
			}
		}

		if len(queryConfig.QueryLag) == 0 {
			log.WithField("query", queryName).Warn("Section queries : query_lag is empty")
		} else {
			_, err := time.ParseDuration(queryConfig.QueryLag)
			if err != nil {
				log.WithFields(log.Fields{"query": queryName, "query_lag": queryConfig.QueryLag, "error": err}).Warn("Section queries : query_lag can not be parsed as duration")
			}
		}

		if queryConfig.GTSQueueSize != "" {
			val, err := strconv.ParseInt(queryConfig.GTSQueueSize, 10, 64)
			if err != nil {
				log.WithFields(log.Fields{"query": queryName, "gts_queue_size": queryConfig.GTSQueueSize, "error": err}).Warn("Section queries : gts-queue-size can not be parsed as int")
			} else if val < 0 {
				log.WithFields(log.Fields{"query": queryName, "val": val}).Warn("Section queries : gts-queue-size is negative")
			}
		}

		if queryConfig.GDQueueSize != "" {
			val, err := strconv.ParseInt(queryConfig.GDQueueSize, 10, 64)
			if err != nil {
				log.WithFields(log.Fields{"query": queryName, "gd_queue_size": queryConfig.GDQueueSize, "error": err}).Warn("Section queries : gd-queue-size can not be parsed as int")
			} else if val < 0 {
				log.WithFields(log.Fields{"query": queryName, "val": val}).Warn("Section queries : gd-queue-size is negative")
			}
		}

		if queryConfig.GMQueueSize != "" {
			val, err := strconv.ParseInt(queryConfig.GMQueueSize, 10, 64)
			if err != nil {
				log.WithFields(log.Fields{"query": queryName, "gm_queue_size": queryConfig.GMQueueSize, "error": err}).Warn("Section queries : gm-queue-size can not be parsed as int")
			} else if val < 0 {
				log.WithFields(log.Fields{"query": queryName, "val": val}).Warn("Section queries : gm-queue-size is negative")
			}
		}

		if queryConfig.MaxHistoryLookup != "" {
			_, err := time.ParseDuration(queryConfig.MaxHistoryLookup)
			if err != nil {
				log.WithFields(log.Fields{"query": queryName, "query_lag": queryConfig.QueryLag, "error": err}).Warn("Section queries : max-history-lookup can not be parsed as duration")
			}
		}

		if pushExport != nil && pushExport.LastTimestampHost != nil {
			if pushExport.LastTimestampHost.Endpoint == "" && queryConfig.LastTimestampEndpoint == "" {
				log.WithField("query", queryName).Warn("Section queries : last-timestamp-endpoint must be set, because the push exporter is configured with last-timestamp-host")
			}
			if pushExport.LastTimestampHost.JsonPath == "" && queryConfig.LastTimestampJsonPath == "" {
				log.WithField("query", queryName).Warn("Section queries : last-timestamp-json-path must be set, because the push exporter is configured with last-timestamp-host")
			}
		}

		if pushExport == nil || pushExport.LastTimestampHost == nil {
			if queryConfig.LastTimestampEndpoint != "" {
				log.WithField("query", queryName).Warn("Section queries : last-timestamp-endpoint is set, but a push exporter with last-timestamp-host is not configured")
			}
			if queryConfig.LastTimestampJsonPath != "" {
				log.WithField("query", queryName).Warn("Section queries : last-timestamp-json-path is set, but a push exporter with last-timestamp-host is not configured")
			}
		}

		if isNewRelic {
			continue
		}

		usedFields := make(map[string]bool)
		availableFields := make(map[string]bool)
		for _, field := range queryConfig.FieldsInOrder {
			if availableFields[field] {
				log.WithFields(log.Fields{"query": queryName, "field": field}).Warn("Section queries : fields_in_order list contains a duplicate value")
			}
			availableFields[field] = true
		}
		for enrichIndex, enrichConfig := range queryConfig.Enrich {
			if enrichConfig.SourceField == "" {
				log.WithFields(log.Fields{"query": queryName, "enrich_index": enrichIndex}).Warn("Section queries : enrich sourceField is empty")
			} else if !availableFields[enrichConfig.SourceField] {
				log.WithFields(log.Fields{"query": queryName, "enrich_index": enrichIndex, "source_field": enrichConfig.SourceField}).Warn("Section queries : enrich sourceField is referring to a sourceField that is not available")
			} else {
				usedFields[enrichConfig.SourceField] = true
			}
			if enrichConfig.Regexp != "" {
				_, err := regexp.Compile(enrichConfig.Regexp)
				if err != nil {
					log.WithFields(log.Fields{"query": queryName, "enrich_index": enrichIndex, "regexp": enrichConfig.Regexp, "error": err}).Warn("Section queries : enrich regexp is compiling with errors")
				}
				for destFieldIndex, destField := range enrichConfig.DestFields {
					if destField.Template == "" {
						log.WithFields(log.Fields{"query": queryName, "enrich_index": enrichIndex, "dest_field_index": destFieldIndex}).Warn("Section queries : enrich destField template is empty, but regexp is specified for enrich")
					}
				}
			} else {
				for destFieldIndex, destField := range enrichConfig.DestFields {
					if destField.Template != "" {
						log.WithFields(log.Fields{"query": queryName, "enrich_index": enrichIndex, "dest_field_index": destFieldIndex}).Warn("Section queries : enrich destField template is set, but regexp is not specified for enrich")
					}
				}
			}
			for destFieldIndex, destField := range enrichConfig.DestFields {
				availableFields[destField.FieldName] = true
				if destField.URIProcessing.IdDigitQuantity < 0 {
					log.WithFields(log.Fields{"query": queryName, "enrich_index": enrichIndex, "dest_field_index": destFieldIndex, "id_digit_quantity": destField.URIProcessing.IdDigitQuantity}).Warn("Section queries : enrich destField uri-processing.id-digit-quantity is less than 0")
				}
			}
			if enrichConfig.Threads < 0 {
				log.WithFields(log.Fields{"query": queryName, "enrich_index": enrichIndex, "threads": enrichConfig.Threads}).Warn("Section queries : enrich threads count is negative")
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
							log.WithFields(log.Fields{"query": queryName, "metric": metricName, "field_name": fieldName}).Warn("Section queries : Metric requests a field that is not evaluated by the query")
						}
						usedFields[fieldName] = true
					}
				}
			}
			if metricConfig.MetricValue != "" {
				if !availableFields[metricConfig.MetricValue] {
					log.WithFields(log.Fields{"query": queryName, "metric": metricName, "metric_value": metricConfig.MetricValue}).Warn("Section queries : Metric requests a field that is not evaluated by the query")
				}
				usedFields[metricConfig.MetricValue] = true
			}
			for _, label := range metricConfig.LabelsInitial {
				if !availableFields[label] {
					log.WithFields(log.Fields{"query": queryName, "metric": metricName, "label": label}).Warn("Section queries : Metric requests a field that is not evaluated by the query")
				}
				usedFields[label] = true
			}
			for _, field := range metricConfig.LabelFieldMap {
				if !availableFields[field] {
					log.WithFields(log.Fields{"query": queryName, "metric": metricName, "field": field}).Warn("Section queries : Metric requests a field that is not evaluated by the query")
				}
				usedFields[field] = true
			}
			for _, mvfc := range metricConfig.MultiValueFields {
				if !availableFields[mvfc.FieldName] {
					log.WithFields(log.Fields{"query": queryName, "metric": metricName, "field_name": mvfc.FieldName}).Warn("Section queries : Metric requests a field that is not evaluated by the query")
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
					log.WithFields(log.Fields{"query": queryName, "available_field": availableField}).Warn("Section queries : Field is evaluated but is never used")
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
			log.WithFields(log.Fields{"gm_queue_self_mon_size": config.General.GMQueueSelfMonSize, "error": err}).Warn("Section general : Parameter gm-queue-self-mon-size can not be parsed as int")
		}
	}
	if config.General.LTSRetryCount != "" {
		_, err := strconv.ParseInt(config.General.LTSRetryCount, 10, 64)
		if err != nil {
			log.WithFields(log.Fields{"lts_retry_count": config.General.LTSRetryCount, "error": err}).Warn("Section general : Parameter last-timestamp-retry-count can not be parsed as int")
		}
	}
	if config.General.LTSRetryPeriod != "" {
		_, err := time.ParseDuration(config.General.LTSRetryPeriod)
		if err != nil {
			log.WithFields(log.Fields{"lts_retry_period": config.General.LTSRetryPeriod, "error": err}).Warn("Section general : Parameter last-timestamp-retry-period can not be parsed as duration")
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
	log.WithFields(log.Fields{"version1": version1, "current_api_version": currentAPIVersion}).Info("apiVersion check completed successfully")
	return nil
}
