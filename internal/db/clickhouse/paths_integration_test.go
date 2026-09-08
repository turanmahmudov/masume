//go:build integration

// The paths of the client that write their own SQL: the sort and the filter of the grid, a
// counted read, an import, and a statement the client stops. Each one reads a real
// ClickHouse named by MASUME_TEST_CLICKHOUSE.
package clickhouse_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/load"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// readColumn returns one column of every row of a result, as text.
func readColumn(result db.QueryResult, at int) []string {
	written := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		written = append(written, core.FormatCell(row[at], ""))
	}
	return written
}

// The grid sorts on the server, and the pages of a sorted read follow one another.
func TestGridSortsAReadAndPagesItInOrder(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	orders := findTable(t, session, "orders")

	read := session.Composer().ComposeRelationRead(orders, core.ReadRewrite{
		Sort: []core.SortState{{Column: "total", Direction: core.SortDescending}},
	})
	first, err := session.ReadPage(ctx, read, db.ReadWindow{Limit: 2})
	if err != nil {
		t.Fatalf("the sorted read answered %v", err)
	}
	if held := readColumn(first, 2); len(held) != 2 || held[0] != "99" {
		t.Errorf("the first page holds the totals %v, wanted the largest first", held)
	}
	next, nextErr := session.ReadPage(ctx, read, db.ReadWindow{Limit: 2, Offset: 2})
	if nextErr != nil {
		t.Fatalf("the second page answered %v", nextErr)
	}
	if held := readColumn(next, 2); len(held) != 1 || held[0] != "0" {
		t.Errorf("the second page holds the totals %v, wanted the smallest", held)
	}
	if !strings.Contains(read.Display, "order by `total` desc") {
		t.Errorf("the read reads as %q", read.Display)
	}
}

// A filter of the grid binds its values, and a count of the read counts the rows it matches.
func TestGridFiltersAReadAndCountsIt(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	orders := findTable(t, session, "orders")

	read := session.Composer().ComposeRelationRead(orders, core.ReadRewrite{
		Filter: []core.FilterStep{{
			Kind: core.FilterCompare, Column: "customer",
			Test: core.FilterEquals, Value: "ada",
		}},
	})
	page, err := session.ReadPage(ctx, read, db.ReadWindow{Limit: 200})
	if err != nil {
		t.Fatalf("the filtered read answered %v", err)
	}
	if len(page.Rows) != 1 {
		t.Fatalf("the filter matched %d rows, wanted 1", len(page.Rows))
	}
	counted, holds, countErr := session.CountRead(ctx, read)
	if countErr != nil {
		t.Fatalf("the count answered %v", countErr)
	}
	if !holds || counted != 1 {
		t.Errorf("the count reads %d, has %v; wanted 1", counted, holds)
	}

	raw, built := core.BuildRawFilter("total > 50")
	if !built {
		t.Fatal("the typed filter was not built")
	}
	typed := session.Composer().ComposeRelationRead(orders, core.ReadRewrite{
		Filter: []core.FilterStep{raw},
	})
	answered, typedErr := session.ReadPage(ctx, typed, db.ReadWindow{Limit: 200})
	if typedErr != nil {
		t.Fatalf("the typed filter answered %v", typedErr)
	}
	if len(answered.Rows) != 1 {
		t.Errorf("the typed filter matched %d rows, wanted 1", len(answered.Rows))
	}
}

