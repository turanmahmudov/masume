package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
)

// bindActionTo gives one action one chord, as a user file does.
func bindActionTo(t *testing.T, model *Model, actionKey, chord string) {
	t.Helper()
	sequence, parsed := cfg.ParseChordSequence(chord)
	if !parsed {
		t.Fatalf("the chord %q does not parse", chord)
	}
	problems := model.registry.ApplyKeySettings(FindKeyPreset(cfg.PresetDefault),
		cfg.ChordChoices{actionKey: []cfg.ChordSequence{sequence}}, true)
	if len(problems) > 0 {
		t.Fatalf("the binding reported %v", problems)
	}
	model.keymap = NewKeymap(model.registry)
}

// The rows of a form are stepped by an action, so a user who binds another chord to it steps
// the rows with that chord.
func TestTheFormStepsItsRowsOnTheBoundKey(t *testing.T) {
	model, connection, _ := buildBatchModel(t)
	bindActionTo(t, model, "dialog:next-field", "ctrl+n")
	connection.Overlay = app.Overlay{Kind: app.OverlayExport}

	model.readOverlayKey(connection, tea.Key{Code: 'n', Mod: uv.ModCtrl})
	if connection.Overlay.Field != 1 {
		t.Errorf("the form stands on row %d, wanted the second row", connection.Overlay.Field)
	}
	if bar := stripEscapes(model.render()); !strings.Contains(bar, "^N") {
		t.Error("the card of the form drew no key for the bound chord")
	}
}

// The chat sends the question on an action, so the bound chord sends it and the key it left
// writes a line instead.
func TestTheChatSendsOnTheBoundKey(t *testing.T) {
	model, _ := buildChatModel(t)
	bindActionTo(t, model, "dialog:send-question", "ctrl+y")
	connection := model.Active()

	connection.Overlay.Draft = app.NewEditorBuffer("how many orders", 15)
	model.readOverlayKey(connection, tea.Key{Code: tea.KeyEnter})
	if written := connection.Overlay.Draft.Text; written != "how many orders\n" {
		t.Errorf("the field holds %q, wanted the line the key it left writes", written)
	}
}

// The review of an import steps back to the form on an action, and the form closes the card,
// so one chord does not do both.
func TestTheImportReviewStepsBackOnTheBoundKey(t *testing.T) {
	model, connection, _ := buildBatchModel(t)
	bindActionTo(t, model, "dialog:step-back", "ctrl+b")
	connection.Overlay = app.Overlay{
		Kind:   app.OverlayImport,
		Import: app.ImportRequest{Stage: app.ImportReview},
	}

	model.readOverlayKey(connection, tea.Key{Code: 'b', Mod: uv.ModCtrl})
	if stage := connection.Overlay.Import.Stage; stage == app.ImportReview {
		t.Error("the review did not step back to the form")
	}
	if !connection.Overlay.IsOpen() {
		t.Error("the review closed the card instead of stepping back")
	}
}

// The password field keeps the password in the keyring on an action.
func TestThePasswordFieldUsesTheKeyringOnTheBoundKey(t *testing.T) {
	model := buildOfflineModelFor(t, 120, 40)
	bindActionTo(t, model, "dialog:use-keyring", "ctrl+k")
	model.screen = ScreenPromptingPassword
	model.picker.pending = cfg.Profile{Name: "shop", Engine: "postgres", Auth: cfg.AuthPrompt}
	before := model.picker.keepInKeyring

	model.readPasswordKey(tea.Key{Code: 'k', Mod: uv.ModCtrl})
	if model.picker.offersKeyring() && model.picker.keepInKeyring == before {
		t.Error("the bound chord did not reach the keyring")
	}
}
