// Copyright 2024-2026 The MathWorks, Inc.
package setup_test

import (
	"controller/internal/certificate"
	"controller/internal/config"
	"controller/internal/setup"
	"errors"
	"testing"

	"github.com/mathworks/mjssetup/pkg/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// If we try to create a profile before checking the shared secret, expect an error
func TestCreateProfileNoSecretError(t *testing.T) {
	s, _ := newSetupWithMocks(t, setup.SetupConfig{}, config.NetworkConfig{
		RequireClientCertificate: true,
		UseSecureCommunication:   true,
	})
	err := s.CreateProfile()
	assert.Error(t, err, "Expect error when attempting to create profile before loading/creating shared secret")
}

// Test creating a cluster profile when no shared secret is needed
func TestCreateProfileNoSecret(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy: true,
		ClusterHost:            "testhost",
		UseSecureCommunication: false, // No need for secret
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	verifyCreateSingleProfile(t, s, mocks, networkConf.ClusterHost, nil, true)
}

// Test creating a profile when there is a shared secret, but the profile doesn't need a certificate
func TestCreateProfileNoCert(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy:   true,
		ClusterHost:              "testhost",
		UseSecureCommunication:   true,  // Triggers secret to be created
		RequireClientCertificate: false, // Profile should not use secret
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	setCreateSecretExpectation(t, mocks, false, false)

	// If we create a profile now, it should not contain a certificate
	verifyCreateSingleProfile(t, s, mocks, networkConf.ClusterHost, nil, true)
}

// Test creating a profile when we need to generate both a shared secret and a client certificate
func TestCreateProfileWithCert(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy:   true,
		ClusterHost:              "testhost",
		UseSecureCommunication:   true,
		RequireClientCertificate: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	secret := setCreateSecretExpectation(t, mocks, true, false)

	// If we create a profile now, it should should contain a certificate generated from the secret
	verifyCreateSingleProfile(t, s, mocks, networkConf.ClusterHost, secret, true)
}

// Test creating a cluster profile from a shared secret that already exists
func TestCreateProfileExistingSecret(t *testing.T) {
	networkConf := config.NetworkConfig{
		ClusterHost:              "myhost",
		UseParallelServerProxy:   true,
		RequireClientCertificate: true,
		UseSecureCommunication:   true,
	}

	// Create an existing shared secret
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	sharedSecret := setExistingSharedSecret(mocks)

	// When we create a profile, it should use the existing secret
	verifyCreateSingleProfile(t, s, mocks, networkConf.ClusterHost, sharedSecret, true)
}

// Test creating a non-Socks profile for a cluster that only supports pre-26a MATLABs
func TestCreateProfilePreSocks(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy:   false,
		UsePoolProxy:             true,
		ClusterHost:              "testhost",
		UseSecureCommunication:   true,
		RequireClientCertificate: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	secret := setCreateSecretExpectation(t, mocks, true, false)

	// If we create a profile now, it should should contain a certificate generated from the secret
	verifyCreateSingleProfile(t, s, mocks, networkConf.ClusterHost, secret, false)
}

// Test creating both a socks and a non-socks profile for a backwards-compatible cluster in the case where both profiles need a certificate
func TestCreateProfilesBackwardsCompatibleWithCerts(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy:   true,
		UsePoolProxy:             true,
		ClusterHost:              "testhost",
		UseSecureCommunication:   true,
		RequireClientCertificate: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	secret := setCreateSecretExpectation(t, mocks, true, false)
	mustCallCreateOrLoadSharedSecret(t, s)

	setProfileExpectations(mocks, testResources.ProfileSecret, networkConf.ClusterHost, secret, true, false)
	setProfileExpectations(mocks, testResources.ProfileSecretPre26a, networkConf.ClusterHost, secret, false, false)

	err := s.CreateProfile()
	require.NoError(t, err, "Expected no error when creating both profiles")
}

// Test creating both a socks and a non-socks profile for a backwards-compatible cluster in the case where only the socks profile needs a certificate
func TestCreateProfilesBackwardsCompatibleSocksCert(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy:          true,
		UsePoolProxy:                    true,
		ClusterHost:                     "testhost",
		ParallelServerProxyUseMutualTLS: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	secret := setCreateSecretExpectation(t, mocks, false, false)
	mustCallCreateOrLoadSharedSecret(t, s)

	setProfileExpectations(mocks, testResources.ProfileSecret, networkConf.ClusterHost, secret, true, false)
	setProfileExpectations(mocks, testResources.ProfileSecretPre26a, networkConf.ClusterHost, nil, false, false)

	err := s.CreateProfile()
	require.NoError(t, err, "Expected no error when creating both profiles")
}

// Test case where the profile already exists
func TestCreateProfileAlreadyExists(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy: true,
		UsePoolProxy:           false,
		ClusterHost:            "test",
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectUpToDateProfileExists(mocks, testResources.ProfileSecret, &profile.Profile{Name: "test"}, networkConf.ClusterHost, nil, false, networkConf.UseParallelServerProxy)
	mustCallCreateOrLoadSharedSecret(t, s)
	err := s.CreateProfile()
	assert.NoError(t, err, "Should not get error when profile already exists")
}

// Test case where the main profile already exists but the pre-26a profile needs to be created
func TestCreateProfileAlreadyExistsButNeedPre26a(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy:   true,
		UsePoolProxy:             true,
		ClusterHost:              "myhost",
		RequireClientCertificate: true,
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	secret := setCreateSecretExpectation(t, mocks, true, false)
	mustCallCreateOrLoadSharedSecret(t, s)

	// Main profile already exists
	expectUpToDateProfileExists(mocks, testResources.ProfileSecret, &profile.Profile{Name: "test"}, networkConf.ClusterHost, secret, true, networkConf.UseParallelServerProxy)

	// Need to create backwards-compatible profile
	setProfileExpectations(mocks, testResources.ProfileSecretPre26a, networkConf.ClusterHost, secret, false, false)

	err := s.CreateProfile()
	assert.NoError(t, err, "Unexpected error creating profile")
}

// Test case where we need a backwards-compatible profile, and both profiles already exist
func TestCreateProfileBothAlreadyExist(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy: true,
		UsePoolProxy:           true,
		ClusterHost:            "test",
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	mustCallCreateOrLoadSharedSecret(t, s)

	// Profiles already exist
	expectUpToDateProfileExists(mocks, testResources.ProfileSecret, &profile.Profile{Name: "test"}, networkConf.ClusterHost, nil, false, true)
	expectUpToDateProfileExists(mocks, testResources.ProfileSecretPre26a, &profile.Profile{Name: "test-pre26a"}, networkConf.ClusterHost, nil, false, false)

	err := s.CreateProfile()
	assert.NoError(t, err, "Should not get error calling CreateProfile when both profiles already exist")
}

// Test creating both a socks and a non-socks profile for a backwards-compatible cluster in the case where neither profile needs a certificate
func TestCreateProfilesBackwardsCompatibleNoCerts(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy: true,
		UsePoolProxy:           true,
		ClusterHost:            "testhost",
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	mustCallCreateOrLoadSharedSecret(t, s)

	setProfileExpectations(mocks, testResources.ProfileSecret, networkConf.ClusterHost, nil, true, false)
	setProfileExpectations(mocks, testResources.ProfileSecretPre26a, networkConf.ClusterHost, nil, false, false)

	err := s.CreateProfile()
	assert.NoError(t, err, "Unexpected error creating both profiles")
}

// Test creating a cluster profile with a hostname extracted from the load balancer
func TestCreateProfileLoadBalancerHostName(t *testing.T) {
	hostname := "load-balancer-host"
	svc := &corev1.Service{
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						Hostname: hostname,
					},
				},
			},
		},
	}
	verifyCreateProfileLoadBalancerHost(t, svc, hostname, false)
}

