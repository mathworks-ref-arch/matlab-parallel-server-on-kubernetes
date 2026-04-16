// Copyright 2025 The MathWorks, Inc
package setup_test

import (
	"controller/internal/config"
	"controller/internal/setup"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestCheckAdminPasswordPositive(t *testing.T) {
	networkConf := config.NetworkConfig{
		SecurityLevel: 2,
	}

	secret := corev1.Secret{
		Data: map[string][]byte{
			testResources.AdminPasswordKey: []byte("password"),
		},
	}

	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectSecretExists(mocks.client, testResources.AdminPasswordSecret, &secret)

	err := s.CheckAdminPassword()
	assert.NoError(t, err, "Expect no error when admin password secret exists and has the correct key")
}

func TestCheckAdminPasswordNotNeeded(t *testing.T) {
	networkConf := config.NetworkConfig{
		SecurityLevel: 1,
	}

	s, _ := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	err := s.CheckAdminPassword() // Should be a no-op
	assert.NoError(t, err, "Expect no error when admin password is not needed at SL<2")
}

func TestCheckAdminPasswordMissing(t *testing.T) {
	networkConf := config.NetworkConfig{
		SecurityLevel: 2,
		Namespace:     "my-ns",
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectSecretNotExists(mocks.client, testResources.AdminPasswordSecret)

	err := s.CheckAdminPassword()
	require.Error(t, err, "Expect error when admin password secret does not exist")
	verifyAdminPasswordError(t, err, networkConf)
}

func TestCheckAdminPasswordClientError(t *testing.T) {
	networkConf := config.NetworkConfig{
		SecurityLevel: 2,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	clientErr := expectCheckSecretError(mocks.client, testResources.AdminPasswordSecret)

	err := s.CheckAdminPassword()
	require.Error(t, err, "Expect error when client fails to get secret")
	verifyErrorContainsClientError(t, err, clientErr)
}

func TestCheckAdminPasswordClientMissingKey(t *testing.T) {
	networkConf := config.NetworkConfig{
		SecurityLevel: 2,
		Namespace:     "test-ns",
	}
	secret := corev1.Secret{
		Data: map[string][]byte{
			"badkey": []byte("password"),
		},
	}

	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectSecretExists(mocks.client, testResources.AdminPasswordSecret, &secret)

	err := s.CheckAdminPassword()
	require.Error(t, err, "Expect error when admin password secret does not contain the expected key")
	verifyAdminPasswordError(t, err, networkConf)
}

func verifyAdminPasswordError(t *testing.T, err error, networkConf config.NetworkConfig) {
	msg := err.Error()
	assert.Contains(t, msg, testResources.AdminPasswordSecret, "Error message should mention admin password secret name")
	assert.Contains(t, msg, testResources.AdminPasswordKey, "Error message should mention admin password key")
	assert.Contains(t, msg, networkConf.Namespace, "Error message should mention Kubernetes namespace")
	assert.Contains(t, msg, fmt.Sprintf("%d", networkConf.SecurityLevel), "Error message should mention security level")
}
