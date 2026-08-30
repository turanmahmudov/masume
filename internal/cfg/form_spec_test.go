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

// buildFormProfile answers a profile the connection form can be built from.
func buildFormProfile() cfg.Profile {
	return cfg.Profile{
		Name: "shop", Engine: core.EnginePostgres, Host: "db.example.com", Port: 6543,
		Database: "shop", User: "reader", Auth: cfg.AuthPassword,
		Environment: cfg.EnvironmentTest, AccessMode: cfg.AccessReadOnly,
		SSLMode: core.SSLVerifyFull, PageSize: 250,
	}
}

// The form is what the reader edits, so it has to carry every value the profile holds and
// read back into the same profile.
func TestBuildProfileFromFieldsRoundTripsAProfile(t *testing.T) {
	source := buildFormProfile()
	fields := cfg.BuildFormFields(source, true)
	if len(fields) == 0 {
		t.Fatal("the form holds no field")
	}

	held, err := cfg.BuildProfileFromFields(fields, source, true)
	if err != nil {
		t.Fatalf("the form does not read back: %v", err)
	}

	for _, one := range []struct {
		field string
		got   any
		want  any
	}{
		{"name", held.Name, source.Name},
		{"engine", held.Engine, source.Engine},
		{"host", held.Host, source.Host},
		{"port", held.Port, source.Port},
		{"database", held.Database, source.Database},
		{"user", held.User, source.User},
		{"environment", held.Environment, source.Environment},
		{"access", held.AccessMode, source.AccessMode},
		{"ssl mode", held.SSLMode, source.SSLMode},
		{"page size", held.PageSize, source.PageSize},
	} {
		if one.got != one.want {
			t.Errorf("the %s reads %v, wanted %v", one.field, one.got, one.want)
		}
	}
}

// A field the reader emptied has to be reported rather than saved as a profile that cannot
// open, because the reader would only find out on the next connect.
func TestBuildProfileFromFieldsReportsAFieldThatIsNeeded(t *testing.T) {
	source := buildFormProfile()
	fields := cfg.ApplyFieldChange(cfg.BuildFormFields(source, true), "name", "")

	if _, err := cfg.BuildProfileFromFields(fields, source, true); err == nil {
		t.Error("a profile with no name was built")
	}
}

// A port that is not a number cannot be dialled, so the form says so rather than saving it.
func TestBuildProfileFromFieldsReportsAPortThatIsNotANumber(t *testing.T) {
	source := buildFormProfile()
	for _, written := range []string{"nope", "-1", "0"} {
		fields := cfg.ApplyFieldChange(cfg.BuildFormFields(source, true), "port", written)
		if _, err := cfg.BuildProfileFromFields(fields, source, true); err == nil {
			t.Errorf("a port of %q was accepted", written)
		}
	}
}

// A field change touches the one field and leaves the rest, so editing the host does not
// clear the user.
func TestApplyFieldChangeTouchesOneFieldOnly(t *testing.T) {
	fields := cfg.BuildFormFields(buildFormProfile(), true)
	before := cfg.ReadField(fields, "user")

	held := cfg.ApplyFieldChange(fields, "host", "other.example.com")
	if cfg.ReadField(held, "host") != "other.example.com" {
		t.Errorf("the host reads %q", cfg.ReadField(held, "host"))
	}
	if cfg.ReadField(held, "user") != before {
		t.Errorf("the user changed to %q", cfg.ReadField(held, "user"))
	}
}

// A URL pasted into the form fills the fields it names, which is the quickest way to open a
// server somebody sent a URL for.
func TestApplyConnectionUrlFillsTheFieldsItNames(t *testing.T) {
	held, is := cfg.ParseConnectionURL("postgres://reader@db.example.com:6543/shop")
	if !is {
		t.Fatal("the URL was not read")
	}

	fields := cfg.ApplyConnectionURL(cfg.BuildFormFields(cfg.Profile{}, false), held)
	for _, one := range []struct{ key, want string }{
		{"host", "db.example.com"},
		{"port", "6543"},
		{"database", "shop"},
		{"user", "reader"},
	} {
		if answered := cfg.ReadField(fields, one.key); answered != one.want {
			t.Errorf("the %s reads %q, wanted %q", one.key, answered, one.want)
		}
	}
}

// A `rediss://` URL asks for TLS by its scheme alone, and a Redis client reads it that way.
// Read as a plain `redis://`, it would open in the clear on a connection the user asked to
// have encrypted.
func TestParseConnectionUrlKeepsTheTlsOfTheScheme(t *testing.T) {
	held, is := cfg.ParseConnectionURL("rediss://cache.example.com:6380/0")
	if !is {
		t.Fatal("the URL was not read")
	}
	if held.SSLMode != string(core.SSLVerifyFull) {
		t.Errorf("the mode reads %q, wanted %q", held.SSLMode, core.SSLVerifyFull)
	}

	// A mode written into the URL still decides.
	named, is := cfg.ParseConnectionURL("rediss://cache.example.com:6380/0?sslmode=require")
	if !is {
		t.Fatal("the URL with a mode was not read")
	}
	if named.SSLMode != string(core.SSLRequire) {
		t.Errorf("the mode reads %q, wanted %q", named.SSLMode, core.SSLRequire)
	}

	// A plain `redis://` asks for nothing, as a Redis client reads it.
	plain, is := cfg.ParseConnectionURL("redis://cache.example.com:6379/0")
	if !is {
		t.Fatal("the plain URL was not read")
	}
	if plain.SSLMode != "" {
		t.Errorf("the mode reads %q, wanted none", plain.SSLMode)
	}
}

