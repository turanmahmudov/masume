// A functional test of the whole run: it opens a real SQLite file through the real adapter
// and reads what the run wrote. SQLite needs no server, so this runs in the ordinary suite.
package headless_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/headless"
)

// buildDatabase returns a profile on a fresh SQLite file that holds three orders.
func buildDatabase(t *testing.T, access cfg.AccessMode) cfg.Profile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	// The adapter refuses a path with no file, so the file is made before it opens.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("the database file cannot be made: %v", err)
	}

	profile := cfg.Profile{
		Name: "shop", Engine: core.EngineSqlite, Database: path,
		AccessMode: cfg.AccessWrite, PageSize: cfg.DefaultPageSize,
	}
	if code := runStatement(t, profile, `
		create table orders (id integer primary key, total_cents integer, status text);
		insert into orders (total_cents, status) values (4990, 'paid');
		insert into orders (total_cents, status) values (1200, 'paid');
		insert into orders (total_cents, status) values (99, 'cancelled');`,
	); code != headless.CodeOK {
		t.Fatalf("the schema was not laid out, and the run answered %d", code)
	}

	profile.AccessMode = access
	return profile
}

// runStatement runs one statement and drops what it wrote, for the setup of a test.
func runStatement(t *testing.T, profile cfg.Profile, sql string) int {
	t.Helper()
	code, _, _ := runOptions(t, headless.Options{Profile: profile, Statement: sql})
	return code
}

// runOptions runs and returns the exit code, what was written, and what was reported.
func runOptions(t *testing.T, options headless.Options) (int, string, string) {
	t.Helper()
	out, reported := &bytes.Buffer{}, &bytes.Buffer{}
	options.Out, options.Err = out, reported
	if options.Format == "" {
		options.Format = headless.FormatTable
	}
	code := headless.Run(context.Background(), engines.CreateAdapters(), options)
	return code, out.String(), reported.String()
}

// A run must write the rows of the statement and answer with the code for a run that
// worked, because a script reads both.
func TestRunWritesTheRowsOfTheStatement(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Statement: "select id, status from orders order by id",
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}

	lines := strings.Split(strings.TrimRight(written, "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("the run wrote %d lines, wanted a head, a rule and three rows: %q",
			len(lines), written)
	}
	if !strings.HasPrefix(lines[0], "id") || !strings.Contains(lines[0], "status") {
		t.Errorf("the first line is %q, wanted the column names", lines[0])
	}
	if strings.HasSuffix(lines[2], " ") {
		t.Errorf("the line %q ends in a space", lines[2])
	}
	if !strings.Contains(lines[4], "cancelled") {
		t.Errorf("the last row is %q, wanted the third order", lines[4])
	}
}

// Each format must be a valid document of that format, because a script pipes it into a
// reader of one.
func TestRunWritesEveryFormat(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	sql := "select id, status from orders order by id"

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Statement: sql, Format: headless.FormatCSV,
	})
	if code != headless.CodeOK {
		t.Fatalf("csv answered %d and said %q", code, reported)
	}
	if written != "id,status\n1,paid\n2,paid\n3,cancelled\n" {
		t.Errorf("csv wrote %q", written)
	}

	code, written, reported = runOptions(t, headless.Options{
		Profile: profile, Statement: sql, Format: headless.FormatJSON,
	})
	if code != headless.CodeOK {
		t.Fatalf("json answered %d and said %q", code, reported)
	}
	records := []map[string]any{}
	if err := json.Unmarshal([]byte(written), &records); err != nil {
		t.Fatalf("json wrote %q, which does not read back: %v", written, err)
	}
	if len(records) != 3 || records[0]["status"] != "paid" {
		t.Errorf("json wrote %v, wanted the three orders", records)
	}

	code, written, reported = runOptions(t, headless.Options{
		Profile: profile, Statement: sql, Format: headless.FormatMarkdown,
	})
	if code != headless.CodeOK {
		t.Fatalf("markdown answered %d and said %q", code, reported)
	}
	if !strings.HasPrefix(written, "| id | status |\n| --- | --- |\n") {
		t.Errorf("markdown wrote %q", written)
	}
}

