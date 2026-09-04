package cfg_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

func TestLoadConfigReadsASecretStore(t *testing.T) {
	path := writeConfig(t, `
[secret.work]
command = "op read {{ref}}"

[profile.shop]
engine     = "postgres"
host       = "db.internal"
database   = "shop"
user       = "reader"
auth       = "secret"
secret     = "work"
secret_ref = "op://eng/shop/password"
`)

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 0 {
		t.Fatalf("the load reported %v", loaded.Problems)
	}
	if len(loaded.Secrets) != 1 || loaded.Secrets[0].Name != "work" {
		t.Fatalf("the load holds the stores %v", loaded.Secrets)
	}

	profile := findProfile(t, loaded, "shop")
	if profile.Auth != cfg.AuthSecret {
		t.Errorf("the auth mode reads %q", profile.Auth)
	}
	// The reference is one argument of the command, so a blank or a quote in it cannot
	// become a second argument or a second command.
	if profile.SecretCommand != `op read 'op://eng/shop/password'` {
		t.Errorf("the command reads %q", profile.SecretCommand)
	}
	// The store answers without the user, so the client draws no field.
	if cfg.NeedsPasswordPrompt(profile) {
		t.Error("a profile that names a store still asks the user")
	}
}

// A profile that names a store and no mode reads the store, because naming one says what it
// is for.
func TestLoadConfigTakesTheSecretModeFromTheStoreName(t *testing.T) {
	path := writeConfig(t, `
[secret.work]
command = "op read {{ref}}"

[profile.shop]
engine     = "postgres"
host       = "db.internal"
database   = "shop"
user       = "reader"
secret     = "work"
secret_ref = "op://eng/shop/password"
`)

	if findProfile(t, cfg.LoadConfig(path), "shop").Auth != cfg.AuthSecret {
		t.Error("the profile does not read its store")
	}
}

// A reference is data, not part of the command. One that carries shell characters is passed
// as a single argument, so it cannot run anything of its own.
func TestBuildSecretCommandQuotesTheReference(t *testing.T) {
	source := cfg.SecretSource{Name: "work", Command: "read {{ref}} | head -1"}
	for _, held := range []struct {
		reference string
		want      string
	}{
		{"op://eng/shop/password", `read 'op://eng/shop/password' | head -1`},
		{"a b", `read 'a b' | head -1`},
		{"; rm -rf /", `read '; rm -rf /' | head -1`},
		{"it's", `read 'it'\''s' | head -1`},
	} {
		if got := cfg.BuildSecretCommand(source, held.reference); got != held.want {
			t.Errorf("the reference %q built %q, wanted %q", held.reference, got, held.want)
		}
	}
}

func TestLoadConfigReportsABrokenSecretStore(t *testing.T) {
	for _, held := range []struct {
		name    string
		written string
		says    string
	}{
		{"no command", "[secret.work]\ndescription = \"nothing\"\n", "command"},
		{"no reference", "[secret.work]\ncommand = \"op read\"\n", "{{ref}}"},
	} {
		t.Run(held.name, func(t *testing.T) {
			loaded := cfg.LoadConfig(writeConfig(t, held.written))
			if len(loaded.Problems) != 1 {
				t.Fatalf("the load reported %v, wanted one problem", loaded.Problems)
			}
			problem := loaded.Problems[0]
			if problem.Name != "secret.work" {
				t.Errorf("the problem names %q, wanted the store", problem.Name)
			}
			if !strings.Contains(problem.Reason, held.says) {
				t.Errorf("the reason reads %q", problem.Reason)
			}
		})
	}
}

// A profile that names a store the config file does not declare is skipped with that reason,
// rather than opening with no password at all.
func TestLoadConfigReportsAProfileThatNamesNoStore(t *testing.T) {
	loaded := cfg.LoadConfig(writeConfig(t, `
[profile.shop]
engine     = "postgres"
host       = "db.internal"
database   = "shop"
user       = "reader"
auth       = "secret"
secret     = "missing"
secret_ref = "op://eng/shop/password"
`))

	if len(loaded.Profiles) != 0 {
		t.Fatalf("the load provided the profile with no store to read")
	}
	if len(loaded.Problems) != 1 ||
		!strings.Contains(loaded.Problems[0].Reason, "[secret.missing]") {
		t.Fatalf("the load reported %v", loaded.Problems)
	}
}

func TestLoadConfigReportsASecretProfileWithoutAReference(t *testing.T) {
	loaded := cfg.LoadConfig(writeConfig(t, `
[secret.work]
command = "op read {{ref}}"

[profile.shop]
engine   = "postgres"
host     = "db.internal"
database = "shop"
user     = "reader"
auth     = "secret"
secret   = "work"
`))

	if len(loaded.Problems) != 1 ||
		!strings.Contains(loaded.Problems[0].Reason, "secret_ref") {
		t.Fatalf("the load reported %v", loaded.Problems)
	}
}

// The form carries the name of the store and the reference. The command is built from the
// stores of the config file, so a profile the form made reads its password like one the file
// carried.
func TestApplySecretCommandBuildsTheCommandOfAFormProfile(t *testing.T) {
	sources := []cfg.SecretSource{{Name: "work", Command: "op read {{ref}}"}}
	profile := cfg.Profile{
		Name: "shop", Auth: cfg.AuthSecret,
		Secret: "work", SecretRef: "op://eng/shop/password",
	}

	built, err := cfg.ApplySecretCommand(profile, sources)
	if err != nil {
		t.Fatalf("the profile was refused: %v", err)
	}
	if built.SecretCommand != `op read 'op://eng/shop/password'` {
		t.Errorf("the command reads %q", built.SecretCommand)
	}

	// A mode that reads no store carries no command, so a profile that was a secret one
	// and is not any more cannot run the old command.
	profile.Auth = cfg.AuthPrompt
	if plain, _ := cfg.ApplySecretCommand(profile, sources); plain.SecretCommand != "" {
		t.Errorf("a profile that reads no store carries %q", plain.SecretCommand)
	}
}
