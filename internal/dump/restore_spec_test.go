package dump_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/dump"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// fakeRunner keeps the statements it was given and fails on the one named.
type fakeRunner struct {
	ran      []string
	failOn   string
	dropsErr error
}

func (runner *fakeRunner) RunQuery(
	_ context.Context, sql string, _ int, _ []any,
) (db.QueryResult, error) {
	runner.ran = append(runner.ran, sql)
	if runner.failOn != "" && strings.Contains(sql, runner.failOn) {
		return db.QueryResult{}, runner.dropsErr
	}
	return db.QueryResult{}, nil
}

// walkStatements returns the statements of the text, in order.
func walkStatements(t *testing.T, text string) []string {
	t.Helper()
	held := []string{}
	err := dump.WalkStatements(strings.NewReader(text), syntax.FlavourStandard,
		func(sql string) error {
			held = append(held, sql)
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	return held
}

func TestWalkStatementsSplitsOnTopLevelSemicolons(t *testing.T) {
	statements := walkStatements(t, "-- masume dump\ncreate table t (id int);\n"+
		"insert into t (id) values (1);\ninsert into t (id) values (2);\n")
	wanted := []string{
		"-- masume dump\ncreate table t (id int)",
		"insert into t (id) values (1)",
		"insert into t (id) values (2)",
	}
	if len(statements) != len(wanted) {
		t.Fatalf("statements: %q, want %q", statements, wanted)
	}
	for at, sql := range wanted {
		if statements[at] != sql {
			t.Errorf("statement %d: %q, want %q", at, statements[at], sql)
		}
	}
}

func TestWalkStatementsKeepsSemicolonsInLiterals(t *testing.T) {
	statements := walkStatements(t,
		"insert into t (note) values ('one; two');\ninsert into t (note) values ('three');")
	if len(statements) != 2 {
		t.Fatalf("statements: %q, want 2", statements)
	}
	if !strings.Contains(statements[0], "'one; two'") {
		t.Errorf("statement 0: %q, want the whole literal", statements[0])
	}
}

func TestWalkStatementsTakesTheLastStatementWithoutSemicolon(t *testing.T) {
	statements := walkStatements(t, "select 1;\nselect 2")
	if len(statements) != 2 || statements[1] != "select 2" {
		t.Fatalf("statements: %q, want the last one as well", statements)
	}
}

func TestWalkStatementsReadsAFileLargerThanTheBuffer(t *testing.T) {
	long := strings.Repeat("x", 100*1024)
	statements := walkStatements(t,
		"insert into t (note) values ('"+long+"');\nselect 1;")
	if len(statements) != 2 {
		t.Fatalf("statements: %d, want 2", len(statements))
	}
	if !strings.Contains(statements[0], long) {
		t.Error("the long statement was cut")
	}
}

func TestWalkStatementsStopsAtACallbackError(t *testing.T) {
	refused := errors.New("stop here")
	err := dump.WalkStatements(strings.NewReader("select 1;select 2;"),
		syntax.FlavourStandard, func(string) error { return refused })
	if !errors.Is(err, refused) {
		t.Fatalf("error: %v, want %v", err, refused)
	}
}

func TestRunFileRunsEveryStatement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shop.sql")
	if err := os.WriteFile(path, []byte("create table t (id int);\ninsert into t values (1);\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	report, err := dump.RunFile(context.Background(), runner, path, syntax.FlavourStandard)
	if err != nil {
		t.Fatal(err)
	}
	if report.Statements != 2 || len(runner.ran) != 2 {
		t.Fatalf("report: %+v, ran: %q", report, runner.ran)
	}
}

func TestRunFileStopsAtTheFirstFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shop.sql")
	if err := os.WriteFile(path,
		[]byte("select 1;\nselect 2;\nselect 3;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("syntax error")
	runner := &fakeRunner{failOn: "select 2", dropsErr: refused}
	report, err := dump.RunFile(context.Background(), runner, path, syntax.FlavourStandard)
	if !errors.Is(err, refused) {
		t.Fatalf("error: %v, want %v", err, refused)
	}
	if report.Statements != 1 {
		t.Errorf("statements: %d, want the one that ran", report.Statements)
	}
	if len(runner.ran) != 2 {
		t.Errorf("ran: %q, want the run to stop at the second", runner.ran)
	}
}

func TestRunFileReportsAMissingFile(t *testing.T) {
	_, err := dump.RunFile(context.Background(), &fakeRunner{},
		filepath.Join(t.TempDir(), "gone.sql"), syntax.FlavourStandard)
	if err == nil {
		t.Fatal("a missing file was read")
	}
}

// A file written on another system holds carriage returns and can open with a byte order
// mark. Both reach the server as part of the text, and neither ends a statement early.
func TestWalkStatementsReadsAFileOfAnotherSystem(t *testing.T) {
	statements := walkStatements(t,
		"\ufeff-- masume dump\r\ncreate table t (id int);\r\ninsert into t values (1);\r\n")
	if len(statements) != 2 {
		t.Fatalf("statements: %q, want two", statements)
	}
	if !strings.Contains(statements[0], "create table t (id int)") {
		t.Errorf("statement 0: %q", statements[0])
	}
	if strings.TrimSpace(statements[1]) != "insert into t values (1)" {
		t.Errorf("statement 1: %q", statements[1])
	}
}

// A trigger body larger than one read of the file is still one statement.
func TestWalkStatementsReadsABlockLargerThanTheBuffer(t *testing.T) {
	body := strings.Repeat("insert into t values (1); ", 8000)
	statements := walkStatements(t,
		"create trigger big after insert on t begin "+body+" end;\nselect 1;")
	if len(statements) != 2 {
		t.Fatalf("statements: %d, want the trigger and the read", len(statements))
	}
	if !strings.HasPrefix(statements[0], "create trigger big") ||
		!strings.HasSuffix(statements[0], "end;") {
		t.Errorf("the trigger was cut: %.60q…%.20q", statements[0],
			statements[0][max(0, len(statements[0])-20):])
	}
	if statements[1] != "select 1" {
		t.Errorf("statement 1: %q", statements[1])
	}
}

// A run that is stopped ends at the statement it stands on.
func TestRunStopsWithItsContext(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	runner := &fakeRunner{failOn: "select 2", dropsErr: context.Canceled}
	stop()

	report, err := dump.Run(ctx, runner,
		strings.NewReader("select 1; select 2; select 3;"), syntax.FlavourStandard)
	if err == nil {
		t.Fatal("the stopped run answered no error")
	}
	if report.Statements > 1 {
		t.Errorf("statements: %d, want the run to stop", report.Statements)
	}
}
