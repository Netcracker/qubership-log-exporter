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
    "log_exporter/internal/config"
    log "github.com/sirupsen/logrus"
    dto "github.com/prometheus/client_model/go"
    "strings"
)

var (
    namespace = "namespace"
    pod = "pod"
    container = "container"
)

type PushProcessor struct {
    appConfig *config.Config
}

func (p *PushProcessor) enrichWithCloudLabels(mfs []*dto.MetricFamily) {
    if p.appConfig.General.DisablePushCloudLabels {
        return
    }

    namespaceValue := p.appConfig.General.NamespaceName
    podValue := p.appConfig.General.PodName
    containerValue := p.appConfig.General.ContainerName
    labelNamespace := dto.LabelPair{Name: &namespace, Value: &namespaceValue}
    labelPod := dto.LabelPair{Name: &pod, Value: &podValue}
    labelContainer := dto.LabelPair{Name: &container, Value: &containerValue}

    pushCloudLabels := p.appConfig.General.PushCloudLabels
    pushCloudLabelsList := make([]*dto.LabelPair, 0, len(pushCloudLabels))
    for key, value := range pushCloudLabels {
        keyClone := strings.Clone(key)
        valueClone := strings.Clone(value)
        lp := dto.LabelPair{Name: &keyClone, Value: &valueClone}
        pushCloudLabelsList = append(pushCloudLabelsList, &lp)
    }

    log.Debugf("Cloud labels were found : %v : %v ; %v : %v ; %v : %v; pushCloudLabels : %+v", namespace, namespaceValue, pod, podValue, container, containerValue, pushCloudLabels)
    for _, mf := range mfs {
        for _, metric := range mf.Metric {
            metric.Label = append(metric.Label, &labelNamespace, &labelPod, &labelContainer)
            metric.Label = append(metric.Label, pushCloudLabelsList...)
        }
    }
}