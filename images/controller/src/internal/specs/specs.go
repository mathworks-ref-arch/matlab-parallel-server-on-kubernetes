// Package specs contains functions for creating Kubernetes resource specs from a template.
// Copyright 2024-2026 The MathWorks, Inc.
package specs

import (
	"controller/internal/annotations"
	"controller/internal/config"
	"controller/internal/logging"
	"controller/internal/resize"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SpecFactory generates Kubernetes resource specs
type SpecFactory struct {
	templateStore  TemplateStore
	annotationKeys config.AnnotationKeys
	logger         *logging.Logger
}

type TemplateStore interface {
	GetWorkerPodTemplate() (*corev1.PodTemplateSpec, string)
	GetPoolProxyPodTemplate() (*corev1.PodTemplateSpec, string)
}

func NewFactory(annotationKeys config.AnnotationKeys, templateStore TemplateStore, logger *logging.Logger) resize.SpecFactory {
	return &SpecFactory{
		annotationKeys: annotationKeys,
		templateStore:  templateStore,
		logger:         logger,
	}
}

// GetWorkerPodSpec creates a spec for an MJS worker pod
func (s *SpecFactory) GetWorkerPodSpec(id int) (*corev1.Pod, error) {
	template, checksum := s.templateStore.GetWorkerPodTemplate()
	conf, err := s.getWorkerConfFromAnnotations(template)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s%d", conf.workerPrefix, id)

	pod := corev1.PodTemplateSpec{}
	template.DeepCopyInto(&pod)

	// Generate unique hostname for this deployment.
	// If we don't do this, the job manager can take a long time to connect
	// to scaled-up workers with the same hostname as a worker that was recently
	// scaled down.
	podName := generateUniqueHostName(name)
	hostName := fmt.Sprintf("%s.%s", podName, conf.workerDomain)
	pod.Spec.Hostname = podName

	// Add environment variables specific to this worker
	workerEnv := map[string]string{
		"WORKER_NAME":                     name,
		"LOGBASE":                         filepath.Join(conf.logDir, name),
		"HOSTNAME":                        hostName,
		"MDCE_OVERRIDE_INTERNAL_HOSTNAME": hostName,
		"MDCE_OVERRIDE_EXTERNAL_HOSTNAME": hostName, // Ensure clients running inside K8s can reach us
	}
	if conf.usePoolProxy {
		proxyID := CalculateProxyIDForWorker(id, conf.workersPerPoolProxy)
		s.logger.Debug(fmt.Sprintf("Worker %d will use pool proxy %d", id, proxyID))
		workerEnv["PARALLEL_SERVER_POOL_PROXY_HOST"] = getProxyName(proxyID, conf.poolProxyPrefix)
		workerEnv["PARALLEL_SERVER_POOL_PROXY_PORT"] = fmt.Sprintf("%d", getProxyPort(proxyID, conf.poolProxyBasePort))

		// Add annotation to say which proxy we are using
		pod.Annotations[s.annotationKeys.PoolProxyID] = fmt.Sprintf("%d", proxyID)

		// Add volume for pool proxy certificate if needed.
		// Just need to add the volume; the template should already include the volumeMount
		if conf.useSecureCommunication {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: conf.certVolume,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: getProxyName(proxyID, conf.poolProxyPrefix),
					},
				},
			})
		}
	}

	addEnv(&pod.Spec.Containers[0], workerEnv)

	// Configure mpich-3 (used by R2024a) to use the pod's IP address
	pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env,
		corev1.EnvVar{
			Name: "MPICH_INTERFACE_HOSTNAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
	)

	// Ensure we can always resolve our own hostname without relying on Kubernetes DNS.
	// This is needed because our DNS entry will not be created until the pod is ready,
	// but we need to set up MJS before that.
	pod.Spec.HostAliases = []corev1.HostAlias{
		{
			IP: "127.0.0.1",
			Hostnames: []string{
				hostName,
			},
		},
	}

	// Apply annotations; these are used by the resizer to determine
	// the settings of this pod
	workerIDStr := fmt.Sprintf("%d", id)
	annotations := pod.Annotations
	annotations[s.annotationKeys.WorkerName] = name
	annotations[s.annotationKeys.WorkerID] = workerIDStr
	annotations[s.annotationKeys.TemplateChecksum] = checksum

	// Also set labels; we don't use these, but they can be useful for
	// interactive filtering with kubectl, for example
	labels := pod.Labels
	labels[s.annotationKeys.WorkerName] = name
	labels[s.annotationKeys.WorkerID] = workerIDStr

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: pod.Spec,
	}, nil
}

// GetPoolProxyDeploymentSpec creates a spec for a deployment to run a parallel pool proxy pod
func (s *SpecFactory) GetPoolProxyDeploymentSpec(id int) (*appsv1.Deployment, error) {
	template, checksum := s.templateStore.GetPoolProxyPodTemplate()
	conf, err := s.getPoolProxyConfFromAnnotations(template)
	if err != nil {
		return nil, err
	}

	name := getProxyName(id, conf.poolProxyPrefix)
	port := getProxyPort(id, conf.poolProxyBasePort)

	pod := corev1.PodTemplateSpec{}
	template.DeepCopyInto(&pod)

	// Add the port argument; this is unique to each proxy
	pod.Spec.Containers[0].Args = append(pod.Spec.Containers[0].Args, "--port", fmt.Sprintf("%d", port))

	// Add the logfile argument so each proxy writes to a different file
	logFile := filepath.Join(conf.logDir, name+".log")
	pod.Spec.Containers[0].Args = append(pod.Spec.Containers[0].Args, "--logfile", logFile)

	// Add volume containing this proxy's certificate.
	// Just need to add the volume; the template should already include the volumeMount
	secretName := name
	if conf.useSecureCommunication {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: conf.certVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		})
	}

	// Annotate the deployment with settings that are used by the resizer
	proxyIDStr := fmt.Sprintf("%d", id)
	annotations := map[string]string{}
	annotations[s.annotationKeys.PoolProxyID] = proxyIDStr
	annotations[s.annotationKeys.TemplateChecksum] = checksum
	if conf.useSecureCommunication {
		// Add an annotation to indicate that this pod needs a secret
		annotations[s.annotationKeys.SecretName] = secretName
	}

	// Add label for proxy ID; this is used by the pool proxy service's selector.
	labels := pod.Labels
	labels[s.annotationKeys.PoolProxyID] = proxyIDStr

	return s.wrapPod(&pod, name, labels, annotations), nil
}

