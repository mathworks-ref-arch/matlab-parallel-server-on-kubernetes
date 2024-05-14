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
	"testing"

	"github.com/mathworks/mjssetup/pkg/certificate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"
)

// Test the functions to add, get and delete workers
func TestWorkerFuncs(t *testing.T) {
	resizer, _ := getResizerWithFakeClient(t)
	verifyGetWorkersResponse(t, resizer, []Worker{})

	// Add a worker
	workerInfo := specs.WorkerInfo{Name: "worker1", ID: 1}
	worker := Worker{Info: workerInfo, IsRunning: false}
	err := resizer.addWorker(&workerInfo)
	assert.NoError(t, err, "error adding worker")
	verifyNumWorkersInK8s(t, resizer, 1)
	verifyDeploymentAndService(t, resizer, workerInfo.HostName)
	verifyGetWorkersResponse(t, resizer, []Worker{worker})

	// Add a second worker
	workerInfo2 := specs.WorkerInfo{Name: "worker2", ID: 2}
	worker2 := Worker{Info: workerInfo2, IsRunning: false}
	err = resizer.addWorker(&workerInfo2)
	assert.NoError(t, err, "error adding worker")
	verifyNumWorkersInK8s(t, resizer, 2)
	verifyDeploymentAndService(t, resizer, workerInfo2.HostName)
	verifyGetWorkersResponse(t, resizer, []Worker{worker, worker2})

	// Check we can delete the first worker
	err = resizer.deleteWorker(workerInfo.HostName)
	assert.NoError(t, err, "error deleting worker")
	verifyNumWorkersInK8s(t, resizer, 1)
	verifyDeploymentAndService(t, resizer, workerInfo2.HostName)
	verifyGetWorkersResponse(t, resizer, []Worker{worker2})
}

// verify GetWorkers returns the expected list of workers
func verifyGetWorkersResponse(t *testing.T, resizer *MJSResizer, expectedWorkers []Worker) {
	gotWorkers, err := resizer.GetWorkers()
	assert.NoError(t, err, "error getting workers")
	require.Equal(t, len(expectedWorkers), len(gotWorkers), "unexpected number of workers")

	// Create hash map of found workers
	foundWorkers := map[string]bool{}
	for _, w := range gotWorkers {
		foundWorkers[w.Info.Name] = true
	}

	// Check we found all expected workers
	if len(expectedWorkers) > 0 {
		for _, w := range expectedWorkers {
			found := foundWorkers[w.Info.Name]
			require.Truef(t, found, "worker %s not found in GetWorkers response", w.Info.Name)
		}
	}
}

// verify that K8s has the expected number of worker resources
func verifyNumWorkersInK8s(t *testing.T, resizer *MJSResizer, n int) {
	workerMatch := fmt.Sprintf("%s=%s", specs.AppKey, specs.WorkerLabels.AppLabel)
	verifyNumDeployments(t, resizer, n, workerMatch)
	verifyNumServices(t, resizer, n, workerMatch)
}

// verify that a K8s deployment and service with a given name exist
func verifyDeploymentAndService(t *testing.T, resizer *MJSResizer, name string) {
	verifyDeploymentExists(t, resizer, name)
	verifyServiceExists(t, resizer, name)
}

// verify that a K8s deployment and secret for a given proxy exist
func verifyProxyResources(t *testing.T, resizer *MJSResizer, name string, useSecureCommunication bool) {
	verifyDeploymentExists(t, resizer, name)
	if useSecureCommunication {
		verifyProxySecret(t, resizer, name)
	} else {
		verifySecretDoesNotExist(t, resizer, name)
	}
}

// Test the functions to add, get and delete proxies
func TestProxyFuncs(t *testing.T) {
	testCases := []struct {
		name                   string
		useSecureCommunication bool
	}{
		{"secure", true},
		{"insecure", false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			verifyProxyFuncs(tt, tc.useSecureCommunication)
		})
	}
}

