package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// streamingSession records whether the export asked the server for the statement again, and
// answers the failure the test asks for.
type streamingSession struct {
	*offlineSession
	streamed bool
	failure  error
}

func (session *streamingSession) StreamQuery(
	_ context.Context, _ string, _ []any, _ int,
	_ func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	session.streamed = true
	return 0, session.failure
}

// buildExportModel answers a model whose grid holds that many rows of one read.
func buildExportModel(t *testing.T, rows int, source string) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()

	values := make([][]any, 0, rows)
	for at := range rows {
		values = append(values, []any{int64(at + 1), "ada"})
	}
	tab.Results.Start([]string{source}, 200)
	tab.Results.Succeed(0,
		db.ComposedRead{Text: source, Display: source, Pageable: true},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "id", DataType: "integer"},
				{Name: "customer", DataType: "text"},
			},
			Rows: values,
		})
	return model, connection, tab
}

// buildExportOverlay answers the export the form would have written.
func buildExportOverlay(path string, format result.ExportFormat, wholeRead bool) app.Overlay {
	return app.Overlay{
		Kind: app.OverlayExport,
		Export: app.ExportRequest{
			Path: path, Format: format, CSV: result.DefaultCSVOptions(), WholeRead: wholeRead,
		},
		Draft: app.NewEditorBuffer(path, len(path)),
	}
}

// runExport writes the export and answers what it reported and what the file holds.
func runExport(
	t *testing.T, model *Model, connection *app.Connection, tab *app.Tab,
	path string, overlay app.Overlay,
) (exportWrittenMsg, string) {
	t.Helper()
	command := model.startExport(connection, tab, path, overlay)
	if command == nil {
		t.Fatal("the export started nothing")
	}
	answered, is := command().(exportWrittenMsg)
	if !is {
		t.Fatal("the export answered something other than a written export")
	}
	written, err := os.ReadFile(path)
	if err != nil {
		return answered, ""
	}
	return answered, string(written)
}

// The rows loaded so far are already in hand, so the export writes them without asking the
// server for the statement again. A statement such as `update … returning` would otherwise
// write a second time.
func TestStartExportWritesTheLoadedRowsWithoutReadingAgain(t *testing.T) {
	model, connection, tab := buildExportModel(t, 3, "update orders set paid = true returning *")
	session := &streamingSession{offlineSession: connection.Session.(*offlineSession)}
	connection.Session = session

	path := filepath.Join(t.TempDir(), "orders.csv")
	answered, written := runExport(t, model, connection, tab, path,
		buildExportOverlay(path, result.ExportCSV, false))

	if session.streamed {
		t.Error("the export read the statement again")
	}
	if answered.Problem != "" {
		t.Fatalf("the export reported %q", answered.Problem)
	}
	if answered.Rows != 3 {
		t.Errorf("the export reported %d rows, wanted the 3 loaded", answered.Rows)
	}
	lines := strings.Split(strings.TrimRight(written, "\n"), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "id") {
		t.Errorf("the file holds %q", written)
	}
}

// Reading every row asks the server for the statement a second time, so a statement that
// writes is refused before the file is opened.
func TestWriteExportRefusesEveryRowOfAStatementThatWrites(t *testing.T) {
	model, connection, tab := buildExportModel(t, 2, "update orders set paid = true returning *")
	path := filepath.Join(t.TempDir(), "orders.csv")

	_, command := model.writeExport(connection, tab, buildExportOverlay(path, result.ExportCSV, true))

	if command != nil {
		t.Error("the export of a statement that writes was started")
	}
	if connection.Notice == nil || !strings.Contains(connection.Notice.Text, "writes") {
		t.Errorf("the report reads %v, wanted it to say the statement writes", connection.Notice)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the file was written")
	}
}

// A read of no rows still writes a file that reads back: an empty JSON array, and a CSV of
// its header alone.
func TestStartExportWritesAFileThatReadsBackForNoRows(t *testing.T) {
	for _, held := range []struct {
		format result.ExportFormat
		want   string
	}{
		{result.ExportJSON, "[\n]\n"},
		{result.ExportCSV, "id,customer\n"},
	} {
		t.Run(string(held.format), func(t *testing.T) {
			model, connection, tab := buildExportModel(t, 0, "select * from orders")
			path := filepath.Join(t.TempDir(), "orders."+string(held.format))
			answered, written := runExport(t, model, connection, tab, path,
				buildExportOverlay(path, held.format, false))

			if answered.Problem != "" {
				t.Fatalf("the export reported %q", answered.Problem)
			}
			if written != held.want {
				t.Errorf("the file holds %q, wanted %q", written, held.want)
			}
		})
	}
}

// A read that fails part way must leave the file that was there whole, because the export
// writes over a file the user already said yes to.
func TestStartExportLeavesTheOldFileWhereTheReadFails(t *testing.T) {
	model, connection, tab := buildExportModel(t, 2, "select * from orders")
	session := &streamingSession{
		offlineSession: connection.Session.(*offlineSession),
		failure:        errors.New("the connection was lost"),
	}
	connection.Session = session

	directory := t.TempDir()
	path := filepath.Join(directory, "orders.csv")
	if err := os.WriteFile(path, []byte("the export of yesterday\n"), 0o600); err != nil {
		t.Fatalf("cannot write the file: %v", err)
	}

	answered, written := runExport(t, model, connection, tab, path,
		buildExportOverlay(path, result.ExportCSV, true))

	if answered.Problem == "" {
		t.Error("a read that failed reported no problem")
	}
	if written != "the export of yesterday\n" {
		t.Errorf("the file holds %q, wanted the export that was there", written)
	}
	left, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("cannot read the directory: %v", err)
	}
	if len(left) != 1 {
		t.Errorf("the directory holds %d files, wanted only the one that was there", len(left))
	}
}

// cancellingStreamSession stops the export from inside the stream, as the cancel key does
// while a whole read runs, and reports what the context of the export then says.
type cancellingStreamSession struct {
	*offlineSession
	cancel func()
}

func (session *cancellingStreamSession) StreamQuery(
	ctx context.Context, _ string, _ []any, _ int,
	_ func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	session.cancel()
	return 0, ctx.Err()
}

// Reading every row asks the server for the statement again, which on a large relation runs
// for minutes. The cancel key has to reach it, so the export streams under a context the
// connection can stop rather than one nothing holds.
func TestCancellingStopsAWholeReadExport(t *testing.T) {
	model, connection, tab := buildExportModel(t, 2, "select * from orders")
	held := connection.Session
	session := &cancellingStreamSession{
		offlineSession: held.(*offlineSession),
		cancel:         func() { model.cancelQuery(connection) },
	}
	connection.Session = session

	path := filepath.Join(t.TempDir(), "orders.csv")
	answered, _ := runExport(t, model, connection, tab,
		path, buildExportOverlay(path, result.ExportCSV, true))

	if answered.Problem == "" {
		t.Error("the export was written whole although it was cancelled while it streamed")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("the cancelled export left a file behind")
	}
}