func CalculateProxyIDForWorker(workerID, workersPerPoolProxy int) int {
	ratio := float64(workerID) / float64(workersPerPoolProxy)
	return int(math.Ceil(ratio))
}

type workerConfig struct {
	certVolume             string
	logDir                 string
	poolProxyBasePort      int
	poolProxyPrefix        string
	usePoolProxy           bool
	useSecureCommunication bool
	workerDomain           string
	workerPrefix           string
	workersPerPoolProxy    int
}

type poolProxyConfig struct {
	certVolume             string
	logDir                 string
	poolProxyBasePort      int
	poolProxyPrefix        string
	useSecureCommunication bool
}

// Extract worker settings from annotations on template
func (s *SpecFactory) getWorkerConfFromAnnotations(template *corev1.PodTemplateSpec) (*workerConfig, error) {
	conf := workerConfig{}

	// Set annotations that are always required
	meta := &template.ObjectMeta
	keys := s.annotationKeys
	err := annotations.SetStringsFromAnnotations(map[string]*string{
		keys.LogDir:       &conf.logDir,
		keys.WorkerDomain: &conf.workerDomain,
		keys.WorkerPrefix: &conf.workerPrefix,
	}, meta)
	if err != nil {
		return nil, err
	}
	err = annotations.SetBoolsFromAnnotations(map[string]*bool{
		keys.UsePoolProxy: &conf.usePoolProxy,
	}, meta)
	if err != nil {
		return nil, err
	}

	if !conf.usePoolProxy {
		return &conf, nil
	}

	// These annotations are only required if using a pool proxy
	err = annotations.SetStringsFromAnnotations(map[string]*string{
		keys.CertVolume:      &conf.certVolume,
		keys.PoolProxyPrefix: &conf.poolProxyPrefix,
	}, meta)
	if err != nil {
		return nil, err
	}
	err = annotations.SetIntsFromAnnotations(map[string]*int{
		keys.WorkersPerPoolProxy: &conf.workersPerPoolProxy,
		keys.PoolProxyBasePort:   &conf.poolProxyBasePort,
	}, meta)
	if err != nil {
		return nil, err
	}
	err = annotations.SetBoolsFromAnnotations(map[string]*bool{
		keys.UseSecureCommunication: &conf.useSecureCommunication,
	}, meta)
	if err != nil {
		return nil, err
	}
	return &conf, nil
}

// Extract proxy settings from annotations on template
func (s *SpecFactory) getPoolProxyConfFromAnnotations(template *corev1.PodTemplateSpec) (*poolProxyConfig, error) {
	conf := poolProxyConfig{}
	meta := &template.ObjectMeta
	keys := s.annotationKeys
	err := annotations.SetStringsFromAnnotations(map[string]*string{
		keys.CertVolume:      &conf.certVolume,
		keys.PoolProxyPrefix: &conf.poolProxyPrefix,
		keys.LogDir:          &conf.logDir,
	}, meta)
	if err != nil {
		return nil, err
	}
	err = annotations.SetIntsFromAnnotations(map[string]*int{
		keys.PoolProxyBasePort: &conf.poolProxyBasePort,
	}, meta)
	if err != nil {
		return nil, err
	}
	err = annotations.SetBoolsFromAnnotations(map[string]*bool{
		keys.UseSecureCommunication: &conf.useSecureCommunication,
	}, meta)
	if err != nil {
		return nil, err
	}
	return &conf, nil
}

func getProxyName(id int, prefix string) string {
	return fmt.Sprintf("%s%d", prefix, id)
}

func getProxyPort(id, poolProxyBasePort int) int {
	return poolProxyBasePort + id - 1 // note we are using 1-based indexing
}

// wrapPod wraps a pod spec into a deployment spec
func (s *SpecFactory) wrapPod(pod *corev1.PodTemplateSpec, name string, labels, annotations map[string]string) *appsv1.Deployment {
	var numReplicas int32 = 1 // Always want only one pod per deployment
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: *pod,
		},
	}
}

// generateUniqueHostName generates a unique host name for a worker
func generateUniqueHostName(name string) string {
	guidWithHyphens := uuid.New()
	guidNoHyphens := strings.ReplaceAll(guidWithHyphens.String(), "-", "")

	// Trim down the UID to prevent the hostname from becoming too long.
	// 5 characters should be enough to prevent a new worker from having
	// the same hostname as its predecessor.
	guidNoHyphens = guidNoHyphens[:5]
	return fmt.Sprintf("%s-%s", name, guidNoHyphens)
}

// addEnv appends environment variables from a map to a container
func addEnv(container *corev1.Container, toAdd map[string]string) {
	env := container.Env
	for key, val := range toAdd {
		env = append(env, corev1.EnvVar{Name: key, Value: val})
	}
	container.Env = env
}