// Test proxy functions with or without secure communication
func verifyProxyFuncs(t *testing.T, useSecureCommunication bool) {
	resizer, _ := getResizerWithFakeClient(t)
	resizer.config.UseSecureCommunication = useSecureCommunication
	verifyGetProxiesResponse(t, resizer, []specs.PoolProxyInfo{})

	// Add a proxy
	proxy := specs.NewPoolProxyInfo(5, resizer.config.PoolProxyBasePort)
	err := resizer.addPoolProxy(proxy)
	assert.NoError(t, err, "error adding proxy")
	verifyNumProxiesInK8s(t, resizer, 1)
	verifyProxyResources(t, resizer, proxy.Name, useSecureCommunication)
	verifyGetProxiesResponse(t, resizer, []specs.PoolProxyInfo{proxy})

	// Add a second proxy
	proxy2 := specs.NewPoolProxyInfo(2, resizer.config.PoolProxyBasePort)
	err = resizer.addPoolProxy(proxy2)
	assert.NoError(t, err, "error adding proxy")
	verifyNumProxiesInK8s(t, resizer, 2)
	verifyProxyResources(t, resizer, proxy2.Name, useSecureCommunication)
	verifyGetProxiesResponse(t, resizer, []specs.PoolProxyInfo{proxy, proxy2})

	// Check we can delete the first proxy
	err = resizer.deletePoolProxy(proxy.Name)
	assert.NoError(t, err, "error deleting proxy")
	verifySecretDoesNotExist(t, resizer, proxy.Name)
	verifyNumProxiesInK8s(t, resizer, 1)
	verifyProxyResources(t, resizer, proxy2.Name, useSecureCommunication)
	verifyGetProxiesResponse(t, resizer, []specs.PoolProxyInfo{proxy2})
}

// verify getProxies returns the expected list of proxies
func verifyGetProxiesResponse(t *testing.T, resizer *MJSResizer, expectedProxies []specs.PoolProxyInfo) {
	proxies, err := resizer.getPoolProxies()
	assert.NoError(t, err, "error getting proxies")
	require.Equal(t, len(expectedProxies), len(proxies), "unexpected number of workers")
	if len(expectedProxies) > 0 {
		for _, proxy := range expectedProxies {
			found := false
			for _, gotProxy := range proxies {
				if proxy == gotProxy {
					found = true
					break
				}
			}
			require.Truef(t, found, "proxy %s not found in getProxies response", proxy.Name)
		}
	}
}

// verify that K8s has the expected number of proxy resources
func verifyNumProxiesInK8s(t *testing.T, resizer *MJSResizer, n int) {
	proxyMatch := fmt.Sprintf("%s=%s", specs.AppKey, specs.PoolProxyLabels.AppLabel)
	verifyNumDeployments(t, resizer, n, proxyMatch)
}

