// Package specs contains functions for creating Kubernetes resource specs
// Copyright 2024-2025 The MathWorks, Inc.
package specs

import (
	"controller/internal/config"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// SpecFactory generates Kubernetes resource specs
type SpecFactory struct {
	config                *config.Config
	ownerRefs             []metav1.OwnerReference
	workerTolerations     []corev1.Toleration
	jobManagerTolerations []corev1.Toleration
}

// Volume names
const (
	matlabVolumeName      = "matlab-volume"
	logVolumeName         = "log-volume"
	secretVolumeName      = "secret-volume"
	mjsDefVolumeName      = "mjs-volume"
	checkpointVolumeName  = "checkpoint-volume"
	proxyCertVolumeName   = "proxy-cert-volume"
	ldapCertVolumeName    = "ldap-cert-volume"
	metricsCertVolumeName = "metrics-cert-volume"
)

// Secret names
const (
	SharedSecretName        = "mjs-shared-secret"
	AdminPasswordSecretName = "mjs-admin-password"
	AdminPasswordKey        = "password"
	LDAPSecretName          = "mjs-ldap-secret"
	MetricsSecretName       = "mjs-metrics-secret"
)

// File names
const (
	ProxyCertFileName   = "certificate.json"
	proxyCertDir        = "/proxy-cert"
	additionalMatlabDir = "/opt/additionalmatlab"
	MetricsCAFileName   = "ca.crt"
	MetricsCertFileName = "jobmanager.crt"
	MetricsKeyFileName  = "jobmanager.key"
)

// NewSpecFactory constructs a SpecFactory
func NewSpecFactory(conf *config.Config, ownerUID types.UID) (*SpecFactory, error) {
	// Store owner reference for all created resources
	ownerRefs := []metav1.OwnerReference{}
	if !conf.LocalDebugMode {
		ownerRefs = []metav1.OwnerReference{
			{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       conf.DeploymentName,
				UID:        ownerUID,
			},
		}
	}

	// Parse tolerations from strings
	jobManagerTolerations, err := parseTolerations(conf.JobManagerTolerations)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job manager tolerations '%s': %v", conf.JobManagerTolerations, err)
	}
	workerTolerations, err := parseTolerations(conf.WorkerTolerations)
	if err != nil {
		return nil, fmt.Errorf("failed to parse worker tolerations '%s': %v", conf.WorkerTolerations, err)
	}

	return &SpecFactory{
		config:                conf,
		ownerRefs:             ownerRefs,
		jobManagerTolerations: jobManagerTolerations,
		workerTolerations:     workerTolerations,
	}, nil
}

