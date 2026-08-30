//go:build integration

// An integration test for the count that guards a write on a table with no key. A table
// like that can hold the same row twice, and the key of such a row is the whole row, so a
// write would otherwise take every copy of it. These tests read a real PostgreSQL.
package postgres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
)

const dropKeylessSchema = `drop schema if exists masume_guard cascade;`

// The table has no primary key, and it holds the same row twice on purpose.
const keylessSchema = `
create schema masume_guard;
create table masume_guard.readings (
  sensor text not null,
  value  integer not null
);
insert into masume_guard.readings (sensor, value)
  values ('a', 1), ('a', 1), ('b', 2);
`

func openKeylessTable(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropKeylessSchema, keylessSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(), dropKeylessSchema, dbtest.ReadEverything, nil)
	})
	return session
}

// buildKeylessTarget answers the target of the readings table, with no key columns, and the
// rows as the grid holds them.
func buildKeylessTarget(rows [][]any) db.ChangeTarget {
	return db.ChangeTarget{
		Table: db.TableRef{Schema: "masume_guard", Name: "readings"},
		Columns: []db.ResultColumn{
			{Name: "sensor", DataType: "text"},
			{Name: "value", DataType: "integer"},
		},
		Rows: rows,
	}
}

// countReadings answers how many rows of the table match the sensor.
func countReadings(t *testing.T, session db.Session, sensor string) int64 {
	t.Helper()
	result, err := session.RunQuery(context.Background(),
		"select count(*)::int8 from masume_guard.readings where sensor = '"+sensor+"'",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("cannot count the rows: %v", err)
	}
	if len(result.Rows) == 0 || len(result.Rows[0]) == 0 {
		t.Fatal("the count answered no rows")
	}
	return db.ReadNonNegativeCount(result.Rows[0][0])
}

// An update to one of two identical rows would take both, so it is refused and nothing is
// written. This is the whole reason the count exists.
func TestAnUpdateThatWouldTakeTwoIdenticalRowsIsRefused(t *testing.T) {
	session := openKeylessTable(t)

	// The grid holds one of the two ('a', 1) rows.
	target := buildKeylessTarget([][]any{{"a", int64(1)}})
	pending := core.NewPendingChanges()
	pending.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1,
		Value: core.CellValue{Kind: core.CellText, Text: "99"},
	}

	changes, err := session.Composer().BuildChanges(target, pending)
	if err != nil {
		t.Fatalf("the changes could not be built: %v", err)
	}

	err = session.ApplyChanges(context.Background(), changes)
	if err == nil {
		t.Fatal("the update ran, and it would have taken both copies of the row")
	}
	if !strings.Contains(err.Error(), "matches 2 rows") {
		t.Errorf("the error is %q, wanted it to name the rows it matched", err.Error())
	}

	// Nothing was written, so both rows still hold 1.
	if held := countReadings(t, session, "a"); held != 2 {
		t.Errorf("the table now holds %d rows for sensor a, wanted both left alone", held)
	}
	result, err := session.RunQuery(context.Background(),
		"select count(*)::int8 from masume_guard.readings where sensor = 'a' and value = 1",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("cannot read the rows back: %v", err)
	}
	if unchanged := db.ReadNonNegativeCount(result.Rows[0][0]); unchanged != 2 {
		t.Errorf("%d of the two rows still hold 1, wanted both", unchanged)
	}
}

// A row that is the only one of its kind matches once, so the write runs.
func TestAnUpdateToARowThatIsTheOnlyOneOfItsKindRuns(t *testing.T) {
	session := openKeylessTable(t)

	target := buildKeylessTarget([][]any{{"b", int64(2)}})
	pending := core.NewPendingChanges()
	pending.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1,
		Value: core.CellValue{Kind: core.CellText, Text: "42"},
	}

	changes, err := session.Composer().BuildChanges(target, pending)
	if err != nil {
		t.Fatalf("the changes could not be built: %v", err)
	}
	if err := session.ApplyChanges(context.Background(), changes); err != nil {
		t.Fatalf("the update was refused, and the row is the only one of its kind: %v", err)
	}

	result, err := session.RunQuery(context.Background(),
		"select count(*)::int8 from masume_guard.readings where sensor = 'b' and value = 42",
		dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("cannot read the row back: %v", err)
	}
	if written := db.ReadNonNegativeCount(result.Rows[0][0]); written != 1 {
		t.Errorf("%d rows hold the new value, wanted 1", written)
	}
}

// A delete of one of two identical rows would take both, so it is refused.
func TestADeleteThatWouldTakeTwoIdenticalRowsIsRefused(t *testing.T) {
	session := openKeylessTable(t)

	target := buildKeylessTarget([][]any{{"a", int64(1)}})
	pending := core.NewPendingChanges()
	pending.DeletedRows[0] = true

	changes, err := session.Composer().BuildChanges(target, pending)
	if err != nil {
		t.Fatalf("the changes could not be built: %v", err)
	}

	if err := session.ApplyChanges(context.Background(), changes); err == nil {
		t.Fatal("the delete ran, and it would have taken both copies of the row")
	}
	if held := countReadings(t, session, "a"); held != 2 {
		t.Errorf("the table now holds %d rows for sensor a, wanted both left alone", held)
	}
}

// Choosing both copies is what the user asked for, so the delete runs.
func TestADeleteOfBothIdenticalRowsRuns(t *testing.T) {
	session := openKeylessTable(t)

	target := buildKeylessTarget([][]any{{"a", int64(1)}, {"a", int64(1)}})
	pending := core.NewPendingChanges()
	pending.DeletedRows[0] = true
	pending.DeletedRows[1] = true

	changes, err := session.Composer().BuildChanges(target, pending)
	if err != nil {
		t.Fatalf("the changes could not be built: %v", err)
	}
	if err := session.ApplyChanges(context.Background(), changes); err != nil {
		t.Fatalf("the delete was refused, and both rows were chosen: %v", err)
	}
	if held := countReadings(t, session, "a"); held != 0 {
		t.Errorf("the table still holds %d rows for sensor a, wanted none", held)
	}
}

// A table with a primary key identifies one row, so the write carries no count and runs.
func TestAWriteOnAKeyedTableRunsWithoutACount(t *testing.T) {
	session := openShop(t)

	target := db.ChangeTarget{
		Table: db.TableRef{Schema: "masume_test", Name: "orders"},
		Columns: []db.ResultColumn{
			{Name: "id", DataType: "integer"},
			{Name: "customer", DataType: "text"},
		},
		Rows:       [][]any{{int64(1), "ada"}},
		KeyColumns: []string{"id"},
	}
	pending := core.NewPendingChanges()
	pending.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1,
		Value: core.CellValue{Kind: core.CellText, Text: "grace"},
	}

	changes, err := session.Composer().BuildChanges(target, pending)
	if err != nil {
		t.Fatalf("the changes could not be built: %v", err)
	}
	if changes[0].Guard != nil {
		t.Error("the write carries a count, and the table has a primary key")
	}
	if err := session.ApplyChanges(context.Background(), changes); err != nil {
		t.Fatalf("the update was refused: %v", err)
	}
}
