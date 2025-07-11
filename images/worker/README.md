# Create a MATLAB Parallel Server Container Image

This Dockerfile in this folder builds an image for MATLAB&reg; Parallel Server workers using [MATLAB Package Manager (*mpm*)](https://github.com/mathworks-ref-arch/matlab-dockerfile/blob/main/MPM.md).
Use the Dockerfile to build a custom image that contains the toolboxes and support packages you need.

## Requirements
- Docker&reg; installed on your computer. For help with installing Docker, see [Get Docker](https://docs.docker.com/get-docker/).

## Build Instructions

### Download Respository

To download a zip file of this repository, at the top of this repository page, select Code > Download ZIP.
Alternatively, to clone this repository to your computer with Git installed, run the following command on your operating system's command line:

```
git clone https://github.com/mathworks-ref-arch/matlab-parallel-server-on-kubernetes
```

Navigate to this directory by running
```
cd matlab-parallel-server-on-kubernetes/images/worker
```

### Modify `mpm` Input Files

The toolboxes and support packages that `mpm` installs on your image are configured using `mpm` input files.
The `mpm-input-files/` folder contains a folder for each MATLAB release that supports MATLAB Job Scheduler in Kubernetes containing the following files:
- `matlab-toolboxes.txt`: An `mpm` input file containing a list of MATLAB toolboxes to install. By default, the input file installs all MATLAB toolboxes.
- `simulink-toolboxes.txt`: An `mpm` input file containing a list of Simulink toolboxes to install. By default, the input file installs all Simulink toolboxes.
- `support-packages.txt`: An `mpm` input file containing a list of support packages to install. By default, the input file only installs Deep Learning support packages.

Edit the files in the directory corresponding to the release you want to build an image for.
For example, if you want to build an image for MATLAB R2025a with custom MATLAB toolboxes, edit the `mpm-input-files/r2025a/toolboxes.txt` file.

Comment out any toolboxes or support packages you do not want to install by adding a `#` symbol before the toolbox or support package name.
Uncomment any support packages you want to install by removing the `#` symbol from the beginning of the line containing the support package name.

### Build Image

Build the Docker image.
- Specify `<release>` as a MATLAB release number with a lowercase `r`. For example, to install MATLAB R2025a, specify `<release>` as `r2025a`.
- Specify `<include-simulink>` as `true` to install Simulink and the Simulink toolboxes, or `false` to only install MATLAB toolboxes and support packages.
- Specify `<my-tag>` as the Docker tag to use for the image.

```
docker build --build-arg MATLAB_RELEASE=<release> --build-arg INCLUDE_SIMULINK=<include-simulink> -t <my-tag> .
```

---

Copyright 2024-2025 The MathWorks, Inc.
