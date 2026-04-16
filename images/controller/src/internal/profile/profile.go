// Implementation of profile creation using mjssetup.
// Copyright 2025 The MathWorks, Inc.
package profile

import (
	"controller/internal/logging"
	"controller/internal/setup"
	"fmt"

	"github.com/mathworks/mjssetup/pkg/certificate"
	"github.com/mathworks/mjssetup/pkg/profile"
	"go.uber.org/zap"
)

type ProfileCreator struct {
	metadata map[string]string
	config   ProfileConfig
	logger   *logging.Logger
}

type ProfileConfig struct {
	JobManagerName                  string
	JobManagerService               string
	BasePort                        int
	ParallelServerProxyPort         int
	ParallelServerProxyUseMutualTLS bool
}

func New(conf ProfileConfig, logger *logging.Logger) setup.ProfileCreator {
	return &ProfileCreator{
		config: conf,
		metadata: map[string]string{
			"SubType": "kubernetes",
		},
		logger: logger,
	}
}

func (p *ProfileCreator) CreateProfile(hostname string, cert *certificate.Certificate, forSocks bool) *profile.Profile {
	profileHost := p.getHostnameForProfile(hostname, forSocks)
	p.logger.Info("Creating cluster profile", zap.String("hostname", profileHost))
	return profile.CreateProfileWithMetadata(p.config.JobManagerName, profileHost, cert, p.metadata)
}

// Returns true if the given inputs are consistent with an existing profile
func (p *ProfileCreator) IsConsistent(prof *profile.Profile, hostname string, secret *certificate.SharedSecret, needCert, forSocks bool) bool {
	profileHost := p.getHostnameForProfile(hostname, forSocks)
	if profileHost != prof.SchedulerComponent.Host {
		p.logger.Info("existing profile does not match current cluster hostname", zap.String("existingProfileHostname", prof.SchedulerComponent.Host), zap.String("currentHostname", profileHost))
		return false
	}
	if p.config.JobManagerName != prof.Name {
		p.logger.Info("existing profile does not match current job manager name", zap.String("existingProfileName", prof.Name), zap.String("currentName", p.config.JobManagerName))
		return false
	}
	if needCert && prof.SchedulerComponent.Certificate == "" {
		p.logger.Info("profile requires certificate, but existing profile does not contain a certificate")
		return false
	}
	if !needCert && prof.SchedulerComponent.Certificate != "" {
		p.logger.Info("profile does not require certificate, but existing profile contains a certificate")
		return false
	}
	if needCert && prof.SchedulerComponent.Certificate != secret.CertPEM {
		p.logger.Info("certificate in profile does not match current shared secret")
		return false
	}
	return true
}

func (p *ProfileCreator) getHostnameForProfile(hostname string, forSocks bool) string {
	if !forSocks {
		return fmt.Sprintf("%s:%d", hostname, p.config.BasePort)
	}
	protocol := "socks5"
	if p.config.ParallelServerProxyUseMutualTLS {
		protocol += "s"
	}
	return fmt.Sprintf("%s:%d?proxy=%s://%s:%d", p.config.JobManagerService, p.config.BasePort, protocol, hostname, p.config.ParallelServerProxyPort)
}
