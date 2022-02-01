{{/*
Expand the name of the chart.
*/}}
{{- define "qnap-csi.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "qnap-csi.fullname" -}}
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
{{- define "qnap-csi.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "qnap-csi.labels" -}}
helm.sh/chart: {{ include "qnap-csi.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "qnap-csi.selectorLabels" -}}
app.kubernetes.io/name: {{ include "qnap-csi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Node Selector labels
*/}}
{{- define "qnap-csi.nodeSelectorLabels" -}}
{{ include "qnap-csi.selectorLabels" . }}
app.kubernetes.io/component: node
{{- end -}}

{{/*
Controller Selector labels
*/}}
{{- define "qnap-csi.controllerSelectorLabels" -}}
{{ include "qnap-csi.selectorLabels" . }}
app.kubernetes.io/component: controller
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "qnap-csi.serviceAccountName" -}}
{{- default (include "qnap-csi.fullname" .) .Values.serviceAccount.name }}
{{- end }}

{{/*
Create the name of the controller deployment to use
*/}}
{{- define "qnap-csi.controllerDeploymentName" -}}
{{- default (printf "%s-controller" (include "qnap-csi.fullname" .)) .Values.controller.name }}
{{- end }}

{{/*
Create the name of the node daemonset to use
*/}}
{{- define "qnap-csi.nodeDaemonsetName" -}}
{{- default (printf "%s-node" (include "qnap-csi.fullname" .)) .Values.node.name }}
{{- end }}