// A result with no rows must still write the shape of the result, so a reader of the format
// finds an empty document and not a broken one.
func TestRunWritesAResultWithNoRows(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	sql := "select id from orders where id < 0"

	for _, one := range []struct {
		format  headless.Format
		written string
	}{
		{headless.FormatCSV, "id\n"},
		{headless.FormatJSON, "[\n]\n"},
	} {
		code, written, reported := runOptions(t, headless.Options{
			Profile: profile, Statement: sql, Format: one.format,
		})
		if code != headless.CodeOK {
			t.Errorf("%s answered %d and said %q", one.format, code, reported)
			continue
		}
		if written != one.written {
			t.Errorf("%s wrote %q, wanted %q", one.format, written, one.written)
		}
	}
}

// A statement with no result set must report what it changed, and on the error stream, so
// the output stream holds a valid document of its format.
func TestRunReportsWhatAStatementChanged(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV,
		Statement: "update orders set status = 'paid' where status = 'cancelled'",
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if written != "" {
		t.Errorf("the run wrote %q to the output, wanted nothing", written)
	}
	if !strings.Contains(reported, "1") {
		t.Errorf("the run said %q, wanted the number of changed rows", reported)
	}
}

// A named parameter must be bound and never written into the statement as text, so a value
// of a script cannot be read as SQL.
func TestRunBindsANamedParameter(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV,
		Statement: "select id from orders where status = :status order by id",
		Params:    map[string]any{"status": "paid"},
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if written != "id\n1\n2\n" {
		t.Errorf("the run wrote %q, wanted the two paid orders", written)
	}
}

// A parameter without a value must stop the run, because a statement that still holds a
// mark is not the statement the caller wrote.
func TestRunReportsAParameterWithoutAValue(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Statement: "select id from orders where status = :status",
	})
	if code != headless.CodeStatement {
		t.Fatalf("the run answered %d, wanted the code of a statement that was refused", code)
	}
	if written != "" {
		t.Errorf("the run wrote %q, wanted nothing", written)
	}
	if !strings.Contains(reported, "status") {
		t.Errorf("the run said %q, wanted the name of the parameter", reported)
	}
}

// A read-only profile must refuse a write in the client, with a code of its own, so a job
// that must not write cannot write by accident.
func TestRunRefusesAWriteOnAReadOnlyProfile(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessReadOnly)

	code, _, reported := runOptions(t, headless.Options{
		Profile: profile, Statement: "delete from orders",
	})
	if code != headless.CodeRefused {
		t.Fatalf("the run answered %d, wanted the code of a refusal", code)
	}
	if !strings.Contains(reported, "read-only") {
		t.Errorf("the run said %q, wanted the reason", reported)
	}

	// The rows are still there, so the statement never reached the server.
	code, written, _ := runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV,
		Statement: "select count(*) as n from orders",
	})
	if code != headless.CodeOK || written != "n\n3\n" {
		t.Errorf("the rows read %q after the refusal, wanted the three orders", written)
	}
}

// A read on a read-only profile must run, because that is what the mode is for.
func TestRunReadsOnAReadOnlyProfile(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessReadOnly)

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV,
		Statement: "select count(*) as n from orders",
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if written != "n\n3\n" {
		t.Errorf("the run wrote %q, wanted the count", written)
	}
}

// A statement the server refuses must answer with the code for a refused statement, so a
// job fails on a query it cannot run.
func TestRunReportsAStatementTheServerRefused(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, _, reported := runOptions(t, headless.Options{
		Profile: profile, Statement: "select * from nosuchtable",
	})
	if code != headless.CodeStatement {
		t.Fatalf("the run answered %d, wanted the code of a statement that was refused", code)
	}
	if !strings.Contains(reported, "nosuchtable") {
		t.Errorf("the run said %q, wanted what the server reported", reported)
	}
}

// A connection that cannot be opened must answer with a code of its own, so a job tells a
// server that is down from a query that is wrong.
func TestRunReportsAConnectionItCannotOpen(t *testing.T) {
	profile := cfg.Profile{
		Name: "gone", Engine: core.EngineSqlite,
		Database:   filepath.Join(t.TempDir(), "no", "such.db"),
		AccessMode: cfg.AccessWrite, PageSize: cfg.DefaultPageSize,
	}

	code, _, reported := runOptions(t, headless.Options{
		Profile: profile, Statement: "select 1",
	})
	if code != headless.CodeConnection {
		t.Fatalf("the run answered %d, wanted the code of a connection that failed", code)
	}
	if reported == "" {
		t.Error("the run said nothing about the connection it could not open")
	}
}