// GetWorkerDeploymentSpec creates a spec for a deployment running an MJS worker pod
func (s *SpecFactory) GetWorkerDeploymentSpec(w *WorkerInfo) *appsv1.Deployment {
	// Create container to run a worker
	workerScript := filepath.Join(s.config.MJSDefDir, "worker.sh")
	container := corev1.Container{
		Name:            "mjs-worker",
		Image:           s.config.WorkerImage,
		ImagePullPolicy: corev1.PullPolicy(s.config.WorkerImagePullPolicy),
		Command:         []string{"/bin/sh"},
		Args:            []string{workerScript},
	}
	addResourceRequests(&container, s.config.WorkerCPURequest, s.config.WorkerMemoryRequest)
	addResourceLimits(&container, s.config.WorkerCPULimit, s.config.WorkerMemoryLimit)

	// Add environment variables
	minMPIPort, maxMPIPort := s.getMPIPorts()
	workerEnv := map[string]string{
		"MJS_WORKER_USERNAME":             s.config.WorkerUsername,
		"MJS_WORKER_PASSWORD":             s.config.WorkerPassword,
		"USER":                            s.config.WorkerUsername,
		"MLM_LICENSE_FILE":                s.config.NetworkLicenseManager,
		"SHELL":                           "/bin/sh",
		"WORKER_NAME":                     w.Name,
		"HOSTNAME":                        w.HostName,
		"MDCE_OVERRIDE_INTERNAL_HOSTNAME": w.HostName,
		"MPICH_PORT_RANGE":                fmt.Sprintf("%d:%d", minMPIPort, maxMPIPort),
	}
	proxyID := s.CalculatePoolProxyForWorker(w.ID)
	proxy := NewPoolProxyInfo(proxyID, s.config.PoolProxyBasePort)
	if s.config.UsePoolProxy() {
		workerEnv["PARALLEL_SERVER_POOL_PROXY_HOST"] = proxy.Name
		workerEnv["PARALLEL_SERVER_POOL_PROXY_PORT"] = fmt.Sprintf("%d", proxy.Port)
		workerEnv["PARALLEL_SERVER_POOL_PROXY_EXTERNAL_HOST"] = "$CLIENT_OVERRIDE"
		if s.config.UseSecureCommunication {
			workerEnv["PARALLEL_SERVER_POOL_PROXY_CERTIFICATE"] = filepath.Join(proxyCertDir, ProxyCertFileName)
		}
	} else {
		workerEnv["MDCE_OVERRIDE_EXTERNAL_HOSTNAME"] = s.GetServiceHostname(w.HostName) // Hostname for clients inside the K8s cluster
	}

	// Add custom worker environment variables
	for key, val := range s.config.ExtraWorkerEnvironment {
		workerEnv[key] = val
	}

	addEnv(&container, workerEnv)

	// Add pre-stop hook to stop the worker cleanly
	stopWorkerPath := filepath.Join(s.config.MJSDefDir, "stopWorker.sh")
	container.Lifecycle = &corev1.Lifecycle{
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/bash", stopWorkerPath},
			},
		},
	}

	// Create pod spec
	var rootUserID int64 = 0 //nolint
	pod := corev1.PodSpec{
		Containers:                    []corev1.Container{container},
		TerminationGracePeriodSeconds: &s.config.StopWorkerGracePeriod,
		Hostname:                      w.HostName,

		// The MJS process must run as root in order to start MATLAB processes as another user
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: &rootUserID,
		},
	}
	s.setEnableServiceLinks(&pod)
	pod.NodeSelector = s.config.WorkerNodeSelector
	pod.Tolerations = s.workerTolerations

	// Add volumes
	addVolumeFromConfigMap(&pod, s.config.MJSDefConfigMap, mjsDefVolumeName, s.config.MJSDefDir)
	if s.config.MatlabPVC != "" {
		addVolumeFromPVC(&pod, s.config.MatlabPVC, matlabVolumeName, s.config.MatlabRoot, true)
	}
	if s.config.WorkerLogPVC != "" {
		addVolumeFromPVC(&pod, s.config.WorkerLogPVC, logVolumeName, s.config.LogBase, false)
	}
	if s.config.RequiresSecret() {
		addVolumeFromSecret(&pod, SharedSecretName, secretVolumeName, s.config.SecretDir, true)
	}
	if s.config.UsePoolProxy() && s.config.UseSecureCommunication {
		addVolumeFromSecret(&pod, proxy.Name, proxyCertVolumeName, proxyCertDir, false)
	}
	s.addAdditionalMatlabPVCs(&pod)

	// Add custom PVCs
	for pvc, path := range s.config.AdditionalWorkerPVCs {
		addVolumeFromPVC(&pod, pvc, pvc, path, false)
	}

	if s.config.OverrideWorkergroupConfig {
		configPath := filepath.Join(s.config.MatlabRoot, "toolbox", "parallel", "config", "workergroup.config")
		addVolumeFromConfigMapFile(&pod, "mjs-worker-config", "worker-config-volume", "workergroup.config", configPath)
	}

	return s.wrapPod(&pod, w.HostName, getLabelsForWorker(w))
}

