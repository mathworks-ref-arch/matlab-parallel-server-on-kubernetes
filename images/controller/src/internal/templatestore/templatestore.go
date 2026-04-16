// Client for fetching and caching the latest pod templates from Kubernetes.
// Copyright 2025-2026 The MathWorks, inc.
package templatestore

import (
	"controller/internal/k8s"
	"controller/internal/logging"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const MAX_CHECKSUM_LENGTH = 63

type Store struct {
	k8sClient            k8s.Client
	workerPodTemplate    *template
	poolProxyPodTemplate *template
	config               StoreConfig
	logger               *logging.Logger
}

type StoreConfig struct {
	Namespace             string
	UsePoolProxy          bool
	WorkerTemplateName    string
	PoolProxyTemplateName string
	TemplateFile          string
}

type template struct {
	spec     *corev1.PodTemplateSpec
	checksum string
}

// Create a new store containing the pod templates.
func New(conf StoreConfig, k8sClient k8s.Client, logger *logging.Logger) (*Store, error) {
	// Get initial templates
	workerTemplateStr, workerChecksum, err := getLatestPodTemplate(conf.WorkerTemplateName, conf.TemplateFile, k8sClient)
	if err != nil {
		return nil, err
	}
	workerTemplate, err := newTemplate(workerTemplateStr, workerChecksum)
	if err != nil {
		return nil, fmt.Errorf("failed to parse worker pod template: %v", err)
	}

	poolProxyTemplate := &template{}
	if conf.UsePoolProxy {
		poolProxyTemplateStr, poolProxyChecksum, err := getLatestPodTemplate(conf.PoolProxyTemplateName, conf.TemplateFile, k8sClient)
		if err != nil {
			return nil, err
		}
		poolProxyTemplate, err = newTemplate(poolProxyTemplateStr, poolProxyChecksum)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pool proxy pod template: %v", err)
		}
	}

	store := &Store{
		k8sClient:            k8sClient,
		config:               conf,
		workerPodTemplate:    workerTemplate,
		poolProxyPodTemplate: poolProxyTemplate,
		logger:               logger,
	}
	return store, nil
}

// Make a call to Kubernetes to fetch the latest pod templates
func (s *Store) Refresh() error {
	if err := s.refreshWorkerTemplate(); err != nil {
		return err
	}
	if s.config.UsePoolProxy {
		return s.refreshProxyTemplate()
	}
	return nil
}

func (s *Store) refreshWorkerTemplate() error {
	return s.refreshTemplate(s.workerPodTemplate, s.config.WorkerTemplateName)
}

func (s *Store) refreshProxyTemplate() error {
	return s.refreshTemplate(s.poolProxyPodTemplate, s.config.PoolProxyTemplateName)
}

func (s *Store) refreshTemplate(tmpl *template, name string) error {
	newTemplateStr, newChecksum, err := getLatestPodTemplate(name, s.config.TemplateFile, s.k8sClient)
	if err != nil {
		return err
	}
	if newChecksum == tmpl.checksum {
		return nil
	}
	newSpec, err := templateToPodSpec(newTemplateStr)
	if err != nil {
		return err
	}
	tmpl.spec = newSpec
	tmpl.checksum = newChecksum
	return nil
}

func (s *Store) GetWorkerTemplateChecksum() string {
	_, checksum := s.GetWorkerPodTemplate()
	return checksum
}

func (s *Store) GetPoolProxyTemplateChecksum() string {
	_, checksum := s.GetPoolProxyPodTemplate()
	return checksum
}

func (s *Store) GetWorkerPodTemplate() (*corev1.PodTemplateSpec, string) {
	return s.workerPodTemplate.spec, s.workerPodTemplate.checksum
}

func (s *Store) GetPoolProxyPodTemplate() (*corev1.PodTemplateSpec, string) {
	if !s.config.UsePoolProxy {
		return nil, ""
	}
	return s.poolProxyPodTemplate.spec, s.poolProxyPodTemplate.checksum
}

func getLatestPodTemplate(name, templateFile string, k8sClient k8s.Client) (string, string, error) {
	configMap, err := k8sClient.GetConfigMap(name)
	var checksum [32]byte
	if err != nil {
		return "", "", err
	}
	templateStr, ok := configMap.Data[templateFile]
	if !ok {
		return "", "", fmt.Errorf("template file %s not found in ConfigMap %s", templateFile, name)
	}

	checksum = sha256.Sum256([]byte(templateStr))
	return templateStr, checksumToString(checksum), nil
}

// Convert a checksum to a string that is a valid Kubernetes pod label
func checksumToString(checksum [32]byte) string {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	encoded := strings.ToLower(enc.EncodeToString(checksum[:]))

	// Trim to MAX_CHECKSUM_LENGTH just to be safe
	if len(encoded) > MAX_CHECKSUM_LENGTH {
		encoded = encoded[:63]
	}
	return encoded
}

func templateToPodSpec(templateStr string) (*corev1.PodTemplateSpec, error) {
	podSpec := &corev1.PodTemplateSpec{}
	err := yaml.UnmarshalStrict([]byte(templateStr), podSpec)
	if err != nil {
		return nil, err
	}
	return podSpec, nil
}

func newTemplate(templateStr, checksum string) (*template, error) {
	podSpec, err := templateToPodSpec(templateStr)
	if err != nil {
		return nil, err
	}
	return &template{
		spec:     podSpec,
		checksum: checksum,
	}, nil
}
