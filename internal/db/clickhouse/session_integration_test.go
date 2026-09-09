//go:build integration

// An integration test: it reads a real ClickHouse. The server is started outside this code
// and named through MASUME_TEST_CLICKHOUSE. Nothing here knows how it was started.
package clickhouse_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/clickhouse"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/db/engines"
)

// testSchema is the database of these tests, which is the database the connection opens.
// The tree of a ClickHouse profile draws that database alone. The server takes one statement
// at a time, so each one runs on its own.
const testSchema = "shop"

var shopSchema = []string{
	`create database if not exists shop`,
	`drop table if exists shop.orders`,
	`create table shop.orders (
	   id       UInt64,
	   customer String,
	   total    Decimal(10,2),
	   paid_at  Nullable(DateTime64(3)),
	   loud     String materialized upper(customer)
	 ) engine = MergeTree order by id`,
	`alter table shop.orders
	   add index customer_idx customer type set(100) granularity 4`,
	`insert into shop.orders (id, customer, total)
	   values (1, 'ada', 12.50), (2, 'grace', 0), (3, 'alan', 99.00)`,
}

func openShop(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Clickhouse)
	dbtest.RunStatements(t, session, shopSchema...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(),
			"drop table if exists shop.orders", dbtest.ReadEverything, nil)
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
		if table.Schema == testSchema && table.Name == name {
			return table
		}
	}
	t.Fatalf("%s was not listed; the server answered %d relations", name, len(tables))
	return db.TableRef{}
}