// GetWorkerServiceSpec creates a spec for an internal Kubernetes service that points to an MJS worker pod; this service is used by other pods to communicate with the worker pod
func (s *SpecFactory) GetWorkerServiceSpec(w *WorkerInfo) *corev1.Service {
	// All workers need to expose their MJS ports to allow the job manager to connect
	workerPorts := []int{}
	for p := 0; p < 10; p++ {
		workerPorts = append(workerPorts, s.config.BasePort+p)
	}

	// All workers must expose their MPI ports
	minMPIPort, maxMPIPort := s.getMPIPorts()
	for p := minMPIPort; p <= maxMPIPort; p++ {
		workerPorts = append(workerPorts, p)
	}

	// Add parpool ports for this worker
	minPort, maxPort := s.CalculateWorkerPorts()
	for p := minPort; p <= maxPort; p++ {
		workerPorts = append(workerPorts, p)
	}

	// Create the service spec
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            w.HostName,
			Labels:          getLabelsForWorker(w),
			OwnerReferences: s.ownerRefs,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: getLabelsForWorker(w),
		},
	}
	AddPorts(&svc, workerPorts)
	return &svc
}

// GetPoolProxyDeploymentSpec creates a spec for a deployment to run a parallel pool proxy pod
func (s *SpecFactory) GetPoolProxyDeploymentSpec(proxy *PoolProxyInfo) *appsv1.Deployment {
	// Input arguments for the pool proxy
	logfile := filepath.Join(s.config.LogBase, fmt.Sprintf("%s.log", proxy.Name))
	proxyArgs := []string{
		"--loglevel",
		fmt.Sprintf("%d", s.config.LogLevel),
		"--port",
		fmt.Sprintf("%d", proxy.Port),
		"--logfile",
		logfile,
	}
	if s.config.UseSecureCommunication {
		proxyArgs = append(proxyArgs, "--certificate", filepath.Join(proxyCertDir, ProxyCertFileName))
	}

	// Create container to run a proxy
	container := corev1.Container{
		Name:            "pool-proxy",
		Image:           s.config.PoolProxyImage,
		ImagePullPolicy: corev1.PullPolicy(s.config.PoolProxyImagePullPolicy),
		Args:            proxyArgs,
	}
	addResourceRequests(&container, s.config.PoolProxyCPURequest, s.config.PoolProxyMemoryRequest)
	addResourceLimits(&container, s.config.PoolProxyCPULimit, s.config.PoolProxyMemoryLimit)

	// Create pod spec
	var proxyGracePeriod int64 = 5
	pod := corev1.PodSpec{
		Containers:                    []corev1.Container{container},
		TerminationGracePeriodSeconds: &proxyGracePeriod,
	}
	s.setEnableServiceLinks(&pod)
	pod.NodeSelector = s.config.WorkerNodeSelector
	pod.Tolerations = s.workerTolerations

	// Add volumes
	if s.config.MatlabPVC != "" {
		addVolumeFromPVC(&pod, s.config.MatlabPVC, matlabVolumeName, s.config.MatlabRoot, true)
	}
	if s.config.WorkerLogPVC != "" {
		addVolumeFromPVC(&pod, s.config.WorkerLogPVC, logVolumeName, s.config.LogBase, false)
	}
	if s.config.UseSecureCommunication {
		addVolumeFromSecret(&pod, proxy.Name, proxyCertVolumeName, proxyCertDir, true)
	}

	labels := map[string]string{
		AppKey:               PoolProxyLabels.AppLabel, // Common label for all pool proxy pods to allow them to be easily listed
		PoolProxyLabels.Name: proxy.Name,
		PoolProxyLabels.ID:   fmt.Sprintf("%d", proxy.ID),
		PoolProxyLabels.Port: fmt.Sprintf("%d", proxy.Port),
	}
	return s.wrapPod(&pod, proxy.Name, labels)
}