// A field the form hides for one engine must not be asked for: a file has no host and no port,
// and asking would leave the reader filling in what nothing reads.
func TestFindShownFieldsLeavesOutWhatAnEngineDoesNotNeed(t *testing.T) {
	file := cfg.BuildFormFields(cfg.Profile{
		Name: "notes", Engine: core.EngineSqlite, Database: "/tmp/notes.db",
	}, true)
	shown := map[string]bool{}
	for _, field := range cfg.FindShownFields(file) {
		shown[field.Key] = true
	}
	if shown["host"] || shown["port"] {
		t.Error("a file is asked for a host or a port")
	}
	if !shown["database"] {
		t.Error("a file is not asked for its path")
	}

	// A server is asked for both.
	server := cfg.BuildFormFields(buildFormProfile(), true)
	shownServer := map[string]bool{}
	for _, field := range cfg.FindShownFields(server) {
		shownServer[field.Key] = true
	}
	if !shownServer["host"] || !shownServer["port"] {
		t.Error("a server is not asked for a host or a port")
	}
}

// A profile is written into the config file and read back as the same profile, or the form
// would lose what the reader typed the moment the client restarts.
func TestSaveProfileToFileWritesAProfileThatReadsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	source := buildFormProfile()

	if err := cfg.SaveProfileToFile(source, "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) != 0 {
		t.Fatalf("the file that was written reports %v", loaded.Problems)
	}
	held := findProfile(t, loaded, source.Name)

	for _, one := range []struct {
		field string
		got   any
		want  any
	}{
		{"engine", held.Engine, source.Engine},
		{"host", held.Host, source.Host},
		{"port", held.Port, source.Port},
		{"database", held.Database, source.Database},
		{"user", held.User, source.User},
		{"environment", held.Environment, source.Environment},
		{"access", held.AccessMode, source.AccessMode},
		{"ssl mode", held.SSLMode, source.SSLMode},
	} {
		if one.got != one.want {
			t.Errorf("the %s read back as %v, wanted %v", one.field, one.got, one.want)
		}
	}

	// The writer writes the keys the connection form edits. A setting the form never shows,
	// such as the page size, is not written for a profile that never had a file, and takes
	// its default. Where such a setting is already in the file it is kept, which is what
	// TestSaveProfileToFileKeepsTheSettingsTheFormNeverShows covers.
	if held.PageSize != cfg.DefaultPageSize {
		t.Errorf("the page size read back as %d, wanted the default", held.PageSize)
	}
}

// A second profile joins the file rather than taking the place of the first.
func TestSaveProfileToFileKeepsTheProfilesAlreadyThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	first := buildFormProfile()
	if err := cfg.SaveProfileToFile(first, "", path); err != nil {
		t.Fatalf("the first profile was not written: %v", err)
	}
	second := buildFormProfile()
	second.Name = "warehouse"
	if err := cfg.SaveProfileToFile(second, "", path); err != nil {
		t.Fatalf("the second profile was not written: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	findProfile(t, loaded, "shop")
	findProfile(t, loaded, "warehouse")
}

// A profile saved under a new name replaces the one it was renamed from, so the file never
// holds both.
func TestSaveProfileToFileReplacesTheProfileItRenames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	source := buildFormProfile()
	if err := cfg.SaveProfileToFile(source, "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	renamed := source
	renamed.Name = "shop_renamed"
	if err := cfg.SaveProfileToFile(renamed, source.Name, path); err != nil {
		t.Fatalf("the renamed profile was not written: %v", err)
	}

	loaded := cfg.LoadConfig(path)
	findProfile(t, loaded, "shop_renamed")
	for _, held := range loaded.Profiles {
		if held.Name == "shop" {
			t.Error("the profile it was renamed from was kept")
		}
	}
}

// The settings of the reader that are not profiles stay as they were, because saving one
// connection must not rewrite the theme.
func TestSaveProfileToFileKeepsTheRestOfTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[ui]\ntheme = \"nord\"\n"), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	if err := cfg.SaveProfileToFile(buildFormProfile(), "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the config file back: %v", err)
	}
	if !strings.Contains(string(written), "nord") {
		t.Errorf("the theme was lost:\n%s", written)
	}
	if cfg.LoadConfig(path).Settings.Theme != "nord" {
		t.Error("the theme does not read back")
	}
}

// The connection form shows some of what a profile holds, not all of it. Saving from the form
// must keep the settings it never showed, or editing a host by hand in the client would erase
// the page size, the keepalive and the pre-connect command written in the file.
func TestSaveProfileToFileKeepsTheSettingsTheFormNeverShows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`[profile.shop]
engine = "postgres"
host = "db.example.com"
port = 6543
database = "shop"
user = "reader"
page_size = 250
keepalive_s = 30
autocommit = true
`), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	// The reader edits the host in the form and saves. Everything else stays as it was.
	loaded := cfg.LoadConfig(path)
	held := findProfile(t, loaded, "shop")
	held.Host = "other.example.com"

	if err := cfg.SaveProfileToFile(held, "", path); err != nil {
		t.Fatalf("the profile was not written: %v", err)
	}

	again := findProfile(t, cfg.LoadConfig(path), "shop")
	if again.Host != "other.example.com" {
		t.Errorf("the host reads %q, wanted the one edited", again.Host)
	}
	if again.PageSize != 250 {
		t.Errorf("the page size reads %d, wanted the 250 in the file", again.PageSize)
	}
	if again.Keepalive != 30*time.Second {
		t.Errorf("the keepalive reads %v, wanted the 30 seconds in the file", again.Keepalive)
	}
	if !again.Autocommit {
		t.Error("autocommit was lost")
	}
}
