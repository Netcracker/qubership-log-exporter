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
	"log_exporter/internal/config"
	"sync"

	log "github.com/sirupsen/logrus"
)

type IdFieldCacheRepozitory struct {
	sync.RWMutex
	Metrics map[string]*MetricIdFieldCache // metric-name -> MetricIdFieldCache
}

type MetricIdFieldCache struct {
	sync.RWMutex
	metricName  string
	maxTTL      int
	age         int
	cache       map[string]bool            // id-field-value -> true
	cacheOld    map[string]bool            // id-field-value -> true
	olvCache    map[string]map[string]bool // ordered-label-value -> id-field-value -> true
	olvCacheOld map[string]map[string]bool // ordered-label-value -> id-field-value -> true
}

func CreateIdFieldCacheRepo(appConfig *config.Config) *IdFieldCacheRepozitory {
	repo := IdFieldCacheRepozitory{}
	repo.Metrics = make(map[string]*MetricIdFieldCache)
	for metric, metricCfg := range appConfig.Metrics {
		if metricCfg.IdField != "" {
			repo.Metrics[metric] = CreateMetricIdFieldCache(metricCfg.IdFieldTTL, metric)
		}
	}
	return &repo
}

func CreateMetricIdFieldCache(idFieldTTL int, metricName string) *MetricIdFieldCache {
	metricCache := MetricIdFieldCache{}
	metricCache.cache = make(map[string]bool)
	metricCache.cacheOld = make(map[string]bool)
	metricCache.olvCache = make(map[string]map[string]bool)
	metricCache.olvCacheOld = make(map[string]map[string]bool)
	if idFieldTTL <= 0 {
		idFieldTTL = 60
	}
	metricCache.maxTTL = idFieldTTL
	metricCache.metricName = metricName
	return &metricCache
}

func (r *IdFieldCacheRepozitory) GetMetricIdFieldCache(metric string) *MetricIdFieldCache {
	r.Lock()
	defer r.Unlock()
	result := r.Metrics[metric]
	if result == nil {
		result = CreateMetricIdFieldCache(60, "UNKNOWN")
		r.Metrics[metric] = result
	}
	return result
}

func (c *MetricIdFieldCache) IsUsed(id string) bool {
	c.Lock()
	defer c.Unlock()
	if !c.cache[id] {
		c.cache[id] = true
		return c.cacheOld[id]
	}
	return true
}

func (c *MetricIdFieldCache) IncAge() {
	c.Lock()
	defer c.Unlock()
	c.age++

	if c.age >= c.maxTTL {
		log.Infof("For metric %v cache shift is started : len(c.cacheOld) = %v, len(c.cache) = %v, len(c.olvCacheOld) = %v, len(c.olvCache) = %v", c.metricName, len(c.cacheOld), len(c.cache), len(c.olvCacheOld), len(c.olvCache))
		log.Tracef("For metric %v cache before shift : c.cacheOld = %+v, c.cache = %+v, c.olvCacheOld = %+v, c.olvCache = %+v", c.metricName, c.cacheOld, c.cache, c.olvCacheOld, c.olvCache)
		c.age = 0
		c.cacheOld = c.cache
		c.olvCacheOld = c.olvCache
		c.cache = make(map[string]bool)
		c.olvCache = make(map[string]map[string]bool)
		log.Infof("For metric %v cache shift is finished : len(c.cacheOld) = %v, len(c.cache) = %v, len(c.olvCacheOld) = %v, len(c.olvCache) = %v", c.metricName, len(c.cacheOld), len(c.cache), len(c.olvCacheOld), len(c.olvCache))
		log.Tracef("For metric %v cache after shift : c.cacheOld = %+v, c.cache = %+v, c.olvCacheOld = %+v, c.olvCache = %+v", c.metricName, c.cacheOld, c.cache, c.olvCacheOld, c.olvCache)
	}
}

func (c *MetricIdFieldCache) IsUsedForOLV(id string, olv string) bool {
	c.Lock()
	defer c.Unlock()
	olvCache := c.olvCache[olv]
	if olvCache == nil {
		olvCache = make(map[string]bool)
		c.olvCache[olv] = olvCache
	}
	olvCacheOld := c.olvCacheOld[olv]
	if olvCacheOld == nil {
		olvCacheOld = make(map[string]bool)
		c.olvCacheOld[olv] = olvCacheOld
	}
	if !olvCache[id] {
		olvCache[id] = true
		return olvCacheOld[id]
	}
	return true
}
