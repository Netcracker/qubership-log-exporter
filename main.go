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
    "fmt"
    "log_exporter/internal/utils"
    ec "log_exporter/internal/utils/errorcodes"
    "log_exporter/internal/config"
    "log_exporter/internal/logger"
    "log_exporter/internal/selfmonitor"
    "log_exporter/internal/httpservice"
    "log_exporter/internal/queues"
    "log_exporter/internal/processors"
    "log_exporter/internal/registry"
    log "github.com/sirupsen/logrus"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "net/http"
    _ "net/http/pprof"
    "flag"
    "github.com/robfig/cron/v3"
    "os"
    "strconv"
    "time"
    "strings"
    "runtime/debug"
)

var (
    printVersion        = flag.Bool("version", false, "Print the log-exporter version and exit")
    checkConfig         = flag.Bool("check-config", false, "Check the log-exporter config and exit")
    addr                = flag.String("listen-address", "", "The address to listen (port) for HTTP requests")
    configPath          = flag.String("config-path", "config.yaml", "Path to the yaml configuration")
    disabledSelfMonitor = flag.Bool("disable-self-monitor", false, "Disables self monitoring")
    appConfig           *config.Config
    croniter            *cron.Cron
    osIntSignals        = make(chan os.Signal, 1)
    osQuitSignals       = make(chan os.Signal, 1)
    victoriaService     *httpservice.VictoriaService
    promRWService       *httpservice.PromRWService
    lastTimestampService *httpservice.LastTimestampService
    pullPort            = int64(-1)
    gtsQueue            *queues.GTSQueue
    gdQueue             *queues.GDQueue
    gmQueue             *queues.GMQueue
    deRegistry          *registry.DERegistry
)

func initExports() {
    for exportName, exportConfig := range appConfig.Exports {
        log.Infof("ExportConfig = %+v", exportConfig.GetSafeCopy())
        switch(exportConfig.Strategy) {
        case "push":
            if exportConfig.Consumer == "" || exportConfig.Consumer == "victoria-vmagent" {
                log.Infof("Initializing victoria pusher %v :", exportName)
                victoriaService = httpservice.NewVictoriaService(exportConfig)
            } else if exportConfig.Consumer == "prometheus-remote-write" {
                log.Infof("Initializing promRW pusher %v :", exportName)
                promRWService = httpservice.NewPromWRService(exportConfig)
            }
            if exportConfig.LastTimestampHost != nil {
                log.Infof("Initializing lastTimestamp service %v :", exportName)
                lastTimestampService = httpservice.NewLastTimestampService(exportConfig.LastTimestampHost)
            }
        case "pull":
            var err error
            pullPort, err = strconv.ParseInt(exportConfig.Port, 10, 64)
            if err != nil {
                log.WithField(ec.FIELD, ec.LME_8102).Errorf("Can not parse port %v for puller %v. Pull mode won't work.", exportConfig.Port, exportName)
            } else {
                log.Infof("Puller %v will expose metrics on port %v in pull mode", exportName, pullPort)
            }
        default:
            log.WithField(ec.FIELD, ec.LME_8102).Errorf("Unknown strategy %v. Export config %v is ignored", exportConfig.Strategy, exportName)
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
        log.Info("LOG-EXPORTER STOPPED, no signal received")
        fmt.Printf("\nstop log-exporter, %v\n", versionString())
    }()
    defer func() {
        if rec := recover(); rec != nil {
            log.WithField(ec.FIELD, ec.LME_1601).Errorf("Panic in main : %+v ; Stacktrace of the panic : %v", rec, string(debug.Stack()))
        }
    }()
    
    fmt.Printf("start log-exporter, %v\n", versionString())

    logger.ConfigureLog()

    log.Infof("LOG-EXPORTER STARTED; %v", versionString())

    var err error
    appConfig, err = config.Read(*configPath)
    if err != nil {
        log.WithField(ec.FIELD, ec.LME_8101).Fatalf("Fatal: %v", err)
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
    processors.NewSelfMonSchedulerProcessor(appConfig, gmQueue, croniter, deRegistry.GetRegistry(utils.SELF_METRICS_REGISTRY_NAME)).Start()

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
        log.WithField(ec.FIELD, ec.LME_8101).Errorf("Error reading yaml config : %+v", err)
        log.WithField(ec.FIELD, ec.LME_8100).Error("Yaml config is invalid")
        return
    }
    err = config.ValidateConfig(appConfig)
    if err != nil {
        log.WithField(ec.FIELD, ec.LME_8101).Errorf("%+v", err)
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
                log.WithField(ec.FIELD, ec.LME_8102).Errorf("Failed to set flag %v with new value %v", name, value)
            } else {
                log.Infof("Flag %v successfully set to new value %v", name, value)
            }
        }
    } 
}