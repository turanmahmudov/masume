//go:build integration

// An integration test: it dumps a real server and runs the dump back into it. PostgreSQL is
// named through MASUME_TEST_POSTGRES, MySQL through MASUME_TEST_MYSQL.
package dump_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/dump"
)

const dropDumpSchema = `drop schema if exists masume_dump cascade;`

// The schema holds one of everything a dump writes: a sequence and a function in front of
// the tables, two tables joined by a foreign key, a view over them, and a trigger on one.
const dumpSchema = `
create schema masume_dump;
create sequence masume_dump.order_number;
create function masume_dump.touch_order() returns trigger as $$ begin return new; end; $$
  language plpgsql;
create function masume_dump.describe(order_id integer) returns text as $body$
begin
  if order_id is null then
    return 'none; nothing';
  end if;
  return 'order';
end;
$body$ language plpgsql;
create table masume_dump.customers (
  id   integer primary key,
  name text not null
);
create table masume_dump.orders (
  id          integer not null,
  customer_id integer not null references masume_dump.customers (id),
  status      text not null,
  note        text
);
create view masume_dump.open_orders as
  select id from masume_dump.orders where status = 'open';
create trigger t_touch_order before update on masume_dump.orders
  for each row execute function masume_dump.touch_order();
insert into masume_dump.customers (id, name) values (1, 'ada');
insert into masume_dump.orders (id, customer_id, status, note)
  values (1, 1, 'open', 'first'), (2, 1, 'sent', null), (3, 1, 'open', 'a '';'' note');
`

const dropDumpDatabase = `drop database if exists masume_dump`

// The MySQL protocol takes one statement per call, so the database is built statement by
// statement.
var dumpDatabase = []string{
	"create database masume_dump",
	`create table masume_dump.orders (
  id     int not null,
  status varchar(20) not null,
  note   varchar(50)
)`,
	`insert into masume_dump.orders (id, status, note)
  values (1, 'open', 'first'), (2, 'sent', null), (3, 'open', 'a '';'' note')`,
}

// writeDump writes the schema to a file and returns the path and the report.
func writeDump(
	t *testing.T, session db.Session, options dump.Options,
) (string, dump.Report) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.sql")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	report, writeErr := dump.Write(context.Background(), session, options, file)
	if closeErr := file.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if writeErr != nil {
		t.Fatalf("the dump answered %v", writeErr)
	}
	return path, report
}

// readOrders returns the note of every order, by id.
func readOrders(t *testing.T, session db.Session, target string) map[int64]string {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"select id, note from "+target+" order by id", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	held := map[int64]string{}
	for _, row := range answered.Rows {
		held[db.ReadNonNegativeCount(row[0])] = db.ReadAnyText(row[1])
	}
	return held
}

func TestPostgresDumpRunsBackIntoTheServer(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropDumpSchema, dumpSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropDumpSchema, dbtest.ReadEverything, nil)
	})

	path, report := writeDump(t, session, dump.Options{
		Schema: "masume_dump", Content: dump.ContentAll, DropsFirst: true,
	})
	if report.Tables != 2 || report.Rows != 4 {
		t.Fatalf("report: %+v, want two tables and four rows", report)
	}
	if report.Views != 1 || report.Objects != 4 {
		t.Fatalf("report: %+v, want the view, the sequence, both functions and the trigger",
			report)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(written))
	for _, wanted := range []string{
		"create sequence", "create table", "create view", "create trigger",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("the dump holds no %q:\n%s", wanted, written)
		}
	}
	// The parent of a foreign key stands in front of the table that names it.
	if strings.Index(text, "masume_dump.customers (") >
		strings.Index(text, "table \"masume_dump\".\"orders\"") {
		t.Fatalf("the parent table stands after the table that names it:\n%s", written)
	}

	// The dump drops what it makes, so the same file rebuilds the schema it came from,
	// rows and all. It runs twice, because a restore of a restore must hold as well.
	for round := range 2 {
		run, runErr := dump.RunFile(context.Background(), session, path,
			session.Dialect().Syntax)
		if runErr != nil {
			t.Fatalf("round %d answered %v after %d statements",
				round+1, runErr, run.Statements)
		}
		notes := readOrders(t, session, "masume_dump.orders")
		if len(notes) != 3 || notes[3] != "a ';' note" {
			t.Fatalf("round %d orders: %v, want the three rows of the dump", round+1, notes)
		}
	}
}

func TestMysqlDumpRunsBackIntoTheServer(t *testing.T) {
	session := dbtest.Open(t, dbtest.MySQL)
	dbtest.RunStatements(t, session, append([]string{dropDumpDatabase}, dumpDatabase...)...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropDumpDatabase, dbtest.ReadEverything, nil)
	})

	path, report := writeDump(t, session, dump.Options{
		Schema: "masume_dump", Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{
			{Schema: "masume_dump", Name: "orders", Kind: db.RelationTable},
		},
	})
	if report.Tables != 1 || report.Rows != 3 {
		t.Fatalf("report: %+v, want one table and three rows", report)
	}

	run, runErr := dump.RunFile(context.Background(), session, path,
		session.Dialect().Syntax)
	if runErr != nil {
		t.Fatalf("the restore answered %v after %d statements", runErr, run.Statements)
	}
	notes := readOrders(t, session, "masume_dump.orders")
	if len(notes) != 3 || notes[3] != "a ';' note" {
		t.Fatalf("orders: %v, want the three rows of the dump", notes)
	}
}

