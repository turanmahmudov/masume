package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// buildUnsavedProfile returns a connection of the kind the command line builds: complete,
// and in no config file.
func buildUnsavedProfile(name string) cfg.Profile {
	return cfg.Profile{
		Name: name, Engine: core.EnginePostgres, Host: "db.internal", Port: 5432,
		Database: name, User: "ada", Auth: cfg.AuthPassword, Password: "secret",
		Environment: cfg.EnvironmentDev, AccessMode: cfg.AccessWrite,
		PageSize: cfg.DefaultPageSize,
	}
}

// pressCtrlC returns the press that quits the client.
func pressCtrlC() tea.Key {
	return tea.Key{Code: 'c', Mod: uv.ModCtrl}
}

// useConfigFile points the client at a config file of its own, so a test that saves a
// profile does not write to the file of the user.
func useConfigFile(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	path := cfg.ResolveConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("the config directory cannot be written: %v", err)
	}
	if err := os.WriteFile(path, []byte("# a config file\n"), 0o600); err != nil {
		t.Fatalf("the config file cannot be written: %v", err)
	}
	return path
}

// A connection opened from the command line is in no file. The client must offer to write
// it before it ends, because the user would otherwise type the URL again.
func TestQuittingAsksToSaveAConnectionThatIsInNoFile(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))

	model.readKey(pressCtrlC())
	if model.confirm == nil {
		t.Fatal("the client ended without offering to save the connection")
	}
	if model.quitting {
		t.Error("the client ended before the question was answered")
	}
	if !strings.Contains(model.confirm.Body, "shop") {
		t.Errorf("the question does not name the connection: %q", model.confirm.Body)
	}
	if !strings.Contains(model.confirm.Body, "password") {
		t.Errorf("the question does not say the password is written: %q", model.confirm.Body)
	}
	if model.confirm.Destructive {
		t.Error("the question is drawn as one that cannot be taken back")
	}
}

// A profile of the config file is already saved, so quitting must ask nothing.
func TestQuittingAsksNothingForAProfileOfTheConfigFile(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	saved := buildUnsavedProfile("shop")
	saved.InConfigFile = true
	model.recordUnsavedConnection(saved)

	if len(model.unsaved) != 0 {
		t.Fatalf("the client holds %d connections to save, wanted none", len(model.unsaved))
	}
	model.readKey(pressCtrlC())
	if model.confirm != nil {
		t.Error("the client asked about a profile the config file already holds")
	}
	if !model.quitting {
		t.Error("the client did not end")
	}
}

// A connection that was opened twice must be offered one time, because the file holds one
// profile per name.
func TestTheQuestionNamesEachConnectionOneTime(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))
	model.recordUnsavedConnection(buildUnsavedProfile("ledger"))

	if len(model.unsaved) != 2 {
		t.Fatalf("the client holds %d connections to save, wanted two", len(model.unsaved))
	}
	model.readKey(pressCtrlC())
	if model.confirm == nil {
		t.Fatal("the client ended without offering to save the connections")
	}
	if !strings.Contains(model.confirm.Body, "2 connections") {
		t.Errorf("the question does not count the connections: %q", model.confirm.Body)
	}
}

// The answer must write the connection to the config file, so the next run opens it by name.
func TestSavingOnExitWritesTheConnectionToTheConfigFile(t *testing.T) {
	path := useConfigFile(t)
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))
	model.readKey(pressCtrlC())

	if command := model.confirm.Answer(true); command == nil {
		t.Fatal("the client did not end after it saved")
	}
	if !model.quitting {
		t.Error("the client did not end after it saved")
	}

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) > 0 {
		t.Fatalf("the saved file reports %v", loaded.Problems)
	}
	if len(loaded.Profiles) != 1 || loaded.Profiles[0].Name != "shop" {
		t.Fatalf("the file holds %v, wanted the connection", loaded.Profiles)
	}
	if loaded.Profiles[0].Database != "shop" || loaded.Profiles[0].User != "ada" {
		t.Errorf("the saved profile reads %v, wanted the one that was open", loaded.Profiles[0])
	}
	if len(model.unsaved) != 0 {
		t.Errorf("the client still holds %d connections to save", len(model.unsaved))
	}
}

// The other answer must end the client and write nothing, because a user who answers no to a
// question must not be asked again by the same press.
func TestDecliningOnExitWritesNothing(t *testing.T) {
	path := useConfigFile(t)
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))
	model.readKey(pressCtrlC())

	if command := model.confirm.Answer(false); command == nil {
		t.Fatal("the client did not end after it was told not to save")
	}
	if !model.quitting {
		t.Error("the client did not end after it was told not to save")
	}
	if profiles := cfg.LoadConfig(path).Profiles; len(profiles) != 0 {
		t.Errorf("the file holds %v, wanted nothing", profiles)
	}
}

// A file that cannot be written must keep the client open with the reason, so the report
// reaches the user and not the terminal the client is about to leave.
func TestAFileThatCannotBeWrittenKeepsTheClientOpen(t *testing.T) {
	// A file stands where the config directory would be, so the directory cannot be made
	// and the write fails.
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("the file cannot be written: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocked)
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))
	model.readKey(pressCtrlC())

	if command := model.confirm.Answer(true); command != nil {
		t.Error("the client ended although it could not write the file")
	}
	if model.quitting {
		t.Error("the client ended although it could not write the file")
	}
	if model.picker.problem == "" {
		t.Error("the client says nothing about the file it could not write")
	}
	if model.screen != ScreenPickingProfile {
		t.Errorf("the screen is %q, wanted the picker with the report", model.screen)
	}
}

// A question that is open owns the keyboard. A second press must neither ask again nor end
// the client: quitting there would drop the connections the question is asking about.
func TestASecondPressLeavesTheQuestionStanding(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))

	model.readKey(pressCtrlC())
	first := model.confirm
	if first == nil {
		t.Fatal("the client ended without offering to save the connection")
	}

	model.readKey(pressCtrlC())
	if model.confirm != first {
		t.Error("a second press asked the question again")
	}
	if model.quitting {
		t.Error("a second press ended the client while the question stood")
	}
	if len(model.unsaved) != 1 {
		t.Error("the connection the question is about was dropped")
	}
}

// A save that fails part way names the one it stopped on, so the reader knows which
// connection is not in the file and what is.
func TestASaveThatFailsNamesWhereItStopped(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("the file cannot be written: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocked)

	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))
	model.recordUnsavedConnection(buildUnsavedProfile("ledger"))
	model.readKey(pressCtrlC())
	model.confirm.Answer(true)

	if !strings.Contains(model.picker.problem, "shop") {
		t.Errorf("the report reads %q, wanted the connection it stopped on",
			model.picker.problem)
	}
	// Nothing was written, so nothing is reported as written.
	if strings.Contains(model.picker.problem, "already written") {
		t.Errorf("the report reads %q, and nothing was written", model.picker.problem)
	}
	if len(model.unsaved) != 2 {
		t.Errorf("%d connections are still to save, wanted both", len(model.unsaved))
	}
}
