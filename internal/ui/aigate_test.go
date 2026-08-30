package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// With `[ai] enabled = false` masume carries no AI feature at all: no chord reaches one, and
// nothing on the screen or in the help names one. A reader who turned it off must not be
// shown a feature the client will refuse.

// buildModelWithoutAi answers a model with the AI features turned off.
func buildModelWithoutAi(t *testing.T, width, height int) *Model {
	t.Helper()
	loaded := loadedConfigForTest("tokyonight")
	loaded.Ai.Enabled = false

	model := NewModel(loaded, nil, nil, nil)
	held, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	model = held.(*Model)

	session := &offlineSession{
		profile:      cfg.Profile{Name: "offline", Engine: "postgres"},
		capabilities: core.Capabilities{SortsRead: true},
	}
	connection := app.NewConnection(session, nil, true)
	model.connections.open(connection)
	model.screen = ScreenWorking
	return model
}

func TestNoAiActionIsBoundWhereTheFeaturesAreOff(t *testing.T) {
	model := buildModelWithoutAi(t, 120, 34)

	for id := range aiActions {
		for _, scope := range cfg.KeyScopes {
			if chords := model.registry.FindActionChords(scope, id); len(chords) != 0 {
				t.Errorf("%q is bound to %v in scope %q, and the AI features are off",
					id, chords, scope)
			}
		}
	}
}

func TestEveryAiActionIsBoundWhereTheFeaturesAreOn(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)

	for id := range aiActions {
		bound := false
		for _, scope := range cfg.KeyScopes {
			if len(model.registry.FindActionChords(scope, id)) != 0 {
				bound = true
			}
		}
		if !bound {
			t.Errorf("%q has no chord, and the AI features are on", id)
		}
	}
}

func TestTheFrameNamesNoAiWhereTheFeaturesAreOff(t *testing.T) {
	model := buildModelWithoutAi(t, 120, 34)
	frame := model.render()

	for _, named := range []string{"ask ai", "ask about this", "ask for a query", "✦"} {
		if strings.Contains(frame, named) {
			t.Errorf("the frame says %q, and the AI features are off", named)
		}
	}
}

func TestTheFrameOffersNoAiButtonWhereTheFeaturesAreOff(t *testing.T) {
	model := buildModelWithoutAi(t, 120, 34)
	model.render()

	for _, held := range model.layout.buttons {
		if IsAiAction(held.action) {
			t.Errorf("the frame offers a press on %q, and the AI features are off",
				held.action)
		}
	}
}

func TestThePaletteOffersNoAiRowWhereTheFeaturesAreOff(t *testing.T) {
	model := buildModelWithoutAi(t, 120, 34)
	connection := model.Active()

	for _, row := range model.buildPaletteActions(connection) {
		if aiPaletteRows[row.ID] || strings.Contains(row.Label, "AI") {
			t.Errorf("the palette offers %q, and the AI features are off", row.Label)
		}
	}
}

func TestThePaletteOffersTheAiRowsWhereTheFeaturesAreOn(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	connection := model.Active()

	offered := map[string]bool{}
	for _, row := range model.buildPaletteActions(connection) {
		offered[row.ID] = true
	}
	for id := range aiPaletteRows {
		if !offered[id] {
			t.Errorf("the palette does not offer %q, and the AI features are on", id)
		}
	}
}

func TestTheHelpNamesNoAiWhereTheFeaturesAreOff(t *testing.T) {
	model := buildModelWithoutAi(t, 120, 34)

	for _, section := range model.listHelpSections() {
		if section.Title == aiHelpSection {
			t.Fatalf("the help holds the %q group, and the AI features are off", aiHelpSection)
		}
		for _, entry := range section.Entries {
			for _, id := range entry.Actions {
				if IsAiAction(id) {
					t.Errorf("the help names %q, and the AI features are off", id)
				}
			}
		}
	}
}

func TestTheHelpHoldsTheAiGroupWhereTheFeaturesAreOn(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)

	for _, section := range model.listHelpSections() {
		if section.Title == aiHelpSection {
			return
		}
	}
	t.Errorf("the help has no %q group, and the AI features are on", aiHelpSection)
}

// Every action the AI features own has to be in the set, or a chord still reaches it after
// the features are turned off. The ids are kebab-case, so "ai" stands as its own word.
func TestTheAiSetHoldsEveryAiAction(t *testing.T) {
	for _, action := range ActionCatalog {
		named := false
		for word := range strings.SplitSeq(string(action.ID), "-") {
			if word == "ai" {
				named = true
			}
		}
		if named && !IsAiAction(action.ID) {
			t.Errorf("%q is named for AI and is not in the set", action.ID)
		}
	}
}
