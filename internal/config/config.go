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
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log_exporter/internal/crypto"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ApiVersion      string `yaml:"apiVersion,omitempty"`
	Kind            string `yaml:",omitempty"`
	Exports         map[string]*ExportConfig
	Datasources     map[string]*DatasourceConfig
	Metrics         map[string]*MetricsConfig
	Queries         map[string]*QueryConfig
	General         *GeneralConfig
	Flags           map[string]string      `yaml:"flags,omitempty"`
	GraylogEmulator *GraylogEmulatorConfig `yaml:"graylog-emulator,omitempty"`
	DsName          string                 `yaml:"-"`
}

type TLSHostConfig struct {
	Host                  string
	User                  string
	Password              string
	DecryptedPassword     string        `yaml:"-"`
	ConnectionTimeout     time.Duration `yaml:"connection-timeout,omitempty"`       // 30s
	TLSInsecureSkipVerify bool          `yaml:"tls-insecure-skip-verify,omitempty"` // false
	TLSCertFile           *string       `yaml:"tls-cert-file,omitempty"`            // no default
	TLSKeyFile            *string       `yaml:"tls-key-file,omitempty"`             // no default
	TLSCACertFile         *string       `yaml:"tls-ca-cert-file,omitempty"`         // no default
	TlsConfig             *tls.Config   `yaml:"-"`
}

type DatasourceConfig struct {
	TLSHostConfig `yaml:",inline"`
	Labels        map[string]string `yaml:",omitempty"`
	Type          string
}

type ExportConfig struct {
	TLSHostConfig     `yaml:",inline"`
	Endpoint          string `yaml:",omitempty"`
	Strategy          string `yaml:",omitempty"`
	Consumer          string `yaml:",omitempty"`
	Port              string
	LastTimestampHost *LastTimestampHostConfig `yaml:"last-timestamp-host,omitempty"`
}

type LastTimestampHostConfig struct {
	TLSHostConfig `yaml:",inline"`
	Endpoint      string `yaml:",omitempty"`
	JsonPath      string `yaml:"json-path,omitempty"`
}

type MetricsConfig struct {
	Type                       string
	Description                string
	LabelsInitial              []string          `yaml:"labels,flow,omitempty"`
	Labels                     []string          `yaml:"-"`
	ConstLabels                map[string]string `yaml:"const-labels,omitempty"`
	MetricValue                string            `yaml:"metric-value,omitempty"`
	Operation                  string
	LabelFieldMap              map[string]string              `yaml:"label-field-map,omitempty"`
	MultiValueFields           []MultiValueFieldConfig        `yaml:"multi-value-fields,omitempty"`
	IdField                    string                         `yaml:"id-field,omitempty"`
	IdFieldStrategy            string                         `yaml:"id-field-strategy,omitempty"`
	IdFieldTTL                 int                            `yaml:"id-field-ttl,omitempty"`
	Buckets                    []float64                      `yaml:",flow,omitempty"`
	Parameters                 map[string]string              `yaml:",omitempty"`
	ChildMetrics               []string                       `yaml:"child-metrics,flow,omitempty"`
	HasDurationNoResponseChild bool                           `yaml:"-"`
	Threads                    int                            `yaml:",omitempty"`
	ExpectedLabels             []map[string][]string          `yaml:"expected-labels,flow"`
	Cond                       []map[string]map[string]string `yaml:"conditions,omitempty"` //slice -> condition type -> parameter name -> parameter value
}

type MultiValueFieldConfig struct {
	FieldName string `yaml:"field-name,omitempty"`
	LabelName string `yaml:"label-name,omitempty"`
	Separator string
}

type QueryConfig struct {
	Metrics                  []string `yaml:",flow"`
	Streams                  []string
	StreamsJson              string `yaml:"-"`
	QueryString              string `yaml:"query_string"`
	QueryStringJson          string `yaml:"-"`
	Timerange                string
	TimerangeDuration        time.Duration `yaml:"-"`
	FieldsInOrder            []string      `yaml:"fields_in_order"`
	FieldsInOrderJson        string        `yaml:"-"`
	Croniter                 string
	Interval                 string                  `yaml:",omitempty"`
	IntervalDuration         time.Duration           `yaml:"-"`
	QueryLag                 string                  `yaml:"query_lag"`
	QueryLagDuration         time.Duration           `yaml:"-"`
	Enrich                   []EnrichConfig          `yaml:",omitempty"`
	Caches                   map[string]*CacheConfig `yaml:",omitempty"`
	CronEntryID              int                     `yaml:"-"`
	IsInvalid                bool                    `yaml:"-"`
	GTSQueueSize             string                  `yaml:"gts-queue-size,omitempty"`
	GDQueueSize              string                  `yaml:"gd-queue-size,omitempty"`
	GMQueueSize              string                  `yaml:"gm-queue-size,omitempty"`
	MaxHistoryLookup         string                  `yaml:"max-history-lookup,omitempty"`
	GTSQueueSizeParsed       int                     `yaml:"-"`
	GDQueueSizeParsed        int                     `yaml:"-"`
	GMQueueSizeParsed        int                     `yaml:"-"`
	MaxHistoryLookupDuration time.Duration           `yaml:"-"`
	LastTimestampEndpoint    string                  `yaml:"last-timestamp-endpoint,omitempty"`
	LastTimestampJsonPath    string                  `yaml:"last-timestamp-json-path,omitempty"`
}

