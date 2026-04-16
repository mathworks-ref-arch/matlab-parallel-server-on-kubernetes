// Copyright 2024-2026 The MathWorks, Inc.
package specs_test

import (
	"controller/internal/config"
	"controller/internal/logging"
	"controller/internal/resize"
	"controller/internal/specs"
	mocks "controller/mocks/specs"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	certVolume          = "cert-vol"
	logDir              = "/test/log"
	poolProxyBasePort   = 4000
	poolProxyPrefix     = "my-proxy-"
	workerDomain        = "test.domain"
	workersPerPoolProxy = 3
	workerPrefix        = "my-worker-"
)

var testKeys = config.AnnotationKeys{
	CertVolume:             "test/certvol",
	LogDir:                 "test/logdir",
	PoolProxyBasePort:      "test/ppbase",
	PoolProxyID:            "test/ppid",
	PoolProxyPrefix:        "test/ppprefix",
	WorkerDomain:           "test/domain",
	WorkerID:               "test/wid",
	WorkerName:             "test/wname",
	WorkerPrefix:           "test/wprefix",
	WorkersPerPoolProxy:    "test/wppp",
	TemplateChecksum:       "test/templatechk",
	UseSecureCommunication: "test/usc",
	UsePoolProxy:           "test/upp",
}

func TestWorkerSpec(t *testing.T) {
	template := getWorkerPodTemplate(false, false)
	workerID := 5
	spec, checksum := getWorkerPodSpecFromTemplate(t, template, workerID)

	// Test properties
	expectedWorkerName := fmt.Sprintf("%s%d", workerPrefix, workerID)
	podName := spec.Name
	expectedHost := podName + "." + workerDomain
	assert.NotEqual(t, expectedWorkerName, podName, "Pod should have been generated a unique name")
	assert.Contains(t, podName, expectedWorkerName, "Generated pod name should contain worker name")
	assert.Equal(t, fmt.Sprintf("%d", workerID), spec.Labels[testKeys.WorkerID], "Pod should have worker ID label")
	assert.Equal(t, fmt.Sprintf("%d", workerID), spec.Annotations[testKeys.WorkerID], "Pod should have worker ID annotation")
	assert.Equal(t, expectedWorkerName, spec.Labels[testKeys.WorkerName], "Pod should have worker name label")
	assert.Equal(t, expectedWorkerName, spec.Annotations[testKeys.WorkerName], "Pod should have worker name annotation")

	// Test pod properties
	assert.Equal(t, spec.Name, spec.Spec.Hostname, "Pod hostname should match pod name")
	verifyPodBasedOnTemplate(t, spec.Spec, spec.ObjectMeta, template)
	verifyTemplateAnnotation(t, spec.ObjectMeta, checksum)

	// Test container properties
	container := &spec.Spec.Containers[0]
	verifyEnvVar(t, container, "WORKER_NAME", expectedWorkerName)
	verifyEnvVar(t, container, "HOSTNAME", expectedHost)
	verifyEnvVar(t, container, "MDCE_OVERRIDE_INTERNAL_HOSTNAME", expectedHost)
	verifyEnvVar(t, container, "MDCE_OVERRIDE_EXTERNAL_HOSTNAME", expectedHost)

	// Check we did not set pool proxy env vars
	verifyEnvUnset(t, container, "PARALLEL_SERVER_POOL_PROXY_HOST")
	verifyEnvUnset(t, container, "PARALLEL_SERVER_POOL_PROXY_PORT")
	assert.Empty(t, spec.Spec.Volumes, "There should be no volumes added when not using a pool proxy")
	assert.Empty(t, spec.Annotations[testKeys.PoolProxyID], "Pod should not be labelled with a pool proxy ID when pool proxies are not needed")
}

// Check multiple deployment specs for the same worker ID have unique names
func TestUniqueHostnames(t *testing.T) {
	checksum := "test"
	sf := newSpecFactory(t, getWorkerPodTemplate(false, false), nil, checksum)

	workerID := 11
	spec1 := getWorkerPodSpec(t, sf, workerID)
	spec2 := getWorkerPodSpec(t, sf, workerID)

	assert.NotEqual(t, spec1.Name, spec2.Name, "Multiple pod specs for same worker ID should have unique names")
	assert.Equal(t, spec1.Annotations[testKeys.WorkerID], spec2.Annotations[testKeys.WorkerID], "Worker ID annotations should match")
	assert.Equal(t, spec1.Annotations[testKeys.WorkerName], spec2.Annotations[testKeys.WorkerName], "Worker name annotations should match")
}

