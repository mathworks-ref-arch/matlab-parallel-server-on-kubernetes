# MATLAB Parallel Server in Kubernetes

[![View MATLAB Parallel Server in Kubernetes on File Exchange](https://www.mathworks.com/matlabcentral/images/matlab-file-exchange.svg)](https://www.mathworks.com/matlabcentral/fileexchange/167676-matlab-parallel-server-in-kubernetes)

This repository contains utilities for using MATLAB&reg; Parallel Server in a Kubernetes&reg; cluster.

## Introduction

This guide explains how to deploy MATLAB Job Scheduler onto your Kubernetes cluster.
You can then connect to the MATLAB Job Scheduler and use it to run MATLAB Parallel Server jobs on the Kubernetes cluster.

For more information on MATLAB Job Scheduler and MATLAB Parallel Server, see the MathWorks documentation on [MATLAB Parallel Server](https://www.mathworks.com/help/matlab-parallel-server/index.html).

## Requirements

To use MATLAB Job Scheduler in Kubernetes, you must have MATLAB R2024a or later.

Before you start, you need the following:
- A running Kubernetes cluster that meets the following conditions:
  - Uses Kubernetes version 1.21.1 or later.
  - Meets the system requirements for running MATLAB Job Scheduler. For details, see the MathWorks documentation for [MATLAB Parallel Server Product Requirements](https://www.mathworks.com/support/requirements/matlab-parallel-server.html).
  - Configured to create external load balancers that allow traffic into the cluster.
- Docker&reg; installed on your computer. For help with installing Docker, see [Get Docker](https://docs.docker.com/get-docker/).
- Kubectl installed on your computer and configured to access your Kubernetes cluster. For help with installing Kubectl, see [Install Tools](https://kubernetes.io/docs/tasks/tools/) on the Kubernetes website.
- Helm&reg; version 3.8.0 or later installed on your computer. For help with installing Helm, see [Quickstart Guide](https://helm.sh/docs/intro/quickstart/).
- Network access to the MathWorks Container Registry, `containers.mathworks.com`, and the GitHub&reg; Container registry, `ghcr.io`.
- A MATLAB Parallel Server license. For more information on licensing, see [Determining License Size for MATLAB Parallel Server](https://www.mathworks.com/products/matlab-parallel-server/license-model.html) on the MathWorks website.

If you do not have a license, submit a request on the MathWorks [Contact Sales](https://www.mathworks.com/company/aboutus/contact_us/contact_sales.html) page.

## Deployment Steps

### Create Namespace for MATLAB Job Scheduler

Kubernetes uses namespaces to separate groups of resources.
To learn more about namespaces, see the Kubernetes documentation for [Namespaces](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/).
To isolate the MATLAB Job Scheduler from other resources on the cluster, you must deploy MATLAB Job Scheduler inside a namespace on your cluster.

For example, to create a custom namespace with the name `mjs`, run this command:
```
kubectl create namespace mjs
```

The commands in this guide assume that you are using a namespace called `mjs`.
Substitute `mjs` with your namespace when using these commands.

### Create Persistent Volumes

MATLAB Job Scheduler uses *PersistentVolumes* to retain data beyond the lifetime of the Kubernetes pods.
Create these volumes using your preferred storage medium.
For instructions, see the [Kubernetes PersistentVolume documentation](https://kubernetes.io/docs/concepts/storage/persistent-volumes/).

The software requires three PersistentVolumes to retain job data and logs.
You can also use a PersistentVolume to mount your own MATLAB Parallel Server installation onto the MATLAB Job Scheduler pods.
If you do not create a PersistentVolume containing a MATLAB Parallel Server installation, you must use a Docker image that has MATLAB Parallel Server installed.

Create a PersistentVolume for each of the following applications:
- An empty PersistentVolume with access mode `ReadWriteOnce` for MATLAB Job Scheduler's checkpoint folder, which retains job data after exiting the session
- An empty PersistentVolume with access mode `ReadWriteOnce` to retain logs from the MATLAB Job Scheduler job manager
- An empty PersistentVolume with access mode `ReadWriteMany` to retain logs from the MATLAB Job Scheduler workers
- A PersistentVolume with access mode `ReadOnlyMany` containing a MATLAB Parallel Server installation

Now create a *PersistentVolumeClaim* for each PersistentVolume.
You can create a PersistentVolumeClaim by using the following example configuration file.
Replace `<my-namespace>` with the namespace of the MATLAB Job Scheduler, `<pvc-name>` with the PersistentVolumeClaim name, and `<capacity>` with the amount of storage you want to provision for your PersistentVolumeClaim.
For information about the units you can use for storage capacity, see [Resource Management for Pods and Containers](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/) on the Kubernetes website.
To use a PersistentVolume, replace `<pv-name>` with the name of the PersistentVolume and `<storage-class-name>` with `""`.
To use a *StorageClass* for dynamic provisioning, replace `<pv-name>` with `""` and `<storage-class-name>` with the name of the StorageClass.
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
namespace: <my-namespace>
metadata:
  name: <pvc-name>
spec:
  volumeName: <pv-name>
  storageClassName: <storage-class-name>
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: <capacity>
```

### Build MATLAB Parallel Server Docker Image (Optional)

The MATLAB Job Scheduler pods require a MATLAB Parallel Server installation.
By default, you mount this from a PersistentVolume, as described in the previous step.
If you do not have a MATLAB Parallel Server installation to mount, you can build a Docker image containing a MATLAB Parallel Server installation instead.

Build a Docker image that contains a MATLAB Parallel Server installation.
- Specify `<release>` as a MATLAB release number with a lowercase `r`. For example, to install MATLAB R2024a, specify `<release>` as `r2024a`. The MATLAB release must be version R2024a or later.
- Specify `<other-products>` as a space-separated list of MATLAB toolboxes you want to install. The toolbox names must match the product names listed on the MathWorks&reg; product page with any spaces replaced by underscores. For example, to install Parallel Computing Toolbox and Deep Learning Toolbox, specify `<other-products>` as `Parallel_Computing_Toolbox Deep_Learning_Toolbox`. For a complete list of product names, see [MathWorks Products](https://www.mathworks.com/products.html).
- Specify `<my-tag>` as the Docker tag to use for the image.
```
docker build https://raw.githubusercontent.com/mathworks-ref-arch/matlab-dockerfile/main/Dockerfile --build-arg MATLAB_INSTALL_LOCATION=/opt/matlab --build-arg MATLAB_RELEASE=<release> --build-arg MATLAB_PRODUCT_LIST="MATLAB MATLAB_Parallel_Server <other-products>" -t <my-tag>
```

Push the image to a repository that is visible to your Kubernetes cluster.

For more information on building a MATLAB Docker image, see [Create a MATLAB Container Image](https://github.com/mathworks-ref-arch/matlab-dockerfile) in the GitHub repository.


### Create Administrator Password Secret

By default, MATLAB Job Scheduler in Kubernetes runs at security level 2.
At security level 2, jobs and tasks are associated with the submitting user and are password protected.
For details about security levels, see [MATLAB Job Scheduler Security](https://www.mathworks.com/help/matlab-parallel-server/set-matlab-job-scheduler-cluster-security.html) in the MathWorks Help Center.

When you run MATLAB Job Scheduler with security level 2, you must provide an administrator password.
Create a Kubernetes Secret for your administrator password named `mjs-admin-password` and replace `<password>` with a password of your choice.
```
kubectl create secret generic mjs-admin-password --from-literal=password=<password> --namespace mjs
```

To keep your Kubernetes Secrets secure, enable encryption at rest and restrict access to your namespace using role-based access control.
For more information, see the Kubernetes documentation for [Secrets](https://kubernetes.io/docs/concepts/configuration/secret/).

### Create Helm Values File

Create a YAML file containing configuration parameters and values for MATLAB Job Scheduler in Kubernetes.
Copy the following lines into a YAML file, `values.yaml`, and modify the values for your cluster configuration.
```yaml
matlabRelease: r2024a
maxWorkers: 100
matlabPVC: "matlab-pvc"
checkpointPVC: "checkpoint-pvc"
logPVC: "log-pvc"
workerLogPVC: "worker-log-pvc"
jobManagerUserID: 0
jobManagerGroupID: 0
matlabImage: ""
```
Modify the following values:
- `matlabRelease` &mdash; Specify the release number of the MATLAB Parallel Server installation.
- `maxWorkers` &mdash; Specify the maximum number of MATLAB Parallel Server workers to run in the cluster. The cluster starts with zero workers and automatically scales up to this number as the cluster becomes busy.
- `matlabPVC` &mdash; Specify the name of a PersistentVolumeClaim that is bound to the PersistentVolume with a MATLAB Parallel Server installation.
- `checkpointPVC` &mdash; Specify the name of a PersistentVolumeClaim that is bound to a PersistentVolume used to retain job data.
- `logPVC` &mdash; Specify the name of a PersistentVolumeClaim that is bound to a PersistentVolume used to retain job manager logs.
- `workerLogPVC` &mdash; Specify the name of a PersistentVolumeClaim that is bound to a PersistentVolume used to retain worker logs.
- `jobManagerUserID` &mdash; Specify the user ID of the user account that MATLAB Job Scheduler should use to run the job manager pod. The user must have write permission for the checkpoint and log PersistentVolumes. To find the user ID, on a Linux machine, run `id -u`.
- `jobManagerGroupID` &mdash; Specify the group ID of the user account that MATLAB Job Scheduler should use to run the job manager pod. The user must have write permission for the checkpoint and log PersistentVolumes. To find the group ID, on a Linux machine, run `id -g`.
- `matlabImage` &mdash; Specify the URI of a Docker image that contains a MATLAB Parallel Server installation. Specify a URI only if you built a Docker image instead of mounting a MATLAB Parallel Server installation from a PersistentVolume. If you specify this parameter, set the `matlabPVC` parameter to an empty string (`""`).

For a full list of the configurable Helm values that you can set in this file, see the [Helm Values](helm_values.md) page.

### Install Helm Chart

Install the MATLAB Job Scheduler Helm chart with your custom values file:
```
helm install mjs oci://ghcr.io/mathworks-ref-arch/matlab-parallel-server-k8s/mjs --values values.yaml --namespace mjs
```

Check the status of the MATLAB Job Scheduler pods:
```
kubectl get pods --namespace mjs
```
When all pods display `1/1` in the `READY` field, MATLAB Job Scheduler is ready to use.
The output of the `kubectl get pods` command looks something like this when MATLAB Job Scheduler is ready:
```
NAME                                 READY   STATUS    RESTARTS   AGE
mjs-controller-7884c9d95d-5wq2g      1/1     Running   0          25s
mjs-job-manager-5576468456-q5klv     1/1     Running   0          22s
mjs-ingress-proxy-56787694fd-ssbd4   1/1     Running   0          25s
```

The Helm chart automatically creates a Kubernetes load balancer service for you.
Check the status of the service:
```
kubectl get services -l app=mjs-ingress-proxy --namespace mjs
```
The output of the `kubectl get services` command looks something like this when the load balancer service is ready:
```
NAME                         TYPE           CLUSTER-IP     EXTERNAL-IP     PORT
mjs-ingress-proxy-ed5e5db8   LoadBalancer   10.233.12.53   192.168.1.200   27356:31387/TCP,27359:31664/TCP,30000:32212/TCP
```

Configure your firewall so that MATLAB clients can route to the IP address or hostname under the `EXTERNAL-IP` column through the ports this service exposes.
For a description of the ports the load balancer service exposes, see the [Customize Load Balancer](#customize-load-balancer) section.

If you want the MATLAB client to route to this load balancer through a different hostname, for example, an intermediate server or a DNS entry, set the value of the `clusterHost` parameter in your Helm values file before you install MATLAB Job Scheduler on your Kubernetes cluster.

## Download Cluster Profile

The cluster profile is a JSON-format file that allows a MATLAB client to connect to your MATLAB Job Scheduler cluster.

Download the cluster profile to a `profile.json` file:
```
kubectl get secrets mjs-cluster-profile --template="{{.data.profile | base64decode}}" --namespace mjs > profile.json
```

Share the cluster profile with MATLAB users that want to connect to the cluster.

By default, connections between MATLAB clients and MATLAB Job Scheduler in Kubernetes are verified using mutual TLS (mTLS).
The MATLAB client must have a cluster profile with the correct certificate to connect to the cluster.
You must store the cluster profile securely and distribute the cluster profile to trusted users through a secure channel.

## Connect to MATLAB Job Scheduler in Kubernetes

To connect to MATLAB Job Scheduler and run MATLAB Parallel Server jobs, open MATLAB using the same version you used for MATLAB Job Scheduler.

Import the cluster profile.
1. On your MATLAB desktop, select **Parallel > Create and Manage Clusters**.
2. Click **Import** in the toolbar.
3. Navigate to the location where you saved the profile you created in the previous step and select it.

### Validate Cluster

Cluster validation submits a job of each type to test whether the cluster profile is configured correctly.
In the Cluster Profile Manager, click **Validate**.
If you make a change to the cluster configuration, run cluster validation again to ensure your changes cause no errors.
You do not need to validate the profile each time you use it or each time you start MATLAB.

## Uninstall MATLAB Job Scheduler

To uninstall MATLAB Job Scheduler from your Kubernetes cluster, run this command:
```
helm uninstall mjs --namespace mjs
```

Delete the administrator password secret:
```
kubectl delete secrets mjs-admin-password --namespace mjs
```

If you created a custom load balancer service, delete the service:
```
kubectl delete service mjs-ingress-proxy --namespace mjs
```

## Examples

Create a cluster object using your cluster profile `<name>`:
```matlab
c = parcluster("<name>")
```

### Submit Work for Batch Processing

The `batch` command runs a MATLAB script or function on a worker on the cluster.
For more information about batch processing, see the MathWorks documentation for [`batch`](https://www.mathworks.com/help/parallel-computing/batch.html).

```matlab
% Create a job and submit it to the cluster
job = batch( ...
    c, ... % Cluster object created using parcluster
    @sqrt, ... % Function or script to run
    1, ... % Number of output arguments
    {[64 100]}); % Input arguments

% Your MATLAB session is now available to do other work. You can
% continue to create and submit more jobs to the cluster. You can also
% shut down your MATLAB session and come back later. The work
% continues to run on the cluster. After you recreate
% the cluster object using the parcluster function, you can view existing
% jobs using the Jobs property of the cluster object.

% Wait for the job to complete. If the job is already complete,
% MATLAB does not block the Command Window and this command
% returns the prompt (>>) immediately.
wait(job);

% Retrieve the output arguments for each task. For this example,
% the output is a 1-by-1 cell array containing the vector [8 10].
results = fetchOutputs(job)
```

### Submit Work for Batch Processing with a Parallel Pool

You can use the `batch` command to create a parallel pool by using the `'Pool'` name-value argument.

```matlab
% Create and submit a batch pool job to the cluster
job = batch(
    c, ... % Cluster object created using parcluster
    @sqrt, ... % Function/script to run
    1, ... % Number of output arguments
    {[64 100]}, ... % Input arguments
    'Pool', 3); ... % Use a parallel pool with three workers
```

### Open an Interactive Parallel Pool

A parallel pool is a group of MATLAB workers on which you can interactively run work.
When you run the `parpool` command, MATLAB submits a special job to the cluster to start the workers.
Once the workers start, your MATLAB session connects to them.
For more information about parallel pools, see the MathWorks documentation for [`parpool`](https://www.mathworks.com/help/parallel-computing/parpool.html).

```matlab
% Open a parallel pool on the cluster. This command
% returns the prompt (>>) when the pool is ready.
pool = parpool(c);

% List the hosts on which the workers are running.
future = parfevalOnAll(pool, @getenv, 1, 'HOSTNAME')
wait(future);
fetchOutputs(future)

% Output the numbers 1 to 10 in a parallel for-loop.
% Unlike a regular for-loop, the software does not
% execute iterations of the loop in order.
parfor idx = 1:10
    disp(idx)
end

% Use the pool to calculate the first 500 magic squares.
parfor idx = 1:500
    magicSquare{idx} = magic(idx);
end
```

## Advanced Setup Steps

### Customize Load Balancer

MATLAB Job Scheduler in Kubernetes uses a Kubernetes load balancer service to expose MATLAB Job Scheduler to MATLAB clients running outside of the Kubernetes cluster.
By default, the Helm chart creates the load balancer for you.
You can also create and customize your own load balancer service before you install the Helm chart.

Create a Kubernetes load balancer service `mjs-ingress-proxy` to expose MATLAB Job Scheduler to MATLAB clients running outside of the Kubernetes cluster.
This service needs to open the following ports:
- `basePort + 6` and `basePort + 9`, where `basePort` is the MATLAB Job Scheduler base port (default 27350). The MATLAB client connects to the MATLAB Job Scheduler job manager through these ports.
- All ports in range `poolProxyBasePort` to `poolProxyBasePort + maxNumPoolProxies - 1`, where `poolProxyBasePort` is the pool proxy base port (default 30000). Calculate `maxNumPoolProxies` by dividing the maximum number of workers in your cluster by the number of workers per pool proxy (default 32) and rounding up to the nearest integer. The MATLAB client connects to workers in interactive parallel pools through these ports.

For example, for a MATLAB Job Scheduler cluster with the default base port (27350), default pool proxy base port (30000) and a maximum size of 64 workers, the maximum number of pool proxies is 2.
To create a load balancer for a cluster with this port configuration, create a YAML file, `load-balancer.yaml`, and copy the following lines.
<!-- BEGIN LOAD BALANCER EXAMPLE -->
```yaml
apiVersion: v1
kind: Service
metadata:
  name: mjs-ingress-proxy
spec:
  type: LoadBalancer
  selector:
    app: mjs-ingress-proxy
  ports:
  - name: job-manager-27356
    port: 27356
    targetPort: 27356
    protocol: TCP
  - name: job-manager-27359
    port: 27359
    targetPort: 27359
    protocol: TCP
  - name: pool-proxy-30000
    port: 30000
    targetPort: 30000
    protocol: TCP
  - name: pool-proxy-30001
    port: 30001
    targetPort: 30001
    protocol: TCP
```
<!-- END LOAD BALANCER EXAMPLE -->

Modify the file to add annotations if needed.
Create the load balancer.
```
kubectl apply -f load-balancer.yaml --namespace mjs
```

Check the status of the load balancer.
```
kubectl get services -n mjs mjs-ingress-proxy
```

The output from the `kubectl get services` command looks something like this:

```
NAME                TYPE           CLUSTER-IP      EXTERNAL-IP     PORT(S)
mjs-ingress-proxy   LoadBalancer   10.233.55.51    192.168.1.200   27356:31186/TCP,27359:30272/TCP,30000:30576/TCP,30001:32290/TCP
```

You must ensure that the output of the `kubectl get services` command displays an IP address or hostname under the `EXTERNAL-IP` column before you continue.
If you do not see an external IP address, wait for some time, then run the same command again.

If you still do not see an external IP address, make sure your Kubernetes cluster is configured to create external load balancers.

If your Kubernetes cluster runs in the cloud, edit the security settings of the load balancer to apply the security rules you need.

## License

The license for the software in this repository is available in the [LICENSE.md](LICENSE.md) file.

## Community Support

[MATLAB Central](https://www.mathworks.com/matlabcentral)

## Technical Support
To request assistance or additional features, contact [MathWorks Technical Support](https://www.mathworks.com/support/contact_us.html).

---

Copyright 2024 The MathWorks, Inc.
