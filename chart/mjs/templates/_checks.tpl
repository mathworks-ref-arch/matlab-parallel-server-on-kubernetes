# Validation checks run at helm install/upgrade time.
# Copyright 2026 The MathWorks, Inc.

# Validate that all additionalSupportedReleases are older than matlabRelease
{{- define "checks.validateAdditionalReleases" -}}
{{- range .Values.additionalSupportedReleases -}}
  {{- if ge . $.Values.matlabRelease -}}
    {{- fail (printf "additionalSupportedReleases contains %s, which is newer than the matlabRelease %s. additionalSupportedReleases must specify only releases older than the matlabRelease." . $.Values.matlabRelease) -}}
  {{- end -}}
{{- end -}}
{{- end -}}

# Validate that all PersistentVolumeClaims referenced in Helm values exist in the namespace.
# Uses the Kubernetes API via the lookup function; skipped during helm template (dry-run).
{{- define "checks.validatePVCs" -}}
{{- $ns := .Release.Namespace -}}
{{- $apiAvailable := lookup "v1" "Namespace" "" $ns -}}
{{- if $apiAvailable -}}
  {{- $pvcNames := list -}}
  {{- if .Values.checkpointPVC -}}
    {{- $pvcNames = append $pvcNames .Values.checkpointPVC -}}
  {{- end -}}
  {{- if .Values.logPVC -}}
    {{- $pvcNames = append $pvcNames .Values.logPVC -}}
  {{- end -}}
  {{- if .Values.workerLogPVC -}}
    {{- $pvcNames = append $pvcNames .Values.workerLogPVC -}}
  {{- end -}}
  {{- if .Values.matlabPVC -}}
    {{- $pvcNames = append $pvcNames .Values.matlabPVC -}}
  {{- end -}}
  {{- if .Values.javaPVC -}}
    {{- $pvcNames = append $pvcNames .Values.javaPVC -}}
  {{- end -}}
  {{- range .Values.additionalMatlabPVCs -}}
    {{- $pvcNames = append $pvcNames . -}}
  {{- end -}}
  {{- range $pvc, $path := .Values.additionalWorkerPVCs -}}
    {{- $pvcNames = append $pvcNames $pvc -}}
  {{- end -}}
  {{- range $pvcNames -}}
    {{- $pvcObj := lookup "v1" "PersistentVolumeClaim" $ns . -}}
    {{- if not $pvcObj -}}
      {{- fail (printf "Unable to find PersistentVolumeClaim %q in namespace %q. Ensure the PVC exists before installing the Helm chart." . $ns) -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- end -}}

# Validate that the administrator password secret exists and contains the expected key.
# Only checked when securityLevel >= 2.
{{- define "checks.validateAdminPassword" -}}
{{- if ge (int .Values.securityLevel) 2 -}}
  {{- $ns := .Release.Namespace -}}
  {{- $apiAvailable := lookup "v1" "Namespace" "" $ns -}}
  {{- if $apiAvailable -}}
    {{- $secretName := include "resources.adminPasswordSecret" . -}}
    {{- $key := include "resources.adminPasswordKey" . -}}
    {{- $secret := lookup "v1" "Secret" $ns $secretName -}}
    {{- if not $secret -}}
      {{- fail (printf "Unable to find administrator password secret %q in namespace %q. To start a MATLAB Job Scheduler cluster at security level %d, create an administrator password secret with command: kubectl create secret generic %s --from-literal=%s=<password> --namespace %s" $secretName $ns (int .Values.securityLevel) $secretName $key $ns) -}}
    {{- else if not (hasKey $secret.data $key) -}}
      {{- fail (printf "Unable to find key %q in administrator password secret %q. To start a MATLAB Job Scheduler cluster at security level %d, create an administrator password secret with command: kubectl create secret generic %s --from-literal=%s=<password> --namespace %s" $key $secretName (int .Values.securityLevel) $secretName $key $ns) -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- end -}}

# Validate that the LDAP certificate secret exists and contains the expected file.
# Only checked when LDAP with TLS is configured.
{{- define "checks.validateLDAPSecret" -}}
{{- if eq (include "derived.addLDAPCert" .) "true" -}}
  {{- $ns := .Release.Namespace -}}
  {{- $apiAvailable := lookup "v1" "Namespace" "" $ns -}}
  {{- if $apiAvailable -}}
    {{- $secretName := include "resources.ldapSecret" . -}}
    {{- $certFile := include "paths.ldapCert" . -}}
    {{- $secret := lookup "v1" "Secret" $ns $secretName -}}
    {{- if not $secret -}}
      {{- fail (printf "Unable to find LDAP certificate secret %q in namespace %q. To start MATLAB Job Scheduler using an LDAPS server, create an LDAP certificate secret with command: kubectl create secret generic %s --from-file=%s=<path> --namespace %s" $secretName $ns $secretName $certFile $ns) -}}
    {{- else if not (hasKey $secret.data $certFile) -}}
      {{- fail (printf "Unable to find file %q in LDAP certificate secret %q. To start MATLAB Job Scheduler using an LDAPS server, create an LDAP certificate secret with command: kubectl create secret generic %s --from-file=%s=<path> --namespace %s" $certFile $secretName $secretName $certFile $ns) -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- end -}}
