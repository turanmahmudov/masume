package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
)

// Replace writes over every match of the term the reader last looked for. Without a term
// there is nothing to replace, and the field must not open and ask for a replacement it
// cannot use.

func TestTheEditorBindsOneKeyForFindingAndReplacing(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)

	if chords := model.registry.FindActionChords(
		cfg.ScopeEditor, ActionReplaceInStatement); len(chords) != 0 {
		t.Errorf("the editor binds %v to replace, and finding and replacing is one key",
			chords)
	}
	if chords := model.registry.FindActionChords(
		cfg.ScopeEditor, ActionFindInStatement); len(chords) == 0 {
		t.Error("the editor binds nothing to find")
	}
	if chords := model.registry.FindActionChords(
		cfg.ScopeDialog, ActionReplaceInStatement); len(chords) == 0 {
		t.Error("the find field offers no key that turns it into the replace field")
	}
}

// The find field carries its term into the replace field, so the reader types the term once.
func TestTheFindFieldTurnsIntoTheReplaceFieldOnTheTermTyped(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select id from orders", 0)
	model.startFinding(connection, tab, app.PromptFind)
	connection.Overlay.Draft = app.NewEditorBuffer("orders", 6)

	model.turnFindIntoReplace(connection, tab, connection.Overlay)

	if tab.Find.Term != "orders" {
		t.Errorf("the term is %q, and it should carry the one typed into find", tab.Find.Term)
	}
	if connection.Overlay.Prompt != app.PromptReplace {
		t.Fatalf("the field is %q, wanted the one that replaces", connection.Overlay.Prompt)
	}
	if title := connection.Overlay.Title; !strings.Contains(title, "orders") {
		t.Errorf("the field is titled %q, and it should name the term", title)
	}
}

func TestTheFindFieldWillNotTurnIntoReplaceWithNothingTyped(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select id from orders", 0)
	model.startFinding(connection, tab, app.PromptFind)
	connection.Overlay.Draft = app.NewEditorBuffer("", 0)

	model.turnFindIntoReplace(connection, tab, connection.Overlay)

	if connection.Overlay.Prompt == app.PromptReplace {
		t.Error("the replace field opened, and nothing was typed to look for")
	}
	if connection.Notice == nil ||
		!strings.Contains(connection.Notice.Text, "type what to look for first") {
		t.Error("it did not say that a term has to be typed first")
	}
}

func TestReplaceOpensOnceATermIsLookedFor(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select id from orders", 0)
	tab.Find.Term = "id"

	model.startFinding(connection, tab, app.PromptReplace)

	if connection.Overlay.Kind != app.OverlayPrompt {
		t.Fatal("the field did not open, and there is a term to replace")
	}
	if connection.Overlay.Prompt != app.PromptReplace {
		t.Errorf("the field is %q, wanted the one that replaces", connection.Overlay.Prompt)
	}
}

// The title names the term, so the field says what it is about to overwrite rather than
// leaving the reader to remember.
func TestTheReplaceFieldNamesTheTermItWillOverwrite(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select id from orders", 0)
	tab.Find.Term = "orders"

	model.startFinding(connection, tab, app.PromptReplace)

	if title := connection.Overlay.Title; !strings.Contains(title, "orders") {
		t.Errorf("the field is titled %q, and it should name the term", title)
	}
}

func TestTheFindFieldIsStillTitledFind(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select id from orders", 0)

	model.startFinding(connection, tab, app.PromptFind)

	if title := connection.Overlay.Title; title != "find" {
		t.Errorf("the field is titled %q, wanted \"find\"", title)
	}
}

// The editor's hint bar has to name the key, or the whole find and replace flow is
// invisible to a reader who does not open the help.
func TestTheEditorHintBarNamesTheFindKey(t *testing.T) {
	model := buildOfflineModel(t, 150, 34)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneEditor

	frame := model.render()
	if !strings.Contains(frame, "find or replace") {
		t.Error("the hint bar does not name the key that finds and replaces")
	}
}
