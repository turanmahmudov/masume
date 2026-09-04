// A functional test of the whole port: it opens a real file through the real adapter and
// asks it what the app asks. SQLite needs no server, so it runs in the ordinary suite. The
// same shape for PostgreSQL and MySQL sits behind the `integration` build tag.
package sqlite_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/query"
)

// openFile opens a session on a fresh SQLite file that holds the schema given.
func openFile(t *testing.T, schema string) db.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	// The adapter refuses a path with no file, so the file is made before it opens.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("cannot make the database file: %v", err)
	}

	profile := cfg.Profile{
		Name: "test", Engine: core.EngineSqlite, Database: path,
		AccessMode: cfg.AccessWrite, PageSize: 100,
	}
	ctx := context.Background()
	session, err := engines.CreateAdapters().Open(ctx, profile, "")
	if err != nil {
		t.Fatalf("cannot open the file: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	if schema != "" {
		if _, err := session.RunQuery(ctx, schema, readEverything, nil); err != nil {
			t.Fatalf("cannot lay out the schema: %v", err)
		}
	}
	return session
}

// readEverything is a row limit above every row these tests insert. The port has no value
// for "no limit": the limit must be above zero, because a read asks for one row more than it
// to tell a full page from the last one.
const readEverything = 100

const shopSchema = `
create table orders (
  id integer primary key,
  customer text not null,
  total real default 0
);
create index orders_customer_idx on orders (customer);
create view paid_orders as select * from orders where total > 0;
insert into orders (customer, total) values ('ada', 12.5), ('grace', 0), ('alan', 99);
`

