// Copyright 2024-2025 The MathWorks, Inc.
package k8swrapper_test

import (
	"context"
	"controller/internal/k8s"
	"controller/internal/k8swrapper"
	"controller/internal/logging"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	namespace      = "test-namespace"
	controllerName = "mjs-controller"
	controllerUID  = types.UID("abcd")
)

// Test the deployment methods
func TestDeployments(t *testing.T) {
	client, _ := newFakeClient(t)
	depToCreate := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "deployment1",
		},
	}

	// Test deployment creation
	createdDep, err := client.CreateDeployment(&depToCreate)
	require.NoError(t, err)
	assert.Equal(t, depToCreate.Name, createdDep.Name)
	assert.Equal(t, namespace, createdDep.Namespace, "deployment should have been created in test namespace")
	verifyHasOwner(t, createdDep.ObjectMeta)

	// Test getting deployment
	gotDep, err := client.GetDeployment(depToCreate.Name)
	require.NoError(t, err)
	assert.Equal(t, createdDep, gotDep)

	// Test the DeploymentExists method
	exists, err := client.DeploymentExists(createdDep.Name)
	require.NoError(t, err)
	assert.True(t, exists, "deployment should exist")
	existsFalse, err := client.DeploymentExists("not-a-deployment")
	require.NoError(t, err)
	assert.False(t, existsFalse, "deployment should not exist")

	// Check we can update the deployment
	annotationKey := "test"
	annotationVal := "value"
	gotDep.Annotations = map[string]string{
		annotationKey: annotationVal,
	}
	updatedDep, err := client.UpdateDeployment(gotDep)
	require.NoError(t, err)
	assert.Equal(t, annotationVal, updatedDep.Annotations[annotationKey], "deployment annotation should be updated")

	// Create a deployment with a label
	labelKey := "myKey"
	labelVal := "myVal"
	depWithLabels := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "deployment-labelled",
			Labels: map[string]string{
				labelKey: labelVal,
			},
		},
	}
	dep2, err := client.CreateDeployment(&depWithLabels)
	require.NoError(t, err)
	verifyHasOwner(t, dep2.ObjectMeta)

	// Verify that we can get the labelled deployment
	labelSelector := fmt.Sprintf("%s=%s", labelKey, labelVal)
	gotDeps, err := client.GetDeploymentsWithLabel(labelSelector)
	require.NoError(t, err)
	assert.Len(t, gotDeps.Items, 1, "should have found 1 deployment with label")
	assert.Equal(t, depWithLabels.Name, gotDeps.Items[0].Name, "should have found deployment with label")

	// Test deployment deletion
	err = client.DeleteDeployment(depWithLabels.Name)
	require.NoError(t, err)
	gotDeps, err = client.GetDeploymentsWithLabel(labelSelector)
	require.NoError(t, err)
	assert.Empty(t, gotDeps.Items, "should not have found any deployments with label after deletion")
	err = client.DeleteDeployment(depToCreate.Name)
	require.NoError(t, err)
	_, err = client.GetDeployment(depToCreate.Name)
	assert.Error(t, err, "should get an error when attempting to get deleted deployment")
}

// Test the pod methods
func TestPods(t *testing.T) {
	client, _ := newFakeClient(t)
	podToCreate := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod1",
		},
	}

	// Test pod creation
	createdPod, err := client.CreatePod(&podToCreate)
	require.NoError(t, err)
	assert.Equal(t, podToCreate.Name, createdPod.Name)
	assert.Equal(t, namespace, createdPod.Namespace, "pod should have been created in test namespace")
	verifyHasOwner(t, createdPod.ObjectMeta)

	// Test getting pod
	gotPod, err := client.GetPod(podToCreate.Name)
	require.NoError(t, err)
	assert.Equal(t, createdPod, gotPod)

	// Test pod deletion
	err = client.DeletePod(podToCreate.Name)
	require.NoError(t, err)
	_, err = client.GetPod(podToCreate.Name)
	require.Error(t, err, "Expected error when pod has been deleted")
}

// Test the service methods
func TestServices(t *testing.T) {
	// Create a service
	client, fakeK8s := newFakeClient(t)
	svcSpec := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
		},
	}
	createdSvc, err := fakeK8s.CoreV1().Services(namespace).Create(context.Background(), &svcSpec, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to add service")

	// Test getting the service
	gotSvc, err := client.GetService(svcSpec.Name)
	require.NoError(t, err)
	assert.Equal(t, createdSvc, gotSvc)

	// Test checking that the service exists
	gotSvc2, exists, err := client.ServiceExists(svcSpec.Name)
	require.NoError(t, err)
	assert.True(t, exists, "service should exist")
	assert.Equal(t, gotSvc, gotSvc2, "ServiceExists should return the service if it exists")

	// Test checking a nonexistant service
	_, exists, err = client.ServiceExists("not-real")
	require.NoError(t, err)
	assert.False(t, exists, "service should not exist")

	// Create a service with a label
	labelKey := "myKey"
	labelVal := "myVal"
	svcWithLabel := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "svc-labelled",
			Labels: map[string]string{
				labelKey: labelVal,
			},
		},
	}
	_, err = fakeK8s.CoreV1().Services(namespace).Create(context.Background(), &svcWithLabel, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to add service")

	// Verify that we can get the labelled service
	labelSelector := fmt.Sprintf("%s=%s", labelKey, labelVal)
	gotDeps, err := client.GetServicesWithLabel(labelSelector)
	require.NoError(t, err)
	assert.Len(t, gotDeps.Items, 1, "should have found 1 service with label")
	assert.Equal(t, svcWithLabel.Name, gotDeps.Items[0].Name, "should have found service with label")
}