func TestWorkerSpecWithPoolProxy(t *testing.T) {
	template := getWorkerPodTemplate(true, false)
	workerID := 20
	spec, checksum := getWorkerPodSpecFromTemplate(t, template, workerID)

	// Test pod properties
	verifyPodBasedOnTemplate(t, spec.Spec, spec.ObjectMeta, template)
	verifyTemplateAnnotation(t, spec.ObjectMeta, checksum)
	assert.Empty(t, spec.Spec.Volumes, "There should be no volumes added when UseSecureCommunication is false")

	// Test container environment
	container := &spec.Spec.Containers[0]
	expectedProxyID := specs.CalculateProxyIDForWorker(workerID, workersPerPoolProxy)
	expectedProxyPort := poolProxyBasePort + expectedProxyID - 1
	verifyEnvVar(t, container, "PARALLEL_SERVER_POOL_PROXY_HOST", fmt.Sprintf("%s%d", poolProxyPrefix, expectedProxyID))
	verifyEnvVar(t, container, "PARALLEL_SERVER_POOL_PROXY_PORT", fmt.Sprintf("%d", expectedProxyPort))

	assert.Equal(t, fmt.Sprintf("%d", expectedProxyID), spec.Annotations[testKeys.PoolProxyID], "Spec should be annotated with the ID of the proxy the worker should use")
}

func TestWorkerSpecWithPoolProxySecureCommunication(t *testing.T) {
	template := getWorkerPodTemplate(true, true)
	workerID := 6
	spec, _ := getWorkerPodSpecFromTemplate(t, template, workerID)

	// Check a volume was created for the proxy certificate secret
	expectedProxyID := specs.CalculateProxyIDForWorker(workerID, workersPerPoolProxy)
	verifyPoolProxyVolume(t, &spec.Spec, expectedProxyID)
}

func TestPoolProxySpec(t *testing.T) {
	template := getProxyPodTemplate(false)
	checksum := "abcde"
	sf := newSpecFactory(t, nil, template, checksum)
	proxyID := 100
	spec := getProxyDeploymentSpec(t, sf, proxyID)

	// Test deployment properties
	expectedName := fmt.Sprintf("%s%d", poolProxyPrefix, proxyID)
	expectedPort := fmt.Sprintf("%d", poolProxyBasePort+proxyID-1)
	assert.Equal(t, expectedName, spec.Name, "Deployment has unexpected name")
	assert.Equal(t, fmt.Sprintf("%d", proxyID), spec.Labels[testKeys.PoolProxyID], "Deployment should have proxy ID label")
	assert.Equal(t, fmt.Sprintf("%d", proxyID), spec.Annotations[testKeys.PoolProxyID], "Deployment should have proxy ID annotation")
	verifyLabelSelector(t, spec)

	// Test pod properties
	pod := spec.Spec.Template
	verifyPodBasedOnTemplate(t, pod.Spec, pod.ObjectMeta, template)
	verifyTemplateAnnotation(t, spec.ObjectMeta, checksum)

	// Test container properties
	container := &pod.Spec.Containers[0]
	args := container.Args
	assert.Contains(t, args, "--port", "Args should include the port argument")
	assert.Contains(t, args, expectedPort, "Args should include the port value")
	assert.Contains(t, args, "--logfile", "Args should include the logfile argument")
	assert.Contains(t, args, filepath.Join(logDir, expectedName+".log"), "Args should include the log file path")
	assert.Empty(t, pod.Spec.Volumes, "There should be no volumes added when not using secure communication")
	assert.Empty(t, pod.Annotations[testKeys.SecretName], "Pod should not be annotated with proxy secret name when not using secure communication")

	// If we generate a new spec with the same ID, it should be identical
	spec2 := getProxyDeploymentSpec(t, sf, proxyID)
	assert.Equal(t, spec, spec2, "Repeated spec generation should produce same spec")
}

func TestPoolProxySpecSecureCommunication(t *testing.T) {
	template := getProxyPodTemplate(true)
	proxyID := 36
	spec, _ := getProxyDeploymentSpecFromTemplate(t, template, proxyID)

	// Check there is a volume for the proxy certificate secret
	verifyPoolProxyVolume(t, &spec.Spec.Template.Spec, proxyID)

	assert.Equal(t, fmt.Sprintf("%s%d", poolProxyPrefix, proxyID), spec.Annotations[testKeys.SecretName], "Spec should be annotated with the name of the secret used by this proxy")
}

