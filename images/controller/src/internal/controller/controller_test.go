// Copyright 2024-2025 The MathWorks, Inc.
package controller

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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

func TestSetup(t *testing.T) {
	testCases := []struct {
		name                             string
		useSecureCommunication           bool
		securityLevel                    int
		useCustomHost                    bool
		useSecureMetrics                 bool
		openMetricsPortOutsideKubernetes bool
	}{
		{"insecure", false, 0, false, false, false},
		{"secure_sl0", true, 0, false, false, false},
		{"insecure_sl2", false, 2, false, false, false},
		{"secure_sl2", true, 2, false, false, false},
		{"custom_host", false, 0, true, false, false},
		{"secure_metrics_internal", false, 0, false, true, false},
		{"secure_metrics_external", false, 0, true, true, true},
		{"secure_metrics_custom_host", false, 0, true, true, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			verifySetup(tt, SetupTestInputs{
				UseSecureCommunication:           tc.useSecureCommunication,
				SecurityLevel:                    tc.securityLevel,
				UseCustomHost:                    tc.useCustomHost,
				UseSecureMetrics:                 tc.useSecureMetrics,
				OpenMetricsPortOutsideKubernetes: tc.openMetricsPortOutsideKubernetes,
			})
		})
	}
}

// Inputs for setup verificiation
type SetupTestInputs struct {
	UseSecureCommunication           bool
	SecurityLevel                    int
	UseCustomHost                    bool
	UseSecureMetrics                 bool
	OpenMetricsPortOutsideKubernetes bool
}

// Test the full setup workflow, with all secrets and the cluster profile being created
func verifySetup(t *testing.T, inputs SetupTestInputs) {
	const metricsDir = "/test/metrics"
	conf := config.Config{
		JobManagerName:                   "my-k8s-mjs",
		Namespace:                        "test",
		DeploymentName:                   "my-controller",
		LoadBalancerName:                 "my-mjs-loadbalancer",
		SecretFileName:                   "secret.json",
		CertFileName:                     "cert.json",
		BasePort:                         5000,
		PoolProxyBasePort:                30000,
		WorkersPerPoolProxy:              100,
		MaxWorkers:                       10,
		UseSecureCommunication:           inputs.UseSecureCommunication,
		SecurityLevel:                    inputs.SecurityLevel,
		MetricsCertDir:                   metricsDir,
		UseSecureMetrics:                 inputs.UseSecureMetrics,
		OpenMetricsPortOutsideKubernetes: inputs.OpenMetricsPortOutsideKubernetes,
	}
	if inputs.UseCustomHost {
		conf.ClusterHost = "my-custom-host"
	}
	controller, lbAddress := createControllerWithFakeClient(t, &conf)
	if inputs.SecurityLevel >= 2 {
		createDummyAdminPassword(t, controller)
	}

	err := controller.setup()
	require.NoError(t, err, "error running first controller setup")

	// Check the shared secret
	var secret *certificate.SharedSecret
	if inputs.UseSecureCommunication {
		secret = verifySharedSecretCreated(t, controller)
	} else {
		verifyNoSecret(t, controller.client, specs.SharedSecretName)
	}

	verifyJobManagerCreated(t, controller)

	// Check the profile was created with either the custom host name or the load balancer external address
	expectedHost := conf.ClusterHost
	if !inputs.UseCustomHost {
		expectedHost = lbAddress
	}
	verifyClusterProfileCreated(t, controller, secret, expectedHost, false)

	// Check the metrics secrets
	if inputs.UseSecureMetrics {
		verifyMetricsSecretsCreated(t, controller, expectedHost)
	} else {
		verifyNoSecret(t, controller.client, specs.MetricsSecretName)
	}

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
			verifyClusterProfileCreated(tt, controller, secret, lbAddress, tc.requireClientCertificate)
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
		BasePort:            40000,
	}
	zl := zaptest.NewLogger(t)
	fakeK8s := fake.NewSimpleClientset()
	specFactory, err := specs.NewSpecFactory(&conf, types.UID("abc123"))
	require.NoError(t, err)
	client := k8s.NewClientWithK8sBackend(&conf, fakeK8s, logging.NewFromZapLogger(zl))
	controller := &Controller{
		client:            client,
		config:            &conf,
		logger:            logging.NewFromZapLogger(zl),
		specFactory:       specFactory,
		waitForJobManager: func() error { return nil },
	}

	err = controller.setup()
	require.NoError(t, err, "error running first controller setup")
	verifyJobManagerCreated(t, controller)

	// Check the profile was created with the internal hostname of the job manager
	verifyClusterProfileCreated(t, controller, nil, specFactory.GetServiceHostname(specs.JobManagerHostname), false)

	// Verify that we can run setup again without erroring
	// (this can occur if the controller container restarts, and a previous container already created the resources)
	err = controller.setup()
	require.NoError(t, err, "error running second controller setup")
}

