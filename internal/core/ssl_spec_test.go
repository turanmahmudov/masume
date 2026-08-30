package core_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
)

func TestFindSSLModeReadsANameInAnyCase(t *testing.T) {
	for _, held := range []struct {
		written string
		want    core.SSLMode
	}{
		{"require", core.SSLRequire},
		{"REQUIRE", core.SSLRequire},
		{"  Verify-Full  ", core.SSLVerifyFull},
		{"disable", core.SSLDisable},
	} {
		mode, found := core.FindSSLMode(held.written)
		if !found || mode != held.want {
			t.Errorf("%q reads as %q, found=%v, wanted %q", held.written, mode, found, held.want)
		}
	}
}

func TestFindSSLModeRefusesANameItCannotRead(t *testing.T) {
	if _, found := core.FindSSLMode("required"); found {
		t.Error("a typo was read as a mode")
	}
	if _, found := core.FindSSLMode(""); found {
		t.Error("an empty name was read as a mode")
	}
}

func TestResolveSSLPolicyMapsEveryMode(t *testing.T) {
	for _, held := range []struct {
		mode   core.SSLMode
		want   core.SSLPolicy
		checks bool
	}{
		{core.SSLUnset, core.PolicyUnset, false},
		{core.SSLDisable, core.PolicyOff, false},
		{core.SSLAllow, core.PolicyPrefer, false},
		{core.SSLPrefer, core.PolicyPrefer, false},
		{core.SSLRequire, core.PolicyEncryptOnly, false},
		{core.SSLVerifyCa, core.PolicyVerifyCa, true},
		{core.SSLVerifyFull, core.PolicyVerifyFull, true},
		{core.SSLMode("unknown"), core.PolicyUnset, false},
	} {
		policy := core.ResolveSSLPolicy(held.mode)
		if policy != held.want {
			t.Errorf("%q resolves to %q, wanted %q", held.mode, policy, held.want)
		}
		if core.VerifiesCertificate(policy) != held.checks {
			t.Errorf("%q checks=%v, wanted %v", policy, core.VerifiesCertificate(policy), held.checks)
		}
	}
}

func TestSSLModeNamesListsEveryMode(t *testing.T) {
	named := core.SSLModeNames()
	for _, mode := range core.SSLModes {
		if !strings.Contains(named, string(mode)) {
			t.Errorf("%q is not in %q", mode, named)
		}
	}
}