// A profile that only the user can answer for cannot be run without a screen, so it must be
// reported and not left waiting for a prompt that never comes.
func TestRunReportsAProfileThatNeedsAPrompt(t *testing.T) {
	profile := cfg.Profile{
		Name: "shop", Engine: core.EnginePostgres, Host: "db.internal", Port: 5432,
		Database: "shop", User: "ada", Auth: cfg.AuthPrompt, AccessMode: cfg.AccessWrite,
	}

	code, _, reported := runOptions(t, headless.Options{
		Profile: profile, Statement: "select 1",
	})
	if code != headless.CodeConnection {
		t.Fatalf("the run answered %d, wanted the code of a connection that failed", code)
	}
	if !strings.Contains(reported, "password") {
		t.Errorf("the run said %q, wanted the reason", reported)
	}
}

// A text with several statements must run them in order, so a report file works.
func TestRunRunsEveryStatementOfAText(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV,
		Statement: "select 1 as first; select status from orders where id = 3;",
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if written != "first\n1\nstatus\ncancelled\n" {
		t.Errorf("the run wrote %q, wanted one result after the other", written)
	}
}

// A statement that fails must stop the text, so a later statement never runs on a state the
// failed one was supposed to make.
func TestAFailedStatementStopsTheText(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, _ := runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV,
		Statement: "select 1 as first; select * from nosuchtable; select 2 as third;",
	})
	if code != headless.CodeStatement {
		t.Fatalf("the run answered %d, wanted the code of a statement that was refused", code)
	}
	if strings.Contains(written, "third") {
		t.Errorf("the run wrote %q, so it went on after the statement that failed", written)
	}
}

// The plan must be written as JSON, so a job reads a number out of it instead of parsing
// the text of a server.
func TestRunWritesThePlanAsJSON(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Explain: true,
		Statement: "select id from orders where status = :status",
		Params:    map[string]any{"status": "paid"},
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}

	plan := struct {
		Analyzed bool
		Summary  string
		Nodes    []map[string]any
	}{}
	if err := json.Unmarshal([]byte(written), &plan); err != nil {
		t.Fatalf("the plan wrote %q, which does not read back: %v", written, err)
	}
	if len(plan.Nodes) == 0 {
		t.Fatalf("the plan holds no node: %q", written)
	}
	if plan.Nodes[0]["label"] == "" {
		t.Errorf("the first node is %v, wanted one that names what the server does",
			plan.Nodes[0])
	}
	// SQLite plans a statement but measures nothing, so the run must not ask it to.
	if plan.Analyzed {
		t.Error("the plan was measured on a server that measures no plan")
	}
}

// A file with nothing in it must be reported, so an empty file does not answer as a run
// that worked.
func TestRunReportsATextWithNoStatement(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	for _, written := range []string{"", "   ", "\n\n"} {
		code, _, reported := runOptions(t, headless.Options{
			Profile: profile, Statement: written,
		})
		if code != headless.CodeStatement {
			t.Errorf("%q answered %d, wanted the code of a statement that was refused",
				written, code)
			continue
		}
		if reported == "" {
			t.Errorf("%q was refused without a reason", written)
		}
	}
}

// A statement that sets no limit of its own is read one page at a time, the same as in the
// client: a `select *` of a whole relation is not a request for every row of it.
func TestRunReadsOnePageWhereTheStatementSetsNoLimit(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	profile.PageSize = 2

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV,
		Statement: "select id from orders order by id",
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if written != "id\n1\n2\n" {
		t.Errorf("the run wrote %q, wanted one page of two rows", written)
	}
	if !strings.Contains(reported, "limit") {
		t.Errorf("the run said %q, wanted what reads more rows", reported)
	}

	// A page that holds the whole result reports nothing about a longer one.
	profile.PageSize = 3
	code, written, reported = runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV,
		Statement: "select id from orders order by id",
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if written != "id\n1\n2\n3\n" {
		t.Errorf("the run wrote %q, wanted the three orders", written)
	}
	if reported != "" {
		t.Errorf("the run said %q about a result that fits in one page", reported)
	}
}

