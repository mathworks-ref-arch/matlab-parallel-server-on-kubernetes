// Copyright 2024 The MathWorks, Inc.
package specs

import (
	"controller/internal/config"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Test the creation and use of a SpecFactory
func TestGetWorkerDeploymentSpec(t *testing.T) {
	testCases := []struct {
		name                   string
		usePoolProxy           bool
		useSecureCommunication bool
	}{
		{"no_proxy_no_secure", false, false},
		{"proxy_no_secure", true, false},
		{"no_proxy_secure", false, true},
		{"proxy_secure", true, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ownerUID := types.UID("abcd1234")
			conf := createTestConfig()
			conf.InternalClientsOnly = !tc.usePoolProxy
			conf.UseSecureCommunication = tc.useSecureCommunication

			// Create a SpecFactory
			specFactory := NewSpecFactory(conf, ownerUID)
			assert.NotNil(t, specFactory)
			assert.Equal(t, conf, specFactory.config)

			// Verify the specs it creates
			verifyWorkerSpecs(t, specFactory, conf, ownerUID)
			verifyProxySpecs(t, specFactory, conf, ownerUID)
		})
	}
}

// Test the calculate of which proxy to use for each worker
func TestCalculateProxyForWorker(t *testing.T) {
	conf := &config.Config{
		WorkersPerPoolProxy: 10,
	}
	specFactory := NewSpecFactory(conf, "abcd")
	assert.Equal(t, 1, specFactory.CalculatePoolProxyForWorker(1))
	assert.Equal(t, 1, specFactory.CalculatePoolProxyForWorker(10))
	assert.Equal(t, 2, specFactory.CalculatePoolProxyForWorker(11))
}

// Test generation of unique hostnames for workers
func TestGenerateUniqueHostname(t *testing.T) {
	assert := assert.New(t)

	worker1 := WorkerInfo{
		Name: "myworker",
		ID:   10,
	}
	assert.Empty(worker1.HostName, "Worker hostname should initially be empty")

	worker1.GenerateUniqueHostName()
	assert.NotEmpty(worker1.HostName, "Worker hostname should be filled after GenerateUniqueHostName")
	assert.Contains(worker1.HostName, worker1.Name, "Worker hostname should contain worker name")

	// Check that a second worker with the same name gets a unique host name
	worker2 := WorkerInfo{
		Name: worker1.Name,
		ID:   worker1.ID,
	}
	worker2.GenerateUniqueHostName()
	assert.NotEmpty(worker2.HostName, "Worker hostname should be filled after GenerateUniqueHostName")
	assert.NotEqual(worker1.HostName, worker2.HostName, "Worker hostnames should be unique")
}

// Check that all of the allowed MPI ports are exposed by the worker service (g3221764)
func TestMPIPorts(t *testing.T) {
	ownerUID := types.UID("abcd1234")
	conf := createTestConfig()
	conf.BasePort = 30000
	specFactory := NewSpecFactory(conf, ownerUID)

	worker := WorkerInfo{
		Name:     "worker",
		ID:       5,
		HostName: "test",
	}
	deployment := specFactory.GetWorkerDeploymentSpec(&worker)
	service := specFactory.GetWorkerServiceSpec(&worker)

	// Check that the MPI port range environment variable is set on the worker
	expectedEnvVar := "MPICH_PORT_RANGE"
	gotEnv := deployment.Spec.Template.Spec.Containers[0].Env
	found := false
	var gotVal string
	for _, e := range gotEnv {
		if e.Name == expectedEnvVar {
			found = true
			gotVal = e.Value
			break
		}
	}
	assert.Truef(t, found, "Worker environment variable %s should be set", expectedEnvVar)

	// Extract the ports from the environment variable
	ports := strings.Split(gotVal, ":")
	assert.Lenf(t, ports, 2, "%s should have format minPort:maxPort", expectedEnvVar)
	minPort, err := strconv.Atoi(ports[0])
	assert.NoErrorf(t, err, "Failed to convert min MPI port '%s' to an int", ports[0])
	maxPort, err := strconv.Atoi(ports[1])
	assert.NoErrorf(t, err, "Failed to convert max MPI port '%s' to an int", ports[1])

	// Check the port numbers
	assert.Equal(t, conf.BasePort+1000, minPort, "Unexpected minimum MPI port")
	assert.Equal(t, minPort+mpiPortsPerWorker-1, maxPort, "Unexpected maximum MPI port")

	// Check that each port was exposed on the worker service
	exposedPorts := map[int]bool{}
	for _, p := range service.Spec.Ports {
		exposedPorts[int(p.Port)] = true
	}
	for p := minPort; p <= maxPort; p++ {
		assert.Contains(t, exposedPorts, minPort, "MPI port should be exposed by the worker service")
	}

	// Ensure the pod's hostname matches the service name
	assert.Equal(t, service.Name, deployment.Spec.Template.Spec.Hostname, "Worker pod hostname must match the service name in order for MPI to work")
}

