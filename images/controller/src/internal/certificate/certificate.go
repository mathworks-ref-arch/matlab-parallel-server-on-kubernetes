// Interface for creation of MJS shared secrets and certificates.
// Copyright 2025 The MathWorks, Inc.
package certificate

import "github.com/mathworks/mjssetup/pkg/certificate"

// Wrapper around the types provided by mjssetup
type SharedSecret = certificate.SharedSecret
type Certificate = certificate.Certificate

// Interface for generating shared secrets and certificates
type CertCreator interface {
	CreateSharedSecret() (*SharedSecret, error)
	GenerateCertificate(*SharedSecret) (*Certificate, error)
	GenerateCertificateWithHostname(*SharedSecret, string) (*Certificate, error)
	LoadSharedSecret([]byte) (*SharedSecret, error)
}
