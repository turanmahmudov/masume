package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

const testProjectFile = "/repo/.masume.toml"

// buildProjectProfile returns a connection of the kind a project file provides.
func buildProjectProfile(name string) cfg.Profile {
	return cfg.Profile{
		Name: name, Engine: core.EnginePostgres, Host: "127.0.0.1", Port: 5432,
		Database: name, User: "dev", Auth: cfg.AuthPrompt,
		Environment: cfg.EnvironmentDev, AccessMode: cfg.AccessWrite,
		PageSize: cfg.DefaultPageSize, ProjectFile: testProjectFile,
	}
}

// A reader must be able to tell which connections the repository provides, and which file
// they come from, because the connection form did not write them.
func TestThePickerMarksTheConnectionsOfTheProjectFile(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	model.profiles = []cfg.Profile{
		buildProjectProfile("shop-dev"),
		{Name: "mine", Engine: core.EnginePostgres, Host: "db", Port: 5432,
			Database: "mine", User: "ada", InConfigFile: true},
	}
	model.project = cfg.ProjectConfig{Path: testProjectFile}

	drawn := stripEscapes(model.renderPicker())
	for _, line := range strings.Split(drawn, "\n") {
		if !strings.Contains(line, "shop-dev") {
			continue
		}
		if !strings.Contains(line, "project") {
			t.Errorf("the row of the project connection reads %q", strings.TrimSpace(line))
		}
	}
	if strings.Contains(drawn, "mine") && !strings.Contains(drawn, testProjectFile) {
		t.Error("the card does not name the project file it read")
	}
}

// The column costs width, so it stands empty for a user who has no project file.
func TestThePickerDrawsNoSourceColumnWithoutAProjectFile(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	model.profiles = []cfg.Profile{
		{Name: "mine", Engine: core.EnginePostgres, Host: "db", Port: 5432,
			Database: "mine", User: "ada", InConfigFile: true},
	}

	drawn := stripEscapes(model.renderPicker())
	if strings.Contains(drawn, "project") {
		t.Error("the card names a project file where there is none")
	}
}

// A project connection is not in the config file of the user, so the client cannot remove it
// from there. It says where the connection comes from instead.
func TestDeletingAProjectConnectionNamesTheFileInstead(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)

	model.askDeleteProfile(buildProjectProfile("shop-dev"))
	if model.confirm != nil {
		t.Fatal("the client asked to remove a connection of the project file")
	}
	if !strings.Contains(model.picker.problem, testProjectFile) {
		t.Errorf("the client reported %q, wanted the project file named", model.picker.problem)
	}
}

// A project connection is already in a file, so the question asked on the way out does not
// offer to write it again.
func TestAProjectConnectionIsNotOfferedForSavingOnExit(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)

	model.recordUnsavedConnection(buildProjectProfile("shop-dev"))
	if len(model.unsaved) != 0 {
		t.Errorf("the client holds %d connections to write, wanted none", len(model.unsaved))
	}
}

// A statement of the project file is removed by editing that file, which the whole team
// shares.
func TestDeletingAProjectQueryNamesTheFileInstead(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlaySaved,
		Saved: []app.SavedRow{
			{Name: "recent-orders", SQL: "select 1", ProjectFile: testProjectFile},
		},
		Draft: app.NewEditorBuffer("", 0),
	}

	_, command := model.deleteSavedQuery(connection, &connection.Overlay)
	if command != nil {
		t.Fatal("the client started a removal of a statement of the project file")
	}
	if connection.Notice == nil ||
		!strings.Contains(connection.Notice.Text, testProjectFile) {
		t.Error("the client did not name the project file the statement comes from")
	}
}

// The card marks the statements of the project file, so a reader knows which ones the team
// shares and which ones only they hold.
func TestTheSavedCardMarksTheQueriesOfTheProjectFile(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	overlay := app.Overlay{
		Kind: app.OverlaySaved,
		Saved: []app.SavedRow{
			{Name: "mine", SQL: "select 1"},
			{Name: "recent-orders", SQL: "select 2",
				Description: "the newest orders", ProjectFile: testProjectFile},
		},
		Draft: app.NewEditorBuffer("", 0),
	}

	drawn := stripEscapes(model.renderSaved(overlay, 86))
	for _, line := range strings.Split(drawn, "\n") {
		switch {
		case strings.Contains(line, "recent-orders"):
			if !strings.Contains(line, "project") {
				t.Errorf("the project row reads %q", strings.TrimSpace(line))
			}
			// The file says what the statement answers, which reads better than its text.
			if !strings.Contains(line, "the newest orders") {
				t.Errorf("the project row does not show its description: %q",
					strings.TrimSpace(line))
			}
		case strings.Contains(line, "mine"):
			if strings.Contains(line, "project") {
				t.Errorf("the row of the user is marked as the project's: %q",
					strings.TrimSpace(line))
			}
		}
	}
}