func TestGetProxiesNeededForWorkers(t *testing.T) {
	testCases := []struct {
		name                string
		addedWorkerIDs      []int
		workersPerPoolProxy int
		existingProxies     []int
		expectedNewProxies  []int
	}{
		{
			"oneWorker", []int{1}, 1, []int{}, []int{1},
		}, {
			"manyWorkersOneProxy", []int{1, 2, 3}, 3, []int{}, []int{1},
		}, {
			"manyWorkersManyProxies", []int{2, 4, 6}, 2, []int{}, []int{1, 2, 3},
		}, {
			"noNewProxiesNeeded", []int{1, 2}, 2, []int{1}, []int{},
		}, {
			"existingProxiesMoreNeeded", []int{2, 4, 6}, 2, []int{2}, []int{1, 3},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conf := config.Config{
				WorkersPerPoolProxy: tc.workersPerPoolProxy,
			}
			resizer, _ := getResizerWithFakeClientAndConfig(t, &conf)

			// add pre-existing proxies
			proxies := []specs.PoolProxyInfo{}
			for _, p := range tc.existingProxies {
				proxy := specs.NewPoolProxyInfo(p, resizer.config.PoolProxyBasePort)
				resizer.addPoolProxy(proxy)
				proxies = append(proxies, proxy)
			}
			verifyGetProxiesResponse(t, resizer, proxies)

			// test the getProxiesNeededForWorkers method
			addedWorkerInfos := []specs.WorkerInfo{}
			for _, w := range tc.addedWorkerIDs {
				addedWorkerInfos = append(addedWorkerInfos, specs.WorkerInfo{ID: w})
			}
			newProxies, noProxyNeeded, err := resizer.getPoolProxiesNeededForWorkers(addedWorkerInfos)
			assert.NoError(t, err)

			// Check we get the expected list of proxies to add
			newProxyIDs := []int{}
			for p := range newProxies {
				newProxyIDs = append(newProxyIDs, p)
			}
			assert.ElementsMatch(t, tc.expectedNewProxies, newProxyIDs, "Did not get expected proxies to add")

			// Check that the lists together contain all of the added workers
			numWorkersWithProxies := 0
			for _, workers := range newProxies {
				numWorkersWithProxies += len(workers)
			}
			assert.Equal(t, len(tc.addedWorkerIDs), numWorkersWithProxies+len(noProxyNeeded), "Number of workers with proxies + number of workers that do not need a proxy should equal the number of added workers")
		})
	}
}

// check that deleteProxiesIfNotNeeded deletes the expected proxies when workers are removed
func TestDeleteProxiesIfNotNeeded(t *testing.T) {
	testCases := []struct {
		name                 string
		workersPerPoolProxy  int
		origWorkerIDs        []int
		deletedWorkerIDs     []int
		existingProxies      []int
		expectedFinalProxies []int
	}{
		{"allProxiesStillNeeded", 2, []int{1, 2, 3, 4}, []int{2, 3}, []int{1, 2}, []int{1, 2}},
		{"removeSomeProxies", 2, []int{1, 2, 3, 4, 5, 6, 7, 8}, []int{1, 3, 4, 6, 7, 8}, []int{1, 2, 3, 4}, []int{1, 3}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conf := config.Config{
				WorkersPerPoolProxy: tc.workersPerPoolProxy,
			}
			resizer, _ := getResizerWithFakeClientAndConfig(t, &conf)

			// Create the pre-existing proxies
			proxies := []specs.PoolProxyInfo{}
			for _, p := range tc.existingProxies {
				proxy := specs.NewPoolProxyInfo(p, resizer.config.PoolProxyBasePort)
				resizer.addPoolProxy(proxy)
				proxies = append(proxies, proxy)
			}
			verifyGetProxiesResponse(t, resizer, proxies)

			// Trigger proxies to be deleted
			origWorkers := []Worker{}
			for _, w := range tc.origWorkerIDs {
				origWorkers = append(origWorkers, Worker{Info: specs.WorkerInfo{Name: "testworker", ID: w}})
			}
			deletedIDsMap := map[int]bool{}
			for _, w := range tc.deletedWorkerIDs {
				deletedIDsMap[w] = true
			}
			resizer.deleteProxiesIfNotNeeded(origWorkers, deletedIDsMap)

			// Check the remaining proxies are as expected
			finalProxies := []specs.PoolProxyInfo{}
			for _, ep := range tc.expectedFinalProxies {
				finalProxies = append(finalProxies, specs.NewPoolProxyInfo(ep, resizer.config.PoolProxyBasePort))
			}
			verifyGetProxiesResponse(t, resizer, finalProxies)
		})
	}
}

// end-to-end test of cluster scaling up and down
func TestClusterScaling(t *testing.T) {
	testCases := []struct {
		name     string
		useProxy bool
	}{
		{"use_proxy", true},
		{"no_proxy", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			verifyEndToEndScaling(t, tc.useProxy)
		})
	}
}

