package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
)

// A term typed into a card keeps fewer rows than the card held, so the cursor goes back to
// the first of them. A cursor left where it stood points past the end of what the term kept,
// and a card whose cursor points past its rows draws no preview and takes no answer.
func TestATermTypedIntoACardPutsTheCursorBackOnTheFirstRow(t *testing.T) {
	for _, held := range []struct {
		name    string
		overlay app.Overlay
	}{
		{"the theme picker", app.Overlay{Kind: app.OverlayThemePicker}},
		{"a menu of actions", app.Overlay{
			Kind: app.OverlayActionMenu,
			Actions: []app.MenuAction{
				{ID: "one", Label: "alpha"}, {ID: "two", Label: "beta"},
			},
		}},
		{"the command palette", app.Overlay{
			Kind: app.OverlayPalette,
			Palette: []app.PaletteAction{
				{ID: "one", Label: "alpha"}, {ID: "two", Label: "beta"},
			},
		}},
	} {
		model, connection, _ := buildEditingModel(t, "select 1", 0)
		overlay := held.overlay
		overlay.Draft = app.NewEditorBuffer("", 0)
		overlay.List.Cursor, overlay.List.Offset = 9, 9
		connection.Overlay = overlay

		model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})

		if connection.Overlay.List.Cursor != 0 || connection.Overlay.List.Offset != 0 {
			t.Errorf("%s left the cursor at %d and the rows at %d",
				held.name, connection.Overlay.List.Cursor, connection.Overlay.List.Offset)
		}
	}
}

// The theme the picker previews is the one under the cursor of the rows the term kept, so a
// term that keeps one theme previews that theme.
func TestTheThemePickerPreviewsWhatTheTermKept(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select 1", 0)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayThemePicker, Draft: app.NewEditorBuffer("", 0),
		Body: model.styles.Theme.Name,
	}

	for _, letter := range "dracula" {
		model.Update(tea.KeyPressMsg{Code: letter, Text: string(letter)})
	}

	if kept := model.filterThemes(connection.Overlay); len(kept) != 1 {
		t.Fatalf("the term kept %d themes, wanted one", len(kept))
	}
	if model.styles.Theme.Name != "dracula" {
		t.Errorf("the picker previews %q, wanted the theme the term kept",
			model.styles.Theme.Name)
	}
}
