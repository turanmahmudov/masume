package mysql

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// The mode of a profile decides what the connection asks of the server. A profile that
// names no mode must still encrypt where the server offers it, because a connection in
// the clear carries the password and every row over the network.
func TestResolveMysqlTLSFollowsTheModeOfTheProfile(t *testing.T) {
	for _, held := range []struct {
		mode core.SSLMode
		want string
	}{
		{core.SSLUnset, "preferred"},
		{core.SSLPrefer, "preferred"},
		{core.SSLAllow, "preferred"},
		{core.SSLDisable, "false"},
		{core.SSLVerifyFull, "true"},
	} {
		answered, err := resolveMysqlTLS(cfg.Profile{SSLMode: held.mode})
		if err != nil {
			t.Fatalf("%q was refused: %v", held.mode, err)
		}
		if answered != held.want {
			t.Errorf("%q asks for %q, wanted %q", held.mode, answered, held.want)
		}
	}
}

// `require` encrypts and never falls back to the clear, so it takes settings of its own
// and not the name the driver reads as a fallback.
func TestResolveMysqlTLSNeverFallsBackWhereTheProfileRequiresTLS(t *testing.T) {
	for _, mode := range []core.SSLMode{core.SSLRequire, core.SSLVerifyCa} {
		answered, err := resolveMysqlTLS(cfg.Profile{SSLMode: mode})
		if err != nil {
			t.Fatalf("%q was refused: %v", mode, err)
		}
		for _, refused := range []string{"preferred", "false"} {
			if answered == refused {
				t.Errorf("%q asks for %q, which may connect in the clear", mode, answered)
			}
		}
	}
}
