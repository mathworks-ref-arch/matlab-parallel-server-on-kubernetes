// Copyright 2025 The MathWorks, Inc
package setup_test

import (
	"controller/internal/certificate"
	"controller/internal/config"
	"controller/internal/setup"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// Test creating metrics secrets for a cluster that only exposes metrics inside K8s
func TestCreateMetricsSecretInternal(t *testing.T) {
	verifyCreateMetricsSecret(t, false)
}

// Test creating metrics secrets for a cluster that exposes metrics inside K8s
func TestCreateMetricsSecretExternal(t *testing.T) {
	verifyCreateMetricsSecret(t, true)
}

// Function should be a no-op when we don't need metrics certificates
func TestCreateMetricsCertsNotNeeded(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseSecureMetrics: false,
	}
	s, _ := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	err := s.CreateCertsForMetricsIfNeeded() // Should be no-op
	assert.NoError(t, err, "Expected no error when metrics certificates are not needed")
}

// Test the case where the metrics secrets already exist, so we don't need to make them
func TestCreateMetricsCertsAlreadyExists(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseSecureMetrics: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)

	// Create existing metrics secret
	secret := corev1.Secret{
		Data: map[string][]byte{
			testResources.MetricsCaCertFile: []byte("ca-cert-content"),
			testResources.MetricsCertFile:   []byte("metrics-cert-content"),
			testResources.MetricsKeyFile:    []byte("metrics-key-content"),
		},
	}
	expectSecretExists(mocks.client, testResources.MetricsSecret, &secret)

	err := s.CreateCertsForMetricsIfNeeded()
	assert.NoError(t, err, "Expected no error when correct metrics secret already exists")
}

// Expect an error if a metrics secret exists but does not contain the correct keys
func TestMetricsSecretMissingKeys(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseSecureMetrics: true,
		Namespace:        "test-ns",
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	keysToCheck := []string{
		testResources.MetricsCaCertFile,
		testResources.MetricsCertFile,
		testResources.MetricsKeyFile,
	}

	for _, key := range keysToCheck {
		secret := corev1.Secret{
			Data: map[string][]byte{
				testResources.MetricsCaCertFile: []byte("ca-cert-content"),
				testResources.MetricsCertFile:   []byte("metrics-cert-content"),
				testResources.MetricsKeyFile:    []byte("metrics-key-content"),
			},
		}
		delete(secret.Data, key)
		expectSecretExists(mocks.client, testResources.MetricsSecret, &secret)

		err := s.CreateCertsForMetricsIfNeeded()
		require.Error(t, err, "Expected error when metrics secret exists but has a missing key")
		verifyMetricsSecretError(t, err, networkConf)
	}
}

func verifyMetricsSecretError(t *testing.T, err error, networkConf config.NetworkConfig) {
	msg := err.Error()
	assert.Contains(t, msg, testResources.MetricsSecret, "Error message should mention secret name")
	assert.Contains(t, msg, testResources.MetricsCaCertFile, "Error message should mention CA cert file")
	assert.Contains(t, msg, testResources.MetricsCertFile, "Error message should mention cert file")
	assert.Contains(t, msg, testResources.MetricsKeyFile, "Error message should mention key file")
	assert.Contains(t, msg, networkConf.Namespace, "Error message should mention Kubernetes namespace")
}

func setExpectedMetricsSecret(mocks *setupMocks, secretName, host string, testResources config.ResourceNames, sharedSecret *certificate.SharedSecret, preserveSecrets bool) {
	cert := &certificate.Certificate{
		ServerCert: "server" + secretName,
		ClientCert: "client" + secretName,
		ClientKey:  "clientkey" + secretName,
	}
	if host == "" {
		mocks.certCreator.EXPECT().GenerateCertificate(sharedSecret).Return(cert, nil).Once()
	} else {
		mocks.certCreator.EXPECT().GenerateCertificateWithHostname(sharedSecret, host).Return(cert, nil).Once()
	}

	// Configure K8s client to expect a secret to be created from this cert
	expectedSecret := map[string][]byte{
		testResources.MetricsCaCertFile: []byte(cert.ServerCert),
	}
	if host == "" {
		expectedSecret[testResources.ClientMetricsCertFile] = []byte(cert.ClientCert)
		expectedSecret[testResources.ClientMetricsKeyFile] = []byte(cert.ClientKey)
	} else {
		expectedSecret[testResources.MetricsCertFile] = []byte(cert.ClientCert)
		expectedSecret[testResources.MetricsKeyFile] = []byte(cert.ClientKey)
	}
	mocks.client.EXPECT().CreateSecret(secretName, expectedSecret, preserveSecrets).Return(nil, nil).Once()
}

func verifyCreateMetricsSecret(t *testing.T, openPortsOutsideK8s bool) {
	setupConfig := setup.SetupConfig{
		PreserveSecrets: true,
	}
	networkConf := config.NetworkConfig{
		UseSecureMetrics:                 true,
		Namespace:                        "test-ns",
		ClusterHost:                      "myhost",
		OpenMetricsPortOutsideKubernetes: openPortsOutsideK8s,
		ClusterDomain:                    "my-domain",
	}
	s, mocks := newSetupWithMocks(t, setupConfig, networkConf)

	// Secret does not exist yet
	expectSecretNotExists(mocks.client, testResources.MetricsSecret)

	expectedHost := networkConf.ClusterHost
	if !openPortsOutsideK8s {
		expectedHost = fmt.Sprintf("%s.%s.svc.%s", testResources.JobManagerService, networkConf.Namespace, networkConf.ClusterDomain)
	}

	// Expect to generate a shared secret to use as CA cert
	sharedSecret, _ := expectGenerateSharedSecret(mocks, "metrics")

	// Expect to generate server certificate
	setExpectedMetricsSecret(mocks, testResources.MetricsSecret, expectedHost, testResources, sharedSecret, setupConfig.PreserveSecrets)

	// Expect to generate client certificate; this does not contain the hostname
	setExpectedMetricsSecret(mocks, testResources.ClientMetricsSecret, "", testResources, sharedSecret, setupConfig.PreserveSecrets)

	err := s.CreateCertsForMetricsIfNeeded()
	assert.NoError(t, err, "Expected no error when creating metrics secret")
}