func TestGetJobManagerSpecs(t *testing.T) {
	testCases := []struct {
		name                   string
		useSecureCommunication bool
	}{
		{"insecure", false},
		{"secure_communication", true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			ownerUID := types.UID("abcd1234")
			conf := createTestConfig()
			conf.UseSecureCommunication = tc.useSecureCommunication
			conf.JobManagerUID = "jm123"

			// Create a SpecFactory
			specFactory := NewSpecFactory(conf, ownerUID)
			assert.NotNil(tt, specFactory)
			assert.Equal(tt, conf, specFactory.config)

			// Verify the job manager spec it creates
			verifyJobManagerSpec(tt, specFactory, conf, ownerUID)
		})
	}
}

// Test the ability to add custom environment variables to the worker pod spec
func TestExtraWorkerEnv(t *testing.T) {
	conf := createTestConfig()
	extraEnv := map[string]string{
		"MY_VAR1":     "test",
		"ANOTHER_VAR": "test2",
	}
	conf.ExtraWorkerEnvironment = extraEnv

	specFactory := NewSpecFactory(conf, types.UID("abc"))
	workerSpec := specFactory.GetWorkerDeploymentSpec(&WorkerInfo{
		Name:     "worker1",
		ID:       1,
		HostName: "host1",
	})
	workerEnv := workerSpec.Spec.Template.Spec.Containers[0].Env

	for key, value := range extraEnv {
		found := false
		var gotValue string
		for _, item := range workerEnv {
			if item.Name == key {
				found = true
				gotValue = item.Value
				break
			}
		}
		assert.Truef(t, found, "Custom worker environment variable %s not found", key)
		assert.Equalf(t, value, gotValue, "Unexpected value for worker environment variable %s", key)
	}
}

func TestAdditionalMatlabRoots(t *testing.T) {
	testCases := []struct {
		name                          string
		matlabPVCs                    []string
		expectedAdditionalMatlabRoots string
	}{
		{"no_additional", []string{}, ""},
		{"single_additional", []string{"my-extra-pvc"}, "/opt/additionalmatlab/my-extra-pvc"},
		{"multiple_additional", []string{"matlab24a", "matlab24b", "matlab25a"}, "/opt/additionalmatlab/matlab24a:/opt/additionalmatlab/matlab24b:/opt/additionalmatlab/matlab25a"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			conf := createTestConfig()
			conf.AdditionalMatlabPVCs = tc.matlabPVCs
			specFactory := NewSpecFactory(conf, types.UID("abc"))
			workerSpec := specFactory.GetWorkerDeploymentSpec(&WorkerInfo{
				Name:     "worker1",
				ID:       1,
				HostName: "host1",
			})
			verifyAdditionalMatlabPVCs(tt, workerSpec, tc.matlabPVCs, tc.expectedAdditionalMatlabRoots)
		})
	}
}

