//go:build integration

// An integration test: it reads a real PostgreSQL. The server is started outside this code,
// by `mise run servers-up` or by whatever runs the build, and named to the test through
// MASUME_TEST_POSTGRES. Nothing here knows how it was started.
//
// The build tag keeps these off `go test ./...` entirely: without `-tags=integration` the
// file is not even compiled.
package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
)

const dropSchema = `drop schema if exists masume_test cascade;`

const shopSchema = `
create schema masume_test;
create table masume_test.orders (
  id       serial primary key,
  customer text not null,
  total    numeric(10,2) default 0,
  paid_at  timestamptz
);
create index orders_customer_idx on masume_test.orders (customer);
create view masume_test.paid_orders as select * from masume_test.orders where total > 0;
insert into masume_test.orders (customer, total)
  values ('ada', 12.50), ('grace', 0), ('alan', 99.00);
`

// openShop answers a session with the schema laid out. It is dropped first, so a run that
// was cut short leaves nothing for the next one, and dropped again after.
func openShop(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropSchema, shopSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(), dropSchema, dbtest.ReadEverything, nil)
	})
	return session
}

func TestServerRunsAReadAndAnswersItsColumns(t *testing.T) {
	session := openShop(t)

	answered, err := session.RunQuery(context.Background(),
		"select customer, total from masume_test.orders order by customer",
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
	// The type comes from the catalog of the server, read at connect, because the map the
	// driver ships knows the standard types only.
	if answered.Columns[1].DataType != "numeric" {
		t.Errorf("the total reads as %q, wanted numeric", answered.Columns[1].DataType)
	}
}

// PostgreSQL refuses every later statement of a transaction one statement failed in, and the
// client has to report that rather than let the user write into a dead transaction.
func TestServerMarksATransactionThatFailed(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction did not open: %v", err)
	}
	if _, err := session.RunQuery(
		ctx, "select * from nothing_here", dbtest.ReadEverything, nil); err == nil {
		t.Fatal("a read of a table that is not there answered no error")
	}
	if held := session.ReadTransactionState(); held != db.TransactionFailed {
		t.Errorf("the state reads %q, wanted it failed", held)
	}
	if err := session.RollbackTransaction(ctx); err != nil {
		t.Fatalf("the rollback answered %v", err)
	}
	if held := session.ReadTransactionState(); held != db.TransactionNone {
		t.Errorf("the state reads %q after a rollback, wanted none", held)
	}
}

// The catalog read runs on a second connection, so it must answer while the first one holds
// a transaction open.
func TestServerReadsTheCatalogWhileATransactionIsOpen(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction did not open: %v", err)
	}
	defer func() { _ = session.RollbackTransaction(ctx) }()

	tables, err := session.ListTables(ctx)
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	kinds := map[string]db.RelationKind{}
	for _, table := range tables {
		if table.Schema == "masume_test" {
			kinds[table.Name] = table.Kind
		}
	}
	if kinds["orders"] != db.RelationTable {
		t.Errorf("orders reads as %q, wanted a table", kinds["orders"])
	}
	if kinds["paid_orders"] != db.RelationView {
		t.Errorf("paid_orders reads as %q, wanted a view", kinds["paid_orders"])
	}
}

func TestServerDescribesATable(t *testing.T) {
	session := openShop(t)

	detail, err := session.DescribeTable(context.Background(),
		db.TableRef{Schema: "masume_test", Name: "orders"})
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
	if byName["customer"].Nullable {
		t.Error("customer is declared not null and reads as nullable")
	}
	if !byName["total"].HasDefault {
		t.Error("total has a default and does not report one")
	}
}

// PostgreSQL has no statement that answers the definition of a table, so the client builds
// one from the catalog.
func TestServerBuildsTheDefinitionOfATable(t *testing.T) {
	session := openShop(t)

	lines, err := session.BuildTableDDL(context.Background(),
		db.TableRef{Schema: "masume_test", Name: "orders"})
	if err != nil {
		t.Fatalf("the definition answered %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("the definition came back empty")
	}
}

func TestServerExplainsAStatement(t *testing.T) {
	session := openShop(t)

	plan, err := session.ExplainQuery(context.Background(),
		"select * from masume_test.orders where customer = 'ada'", false)
	if err != nil {
		t.Fatalf("the plan answered %v", err)
	}
	if plan.Root.Label == "" && plan.Raw == "" {
		t.Error("the server answered a plan with nothing in it")
	}
}

// A statement the server refuses keeps the error of the driver in the chain, so the message
// the server wrote reaches the user and a caller can still read the type.
func TestServerAnswersAStatementItRefuses(t *testing.T) {
	session := openShop(t)

	_, err := session.RunQuery(context.Background(),
		"insert into masume_test.orders (customer) values (null)", dbtest.ReadEverything, nil)
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

func TestServerEndsAStatementWithItsContext(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)

	ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if _, err := session.RunQuery(
		ctx, "select pg_sleep(30)", dbtest.ReadEverything, nil); err == nil {
		t.Error("a statement that ran past its context answered no error")
	}
}