// Verify checks for the LDAP certificate secret
func TestCheckLDAPCert(t *testing.T) {
	testCases := []struct {
		name          string
		ldapMountPath string
		addSecret     bool
		expectError   bool
	}{
		{
			name:          "no_ldap_cert_needed",
			ldapMountPath: "",
			addSecret:     false,
			expectError:   false, // Secret is missing, but we don't need it anyway
		}, {
			name:          "ldap_cert_present",
			ldapMountPath: "/test/dir",
			addSecret:     true,
			expectError:   false, // Secret is needed and present
		}, {
			name:          "ldap_cert_missing",
			ldapMountPath: "/test/dir",
			addSecret:     false,
			expectError:   true, // Secret is needed but missing
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			conf := config.Config{
				LDAPCertPath: tc.ldapMountPath,
			}
			controller, _ := createControllerWithFakeClient(tt, &conf)
			if tc.addSecret {
				createDummyLDAPCert(tt, controller)
			}
			err := controller.setup()
			if tc.expectError {
				assert.Error(tt, err, "expected error when LDAP certificate secret is missing")
				assert.Contains(tt, err.Error(), specs.LDAPSecretName, "error message should contain name of LDAP certificate secret")
			} else {
				require.NoError(tt, err, "should not get an error for missing LDAP certificate secret")
			}
		})
	}
}

// Check we can use a pre-existing metrics certificate for encrypted metrics
func TestPreExistingMetricsCert(t *testing.T) {
	conf := config.Config{
		UseSecureCommunication: false,
		UseSecureMetrics:       true,
	}
	controller, _ := createControllerWithFakeClient(t, &conf)
	createDummyMetricsCert(t, controller, "")
	err := controller.setup()
	require.NoError(t, err, "error setting up controller with pre-existing metrics certificate")
}

