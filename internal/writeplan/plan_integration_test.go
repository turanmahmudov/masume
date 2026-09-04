//go:build integration

// An integration test: it measures a write against a real PostgreSQL, named through
// MASUME_TEST_POSTGRES.
package writeplan_test

import (
	"context"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

const dropPlanSchema = `drop schema if exists masume_plan cascade;`

const planSchema = `
create schema masume_plan;
create table masume_plan.orders (
  id     serial primary key,
  status text not null,
  total  numeric(10,2) default 0
);
create table masume_plan.lines (
  id       serial primary key,
  order_id integer references masume_plan.orders (id) on delete cascade
);
create table masume_plan.notes (
  id       serial primary key,
  order_id integer references masume_plan.orders (id) on delete restrict
);
create function masume_plan.note_order() returns trigger as $$ begin return new; end; $$
  language plpgsql;
create trigger t_order_audit after update on masume_plan.orders
  for each row execute function masume_plan.note_order();
insert into masume_plan.orders (status, total)
  values ('open', 1), ('open', 2), ('sent', 3), ('sent', 4);
insert into masume_plan.lines (order_id) values (1), (1), (2);
insert into masume_plan.notes (order_id) values (1);
`

var planTable = db.TableRef{Schema: "masume_plan", Name: "orders", Kind: db.RelationTable}

// openPlanShop answers a session with four orders, three lines that cascade, and a trigger
// that runs on an update.
func openPlanShop(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropPlanSchema, planSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), dropPlanSchema, dbtest.ReadEverything, nil)
	})
	return session
}

// measure builds the plan of one write against that server.
func measure(t *testing.T, session db.Session, sql string) writeplan.Plan {
	t.Helper()
	plan, measured := writeplan.Build(context.Background(), session, writeplan.Request{
		SQL: sql, Tables: []db.TableRef{planTable}, Mode: cfg.PlanUndo,
		UndoRows: cfg.DefaultUndoRows,
	})
	if !measured {
		t.Fatalf("%q was not measured", sql)
	}
	return plan
}

// readStatus returns the status of every order, by id.
func readStatus(t *testing.T, session db.Session) map[int64]string {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"select id, status from masume_plan.orders order by id", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	held := map[int64]string{}
	for _, row := range answered.Rows {
		held[db.ReadNonNegativeCount(row[0])] = db.ReadAnyText(row[1])
	}
	return held
}

func TestServerCountsTheRowsAndTheRelation(t *testing.T) {
	session := openPlanShop(t)
	plan := measure(t, session,
		"update masume_plan.orders set status = 'sent' where status = 'open'")

	if !plan.HasRows || plan.Rows != 2 {
		t.Errorf("the plan counted %d rows", plan.Rows)
	}
	if !plan.HasTotal || plan.Total != 4 {
		t.Errorf("the relation counted %d rows", plan.Total)
	}
	if share, held := plan.ReadShare(); !held || share != 0.5 {
		t.Errorf("the share is %v, present %v", share, held)
	}
}

func TestServerReportsTheTriggerAndTheCascade(t *testing.T) {
	session := openPlanShop(t)

	updated := measure(t, session,
		"update masume_plan.orders set status = 'sent' where id = 1")
	if len(updated.Cascades) != 1 ||
		updated.Cascades[0].Reason != "trigger t_order_audit" {
		t.Fatalf("the update reports %+v", updated.Cascades)
	}

	// The trigger runs on an update, so it is no cascade of a delete.
	removed := measure(t, session, "delete from masume_plan.orders where id = 1")
	if len(removed.Cascades) != 1 || removed.Cascades[0].Table != "masume_plan.lines" {
		t.Fatalf("the delete reports %+v", removed.Cascades)
	}
	if !removed.Cascades[0].HasRows || removed.Cascades[0].Rows != 2 {
		t.Errorf("the cascade counted %d rows, present %v",
			removed.Cascades[0].Rows, removed.Cascades[0].HasRows)
	}
	if len(removed.Blockers) != 1 || removed.Blockers[0].Rows != 1 {
		t.Errorf("the delete blockers are %+v", removed.Blockers)
	}
}

// The server rejects the delete while the restricting relation references the rows, and the
// plan says so before it runs.
func TestServerReportsTheKeyThatBlocksTheDelete(t *testing.T) {
	session := openPlanShop(t)

	written := "delete from masume_plan.orders where id = 1"
	plan := measure(t, session, written)
	if len(plan.Blockers) != 1 || plan.Blockers[0].Table != "masume_plan.notes" {
		t.Fatalf("the plan reports %+v", plan.Blockers)
	}

	_, err := session.RunQuery(context.Background(), written, dbtest.ReadEverything, nil)
	if err == nil {
		t.Error("the server took a delete the plan said it would block")
	}
}

