# Copyright 2026 The MathWorks, Inc.
# Base image for MJS job manager and worker images. Wraps mathworks/matlab-deps
# with the JRE that MJS requires (no longer shipped inside MATLAB from r2026b).

ARG MATLAB_RELEASE=r2026b
ARG MATLAB_DEPS=mathworks/matlab-deps
FROM ${MATLAB_DEPS}:${MATLAB_RELEASE}

ARG MATLAB_RELEASE
ARG MATLAB_DEPS

# Install openjdk-11-jre for releases r2026b and newer.
# This adds java to $PATH so it will get picked up by MJS.
RUN if [ ! "${MATLAB_RELEASE}" \< "r2026b" ]; then \
    export DEBIAN_FRONTEND=noninteractive \
    && apt-get update \
    && apt-get install --no-install-recommends --yes openjdk-11-jre \
    && apt-get clean \
    && apt-get autoremove \
    && rm -rf /var/lib/apt/lists/*; \
    fi