type CacheConfig struct {
	Type       string `yaml:",omitempty"`
	Size       int
	Key        string            `yaml:",omitempty"`
	Value      string            `yaml:",omitempty"`
	Parameters map[string]string `yaml:",omitempty"`
}

type MetricValueRegexpConfig struct {
	Regexp       string
	Template     string
	DefaultValue string `yaml:"default-value,omitempty"`
}

type URIProcessingConfig struct {
	UUIDReplacer     string `yaml:"uuid-replacer,omitempty"`
	IdDigitQuantity  int    `yaml:"id-digit-quantity,omitempty"`
	IDReplacer       string `yaml:"id-replacer,omitempty"`
	FSMReplacer      string `yaml:"fsm-replacer,omitempty"`
	FSMReplacerLimit int    `yaml:"fsm-replacer-limit,omitempty"`
	NumberReplacer   string `yaml:"number-replacer,omitempty"`
}

type GeneralConfig struct {
	GMQueueSelfMonSize          string            `yaml:"gm-queue-self-mon-size,omitempty"`
	GMQueueSelfMonSizeParsed    int               `yaml:"-"`
	DisablePushCloudLabels      bool              `yaml:"disable-push-cloud-labels,omitempty"`
	PushCloudLabels             map[string]string `yaml:"push-cloud-labels,omitempty"`
	NamespaceName               string            `yaml:"-"`
	PodName                     string            `yaml:"-"`
	ContainerName               string            `yaml:"-"`
	LTSRetryCount               string            `yaml:"last-timestamp-retry-count,omitempty"`
	LTSRetryPeriod              string            `yaml:"last-timestamp-retry-period,omitempty"`
	LTSRetryCountParsed         int               `yaml:"-"`
	LTSRetryPeriodParsed        time.Duration     `yaml:"-"`
	DatasourceRetry             *bool             `yaml:"datasource-retry,omitempty"`
	DatasourceRetryPeriod       string            `yaml:"datasource-retry-period,omitempty"`
	DatasourceRetryPeriodParsed time.Duration     `yaml:"-"`
	PushRetry                   *bool             `yaml:"push-retry,omitempty"`
	PushRetryPeriod             string            `yaml:"push-retry-period,omitempty"`
	PushRetryPeriodParsed       time.Duration     `yaml:"-"`
}

type GraylogEmulatorConfig struct {
	SourceFiles []string `yaml:"source-files,omitempty"`
	Endpoint    string   `yaml:",omitempty"`
	Data        []string `yaml:",omitempty"`
}

type EnrichConfig struct {
	SourceField    string            `yaml:"source-field,omitempty"`
	JsonPath       string            `yaml:"json-path,omitempty"`
	Regexp         string            `yaml:",omitempty"`
	RegexpCompiled *regexp.Regexp    `yaml:"-"`
	DestFields     []DestFieldConfig `yaml:"dest-fields,omitempty"`
	Threads        int               `yaml:",omitempty"`
}

type DestFieldConfig struct {
	LabelIndex       int                 `yaml:"-"`
	FieldName        string              `yaml:"field-name"`
	Template         string              `yaml:",omitempty"`
	TemplateCompiled []byte              `yaml:"-"`
	DefaultValue     string              `yaml:"default-value,omitempty"`
	URIProcessing    URIProcessingConfig `yaml:"uri-processing,omitempty"`
}

var (
	keyPath = flag.String("key-path", "", "Path to the key for Password encryption")
)

const (
	enc_prefix = "{ENC}"
	keySize    = 32
)

func Read(path string) (*Config, error) {
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

	length, err := io.Copy(&buf, configFile)
	if err != nil {
		return nil, fmt.Errorf("error copying config file %v : %+v", path, err)
	}
	log.Debugf("Copied %v bytes successfully to the buffer from file %v", length, path)

	err = yaml.Unmarshal(buf.Bytes(), &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling config file %v : %+v", path, err)
	}

	initDSName(&config)
	enrichFromEnvironmentVariables(&config)
	err = ValidateConfig(&config)
	if err != nil {
		return nil, fmt.Errorf("for config file %v got error : %+v", path, err)
	}

	processHiddenFields(&config)

	err = processCrypto(path, &config)
	if err != nil {
		return nil, fmt.Errorf("crypto error for config %v : %+v", path, err)
	}

	for dsName, dsConfig := range config.Datasources {
		err = dsConfig.processTlsSettings()
		if err != nil {
			return nil, fmt.Errorf("error processing TLS settings for datasource %v : %+v", dsName, err)
		} else {
			log.Infof("TLS settings for datasource %v processed successfully", dsName)
		}
	}

	for exportName, exportConfig := range config.Exports {
		err = exportConfig.processTlsSettings()
		if err != nil {
			return nil, fmt.Errorf("error processing TLS settings for export config %v : %+v", exportName, err)
		} else {
			log.Infof("TLS settings for export config %v processed successfully", exportName)
		}
		if exportConfig.LastTimestampHost != nil {
			if strings.ToUpper(exportConfig.LastTimestampHost.Host) == "NONE" {
				exportConfig.LastTimestampHost = nil
				continue
			}
			err = exportConfig.LastTimestampHost.processTlsSettings()
			if err != nil {
				return nil, fmt.Errorf("error processing TLS settings for last-timestamp-host of export config %v: %+v", exportName, err)
			} else {
				log.Infof("TLS settings for last-timestamp-host of export config %v processed successfully", exportName)
			}
		}
	}

	return &config, nil
}

