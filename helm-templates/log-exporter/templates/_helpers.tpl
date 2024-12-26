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
