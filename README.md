# MATLAB Parallel Server in Kubernetes

[![View MATLAB Parallel Server in Kubernetes on File Exchange](https://www.mathworks.com/matlabcentral/images/matlab-file-exchange.svg)](https://www.mathworks.com/matlabcentral/fileexchange/167676-matlab-parallel-server-in-kubernetes)

This repository contains utilities for using MATLAB&reg; Parallel Server&trade; in a Kubernetes&reg; cluster.

## Introduction

This guide explains how to deploy MATLAB Job Scheduler onto your Kubernetes cluster.
You can then connect to the MATLAB Job Scheduler and use it to run MATLAB Parallel Server jobs on the Kubernetes cluster.

For more information on MATLAB Job Scheduler and MATLAB Parallel Server, see the MathWorks&reg; documentation on [MATLAB Parallel Server](https://www.mathworks.com/help/matlab-parallel-server/index.html).

## Requirements

To use MATLAB Job Scheduler in Kubernetes, you must have MATLAB R2024a or later.

Before you start, you need the following:
- A running Kubernetes cluster that meets the following conditions:
  - Uses Kubernetes version 1.21.1 or later.
  - Meets the system requirements for running MATLAB Job Scheduler. For details, see the MathWorks documentation for [MATLAB Parallel Server Product Requirements](https://www.mathworks.com/support/requirements/matlab-parallel-server.html).
  - Configured to create external load balancers that allow traffic into the cluster.
  - Has adequate storage on cluster nodes. When using a MATLAB Parallel Server Docker image for your workers (default behavior), ensure that each cluster node has at least 50GB of storage. If mounting MATLAB Parallel Server from a persistent volume, each cluster node must have at least 20GB of storage.
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

Create a PersistentVolume for each of the following applications:
- An empty PersistentVolume with access mode `ReadWriteOnce` for MATLAB Job Scheduler's checkpoint folder, which retains job data after exiting the session
- An empty PersistentVolume with access mode `ReadWriteOnce` to retain logs from the MATLAB Job Scheduler job manager
- An empty PersistentVolume with access mode `ReadWriteMany` to retain logs from the MATLAB Job Scheduler workers

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

### Create Administrator Password Secret

By default, MATLAB Job Scheduler in Kubernetes runs at security level 2.
At security level 2, jobs and tasks are associated with the submitting user and are password protected.
For details about security levels, see [MATLAB Job Scheduler Security](https://www.mathworks.com/help/matlab-parallel-server/set-matlab-job-scheduler-cluster-security.html) in the MathWorks Help Center.

When you run MATLAB Job Scheduler with security level 2, you must provide an administrator password.
Create a Kubernetes Secret for your administrator password named `mjs-admin-password` and replace `<password>` with a password of your choice.
If you are using LDAP to authenticate user credentials, this must be the password of the cluster administrator in the LDAP server.
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

# Licensing settings
useOnlineLicensing: true
networkLicenseManager: ""

# PersistentVolumeClaim settings
checkpointPVC: "checkpoint-pvc"
logPVC: "log-pvc"
workerLogPVC: "worker-log-pvc"

# Security settings
jobManagerUserID: 0
jobManagerGroupID: 0
```
Modify the following values:
- `matlabRelease` &mdash; Specify the release number of the MATLAB Parallel Server installation.
- `maxWorkers` &mdash; Specify the maximum number of MATLAB Parallel Server workers to run in the cluster. The cluster starts with zero workers and automatically scales up to this number as the cluster becomes busy.
- `useOnlineLicensing` &mdash; Option to use MathWorks online licensing. Set this parameter to true to use online licensing to manage licensing for your cluster users. When enabled, users must log in to their MathWorks account to connect to the cluster, and their account must be linked to a MATLAB Parallel Server license that is managed online. For more information about online licensing, see [Use Online Licensing for MATLAB Parallel Server](https://www.mathworks.com/products/matlab-parallel-server/online-licensing.html) on the MathWorks website. To learn how to set up online licensing, see the MathWorks documentation [Configure MATLAB Parallel Server Licensing for Cloud Platforms](https://www.mathworks.com/help/matlab-parallel-server/configure-matlab-parallel-server-licensing-for-cloud-platforms.html).
- `networkLicenseManager` &mdash; To use a network license manager to manage licensing for your cluster users, specify the address of your network license manager in the format `port@host`. The license manager must be accessible from the Kubernetes cluster. You can install or use an existing network license manager running on-premises or on AWS&reg;. To install a network license manager on-premises, see the MathWorks documentation [Install License Manager on License Server](https://www.mathworks.com/help/install/ug/install-license-manager-on-license-server.html). To deploy a network license manager reference architecture on AWS, select a MATLAB release from [Network License Manager for MATLAB on AWS](https://github.com/mathworks-ref-arch/license-manager-for-matlab-on-aws).
- `checkpointPVC` &mdash; Specify the name of a PersistentVolumeClaim that is bound to a PersistentVolume used to retain job data.
- `logPVC` &mdash; Specify the name of a PersistentVolumeClaim that is bound to a PersistentVolume used to retain job manager logs.
- `workerLogPVC` &mdash; Specify the name of a PersistentVolumeClaim that is bound to a PersistentVolume used to retain worker logs.
- `jobManagerUserID` &mdash; Specify the user ID of the user account that MATLAB Job Scheduler should use to run the job manager pod. The user must have write permission for the checkpoint and log PersistentVolumes. To find the user ID, on a Linux machine, run `id -u`.
- `jobManagerGroupID` &mdash; Specify the group ID of the user account that MATLAB Job Scheduler should use to run the job manager pod. The user must have write permission for the checkpoint and log PersistentVolumes. To find the group ID, on a Linux machine, run `id -g`.

For a full list of the configurable Helm values that you can set in this file, see the [Helm Values](helm_values.md) page.

To use an LDAP server to authenticate user credentials, add the following parameters to the `values.yaml` file:
```yaml
adminUser: "admin"
ldapURL: "ldaps://HOST:PORT"
ldapSecurityPrincipalFormat: "[username]@domain.com"
```
Modify the following values:
- `adminUser` &mdash; Specify the username of a valid user in the LDAP server. The secret you created in the [Create Administrator Password Secret](#create-administrator-password-secret) must contain this user's password.
- `ldapURL` &mdash; Specify the URL of the LDAP server as `ldaps://HOST:PORT`. If you have not configured your LDAP server over SSL, specify the URL as `ldap://HOST:PORT`.
- `ldapSecurityPrincipalFormat` &mdash; Specify the format of a security principal (user) for your LDAP server.

**Security Considerations:** Use LDAP over SSL (LDAPS) to encrypt communication between the LDAP server and clients. For additional LDAPS configuration steps, see [Configure LDAP over SSL](#configure-ldap-over-ssl).

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

Configure your firewall so that MATLAB clients can connect to the IP address or hostname under the `EXTERNAL-IP` column through the ports this service exposes.
For a description of the ports the load balancer service exposes, see the [Customize Load Balancer](#customize-load-balancer) section.

If you want the MATLAB client to connect to this load balancer through a different hostname, for example, an intermediate server or a DNS entry, set the value of the `clusterHost` parameter in your Helm values file before you install MATLAB Job Scheduler on your Kubernetes cluster.

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

## Validate Cluster

Cluster validation submits a job of each type to test whether the cluster profile is configured correctly.
In the Cluster Profile Manager, click **Validate**.
If you make a change to the cluster configuration, run cluster validation again to ensure your changes cause no errors.
You do not need to validate the profile each time you use it or each time you start MATLAB.

### Troubleshoot Cluster Validation Failures

The following sections explain how to resolve some common cluster validation failures.

#### Cluster Connection Test Failure

Incorrect cluster profiles or networking issues can cause failures during the "Cluster connection test (parcluster)" cluster validation stage.

If you have uninstalled and reinstalled the MATLAB Job Scheduler Helm chart, make sure you download and import the new cluster profile following the instructions in [Download Cluster Profile](#download-cluster-profile).
Using a cluster profile from a previous deployment in the same Kubernetes cluster results in cluster validation errors.

You must ensure that your MATLAB client can connect to the IP address of the load balancer and that your firewall allows traffic to the MATLAB Job Scheduler ports.
To check the load balancer's IP address, see [Install Helm Chart](#install-helm-chart).
For a description of the MATLAB Job Scheduler ports, see [Customize Load Balancer](#customize-load-balancer).

#### License Checkout Failure

If you have not correctly configured the MATLAB Parallel Server license for your cluster, the "Job test (createJob)" cluster validation stage fails with this message:
```
License checkout failed
```
Make sure you have set either the `useOnlineLicensing` parameter or the `networkLicenseManager` parameter in your `values.yaml` file. 
To learn more about the `useOnlineLicensing` and `networkLicenseManager` parameters, see [Create Helm Values File](#create-helm-values-file).

If you continue to experience licensing errors, contact [MathWorks Technical Support](https://www.mathworks.com/support/contact_us.html).

#### Job Test Unresponsive

If the "Job test (createJob)", "SPMD job test (createCommunicatingJob)" or "Pool job test (createCommunicatingJob)" stage takes a very long time to run (> 5 minutes), your Kubernetes cluster might not have sufficient resources to start the worker pods.

Check the status of the worker pods while cluster validation is in progress by running
```
kubectl get pods --label app=mjs-worker --namespace mjs
```

If a worker pod has the `Pending`, `ContainerCreating` or `ContainerStatusUnknown` status, check the pod's details by running
```
kubectl describe pods --namespace mjs <pod-name>
```
Replace `<pod-name>` with the name of the worker pod.

If your Kubernetes cluster does not have enough CPU resources to run the pod, the output might include messages like:
```
Events:
  Type     Reason            Age   From               Message
  ----     ------            ----  ----               -------
  Warning  FailedScheduling  7s    default-scheduler  0/2 nodes are available: 2 Insufficient cpu.
```

If your Kubernetes cluster does not have enough memory resources to run the pod, the output might include messages like:
```
Events:
  Type     Reason            Age   From               Message
  ----     ------            ----  ----               -------
  Warning  FailedScheduling  43s   default-scheduler  0/2 nodes are available: 2 Insufficient memory.
```

If you see either output, your Kubernetes cluster does not have enough resources to run the number of workers you specified in the `maxWorkers` parameter in your `values.yaml` file.
For details on the resource requirements for MATLAB Parallel Server workers, see the MathWorks documentation for [MATLAB Parallel Server Product Requirements](https://www.mathworks.com/support/requirements/matlab-parallel-server.html).
By default, each worker pod requests 2 vCPU and 8GB of memory. If your cluster does not have enough resources, either
- Add more nodes to your Kubernetes cluster or replace your existing nodes with nodes that have more CPU and memory resources.
- Modify your `values.yaml` file to decrease the value of the `maxWorkers` parameter.
- Modify your `values.yaml` file to decrease the values of the `workerMemoryRequest` and `workerMemoryLimit` parameters. A minimum of 4GB per MATLAB worker is recommended. If you are using Simulink, a minimum of 8GB per worker is recommended.

If you modified your `values.yaml` file, uninstall the MATLAB Job Scheduler Helm chart following the instructions in [Uninstall MATLAB Job Scheduler](#uninstall-matlab-job-scheduler), then reinstall the Helm chart following the instructions in [Install Helm chart](#install-helm-chart).

If your Kubernetes cluster nodes do not have enough ephemeral storage to pull the MATLAB Parallel Server Docker image, the output of `kubectl describe pods` might include messages like:
```
Events:
  Type     Reason               Age    From               Message
  ----     ------               ----   ----               -------
  Normal   Scheduled            4m49s  default-scheduler  Successfully assigned default/mjs-worker-1-fd697549aeca4c2ab0f1bcb4fe819b0f-5d78457d5c-lcpv5 to my-node
  Normal   Pulling              4m49s  kubelet            Pulling image "ghcr.io/mathworks-ref-arch/matlab-parallel-server-k8s/mjs-worker-image:r2024a"
  Warning  Evicted              88s    kubelet            The node was low on resource: ephemeral-storage. Threshold quantity: 3219965180, available: 1313816Ki.
```

For details on the node storage requirements, see [Requirements](#requirements).
If your nodes do not have enough ephemeral storage, either
- Replace your Kubernetes nodes with nodes that have more storage.
- Instead of pulling the MATLAB Parallel Server Docker image, mount MATLAB Parallel Server from a PersistentVolume. To learn more, see [Mount MATLAB from a PersistentVolume](#mount-matlab-from-a-persistentvolume).

## Uninstall MATLAB Job Scheduler

To uninstall the MATLAB Job Scheduler Helm chart from your Kubernetes cluster, run this command:
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

If you want to reinstall the MATLAB Job Scheduler Helm chart, you must ensure that the load balancer service is deleted first.
To check the status of the load balancer service, run:
```
kubectl get service mjs-ingress-proxy --namespace mjs
```
If the load balancer service appears, wait for some time, then run the command again to confirm that the load balancer service is not found before proceeding with the MATLAB Job Scheduler Helm chart reinstallation.

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

### Customize Worker Image

The MATLAB Parallel Server workers run on an image that contains MATLAB, Simulink, all MathWorks toolboxes, and the Deep Learning Support Packages by default.
If you want to increase the performance of creating worker pods or customise the toolboxes or support packages used, you have two options:
1. Build a custom Docker image with only the toolboxes you need
2. Mount the MATLAB installation from a PersistentVolume

#### Build Custom Docker Image

To build a custom Docker image, see [Create a MATLAB Parallel Server Container Image](images/worker/README.md).
Push the image to a repository that is visible to your Kubernetes cluster.

Modify your `values.yaml` file to set the `workerImage` and `workerImageTag` parameters to the URI and tag of your image before installating the Helm chart.

#### Mount MATLAB from a PersistentVolume

To mount MATLAB from a PersistentVolume, create a PersistentVolume and PersistentVolumeClaim with access mode `ReadOnlyMany` containing a MATLAB Parallel Server installation.
For example, if your Kubernetes cluster runs on-premise, you could create a PersistentVolume from an NFS server containing the MATLAB Parallel Server installation.
For details on creating the PersistentVolumeClaim, see the [Create Persistent Volumes](#create-persistent-volumes) section.

Modify your `values.yaml` file to set the `matlabPVC` parameter to the name of your PersistentVolumeClaim before installating the Helm chart.
The worker pods will now use the image URI specified in the `matlabDepsImage` parameter instead of the `workerImage` parameter.

### Run Multiple MATLAB Parallel Server Versions

You can use multiple versions of MATLAB Parallel Server in a single MATLAB Job Scheduler cluster.
When you upgrade to a newer release of MATLAB Parallel Server on your cluster, users can continue to submit jobs from both newer and older releases of the MATLAB client.
The additional MATLAB Parallel Server versions you use must be version R2024a or newer and must be older than the version of MATLAB Job Scheduler you are using.

Create a PersistentVolume and PersistentVolumeClaim for each additional MATLAB Parallel Server installation you want to use.
The root directory of each PersistentVolume must be the MATLAB root folder.
Modify your `values.yaml` file to set the `additionalMatlabPVCs` parameter to the names of the PersistentVolumeClaims.

For example, to use an additional PersistentVolumeClaim `matlab-r2024a-pvc`, add the following line to your `values.yaml` file:
```
additionalMatlabPVCs:
- matlab-r2024a-pvc
```

### Configure LDAP over SSL

When you use an LDAP server configured over SSL, you must add the LDAPS SSL certificate to your Kubernetes cluster.
To obtain the SSL server certificate, follow the instructions in [Connect to LDAP Server to Get Server SSL Certificate](https://www.mathworks.com/help/matlab-parallel-server/configure-ldap-server-authentication-for-matlab-job-scheduler.html#mw_fe8d0f90-2854-42b9-9e04-a2f25a295e61) on the MathWorks website.

If you use a prebuilt job manager image (default behavior), create a Kubernetes secret containing the server SSL certificate.
Replace `<path>` with the path to your server SSL certificate.
```
kubectl create secret generic mjs-ldap-secret --namespace mjs --from-file=cert.pem=<path>
```

If you use a persistent volume for the job manager pod (`matlabPVC` is set to a non-empty string and `jobManagerUsesPVC` is set to `true` in your `values.yaml` file), you must add your certificate to the Java trust store of the MATLAB Parallel Server installation in your persistent volume. For detailed instructions, see [Add Certificate to Java Trust Store](https://mathworks.com/help/matlab-parallel-server/configure-ldap-server-authentication-for-matlab-job-scheduler.html#mw_fe8d0f90-2854-42b9-9e04-a2f25a295e61) on the MathWorks website.

### Configure Cluster Monitoring Metrics

You can configure MATLAB Job Scheduler to export cluster monitoring metrics.
This feature is supported for MATLAB Job Scheduler release R2024b or later.
To export cluster monitoring metrics for MATLAB Job Scheduler releases older than R2024b, set the `jobManagerImageTag` parameter in your Helm values file to `r2024b` to use a newer release for the job manager.

To enable cluster monitoring metrics, set the following values in your `values.yaml` file:
```yaml
exportMetrics: true
metricsPort: 8001
useSecureMetrics: true
openMetricsPortOutsideKubernetes: false
```
Modify the following values:
- `metricsPort` &mdash; Specify the port for exporting metrics on the HTTP(S) server.
- `useSecureMetrics` &mdash; Set this to true to export metrics over an encrypted HTTPS connection. Set to false to disable encryption and export metrics on an HTTP server.
- `openMetricsPortOutsideKubernetes` &mdash; Set this to true to expose the metrics endpoint outside of the Kubernetes cluster. Set to false if you only want to scrape metrics from another pod inside the Kubernetes cluster.

If you set `useSecureMetrics` to true, by default the Helm chart generates SSL certificates for you.

Optionally, you can provide your own SSL certificates that the job manager uses to encrypt metrics.
The server SSL certificate must include a Subject Alternative Name (SAN) that corresponds to a DNS name or domain directed at the job manager.
- If you set `openMetricsPortOutsideKubernetes` to true, use the domain associated with the load balancer addresses generated within your Kubernetes cluster, or configure a static DNS name that routes to your load balancer after installing the MJS Helm Chart.
- If you set `openMetricsPortOutsideKubernetes` to false, the DNS name of the job manager is `mjs-job-manager.mjs.svc.cluster.local`.

To use your own SSL certificates, create a Kubernetes secret.
Using the `kubectl` command, specify the paths to the CA certificate used to sign your client certificate, the certificate to use for the server, and the private key to use for the server. For example, use the CA certificate `ca_cert`, server certificate `server_cert`, and server private key `server_key`:
```
kubectl create secret generic mjs-metrics-secret --from-file=ca.crt=ca_cert --from-file=jobmanager.crt=server_cert --from-file=jobmanager.key=server_key --namespace mjs
```

Install the MATLAB Job Scheduler Helm chart.

#### Integrate with Grafana and Prometheus

To integrate your cluster with Grafana&reg; and Prometheus&reg;, follow the instructions in the [Cluster Monitoring Integration for MATLAB Job Scheduler](https://github.com/mathworks/cluster-monitoring-integration-for-matlab-job-scheduler) GitHub repository.

Configure Prometheus to target the metrics endpoint `job-manager-host:metricsPort`, where `metricsPort` is the value you set in your `values.yaml` file.
- If you set `openMetricsPortOutsideKubernetes` to true, `job-manager-host` is the external IP address or DNS name of your load balancer service. Find this by running `kubectl get services -n mjs mjs-ingress-proxy`.
- If you set `openMetricsPortOutsideKubernetes` to false, Prometheus must run inside the same Kubernetes cluster as MATLAB Job Scheduler. Set `job-manager-host` to `mjs-job-manager.mjs.svc.cluster.local`.

If you set `useSecureMetrics` to true, configure Prometheus with certificates to authenticate with the metrics server.
- If you provided your own SSL certificates, use client certificates corresponding to the certificates you used to set up the metrics server.
- If the Helm chart generated SSL certificates for you, download and use the generated client certificates from the Kubernetes secret `mjs-metrics-client-certs`:
```
kubectl get secrets mjs-metrics-client-certs --template="{{.data.ca.crt | base64decode}}" --namespace mjs > ca.crt
kubectl get secrets mjs-metrics-client-certs --template="{{.data.prometheus.crt | base64decode}}" --namespace mjs > prometheus.crt
kubectl get secrets mjs-metrics-client-certs --template="{{.data.prometheus.key | base64decode}}" --namespace mjs > prometheus.key
```

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
