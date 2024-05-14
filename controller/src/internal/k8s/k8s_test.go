// Copyright 2024 The MathWorks, Inc.
package k8s

import (
	"context"
	"controller/internal/config"
	"controller/internal/logging"
	"controller/internal/specs"
	"fmt"
	"testing"

	"github.com/google/uuid"
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
	namespace     = "test-namespace"
	jobManagerUID = "jm-1234"
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
	_, err = client.CreateDeployment(&depWithLabels)
	require.NoError(t, err)

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

// Test the service methods
func TestServices(t *testing.T) {
	// Test service creation
	client, _ := newFakeClient(t)
	svcSpec := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
		},
	}
	createdSvc, err := client.CreateService(&svcSpec)
	require.NoError(t, err)
	assert.Equal(t, svcSpec.Name, createdSvc.Name)
	assert.Equal(t, namespace, createdSvc.Namespace, "service should have been created in test namespace")

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

	// Test updating the service
	svcSpec.Spec.Ports = append(svcSpec.Spec.Ports, corev1.ServicePort{Port: 8080})
	err = client.UpdateService(&svcSpec)
	require.NoError(t, err)
	gotSvcAfterUpdate, err := client.GetService(svcSpec.Name)
	require.NoError(t, err)
	assert.NotEqual(t, gotSvc, gotSvcAfterUpdate)
	assert.Equal(t, svcSpec.Spec, gotSvcAfterUpdate.Spec)

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
	_, err = client.CreateService(&svcWithLabel)
	require.NoError(t, err)

	// Verify that we can get the labelled service
	labelSelector := fmt.Sprintf("%s=%s", labelKey, labelVal)
	gotDeps, err := client.GetServicesWithLabel(labelSelector)
	require.NoError(t, err)
	assert.Len(t, gotDeps.Items, 1, "should have found 1 service with label")
	assert.Equal(t, svcWithLabel.Name, gotDeps.Items[0].Name, "should have found service with label")

	// Test service deletion
	err = client.DeleteService(svcSpec.Name)
	require.NoError(t, err)
	_, err = client.GetService(svcSpec.Name)
	assert.Error(t, err, "should get an error when attempting to get deleted service")
}

// Test the secret methods
func TestSecrets(t *testing.T) {
	// Test secret creation
	client, _ := newFakeClient(t)
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret",
		},
	}
	createdSecret, err := client.CreateSecret(&secret)
	require.NoError(t, err)
	assert.Equal(t, secret.Name, createdSecret.Name)
	assert.Equal(t, namespace, createdSecret.Namespace, "secret should have been created in test namespace")

	// Test getting a secret
	gotSecret, err := client.GetSecret(secret.Name)
	require.NoError(t, err)
	assert.Equal(t, createdSecret, gotSecret)

	// Test checking whether secret exists
	existingSecret, existsTrue, err := client.SecretExists(secret.Name)
	require.NoError(t, err)
	assert.True(t, existsTrue, "secret should exist")
	assert.NotNil(t, existingSecret)
	notExistingSecret, existsFalse, err := client.SecretExists("not-a-secret")
	require.NoError(t, err)
	assert.False(t, existsFalse, "secret should not exist")
	assert.Nil(t, notExistingSecret)

	// Test deleting a secret
	err = client.DeleteSecret(secret.Name)
	require.NoError(t, err)
	_, exists, err := client.SecretExists(secret.Name)
	require.NoError(t, err)
	assert.False(t, exists, "secret should no longer exist after deletion")
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
	_, err := k8sBackend.CoreV1().Pods(client.config.Namespace).Create(context.Background(), &podNoLabel, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = k8sBackend.CoreV1().Pods(client.config.Namespace).Create(context.Background(), &podWithLabel, metav1.CreateOptions{})
	require.NoError(t, err)

	// Check we can get the expected pod
	labelSelector := fmt.Sprintf("%s=%s", labelKey, labelVal)
	gotPods, err := client.GetPodsWithLabel(labelSelector)
	require.NoError(t, err)
	assert.Len(t, gotPods.Items, 1, "should have found 1 pod with label")
	assert.Equal(t, podWithLabel.Name, gotPods.Items[0].Name, "did not find expected pod with label")
}

func TestGetLoadBalancer(t *testing.T) {
	// Create a mock MJS LoadBalancer
	client, _ := newFakeClient(t)
	lbName := "mjs-load-balancer"
	client.config.LoadBalancerName = lbName
	lbSpec := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: lbName,
		},
	}
	lb, err := client.CreateService(&lbSpec)
	require.NoError(t, err)

	// Verify that we can get the load balancer
	gotLB, err := client.GetLoadBalancer()
	require.NoError(t, err)
	assert.Equal(t, lb, gotLB)
}

