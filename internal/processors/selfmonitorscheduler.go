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

package processors

import (
	"log_exporter/internal/config"
	"log_exporter/internal/queues"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
)

type SelfMonSchedulerProcessor struct {
	appConfig       *config.Config
	gmQueue         *queues.GMQueue
	croniter        *cron.Cron
	selfMonregistry *prometheus.Registry
}

func NewSelfMonSchedulerProcessor(appConfig *config.Config, gmQueue *queues.GMQueue, croniter *cron.Cron, selfMonregistry *prometheus.Registry) *SelfMonSchedulerProcessor {
	return &SelfMonSchedulerProcessor{
		appConfig:       appConfig,
		gmQueue:         gmQueue,
		croniter:        croniter,
		selfMonregistry: selfMonregistry,
	}
}

func (smsp *SelfMonSchedulerProcessor) Start() {
	log.Info("SelfMonSchedulerProcessor.Start()")
	if smsp.gmQueue == nil {
		log.Info("SelfMonSchedulerProcessor.Start() : smsp.gmQueue == nil, SelfMonSchedulerProcessor will be disabled")
		return
	}
	_, err := smsp.croniter.AddFunc("* * * * *", func() {
		metricFamilies := utils.CopyMetricFamiliesFromRegistry(smsp.selfMonregistry, utils.SELF_METRICS_REGISTRY_NAME)
		if len(metricFamilies) > 0 {
			smsp.gmQueue.Put(utils.SELF_METRICS_REGISTRY_NAME, metricFamilies, false)
		}
	})
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_1608).Errorf("SelfMonSchedulerProcessor : Error registering in croniter : %v", err)
		return
	}
	log.Info("SelfMonSchedulerProcessor.Start() finished")
}
