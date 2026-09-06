package core

import "strings"

// SSLMode is a libpq-compatible SSL setting. An empty value uses the engine default.
type SSLMode string

// The modes libpq accepts, in its own order, from the least to the most secure.
const (
	SSLUnset      SSLMode = ""
	SSLDisable    SSLMode = "disable"
	SSLAllow      SSLMode = "allow"
	SSLPrefer     SSLMode = "prefer"
	SSLRequire    SSLMode = "require"
	SSLVerifyCa   SSLMode = "verify-ca"
	SSLVerifyFull SSLMode = "verify-full"
)

// SSLModes lists the modes a profile can use.
var SSLModes = []SSLMode{SSLDisable, SSLAllow, SSLPrefer, SSLRequire, SSLVerifyCa, SSLVerifyFull}

// FindSSLMode parses an SSL mode and returns false for an unknown name.
func FindSSLMode(written string) (SSLMode, bool) {
	return FindAllowed(SSLModes, strings.ToLower(strings.TrimSpace(written)))
}

// SSLPolicy is the requirement the mode puts on the connection.
type SSLPolicy string

// The policies a mode can resolve to. Only the two verifying policies check the
// certificate.
const (
	PolicyUnset       SSLPolicy = "unset"
	PolicyOff         SSLPolicy = "off"
	PolicyPrefer      SSLPolicy = "prefer"
	PolicyEncryptOnly SSLPolicy = "encrypt-only"
	PolicyVerifyCa    SSLPolicy = "verify-ca"
	PolicyVerifyFull  SSLPolicy = "verify-full"
)

// ResolveSSLPolicy returns the policy of the mode.
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

// SSLModeNames returns the mode names for use in an error message.
func SSLModeNames() string {
	names := make([]string, 0, len(SSLModes))
	for _, mode := range SSLModes {
		names = append(names, string(mode))
	}
	return strings.Join(names, ", ")
}
