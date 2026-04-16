// Copyright 2024-2026 The MathWorks, Inc.
package resize_test

import (
	"controller/internal/certificate"
	"controller/internal/config"
	"controller/internal/controller"
	"controller/internal/logging"
	"controller/internal/resize"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	mockCert "controller/mocks/certificate"
	mockK8s "controller/mocks/k8s"
	mocks "controller/mocks/resize"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var testConfig = resize.ResizeConfig{
	PoolProxyLabel: "app=test-proxy",
	WorkerLabel:    "app=test-worker",
	ProxyCertFile:  "cert.json",
	AnnotationKeys: config.AnnotationKeys{
		PoolProxyID:      "test/proxyid",
		SecretName:       "test/secret",
		TemplateChecksum: "test/checksum",
		WorkerID:         "test/workerid",
		WorkerName:       "test/workername",
	},
}

func TestAddSingleWorker(t *testing.T) {
	resize, mocks := newWithMocks(t)
	expectNoProxiesRunning(mocks.client)

	workerID := 4
	setAddWorkerExpectations(workerID, mocks)

	err := resize.AddWorkers([]int{workerID})
	assert.NoError(t, err, "Error adding a single worker")
}

func TestAddMultipleWorkers(t *testing.T) {
	resize, mocks := newWithMocks(t)
	expectNoProxiesRunning(mocks.client)

	workerIDs := []int{2, 5, 10}
	for _, w := range workerIDs {
		setAddWorkerExpectations(w, mocks)
	}

	err := resize.AddWorkers(workerIDs)
	assert.NoError(t, err, "Error adding multiple workers")
}

func TestAddSingleWorkerAndProxy(t *testing.T) {
	resize, mocks := newWithMocks(t)
	expectNoProxiesRunning(mocks.client)

	workerID := 10
	proxyID := 4
	setAddWorkerExpectationsUsesProxy(workerID, proxyID, mocks)
	setAddProxyExpectations(proxyID, mocks)

	err := resize.AddWorkers([]int{workerID})
	assert.NoError(t, err, "Error adding a single worker and proxy")
}

func TestAddMultipleWorkersAndProxies(t *testing.T) {
	resize, mocks := newWithMocks(t)

	// Configure required proxies for these workers
	workerIDs := []int{1, 2, 4, 5}
	existingProxyID := 3
	newProxyID1 := 6
	newProxyID2 := 7
	setAddWorkerExpectationsUsesProxy(workerIDs[0], existingProxyID, mocks) // Uses existing proxy
	setAddWorkerExpectationsUsesProxy(workerIDs[1], newProxyID1, mocks)
	setAddWorkerExpectationsUsesProxy(workerIDs[2], newProxyID2, mocks)
	setAddWorkerExpectationsUsesProxy(workerIDs[3], newProxyID2, mocks) // Two workers use the same proxy

	// Configure cluster to have one of the required proxies already, plus another proxy
	otherProxyID := 1
	setExistingProxyExpectations([]int{otherProxyID, existingProxyID}, mocks, "test", false)

	for _, p := range []int{newProxyID1, newProxyID2} {
		setAddProxyExpectations(p, mocks)
	}

	err := resize.AddWorkers(workerIDs)
	assert.NoError(t, err, "Unexpected error adding workers")
}

// If all workers associated with a new proxy fail to be created, we should not create the proxy
func TestAddWorkersAndProxiesWorkersFail(t *testing.T) {
	r, mocks := newWithMocks(t)
	expectNoProxiesRunning(mocks.client)

	workerIDs := []int{1, 2, 3}
	proxyID := 10
	for _, w := range workerIDs {
		podSpec := getWorkerSpecUsingProxy(w, proxyID) // All workers use same proxy
		mocks.specFactory.EXPECT().GetWorkerPodSpec(w).Return(&podSpec, nil).Once()
		mocks.client.EXPECT().CreatePod(&podSpec).Return(nil, errors.New("failed")).Once()
	}

	// Expect the proxy to be added, then deleted since we failed to add the workers
	proxyName := setAddProxyExpectations(proxyID, mocks)
	expectDeleteProxy(mocks, proxyName, false)

	err := r.AddWorkers(workerIDs)
	assert.NoError(t, err, "Unexpected error adding workers")
}

// If creating a proxy fails, we should not create corresponding workers
func TestAddWorkersAndProxiesProxyFails(t *testing.T) {
	resize, mocks := newWithMocks(t)
	expectNoProxiesRunning(mocks.client)

	workerIDs := []int{1, 2, 3}
	goodProxyID := 10
	setAddProxyExpectations(goodProxyID, mocks)
	setAddWorkerExpectationsUsesProxy(workerIDs[0], goodProxyID, mocks)

	badProxyID := 11
	spec2 := getWorkerSpecUsingProxy(workerIDs[1], badProxyID)
	spec3 := getWorkerSpecUsingProxy(workerIDs[2], badProxyID)
	mocks.specFactory.EXPECT().GetWorkerPodSpec(workerIDs[1]).Return(&spec2, nil).Once()
	mocks.specFactory.EXPECT().GetWorkerPodSpec(workerIDs[2]).Return(&spec3, nil).Once()
	// These workers should not get created

	// Configure proxy creation to fail
	proxySpec, _ := createProxySpecWithAnnotations(badProxyID, "test", false)
	mocks.specFactory.EXPECT().GetPoolProxyDeploymentSpec(badProxyID).Return(proxySpec, nil).Once()
	mocks.client.EXPECT().CreateDeployment(proxySpec).Return(proxySpec, errors.New("failed")).Once()

	err := resize.AddWorkers(workerIDs)
	assert.NoError(t, err, "Unexpected error adding workers")
}

// Check we can add workers and proxies when proxies need TLS secrets
func TestAddWorkersAndProxiesWithSecrets(t *testing.T) {
	verifyAddWorkersAndProxiesWithSecrets(t, false)
}

// Check that if a proxy secret already exists, we delete it and create a fresh one
func TestAddWorkersAndProxiesWithDrooledSecrets(t *testing.T) {
	verifyAddWorkersAndProxiesWithSecrets(t, true)
}

func verifyAddWorkersAndProxiesWithSecrets(t *testing.T, secretAlreadyExists bool) {
	resize, mocks := newWithMocks(t)
	expectNoProxiesRunning(mocks.client)

	workerIDs := []int{1, 2}
	proxyIDs := []int{3, 4}
	for idx, w := range workerIDs {
		setAddWorkerExpectationsUsesProxy(w, proxyIDs[idx], mocks)
	}

	for _, p := range proxyIDs {
		secretName := setAddProxyExpectationsUsesCert(p, mocks)
		setNewProxyCertExpectations(t, secretName, mocks, secretAlreadyExists)
	}

	err := resize.AddWorkers(workerIDs)
	assert.NoError(t, err, "Unexpected error adding workers")
}

func TestGetWorkersNoWorkers(t *testing.T) {
	resize, mocks := newWithMocks(t)

	// Configure no existing workers
	expectNoWorkersRunning(mocks.client)
	expectNoProxiesRunning(mocks.client)
	mocks.templateChecker.EXPECT().Refresh().Return(nil).Once()
	mocks.templateChecker.EXPECT().GetWorkerTemplateChecksum().Return("abc").Once()

	workers, err := resize.GetAndUpdateWorkers()
	require.NoError(t, err, "Unexpected error getting workers")
	assert.Empty(t, workers, "Expected no workers")
}

func TestGetWorkersClientError(t *testing.T) {
	resize, mocks := newWithMocks(t)
	clientErr := errors.New("could not get deployments")
	mocks.client.EXPECT().GetPodsWithLabel(testConfig.WorkerLabel).Return(nil, clientErr).Once()

	_, err := resize.GetAndUpdateWorkers()
	require.Error(t, err, "Expected error when client call fails")
	assert.Contains(t, err.Error(), clientErr.Error(), "Message should contain client error")
}

func TestGetWorkers(t *testing.T) {
	resize, mocks := newWithMocks(t)

	// Configure existing workers
	expectedWorkers := []controller.DeployedWorker{
		{
			Name:      "worker1",
			ID:        1,
			PodName:   "dep1",
			IsRunning: true,
		},
		{
			Name:      "worker2",
			ID:        2,
			PodName:   "dep2",
			IsRunning: false,
		},
	}
	checksum := "1234"
	setExistingWorkerExpectations(expectedWorkers, mocks, checksum)
	expectNoProxiesRunning(mocks.client)
	mocks.templateChecker.EXPECT().Refresh().Return(nil).Once()
	mocks.templateChecker.EXPECT().GetWorkerTemplateChecksum().Return(checksum).Once()

	gotWorkers, err := resize.GetAndUpdateWorkers()
	require.NoError(t, err, "Unexpected error getting workers")
	assert.ElementsMatch(t, expectedWorkers, gotWorkers, "Unexpected workers returned")
}

func TestGetWorkersDeleteOutdated(t *testing.T) {
	resize, mocks := newWithMocks(t)

	// Configure existing workers
	goodWorker := controller.DeployedWorker{
		Name:      "worker1",
		ID:        1,
		PodName:   "dep1",
		IsRunning: true,
	}
	oldWorker := controller.DeployedWorker{
		Name:      "worker2",
		ID:        2,
		PodName:   "dep2",
		IsRunning: true,
	}
	goodChecksum := "good123"
	goodPod := createWorkerSpecsWithAnnotations([]controller.DeployedWorker{goodWorker}, goodChecksum)[0]
	badPod := createWorkerSpecsWithAnnotations([]controller.DeployedWorker{oldWorker}, "bad456")[0]
	expectGetWorkerPods(mocks, []corev1.Pod{goodPod, badPod})
	expectNoProxiesRunning(mocks.client)

	// Template cheker will return the latest version
	mocks.templateChecker.EXPECT().Refresh().Return(nil).Once()
	mocks.templateChecker.EXPECT().GetWorkerTemplateChecksum().Return(goodChecksum).Once()

	// Expect the outdated worker to be deleted
	mocks.client.EXPECT().DeletePod(badPod.Name).Return(nil).Once()

	gotWorkers, err := resize.GetAndUpdateWorkers()
	require.NoError(t, err, "Unexpected error getting workers")
	assert.ElementsMatch(t, []controller.DeployedWorker{goodWorker}, gotWorkers, "Unexpected workers returned")
}

// Check that if there are proxies that no workers needs, we delete them.
// This should occur if the helm chart is upgraded to a newer MATLAB release that does
// not need pool proxies.
func TestDeleteUnusedProxies(t *testing.T) {
	resize, mocks := newWithMocks(t)

	checksum := "abc"
	proxyIDs := []int{1, 2, 5}
	proxiesUseCerts := true // Make sure we're cleaning up secrets too
	proxyNames := setExistingProxyExpectations(proxyIDs, mocks, "test", proxiesUseCerts)
	expectNoWorkersRunning(mocks.client)
	mocks.templateChecker.EXPECT().Refresh().Return(nil).Once()
	mocks.templateChecker.EXPECT().GetWorkerTemplateChecksum().Return(checksum).Once()

	// Expect all proxy deployments to be deleted
	for _, p := range proxyNames {
		expectDeleteProxy(mocks, p, proxiesUseCerts)
	}

	_, err := resize.GetAndUpdateWorkers()
	assert.NoError(t, err, "Failed to call GetAndUpdateWorkers when stale proxies are present")
}

// Expect GetAndUpdateWorkers to return an error if checking for the latest template version fails
func TestTemplateCheckFailure(t *testing.T) {
	resize, mocks := newWithMocks(t)
	expectNoWorkersRunning(mocks.client)

	// Configure template checker to error
	mocks.templateChecker.EXPECT().Refresh().Return(errors.New("failed to get templates")).Once()
	_, err := resize.GetAndUpdateWorkers()
	require.Error(t, err, "Expect error when checking templates fails")
}

func TestUpdateOutdatedProxies(t *testing.T) {
	resize, mocks := newWithMocks(t)

	goodProxyID := 1 // ID of proxy that should not get updated
	badProxyID := 2  // ID of proxy that should get updated
	createExistingWorkersForProxyIDs([]int{goodProxyID, badProxyID}, mocks)

	latestChecksum := "good"
	goodSpec, _ := createProxySpecWithAnnotations(1, latestChecksum, false)
	outdatedProxyID := 2
	outdatedSpec, _ := createProxySpecWithAnnotations(outdatedProxyID, "badchecksum", false)
	expectGetProxyDeployments(mocks, []appsv1.Deployment{*goodSpec, *outdatedSpec})

	mocks.templateChecker.EXPECT().Refresh().Return(nil).Once()
	mocks.templateChecker.EXPECT().GetPoolProxyTemplateChecksum().Return(latestChecksum).Once()

	// Expect the outdated proxy to be updated with a fresh spec
	newSpec, _ := createProxySpecWithAnnotations(outdatedProxyID, latestChecksum, false)
	expectUpdateProxy(mocks, outdatedProxyID, newSpec)

	_, err := resize.GetAndUpdateWorkers()
	require.NoError(t, err, "Unexpected error updating cluster")
}

// If we update proxies such that they now need certificates, these should be added
func TestAddMissingProxyCerts(t *testing.T) {
	resize, mocks := newWithMocks(t)

	proxyID := 4
	createExistingWorkersForProxyIDs([]int{proxyID}, mocks)

	latestChecksum := "newchecksum"
	outdatedSpec, _ := createProxySpecWithAnnotations(proxyID, "badchecksum", false)
	expectGetProxyDeployments(mocks, []appsv1.Deployment{*outdatedSpec})

	mocks.templateChecker.EXPECT().Refresh().Return(nil).Once()
	mocks.templateChecker.EXPECT().GetPoolProxyTemplateChecksum().Return(latestChecksum).Once()

	// Create new spec that needs a certificate
	newSpec, name := createProxySpecWithAnnotations(proxyID, latestChecksum, true)
	expectUpdateProxy(mocks, proxyID, newSpec)

	// Expect to check for existing secret and create it when it doesn't exist
	mocks.client.EXPECT().SecretExists(name).Return(nil, false, nil).Once()
	setProxyCertExpectations(t, name, mocks)

	_, err := resize.GetAndUpdateWorkers()
	require.NoError(t, err, "Unexpected error updating cluster")
}

// If we update proxies such that they no longer need certificates, these should be deleted
func TestRemovedUnneededProxyCerts(t *testing.T) {
	resize, mocks := newWithMocks(t)

	proxyID := 4
	createExistingWorkersForProxyIDs([]int{proxyID}, mocks)

	latestChecksum := "newchecksum"
	outdatedSpec, _ := createProxySpecWithAnnotations(proxyID, "badchecksum", true) // Old spec that needed a certificate
	expectGetProxyDeployments(mocks, []appsv1.Deployment{*outdatedSpec})

	mocks.templateChecker.EXPECT().Refresh().Return(nil).Once()
	mocks.templateChecker.EXPECT().GetPoolProxyTemplateChecksum().Return(latestChecksum).Once()

	// Create new spec that does not need a certificate
	newSpec, name := createProxySpecWithAnnotations(proxyID, latestChecksum, false)
	expectUpdateProxy(mocks, proxyID, newSpec)

	// Expect to check for existing secret and delete it when it exists
	mocks.client.EXPECT().SecretExists(name).Return(nil, true, nil).Once()
	mocks.client.EXPECT().DeleteSecret(name).Return(nil).Once()

	_, err := resize.GetAndUpdateWorkers()
	require.NoError(t, err, "Unexpected error updating cluster")
}

func TestGetWorkersMissingAnnotation(t *testing.T) {
	resize, mocks := newWithMocks(t)

	workers := []controller.DeployedWorker{
		{
			Name:      "worker1",
			ID:        1,
			PodName:   "dep1",
			IsRunning: true,
		},
		{
			Name:      "worker2",
			ID:        2,
			PodName:   "dep2",
			IsRunning: false,
		},
		{
			Name:      "worker3",
			ID:        3,
			PodName:   "dep3",
			IsRunning: false,
		},
	}
	checksum := "234"
	pods := createWorkerSpecsWithAnnotations(workers, checksum)
	expectNoProxiesRunning(mocks.client)
	mocks.templateChecker.EXPECT().Refresh().Return(nil).Once()
	mocks.templateChecker.EXPECT().GetWorkerTemplateChecksum().Return(checksum).Once()

	// Remove annotations from the second worker
	pods[1].Annotations = map[string]string{}
	expectGetWorkerPods(mocks, pods)

	expectedWorkers := []controller.DeployedWorker{
		workers[0],
		workers[2],
	}
	gotWorkers, err := resize.GetAndUpdateWorkers()
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedWorkers, gotWorkers, "Unexpected workers returned")
}

