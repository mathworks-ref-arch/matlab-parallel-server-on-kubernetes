# Define Kubernetes resource names for use in multiple template files.
# Copyright 2024 The MathWorks, Inc.

# Job manager
{{- define "resources.jobManager" -}}
mjs-job-manager
{{- end -}}

# Ingress proxy
{{- define "resources.ingressProxy" -}}
mjs-ingress-proxy
{{- end -}}

# Controller
{{- define "resources.controller" -}}
mjs-controller
{{- end -}}

# Pool proxy
{{- define "resources.poolProxy" -}}
mjs-pool-proxy
{{- end -}}

# MJS config map
{{- define "resources.mjsConfigMap" -}}
mjs-config
{{- end -}}

# Controller config map
{{- define "resources.controllerConfigMap" -}}
mjs-controller-config
{{- end -}}

# Ingress proxy config map
{{- define "resources.ingressConfigMap" -}}
mjs-ingress-proxy-config
{{- end -}}

# Controller service account
{{- define "resources.controllerServiceAccount" -}}
{{ printf "%s-serviceaccount" (include "resources.controller" .) }}
{{- end -}}

# Controller role
{{- define "resources.controllerRole" -}}
{{ printf "%s-role" (include "resources.controller" .) }}
{{- end -}}