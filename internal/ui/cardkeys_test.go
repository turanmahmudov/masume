package ui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
)

// A card names its keys at its foot. Each one is a button, so the cells it is recorded at
// have to be the cells its own text was drawn in.
func TestTheKeysOfACardStandWhereTheyAreDrawn(t *testing.T) {
	model, _, _ := buildMenuModel(t)
	frame := strings.Split(model.render(), "\n")

	found := 0
	for _, held := range model.layout.buttons {
		text := cutRowText(frame[held.row], held.from, held.to)
		if strings.TrimSpace(text) == "" {
			t.Errorf("the key of %q covers %q", held.action, text)
			continue
		}
		if strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ") {
			t.Errorf("the key of %q covers %q, which is not the key alone", held.action, text)
		}
		found++
	}
	if found == 0 {
		t.Error("a card with a list recorded no key of its own")
	}
}

// A press on a key a card names runs it, so the card is answered without the keyboard.
func TestAPressOnAKeyOfACardRunsIt(t *testing.T) {
	model, connection, _ := buildMenuModel(t)
	model.render()

	held, found := findCardButton(model, ActionClose)
	if !found {
		t.Fatal("the card recorded no key that closes it")
	}
	model.readMouse(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	if connection.Overlay.IsOpen() {
		t.Error("a press on the close key left the card open")
	}
}

// The bar under a card names the keys of the card. Without this it offers the keys of the
// pane behind it, which neither a press nor a key reaches while the card is open.
func TestTheStatusBarNamesTheKeysOfTheCardOnShow(t *testing.T) {
	model, _, _ := buildMenuModel(t)
	frame := strings.Split(model.render(), "\n")

	bar := stripStyles(frame[model.layout.hintRow])
	if !strings.Contains(bar, "close") {
		t.Errorf("the bar under the menu reads %q", strings.TrimSpace(bar))
	}
	if strings.Contains(bar, "go to column") {
		t.Errorf("the bar under the menu still offers the keys of the grid: %q",
			strings.TrimSpace(bar))
	}
}

func TestAPressOnAKeyOfTheStatusBarUnderACardRunsTheCard(t *testing.T) {
	model, connection, _ := buildMenuModel(t)
	model.render()

	held, found := findBarButton(model, ActionClose)
	if !found {
		t.Fatal("the bar under the card recorded no key that closes it")
	}
	model.readMouse(tea.MouseClickMsg{X: held.from, Y: held.row, Button: tea.MouseLeft})
	if connection.Overlay.IsOpen() {
		t.Error("a press on the close key of the bar left the card open")
	}
}

// The keys of the title bar are drawn a step back while a card is open, and answer no press,
// because neither the keyboard nor the pointer reaches them then.
func TestTheTitleBarKeysAnswerNoPressWhileACardIsOpen(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	model.render()
	if _, found := findCardButton(model, ActionShowPalette); !found {
		t.Fatal("the title bar recorded no key while no card was open")
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayObjectMenu, Title: " public.orders ",
		Actions: []app.MenuAction{{Label: "Select 100 rows", Chord: "s"}},
	}
	model.render()
	if _, found := findCardButton(model, ActionShowPalette); found {
		t.Error("the title bar still answers a press while a card is open")
	}
}

// findCardButton answers the key of an action anywhere on the frame.
func findCardButton(model *Model, action ActionID) (buttonHit, bool) {
	for _, held := range model.layout.buttons {
		if held.action == action {
			return held, true
		}
	}
	return buttonHit{}, false
}

// findBarButton answers the key of an action on the status bar.
func findBarButton(model *Model, action ActionID) (buttonHit, bool) {
	for _, held := range model.layout.buttons {
		if held.action == action && held.row == model.layout.hintRow {
			return held, true
		}
	}
	return buttonHit{}, false
}

// A key of a card is drawn as a key wherever it stands: the chord in the ink of a key and what
// it does in the quiet ink. A key the field answers itself is still a key, so it is drawn as
// one, and a card whose keys read as a paragraph is a card a reader has to parse.
func TestEveryKeyOfACardIsDrawnAsAKey(t *testing.T) {
	for _, held := range []struct {
		name  string
		open  func(*Model, *app.Connection)
		keys  []string
		words []string
	}{
		{
			name: "the chat",
			open: func(model *Model, connection *app.Connection) {
				connection.Overlay = app.Overlay{
					Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer("", 0),
				}
			},
			keys:  []string{"↵", "⇧↵", "↑↓"},
			words: []string{"ask", "newline", "scroll"},
		},
		{
			name: "the card of a row",
			open: func(model *Model, connection *app.Connection) {
				connection.Overlay = app.Overlay{
					Kind: app.OverlayRowDetail, Draft: app.NewEditorBuffer("", 0),
					Window: app.RowWindow{
						Columns: []db.ResultColumn{{Name: "id", DataType: "int"}},
						Rows:    [][]any{{1}}, Index: 0,
					},
				}
			},
			keys:  []string{"←→", "↑↓"},
			words: []string{"another row", "scroll"},
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			model := buildLoadedModel(t, 1, 3, 8, 3)
			connection := model.Active()
			held.open(model, connection)
			frame := strings.Split(model.render(), "\n")

			lit, quiet := readInkedCells(model, frame)
			for _, chord := range held.keys {
				if !strings.Contains(lit, chord) {
					t.Errorf("the key %q is not drawn in the ink of a key, and the card "+
						"draws %q in it", chord, lit)
				}
			}
			for _, word := range held.words {
				if !strings.Contains(quiet, strings.ReplaceAll(word, " ", "")) {
					t.Errorf("what the key does, %q, is not drawn in the quiet ink", word)
				}
			}
		})
	}
}