// A statement that bounds its own result already holds how many rows it wants, so every one
// of them must be read whatever the page of the profile is.
func TestRunKeepsALimitTheStatementSets(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	profile.PageSize = 1

	for _, sql := range []string{
		"select id from orders order by id limit 2",
		"select id from orders order by id LIMIT 2",
		"select id from orders order by id limit 2 offset 0",
	} {
		code, written, reported := runOptions(t, headless.Options{
			Profile: profile, Format: headless.FormatCSV, Statement: sql,
		})
		if code != headless.CodeOK {
			t.Errorf("%q answered %d and said %q", sql, code, reported)
			continue
		}
		if written != "id\n1\n2\n" {
			t.Errorf("%q wrote %q, wanted the two rows it asked for", sql, written)
		}
		if reported != "" {
			t.Errorf("%q said %q about a result it read whole", sql, reported)
		}
	}
}

// Every format must read the whole of a bounded result, not only the formats that stream.
func TestRunKeepsALimitInEveryFormat(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	profile.PageSize = 1

	for _, format := range []headless.Format{
		headless.FormatCSV, headless.FormatJSON,
		headless.FormatTable, headless.FormatMarkdown,
	} {
		code, written, reported := runOptions(t, headless.Options{
			Profile: profile, Format: format,
			Statement: "select id from orders order by id limit 3",
		})
		if code != headless.CodeOK {
			t.Errorf("%s answered %d and said %q", format, code, reported)
			continue
		}
		if reported != "" {
			t.Errorf("%s said %q about a result it read whole", format, reported)
		}
		for _, id := range []string{"1", "2", "3"} {
			if !strings.Contains(written, id) {
				t.Errorf("%s wrote %q, which is missing row %s", format, written, id)
			}
		}
	}
}

// A caller that asks for a number of rows is answered with that number, and told there are
// more where the result is longer.
func TestRunReadsTheRowsItWasAskedFor(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV, RowLimit: 2,
		Statement: "select id from orders order by id",
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if written != "id\n1\n2\n" {
		t.Errorf("the run wrote %q, wanted the two rows it was asked for", written)
	}
	if !strings.Contains(reported, "asked for") {
		t.Errorf("the run said %q, wanted that the result is longer", reported)
	}

	// A limit above the result reads it whole and reports nothing.
	code, written, reported = runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV, RowLimit: 100,
		Statement: "select id from orders order by id",
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if written != "id\n1\n2\n3\n" {
		t.Errorf("the run wrote %q, wanted the three orders", written)
	}
	if reported != "" {
		t.Errorf("the run said %q about a result the limit holds", reported)
	}
}

// A profile without a page size still reads, because a read with a batch of zero returns
// nothing.
func TestRunReadsWithTheDefaultPageWhereTheProfileSetsNone(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	profile.PageSize = 0

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatCSV,
		Statement: "select count(*) as n from orders",
	})
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if written != "n\n3\n" {
		t.Errorf("the run wrote %q, wanted the count", written)
	}
}

// The format names are what the command line reads, so each one must parse and an unknown
// one must not fall back to another.
func TestFindFormatReadsEveryName(t *testing.T) {
	for _, held := range headless.Formats {
		found, known := headless.FindFormat(strings.ToUpper(string(held)))
		if !known || found != held {
			t.Errorf("%q reads as %q, %v", held, found, known)
		}
	}
	if _, known := headless.FindFormat("parquet"); known {
		t.Error("a format there is not was read as one there is")
	}
	if !strings.Contains(headless.FormatNames(), "markdown") {
		t.Errorf("the format names read %q", headless.FormatNames())
	}
}

// JSON holds one document, so a run of several statements is refused rather than writing
// one array after another that no reader parses.
func TestRunRefusesSeveralStatementsAsJSON(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runOptions(t, headless.Options{
		Profile: profile, Format: headless.FormatJSON,
		Statement: "select id from orders; select id from orders",
	})
	if code != headless.CodeStatement {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if written != "" {
		t.Errorf("the run wrote %q, wanted nothing", written)
	}
	if !strings.Contains(reported, "json holds one result") {
		t.Errorf("the run said %q, wanted why JSON takes one statement", reported)
	}
}
