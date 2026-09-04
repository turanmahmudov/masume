package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/load"
)

// The picker offers the files this client can read and no others, so a file it would refuse
// is never chosen by mistake.
func TestBuildFilePickerOffersTheFilesAnImportReads(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	picker := model.buildFilePicker()

	for _, extension := range []string{".csv", ".tsv", ".json", ".jsonl", ".ndjson"} {
		if !slices.Contains(picker.AllowedTypes, extension) {
			t.Errorf("the picker refuses %s, which an import reads", extension)
		}
	}
	if slices.Contains(picker.AllowedTypes, ".md") {
		t.Error("the picker offers a file no import reads")
	}
	if picker.DirAllowed || !picker.FileAllowed {
		t.Error("the picker chooses a directory instead of a file")
	}
	// The card keeps its height, so the picker draws a fixed number of rows.
	if picker.AutoHeight || picker.Height() != pickerRows {
		t.Errorf("the picker draws %d rows and follows the screen: %v",
			picker.Height(), picker.AutoHeight)
	}
	if picker.ShowPermissions {
		t.Error("the picker draws the permissions of a file, which say nothing about it")
	}
}

// The size of a file is padded to the room the style holds, so the sizes of a directory
// stand under one another.
func TestBuildFilePickerAlignsTheSizeOfAFile(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	styles := model.buildPickerStyles()

	if styles.FileSize.GetWidth() != fileSizeWidth {
		t.Errorf("the size takes %d cells, wanted %d",
			styles.FileSize.GetWidth(), fileSizeWidth)
	}
	if written := styles.FileSize.Render("31B"); !strings.HasPrefix(written, " ") {
		t.Errorf("the size reads %q, wanted it to the right of its room", written)
	}
}

// The file the user picked is written into the row of the form the picker fills in, and the
// file is read at once: the form asked for a path and now it has one.
func TestReadPickedFileWritesThePathAndReadsTheFile(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	path := filepath.Join(t.TempDir(), "orders.tsv")
	if err := os.WriteFile(path, []byte("a\tb\n1\t2\n"), 0o600); err != nil {
		t.Fatalf("the file cannot be written: %v", err)
	}

	connection := buildImportConnection()
	held, command, taken := model.readPickedFile(connection, 1, path)
	if !taken || command == nil {
		t.Fatalf("the file was picked and nothing read it: %v, %v", taken, command)
	}
	_ = held

	overlay := connection.Overlay
	if overlay.Import.Plan.Path != path {
		t.Errorf("the path reads %q, wanted %q", overlay.Import.Plan.Path, path)
	}
	if overlay.Import.Stage != app.ImportFile {
		t.Errorf("the stage is %q, wanted the one that reads the file",
			overlay.Import.Stage)
	}
	if !overlay.Import.Running {
		t.Error("the card does not say that the file is being read")
	}
	// The row of the form reads the path as well, so the form shows what was picked.
	if overlay.Field != importFileField || overlay.Draft.Text != path {
		t.Errorf("the form holds %q on row %d, wanted the path on the file row",
			overlay.Draft.Text, overlay.Field)
	}
	// The name of the file marks it as separated by tabs, which the picker fills in too.
	if overlay.Import.Plan.Options.Delimiter != "\t" {
		t.Errorf("the delimiter reads %q, wanted a tab",
			overlay.Import.Plan.Options.Delimiter)
	}
}

// buildImportConnection returns a connection with an import open on the picker.
func buildImportConnection() *app.Connection {
	connection := &app.Connection{}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayImport,
		Import: app.ImportRequest{
			Stage: app.ImportPick,
			Plan:  load.Plan{Options: load.DefaultReadOptions()},
		},
		Draft: app.NewEditorBuffer("", 0),
	}
	return connection
}

// A picker belongs to the connection its import is open on, so an import on one connection
// does not read the directory of another.
func TestFilePickersBelongToTheirConnection(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)

	if model.findFilePicker(1) != nil {
		t.Error("a connection with no import holds a picker")
	}
	model.openFilePicker(1)
	model.openFilePicker(2)

	first, second := model.findFilePicker(1), model.findFilePicker(2)
	if first == nil || second == nil {
		t.Fatal("a picker is missing")
	}
	if first == second {
		t.Error("two imports share one picker")
	}
}
