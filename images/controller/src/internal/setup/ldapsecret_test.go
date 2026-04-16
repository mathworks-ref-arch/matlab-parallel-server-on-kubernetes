// Copyright 2025 The MathWorks, Inc
package setup_test

import (
	"controller/internal/config"
	"controller/internal/setup"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestCheckLdapCertPositive(t *testing.T) {
	networkConf := config.NetworkConfig{
		RequireLdapCert: true,
	}
	secret := corev1.Secret{
		Data: map[string][]byte{
			testResources.LdapCertFile: []byte("mycert"),
		},
	}

	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectSecretExists(mocks.client, testResources.LdapSecret, &secret)

	err := s.CheckLDAPSecret()
	assert.NoError(t, err, "Expect no error when LDAP secret exists and has the correct key")
}

func TestCheckLdapCertNotNeeded(t *testing.T) {
	networkConf := config.NetworkConfig{
		RequireLdapCert: false,
	}

	s, _ := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	err := s.CheckLDAPSecret() // Should be a no-op
	assert.NoError(t, err, "Expect no error when LDAP secret is not needed")
}

func TestCheckLdapCertMissing(t *testing.T) {
	networkConf := config.NetworkConfig{
		RequireLdapCert: true,
		Namespace:       "my-ns",
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectSecretNotExists(mocks.client, testResources.LdapSecret)

	err := s.CheckLDAPSecret()
	require.Error(t, err, "Expect error when LDAP secret does not exist")
	verifyLdapSecretError(t, err, networkConf)
}

func TestCheckLdapCertClientError(t *testing.T) {
	networkConf := config.NetworkConfig{
		RequireLdapCert: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	clientErr := expectCheckSecretError(mocks.client, testResources.LdapSecret)

	err := s.CheckLDAPSecret()
	require.Error(t, err, "Expect error when client fails to get LDAP secret")
	verifyErrorContainsClientError(t, err, clientErr)
}

func TestCheckLdapCertMissingKey(t *testing.T) {
	networkConf := config.NetworkConfig{
		RequireLdapCert: true,
		Namespace:       "test-ns",
	}
	secret := corev1.Secret{
		Data: map[string][]byte{
			"bad key": []byte("mycert"),
		},
	}

	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectSecretExists(mocks.client, testResources.LdapSecret, &secret)

	err := s.CheckLDAPSecret()
	require.Error(t, err, "Expect error when LDAP secret does not contain the expected key")
	verifyLdapSecretError(t, err, networkConf)
}

func verifyLdapSecretError(t *testing.T, err error, networkConf config.NetworkConfig) {
	msg := err.Error()
	assert.Contains(t, msg, testResources.LdapSecret, "Error message should mention LDAP secret name")
	assert.Contains(t, msg, testResources.LdapCertFile, "Error message should mention LDAP secret key")
	assert.Contains(t, msg, networkConf.Namespace, "Error message should mention Kubernetes namespace")
}
