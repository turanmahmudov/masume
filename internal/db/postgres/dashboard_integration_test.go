//go:build integration

// An integration test for what the dashboard reads of the server itself: its sessions, the
// sessions waiting for a lock, and the load it is under. These read a real PostgreSQL,
// because nothing in the unit suite can prove that this SQL parses or that the columns it
// names exist.
package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/db/postgres"
)

const dropLockSchema = `drop schema if exists masume_locks cascade;`

const lockSchema = `
create schema masume_locks;
create table masume_locks.orders (
  id    serial primary key,
  total integer not null
);
insert into masume_locks.orders (total) values (1), (2);
`

func openLockTable(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session, dropLockSchema, lockSchema)
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(), dropLockSchema, dbtest.ReadEverything, nil)
	})
	return session
}

// The activity list is the oldest part of this port and had no test against a server. The
// statement names nine columns of pg_stat_activity, and a name that is wrong is a fault the
// unit suite cannot see.
func TestServerListsItsOwnSessions(t *testing.T) {
	session := openLockTable(t)
	other := dbtest.Open(t, dbtest.Postgres)

	// The other session runs something, so there is a session to find.
	if _, err := other.RunQuery(context.Background(),
		"select pg_sleep(0)", dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the second session ran nothing: %v", err)
	}

	sessions, err := session.ListActivity(context.Background())
	if err != nil {
		t.Fatalf("the server did not list its sessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("the server listed no session, and a second one is connected")
	}
	for _, held := range sessions {
		if held.PID <= 0 {
			t.Errorf("a session was listed with no PID: %+v", held)
		}
		if held.User == "" {
			t.Errorf("session %d was listed with no user", held.PID)
		}
	}
}

// The server reports the load through three values in one statement. A count, a setting read
// as a number, and a timestamp: each is a shape the driver has to hand back.
func TestServerReportsTheLoadItIsUnder(t *testing.T) {
	session := openLockTable(t)

	load, err := session.ReadServerLoad(context.Background())
	if err != nil {
		t.Fatalf("the server did not report its load: %v", err)
	}
	if load.Connections <= 0 {
		t.Errorf("the server reports %d connections while this one is open",
			load.Connections)
	}
	if load.MaxConnections <= 0 {
		t.Errorf("the server reports a limit of %d connections", load.MaxConnections)
	}
	if load.Connections > load.MaxConnections {
		t.Errorf("the server reports %d connections of %d, which cannot be",
			load.Connections, load.MaxConnections)
	}
	if load.StartedAt.IsZero() {
		t.Error("the server did not say when it started")
	}
	if load.StartedAt.After(time.Now()) {
		t.Errorf("the server says it started at %s, which is ahead of now", load.StartedAt)
	}
}

// A server with nothing blocked answers an empty list rather than a fault. The dashboard
// draws no panel for it, so an error here would report a fault every two seconds on a server
// that is perfectly healthy.
func TestServerReportsNoLockWaitWhereNothingIsBlocked(t *testing.T) {
	session := openLockTable(t)

	waits, err := session.ListLockWaits(context.Background())
	if err != nil {
		t.Fatalf("the server did not report its lock waits: %v", err)
	}
	for _, wait := range waits {
		// Another test of this package may hold a lock while this one runs, so the list
		// is not asserted to be empty. Nothing here may be a wait of this session.
		if wait.BlockedPID == 0 || wait.BlockingPID == 0 {
			t.Errorf("a wait was reported with no session: %+v", wait)
		}
	}
}

// The one case the panel exists for: one session holds a lock and another waits for it. It
// needs two connections and a deliberate block, so only a real server can prove it.
func TestServerReportsWhichSessionWaitsForALock(t *testing.T) {
	session := openLockTable(t)
	holder := dbtest.Open(t, dbtest.Postgres)
	waiter := dbtest.Open(t, dbtest.Postgres)

	ctx := context.Background()
	if err := holder.BeginTransaction(ctx); err != nil {
		t.Fatalf("the holder opened no transaction: %v", err)
	}
	t.Cleanup(func() { _ = holder.RollbackTransaction(context.Background()) })
	if _, err := holder.RunQuery(ctx,
		"lock table masume_locks.orders in access exclusive mode",
		dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the holder took no lock: %v", err)
	}

	// The waiter blocks until the holder rolls back, so it runs on its own and this test
	// waits for the server to report it. It is a write and not a LOCK statement, because
	// PostgreSQL takes a LOCK only inside a transaction block and this one runs alone.
	blocked := make(chan error, 1)
	go func() {
		_, err := waiter.RunQuery(context.Background(),
			"update masume_locks.orders set total = total + 1",
			dbtest.ReadEverything, nil)
		blocked <- err
	}()

	var found db.LockWait
	for range 100 {
		waits, err := session.ListLockWaits(ctx)
		if err != nil {
			t.Fatalf("the server did not report its lock waits: %v", err)
		}
		for _, wait := range waits {
			if strings.Contains(wait.BlockedQuery, "update masume_locks.orders") {
				found = wait
			}
		}
		if found.BlockedPID != 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if found.BlockedPID == 0 {
		t.Fatal("the server reported no session waiting for the lock that is held")
	}
	if found.BlockingPID == 0 {
		t.Error("the wait names no session as holding the lock")
	}
	if found.BlockedPID == found.BlockingPID {
		t.Errorf("session %d is reported as waiting for itself", found.BlockedPID)
	}
	if found.Mode == "" {
		t.Error("the wait names no lock mode, so the panel has nothing to draw")
	}
	if found.Relation != "orders" {
		t.Errorf("the wait is on relation %q, wanted the orders table", found.Relation)
	}
	if !strings.Contains(found.BlockingQuery, "masume_locks.orders") {
		t.Errorf("the holder is reported as running %q", found.BlockingQuery)
	}
	if found.Waiting < 0 || found.BlockingFor < 0 {
		t.Errorf("the wait reports a time before it started: %+v", found)
	}

	// The holder lets go, so the waiter finishes rather than being left on the server.
	if err := holder.RollbackTransaction(ctx); err != nil {
		t.Fatalf("the holder did not roll back: %v", err)
	}
	select {
	case err := <-blocked:
		if err != nil {
			t.Errorf("the waiter failed once the lock was free: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Error("the waiter never finished after the lock was freed")
	}
}

// The kill this card offers has two shapes and neither had a test against a server. A
// cancel ends the statement and leaves the connection; ending the session closes it as
// well. Both go through pg_cancel_backend and pg_terminate_backend, and only one of the two
// was ever passed by this client.
//
// The session to stop is found by a marker in the statement it ran, because a server shared
// with the other tests of this package holds sessions that are none of this test's business.
func TestServerStopsAnotherSessionBothWays(t *testing.T) {
	for _, held := range []struct {
		name   string
		marker string
		ends   bool
	}{
		{"cancel the statement", "masume-cancel-marker", false},
		{"end the session", "masume-end-marker", true},
	} {
		t.Run(held.name, func(t *testing.T) {
			session := openLockTable(t)
			other := dbtest.Open(t, dbtest.Postgres)

			ctx := context.Background()
			if _, err := other.RunQuery(ctx,
				"select 1 /* "+held.marker+" */", dbtest.ReadEverything, nil); err != nil {
				t.Fatalf("the session to stop ran nothing: %v", err)
			}

			pid := findSessionByMarker(t, session, held.marker)
			stopped, stopErr := session.CancelBackend(ctx, pid, held.ends)
			if stopErr != nil {
				t.Fatalf("the server refused to stop session %d: %v", pid, stopErr)
			}
			if !stopped {
				t.Errorf("the server did not stop session %d", pid)
			}
		})
	}
}

// findSessionByMarker returns the PID of the session whose last statement carries the
// marker. The server takes a moment to record it, so the list is read again until it does.
func findSessionByMarker(t *testing.T, session db.Session, marker string) int64 {
	t.Helper()
	for range 100 {
		sessions, err := session.ListActivity(context.Background())
		if err != nil {
			t.Fatalf("the server did not list its sessions: %v", err)
		}
		for _, held := range sessions {
			if strings.Contains(held.Query, marker) {
				return held.PID
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no session of the server ran the statement marked %q", marker)
	return 0
}

// The server counts the statements it runs only where the extension that keeps the count is
// installed, so the client asks the connection rather than the engine. The test server loads
// it, which is what compose.yaml sets up.
func TestServerReportsTheStatementsItSpendsItsTimeIn(t *testing.T) {
	ctx := context.Background()
	session := openCountingSession(t, ctx)

	admin, holds := session.(db.ServerAdmin)
	if !holds {
		t.Fatal("a PostgreSQL session does not reach the server itself")
	}

	// A statement of its own, so the count has something of this test in it. The name is
	// on the column and not in a value: the server replaces the values of a statement
	// with marks before it counts it, so a value would not be there to find.
	marker := "select pg_sleep(0.05) as masume_statement_probe"
	for range 2 {
		if _, err := session.RunQuery(ctx, marker, dbtest.ReadEverything, nil); err != nil {
			t.Fatalf("the probe statement does not run: %v", err)
		}
	}

	held, err := admin.ListSlowStatements(ctx, 50)
	if err != nil {
		t.Fatalf("the server does not report its statements: %v", err)
	}
	if len(held) == 0 {
		t.Fatal("the server reports no statement at all")
	}

	// The slowest by mean time comes first, which is the order the panel draws.
	for at := 1; at < len(held); at++ {
		if held[at].MeanTime > held[at-1].MeanTime {
			t.Fatalf("row %d is slower than the one before it, so the order is wrong", at)
		}
	}

	found := false
	for _, one := range held {
		if !strings.Contains(one.Query, "masume_statement_probe") {
			continue
		}
		found = true
		if one.Calls < 2 {
			t.Errorf("the probe ran twice and the server counted %d", one.Calls)
		}
		if one.MeanTime <= 0 {
			t.Error("the probe took no time at all")
		}
	}
	if !found {
		t.Error("the server counted no run of the probe statement")
	}

	// The dashboard reads the server every two seconds, so a panel of its own reads carries
	// nothing about the database. Every read it makes holds the mark.
	for _, one := range held {
		if strings.Contains(one.Query, postgres.DashboardMark) {
			t.Errorf("the panel reports its own read: %q", one.Query)
		}
	}
}

// A limit of nothing asks for nothing rather than for every statement the server holds.
func TestServerReportsNoStatementForALimitOfNothing(t *testing.T) {
	ctx := context.Background()
	session := openCountingSession(t, ctx)
	admin, holds := session.(db.ServerAdmin)
	if !holds {
		t.Fatal("a PostgreSQL session does not reach the server itself")
	}

	held, err := admin.ListSlowStatements(ctx, 0)
	if err != nil {
		t.Fatalf("a limit of nothing answered %v", err)
	}
	if len(held) != 0 {
		t.Errorf("a limit of nothing answered %d statements", len(held))
	}
}

// openCountingSession returns a session on a server that counts the statements it runs. The
// extension that keeps the count is created first, and the session is opened after it: the
// client asks the server for the extension once, when the connection opens, so a session
// opened before it would report that the server counts nothing.
//
// A server that cannot load the extension skips the test rather than failing it. It has to
// be in shared_preload_libraries, which compose.yaml sets for the test server.
func openCountingSession(t *testing.T, ctx context.Context) db.Session {
	t.Helper()
	setup := dbtest.Open(t, dbtest.Postgres)
	if _, err := setup.RunQuery(
		ctx, "create extension if not exists pg_stat_statements", 1, nil); err != nil {
		t.Skipf("this server cannot count its statements: %v", err)
	}

	session := dbtest.Open(t, dbtest.Postgres)
	if !session.Capabilities().ReportsStatementStats {
		t.Fatal("the extension is there and the connection reports no count")
	}
	return session
}

// The dashboard reads the server every two seconds, so its own reads would fill the panel
// of the statements the server spends its time in. Its reads carry a mark and nothing else
// does, so a reader's own statement about the statistics is still counted.
func TestServerCountsAReadersOwnStatisticsStatement(t *testing.T) {
	ctx := context.Background()
	session := openCountingSession(t, ctx)
	admin, holds := session.(db.ServerAdmin)
	if !holds {
		t.Fatal("a PostgreSQL session does not reach the server itself")
	}

	// A statement of the reader that names the statistics, which the panel must keep. It
	// reads through a view of its own, because the server counts a statement by the shape
	// of its parse tree and a column alias is not part of that: two reads that differ
	// only by an alias are counted as one, and the first text seen is the one kept.
	view := "masume_reader_statistics"
	for _, written := range []string{
		"drop view if exists " + view,
		"create view " + view + " as select pid from pg_stat_activity",
	} {
		if _, err := session.RunQuery(ctx, written, 1, nil); err != nil {
			t.Fatalf("the view does not build: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(), "drop view if exists "+view, 1, nil)
	})
	if _, err := session.RunQuery(
		ctx, "select count(*) from "+view, dbtest.ReadEverything, nil); err != nil {
		t.Fatalf("the statement does not run: %v", err)
	}
	// A read of the dashboard itself, which the panel must leave out.
	if _, err := admin.ListActivity(ctx); err != nil {
		t.Fatalf("the activity does not read: %v", err)
	}

	held, err := admin.ListSlowStatements(ctx, 200)
	if err != nil {
		t.Fatalf("the server does not report its statements: %v", err)
	}

	keptTheReaders, keptItsOwn := false, false
	for _, one := range held {
		if strings.Contains(one.Query, view) {
			keptTheReaders = true
		}
		if strings.Contains(one.Query, postgres.DashboardMark) {
			keptItsOwn = true
		}
	}
	if !keptTheReaders {
		t.Error("a statement of the reader about the statistics was left out")
	}
	if keptItsOwn {
		t.Error("the panel reports a read the dashboard made itself")
	}
}
