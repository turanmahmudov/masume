package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/engines"
)

// buildStartProfiles returns the profiles a client started from the command line lists: the
// target first, then the profiles of the config file.
func buildStartProfiles(target cfg.Profile) []cfg.Profile {
	return []cfg.Profile{target, {Name: "ledger"}, {Name: "shop"}}
}

// A connection named on the command line must open as the client starts, so the user reads
// the data and not the picker.
func TestOpenAtStartConnectsWithoutThePicker(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), engines.CreateAdapters(), nil, nil)
	target := cfg.Profile{
		Name: "orders", Engine: core.EnginePostgres, Host: "127.0.0.1", Port: 5432,
		Database: "orders", User: "ada", Auth: cfg.AuthPassword, Password: "secret",
	}
	model.profiles = buildStartProfiles(target)
	model.OpenAtStart(target)

	if command := model.Init(); command == nil {
		t.Fatal("the client starts without opening the connection")
	}
	if model.screen != ScreenConnecting {
		t.Errorf("the screen is %q, wanted the connecting screen", model.screen)
	}
	if !model.picker.waitsFor("orders") {
		t.Error("the client does not wait for the profile of the command line")
	}
}

// A connection without a password must ask for one as the client starts, and not open a
// connection that the server refuses.
func TestOpenAtStartAsksForAPasswordItHasNot(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), engines.CreateAdapters(), nil, nil)
	target := cfg.Profile{
		Name: "orders", Engine: core.EnginePostgres, Host: "127.0.0.1", Port: 5432,
		Database: "orders", User: "ada", Auth: cfg.AuthPassword,
	}
	model.profiles = buildStartProfiles(target)
	model.OpenAtStart(target)
	model.Init()

	if model.screen != ScreenPromptingPassword {
		t.Errorf("the screen is %q, wanted the password prompt", model.screen)
	}
}

// The picker must stand on the connection the command line opened, so `Esc` from it goes
// back to a list with that row under the cursor.
func TestOpenAtStartLeavesTheCursorOnItsProfile(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), engines.CreateAdapters(), nil, nil)
	target := cfg.Profile{Name: "shop", Engine: core.EngineSqlite, Database: "./shop.db"}
	model.profiles = []cfg.Profile{{Name: "ledger"}, target}
	model.OpenAtStart(target)

	if model.picker.cursor != 1 {
		t.Errorf("the cursor stands on row %d, wanted the row of the target", model.picker.cursor)
	}
}

// A client that was given no target must draw the picker, because it has nothing to open.
func TestAClientWithoutATargetDrawsThePicker(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), engines.CreateAdapters(), nil, nil)
	model.profiles = []cfg.Profile{{Name: "shop"}}
	model.Init()

	if model.screen != ScreenPickingProfile {
		t.Errorf("the screen is %q, wanted the picker", model.screen)
	}
}