// Check whether the LDAP certificate is mounted or not mounted based on the value of LDAPCertPath
func TestMountLDAPCert(t *testing.T) {
	testCases := []struct {
		name              string
		ldapCertDir       string
		certFile          string
		expectMountedCert bool
	}{
		{"no_ldap_cert", "", "", false},
		{"ldap_cert", "/mount/dir", "my-cert.pem", true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			ownerUID := types.UID("abcd1234")
			conf := createTestConfig()
			conf.LDAPCertPath = filepath.Join(tc.ldapCertDir, tc.certFile)

			// Create a job manager spec
			specFactory := NewSpecFactory(conf, ownerUID)
			require.NotNil(tt, specFactory)
			require.Equal(tt, conf, specFactory.config)
			spec := specFactory.GetJobManagerDeploymentSpec()

			// Check for the LDAP volume
			pod := spec.Spec.Template.Spec
			if tc.expectMountedCert {
				vol, volMount := verifyPodHasVolume(tt, &pod, ldapCertVolumeName)
				assert.NotNil(tt, vol.VolumeSource.Secret, "LDAP certificate volume should be a secret volume")
				assert.Equal(tt, LDAPSecretName, vol.VolumeSource.Secret.SecretName, "LDAP volume should use LDAP secret name")
				assert.Equal(tt, tc.ldapCertDir, volMount.MountPath, "LDAP volume should be mounted at LDAP mount path")
			} else {
				verifyPodDoesNotHaveVolume(tt, &pod, ldapCertVolumeName)
			}
		})
	}
}

// Check whether the metrics secret is mounted or not
func TestMountMetricsSecret(t *testing.T) {
	const metricsDir = "/test/metrics"
	testCases := []struct {
		name             string
		useSecureMetrics bool
	}{
		{"insecure_metrics", false},
		{"secure_metrics", true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			ownerUID := types.UID("abcd1234")
			conf := createTestConfig()
			conf.UseSecureMetrics = tc.useSecureMetrics
			conf.MetricsCertDir = metricsDir

			// Create a job manager spec
			specFactory := NewSpecFactory(conf, ownerUID)
			require.NotNil(tt, specFactory)
			require.Equal(tt, conf, specFactory.config)
			spec := specFactory.GetJobManagerDeploymentSpec()

			// Check for the metrics secret volume
			pod := spec.Spec.Template.Spec
			if tc.useSecureMetrics {
				vol, volMount := verifyPodHasVolume(tt, &pod, metricsCertVolumeName)
				assert.NotNil(tt, vol.VolumeSource.Secret, "LDAP certificate volume should be a secret volume")
				assert.Equal(tt, MetricsSecretName, vol.VolumeSource.Secret.SecretName, "Metrics volume should use metrics secret name")
				assert.Equal(tt, metricsDir, volMount.MountPath, "Metrics volume should be mounted at metrics certificate mount path")
			} else {
				verifyPodDoesNotHaveVolume(tt, &pod, metricsCertVolumeName)
			}
		})
	}
}