// Check for errors when the metrics certificate does not contain the correct data
func TestBadMetricsCert(t *testing.T) {
	toExclude := []string{
		specs.MetricsCAFileName,
		specs.MetricsCertFileName,
		specs.MetricsKeyFileName,
	}
	for _, field := range toExclude {
		conf := config.Config{
			UseSecureCommunication: false,
			UseSecureMetrics:       true,
		}
		controller, _ := createControllerWithFakeClient(t, &conf)
		createDummyMetricsCert(t, controller, field)
		err := controller.setup()
		assert.Error(t, err, "expected error when metrics certificate secret is missing some data")
	}
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

// Verify that metrics secrets for the server and client were added to the K8s cluster
func verifyMetricsSecretsCreated(t *testing.T, controller *Controller, externalHost string) {
	// Check the server secret
	secret, exists, err := controller.client.SecretExists(specs.MetricsSecretName)
	require.NoError(t, err)
	require.True(t, exists, "metrics secret should exist")
	require.NotNil(t, secret, "metrics secret should not be nil")
	require.Contains(t, secret.Data, specs.MetricsCAFileName, "missing data in metrics secret")
	require.Contains(t, secret.Data, specs.MetricsCertFileName, "missing data in metrics secret")
	require.Contains(t, secret.Data, specs.MetricsKeyFileName, "missing data in metrics secret")

	// Decode the certificates
	caCert := decodeCert(t, secret.Data[specs.MetricsCAFileName])
	serverCert := decodeCert(t, secret.Data[specs.MetricsCertFileName])

	// Check the certificate contents
	assert.True(t, caCert.IsCA, "expected CA cert")
	err = serverCert.CheckSignatureFrom(caCert)
	assert.NoError(t, err, "server certificate should be signed by CA")

	// Check the hostname on the server certificate
	expectedHost := externalHost
	if !controller.config.OpenMetricsPortOutsideKubernetes {
		expectedHost = controller.specFactory.GetServiceHostname(specs.JobManagerHostname)
	}
	err = serverCert.VerifyHostname(expectedHost)
	assert.NoError(t, err, "server certificate should have the expected hostname")

	// There should be a second secret containing certificates for the client to use
	clientSecret, exists, err := controller.client.SecretExists(clientMetricsCertSecret)
	require.NoError(t, err)
	require.True(t, exists, "client metrics secret should exist")
	require.NotNil(t, secret, "client secret should not be nil")
	require.Contains(t, clientSecret.Data, specs.MetricsCAFileName, "missing data in client metrics secret")
	require.Contains(t, clientSecret.Data, clientCertFilename, "missing data in client metrics secret")
	require.Contains(t, clientSecret.Data, clientKeyFilename, "missing data in client metrics secret")

	// Check the client certificate contents
	clientCACert := decodeCert(t, clientSecret.Data[specs.MetricsCAFileName])
	assert.Equal(t, clientCACert, caCert, "client CA certificate should match the server CA certificate")
	clientCert := decodeCert(t, clientSecret.Data[clientCertFilename])
	err = clientCert.CheckSignatureFrom(caCert)
	assert.NoError(t, err, "client certificate should be signed by CA")
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
	expectedHostWithPort := fmt.Sprintf("%s:%d", expectedHost, controller.config.BasePort)
	assert.Equal(t, expectedHostWithPort, profile.SchedulerComponent.Host, "unexpected profile host")
	if expectCertInProfile {
		assert.Equal(t, secret.CertPEM, profile.SchedulerComponent.Certificate, "profile server certificate should match shared secret certificate")
	} else {
		assert.Empty(t, profile.SchedulerComponent.Certificate, "profile certificate should be empty when not using a shared secret")
	}
}

// Create controller and mock K8s client with a Load Balancer
func createControllerWithFakeClient(t *testing.T, conf *config.Config) (*Controller, string) {
	zl := zaptest.NewLogger(t)
	specFactory, err := specs.NewSpecFactory(conf, types.UID("abcd"))
	require.NoError(t, err)
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

	_, err = client.CreateService(&lb)
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
	secretSpec := controller.specFactory.GetSecretSpec(specs.AdminPasswordSecretName, false)
	secretSpec.Data[specs.AdminPasswordKey] = []byte("testpw")
	_, err := controller.client.CreateSecret(secretSpec)
	require.NoError(t, err)
}

func createDummyLDAPCert(t *testing.T, controller *Controller) {
	secretSpec := controller.specFactory.GetSecretSpec(specs.LDAPSecretName, false)
	secretSpec.Data[controller.config.LDAPCertFile()] = []byte("my-cert-pem")
	_, err := controller.client.CreateSecret(secretSpec)
	require.NoError(t, err)
}

func createDummyMetricsCert(t *testing.T, controller *Controller, fieldToExclude string) {
	secretSpec := controller.specFactory.GetSecretSpec(specs.MetricsSecretName, false)
	secretSpec.Data[specs.MetricsCAFileName] = []byte("my-cert-pem")
	secretSpec.Data[specs.MetricsCertFileName] = []byte("my-server-pem")
	secretSpec.Data[specs.MetricsKeyFileName] = []byte("my-key-pem")
	if fieldToExclude != "" {
		delete(secretSpec.Data, fieldToExclude)
	}
	_, err := controller.client.CreateSecret(secretSpec)
	require.NoError(t, err)
}

func addPortToService(svc *corev1.Service, port int) {
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Port:       int32(port),
		TargetPort: intstr.FromInt(port),
	})
}

func decodeCert(t *testing.T, certPEM []byte) *x509.Certificate {
	block, rest := pem.Decode(certPEM)
	assert.NotNil(t, block, "failed to decode PEM block")
	require.Equal(t, block.Type, "CERTIFICATE", "expected certificate PEM block")
	assert.Empty(t, rest, "expected no remaining data in PEM block")
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}
