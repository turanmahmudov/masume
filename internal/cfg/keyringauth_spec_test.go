package cfg_test

import (
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/secret"
)

// useMockKeyring points the keyring at one in memory, so a case neither reads nor writes the
// keyring of the person running the tests.
func useMockKeyring(t *testing.T) {
	t.Helper()
	held := secret.IsAvailable
	keyring.MockInit()
	secret.IsAvailable = func() bool { return true }
	t.Cleanup(func() { secret.IsAvailable = held })
}

// buildKeyringProfile returns a profile that reads its password from the keyring.
func buildKeyringProfile(name string) cfg.Profile {
	return cfg.Profile{
		Name: name, Engine: core.EnginePostgres, Host: "db.internal", Port: 5432,
		Database: name, User: "reader", Auth: cfg.AuthKeyring,
	}
}

func TestAKeyringProfileReadsTheStoredPassword(t *testing.T) {
	useMockKeyring(t)
	profile := buildKeyringProfile("shop")
	if err := secret.SavePassword(profile.Name, "hunter2"); err != nil {
		t.Fatalf("the password was not stored: %v", err)
	}

	password, err := cfg.ResolveProfilePassword(profile)
	if err != nil || password != "hunter2" {
		t.Fatalf("the profile resolved to %q, err=%v", password, err)
	}
	// The keyring answered, so the client draws no field.
	if cfg.NeedsPasswordPrompt(profile) {
		t.Error("a profile whose password the keyring holds still asks the user")
	}
}

// The first connection of such a profile is typed, because the keyring holds nothing yet.
func TestAKeyringProfileAsksTheUserOnTheFirstConnection(t *testing.T) {
	useMockKeyring(t)
	profile := buildKeyringProfile("fresh")

	password, err := cfg.ResolveProfilePassword(profile)
	if err != nil {
		t.Fatalf("the profile reported %v", err)
	}
	if password != "" {
		t.Errorf("the profile resolved to %q, wanted nothing", password)
	}
	if !cfg.NeedsPasswordPrompt(profile) {
		t.Error("a profile the keyring does not hold does not ask the user")
	}
}

// A machine with no keyring leaves the user as the source, rather than refusing the profile.
func TestAKeyringProfileAsksTheUserWithoutAKeyring(t *testing.T) {
	held := secret.IsAvailable
	secret.IsAvailable = func() bool { return false }
	t.Cleanup(func() { secret.IsAvailable = held })

	if !cfg.NeedsPasswordPrompt(buildKeyringProfile("shop")) {
		t.Error("a profile on a machine with no keyring does not ask the user")
	}
}

func TestLoadConfigReadsAKeyringProfile(t *testing.T) {
	loaded := cfg.LoadConfig(writeConfig(t, `
[profile.shop]
engine   = "postgres"
host     = "db.internal"
database = "shop"
user     = "reader"
auth     = "keyring"
`))

	if len(loaded.Problems) != 0 {
		t.Fatalf("the load reported %v", loaded.Problems)
	}
	if findProfile(t, loaded, "shop").Auth != cfg.AuthKeyring {
		t.Error("the profile does not read the keyring")
	}
}

// The store answers on its own, so a profile that names one never asks the user. This runs a
// real command, because that is the whole of the mechanism.
func TestASecretProfileRunsTheCommandOfItsStore(t *testing.T) {
	loaded := cfg.LoadConfig(writeConfig(t, `
[secret.work]
command = "printf %s hunter2-{{ref}}"

[profile.shop]
engine     = "postgres"
host       = "db.internal"
database   = "shop"
user       = "reader"
secret     = "work"
secret_ref = "shop"
`))

	password, err := cfg.ResolveProfilePassword(findProfile(t, loaded, "shop"))
	if err != nil || password != "hunter2-shop" {
		t.Fatalf("the store answered %q, err=%v", password, err)
	}
}

// A store whose command fails reports the reason, so a locked store does not read as a wrong
// password.
func TestASecretProfileReportsAStoreThatFails(t *testing.T) {
	loaded := cfg.LoadConfig(writeConfig(t, `
[secret.work]
command = "echo the store is locked >&2; exit 1 # {{ref}}"

[profile.shop]
engine     = "postgres"
host       = "db.internal"
database   = "shop"
user       = "reader"
secret     = "work"
secret_ref = "shop"
`))

	_, err := cfg.ResolveProfilePassword(findProfile(t, loaded, "shop"))
	if err == nil {
		t.Fatal("a store that failed answered with no error")
	}
	if !strings.Contains(err.Error(), "the store is locked") {
		t.Errorf("the error reads %q", err.Error())
	}
	// The report names the store, so the reader knows which tool to look at.
	if !strings.Contains(err.Error(), `secret store "work"`) {
		t.Errorf("the error does not name the store: %q", err.Error())
	}
}
