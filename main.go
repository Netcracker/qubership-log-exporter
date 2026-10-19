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

package main

import (
	"flag"
	"fmt"
	"log_exporter/internal/config"
	"log_exporter/internal/httpservice"
	"log_exporter/internal/logger"
	"log_exporter/internal/processors"
	"log_exporter/internal/queues"
	"log_exporter/internal/registry"
	"log_exporter/internal/selfmonitor"
	"log_exporter/internal/utils"
	ec "log_exporter/internal/utils/errorcodes"
	"net/http"
	_ "net/http/pprof"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
)

var (
	printVersion         = flag.Bool("version", false, "Print the log-exporter version and exit")
	checkConfig          = flag.Bool("check-config", false, "Check the log-exporter config and exit")
	addr                 = flag.String("listen-address", "", "The address to listen (port) for HTTP requests")
	configPath           = flag.String("config-path", "config.yaml", "Path to the yaml configuration")
	disabledSelfMonitor  = flag.Bool("disable-self-monitor", false, "Disables self monitoring")
	appConfig            *config.Config
	croniter             *cron.Cron
	victoriaService      *httpservice.VictoriaService
	promRWService        *httpservice.PromRWService
	lastTimestampService *httpservice.LastTimestampService
	pullPort             = int64(-1)
	gtsQueue             *queues.GTSQueue
	gdQueue              *queues.GDQueue
	gmQueue              *queues.GMQueue
	deRegistry           *registry.DERegistry
)

func initExports() {
	for exportName, exportConfig := range appConfig.Exports {
		log.WithField("export_config", exportConfig.GetSafeCopy()).Info("Export config read")
		switch exportConfig.Strategy {
		case "push":
			switch exportConfig.Consumer {
			case "", "victoria-vmagent":
				log.WithField("export", exportName).Info("Initializing the victoria pusher")
				victoriaService = httpservice.NewVictoriaService(exportConfig)
			case "prometheus-remote-write":
				log.WithField("export", exportName).Info("Initializing the promRW pusher")
				promRWService = httpservice.NewPromWRService(exportConfig)
			}
			if exportConfig.LastTimestampHost != nil {
				log.WithField("export", exportName).Info("Initializing the lastTimestamp service")
				lastTimestampService = httpservice.NewLastTimestampService(exportConfig.LastTimestampHost)
			}
		case "pull":
			var err error
			pullPort, err = strconv.ParseInt(exportConfig.Port, 10, 64)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"port": exportConfig.Port, "export": exportName}).Error("Cannot parse the port for the puller. Pull mode won't work.")
			} else {
				log.WithFields(log.Fields{"export": exportName, "pull_port": pullPort}).Info("The puller exposes metrics in pull mode")
			}
		default:
			log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"strategy": exportConfig.Strategy, "export": exportName}).Error("Unknown strategy, the export config is ignored")
		}
	}
}

