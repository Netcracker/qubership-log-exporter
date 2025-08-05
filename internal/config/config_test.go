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
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestExampleConfigs(t *testing.T) {
	log.SetLevel(log.ErrorLevel)
	exampleConfigs := []string{
		"../../examples/config_cloud_promrw.yaml",
		"../../examples/config_cloud_victoria.yaml",
		"../../examples/config_emu.yaml",
		"../../examples/config_nr.yaml",
		"../../examples/unit_test.yaml",
	}
	for i, path := range exampleConfigs {
		t.Run("TestExampleConfig"+fmt.Sprintf("%v", i), func(t *testing.T) {
			testConfig, err := SimpleSilentRead(path)
			if err != nil {
				t.Errorf("Error parsing test config %v : %+v", path, err)
			}
			err = ValidateConfig(testConfig)
			if err != nil {
				t.Errorf("Error validating test config %v : %+v", path, err)
			}
		})
	}
}