func enrichFromEnvironmentVariables(config *Config) {
	if config.General == nil {
		log.Info("Config.General is nil, creating an empty one")
		config.General = &GeneralConfig{}
	}
	if !config.General.DisablePushCloudLabels {
		config.General.NamespaceName = os.Getenv("NAMESPACE")
		config.General.PodName = os.Getenv("HOSTNAME")
		config.General.ContainerName = os.Getenv("CONTAINER_NAME")
		log.Infof("Values for push cloud labels were set up : %v, %v, %v", config.General.NamespaceName, config.General.PodName, config.General.ContainerName)
	}

	for _, datasource := range config.Datasources {
		if (datasource.Type == "" || strings.ToUpper(datasource.Type) == "GRAYLOG") && datasource.User == "" {
			datasource.User = os.Getenv("GRAYLOG_USER")
			if datasource.User == "" {
				log.WithField(ec.FIELD, ec.LME_8103).Error("GRAYLOG_USER is not defined neither in config, nor in the environment variable")
			} else {
				log.Debug("GRAYLOG_USER is extracted from environment variable")
			}
			datasource.Password = os.Getenv("GRAYLOG_PASSWORD")
			if datasource.Password == "" {
				log.WithField(ec.FIELD, ec.LME_8103).Error("GRAYLOG_PASSWORD is not defined in the environment variable")
			} else {
				log.Debug("GRAYLOG_PASSWORD is extracted from environment variable")
			}
		} else if strings.ToUpper(datasource.Type) == "NEWRELIC" && datasource.User == "" {
			datasource.User = os.Getenv("NEWRELIC_ACCOUNT_ID")
			if datasource.User == "" {
				log.WithField(ec.FIELD, ec.LME_8103).Error("NEWRELIC_ACCOUNT_ID is not defined neither in config, nor in the environment variable")
			} else {
				log.Debug("NEWRELIC_ACCOUNT_ID is extracted from environment variable")
			}
			datasource.Password = os.Getenv("NEWRELIC_X_QUERY_KEY")
			if datasource.Password == "" {
				log.WithField(ec.FIELD, ec.LME_8103).Error("NEWRELIC_X_QUERY_KEY is not defined in the environment variable")
			} else {
				log.Debug("NEWRELIC_X_QUERY_KEY is extracted from environment variable")
			}
		} else if strings.ToUpper(datasource.Type) == "LOKI" && datasource.User == "" {
			datasource.User = os.Getenv("LOKI_USER")
			if datasource.User == "" {
				log.WithField(ec.FIELD, ec.LME_8103).Error("LOKI_USER is not defined neither in config, nor in the environment variable")
			} else {
				log.Debug("LOKI_USER is extracted from environment variable")
			}
			datasource.Password = os.Getenv("LOKI_PASSWORD")
			if datasource.Password == "" {
				log.WithField(ec.FIELD, ec.LME_8103).Error("LOKI_PASSWORD is not defined in the environment variable")
			} else {
				log.Debug("LOKI_PASSWORD is extracted from environment variable")
			}
		}
		break
	}

	for _, exportConfig := range config.Exports {
		if exportConfig.Strategy != "push" {
			continue
		}
		switch exportConfig.Consumer {
		case "victoria-vmagent", "":
			if exportConfig.User == "" {
				exportConfig.User = os.Getenv("VICTORIA_USER")
				if exportConfig.User == "" {
					log.Warn("VICTORIA_USER is not defined neither in config, nor in the environment variable")
				} else {
					log.Debug("VICTORIA_USER is extracted from environment variable")
				}
				exportConfig.Password = os.Getenv("VICTORIA_PASSWORD")
				if exportConfig.Password == "" {
					log.Warn("VICTORIA_PASSWORD is not defined in the environment variable")
				} else {
					log.Debug("VICTORIA_PASSWORD is extracted from environment variable")
				}
			}
		case "prometheus-remote-write":
			if exportConfig.User == "" {
				exportConfig.User = os.Getenv("PROMRW_USER")
				if exportConfig.User == "" {
					log.Info("PROMRW_USER is expectedly not defined neither in config, nor in the environment variable")
				} else {
					log.Debug("PROMRW_USER is extracted from environment variable")
				}
				exportConfig.Password = os.Getenv("PROMRW_PASSWORD")
				if exportConfig.Password == "" {
					log.Info("PROMRW_PASSWORD is expectedly not defined in the environment variables")
				} else {
					log.Debug("PROMRW_PASSWORD password is extracted from environment variable")
				}
			}
		}

		ltHost := exportConfig.LastTimestampHost
		if ltHost != nil {
			if ltHost.User == "" {
				ltHost.User = os.Getenv("LAST_TIMESTAMP_USER")
				if ltHost.User == "" {
					log.Info("LAST_TIMESTAMP_USER is not defined neither in config, nor in the environment variables")
				} else {
					log.Debug("LAST_TIMESTAMP_USER is extracted from environment variable")
				}
				ltHost.Password = os.Getenv("LAST_TIMESTAMP_PASSWORD")
				if ltHost.Password == "" {
					log.Info("LAST_TIMESTAMP_PASSWORD is not defined in the environment variables")
				} else {
					log.Debug("LAST_TIMESTAMP_PASSWORD is extracted from environment variable")
				}
			}
		}
		break
	}
}

