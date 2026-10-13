{{- define "valkey-scale-lab.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "valkey-scale-lab.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "valkey-scale-lab.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "valkey-scale-lab.labels" -}}
app.kubernetes.io/name: {{ include "valkey-scale-lab.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end }}

{{- define "valkey-scale-lab.selectorLabels" -}}
app.kubernetes.io/name: {{ include "valkey-scale-lab.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
