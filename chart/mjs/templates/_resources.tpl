# Define Kubernetes resource names for use in multiple template files.
# Copyright 2024-2026 The MathWorks, Inc.

# Job manager
{{- define "resources.jobManager" -}}
mjs-job-manager
{{- end -}}

# Ingress proxy
{{- define "resources.ingressProxy" -}}
mjs-ingress-proxy
{{- end -}}

# Parallel server proxy
{{- define "resources.parallelServerProxy" -}}
mjs-parallel-server-proxy
{{- end -}}

# Controller
{{- define "resources.controller" -}}
mjs-controller
{{- end -}}

# Pool proxy
{{- define "resources.poolProxy" -}}
mjs-pool-proxy
{{- end -}}

# Prefix for pool proxy pods
{{- define "resources.poolProxyPrefix" -}}
mjs-pool-proxy-
{{- end -}}

# Job manager config map
{{- define "resources.jobManagerConfigMap" -}}
mjs-job-manager-config
{{- end -}}

# Worker config map
{{- define "resources.workerConfigMap" -}}
mjs-worker-config
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

# Shared secret
{{- define "resources.sharedSecret" -}}
mjs-shared-secret
{{- end -}}

# Metrics secret containing server certificates
{{- define "resources.metricsSecret" -}}
mjs-metrics-secret
{{- end -}}

# Metrics secret containing client certificates
{{- define "resources.clientMetricsSecret" -}}
mjs-metrics-client-certs
{{- end -}}

# LDAP secret
{{- define "resources.ldapSecret" -}}
mjs-ldap-secret
{{- end -}}

# Worker label
{{- define "resources.worker" -}}
mjs-worker
{{- end -}}

# Prefix for worker pods
{{- define "resources.workerPrefix" -}}
mjs-worker-
{{- end -}}

# Worker pod template name
{{- define "resources.workerTemplate" -}}
mjs-worker-template
{{- end -}}

# Pool proxy pod template name
{{- define "resources.poolProxyTemplate" -}}
mjs-pool-proxy-template
{{- end -}}

# Name of template file within a config map
{{- define "resources.templateFile" -}}
pod-template.yaml
{{- end -}}

# Worker headless service
{{- define "resources.workerService" -}}
mjs-workers
{{- end -}}

# Parallel Server proxy secret
{{- define "resources.parallelServerProxySecret" -}}
mjs-parallel-server-proxy-secret
{{- end -}}

# Admin password secret
{{- define "resources.adminPasswordSecret" -}}
mjs-admin-password
{{- end -}}

# Key for admin password inside secret
{{- define "resources.adminPasswordKey" -}}
password
{{- end -}}

# Volume for pool proxy certificate secrets
{{- define "resources.proxyCertVol" -}}
proxy-cert-vol
{{- end -}}

# Volume for the MJS shared secret
{{- define "resources.sharedSecretVol" -}}
shared-secret-vol
{{- end -}}

# Cluster profile secret
{{- define "resources.profileSecret" -}}
mjs-cluster-profile
{{- end -}}

# Cluster profile secret for pre-26a MATLAB clients in backwards compatible clusters
{{- define "resources.profileSecretPre26a" -}}
mjs-pre26a-cluster-profile
{{- end -}}

# Key of the cluster profile inside the secret
{{- define "resources.profileKey" -}}
profile
{{- end -}}