func verifyEndToEndScaling(t *testing.T, useProxy bool) {
	conf := config.Config{
		InternalClientsOnly: !useProxy,
		WorkersPerPoolProxy: 2,
	}
	resizer, _ := getResizerWithFakeClientAndConfig(t, &conf)

	// Check there are no workers initially
	verifyClusterState(t, resizer, []specs.WorkerInfo{}, useProxy)

	// Add some workers
	workers1 := []specs.WorkerInfo{
		{Name: "worker1", ID: 1},
		{Name: "worker2", ID: 2},
		{Name: "worker3", ID: 3},
	}
	err := resizer.AddWorkers(workers1)
	assert.NoError(t, err)
	verifyClusterState(t, resizer, workers1, useProxy)

	// Add some more workers
	workers2 := []specs.WorkerInfo{
		{Name: "worker4", ID: 4},
		{Name: "worker5", ID: 5},
	}
	resizer.AddWorkers(workers2)
	allWorkers := append(workers1, workers2...)
	verifyClusterState(t, resizer, allWorkers, useProxy)

	// Delete some workers
	toDelete := []string{"worker2", "worker4"}
	resizer.DeleteWorkers(toDelete)

	// Check remaining workers
	remainingWorkers := []specs.WorkerInfo{}
	remainingWorkerNames := []string{}
	for _, w := range allWorkers {
		keeping := true
		for _, d := range toDelete {
			if d == w.Name {
				keeping = false
				break
			}
		}
		if keeping {
			remainingWorkers = append(remainingWorkers, w)
			remainingWorkerNames = append(remainingWorkerNames, w.Name)
		}
	}
	assert.Equal(t, len(allWorkers)-len(toDelete), len(remainingWorkers), "remaining workers list has incorrect length")
	verifyClusterState(t, resizer, remainingWorkers, useProxy)

	// Clean up remaining workers
	resizer.DeleteWorkers(remainingWorkerNames)
	verifyClusterState(t, resizer, []specs.WorkerInfo{}, useProxy)
}

// verify that K8s cluster has resources corresponding to a set of workers, including the required proxies if using proxies
func verifyClusterState(t *testing.T, resizer *MJSResizer, workers []specs.WorkerInfo, usingProxy bool) {

	// verify workers
	verifyNumWorkersInK8s(t, resizer, len(workers))
	proxies := []specs.PoolProxyInfo{}
	for _, w := range workers {
		// note that AddWorkers generates unique names for the worker deployments, so we should check using the worker name label rather than the deployment name
		verifyDeploymentAndServiceForWorkerName(t, resizer, w.Name)

		if usingProxy {
			// Verify that we have the required proxy for this worker
			proxy := specs.NewPoolProxyInfo(resizer.specFactory.CalculatePoolProxyForWorker(w.ID), resizer.config.PoolProxyBasePort)
			verifyDeploymentExists(t, resizer, proxy.Name)

			// add to list of proxies to check on load balancer
			isInList := false
			for _, p := range proxies {
				if p.ID == proxy.ID {
					isInList = true
					break
				}
			}
			if !isInList {
				proxies = append(proxies, proxy)
			}
		}
	}
}

// Get an MJSResizer with a mocked-out Kubernetes backend
func getResizerWithFakeClient(t *testing.T) (*MJSResizer, *fake.Clientset) {
	conf := config.Config{}
	return getResizerWithFakeClientAndConfig(t, &conf)
}

// Get an MJSResizer with a mocked-out Kubernetes backend for a given config struct
func getResizerWithFakeClientAndConfig(t *testing.T, conf *config.Config) (*MJSResizer, *fake.Clientset) {
	fakeK8s := fake.NewSimpleClientset()

	// Add some standard config settings
	lbName := "test-proxy"
	namespace := "test-ns"
	conf.BasePort = 27350
	conf.PortsPerWorker = 2
	conf.LoadBalancerName = lbName
	conf.Namespace = namespace

	// Add dummy LoadBalancer service
	logger := logging.NewFromZapLogger(zaptest.NewLogger(t))
	fakeClient := k8s.NewClientWithK8sBackend(conf, fakeK8s, logger)
	lb := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: lbName,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
		},
	}
	_, err := fakeClient.CreateService(&lb)
	require.NoError(t, err, "Error creating load balancer service against fake K8s client")

	return &MJSResizer{
		client:      fakeClient,
		logger:      logger,
		config:      conf,
		specFactory: specs.NewSpecFactory(conf, "abcd"),
	}, fakeK8s
}

