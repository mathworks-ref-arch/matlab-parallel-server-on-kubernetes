# Copyright 2024 The MathWorks, Inc.
# This Dockerfile allows you to build a Docker® image for MATLAB® Parallel Server workers.
# The image has MATLAB and MATLAB Parallel Server installed using the MATLAB Package Manager.
# Use the optional build argument to customize the version of MATLAB.

# Here is an example docker build command with the optional build arguments.
# docker build --build-arg MATLAB_RELEASE=r2024a -t matlab-parallel-server .

# To specify which MATLAB release to install in the container, edit the value of the MATLAB_RELEASE argument.
# Use lowercase to specify the release, for example: ARG MATLAB_RELEASE=r2024a
ARG MATLAB_RELEASE=r2024a
FROM ghcr.io/mathworks-ref-arch/matlab-parallel-server-k8s/mjs-job-manager-image:${MATLAB_RELEASE}

# Declare build arguments to use at the current build stage.
ARG MATLAB_RELEASE

# Install mpm dependencies.
RUN export DEBIAN_FRONTEND=noninteractive \
    && apt-get update \
    && apt-get install --no-install-recommends --yes \
    wget \
    unzip \
    ca-certificates \
    && apt-get clean \
    && apt-get autoremove \
    && rm -rf /var/lib/apt/lists/*

# Add the matlab user for workers to run as
ENV USER_HOME=/home/matlab
RUN useradd matlab --shell /bin/bash --home ${USER_HOME}

# Copy the input file onto the container
ENV INPUT_DIR=/tmp/${MATLAB_RELEASE}
COPY mpm-input-files/${MATLAB_RELEASE} ${INPUT_DIR}

# Run mpm to install simulink.
RUN wget -q https://www.mathworks.com/mpm/glnxa64/mpm \
    && chmod +x mpm \
    && sudo HOME=${USER_HOME} ./mpm install \
    --release=${MATLAB_RELEASE}u0 \
    --destination=/opt/matlab \
    --products Simulink \
    || (echo "MPM Installation Failure. See below for more information:" && cat /tmp/mathworks_root.log && false) \
    && sudo rm -rf mpm /tmp/mathworks_root.log ${INPUT_FILE}

# Run mpm to install MATLAB toolboxes.
RUN wget -q https://www.mathworks.com/mpm/glnxa64/mpm \
    && chmod +x mpm \
    && sudo HOME=${USER_HOME} ./mpm install \
    --inputfile ${INPUT_FILE}/toolboxes.txt \
    || (echo "MPM Installation Failure. See below for more information:" && cat /tmp/mathworks_root.log && false) \
    && sudo rm -rf mpm /tmp/mathworks_root.log ${INPUT_FILE}

# Run mpm to install support packages into the matlab user's HOME folder.
RUN wget -q https://www.mathworks.com/mpm/glnxa64/mpm \
    && chmod +x mpm \
    && sudo HOME=${USER_HOME} ./mpm install \
    --inputfile ${INPUT_FILE}/support_packages.txt \
    || (echo "MPM Installation Failure. See below for more information:" && cat /tmp/mathworks_root.log && false) \
    && sudo rm -rf mpm /tmp/mathworks_root.log ${INPUT_FILE}