func main() {
	flag.Parse()

	if *printVersion {
		fmt.Printf("%v\n", versionString())
		return
	}
	if *checkConfig {
		checkConfigAndExit()
		return
	}
	processors.NewSignalProcessor(stopCroniter, versionString).Start()

	defer func() {
		stopCroniter()
		log.WithField("version_string", versionString()).Info("LOG-EXPORTER STOPPED, no signal received")
	}()
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField(ec.FIELD, ec.LME_1601).WithFields(log.Fields{"panic": rec, "stacktrace": string(debug.Stack())}).Error("Panic in main")
		}
	}()

	logger.ConfigureLog()

	log.WithField("version_string", versionString()).Info("LOG-EXPORTER STARTED")

	var err error
	appConfig, err = config.Read(*configPath)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_8101).WithField("error", err).Fatal("Cannot read the configuration")
		return
	}
	reapplyFlags()
	go http.HandleFunc("/probe", probeHandler)
	config.StartConsulChecker()

	croniter = utils.GetCron()

	initExports()

	gtsQueue = queues.NewGTSQueue(appConfig, lastTimestampService, croniter)
	gdQueue = queues.NewGDQueue(appConfig)
	if victoriaService != nil || promRWService != nil {
		gmQueue = queues.NewGMQueue(appConfig)
	}

	deRegistry = registry.NewDERegistry(appConfig)
	selfmonitor.InitSelfMonitoring(appConfig, appConfig.Datasources[appConfig.DsName].Labels, deRegistry)
	if strings.ToUpper(appConfig.Datasources[appConfig.DsName].Type) == "NEWRELIC" {
		processors.NewNewRelicCallsProcessor(appConfig, gtsQueue, gdQueue).Start()
	} else if strings.ToUpper(appConfig.Datasources[appConfig.DsName].Type) == "LOKI" {
		processors.NewLokiCallsProcessor(appConfig, gtsQueue, gdQueue).Start()
	} else {
		processors.NewGraylogCallsProcessor(appConfig, gtsQueue, gdQueue).Start()
	}
	processors.NewMetricsEvaluationProcessor(appConfig, gdQueue, gmQueue, deRegistry).Start()
	if victoriaService != nil {
		processors.NewVictoriaProcessor(appConfig, gmQueue, victoriaService).Start()
	}
	if promRWService != nil {
		processors.NewPromRemoteWriteProcessor(appConfig, gmQueue, promRWService).Start()
	}
	if !*disabledSelfMonitor {
		processors.NewSelfMonSchedulerProcessor(appConfig, gmQueue, croniter, deRegistry.GetRegistry(utils.SELF_METRICS_REGISTRY_NAME)).Start()
	} else {
		log.Info("Self monitoring is disabled")
	}

	croniter.Start()

	httpservice.CreateGraylogEmulator(appConfig).Start()

	if pullPort > 0 {
		http.HandleFunc("/metrics", httpHandlerFunc)
		log.WithField(ec.FIELD, ec.LME_1606).Error(http.ListenAndServe(fmt.Sprintf(":%v", pullPort), nil))
	} else if *addr != "" {
		http.HandleFunc("/metrics", httpHandlerFunc)
		log.WithField(ec.FIELD, ec.LME_1606).Error(http.ListenAndServe(*addr, nil))
	} else if victoriaService == nil && promRWService == nil {
		log.WithField(ec.FIELD, ec.LME_8101).Fatal("Neither pull nor push strategy is defined. Exiting.")
	} else {
		for {
			time.Sleep(time.Minute)
		}
	}
}

func checkConfigAndExit() {
	logger.ConfigureLog()
	log.Info("Log-exporter started with option -check-config.")
	appConfig, err := config.SimpleSilentRead(*configPath)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_8101).WithField("error", err).Error("Cannot read the yaml config")
		log.WithField(ec.FIELD, ec.LME_8100).Error("Yaml config is invalid")
		return
	}
	err = config.ValidateConfig(appConfig)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_8101).WithField("error", err).Error("The configuration is invalid")
		return
	}
	log.Info("Log-exporter is able to start with the provided configuration")
}

func httpHandlerFunc(w http.ResponseWriter, r *http.Request) {
	log.Debug("HttpHandler started")
	defer log.Debug("HttpHandler finished")

	promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{},
	).ServeHTTP(w, r)
}

func probeHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug("ProbeHandler call")
}

func stopCroniter() {
	if croniter != nil {
		croniter.Stop()
	} else {
		log.Warn("croniter is nil before exiting, nothing to stop")
	}
}

func reapplyFlags() {
	if len(appConfig.Flags) == 0 {
		log.Info("No flags need to be reloaded from YAML config")
	} else {
		for name, value := range appConfig.Flags {
			err := flag.Set(name, value)
			if err != nil {
				log.WithField(ec.FIELD, ec.LME_8102).WithFields(log.Fields{"name": name, "value": value}).Error("Cannot set the flag to the new value")
			} else {
				log.WithFields(log.Fields{"name": name, "value": value}).Info("Flag set to the new value")
			}
		}
	}
}
