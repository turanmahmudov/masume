//go:build integration

// An integration test: it reads a real Redis. The server is started outside this code and
// named through MASUME_TEST_REDIS. Nothing here knows how it was started.
//
// Redis takes commands rather than SQL, so a read is a command and the "relations" the
// catalog answers are the prefixes the keys fall into.
package redis_test

import (
	"context"
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/db/redis"
)

// openKeys answers a session on an empty database with a few keys written.
func openKeys(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Redis)
	dbtest.RunStatements(t, session,
		"FLUSHDB",
		"SET order:1 ada",
		"SET order:2 grace",
		"SET customer:1 alan",
	)
	t.Cleanup(func() {
		_, _ = session.RunQuery(context.Background(), "FLUSHDB", dbtest.ReadEverything, nil)
	})
	return session
}

func TestServerRunsACommandAndAnswersIt(t *testing.T) {
	session := openKeys(t)

	answered, err := session.RunQuery(context.Background(),
		"GET order:1", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the command answered %v", err)
	}
	if len(answered.Rows) != 1 || len(answered.Rows[0]) == 0 {
		t.Fatalf("the command gave %d rows, wanted one value", len(answered.Rows))
	}
	if held := db.ReadAnyText(answered.Rows[0][0]); held != "ada" {
		t.Errorf("the value reads %q, wanted ada", held)
	}
}

func TestServerCountsTheKeysItHolds(t *testing.T) {
	session := openKeys(t)

	answered, err := session.RunQuery(context.Background(), "DBSIZE", dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if held := db.ReadNonNegativeCount(answered.Rows[0][0]); held != 3 {
		t.Errorf("the database holds %d keys, wanted the 3 written", held)
	}
}

// The catalog of a key store is the prefixes its keys fall into, so a keyspace reads like a
// list of relations in the tree.
func TestServerListsThePrefixesOfItsKeys(t *testing.T) {
	session := openKeys(t)

	tables, err := session.ListTables(context.Background())
	if err != nil {
		t.Fatalf("the catalog answered %v", err)
	}
	names := map[string]bool{}
	for _, table := range tables {
		names[table.Name] = true
	}
	if !names["order"] || !names["customer"] {
		t.Errorf("the prefixes read %v, wanted order and customer among them", names)
	}
}

// Redis has no transaction the client drives, and the port must say so rather than pretend.
func TestServerReportsWhatItCannotDo(t *testing.T) {
	session := dbtest.Open(t, dbtest.Redis)

	held := session.Capabilities()
	if held.HasTransactions {
		t.Error("redis reports transactions the client can drive")
	}
	if held.PlansStatement {
		t.Error("redis reports a plan for a statement")
	}
	if err := session.BeginTransaction(context.Background()); err == nil {
		t.Error("a transaction opened on a server that has none")
	}
}

func TestServerAnswersACommandItRefuses(t *testing.T) {
	session := openKeys(t)

	_, err := session.RunQuery(context.Background(),
		"NOTACOMMAND order:1", dbtest.ReadEverything, nil)
	if err == nil {
		t.Fatal("a command that does not exist answered no error")
	}
	if described := db.DescribeError(err); described == "" {
		t.Error("the error is described as an empty text")
	}
}

// The staged work of the grid goes inside a MULTI, so the server runs the whole set with
// nothing of another connection in between, and a set holding a change it cannot read is
// never sent at all.
func TestApplyChangesSendsTheStagedSetAsOne(t *testing.T) {
	session := openKeys(t)
	ctx := context.Background()

	if err := session.ApplyChanges(ctx, []db.Change{
		{Description: "set order:1", Payload: redis.RedisCommand{
			Name: "SET", Args: []string{"order:1", "written"}}},
		{Description: "set order:2", Payload: redis.RedisCommand{
			Name: "SET", Args: []string{"order:2", "written too"}}},
	}); err != nil {
		t.Fatalf("the staged set failed: %v", err)
	}
	if held := readKeyValue(t, session, "order:1"); held != "written" {
		t.Errorf("order:1 reads %q after the set", held)
	}

	// A set that holds a change built by another engine is refused before anything is
	// sent, so the first command of it never reaches the server.
	if err := session.ApplyChanges(ctx, []db.Change{
		{Description: "set order:1", Payload: redis.RedisCommand{
			Name: "SET", Args: []string{"order:1", "never sent"}}},
		{Description: "not a command", Payload: "wrong"},
	}); err == nil {
		t.Error("a set holding a change of another engine was accepted")
	}
	if held := readKeyValue(t, session, "order:1"); held != "written" {
		t.Errorf("order:1 reads %q, so a refused set still reached the server", held)
	}
}

// An export reads what the buffer answered. A command that is no SCAN answers once, and
// exporting the keyspace around it would hand over every key the connection can see.
func TestStreamQueryExportsWhatTheCommandAnswered(t *testing.T) {
	session := openKeys(t)

	rows := 0
	total, err := session.StreamQuery(context.Background(), "GET order:1", nil, 100,
		func(batch [][]any, _ []db.ResultColumn) error {
			rows += len(batch)
			return nil
		})
	if err != nil {
		t.Fatalf("the export failed: %v", err)
	}
	if total != 1 || rows != 1 {
		t.Errorf("an export of one key wrote %d rows, wanted 1", total)
	}

	// A browse of one prefix still walks that prefix, and nothing outside it.
	walked, err := session.StreamQuery(
		context.Background(), "SCAN 0 MATCH order:* COUNT 500", nil, 100,
		func([][]any, []db.ResultColumn) error { return nil })
	if err != nil {
		t.Fatalf("the browse export failed: %v", err)
	}
	if walked != 2 {
		t.Errorf("an export of the order prefix wrote %d rows, wanted the 2 of it", walked)
	}
}

// readKeyValue answers the value of one key, as the pane reads it.
func readKeyValue(t *testing.T, session db.Session, key string) string {
	t.Helper()
	held, err := session.RunQuery(
		context.Background(), "GET "+key, dbtest.ReadEverything, nil)
	if err != nil {
		t.Fatalf("the read of %s failed: %v", key, err)
	}
	if len(held.Rows) == 0 || len(held.Rows[0]) == 0 {
		return ""
	}
	return db.ReadAnyText(held.Rows[0][0])
}