func TestCalculateProxyIDForWorker(t *testing.T) {
	testCases := []struct {
		workerID        int
		workersPerProxy int
		expectedProxyID int
	}{
		{workerID: 1, workersPerProxy: 1, expectedProxyID: 1},
		{workerID: 2, workersPerProxy: 1, expectedProxyID: 2},
		{workerID: 1, workersPerProxy: 2, expectedProxyID: 1},
		{workerID: 3, workersPerProxy: 2, expectedProxyID: 2},
		{workerID: 100, workersPerProxy: 1, expectedProxyID: 100},
		{workerID: 101, workersPerProxy: 10, expectedProxyID: 11},
	}
	for _, tc := range testCases {
		actualProxyID := specs.CalculateProxyIDForWorker(tc.workerID, tc.workersPerProxy)
		assert.Equalf(t, tc.expectedProxyID, actualProxyID, "Unxpected proxy ID for worker %d with %d workers per proxy", tc.workerID, tc.workersPerProxy)
	}
}

func TestMissingWorkerAnnotations(t *testing.T) {
	requiredLabels := []string{
		testKeys.LogDir,
		testKeys.UsePoolProxy,
		testKeys.WorkerDomain,
		testKeys.WorkerPrefix,
	}
	for _, label := range requiredLabels {
		t.Run(label, func(tt *testing.T) {
			annotations := getRequiredWorkerAnnotations(false, false)
			delete(annotations, label)
			tmpl := getEmptyPodTemplate(annotations)

			sf := newSpecFactory(tt, tmpl, nil, "test")
			_, err := sf.GetWorkerPodSpec(1)
			require.Error(tt, err, "Expected error when required label is missing")
			assert.Contains(tt, err.Error(), label, "Error message should mention missing label")
		})
	}
}

func TestMissingWorkerAnnotationsWithPoolProxy(t *testing.T) {
	proxyOnlyLabels := []string{
		testKeys.CertVolume,
		testKeys.PoolProxyBasePort,
		testKeys.PoolProxyPrefix,
		testKeys.UseSecureCommunication,
		testKeys.WorkersPerPoolProxy,
	}
	for _, label := range proxyOnlyLabels {
		t.Run(label, func(tt *testing.T) {
			annotations := getRequiredWorkerAnnotations(false, false)
			delete(annotations, label)
			tmpl := getEmptyPodTemplate(annotations)
			sf := newSpecFactory(tt, tmpl, nil, "test")
			_, err := sf.GetWorkerPodSpec(1)
			require.NoError(tt, err, "Should be allowed to omit label when not using pool proxy")

			// Now make sure we require this label when using a pool proxy
			annotations[testKeys.UsePoolProxy] = "true"
			tmpl = getEmptyPodTemplate(annotations)
			sf = newSpecFactory(tt, tmpl, nil, "test")
			_, err = sf.GetWorkerPodSpec(1)
			require.Error(tt, err, "Expected error when required label is missing")
			assert.Contains(tt, err.Error(), label, "Error message should mention missing label")
		})
	}
}

func TestBadWorkerAnnotations(t *testing.T) {
	labelsToTest := []string{
		testKeys.PoolProxyBasePort,
		testKeys.UsePoolProxy,
		testKeys.UseSecureCommunication,
		testKeys.WorkersPerPoolProxy,
	}
	for _, label := range labelsToTest {
		t.Run(label, func(tt *testing.T) {
			badStr := "jadkfjadslkfj" // Can't be parsed as bool or int
			annotations := getRequiredWorkerAnnotations(true, true)
			annotations[label] = badStr
			tmpl := getEmptyPodTemplate(annotations)

			sf := newSpecFactory(tt, tmpl, nil, "test")
			_, err := sf.GetWorkerPodSpec(1)
			require.Error(tt, err, "Expected error when label has invalid value")
			assert.Contains(tt, err.Error(), label, "Error message should mention bad label")
			assert.Contains(tt, err.Error(), badStr, "Error message should mention bad value")
		})
	}
}