// Test creating a cluster profile with an IP address extracted from the load balancer
func TestCreateProfileLoadBalancerIP(t *testing.T) {
	hostname := "12.13.14.15"
	svc := &corev1.Service{
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						IP: hostname,
					},
				},
			},
		},
	}
	verifyCreateProfileLoadBalancerHost(t, svc, hostname, false)
}

// Test creating a cluster profile with an IP address extracted from the load balancer in the case where the load balancer initially has no external address yet
func TestCreateProfileLoadBalancerWithDelay(t *testing.T) {
	hostname := "12.13.14.15"
	svc := &corev1.Service{
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						IP: hostname,
					},
				},
			},
		},
	}
	verifyCreateProfileLoadBalancerHost(t, svc, hostname, true)
}

// Test the case where we need to update the profile because it is outdated
func TestUpdateProfile(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy: true,
		ClusterHost:            "testhost",
		UseSecureCommunication: false, // No need for secret
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	expectOutdatedProfileExists(mocks, testResources.ProfileSecret, networkConf.ClusterHost, nil, true)
	verifyUpdateSingleProfile(t, s, mocks, networkConf.ClusterHost, nil, true)
}

// Test the case where we need to update both profiles (current and backwards compatible) because they are outdated
func TestUpdateBothProfiles(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy: true,
		UsePoolProxy:           true,
		ClusterHost:            "testhost",
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)
	mustCallCreateOrLoadSharedSecret(t, s)

	expectUpdate := true
	expectOutdatedProfileExists(mocks, testResources.ProfileSecret, networkConf.ClusterHost, nil, true)
	setProfileExpectations(mocks, testResources.ProfileSecret, networkConf.ClusterHost, nil, true, expectUpdate)
	expectOutdatedProfileExists(mocks, testResources.ProfileSecretPre26a, networkConf.ClusterHost, nil, false)
	setProfileExpectations(mocks, testResources.ProfileSecretPre26a, networkConf.ClusterHost, nil, false, expectUpdate)

	err := s.CreateProfile()
	assert.NoError(t, err, "Unexpected error updating both profiles")
}