// An import writes a table of its own and fills it from a file.
func TestImportWritesATableAndFillsIt(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	path := filepath.Join(t.TempDir(), "people.csv")
	const rows = "name,age,paid,made_at\n" +
		"ada,36,true,2026-01-02T03:04:05Z\n" +
		"grace,45,false,2026-02-03T04:05:06Z\n"
	if err := os.WriteFile(path, []byte(rows), 0o600); err != nil {
		t.Fatalf("the file was not written: %v", err)
	}

	options := load.DefaultReadOptions()
	sample, sampleErr := load.ReadSample(path, options)
	if sampleErr != nil {
		t.Fatalf("the file did not read: %v", sampleErr)
	}
	plan := load.BuildPlan(path, options, sample,
		query.QualifiedName{Schema: testSchema, Name: "imported_people"}, nil)
	if problem := plan.FindPlanProblem(session.Dialect()); problem != "" {
		t.Fatalf("the plan reads as faulty: %s", problem)
	}
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(),
			"drop table if exists masume_test.imported_people", dbtest.ReadEverything, nil)
	})

	// A new table of this server needs an engine, which the dialect writes.
	created := load.BuildCreateTable(plan, session.Dialect())
	if !strings.Contains(created, "engine = MergeTree") {
		t.Fatalf("the table of the import names no engine: %s", created)
	}
	if _, err := session.RunQuery(
		ctx, created, dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the table was not created: %v", err)
	}
	values, rowErr := load.BuildRows(plan, sample.Rows)
	if rowErr != nil {
		t.Fatalf("the rows were not built: %v", rowErr)
	}
	insert, insertErr := load.BuildInsert(plan, values, session.Dialect())
	if insertErr != nil {
		t.Fatalf("the insert was not built: %v", insertErr)
	}
	if _, err := session.RunQuery(
		ctx, insert.SQL, dbtest.ReadEverything, insert.Params); err != nil {
		t.Fatalf("the import answered %v", err)
	}

	answered, readErr := session.RunQuery(ctx,
		"select name, age, paid from masume_test.imported_people order by name",
		dbtest.ReadEverything, nil)
	if readErr != nil {
		t.Fatalf("the read answered %v", readErr)
	}
	if len(answered.Rows) != 2 {
		t.Fatalf("the import wrote %d rows, wanted 2", len(answered.Rows))
	}
	if held := core.FormatCell(answered.Rows[0][0], ""); held != "ada" {
		t.Errorf("the first row reads %q", held)
	}
}

// A write of this server is a mutation of its own shape, which the write plan does not
// read, so the client offers no plan rather than a wrong one.
func TestWritePlanIsNotOffered(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	tables, err := session.ListTables(ctx)
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}

	if session.Capabilities().PlansWrites {
		t.Error("the engine reads as one that measures a write")
	}
	if _, built := writeplan.Build(ctx, session, writeplan.Request{
		SQL:    "alter table masume_test.orders update total = 1 where customer = 'ada'",
		Tables: tables, Mode: cfg.PlanUndo, UndoRows: 100,
	}); built {
		t.Error("a write was measured on a server that plans none")
	}
}

// A statement the client stops answers an error, and the connection takes the next
// statement. The statement after the stopped one carries the time limit of the profile as
// well, so the limit is wide enough for a machine under load.
func TestStatementTimeoutStopsAStatement(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Clickhouse)
	profile.StatementTimeout = 3 * time.Second

	ctx, stop := context.WithTimeout(context.Background(), time.Minute)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatalf("the connection answered %v", err)
	}
	defer func() { _ = session.Close() }()

	// The statement sleeps a second per row and would run for ten, so the client stops it
	// after three. A block of one row keeps every sleep inside the limit of the server.
	startedAt := time.Now()
	_, waitErr := session.RunQuery(ctx,
		"select sleepEachRow(1) from numbers(10) settings max_block_size = 1",
		dbtest.ReadEverything, nil)
	stopped := time.Since(startedAt)
	if waitErr == nil {
		t.Fatal("a statement over the time limit answered no error")
	}
	// The statement must have run into the limit, and not have been refused at once.
	if stopped < profile.StatementTimeout/2 || stopped > 3*profile.StatementTimeout {
		t.Errorf("the statement was stopped after %s and answered %v; wanted about %s",
			stopped.Round(time.Millisecond), waitErr, profile.StatementTimeout)
	}

	startedAt = time.Now()
	answered, readErr := session.RunQuery(ctx, "select 1 as one", dbtest.ReadEverything, nil)
	if readErr != nil {
		t.Fatalf("the statement after the stopped one answered %v after %s",
			readErr, time.Since(startedAt).Round(time.Millisecond))
	}
	if len(answered.Rows) != 1 {
		t.Errorf("the read gave %d rows, wanted 1", len(answered.Rows))
	}
}
