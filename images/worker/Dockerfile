# Copyright 2024 The MathWorks, Inc.
# This Dockerfile allows you to build a Docker® image for MATLAB® Parallel Server workers.
# The image has MATLAB and MATLAB Parallel Server installed using the MATLAB Package Manager.
# Use the optional build argument to customize the version of MATLAB.

# Here is an example docker build command with the optional build arguments.
# docker build --build-arg MATLAB_RELEASE=r2024a -t matlab-parallel-server .

# To specify which MATLAB release to install in the container, edit the value of the MATLAB_RELEASE argument.
# Use lowercase to specify the release, for example: ARG MATLAB_RELEASE=r2024a
ARG MATLAB_RELEASE=r2024a

# When you start the build stage, this Dockerfile by default uses the Ubuntu-based matlab-deps image.
# To check the available matlab-deps images, see: https://hub.docker.com/r/mathworks/matlab-deps
FROM mjs-job-manager:latest

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
ENV MATLAB_USER_HOME=/home/matlab
RUN useradd --shell /bin/bash --home ${MATLAB_USER_HOME}

# Copy the input file onto the container
ENV INPUT_FILE=/tmp/${MATLAB_RELEASE}.txt
COPY mpm-input-files/${MATLAB_RELEASE}.txt ${INPUT_FILE}

# Run mpm to install MATLAB using the input file delete the mpm installation afterwards.
# If mpm fails to install successfully, then print the logfile in the terminal, otherwise clean up.
# Install support packages into the matlab user's HOME folder.
RUN wget -q https://www.mathworks.com/mpm/glnxa64/mpm \
    && chmod +x mpm \
    && sudo HOME=${MATLAB_USER_HOME} ./mpm install \
    --inputfile ${INPUT_FILE} \
    || (echo "MPM Installation Failure. See below for more information:" && cat /tmp/mathworks_root.log && false) \
    && sudo rm -rf mpm /tmp/mathworks_root.log ${INPUT_FILE}

