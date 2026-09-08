//go:build integration

// An integration test: it reads a real SQL Server. The server is started outside this code
// and named through MASUME_TEST_SQLSERVER. Nothing here knows how it was started.
package sqlserver_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/query"
)

// The relation every test here reads. Each statement runs on its own, because the server
// takes a CREATE of a view or a routine as the first statement of a batch only.
var shopSchema = []string{
	`if object_id('dbo.order_lines', 'U') is not null drop table dbo.order_lines`,
	`if object_id('dbo.orders', 'U') is not null drop table dbo.orders`,
	`create table dbo.orders (
	   id       bigint identity(1,1) primary key,
	   customer nvarchar(64) not null,
	   total    decimal(10,2) default 0
	     constraint orders_total_ck check (total >= 0),
	   paid_at  datetime2,
	   loud     as upper(customer)
	 )`,
	`create index orders_customer_idx on dbo.orders (customer)`,
	`insert into dbo.orders (customer, total)
	   values (N'ada', 12.50), (N'grace', 0), (N'alan', 99.00)`,
	`create table dbo.order_lines (
	   id       bigint identity(1,1) primary key,
	   order_id bigint not null
	     constraint order_lines_order_fk references dbo.orders (id) on delete cascade
	 )`,
}

// The objects beside the relations. Each CREATE opens a batch of its own.
var shopObjects = []string{
	`if object_id('dbo.order_notes', 'V') is not null drop view dbo.order_notes`,
	`if object_id('dbo.orders_touch', 'TR') is not null drop trigger dbo.orders_touch`,
	`if object_id('dbo.double_it', 'FN') is not null drop function dbo.double_it`,
	`if object_id('dbo.order_seq', 'SO') is not null drop sequence dbo.order_seq`,
	`create view dbo.order_notes as select customer, total from dbo.orders`,
	`create trigger dbo.orders_touch on dbo.orders after insert, update
	   as begin set nocount on end`,
	`create function dbo.double_it(@n int) returns int as begin return @n * 2 end`,
	`create sequence dbo.order_seq start with 1 increment by 1`,
}

func openShop(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Sqlserver)
	dbtest.RunStatements(t, session, shopSchema...)
	t.Cleanup(func() {
		for _, written := range shopSchema[:2] {
			_, _ = session.RunQuery(
				context.Background(), written, dbtest.ReadEverything, nil)
		}
	})
	return session
}

// findTable returns the relation of that name out of the catalog.
func findTable(t *testing.T, session db.Session, name string) db.TableRef {
	t.Helper()
	tables, err := session.ListTables(context.Background())
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	for _, table := range tables {
		if table.Schema == "dbo" && table.Name == name {
			return table
		}
	}
	t.Fatalf("%s was not listed; the server answered %d relations", name, len(tables))
	return db.TableRef{}
}

// openShopWithObjects lays out the relation and the objects beside it.
func openShopWithObjects(t *testing.T) db.Session {
	session := openShop(t)
	dbtest.RunStatements(t, session, shopObjects...)
	t.Cleanup(func() {
		for _, written := range shopObjects[:4] {
			_, _ = session.RunQuery(
				context.Background(), written, dbtest.ReadEverything, nil)
		}
	})
	return session
}