// The SQL Server schema lives inside the connected database, so the tables are dropped and
// made again rather than a database of their own.
var dropSqlserverTable = []string{
	"drop table if exists dbo.masume_dump_orders",
}

var sqlserverTable = []string{
	`create table dbo.masume_dump_orders (
  id     int not null,
  status varchar(20) not null,
  note   varchar(50) null
)`,
	`insert into dbo.masume_dump_orders (id, status, note)
  values (1, 'open', 'first'), (2, 'sent', null), (3, 'open', 'a '';'' note')`,
}

func TestSqlserverDumpRunsBackIntoTheServer(t *testing.T) {
	session := dbtest.Open(t, dbtest.Sqlserver)
	dbtest.RunStatements(t, session, append(dropSqlserverTable, sqlserverTable...)...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropSqlserverTable[0], dbtest.ReadEverything, nil)
	})

	table := db.TableRef{Schema: "dbo", Name: "masume_dump_orders", Kind: db.RelationTable}
	path, report := writeDump(t, session, dump.Options{
		Schema: "dbo", Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{table},
	})
	if report.Tables != 1 || report.Rows != 3 {
		t.Fatalf("report: %+v, want one table and three rows", report)
	}

	run, runErr := dump.RunFile(context.Background(), session, path,
		session.Dialect().Syntax)
	if runErr != nil {
		t.Fatalf("the restore answered %v after %d statements", runErr, run.Statements)
	}
	notes := readOrders(t, session, "dbo.masume_dump_orders")
	if len(notes) != 3 || notes[3] != "a ';' note" {
		t.Fatalf("orders: %v, want the three rows of the dump", notes)
	}
}

// ClickHouse holds no transaction, so the statements of a restore stand on their own.
var dropClickhouseTable = []string{"drop table if exists masume_dump_orders"}

var clickhouseTable = []string{
	`create table masume_dump_orders (
  id     Int32,
  status String,
  note   Nullable(String)
) engine = MergeTree order by id`,
	`insert into masume_dump_orders (id, status, note)
  values (1, 'open', 'first'), (2, 'sent', null), (3, 'open', 'a '';'' note')`,
}

func TestClickhouseDumpRunsBackIntoTheServer(t *testing.T) {
	session := dbtest.Open(t, dbtest.Clickhouse)
	dbtest.RunStatements(t, session, append(dropClickhouseTable, clickhouseTable...)...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropClickhouseTable[0], dbtest.ReadEverything, nil)
	})

	schema := session.Describe().Profile.Database
	table := db.TableRef{Schema: schema, Name: "masume_dump_orders", Kind: db.RelationTable}
	path, report := writeDump(t, session, dump.Options{
		Schema: schema, Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{table},
	})
	if report.Tables != 1 || report.Rows != 3 {
		t.Fatalf("report: %+v, want one table and three rows", report)
	}

	run, runErr := dump.RunFile(context.Background(), session, path,
		session.Dialect().Syntax)
	if runErr != nil {
		t.Fatalf("the restore answered %v after %d statements", runErr, run.Statements)
	}
	notes := readOrders(t, session, "masume_dump_orders")
	if len(notes) != 3 || notes[3] != "a ';' note" {
		t.Fatalf("orders: %v, want the three rows of the dump", notes)
	}
}

// MongoDB reports no definition and holds another language, so neither command runs on it.
func TestMongoIsRefusedByBothCommands(t *testing.T) {
	session := dbtest.Open(t, dbtest.Mongo)
	if session.Capabilities().WritesDDL {
		t.Fatal("this MongoDB reports definitions")
	}

	_, err := dump.Write(context.Background(), session, dump.Options{
		Schema: "shop", Content: dump.ContentAll,
	}, &strings.Builder{})
	if err == nil {
		t.Error("a dump of MongoDB wrote a file")
	}
}

// The values of a table must read back exactly as they were: a dump that changes a moment, a
// blob or a list is worse than one that fails.

const dropTypeSchema = `drop schema if exists masume_values cascade;`

