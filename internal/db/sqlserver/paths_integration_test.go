//go:build integration

// The paths of the client that write their own SQL: the sort and the filter of the grid, a
// counted read, an import, a write plan with its undo, a transaction the user holds, and a
// statement the client stops. Each one reads a real SQL Server named by
// MASUME_TEST_SQLSERVER.
package sqlserver_test

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
	first, err := session.ReadPage(ctx, read, db.ReadWindow{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("the sorted read answered %v", err)
	}
	if held := readColumn(first, 2); len(held) != 2 || held[0] != "99.00" {
		t.Errorf("the first page holds the totals %v, wanted the largest first", held)
	}
	next, nextErr := session.ReadPage(ctx, read, db.ReadWindow{Limit: 2, Offset: 2})
	if nextErr != nil {
		t.Fatalf("the second page answered %v", nextErr)
	}
	if held := readColumn(next, 2); len(held) != 1 || held[0] != "0.00" {
		t.Errorf("the second page holds the totals %v, wanted the smallest", held)
	}
	// The read carries the sort into its display text, which the user sees.
	if !strings.Contains(read.Display, "order by [total] desc") {
		t.Errorf("the read reads as %q", read.Display)
	}
}

// A filter of the grid binds its values, and a count of the read counts the rows it matches.
func TestGridFiltersAReadAndCountsIt(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	orders := findTable(t, session, "orders")

	rewrite := core.ReadRewrite{Filter: []core.FilterStep{
		{Kind: core.FilterCompare, Column: "customer", Test: core.FilterEquals, Value: "ada"},
	}}
	read := session.Composer().ComposeRelationRead(orders, rewrite)
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

	// A step the user typed goes to the server as it was written.
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

// A read of the rows a foreign key points at filters on the key column.
func TestReadFollowsAForeignKey(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	lines := findTable(t, session, "order_lines")

	detail, err := session.DescribeTable(ctx, lines)
	if err != nil {
		t.Fatalf("the describe answered %v", err)
	}
	if len(detail.ForeignKeys) != 1 {
		t.Fatalf("the table holds %d foreign keys, wanted 1", len(detail.ForeignKeys))
	}
	key := detail.ForeignKeys[0]

	orders := findTable(t, session, "orders")
	read := session.Composer().ComposeRelationRead(orders, core.ReadRewrite{
		Filter: []core.FilterStep{{
			Kind: core.FilterCompare, Column: key.TargetColumns[0],
			Test: core.FilterEquals, Value: int64(1),
		}},
	})
	if _, pageErr := session.ReadPage(
		ctx, read, db.ReadWindow{Limit: 200}); pageErr != nil {
		t.Errorf("the read of the referenced rows answered %v", pageErr)
	}
}

// A write plan counts the rows the write matches, names the columns it assigns, names the
// trigger that runs, and keeps the rows for an undo.
func TestWritePlanMeasuresAWriteAndUndoesIt(t *testing.T) {
	session := openShopWithObjects(t)
	ctx := context.Background()
	tables, err := session.ListTables(ctx)
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}

	const written = "update dbo.orders set total = 1 where customer = N'ada'"
	plan, built := writeplan.Build(ctx, session, writeplan.Request{
		SQL: written, Tables: tables, Mode: cfg.PlanUndo, UndoRows: 100,
	})
	if !built {
		t.Fatal("the write was not measured")
	}
	if !plan.HasRows || plan.Rows != 1 {
		t.Errorf("the plan counts %d rows, has %v; wanted 1", plan.Rows, plan.HasRows)
	}
	if !plan.HasTotal || plan.Total != 3 {
		t.Errorf("the plan counts %d rows in the table, wanted 3", plan.Total)
	}
	if len(plan.Columns) != 1 || plan.Columns[0] != "total" {
		t.Errorf("the plan names the columns %v, wanted total", plan.Columns)
	}
	if len(plan.Cascades) != 1 ||
		!strings.Contains(plan.Cascades[0].Reason, "orders_touch") {
		t.Errorf("the plan names the effects %v, wanted the trigger", plan.Cascades)
	}
	if !plan.Undo.Kept {
		t.Fatalf("the plan keeps no undo: %s", plan.Undo.Reason)
	}

	result, undo, runErr := writeplan.RunWithUndo(ctx, session, plan.Undo,
		func(ctx context.Context) (db.QueryResult, error) {
			return session.RunQuery(ctx, written, dbtest.ReadEverything, nil)
		})
	if runErr != nil {
		t.Fatalf("the write answered %v", runErr)
	}
	if !result.HasAffected || result.Affected != 1 {
		t.Errorf("the write reports %d rows changed, has %v",
			result.Affected, result.HasAffected)
	}
	if undo.Rows != 1 || !undo.IsHeld() {
		t.Fatalf("the undo kept %d rows and holds %v, wanted one row", undo.Rows, undo.IsHeld())
	}

	// The undo writes the rows back as they were.
	if err := session.ApplyChanges(ctx, undo.Changes); err != nil {
		t.Fatalf("the undo answered %v", err)
	}
	after, afterErr := session.RunQuery(ctx,
		"select total from dbo.orders where customer = N'ada'", dbtest.ReadEverything, nil)
	if afterErr != nil {
		t.Fatalf("the read answered %v", afterErr)
	}
	if held := core.FormatCell(after.Rows[0][0], ""); held != "12.50" {
		t.Errorf("the undo left the total at %s, wanted 12.50", held)
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
	if len(sample.Columns) != 4 {
		t.Fatalf("the file reads %d columns, wanted 4", len(sample.Columns))
	}

	// An import into a table of its own maps every column of the file.
	plan := load.BuildPlan(path, options, sample,
		query.QualifiedName{Schema: "dbo", Name: "imported_people"}, nil)
	if problem := plan.FindPlanProblem(session.Dialect()); problem != "" {
		t.Fatalf("the plan reads as faulty: %s", problem)
	}
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(),
			"if object_id('dbo.imported_people', 'U') is not null "+
				"drop table dbo.imported_people", dbtest.ReadEverything, nil)
	})

	if _, err := session.RunQuery(ctx, load.BuildCreateTable(plan, session.Dialect()),
		dbtest.ReadEverything, nil); err != nil {
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
		"select name, age, paid from dbo.imported_people order by name",
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

// A transaction the user holds keeps its writes until a commit, and the client reports it as
// open the whole time.
func TestTransactionOfTheUserHoldsItsWrites(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction did not open: %v", err)
	}
	if _, err := session.RunQuery(ctx,
		"insert into dbo.orders (customer, total) values (N'lin', 5)",
		dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the write answered %v", err)
	}
	if held := session.ReadTransactionState(); held != db.TransactionOpen {
		t.Errorf("the state reads %q, wanted it open", held)
	}
	if err := session.CommitTransaction(ctx); err != nil {
		t.Fatalf("the commit answered %v", err)
	}
	if held := session.ReadTransactionState(); held != db.TransactionNone {
		t.Errorf("the state reads %q after the commit, wanted none", held)
	}

	answered, err := session.RunQuery(ctx,
		"select count_big(*) from dbo.orders where customer = N'lin'",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if held := db.ReadNonNegativeCount(answered.Rows[0][0]); held != 1 {
		t.Errorf("the commit left %d rows, wanted 1", held)
	}
}

// A statement the client stops answers an error, and the connection takes the next statement.
func TestStatementTimeoutStopsAStatement(t *testing.T) {
	profile, password := dbtest.BuildProfile(t, dbtest.Sqlserver)
	profile.StatementTimeout = time.Second

	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatalf("the connection answered %v", err)
	}
	defer func() { _ = session.Close() }()

	if _, waitErr := session.RunQuery(
		ctx, "waitfor delay '00:00:10'", dbtest.ReadEverything, nil); waitErr == nil {
		t.Fatal("a statement over the time limit answered no error")
	}
	answered, readErr := session.RunQuery(ctx, "select 1 as one", dbtest.ReadEverything, nil)
	if readErr != nil {
		t.Fatalf("the statement after the stopped one answered %v", readErr)
	}
	if len(answered.Rows) != 1 {
		t.Errorf("the read gave %d rows, wanted 1", len(answered.Rows))
	}
}