func TestDeleteWorkers(t *testing.T) {
	resize, mocks := newWithMocks(t)
	expectNoProxiesRunning(mocks.client)

	toDelete := []controller.DeployedWorker{
		{
			Name:    "worker2",
			ID:      2,
			PodName: "worker2-deployment",
		},
		{
			Name:    "worker3",
			ID:      3,
			PodName: "worker3-deployment",
		},
	}
	expectGetWorkers(mocks, toDelete)
	for _, d := range toDelete {
		mocks.client.EXPECT().DeletePod(d.PodName).Return(nil).Once()
	}

	err := resize.DeleteWorkers(toDelete)
	require.NoError(t, err, "Unexpected error deleting workers")
}

// If we fail to delete a worker, we should still delete the others
func TestDeleteWorkersWithFailure(t *testing.T) {
	resize, mocks := newWithMocks(t)
	expectNoProxiesRunning(mocks.client)

	toDelete := []controller.DeployedWorker{
		{
			Name:    "worker2",
			ID:      2,
			PodName: "worker2-deployment",
		},
		{
			Name:    "worker3",
			ID:      3,
			PodName: "worker3-deployment",
		},
	}
	expectGetWorkers(mocks, toDelete)
	mocks.client.EXPECT().DeletePod(toDelete[0].PodName).Return(errors.New("could not delete")).Once()
	mocks.client.EXPECT().DeletePod(toDelete[1].PodName).Return(nil).Once()

	err := resize.DeleteWorkers(toDelete)
	require.NoError(t, err, "Unexpected error deleting workers")
}