// Test the secret methods
func TestSecrets(t *testing.T) {
	// Test secret creation
	client, _ := newFakeClient(t)
	secretName := "test-secret"
	data := map[string][]byte{
		"key": []byte("value"),
	}
	createdSecret, err := client.CreateSecret(secretName, data, false)
	require.NoError(t, err)
	assert.Equal(t, secretName, createdSecret.Name)
	assert.Equal(t, namespace, createdSecret.Namespace, "secret should have been created in test namespace")
	verifyHasOwner(t, createdSecret.ObjectMeta)

	// Test getting a secret
	gotSecret, err := client.GetSecret(secretName)
	require.NoError(t, err)
	assert.Equal(t, createdSecret, gotSecret)

	// Test checking whether secret exists
	existingSecret, existsTrue, err := client.SecretExists(secretName)
	require.NoError(t, err)
	assert.True(t, existsTrue, "secret should exist")
	assert.NotNil(t, existingSecret)
	notExistingSecret, existsFalse, err := client.SecretExists("not-a-secret")
	require.NoError(t, err)
	assert.False(t, existsFalse, "secret should not exist")
	assert.Nil(t, notExistingSecret)

	// Test deleting a secret
	err = client.DeleteSecret(secretName)
	require.NoError(t, err)
	_, exists, err := client.SecretExists(secretName)
	require.NoError(t, err)
	assert.False(t, exists, "secret should no longer exist after deletion")
}

// Test creating a preserve secret that should not be deleted along with the controller
func TestPreservedSecret(t *testing.T) {
	client, _ := newFakeClient(t)
	secretName := "test-preserve-secret"
	data := map[string][]byte{
		"key": []byte("value"),
	}
	createdSecret, err := client.CreateSecret(secretName, data, true)
	require.NoError(t, err)
	assert.Equal(t, secretName, createdSecret.Name)
	assert.Empty(t, createdSecret.OwnerReferences, "Preserved secret should not have owner references")
}

func TestGetPodsWithLabel(t *testing.T) {
	podNoLabel := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "no-label",
		},
	}
	labelKey := "myPod"
	labelVal := "labelled"
	podWithLabel := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "with-label",
			Labels: map[string]string{
				labelKey: labelVal,
			},
		},
	}

	// Create the pods on a fake K8s backend
	client, k8sBackend := newFakeClient(t)
	_, err := k8sBackend.CoreV1().Pods(namespace).Create(context.Background(), &podNoLabel, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = k8sBackend.CoreV1().Pods(namespace).Create(context.Background(), &podWithLabel, metav1.CreateOptions{})
	require.NoError(t, err)

	// Check we can get the expected pod
	labelSelector := fmt.Sprintf("%s=%s", labelKey, labelVal)
	gotPods, err := client.GetPodsWithLabel(labelSelector)
	require.NoError(t, err)
	assert.Len(t, gotPods.Items, 1, "should have found 1 pod with label")
	assert.Equal(t, podWithLabel.Name, gotPods.Items[0].Name, "did not find expected pod with label")
}

// Test the pod readiness functions
func TestPodReady(t *testing.T) {
	client, fakeK8s := newFakeClient(t)
	labelKey := "app"
	labelVal := "ready"
	pod := createFakePod(t, fakeK8s, "ready", labelKey, labelVal)

	containerName := "ready-container"
	pod = updatePodReadiness(t, fakeK8s, pod, containerName, true)
	verifyPodReadinessFuncs(t, client, true, pod, labelKey, labelVal, containerName)
}

// Test the pod readiness functions when no pod exists
func TestPodReadyNoPod(t *testing.T) {
	client, _ := newFakeClient(t)
	verifyPodReadinessFuncs(t, client, false, nil, "app", "pod", "mycontainer")
}

// Test the pod readiness functions when the container is not ready
func TestContainerNotReady(t *testing.T) {
	client, fakeK8s := newFakeClient(t)
	labelKey := "app"
	labelVal := "nonready"
	pod := createFakePod(t, fakeK8s, "nonready", labelKey, labelVal)

	containerName := "nonready-container"
	pod = updatePodReadiness(t, fakeK8s, pod, containerName, false)
	verifyPodReadinessFuncs(t, client, false, pod, labelKey, labelVal, containerName)
}