// Test the case where we need to update the profile because the existing secret does not contain the expected data (e.g. because the chart was modified to use a different key)
func TestUpdateProfileMissingData(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy: true,
		ClusterHost:            "testhost",
		UseSecureCommunication: false, // No need for secret
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)

	// Expect Kubernetes secret to exist, but does not contain the expected key
	expectSecretExists(mocks.client, testResources.ProfileSecret, &corev1.Secret{
		Data: map[string][]byte{
			"bad-key": []byte("test data"),
		},
	})
	verifyUpdateSingleProfile(t, s, mocks, networkConf.ClusterHost, nil, true)
}

// Test the case where we need to update the profile because the existing secret does not contain a valid profile
// (Unlikely to happen, but could occur if e.g. someone manually modified the secret)
func TestUpdateProfileBadData(t *testing.T) {
	networkConf := config.NetworkConfig{
		UseParallelServerProxy: true,
		ClusterHost:            "testhost",
		UseSecureCommunication: false, // No need for secret
	}
	s, mocks := newSetupWithMocks(t, setup.SetupConfig{}, networkConf)

	// Expect Kubernetes secret to exist, but cannot be parsed
	badData := []byte("can't parse this")
	expectSecretExists(mocks.client, testResources.ProfileSecret, &corev1.Secret{
		Data: map[string][]byte{
			testResources.ProfileKey: badData,
		},
	})
	mocks.marshaller.EXPECT().UnmarshalProfile(badData).Return(nil, errors.New("failed to extract profile")).Once()
	verifyUpdateSingleProfile(t, s, mocks, networkConf.ClusterHost, nil, true)
}

func verifyCreateSingleProfile(t *testing.T, s *setup.Setup, mocks *setupMocks, host string, sharedSecret *certificate.SharedSecret, forSocks bool) {
	mustCallCreateOrLoadSharedSecret(t, s)
	setProfileExpectations(mocks, testResources.ProfileSecret, host, sharedSecret, forSocks, false)
	err := s.CreateProfile()
	assert.NoError(t, err, "Unexpected error from CreateProfile")
}