func TestDeleteWorkersAndProxies(t *testing.T) {
	verifyDeleteWorkersAndProxies(t, false)
}

func TestDeleteWorkersAndProxiesSecureCommuncation(t *testing.T) {
	verifyDeleteWorkersAndProxies(t, true)
}

func setAddWorkerExpectations(workerID int, mocks *resizeMocks) {
	spec := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("worker-%d", workerID),
		},
	}
	setAddWorkerFromSpecExpectation(workerID, spec, mocks)
}

// Expect to add a worker that uses a pool proxy
func setAddWorkerExpectationsUsesProxy(workerID, proxyID int, mocks *resizeMocks) {
	spec := getWorkerSpecUsingProxy(workerID, proxyID)
	setAddWorkerFromSpecExpectation(workerID, spec, mocks)
}

func getWorkerSpecUsingProxy(workerID, proxyID int) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("worker-%d", workerID),
			Annotations: map[string]string{
				testConfig.AnnotationKeys.PoolProxyID: fmt.Sprintf("%d", proxyID),
			},
		},
	}
}

func setAddWorkerFromSpecExpectation(workerID int, spec corev1.Pod, mocks *resizeMocks) {
	mocks.specFactory.EXPECT().GetWorkerPodSpec(workerID).Return(&spec, nil).Once()
	mocks.client.EXPECT().CreatePod(&spec).Return(&spec, nil).Once()
}

