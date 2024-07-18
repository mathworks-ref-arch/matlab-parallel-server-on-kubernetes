// Copyright 2024 The MathWorks, Inc.
package controller

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/mathworks/mjssetup/pkg/certificate"
	"github.com/mathworks/mjssetup/pkg/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"controller/internal/config"
	"controller/internal/k8s"
	"controller/internal/logging"
	"controller/internal/specs"
	mockClient "controller/mocks/k8s"
	mockRescaler "controller/mocks/rescaler"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Verify that the controller automatically calls the rescaling function
func TestRunAutoscaling(t *testing.T) {
	period := 1

	mockRescaler := mockRescaler.NewRescaler(t)
	mockRescaler.EXPECT().Rescale()

	controller := Controller{
		rescaler: mockRescaler,
		period:   time.Duration(period) * time.Second,
		stopChan: make(chan bool),
		logger:   logging.NewFromZapLogger(zaptest.NewLogger(t)),
	}

	// Run for long enough to ensure the scaler gets called at least once
	runFor := 5.0 * period
	doneChan := make(chan bool)
	go func() {
		controller.Run()
		doneChan <- true
	}()
	time.Sleep(time.Duration(runFor) * time.Second)
	controller.Stop()

	// Wait for controller.Run() to return
	<-doneChan
}

func TestVerifySetup(t *testing.T) {
	testCases := []struct {
		name                   string
		useSecureCommunication bool
		securityLevel          int
	}{
		{"insecure", false, 0},
		{"secure_sl0", true, 0},
		{"insecure_sl2", false, 2},
		{"secure_sl2", true, 2},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			verifySetup(tt, tc.useSecureCommunication, tc.securityLevel, false)
		})
	}
}

// Test use of a custom cluster host name in the cluster profile
func TestCustomClusterHost(t *testing.T) {
	verifySetup(t, false, 0, true)
}

// Test the full setup workflow, with all secrets and the cluster profile being created
func verifySetup(t *testing.T, useSecureCommunication bool, securityLevel int, useCustomHost bool) {
	conf := config.Config{
		JobManagerName:         "my-k8s-mjs",
		Namespace:              "test",
		DeploymentName:         "my-controller",
		LoadBalancerName:       "my-mjs-loadbalancer",
		SecretFileName:         "secret.json",
		CertFileName:           "cert.json",
		BasePort:               5000,
		PoolProxyBasePort:      30000,
		WorkersPerPoolProxy:    100,
		MaxWorkers:             10,
		UseSecureCommunication: useSecureCommunication,
		SecurityLevel:          securityLevel,
	}
	if useCustomHost {
		conf.ClusterHost = "my-custom-host"
	}
	controller, lbAddress := createControllerWithFakeClient(t, &conf)
	if securityLevel >= 2 {
		createDummyAdminPassword(t, controller)
	}

	err := controller.setup()
	require.NoError(t, err, "error running first controller setup")

	var secret *certificate.SharedSecret
	if useSecureCommunication {
		secret = verifySharedSecretCreated(t, controller)
	} else {
		verifyNoSecret(t, controller.client, specs.SharedSecretName)
	}

	verifyJobManagerCreated(t, controller)

	// Check the profile was created with either the custom host name or the load balancer external address
	expectedHost := conf.ClusterHost
	if !useCustomHost {
		expectedHost = fmt.Sprintf("%s:%d", lbAddress, controller.config.BasePort)
	}
	verifyClusterProfileCreated(t, controller, secret, expectedHost, false)

	// Verify that we can run setup again without erroring
	// (this can occur if the controller container restarts, and a previous container already created the resources)
	err = controller.setup()
	require.NoError(t, err, "error running second controller setup")
}

// Check we get the expected error when the admin password secret is missing
func TestErrorMissingAdminPassword(t *testing.T) {
	testCases := []struct {
		securityLevel int
		expectError   bool
	}{
		{1, false},
		{2, true},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("sl%d", tc.securityLevel), func(tt *testing.T) {
			conf := config.Config{
				SecurityLevel: tc.securityLevel,
			}
			controller, _ := createControllerWithFakeClient(tt, &conf)
			err := controller.setup()
			if tc.expectError {
				assert.Error(tt, err, "expected error when admin password secret is missing")
				assert.Contains(tt, err.Error(), specs.AdminPasswordSecretName, "error message should contain name of admin password secret")
			} else {
				require.NoError(tt, err, "should not get an error for missing admin password secret with security level < 2")
			}
		})
	}
}

func TestWaitForJobManager(t *testing.T) {
	client := mockClient.NewClient(t)

	// First return false, then true
	client.EXPECT().IsJobManagerReady().Once().Return(false, nil)
	client.EXPECT().IsJobManagerReady().Once().Return(true, nil)

	err := waitForJobManager(client, 1)
	require.NoError(t, err)
}