func TestMissingProxyAnnotations(t *testing.T) {
	requiredLabels := []string{
		testKeys.CertVolume,
		testKeys.LogDir,
		testKeys.PoolProxyBasePort,
		testKeys.PoolProxyPrefix,
		testKeys.UseSecureCommunication,
	}
	for _, label := range requiredLabels {
		t.Run(label, func(tt *testing.T) {
			annotations := getRequiredPoolProxyAnnotations(true)
			delete(annotations, label)
			tmpl := getEmptyPodTemplate(annotations)

			sf := newSpecFactory(tt, nil, tmpl, "test")
			_, err := sf.GetPoolProxyDeploymentSpec(1)
			require.Error(tt, err, "Expected error when required label is missing")
			assert.Contains(tt, err.Error(), label, "Error message should mention missing label")
		})
	}
}

func TestBadProxyAnnotations(t *testing.T) {
	labelsToTest := []string{
		testKeys.PoolProxyBasePort,
		testKeys.UseSecureCommunication,
	}
	for _, label := range labelsToTest {
		t.Run(label, func(tt *testing.T) {
			badStr := "jadkfjadslkfj" // Can't be parsed as bool or int
			annotations := getRequiredPoolProxyAnnotations(true)
			annotations[label] = badStr
			tmpl := getEmptyPodTemplate(annotations)

			sf := newSpecFactory(tt, nil, tmpl, "test")
			_, err := sf.GetPoolProxyDeploymentSpec(1)
			require.Error(tt, err, "Expected error when label has invalid value")
			assert.Contains(tt, err.Error(), label, "Error message should mention bad label")
			assert.Contains(tt, err.Error(), badStr, "Error message should mention bad value")
		})
	}
}

func getWorkerPodTemplate(usePoolProxy, useSecureCommunication bool) *corev1.PodTemplateSpec {
	annotations := getRequiredWorkerAnnotations(usePoolProxy, useSecureCommunication)
	return getEmptyPodTemplate(annotations)
}

func getProxyPodTemplate(useSecureCommunication bool) *corev1.PodTemplateSpec {
	annotations := getRequiredPoolProxyAnnotations(useSecureCommunication)
	return getEmptyPodTemplate(annotations)
}

func getEmptyPodTemplate(annotations map[string]string) *corev1.PodTemplateSpec {
	return &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"testlabel": "testval",
			},
			Annotations: annotations,
			Generation:  4,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "template",
					Image: "template-image",
					Env: []corev1.EnvVar{
						{
							Name:  "test-env",
							Value: "test-val",
						},
					},
				},
			},
		},
	}
}

func verifyEnvUnset(t *testing.T, container *corev1.Container, varName string) {
	_, isSet := getContainerEnvVar(container, varName)
	assert.Falsef(t, isSet, "Container should not have environment variable %s", varName)
}

// Verify that an environment variable is set and has the expected value
func verifyEnvVar(t *testing.T, container *corev1.Container, key, value string) {
	gotValue, isSet := getContainerEnvVar(container, key)
	assert.Truef(t, isSet, "Container should have environment variable %s", key)
	assert.Equalf(t, gotValue, value, "Unexpected value for environment variable %s", key)
}

// Return a containers's environment variable value and whether or not it is set
func getContainerEnvVar(container *corev1.Container, varName string) (string, bool) {
	for _, e := range container.Env {
		if e.Name == varName {
			return e.Value, true
		}
	}
	return "", false
}

func verifyPodBasedOnTemplate(t *testing.T, pod corev1.PodSpec, podMeta metav1.ObjectMeta, template *corev1.PodTemplateSpec) {
	assert.Len(t, pod.Containers, len(template.Spec.Containers), "Number of containers should match template")

	// Check pod has all labels from the template
	for k, v := range template.Labels {
		assert.Equal(t, v, podMeta.Labels[k], fmt.Sprintf("Pod label %s should match template", k))
	}

	// Check container properties
	container := pod.Containers[0]
	templateContainer := template.Spec.Containers[0]
	assert.Equal(t, templateContainer.Name, container.Name, "Container name should match template")
	assert.Equal(t, templateContainer.Image, container.Image, "Container image should match template")

	// Check our pod has all environment variables from the template
	for _, v := range templateContainer.Env {
		verifyEnvVar(t, &container, v.Name, v.Value)
	}

	// Check our pod has all args from the template
	for _, a := range templateContainer.Args {
		assert.Contains(t, container.Args, a, "Container args should include template args")
	}
}

