// Package k8s contains methods for interacting with MJS resources in a Kubernetes cluster.
// Copyright 2024 The MathWorks, Inc.
package k8s

import (
	"bytes"
	"context"
	"controller/internal/config"
	"controller/internal/logging"
	"controller/internal/specs"
	"fmt"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

// Interface for interacting with Kubernetes resources
type Client interface {
	CreateDeployment(*appsv1.Deployment) (*appsv1.Deployment, error)
	GetDeployment(string) (*appsv1.Deployment, error)
	DeleteDeployment(string) error
	DeploymentExists(string) (bool, error)
	CreateService(*corev1.Service) (*corev1.Service, error)
	GetService(string) (*corev1.Service, error)
	UpdateService(*corev1.Service) error
	DeleteService(string) error
	ServiceExists(string) (*corev1.Service, bool, error)
	CreateSecret(*corev1.Secret) (*corev1.Secret, error)
	GetSecret(string) (*corev1.Secret, error)
	DeleteSecret(string) error
	SecretExists(string) (*corev1.Secret, bool, error)
	GetLoadBalancer() (*corev1.Service, error)
	GetPodsWithLabel(string) (*corev1.PodList, error)
	GetDeploymentsWithLabel(string) (*appsv1.DeploymentList, error)
	GetServicesWithLabel(string) (*corev1.ServiceList, error)
	IsJobManagerReady() (bool, error)
	GetJobManagerPod() (*corev1.Pod, error)
	ExecOnPod(string, []string) (*bytes.Buffer, error)
	GetControllerDeploymentUID() (types.UID, error)
}

// Implementation of clientImpl
type clientImpl struct {
	client     kubernetes.Interface
	config     *config.Config
	kubeConfig *rest.Config
	logger     *logging.Logger
}

// Create a new client
func NewClient(conf *config.Config, logger *logging.Logger) (Client, error) {
	kubeConfig, err := getKubeConfig(conf)
	if err != nil {
		return nil, err
	}
	k8sclientImpl, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating Kubernetes client: %v", err)
	}
	return &clientImpl{
		config:     conf,
		client:     k8sclientImpl,
		kubeConfig: kubeConfig,
		logger:     logger,
	}, nil
}

// Create a new client with a given Kubernetes backend client
func NewClientWithK8sBackend(conf *config.Config, k8sClient kubernetes.Interface, logger *logging.Logger) Client {
	return &clientImpl{
		config: conf,
		client: k8sClient,
		logger: logger,
	}
}

func (c *clientImpl) CreateDeployment(spec *appsv1.Deployment) (*appsv1.Deployment, error) {
	c.logger.Debug("creating Kubernetes Deployment", zap.String("name", spec.Name))
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	return c.client.AppsV1().Deployments(c.config.Namespace).Create(ctx, spec, metav1.CreateOptions{})
}

