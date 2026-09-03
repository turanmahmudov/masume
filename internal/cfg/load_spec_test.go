package cfg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// writeConfig writes a config file where LoadConfig reads it, and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}
	return path
}

// findProfile returns the profile with that name from a load.
func findProfile(t *testing.T, loaded cfg.LoadedConfig, name string) cfg.Profile {
	t.Helper()
	for _, profile := range loaded.Profiles {
		if profile.Name == name {
			return profile
		}
	}
	t.Fatalf("the load holds no profile named %q; it holds %d and %d were skipped",
		name, len(loaded.Profiles), len(loaded.Problems))
	return cfg.Profile{}
}

func TestLoadConfigReadsAProfile(t *testing.T) {
	path := writeConfig(t, `
[profile.staging]
engine = "postgres"
host = "db.example.com"
port = 6543
database = "shop"
user = "reader"
env = "test"
mode = "read-only"
sslmode = "verify-full"
page_size = 250
`)

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 0 {
		t.Fatalf("the load reported %v", loaded.Problems)
	}

	profile := findProfile(t, loaded, "staging")
	for _, held := range []struct {
		field string
		got   any
		want  any
	}{
		{"engine", profile.Engine, core.EnginePostgres},
		{"host", profile.Host, "db.example.com"},
		{"port", profile.Port, 6543},
		{"database", profile.Database, "shop"},
		{"user", profile.User, "reader"},
		{"environment", profile.Environment, cfg.EnvironmentTest},
		{"access", profile.AccessMode, cfg.AccessReadOnly},
		{"ssl mode", profile.SSLMode, core.SSLVerifyFull},
		{"page size", profile.PageSize, 250},
	} {
		if held.got != held.want {
			t.Errorf("the %s reads %v, wanted %v", held.field, held.got, held.want)
		}
	}
}

// The statement timeout is set in milliseconds. A profile without one leaves the limit to
// the server.
func TestLoadConfigReadsTheStatementTimeout(t *testing.T) {
	path := writeConfig(t, `
[profile.limited]
engine = "postgres"
host = "127.0.0.1"
database = "shop"
user = "reader"
statement_timeout_ms = 2500

[profile.open]
engine = "postgres"
host = "127.0.0.1"
database = "shop"
user = "reader"

[profile.bad]
engine = "postgres"
host = "127.0.0.1"
database = "shop"
user = "reader"
statement_timeout_ms = -1
`)

	loaded := cfg.LoadConfig(path)
	if held := findProfile(t, loaded, "limited").StatementTimeout; held != 2500*time.Millisecond {
		t.Errorf("the limit reads %v, wanted 2.5s", held)
	}
	if held := findProfile(t, loaded, "open").StatementTimeout; held != 0 {
		t.Errorf("the limit reads %v, wanted none", held)
	}
	if len(loaded.Problems) != 1 || loaded.Problems[0].Name != "bad" {
		t.Errorf("a timeout below zero was read, and the load reported %v", loaded.Problems)
	}
}

// A missing port uses the default port of the engine, so a profile only has to set the
// values that are different.
func TestLoadConfigGivesAnEngineItsDefaultPort(t *testing.T) {
	path := writeConfig(t, `
[profile.local]
engine = "postgres"
host = "localhost"
database = "shop"
user = "reader"
`)

	profile := findProfile(t, cfg.LoadConfig(path), "local")
	if profile.Port != core.ResolveEngineInfo(core.EnginePostgres).DefaultPort {
		t.Errorf("the port reads %d, wanted the default of the engine", profile.Port)
	}
}

// One bad profile in the file must not stop the other profiles.
func TestLoadConfigKeepsTheProfilesAroundABadOne(t *testing.T) {
	path := writeConfig(t, `
[profile.good]
engine = "sqlite"
database = "/tmp/shop.db"

[profile.broken]
engine = "not-an-engine"
database = "shop"
`)

	loaded := cfg.LoadConfig(path)
	findProfile(t, loaded, "good")
	if len(loaded.Problems) != 1 {
		t.Fatalf("the load reported %d problems, wanted the one bad profile", len(loaded.Problems))
	}
	if loaded.Problems[0].Name != "broken" {
		t.Errorf("the problem names %q, wanted the bad profile", loaded.Problems[0].Name)
	}
}

// A missing file is the first run of the client, and every default applies.
func TestLoadConfigAnswersDefaultsWhereThereIsNoFile(t *testing.T) {
	loaded := cfg.LoadConfig(filepath.Join(t.TempDir(), "none.toml"))
	if len(loaded.Profiles) != 0 {
		t.Errorf("a file that is not there gave %d profiles", len(loaded.Profiles))
	}
	if len(loaded.Problems) != 1 || !strings.Contains(loaded.Problems[0].Reason, "not found") {
		t.Errorf("the reason reads %v, wanted that the file is not there", loaded.Problems)
	}
	if loaded.Settings.Theme == "" && loaded.Keys.Preset == "" {
		t.Error("the defaults of the interface were not filled in")
	}
}

// A file that exists but cannot be read is not a first run. Every profile is lost, and the
// message must say so and not report a missing file.
func TestLoadConfigTellsAnUnreadableFileFromAMissingOne(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a file whatever its mode says")
	}
	path := writeConfig(t, "[profile.shop]\nengine = \"sqlite\"\ndatabase = \"/tmp/x.db\"\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("cannot take the permissions off: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 1 {
		t.Fatalf("the load reported %d problems, wanted one", len(loaded.Problems))
	}
	reason := loaded.Problems[0].Reason
	if strings.Contains(reason, "not found") {
		t.Errorf("the reason reads %q, which says the file is missing when it is unreadable", reason)
	}
	if !strings.Contains(reason, "cannot be read") {
		t.Errorf("the reason reads %q, wanted it to say the file cannot be read", reason)
	}
}

func TestLoadConfigReportsTomlItCannotRead(t *testing.T) {
	path := writeConfig(t, "[profile.shop\nengine =\n")
	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 1 || !strings.Contains(loaded.Problems[0].Reason, "TOML") {
		t.Errorf("the reason reads %v, wanted it to name the TOML", loaded.Problems)
	}
}
