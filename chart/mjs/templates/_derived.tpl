# Define derived settings for use in multiple template files.
# Copyright 2024 The MathWorks, Inc.

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
{{ add .Values.basePort 9 | int }}
{{- end -}}

# Whether to create environment variables for each service in the namespace (default false to prevent creation of too many environment variables)
{{- define "derived.enableServiceLinks" -}}
{{ .Values.enableServiceLinks | default false }}
{{- end -}}

# If we are using a secure LDAP server and not using a persistent volume claim for the job manager pod, we need to add the LDAP certificate to the job manager's secret store
{{- define "derived.addLDAPCert" -}}
{{ and (hasPrefix "ldaps://" .Values.ldapURL) (or (empty .Values.matlabPVC) (not .Values.jobManagerUsesPVC)) }}
{{- end -}}

# Whether to override the workergroup config file
{{- define "derived.overrideWorkergroupConfig" -}}
{{ and (eq .Values.matlabRelease "r2024b") (not (empty .Values.networkLicenseManager)) }}
{{- end -}}