// verify that a deployment of a given name exists
func verifyDeploymentExists(t *testing.T, resizer *MJSResizer, name string) {
	dep, err := resizer.client.GetDeployment(name)
	assert.NoError(t, err, "error getting deployment")
	assert.NotNil(t, dep, "deployment should not be nil")
}

// verify that a service of a given name exists
func verifyServiceExists(t *testing.T, resizer *MJSResizer, name string) {
	svc, err := resizer.client.GetService(name)
	assert.NoError(t, err, "error getting service")
	assert.NotNil(t, svc, "service should not be nil")
}

// verify that a proxy secret exists and contains the expected data
func verifyProxySecret(t *testing.T, resizer *MJSResizer, name string) {
	secret, exists, err := resizer.client.SecretExists(name)
	require.NoError(t, err, "error getting secret")
	require.Truef(t, exists, "secret %s should exist", name)
	require.NotNil(t, secret, "secret should not be nil")

	// Check the secret's contents
	assert.Contains(t, secret.Data, specs.ProxyCertFileName, "proxy secret should contain certificate file")
	certBytes := secret.Data[specs.ProxyCertFileName]
	assert.NotEmpty(t, certBytes, "proxy secret should not be empty")
	gotCert := certificate.Certificate{}
	err = json.Unmarshal(certBytes, &gotCert)
	assert.NoError(t, err, "error unmarshaling proxy certificate")
	assert.NotEmpty(t, gotCert.ClientCert)
	assert.NotEmpty(t, gotCert.ClientKey)
	assert.NotEmpty(t, gotCert.ServerCert)
}

// verify that a secret of a given name does not exist
func verifySecretDoesNotExist(t *testing.T, resizer *MJSResizer, name string) {
	_, exists, err := resizer.client.SecretExists(name)
	require.NoError(t, err, "error getting secret")
	assert.Falsef(t, exists, "secret %s should not exist", name)
}

// verify that a deployment and a service exist for a given worker name
// note that the deployment name will be a unique, auto-generated name
func verifyDeploymentAndServiceForWorkerName(t *testing.T, resizer *MJSResizer, name string) {
	deps, err := resizer.client.GetDeploymentsWithLabel(fmt.Sprintf("workerName=%s", name))
	assert.NoError(t, err, "error getting deployments")
	require.Equalf(t, 1, len(deps.Items), "expected 1 deployment for worker name %s", name)

	svcs, err := resizer.client.GetServicesWithLabel(fmt.Sprintf("workerName=%s", name))
	assert.NoError(t, err, "error getting service")
	require.Equalf(t, 1, len(svcs.Items), "expected 1 service for worker name %s", name)
}

// verify that a K8s cluster has a given number of deployments with a given label selector
func verifyNumDeployments(t *testing.T, resizer *MJSResizer, n int, labelSelector string) {
	deps, err := resizer.client.GetDeploymentsWithLabel(labelSelector)
	assert.NoError(t, err, "error getting deployments")
	require.Equal(t, n, len(deps.Items), "did not find expected number of deployments")
}

// verify that a K8s cluster has a given number of services with a given label selector
func verifyNumServices(t *testing.T, resizer *MJSResizer, n int, labelSelector string) {
	svcs, err := resizer.client.GetServicesWithLabel(labelSelector)
	assert.NoError(t, err, "error getting services")
	require.Equal(t, n, len(svcs.Items), "did not find expected number of services")
}