// Test the pod readiness functions when the pod doesn't have a container yet
func TestContainerNoContainer(t *testing.T) {
	client, fakeK8s := newFakeClient(t)
	labelKey := "app"
	labelVal := "no-container"
	pod := createFakePod(t, fakeK8s, "nocontainer", labelKey, labelVal)
	require.Empty(t, pod.Spec.Containers, "Pod should initially have no containers")

	verifyPodReadinessFuncs(t, client, false, pod, labelKey, labelVal, "mycontainer")
}

// Test the pod readiness functions when there are multiple pods
// (this can occur if one pod hasn't been cleaned up yet after a restart)
func TestReadinessMultiplePods(t *testing.T) {
	client, fakeK8s := newFakeClient(t)
	labelKey := "app"
	labelVal := "many"
	oldPod := createFakePod(t, fakeK8s, "old", labelKey, labelVal)
	newPod := createFakePod(t, fakeK8s, "new", labelKey, labelVal)

	// Set the new pod to be ready
	containerName := "mycontainer"
	updatePodReadiness(t, fakeK8s, oldPod, containerName, false)
	newPod = updatePodReadiness(t, fakeK8s, newPod, containerName, true)

	// Verify that the new pod shows as ready
	verifyPodReadinessFuncs(t, client, true, newPod, labelKey, labelVal, containerName)
}

// Should get an error if we try to create the client when owner deployment doesn't exist
func TestNewOwnerNotFound(t *testing.T) {
	fakeK8s := fake.NewSimpleClientset()
	ownerName := "owner-not-found"
	conf := k8swrapper.K8sConfig{
		Namespace:           namespace,
		OwnerDeploymentName: ownerName,
	}
	logger := logging.NewFromZapLogger(zaptest.NewLogger(t))

	_, err := k8swrapper.NewClientWrapper(conf, nil, fakeK8s, logger)
	require.Error(t, err, "Expect error when client cannot find owner deployment")
	assert.Contains(t, err.Error(), ownerName, "Error should mention missing owner name")
}

// Create a Client with a fake underlying Kubernetes client
func newFakeClient(t *testing.T) (k8s.Client, *fake.Clientset) {
	fakeK8s := fake.NewSimpleClientset()
	conf := k8swrapper.K8sConfig{
		Namespace:           namespace,
		OwnerDeploymentName: controllerName,
	}

	// Create a fake controller deployment for us to fetch UID from
	controllerDep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: controllerName,
			UID:  controllerUID,
		},
	}
	_, err := fakeK8s.AppsV1().Deployments(namespace).Create(context.Background(), &controllerDep, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to add controller deployment")

	logger := logging.NewFromZapLogger(zaptest.NewLogger(t))

	client, err := k8swrapper.NewClientWrapper(conf, nil, fakeK8s, logger)
	require.NoError(t, err, "Error creating K8s client")
	return client, fakeK8s
}

// Create a pod and install it onto the fake K8s backend
func createFakePod(t *testing.T, fakeK8s *fake.Clientset, name, labelKey, labelVal string) *corev1.Pod {
	podSpec := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				labelKey: labelVal,
			},
		},
	}
	pod, err := fakeK8s.CoreV1().Pods(namespace).Create(context.Background(), &podSpec, metav1.CreateOptions{})
	require.NoError(t, err)
	return pod
}

func verifyPodReadinessFuncs(t *testing.T, client k8s.Client, expectReady bool, expectedPod *corev1.Pod, labelKey, labelVal, containerName string) {
	label := labelKey + "=" + labelVal
	ready, err := client.IsPodReady(label, containerName)
	require.NoError(t, err)
	assert.Equal(t, expectReady, ready)

	pod, err := client.GetReadyPod(label, containerName)
	if expectReady {
		require.NoError(t, err)
		assert.Equal(t, expectedPod, pod, "GetReadyPod did not return the expected pod")
	} else {
		assert.Error(t, err, "expect error from GetReadyPod when the pod is not ready")
	}
}

// Update a pod with a given readiness status
func updatePodReadiness(t *testing.T, fakeK8s *fake.Clientset, podSpec *corev1.Pod, containerName string, ready bool) *corev1.Pod {
	podSpec.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name:  containerName,
			Ready: ready,
		},
	}
	newPod, err := fakeK8s.CoreV1().Pods(namespace).Update(context.Background(), podSpec, metav1.UpdateOptions{})
	require.NoError(t, err)
	return newPod
}

func verifyHasOwner(t *testing.T, meta metav1.ObjectMeta) {
	require.NotEmpty(t, meta.OwnerReferences, "Expected owner references to be set")
	require.Len(t, meta.OwnerReferences, 1, "Expected exactly one owner reference")

	ownerRef := meta.OwnerReferences[0]
	assert.Equal(t, "apps/v1", ownerRef.APIVersion, "Incorrect owner API version")
	assert.Equal(t, "Deployment", ownerRef.Kind, "Incorrect owner kind")
	assert.Equal(t, controllerUID, ownerRef.UID, "Incorrect owner UID")
	assert.Equal(t, controllerName, ownerRef.Name, "Incorrect owner name")
}
