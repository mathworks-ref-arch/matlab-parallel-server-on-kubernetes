// Copyright 2025 The MathWorks, Inc
package setup_test

import (
	"controller/internal/certificate"
	"controller/internal/config"
	"controller/internal/setup"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// Test generating a shared secret and certificate when preserveSecrets=true
func TestPreserveSecrets(t *testing.T) {
	setupConf := setup.SetupConfig{
		PreserveSecrets: true,
	}
	networkConf := config.NetworkConfig{
		UseSecureCommunication:   true,
		RequireClientCertificate: true,
	}
	s, mocks := newSetupWithMocks(t, setupConf, networkConf)
	setCreateSecretExpectation(t, mocks, true, true)
	mustCallCreateOrLoadSharedSecret(t, s)
}

func TestCreateSharedSecretClientErr(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseSecureCommunication: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	clientErr := expectCheckSecretError(mocks.client, testResources.SharedSecret)

	err := s.CreateOrLoadSharedSecret()
	require.Error(t, err, "Expected error when failing to get secret")
	verifyErrorContainsClientError(t, err, clientErr)
}

func TestCreateSharedSecretCreationErr(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseSecureCommunication: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectSecretNotExists(mocks.client, testResources.SharedSecret)

	createErr := errors.New("failed to create shared secret")
	mocks.certCreator.EXPECT().CreateSharedSecret().Return(nil, createErr).Once()
	err := s.CreateOrLoadSharedSecret()
	require.Error(t, err, "Expected error when failing to create shared secret")
	verifyErrorContainsClientError(t, err, createErr)
}

func TestCreateSharedSecretGenerateCertError(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseSecureCommunication:   true,
		RequireClientCertificate: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectSecretNotExists(mocks.client, testResources.SharedSecret)

	// Successfully create a shared secret
	secret, _ := expectGenerateSharedSecret(mocks, "mysecret")

	// Fail to generate a certificate from it
	createErr := errors.New("failed to generate certificate")
	mocks.certCreator.EXPECT().GenerateCertificate(secret).Return(nil, createErr).Once()
	err := s.CreateOrLoadSharedSecret()
	require.Error(t, err, "Expected error when failing to generate certificate")
	verifyErrorContainsClientError(t, err, createErr)
}

func TestCreateSharedSecretK8sCreationError(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseSecureCommunication: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectSecretNotExists(mocks.client, testResources.SharedSecret)

	// Successfully create a shared secret
	expectGenerateSharedSecret(mocks, "secret")

	// Fail to create the Kubernetes secret
	clientErr := expectCreateSecretError(mocks.client, testResources.SharedSecret)
	err := s.CreateOrLoadSharedSecret()
	require.Error(t, err, "Expected error when failing to create K8s secret")
	verifyErrorContainsClientError(t, err, clientErr)
}

func TestLoadSharedSecretMissingFile(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseSecureCommunication: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)

	// Configure client to return a secret that does not contain the expected shared secret file
	secret := corev1.Secret{
		Data: map[string][]byte{
			"some-other-file.json": []byte("some data"),
		},
	}
	mocks.client.EXPECT().SecretExists(testResources.SharedSecret).Return(&secret, true, nil).Once()

	err := s.CreateOrLoadSharedSecret()
	require.Error(t, err, "Expect error when shared secret does not contain the expected shared secret file")
	assert.Contains(t, err.Error(), testResources.SharedSecretFile, "Message should mention the missing filename")
}

func TestLoadSharedSecretCannotLoad(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseSecureCommunication: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)

	// Configure client to return a secret with some bytes that cannot be successfully loaded into a shared secret
	badBytes := []byte("this is invalid")
	secret := corev1.Secret{
		Data: map[string][]byte{
			testResources.SharedSecretFile: badBytes,
		},
	}
	mocks.client.EXPECT().SecretExists(testResources.SharedSecret).Return(&secret, true, nil).Once()
	loadErr := errors.New("could not parse secret")
	mocks.certCreator.EXPECT().LoadSharedSecret(badBytes).Return(nil, loadErr).Once()

	err := s.CreateOrLoadSharedSecret()
	require.Error(t, err, "Expect error when shared secret cannot be parsed")
	verifyErrorContainsClientError(t, err, loadErr)
}

func mustCallCreateOrLoadSharedSecret(t *testing.T, s *setup.Setup) {
	err := s.CreateOrLoadSharedSecret()
	assert.NoError(t, err, "Expect no error from CreateOrLoadSharedSecret")
}

func setCreateSecretExpectation(t *testing.T, mocks *setupMocks, needCert, preserveSecrets bool) *certificate.SharedSecret {
	sharedSecret, secretBytes := expectGenerateSharedSecret(mocks, "mysecret")
	secretData := map[string][]byte{
		testResources.SharedSecretFile: secretBytes,
	}

	// Generate a certificate if needed
	if needCert {
		_, bytes := expectGenerateCertificate(mocks, sharedSecret, "mycert")
		secretData[testResources.CertificateFile] = bytes
	}

	// Configure mock client to expect secret to be checked and created
	mocks.client.EXPECT().SecretExists(testResources.SharedSecret).Return(nil, false, nil).Once()
	mocks.client.EXPECT().CreateSecret(testResources.SharedSecret, secretData, preserveSecrets).Return(nil, nil).Once()
	return sharedSecret
}

// Configure mocks so that there is an existing shared secret
func setExistingSharedSecret(mocks *setupMocks) *certificate.SharedSecret {
	sharedSecret := &certificate.SharedSecret{
		CertPEM: "existing-certpem",
		KeyPEM:  "existing-keypem",
	}
	secretBytes := []byte("mysecret")
	secret := corev1.Secret{
		Data: map[string][]byte{
			testResources.SharedSecretFile: secretBytes,
		},
	}
	mocks.client.EXPECT().SecretExists(testResources.SharedSecret).Return(&secret, true, nil).Once()
	mocks.certCreator.EXPECT().LoadSharedSecret(secretBytes).Return(sharedSecret, nil).Once()
	return sharedSecret
}

func expectGenerateSharedSecret(mocks *setupMocks, uniqueName string) (*certificate.SharedSecret, []byte) {
	sharedSecret := certificate.SharedSecret{
		CertPEM: "certpem" + uniqueName,
		KeyPEM:  "keypem" + uniqueName,
	}
	mocks.certCreator.EXPECT().CreateSharedSecret().Return(&sharedSecret, nil).Once()
	secretBytes := []byte(uniqueName)
	mocks.marshaller.EXPECT().Marshal(&sharedSecret).Return(secretBytes, nil).Maybe()
	return &sharedSecret, secretBytes
}

func expectGenerateCertificate(mocks *setupMocks, sharedSecret *certificate.SharedSecret, uniqueName string) (*certificate.Certificate, []byte) {
	cert := &certificate.Certificate{
		ServerCert: "server-cert-" + uniqueName,
		ClientCert: "test-cert-" + uniqueName,
		ClientKey:  "test-key-" + uniqueName,
	}
	mocks.certCreator.EXPECT().GenerateCertificate(sharedSecret).Return(cert, nil).Once()
	certBytes := []byte(uniqueName)
	mocks.marshaller.EXPECT().Marshal(cert).Return(certBytes, nil).Maybe()
	return cert, certBytes
}
