package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/dump"
)

// dumpSession answers the catalog reads of a dump and keeps the statements of a restore.
type dumpSession struct {
	*offlineSession
	ran     []string
	failure error
	// transaction is the state the session reports.
	transaction db.TransactionState
	// began counts the transactions the session opened.
	began int
}

func (session *dumpSession) ReadTransactionState() db.TransactionState {
	return session.transaction
}

func (session *dumpSession) BeginTransaction(context.Context) error {
	session.began++
	session.transaction = db.TransactionOpen
	return nil
}

func (session *dumpSession) ListTables(context.Context) ([]db.TableRef, error) {
	return []db.TableRef{
		{Schema: "public", Name: "orders", Kind: db.RelationTable},
	}, nil
}

func (session *dumpSession) ListSchemaObjects(context.Context) ([]db.SchemaObject, error) {
	return nil, nil
}

func (session *dumpSession) ListRelationships(context.Context) ([]db.Relationship, error) {
	return nil, nil
}

func (session *dumpSession) BuildObjectDDL(
	context.Context, db.SchemaObject,
) ([]string, error) {
	return nil, nil
}

func (session *dumpSession) BuildTableDDL(
	context.Context, db.TableRef,
) ([]string, error) {
	return []string{"create table public.orders (id integer);"}, nil
}

func (session *dumpSession) StreamQuery(
	_ context.Context, _ string, _ []any, _ int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	rows := [][]any{{int64(1)}}
	if err := onBatch(rows, []db.ResultColumn{{Name: "id", DataType: "integer"}}); err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}

func (session *dumpSession) RunQuery(
	_ context.Context, sql string, _ int, _ []any,
) (db.QueryResult, error) {
	session.ran = append(session.ran, sql)
	if session.failure != nil {
		return db.QueryResult{}, session.failure
	}
	return db.QueryResult{}, nil
}

// findMessage runs the command, and every command of a batch, and answers the first message
// of that kind. The command that waits for the reports of a run ends with the run itself.
func findMessage[T tea.Msg](t *testing.T, command tea.Cmd) T {
	t.Helper()
	answers := make(chan tea.Msg, 8)
	var run func(held tea.Cmd)
	run = func(held tea.Cmd) {
		if held == nil {
			return
		}
		go func() {
			answered := held()
			if batch, is := answered.(tea.BatchMsg); is {
				for _, one := range batch {
					run(one)
				}
				return
			}
			answers <- answered
		}()
	}
	run(command)

	waited := time.After(10 * time.Second)
	for {
		select {
		case answered := <-answers:
			if wanted, is := answered.(T); is {
				return wanted
			}
		case <-waited:
			t.Fatal("the run answered no message of that kind")
		}
	}
}

// buildDumpModel answers a model whose connection answers the reads of a dump.
func buildDumpModel(t *testing.T) (*Model, *app.Connection, *dumpSession) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	session := &dumpSession{
		offlineSession: connection.Session.(*offlineSession),
		transaction:    db.TransactionNone,
	}
	connection.Session = session
	return model, connection, session
}

func TestOpenDumpOffersAnSQLFile(t *testing.T) {
	model, connection, _ := buildDumpModel(t)
	model.openDump(connection, "public", dump.Options{Schema: "public"})

	held := connection.Overlay
	if held.Kind != app.OverlayDump || held.Dump.Stage != app.DumpForm {
		t.Fatalf("overlay: %s %s, want the dump form", held.Kind, held.Dump.Stage)
	}
	if !strings.HasSuffix(held.Dump.Path, ".sql") {
		t.Errorf("path: %q, want a .sql file", held.Dump.Path)
	}
	if held.Dump.Options.Content != dump.ContentAll {
		t.Errorf("content: %q, want %q", held.Dump.Options.Content, dump.ContentAll)
	}
}

