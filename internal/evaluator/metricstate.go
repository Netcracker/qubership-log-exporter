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
    "sync"
    "github.com/prometheus/client_golang/prometheus"
)

type MetricState struct {
    sync.RWMutex
    m map[string]prometheus.Labels
}

func CreateMetricState() (*MetricState) {
    ms := MetricState{}
    ms.m = make(map[string]prometheus.Labels)
    return &ms
}

func (ms *MetricState) Initialize() {
    ms.m = make(map[string]prometheus.Labels)
}

func (ms *MetricState) Get(key string) prometheus.Labels {
    ms.RLock()
    defer ms.RUnlock()
    return ms.m[key]
}

func (ms *MetricState) Set(key string, val prometheus.Labels) {
    ms.Lock()
    defer ms.Unlock()
    ms.m[key] = val
}

func (ms *MetricState) GetAllKeys() []string {
    ms.RLock()
    defer ms.RUnlock()
    result := make([]string, 0, len(ms.m))
    for key := range ms.m {
        result = append(result, key)
    }
    return result 
}

func (ms *MetricState) Size() int64 {
    ms.RLock()
    defer ms.RUnlock()
    return int64(len(ms.m))
}