func verifyUpdateSingleProfile(t *testing.T, s *setup.Setup, mocks *setupMocks, host string, sharedSecret *certificate.SharedSecret, forSocks bool) {
	mustCallCreateOrLoadSharedSecret(t, s)
	setProfileExpectations(mocks, testResources.ProfileSecret, host, sharedSecret, forSocks, true)
	err := s.CreateProfile()
	assert.NoError(t, err, "Unexpected error from CreateProfile")
}

func setProfileExpectations(mocks *setupMocks, name, hostname string, sharedSecret *certificate.SharedSecret, forSocks, isUpdate bool) *certificate.Certificate {
	// If there is a shared secret, expect to generate a certificate from it
	var cert *certificate.Certificate
	if sharedSecret != nil {
		cert, _ = expectGenerateCertificate(mocks, sharedSecret, name)
	}

	// Configure profile creator to give us some bytes
	prof := &profile.Profile{Name: name}
	profBytes := []byte(name)
	mocks.profileCreator.EXPECT().CreateProfile(hostname, cert, forSocks).Return(prof).Once()
	mocks.marshaller.EXPECT().Marshal(prof).Return(profBytes, nil).Once()

	if isUpdate {
		// Expect client call to update profile
		mocks.client.EXPECT().UpdateSecret(name, map[string][]byte{
			testResources.ProfileKey: profBytes,
		}, false).Return(nil, nil).Once()
	} else {
		// Expect Kubernetes secret to be checked and created
		expectCheckAndCreateSecret(mocks.client, name, testResources.ProfileKey, profBytes, false)
	}
	return cert
}

func expectProfileExists(mocks *setupMocks, secretName string, prof *profile.Profile) {
	marshalledData := []byte("data_for_" + secretName)
	mocks.marshaller.EXPECT().UnmarshalProfile(marshalledData).Return(prof, nil)
	secret := &corev1.Secret{
		Data: map[string][]byte{
			testResources.ProfileKey: marshalledData,
		},
	}
	expectSecretExists(mocks.client, secretName, secret)
}

func expectOutdatedProfileExists(mocks *setupMocks, name, hostname string, sharedSecret *certificate.SharedSecret, forSocks bool) {
	// Expect Kubernetes secret to exist, but contain an outdated profile
	oldProf := &profile.Profile{Name: "this is old"}
	expectProfileExists(mocks, name, oldProf)
	mocks.profileCreator.EXPECT().IsConsistent(oldProf, hostname, sharedSecret, sharedSecret != nil, forSocks).Return(false).Once()
}

func expectUpToDateProfileExists(mocks *setupMocks, secretName string, prof *profile.Profile, hostname string, secret *certificate.SharedSecret, needCert, isSocks bool) {
	expectProfileExists(mocks, secretName, prof)
	mocks.profileCreator.EXPECT().IsConsistent(prof, hostname, secret, needCert, isSocks).Return(true)
}

func verifyCreateProfileLoadBalancerHost(t *testing.T, lbSpec *corev1.Service, expectedHost string, withDelay bool) {
	setupConf := setup.SetupConfig{
		LoadBalancerCheckPeriod: 0,
	}
	networkConf := config.NetworkConfig{
		UseParallelServerProxy: true,
	}

	s, mocks := newSetupWithMocks(t, setupConf, networkConf)

	// Configure client to return load balancer spec
	if withDelay {
		// Initially return a spec with no hostname
		mocks.client.EXPECT().GetService(testResources.LoadBalancer).Return(&corev1.Service{}, nil).Once()
	}
	mocks.client.EXPECT().GetService(testResources.LoadBalancer).Return(lbSpec, nil).Once()

	// Verify profile
	verifyCreateSingleProfile(t, s, mocks, expectedHost, nil, true)
}
