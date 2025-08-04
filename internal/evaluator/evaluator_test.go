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

package evaluator

import (
	"fmt"
	"log_exporter/internal/config"
	"log_exporter/internal/evaluator/enrichers"
	"log_exporter/internal/httpservice"
	"log_exporter/internal/queues"
	"log_exporter/internal/registry"
	"log_exporter/internal/selfmonitor"
	"strconv"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

var expectedResults = make(map[string]map[string]bool)

func initER() {
	expectedResults["envoy_duration#0"] = map[string]bool{
		"1_1_1_map[container_name:container10 method:PUT node:host00 path:/api/v1/call/_NUMBER_ service:v1 status:3xx]":           true,
		"5500_2_11000_map[container_name:container10 method:DELETE node:host00 path:/api/v1/call/_NUMBER_ service:v1 status:2xx]": true,
		"2778.5_4_11114_map[container_name:container10 method:GET node:host00 path:/api/v1/call/_NUMBER_ service:v1 status:2xx]":  true,
		"113_1_113_map[container_name:container10 method:PUT node:host00 path:/api/v1/call/_NUMBER_ service:v1 status:2xx]":       true,
	}

	expectedResults["envoy_duration#1"] = map[string]bool{
		"4166.125_8_33329_map[container_name:container10 method:GET node:host00 path:/api/v1/call/_NUMBER_ service:v1 status:2xx]": true,
	}

	expectedResults["envoy_duration#2"] = map[string]bool{
		"30000_1_30000_map[container_name:container10 method:GET node:host00 path:/api/v2/call/abc service:v2 status:5xx]":  true,
		"3000_1_3000_map[container_name:container10 method:GET node:host00 path:/api/v2/call/def service:v2 status:5xx]":    true,
		"4567_1_4567_map[container_name:container11 method:GET node:host01 path:/api/v2/call/def service:v2 status:5xx]":    true,
		"158_2_316_map[container_name:container10 method:GET node:host00 path:/api/v1/call/_NUMBER_ service:v1 status:2xx]": true,
	}

	expectedResults["graylog_messages_count_multi_label#0"] = map[string]bool{
		"3_3_3_map[container_name:container10 hostname:host00 partner-id:2 partner-id2:5 pod_name:pod10]":    true,
		"3_3_3_map[container_name:container10 hostname:host00 partner-id:1 partner-id2:5 pod_name:pod10]":    true,
		"3_3_3_map[container_name:container10 hostname:host00 partner-id:3 partner-id2:5 pod_name:pod10]":    true,
		"3_3_3_map[container_name:container10 hostname:host00 partner-id:1 partner-id2:6 pod_name:pod10]":    true,
		"5_5_5_map[container_name:container10 hostname:host00 partner-id:1 partner-id2:9 pod_name:pod10]":    true,
		"4_4_4_map[container_name:container10 hostname:host00 partner-id:2 partner-id2:5 pod_name:pod11]":    true,
		"4_4_4_map[container_name:container10 hostname:host00 partner-id:3 partner-id2:5 pod_name:pod11]":    true,
		"4_4_4_map[container_name:container10 hostname:host00 partner-id:2 partner-id2:6 pod_name:pod11]":    true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:3 partner-id2:7  8 pod_name:pod10]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:2 partner-id2:7  8 pod_name:pod10]": true,
		"3_3_3_map[container_name:container10 hostname:host00 partner-id:3 partner-id2:6 pod_name:pod10]":    true,
		"5_5_5_map[container_name:container10 hostname:host00 partner-id:3 partner-id2:9 pod_name:pod10]":    true,
		"4_4_4_map[container_name:container10 hostname:host00 partner-id:1 partner-id2:5 pod_name:pod11]":    true,
		"4_4_4_map[container_name:container10 hostname:host00 partner-id:1 partner-id2:6 pod_name:pod11]":    true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:1 partner-id2:7  8 pod_name:pod10]": true,
		"5_5_5_map[container_name:container10 hostname:host00 partner-id:2 partner-id2:9 pod_name:pod10]":    true,
		"4_4_4_map[container_name:container10 hostname:host00 partner-id:3 partner-id2:6 pod_name:pod11]":    true,
		"3_3_3_map[container_name:container10 hostname:host00 partner-id:2 partner-id2:6 pod_name:pod10]":    true,
	}

	expectedResults["graylog_messages_count_multi_label#1"] = map[string]bool{
		"2_2_2_map[container_name:container10 hostname:host00 partner-id:1 partner-id2:1 pod_name:pod11]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:6 partner-id2:4 pod_name:pod11]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:4 partner-id2:5 pod_name:pod11]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:4 partner-id2:6 pod_name:pod11]": true,
		"2_2_2_map[container_name:container10 hostname:host00 partner-id:1 partner-id2:1 pod_name:pod10]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:4 partner-id2:4 pod_name:pod11]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:5 partner-id2:5 pod_name:pod11]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:6 partner-id2:6 pod_name:pod11]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:5 partner-id2:4 pod_name:pod11]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:5 partner-id2:6 pod_name:pod11]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:7 partner-id2: pod_name:pod10]":  true,
		"2_2_2_map[container_name:container10 hostname:host00 partner-id:8 partner-id2: pod_name:pod10]":  true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:9 partner-id2: pod_name:pod10]":  true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:1 partner-id2:1 pod_name:pod12]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:6 partner-id2:5 pod_name:pod11]": true,
	}

	expectedResults["graylog_messages_count_multi_label#2"] = map[string]bool{
		"2_2_2_map[container_name:container10 hostname:host00 partner-id:a partner-id2:A pod_name:pod10]": true,
		"2_2_2_map[container_name:container10 hostname:host00 partner-id:b partner-id2:A pod_name:pod10]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id:c partner-id2:A pod_name:pod10]": true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id: partner-id2: pod_name:pod11]":   true,
		"1_1_1_map[container_name:container10 hostname:host00 partner-id: partner-id2: pod_name:pod22]":   true,
		"1_1_1_map[container_name:container11 hostname:host01 partner-id: partner-id2: pod_name:pod22]":   true,
	}

	expectedResults["graylog_messages_count_uniq_label#0"] = map[string]bool{
		"3_3_3_map[container_name:container10 hostname:host00 pod_name:pod10]": true,
		"4_4_4_map[container_name:container10 hostname:host00 pod_name:pod11]": true,
	}

	expectedResults["graylog_messages_count_uniq_label#1"] = map[string]bool{
		"1_1_1_map[container_name:container10 hostname:host00 pod_name:pod10]": true,
		"2_2_2_map[container_name:container10 hostname:host00 pod_name:pod11]": true,
		"1_1_1_map[container_name:container10 hostname:host00 pod_name:pod12]": true,
	}

	expectedResults["graylog_messages_count_uniq_label#2"] = map[string]bool{
		"2_2_2_map[container_name:container10 hostname:host00 pod_name:pod10]": true,
		"1_1_1_map[container_name:container10 hostname:host00 pod_name:pod11]": true,
		"1_1_1_map[container_name:container10 hostname:host00 pod_name:pod22]": true,
		"1_1_1_map[container_name:container11 hostname:host01 pod_name:pod22]": true,
	}

	expectedResults["graylog_messages_count_total#0"] = map[string]bool{
		"8_8_8_map[]": true,
	}

	expectedResults["graylog_messages_count_total#1"] = map[string]bool{
		"8_8_8_map[]": true,
	}

	expectedResults["graylog_messages_count_total#2"] = map[string]bool{
		"5_5_5_map[]": true,
	}

	expectedResults["graylog_messages_count_total_by_host_by_container#0"] = map[string]bool{
		"8_8_8_map[container_name:container10 hostname:host00]": true,
	}

	expectedResults["graylog_messages_count_total_by_host_by_container#1"] = map[string]bool{
		"8_8_8_map[container_name:container10 hostname:host00]": true,
	}

	expectedResults["graylog_messages_count_total_by_host_by_container#2"] = map[string]bool{
		"4_4_4_map[container_name:container10 hostname:host00]": true,
		"1_1_1_map[container_name:container11 hostname:host01]": true,
	}

	expectedResults["graylog_messages_count_uniq_metric#0"] = map[string]bool{
		"1_1_1_map[container_name:container10 hostname:host00 pod_name:pod11]": true,
		"3_3_3_map[container_name:container10 hostname:host00 pod_name:pod10]": true,
	}

	expectedResults["graylog_messages_count_uniq_metric#1"] = map[string]bool{
		"1_1_1_map[container_name:container10 hostname:host00 pod_name:pod10]": true,
		"2_2_2_map[container_name:container10 hostname:host00 pod_name:pod11]": true,
	}

	expectedResults["graylog_messages_count_uniq_metric#2"] = map[string]bool{
		"2_2_2_map[container_name:container10 hostname:host00 pod_name:pod10]": true,
	}

	expectedResults["graylog_messages_gauge_total_by_host_by_container#0"] = map[string]bool{
		"8_8_8_map[container_name:container10 hostname:host00]":           true,
		"NaN_0_0_map[container_name:container_name10 hostname:instance1]": true,
		"NaN_0_0_map[container_name:container_name11 hostname:instance1]": true,
		"NaN_0_0_map[container_name:container_name20 hostname:instance2]": true,
		"NaN_0_0_map[container_name:container_name21 hostname:instance2]": true,
	}

	expectedResults["graylog_messages_gauge_total_by_host_by_container#1"] = map[string]bool{
		"8_8_8_map[container_name:container10 hostname:host00]":           true,
		"NaN_0_0_map[container_name:container_name10 hostname:instance1]": true,
		"NaN_0_0_map[container_name:container_name11 hostname:instance1]": true,
		"NaN_0_0_map[container_name:container_name20 hostname:instance2]": true,
		"NaN_0_0_map[container_name:container_name21 hostname:instance2]": true,
	}

	expectedResults["graylog_messages_gauge_total_by_host_by_container#2"] = map[string]bool{
		"4_4_4_map[container_name:container10 hostname:host00]":           true,
		"1_1_1_map[container_name:container11 hostname:host01]":           true,
		"NaN_0_0_map[container_name:container_name10 hostname:instance1]": true,
		"NaN_0_0_map[container_name:container_name11 hostname:instance1]": true,
		"NaN_0_0_map[container_name:container_name20 hostname:instance2]": true,
		"NaN_0_0_map[container_name:container_name21 hostname:instance2]": true,
	}
}

