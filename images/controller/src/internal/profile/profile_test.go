// Copyright 2025-2026 The MathWorks, Inc.
package profile_test

import (
	"controller/internal/logging"
	"controller/internal/profile"
	"testing"

	"github.com/mathworks/mjssetup/pkg/certificate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	mjssetup "github.com/mathworks/mjssetup/pkg/profile"
)

const (
	basePort                = 4000
	parallelServerProxyPort = 5000
	jobManagerName          = "my-jm"
	jobManagerService       = "jm-host-in-k8s"
)

func TestCreateProfileNoCert(t *testing.T) {
	var cert *certificate.Certificate = nil
	forSocks := false
	createAndVerifyProfile(t, getBasicProfileConfig(), "myhost", cert, forSocks, "myhost:4000")
}

func TestCreateProfileSocksNoCert(t *testing.T) {
	var cert *certificate.Certificate = nil
	forSocks := true
	createAndVerifyProfile(t, getBasicProfileConfig(), "myhost", cert, forSocks, "jm-host-in-k8s:4000?proxy=socks5://myhost:5000")
}

func TestCreateProfileWithCert(t *testing.T) {
	cert := getDummyCert()
	forSocks := false
	createAndVerifyProfile(t, getBasicProfileConfig(), "myhost", cert, forSocks, "myhost:4000")
}

func TestCreateProfileSocksSecureWithCert(t *testing.T) {
	conf := getBasicProfileConfig()
	conf.ParallelServerProxyUseMutualTLS = true
	cert := getDummyCert()
	forSocks := true
	createAndVerifyProfile(t, conf, "secure-host", cert, forSocks, "jm-host-in-k8s:4000?proxy=socks5s://secure-host:5000")
}

func TestIsConsistentPositive(t *testing.T) {
	// Create a profile
	conf := getBasicProfileConfig()
	hostName := "clusterhost"
	forSocks := false
	prof := createProfile(t, conf, hostName, nil, forSocks)

	// Check we get IsConsistent=true when we reuse the same settings
	verifyProfileConsistent(t, prof, conf, hostName, nil, false, forSocks)
}

func TestIsConsistentPositiveWithCert(t *testing.T) {
	// Create a profile
	conf := getBasicProfileConfig()
	hostName := "clusterhost"
	forSocks := false
	secret := getDummySecret()
	cert := getDummyCertForSecret(secret)
	prof := createProfile(t, conf, hostName, cert, forSocks)

	// Check we get IsConsistent=true when we reuse the same settings,
	// including the original shared secret
	needsCert := true
	verifyProfileConsistent(t, prof, conf, hostName, secret, needsCert, forSocks)
}

func TestIsConsistentNewJobManagerName(t *testing.T) {
	// Create a profile
	conf := getBasicProfileConfig()
	hostName := "clusterhost"
	forSocks := false
	prof := createProfile(t, conf, hostName, nil, forSocks)

	// Change the job manager name and check we get IsConsistent=false
	conf.JobManagerName = "newName"
	verifyProfileNotConsistent(t, prof, conf, hostName, nil, false, forSocks)
}

func TestNotConsistentNewHostname(t *testing.T) {
	// Create a profile
	conf := getBasicProfileConfig()
	initHostName := "orig-host"
	forSocks := false
	prof := createProfile(t, conf, initHostName, nil, forSocks)

	// Change the hostname and check we get IsConsistent=false
	verifyProfileNotConsistent(t, prof, conf, "new-host", nil, false, forSocks)
}

func TestNotConsistentSocks(t *testing.T) {
	// Create a profile
	conf := getBasicProfileConfig()
	hostName := "myhost"
	forSocks := false // Don't need socks originally
	prof := createProfile(t, conf, hostName, nil, forSocks)

	// Now set forSocks=true and check we get IsConsistent=false
	forSocks = true
	verifyProfileNotConsistent(t, prof, conf, hostName, nil, false, forSocks)
}

func TestNotConsistentMissingCert(t *testing.T) {
	// Create a profile with no certificate
	conf := getBasicProfileConfig()
	hostName := "myhost"
	var cert *certificate.Certificate = nil
	forSocks := false
	prof := createProfile(t, conf, hostName, cert, forSocks)

	// Expect IsConsistent=false if we now need a certificate
	secret := getDummySecret()
	needsCert := true
	verifyProfileNotConsistent(t, prof, conf, hostName, secret, needsCert, forSocks)
}

