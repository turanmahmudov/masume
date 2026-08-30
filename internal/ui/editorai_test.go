package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
)

// The editor offers the model one key, and which one it is follows what the statement is in.
// Two keys at once would leave a reader choosing between them instead of pressing one.

// findEditorAiKey answers the key the top border of the editor drew, and what it covers.
func findEditorAiKey(model *Model, frame []string) (buttonHit, string, bool) {
	for _, held := range model.layout.buttons {
		if held.row != firstPaneRow {
			continue
		}
		return held, cutRowText(frame[held.row], held.from, held.to), true
	}
	return buttonHit{}, "", false
}

// checkEditorOffers renders the frame and fails where the editor offers anything but this one
// key, drawn with the mark of the model.
func checkEditorOffers(t *testing.T, model *Model, action ActionID, said string) {
	t.Helper()
	frame := strings.Split(model.render(), "\n")

	held, text, found := findEditorAiKey(model, frame)
	if !found {
		t.Fatalf("the editor offered no key, and it should offer %q", said)
	}
	if held.action != action {
		t.Errorf("the editor offers %q, and it should offer %q", held.action, action)
	}
	if !strings.Contains(text, said) {
		t.Errorf("the key covers %q, and it should say %q", text, said)
	}
	// The glyph stands between the key and what it does, as it does on the title bar.
	if glyph := model.icons.Icon(cfg.IconAi); !strings.Contains(text, glyph+" "+said) {
		t.Errorf("the key covers %q, and it should carry the mark %q before %q",
			text, glyph, said)
	}

	// Only one key is ever drawn, so a reader has one thing to press and not a choice.
	offered := 0
	for _, drawn := range model.layout.buttons {
		if drawn.action == ActionAiFixError || drawn.action == ActionSendToAi ||
			drawn.action == ActionShowAiChat {
			offered++
		}
	}
	// The title bar names the chat itself, which is the second of the two.
	if offered != 2 {
		t.Errorf("the frame offers the model %d keys, and it should offer two", offered)
	}
}

// An empty editor is written for.
func TestAnEmptyEditorAsksTheModelForAQuery(t *testing.T) {
	model, _, tab := buildEditingModel(t, "", 0)
	tab.Focus = app.PaneEditor
	checkEditorOffers(t, model, ActionShowAiChat, "ask for a query")
}

// A statement that stands is asked about.
func TestAWrittenStatementIsAskedAbout(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id from orders", 0)
	tab.Focus = app.PaneEditor
	checkEditorOffers(t, model, ActionSendToAi, "ask about this")
}

// A statement the scanner marked is diagnosed.
func TestAMarkedStatementIsDiagnosed(t *testing.T) {
	model, _, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	tab.Editor = app.NewEditorBuffer("select * from public.nope", 0)
	if len(model.resolveLocalDiagnostics(model.Active(), tab)) == 0 {
		t.Skip("the scanner found no fault in this statement")
	}
	checkEditorOffers(t, model, ActionAiFixError, "diagnose this")
}

// A run the server refused is explained.
func TestARefusedRunIsExplained(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id from ordrs", 0)
	tab.Focus = app.PaneEditor
	tab.Results.Start([]string{"select id from ordrs"}, 10)
	tab.Results.Fail(0, "relation ordrs does not exist")
	checkEditorOffers(t, model, ActionAiFixError, "explain the failure")
}

// A press on the key runs it, so the model is reached without the keyboard.
func TestAPressOnTheEditorAiKeyRunsIt(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select id from orders", 0)
	tab.Focus = app.PaneEditor
	frame := strings.Split(model.render(), "\n")

	held, _, found := findEditorAiKey(model, frame)
	if !found {
		t.Fatal("the editor offered no key")
	}
	model.readMouse(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	if connection.Overlay.Kind != app.OverlayAiChat {
		t.Errorf("a press on the key opened %q", connection.Overlay.Kind)
	}
	if connection.Overlay.Draft.Text != "select id from orders" {
		t.Errorf("the panel opened on %q", connection.Overlay.Draft.Text)
	}
}

// The key on the title bar carries the mark of the model as well, so the two read as one
// thing in two places.
func TestTheTitleBarAiKeyCarriesItsMark(t *testing.T) {
	model, _, _ := buildEditingModel(t, "select id from orders", 0)
	frame := strings.Split(model.render(), "\n")

	held := findButtonOfAction(model, ActionShowAiChat)
	if held.row != model.layout.titleRow {
		t.Fatalf("the chat key stands on row %d", held.row)
	}
	text := cutRowText(frame[held.row], held.from, held.to)
	if !strings.Contains(text, model.icons.Icon(cfg.IconAi)+" ask ai") {
		t.Errorf("the key on the title bar covers %q", text)
	}
}