func verifyTemplateAnnotation(t *testing.T, objMeta metav1.ObjectMeta, checksum string) {
	assert.Equal(t, checksum, objMeta.Annotations[testKeys.TemplateChecksum], "Deployment template should be labelled with template checksum")
}

func verifyPoolProxyVolume(t *testing.T, pod *corev1.PodSpec, proxyID int) {
	volumes := pod.Volumes
	require.Len(t, volumes, 1, "One volume should be added when UseSecureCommunication and UsePoolProxy are both true")
	expectedSecretName := fmt.Sprintf("%s%d", poolProxyPrefix, proxyID)
	vol := volumes[0]
	assert.Equal(t, certVolume, vol.Name, "Volume name should match configured cert volume name")
	assert.Equal(t, expectedSecretName, vol.Secret.SecretName, "Volume secret name should match proxy certificate secret name")
}

func verifyLabelSelector(t *testing.T, dep *appsv1.Deployment) {
	for k, v := range dep.Spec.Selector.MatchLabels {
		assert.Equal(t, v, dep.Spec.Template.Labels[k], "Label selector should match pod template labels")
	}
}

func newLogger(t *testing.T) *logging.Logger {
	return logging.NewFromZapLogger(zaptest.NewLogger(t))
}

// Get the annotations the worker template needs to have
func getRequiredWorkerAnnotations(usePoolProxy, useSecureCommunication bool) map[string]string {
	annotations := map[string]string{
		testKeys.CertVolume:             certVolume,
		testKeys.LogDir:                 logDir,
		testKeys.PoolProxyBasePort:      fmt.Sprintf("%d", poolProxyBasePort),
		testKeys.PoolProxyPrefix:        poolProxyPrefix,
		testKeys.UsePoolProxy:           fmt.Sprintf("%t", usePoolProxy),
		testKeys.UseSecureCommunication: fmt.Sprintf("%t", useSecureCommunication),
		testKeys.WorkerDomain:           workerDomain,
		testKeys.WorkersPerPoolProxy:    fmt.Sprintf("%d", workersPerPoolProxy),
		testKeys.WorkerPrefix:           workerPrefix,
	}
	if usePoolProxy {

	}
	return annotations
}

// Get the annotations the pool proxy template needs to have
func getRequiredPoolProxyAnnotations(useSecureCommunication bool) map[string]string {
	return map[string]string{
		testKeys.CertVolume:             certVolume,
		testKeys.LogDir:                 logDir,
		testKeys.PoolProxyBasePort:      fmt.Sprintf("%d", poolProxyBasePort),
		testKeys.PoolProxyPrefix:        poolProxyPrefix,
		testKeys.UseSecureCommunication: fmt.Sprintf("%t", useSecureCommunication),
	}
}

func newSpecFactory(t *testing.T, workerTmpl, proxyTmpl *corev1.PodTemplateSpec, checksum string) resize.SpecFactory {
	mockStore := mocks.NewMockTemplateStore(t)
	mockStore.EXPECT().GetWorkerPodTemplate().Return(workerTmpl, checksum).Maybe()
	mockStore.EXPECT().GetPoolProxyPodTemplate().Return(proxyTmpl, checksum).Maybe()
	return specs.NewFactory(testKeys, mockStore, newLogger(t))
}

func getWorkerPodSpecFromTemplate(t *testing.T, tmpl *corev1.PodTemplateSpec, workerID int) (*corev1.Pod, string) {
	checksum := "abc"
	sf := newSpecFactory(t, tmpl, nil, checksum)
	return getWorkerPodSpec(t, sf, workerID), checksum
}

func getWorkerPodSpec(t *testing.T, sf resize.SpecFactory, workerID int) *corev1.Pod {
	spec, err := sf.GetWorkerPodSpec(workerID)
	require.NoError(t, err, "Failed to get worker pod template")
	return spec
}

func getProxyDeploymentSpecFromTemplate(t *testing.T, tmpl *corev1.PodTemplateSpec, proxyID int) (*appsv1.Deployment, string) {
	checksum := "abcde"
	sf := newSpecFactory(t, nil, tmpl, checksum)
	return getProxyDeploymentSpec(t, sf, proxyID), checksum
}

func getProxyDeploymentSpec(t *testing.T, sf resize.SpecFactory, proxyID int) *appsv1.Deployment {
	spec, err := sf.GetPoolProxyDeploymentSpec(proxyID)
	require.NoError(t, err, "Failed to get proxy deployment template")
	return spec
}
