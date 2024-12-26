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
    "strconv"
    "math"
    "log_exporter/internal/config"
    ec "log_exporter/internal/utils/errorcodes"
    log "github.com/sirupsen/logrus"
)

type MetricDefaultValues struct {
    DefaultValue float64
}

func CreateMetricDefaultValues(metricName string, defaultValue string) (*MetricDefaultValues) {
    mdv := MetricDefaultValues{}

    val, err := strconv.ParseFloat(defaultValue, 64)
    if err != nil {
        if defaultValue == "" {
            log.Debugf("Empty defaultValue received for metric %v; defaultValue is set to NaN", metricName)
        } else {
            log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing defaultValue %v for metric %v : %+v; defaultValue is set to NaN", defaultValue, metricName, err)
        }
        mdv.DefaultValue = math.NaN()
    } else {
        mdv.DefaultValue = val
    }

    return &mdv
}

type MetricDefaultValuesRepository struct {
    m map[string]*MetricDefaultValues
}

func CreateMetricDefaultValuesRepository(metricsMap map[string]*config.MetricsConfig) (*MetricDefaultValuesRepository) {
    repo := MetricDefaultValuesRepository{}
    repo.m = make(map[string]*MetricDefaultValues)

    for metricName, metricCfg := range metricsMap {
        if len(metricCfg.Parameters) == 0 {
            log.Debugf("DefaultValues : No parameters found for metric %v", metricName)
            continue
        }
        defaultValue := metricCfg.Parameters["default-value"]
        if defaultValue == "" {
            log.Debugf("DefaultValues : No default values found for metric %v", metricName)
            continue
        }
        mdv := CreateMetricDefaultValues(metricName, defaultValue)
        repo.m[metricName] = mdv
        log.Infof("Default values successfully created for metric %v : %+v", metricName, mdv)
    }

    return &repo
}

func (repo *MetricDefaultValuesRepository) GetMetricDefaultValue(metric string) float64 {
    if repo.m[metric] == nil {
        log.Tracef("For metric %v DefaultValue is NaN", metric)
        return math.NaN()
    }

    return repo.m[metric].DefaultValue
}