func setExistingWorkerExpectations(workers []controller.DeployedWorker, mocks *resizeMocks, checksum string) {
	pods := createWorkerSpecsWithAnnotations(workers, checksum)
	expectGetWorkerPods(mocks, pods)
}

func setExistingWorkerExpectationsWithProxies(workers []controller.DeployedWorker, proxyIDs []int, mocks *resizeMocks, checksum string) {
	pods := createWorkerSpecsWithAnnotations(workers, checksum)
	for idx := range pods {
		pods[idx].Annotations[testConfig.AnnotationKeys.PoolProxyID] = fmt.Sprintf("%d", proxyIDs[idx])
	}
	expectGetWorkerPods(mocks, pods)
}

func setExistingProxyExpectations(proxyIDs []int, mocks *resizeMocks, checksum string, proxyUsesCert bool) []string {
	names := make([]string, len(proxyIDs))
	deployments := make([]appsv1.Deployment, len(proxyIDs))
	for i, proxyID := range proxyIDs {
		spec, name := createProxySpecWithAnnotations(proxyID, checksum, proxyUsesCert)
		names[i] = name
		deployments[i] = *spec
	}
	expectGetProxyDeployments(mocks, deployments)
	return names
}

func setAddProxyExpectations(proxyID int, mocks *resizeMocks) string {
	spec, name := createProxySpecWithAnnotations(proxyID, "test", false)
	mocks.specFactory.EXPECT().GetPoolProxyDeploymentSpec(proxyID).Return(spec, nil).Once()
	mocks.client.EXPECT().CreateDeployment(spec).Return(spec, nil).Once()
	return name
}