// Verify that the correct profile is created with different settings for RequireClientCertificate
func TestCreateProfile(t *testing.T) {
	testCases := []struct {
		name                     string
		requireClientCertificate bool
	}{
		{"no_client_cert", false},
		{"with_client_cert", true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			conf := config.Config{
				RequireClientCertificate: tc.requireClientCertificate,
				JobManagerName:           tc.name,
			}
			controller, lbAddress := createControllerWithFakeClient(tt, &conf)
			var secret *certificate.SharedSecret
			var err error
			if tc.requireClientCertificate {
				secret, err = certificate.New().CreateSharedSecret()
				require.NoError(tt, err)
			}
			err = controller.createProfile(secret)
			require.NoError(t, err)
			expectedHost := fmt.Sprintf("%s:%d", lbAddress, controller.config.BasePort)
			verifyClusterProfileCreated(tt, controller, secret, expectedHost, tc.requireClientCertificate)
		})
	}
}

// Verify that we get an error when the load balancer has not been created prior to setup
func TestErrorMissingLoadBalancer(t *testing.T) {
	conf := config.Config{
		LoadBalancerName: "missing-lb",
	}
	controller, _ := createControllerWithFakeClient(t, &conf)

	// Delete the Load Balancer
	err := controller.client.DeleteService(conf.LoadBalancerName)
	require.NoError(t, err)

	// Check that setup fails with an appropriate error
	err = controller.setup()
	assert.Error(t, err, "expected an error when attempting to set up controller with missing load balancer")
	assert.Contains(t, err.Error(), conf.LoadBalancerName, "expected error message to contain name of missing load balancer")
}

// Verify that we get an error when the load balancer has a mismatched port/targetport
func TestErrorLoadBalancerMismatch(t *testing.T) {
	conf := config.Config{
		LoadBalancerName: "my-lb",
	}
	controller, _ := createControllerWithFakeClient(t, &conf)

	// Modify the load balancer
	svc, err := controller.client.GetService(conf.LoadBalancerName)
	require.NoError(t, err)
	targetPort := svc.Spec.Ports[0].TargetPort.IntVal
	newPort := targetPort + 1
	svc.Spec.Ports[0].Port = newPort
	err = controller.client.UpdateService(svc)
	require.NoError(t, err)

	// Check that setup fails with an appropriate error
	err = controller.setup()
	assert.Error(t, err, "expected an error when attempting to set up controller with a load balancer where the target port does not match the service port")
	assert.Contains(t, err.Error(), fmt.Sprintf("%d", targetPort), "expected error message to contain mismatched target port")
	assert.Contains(t, err.Error(), fmt.Sprintf("%d", newPort), "expected error message to contain mismatched service port")
}

// Verify that we get an error when the load balancer is missing a required port
func TestErrorLoadBalancerMissingPort(t *testing.T) {
	conf := config.Config{
		LoadBalancerName:    "my-lb",
		WorkersPerPoolProxy: 2,
		MaxWorkers:          4,
		BasePort:            1000,
		PoolProxyBasePort:   2000,
	}
	requiredPorts := []int{conf.BasePort + 6, conf.BasePort + 9, conf.PoolProxyBasePort, conf.PoolProxyBasePort + 1}

	for _, port := range requiredPorts {
		controller, _ := createControllerWithFakeClient(t, &conf)

		// Modify the load balancer to remove a required port
		svc, err := controller.client.GetService(conf.LoadBalancerName)
		require.NoError(t, err)
		portsToKeep := []corev1.ServicePort{}
		for _, svcPort := range svc.Spec.Ports {
			if svcPort.Port != int32(port) {
				portsToKeep = append(portsToKeep, svcPort)
			}
		}
		require.Len(t, portsToKeep, len(requiredPorts)-1, "expected one port to be removed from the load balancer")
		svc.Spec.Ports = portsToKeep
		err = controller.client.UpdateService(svc)
		require.NoError(t, err)

		// Check that setup fails with an appropriate error
		err = controller.setup()
		assert.Error(t, err, "expected an error when attempting to set up controller with a load balancer with a missing port")
		assert.Contains(t, err.Error(), fmt.Sprintf("%d", port), "expected error message to contain missing port number")
	}
}

