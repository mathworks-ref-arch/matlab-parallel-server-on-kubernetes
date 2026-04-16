// Copyright 2025 The MathWorks, Inc.
package k8swrapper

import (
	"bytes"
	"context"
	"controller/internal/k8s"
	"controller/internal/logging"
	"fmt"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// Wrapper around a kubernetes.Interface client that implements our own k8s.Client interface
type K8sClientWrapper struct {
	client     kubernetes.Interface
	namespace  string
	kubeConfig *rest.Config
	ownerRefs  []metav1.OwnerReference
	logger     *logging.Logger
	timeout    time.Duration
}

type K8sConfig struct {
	OwnerDeploymentName string
	Namespace           string
	TimeoutSecs         int
}

// Create a new client wrapper
func NewClientWrapper(conf K8sConfig, kubeConfig *rest.Config, k8sClient kubernetes.Interface, logger *logging.Logger) (k8s.Client, error) {
	// Create owner reference to apply to the resources we create.
	// This ensures they are torn down by the Kubernetes garbage collector when the helm chart is uninstalled.
	timeout := time.Duration(conf.TimeoutSecs) * time.Second
	thisDeployment, err := getDeployment(k8sClient, timeout, conf.OwnerDeploymentName, conf.Namespace)
	if err != nil {
		return nil, fmt.Errorf("error getting owner deployment '%s': %v", conf.OwnerDeploymentName, err)
	}
	ownerRefs := []metav1.OwnerReference{
		{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       thisDeployment.Name,
			UID:        thisDeployment.UID,
		},
	}

	return &K8sClientWrapper{
		client:     k8sClient,
		kubeConfig: kubeConfig,
		namespace:  conf.Namespace,
		ownerRefs:  ownerRefs,
		logger:     logger,
		timeout:    timeout,
	}, nil
}

func (c *K8sClientWrapper) CreateDeployment(spec *appsv1.Deployment) (*appsv1.Deployment, error) {
	c.logger.Debug("creating Kubernetes Deployment", zap.String("name", spec.Name))
	spec.OwnerReferences = c.ownerRefs
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.AppsV1().Deployments(c.namespace).Create(ctx, spec, metav1.CreateOptions{})
}

func (c *K8sClientWrapper) GetDeployment(name string) (*appsv1.Deployment, error) {
	return getDeployment(c.client, c.timeout, name, c.namespace)
}

func (c *K8sClientWrapper) UpdateDeployment(spec *appsv1.Deployment) (*appsv1.Deployment, error) {
	c.logger.Debug("upgrading Kubernetes Deployment", zap.String("name", spec.Name))
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.AppsV1().Deployments(c.namespace).Update(ctx, spec, metav1.UpdateOptions{})
}

func (c *K8sClientWrapper) DeleteDeployment(name string) error {
	c.logger.Debug("deleting Kubernetes Deployment", zap.String("name", name))
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.AppsV1().Deployments(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *K8sClientWrapper) CreatePod(spec *corev1.Pod) (*corev1.Pod, error) {
	c.logger.Debug("creating Kubernetes Pod", zap.String("name", spec.Name))
	spec.OwnerReferences = c.ownerRefs
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.CoreV1().Pods(c.namespace).Create(ctx, spec, metav1.CreateOptions{})
}

func (c *K8sClientWrapper) GetPod(name string) (*corev1.Pod, error) {
	ctx, cancel := newContext(c.timeout)
	defer cancel()
	return c.client.CoreV1().Pods(c.namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *K8sClientWrapper) DeletePod(name string) error {
	c.logger.Debug("deleting Kubernetes Pod", zap.String("name", name))
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.CoreV1().Pods(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *K8sClientWrapper) DeploymentExists(name string) (bool, error) {
	_, err := c.GetDeployment(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *K8sClientWrapper) GetService(name string) (*corev1.Service, error) {
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.CoreV1().Services(c.namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *K8sClientWrapper) ServiceExists(name string) (*corev1.Service, bool, error) {
	svc, err := c.GetService(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return svc, true, nil
}

func (c *K8sClientWrapper) CreateSecret(name string, data map[string][]byte, preserve bool) (*corev1.Secret, error) {
	c.logger.Debug("creating Kubernetes Secret", zap.String("name", name))
	spec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: data,
	}
	if !preserve {
		spec.OwnerReferences = c.ownerRefs
	}
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.CoreV1().Secrets(c.namespace).Create(ctx, spec, metav1.CreateOptions{})
}

func (c *K8sClientWrapper) UpdateSecret(name string, data map[string][]byte, preserve bool) (*corev1.Secret, error) {
	c.logger.Debug("updating Kubernetes Secret", zap.String("name", name))
	spec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: data,
	}
	if !preserve {
		spec.OwnerReferences = c.ownerRefs
	}
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.CoreV1().Secrets(c.namespace).Update(ctx, spec, metav1.UpdateOptions{})
}

func (c *K8sClientWrapper) GetSecret(name string) (*corev1.Secret, error) {
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.CoreV1().Secrets(c.namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *K8sClientWrapper) DeleteSecret(name string) error {
	c.logger.Debug("deleting Kubernetes Secret", zap.String("name", name))
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.CoreV1().Secrets(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *K8sClientWrapper) SecretExists(name string) (*corev1.Secret, bool, error) {
	secret, err := c.GetSecret(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return secret, true, nil
}

func (c *K8sClientWrapper) GetConfigMap(name string) (*corev1.ConfigMap, error) {
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	return c.client.CoreV1().ConfigMaps(c.namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *K8sClientWrapper) GetPodsWithLabel(label string) (*corev1.PodList, error) {
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	pods, err := c.client.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting pods with label '%s': %v", label, err)
	}
	return pods, nil
}

func (c *K8sClientWrapper) GetDeploymentsWithLabel(label string) (*appsv1.DeploymentList, error) {
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	deployments, err := c.client.AppsV1().Deployments(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting deployments with label '%s': %v", label, err)
	}
	return deployments, nil
}

func (c *K8sClientWrapper) GetServicesWithLabel(label string) (*corev1.ServiceList, error) {
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	services, err := c.client.CoreV1().Services(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting services with label '%s': %v", label, err)
	}
	return services, nil
}

// IsPodReady returns true if a pod with a given label is ready
func (c *K8sClientWrapper) IsPodReady(label, containerName string) (bool, error) {
	c.logger.Debug("checking pod readiness", zap.String("label", label))
	pods, err := c.GetPodsWithLabel(label)
	if err != nil {
		return false, err
	}
	_, isReady := findReadyPod(pods, containerName)
	return isReady, nil
}

// GetReadyPod gets a pod with a ready container and returns an error if no pod is ready
func (c *K8sClientWrapper) GetReadyPod(label, containerName string) (*corev1.Pod, error) {
	pods, err := c.GetPodsWithLabel(label)
	if err != nil {
		return nil, err
	}
	readyPod, isReady := findReadyPod(pods, containerName)
	if !isReady {
		return nil, fmt.Errorf("found %d pods with label '%s', but none were ready", len(pods.Items), label)
	}
	return readyPod, nil
}

// Execute a command on a pod and return the stdout in a byte buffer
func (c *K8sClientWrapper) ExecOnPod(podName, containerName string, cmd []string) (*bytes.Buffer, error) {
	// Create REST request
	req := c.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(c.namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Command:   cmd,
		Stdout:    true,
		Stderr:    true,
		Container: containerName,
	}, scheme.ParameterCodec)

	// Create executor
	exec, err := remotecommand.NewSPDYExecutor(c.kubeConfig, "POST", req.URL())
	if err != nil {
		return nil, err
	}

	// Stream results
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	ctx, cancelFunc := newContext(c.timeout)
	defer cancelFunc()
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdoutBuf,
		Stderr: stderrBuf,
	})
	if err != nil {
		return nil, fmt.Errorf("error executing remote command: %v, stdout: %s, stderr: %s", err, stdoutBuf.String(), stderrBuf.String())
	}
	return stdoutBuf, nil
}

// Return a pod from a list of pods that has the expected container in the "ready" state
func findReadyPod(pods *corev1.PodList, containerName string) (*corev1.Pod, bool) {
	numPods := len(pods.Items)
	if numPods == 0 {
		// No pods yet
		return nil, false
	}

	// There can be multiple pods if one was recently terminated and another has started; find the pod that is ready
	for _, pod := range pods.Items {
		isReady, _ := hasReadyContainer(&pod, containerName)
		if isReady {
			return &pod, true
		}
	}
	return nil, false
}

// hasReadyContainer returns true if the specified container is found in a pod, and that container is ready
// An error is returned if the specified container is not found
func hasReadyContainer(pod *corev1.Pod, containerName string) (bool, error) {
	for _, container := range pod.Status.ContainerStatuses {
		if container.Name == containerName {
			return container.Ready, nil
		}
	}
	return false, fmt.Errorf("container %s not found in pod %s", containerName, pod.Name)
}

// newContext creates a context to use when calling the K8s client
func newContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

func getDeployment(client kubernetes.Interface, timeout time.Duration, name, namespace string) (*appsv1.Deployment, error) {
	ctx, cancel := newContext(timeout)
	defer cancel()
	return client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}
