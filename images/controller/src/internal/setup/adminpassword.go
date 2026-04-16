// Copyright 2025 The MathWorks, Inc.
package setup

import (
	"fmt"

	"go.uber.org/zap"
)

// Check that the administrator password exists if needed
func (s *Setup) CheckAdminPassword() error {
	if s.network.SecurityLevel < 2 {
		// No admin password required for security level 1 or below
		return nil
	}
	name := s.resources.AdminPasswordSecret
	key := s.resources.AdminPasswordKey
	s.logger.Info("checking admin password secret", zap.String("name", name))

	secret, adminPasswordExists, err := s.client.SecretExists(name)
	if err != nil {
		return err
	}
	createSecretInstruction := fmt.Sprintf(`To start an MJS cluster at security level %d, create an administrator password secret with command "kubectl create secret generic %s --from-literal=%s=<password> --namespace %s", replacing "<password>" with a password of your choice.`, s.network.SecurityLevel, name, key, s.network.Namespace)
	if !adminPasswordExists {
		return fmt.Errorf(`error: Administrator password secret "%s" does not exist in namespace "%s". %s`, name, s.network.Namespace, createSecretInstruction)
	}

	// Check that the secret contains the password key
	if _, ok := secret.Data[key]; !ok {
		return fmt.Errorf(`error: Administrator password secret "%s" does not contain the key "%s". %s`, name, key, createSecretInstruction)
	}
	s.logger.Info("admin password secret is correctly configured")
	return nil
}
