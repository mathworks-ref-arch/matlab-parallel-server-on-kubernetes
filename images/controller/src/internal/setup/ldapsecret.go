// Copyright 2025 The MathWorks, Inc.
package setup

import (
	"fmt"

	"go.uber.org/zap"
)

// Check that the LDAP certificate secret exists, if needed
func (s *Setup) CheckLDAPSecret() error {
	if !s.network.RequireLdapCert {
		// No need for LDAP certificate
		return nil
	}
	secretName := s.resources.LdapSecret
	s.logger.Info("checking LDAP secret", zap.String("name", secretName))
	certFile := s.resources.LdapCertFile
	secret, exists, err := s.client.SecretExists(secretName)
	if err != nil {
		return err
	}
	createSecretInstruction := fmt.Sprintf(`To start an MJS using a secure LDAP server to authenticate user credentials, create an LDAP certificate secret with command "kubectl create secret generic %s --from-file=%s=<path> --namespace %s", replacing "<path>" with the path to the SSL certificate for your LDAP server.`, secretName, certFile, s.network.Namespace)
	if !exists {
		return fmt.Errorf(`error: LDAP certificate secret "%s" does not exist in namespace "%s". %s`, secretName, s.network.Namespace, createSecretInstruction)
	}

	// Check that the secret contains the expected filename
	if _, ok := secret.Data[certFile]; !ok {
		return fmt.Errorf(`error: LDAP certificate secret "%s" does not contain the file "%s". %s`, secretName, certFile, createSecretInstruction)
	}
	s.logger.Info("ldap secret is correctly configured")
	return nil
}