func verifyWorkerSpecs(t *testing.T, specFactory *SpecFactory, conf *config.Config, ownerUID types.UID) {
	assert := assert.New(t)
	testWorker := &WorkerInfo{
		Name:     "worker1",
		ID:       3,
		HostName: "my-worker-host",
	}

	// Create a worker deployment
	deployment := specFactory.GetWorkerDeploymentSpec(testWorker)
	assert.NotNil(deployment)

	// Check basic properties
	verifyDeployment(t, deployment, ownerUID)
	assert.Equalf(WorkerLabels.AppLabel, deployment.Labels[AppKey], "Missing deployment label")
	assert.Equal(testWorker.HostName, deployment.ObjectMeta.Name, "Deployment name should match worker hostname")

	// Check the underlying pod spec
	pod := deployment.Spec.Template.Spec
	assert.Len(pod.Containers, 1, "Worker pod should have 1 container")
	container := pod.Containers[0]
	assert.Equal(conf.WorkerImage, container.Image, "Worker container has incorrect image")
	assert.Equal(conf.WorkerImagePullPolicy, string(container.ImagePullPolicy), "Worker container has incorrect image pull policy")
	assert.Equal(conf.WorkerCPULimit, container.Resources.Limits.Cpu().String(), "Worker container has incorrect CPU limit")
	assert.Equal(conf.WorkerCPURequest, container.Resources.Requests.Cpu().String(), "Worker container has incorrect CPU request")
	assert.Equal(conf.WorkerMemoryLimit, container.Resources.Limits.Memory().String(), "Worker container has incorrect memory limit")
	assert.Equal(conf.WorkerMemoryRequest, container.Resources.Requests.Memory().String(), "Worker container has incorrect memory request")
	assert.Contains(container.Args[0], conf.MJSDefDir, "Worker container arg should point to worker script under the mjs_def mount directory")
	assert.NotNil(container.Lifecycle.PreStop, "Worker container should have pre-stop hook set")
	assert.Equal(int64(0), *pod.SecurityContext.RunAsUser, "Worker pod should run as root")
	verifyEnableServiceLinksFalse(t, &pod)

	// Check volumes
	verifyPodHasVolume(t, &pod, mjsDefVolumeName)
	if conf.UseSecureCommunication {
		verifyPodHasVolume(t, &pod, secretVolumeName)
	} else {
		verifyPodDoesNotHaveVolume(t, &pod, secretVolumeName)
	}
	if conf.UsePoolProxy() && conf.UseSecureCommunication {
		verifyPodHasVolume(t, &pod, proxyCertVolumeName)
	} else {
		verifyPodDoesNotHaveVolume(t, &pod, proxyCertVolumeName)
	}

	// Check environment
	verifyEnvVar(t, &pod, "MLM_LICENSE_FILE", conf.NetworkLicenseManager)
	verifyEnvVar(t, &pod, "MJS_WORKER_USERNAME", conf.WorkerUsername)
	verifyEnvVar(t, &pod, "MJS_WORKER_PASSWORD", conf.WorkerPassword)
	verifyEnvVar(t, &pod, "USER", conf.WorkerUsername) // USER should be set (g3299166)
	verifyEnvVar(t, &pod, "SHELL", "/bin/sh")
	verifyEnvVar(t, &pod, "WORKER_NAME", testWorker.Name)
	verifyEnvVar(t, &pod, "HOSTNAME", testWorker.HostName)
	verifyEnvVar(t, &pod, "MDCE_OVERRIDE_INTERNAL_HOSTNAME", testWorker.HostName)
	if conf.UsePoolProxy() {
		// Make sure the proxy environment variables are set
		proxyID := specFactory.CalculatePoolProxyForWorker(testWorker.ID)
		proxy := NewPoolProxyInfo(proxyID, conf.PoolProxyBasePort)
		verifyEnvVar(t, &pod, "PARALLEL_SERVER_POOL_PROXY_PORT", fmt.Sprintf("%d", proxy.Port))
		verifyEnvVar(t, &pod, "PARALLEL_SERVER_POOL_PROXY_HOST", proxy.Name)
		verifyEnvVar(t, &pod, "PARALLEL_SERVER_POOL_PROXY_EXTERNAL_HOST", "$CLIENT_OVERRIDE")

		// Check the proxy certificate variable; this should only be set if USE_SECURE_COMMUNICATION=true
		if conf.UseSecureCommunication {
			verifyEnvVar(t, &pod, "PARALLEL_SERVER_POOL_PROXY_CERTIFICATE", filepath.Join(proxyCertDir, ProxyCertFileName))
		} else {
			verifyEnvUnset(t, &pod, "PARALLEL_SERVER_POOL_PROXY_CERTIFICATE")
		}

		// Make sure the port range override is not set; this is only needed for the many-ports configuration
		verifyEnvUnset(t, &pod, "PARALLEL_SERVER_OVERRIDE_PORT_RANGE")
	} else {
		// Make sure none of the proxy environment variables are set
		verifyEnvUnset(t, &pod, "PARALLEL_SERVER_POOL_PROXY_PORT")
		verifyEnvUnset(t, &pod, "PARALLEL_SERVER_POOL_PROXY_HOST")
		verifyEnvUnset(t, &pod, "PARALLEL_SERVER_POOL_PROXY_EXTERNAL_HOST")
		verifyEnvUnset(t, &pod, "PARALLEL_SERVER_POOL_PROXY_CERTIFICATE")

		// Check we are using the full DNS name of the service for internal clients
		expectedHostname := specFactory.GetServiceHostname(testWorker.HostName)
		verifyEnvVar(t, &pod, "MDCE_OVERRIDE_EXTERNAL_HOSTNAME", expectedHostname)
	}

	// Create a service spec for the same worker
	svc := specFactory.GetWorkerServiceSpec(testWorker)
	assert.Equal(deployment.Spec.Template.ObjectMeta.Labels, svc.Spec.Selector, "Service selector should match worker pod's labels")
	ownerRefs := svc.ObjectMeta.OwnerReferences
	assert.Len(ownerRefs, 1, "Service should have 1 owner reference")
	assert.Equal(ownerUID, ownerRefs[0].UID, "Service has incorrect owner UID")

	// Check ports
	minPort, maxPort := specFactory.CalculateWorkerPorts()
	for p := minPort; p <= maxPort; p++ {
		assert.Truef(serviceHasPort(svc, p), "Worker parpool port %d missing from service", p)
	}
}

