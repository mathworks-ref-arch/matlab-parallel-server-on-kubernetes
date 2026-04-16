// Copyright 2025 The MathWorks, Inc.
package setup

import (
	"controller/internal/certificate"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Create the cluster profile
func (s *Setup) CreateProfile() error {
	if !s.hasCheckedSharedSecret {
		return errors.New("must run CreateOrLoadSharedSecret before running CreateProfile")
	}

	// Get MJS hostname
	clusterHost, err := s.getJobManagerHost(s.network.InternalClientsOnly)
	if err != nil {
		return err
	}

	// Create main profile (for the current MATLAB release) if needed
	if err := s.ensureProfileIsUpToDate(s.resources.ProfileSecret, clusterHost, s.network.UseParallelServerProxy); err != nil {
		return err
	}

	// Create a pre-26a profile for backwards compatible clusters if needed
	if s.network.UseParallelServerProxy && s.network.UsePoolProxy {
		return s.ensureProfileIsUpToDate(s.resources.ProfileSecretPre26a, clusterHost, false)
	}
	return nil
}

// Get the external address of MJS
func (s *Setup) getExternalAddress() (string, error) {
	addressFound := false
	retryPeriod := time.Duration(s.config.LoadBalancerCheckPeriod) * time.Second
	lbName := s.resources.LoadBalancer
	s.logger.Info("waiting for LoadBalancer service to have external hostname", zap.String("serviceName", lbName))
	address := ""
	for !addressFound {
		loadBalancer, err := s.client.GetService(lbName)
		if err != nil {
			return "", err
		}
		for _, ingress := range loadBalancer.Status.LoadBalancer.Ingress {
			if ingress.IP != "" {
				address = ingress.IP
				addressFound = true
			} else if ingress.Hostname != "" {
				address = ingress.Hostname
				addressFound = true
			}
		}
		time.Sleep(retryPeriod)
	}
	s.logger.Info("found LoadBalancer external hostname", zap.String("hostname", address))
	return address, nil
}

// If a cluster profile doesn't exist yet, create; otherwise, check that the existing profile is
// consistent with our configuration and update it if not.
func (s *Setup) ensureProfileIsUpToDate(name, clusterHost string, isSocks bool) error {
	existingSecret, exists, err := s.client.SecretExists(name)
	if err != nil {
		return fmt.Errorf("error checking for cluster profile secret: %v", err)
	}

	needCert := s.network.RequireClientCertificate || (isSocks && s.network.ParallelServerProxyUseMutualTLS)
	if needCert && s.sharedSecret == nil {
		return errors.New("shared secret must not be nil")
	}

	needNewProfile := !exists
	if exists {
		// Check whether we need to replace the existing profile
		profBytes, ok := existingSecret.Data[s.resources.ProfileKey]
		if !ok {
			s.logger.Warn("Existing profile does not contain expected key. Profile will be updated.", zap.String("key", s.resources.ProfileKey))
			needNewProfile = true // Existing secret has missing data
		} else {
			prof, err := s.marshaller.UnmarshalProfile(profBytes)
			if err != nil {
				s.logger.Warn("Error unmarshalling existing profile. Profile will be updated.", zap.Error(err))
				needNewProfile = true // Existing secret contains bad data
			} else {
				needNewProfile = !s.profileCreator.IsConsistent(prof, clusterHost, s.sharedSecret, needCert, isSocks)
			}
		}
	}
	if !needNewProfile {
		return nil
	}

	// Generate a certificate for this profile if needed
	var cert *certificate.Certificate
	if needCert {
		if s.sharedSecret == nil {
			return errors.New("shared secret must not be nil")
		}
		cert, err = s.certCreator.GenerateCertificate(s.sharedSecret)
		if err != nil {
			return fmt.Errorf("error generating certificate for cluster profile: %v", err)
		}
	}

	prof := s.profileCreator.CreateProfile(clusterHost, cert, isSocks)

	// Create or update K8s secret containing the profile's bytes
	profBytes, err := s.marshaller.Marshal(prof)
	if err != nil {
		return err
	}
	secretData := map[string][]byte{
		s.resources.ProfileKey: profBytes,
	}

	if exists {
		if _, err := s.client.UpdateSecret(name, secretData, s.config.PreserveSecrets); err != nil {
			return fmt.Errorf("error updating Kubernetes secret for MJS cluster profile: %v", err)
		}
		s.logger.Info("updated MJS cluster profile secret", zap.String("name", name))
	} else {
		if _, err := s.client.CreateSecret(name, secretData, s.config.PreserveSecrets); err != nil {
			return fmt.Errorf("error creating Kubernetes secret for MJS cluster profile: %v", err)
		}
		s.logger.Info("created MJS cluster profile secret", zap.String("name", name))
	}
	return nil
}