// Test the extraction of worker info from a list of deployments
func TestGetWorkersFromDeployments(t *testing.T) {
	// Create list of workers
	workers := []Worker{
		{
			Info: specs.WorkerInfo{
				Name:     "runningWorker",
				ID:       1,
				HostName: "worker1-uid",
			},
			IsRunning: true,
		},
		{
			Info: specs.WorkerInfo{
				Name:     "pendingWorker",
				ID:       2,
				HostName: "worker2-uid",
			},
			IsRunning: false,
		},
	}

	// Create deployments for these workers
	specFactory := specs.NewSpecFactory(&config.Config{}, "abcd")
	depList := appsv1.DeploymentList{}
	for _, w := range workers {
		deployment := specFactory.GetWorkerDeploymentSpec(&w.Info)
		if w.IsRunning {
			deployment.Status.ReadyReplicas = 1
		}
		depList.Items = append(depList.Items, *deployment)
	}

	gotWorkers := getWorkersFromDeployments(&depList, logging.NewFromZapLogger(zaptest.NewLogger(t)))
	assert.Equal(t, workers, gotWorkers, "Incorrect worker list returned by GetWorkersFromDeployments")
}

func TestGetProxiesFromDeployments(t *testing.T) {
	// Create list of proxies
	proxies := []specs.PoolProxyInfo{
		{
			ID:   1,
			Name: "proxy1",
			Port: 30000,
		}, {
			ID:   4,
			Name: "proxy4",
			Port: 30005,
		}, {
			ID:   6,
			Name: "proxy6",
			Port: 30009,
		},
	}

	// Create deployments for these proxies
	specFactory := specs.NewSpecFactory(&config.Config{}, "abcd")
	depList := appsv1.DeploymentList{}
	for _, p := range proxies {
		depList.Items = append(depList.Items, *specFactory.GetPoolProxyDeploymentSpec(&p))
	}

	gotProxies := getProxiesFromDeployments(&depList, logging.NewFromZapLogger(zaptest.NewLogger(t)))
	assert.Equal(t, proxies, gotProxies, "Incorrect proxies list returned by GetProxiesFromDeployments")
}

// Verify that we don't get drooled resources when adding a worker fails
func TestAddWorkerNegative(t *testing.T) {
	t.Run("deployment_fails", func(tt *testing.T) {
		resizer, fakeK8s := getResizerWithFakeClient(t)
		setupDeploymentFailure(fakeK8s, "", "")
		w := specs.WorkerInfo{Name: "test", ID: 10}
		err := resizer.addWorker(&w)
		assert.Error(tt, err, "Should get error when worker deployment creation fails")
		verifyNumWorkersInK8s(tt, resizer, 0)
	})

	t.Run("service_fails", func(tt *testing.T) {
		resizer, client := getResizerWithFakeClient(t)
		setupServiceFailure(client, "", "")
		w := specs.WorkerInfo{Name: "test", ID: 10}
		err := resizer.addWorker(&w)
		assert.Error(tt, err, "Should get error when worker service creation fails")
		verifyNumWorkersInK8s(tt, resizer, 0)
	})
}

// Verify that we don't get drooled resources when adding a proxy fails
func TestAddProxyNegative(t *testing.T) {
	resizer, client := getResizerWithFakeClient(t)
	setupDeploymentFailure(client, "", "")
	p := specs.PoolProxyInfo{Name: "test", ID: 3, Port: 2000}
	err := resizer.addPoolProxy(p)
	assert.Error(t, err, "Should get error when proxy deployment creation fails")
	verifyNumProxiesInK8s(t, resizer, 0)
}

