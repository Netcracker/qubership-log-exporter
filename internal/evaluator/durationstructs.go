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
	ec "log_exporter/internal/utils/errorcodes"
	"math"
	"strconv"
	"sync"

	log "github.com/sirupsen/logrus"
)

type NoResponseCacheRepozitory struct {
	caches map[string]*NoResponseCache // metric-name -> cache
}

func CreateNoResponseCacheRepo(appConfig *config.Config) *NoResponseCacheRepozitory {
	repo := NoResponseCacheRepozitory{}
	repo.caches = make(map[string]*NoResponseCache)
	for metricName, metricCfg := range appConfig.Metrics {
		if metricCfg.Operation != "duration-no-response" {
			continue
		}
		cacheSizeStr := metricCfg.Parameters["cache_size"]
		cacheSize, err := strconv.ParseInt(cacheSizeStr, 10, 64)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing value '%v' for parameter cache_size for the metric %v : %+v ; default value 30 will be used", cacheSizeStr, metricName, err)
			cacheSize = 30
		}
		// Ensure cacheSize is within valid int range and positive
		if cacheSize < 1 || cacheSize > int64(math.MaxInt32) {
			log.WithField(ec.FIELD, ec.LME_8104).Errorf("Parameter cache_size for the metric %v is out of bounds (%v); default value 30 will be used", metricName, cacheSize)
			cacheSize = 30
		}
		repo.caches[metricName] = CreateNoResponseCache(int(cacheSize))
	}
	return &repo
}

func (repo *NoResponseCacheRepozitory) GetCache(metric string) *NoResponseCache {
	return repo.caches[metric]
}

type NoResponseCache struct {
	sync.RWMutex
	cacheSize    int
	cacheBatches []*NRCacheBatch
}

type NRCacheBatch struct {
	cache map[string]*CachedRequest
}

func (nrcb *NRCacheBatch) PutCachedResult(correlationId string, time int64, olv *string, hasResponse bool) {
	cachedRequest := CachedRequest{}
	cachedRequest.Time = time
	cachedRequest.Olv = olv
	cachedRequest.HasResponse = hasResponse
	nrcb.cache[correlationId] = &cachedRequest
}

func CreateNRCacheBatch() *NRCacheBatch {
	nrCacheBatch := NRCacheBatch{}
	nrCacheBatch.cache = make(map[string]*CachedRequest)
	return &nrCacheBatch
}

type CachedRequest struct {
	Time        int64
	Olv         *string
	HasResponse bool
}

func CreateNoResponseCache(size int) *NoResponseCache {
	requestCache := NoResponseCache{}
	requestCache.cacheSize = size
	requestCache.cacheBatches = make([]*NRCacheBatch, size)
	return &requestCache
}

func (nrc *NoResponseCache) shiftBatches() {
	for i := len(nrc.cacheBatches) - 1; i > 0; i-- {
		nrc.cacheBatches[i] = nrc.cacheBatches[i-1]
		if nrc.cacheBatches[i] != nil {
			log.Debugf("Shifting batch %v to %v ; batch size is %v", i-1, i, len(nrc.cacheBatches[i].cache))
		}
	}
}

func (nrc *NoResponseCache) PutBatchToCache(nrCacheBatch *NRCacheBatch) {
	nrc.Lock()
	defer nrc.Unlock()
	nrc.shiftBatches()
	nrc.cacheBatches[0] = nrCacheBatch
}

func (nrc *NoResponseCache) MarkAsHasResponse(correlationId string) {
	for _, nrcb := range nrc.cacheBatches {
		if nrcb == nil {
			continue
		}
		cachedRequest := nrcb.cache[correlationId]
		if cachedRequest != nil {
			log.Debugf("MarkAsHasResponse : For correlationId %v response was found, hasResponse is set to true", correlationId)
			cachedRequest.HasResponse = true
			return
		}
	}
	log.Debugf("MarkAsHasResponse : For correlationId %v response was not found", correlationId)
}

