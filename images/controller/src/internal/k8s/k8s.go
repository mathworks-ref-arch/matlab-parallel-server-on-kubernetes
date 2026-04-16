// Interface for interacting with resources in a Kubernetes cluster.
// Copyright 2024-2025 The MathWorks, Inc.
package k8s

import (
	"bytes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type Client interface {
	CreateDeployment(*appsv1.Deployment) (*appsv1.Deployment, error)
	GetDeployment(string) (*appsv1.Deployment, error)
	DeleteDeployment(string) error
	UpdateDeployment(*appsv1.Deployment) (*appsv1.Deployment, error)
	DeploymentExists(string) (bool, error)
	CreatePod(*corev1.Pod) (*corev1.Pod, error)
	GetPod(string) (*corev1.Pod, error)
	DeletePod(string) error
	GetService(string) (*corev1.Service, error)
	ServiceExists(string) (*corev1.Service, bool, error)
	CreateSecret(string, map[string][]byte, bool) (*corev1.Secret, error)
	UpdateSecret(string, map[string][]byte, bool) (*corev1.Secret, error)
	GetSecret(string) (*corev1.Secret, error)
	DeleteSecret(string) error
	SecretExists(string) (*corev1.Secret, bool, error)
	GetConfigMap(string) (*corev1.ConfigMap, error)
	GetPodsWithLabel(string) (*corev1.PodList, error)
	GetDeploymentsWithLabel(string) (*appsv1.DeploymentList, error)
	GetServicesWithLabel(string) (*corev1.ServiceList, error)
	IsPodReady(label, containerName string) (bool, error)
	GetReadyPod(label, containerName string) (*corev1.Pod, error)
	ExecOnPod(podName, containerName string, cmd []string) (*bytes.Buffer, error)
}