func setAddProxyExpectationsUsesCert(proxyID int, mocks *resizeMocks) string {
	spec, name := createProxySpecWithAnnotations(proxyID, "test", true)
	mocks.specFactory.EXPECT().GetPoolProxyDeploymentSpec(proxyID).Return(spec, nil).Once()
	mocks.client.EXPECT().CreateDeployment(spec).Return(spec, nil).Once()
	return name
}

func setNewProxyCertExpectations(t *testing.T, name string, mocks *resizeMocks, alreadyExists bool) {
	mocks.client.EXPECT().SecretExists(name).Return(nil, alreadyExists, nil).Once()
	if alreadyExists {
		// Should clean up existing secret
		mocks.client.EXPECT().DeleteSecret(name).Return(nil).Once()
	}
	setProxyCertExpectations(t, name, mocks)
}

func setProxyCertExpectations(t *testing.T, name string, mocks *resizeMocks) {
	// Expect to generate a new certificate for this proxy
	secret := certificate.SharedSecret{
		CertPEM: "cert",
		KeyPEM:  "key",
	}
	mocks.certCreator.EXPECT().CreateSharedSecret().Return(&secret, nil).Once()
	cert := certificate.Certificate{
		ServerCert: "server",
		ClientCert: "cert",
		ClientKey:  "key",
	}
	mocks.certCreator.EXPECT().GenerateCertificate(&secret).Return(&cert, nil).Once()

	certBytes, err := json.Marshal(cert)
	require.NoError(t, err, "Failed to marshal certificate")
	secretData := map[string][]byte{
		testConfig.ProxyCertFile: certBytes,
	}
	mocks.client.EXPECT().CreateSecret(name, secretData, false).Return(nil, nil).Once()
}