func (nrc *NoResponseCache) CountNoResponseInTheLastBatchByOLV() map[string]*MetricSeries {
	result := make(map[string]*MetricSeries)
	if nrc.cacheBatches[nrc.cacheSize-1] == nil {
		return result
	}
	for correlationId, cachedRequest := range nrc.cacheBatches[nrc.cacheSize-1].cache {
		if !cachedRequest.HasResponse {
			ms := result[*cachedRequest.Olv]
			if ms == nil {
				ms := MetricSeries{}
				ms.Count = 1
				result[*cachedRequest.Olv] = &ms
			} else {
				ms.Count++
			}
			log.Debugf("Request %v with olv %v has no response", correlationId, cachedRequest.Olv)
		}
	}
	for _, ms := range result {
		ms.Sum = float64(ms.Count)
		ms.Average = float64(ms.Count)
	}
	return result
}

func (nrc *NoResponseCache) Size() int64 {
	result := int64(0)
	for _, batch := range nrc.cacheBatches {
		result += int64(len(batch.cache))
	}
	return result
}

type RequestTimeCacheRepozitory struct {
	caches map[string]map[string]*RequestTimeCache // query-name -> cache-name -> cache
}

func CreateRequestTimeCacheRepozitory(appConfig *config.Config) *RequestTimeCacheRepozitory {
	repo := RequestTimeCacheRepozitory{}
	repo.caches = make(map[string]map[string]*RequestTimeCache)

	for query, qCfg := range appConfig.Queries {
		for cache, cacheCfg := range qCfg.Caches {
			repo.addCache(query, cache, cacheCfg)
		}
	}

	return &repo
}

func (repo *RequestTimeCacheRepozitory) GetCache(query string, cacheName string) *RequestTimeCache {
	return repo.caches[query][cacheName]
}

func (repo *RequestTimeCacheRepozitory) addCache(query string, cacheName string, cacheCfg *config.CacheConfig) {
	if repo.caches[query] == nil {
		log.Infof("Caches for query %v is null, creating new instance in caches repository", query)
		queryCaches := make(map[string]*RequestTimeCache)
		repo.caches[query] = queryCaches
	}

	log.Infof("Add caches %v for query %v; cacheCfg = %+v", cacheName, query, cacheCfg)
	requestTimeCache := CreateRequestTimeCache(cacheCfg)
	repo.caches[query][cacheName] = requestTimeCache
}

type RequestTimeCache struct {
	sync.RWMutex
	cacheCfg *config.CacheConfig
	cache    []map[string]int64
}

func CreateRequestTimeCache(cacheCfg *config.CacheConfig) *RequestTimeCache {
	requestTimeCache := RequestTimeCache{}
	requestTimeCache.cacheCfg = cacheCfg
	requestTimeCache.cache = make([]map[string]int64, cacheCfg.Size)
	return &requestTimeCache
}

func (rtc *RequestTimeCache) shiftBatches() {
	for i := rtc.cacheCfg.Size - 1; i > 0; i-- {
		rtc.cache[i] = rtc.cache[i-1]
		log.Debugf("Shifting... %v batch size is %v", i, len(rtc.cache[i]))
	}
}

func (rtc *RequestTimeCache) PutBatchToCache(batch map[string]int64) {
	rtc.Lock()
	defer rtc.Unlock()
	rtc.shiftBatches()
	rtc.cache[0] = batch
}

func (rtc *RequestTimeCache) SearchRequestTimeInCache(correlationId string) int64 {
	rtc.RLock()
	defer rtc.RUnlock()
	for i, batch := range rtc.cache {
		value := batch[correlationId]
		if value != 0 {
			log.Debugf("For correlationId %v found value %v in batch %v", correlationId, value, i)
			return value
		}
	}
	return 0
}

func (rtc *RequestTimeCache) Size() int64 {
	result := int64(0)
	for _, batch := range rtc.cache {
		result += int64(len(batch))
	}
	return result
}

type IntCall struct {
	RequestTime        int64
	ResponseTime       int64
	OrderedLabelValues string
}

func CreateIntCall(requestTime int64, responseTime int64, orderedLabelValues string) *IntCall {
	intCall := IntCall{}
	intCall.RequestTime = requestTime
	intCall.ResponseTime = responseTime
	intCall.OrderedLabelValues = orderedLabelValues
	return &intCall
}
