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
	ec "log_exporter/internal/utils/errorcodes"
	"os"
	"strings"
	"time"

	capi "github.com/hashicorp/consul/api"
	log "github.com/sirupsen/logrus"
)

func StartConsulChecker() {

	enabled := os.Getenv("LME_CONSUL_ENABLED")

	if strings.ToUpper(enabled) != "TRUE" {
		log.WithField("enabled", enabled).Info("ConsulChecker : LME_CONSUL_ENABLED is not set to TRUE, log-level will not be managed by Consul")
		return
	}

	url := os.Getenv("CONSUL_URL")
	token := os.Getenv("CONSUL_ADMIN_TOKEN")

	if url == "" {
		log.WithField(ec.FIELD, ec.LME_8105).Error("ConsulChecker : Consul client can not be set up without CONSUL_URL environment variable. Log-level will not be managed by Consul")
		return
	}

	if token == "" {
		log.Warn("ConsulChecker : Token for Consul is not specified in CONSUL_ADMIN_TOKEN environment variable")
	}

	client, err := capi.NewClient(&capi.Config{
		Address: url,
		//Scheme: "https",
		Namespace: "",
		Token:     token,
	})
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_8105).WithField("error", err).Error("ConsulChecker : Error creating consul client")
		return
	}

	var consulLogLevelPath string
	consulLogLevelPath = os.Getenv("LME_CONSUL_LOG_LEVEL_PATH")
	if consulLogLevelPath == "" {
		namespace := os.Getenv("NAMESPACE")
		if namespace == "" {
			log.WithField(ec.FIELD, ec.LME_8105).Error("ConsulChecker : Consul log-level property path is unknown. Log-level will not be managed by Consul")
			return
		}
		consulLogLevelPath = "config/" + namespace + "/lme/log.level"
	}

	consulPeriodString := os.Getenv("LME_CONSUL_CHECK_PERIOD")
	consulPeriodDuration, err := time.ParseDuration(consulPeriodString)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_8105).WithFields(log.Fields{"consul_period_string": consulPeriodString, "error": err}).Error("ConsulChecker : Error parsing the duration of the LME_CONSUL_CHECK_PERIOD environment variable. Log-level will not be managed by Consul")
		return
	}

	log.WithFields(log.Fields{"url": url, "token": getSafeToken(token), "consul_log_level_path": consulLogLevelPath, "consul_period_string": consulPeriodString}).Info("ConsulChecker : Consul client configuration")

	currentLevel := log.GetLevel().String()
	kv := client.KV()
	go func() {
		for {
			pair, _, err := kv.Get(consulLogLevelPath, nil)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_7151).WithFields(log.Fields{"consul_log_level_path": consulLogLevelPath, "error": err}).Error("ConsulChecker : Error getting the value for the log-level key from Consul")
			} else if pair == nil {
				log.WithField(ec.FIELD, ec.LME_7151).Error("ConsulChecker : Nil pair received from consul for log-level")
			} else {
				log.WithFields(log.Fields{"key": pair.Key, "value": string(pair.Value)}).Debug("ConsulChecker : Key and value received from Consul")
				newLevel := string(pair.Value)

				if newLevel != currentLevel {
					level, err := log.ParseLevel(newLevel)
					if err != nil {
						log.WithField(ec.FIELD, ec.LME_7154).WithFields(log.Fields{"new_level": newLevel, "error": err}).Error("ConsulChecker : Got an incorrect log-level from Consul")
					} else {
						log.WithField("new_level", newLevel).Warn("ConsulChecker : Setting the new log-level from Consul")
						log.SetLevel(level)
						currentLevel = newLevel
					}
				}
			}
			time.Sleep(consulPeriodDuration)
		}
	}()
}

func getSafeToken(token string) string {
	size := len(token)
	if size <= 8 {
		return strings.Repeat("*", size)
	}
	return token[:2] + strings.Repeat("*", size-4) + token[size-2:]
}