func processHiddenFields(config *Config) {

	GTS_QUEUE_SIZE_DEFAULT := 60 * 24 * 10 // 10 days
	GD_QUEUE_SIZE_DEFAULT := 1
	GM_QUEUE_SIZE_DEFAULT := 10
	GM_QUEUE_SELF_MON_SIZE_DEFAULT := 60
	MAX_HISTORY_LOOKUP_DURATION := time.Hour * 24 * 8 // 8 days
	LAST_TIMESTAMP_RETRY_COUNT_DEFAULT := 5
	LAST_TIMESTAMP_RETRY_PERIOD_DEFAULT := time.Second * 10
	DATASOURCE_RETRY_PERIOD_DEFAULT := time.Second * 5
	PUSH_RETRY_PERIOD_DEFAULT := time.Second * 5

	if config.General.GMQueueSelfMonSize == "" {
		config.General.GMQueueSelfMonSizeParsed = GM_QUEUE_SELF_MON_SIZE_DEFAULT
		log.Infof("gm-queue-self-mon-size is empty, using the default value %v for the queue", GM_QUEUE_SELF_MON_SIZE_DEFAULT)
	} else {
		val, err := strconv.ParseInt(config.General.GMQueueSelfMonSize, 10, 64)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing gm-queue-self-mon-size default value %v will be used : %+v", GM_QUEUE_SELF_MON_SIZE_DEFAULT, err)
			config.General.GMQueueSelfMonSizeParsed = GM_QUEUE_SELF_MON_SIZE_DEFAULT
		} else {
			log.Infof("gm-queue-self-mon-size %v parsed successfully", val)
			if val < 0 || val > math.MaxInt32 {
				log.WithField(ec.FIELD, ec.LME_8104).Errorf("gm-queue-self-mon-size can not be out of range [0 ; %v], default value %v will be used instead", math.MaxInt32, GM_QUEUE_SELF_MON_SIZE_DEFAULT)
				config.General.GMQueueSelfMonSizeParsed = GM_QUEUE_SELF_MON_SIZE_DEFAULT
			} else {
				config.General.GMQueueSelfMonSizeParsed = int(val)
			}
		}
	}

	defaultTrue := true

	if config.General.LTSRetryCount == "" {
		config.General.LTSRetryCountParsed = LAST_TIMESTAMP_RETRY_COUNT_DEFAULT
		log.Infof("last-timestamp-retry-count is empty, using the default value %v", LAST_TIMESTAMP_RETRY_COUNT_DEFAULT)
	} else {
		val, err := strconv.ParseInt(config.General.LTSRetryCount, 10, 64)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing last-timestamp-retry-count default value %v will be used : %+v", LAST_TIMESTAMP_RETRY_COUNT_DEFAULT, err)
			config.General.LTSRetryCountParsed = LAST_TIMESTAMP_RETRY_COUNT_DEFAULT
		} else {
			log.Infof("last-timestamp-retry-count %v parsed successfully", val)
			if val < 0 || val > math.MaxInt32 {
				log.WithField(ec.FIELD, ec.LME_8104).Errorf("last-timestamp-retry-count can not be out of range [0 ; %v], default value %v will be used instead", math.MaxInt32, LAST_TIMESTAMP_RETRY_COUNT_DEFAULT)
				config.General.LTSRetryCountParsed = LAST_TIMESTAMP_RETRY_COUNT_DEFAULT
			} else {
				config.General.LTSRetryCountParsed = int(val)
			}
		}
	}

	if config.General.LTSRetryPeriod == "" {
		config.General.LTSRetryPeriodParsed = LAST_TIMESTAMP_RETRY_PERIOD_DEFAULT
		log.Infof("last-timestamp-retry-period is empty, using the default value %v", LAST_TIMESTAMP_RETRY_PERIOD_DEFAULT)
	} else {
		val, err := time.ParseDuration(config.General.LTSRetryPeriod)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing last-timestamp-retry-period default value %v will be used : %+v", LAST_TIMESTAMP_RETRY_PERIOD_DEFAULT, err)
			config.General.LTSRetryPeriodParsed = LAST_TIMESTAMP_RETRY_PERIOD_DEFAULT
		} else {
			log.Infof("last-timestamp-retry-period %+v parsed successfully", val)
			config.General.LTSRetryPeriodParsed = val
		}
	}

	if config.General.DatasourceRetry == nil {
		config.General.DatasourceRetry = &defaultTrue
	}

	if config.General.DatasourceRetryPeriod == "" {
		config.General.DatasourceRetryPeriodParsed = DATASOURCE_RETRY_PERIOD_DEFAULT
		log.Infof("datasource-retry-period is empty, using the default value %v", DATASOURCE_RETRY_PERIOD_DEFAULT)
	} else {
		val, err := time.ParseDuration(config.General.DatasourceRetryPeriod)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing datasource-retry-period default value %v will be used : %+v", DATASOURCE_RETRY_PERIOD_DEFAULT, err)
			config.General.DatasourceRetryPeriodParsed = DATASOURCE_RETRY_PERIOD_DEFAULT
		} else {
			log.Infof("datasource-retry-period %+v parsed successfully", val)
			config.General.DatasourceRetryPeriodParsed = val
		}
	}

	if config.General.PushRetry == nil {
		config.General.PushRetry = &defaultTrue
	}

	if config.General.PushRetryPeriod == "" {
		config.General.PushRetryPeriodParsed = PUSH_RETRY_PERIOD_DEFAULT
		log.Infof("push-retry-period is empty, using the default value %v", PUSH_RETRY_PERIOD_DEFAULT)
	} else {
		val, err := time.ParseDuration(config.General.PushRetryPeriod)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing push-retry-period default value %v will be used : %+v", PUSH_RETRY_PERIOD_DEFAULT, err)
			config.General.PushRetryPeriodParsed = PUSH_RETRY_PERIOD_DEFAULT
		} else {
			log.Infof("push-retry-period %+v parsed successfully", val)
			config.General.PushRetryPeriodParsed = val
		}
	}

	for queryName, queryConfig := range config.Queries {
		if queryConfig.GTSQueueSize == "" {
			queryConfig.GTSQueueSizeParsed = GTS_QUEUE_SIZE_DEFAULT
			log.Infof("For query %v gts-queue-size is empty, using the default value %v for the queue", queryName, GTS_QUEUE_SIZE_DEFAULT)
		} else {
			val, err := strconv.ParseInt(queryConfig.GTSQueueSize, 10, 64)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing gts-queue-size for query %v, default value %v will be used : %+v", queryName, GTS_QUEUE_SIZE_DEFAULT, err)
				queryConfig.GTSQueueSizeParsed = GTS_QUEUE_SIZE_DEFAULT
			} else {
				log.Infof("For query %v gts-queue-size %v parsed successfully", queryName, val)
				if val < 0 || val > math.MaxInt32 {
					log.WithField(ec.FIELD, ec.LME_8104).Errorf("For query %v gts-queue-size can not be out of range [0 ; %v], default value %v will be used instead", queryName, math.MaxInt32, GTS_QUEUE_SIZE_DEFAULT)
					queryConfig.GTSQueueSizeParsed = GTS_QUEUE_SIZE_DEFAULT
				} else {
					queryConfig.GTSQueueSizeParsed = int(val)
				}
			}
		}

		if queryConfig.GDQueueSize == "" {
			queryConfig.GDQueueSizeParsed = GD_QUEUE_SIZE_DEFAULT
			log.Infof("For query %v gd-queue-size is empty, using the default value %v for the queue", queryName, GD_QUEUE_SIZE_DEFAULT)
		} else {
			val, err := strconv.ParseInt(queryConfig.GDQueueSize, 10, 64)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing gd-queue-size for query %v, default value %v will be used : %+v", queryName, GD_QUEUE_SIZE_DEFAULT, err)
				queryConfig.GDQueueSizeParsed = GD_QUEUE_SIZE_DEFAULT
			} else {
				log.Infof("For query %v gd-queue-size %v parsed successfully", queryName, val)
				if val < 0 || val > math.MaxInt32 {
					log.WithField(ec.FIELD, ec.LME_8104).Errorf("For query %v gd-queue-size can not be out of range [0 ; %v], default value %v will be used instead", queryName, math.MaxInt32, GD_QUEUE_SIZE_DEFAULT)
					queryConfig.GDQueueSizeParsed = GD_QUEUE_SIZE_DEFAULT
				} else {
					queryConfig.GDQueueSizeParsed = int(val)
				}
			}
		}

		if queryConfig.GMQueueSize == "" {
			queryConfig.GMQueueSizeParsed = GM_QUEUE_SIZE_DEFAULT
			log.Infof("For query %v gm-queue-size is empty, using the default value %v for the queue", queryName, GM_QUEUE_SIZE_DEFAULT)
		} else {
			val, err := strconv.ParseInt(queryConfig.GMQueueSize, 10, 64)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing gm-queue-size for query %v, default value %v will be used : %+v", queryName, GM_QUEUE_SIZE_DEFAULT, err)
				queryConfig.GMQueueSizeParsed = GM_QUEUE_SIZE_DEFAULT
			} else {
				log.Infof("For query %v gm-queue-size %v parsed successfully", queryName, val)
				if val < 0 || val > math.MaxInt32 {
					log.WithField(ec.FIELD, ec.LME_8104).Errorf("For query %v gm-queue-size can not be out of range [0 ; %v], default value %v will be used instead", queryName, math.MaxInt32, GM_QUEUE_SIZE_DEFAULT)
					queryConfig.GMQueueSizeParsed = GM_QUEUE_SIZE_DEFAULT
				} else {
					queryConfig.GMQueueSizeParsed = int(val)
				}
			}
		}

		if queryConfig.MaxHistoryLookup == "" {
			queryConfig.MaxHistoryLookupDuration = MAX_HISTORY_LOOKUP_DURATION
			log.Infof("For query %v max-history-lookup is empty, using the default value %v", queryName, MAX_HISTORY_LOOKUP_DURATION)
		} else {
			interval, err := time.ParseDuration(queryConfig.MaxHistoryLookup)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_8104).Errorf("Error parsing max-history-lookup for query %v, default value %v will be used : %+v", queryName, MAX_HISTORY_LOOKUP_DURATION, err)
				queryConfig.MaxHistoryLookupDuration = MAX_HISTORY_LOOKUP_DURATION
			} else {
				log.Infof("For query %v max-history-lookup %v parsed successfully", queryName, queryConfig.MaxHistoryLookup)
				queryConfig.MaxHistoryLookupDuration = interval
			}
		}
	}

	for queryName, queryConfig := range config.Queries {
		timerange, err := time.ParseDuration(queryConfig.Timerange)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_8102).Errorf("Error parsing timerange duration %v for query %v : %+v", queryConfig.Timerange, queryName, err)
			timerange = -1
		}
		queryConfig.TimerangeDuration = timerange

		if len(queryConfig.Interval) == 0 {
			if lastTimestampServicesCount > 0 {
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("Interval duration is empty for query %v", queryName)
			} else {
				log.Infof("For query %v interval is empty", queryName)
			}
			queryConfig.IntervalDuration = -1
		} else {
			interval, err := time.ParseDuration(queryConfig.Interval)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("Error parsing interval duration %v for query %v : %+v", queryConfig.Interval, queryName, err)
				interval = -1
			}
			queryConfig.IntervalDuration = interval
		}

		queryLag, err := time.ParseDuration(queryConfig.QueryLag)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_8102).Errorf("Error parsing time lag duration %v for query %v : %+v", queryConfig.QueryLag, queryName, err)
			queryLag = -1
		}
		queryConfig.QueryLagDuration = queryLag
	}

	for queryName, queryConfig := range config.Queries {
		for enrichIndex, enrich := range queryConfig.Enrich {
			if enrich.Regexp == "" {
				queryConfig.Enrich[enrichIndex].RegexpCompiled = nil
				log.Infof("For query %v enrich %v regexp is not defined", queryName, enrichIndex)
			} else {
				pattern, err := regexp.Compile(enrich.Regexp)
				if err != nil {
					log.WithField(ec.FIELD, ec.LME_8102).Errorf("Error processing hidden fields for query %v enrich %v : Regexp %v compilation returned error: %+v . Metric configuration is invalid and query won't be executed", queryName, enrichIndex, enrich.Regexp, err)
					queryConfig.IsInvalid = true
				} else {
					queryConfig.Enrich[enrichIndex].RegexpCompiled = pattern
					log.Infof("For query %v enrich %v regexp %v was successfully compiled", queryName, enrichIndex, enrich.Regexp)
				}
			}
			for destFieldIndex, destField := range enrich.DestFields {
				enrich.DestFields[destFieldIndex].TemplateCompiled = []byte(destField.Template)
				log.Infof("For query %v enrich %v and destField %v template %v was successfully added", queryName, enrichIndex, destFieldIndex, destField.Template)
				log.Infof("For query %v enrich %v and destField %v defaultValue %v is configured", queryName, enrichIndex, destFieldIndex, destField.DefaultValue)
			}
		}
	}

	for queryName, queryConfig := range config.Queries {
		res, err := json.Marshal(queryConfig.FieldsInOrder)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_8102).Errorf("Error marshalling FieldsInOrder for query %v : %+v", queryName, err)
		} else {
			queryConfig.FieldsInOrderJson = string(res)
		}

		if len(queryConfig.Streams) > 0 {
			res, err = json.Marshal(queryConfig.Streams)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("Error marshalling Streams for query %v : %+v", queryName, err)
			} else {
				queryConfig.StreamsJson = "\"streams\": " + string(res) + ","
			}
		}

		res, err = json.Marshal(queryConfig.QueryString)
		if err != nil {
			log.WithField(ec.FIELD, ec.LME_8102).Errorf("Error marshalling QueryString for query %v : %+v", queryName, err)
		} else {
			queryConfig.QueryStringJson = string(res)
		}
	}

	for metricName, metric := range config.Metrics {
		metric.Labels = append(metric.Labels, metric.LabelsInitial...)
		for label := range metric.LabelFieldMap {
			if utils.FindStringIndexInArray(metric.Labels, label) < 0 {
				metric.Labels = append(metric.Labels, label)
			}
		}
		for i, mvfc := range metric.MultiValueFields {
			if utils.FindStringIndexInArray(metric.Labels, mvfc.LabelName) < 0 {
				metric.Labels = append(metric.Labels, mvfc.LabelName)
			}
			if mvfc.Separator == "" {
				metric.MultiValueFields[i].Separator = ","
				log.Infof("For metric %v separator is empty for multi-value field configuration %v, default value ',' will be used as a separator", metricName, i)
			}
		}
		log.Infof("For metric %v found labels : %+v", metricName, metric.Labels)
	}

	for metricName, metric := range config.Metrics {
		if len(metric.ChildMetrics) == 0 {
			continue
		}
		if metric.Operation != "duration" {
			log.WithField(ec.FIELD, ec.LME_8102).Errorf("Metric %v has operation %v, child metrics are not supported for this type and will be ignored", metricName, metric.Operation)
			continue
		}
		for _, childMetricName := range metric.ChildMetrics {
			log.Infof("Metric %v has child %v", metricName, childMetricName)
			childMetricCfg := config.Metrics[childMetricName]
			if childMetricCfg == nil {
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("Metric %v has non-existent child metric %v", metricName, childMetricName)
				continue
			}
			switch childMetricCfg.Operation {
			case "duration-no-response":
				log.Infof("Metric %v has duration-no-response child metric %v", metricName, childMetricName)
				metric.HasDurationNoResponseChild = true
			default:
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("Child metric %v for metric %v has operation %v, child metrics of this type are not supported and will be ignored", childMetricName, metricName, metric.Operation)
			}
		}
	}

	for mName, mCfg := range config.Metrics {
		if !checkExpectedLabelsFields(mName, mCfg) {
			log.Warnf("Expected labels for metric %v were reset to nil", mName)
			mCfg.ExpectedLabels = nil
		}
	}
}

