# Define derived settings for use in multiple template files.
# Copyright 2024-2026 The MathWorks, Inc.

# Maximum number of pool proxies needed by this chart
{{- define "derived.numProxies" -}}
{{ divf .Values.maxWorkers .Values.workersPerPoolProxy | ceil }}
{{- end -}}

# MJS lookup port
{{- define "derived.lookupPort" -}}
{{ add .Values.basePort 6 | int }}
{{- end -}}

# Job manager port
{{- define "derived.jobManagerPort" -}}
{{- if ge .Values.matlabRelease "r2026b" -}}
{{ add .Values.basePort 7 | int }}
{{- else -}}
{{ add .Values.basePort 9 | int }}
{{- end -}}
{{- end -}}

# Whether to create environment variables for each service in the namespace (default false to prevent creation of too many environment variables)
{{- define "derived.enableServiceLinks" -}}
{{ .Values.enableServiceLinks | default false }}
{{- end -}}

# If we are using a secure LDAP server and not using a persistent volume claim for the job manager pod, we need to add the LDAP certificate to the job manager's secret store
{{- define "derived.addLDAPCert" -}}
{{ and (hasPrefix "ldaps://" .Values.ldapURL) (or (empty .Values.matlabPVC) (not .Values.jobManagerUsesPVC)) }}
{{- end -}}

# Which proxy mode to use. The options are:
# no-proxy-mode: No proxies started, used when there are only internal clients
#
# legacy-proxy-mode: Used when all clients < 26a, starts HAProxy and pool proxy
#
# backwards-compatible-mode: Used when there are clients both  < 26a and >= 26a,
# starts HAProxy, pool proxy and parallel server proxy
#
# new-proxy-mode: Used when all clients are >= 26a, and only MATLAB client access
# is needed. Only starts parallel server proxy
#
# new-multi-proxy-mode: Used when all clients are >= 26a, and more than just MATLAB 
# client access is needed (e.g. access is needed for metrics). Starts parallel server proxy and HAProxy
{{- define "derived.proxyMode" -}}
{{- $firstNewProxyRelease := "r2026a" -}}
{{- $oldestRelease := .Values.matlabRelease -}}
{{- $newestRelease := .Values.matlabRelease -}}
{{- if not (empty .Values.additionalSupportedReleases) -}}
    {{- $sortedReleases := sortAlpha .Values.additionalSupportedReleases -}}
    {{- $oldestRelease = index $sortedReleases 0 -}}
{{- end -}}
{{- if .Values.internalClientsOnly -}}
no-proxy-mode
{{- else -}}
  {{- if ge $oldestRelease $firstNewProxyRelease -}}
    {{- if and .Values.exportMetrics .Values.openMetricsPortOutsideKubernetes -}}
new-multi-proxy-mode
    {{- else -}}
new-proxy-mode
    {{- end -}}
  {{- else if lt $newestRelease $firstNewProxyRelease -}}
legacy-proxy-mode  
  {{- else -}}
backwards-compatible-mode
  {{- end -}}
{{- end -}}
{{- end -}}

{{- define "derived.usePoolProxy" -}}
{{- $proxyMode := include "derived.proxyMode" . -}}
{{- or (eq $proxyMode "legacy-proxy-mode") (eq $proxyMode "backwards-compatible-mode") -}}
{{- end -}}

{{- define "derived.useHAProxy" -}}
{{- $proxyMode := include "derived.proxyMode" . -}}
{{- or (or (eq $proxyMode "legacy-proxy-mode") (eq $proxyMode "backwards-compatible-mode")) (eq $proxyMode "new-multi-proxy-mode") -}}
{{- end -}}

{{- define "derived.useHAProxyForMATLABClients" -}}
{{- $proxyMode := include "derived.proxyMode" . -}}
{{- ne $proxyMode "new-multi-proxy-mode" -}}
{{- end -}}

{{- define "derived.useParallelServerProxy" -}}
{{- $proxyMode := include "derived.proxyMode" . -}}
{{- or (or (eq $proxyMode "new-proxy-mode") (eq $proxyMode "backwards-compatible-mode")) (eq $proxyMode "new-multi-proxy-mode") -}}
{{- end -}}

# We need a shared secret if any of requireScriptVerification, useSecureCommunication or requireClientCertificate are true
{{- define "derived.requiresSecret" -}}
{{ or .Values.requireScriptVerification (or .Values.useSecureCommunication .Values.requireClientCertificate) }}
{{- end -}}

# Domain of the Kubernetes cluster
{{- define "derived.clusterDomain" -}}
{{ .Values.clusterDomain | default "cluster.local" }}
{{- end -}}

# Tag for pool proxy image, we want to use the latest of (matlabRelease, additionalSupportedReleases) older than 26a
{{- define "derived.computedPoolProxyImageTag" -}}
{{- $tag := "nil" -}}
{{- $firstNewProxyRelease := "r2026a" -}}
{{- if (lt .Values.matlabRelease $firstNewProxyRelease) -}}
  {{- $tag = .Values.matlabRelease -}}
{{- end -}}
{{- range $i, $v := .Values.additionalSupportedReleases }}
  {{- if and (lt $v $firstNewProxyRelease) (or (eq $tag "nil") (gt $v $tag)) }}
    {{- $tag = $v -}}
  {{- end }}
{{- end }}
{{- $tag -}}
{{- end -}}
