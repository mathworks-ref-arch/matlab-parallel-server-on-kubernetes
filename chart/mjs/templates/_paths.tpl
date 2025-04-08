# Define paths of files and directories for use in multiple template files.
# Copyright 2024 The MathWorks, Inc.

# MATLAB root on containers
{{- define "paths.matlabroot" -}}
/opt/matlab
{{- end -}}

# Checkpoint base on containers; all MJS data except the job database is stored here.
# Note that this data is not persisted across pod restarts, as we want a fresh MJS every time.
{{- define "paths.checkpointbase" -}}
/tmp/checkpoint
{{- end -}}

# Database directory on the job manager pod.
# If running as non-root user and not mounting a checkpoint PVC, use a directory that a non-root user can create 
{{- define "paths.databasedir" -}}
{{- $isNonRoot := ne (int $.Values.jobManagerUserID) 0 -}}
{{- if and $isNonRoot (empty $.Values.checkpointPVC) -}} 
/tmp/database
{{- else -}}
/mjs/database
{{- end -}}
{{- end -}}

# Log base on containers
# If running as non-root user and not mounting log directory, use a directory that a non-root user can create 
{{- define "paths.logbase" -}}
{{- $isNonRoot := ne (int $.Values.jobManagerUserID) 0 -}}
{{- if and $isNonRoot (empty $.Values.logPVC) -}} 
/tmp/log
{{- else -}}
/mjs/log
{{- end -}}
{{- end -}}

# MJS config file directory
{{- define "paths.configDir" -}}
/mjs/config
{{- end -}}

# Shared secret directory
{{- define "paths.secretDir" -}}
/mjs/secret
{{- end -}}

# Name of the shared secret file
{{- define "paths.secretFile" -}}
secret.json
{{- end -}}

# Name of the certificate file
{{- define "paths.certFile" -}}
certificate.json
{{- end -}}

# Full path of the shared secret file
{{- define "paths.secretPath" -}}
{{- printf "%s/%s" (include "paths.secretDir" .) (include "paths.secretFile" .) -}}
{{- end -}}

# Full path of the certificate file
{{- define "paths.certPath" -}}
{{- printf "%s/%s" (include "paths.secretDir" .) (include "paths.certFile" .) -}}
{{- end -}}

# Name of controller config file
{{- define "paths.controllerConfig" -}}
config.json
{{- end -}}

# Controller log file
# If not mounting logs, do not write controller logs to a file
{{- define "paths.controllerLog" -}}
{{- if .Values.logPVC -}}
{{- printf "%s/%s" (include "paths.logbase" .) "controller.log" -}}
{{- else -}}
{{- end -}}
{{- end -}}

# Ingress proxy config file
{{- define "paths.ingressProxyConfig" -}}
haproxy.cfg
{{- end -}}

# Path to mounted LDAP certificate
{{- define "paths.ldapCert" -}}
/mjs/ldap/cert.pem
{{- end -}}

# Path to directory containing mounted metrics certificates
{{- define "paths.metricsCertDir" -}}
/mjs/metrics/
{{- end -}}