func TestDumpWritesTheFile(t *testing.T) {
	model, connection, _ := buildDumpModel(t)
	path := filepath.Join(t.TempDir(), "shop.sql")

	_, command := model.startDump(connection, path, "public", dump.Options{
		Schema: "public", Content: dump.ContentAll,
	})
	if command == nil {
		t.Fatal("the dump started nothing")
	}
	answered := findMessage[dumpWrittenMsg](t, command)
	if answered.Problem != "" {
		t.Fatalf("problem: %s", answered.Problem)
	}
	if answered.Report.Tables != 1 || answered.Report.Rows != 1 {
		t.Errorf("report: %+v, want one table and one row", answered.Report)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(written)
	if !strings.Contains(text, "create table public.orders") ||
		!strings.Contains(text, "insert into") {
		t.Errorf("the file holds neither the definition nor the rows:\n%s", text)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode: %v, want 0600", info.Mode().Perm())
	}
}

func TestDumpAsksBeforeOverwritingAFile(t *testing.T) {
	model, connection, _ := buildDumpModel(t)
	path := filepath.Join(t.TempDir(), "shop.sql")
	if err := os.WriteFile(path, []byte("-- old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{
			Mode: app.DumpWrite, Stage: app.DumpForm, Path: path, Target: "public",
			Options: dump.Options{Schema: "public", Content: dump.ContentAll},
		},
	}
	overlay := &connection.Overlay
	model.stepDump(connection, overlay)
	if connection.Overlay.Kind != app.OverlayConfirm {
		t.Fatalf("overlay: %s, want the question", connection.Overlay.Kind)
	}
}

func TestRestoreRunsEveryStatement(t *testing.T) {
	model, connection, session := buildDumpModel(t)
	path := filepath.Join(t.TempDir(), "shop.sql")
	if err := os.WriteFile(path,
		[]byte("create table t (id int);\ninsert into t values (1);\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{Mode: app.DumpRestore, Stage: app.DumpForm, Path: path},
	}
	_, command := model.startRestore(connection, path)
	if command == nil {
		t.Fatal("the restore started nothing")
	}
	answered := findMessage[restoreRanMsg](t, command)
	if answered.Problem != "" || answered.Report.Statements != 2 {
		t.Fatalf("answer: %+v, want two statements", answered)
	}
	if len(session.ran) != 2 {
		t.Errorf("ran: %q, want both statements", session.ran)
	}
}

func TestRestoreNamesTheStatementThatFailed(t *testing.T) {
	model, connection, session := buildDumpModel(t)
	session.failure = errors.New("syntax error")
	path := filepath.Join(t.TempDir(), "shop.sql")
	if err := os.WriteFile(path, []byte("select 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{Mode: app.DumpRestore, Stage: app.DumpForm, Path: path},
	}
	_, command := model.startRestore(connection, path)
	answered := findMessage[restoreRanMsg](t, command)
	if !strings.Contains(answered.Problem, "statement 1 failed") ||
		!strings.Contains(answered.Problem, "syntax error") {
		t.Errorf("problem: %q, want the statement and the reason", answered.Problem)
	}
}

// A restore runs the statements the way the editor runs them: with autocommit off, masume
// opens one transaction and leaves it for the user to commit.
func TestARestoreWithAutocommitOffOpensATransaction(t *testing.T) {
	model, connection, session := buildDumpModel(t)
	session.offlineSession.capabilities = core.Capabilities{HasTransactions: true}
	connection.Autocommit = false
	path := filepath.Join(t.TempDir(), "shop.sql")
	if err := os.WriteFile(path,
		[]byte("insert into t values (1);\ninsert into t values (2);\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{Mode: app.DumpRestore, Stage: app.DumpForm, Path: path},
	}
	_, command := model.startRestore(connection, path)
	answered := findMessage[restoreRanMsg](t, command)
	if answered.Problem != "" || answered.Report.Statements != 2 {
		t.Fatalf("answer: %+v, want two statements", answered)
	}
	if session.began != 1 {
		t.Errorf("transactions: %d, want the one the run opened", session.began)
	}
}

func TestDumpFormStepsThroughItsValues(t *testing.T) {
	overlay := &app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{
			Mode: app.DumpWrite, Stage: app.DumpForm, Path: "shop.sql", Target: "public",
			Options: dump.Options{Schema: "public", Content: dump.ContentAll},
		},
	}
	fields := BuildDumpFields(*overlay)
	if len(fields) != 3 {
		t.Fatalf("fields: %d, want the file, the content and the drop", len(fields))
	}

	overlay.Field = 1
	StepDumpChoice(overlay, 1)
	if overlay.Dump.Options.Content != dump.ContentSchema {
		t.Errorf("content: %q, want %q", overlay.Dump.Options.Content, dump.ContentSchema)
	}
	overlay.Field = 2
	StepDumpChoice(overlay, 1)
	if !overlay.Dump.Options.DropsFirst {
		t.Error("the drop was not taken")
	}

	overlay.Field = dumpPathField
	ReadDumpField(overlay, "~/other.sql")
	if overlay.Dump.Path != "~/other.sql" {
		t.Errorf("path: %q, want the typed one", overlay.Dump.Path)
	}
}

func TestDumpFormOfARestoreHoldsTheFileAlone(t *testing.T) {
	overlay := app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{Mode: app.DumpRestore, Stage: app.DumpForm, Path: "shop.sql"},
	}
	if fields := BuildDumpFields(overlay); len(fields) != 1 {
		t.Fatalf("fields: %d, want the file alone", len(fields))
	}
	if problem := FindDumpProblem(app.Overlay{Kind: app.OverlayDump}); problem == "" {
		t.Error("an empty path was taken")
	}
}

func TestDumpCardDraws(t *testing.T) {
	model, connection, _ := buildDumpModel(t)
	model.openDump(connection, "public", dump.Options{Schema: "public"})

	drawn := model.View().Content
	if !strings.Contains(drawn, "dump public") || !strings.Contains(drawn, "content") {
		t.Errorf("the card was not drawn:\n%s", drawn)
	}
}

// cancelSession blocks the read of a table until the context of the dump ends, so a test can
// stop a dump that is running.
type cancelSession struct {
	*dumpSession
	reading chan struct{}
}

func (session *cancelSession) StreamQuery(
	ctx context.Context, _ string, _ []any, _ int,
	_ func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	close(session.reading)
	<-ctx.Done()
	return 0, ctx.Err()
}

// Closing the card ends the dump, and the part file it was writing is taken away.
func TestClosingTheCardEndsTheDumpAndLeavesNoPartFile(t *testing.T) {
	model, connection, session := buildDumpModel(t)
	held := &cancelSession{dumpSession: session, reading: make(chan struct{})}
	connection.Session = held

	directory := t.TempDir()
	path := filepath.Join(directory, "shop.sql")
	_, command := model.startDump(connection, path, "public", dump.Options{
		Schema: "public", Content: dump.ContentAll,
	})

	answers := make(chan dumpWrittenMsg, 1)
	go func() { answers <- findMessage[dumpWrittenMsg](t, command) }()
	<-held.reading
	connection.StopDump()

	answered := <-answers
	if answered.Problem == "" {
		t.Fatalf("the stopped dump answered %+v", answered)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the directory holds %v, want no part file", entries)
	}
}

// The question about an existing file writes it once the answer is yes.
func TestTheOverwriteAnswerWritesTheDump(t *testing.T) {
	model, connection, _ := buildDumpModel(t)
	path := filepath.Join(t.TempDir(), "shop.sql")
	if err := os.WriteFile(path, []byte("-- old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{
			Mode: app.DumpWrite, Stage: app.DumpForm, Path: path, Target: "public",
			Options: dump.Options{Schema: "public", Content: dump.ContentAll},
		},
	}
	model.stepDump(connection, &connection.Overlay)
	answer := connection.Overlay.Answers.Answer
	if answer == nil {
		t.Fatal("the question asks nothing")
	}

	command := answer(true)
	if command == nil {
		t.Fatal("the answer started nothing")
	}
	answered := findMessage[dumpWrittenMsg](t, func() tea.Msg { return command() })
	if answered.Problem != "" {
		t.Fatalf("the answer wrote %+v", answered)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "-- old") {
		t.Errorf("the file was not written over:\n%s", written)
	}
}

// A dump makes the directory of the file it writes.
func TestTheDumpMakesTheDirectoryOfItsFile(t *testing.T) {
	model, connection, _ := buildDumpModel(t)
	path := filepath.Join(t.TempDir(), "backups", "day", "shop.sql")

	_, command := model.startDump(connection, path, "public", dump.Options{
		Schema: "public", Content: dump.ContentSchema,
	})
	answered := findMessage[dumpWrittenMsg](t, command)
	if answered.Problem != "" {
		t.Fatalf("the dump answered %q", answered.Problem)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the file was not written: %v", err)
	}
}

// A schema that holds nothing reports it instead of writing an empty file.
func TestADumpOfAnEmptySchemaReportsIt(t *testing.T) {
	model, connection, _ := buildDumpModel(t)
	path := filepath.Join(t.TempDir(), "shop.sql")

	_, command := model.startDump(connection, path, "empty", dump.Options{
		Schema: "empty", Content: dump.ContentAll,
	})
	answered := findMessage[dumpWrittenMsg](t, command)
	if answered.Problem == "" {
		t.Fatal("the dump of an empty schema wrote a file")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the dump left a file behind")
	}

	held, _ := model.Update(answered)
	if held != model {
		t.Fatal("the answer replaced the model")
	}
	if connection.Overlay.Kind != app.OverlayDump || connection.Overlay.Notice == "" {
		t.Errorf("the card holds no notice: %+v", connection.Overlay)
	}
}

// The card follows a run: the dump reports the tables and the rows it has written, and the
// card draws the share done.
func TestTheDumpReportsHowFarItHasCome(t *testing.T) {
	server := &dumpSession{
		offlineSession: buildOfflineModel(t, 80, 24).Active().Session.(*offlineSession),
		transaction:    db.TransactionNone,
	}
	held := []dump.Progress{}
	if _, err := dump.Write(context.Background(), server, dump.Options{
		Schema: "public", Content: dump.ContentAll,
		OnProgress: func(progress dump.Progress) { held = append(held, progress) },
	}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}

	if len(held) < 2 {
		t.Fatalf("reports: %v, want the start and the end", held)
	}
	last := held[len(held)-1]
	if last.Tables != 1 || last.OfTables != 1 || last.Rows != 1 {
		t.Fatalf("the last report is %+v, want one table and one row", last)
	}
}

func TestTheCardDrawsTheBarOfARun(t *testing.T) {
	model, connection, _ := buildDumpModel(t)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{
			Mode: app.DumpWrite, Stage: app.DumpForm, Path: "shop.sql", Target: "public",
			Running: true,
			Progress: app.Progress{
				Done: 3, Total: 12, Label: "tables", Detail: "orders · 1,200 rows",
			},
		},
	}

	drawn := model.View().Content
	for _, wanted := range []string{"▇", "░", "3 of 12 tables", "orders · 1,200 rows"} {
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the card holds no %q:\n%s", wanted, drawn)
		}
	}
}