const (
	JobManagerUIDKey   = "job-manager-uid"
	JobManagerHostname = "mjs-job-manager"
	AppKey             = "app"
)

// GetJobManagerDeploymentSpec creates a spec for a deployment to run the MJS job manager pod
func (s *SpecFactory) GetJobManagerDeploymentSpec() *appsv1.Deployment {
	// Create container to run the MJS job manager
	jobManagerScript := filepath.Join(s.config.MJSDefDir, "jobManager.sh")
	container := corev1.Container{
		Name:            JobManagerHostname,
		Image:           s.config.JobManagerImage,
		ImagePullPolicy: corev1.PullPolicy(s.config.JobManagerImagePullPolicy),
		Command:         []string{"/bin/sh", jobManagerScript},
	}
	addResourceRequests(&container, s.config.JobManagerCPURequest, s.config.JobManagerMemoryRequest)
	addResourceLimits(&container, s.config.JobManagerCPULimit, s.config.JobManagerMemoryLimit)

	// Include admin password if using Security Level >= 2
	if s.config.SecurityLevel >= 2 {
		addEnvFromSecret(&container, "PARALLEL_SERVER_JOBMANAGER_ADMIN_PASSWORD", AdminPasswordSecretName, AdminPasswordKey)
	}

	// Add startup and liveness probes
	binDir := filepath.Join(s.config.MatlabRoot, "toolbox", "parallel", "bin")
	healthcheckPath := filepath.Join(binDir, "glnxa64", "mjshealthcheck")
	probeHandler := &corev1.ProbeHandler{Exec: &corev1.ExecAction{
		Command: []string{healthcheckPath,
			"-jobmanager",
			s.config.JobManagerName,
			"-matlabroot",
			s.config.MatlabRoot,
			"-baseport",
			fmt.Sprintf("%d", s.config.BasePort),
			"-timeout",
			fmt.Sprintf("%d", s.config.LivenessProbeTimeout),
		},
	}}
	container.StartupProbe = &corev1.Probe{
		InitialDelaySeconds: s.config.StartupProbeInitialDelay,
		PeriodSeconds:       s.config.StartupProbePeriod,
		TimeoutSeconds:      s.config.LivenessProbeTimeout, // Use same timeout as liveness probe, since the probe handler is the same
		FailureThreshold:    s.config.StartupProbeFailureThreshold,
		ProbeHandler:        *probeHandler,
	}
	container.LivenessProbe = &corev1.Probe{
		InitialDelaySeconds: s.config.LivenessProbePeriod,
		PeriodSeconds:       s.config.LivenessProbePeriod,
		TimeoutSeconds:      s.config.LivenessProbeTimeout,
		FailureThreshold:    s.config.LivenessProbeFailureThreshold,
		ProbeHandler:        *probeHandler,
	}

	// Add pre-stop hook to stop the job manager cleanly
	stopJobManagerCmd := []string{
		filepath.Join(binDir, "stopjobmanager"),
		"-name",
		s.config.JobManagerName,
		"-cleanPreserveJobs",
	}
	if s.config.RequireScriptVerification {
		stopJobManagerCmd = append(stopJobManagerCmd, "-secretfile", filepath.Join(s.config.SecretDir, s.config.SecretFileName))
	}
	container.Lifecycle = &corev1.Lifecycle{
		PreStop: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: stopJobManagerCmd,
			},
		},
	}
	// Create pod spec
	pod := corev1.PodSpec{
		Containers: []corev1.Container{container},
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser:  &s.config.JobManagerUserID,
			RunAsGroup: &s.config.JobManagerGroupID,
			FSGroup:    &s.config.JobManagerGroupID,
		},
	}
	s.setEnableServiceLinks(&pod)
	pod.NodeSelector = s.config.JobManagerNodeSelector
	pod.Tolerations = s.jobManagerTolerations

	// Add volumes
	addVolumeFromConfigMap(&pod, s.config.MJSDefConfigMap, mjsDefVolumeName, s.config.MJSDefDir)
	if s.config.MatlabPVC != "" && s.config.JobManagerUsesPVC {
		addVolumeFromPVC(&pod, s.config.MatlabPVC, matlabVolumeName, s.config.MatlabRoot, true)
	}
	if s.config.LogPVC != "" {
		addVolumeFromPVC(&pod, s.config.LogPVC, logVolumeName, s.config.LogBase, false)
	}
	if s.config.CheckpointPVC != "" {
		addVolumeFromPVC(&pod, s.config.CheckpointPVC, checkpointVolumeName, s.config.CheckpointBase, false)
	}
	if s.config.RequiresSecret() {
		addVolumeFromSecret(&pod, SharedSecretName, secretVolumeName, s.config.SecretDir, true)
	}
	if s.config.LDAPCertPath != "" {
		addVolumeFromSecret(&pod, LDAPSecretName, ldapCertVolumeName, s.config.LDAPCertDir(), true)
	}
	if s.config.UseSecureMetrics {
		addVolumeFromSecret(&pod, MetricsSecretName, metricsCertVolumeName, s.config.MetricsCertDir, true)
	}

	// Ensure this pod can resolve itself via its service name without having to use the Kubernetes service; this ensures it can resolve its own MJS service even if the Kubernetes service does not map to this pod
	pod.HostAliases = []corev1.HostAlias{{
		IP:        "127.0.0.1",
		Hostnames: []string{JobManagerHostname},
	}}

	return s.wrapPod(&pod, JobManagerHostname, s.getLabelsForJobManager())
}

