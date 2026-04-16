# Define keys for annotations on Kubernetes resources.
# Copyright 2026 The MathWorks, Inc.

{{- define "annotations.certVolume" -}}
mjs/cert-volume
{{- end -}}

{{- define "annotations.poolProxyBasePort" -}}
mjs/pool-proxy-base-port
{{- end -}}

{{- define "annotations.poolProxyPrefix" -}}
mjs/pool-proxy-prefix
{{- end -}}

{{- define "annotations.workerPrefix" -}}
mjs/worker-prefix
{{- end -}}

{{- define "annotations.useSecureCommunication" -}}
mjs/use-secure-communication
{{- end -}}

{{- define "annotations.usePoolProxy" -}}
mjs/use-pool-proxy
{{- end -}}

{{- define "annotations.logDir" -}}
mjs/log-dir
{{- end -}}

{{- define "annotations.workerDomain" -}}
mjs/worker-domain
{{- end -}}

{{- define "annotations.workersPerPoolProxy" -}}
mjs/workers-per-pool-proxy
{{- end -}}

{{- define "annotations.poolProxyID" -}}
mjs/pool-proxy-id
{{- end -}}

{{- define "annotations.secretName" -}}
mjs/secret-name
{{- end -}}

{{- define "annotations.workerName" -}}
mjs/worker-name
{{- end -}}

{{- define "annotations.workerID" -}}
mjs/worker-id
{{- end -}}

{{- define "annotations.templateChecksum" -}}
checksum/template
{{- end -}}