// A run whose size is not known draws the count and no bar.
func TestACountedRunDrawsNoBar(t *testing.T) {
	model, connection, _ := buildDumpModel(t)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{
			Mode: app.DumpRestore, Stage: app.DumpForm, Path: "shop.sql", Running: true,
			Progress: app.Progress{Done: 7, Label: "statements"},
		},
	}

	drawn := model.View().Content
	if !strings.Contains(drawn, "7 statements") {
		t.Errorf("the card holds no count:\n%s", drawn)
	}
	if strings.Contains(drawn, "░") {
		t.Errorf("the card holds a bar of a size it cannot know:\n%s", drawn)
	}
}

// A report reaches the card it belongs to, and no other.
func TestAReportReachesItsOwnCard(t *testing.T) {
	model, connection, _ := buildDumpModel(t)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayDump,
		Dump: app.DumpRequest{Mode: app.DumpWrite, Stage: app.DumpForm, Running: true},
	}
	id := model.ActiveID()

	model.readProgress(progressMsg{
		ConnectionID: id, Kind: app.OverlayImport,
		Progress: app.Progress{Done: 5, Label: "rows"},
	})
	if connection.Overlay.Dump.Progress.IsStarted() {
		t.Error("a report of an import reached the dump card")
	}

	model.readProgress(progressMsg{
		ConnectionID: id, Kind: app.OverlayDump,
		Progress: app.Progress{Done: 2, Total: 4, Label: "tables"},
	})
	if connection.Overlay.Dump.Progress.Done != 2 {
		t.Errorf("progress: %+v, want the report", connection.Overlay.Dump.Progress)
	}
}