func TestServerRunsAReadAndAnswersItsColumns(t *testing.T) {
	session := openShop(t)

	answered, err := session.RunQuery(context.Background(),
		"select customer, total from shop.orders order by customer",
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
	if answered.Columns[1].DataType != "Decimal(10, 2)" {
		t.Errorf("the total carries the type %q", answered.Columns[1].DataType)
	}
}

func TestServerReadsOnePageOfARelation(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	orders := findTable(t, session, "orders")

	read := session.Composer().ComposeRelationRead(orders, core.ReadRewrite{})
	page, err := session.ReadPage(ctx, read, db.ReadWindow{Limit: 2})
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

// The protocol takes one statement per call, so a buffer of several runs one at a time and
// answers with the result of the last one.
func TestServerRunsEveryStatementOfABuffer(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	read, err := session.RunQuery(ctx,
		"insert into shop.orders (id, customer, total) values (4, 'lin', 1); "+
			"select customer from shop.orders order by customer",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the buffer answered %v", err)
	}
	if read.Command != "SELECT" {
		t.Errorf("the buffer is named %q, wanted SELECT", read.Command)
	}
	if len(read.Rows) != 4 {
		t.Errorf("the buffer gave %d rows, wanted 4", len(read.Rows))
	}

	// A write answers with no count of its own on this server.
	written, writeErr := session.RunQuery(ctx,
		"select customer from shop.orders; "+
			"alter table shop.orders update total = 2 where id = 1",
		dbtest.ReadEverything, nil)
	if writeErr != nil {
		t.Fatalf("the buffer answered %v", writeErr)
	}
	if written.Command != "ALTER" {
		t.Errorf("the buffer is named %q, wanted ALTER", written.Command)
	}
	if written.HasAffected {
		t.Errorf("the write reported %d rows changed", written.Affected)
	}
	if len(written.Rows) != 0 {
		t.Errorf("a buffer that ends in a write gave %d rows", len(written.Rows))
	}
}

// A buffer of several statements binds no values, because each statement is a call of its
// own and the values belong to one of them.
func TestServerRefusesValuesForSeveralStatements(t *testing.T) {
	session := openShop(t)

	_, err := session.RunQuery(context.Background(),
		"select ?; select 2", dbtest.ReadEverything, []any{1})
	if err == nil {
		t.Fatal("a buffer of two statements took the values of the user")
	}
	if !strings.Contains(db.DescribeError(err), "one statement") {
		t.Errorf("the refusal reads %q", db.DescribeError(err))
	}
}

func TestServerListsAndDescribesATable(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	orders := findTable(t, session, "orders")

	if orders.Kind != db.RelationTable {
		t.Errorf("the table reads as %q", orders.Kind)
	}
	if orders.EstimatedRows != 3 {
		t.Errorf("the table reports %d rows, wanted 3", orders.EstimatedRows)
	}

	detail, err := session.DescribeTable(ctx, orders)
	if err != nil {
		t.Fatalf("the describe answered %v", err)
	}
	byName := map[string]db.ColumnDetail{}
	for _, column := range detail.Columns {
		byName[column.Name] = column
	}
	if !byName["id"].IsPrimaryKey {
		t.Error("id is the sorting key and does not read as the primary key")
	}
	if byName["customer"].Nullable {
		t.Error("customer takes no null and reads as nullable")
	}
	if !byName["paid_at"].Nullable {
		t.Error("paid_at takes a null and reads as not null")
	}
	if !byName["loud"].IsGenerated {
		t.Error("a materialized column does not read as one the server fills")
	}
	if byName["total"].DataType != "Decimal(10, 2)" {
		t.Errorf("total reads as %q", byName["total"].DataType)
	}

	indexes, indexErr := session.ListIndexes(ctx, orders)
	if indexErr != nil {
		t.Fatalf("the indexes answered %v", indexErr)
	}
	found, sorted := false, false
	for _, index := range indexes {
		if index.Name == "customer_idx" {
			found = true
			if !strings.Contains(index.Definition, "type set(100)") {
				t.Errorf("the index reads as %q", index.Definition)
			}
		}
		if index.IsPrimary {
			sorted = true
			if index.Definition != "order by id" {
				t.Errorf("the sorting key reads as %q", index.Definition)
			}
		}
	}
	if !found {
		t.Errorf("the index was not listed; the server answered %d of them", len(indexes))
	}
	if !sorted {
		t.Error("the sorting key was not listed")
	}
}

// The server writes the statement that made a relation.
func TestServerAnswersTheCreateStatementOfATable(t *testing.T) {
	session := openShop(t)
	orders := findTable(t, session, "orders")

	lines, err := session.BuildTableDDL(context.Background(), orders)
	if err != nil {
		t.Fatalf("the definition answered %v", err)
	}
	written := strings.Join(lines, "\n")
	if !strings.Contains(strings.ToLower(written), "create table") {
		t.Errorf("the definition holds no create statement:\n%s", written)
	}
	if !strings.Contains(written, "ENGINE = MergeTree") {
		t.Errorf("the definition names no engine:\n%s", written)
	}
}

func TestServerAnswersAStatementItRefuses(t *testing.T) {
	session := openShop(t)

	_, err := session.RunQuery(context.Background(),
		"select * from shop.nothing_here", dbtest.ReadEverything, nil)
	if err == nil {
		t.Fatal("a read of a table that is not there answered no error")
	}
	if !errors.Is(err, db.ErrDatabase) {
		t.Error("the error does not read as one from the database")
	}
	described := db.DescribeError(err)
	if described == "" || strings.Contains(described, "DB::Exception") {
		t.Errorf("the error is described as %q", described)
	}
}

func TestServerPlansAStatement(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	plan, err := session.ExplainQuery(ctx,
		"select customer from shop.orders where id = 2", false)
	if err != nil {
		t.Fatalf("the plan answered %v", err)
	}
	if plan.Root.Label == "" {
		t.Error("the plan names no step")
	}
	if len(plan.Root.Children) == 0 {
		t.Error("the plan holds no step under its root")
	}
	if !strings.Contains(plan.Raw, "ReadFromMergeTree") {
		t.Errorf("the plan reads as:\n%s", plan.Raw)
	}
	if plan.Measurable {
		t.Error("the plan reads as measurable, and no plan of this server is measured")
	}

	// No plan of this server carries a measurement, so a measured plan is refused.
	if _, measureErr := session.ExplainQuery(ctx,
		"select customer from shop.orders", true); measureErr == nil {
		t.Error("a measured plan answered no error")
	}
}

func TestServerNamesTheFaultOfAStatement(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	problem, faulty := session.CheckStatement(ctx, "select * from shop.nothing_here")
	if !faulty {
		t.Fatal("a read of a table that is not there was checked as good")
	}
	if !strings.Contains(problem.Message, "nothing_here") {
		t.Errorf("the fault reads %q and does not name the table", problem.Message)
	}
	if _, held := session.CheckStatement(
		ctx, "select customer from shop.orders"); held {
		t.Error("a statement the server reads was checked as faulty")
	}

	// A parse error names the place it stopped at, counted in the statement of the user.
	const written = "select customer from where id = 1"
	broken, isFaulty := session.CheckStatement(ctx, written)
	if !isFaulty {
		t.Fatal("a statement the server cannot parse was checked as good")
	}
	if !broken.HasOffset || broken.Offset < 0 || broken.Offset > len(written) {
		t.Errorf("the fault points at offset %d, has %v", broken.Offset, broken.HasOffset)
	}
}

func TestServerReportsItsSessionsAndItsLoad(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	if _, err := session.ListActivity(ctx); err != nil {
		t.Errorf("the activity answered %v", err)
	}
	// The server takes no lock a session waits for, so the pane is refused.
	if _, err := session.ListLockWaits(ctx); err == nil {
		t.Error("the lock waits answered a list, and this server holds none")
	}
	load, err := session.ReadServerLoad(ctx)
	if err != nil {
		t.Fatalf("the load answered %v", err)
	}
	if load.Connections <= 0 {
		t.Errorf("the server reports %d connections", load.Connections)
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

	batches, rows := 0, 0
	counted, err := session.StreamQuery(context.Background(),
		"select customer from shop.orders", nil, 2,
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

// A staged edit of a row is a mutation of the table, and the server finishes it before it
// answers.
func TestServerAppliesStagedChanges(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	orders := findTable(t, session, "orders")

	read, err := session.RunQuery(ctx,
		"select id, customer from shop.orders where customer = 'ada'",
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
	if len(changes) != 1 || !strings.HasPrefix(changes[0].Display, "alter table") {
		t.Fatalf("the change reads as %q", changes[0].Display)
	}
	if err := session.ApplyChanges(ctx, changes); err != nil {
		t.Fatalf("the change answered %v", err)
	}

	after, afterErr := session.RunQuery(ctx,
		"select customer from shop.orders where customer = 'ida'",
		dbtest.ReadEverything, nil)
	if afterErr != nil {
		t.Fatalf("the read answered %v", afterErr)
	}
	if len(after.Rows) != 1 {
		t.Errorf("the change left %d rows named ida, wanted 1", len(after.Rows))
	}
}

// A staged delete of a row removes it, and the row is gone from the next read.
func TestServerAppliesAStagedDelete(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	orders := findTable(t, session, "orders")

	read, err := session.RunQuery(ctx,
		"select id, customer from shop.orders where customer = 'grace'",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	pending := core.NewPendingChanges()
	pending.DeletedRows[0] = true
	changes, buildErr := session.Composer().BuildChanges(db.ChangeTarget{
		Table: orders, Columns: read.Columns, Rows: read.Rows, KeyColumns: []string{"id"},
	}, pending)
	if buildErr != nil {
		t.Fatalf("the change was not built: %v", buildErr)
	}
	if err := session.ApplyChanges(ctx, changes); err != nil {
		t.Fatalf("the delete answered %v", err)
	}

	counted, countErr := session.RunQuery(ctx,
		"select count() from shop.orders", dbtest.ReadEverything, nil)
	if countErr != nil {
		t.Fatalf("the count answered %v", countErr)
	}
	if held := db.ReadNonNegativeCount(counted.Rows[0][0]); held != 2 {
		t.Errorf("the delete left %d rows, wanted 2", held)
	}
}

// The server holds no transaction of the user, so the client refuses to open one rather
// than promising something the server does not keep.
func TestServerHoldsNoTransaction(t *testing.T) {
	session := openShop(t)

	if session.Capabilities().HasTransactions {
		t.Error("the engine reads as one that holds a transaction")
	}
	if err := session.BeginTransaction(context.Background()); err == nil {
		t.Error("a transaction opened on a server that holds none")
	}
	if held := session.ReadTransactionState(); held != db.TransactionNone {
		t.Errorf("the state reads %q, wanted none", held)
	}
}

// A function of the user is listed with the statement that made it.
func TestServerListsTheFunctionsOfTheUser(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()
	dbtest.RunStatements(t, session,
		"create function if not exists masume_double as (x) -> x * 2")
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(),
			"drop function if exists masume_double", dbtest.ReadEverything, nil)
	})

	objects, err := session.ListSchemaObjects(ctx)
	if err != nil {
		t.Fatalf("the objects answered %v", err)
	}
	for _, object := range objects {
		if object.Name != "masume_double" {
			continue
		}
		if object.Kind != db.ObjectFunction {
			t.Errorf("the function reads as %q", object.Kind)
		}
		lines, ddlErr := session.BuildObjectDDL(ctx, object)
		if ddlErr != nil {
			t.Fatalf("the definition answered %v", ddlErr)
		}
		if !strings.Contains(strings.ToLower(strings.Join(lines, "\n")), "create function") {
			t.Errorf("the definition reads as %v", lines)
		}
		return
	}
	t.Errorf("the function was not listed; the server answered %d objects", len(objects))
}

// A statement of this connection is stopped through a second connection, and the connection
// takes the next statement afterwards.
func TestServerStopsARunningStatement(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	var group sync.WaitGroup
	var runErr error
	group.Add(1)
	go func() {
		defer group.Done()
		_, runErr = session.RunQuery(ctx, "select sleep(3)", dbtest.ReadEverything, nil)
	}()

	// The statement needs a moment to reach the server before it can be stopped.
	stopped := false
	for at := 0; at < 20 && !stopped; at++ {
		time.Sleep(100 * time.Millisecond)
		held, err := session.CancelRunningQuery(ctx)
		if err == nil && held {
			stopped = true
		}
	}
	group.Wait()

	if !stopped {
		t.Fatal("the statement was not stopped")
	}
	if runErr == nil {
		t.Error("the stopped statement answered no error")
	}
	if _, err := session.RunQuery(
		ctx, "select 1", dbtest.ReadEverything, nil); err != nil {
		t.Errorf("the statement after the stopped one answered %v", err)
	}
}

// A read-only profile opens a session the server itself refuses every write on. The client
// refuses every write it recognizes before it is sent, so this reads the session of the
// adapter, without the wrapper of the client over it.
func TestServerRefusesAWriteOnAReadOnlyConnection(t *testing.T) {
	// The relation of the tests must exist before the read-only session reads it.
	openShop(t)

	profile, password := dbtest.BuildProfile(t, dbtest.Clickhouse)
	profile.AccessMode = cfg.AccessReadOnly

	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	session, err := clickhouse.NewAdapter(clickhouse.Support).Connect(ctx, profile, password)
	if err != nil {
		t.Fatalf("the connection answered %v", err)
	}
	defer func() { _ = session.Close() }()

	_, writeErr := session.RunQuery(ctx,
		"optimize table shop.orders", dbtest.ReadEverything, nil)
	if writeErr == nil {
		t.Fatal("a read-only session ran a write on the server")
	}
	if !strings.Contains(db.DescribeError(writeErr), "readonly mode") {
		t.Errorf("the refusal reads %q, wanted the one of the server",
			db.DescribeError(writeErr))
	}
	// A read still answers, and so does one that carries a time limit of its own.
	if _, readErr := session.RunQuery(
		ctx, "select 1", dbtest.ReadEverything, nil); readErr != nil {
		t.Errorf("a read on a read-only session answered %v", readErr)
	}

	// The client refuses a write of its own accord as well, before it reaches the server.
	held, openErr := engines.CreateAdapters().Open(ctx, profile, password)
	if openErr != nil {
		t.Fatalf("the connection answered %v", openErr)
	}
	defer func() { _ = held.Close() }()
	if _, err := held.RunQuery(ctx,
		"insert into shop.orders (id) values (9)", dbtest.ReadEverything,
		nil); err == nil {
		t.Error("the client sent a write on a read-only connection")
	}
}
