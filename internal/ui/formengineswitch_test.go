package ui

import (
	"path/filepath"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/hist"
)

// openStoredState returns a store holding one tab of a PostgreSQL schema for that profile.
func openStoredState(t *testing.T, profileName string) *hist.Store {
	t.Helper()
	store, err := hist.Open(filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatalf("the history file was not opened: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SaveWorkspace(profileName, hist.SavedWorkspace{
		Tabs: []hist.SavedTab{{Kind: "table", Schema: "public", Name: "order"}},
	}); err != nil {
		t.Fatalf("the tabs were not saved: %v", err)
	}
	return store
}

// buildSavedFormModel returns a model editing that profile, with a config file of its own.
func buildSavedFormModel(t *testing.T, profile cfg.Profile, store *hist.Store) *Model {
	t.Helper()
	useConfigFile(t)
	useNoKeyring(t)
	model := NewModel(loadedConfigForTest("tokyonight"), nil, store, nil)
	model.profiles = []cfg.Profile{profile}
	model.form = NewFormState(profile, true, nil)
	model.screen = ScreenEditingConnection
	return model
}

// The stored tabs of a profile name the schemas and the tables of the server it opened. A
// profile that now opens another engine holds none of those names, and a restored tab asks
// the new server for a schema it does not have.
func TestSavingAnotherEngineForgetsTheTabsOfTheOldOne(t *testing.T) {
	profile := buildPromptingProfile("shop")
	store := openStoredState(t, profile.Name)
	model := buildSavedFormModel(t, profile, store)

	model.form.Fields = cfg.ApplyFieldChange(
		model.form.Fields, "engine", string(core.EngineMysql))
	model.saveForm()

	if _, found, err := store.FindWorkspace("shop"); found || err != nil {
		t.Errorf("the tabs of the old engine are still stored, and the read answered %v", err)
	}
}

// An edit that leaves the engine and the database alone keeps the tabs, because the same
// server still holds the same relations.
func TestSavingTheSameTargetKeepsTheTabs(t *testing.T) {
	profile := buildPromptingProfile("shop")
	store := openStoredState(t, profile.Name)
	model := buildSavedFormModel(t, profile, store)

	model.form.Fields = cfg.ApplyFieldChange(model.form.Fields, "host", "other.internal")
	model.saveForm()

	if _, found, err := store.FindWorkspace("shop"); !found || err != nil {
		t.Errorf("the tabs were dropped, and the read answered %v", err)
	}
}

// A profile of one database per connection names its relations under that database, so
// another database leaves the stored names pointing at nothing.
func TestSavingAnotherDatabaseForgetsTheTabs(t *testing.T) {
	profile := buildPromptingProfile("shop")
	store := openStoredState(t, profile.Name)
	model := buildSavedFormModel(t, profile, store)

	model.form.Fields = cfg.ApplyFieldChange(model.form.Fields, "database", "warehouse")
	model.saveForm()

	if _, found, err := store.FindWorkspace("shop"); found || err != nil {
		t.Errorf("the tabs of the old database are still stored, and the read answered %v", err)
	}
}
