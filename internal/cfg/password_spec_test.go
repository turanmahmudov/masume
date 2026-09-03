package cfg_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// buildServerProfile returns a profile on a server, which is the case that needs a
// password.
func buildServerProfile(auth cfg.AuthMode) cfg.Profile {
	return cfg.Profile{
		Name: "shop", Engine: core.EnginePostgres, Host: "localhost", Port: 5432,
		Database: "shop", User: "reader", Auth: auth,
	}
}

// A stored password is read only in the mode that stores one. A profile that runs a command
// or asks the user must not fall back to a password left in the file, or an old password
// would be sent to the server and the user would not know which one was used.
func TestFindStoredPasswordReadsOnlyThePasswordMode(t *testing.T) {
	for _, held := range []struct {
		name string
		auth cfg.AuthMode
		want string
	}{
		{"the password mode reads it", cfg.AuthPassword, "secret"},
		{"the command mode reads none", cfg.AuthCommand, ""},
		{"the prompt mode reads none", cfg.AuthPrompt, ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			profile := buildServerProfile(held.auth)
			profile.Password = "secret"
			if answered := cfg.FindStoredPassword(profile); answered != held.want {
				t.Errorf("the password reads %q, wanted %q", answered, held.want)
			}
		})
	}
}

// A password can name an environment variable instead of holding the secret, so a config
// file can be shared without the secret.
func TestFindStoredPasswordReadsAnEnvironmentVariable(t *testing.T) {
	const variable = "MASUME_SPEC_PASSWORD"
	t.Setenv(variable, "from-the-environment")

	profile := buildServerProfile(cfg.AuthPassword)
	profile.PasswordEnv = variable
	if answered := cfg.FindStoredPassword(profile); answered != "from-the-environment" {
		t.Errorf("the password reads %q, wanted the value of the variable", answered)
	}

	// A variable that is not set gives an empty password and not its own name.
	profile.PasswordEnv = "MASUME_SPEC_PASSWORD_NOT_SET"
	if answered := cfg.FindStoredPassword(profile); answered != "" {
		t.Errorf("a variable that is not set read %q", answered)
	}
}

// The password in the file has priority over the variable, because it is more specific.
func TestFindStoredPasswordPrefersTheValueInTheFile(t *testing.T) {
	const variable = "MASUME_SPEC_PASSWORD"
	t.Setenv(variable, "from-the-environment")

	profile := buildServerProfile(cfg.AuthPassword)
	profile.Password = "from-the-file"
	profile.PasswordEnv = variable
	if answered := cfg.FindStoredPassword(profile); answered != "from-the-file" {
		t.Errorf("the password reads %q, wanted the one in the file", answered)
	}
}

// A command that prints the password is run, and its output is the password.
func TestResolveProfilePasswordRunsTheCommand(t *testing.T) {
	profile := buildServerProfile(cfg.AuthCommand)
	profile.PasswordCommand = "printf secret-from-command"

	answered, err := cfg.ResolveProfilePassword(profile)
	if err != nil {
		t.Fatalf("the command answered %v", err)
	}
	if answered != "secret-from-command" {
		t.Errorf("the password reads %q, wanted what the command printed", answered)
	}
}

// A command that fails must return an error and not an empty password. The server would
// refuse an empty password with a message about the password and not about the command.
func TestResolveProfilePasswordReportsACommandThatFails(t *testing.T) {
	profile := buildServerProfile(cfg.AuthCommand)
	profile.PasswordCommand = "exit 3"

	if _, err := cfg.ResolveProfilePassword(profile); err == nil {
		t.Error("a command that failed answered no error")
	}
}

// A file needs no password, whatever the profile sets, so the client never asks for one.
func TestNeedsPasswordPromptIsFalseForAFile(t *testing.T) {
	profile := cfg.Profile{
		Name: "notes", Engine: core.EngineSqlite, Database: "/tmp/notes.db",
		Auth: cfg.AuthPrompt,
	}
	if cfg.NeedsPasswordPrompt(profile) {
		t.Error("a file was asked for a password")
	}
}

func TestNeedsPasswordPromptFollowsTheMode(t *testing.T) {
	for _, held := range []struct {
		name     string
		auth     cfg.AuthMode
		password string
		want     bool
	}{
		{"the prompt mode always asks", cfg.AuthPrompt, "", true},
		{"a stored password asks nothing", cfg.AuthPassword, "secret", false},
		{"the command mode asks nothing", cfg.AuthCommand, "", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			profile := buildServerProfile(held.auth)
			profile.Password = held.password
			if held.auth == cfg.AuthCommand {
				profile.PasswordCommand = "printf secret"
			}
			if answered := cfg.NeedsPasswordPrompt(profile); answered != held.want {
				t.Errorf("the prompt reads %v, wanted %v", answered, held.want)
			}
		})
	}
}

// MongoDB authenticates as a named user. A profile with a user asks for a password like any
// other server. A profile without a user does not: a server with authentication off refuses
// a connection that sends a user, so there is nothing to check the password against.
func TestNeedsPasswordPromptFollowsTheUserWhereTheEngineLeavesItOpen(t *testing.T) {
	for _, held := range []struct {
		name string
		user string
		want bool
	}{
		{"a named user is asked for its password", "reader", true},
		{"a profile that names no user is not asked", "", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			profile := cfg.Profile{
				Name: "orders", Engine: core.EngineMongo, Host: "localhost", Port: 27017,
				Database: "shop", User: held.user, Auth: cfg.AuthPassword,
			}
			if answered := cfg.NeedsPasswordPrompt(profile); answered != held.want {
				t.Errorf("the prompt reads %v, wanted %v", answered, held.want)
			}
		})
	}
}