// readInkedCells answers the cells of the card drawn in the ink of a key, and the cells drawn
// in the quiet ink, so a key line can be read back off the frame by its colours.
func readInkedCells(model *Model, frame []string) (string, string) {
	key := describeInk(model.styles.Theme.Accent)
	said := describeInk(model.styles.Theme.Muted)
	lit, quiet := strings.Builder{}, strings.Builder{}
	for row, line := range frame {
		if row == model.layout.hintRow || row == model.layout.titleRow {
			continue
		}
		for _, cell := range mapCells(line) {
			if strings.TrimSpace(cell.text) == "" {
				continue
			}
			switch {
			case strings.Contains(cell.sgr, key):
				lit.WriteString(cell.text)
			case strings.Contains(cell.sgr, said):
				quiet.WriteString(cell.text)
			}
		}
	}
	return lit.String(), quiet.String()
}

// describeInk writes the part of an escape that sets this ink.
func describeInk(ink color.Color) string {
	red, green, blue, _ := ink.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", red>>8, green>>8, blue>>8)
}

// The removal of a saved statement is written to a file away from the draw loop. The note that
// says it is gone waits for that write, because reporting first says "deleted" for a removal
// the file refused.
func TestDeletingASavedQueryReportsAfterTheFileTookIt(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind:  app.OverlaySaved,
		Saved: []hist.SavedQuery{{Name: "nightly", SQL: "select 1"}},
		Draft: app.NewEditorBuffer("", 0),
	}

	_, command := model.deleteSavedQuery(connection, &connection.Overlay)
	if connection.Notice != nil {
		t.Fatalf("the removal was reported before the file took it: %q", connection.Notice.Text)
	}
	if command == nil {
		t.Fatal("the removal started nothing")
	}
	answered, is := command().(savedQueryRemovedMsg)
	if !is {
		t.Fatalf("the removal answered %T", command())
	}

	model.readSavedQueryRemoved(answered)
	if connection.Notice == nil || !strings.Contains(connection.Notice.Text, "nightly") {
		t.Fatal("the removal was not reported once the file took it")
	}

	connection.Notice = nil
	model.readSavedQueryRemoved(savedQueryRemovedMsg{
		ConnectionID: answered.ConnectionID, Name: "nightly",
		Problem: "the query was not removed: the file is read-only",
	})
	if connection.Notice == nil || connection.Notice.Tone != app.NoticeError {
		t.Error("a removal the file refused was not reported as a failure")
	}
}
