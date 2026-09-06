// TLS configuration for database drivers. Core defines the profile SSL modes.
package db

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/turanmahmudov/masume/internal/core"
)

// BuildPolicyTLS returns TLS settings for the policy, or nil for unset and disabled policies.
func BuildPolicyTLS(policy core.SSLPolicy, host string) *tls.Config {
	if policy == core.PolicyUnset || policy == core.PolicyOff {
		return nil
	}
	if !core.VerifiesCertificate(policy) {
		// Prefer and require encrypt without certificate verification.
		return &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	}
	if policy == core.PolicyVerifyFull {
		return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	}
	return BuildAuthorityOnlyTLS()
}

// BuildAuthorityOnlyTLS verifies certificate chains against system roots without hostname verification.
func BuildAuthorityOnlyTLS() *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			certificates := make([]*x509.Certificate, 0, len(rawCerts))
			for _, raw := range rawCerts {
				parsed, err := x509.ParseCertificate(raw)
				if err != nil {
					return err
				}
				certificates = append(certificates, parsed)
			}
			if len(certificates) == 0 {
				return NewDatabaseError("the server sent no certificate")
			}

			roots, err := x509.SystemCertPool()
			if err != nil {
				return err
			}
			intermediates := x509.NewCertPool()
			for _, intermediate := range certificates[1:] {
				intermediates.AddCert(intermediate)
			}
			_, err = certificates[0].Verify(x509.VerifyOptions{
				Roots: roots, Intermediates: intermediates,
			})
			return err
		},
	}
}
