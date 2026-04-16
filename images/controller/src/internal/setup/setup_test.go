// Shared functions for setup tests.
// Copyright 2024-2025 The MathWorks, Inc.
package setup_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"

	"controller/internal/config"
	"controller/internal/logging"
	"controller/internal/setup"
	mockCert "controller/mocks/certificate"
	mockK8s "controller/mocks/k8s"
	mocks "controller/mocks/setup"

	corev1 "k8s.io/api/core/v1"
)

type setupMocks struct {
	client         *mockK8s.MockClient
	certCreator    *mockCert.MockCertCreator
	profileCreator *mocks.MockProfileCreator
	marshaller     *mocks.MockMarshaller
}

const testJobManagerName = "TestJobManager"

func newSetupWithMocks(t *testing.T, setupConf setup.SetupConfig, networkConf config.NetworkConfig) (*setup.Setup, *setupMocks) {
	if setupConf.JobManagerName == "" {
		setupConf.JobManagerName = testJobManagerName
	}
	mockClient := mockK8s.NewMockClient(t)
	mockCerts := mockCert.NewMockCertCreator(t)
	mockProfile := mocks.NewMockProfileCreator(t)
	mockMarshal := mocks.NewMockMarshaller(t)
	logger := logging.NewFromZapLogger(zaptest.NewLogger(t))
	return setup.New(setupConf, networkConf, testResources, mockClient, mockCerts, mockProfile, mockMarshal, logger), &setupMocks{
		client:         mockClient,
		certCreator:    mockCerts,
		profileCreator: mockProfile,
		marshaller:     mockMarshal,
	}
}

func expectSecretNotExists(client *mockK8s.MockClient, name string) {
	client.EXPECT().SecretExists(name).Return(nil, false, nil).Once()
}

func expectCheckAndCreateSecret(client *mockK8s.MockClient, name string, key string, data []byte, preserveSecrets bool) {
	// First, check and see that the secret doesn't exist yet
	expectSecretNotExists(client, name)

	// Mock call to create the secret
	client.EXPECT().CreateSecret(name, map[string][]byte{
		key: data,
	}, preserveSecrets).Return(nil, nil).Once()
}

func expectSecretExists(client *mockK8s.MockClient, name string, spec *corev1.Secret) {
	client.EXPECT().SecretExists(name).Return(spec, true, nil).Once()
}

func expectCheckServiceError(client *mockK8s.MockClient, name string) error {
	err := errors.New("client failed to check service")
	client.EXPECT().ServiceExists(name).Return(nil, false, err).Once()
	return err
}

func expectCheckSecretError(client *mockK8s.MockClient, name string) error {
	err := errors.New("client failed to check secret")
	client.EXPECT().SecretExists(name).Return(nil, false, err).Once()
	return err
}

func expectCreateSecretError(client *mockK8s.MockClient, name string) error {
	err := errors.New("client failed create secret")
	client.EXPECT().CreateSecret(name, mock.Anything, mock.Anything).Return(nil, err).Once()
	return err
}

// Create some fake resource names for use in tests
var testResources = config.ResourceNames{
	AdminPasswordSecret:       "test-admin-password",
	AdminPasswordKey:          "test-admin-key",
	CertificateFile:           "test-cert.json",
	ClientMetricsSecret:       "test-client-metrics-secret",
	ClientMetricsCertFile:     "test-client.crt",
	ClientMetricsKeyFile:      "test-client.key",
	Controller:                "test-controller",
	JobManagerContainer:       "test-jm-container",
	JobManagerLabel:           "app=test-jm-label",
	JobManagerService:         "test-jm-service",
	LdapSecret:                "test-ldap-secret",
	LdapCertFile:              "test-ldap.crt",
	LoadBalancer:              "test-lb",
	MetricsSecret:             "test-metrics-secret",
	MetricsCaCertFile:         "test-metrics-ca.crt",
	MetricsCertFile:           "test-metrics.crt",
	MetricsKeyFile:            "test-metrics.key",
	ParallelServerProxySecret: "test-parallel-server-proxy-secret",
	ProfileKey:                "test-profile",
	ProfileSecret:             "test-profile-secret",
	ProfileSecretPre26a:       "test-profile-pre26a-secret",
	SharedSecret:              "test-shared-secret",
	SharedSecretFile:          "test-secret.json",
}

func verifyErrorContainsClientError(t *testing.T, err, clientErr error) {
	assert.Contains(t, err.Error(), clientErr.Error(), "Message should contain error from client")
}
