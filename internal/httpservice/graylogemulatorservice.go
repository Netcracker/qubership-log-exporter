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

package httpservice

import (
	"io"
	"log_exporter/internal/config"
	ec "log_exporter/internal/utils/errorcodes"
	"net/http"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
)

type GraylogEmulator struct {
	sync.RWMutex
	appConfig    *config.Config
	currentIndex int
	sourcesCount int
}

func CreateGraylogEmulator(appConfig *config.Config) *GraylogEmulator {
	g := GraylogEmulator{}
	g.appConfig = appConfig
	return &g
}

func (g *GraylogEmulator) Start() {
	graylogEmuCfg := g.appConfig.GraylogEmulator
	if graylogEmuCfg == nil {
		log.Info("GraylogEmulator : Graylog emulator is not configured")
		return
	}
	log.Info("GraylogEmulator : Graylog emulator is configured")
	g.sourcesCount = len(graylogEmuCfg.Data)
	if g.sourcesCount == 0 {
		g.sourcesCount = len(graylogEmuCfg.SourceFiles)
	}
	if g.sourcesCount == 0 {
		log.WithField(ec.FIELD, ec.LME_8106).Error("GraylogEmulator : Error starting emulator, no data is specified in the config file and no source files are defined")
		return
	}
	g.currentIndex = g.sourcesCount - 1
	graylogEmuEndpoint := "/api/views/search/messages"
	if graylogEmuCfg.Endpoint != "" {
		graylogEmuEndpoint = graylogEmuCfg.Endpoint
	}
	if len(graylogEmuCfg.Data) > 0 {
		http.HandleFunc(graylogEmuEndpoint, g.graylogEmuHandlerFromConfig)
	} else if len(graylogEmuCfg.SourceFiles) > 0 {
		http.HandleFunc(graylogEmuEndpoint, g.graylogEmuHandlerFromFiles)
	} else {
		log.WithField(ec.FIELD, ec.LME_8106).Error("GraylogEmulator : No data for graylog emulator is defined.")
	}
}

func (g *GraylogEmulator) graylogEmuHandlerFromFiles(w http.ResponseWriter, r *http.Request) {
	graylogEmuCfg := g.appConfig.GraylogEmulator
	filename := graylogEmuCfg.SourceFiles[g.getNextIndex()]
	file, err := os.Open(filename)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_1605).Errorf("GraylogEmulator : Error opening file : %+v", err)
		return
	}
	defer func() {
		if err = file.Close(); err != nil {
			log.WithField(ec.FIELD, ec.LME_1605).Errorf("GraylogEmulator : Error closing file : %+v", err)
		}
	}()

	b, err := io.ReadAll(file)
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_1605).Errorf("GraylogEmulator : Error reading file : %+v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(b); err != nil {
		log.WithField(ec.FIELD, ec.LME_1605).Errorf("GraylogEmulator : Error writing response : %+v", err)
	}
}

func (g *GraylogEmulator) graylogEmuHandlerFromConfig(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte(g.appConfig.GraylogEmulator.Data[g.getNextIndex()])); err != nil {
		log.WithField(ec.FIELD, ec.LME_1605).Errorf("GraylogEmulator : Error writing response : %+v", err)
	}
}

func (g *GraylogEmulator) getNextIndex() int {
	g.Lock()
	defer g.Unlock()
	g.currentIndex++
	g.currentIndex %= g.sourcesCount
	log.Debugf("GraylogEmulator : currentIndex %v is generated", g.currentIndex)
	return g.currentIndex
}