// The write runs, and the undo read inside its transaction takes the rows back to what
// that write found.
func TestTheUndoOfAnUpdatePutsTheRowsBack(t *testing.T) {
	session := openPlanShop(t)
	before := readStatus(t, session)

	written := "update masume_plan.orders set status = 'sent' where status = 'open'"
	plan := measure(t, session, written)
	if !plan.Undo.Kept {
		t.Fatalf("no undo was planned: %s", plan.Undo.Reason)
	}

	_, undo := runPlanned(t, session, plan, written)
	if !undo.IsHeld() || undo.Rows != 2 {
		t.Fatalf("the undo holds %d rows: %s", undo.Rows, undo.Reason)
	}
	if changed := readStatus(t, session); changed[1] != "sent" {
		t.Fatalf("the write left order 1 as %q", changed[1])
	}

	if err := session.ApplyChanges(context.Background(), undo.Changes); err != nil {
		t.Fatalf("the undo answered %v", err)
	}
	after := readStatus(t, session)
	for id, status := range before {
		if after[id] != status {
			t.Errorf("order %d is %q, was %q before the write", id, after[id], status)
		}
	}
}

func TestTheUndoOfADeletePutsTheRowsBack(t *testing.T) {
	session := openPlanShop(t)

	written := "delete from masume_plan.orders where status = 'sent'"
	plan := measure(t, session, written)
	_, undo := runPlanned(t, session, plan, written)
	if !undo.IsHeld() || undo.Rows != 2 {
		t.Fatalf("the undo holds %d rows: %s", undo.Rows, undo.Reason)
	}
	if len(readStatus(t, session)) != 2 {
		t.Fatal("the delete left the wrong number of rows")
	}

	if err := session.ApplyChanges(context.Background(), undo.Changes); err != nil {
		t.Fatalf("the undo answered %v", err)
	}
	after := readStatus(t, session)
	if len(after) != 4 || after[3] != "sent" || after[4] != "sent" {
		t.Errorf("the relation holds %v", after)
	}
}

// The rows of the undo are held from the read until the write commits. Without that hold a
// second session can move a row out of the predicate between the two, and the undo then
// covers a row the write never changed: restoring it would revert the other session.
func TestTheUndoCoversTheRowsTheWriteChanged(t *testing.T) {
	session := openPlanShop(t)
	other := dbtest.Open(t, dbtest.Postgres)

	written := "update masume_plan.orders set status = 'sent' where status = 'open'"
	plan := measure(t, session, written)

	// The second session takes one row of the plan out of the predicate, between the read
	// of the undo and the write.
	waiting := make(chan error, 1)
	result, undo, err := writeplan.RunWithUndo(context.Background(), session, plan.Undo,
		func(running context.Context) (db.QueryResult, error) {
			go func() {
				_, err := other.RunQuery(context.Background(),
					"update masume_plan.orders set status = 'taken' where id = 1",
					dbtest.ReadEverything, nil)
				waiting <- err
			}()
			time.Sleep(300 * time.Millisecond)
			return session.ReadPage(running,
				db.ComposedRead{Text: written}, db.ReadWindow{Limit: 100})
		})
	if err != nil {
		t.Fatalf("the write answered %v", err)
	}
	if writeErr := <-waiting; writeErr != nil {
		t.Fatalf("the second session answered %v", writeErr)
	}

	if !result.HasAffected {
		t.Fatal("the server reported no affected rows")
	}
	if result.Affected != int64(undo.Rows) {
		t.Errorf("the write changed %d rows and the undo covers %d",
			result.Affected, undo.Rows)
	}
}

// runPlanned runs the write and its undo as the client runs them.
func runPlanned(
	t *testing.T, session db.Session, plan writeplan.Plan, written string,
) (db.QueryResult, writeplan.Undo) {
	t.Helper()
	result, undo, err := writeplan.RunWithUndo(context.Background(), session, plan.Undo,
		func(running context.Context) (db.QueryResult, error) {
			return session.ReadPage(running,
				db.ComposedRead{Text: written}, db.ReadWindow{Limit: 100})
		})
	if err != nil {
		t.Fatalf("the write answered %v", err)
	}
	return result, undo
}
