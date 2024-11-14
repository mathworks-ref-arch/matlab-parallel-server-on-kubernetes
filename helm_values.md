# Helm Values for MATLAB Parallel Server in Kubernetes

The following table lists the configurable Helm values that you can set in the YAML file you use to configure MATLAB Parallel Server in Kubernetes.
If you do not include a parameter in your YAML file, your configuration uses the default value.

<!-- BEGIN HELM VALUES TABLE -->
**Parameter**                         | **Description** | **Default Value**
--------------------------------------|-----------------|-------------------
`additionalMatlabPVCs`                | Names of PersistentVolumeClaims containing installations of older MATLAB Parallel Server releases, specified as an array. | `[]`
`adminUser`                           | Username of the cluster administrator. | `admin`
`autoCreateLoadBalancer`              | Flag to automatically create a Kubernetes load balancer to expose MATLAB Job Scheduler to MATLAB clients outside the cluster. See the [Customize Load Balancer](README.md#customize-load-balancer) section for instructions to create your own load balancer. | `true`
`autoScalingPeriod`                   | Period in seconds with which the controller checks the cluster's size requirements and automatically scales the number of workers up and down if needed. | `15`
`basePort`                            | Base port of the MATLAB Job Scheduler service. | `27350`
`checkpointPVC`                       | Name of the PersistentVolumeClaim that is bound to the PersistentVolume used to retain job data. | &mdash;
`clusterHost`                         | Custom host to use in the cluster profile. If unset, the cluster profile uses the external address of the load balancer. | &mdash;
`controllerImage`                     | URI of the image to use for the MATLAB Job Scheduler controller, a pod that creates the job manager and automatically scales the number workers up and down. Set this value if you want to use a privately hosted version of this image rather than the version hosted on the GitHub Container registry. | `ghcr.io/mathworks-ref-arch/matlab-parallel-server-k8s/mjs-controller-image`
`controllerImagePullPolicy`           | Pull policy for the MATLAB Job Scheduler controller. | `IfNotPresent`
`controllerImageTag`                  | Tag of the image to use for the MATLAB Job Scheduler controller. If you do not set this value, the Helm chart uses the `appVersion` defined in `Chart.yaml` as the tag. | &mdash;
`exportMetrics`                       | Flag to export cluster monitoring metrics from the job manager. | `false`
`haproxyImage`                        | URI of the [HAproxy Docker image](https://hub.docker.com/_/haproxy/), which is used to proxy incoming traffic. Set this value if you want to use a privately hosted version of this image. | `haproxy`
`haproxyImagePullPolicy`              | Pull policy for the HAproxy image. | `IfNotPresent`
`idleStop`                            | Time in seconds after which idle worker pods are removed. | `300`
`internalClientsOnly`                 | Flag to allow only MATLAB clients running inside the Kubernetes cluster to connect to the MATLAB Job Scheduler. | `false`
`jobManagerCPULimit`                  | CPU limit for the job manager pod. | &mdash;
`jobManagerCPURequest`                | CPU request for the job manager pod. | `1`
`jobManagerGroupID`                   | Group ID of the user account that MATLAB Job Scheduler uses to run the job manager pod. The user must have write permission for the checkpoint and log PersistentVolumes. To find the group ID, on a Linux machine, run the command `id -g` in the terminal. | `0`
`jobManagerImage`                     | URI of the MATLAB Parallel Server image to use for the MATLAB Job Scheduler job manager. | `ghcr.io/mathworks-ref-arch/matlab-parallel-server-k8s/mjs-job-manager-image`
`jobManagerImagePullPolicy`           | Pull policy for the job manager image. | `IfNotPresent`
`jobManagerImageTag`                  | Tag of the image to use for the job manager image. If you do not set this value, the Helm chart uses the `matlabRelease` parameter as the tag. | &mdash;
`jobManagerMemoryLimit`               | Memory limit for the job manager pod. | &mdash;
`jobManagerMemoryRequest`             | Memory request for the job manager pod. | `4Gi`
`jobManagerName`                      | Name of the MATLAB Job Scheduler job manager. | `MJS_Kubernetes`
`jobManagerNodeSelector`              | Node selector for the job manager pod, specified as key-value pairs that match the labels of the Kubernetes nodes you want to run the job manager on. For example, to run the job manager on nodes with label `node-type=jobmanager`, set this parameter to `{"node-type":"jobmanager"}`. You must assign the appropriate labels to your nodes before you can use the `nodeSelector` feature. For more information, see [Assigning Pods to Nodes](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node) on the Kubernetes website. | `{}`
`jobManagerUserID`                    | User ID of the user account that MATLAB Job Scheduler uses to run the job manager pod. The user must have write permission for the checkpoint and log PersistentVolumes. To find the user ID, on a Linux machine, run `id -u` in the terminal. | `0`
`jobManagerUsesPVC`                   | Flag to mount a MATLAB Parallel Server installation from a PersistentVolume onto the job manager pod if the `matlabPVC` parameter is set. If this flag is set to true, the job manager pod uses the image specified in the `matlabDepsImage` parameter. | `false`
`ldapSecurityPrincipalFormat`         | Format of a security principal (user) for your LDAP server. | &mdash;
`ldapSynchronizationIntervalSecs`     | Frequency at which the cluster synchronizes with the LDAP server. | `1800`
`ldapURL`                             | URL of an LDAP server to authenticate user credentials. If you are using LDAP over SSL (LDAPS) to encrypt communication between the LDAP server and clients, follow the instructions to [configure LDAP over SSL](README.md#configure-ldap-over-ssl). | &mdash;
`logLevel`                            | Verbosity level of MATLAB Job Scheduler logging. | `0`
`logPVC`                              | Name of the PersistentVolumeClaim that is bound to the PersistentVolume used to retain job manager logs. | &mdash;
`matlabDepsImage`                     | URI of the MATLAB dependencies image to use for the MATLAB Job Scheduler worker pods if the MATLAB Parallel Server installation is mounted from a PersistentVolume. The worker pods only use this image if the `matlabPVC` parameter is set. The worker pods use the `matlabImageTag` parameter as the image tag. | `mathworks/matlab-deps`
`matlabImage`                         | URI of the image to use for the MATLAB Job Scheduler workers. This image should contain a MATLAB Parallel Server installation, plus any MathWorks toolboxes you want to use in your MATLAB Parallel Server jobs. | `ghcr.io/mathworks-ref-arch/matlab-parallel-server-k8s/mjs-worker-image`
`matlabImagePullPolicy`               | Pull policy for the worker image. | `IfNotPresent`
`matlabImageTag`                      | Tag of the image to use for the worker image. If you do not set this value, the Helm chart uses the `matlabRelease` parameter as the tag. | &mdash;
`matlabPVC`                           | Name of the PersistentVolumeClaim that is bound to the PersistentVolume with a MATLAB Parallel Server installation. Set this option only if you did not build a Docker image containing a MATLAB Parallel Server installation. | &mdash;
`matlabRelease`                       | Release number of the MATLAB version to use. | `r2024a`
`maxWorkers`                          | Maximum number of workers that the cluster can automatically resize to. | &mdash;
`metricsPort`                         | Port for exporting cluster monitoring metrics. The job manager uses an HTTP(S) server at the port you specify to export metrics. | `8001`
`minWorkers`                          | Minimum number of workers to run in the cluster. | `0`
`networkLicenseManager`               | Address of a network license manager with format `port@host`. | &mdash;
`openMetricsPortOutsideKubernetes`    | Flag to expose the cluster monitoring metrics server outside of the Kubernetes cluster. | `false`
`poolProxyBasePort`                   | Base port for the parallel pool proxy pods. | `30000`
`poolProxyCPULimit`                   | CPU limit for each parallel pool proxy pod. | &mdash;
`poolProxyCPURequest`                 | CPU request for each parallel pool proxy pod. | `0.5`
`poolProxyImage`                      | URI of the image to use for pods that proxy connections in interactive parallel pools. Set this value if you want to use a privately hosted version of this image rather than the version hosted by MathWorks. | `containers.mathworks.com/matlab-parallel-server-k8s/parallel-server-proxy-image`
`poolProxyImagePullPolicy`            | Pull policy for the pool proxy image. | `IfNotPresent`
`poolProxyImageTag`                   | Tag of the image to use for the pool proxy. If you do not set this value, the Helm chart uses the `matlabRelease` parameter as the tag. | &mdash;
`poolProxyMemoryLimit`                | Memory limit for each parallel pool proxy pod. | &mdash;
`poolProxyMemoryRequest`              | Memory request for each parallel pool proxy pod. | `500Mi`
`requireClientCertificate`            | Flag that requires MATLAB clients to have a certificate to connect to the job manager. | `true`
`requireScriptVerification`           | Flag that requires verification to run privileged commands on the cluster. | `true`
`securityLevel`                       | MATLAB Job Scheduler security level. | `2`
`stopWorkerGracePeriod`               | Grace period in seconds for stopping worker pods. | `60`
`useOnlineLicensing`                  | Flag to use online licensing. | `false`
`useSecureCommunication`              | Flag to use secure communication between job manager and workers. | `true`
`useSecureMetrics`                    | Flag to export cluster monitoring metrics on an encrypted HTTPS server.  | `true`
`workerCPULimit`                      | CPU limit for each worker pod. | &mdash;
`workerCPURequest`                    | CPU request for each worker pod. | `2`
`workerLogPVC`                        | Name of the PersistentVolumeClaim that is bound to the PersistentVolume used to retain worker logs. | &mdash;
`workerMemoryLimit`                   | Memory limit for each worker pod. | `8Gi`
`workerMemoryRequest`                 | Memory request for each worker pod. | `8Gi`
`workerNodeSelector`                  | Node selector for the worker pods, specified as key-value pairs that match the labels of the Kubernetes nodes you want to run the workers on. For example, to run the workers on nodes with label `node-type=worker`, set this parameter to `{"node-type":"worker"}`. You must assign the appropriate labels to your nodes before you can use the `nodeSelector` feature. For more information, see [Assigning Pods to Nodes](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node) on the Kubernetes website. | `{}`
`workerPassword`                      | Password of the username that MATLAB Parallel Server uses to run jobs. | `matlab`
`workerUsername`                      | Username that MATLAB Parallel Server uses to run jobs. | `matlab`
`workersPerPoolProxy`                 | Maximum number of workers using each parallel pool proxy. | `32`
<!-- END HELM VALUES TABLE -->
