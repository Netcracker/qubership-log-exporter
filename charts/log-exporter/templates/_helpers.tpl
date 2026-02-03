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
Resource labels per spec: name, app.kubernetes.io/name, component (workloads only), part-of, managed-by.
For CRs: pass processedByOperator to add processed-by-operator.
Usage: {{- include "log-exporter.resourceLabels" (dict "ctx" . "name" $name "component" $component) | nindent 4 }}
       For CRs: add "processedByOperator" "prometheus-operator"
       For workloads (Deployment, StatefulSet, DaemonSet): add instance, version, technology inline.
*/}}
{{- define "log-exporter.resourceLabels" -}}
{{- $ctx := .ctx -}}
{{- $name := .name -}}
{{- $component := .component -}}
name: {{ $name }}
app.kubernetes.io/name: {{ $name }}
{{- if $component }}
app.kubernetes.io/component: {{ $component }}
{{- end }}
app.kubernetes.io/part-of: {{ default (include "log-exporter.fullname" $ctx) (index $ctx.Values "PART_OF") | quote }}
app.kubernetes.io/managed-by: {{ $ctx.Release.Service }}
{{- if .processedByOperator }}
app.kubernetes.io/processed-by-operator: {{ .processedByOperator }}
{{- end }}
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
