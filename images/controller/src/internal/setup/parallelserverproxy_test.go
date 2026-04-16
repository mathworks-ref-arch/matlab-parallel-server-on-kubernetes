// Copyright 2025 The MathWorks, Inc
package setup_test

import (
	"controller/internal/config"
	"controller/internal/setup"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestCreateProxySecretNoSecretError(t *testing.T) {
	s, _ := newSetupWithMocks(t, setup.SetupConfig{}, config.NetworkConfig{
		UseParallelServerProxy:          true,
		ParallelServerProxyUseMutualTLS: true,
	})
	err := s.CreateParallelServerProxySecretIfNeeded()
	assert.Error(t, err, "Expect error when attempting to create proxy secret before loading/creating shared secret")
}

func TestCreateProxySecretAlreadyExists(t *testing.T) {
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, config.NetworkConfig{
		UseParallelServerProxy:          true,
		ParallelServerProxyUseMutualTLS: true,
	})
	setCreateSecretExpectation(t, mocks, false, false)
	mustCallCreateOrLoadSharedSecret(t, s)

	expectSecretExists(mocks.client, testResources.ParallelServerProxySecret, &corev1.Secret{})
	err := s.CreateParallelServerProxySecretIfNeeded()
	assert.NoError(t, err, "Should not get error when secret already exists")
}

func TestCreateProxySecretNotNeeded(t *testing.T) {
	s, _ := newSetupWithMocks(t, setup.SetupConfig{}, config.NetworkConfig{
		UseParallelServerProxy:          true,
		ParallelServerProxyUseMutualTLS: false,
	})
	err := s.CreateParallelServerProxySecretIfNeeded()
	assert.NoError(t, err, "Expected no error when proxy secret is not needed")
}

func TestCreateProxySecret(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy:          true,
		ParallelServerProxyUseMutualTLS: true,
	}

	// Expect shared secret to be created first
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	secret := setCreateSecretExpectation(t, mocks, false, false)
	mustCallCreateOrLoadSharedSecret(t, s)

	// Expect proxy secret to be created
	_, certBytes := expectGenerateCertificate(mocks, secret, "proxy")
	expectCheckAndCreateSecret(mocks.client, testResources.ParallelServerProxySecret, testResources.CertificateFile, certBytes, false)

	err := s.CreateParallelServerProxySecretIfNeeded()
	assert.NoError(t, err, "Expected no error when creating proxy secret")
}