// Verify that workers associated with a proxy do not get started if we fail to add that proxy
func TestAddWorkersProxyFailure(t *testing.T) {
	conf := config.Config{
		WorkersPerPoolProxy: 4,
	}
	resizer, client := getResizerWithFakeClientAndConfig(t, &conf)

	// Add some pre-existing workers
	initWorkers := []specs.WorkerInfo{
		{Name: "worker1", ID: 1},
		{Name: "worker2", ID: 2},
	}
	err := resizer.addWorkersAndProxies(initWorkers)
	assert.NoError(t, err)
	verifyNumWorkersInK8s(t, resizer, len(initWorkers))
	verifyNumProxiesInK8s(t, resizer, 1)

	// Add some more workers, but this time error when creating proxy #2
	// All workers associated with that proxy should not be created, whereas all other workers should be successfully created
	setupDeploymentFailure(client, specs.PoolProxyLabels.ID, "2")
	toAddExistingProxy := []specs.WorkerInfo{ // Workers that use the already-existing proxy #1
		{Name: "worker3", ID: 3},
		{Name: "worker4", ID: 4},
	}
	toAddProxyError := []specs.WorkerInfo{ // Workers that will trigger creation of proxy #2, which fails
		{Name: "worker5", ID: 5},
		{Name: "worker6", ID: 6},
	}
	toAddNoProxyError := []specs.WorkerInfo{ // Workers that will trigger creation of proxy #3, which succeeds
		{Name: "worker10", ID: 10},
	}
	toAddSuccess := append(toAddExistingProxy, toAddNoProxyError...)
	toAdd := append(toAddSuccess, toAddProxyError...)
	err = resizer.addWorkersAndProxies(toAdd)
	assert.NoError(t, err) // Note we don't expect an error, since some parts of the creation succeeded

	// Check the cluster only contains the original workers + the workers that did not need a new proxy
	verifyNumProxiesInK8s(t, resizer, 2)
	verifyNumWorkersInK8s(t, resizer, len(initWorkers)+len(toAddSuccess))
	for _, w := range toAddSuccess {
		verifyDeploymentAndServiceForWorkerName(t, resizer, w.Name)
	}
}

// Verify that a proxy is torn down if we fail to add any associated workers
func TestAddProxyWorkersAllFail(t *testing.T) {
	conf := config.Config{
		WorkersPerPoolProxy: 2,
	}
	resizer, client := getResizerWithFakeClientAndConfig(t, &conf)
	setupDeploymentFailure(client, specs.AppKey, specs.WorkerLabels.AppLabel)
	toAdd := []specs.WorkerInfo{
		{Name: "worker1", ID: 1},
		{Name: "worker2", ID: 2},
	}
	err := resizer.addWorkersAndProxies(toAdd)
	assert.NoError(t, err)
	verifyNumWorkersInK8s(t, resizer, 0)
	verifyNumProxiesInK8s(t, resizer, 0)
}

// Return a fake Kubernetes client that errors when trying to create a deployment, optionally only for deployments with a given label
func setupDeploymentFailure(fakeK8s *fake.Clientset, labelKey, labelVal string) {
	useCustomLabel := labelKey != ""
	fakeK8s.PrependReactor("create", "deployments", func(action k8sTesting.Action) (bool, runtime.Object, error) {
		if useCustomLabel {
			createAction := action.(k8sTesting.CreateAction)
			dep := createAction.GetObject().(*appsv1.Deployment)
			hasLabel := dep.Labels[labelKey] == labelVal
			if hasLabel {
				return true, nil, fmt.Errorf("failed to create deployment with label %s=%s", labelKey, labelVal)
			}
			return false, nil, nil // Let the action progress to the fake clientset
		}
		return true, nil, errors.New("failed to create a deployment")
	})
}

// Return a fake Kubernetes client that errors when trying to create a service, optionally only for services with a given label
func setupServiceFailure(client *fake.Clientset, labelKey, labelVal string) {
	useCustomLabel := labelKey != ""
	client.PrependReactor("create", "services", func(action k8sTesting.Action) (bool, runtime.Object, error) {
		if useCustomLabel {
			createAction := action.(k8sTesting.CreateAction)
			svc := createAction.GetObject().(*corev1.Service)
			hasLabel := svc.Labels[labelKey] == labelVal
			if hasLabel {
				return true, nil, fmt.Errorf("failed to create service with label %s=%s", labelKey, labelVal)
			}
			return false, nil, nil // Let the action progress to the fake clientset
		}
		return true, nil, errors.New("failed to create a service")
	})
}
