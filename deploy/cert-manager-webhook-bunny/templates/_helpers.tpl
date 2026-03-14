{{/*
Expand the name of the chart.
*/}}
{{- define "cert-manager-webhook-bunny.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "cert-manager-webhook-bunny.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "cert-manager-webhook-bunny.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "cert-manager-webhook-bunny.labels" -}}
helm.sh/chart: {{ include "cert-manager-webhook-bunny.chart" . }}
{{ include "cert-manager-webhook-bunny.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "cert-manager-webhook-bunny.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cert-manager-webhook-bunny.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
PKI helper names
*/}}
{{- define "cert-manager-webhook-bunny.selfSignedIssuer" -}}
{{ printf "%s-selfsign" (include "cert-manager-webhook-bunny.fullname" .) }}
{{- end }}

{{- define "cert-manager-webhook-bunny.rootCAIssuer" -}}
{{ printf "%s-ca" (include "cert-manager-webhook-bunny.fullname" .) }}
{{- end }}

{{- define "cert-manager-webhook-bunny.rootCACertificate" -}}
{{ printf "%s-ca" (include "cert-manager-webhook-bunny.fullname" .) }}
{{- end }}

{{- define "cert-manager-webhook-bunny.servingCertificate" -}}
{{ printf "%s-webhook-tls" (include "cert-manager-webhook-bunny.fullname" .) }}
{{- end }}