func (c *clientImpl) GetDeployment(name string) (*appsv1.Deployment, error) {
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	return c.client.AppsV1().Deployments(c.config.Namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *clientImpl) DeleteDeployment(name string) error {
	c.logger.Debug("deleting Kubernetes Deployment", zap.String("name", name))
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	return c.client.AppsV1().Deployments(c.config.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *clientImpl) DeploymentExists(name string) (bool, error) {
	_, err := c.GetDeployment(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *clientImpl) CreateService(spec *corev1.Service) (*corev1.Service, error) {
	c.logger.Debug("creating Kubernetes Service", zap.String("name", spec.Name))
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	return c.client.CoreV1().Services(c.config.Namespace).Create(ctx, spec, metav1.CreateOptions{})
}

func (c *clientImpl) GetService(name string) (*corev1.Service, error) {
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	return c.client.CoreV1().Services(c.config.Namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *clientImpl) UpdateService(spec *corev1.Service) error {
	c.logger.Debug("updating Kubernetes Service", zap.String("name", spec.Name))
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	_, err := c.client.CoreV1().Services(c.config.Namespace).Update(ctx, spec, metav1.UpdateOptions{})
	return err
}

func (c *clientImpl) DeleteService(name string) error {
	c.logger.Debug("deleting Kubernetes Service", zap.String("name", name))
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	return c.client.CoreV1().Services(c.config.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *clientImpl) ServiceExists(name string) (*corev1.Service, bool, error) {
	svc, err := c.GetService(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return svc, true, nil
}

func (c *clientImpl) CreateSecret(spec *corev1.Secret) (*corev1.Secret, error) {
	c.logger.Debug("creating Kubernetes Secret", zap.String("name", spec.Name))
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	return c.client.CoreV1().Secrets(c.config.Namespace).Create(ctx, spec, metav1.CreateOptions{})
}

func (c *clientImpl) GetSecret(name string) (*corev1.Secret, error) {
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	return c.client.CoreV1().Secrets(c.config.Namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *clientImpl) DeleteSecret(name string) error {
	c.logger.Debug("deleting Kubernetes Secret", zap.String("name", name))
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	return c.client.CoreV1().Secrets(c.config.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *clientImpl) SecretExists(name string) (*corev1.Secret, bool, error) {
	secret, err := c.GetSecret(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return secret, true, nil
}

// GetLoadBalancer gets the spec of the external LoadBalancer service for the MJS cluster
func (c *clientImpl) GetLoadBalancer() (*corev1.Service, error) {
	lbName := c.config.LoadBalancerName
	lbSpec, err := c.GetService(lbName)
	if err != nil {
		return nil, fmt.Errorf("error getting MJS LoadBalancer %s: %v", lbName, err)
	}
	return lbSpec, nil
}

func (c *clientImpl) GetPodsWithLabel(label string) (*corev1.PodList, error) {
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	pods, err := c.client.CoreV1().Pods(c.config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting pods with label '%s': %v", label, err)
	}
	return pods, nil
}

func (c *clientImpl) GetDeploymentsWithLabel(label string) (*appsv1.DeploymentList, error) {
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	deployments, err := c.client.AppsV1().Deployments(c.config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting deployments with label '%s': %v", label, err)
	}
	return deployments, nil
}

func (c *clientImpl) GetServicesWithLabel(label string) (*corev1.ServiceList, error) {
	ctx, cancelFunc := newContext()
	defer cancelFunc()
	services, err := c.client.CoreV1().Services(c.config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting services with label '%s': %v", label, err)
	}
	return services, nil
}

// IsJobManagerReady returns true if the job manager pod is ready
func (c *clientImpl) IsJobManagerReady() (bool, error) {
	pods, err := c.GetPodsWithLabel(c.getJobManagerPodLabel())
	if err != nil {
		return false, err
	}
	_, isReady := findReadyPod(pods, specs.JobManagerHostname)
	return isReady, nil
}

// GetJobManagerPod gets the job manager pod and returns an error if no job manager pod is ready
func (c *clientImpl) GetJobManagerPod() (*corev1.Pod, error) {
	pods, err := c.GetPodsWithLabel(c.getJobManagerPodLabel())
	if err != nil {
		return nil, err
	}
	readyPod, isReady := findReadyPod(pods, specs.JobManagerHostname)
	if !isReady {
		return nil, fmt.Errorf("found %d job manager pods, but none were ready", len(pods.Items))
	}
	return readyPod, nil
}

// Execute a command on a pod and return the stdout in a byte buffer
func (c *clientImpl) ExecOnPod(podName string, cmd []string) (*bytes.Buffer, error) {
	// Create REST request
	req := c.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(c.config.Namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Command:   cmd,
		Stdout:    true,
		Stderr:    true,
		Container: specs.JobManagerHostname,
	}, scheme.ParameterCodec)

	// Create executor
	exec, err := remotecommand.NewSPDYExecutor(c.kubeConfig, "POST", req.URL())
	if err != nil {
		return nil, err
	}

	// Stream results
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	ctx, cancelFunc := newContext()
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

// Get the UID of the Deployment in which the current controller is running
func (c *clientImpl) GetControllerDeploymentUID() (types.UID, error) {
	if c.config.LocalDebugMode {
		// In LocalDebugMode, the controller runs outside of the Kubernetes cluster so there is no deployment; return an empty UID
		return types.UID(""), nil
	}
	deployment, err := c.GetDeployment(c.config.DeploymentName)
	if err != nil {
		return "", fmt.Errorf("error getting controller deployment: %v", err)
	}
	return deployment.UID, nil
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
	foundContainer := false
	ready := false
	for _, c := range pod.Status.ContainerStatuses {
		if c.Name == containerName {
			foundContainer = true
			ready = c.Ready
		}
	}
	if !foundContainer {
		return false, fmt.Errorf("container %s not found in pod %s", containerName, pod.Name)
	}
	return ready, nil
}

func (c *clientImpl) getJobManagerPodLabel() string {
	return fmt.Sprintf("%s=%s", specs.JobManagerUIDKey, c.config.JobManagerUID)
}

// getKubeConfig returns a Kubernetes REST config object, used to make RESTful Kubernetes API calls
func getKubeConfig(conf *config.Config) (*rest.Config, error) {
	var config *rest.Config
	var err error

	if conf.LocalDebugMode {
		kubeConfig := conf.KubeConfig
		if kubeConfig == "" {
			kubeConfig = filepath.Join(homedir.HomeDir(), ".kube", "config")
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("error getting Kubernetes REST config: %v", err)
	}
	return config, nil
}

// Timeout to use when calling Kubernetes API
const Timeout = 60

// newContext creates a context to use when calling the K8s client
func newContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), Timeout*time.Second)
}
