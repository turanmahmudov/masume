package cfg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// buildStoredProfile returns a profile the form could have written.
func buildStoredProfile() cfg.Profile {
	return cfg.Profile{
		Name: "shop", Engine: core.EnginePostgres, Host: "127.0.0.1", Port: 5432,
		Database: "shop", User: "you", Auth: cfg.AuthPassword,
		Environment: cfg.EnvironmentDev, AccessMode: cfg.AccessWrite,
	}
}

// saveProfile writes the profile into a file with that text, and returns the new text.
func saveProfile(t *testing.T, body string, profile cfg.Profile) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}
	if err := cfg.SaveProfileToFile(profile, "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the config file was not read back: %v", err)
	}
	return string(written)
}

// A password the user cleared must be deleted from the file. A line left behind keeps the
// connection on the old password and stores it on disk.
func TestSaveProfileToFileTakesOutAValueTheFormCleared(t *testing.T) {
	written := saveProfile(t, `
[profile.shop]
engine = "postgres"
host = "127.0.0.1"
port = 5432
database = "shop"
user = "you"
password = "old-secret"
`, buildStoredProfile())

	if strings.Contains(written, "old-secret") {
		t.Errorf("the old password is still in the file:\n%s", written)
	}
	if strings.Contains(written, "password =") {
		t.Errorf("the password line is still in the file:\n%s", written)
	}
}

// A setting the form does not show keeps its line, so an edit of a connection never deletes
// the page size or a comment from the file.
func TestSaveProfileToFileKeepsWhatTheFormNeverShowed(t *testing.T) {
	written := saveProfile(t, `
[profile.shop]
engine = "postgres"
host = "127.0.0.1"
port = 5432
database = "shop"
user = "you"
page_size = 500                      # as many rows as this screen draws
statement_timeout_ms = 30000
`, buildStoredProfile())

	for _, wanted := range []string{"page_size = 500", "statement_timeout_ms = 30000",
		"as many rows as this screen draws"} {
		if !strings.Contains(written, wanted) {
			t.Errorf("%q left the file:\n%s", wanted, written)
		}
	}
}

// A rename keeps the settings the form does not show, in the same way as an edit. A rename
// that deleted the block and wrote a new one would lose `mcp`, and the profile would use the
// access level of the whole server.
func TestSaveProfileToFileKeepsWhatTheFormNeverShowedThroughARename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`
[profile.shop]
engine = "postgres"
host = "127.0.0.1"
port = 5432
database = "shop"
user = "you"
# an agent may only read this one
mcp = "read-only"
page_size = 500
`), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	renamed := buildStoredProfile()
	renamed.Name = "shop-prod"
	if err := cfg.SaveProfileToFile(renamed, "shop", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}
	held, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the config file was not read back: %v", err)
	}
	written := string(held)

	if !strings.Contains(written, "[profile.shop-prod]") {
		t.Errorf("the profile was not renamed:\n%s", written)
	}
	if strings.Contains(written, "[profile.shop]") {
		t.Errorf("the old name is still in the file:\n%s", written)
	}
	for _, wanted := range []string{`mcp = "read-only"`, "page_size = 500",
		"an agent may only read this one"} {
		if !strings.Contains(written, wanted) {
			t.Errorf("%q left the file on a rename:\n%s", wanted, written)
		}
	}

	// The file still contains one profile with the new name.
	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) > 0 {
		t.Fatalf("the file does not read back: %+v", loaded.Problems)
	}
	if len(loaded.Profiles) != 1 || loaded.Profiles[0].Name != "shop-prod" {
		t.Fatalf("the file holds %d profiles: %+v", len(loaded.Profiles), loaded.Profiles)
	}
	if loaded.Profiles[0].McpAccess != cfg.McpReadOnly {
		t.Errorf("the mcp level reads %q after the rename", loaded.Profiles[0].McpAccess)
	}
}

// A rename to a name that is already in the file writes into the block that stays, so the
// file never contains two blocks with one name.
func TestSaveProfileToFileRenamingOntoANameTheFileHolds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`
[profile.shop]
engine = "postgres"
host = "127.0.0.1"
port = 5432
database = "shop"
user = "you"

[profile.shop-prod]
engine = "postgres"
host = "old.example.com"
port = 5432
database = "shop"
user = "you"
`), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	renamed := buildStoredProfile()
	renamed.Name = "shop-prod"
	renamed.Host = "new.example.com"
	if err := cfg.SaveProfileToFile(renamed, "shop", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) > 0 {
		t.Fatalf("the file does not read back: %+v", loaded.Problems)
	}
	if len(loaded.Profiles) != 1 || loaded.Profiles[0].Name != "shop-prod" {
		t.Fatalf("the file holds %d profiles: %+v", len(loaded.Profiles), loaded.Profiles)
	}
	if loaded.Profiles[0].Host != "new.example.com" {
		t.Errorf("the host reads %q, wanted the one just written", loaded.Profiles[0].Host)
	}
}

// The file is written to a temporary file in the same directory and then moved over the
// target, so a write that fails part way leaves the old file complete. No temporary file can
// stay behind, and only the owner can read the file.
func TestSaveProfileToFileLeavesNoHalfWrittenFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.toml")
	if err := os.WriteFile(path, []byte("[profile.other]\nengine = \"sqlite\"\n"), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}
	if err := cfg.SaveProfileToFile(buildStoredProfile(), "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	left, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("cannot read the directory: %v", err)
	}
	if len(left) != 1 || left[0].Name() != "config.toml" {
		names := []string{}
		for _, entry := range left {
			names = append(names, entry.Name())
		}
		t.Errorf("the directory holds %v, wanted the config file alone", names)
	}

	found, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("cannot read the config file back: %v", statErr)
	}
	if held := found.Mode().Perm(); held != 0o600 {
		t.Errorf("the config file is written %o, wanted 600", held)
	}
	written, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("cannot read the config file back: %v", readErr)
	}
	for _, wanted := range []string{"[profile.other]", "[profile.shop]"} {
		if !strings.Contains(string(written), wanted) {
			t.Errorf("%q left the file:\n%s", wanted, written)
		}
	}
}
