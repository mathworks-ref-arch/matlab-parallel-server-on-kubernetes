// Copyright 2025 The MathWorks, Inc.
package setup

import (
	"errors"
	"fmt"

	"go.uber.org/zap"
)

// Create a secret for the parallel server proxy
func (s *Setup) CreateParallelServerProxySecretIfNeeded() error {
	needSecret := s.network.UseParallelServerProxy && s.network.ParallelServerProxyUseMutualTLS
	if !needSecret {
		return nil
	}

	if !s.hasCheckedSharedSecret {
		return errors.New("must run CreateOrLoadSharedSecret before running CreateParallelServerProxySecretIfNeeded")
	}

	proxySecret := s.resources.ParallelServerProxySecret
	_, exists, err := s.client.SecretExists(proxySecret)
	if err != nil {
		return err
	}
	if exists {
		s.logger.Info("using existing parallel server proxy secret", zap.String("name", proxySecret))
		return nil
	}

	s.logger.Info("generating parallel server proxy secret", zap.String("name", proxySecret))
	cert, err := s.certCreator.GenerateCertificate(s.sharedSecret)
	if err != nil {
		return fmt.Errorf("error generating certificate for parallel server proxy: %v", err)
	}

	secretBytes, err := s.marshaller.Marshal(cert)
	if err != nil {
		return fmt.Errorf("error marshalling certificate for parallel server proxy secret : %v", err)
	}

	secretData := map[string][]byte{
		s.resources.CertificateFile: secretBytes,
	}
	_, err = s.client.CreateSecret(proxySecret, secretData, s.config.PreserveSecrets)
	if err != nil {
		return fmt.Errorf("error creating Kubernetes secret for parallel server proxy secret: %v", err)
	}
	s.logger.Info("created certificate secret for parallel server proxy", zap.String("name", proxySecret))
	return nil
}
