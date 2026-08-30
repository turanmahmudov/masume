package core

import "strings"

// SSLMode is an sslmode as libpq reads it. An empty value means the profile named
// none, and each engine has its own default.
type SSLMode string

// The modes libpq reads, in the order it lists them, from least to most safe.
const (
	SSLUnset      SSLMode = ""
	SSLDisable    SSLMode = "disable"
	SSLAllow      SSLMode = "allow"
	SSLPrefer     SSLMode = "prefer"
	SSLRequire    SSLMode = "require"
	SSLVerifyCa   SSLMode = "verify-ca"
	SSLVerifyFull SSLMode = "verify-full"
)

// SSLModes lists the modes a profile may name.
var SSLModes = []SSLMode{SSLDisable, SSLAllow, SSLPrefer, SSLRequire, SSLVerifyCa, SSLVerifyFull}

// FindSSLMode reads this text as a mode. A name nobody can read is refused rather
// than read as a weaker mode, because a profile that asks to be encrypted must not
// connect in the clear on a typo.
func FindSSLMode(written string) (SSLMode, bool) {
	return FindAllowed(SSLModes, strings.ToLower(strings.TrimSpace(written)))
}

// SSLPolicy is what the mode asks of the connection.
type SSLPolicy string

// The policies a mode resolves to. Only the two verifying ones check the certificate.
const (
	PolicyUnset       SSLPolicy = "unset"
	PolicyOff         SSLPolicy = "off"
	PolicyPrefer      SSLPolicy = "prefer"
	PolicyEncryptOnly SSLPolicy = "encrypt-only"
	PolicyVerifyCa    SSLPolicy = "verify-ca"
	PolicyVerifyFull  SSLPolicy = "verify-full"
)

// ResolveSSLPolicy returns what the mode asks of the connection.
func ResolveSSLPolicy(mode SSLMode) SSLPolicy {
	switch mode {
	case SSLUnset:
		return PolicyUnset
	case SSLDisable:
		return PolicyOff
	case SSLAllow, SSLPrefer:
		return PolicyPrefer
	case SSLRequire:
		return PolicyEncryptOnly
	case SSLVerifyCa:
		return PolicyVerifyCa
	case SSLVerifyFull:
		return PolicyVerifyFull
	}
	return PolicyUnset
}

// VerifiesCertificate is true for a policy that checks the certificate.
func VerifiesCertificate(policy SSLPolicy) bool {
	return policy == PolicyVerifyCa || policy == PolicyVerifyFull
}

// SSLModeNames writes the modes as a message lists them.
func SSLModeNames() string {
	names := make([]string, 0, len(SSLModes))
	for _, mode := range SSLModes {
		names = append(names, string(mode))
	}
	return strings.Join(names, ", ")
}
