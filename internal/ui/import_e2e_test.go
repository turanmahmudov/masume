// A functional test of the whole import: it opens a real SQLite file through the real
// adapter, walks the stages the card walks, and reads back the rows the import wrote.
package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/load"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// openImportModel answers a model on a real SQLite file that holds an empty orders table.
func openImportModel(t *testing.T) (*Model, *app.Connection, db.Session) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	// The adapter refuses a path with no file, so the file is made before it opens.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	session, err := engines.CreateAdapters().Open(ctx, cfg.Profile{
		Name: "shop", Engine: core.EngineSqlite, Database: path,
		AccessMode: cfg.AccessWrite, PageSize: cfg.DefaultPageSize, Autocommit: true,
	}, "")
	if err != nil {
		t.Fatalf("the file cannot be opened: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if _, err := session.RunQuery(ctx,
		"create table orders (id integer primary key, status text, note text)",
		10, nil); err != nil {
		t.Fatal(err)
	}

	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	connection.Session = session
	return model, connection, session
}

// writeCSV writes a file of that text and answers its path.
func writeCSV(t *testing.T, name, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// runImportStages walks the card from the file to the written rows, the way the keys of the
// card walk it, and answers what the import reported.
func runImportStages(
	t *testing.T, model *Model, connection *app.Connection, plan load.Plan,
) importRanMsg {
	t.Helper()
	session := connection.Session

	read, is := readImportFile(1, session, plan)().(importReadMsg)
	if !is || read.Problem != "" {
		t.Fatalf("the read of the file answered %+v", read)
	}
	plan = load.BuildPlan(plan.Path, plan.Options, read.Sample, plan.Table, read.Target)

	checked, is := checkImportFile(1, plan, session.Dialect())().(importCheckedMsg)
	if !is || checked.Problem != "" {
		t.Fatalf("the check of the file answered %+v", checked)
	}
	if len(checked.Statements) == 0 {
		t.Fatal("the review holds no statement")
	}

	answered, is := runImport(connection, 1, plan, session.Dialect(), 0, nil)().(importRanMsg)
	if !is {
		t.Fatal("the import answered something other than a run")
	}
	return answered
}

// readImportedRows returns the note of every row of that relation, by id.
func readImportedRows(t *testing.T, session db.Session, target string) map[int64]string {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"select id, note from "+target+" order by id", 100, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	held := map[int64]string{}
	for _, row := range answered.Rows {
		held[db.ReadNonNegativeCount(row[0])] = db.ReadAnyText(row[1])
	}
	return held
}

func TestImportWritesTheRowsOfACSVIntoATable(t *testing.T) {
	model, connection, session := openImportModel(t)
	path := writeCSV(t, "orders.csv",
		"id,status,note\n1,open,first\n2,sent,\n3,open,\"a ; note\"\n")

	answered := runImportStages(t, model, connection, load.Plan{
		Path: path, Options: load.DefaultReadOptions(),
		Table: db.TableRef{Schema: "main", Name: "orders"}.Qualified(),
	})
	if answered.Problem != "" {
		t.Fatalf("the import answered %q", answered.Problem)
	}
	if answered.Written != 3 {
		t.Fatalf("written: %d, want three rows", answered.Written)
	}

	rows := readImportedRows(t, session, "orders")
	if len(rows) != 3 || rows[1] != "first" || rows[3] != "a ; note" {
		t.Fatalf("rows: %v, want the three rows of the file", rows)
	}
}

func TestImportMakesTheTableItWritesInto(t *testing.T) {
	model, connection, session := openImportModel(t)
	path := writeCSV(t, "notes.csv", "id,note\n1,first\n2,second\n")

	answered := runImportStages(t, model, connection, load.Plan{
		Path: path, Options: load.DefaultReadOptions(), CreatesTable: true,
		Table: db.TableRef{Schema: "main", Name: "notes"}.Qualified(),
	})
	if answered.Problem != "" || answered.Written != 2 {
		t.Fatalf("the import answered %+v", answered)
	}

	rows := readImportedRows(t, session, "notes")
	if len(rows) != 2 || rows[2] != "second" {
		t.Fatalf("rows: %v, want the two rows of the file", rows)
	}
}

func TestImportRefusesARowTheTableCannotHold(t *testing.T) {
	model, connection, session := openImportModel(t)
	// The second row holds a word where the column holds a number, and the check of the
	// file refuses it.
	path := writeCSV(t, "orders.csv", "id,status,note\n1,open,first\nabc,sent,second\n")

	answered := runImportStages(t, model, connection, load.Plan{
		Path: path, Options: load.DefaultReadOptions(),
		Table: db.TableRef{Schema: "main", Name: "orders"}.Qualified(),
	})
	if answered.Problem != "" {
		t.Fatalf("the import answered %q", answered.Problem)
	}
	if answered.Written != 1 {
		t.Fatalf("written: %d, want the one row the table holds", answered.Written)
	}
	if rows := readImportedRows(t, session, "orders"); len(rows) != 1 {
		t.Fatalf("rows: %v, want the one row of the file", rows)
	}
}

func TestImportOfAJSONFileWritesItsDocuments(t *testing.T) {
	model, connection, session := openImportModel(t)
	path := writeCSV(t, "orders.json",
		`[{"id":1,"status":"open","note":"first"},{"id":2,"status":"sent","note":null}]`)

	answered := runImportStages(t, model, connection, load.Plan{
		Path: path, Options: load.BuildReadOptions(path),
		Table: db.TableRef{Schema: "main", Name: "orders"}.Qualified(),
	})
	if answered.Problem != "" || answered.Written != 2 {
		t.Fatalf("the import answered %+v", answered)
	}
	if rows := readImportedRows(t, session, "orders"); len(rows) != 2 {
		t.Fatalf("rows: %v, want both documents", rows)
	}
}

// The file picker is shared by the import and the restore, so a picked file must reach the
// card that opened it.
func TestThePickedFileReachesTheCardThatOpenedThePicker(t *testing.T) {
	model, connection, _ := openImportModel(t)
	path := writeCSV(t, "orders.csv", "id,status\n1,open\n")

	model.openImport(connection, db.TableRef{Schema: "main", Name: "orders"},
		intoTableThatIsThere)
	if connection.Overlay.Kind != app.OverlayImport ||
		connection.Overlay.Import.Stage != app.ImportPick {
		t.Fatalf("the import opened %q at %q", connection.Overlay.Kind,
			connection.Overlay.Import.Stage)
	}
	if picker := model.findFilePicker(model.ActiveID()); picker == nil {
		t.Fatal("the import holds no picker")
	} else if !strings.Contains(strings.Join(picker.AllowedTypes, " "), ".csv") {
		t.Errorf("the picker of an import offers %v", picker.AllowedTypes)
	}

	model.readPickedFile(connection, model.ActiveID(), path)
	if connection.Overlay.Import.Plan.Path != path {
		t.Errorf("the import holds %q, want the picked file", connection.Overlay.Import.Plan.Path)
	}

	model.openRestore(connection)
	if picker := model.findFilePicker(model.ActiveID()); picker == nil {
		t.Fatal("the restore holds no picker")
	} else if strings.Join(picker.AllowedTypes, " ") != ".sql" {
		t.Errorf("the picker of a restore offers %v", picker.AllowedTypes)
	}
	model.readRestoreFile(connection, "/tmp/shop.sql")
	if connection.Overlay.Dump.Path != "/tmp/shop.sql" {
		t.Errorf("the restore holds %q", connection.Overlay.Dump.Path)
	}
}

// Typing into the path row of each card writes into that card and into no other.
func TestTypingIntoAFormRowWritesIntoItsOwnCard(t *testing.T) {
	model, connection, _ := openImportModel(t)

	connection.Overlay = app.Overlay{
		Kind: app.OverlayImport,
		Import: app.ImportRequest{
			Stage: app.ImportFile, Plan: load.Plan{Options: load.DefaultReadOptions()},
		},
		Draft: app.NewEditorBuffer("", 0),
	}
	pressKey(t, model, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if connection.Overlay.Import.Plan.Path != "a" {
		t.Errorf("the import holds %q", connection.Overlay.Import.Plan.Path)
	}

	connection.Overlay = app.Overlay{
		Kind:  app.OverlayDump,
		Dump:  app.DumpRequest{Mode: app.DumpWrite, Stage: app.DumpForm},
		Draft: app.NewEditorBuffer("", 0),
	}
	pressKey(t, model, tea.KeyPressMsg{Code: 'b', Text: "b"})
	if connection.Overlay.Dump.Path != "b" {
		t.Errorf("the dump holds %q", connection.Overlay.Dump.Path)
	}

	connection.Overlay = app.Overlay{
		Kind:   app.OverlayExport,
		Export: app.ExportRequest{Format: result.ExportCSV, CSV: result.DefaultCSVOptions()},
		Draft:  app.NewEditorBuffer("", 0),
	}
	pressKey(t, model, tea.KeyPressMsg{Code: 'c', Text: "c"})
	if connection.Overlay.Export.Path != "c" {
		t.Errorf("the export holds %q", connection.Overlay.Export.Path)
	}
}

// The card follows the write: the import reports the rows it has written as it goes.
func TestTheImportReportsTheRowsItWrites(t *testing.T) {
	model, connection, _ := openImportModel(t)
	rows := strings.Builder{}
	rows.WriteString("id,status,note\n")
	for at := 1; at <= 2500; at++ {
		rows.WriteString(fmt.Sprintf("%d,open,note %d\n", at, at))
	}
	path := writeCSV(t, "many.csv", rows.String())

	plan := load.Plan{
		Path: path, Options: load.DefaultReadOptions(),
		Table: db.TableRef{Schema: "main", Name: "orders"}.Qualified(),
	}
	read := readImportFile(1, connection.Session, plan)().(importReadMsg)
	plan = load.BuildPlan(plan.Path, plan.Options, read.Sample, plan.Table, read.Target)

	updates := make(chan app.Progress, 64)
	command := runImport(connection, 1, plan, connection.Session.Dialect(), 2500, updates)
	answered := command().(importRanMsg)
	if answered.Problem != "" || answered.Written != 2500 {
		t.Fatalf("the import answered %+v", answered)
	}

	held := []app.Progress{}
	for report := range updates {
		held = append(held, report)
	}
	if len(held) == 0 {
		t.Fatal("the import reported nothing")
	}
	last := held[len(held)-1]
	if last.Done != 2500 || last.Total != 2500 || last.Label != "rows" {
		t.Fatalf("the last report is %+v, want every row", last)
	}
	// The card draws the last report it read.
	connection.Overlay = app.Overlay{
		Kind: app.OverlayImport,
		Import: app.ImportRequest{
			Stage: app.ImportReview, Running: true, Plan: plan, Progress: last,
		},
	}
	if drawn := model.View().Content; !strings.Contains(drawn, "2,500 of 2,500 rows") {
		t.Errorf("the card draws no count:\n%s", drawn)
	}
}
