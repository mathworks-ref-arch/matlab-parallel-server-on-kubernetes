# Copyright 2024-2025 The MathWorks, Inc.
# This Dockerfile allows you to build a Docker® image for the MATLAB® Job Scheduler job manager.
# The MATLAB Package Manager installs MATLAB and MATLAB Parallel Server on the image.
# Use the optional build argument to customize the version of MATLAB the MATLAB Package Manager installs.

# Here is an example docker build command with the optional build argument.
# docker build --build-arg MATLAB_RELEASE=r2025a -t mjs-job-manager .

# To specify which MATLAB release to install in the container, edit the value of the MATLAB_RELEASE argument.
# Use lowercase to specify the release, for example: ARG MATLAB_RELEASE=r2025a
ARG MATLAB_RELEASE=r2025a

# When you start the build stage, by default this Dockerfile uses the Ubuntu-based matlab-deps image.
# To check the available matlab-deps images, see: https://hub.docker.com/r/mathworks/matlab-deps
FROM mathworks/matlab-deps:${MATLAB_RELEASE}

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

# Run mpm to install MATLAB.
RUN wget -q https://www.mathworks.com/mpm/glnxa64/mpm \
    && chmod +x mpm \
    && sudo ./mpm install \
    --release=${MATLAB_RELEASE} \
    --destination=/opt/matlab \
    --products MATLAB \
    || (echo "MPM Installation Failure. See below for more information:" && cat /tmp/mathworks_root.log && false) \
    && sudo rm -rf mpm /tmp/mathworks_root.log

# Run mpm to install MATLAB Parallel Server.
RUN wget -q https://www.mathworks.com/mpm/glnxa64/mpm \
    && chmod +x mpm \
    && sudo ./mpm install \
    --release=${MATLAB_RELEASE} \
    --destination=/opt/matlab \
    --products MATLAB_Parallel_Server \
    || (echo "MPM Installation Failure. See below for more information:" && cat /tmp/mathworks_root.log && false) \
    && sudo rm -rf mpm /tmp/mathworks_root.log

