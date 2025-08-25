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

package utils

import (
	ec "log_exporter/internal/utils/errorcodes"
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"
)

func GetOctalUintEnvironmentVariable(name string, defaultValue uint32) uint32 {
	valueStr := os.Getenv(name)
	if valueStr == "" {
		log.Infof("Environment variable %v is empty. Default value %o is used", name, defaultValue)
		return defaultValue
	}
	result, err := strconv.ParseUint(valueStr, 8, 32)

	if err != nil {
		log.WithField(ec.FIELD, ec.LME_8102).Errorf("Error trying to parse uint octal environment variable %v with value %v, default value %o is used instead. Error : %+v", name, valueStr, defaultValue, err)
		return defaultValue
	}

	log.Infof("Environment variable %v with value %v parsed successfully as octal uint %o", name, valueStr, result)

	return uint32(result)
}