func TestNotConsistentShouldNotContainCert(t *testing.T) {
	// Create a profile containing a certificate
	conf := getBasicProfileConfig()
	hostName := "myhost"
	forSocks := false
	prof := createProfile(t, conf, hostName, getDummyCert(), forSocks)

	// Expect IsConsistent=false if we no longer need a certificate
	needCert := false
	verifyProfileNotConsistent(t, prof, conf, hostName, nil, needCert, forSocks)
}

func TestNotConsistentOutdatedCert(t *testing.T) {
	// Create a profile containing a certificate
	conf := getBasicProfileConfig()
	hostName := "myhost"
	cert := getDummyCert()
	forSocks := false
	prof := createProfile(t, conf, hostName, cert, forSocks)

	// Expect IsConsistent=false if the certificate does not match
	// the current shared secret
	secret := getDummySecret()
	require.NotEqual(t, secret.CertPEM, cert.ServerCert, "Certificate should not match secret")
	needCert := true
	verifyProfileNotConsistent(t, prof, conf, hostName, secret, needCert, forSocks)
}

func getBasicProfileConfig() profile.ProfileConfig {
	return profile.ProfileConfig{
		JobManagerName:                  jobManagerName,
		BasePort:                        basePort,
		ParallelServerProxyUseMutualTLS: false,
		ParallelServerProxyPort:         parallelServerProxyPort,
		JobManagerService:               jobManagerService,
	}
}

func getDummyCert() *certificate.Certificate {
	return &certificate.Certificate{
		ServerCert: "server",
		ClientCert: "client",
		ClientKey:  "key",
	}
}

func getDummySecret() *certificate.SharedSecret {
	return &certificate.SharedSecret{
		CertPEM: "abc123",
		KeyPEM:  "def456",
	}
}

func getDummyCertForSecret(secret *certificate.SharedSecret) *certificate.Certificate {
	return &certificate.Certificate{
		ServerCert: secret.CertPEM, // Certificate was signed by the shared secret
		ClientCert: "client",
		ClientKey:  "key",
	}
}

func createProfile(t *testing.T, conf profile.ProfileConfig, hostName string, cert *certificate.Certificate, forSocks bool) *mjssetup.Profile {
	profMaker := profile.New(conf, logging.NewFromZapLogger(zaptest.NewLogger(t)))
	return profMaker.CreateProfile(hostName, cert, forSocks)
}

func createAndVerifyProfile(t *testing.T, conf profile.ProfileConfig, hostName string, cert *certificate.Certificate, forSocks bool, expectedHostname string) {
	prof := createProfile(t, conf, hostName, cert, forSocks)
	assert.Equal(t, expectedHostname, prof.SchedulerComponent.Host, "Incorrect hostname in profile")
	assert.Equal(t, conf.JobManagerName, prof.SchedulerComponent.Name, "Incorrect job manager name in profile")
	if cert == nil {
		assert.Empty(t, prof.SchedulerComponent.Certificate, "Certificate should be empty when not provided")
	} else {
		assert.Equal(t, cert.ServerCert, prof.SchedulerComponent.Certificate, "Certificate should match")
		assert.Equal(t, cert.ClientCert, prof.SchedulerComponent.ClientCertificate, "Client certificate should match")
		assert.Equal(t, cert.ClientKey, prof.SchedulerComponent.ClientPrivateKey, "Client private key should match")
	}
}

func isProfileConsistent(t *testing.T, prof *mjssetup.Profile, conf profile.ProfileConfig, hostname string, secret *certificate.SharedSecret, needCert, forSocks bool) bool {
	// Create a new profile maker with the supplied config
	profMaker := profile.New(conf, logging.NewFromZapLogger(zaptest.NewLogger(t)))

	// Check consistency with the supplied profile
	return profMaker.IsConsistent(prof, hostname, secret, needCert, forSocks)
}

func verifyProfileConsistent(t *testing.T, prof *mjssetup.Profile, conf profile.ProfileConfig, hostname string, secret *certificate.SharedSecret, needCert, forSocks bool) {
	isConsistent := isProfileConsistent(t, prof, conf, hostname, secret, needCert, forSocks)
	assert.True(t, isConsistent, "Profile should be consistent")
}

func verifyProfileNotConsistent(t *testing.T, prof *mjssetup.Profile, conf profile.ProfileConfig, hostname string, secret *certificate.SharedSecret, needCert, forSocks bool) {
	isConsistent := isProfileConsistent(t, prof, conf, hostname, secret, needCert, forSocks)
	assert.False(t, isConsistent, "Profile should not be consistent")
}
