// The TLS a driver is handed. The modes themselves are named in core, because a
// profile carries one before any driver is chosen.
package db

import (
	"crypto/tls"
	"crypto/x509"

	"github.com/turanmahmudov/masume/internal/core"
)

// BuildPolicyTLS returns how a connection of this policy is encrypted, and nothing where
// the profile names no mode, which connects in the clear.
func BuildPolicyTLS(policy core.SSLPolicy, host string) *tls.Config {
	if policy == core.PolicyUnset || policy == core.PolicyOff {
		return nil
	}
	if !core.VerifiesCertificate(policy) {
		// `prefer` and `require` encrypt and check nothing.
		return &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	}
	if policy == core.PolicyVerifyFull {
		return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	}
	return BuildAuthorityOnlyTLS()
}

// BuildAuthorityOnlyTLS checks the chain against the roots of the machine and leaves
// the name alone, because a server behind a tunnel returns on a name no certificate
// holds. The standard verification is turned off and run again here without the name.
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