func verifyProxySpecs(t *testing.T, specFactory *SpecFactory, conf *config.Config, ownerUID types.UID) {
	assert := assert.New(t)
	testProxy := &PoolProxyInfo{
		Name: "myproxy",
		ID:   10,
		Port: 40000,
	}

	// Create proxy deployment spec
	deployment := specFactory.GetPoolProxyDeploymentSpec(testProxy)
	verifyDeployment(t, deployment, ownerUID)
	assert.Equal(testProxy.Name, deployment.ObjectMeta.Name, "Deployment name should match proxy name")

	// Check the pod has the label used to match it to the corresponding service created in the Helm template
	assert.Equal(testProxy.Name, deployment.Spec.Template.ObjectMeta.Labels[PoolProxyLabels.Name], "Proxy pod spec is missing the label required to match it to its corresponding service")

	// Check resource limits/requests
	assert.Len(deployment.Spec.Template.Spec.Containers, 1, "Proxy pod should have one container")
	container := deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(conf.PoolProxyCPULimit, container.Resources.Limits.Cpu().String(), "Proxy container has incorrect CPU limit")
	assert.Equal(conf.PoolProxyCPURequest, container.Resources.Requests.Cpu().String(), "Proxy container has incorrect CPU request")
	assert.Equal(conf.PoolProxyMemoryLimit, container.Resources.Limits.Memory().String(), "Proxy container has incorrect memory limit")
	assert.Equal(conf.PoolProxyMemoryRequest, container.Resources.Requests.Memory().String(), "Proxy container has incorrect memory request")
	assert.Equal(conf.PoolProxyImage, container.Image, "Proxy container has unexpected image")
	assert.Equal(conf.PoolProxyImagePullPolicy, string(container.ImagePullPolicy), "Proxy container has unexpected image pull policy")
	verifyEnableServiceLinksFalse(t, &deployment.Spec.Template.Spec)

	// Check the proxy input args
	args := container.Args
	assert.Contains(args, fmt.Sprintf("%d", testProxy.Port), "Proxy port should appear in container args")
	if conf.UseSecureCommunication {
		assert.Contains(args, "--certificate", "Container args should include --certificate flag when UseSecureCommunication is true")
	} else {
		assert.NotContains(args, "--certificate", "Container args should not include --certificate flag when UseSecureCommunication is false")
	}

	// Check volumes
	pod := deployment.Spec.Template.Spec
	if conf.UseSecureCommunication {
		verifyPodHasVolume(t, &pod, proxyCertVolumeName)
	} else {
		verifyPodDoesNotHaveVolume(t, &pod, proxyCertVolumeName)
	}
}