const typeSchema = `
create schema masume_values;
create table masume_values."Odd Name" (
  id           integer primary key,
  amount       numeric(12,4),
  ratio        double precision,
  flag         boolean,
  note         text,
  raw          bytea,
  made_at      timestamp,
  made_at_zone timestamptz,
  day          date,
  clock        time,
  payload      jsonb,
  tags         text[],
  missing      text
);
insert into masume_values."Odd Name" values
  (1, 1234.5678, 0.1, true, 'a ; note', '\x00ff10'::bytea,
   '2026-09-10 14:30:00', '2026-09-10 14:30:00+02', '2026-09-10', '14:30:00',
   '{"a": 1, "c": "x; y"}'::jsonb, array['one','two;three'], null),
  (2, -0.0001, 1e300, false, e'line\nbreak\ttab', '\x'::bytea,
   '0001-01-01 00:00:00', '2026-01-01 00:00:00+00', '0001-01-01', '00:00:00',
   'null'::jsonb, array[]::text[], null);
`

// A MySQL-protocol connection lists the database it opened alone, so the table stands in the
// database of the test connection.
const dropTypeDatabase = "drop table if exists `Odd Name`"

var typeDatabase = []string{
	"create table `Odd Name` (\n" +
		"  id int primary key, amount decimal(12,4), ratio double, flag tinyint(1),\n" +
		"  note text, raw blob, made_at datetime, day date, clock time, payload json,\n" +
		"  kind enum('one','two'), bits bit(8), missing varchar(10))",
	"insert into `Odd Name` values (1, 1234.5678, 0.1, 1, 'a ; note', " +
		"x'00ff10', '2026-09-10 14:30:00', '2026-09-10', '14:30:00', " +
		`'{"a": 1, "c": "x; y"}', 'two', b'10101010', null)`,
}

// readEveryCell returns every cell of the relation as the client formats it.
func readEveryCell(t *testing.T, session db.Session, target string) []string {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"select * from "+target+" order by id", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	held := []string{}
	for _, row := range answered.Rows {
		for at, value := range row {
			dataType := ""
			if at < len(answered.Columns) {
				dataType = answered.Columns[at].DataType
			}
			held = append(held, core.FormatCell(value, dataType))
		}
	}
	return held
}

// checkValuesSurviveTheRoundTrip dumps the relation, runs the dump back and compares every
// cell with what stood there before.
func checkValuesSurviveTheRoundTrip(
	t *testing.T, session db.Session, options dump.Options, target string,
) {
	t.Helper()
	before := readEveryCell(t, session, target)
	if len(before) == 0 {
		t.Fatal("the relation holds no cell to compare")
	}

	path, _ := writeDump(t, session, options)
	run, err := dump.RunFile(context.Background(), session, path, session.Dialect().Syntax)
	if err != nil {
		t.Fatalf("the restore answered %v after %d statements", err, run.Statements)
	}

	after := readEveryCell(t, session, target)
	if len(after) != len(before) {
		t.Fatalf("%d cells came back, want %d", len(after), len(before))
	}
	for at, held := range before {
		if after[at] != held {
			t.Errorf("cell %d: %q, want %q", at, after[at], held)
		}
	}
}

func TestPostgresValuesSurviveTheRoundTrip(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropTypeSchema, typeSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropTypeSchema, dbtest.ReadEverything, nil)
	})

	checkValuesSurviveTheRoundTrip(t, session, dump.Options{
		Schema: "masume_values", Content: dump.ContentAll, DropsFirst: true,
	}, `masume_values."Odd Name"`)
}

func TestMysqlValuesSurviveTheRoundTrip(t *testing.T) {
	session := dbtest.Open(t, dbtest.MySQL)
	dbtest.RunStatements(t, session, append([]string{dropTypeDatabase}, typeDatabase...)...)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropTypeDatabase, dbtest.ReadEverything, nil)
	})

	schema := session.Describe().Profile.Database
	checkValuesSurviveTheRoundTrip(t, session, dump.Options{
		Schema: schema, Content: dump.ContentAll, DropsFirst: true,
		Tables: []db.TableRef{
			{Schema: schema, Name: "Odd Name", Kind: db.RelationTable},
		},
	}, "`Odd Name`")
}

// A dump of a table larger than one batch reports its progress as it goes, so a card can
// draw how far it has come.
func TestPostgresDumpReportsItsProgress(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropDumpSchema, `
create schema masume_dump;
create table masume_dump.many (id integer primary key);
insert into masume_dump.many select generate_series(1, 1500);
`)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropDumpSchema, dbtest.ReadEverything, nil)
	})

	held := []dump.Progress{}
	if _, err := dump.Write(context.Background(), session, dump.Options{
		Schema: "masume_dump", Content: dump.ContentAll,
		OnProgress: func(progress dump.Progress) { held = append(held, progress) },
	}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}

	// 1500 rows are read in batches of 500, so the rows move more than once.
	if len(held) < 4 {
		t.Fatalf("reports: %d, want one per batch and one per table", len(held))
	}
	last := held[len(held)-1]
	if last.Rows != 1500 || last.Tables != 1 || last.OfTables != 1 {
		t.Fatalf("the last report is %+v, want every row of the one table", last)
	}
	moved := false
	for _, report := range held {
		if report.Rows > 0 && report.Rows < 1500 {
			moved = true
		}
	}
	if !moved {
		t.Errorf("the rows never moved: %+v", held)
	}
}