func TestEvaluateMetric(t *testing.T) {
	initER()
	log.SetLevel(log.ErrorLevel)
	testCfg, err := config.Read("../../examples/unit_test.yaml")
	if err != nil {
		t.Fatalf("Error parsing test config : %+v", err)
	}

	queryName := "query_envoys"
	queryConfig := testCfg.Queries[queryName]
	currTime := time.Now()
	gds := make([]*queues.GraylogData, 0, len(testCfg.GraylogEmulator.Data))
	deRegistry := registry.NewDERegistry(testCfg)
	selfmonitor.InitSelfMonitoring(testCfg, nil, deRegistry)
	for _, csv := range testCfg.GraylogEmulator.Data {
		emuData, _, err := httpservice.ProcessCsv(csv, "test_query")
		if err != nil {
			t.Fatalf("Error processing csv for test config : %+v", err)
		}
		gd := &queues.GraylogData{
			Data:      emuData,
			StartTime: currTime,
			EndTime:   currTime,
		}
		enrichers.Enrich(queryName, gd, queryConfig)
		gds = append(gds, gd)
	}

	tests := make([]string, 0, len(expectedResults))
	for k := range expectedResults {
		tests = append(tests, k)
	}

	for _, testName := range tests {
		t.Run("TestMetricEvaluation "+testName, func(t *testing.T) {
			metricName, gdIndex, err := getMetricAndIndex(testName)
			if err != nil {
				t.Fatal(err.Error())
			}
			e := CreateEvaluator(testCfg)
			metricConfig := testCfg.Metrics[metricName]

			mer := e.EvaluateMetric(gds[gdIndex].Data, metricName, metricConfig, queryName, &currTime)
			er := expectedResults[testName]

			for _, ms := range mer.Series {
				str := fmt.Sprintf("%v_%v_%v_%+v", ms.Average, ms.Count, ms.Sum, ms.Labels)
				if !er[str] {
					t.Errorf("Unexpected test %v result : %v", testName, str)
				}
			}

			if len(mer.Series) != len(er) {
				t.Errorf("Unexpected number of metric series for test %v : %v (expected : %v)", testName, len(mer.Series), len(er))
			}
		})
	}
}

func getMetricAndIndex(testName string) (metricName string, gdIndex int64, err error) {
	testNameSplitted := strings.Split(testName, "#")
	if len(testNameSplitted) > 1 {
		metricName = testNameSplitted[0]
		gdIndex, err = strconv.ParseInt(testNameSplitted[1], 10, 64)
		if err != nil {
			gdIndex = 0
			fmt.Printf("Error parsing index for test %v : %+v", testName, err)
		}
	} else if len(testNameSplitted) == 1 {
		metricName = testNameSplitted[0]
		gdIndex = 0
	} else {
		err = fmt.Errorf("Name of the test %v can not be splitted", testName)
	}
	return
}