func TestServerRunsAReadAndAnswersItsColumns(t *testing.T) {
	session := openShop(t)

	answered, err := session.RunQuery(context.Background(),
		"select customer, total from dbo.orders order by customer",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if len(answered.Rows) != 3 {
		t.Fatalf("the read gave %d rows, wanted 3", len(answered.Rows))
	}
	if answered.Columns[0].Name != "customer" {
		t.Errorf("the first column is %q, wanted customer", answered.Columns[0].Name)
	}
	if answered.Columns[1].DataType == "" {
		t.Error("the total carries no type name")
	}
}

// The server takes a page with OFFSET and FETCH, which it accepts after a sort only.
func TestServerReadsOnePageOfARelation(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	orders := findTable(t, session, "orders")

	read := session.Composer().ComposeRelationRead(orders, core.ReadRewrite{})
	page, err := session.ReadPage(ctx, read, db.ReadWindow{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("the first page answered %v", err)
	}
	if len(page.Rows) != 2 || !page.Truncated {
		t.Errorf("the first page gave %d rows, truncated %v; wanted 2 and true",
			len(page.Rows), page.Truncated)
	}

	rest, restErr := session.ReadPage(ctx, read, db.ReadWindow{Limit: 2, Offset: 2})
	if restErr != nil {
		t.Fatalf("the second page answered %v", restErr)
	}
	if len(rest.Rows) != 1 || rest.Truncated {
		t.Errorf("the second page gave %d rows, truncated %v; wanted 1 and false",
			len(rest.Rows), rest.Truncated)
	}

	counted, holds, countErr := session.CountRead(ctx, read)
	if countErr != nil {
		t.Fatalf("the count answered %v", countErr)
	}
	if !holds || counted != 3 {
		t.Errorf("the count reads %d, has %v; wanted 3", counted, holds)
	}
}

// A statement the server refuses inside a transaction leaves the transaction open, and the
// next statement of it still runs.
func TestServerKeepsATransactionAfterAStatementItRefused(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction did not open: %v", err)
	}
	defer func() { _ = session.RollbackTransaction(ctx) }()

	if _, err := session.RunQuery(
		ctx, "select * from dbo.nothing_here", dbtest.ReadEverything, nil); err == nil {
		t.Fatal("a read of a table that is not there answered no error")
	}
	if held := session.ReadTransactionState(); held != db.TransactionOpen {
		t.Errorf("the state reads %q, wanted it still open", held)
	}
	if _, err := session.RunQuery(
		ctx, "select count_big(*) from dbo.orders", dbtest.ReadEverything, nil); err != nil {
		t.Errorf("a read after the refused one answered %v", err)
	}
}

func TestServerRollsBackATransaction(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction did not open: %v", err)
	}
	if _, err := session.RunQuery(
		ctx, "delete from dbo.orders", dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the delete answered %v", err)
	}
	if err := session.RollbackTransaction(ctx); err != nil {
		t.Fatalf("the rollback answered %v", err)
	}

	answered, err := session.RunQuery(
		ctx, "select count_big(*) from dbo.orders", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if held := db.ReadNonNegativeCount(answered.Rows[0][0]); held != 3 {
		t.Errorf("the rollback left %d rows, wanted the 3 it started with", held)
	}
}

func TestServerListsAndDescribesATable(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	orders := findTable(t, session, "orders")

	detail, err := session.DescribeTable(ctx, orders)
	if err != nil {
		t.Fatalf("the describe answered %v", err)
	}
	byName := map[string]db.ColumnDetail{}
	for _, column := range detail.Columns {
		byName[column.Name] = column
	}
	if !byName["id"].IsPrimaryKey {
		t.Error("id does not read as the primary key")
	}
	if !byName["id"].IsGenerated {
		t.Error("an identity column does not read as one the server fills")
	}
	if !byName["loud"].IsGenerated {
		t.Error("a computed column does not read as one the server fills")
	}
	if byName["customer"].Nullable {
		t.Error("customer is declared not null and reads as nullable")
	}
	if !byName["paid_at"].Nullable {
		t.Error("paid_at takes a null and reads as not null")
	}
	if byName["customer"].DataType != "nvarchar(64)" {
		t.Errorf("customer reads as %q, wanted nvarchar(64)", byName["customer"].DataType)
	}
	if !byName["total"].HasDefault {
		t.Error("total has a default and reads as having none")
	}

	indexes, indexErr := session.ListIndexes(ctx, orders)
	if indexErr != nil {
		t.Fatalf("the indexes answered %v", indexErr)
	}
	found := false
	for _, index := range indexes {
		if index.Name == "orders_customer_idx" {
			found = true
			if index.IsPrimary || index.IsUnique {
				t.Errorf("the index reads as primary %v and unique %v, wanted neither",
					index.IsPrimary, index.IsUnique)
			}
		}
	}
	if !found {
		t.Errorf("the index was not listed; the server answered %d of them", len(indexes))
	}
	// The key of the table is the one index the definition of the table carries itself.
	primaries := 0
	for _, index := range indexes {
		if index.IsPrimary {
			primaries++
			if !index.IsUnique {
				t.Error("the primary index reads as not unique")
			}
			if !strings.HasPrefix(index.Definition, "primary key (") {
				t.Errorf("the primary index reads as %q", index.Definition)
			}
		}
	}
	if primaries != 1 {
		t.Errorf("the table answers %d primary indexes, wanted 1", primaries)
	}

	constraints, constraintErr := session.ListConstraints(ctx, orders)
	if constraintErr != nil {
		t.Fatalf("the constraints answered %v", constraintErr)
	}
	byKind := map[db.ConstraintKind]db.ConstraintDetail{}
	for _, constraint := range constraints {
		byKind[constraint.Kind] = constraint
	}
	if _, held := byKind[db.ConstraintPrimaryKey]; !held {
		t.Error("the primary key was not listed as a constraint")
	}
	check, held := byKind[db.ConstraintCheck]
	if !held {
		t.Fatal("the check constraint was not listed")
	}
	// A definition of a constraint carries the word the statement of it opens with.
	if !strings.HasPrefix(check.Definition, "check (") {
		t.Errorf("the check reads as %q", check.Definition)
	}
}

// The server keeps no CREATE statement of a table, so the client builds one.
func TestServerAnswersTheCreateStatementOfATable(t *testing.T) {
	session := openShop(t)
	orders := findTable(t, session, "orders")

	lines, err := session.BuildTableDDL(context.Background(), orders)
	if err != nil {
		t.Fatalf("the definition answered %v", err)
	}
	written := strings.ToLower(strings.Join(lines, "\n"))
	if !strings.Contains(written, "create table") {
		t.Errorf("the definition does not hold a create statement:\n%s", written)
	}
	if !strings.Contains(written, "[customer] nvarchar(64) not null") {
		t.Errorf("the definition does not hold the customer column:\n%s", written)
	}
}

func TestServerAnswersAStatementItRefuses(t *testing.T) {
	session := openShop(t)

	_, err := session.RunQuery(context.Background(),
		"insert into dbo.orders (customer) values (null)", dbtest.ReadEverything, nil)
	if err == nil {
		t.Fatal("a null in a not-null column answered no error")
	}
	if !errors.Is(err, db.ErrDatabase) {
		t.Error("the error does not read as one from the database")
	}
	if described := db.DescribeError(err); described == "" {
		t.Error("the error is described as an empty text")
	}
}

// A buffer answers with the result of its last statement. A write after a read reports the
// rows it changed rather than the rows the read gave.
func TestServerNamesABufferAfterItsLastStatement(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	read, err := session.RunQuery(ctx,
		"insert into dbo.orders (customer, total) values (N'lin', 1); "+
			"select customer from dbo.orders order by customer",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the buffer answered %v", err)
	}
	if read.Command != "SELECT" {
		t.Errorf("the buffer is named %q, wanted SELECT", read.Command)
	}
	if read.HasAffected {
		t.Errorf("a buffer that ends in a read reported %d rows changed", read.Affected)
	}
	if len(read.Rows) != 4 {
		t.Errorf("the buffer gave %d rows, wanted 4", len(read.Rows))
	}

	written, writeErr := session.RunQuery(ctx,
		"select customer from dbo.orders; "+
			"update dbo.orders set total = total + 1 where customer = N'ada'",
		dbtest.ReadEverything, nil)
	if writeErr != nil {
		t.Fatalf("the buffer answered %v", writeErr)
	}
	if written.Command != "UPDATE" {
		t.Errorf("the buffer is named %q, wanted UPDATE", written.Command)
	}
	if !written.HasAffected || written.Affected != 1 {
		t.Errorf("the buffer reports %d rows changed, has %v",
			written.Affected, written.HasAffected)
	}
	if len(written.Rows) != 0 {
		t.Errorf("a buffer that ends in a write gave %d rows", len(written.Rows))
	}
}

// A write with an OUTPUT clause answers with rows, so it is read and not counted.
func TestServerAnswersTheRowsOfAWriteWithAnOutputClause(t *testing.T) {
	session := openShop(t)

	answered, err := session.RunQuery(context.Background(),
		"insert into dbo.orders (customer, total) output inserted.id, inserted.customer "+
			"values (N'mary', 3)", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the write answered %v", err)
	}
	if len(answered.Rows) != 1 {
		t.Fatalf("the write gave %d rows, wanted the one it wrote", len(answered.Rows))
	}
	if len(answered.Columns) != 2 {
		t.Errorf("the write named %d columns, wanted 2", len(answered.Columns))
	}
}

func TestServerPlansAStatementAndMeasuresIt(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	estimated, err := session.ExplainQuery(ctx,
		"select customer from dbo.orders where total > 1", false)
	if err != nil {
		t.Fatalf("the plan answered %v", err)
	}
	if estimated.Root.Label == "" {
		t.Error("the plan names no step")
	}
	if estimated.Root.Children == nil {
		t.Error("the plan holds no step under its root")
	}

	measured, measureErr := session.ExplainQuery(ctx,
		"select customer from dbo.orders where total > 1", true)
	if measureErr != nil {
		t.Fatalf("the measured plan answered %v", measureErr)
	}
	if !measured.Analyzed {
		t.Error("the measured plan does not read as measured")
	}
	if !measured.Root.HasActualRows {
		t.Error("the measured plan counts no rows")
	}

	// The plan setting must be off again, or every later read would answer with a plan.
	after, afterErr := session.RunQuery(ctx,
		"select customer from dbo.orders", dbtest.ReadEverything, nil)
	if afterErr != nil {
		t.Fatalf("the read after the plan answered %v", afterErr)
	}
	if len(after.Columns) != 1 || after.Columns[0].Name != "customer" {
		t.Errorf("the read after the plan answered %d columns of the plan", len(after.Columns))
	}
}

func TestServerNamesTheFaultOfAStatement(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	problem, faulty := session.CheckStatement(ctx, "select * from dbo.nothing_here")
	if !faulty {
		t.Fatal("a read of a table that is not there was checked as good")
	}
	if !strings.Contains(problem.Message, "nothing_here") {
		t.Errorf("the fault reads %q and does not name the table", problem.Message)
	}
	if _, held := session.CheckStatement(ctx, "select customer from dbo.orders"); held {
		t.Error("a statement the server accepts was checked as faulty")
	}
	// A statement the user has not finished is not a fault.
	if _, held := session.CheckStatement(ctx, "select customer from"); held {
		t.Error("an unfinished statement was checked as faulty")
	}
}

func TestServerReportsItsSessionsAndItsLoad(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	if _, err := session.ListActivity(ctx); err != nil {
		t.Errorf("the activity answered %v", err)
	}
	if _, err := session.ListLockWaits(ctx); err != nil {
		t.Errorf("the lock waits answered %v", err)
	}
	load, err := session.ReadServerLoad(ctx)
	if err != nil {
		t.Fatalf("the load answered %v", err)
	}
	if load.MaxConnections <= 0 {
		t.Errorf("the server reports a connection limit of %d", load.MaxConnections)
	}
	if load.StartedAt.IsZero() {
		t.Error("the server reports no start time")
	}
	if session.Capabilities().ReportsStatementStats {
		if _, statErr := session.ListSlowStatements(ctx, 5); statErr != nil {
			t.Errorf("the slow statements answered %v", statErr)
		}
	}
}

func TestServerStreamsAResultInBatches(t *testing.T) {
	session := openShop(t)

	batches := 0
	rows := 0
	counted, err := session.StreamQuery(context.Background(),
		"select customer from dbo.orders", nil, 2,
		func(batch [][]any, columns []db.ResultColumn) error {
			batches++
			rows += len(batch)
			if len(columns) != 1 {
				t.Errorf("a batch named %d columns, wanted 1", len(columns))
			}
			return nil
		})
	if err != nil {
		t.Fatalf("the stream answered %v", err)
	}
	if counted != 3 || rows != 3 {
		t.Errorf("the stream read %d rows and handed over %d, wanted 3", counted, rows)
	}
	if batches != 2 {
		t.Errorf("the stream handed over %d batches, wanted 2", batches)
	}
}

func TestServerAppliesStagedChanges(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	orders := findTable(t, session, "orders")

	read, err := session.RunQuery(ctx,
		"select id, customer from dbo.orders where customer = N'ada'",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	pending := core.NewPendingChanges()
	pending.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1,
		Value: core.CellValue{Kind: core.CellText, Text: "ida"},
	}
	changes, buildErr := session.Composer().BuildChanges(db.ChangeTarget{
		Table: orders, Columns: read.Columns, Rows: read.Rows, KeyColumns: []string{"id"},
	}, pending)
	if buildErr != nil {
		t.Fatalf("the change was not built: %v", buildErr)
	}
	if err := session.ApplyChanges(ctx, changes); err != nil {
		t.Fatalf("the change answered %v", err)
	}

	after, afterErr := session.RunQuery(ctx,
		"select customer from dbo.orders where customer = N'ida'", dbtest.ReadEverything, nil)
	if afterErr != nil {
		t.Fatalf("the read answered %v", afterErr)
	}
	if len(after.Rows) != 1 {
		t.Errorf("the change left %d rows named ida, wanted 1", len(after.Rows))
	}
}

// The tree draws a view beside the tables, and a routine, a trigger and a sequence under
// the schema that holds them.
func TestServerListsTheObjectsOfASchema(t *testing.T) {
	session := openShopWithObjects(t)
	ctx := context.Background()

	tables, err := session.ListTables(ctx)
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	views := 0
	for _, table := range tables {
		if table.Schema == "dbo" && table.Name == "order_notes" &&
			table.Kind == db.RelationView {
			views++
		}
	}
	if views != 1 {
		t.Errorf("the view was not listed as a view")
	}

	objects, objectErr := session.ListSchemaObjects(ctx)
	if objectErr != nil {
		t.Fatalf("the objects answered %v", objectErr)
	}
	byKind := map[db.SchemaObjectKind][]db.SchemaObject{}
	for _, object := range objects {
		byKind[object.Kind] = append(byKind[object.Kind], object)
	}
	for _, kind := range []db.SchemaObjectKind{
		db.ObjectFunction, db.ObjectSequence, db.ObjectTrigger,
	} {
		if len(byKind[kind]) == 0 {
			t.Errorf("the schema lists no %s", kind)
		}
	}
	for _, trigger := range byKind[db.ObjectTrigger] {
		if trigger.Schema != "dbo" || trigger.Name != "orders_touch" {
			continue
		}
		if trigger.Events != "insert, update" {
			t.Errorf("the trigger answers the events %q, wanted insert and update",
				trigger.Events)
		}
	}

	// The server keeps the statement that made a routine, so its definition is its own text.
	for _, object := range byKind[db.ObjectFunction] {
		if object.Schema != "dbo" || object.Name != "double_it" {
			continue
		}
		lines, ddlErr := session.BuildObjectDDL(ctx, object)
		if ddlErr != nil {
			t.Fatalf("the definition answered %v", ddlErr)
		}
		if !strings.Contains(strings.ToLower(strings.Join(lines, "\n")), "create function") {
			t.Errorf("the definition of the function reads %v", lines)
		}
	}
}

// The diagram draws every foreign key of the database, with the table it starts from.
func TestServerListsEveryForeignKey(t *testing.T) {
	session := openShop(t)

	relationships, err := session.ListRelationships(context.Background())
	if err != nil {
		t.Fatalf("the relationships answered %v", err)
	}
	for _, held := range relationships {
		if held.Name != "order_lines_order_fk" {
			continue
		}
		if held.Schema != "dbo" || held.Table != "order_lines" {
			t.Errorf("the key starts at %s.%s, wanted dbo.order_lines", held.Schema, held.Table)
		}
		if len(held.Columns) != 1 || held.Columns[0] != "order_id" {
			t.Errorf("the key holds the columns %v, wanted order_id", held.Columns)
		}
		if held.TargetTable != "orders" || len(held.TargetColumns) != 1 {
			t.Errorf("the key points at %s (%v)", held.TargetTable, held.TargetColumns)
		}
		if held.DeleteRule != query.DeleteRuleCascade {
			t.Errorf("the delete reads as %q, wanted cascade", held.DeleteRule)
		}
		return
	}
	t.Errorf("the key was not listed; the server answered %d of them", len(relationships))
}

// A read of a sequence takes no sort: the server refuses `next value for` in a statement
// with an ORDER BY. The first page of a read is capped by the client instead of a window.
func TestServerReadsTheNextValueOfASequence(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	dbtest.RunStatements(t, session,
		"if object_id('dbo.ticket_seq', 'SO') is not null drop sequence dbo.ticket_seq",
		"create sequence dbo.ticket_seq as bigint start with 1 increment by 1")
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(),
			"if object_id('dbo.ticket_seq', 'SO') is not null drop sequence dbo.ticket_seq",
			dbtest.ReadEverything, nil)
	})

	const written = "select next value for dbo.ticket_seq as ticket_no"
	read := session.Composer().ComposeStatementRead(db.BoundText{Text: written}, core.ReadRewrite{})
	page, err := session.ReadPage(ctx, read, db.ReadWindow{Limit: 200})
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if len(page.Rows) != 1 {
		t.Fatalf("the read gave %d rows, wanted 1", len(page.Rows))
	}
	if held := db.ReadNonNegativeCount(page.Rows[0][0]); held < 1 {
		t.Errorf("the sequence answered %d", held)
	}
}

// Every object of a schema answers a definition the object menu can show. The server keeps
// the statement of a routine, a trigger and a view; a sequence has none, so the client says
// so instead of showing an empty pane.
func TestServerAnswersTheDefinitionOfEveryObject(t *testing.T) {
	session := openShopWithObjects(t)
	ctx := context.Background()

	objects, err := session.ListSchemaObjects(ctx)
	if err != nil {
		t.Fatalf("the objects answered %v", err)
	}
	for _, object := range objects {
		if object.Schema != "dbo" {
			continue
		}
		lines, ddlErr := session.BuildObjectDDL(ctx, object)
		if ddlErr != nil {
			t.Errorf("the definition of %s %s answered %v", object.Kind, object.Name, ddlErr)
			continue
		}
		written := strings.TrimSpace(strings.Join(lines, "\n"))
		if written == "" {
			t.Errorf("%s %s answers an empty definition", object.Kind, object.Name)
			continue
		}
		t.Logf("%s %s: %.60s", object.Kind, object.Name, written)
	}
}
