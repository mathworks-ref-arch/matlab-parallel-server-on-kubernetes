# Shared shell script snippets for use in multiple template files.
# Copyright 2026 The MathWorks, Inc.

# Shell script snippet to determine the MATLAB root directory and set MATLAB_ROOT and BIN_DIR.
{{- define "scripts.setMatlabRoot" -}}
# Determine the MATLAB root directory
{{- if .Values.matlabRoot }}
MATLAB_ROOT={{ .Values.matlabRoot | quote }}
{{- else }}
if [ -d "/opt/matlab/toolbox/parallel/bin" ]; then
    MATLAB_ROOT="/opt/matlab"
elif [ -d "/opt/matlab/{{ .Values.matlabRelease | replace "r" "R" }}/toolbox/parallel/bin" ]; then
    MATLAB_ROOT="/opt/matlab/{{ .Values.matlabRelease | replace "r" "R" }}"
else
    echo "Unable to find a MATLAB installation. Set the matlabRoot Helm parameter to the MATLAB root directory of your container image."
    exit 1
fi
{{- end }}
echo "${MATLAB_ROOT}" > {{ include "paths.matlabRootFile" . }}
BIN_DIR="${MATLAB_ROOT}/toolbox/parallel/bin"
{{- end -}}
