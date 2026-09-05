package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
)

// Every card that draws a field takes a paste, so a value too long to type by hand can be
// pasted into the cell editor, the chat, a form and a prompt.
func TestAPasteLandsInTheFieldOfTheCardOnShow(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select 1", 0)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayPrompt, Prompt: app.PromptSaveName,
		Draft: app.NewEditorBuffer("", 0),
	}

	model.readPaste("nightly report")

	if connection.Overlay.Draft.Text != "nightly report" {
		t.Errorf("the field holds %q, wanted the pasted text",
			connection.Overlay.Draft.Text)
	}
}

// A field of one line takes the breaks of a paste as blanks, and a field of many keeps them.
func TestAPasteKeepsItsBreaksOnlyWhereTheFieldHoldsMoreThanOneLine(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select 1", 0)

	connection.Overlay = app.Overlay{
		Kind: app.OverlayPrompt, Prompt: app.PromptWhere,
		Draft: app.NewEditorBuffer("", 0),
	}
	model.readPaste("id = 1\nand country = 'DE'")
	if strings.Contains(connection.Overlay.Draft.Text, "\n") {
		t.Errorf("a field of one line holds %q", connection.Overlay.Draft.Text)
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayCellEdit, Draft: app.NewEditorBuffer("", 0),
	}
	model.readPaste("first\r\nsecond")
	if connection.Overlay.Draft.Text != "first\nsecond" {
		t.Errorf("the cell editor holds %q, wanted both lines",
			connection.Overlay.Draft.Text)
	}
}

// The export form reads its own field back, so a pasted path is the path it writes to.
func TestAPasteIntoTheExportFormWritesThePath(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select 1", 0)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayExport, Field: exportPathField,
		Draft: app.NewEditorBuffer("", 0),
	}

	model.readPaste("/tmp/rows.csv")

	if connection.Overlay.Export.Path != "/tmp/rows.csv" {
		t.Errorf("the export writes to %q, wanted the pasted path",
			connection.Overlay.Export.Path)
	}
}
