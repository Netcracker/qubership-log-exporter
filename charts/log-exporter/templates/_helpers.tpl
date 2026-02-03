{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to
this (by the DNS naming spec). Supports fullnameOverride, global.name,
LME_APPLICATION_NAME (deployer), and standard Helm Release.Name+nameOverride.
*/}}
{{- define "log-exporter.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else if and .Values.global .Values.global.name -}}
{{- .Values.global.name | trunc 63 | trimSuffix "-" -}}
{{- else if .Values.LME_APPLICATION_NAME -}}
{{- .Values.LME_APPLICATION_NAME | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/*
Expand the name of the chart.
*/}}
{{- define "log-exporter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "log-exporter.chart" -}}
{{- printf "%s-helm" .Chart.Name | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
The most common log-exporter chart related resources labels.
*/}}
{{- define "log-exporter.coreLabels" -}}
app.kubernetes.io/name: {{ include "log-exporter.fullname" . | quote }}
app.kubernetes.io/instance: {{ include "log-exporter.instance" . | quote }}
app.kubernetes.io/version: {{ default "0.0.0" .Values.ARTIFACT_DESCRIPTOR_VERSION | trunc 63 | trimAll "-_." | quote }}
app.kubernetes.io/part-of: {{ default (include "log-exporter.fullname" .) .Values.PART_OF | quote }}
{{- end -}}

{{/*
Core log-exporter chart related resources labels with backend component label.
For Deployment, StatefulSet, DaemonSet.
*/}}
{{- define "log-exporter.defaultLabels" -}}
{{ include "log-exporter.coreLabels" . }}
app.kubernetes.io/component: backend
app.kubernetes.io/managed-by: saasDeployer
{{- end -}}

{{/*
Common labels for all objects (ConfigMap, Secret, Service, ServiceMonitor).
*/}}
{{- define "log-exporter.commonLabels" -}}
app.kubernetes.io/name: {{ include "log-exporter.fullname" . | quote }}
app.kubernetes.io/part-of: {{ default (include "log-exporter.fullname" .) .Values.PART_OF | quote }}
app.kubernetes.io/managed-by: saasDeployer
{{- end -}}

{{/*
Instance label value: fullname-namespace for unique identification per release.
*/}}
{{- define "log-exporter.instance" -}}
{{- printf "%s-%s" (include "log-exporter.fullname" .) .Values.NAMESPACE | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Find a qubership-log-exporter image in various places.
Image can be found from:
* from default values .Values.qubershipLogExporter.image
*/}}
{{- define "qubership-log-exporter.image" -}}
  {{- if .Values.qubershipLogExporter.image -}}
    {{- printf "%s" .Values.qubershipLogExporter.image -}}
  {{- else -}}
    {{- print "ghcr.io/netcracker/qubership-log-exporter:main" -}}
  {{- end -}}
{{- end -}}