// GetSecretSpec creates a spec for an empty Kubernetes secret.
// If preserve=true, the secret will not be deleted when the controller is deleted.
func (s *SpecFactory) GetSecretSpec(name string, preserve bool) *corev1.Secret {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: map[string][]byte{},
	}
	if !preserve {
		secret.ObjectMeta.OwnerReferences = s.ownerRefs
	}
	return &secret
}

// CalculateWorkerPorts returns the min and max port to use for parpools on worker pods
func (s *SpecFactory) CalculateWorkerPorts() (int, int) {
	minPort := s.config.BasePort + 10
	maxPort := minPort + s.config.PortsPerWorker - 1
	return minPort, maxPort
}

// CalculateProxyForWorker calculates the proxy that a worker should use based on its ID
func (s *SpecFactory) CalculatePoolProxyForWorker(workerID int) int {
	ratio := float64(workerID) / float64(s.config.WorkersPerPoolProxy)
	return int(math.Ceil(ratio))
}

// PoolProxyInfo is a struct containing proxy metadata
type PoolProxyInfo struct {
	ID   int    // Proxy ID; used to relate workers to proxies
	Name string // Proxy name; this is also the host name
	Port int    // Proxy port
}

// NewPoolProxyInfo creates a PoolProxyInfo struct for a given proxy ID
func NewPoolProxyInfo(id int, basePort int) PoolProxyInfo {
	return PoolProxyInfo{
		ID:   id,
		Name: fmt.Sprintf("mjs-pool-proxy-%d", id),
		Port: basePort + id - 1, // note we are using 1-based indexing
	}
}

// addVolumeFromPVC adds a volume mounted from a PersistentVolumeClaim to a pod spec
func addVolumeFromPVC(pod *corev1.PodSpec, pvcName, volumeName, mountPath string, readOnly bool) {
	source := corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: pvcName,
		},
	}
	addVolume(pod, source, volumeName, mountPath, readOnly)
}

// addVolumeFromConfigMap adds a volume mounted from a ConfigMap to a pod spec
func addVolumeFromConfigMap(pod *corev1.PodSpec, configMapName, volumeName, mountPath string) {
	source := corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: configMapName,
			},
		},
	}
	addVolume(pod, source, volumeName, mountPath, true)
}