// Test the job manager pod readiness functions
func TestJobManagerReady(t *testing.T) {
	client, fakeK8s := newFakeClient(t)
	jmPod := createFakeJobManagerPod(t, fakeK8s)
	jmPod = updateJobManagerPodReadiness(t, fakeK8s, jmPod, true)
	verifyJobManagerFuncs(t, client, true, jmPod)
}

// Test the job manager pod functions when the container is not ready
func TestJobManagerContainerNotReady(t *testing.T) {
	client, fakeK8s := newFakeClient(t)
	jmPod := createFakeJobManagerPod(t, fakeK8s)
	jmPod = updateJobManagerPodReadiness(t, fakeK8s, jmPod, false)
	verifyJobManagerFuncs(t, client, false, jmPod)
}

// Test the job manager pod functions when there are multiple pods
// (this can occur if one pod hasn't been cleaned up yet after a restart)
func TestJobManagerMultiplePods(t *testing.T) {
	client, fakeK8s := newFakeClient(t)
	oldJmPod := createFakeJobManagerPod(t, fakeK8s)
	newJmPod := createFakeJobManagerPod(t, fakeK8s)

	// Set the new pod to be ready
	updateJobManagerPodReadiness(t, fakeK8s, oldJmPod, false)
	newJmPod = updateJobManagerPodReadiness(t, fakeK8s, newJmPod, true)

	// Verify that the new pod shows as ready
	verifyJobManagerFuncs(t, client, true, newJmPod)
}

// Test getting the UID of the controller deployment
func TestGetDeploymentUID(t *testing.T) {
	depName := "my-controller"
	client, _ := newFakeClient(t)
	client.config.DeploymentName = depName

	// Create mock controller deployment
	depUID := types.UID("abcd")
	spec := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: depName,
			UID:  depUID,
		},
	}
	_, err := client.CreateDeployment(&spec)
	require.NoError(t, err)

	// Check we can retrieve the UID
	gotUID, err := client.GetControllerDeploymentUID()
	require.NoError(t, err)
	assert.Equal(t, depUID, gotUID, "did not retrieve expected controller deployment UID")
}

// Create a Client with a fake underlying Kubernetes client
func newFakeClient(t *testing.T) (*clientImpl, *fake.Clientset) {
	fakeK8s := fake.NewSimpleClientset()
	conf := config.Config{
		Namespace:     namespace,
		JobManagerUID: jobManagerUID,
	}
	return &clientImpl{
		client: fakeK8s,
		config: &conf,
		logger: logging.NewFromZapLogger(zaptest.NewLogger(t)),
	}, fakeK8s
}

// Create a fake job manager node pod and install it onto the fake K8s backend
func createFakeJobManagerPod(t *testing.T, fakeK8s *fake.Clientset) *corev1.Pod {
	podUID := uuid.New().String()
	podSpec := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("mjs-job-manager-%s", podUID),
			Labels: map[string]string{
				specs.JobManagerUIDKey: jobManagerUID,
			},
		},
	}
	jmPod, err := fakeK8s.CoreV1().Pods(namespace).Create(context.Background(), &podSpec, metav1.CreateOptions{})
	require.NoError(t, err)
	return jmPod
}

func verifyJobManagerFuncs(t *testing.T, client *clientImpl, expectReady bool, expectedPod *corev1.Pod) {
	ready, err := client.IsJobManagerReady()
	require.NoError(t, err)
	assert.Equal(t, expectReady, ready)

	pod, err := client.GetJobManagerPod()
	if expectReady {
		require.NoError(t, err)
		assert.Equal(t, expectedPod, pod, "GetJobManagerPod did not return the expected pod")
	} else {
		assert.Error(t, err, "expect error from GetJobManagerPod when the pod is not ready")
	}
}

// Update the job manager pod with a given readiness status
func updateJobManagerPodReadiness(t *testing.T, fakeK8s *fake.Clientset, podSpec *corev1.Pod, ready bool) *corev1.Pod {
	podSpec.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name:  specs.JobManagerHostname,
			Ready: ready,
		},
	}
	newPod, err := fakeK8s.CoreV1().Pods(namespace).Update(context.Background(), podSpec, metav1.UpdateOptions{})
	require.NoError(t, err)
	return newPod
}
