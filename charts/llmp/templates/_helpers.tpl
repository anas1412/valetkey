{{- define "llmp.fullname" -}}
{{- default .Release.Name .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "llmp.labels" -}}
app.kubernetes.io/name: llmp
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{- define "llmp.selectorLabels" -}}
app.kubernetes.io/name: llmp
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