// addVolumeFromSecret adds a volume mounted from a Kubernetes Secret to a pod spec
func addVolumeFromSecret(pod *corev1.PodSpec, secretName, volumeName, mountPath string, restrictReadAccess bool) {
	source := corev1.VolumeSource{
		Secret: &corev1.SecretVolumeSource{
			SecretName: secretName,
		},
	}
	if restrictReadAccess {
		var accessMode int32 = 400 // only readable by owner
		source.Secret.DefaultMode = &accessMode
	}
	addVolume(pod, source, volumeName, mountPath, true)
}

// addVolume adds a generic volume source to a pod spec
func addVolume(pod *corev1.PodSpec, source corev1.VolumeSource, volumeName, mountPath string, readOnly bool) {
	pod.Containers[0].VolumeMounts = append(pod.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
		ReadOnly:  readOnly,
	})
	pod.Volumes = append(pod.Volumes, corev1.Volume{
		Name:         volumeName,
		VolumeSource: source,
	})
}

// addEnv appends environment variables from a map to a container
func addEnv(container *corev1.Container, toAdd map[string]string) {
	env := container.Env
	for key, val := range toAdd {
		env = append(env, corev1.EnvVar{Name: key, Value: val})
	}
	container.Env = env
}

// addEnvFromSecret appends an environment variable from a secret to a container
func addEnvFromSecret(container *corev1.Container, varName, secretName, secretKey string) {
	container.Env = append(container.Env, corev1.EnvVar{
		Name: varName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
				Key: secretKey,
			},
		},
	})
}

// addResourceLimits adds resource limits to a container
func addResourceLimits(container *corev1.Container, cpu, memory string) {
	container.Resources.Limits = getResourceList(cpu, memory)
}

// addResourceRequests adds resource requests to a container
func addResourceRequests(container *corev1.Container, cpu, memory string) {
	container.Resources.Requests = getResourceList(cpu, memory)
}

// getResourceList converts CPU and memory strings to a pod ResourceList
func getResourceList(cpu, memory string) corev1.ResourceList {
	resourceList := corev1.ResourceList{}
	if cpu != "" {
		resourceList[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		resourceList[corev1.ResourceMemory] = resource.MustParse(memory)
	}
	return resourceList
}

// AddPorts appends ports to a service spec
func AddPorts(svc *corev1.Service, toAdd []int) {
	ports := svc.Spec.Ports
	existingPorts := map[int]bool{}
	for _, p := range ports {
		existingPorts[int(p.Port)] = true
	}
	for _, p := range toAdd {
		if existingPorts[p] {
			continue
		}
		ports = append(ports, corev1.ServicePort{
			Name:       fmt.Sprintf("tcp-%d", p),
			Port:       int32(p),
			TargetPort: intstr.FromInt(p),
			Protocol:   "TCP",
		})
	}
	svc.Spec.Ports = ports
}

// WorkerLabels defines labels to apply to worker pods
var WorkerLabels = struct {
	Name     string
	ID       string
	HostName string
	AppLabel string
}{
	Name:     "workerName",
	ID:       "workerID",
	HostName: "hostName",
	AppLabel: "mjs-worker",
}

// getLabelsForWorker returns a map of labels to apply to a worker resource
func getLabelsForWorker(w *WorkerInfo) map[string]string {
	return map[string]string{
		AppKey:                WorkerLabels.AppLabel, // Common label for all worker pods to allow them to be easily listed
		WorkerLabels.Name:     w.Name,
		WorkerLabels.ID:       fmt.Sprintf("%d", w.ID),
		WorkerLabels.HostName: w.HostName,
	}
}

// getLabelsForJobManager returns a map of labels to apply to a job manager pod or service
func (s *SpecFactory) getLabelsForJobManager() map[string]string {
	return map[string]string{
		AppKey:           JobManagerHostname,
		JobManagerUIDKey: s.config.JobManagerUID,
	}
}

// PoolProxyLabels defines labels to apply to pool proxy pods
var PoolProxyLabels = struct {
	Name     string
	ID       string
	Port     string
	AppLabel string
}{
	Name:     "proxyName",
	ID:       "proxyID",
	Port:     "port",
	AppLabel: "mjs-pool-proxy",
}

// wrapPod wraps a pod spec into a deployment spec
func (s *SpecFactory) wrapPod(pod *corev1.PodSpec, name string, labels map[string]string) *appsv1.Deployment {
	var numReplicas int32 = 1 // Always want only one pod per deployment
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Labels:          labels,
			OwnerReferences: s.ownerRefs,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: *pod,
			},
		},
	}
}

