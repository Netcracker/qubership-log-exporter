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

package registry

import (
    "sync"
    "log_exporter/internal/config"
    "log_exporter/internal/utils"
    ec "log_exporter/internal/utils/errorcodes"
    "github.com/prometheus/client_golang/prometheus"
    dto "github.com/prometheus/client_model/go"
    log "github.com/sirupsen/logrus"
)

type DERegistry struct {
    sync.RWMutex
    registries map[string]*prometheus.Registry
}

func NewDERegistry(appConfig *config.Config) *DERegistry {
    result := DERegistry{
        registries: make(map[string]*prometheus.Registry),
    }
    for queryName := range appConfig.Queries {
        result.registries[queryName] = prometheus.NewRegistry()
    }
    result.registries[utils.SELF_METRICS_REGISTRY_NAME] = prometheus.NewRegistry()
    return &result
}

func (der *DERegistry) MustRegister(queryName string, collector prometheus.Collector) {
    reg := der.registries[queryName]
    if reg == nil {
        log.WithField(ec.FIELD, ec.LME_1604).Errorf("Can not register metric for query %v : registry does not exist", queryName)
        return
    }
    reg.MustRegister(collector)
}

func (der *DERegistry) Gather() ([]*dto.MetricFamily, error) {
    der.RLock()
    defer der.RUnlock()
    result := make([]*dto.MetricFamily, 0)
    for _, registry := range der.registries {
        gathered, err := registry.Gather()
        if err != nil {
            return nil, err
        }
        result = append(result, gathered...)
    }
    log.Tracef("DERegistry : gather performed : %+v", result)
    return result, nil
}

func (der *DERegistry) GetRegistry(queryName string) (*prometheus.Registry) {
    return der.registries[queryName]
}