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
	"fmt"
	"os"
	"syscall"
    "os/signal"
	"runtime"
    ec "log_exporter/internal/utils/errorcodes"
	log "github.com/sirupsen/logrus"
)

type SignalProcessor struct {
    osIntSignals  chan os.Signal
    osQuitSignals chan os.Signal
	stopCroniter func()
	versionString func() string
}

func NewSignalProcessor(stopCroniter func(), versionString func() string) *SignalProcessor {
    result := SignalProcessor{
		stopCroniter: stopCroniter,
		versionString: versionString,
	}
	result.osIntSignals        = make(chan os.Signal, 1)
    result.osQuitSignals       = make(chan os.Signal, 1)	
	return &result
}

func (sp *SignalProcessor) Start() {
    signal.Notify(sp.osIntSignals, syscall.SIGINT, syscall.SIGTERM)
    signal.Notify(sp.osQuitSignals, syscall.SIGQUIT)
    go sp.interruptionHandler()
    go sp.quitHandler()
}

func (sp *SignalProcessor) interruptionHandler() {
    signal := <- sp.osIntSignals

    sp.logRuntimeInfo()
	sp.stopCroniter()

    log.Infof("STOPPING LOG-EXPORTER (received %+v signal)", signal)
    fmt.Printf("\nstop exporter, %v\n", sp.versionString())
    os.Exit(0)
}

func (sp *SignalProcessor) quitHandler() {
    for {
        signal := <- sp.osQuitSignals
        log.Infof("Received %+v signal, printing thread dumps...", signal)
        sp.logRuntimeInfo()
    }
}

func (sp *SignalProcessor) logRuntimeInfo() {
    buf := make([]byte, 1<<20)
    stacklen := runtime.Stack(buf, true)
    log.WithField(ec.FIELD, ec.LME_1607).Errorf("GOROUTINES DUMP: %s", buf[:stacklen])
}