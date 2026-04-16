// Copyright 2025 The MathWorks, Ins.
package setup

import (
	"controller/internal/certificate"
	"fmt"

	"go.uber.org/zap"
)

// Create or load shared secret and certificate for MJS and return the shared secret
func (s *Setup) CreateOrLoadSharedSecret() error {
	required := s.network.UseSecureCommunication || s.network.RequireClientCertificate || s.network.RequireScriptVerification || s.network.ParallelServerProxyUseMutualTLS
	if !required {
		s.hasCheckedSharedSecret = true
		return nil
	}

	secret, alreadyExists, err := s.getExistingSharedSecret()
	if err != nil {
		return fmt.Errorf("error checking for shared secret: %v", err)
	}
	if alreadyExists {
		s.hasCheckedSharedSecret = true
		s.sharedSecret = secret
		return nil
	}

	// Generate the shared secret
	s.logger.Info("generating shared secret", zap.String("name", s.resources.SharedSecret))
	secret, err = s.certCreator.CreateSharedSecret()
	if err != nil {
		return err
	}
	secretBytes, err := s.marshaller.Marshal(secret)
	if err != nil {
		return fmt.Errorf("error marshalling shared secret: %v", err)
	}
	secretData := map[string][]byte{
		s.resources.SharedSecretFile: secretBytes,
	}

	// Generate a certificate if needed
	if s.network.RequireClientCertificate {
		cert, err := s.certCreator.GenerateCertificate(secret)
		if err != nil {
			return err
		}
		certBytes, err := s.marshaller.Marshal(cert)
		if err != nil {
			return fmt.Errorf("error marshalling certificate: %v", err)
		}
		secretData[s.resources.CertificateFile] = certBytes
	}

	// Create the Kubernetes secret
	_, err = s.client.CreateSecret(s.resources.SharedSecret, secretData, s.config.PreserveSecrets)
	if err != nil {
		return fmt.Errorf("error creating Kubernetes secret for MJS shared secret: %v", err)
	}

	s.hasCheckedSharedSecret = true
	s.sharedSecret = secret
	s.logger.Info("generated shared secret successfully")
	return nil
}

// Extract a shared secret from a Kubernetes secret if one already exists
func (s *Setup) getExistingSharedSecret() (*certificate.SharedSecret, bool, error) {
	k8sSecret, alreadyExists, err := s.client.SecretExists(s.resources.SharedSecret)
	if err != nil {
		return nil, false, fmt.Errorf("error checking for shared secret: %v", err)
	}
	if !alreadyExists {
		return nil, false, nil
	}

	s.logger.Info("found existing shared secret", zap.String("name", k8sSecret.Name))
	secretData, hasSecret := k8sSecret.Data[s.resources.SharedSecretFile]
	if !hasSecret {
		return nil, false, fmt.Errorf("secret file '%s' not found in Kubernetes Secret '%s'", s.resources.SharedSecretFile, k8sSecret.Name)
	}
	secret, err := s.certCreator.LoadSharedSecret(secretData)
	if err != nil {
		return nil, false, fmt.Errorf("error extracting shared secret from Kubernetes Secret '%s': %v", k8sSecret.Name, err)
	}
	return secret, true, nil
}
