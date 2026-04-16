// Package resize contains code for resizing an MJS cluster in Kubernetes
// Copyright 2024-2026 The MathWorks, Inc.
package resize

import (
	"controller/internal/annotations"
	"controller/internal/certificate"
	"controller/internal/config"
	"controller/internal/controller"
	"controller/internal/k8s"
	"controller/internal/logging"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// K8sResizer implements resizing of an MJS cluster in Kubernetes
type K8sResizer struct {
	config          ResizeConfig
	logger          *logging.Logger
	client          k8s.Client
	specFactory     SpecFactory
	certCreator     certificate.CertCreator
	templateChecker TemplateChecker
}

type ResizeConfig struct {
	WorkerLabel    string
	PoolProxyLabel string
	ProxyCertFile  string
	AnnotationKeys config.AnnotationKeys
}

type SpecFactory interface {
	GetWorkerPodSpec(int) (*corev1.Pod, error)
	GetPoolProxyDeploymentSpec(int) (*appsv1.Deployment, error)
}

type TemplateChecker interface {
	Refresh() error
	GetWorkerTemplateChecksum() string
	GetPoolProxyTemplateChecksum() string
}

func NewK8sResizer(conf ResizeConfig, client k8s.Client, specFactory SpecFactory, templateChecker TemplateChecker, certCreator certificate.CertCreator, logger *logging.Logger) controller.Resizer {
	return &K8sResizer{
		config:          conf,
		logger:          logger,
		client:          client,
		specFactory:     specFactory,
		certCreator:     certCreator,
		templateChecker: templateChecker,
	}
}

// AddWorkers adds workers to the Kubernetes cluster
func (r *K8sResizer) AddWorkers(workerIDs []int) error {
	r.logger.Info("Adding workers", zap.Any("workerIDs", workerIDs))

	// Get pod specs for new workers
	workerSpecs := []*corev1.Pod{}
	for _, w := range workerIDs {
		spec, err := r.specFactory.GetWorkerPodSpec(w)
		if err != nil {
			return err
		}
		workerSpecs = append(workerSpecs, spec)
	}

	// Check which proxies we need to create
	newProxiesNeeded, noProxyNeeded, err := r.getPoolProxiesNeededForWorkers(workerSpecs)
	if err != nil {
		return err
	}

	// First, add workers that do not need a new parallel pool proxy
	for _, spec := range noProxyNeeded {
		_, err := r.client.CreatePod(spec)
		if err != nil {
			logAddWorkerError(r.logger, spec.Name, err)
		}
	}

	// Next, add each parallel pool proxy and then its associated workers
	for proxyID, workers := range newProxiesNeeded {
		proxy, err := r.addPoolProxy(proxyID)
		if err != nil {
			r.logger.Error("error adding parallel pool proxy", zap.Int("ID", proxyID), zap.Error(err))
			// We should not add the workers associated with this proxy, since proxy creation failed
			continue
		}

		workersWereAdded := false
		for _, spec := range workers {
			_, err := r.client.CreatePod(spec)
			if err != nil {
				logAddWorkerError(r.logger, spec.Name, err)
			} else {
				workersWereAdded = true
			}
		}

		// Clean up the proxy if none of its associated workers were successfully added
		if !workersWereAdded {
			return r.deletePoolProxy(proxy)
		}
	}
	return nil
}

// GetAndUpdateWorkers returns a list of the IDs of MJS workers currently running on the Kubernetes cluster
// The list is determined by examining the worker pods present on the cluster.
// The workers are checked for consistency with the latest template and deleted if outdated.
func (r *K8sResizer) GetAndUpdateWorkers() ([]controller.DeployedWorker, error) {
	workers, err := r.getWorkers()
	if err != nil {
		return nil, err
	}

	// Make sure everything is up to date
	if err := r.templateChecker.Refresh(); err != nil {
		return nil, err
	}
	updatedWorkers, err := r.purgeOutdatedWorkers(workers)
	if err != nil {
		return nil, err
	}
	if err := r.updateOutdatedProxies(updatedWorkers); err != nil {
		return nil, err
	}

	currentWorkers := []controller.DeployedWorker{}
	for _, w := range updatedWorkers {
		currentWorkers = append(currentWorkers, w.DeployedWorker)
	}
	return currentWorkers, nil
}

func (r *K8sResizer) getWorkers() ([]versionedWorker, error) {
	pods, err := r.client.GetPodsWithLabel(r.config.WorkerLabel)
	if err != nil {
		return nil, err
	}
	return r.getWorkersFromPods(pods.Items), nil
}

// DeleteWorkers deletes a list of MJS workers from a Kubernetes cluster
func (r *K8sResizer) DeleteWorkers(workers []controller.DeployedWorker) error {
	r.logger.Info("Deleting workers", zap.Any("workers", workers))
	allWorkers, err := r.getWorkers()
	if err != nil {
		r.logger.Error("Failed to check existing workers", zap.Error(err))
		return err
	}
	return r.deleteWorkers(workers, allWorkers)
}

func (r *K8sResizer) deleteWorkers(toDelete []controller.DeployedWorker, allWorkers []versionedWorker) error {
	// Attempt to delete each worker in the list
	deletedWorkerIDs := map[int]bool{}
	for _, w := range toDelete {
		err := r.deleteWorker(w.PodName)
		if err != nil {
			r.logger.Error("Error deleting worker", zap.String("name", w.Name), zap.Error(err))
		}
		deletedWorkerIDs[w.ID] = true
	}

	// Remove any proxies that are no longer needed
	proxies, err := r.getPoolProxies()
	if err != nil {
		r.logger.Error("Failed to check existing pool proxies", zap.Error(err))
		return err
	}
	remainingWorkers := []versionedWorker{}
	for _, w := range allWorkers {
		if !deletedWorkerIDs[w.ID] {
			remainingWorkers = append(remainingWorkers, w)
		}
	}
	r.deleteUnusedProxies(proxies, remainingWorkers)
	return nil
}

// getPoolProxiesNeededForWorkers returns a map of IDs of parallel pool proxies to create and the IDs of workers dependent on them, plus a list of IDs of workers that do not need a new proxy
func (r *K8sResizer) getPoolProxiesNeededForWorkers(workerSpecs []*corev1.Pod) (map[int][]*corev1.Pod, []*corev1.Pod, error) {
	existingProxies, err := r.getPoolProxies()
	if err != nil {
		return nil, nil, err
	}

	// Create hash map of existing proxies
	existingProxyIDs := map[int]bool{}
	for _, p := range existingProxies {
		existingProxyIDs[p.id] = true
	}

	// Check which proxy is needed by each worker
	newProxiesNeeded := map[int][]*corev1.Pod{}
	noProxyNeeded := []*corev1.Pod{}
	for _, w := range workerSpecs {
		p, needProxy := r.getProxyNeededByWorker(w)
		if !needProxy || existingProxyIDs[p] {
			noProxyNeeded = append(noProxyNeeded, w)
		} else {
			newProxiesNeeded[p] = append(newProxiesNeeded[p], w)
		}
	}
	return newProxiesNeeded, noProxyNeeded, nil
}

func (r *K8sResizer) getProxyNeededByWorker(workerSpec *corev1.Pod) (int, bool) {
	proxyID, ok, err := annotations.GetOptionalAnnotationInt(r.config.AnnotationKeys.PoolProxyID, &workerSpec.ObjectMeta)
	if err != nil {
		r.logger.Error(err.Error())
		return 0, false
	}
	if !ok {
		// This worker does not need a proxy
		return 0, false
	}
	return proxyID, true
}

func (r *K8sResizer) getSecretNeededByProxy(proxySpec *appsv1.Deployment) (string, bool) {
	return annotations.GetOptionalAnnotationString(r.config.AnnotationKeys.SecretName, &proxySpec.ObjectMeta)
}

// deleteWorker removes a single MJS worker from the Kubernetes cluster
func (r *K8sResizer) deleteWorker(podName string) error {
	return r.client.DeletePod(podName)
}

// getPoolProxies gets a list of parallel pool proxies currently running on the cluster; this list is determined by examining the deployments present on the cluster
func (r *K8sResizer) getPoolProxies() ([]*versionedProxy, error) {
	deployments, err := r.client.GetDeploymentsWithLabel(r.config.PoolProxyLabel)
	if err != nil {
		return nil, err
	}
	existingProxies := r.getProxiesFromDeployments(deployments)
	return existingProxies, nil
}

func (r *K8sResizer) addPoolProxy(id int) (*versionedProxy, error) {
	spec, err := r.specFactory.GetPoolProxyDeploymentSpec(id)
	if err != nil {
		return nil, err
	}
	proxyMetadata, err := r.getProxyFromDeployment(spec)
	if err != nil {
		return nil, err
	}

	secretName, needSecret := r.getSecretNeededByProxy(spec)
	if needSecret {
		err := r.createOrOverwriteProxySecret(secretName)
		if err != nil {
			r.logger.Error("Error creating certificate for pool proxy", zap.Error(err))
			return nil, err
		}
	}

	_, err = r.client.CreateDeployment(spec)
	if err != nil {
		r.logger.Error("Error creating parallel pool proxy deployment", zap.Error(err))
		return nil, err
	}
	return proxyMetadata, nil
}

// Create a new proxy certificate; if one already exists (e.g. was drooled from a previous proxy), delete it.
func (r *K8sResizer) createOrOverwriteProxySecret(name string) error {
	_, exists, err := r.client.SecretExists(name)
	if err != nil {
		return fmt.Errorf("failed to check pool proxy secret: %v", err)
	}
	if exists {
		r.logger.Info("Cleaning up existing pool proxy secret", zap.String("name", name))
		err := r.client.DeleteSecret(name)
		if err != nil {
			return fmt.Errorf("failed to delete pool proxy secret: %v", err)
		}
	}

	return r.createProxySecret(name)
}

func (r *K8sResizer) ensureProxySecretExists(name string) error {
	_, exists, err := r.client.SecretExists(name)
	if err != nil {
		return fmt.Errorf("failed to check pool proxy secret: %v", err)
	}
	if exists {
		return nil
	}
	return r.createProxySecret(name)
}

func (r *K8sResizer) ensureNoProxySecret(name string) error {
	_, exists, err := r.client.SecretExists(name)
	if err != nil {
		return fmt.Errorf("failed to check pool proxy secret: %v", err)
	}
	if !exists {
		return nil
	}
	r.logger.Debug("Cleaning up proxy certificate", zap.String("name", name))
	return r.client.DeleteSecret(name)
}

// Create a Kubernetes secret containing a certificate for workers to use when connecting to the proxy
func (r *K8sResizer) createProxySecret(name string) error {
	// Generate the certificate
	sharedSecret, err := r.certCreator.CreateSharedSecret()
	if err != nil {
		return err
	}
	certificate, err := r.certCreator.GenerateCertificate(sharedSecret)
	if err != nil {
		return err
	}

	// Create the Kubernetes secret
	certBytes, err := json.Marshal(certificate)
	if err != nil {
		return fmt.Errorf("error marshaling certificate: %v", err)
	}
	r.logger.Debug("Creating pool proxy certificate", zap.String("name", name))
	_, err = r.client.CreateSecret(name, map[string][]byte{
		r.config.ProxyCertFile: certBytes,
	}, false)
	if err != nil {
		return err
	}
	return nil
}

// deletePoolProxy removes a parallel pool proxy from the Kubernetes cluster
func (r *K8sResizer) deletePoolProxy(p *versionedProxy) error {
	r.logger.Info("Deleting parallel pool proxy", zap.String("name", p.deploymentName))

	if p.usesSecret {
		_, exists, err := r.client.SecretExists(p.secretName)
		if err != nil {
			return err
		}
		if exists {
			err := r.client.DeleteSecret(p.secretName)
			if err != nil {
				return err
			}
		}
	}

	return r.client.DeleteDeployment(p.deploymentName)
}

// Convert a list of worker pods to a list of worker details
func (r *K8sResizer) getWorkersFromPods(pods []corev1.Pod) []versionedWorker {
	var workers []versionedWorker
	for _, p := range pods {
		// Skip pods that are already being deleted
		if p.DeletionTimestamp != nil {
			continue
		}

		worker, err := r.getWorkerFromPod(p)
		if err != nil {
			r.logger.Error(err.Error())
			continue
		}
		workers = append(workers, *worker)
	}
	return workers
}

func (r *K8sResizer) getWorkerFromPod(p corev1.Pod) (*versionedWorker, error) {
	keys := r.config.AnnotationKeys
	meta := &p.ObjectMeta
	workerID, err := annotations.GetAnnotationInt(keys.WorkerID, meta)
	if err != nil {
		return nil, err
	}
	checksum, err := annotations.GetAnnotationString(keys.TemplateChecksum, meta)
	if err != nil {
		return nil, err
	}
	workerName, err := annotations.GetAnnotationString(keys.WorkerName, meta)
	if err != nil {
		return nil, err
	}
	proxyID, usesProxy := r.getProxyNeededByWorker(&p)

	return &versionedWorker{
		DeployedWorker: controller.DeployedWorker{
			Name:      workerName,
			ID:        workerID,
			IsRunning: p.Status.Phase == corev1.PodRunning,
			PodName:   p.Name,
		},
		templateChecksum: checksum,
		proxyID:          proxyID,
		usesProxy:        usesProxy,
	}, nil
}

type versionedWorker struct {
	controller.DeployedWorker
	templateChecksum string
	usesProxy        bool
	proxyID          int
}

type versionedProxy struct {
	id               int
	deploymentName   string
	templateChecksum string
	usesSecret       bool
	secretName       string
	certFile         string
}

// getProxiesFromDeployments converts a list of proxy deployments to a list of proxy details
func (r *K8sResizer) getProxiesFromDeployments(deployments *appsv1.DeploymentList) []*versionedProxy {
	var proxies []*versionedProxy
	for _, d := range deployments.Items {
		proxy, err := r.getProxyFromDeployment(&d)
		if err != nil {
			r.logger.Error(err.Error())
			continue
		}
		proxies = append(proxies, proxy)
	}
	return proxies
}

func (r *K8sResizer) getProxyFromDeployment(d *appsv1.Deployment) (*versionedProxy, error) {
	keys := r.config.AnnotationKeys
	meta := &d.ObjectMeta
	proxyID, err := annotations.GetAnnotationInt(keys.PoolProxyID, meta)
	if err != nil {
		return nil, err
	}
	checksum, err := annotations.GetAnnotationString(keys.TemplateChecksum, meta)
	if err != nil {
		return nil, err
	}
	secretName, usesSecret := r.getSecretNeededByProxy(d)
	return &versionedProxy{
		deploymentName:   d.Name,
		id:               proxyID,
		templateChecksum: checksum,
		usesSecret:       usesSecret,
		secretName:       secretName,
	}, nil
}

func logAddWorkerError(logger *logging.Logger, name string, err error) {
	logger.Error("error adding worker", zap.String("name", name), zap.Error(err))
}

// Check for any workers that are out of sync with the latest template and delete them.
// Note that we choose to delete rather than upgrade because we want the replaced workers to have
// a fresh hostname to avoid issues with reconnections to the job manager.
func (r *K8sResizer) purgeOutdatedWorkers(workers []versionedWorker) ([]versionedWorker, error) {
	latestChecksum := r.templateChecker.GetWorkerTemplateChecksum()
	toDelete := []controller.DeployedWorker{}
	toKeep := []versionedWorker{}
	for _, w := range workers {
		if w.templateChecksum != latestChecksum {
			toDelete = append(toDelete, w.DeployedWorker)
		} else {
			toKeep = append(toKeep, w)
		}
	}

	if len(toDelete) > 0 {
		r.logger.Info("deleting outdated workers", zap.Any("workers", toDelete))
		if err := r.deleteWorkers(toDelete, workers); err != nil {
			return nil, err
		}
	}
	return toKeep, nil
}

// Update any outdated proxies, and delete proxies that are no longer needed.
// We do this with the Kubernetes API to update a deployment spec.
func (r *K8sResizer) updateOutdatedProxies(currentWorkers []versionedWorker) error {
	proxies, err := r.getPoolProxies()
	if err != nil {
		return err
	}
	if len(proxies) == 0 {
		return err
	}

	toKeep := r.deleteUnusedProxies(proxies, currentWorkers)
	if len(toKeep) == 0 {
		return nil
	}

	// Check the remaining proxies are up-to-date
	latestChecksum := r.templateChecker.GetPoolProxyTemplateChecksum()
	for _, p := range toKeep {
		if p.templateChecksum == latestChecksum {
			continue
		}
		err := r.updateProxy(p)
		if err != nil {
			r.logger.Error(err.Error())
		}
	}
	return nil
}

func (r *K8sResizer) updateProxy(oldProxy *versionedProxy) error {
	r.logger.Info("updating outdated proxy", zap.Any("proxy", oldProxy))
	spec, err := r.specFactory.GetPoolProxyDeploymentSpec(oldProxy.id)
	if err != nil {
		return err
	}
	newProxy, err := r.getProxyFromDeployment(spec)
	if err != nil {
		return err
	}
	_, err = r.client.UpdateDeployment(spec)
	if err != nil {
		return err
	}

	if newProxy.usesSecret {
		// If the updated proxy needs a certificate, make sure it exists
		// (e.g. in case helm upgrade was used to change useSecureCommunication to true
		r.ensureProxySecretExists(newProxy.secretName)
	} else if oldProxy.usesSecret {
		// If the previous spec used a secret but the new one does not, delete the secret
		r.ensureNoProxySecret(oldProxy.secretName)
	}
	return nil
}

// Delete proxies that are not used by any worker; return list of remaining proxies.
func (r *K8sResizer) deleteUnusedProxies(currentProxies []*versionedProxy, currentWorkers []versionedWorker) []*versionedProxy {
	// Check which proxies are still needed by workers
	neededProxies := map[int]bool{}
	for _, w := range currentWorkers {
		if w.usesProxy {
			neededProxies[w.proxyID] = true
		}
	}
	toKeep := []*versionedProxy{}
	toDelete := []*versionedProxy{}
	for _, p := range currentProxies {
		if neededProxies[p.id] {
			toKeep = append(toKeep, p)
		} else {
			toDelete = append(toDelete, p)
		}
	}

	// Delete the proxies we don't need
	for _, p := range toDelete {
		err := r.deletePoolProxy(p)
		if err != nil {
			r.logger.Warn("Failed to delete unneeded pool proxy", zap.String("name", p.deploymentName), zap.Error(err))
		}
	}
	return toKeep
}
