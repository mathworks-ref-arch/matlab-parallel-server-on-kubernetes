# Copyright 2024-2025 The MathWorks, Inc.
# This Dockerfile allows you to build a Docker® image for MATLAB® Parallel Server workers.
# The MATLAB Package Manager installs MATLAB toolboxes on the image.
# Use the optional build argument to customize the version of MATLAB the MATLAB Package Manager installs.

# Here is an example docker build command with the optional build arguments.
# docker build --build-arg MATLAB_RELEASE=r2025a -t matlab-parallel-server .

# To specify which MATLAB release to install in the container, edit the value of the MATLAB_RELEASE argument.
# Use lowercase to specify the release, for example: ARG MATLAB_RELEASE=r2025a
ARG MATLAB_RELEASE=r2025a
FROM ghcr.io/mathworks-ref-arch/matlab-parallel-server-k8s/mjs-job-manager-image:${MATLAB_RELEASE}

# Declare build arguments to use at the current build stage.
ARG MATLAB_RELEASE
ARG INCLUDE_SIMULINK=true

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

# Run mpm to install Simulink.
RUN if [ ${INCLUDE_SIMULINK} = true ]; then \
    wget -q https://www.mathworks.com/mpm/glnxa64/mpm \
    && chmod +x mpm \
    && sudo ./mpm install \
    --release=${MATLAB_RELEASE} \
    --destination=/opt/matlab \
    --products Simulink \
    || (echo "MPM Installation Failure. See below for more information:" && cat /tmp/mathworks_root.log && false) \
    && sudo rm -rf mpm /tmp/mathworks_root.log; \
    fi

# Run mpm to install MATLAB toolboxes.
ENV MATLAB_TOOLBOX_FILE=${INPUT_DIR}/matlab-toolboxes.txt
RUN wget -q https://www.mathworks.com/mpm/glnxa64/mpm \
    && chmod +x mpm \
    && sudo ./mpm install \
    --inputfile ${MATLAB_TOOLBOX_FILE} \
    || (echo "MPM Installation Failure. See below for more information:" && cat /tmp/mathworks_root.log && false) \
    && sudo rm -rf mpm /tmp/mathworks_root.log ${MATLAB_TOOLBOX_FILE}

# Run mpm to install Simulink toolboxes.
ENV SIMULINK_TOOLBOX_FILE=${INPUT_DIR}/simulink-toolboxes.txt
RUN if [ ${INCLUDE_SIMULINK} = true ]; then \
    wget -q https://www.mathworks.com/mpm/glnxa64/mpm \
    && chmod +x mpm \
    && sudo ./mpm install \
    --inputfile ${SIMULINK_TOOLBOX_FILE} \
    || (echo "MPM Installation Failure. See below for more information:" && cat /tmp/mathworks_root.log && false) \
    && sudo rm -rf mpm /tmp/mathworks_root.log ${SIMULINK_TOOLBOX_FILE}; \
    fi

# Run mpm to install support packages into the matlab user's HOME folder.
ENV SUPPORT_PACKAGE_FILE=${INPUT_DIR}/support-packages.txt
RUN wget -q https://www.mathworks.com/mpm/glnxa64/mpm \
    && chmod +x mpm \
    && sudo HOME=${USER_HOME} ./mpm install \
    --inputfile ${SUPPORT_PACKAGE_FILE} \
    || (echo "MPM Installation Failure. See below for more information:" && cat /tmp/mathworks_root.log && false) \
    && sudo rm -rf mpm /tmp/mathworks_root.log ${SUPPORT_PACKAGE_FILE}
