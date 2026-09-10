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
	"reflect"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	log "github.com/sirupsen/logrus"
)

func CopyMetricFamiliesFromRegistry(registry *prometheus.Registry, queryName string) []*dto.MetricFamily {
	log.WithField("query", queryName).Debug("Deep copying metric families")
	mfs, err := registry.Gather()
	if err != nil {
		log.WithField(ec.FIELD, ec.LME_1005).WithFields(log.Fields{"query": queryName, "error": err}).Error("Error during gathering, skipping pushing the metric families to the gmQueue")
	}
	copy := Copy(mfs)
	copyT, ok := copy.([]*dto.MetricFamily)
	if !ok {
		log.WithField(ec.FIELD, ec.LME_1602).WithField("copy_type", reflect.TypeOf(copy)).Error("Error asserting the copy to the []*dto.MetricFamily type, skipping pushing the metric families to the gmQueue")
		return make([]*dto.MetricFamily, 0)
	}
	return copyT
}

func Copy(src interface{}) interface{} {
	if src == nil {
		return nil
	}

	original := reflect.ValueOf(src)
	log.WithField("original", original).Trace("Deep copy call, original")

	cpy := reflect.New(original.Type()).Elem()

	recursiveCopy(original, cpy)
	log.WithField("cpy", cpy).Trace("Deep copy call, result")

	return cpy.Interface()
}

func recursiveCopy(original, cpy reflect.Value) {
	switch original.Kind() {
	case reflect.Ptr:
		originalValue := original.Elem()
		if !originalValue.IsValid() {
			return
		}
		cpy.Set(reflect.New(originalValue.Type()))
		recursiveCopy(originalValue, cpy.Elem())

	case reflect.Interface:
		if original.IsNil() {
			return
		}
		originalValue := original.Elem()
		copyValue := reflect.New(originalValue.Type()).Elem()
		recursiveCopy(originalValue, copyValue)
		cpy.Set(copyValue)

	case reflect.Struct:
		t, ok := original.Interface().(time.Time)
		if ok {
			cpy.Set(reflect.ValueOf(t))
			return
		}
		for i := 0; i < original.NumField(); i++ {
			if original.Type().Field(i).PkgPath == "" {
				recursiveCopy(original.Field(i), cpy.Field(i))
			}
		}

	case reflect.Slice:
		if original.IsNil() {
			return
		}
		cpy.Set(reflect.MakeSlice(original.Type(), original.Len(), original.Cap()))
		for i := 0; i < original.Len(); i++ {
			recursiveCopy(original.Index(i), cpy.Index(i))
		}

	case reflect.Map:
		if original.IsNil() {
			return
		}
		cpy.Set(reflect.MakeMap(original.Type()))
		for _, key := range original.MapKeys() {
			originalValue := original.MapIndex(key)
			copyValue := reflect.New(originalValue.Type()).Elem()
			recursiveCopy(originalValue, copyValue)
			copyKey := Copy(key.Interface())
			cpy.SetMapIndex(reflect.ValueOf(copyKey), copyValue)
		}

	default:
		cpy.Set(original)
	}
}