type resizeMocks struct {
	client          *mockK8s.MockClient
	specFactory     *mocks.MockSpecFactory
	certCreator     *mockCert.MockCertCreator
	templateChecker *mocks.MockTemplateChecker
}

func newWithMocks(t *testing.T) (controller.Resizer, *resizeMocks) {
	mockClient := mockK8s.NewMockClient(t)
	mockCerts := mockCert.NewMockCertCreator(t)
	mockSpecs := mocks.NewMockSpecFactory(t)
	mockTemplate := mocks.NewMockTemplateChecker(t)
	return resize.NewK8sResizer(testConfig, mockClient, mockSpecs, mockTemplate, mockCerts, logging.NewFromZapLogger(zaptest.NewLogger(t))), &resizeMocks{
		client:          mockClient,
		certCreator:     mockCerts,
		specFactory:     mockSpecs,
		templateChecker: mockTemplate,
	}
}

func createWorkerSpecsWithAnnotations(workers []controller.DeployedWorker, checksum string) []corev1.Pod {
	pods := make([]corev1.Pod, len(workers))
	keys := testConfig.AnnotationKeys
	for idx, w := range workers {
		spec := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: w.PodName,
				Annotations: map[string]string{
					keys.WorkerID:         fmt.Sprintf("%d", w.ID),
					keys.WorkerName:       w.Name,
					keys.TemplateChecksum: checksum,
				},
			},
		}
		if w.IsRunning {
			spec.Status.Phase = corev1.PodRunning
		}
		pods[idx] = spec
	}
	return pods
}

func createProxySpecWithAnnotations(p int, checksum string, usesCert bool) (*appsv1.Deployment, string) {
	name := fmt.Sprintf("proxy-%d", p)
	spec := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				testConfig.AnnotationKeys.PoolProxyID:      fmt.Sprintf("%d", p),
				testConfig.AnnotationKeys.TemplateChecksum: checksum,
			},
		},
	}
	if usesCert {
		spec.ObjectMeta.Annotations[testConfig.AnnotationKeys.SecretName] = name
	}
	return &spec, name
}

func expectGetWorkers(mocks *resizeMocks, workers []controller.DeployedWorker) {
	pods := createWorkerSpecsWithAnnotations(workers, "test")
	expectGetWorkerPods(mocks, pods)
}

