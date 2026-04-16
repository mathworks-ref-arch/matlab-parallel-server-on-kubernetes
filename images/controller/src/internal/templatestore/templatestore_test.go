// Copyright 2025-2026 The MathWorks, Inc.
package templatestore_test

import (
	"controller/internal/templatestore"
	mocks "controller/mocks/k8s"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewStore(t *testing.T) {
	mockK8s := mocks.NewMockClient(t)
	conf := getTestConfig()

	workerName := "myWorker"
	poolProxyName := "myPoolProxy"
	expectGetConfigMapForPodName(mockK8s, conf, workerName, conf.WorkerTemplateName)
	expectGetConfigMapForPodName(mockK8s, conf, poolProxyName, conf.PoolProxyTemplateName)

	store, err := templatestore.New(conf, mockK8s, nil)
	require.NoError(t, err, "Failed to create store")

	workerChecksum := store.GetWorkerTemplateChecksum()
	assert.NotEmpty(t, workerChecksum, "Initial worker pod template checksum should be set")
	proxyChecksum := store.GetPoolProxyTemplateChecksum()
	assert.NotEmpty(t, proxyChecksum, "Initial proxy pod template checksum should be set")
	assert.NotEqual(t, workerChecksum, proxyChecksum, "Worker and proxy template checksums should be different")

	assert.LessOrEqual(t, len(workerChecksum), templatestore.MAX_CHECKSUM_LENGTH, "Worker checksum string is too long")
	assert.LessOrEqual(t, len(proxyChecksum), templatestore.MAX_CHECKSUM_LENGTH, "Proxy checksum string is too long")

	// Check we get the expected pod templates
	gotWorker, gotWorkerChecksum := store.GetWorkerPodTemplate()
	verifyPodTemplate(t, gotWorker, workerName)
	assert.Equal(t, workerChecksum, gotWorkerChecksum, "Unexpected worker checksum")

	gotProxy, gotProxyChecksum := store.GetPoolProxyPodTemplate()
	verifyPodTemplate(t, gotProxy, poolProxyName)
	assert.Equal(t, proxyChecksum, gotProxyChecksum, "Unexpected proxy checksum")
}

func TestRefresh(t *testing.T) {
	mockK8s := mocks.NewMockClient(t)
	conf := getTestConfig()

	workerName := "myWorker"
	proxyName := "myProxy"
	expectGetConfigMapForPodName(mockK8s, conf, workerName, conf.WorkerTemplateName)
	expectGetConfigMapForPodName(mockK8s, conf, proxyName, conf.PoolProxyTemplateName)
	store, err := templatestore.New(conf, mockK8s, nil)
	require.NoError(t, err, "Failed to create store")

	// Store initial checksums
	initWorkerChecksum := store.GetWorkerTemplateChecksum()
	initProxyChecksum := store.GetPoolProxyTemplateChecksum()

	// Configure K8s to return a fresh worker template, but the same proxy template
	newName := "modifiedWorker"
	expectGetConfigMapForPodName(mockK8s, conf, newName, conf.WorkerTemplateName)
	expectGetConfigMapForPodName(mockK8s, conf, proxyName, conf.PoolProxyTemplateName)

	err = store.Refresh()
	require.NoError(t, err, "Failed to refresh templates")

	// Check we get the expected templates
	gotWorker, gotWorkerChecksum := store.GetWorkerPodTemplate()
	verifyPodTemplate(t, gotWorker, newName)
	assert.NotEqual(t, initWorkerChecksum, gotWorkerChecksum, "Worker pod template checksum after refresh should be different")

	gotProxy, gotProxyChecksum := store.GetPoolProxyPodTemplate()
	verifyPodTemplate(t, gotProxy, proxyName)
	assert.Equal(t, initProxyChecksum, gotProxyChecksum, "Proxy pod template checksum after refresh should be unchanged")
}

func TestNoPoolProxy(t *testing.T) {
	mockK8s := mocks.NewMockClient(t)
	conf := getTestConfig()
	conf.UsePoolProxy = false

	workerName := "myWorker"
	expectGetConfigMapForPodName(mockK8s, conf, workerName, conf.WorkerTemplateName)
	store, err := templatestore.New(conf, mockK8s, nil)
	require.NoError(t, err, "Failed to create store")

	// Should get nil if we try to fetch proxy template
	gotTemplate, _ := store.GetPoolProxyPodTemplate()
	assert.Nil(t, gotTemplate, "Should not be able to get pool proxy template when UsePoolProxy=false")

	// Check we can refresh
	expectGetConfigMapForPodName(mockK8s, conf, workerName, conf.WorkerTemplateName)
	err = store.Refresh()
	assert.NoError(t, err, "Refresh errored unexpectedly")
}

// Check we get an error if our template contains fields
// that are not part of a valid pod spec
func TestBadTemplate(t *testing.T) {
	mockK8s := mocks.NewMockClient(t)
	conf := getTestConfig()
	conf.UsePoolProxy = false

	name := "myWorker"
	badField := "notValidField"
	badTemplateTxt := fmt.Sprintf(`
metadata:
  labels:
    app: %s
spec:
  containers:
  - name: %s
    image: my-image
    %s: bad
`, name, name, badField)
	expectGetConfigMapForTemplate(mockK8s, conf, badTemplateTxt, conf.WorkerTemplateName)
	_, err := templatestore.New(conf, mockK8s, nil)
	require.Error(t, err, "Expected error when template is not a valid pod spec")
	assert.Contains(t, err.Error(), badField, "Error message should mention the invalid field")
}

func verifyPodTemplate(t *testing.T, template *corev1.PodTemplateSpec, name string) {
	assert.Equal(t, name, template.Labels["app"], "Unexpected pod template label")
	assert.Equal(t, name, template.Spec.Containers[0].Name, "Unexpected container name in pod template")
}

func expectGetConfigMapForPodName(mockK8s *mocks.MockClient, conf templatestore.StoreConfig, podName, configMapName string) {
	expectGetConfigMapForTemplate(mockK8s, conf, getTemplateTxt(podName), configMapName)
}

func expectGetConfigMapForTemplate(mockK8s *mocks.MockClient, conf templatestore.StoreConfig, templateTxt, configMapName string) {
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMapName,
		},
		Data: map[string]string{
			conf.TemplateFile: templateTxt,
		},
	}
	mockK8s.EXPECT().GetConfigMap(configMapName).Return(&configMap, nil).Once()
}

func getTestConfig() templatestore.StoreConfig {
	return templatestore.StoreConfig{
		Namespace:             "my-ns",
		UsePoolProxy:          true,
		WorkerTemplateName:    "worker-template",
		PoolProxyTemplateName: "proxy-template",
		TemplateFile:          "template.yaml",
	}
}

func getTemplateTxt(name string) string {
	return fmt.Sprintf(`
metadata:
  labels:
    app: %s
spec:
  containers:
  - name: %s
    image: my-image
`, name, name)
}
