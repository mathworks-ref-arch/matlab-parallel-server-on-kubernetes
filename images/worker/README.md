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

### Modify MPM Input File

The toolboxes and support packages that will be installed on your image are configured using an MPM input file.
The `mpm-input-files/` folder contains an input file for each MATLAB release that supports MATLAB Job Scheduler in Kubernetes.
Edit the file corresponding to the release you want to build an image for.
For example, if you want to build an image for MATLAB R2024a, edit the `mpm-input-files/r2024a.txt`.

By default, these files are configured to install all toolboxes and the Deep Learning Support Packages.
Comment out any toolboxes or support packages you do not want to install by adding a `#` symbol before the toolbox or support package name.
Uncomment any support packages you want to install by removing the `#` symbol from the beginning of the line containing the support package name.

### Build Image

Build the Docker image.
- Specify `<release>` as a MATLAB release number with a lowercase `r`. For example, to install MATLAB R2024a, specify `<release>` as `r2024a`.
- Specify `<my-tag>` as the Docker tag to use for the image.

```
docker build --build-arg MATLAB_RELEASE=<release> -t <my-tag> .
```

The Docker image contains a Simulink installation by default.
To build an image without Simulink, run
```
docker build --build-arg MATLAB_RELEASE=<release>,INCLUDE_SIMULINK=false -t <my-tag> .
```
