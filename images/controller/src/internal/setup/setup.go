// Package setup provides code for setting up the Kubernetes cluster for running MATLAB Job Scheduler,
// including creating the required secrets and cluster profile.
// Copyright 2024-2025 The MathWorks, Ins.
package setup

import (
	"controller/internal/certificate"
	"controller/internal/config"
	"controller/internal/k8s"
	"controller/internal/logging"
	"fmt"

	"github.com/mathworks/mjssetup/pkg/profile"
)

type Setup struct {
	client                 k8s.Client
	logger                 *logging.Logger
	config                 SetupConfig
	network                config.NetworkConfig
	resources              config.ResourceNames
	certCreator            certificate.CertCreator
	profileCreator         ProfileCreator
	marshaller             Marshaller
	hasCheckedSharedSecret bool
	sharedSecret           *certificate.SharedSecret
}

type SetupConfig struct {
	JobManagerName          string
	JobManagerWaitPeriod    int
	LoadBalancerCheckPeriod int
	PreserveSecrets         bool
}

type ProfileCreator interface {
	CreateProfile(hostname string, cert *certificate.Certificate, forSocks bool) *profile.Profile
	IsConsistent(profile *profile.Profile, hostname string, secret *certificate.SharedSecret, needCert, forSocks bool) bool
}

type Marshaller interface {
	Marshal(any) ([]byte, error)
	UnmarshalProfile([]byte) (*profile.Profile, error)
}

func New(conf SetupConfig, networkConf config.NetworkConfig, resourceNames config.ResourceNames, client k8s.Client, certCreator certificate.CertCreator, profileCreator ProfileCreator, marshaller Marshaller, logger *logging.Logger) *Setup {
	return &Setup{
		config:         conf,
		network:        networkConf,
		resources:      resourceNames,
		client:         client,
		certCreator:    certCreator,
		profileCreator: profileCreator,
		marshaller:     marshaller,
		logger:         logger,
	}
}

// Get the job manager hostname
func (s *Setup) getJobManagerHost(internalOnly bool) (string, error) {
	if internalOnly {
		// If all clients are inside the Kubernetes cluster, use the job manager service's full DNS name
		// Use the job manager hostname if all clients are inside the Kubernetes cluster
		return fmt.Sprintf("%s.%s.svc.%s", s.resources.JobManagerService, s.network.Namespace, s.network.ClusterDomain), nil
	}

	// Use a custom hostname if one was set
	if s.network.ClusterHost != "" {
		return s.network.ClusterHost, nil
	}

	// Otherwise, get the hostname from the load balancer
	return s.getExternalAddress()
}
