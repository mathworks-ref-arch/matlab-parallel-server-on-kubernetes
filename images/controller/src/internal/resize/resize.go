// Package resize contains code for resizing an MJS cluster in Kubernetes
// Copyright 2024 The MathWorks, Inc.
package resize

import (
	"controller/internal/config"
	"controller/internal/k8s"
	"controller/internal/logging"
	"controller/internal/specs"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/mathworks/mjssetup/pkg/certificate"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Resize is an interface for resizing a cluster
type Resizer interface {
	GetWorkers() ([]Worker, error)
	AddWorkers([]specs.WorkerInfo) error
	DeleteWorkers([]string) error
}

// Worker represents a worker currently deployed in the Kubernetes cluster
type Worker struct {
	Info      specs.WorkerInfo // Metadata for this worker
	IsRunning bool             // Whether a worker deployment has started running a pod
}

// MJSResizer implements resizing of an MJS cluster in Kubernetes
type MJSResizer struct {
	config      *config.Config
	logger      *logging.Logger
	client      k8s.Client
	specFactory *specs.SpecFactory
}

// NewMJSResizer constructs an MJSResizer
func NewMJSResizer(conf *config.Config, uid types.UID, logger *logging.Logger) (*MJSResizer, error) {
	client, err := k8s.NewClient(conf, logger)
	if err != nil {
		return nil, err
	}
	m := MJSResizer{
		config: conf,
		logger: logger,
		client: client,
	}
	m.specFactory, err = specs.NewSpecFactory(conf, uid)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// AddWorkers adds workers to the Kubernetes cluster
func (m *MJSResizer) AddWorkers(workers []specs.WorkerInfo) error {
	m.logger.Info("Adding workers", zap.Any("workers", workers))
	if m.config.UsePoolProxy() {
		return m.addWorkersAndProxies(workers)
	}
	m.addWorkersForInternalCluster(workers)
	return nil
}

// GetWorkers returns a list of the MJS workers currently running on the Kubernetes cluster; this list is determined by examining the worker deployments present on the cluster
func (m *MJSResizer) GetWorkers() ([]Worker, error) {
	deployments, err := m.client.GetDeploymentsWithLabel(fmt.Sprintf("%s=%s", specs.AppKey, specs.WorkerLabels.AppLabel))
	if err != nil {
		return []Worker{}, err
	}
	workers := getWorkersFromDeployments(deployments, m.logger)
	return workers, nil
}

// DeleteWorkers deletes a list of MJS workers from a Kubernetes cluster
func (m *MJSResizer) DeleteWorkers(names []string) error {
	m.logger.Info("Deleting workers", zap.Any("workers", names))
	existingWorkers, err := m.GetWorkers()
	if err != nil {
		return fmt.Errorf("error getting existing workers: %v", err)
	}

	// Create a hash map of existing workers for efficient lookup
	existingWorkerMap := map[string]specs.WorkerInfo{}
	for _, w := range existingWorkers {
		existingWorkerMap[w.Info.Name] = w.Info
	}

	// Attempt to delete each worker in the list
	deletedWorkerIDMap := map[int]bool{}
	for _, name := range names {
		workerInfo, exists := existingWorkerMap[name]
		if exists {
			err := m.deleteWorker(workerInfo.HostName)
			if err != nil {
				m.logger.Error("Error deleting worker", zap.String("hostname", workerInfo.HostName), zap.Error(err))
			} else if m.config.UsePoolProxy() {
				// Track successfully deleted workers so we know which pool proxies to delete later
				deletedWorkerIDMap[workerInfo.ID] = true
			}
		}
	}

	// Clean up resources associated with the workers we successfully deleted
	if m.config.UsePoolProxy() {
		return m.deleteProxiesIfNotNeeded(existingWorkers, deletedWorkerIDMap)
	}
	return nil
}

// addWorkersAndProxies adds a list of workers plus any parallel pool proxies they need
func (m *MJSResizer) addWorkersAndProxies(workers []specs.WorkerInfo) error {
	newProxiesNeeded, noProxyNeeded, err := m.getPoolProxiesNeededForWorkers(workers)
	if err != nil {
		return err
	}

	// First, add workers that do not need a new parallel pool proxy
	for _, w := range noProxyNeeded {
		err := m.addWorker(&w)
		if err != nil {
			logAddWorkerError(m.logger, &w, err)
		}
	}

	// Next, add each parallel pool proxy and then its associated workers
	for p, workers := range newProxiesNeeded {
		proxy := specs.NewPoolProxyInfo(p, m.config.PoolProxyBasePort)
		err := m.addPoolProxy(proxy)
		if err != nil {
			m.logger.Error("error adding parallel pool proxy", zap.String("name", proxy.Name), zap.Int("ID", proxy.ID), zap.Error(err))
			// We should not add the workers associated with this proxy, since proxy creation failed
			continue
		}

		workersWereAdded := false
		for _, w := range workers {
			err := m.addWorker(&w)
			if err != nil {
				logAddWorkerError(m.logger, &w, err)
			} else {
				workersWereAdded = true
			}
		}

		// Clean up the proxy if none of its associated workers were successfully added
		if !workersWereAdded {
			return m.deletePoolProxy(proxy.Name)
		}
	}
	return nil
}

// getPoolProxiesNeededForWorkers returns a map of parallel pool proxies to create and the workers dependent on them, plus a list of workers that do not need a new proxy
func (m *MJSResizer) getPoolProxiesNeededForWorkers(workers []specs.WorkerInfo) (map[int][]specs.WorkerInfo, []specs.WorkerInfo, error) {
	existingProxies, err := m.getPoolProxies()
	if err != nil {
		return nil, nil, err
	}

	// Create hash map of existing proxies
	existingProxyIDs := map[int]bool{}
	for _, p := range existingProxies {
		existingProxyIDs[p.ID] = true
	}

	// Check which proxy is needed by each worker
	newProxiesNeeded := map[int][]specs.WorkerInfo{}
	noProxyNeeded := []specs.WorkerInfo{}
	for _, w := range workers {
		p := m.specFactory.CalculatePoolProxyForWorker(w.ID)
		if existingProxyIDs[p] {
			noProxyNeeded = append(noProxyNeeded, w)
		} else {
			newProxiesNeeded[p] = append(newProxiesNeeded[p], w)
		}
	}
	return newProxiesNeeded, noProxyNeeded, nil
}

// addWorkers adds a list of workers, without adding pool proxies or exposing worker ports
func (m *MJSResizer) addWorkersForInternalCluster(workers []specs.WorkerInfo) {
	for _, w := range workers {
		err := m.addWorker(&w)
		if err != nil {
			logAddWorkerError(m.logger, &w, err)
		}
	}
}

// addWorker adds a single MJS worker to the Kubernetes cluster
func (m *MJSResizer) addWorker(w *specs.WorkerInfo) error {
	w.GenerateUniqueHostName()
	svcSpec := m.specFactory.GetWorkerServiceSpec(w)
	_, err := m.client.CreateService(svcSpec)
	if err != nil {
		return err
	}
	_, err = m.client.CreateDeployment(m.specFactory.GetWorkerDeploymentSpec(w))
	if err != nil {
		m.logger.Error("Error creating pod", zap.Error(err))
		m.logger.Debug("Cleaning up worker service since pod creation failed")
		errDeleteSvc := m.client.DeleteService(svcSpec.Name)
		if errDeleteSvc != nil {
			m.logger.Error("Error cleaning up service after failed pod creation", zap.Error(err))
		}
	}
	return err
}

// deleteWorker removes a single MJS worker from the Kubernetes cluster
func (m *MJSResizer) deleteWorker(name string) error {
	err := m.client.DeleteDeployment(name)
	if err != nil {
		// If we failed to delete the deployment, we should not proceed to deleting the service, as this will leave an orphaned deployment
		return err
	}
	err = m.client.DeleteService(name)
	if err != nil {
		return err
	}
	return nil
}

// getPoolProxies gets a list of parallel pool proxies currently running on the cluster; this list is determined by examining the deployments present on the cluster
func (m *MJSResizer) getPoolProxies() ([]specs.PoolProxyInfo, error) {
	deployments, err := m.client.GetDeploymentsWithLabel(fmt.Sprintf("%s=%s", specs.AppKey, specs.PoolProxyLabels.AppLabel))
	if err != nil {
		return []specs.PoolProxyInfo{}, err
	}
	existingProxies := getProxiesFromDeployments(deployments, m.logger)
	return existingProxies, nil
}

// addPoolProxy adds a parallel pool proxy to the Kubernetes cluster
func (m *MJSResizer) addPoolProxy(proxy specs.PoolProxyInfo) error {
	if m.config.UseSecureCommunication {
		err := m.createProxyCertificate(proxy.Name)
		if err != nil {
			m.logger.Error("Error creating certificate for pool proxy", zap.Error(err))
			return err
		}
	}

	_, err := m.client.CreateDeployment(m.specFactory.GetPoolProxyDeploymentSpec(&proxy))
	if err != nil {
		m.logger.Error("Error creating parallel pool proxy deployment", zap.Error(err))
		return err
	}

	return err
}

// createProxyCertificate creates a Kubernetes secret containing a certificate for workers to use when connecting to the proxy
func (m *MJSResizer) createProxyCertificate(name string) error {
	// Generate the certificate
	certCreator := certificate.New()
	sharedSecret, err := certCreator.CreateSharedSecret()
	if err != nil {
		return err
	}
	certificate, err := certCreator.GenerateCertificate(sharedSecret)
	if err != nil {
		return err
	}

	// Create spec for Kubernetes secret containing this certificate
	certBytes, err := json.Marshal(certificate)
	if err != nil {
		return fmt.Errorf("error marshaling certificate: %v", err)
	}
	secretSpec := m.specFactory.GetSecretSpec(name, false)
	secretSpec.Data[specs.ProxyCertFileName] = certBytes

	// Create the Kubernetes secret
	_, err = m.client.CreateSecret(secretSpec)
	if err != nil {
		return err
	}
	return nil
}

// deleteProxiesIfNotNeeded deletes any proxies that are no longer needed after worker deletion
func (m *MJSResizer) deleteProxiesIfNotNeeded(originalWorkers []Worker, deletedIDs map[int]bool) error {
	toKeep := map[int]bool{}
	for _, worker := range originalWorkers {
		wasDeleted := deletedIDs[worker.Info.ID]
		if !wasDeleted {
			toKeep[m.specFactory.CalculatePoolProxyForWorker(worker.Info.ID)] = true
		}
	}

	existingProxies, err := m.getPoolProxies()
	if err != nil {
		return err
	}

	for _, proxy := range existingProxies {
		shouldKeep := toKeep[proxy.ID]
		if !shouldKeep {
			err = m.deletePoolProxy(proxy.Name)
			if err != nil {
				m.logger.Warn("error deleting pool proxy", zap.Error(err))
			}
		}
	}
	return nil
}

// deletePoolProxy removes a parallel pool proxy from the Kubernetes cluster
func (m *MJSResizer) deletePoolProxy(proxyName string) error {
	m.logger.Info("Deleting parallel pool proxy", zap.String("name", proxyName))
	if m.config.UseSecureCommunication {
		err := m.client.DeleteSecret(proxyName)
		if err != nil {
			return err
		}
	}
	return m.client.DeleteDeployment(proxyName)
}

// getWorkersFromDeployments converts a list of worker deployments to a list of worker details
func getWorkersFromDeployments(deployments *appsv1.DeploymentList, logger *logging.Logger) []Worker {
	var workers []Worker
	for _, d := range deployments.Items {
		// Make sure the required labels are present
		err := checkLabelsExist(d.Labels, []string{specs.WorkerLabels.Name, specs.WorkerLabels.ID, specs.WorkerLabels.HostName})
		if err != nil {
			logger.Error("Worker deployment has missing label", zap.Error(err), zap.String("name", d.Name))
		}

		// Convert the worker ID label to an int
		workerID, err := getIntFromLabel(d.Labels, specs.WorkerLabels.ID, logger)
		if err != nil {
			continue
		}

		// Populate worker information from labels
		workers = append(workers, Worker{
			Info: specs.WorkerInfo{
				Name:     d.Labels[specs.WorkerLabels.Name],
				ID:       workerID,
				HostName: d.Labels[specs.WorkerLabels.HostName],
			},
			IsRunning: d.Status.ReadyReplicas == 1,
		})
	}
	return workers
}

// getProxiesFromDeployments converts a list of proxy deployments to a list of proxy details
func getProxiesFromDeployments(deployments *appsv1.DeploymentList, logger *logging.Logger) []specs.PoolProxyInfo {
	var proxies []specs.PoolProxyInfo
	for _, d := range deployments.Items {
		// Make sure the required labels are present
		err := checkLabelsExist(d.Labels, []string{specs.PoolProxyLabels.Name, specs.PoolProxyLabels.ID, specs.PoolProxyLabels.Port})
		if err != nil {
			logger.Error("Proxy deployment has missing label", zap.Error(err), zap.String("name", d.Name))
		}

		// Convert labels to ints
		proxyID, err := getIntFromLabel(d.Labels, specs.PoolProxyLabels.ID, logger)
		if err != nil {
			continue
		}
		port, err := getIntFromLabel(d.Labels, specs.PoolProxyLabels.Port, logger)
		if err != nil {
			continue
		}

		// Populate proxy information from labels
		proxies = append(proxies, specs.PoolProxyInfo{
			Name: d.Labels[specs.PoolProxyLabels.Name],
			ID:   proxyID,
			Port: port,
		})
	}
	return proxies
}

// checkLabelsExist errors if every element in a list of labels is not present in a label map
func checkLabelsExist(labels map[string]string, checkFor []string) error {
	for _, label := range checkFor {
		_, ok := labels[label]
		if !ok {
			return fmt.Errorf("missing label %s", label)
		}
	}
	return nil
}

// getIntFromLabel extracts an int from a label map and returns an error if the conversion fails
func getIntFromLabel(labels map[string]string, key string, logger *logging.Logger) (int, error) {
	resultStr := labels[key]
	result, err := strconv.Atoi(resultStr)
	if err != nil {
		logger.Error("label could not be converted to int", zap.String("label", key), zap.String("value", resultStr), zap.Error(err))
		return 0, errors.New("invalid label")
	}
	return result, nil
}

func logAddWorkerError(logger *logging.Logger, w *specs.WorkerInfo, err error) {
	logger.Error("error adding worker", zap.String("name", w.Name), zap.Int("ID", w.ID), zap.String("hostname", w.HostName), zap.Error(err))
}