// Test setup for a cluster with internalClientsOnly=true
func TestInternalClientsOnly(t *testing.T) {
	conf := config.Config{
		JobManagerName:      "my-k8s-mjs",
		Namespace:           "test",
		DeploymentName:      "my-controller",
		LoadBalancerName:    "my-mjs-loadbalancer",
		InternalClientsOnly: true,
		MaxWorkers:          10,
	}
	zl := zaptest.NewLogger(t)
	fakeK8s := fake.NewSimpleClientset()
	specFactory := specs.NewSpecFactory(&conf, types.UID("abc123"))
	client := k8s.NewClientWithK8sBackend(&conf, fakeK8s, logging.NewFromZapLogger(zl))
	controller := &Controller{
		client:            client,
		config:            &conf,
		logger:            logging.NewFromZapLogger(zl),
		specFactory:       specFactory,
		waitForJobManager: func() error { return nil },
	}

	err := controller.setup()
	require.NoError(t, err, "error running first controller setup")
	verifyJobManagerCreated(t, controller)

	// Check the profile was created with the internal hostname of the job manager
	expectedHost := specFactory.GetServiceHostname(specs.JobManagerHostname)
	verifyClusterProfileCreated(t, controller, nil, expectedHost, false)

	// Verify that we can run setup again without erroring
	// (this can occur if the controller container restarts, and a previous container already created the resources)
	err = controller.setup()
	require.NoError(t, err, "error running second controller setup")
}

// Verify that a shared secret was added to the K8s cluster
func verifySharedSecretCreated(t *testing.T, controller *Controller) *certificate.SharedSecret {
	secret, exists, err := controller.getExistingSharedSecret()
	require.NoError(t, err)
	require.True(t, exists, "shared secret should exist")
	require.NotNil(t, secret, "shared secret should not be nil")
	return secret
}

// Verify that a secret does not exist
func verifyNoSecret(t *testing.T, client k8s.Client, name string) {
	_, exists, err := client.SecretExists(name)
	require.NoError(t, err)
	assert.Falsef(t, exists, "secret %s should not exist", name)
}

// Verify that the job manager was created
func verifyJobManagerCreated(t *testing.T, controller *Controller) {
	expectedDeployment := controller.specFactory.GetJobManagerDeploymentSpec()
	_, err := controller.client.GetDeployment(expectedDeployment.Name)
	require.NoError(t, err)
}

// Verify that the cluster profile was created
func verifyClusterProfileCreated(t *testing.T, controller *Controller, secret *certificate.SharedSecret, expectedHost string, expectCertInProfile bool) {
	k8sSecret, exists, err := controller.client.SecretExists(profileSecretName)
	require.NoError(t, err)
	require.True(t, exists, "cluster profile secret should exist")
	require.Contains(t, k8sSecret.Data, profileKey, "profile secret should contain profile data key")

	// Extract the profile
	profBytes := k8sSecret.Data[profileKey]
	var profile profile.Profile
	err = json.Unmarshal(profBytes, &profile)
	require.NoError(t, err, "error unmarshaling profile from K8s secret")

	// Check the profile contents
	assert.Equal(t, controller.config.JobManagerName, profile.Name, "profile name should match job manager name")
	assert.Equal(t, expectedHost, profile.SchedulerComponent.Host, "unexpected profile host")
	if expectCertInProfile {
		assert.Equal(t, secret.CertPEM, profile.SchedulerComponent.Certificate, "profile server certificate should match shared secret certificate")
	} else {
		assert.Empty(t, profile.SchedulerComponent.Certificate, "profile certificate should be empty when not using a shared secret")
	}
}

// Create controller and mock K8s client with a Load Balancer
func createControllerWithFakeClient(t *testing.T, conf *config.Config) (*Controller, string) {
	zl := zaptest.NewLogger(t)
	specFactory := specs.NewSpecFactory(conf, types.UID("abcd"))
	fakeK8s := fake.NewSimpleClientset()
	client := k8s.NewClientWithK8sBackend(conf, fakeK8s, logging.NewFromZapLogger(zl))

	// Create a dummy LoadBalancer on the cluster
	lbAddress := "1.2.3.4"
	lb := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: conf.LoadBalancerName,
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						IP: lbAddress,
					},
				},
			},
		},
	}

	// Add job manager ports to the load balancer
	addPortToService(&lb, conf.BasePort+6)
	addPortToService(&lb, conf.BasePort+9)

	// Add pool proxy ports to the load balancer
	workersCovered := 0
	idx := 0
	for workersCovered < conf.MaxWorkers {
		addPortToService(&lb, conf.PoolProxyBasePort+idx)
		idx++
		workersCovered += conf.WorkersPerPoolProxy
	}

	_, err := client.CreateService(&lb)
	require.NoError(t, err, "error creating dummy load balancer")

	controller := &Controller{
		client:            client,
		config:            conf,
		logger:            logging.NewFromZapLogger(zl),
		specFactory:       specFactory,
		waitForJobManager: func() error { return nil },
	}
	return controller, lbAddress
}

func createDummyAdminPassword(t *testing.T, controller *Controller) {
	secretSpec := controller.specFactory.GetSecretSpec(specs.AdminPasswordSecretName)
	secretSpec.Data[specs.AdminPasswordKey] = []byte("testpw")
	_, err := controller.client.CreateSecret(secretSpec)
	require.NoError(t, err)
}

func addPortToService(svc *corev1.Service, port int) {
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Port:       int32(port),
		TargetPort: intstr.FromInt(port),
	})
}