func initDSName(config *Config) {
	for k := range config.Datasources {
		config.DsName = k
		break
	}
}

func processCrypto(path string, config *Config) error {
	if *keyPath == "" {
		log.Info("Key-path is not specified, so read password as plain-text")
		for _, datasourceConfig := range config.Datasources {
			datasourceConfig.DecryptedPassword = datasourceConfig.Password
		}
		return nil
	}

	key, isNew, err := getOrCreateKey()
	if err != nil {
		return fmt.Errorf("key error : %+v", err)
	}

	cryptoService, err := crypto.NewCrypto(key)
	if err != nil {
		return fmt.Errorf("crypto error : %+v", err)
	}

	configModified := false
	for dsName, dsConfig := range config.Datasources {
		if !strings.HasPrefix(dsConfig.Password, enc_prefix) {
			dsConfig.DecryptedPassword = dsConfig.Password
			encryptedPassword, err := cryptoService.Encrypt([]byte(dsConfig.DecryptedPassword))
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_1603).Errorf("Failed to encrypt password for %v : %+v", dsName, err)
			} else {
				dsConfig.Password = enc_prefix + encryptedPassword
				log.Infof("Password encrypted successfully for %v", dsName)
				configModified = true
			}
		} else {
			if isNew {
				return fmt.Errorf("there is no sense trying to decipher with fresh key %v", *keyPath)
			}
			dsConfig.DecryptedPassword, err = cryptoService.Decrypt([]byte(dsConfig.Password[len(enc_prefix):]))
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_1603).Errorf("Failed to decrypt password for %v", dsName)
			} else {
				log.Infof("Password decrypted successfully for %v", dsName)
			}
		}
	}

	if configModified {
		log.Infof("Modifying %v ...", path)
		err = writeConfigToFile(path, *config)
		if err != nil {
			return fmt.Errorf("error modifying %v : %+v", path, err)
		} else {
			log.Infof("Config %v modified successfully", path)
		}
	}

	return nil
}

