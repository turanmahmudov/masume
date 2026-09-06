package cfg_test

import (
	"os/exec"
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
		if got, err := cfg.BuildSecretCommand(source, held.reference); err != nil || got != held.want {
			t.Errorf("the reference %q built %q, wanted %q", held.reference, got, held.want)
		}
	}
}

func TestBuildSecretCommandPassesReferencesAsLiteralArguments(t *testing.T) {
	for _, reference := range []string{
		"", "a b", "it's", `$(printf injected)`, "`printf injected`",
		"'; printf injected; #", "\"; printf injected; #", "first\nsecond", `a\b`,
	} {
		t.Run(reference, func(t *testing.T) {
			command, err := cfg.BuildSecretCommand(cfg.SecretSource{
				Command: `printf '<%s>' {{ref}} {{ref}} | tr -d '\000'`,
			}, reference)
			if err != nil {
				t.Fatal(err)
			}
			output, err := exec.Command("sh", "-c", command).CombinedOutput()
			if err != nil {
				t.Fatalf("shell error: %v: %s", err, output)
			}
			if wanted := "<" + reference + "><" + reference + ">"; string(output) != wanted {
				t.Errorf("output %q, want %q", output, wanted)
			}
		})
	}
}

func TestParseSecretSourcesAcceptsQuotedFlags(t *testing.T) {
	for _, command := range []string{
		"op read {{ref}}",
		"vault kv get -field=password {{ref}}",
		`sops -d --extract '["db"]["password"]' {{ref}}`,
		`read-secret --label "work database" {{ref}} | head -1`,
		"read-secret\t{{ref}}\t{{ref}}",
	} {
		sources, problems := cfg.ParseSecretSources(cfg.Table{
			"secret": cfg.Table{"work": cfg.Table{"command": command}},
		})
		if len(problems) != 0 || len(sources) != 1 {
			t.Errorf("command %q: sources %v, problems %v", command, sources, problems)
		}
	}
}

func TestRejectSecretCommandsWithUnsafeReferenceContexts(t *testing.T) {
	for _, command := range []string{
		`op read '{{ref}}'`, `op read "{{ref}}"`,
		`op read 'prefix {{ref}} suffix'`, `op read "prefix {{ref}} suffix"`,
		`op read prefix{{ref}}`, `op read {{ref}}suffix`,
		`op read --path={{ref}}`, `op read {{ref}}"suffix"`,
		`op read \{{ref}}`, `op read {{ref}}{{ref}}`,
		`op read {{ref}} "{{ref}}"`, `op read {{ref}} 'unterminated`,
		`op read $(printf {{ref}})`, "op read `printf {{ref}}`",
		`op read "$(printf {{ref}})"`, `op read ${value:- {{ref}} }`,
		`op read $'escaped\' {{ref}} '`, `op read $" {{ref}} "`,
		"op read <<EOF\n{{ref}}\nEOF", "op read {{ref}}\x00",
		`op read {{ref}} # comment`, `op read {{ref}} || other`,
		`op read {{ref}} && other`, `op read {{ref}}; other`,
		`op read {{ref}} > output`, `op read {{ref}} |`,
		`{{ref}}`, `op read file | {{ref}}`,
	} {
		t.Run(command, func(t *testing.T) {
			sources, problems := cfg.ParseSecretSources(cfg.Table{
				"secret": cfg.Table{"work": cfg.Table{"command": command}},
			})
			if len(sources) != 0 || len(problems) != 1 ||
				!strings.Contains(problems[0].Reason, "standalone unquoted {{ref}}") {
				t.Errorf("sources %v, problems %v", sources, problems)
			}
			source := cfg.SecretSource{Name: "work", Command: command}
			if built, err := cfg.BuildSecretCommand(source, "$(printf injected)"); err == nil || built != "" {
				t.Errorf("unsafe command %q, error %v", built, err)
			}
			profile := cfg.Profile{Auth: cfg.AuthSecret, Secret: "work", SecretRef: "reference"}
			if _, err := cfg.ApplySecretCommand(profile, []cfg.SecretSource{source}); err == nil {
				t.Error("the profile accepted an unsafe store")
			}
		})
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
		{"quoted reference", "[secret.work]\ncommand = \"op read '{{ref}}'\"\n", "standalone unquoted {{ref}}"},
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

func TestLoadConfigSkipsProfilesWithUnsafeSecretStores(t *testing.T) {
	loaded := cfg.LoadConfig(writeConfig(t, `
[secret.unsafe]
command = "op read '{{ref}}'"
[secret.safe]
command = "op read {{ref}}"
[profile.unsafe]
engine = "postgres"
host = "localhost"
database = "shop"
user = "reader"
secret = "unsafe"
secret_ref = "reference"
[profile.safe]
engine = "postgres"
host = "localhost"
database = "shop"
user = "reader"
secret = "safe"
secret_ref = "reference"
`))
	if len(loaded.Problems) != 2 || loaded.Problems[0].Name != "secret.unsafe" ||
		loaded.Problems[1].Name != "unsafe" {
		t.Errorf("problems: %v", loaded.Problems)
	}
	if len(loaded.Secrets) != 1 || loaded.Secrets[0].Name != "safe" ||
		len(loaded.Profiles) != 1 || loaded.Profiles[0].Name != "safe" {
		t.Errorf("stores: %v, profiles: %v", loaded.Secrets, loaded.Profiles)
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
