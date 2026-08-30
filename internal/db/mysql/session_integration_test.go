//go:build integration

// An integration test: it reads a real MySQL. The server is started outside this code and
// named through MASUME_TEST_MYSQL. Nothing here knows how it was started.
package mysql_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
)

const shopSchema = `
drop table if exists orders;
create table orders (
  id       int auto_increment primary key,
  customer varchar(64) not null,
  total    decimal(10,2) default 0,
  paid_at  datetime
);
create index orders_customer_idx on orders (customer);
insert into orders (customer, total) values ('ada', 12.50), ('grace', 0), ('alan', 99.00);
`

func openShop(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.MySQL)
	dbtest.RunStatements(t, session, shopSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(),
			"drop table if exists orders", dbtest.ReadEverything, nil)
	})
	return session
}

func TestServerRunsAReadAndAnswersItsColumns(t *testing.T) {
	session := openShop(t)

	answered, err := session.RunQuery(context.Background(),
		"select customer, total from orders order by customer", dbtest.ReadEverything, nil)
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

// MySQL keeps a transaction alive after a statement it refused, apart from a deadlock or a
// lock-wait timeout. This is where it differs from PostgreSQL, and the client must not report
// the transaction as failed.
func TestServerKeepsATransactionAfterAStatementItRefused(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction did not open: %v", err)
	}
	defer func() { _ = session.RollbackTransaction(ctx) }()

	if _, err := session.RunQuery(
		ctx, "select * from nothing_here", dbtest.ReadEverything, nil); err == nil {
		t.Fatal("a read of a table that is not there answered no error")
	}
	if held := session.ReadTransactionState(); held != db.TransactionOpen {
		t.Errorf("the state reads %q, wanted it still open", held)
	}
	// The transaction still works, which is what "still open" has to mean.
	if _, err := session.RunQuery(
		ctx, "select count(*) from orders", dbtest.ReadEverything, nil); err != nil {
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
		ctx, "delete from orders", dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the delete answered %v", err)
	}
	if err := session.RollbackTransaction(ctx); err != nil {
		t.Fatalf("the rollback answered %v", err)
	}

	answered, err := session.RunQuery(
		ctx, "select count(*) from orders", dbtest.ReadEverything, nil)
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

	tables, err := session.ListTables(ctx)
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	var orders db.TableRef
	for _, table := range tables {
		if table.Name == "orders" {
			orders = table
		}
	}
	if orders.Name == "" {
		t.Fatalf("orders was not listed; the server answered %d relations", len(tables))
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
		t.Error("id does not read as the primary key")
	}
	if byName["customer"].Nullable {
		t.Error("customer is declared not null and reads as nullable")
	}

	indexes, err := session.ListIndexes(ctx, orders)
	if err != nil {
		t.Fatalf("the indexes answered %v", err)
	}
	found := false
	for _, index := range indexes {
		if index.Name == "orders_customer_idx" {
			found = true
		}
	}
	if !found {
		t.Errorf("the index was not listed; the server answered %d of them", len(indexes))
	}
}

// MySQL answers the CREATE statement of a table itself, which PostgreSQL does not.
func TestServerAnswersTheCreateStatementOfATable(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	tables, err := session.ListTables(ctx)
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	var orders db.TableRef
	for _, table := range tables {
		if table.Name == "orders" {
			orders = table
		}
	}

	lines, err := session.BuildTableDDL(ctx, orders)
	if err != nil {
		t.Fatalf("the definition answered %v", err)
	}
	written := strings.ToLower(strings.Join(lines, "\n"))
	if !strings.Contains(written, "create table") {
		t.Errorf("the definition does not hold a create statement:\n%s", written)
	}
}

func TestServerAnswersAStatementItRefuses(t *testing.T) {
	session := openShop(t)

	_, err := session.RunQuery(context.Background(),
		"insert into orders (customer) values (null)", dbtest.ReadEverything, nil)
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

// A lock timeout on a stock server leaves the transaction open, so the mark must not read
// failed: the next staged write would otherwise begin its own transaction, and MySQL would
// commit the work the user never committed.
func TestALockTimeoutLeavesTheTransactionOpen(t *testing.T) {
	first := openShop(t)
	second := dbtest.Open(t, dbtest.MySQL)
	ctx := context.Background()

	dbtest.RunStatements(t, first, "insert into orders (customer, total) values ('ada', 1)")

	// The second connection shortens its wait, so the timeout comes quickly.
	if _, err := second.RunQuery(ctx,
		"set session innodb_lock_wait_timeout = 1", dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the timeout was not shortened: %v", err)
	}
	if err := first.BeginTransaction(ctx); err != nil {
		t.Fatalf("the first transaction did not open: %v", err)
	}
	if _, err := first.RunQuery(ctx,
		"update orders set total = 2 where customer = 'ada'",
		dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the first write failed: %v", err)
	}

	if err := second.BeginTransaction(ctx); err != nil {
		t.Fatalf("the second transaction did not open: %v", err)
	}
	_, timeoutErr := second.RunQuery(ctx,
		"update orders set total = 3 where customer = 'ada'", dbtest.ReadEverything, nil)
	if timeoutErr == nil {
		t.Fatal("the second write was not held up by the lock")
	}
	t.Logf("the server answered: %v", timeoutErr)
	t.Logf("the state of the second connection reads %q", second.ReadTransactionState())
	if state := second.ReadTransactionState(); state != db.TransactionOpen {
		t.Errorf("a lock timeout left the state %q, wanted it open", state)
	}

	_ = second.RollbackTransaction(ctx)
	_ = first.RollbackTransaction(ctx)
}

// A buffer answers with the result of its last statement, as PostgreSQL and SQLite do. A
// write before a read must not name the result, and a write after a read must report the
// rows it changed rather than the count the read left behind.
func TestServerNamesABufferAfterItsLastStatement(t *testing.T) {
	session := openShop(t)
	ctx := context.Background()

	read, err := session.RunQuery(ctx,
		"insert into orders (customer, total) values ('lin', 1); "+
			"select customer from orders order by customer", dbtest.ReadEverything, nil)
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
		"select customer from orders; update orders set total = total + 1 where customer = 'ada'",
		dbtest.ReadEverything, nil)
	if writeErr != nil {
		t.Fatalf("the buffer answered %v", writeErr)
	}
	if written.Command != "UPDATE" {
		t.Errorf("the buffer is named %q, wanted UPDATE", written.Command)
	}
	if !written.HasAffected || written.Affected != 1 {
		t.Errorf("the buffer reports %d rows changed, has %v", written.Affected, written.HasAffected)
	}
	if len(written.Rows) != 0 {
		t.Errorf("a buffer that ends in a write gave %d rows", len(written.Rows))
	}
}