func (tlsh *TLSHostConfig) processTlsSettings() error {
	tlsCfg, err := tlsh.getTLSConfig()
	if err != nil {
		return err
	}
	tlsh.TlsConfig = tlsCfg
	if tlsh.ConnectionTimeout == 0 {
		tlsh.ConnectionTimeout = time.Second * 30
	}
	return nil
}

func (tlsh *TLSHostConfig) getTLSConfig() (*tls.Config, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: tlsh.TLSInsecureSkipVerify,
	}
	if tlsh.TLSCACertFile != nil {
		caCert, err := os.ReadFile(*tlsh.TLSCACertFile)
		if err != nil {
			return nil, fmt.Errorf("could not read TLS CA from %v, %w", *tlsh.TLSCACertFile, err)
		}

		tlsCfg.ClientCAs = x509.NewCertPool()
		tlsCfg.ClientCAs.AppendCertsFromPEM(caCert)
	}
	if tlsh.TLSCertFile != nil && tlsh.TLSKeyFile != nil {
		cert, err := tls.LoadX509KeyPair(*tlsh.TLSCertFile, *tlsh.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed reading TLS credentials, %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

func writeConfigToFile(path string, cfg Config) error {
	err := os.Truncate(path, 0)
	if err != nil {
		return fmt.Errorf("error truncating file %v : %+v", path, err)
	}
	perms := os.FileMode(utils.GetOctalUintEnvironmentVariable("CONFIG_FILE_PERMS", 0600))
	err = os.Chmod(path, perms)
	if err != nil {
		return fmt.Errorf("error chmod file %v : %+v", path, err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, perms)
	defer func() {
		log.Infof("File %v closed", path)
		_ = f.Close()
	}()
	if err != nil {
		return fmt.Errorf("error opening file %v : %+v", path, err)
	}
	_, err = f.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("error executing Seek for file %v : %+v", path, err)
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("error marshalling yaml : %+v", err)
	}
	_, err = f.Write(out)
	if err != nil {
		return fmt.Errorf("error writing to file %v : %+v", path, err)
	}
	return nil
}

func getOrCreateKey() (key []byte, new bool, err error) {
	file, err := os.Open(*keyPath)
	defer func() {
		if file != nil {
			if err := file.Close(); err != nil {
				log.Errorf("error closing file %v : %+v", *keyPath, err)
			}
		}
	}()

	if err != nil && !os.IsNotExist(err) {
		return nil, false, err
	}
	buf := bytes.Buffer{}
	length, err := io.Copy(&buf, file)
	if err != nil {
		log.Infof("Error copying key-file %v, probably it doesn't exist : %+v", *keyPath, err)
	} else {
		log.Debugf("Copied %v bytes successfully to the buffer from key-file %v", length, *keyPath)
	}

	encodedKey := buf.String()
	if len(encodedKey) != 0 {
		log.Infof("Key file is not empty. Using key from the file %v", *keyPath)
		key, err := base64.StdEncoding.DecodeString(encodedKey)
		if err != nil {
			return nil, false, err
		}
		return key, false, nil
	}

	log.Infof("Key file is empty. Generating new key and writing it to %v...", *keyPath)
	newKey, err := randStringBytes(keySize)
	if err != nil {
		return nil, false, fmt.Errorf("error generating crypto key %+v", err)
	}
	key = []byte(base64.StdEncoding.EncodeToString(newKey))
	file, err = os.Create(*keyPath)
	if err != nil {
		return nil, false, fmt.Errorf("error creating file %v : %+v", *keyPath, err)
	}
	err = os.Chmod(*keyPath, os.FileMode(utils.GetOctalUintEnvironmentVariable("KEY_FILE_PERMS", 0600)))
	if err != nil {
		return nil, false, fmt.Errorf("error chmod file %v : %+v", *keyPath, err)
	}
	_, err = file.Write(key)
	if err != nil {
		return nil, false, fmt.Errorf("error writing file %v : %+v", *keyPath, err)
	}
	log.Infof("Key file generated successfully and written to %v", *keyPath)
	return newKey, true, nil
}

func randStringBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func checkExpectedLabelsFields(mName string, mCfg *MetricsConfig) bool {
	labelsCount := len(mCfg.Labels)
	for itemNum, expectedLabelsItem := range mCfg.ExpectedLabels {
		if len(expectedLabelsItem) != labelsCount {
			log.WithField(ec.FIELD, ec.LME_8102).Errorf("Invalid expected labels configuration for metric %v, itemNum %v : Metric has %v labels defined while in expected labels item %v labels defined", mName, itemNum, labelsCount, len(expectedLabelsItem))
			return false
		}
		for _, labelName := range mCfg.Labels {
			if len(expectedLabelsItem[labelName]) == 0 {
				log.WithField(ec.FIELD, ec.LME_8102).Errorf("Invalid expected labels configuration for metric %v, itemNum %v : Metric has label %v defined while in expected labels this label is not defined", mName, itemNum, labelName)
				return false
			}
		}
	}
	return true
}

const sensitiveDataReplacer string = "REMOVED_FROM_LOGS"

func (c *ExportConfig) GetSafeCopy() *ExportConfig {
	copy := utils.Copy(c)
	safeCopy, ok := copy.(*ExportConfig)
	if !ok {
		log.WithField(ec.FIELD, ec.LME_1602).Error("getSafeCopy : Cannot perform type assertion to *ExportConfig. ExportConfig will be presented in logs as empty")
		return &ExportConfig{}
	}
	if safeCopy.Password != "" {
		safeCopy.Password = sensitiveDataReplacer
	}
	lastTimestampHost := safeCopy.LastTimestampHost
	if lastTimestampHost != nil && lastTimestampHost.Password != "" {
		lastTimestampHost.Password = sensitiveDataReplacer
	}
	return safeCopy
}