func expectGetWorkerPods(mocks *resizeMocks, pods []corev1.Pod) {
	list := corev1.PodList{
		Items: pods,
	}
	mocks.client.EXPECT().GetPodsWithLabel(testConfig.WorkerLabel).Return(&list, nil).Once()
}

func expectGetProxyDeployments(mocks *resizeMocks, deps []appsv1.Deployment) {
	list := appsv1.DeploymentList{
		Items: deps,
	}
	mocks.client.EXPECT().GetDeploymentsWithLabel(testConfig.PoolProxyLabel).Return(&list, nil).Once()
}

func expectDeleteProxy(mocks *resizeMocks, name string, hasCert bool) {
	mocks.client.EXPECT().DeleteDeployment(name).Return(nil).Once()
	mocks.client.EXPECT().SecretExists(name).Return(nil, hasCert, nil).Maybe()
	if hasCert {
		mocks.client.EXPECT().DeleteSecret(name).Return(nil).Once()
	}
}

func verifyDeleteWorkersAndProxies(t *testing.T, secure bool) {
	resize, mocks := newWithMocks(t)

	workersToKeep := []controller.DeployedWorker{
		{
			Name:    "worker1",
			ID:      1,
			PodName: "worker1-deployment",
		},
		{
			Name:    "worker2",
			ID:      2,
			PodName: "worker2-deployment",
		},
	}
	workersToDelete := []controller.DeployedWorker{
		{
			Name:    "worker3",
			ID:      3,
			PodName: "worker3-deployment",
		},
		{
			Name:    "worker4",
			ID:      4,
			PodName: "worker4-deployment",
		},
	}
	checksum := "testchecksum"

	// Create some proxies corresponding to these workers
	// Proxy corresponding only to workers that get deleted
	idToDelete := 1 // Corresponds only to workers that get deleted
	idToKeep1 := 2  // Corresponds to a mix of deleted and non-deleted workers
	idToKeep2 := 3  // Only correspoinds to non-deleted workers

	allWorkers := append(workersToKeep, workersToDelete...)
	setExistingWorkerExpectationsWithProxies(allWorkers, []int{idToKeep1, idToKeep2, idToKeep1, idToDelete}, mocks, checksum)

	proxyNames := setExistingProxyExpectations([]int{idToDelete, idToKeep1, idToKeep2}, mocks, "proxychecksum", secure)
	expectDeleteProxy(mocks, proxyNames[0], secure)

	for _, d := range workersToDelete {
		mocks.client.EXPECT().DeletePod(d.PodName).Return(nil).Once()
	}
	err := resize.DeleteWorkers(workersToDelete)
	require.NoError(t, err, "Unexpected error deleting workers")
}

func expectNoWorkersRunning(client *mockK8s.MockClient) {
	client.EXPECT().GetPodsWithLabel(testConfig.WorkerLabel).Return(&corev1.PodList{}, nil).Maybe()
}

func expectNoProxiesRunning(client *mockK8s.MockClient) {
	client.EXPECT().GetDeploymentsWithLabel(testConfig.PoolProxyLabel).Return(&appsv1.DeploymentList{}, nil).Maybe()
}

func createExistingWorkersForProxyIDs(proxyIDs []int, mocks *resizeMocks) {
	// Add a worker that use given proxy IDs
	workerChecksum := "worker123"
	workers := []controller.DeployedWorker{}
	for _, p := range proxyIDs {
		name := fmt.Sprintf("worker-for-%d", p)
		workers = append(workers, controller.DeployedWorker{
			Name:      name,
			PodName:   name,
			IsRunning: true,
			ID:        p,
		})
	}
	setExistingWorkerExpectationsWithProxies(workers, proxyIDs, mocks, workerChecksum)

	// Template checker shows these workers as up-to-date
	mocks.templateChecker.EXPECT().GetWorkerTemplateChecksum().Return(workerChecksum).Maybe()
}

func expectUpdateProxy(mocks *resizeMocks, proxyID int, newSpec *appsv1.Deployment) {
	mocks.specFactory.EXPECT().GetPoolProxyDeploymentSpec(proxyID).Return(newSpec, nil).Once()
	mocks.client.EXPECT().UpdateDeployment(newSpec).Return(newSpec, nil).Once()
}