func verifyJobManagerSpec(t *testing.T, specFactory *SpecFactory, conf *config.Config, ownerUID types.UID) {
	assert := assert.New(t)

	// Create the job manager deployment spec
	deployment := specFactory.GetJobManagerDeploymentSpec()
	assert.NotNil(deployment)

	// Check basic properties
	verifyDeployment(t, deployment, ownerUID)
	assert.Equal(JobManagerHostname, deployment.Labels[AppKey], "Deployment should have the job manager app label")
	assert.Equal(conf.JobManagerUID, deployment.Labels[JobManagerUIDKey], "Deployment should have the job manager UID label")
	assert.Equal(JobManagerHostname, deployment.ObjectMeta.Name, "Deployment should have job manager name")

	// Check the underlying pod spec
	pod := deployment.Spec.Template.Spec
	assert.Len(pod.Containers, 1, "Job manager pod should have 1 container")
	container := pod.Containers[0]
	assert.Equal(conf.JobManagerImage, container.Image, "Job manager container has incorrect image")
	assert.Equal(conf.JobManagerImagePullPolicy, string(container.ImagePullPolicy), "Job manager container has incorrect image pull policy")
	assert.Equal(conf.JobManagerCPULimit, container.Resources.Limits.Cpu().String(), "Job manager container has incorrect CPU limit")
	assert.Equal(conf.JobManagerCPURequest, container.Resources.Requests.Cpu().String(), "Job manager container has incorrect CPU request")
	assert.Equal(conf.JobManagerMemoryLimit, container.Resources.Limits.Memory().String(), "Job manager container has incorrect memory limit")
	assert.Equal(conf.JobManagerMemoryRequest, container.Resources.Requests.Memory().String(), "Job manager container has incorrect memory request")
	assert.Contains(container.Command[1], conf.MJSDefDir, "Job manager container arg should point to job manager script under the mjs_def mount directory")
	assert.NotNil(container.Lifecycle.PreStop, "Job manager container should have pre-stop hook set")
	assert.NotNil(container.LivenessProbe, "Job manager container should have liveness probe")
	assert.NotNil(container.StartupProbe, "Job manager container should have startup probe")
	assert.Equal(int64(conf.JobManagerUserID), *pod.SecurityContext.RunAsUser, "Job manager pod should run as job manager user UID")
	assert.Equal(int64(conf.JobManagerGroupID), *pod.SecurityContext.RunAsGroup, "Job manager pod should run as job manager group UID")
	verifyEnableServiceLinksFalse(t, &pod)

	// Check volumes
	verifyPodHasVolume(t, &pod, mjsDefVolumeName)
	verifyPodHasVolume(t, &pod, checkpointVolumeName)
	verifyPodHasVolume(t, &pod, matlabVolumeName)
	if conf.UseSecureCommunication {
		verifyPodHasVolume(t, &pod, secretVolumeName)
	} else {
		verifyPodDoesNotHaveVolume(t, &pod, secretVolumeName)
	}
}

// Verify common properties of all created deployment specs
func verifyDeployment(t *testing.T, deployment *appsv1.Deployment, ownerUID types.UID) {
	assert := assert.New(t)
	assert.Equal(int32(1), *deployment.Spec.Replicas, "Deployment should have 1 replica")
	assert.Equal(deployment.Spec.Selector.MatchLabels, deployment.Spec.Template.ObjectMeta.Labels, "Deployment selector labels should match template labels")
	ownerRefs := deployment.ObjectMeta.OwnerReferences
	assert.Len(ownerRefs, 1, "Deployment should have 1 owner reference")
	assert.Equal(ownerUID, ownerRefs[0].UID, "Deployment has incorrect owner UID")

	// Verify volumes common to all pods
	pod := deployment.Spec.Template.Spec
	verifyPodHasVolume(t, &pod, matlabVolumeName)
	verifyPodHasVolume(t, &pod, logVolumeName)
}

func verifyPodHasVolume(t *testing.T, pod *corev1.PodSpec, volumeName string) (*corev1.Volume, *corev1.VolumeMount) {
	vol, hasVol := podHasVolume(pod, volumeName)
	assert.Truef(t, hasVol, "Volume %s not found in pod spec", volumeName)
	volMount, hasMount := podHasVolumeMount(pod, volumeName)
	assert.Truef(t, hasMount, "Volume mount %s not found in container spec", volumeName)
	return vol, volMount
}

func verifyPodDoesNotHaveVolume(t *testing.T, pod *corev1.PodSpec, volumeName string) {
	_, hasVol := podHasVolume(pod, volumeName)
	assert.Falsef(t, hasVol, "Pod spec should not have volume %s", volumeName)
	_, hasMount := podHasVolumeMount(pod, volumeName)
	assert.Falsef(t, hasMount, "Container spec should not have volume mount %s", volumeName)
}

func verifyEnvUnset(t *testing.T, pod *corev1.PodSpec, varName string) {
	_, isSet := getPodEnvVar(pod, varName)
	assert.Falsef(t, isSet, "Pod should not have environment variable %s", varName)
}

// Verify that an environment variable is set and has the expected value
func verifyEnvVar(t *testing.T, pod *corev1.PodSpec, key, value string) {
	gotValue, isSet := getPodEnvVar(pod, key)
	assert.Truef(t, isSet, "Pod should have environment variable %s", key)
	assert.Equalf(t, gotValue, value, "Unexpected value for environment variable %s", key)
}

