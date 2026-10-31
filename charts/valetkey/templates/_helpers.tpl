{{- define "valetkey.fullname" -}}
{{- default .Release.Name .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "valetkey.labels" -}}
app.kubernetes.io/name: valetkey
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{- define "valetkey.selectorLabels" -}}
app.kubernetes.io/name: valetkey
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
