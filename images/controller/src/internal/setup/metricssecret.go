// Copyright 2025 The MathWorks, Inc.
package setup

import (
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
)

// Create a secret containing certificates for a secure metrics server
func (s *Setup) CreateCertsForMetricsIfNeeded() error {
	if !s.network.UseSecureMetrics {
		return nil
	}

	secret, alreadyExists, err := s.client.SecretExists(s.resources.MetricsSecret)
	if err != nil {
		return fmt.Errorf("error checking for metrics secret: %v", err)
	}
	if alreadyExists {
		s.logger.Info("using existing metrics secret")
		return s.checkMetricsSecret(secret)
	}

	// Create a CA cert
	s.logger.Info("generating metrics secret", zap.String("name", s.resources.MetricsSecret))
	caCert, err := s.certCreator.CreateSharedSecret()
	if err != nil {
		return err
	}

	// Create secret containing certificates for the metrics server
	jmHost, err := s.getJobManagerHost(!s.network.OpenMetricsPortOutsideKubernetes)
	s.logger.Info("generating metrics server certificates", zap.String("host", jmHost))
	if err != nil {
		return err
	}
	serverCert, err := s.certCreator.GenerateCertificateWithHostname(caCert, jmHost)
	if err != nil {
		return err
	}
	serverSecretData := map[string][]byte{
		s.resources.MetricsCaCertFile: []byte(serverCert.ServerCert),
		s.resources.MetricsCertFile:   []byte(serverCert.ClientCert),
		s.resources.MetricsKeyFile:    []byte(serverCert.ClientKey),
	}
	serverSecret := s.resources.MetricsSecret
	_, err = s.client.CreateSecret(serverSecret, serverSecretData, s.config.PreserveSecrets)
	if err != nil {
		return fmt.Errorf("error creating Kubernetes secret for metrics certificates: %v", err)
	}
	s.logger.Info("created metrics secret for job manager", zap.String("name", serverSecret))

	// Create another secret containing certificates for the client to use
	clientCert, err := s.certCreator.GenerateCertificate(caCert)
	if err != nil {
		return err
	}
	clientSecretData := map[string][]byte{
		s.resources.MetricsCaCertFile:     []byte(clientCert.ServerCert),
		s.resources.ClientMetricsCertFile: []byte(clientCert.ClientCert),
		s.resources.ClientMetricsKeyFile:  []byte(clientCert.ClientKey),
	}
	clientSecret := s.resources.ClientMetricsSecret
	_, err = s.client.CreateSecret(clientSecret, clientSecretData, s.config.PreserveSecrets)
	if err != nil {
		return fmt.Errorf("error creating Kubernetes secret for client certificates for metrics server: %v", err)
	}
	s.logger.Info("created metrics secret for client", zap.String("name", clientSecret))
	return nil
}

// Check the contents of an existing metrics certificate secret
func (s *Setup) checkMetricsSecret(secret *corev1.Secret) error {
	secretName := s.resources.MetricsSecret
	caFile := s.resources.MetricsCaCertFile
	certFile := s.resources.MetricsCertFile
	keyFile := s.resources.MetricsKeyFile

	createSecretInstruction := fmt.Sprintf(`To start an MJS with a server exporting encrypted metrics, create a secret with command "kubectl create secret generic %s --from-file=%s=<ca-cert> --from-file=%s=<server-cert> --from-file=%s=<server-key> --namespace %s", replacing "<ca-cert>" with the path to the CA certificate used to sign your client certificate, <server-cert> with the path to the SSL certificate to use for the server, and <server-key> with the path to the private key file to use for the server.`, secretName, caFile, certFile, keyFile, s.network.Namespace)

	// Check that the secret contains the expected filenames
	toCheck := []string{caFile, certFile, keyFile}
	for _, key := range toCheck {
		if _, ok := secret.Data[key]; !ok {
			return fmt.Errorf(`error: metrics certificate secret "%s" does not contain the file "%s". %s`, secretName, key, createSecretInstruction)
		}
	}
	return nil
}