// Create a Config object populated with test values
func createTestConfig() *config.Config {
	return &config.Config{
		BasePort:                  20000,
		CheckpointPVC:             "checkpoint-pvc",
		EnableServiceLinks:        false,
		WorkerImagePullPolicy:     "Never",
		WorkerImage:               "my-matlab-image",
		JobManagerImagePullPolicy: "IfNotPresent",
		JobManagerImage:           "my-jm-image",
		LogBase:                   "/var/logs",
		LogLevel:                  3,
		MatlabRoot:                "/opt/matlab",
		MatlabPVC:                 "matlab-pvc",
		MJSDefConfigMap:           "mjsdef-cm",
		MJSDefDir:                 "/def-dir",
		NetworkLicenseManager:     "20000@mylicensemanager",
		PortsPerWorker:            3,
		PoolProxyImage:            "my-proxy",
		PoolProxyImagePullPolicy:  "Always",
		PoolProxyBasePort:         40000,
		PoolProxyCPURequest:       "500m",
		PoolProxyCPULimit:         "500m",
		PoolProxyMemoryLimit:      "2Gi",
		PoolProxyMemoryRequest:    "1Gi",
		WorkerCPURequest:          "3",
		WorkerCPULimit:            "4",
		WorkerMemoryRequest:       "3Gi",
		WorkerMemoryLimit:         "4Gi",
		JobManagerCPURequest:      "3",
		JobManagerCPULimit:        "4",
		JobManagerMemoryRequest:   "3Gi",
		JobManagerMemoryLimit:     "4Gi",
		JobManagerGroupID:         1000,
		JobManagerUserID:          2000,
		WorkerLogPVC:              "worker-pvc",
		LogPVC:                    "log-pvc",
		WorkerPassword:            "workerpw",
		WorkersPerPoolProxy:       10,
		WorkerUsername:            "myuser",
	}
}

func podHasVolume(pod *corev1.PodSpec, volumeName string) (*corev1.Volume, bool) {
	for _, v := range pod.Volumes {
		if v.Name == volumeName {
			return &v, true
		}
	}
	return nil, false
}

func podHasVolumeMount(pod *corev1.PodSpec, volumeName string) (*corev1.VolumeMount, bool) {
	for _, v := range pod.Containers[0].VolumeMounts {
		if v.Name == volumeName {
			return &v, true
		}
	}
	return nil, false
}

// Return a pod's environment variable value and whether or not it is set
func getPodEnvVar(pod *corev1.PodSpec, varName string) (string, bool) {
	for _, e := range pod.Containers[0].Env {
		if e.Name == varName {
			return e.Value, true
		}
	}
	return "", false
}

func serviceHasPort(svc *corev1.Service, portNum int) bool {
	for _, p := range svc.Spec.Ports {
		if p.Port == int32(portNum) {
			return true
		}
	}
	return false
}

func verifyEnableServiceLinksFalse(t *testing.T, pod *corev1.PodSpec) {
	assert.Falsef(t, *pod.EnableServiceLinks, "Pod should have EnableServiceLinks set to false")
}

func verifyAdditionalMatlabPVCs(t *testing.T, spec *appsv1.Deployment, matlabPVCs []string, expectedAdditionalMatlabRoots string) {
	pod := spec.Spec.Template
	for _, matlabPVC := range matlabPVCs {
		found := false
		for _, vol := range pod.Spec.Volumes {
			if pvc := vol.VolumeSource.PersistentVolumeClaim; pvc != nil && pvc.ClaimName == matlabPVC {
				found = true
				break
			}
		}
		assert.Truef(t, found, "Additional MATLAB volume %s should be mounted on pod %s", matlabPVC, pod.Name)
	}

	// Check the additional MATLAB root environment variable
	varName := "MJS_ADDITIONAL_MATLABROOTS"
	if expectedAdditionalMatlabRoots == "" {
		verifyEnvUnset(t, &pod.Spec, varName)
	} else {
		verifyEnvVar(t, &pod.Spec, varName, expectedAdditionalMatlabRoots)
	}
}
