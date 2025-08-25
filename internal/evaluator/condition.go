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
	"log_exporter/internal/utils"

	log "github.com/sirupsen/logrus"
)

type MECondition struct { // Metric Evaluation conditions
	subConditions []MESubCondition
}

type MESubCondition struct {
	equConditions []EquCondition
}

const EQU_COND_NAME string = "equ"

type EquCondition struct {
	FieldIndex int
	FieldValue string
}

func CreateMECondition(metric string, condConfig []map[string]map[string]string, header []string) *MECondition {
	res := MECondition{}

	res.subConditions = make([]MESubCondition, 0, len(condConfig))

	for i, cond := range condConfig {
		log.Debugf("For metric %v processing condition %v ...", metric, i)
		equCond := cond[EQU_COND_NAME]
		if equCond == nil {
			log.Debugf("For metric %v, condition %v equCond = nil", metric, i)
			continue
		}
		mesc := MESubCondition{}
		equConditions := make([]EquCondition, 0, len(equCond))
		for fieldName, fieldValue := range equCond {
			log.Debugf("For metric %v, condition %v processing equCond : fieldName %v, fieldValue %v", metric, i, fieldName, fieldValue)
			equCondition := EquCondition{}
			equCondition.FieldValue = fieldValue
			equCondition.FieldIndex = utils.FindStringIndexInArray(header, fieldName)
			equConditions = append(equConditions, equCondition)
		}
		log.Debugf("For metric %v, condition %v equConditions = %+v", metric, i, equConditions)
		mesc.equConditions = equConditions

		res.subConditions = append(res.subConditions, mesc)
	}
	log.Debugf("For metric %v MECondition = %+v", metric, res)

	return &res
}

func (c *MECondition) Apply(row []string) bool {
	for _, subCondition := range c.subConditions {
		if subCondition.apply(row) {
			return true
		}
	}
	return false
}

func (c *MESubCondition) apply(row []string) bool {
	for _, equCondition := range c.equConditions {
		if equCondition.FieldValue != row[equCondition.FieldIndex] {
			return false
		}
	}
	return true
}