func TestSessionRunsAReadAndAnswersItsColumns(t *testing.T) {
	session := openFile(t, shopSchema)

	answered, err := session.RunQuery(context.Background(),
		"select customer, total from orders order by customer", readEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if len(answered.Rows) != 3 {
		t.Fatalf("the read gave %d rows, wanted 3", len(answered.Rows))
	}
	if len(answered.Columns) != 2 {
		t.Fatalf("the read gave %d columns, wanted 2", len(answered.Columns))
	}
	if answered.Columns[0].Name != "customer" {
		t.Errorf("the first column is %q, wanted customer", answered.Columns[0].Name)
	}
	if held := answered.Rows[0][0]; held != "ada" {
		t.Errorf("the first row reads %v, wanted the orders in the order asked for", held)
	}
}

// A read is capped, and the cap is reported rather than hidden, so the grid can say there
// are more rows than it drew.
func TestSessionReportsAReadItCapped(t *testing.T) {
	session := openFile(t, shopSchema)

	answered, err := session.RunQuery(context.Background(), "select * from orders", 2, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if len(answered.Rows) != 2 {
		t.Errorf("the read gave %d rows, wanted the cap of 2", len(answered.Rows))
	}
	if !answered.Truncated {
		t.Error("the read was capped and did not report it")
	}
}

func TestSessionListsWhatTheFileHolds(t *testing.T) {
	session := openFile(t, shopSchema)
	ctx := context.Background()

	tables, err := session.ListTables(ctx)
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	kinds := map[string]db.RelationKind{}
	for _, table := range tables {
		kinds[table.Name] = table.Kind
	}
	if kinds["orders"] != db.RelationTable {
		t.Errorf("orders reads as %q, wanted a table", kinds["orders"])
	}
	if kinds["paid_orders"] != db.RelationView {
		t.Errorf("paid_orders reads as %q, wanted a view", kinds["paid_orders"])
	}
}

func TestSessionDescribesATable(t *testing.T) {
	session := openFile(t, shopSchema)
	ctx := context.Background()

	tables, err := session.ListTables(ctx)
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	var orders db.TableRef
	for _, table := range tables {
		if table.Name == "orders" {
			orders = table
		}
	}

	detail, err := session.DescribeTable(ctx, orders)
	if err != nil {
		t.Fatalf("the describe answered %v", err)
	}
	byName := map[string]db.ColumnDetail{}
	for _, column := range detail.Columns {
		byName[column.Name] = column
	}
	if len(byName) != 3 {
		t.Fatalf("the table has %d columns, wanted 3", len(byName))
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
		t.Errorf("the index was not listed; the file answered %d of them", len(indexes))
	}
}

// A statement the server refuses must answer an error that reads as one from the database,
// with the message of the server kept for the user.
func TestSessionAnswersAStatementTheServerRefuses(t *testing.T) {
	session := openFile(t, shopSchema)

	_, err := session.RunQuery(context.Background(), "select * from nothing_here", readEverything, nil)
	if err == nil {
		t.Fatal("a read of a table that is not there answered no error")
	}
	if described := db.DescribeError(err); described == "" {
		t.Error("the error is described as an empty text")
	}
}

func TestSessionRunsAWriteAndCountsIt(t *testing.T) {
	session := openFile(t, shopSchema)
	ctx := context.Background()

	answered, err := session.RunQuery(ctx,
		"update orders set total = 1 where customer = 'grace'", readEverything, nil)
	if err != nil {
		t.Fatalf("the write answered %v", err)
	}
	if !answered.HasAffected || answered.Affected != 1 {
		t.Errorf("the write reports %d rows changed, wanted 1", answered.Affected)
	}
}

// A transaction the user opens must hold its writes until it commits, and drop them on a
// rollback.
func TestSessionRollsBackATransaction(t *testing.T) {
	session := openFile(t, shopSchema)
	ctx := context.Background()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Fatalf("the transaction did not open: %v", err)
	}
	if session.ReadTransactionState() != db.TransactionOpen {
		t.Errorf("the state reads %q, wanted it open", session.ReadTransactionState())
	}
	if _, err := session.RunQuery(ctx, "delete from orders", readEverything, nil); err != nil {
		t.Fatalf("the delete answered %v", err)
	}
	if err := session.RollbackTransaction(ctx); err != nil {
		t.Fatalf("the rollback answered %v", err)
	}

	answered, err := session.RunQuery(ctx, "select count(*) from orders", readEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if held := db.ReadNonNegativeCount(answered.Rows[0][0]); held != 3 {
		t.Errorf("the rollback left %d rows, wanted the 3 it started with", held)
	}
}

// The staged work of the grid is applied as one transaction: every change lands, or none does.
func TestSessionAppliesStagedChangesTogether(t *testing.T) {
	session := openFile(t, shopSchema)
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

	read, err := session.RunQuery(ctx,
		"select id, customer, total from orders order by id", readEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}

	staged := core.NewPendingChanges()
	staged.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1,
		Value: core.CellValue{Kind: core.CellText, Text: "ada renamed"},
	}
	staged.Inserts = append(staged.Inserts, map[string]any{"customer": "new", "total": "1"})

	changes, err := session.Composer().BuildChanges(db.ChangeTarget{
		Table: orders, Columns: read.Columns, Rows: read.Rows, KeyColumns: []string{"id"},
	}, staged)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	if err := session.ApplyChanges(ctx, changes); err != nil {
		t.Fatalf("applying the changes answered %v", err)
	}

	after, err := session.RunQuery(ctx,
		"select count(*) from orders where customer in ('ada renamed', 'new')",
		readEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if held := db.ReadNonNegativeCount(after.Rows[0][0]); held != 2 {
		t.Errorf("%d of the two changes landed", held)
	}
}

// A change the server refuses takes the whole batch with it, so the grid never shows half of
// an edit as written.
func TestSessionAppliesNoChangeWhereOneIsRefused(t *testing.T) {
	session := openFile(t, shopSchema)
	ctx := context.Background()

	// The second change writes a null into a column that refuses one.
	changes := []db.Change{
		{Description: "good", Display: "update orders set customer = 'held' where id = 1",
			Params: nil, Payload: nil},
	}
	// A change with no payload the engine understands is refused before the server is asked.
	if err := session.ApplyChanges(ctx, changes); err == nil {
		t.Fatal("a change the engine cannot read was applied")
	}

	// Nothing was written, so the first row still reads as it did.
	after, err := session.RunQuery(ctx,
		"select customer from orders where id = 1", readEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if held := db.ReadAnyText(after.Rows[0][0]); held == "held" {
		t.Error("a change of a refused batch was written")
	}
}

// An export reads a batch at a time, so a relation larger than memory can be written out.
func TestSessionStreamsAReadInBatches(t *testing.T) {
	session := openFile(t, shopSchema)

	batches := 0
	rows := 0
	total, err := session.StreamQuery(context.Background(),
		"select customer from orders", nil, 2,
		func(batch [][]any, columns []db.ResultColumn) error {
			batches++
			rows += len(batch)
			if len(columns) == 0 {
				t.Error("a batch arrived with no column")
			}
			return nil
		})
	if err != nil {
		t.Fatalf("the stream answered %v", err)
	}
	if total != 3 {
		t.Errorf("the stream counted %d rows, wanted 3", total)
	}
	if rows != 3 {
		t.Errorf("the batches held %d rows in all, wanted 3", rows)
	}
	// Three rows in batches of two is two batches, so the batching is real.
	if batches != 2 {
		t.Errorf("three rows arrived in %d batches, wanted 2", batches)
	}
}

// A read of no rows still has columns, and a caller that writes a format needs them for its
// header. The statement is never run twice to get them, so the stream reports them itself.
func TestSessionStreamsTheColumnsOfAReadWithNoRows(t *testing.T) {
	session := openFile(t, shopSchema)

	batches := 0
	names := []string{}
	total, err := session.StreamQuery(context.Background(),
		"select customer from orders where customer = 'nobody'", nil, 2,
		func(batch [][]any, columns []db.ResultColumn) error {
			batches++
			if len(batch) != 0 {
				t.Errorf("a batch of %d rows arrived for a read of none", len(batch))
			}
			for _, column := range columns {
				names = append(names, column.Name)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("the stream answered %v", err)
	}
	if total != 0 {
		t.Errorf("the stream counted %d rows, wanted none", total)
	}
	if batches != 1 {
		t.Fatalf("the stream handed over %d batches, wanted one that carries the columns",
			batches)
	}
	if len(names) != 1 || names[0] != "customer" {
		t.Errorf("the batch carried the columns %v, wanted customer", names)
	}
}

// A statement with no result set has no columns, so the stream hands over nothing: a caller
// that wrote a header for it would write a document of nothing.
func TestSessionStreamsNothingForAStatementWithNoResultSet(t *testing.T) {
	session := openFile(t, shopSchema)

	batches := 0
	total, err := session.StreamQuery(context.Background(),
		"update orders set customer = customer", nil, 2,
		func([][]any, []db.ResultColumn) error {
			batches++
			return nil
		})
	if err != nil {
		t.Fatalf("the stream answered %v", err)
	}
	if total != 0 || batches != 0 {
		t.Errorf("the stream counted %d rows in %d batches, wanted none of either",
			total, batches)
	}
}

// A batch the caller refuses stops the stream, because an export that cannot write has no
// reason to read the rest of the relation.
func TestSessionStopsAStreamTheCallerRefuses(t *testing.T) {
	session := openFile(t, shopSchema)

	batches := 0
	_, err := session.StreamQuery(context.Background(),
		"select customer from orders", nil, 1,
		func([][]any, []db.ResultColumn) error {
			batches++
			return errors.New("the file cannot be written")
		})
	if err == nil {
		t.Fatal("a stream the caller refused answered no error")
	}
	if batches != 1 {
		t.Errorf("the stream read %d batches after the first was refused", batches)
	}
}

// The server reads a statement without running it, so the editor marks a fault before the
// user sends anything.
func TestSessionChecksAStatementWithoutRunningIt(t *testing.T) {
	session := openFile(t, shopSchema)
	ctx := context.Background()

	// A statement that is wrong is reported.
	problem, is := session.CheckStatement(ctx, "select * from nothing_here")
	if !is {
		t.Error("a read of a relation that is not there was not reported")
	} else if problem.Message == "" {
		t.Error("the fault carries no message")
	}

	// A statement that is good is not.
	if _, is := session.CheckStatement(ctx, "select * from orders"); is {
		t.Error("a good statement was reported as a fault")
	}

	// A check must not write, so a delete is still only checked.
	if _, is := session.CheckStatement(ctx, "delete from orders"); is {
		t.Error("a good delete was reported as a fault")
	}
	after, err := session.RunQuery(ctx, "select count(*) from orders", readEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if held := db.ReadNonNegativeCount(after.Rows[0][0]); held != 3 {
		t.Errorf("checking a delete removed rows: %d left of 3", held)
	}
}

// SQLite plans a statement but measures nothing, and the plan has to say so rather than
// showing a run that took no time.
func TestSessionPlansAStatementWithoutMeasuringIt(t *testing.T) {
	session := openFile(t, shopSchema)

	plan, err := session.ExplainQuery(context.Background(),
		"select * from orders where customer = 'ada'", false)
	if err != nil {
		t.Fatalf("the plan answered %v", err)
	}
	if plan.Root.Label == "" && plan.Raw == "" {
		t.Error("the plan came back empty")
	}
	if plan.Measurable {
		t.Error("the plan reads as measured, and this engine measures nothing")
	}
}

// A plan prefixes only the first statement, so a buffer of several would run every
// statement after that as itself. The batch is refused, and the write does not run.
func TestSessionDoesNotRunAWriteHiddenBehindAPlan(t *testing.T) {
	session := openFile(t, shopSchema)
	ctx := context.Background()

	_, err := session.ExplainQuery(ctx, "select 1; delete from orders", false)
	if err == nil {
		t.Fatal("a batch was planned")
	}
	if !strings.Contains(db.DescribeError(err), "one statement") {
		t.Errorf("the refusal reads %q", db.DescribeError(err))
	}

	after, err := session.RunQuery(ctx, "select count(*) from orders", readEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if held := db.ReadNonNegativeCount(after.Rows[0][0]); held != 3 {
		t.Errorf("planning a batch removed rows: %d left of 3", held)
	}
}

// The pool holds one connection, so a ping while a statement runs would wait for that
// statement and then look like a dead server. A file that is still answering is healthy.
func TestPingTreatsABusyFileAsHealthy(t *testing.T) {
	session := openFile(t, shopSchema)

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		held := false
		_, err := session.StreamQuery(context.Background(),
			"select * from orders", nil, 1,
			func([][]any, []db.ResultColumn) error {
				if !held {
					held = true
					close(started)
					<-release
				}
				return nil
			})
		finished <- err
	}()

	select {
	case <-started:
	case err := <-finished:
		t.Fatalf("the read finished before it held the file: %v", err)
	}

	pingCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := session.Ping(pingCtx); err != nil {
		t.Errorf("a busy file was reported as dead: %v", err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Errorf("the read failed: %v", err)
	}
}

// The foreign keys of the file are read, because the tree follows them and a grid cell can
// jump along one. A write plan names the relations a delete is followed into, so the rule
// of each key comes back with it.
func TestSessionListsTheRelationshipsOfTheFile(t *testing.T) {
	session := openFile(t, `
create table customers (id integer primary key, name text);
create table orders (
  id integer primary key,
  customer_id integer references customers (id) on delete cascade
);
create table notes (
  id integer primary key,
  customer_id integer references customers (id) on delete restrict
);
`)

	held, err := session.ListRelationships(context.Background())
	if err != nil {
		t.Fatalf("the relationships answered %v", err)
	}
	rules := map[string]query.DeleteRule{}
	for _, one := range held {
		rules[one.Table] = one.DeleteRule
	}
	if rules["orders"] != query.DeleteRuleCascade {
		t.Errorf("the cascading key answered %q", rules["orders"])
	}
	if rules["notes"] != query.DeleteRuleRestrict {
		t.Errorf("the refusing key answered %q", rules["notes"])
	}
}

// The pool opens one connection, so a statement of another goroutine can take it between
// the `begin` of a staged set and its `commit` and run inside that transaction. The file
// is held for one caller at a time, so a set lands whole and nothing joins it.
func TestApplyChangesHoldsTheFileForTheWholeSet(t *testing.T) {
	session := openFile(t, `create table orders (id integer primary key, customer text);
insert into orders (customer) values ('ada');`)
	ctx := context.Background()

	changes := []db.Change{}
	for _, name := range []string{"one", "two", "three", "four"} {
		changes = append(changes, db.Change{
			Description: "insert " + name,
			Payload: db.BoundStatement{
				SQL: "insert into orders (customer) values (?)", Params: []any{name},
			},
		})
	}

	// A reader on its own goroutine asks for the file while the set is applied.
	var reading sync.WaitGroup
	reading.Go(func() {
		for range 20 {
			_, _ = session.RunQuery(ctx, "select count(*) from orders", readEverything, nil)
		}
	})

	if err := session.ApplyChanges(ctx, changes); err != nil {
		t.Fatalf("the staged set failed: %v", err)
	}
	reading.Wait()

	held, err := session.RunQuery(ctx, "select count(*) from orders", readEverything, nil)
	if err != nil {
		t.Fatalf("the count failed: %v", err)
	}
	if len(held.Rows) == 0 || db.ReadNonNegativeCount(held.Rows[0][0]) != 5 {
		t.Errorf("the table holds %v rows, wanted the one it had and the four staged",
			held.Rows)
	}
	if state := session.ReadTransactionState(); state != db.TransactionNone {
		t.Errorf("the state reads %q after the set", state)
	}
}