// WorkerInfo is a struct containing a worker's name, ID and and unique host name
type WorkerInfo struct {
	Name     string
	ID       int
	HostName string
}

// GenerateUniqueHostName generates a unique host name for a worker
func (w *WorkerInfo) GenerateUniqueHostName() {
	guidWithHyphens := uuid.New()
	guidNoHyphens := strings.Replace(guidWithHyphens.String(), "-", "", -1)
	uniqueHostName := fmt.Sprintf("%s-%s", w.Name, guidNoHyphens)
	maxLen := 64
	if len(uniqueHostName) > maxLen {
		uniqueHostName = uniqueHostName[:maxLen]
	}
	w.HostName = uniqueHostName
}

// Number of MPI ports to open on each worker pod
const mpiPortsPerWorker = 10

// Get the minimum and maximum MPI ports that workers should use
func (s *SpecFactory) getMPIPorts() (int, int) {
	minPort := s.config.BasePort + 1000
	maxPort := minPort + mpiPortsPerWorker - 1
	return minPort, maxPort
}

// Set enableServiceLinks; setting this to false to prevents large numbers of environment variables being created for pods in large clusters
func (s *SpecFactory) setEnableServiceLinks(pod *corev1.PodSpec) {
	pod.EnableServiceLinks = &s.config.EnableServiceLinks
}

// Convert a service hostname to a hostname that can be resolved by pods in other namespaces
func (s *SpecFactory) GetServiceHostname(svcName string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", svcName, s.config.Namespace)
}

// Mount volumes containing additional MATLAB installations from previous releases
func (s *SpecFactory) addAdditionalMatlabPVCs(pod *corev1.PodSpec) {
	if len(s.config.AdditionalMatlabPVCs) == 0 {
		return
	}
	additionalMATLABRoots := []string{}
	for _, matlabPVC := range s.config.AdditionalMatlabPVCs {
		mountPath := filepath.Join(additionalMatlabDir, matlabPVC)
		addVolumeFromPVC(pod, matlabPVC, matlabPVC, mountPath, true)
		additionalMATLABRoots = append(additionalMATLABRoots, mountPath)
	}
	addEnv(&pod.Containers[0], map[string]string{
		"MJS_ADDITIONAL_MATLABROOTS": strings.Join(additionalMATLABRoots, ":"),
	})
}

// addVolumeFromConfigMapFile adds a volume mounted from a ConfigMap file to a single file on the pod
func addVolumeFromConfigMapFile(pod *corev1.PodSpec, configMapName, volumeName, fileName, mountPath string) {
	source := corev1.VolumeSource{
		ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: configMapName,
			},
			Items: []corev1.KeyToPath{
				{
					Key:  fileName,
					Path: fileName,
				},
			},
		},
	}
	pod.Containers[0].VolumeMounts = append(pod.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
		SubPath:   fileName,
		ReadOnly:  true,
	})
	pod.Volumes = append(pod.Volumes, corev1.Volume{
		Name:         volumeName,
		VolumeSource: source,
	})
}

func parseTolerations(input string) ([]corev1.Toleration, error) {
	var tolerations []corev1.Toleration
	if input == "" {
		return tolerations, nil
	}
	err := json.Unmarshal([]byte(input), &tolerations)
	return tolerations, err
}
