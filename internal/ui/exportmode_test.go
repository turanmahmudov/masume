package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// An export holds the rows of the database, so it is written for its owner alone. A file
// every user of the machine can read hands them the data the connection was opened for.
func TestAnExportIsWrittenForItsOwnerAlone(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()

	tab.Results.Start([]string{"select * from orders"}, 200)
	tab.Results.Succeed(0,
		db.ComposedRead{Text: "select * from orders", Display: "select * from orders"},
		db.QueryResult{
			Columns: []db.ResultColumn{{Name: "id", DataType: "integer"}},
			Rows:    [][]any{{int64(1)}, {int64(2)}},
		})

	path := filepath.Join(t.TempDir(), "orders.csv")
	overlay := app.Overlay{Export: app.ExportRequest{
		Format: result.ExportCSV, CSV: result.DefaultCSVOptions(),
	}}
	command := model.startExport(connection, tab, path, overlay)
	if command == nil {
		t.Fatal("the export started nothing")
	}
	answered, is := command().(exportWrittenMsg)
	if !is {
		t.Fatalf("the export answered %T", command())
	}
	if answered.Problem != "" {
		t.Fatalf("the export failed: %s", answered.Problem)
	}

	found, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the export was not written: %v", err)
	}
	if found.Mode().Perm() != 0o600 {
		t.Errorf("the export reads %o, wanted 600", found.Mode().Perm())
	}
}

// The field of the export form and the export it writes are two stores of the same value, so
// every key that edits the field has to write it back. Delete edited the field alone, and the
// export was written to the path the field held before it.
func TestDeleteInTheExportPathWritesItBack(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()

	const path = "/tmp/orders.csv"
	connection.Overlay = app.Overlay{
		Kind:  app.OverlayExport,
		Field: exportPathField,
		Export: app.ExportRequest{
			Path: path, Format: result.ExportCSV, CSV: result.DefaultCSVOptions(),
		},
		Draft: app.NewEditorBuffer(path, 0),
	}

	model.readOverlayKey(connection, tea.Key{Code: tea.KeyDelete})

	held := connection.Overlay
	if held.Draft.Text != path[1:] {
		t.Fatalf("the field holds %q after a delete", held.Draft.Text)
	}
	if held.Export.Path != held.Draft.Text {
		t.Errorf("the export writes to %q, and the field holds %q",
			held.Export.Path, held.Draft.Text)
	}
}
