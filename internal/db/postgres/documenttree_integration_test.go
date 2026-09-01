//go:build integration

// An integration test: it reads a real PostgreSQL, to hold the document tree to a server
// that is not MongoDB. A `json` and a `jsonb` column hold a document the same way an
// embedded document does, and the tree opens either one.
package postgres_test

import (
	"context"
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/present"
)

// openProfiles answers a session on a table holding a document in a column.
func openProfiles(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session,
		"drop table if exists profiles",
		`create table profiles (
			id serial primary key, name text, detail jsonb, tags json)`,
		`insert into profiles (name, detail, tags) values
			('ada', '{"city":"berlin","age":36,
				"address":{"zip":"10115","street":"unter den linden"}}', '["web","admin"]'),
			('grace', '{"city":"hamburg","age":45,
				"address":{"zip":"20095","street":"steinstrasse"}}', '["api"]')`,
	)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), "drop table if exists profiles", 10, nil)
	})
	return session
}

// A column of JSON opens in the tree the way a document of a collection does. The tree
// belongs wherever a value holds fields, and not to one server.
func TestTheDocumentTreeOpensAJSONColumn(t *testing.T) {
	session := openProfiles(t)
	answered, err := session.RunQuery(context.Background(),
		"select id, name, detail, tags from profiles order by id", 100, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if !present.HasDocumentColumn(answered.Columns, answered.Rows) {
		t.Fatal("a result with a jsonb column offers no tree")
	}

	indexes := make([]int, len(answered.Rows))
	for at := range answered.Rows {
		indexes[at] = at
	}
	row := present.BuildRowPath(0)
	tree := present.BuildDocumentTree(present.DocumentTreeInput{
		Columns: answered.Columns, Rows: answered.Rows, RowIndexes: indexes,
		Opened: map[string]bool{
			row: true, row + "\x1f" + "detail": true, row + "\x1f" + "tags": true,
		},
	})

	held := map[string]present.DocumentNode{}
	for _, node := range tree.ReadWindow(0, tree.CountRows()) {
		held[node.Key] = node
	}
	for _, want := range []struct{ key, value string }{
		{"detail", "{ 3 fields }"},
		{"city", "berlin"},
		{"age", "36"},
		{"address", "{ 2 fields }"},
		// An array is opened by the place of each element, because an element has no name.
		{"0", "web"},
		{"1", "admin"},
	} {
		node, found := held[want.key]
		if !found {
			t.Errorf("the tree has no %q", want.key)
			continue
		}
		if node.Value != want.value {
			t.Errorf("%q reads %q, wanted %q", want.key, node.Value, want.value)
		}
	}

	// A column of no document is drawn and not opened, whatever else the row holds.
	if held["name"].Opens {
		t.Error("a column of text opens in the tree")
	}
}

// A table of plain columns offers no tree, because there is nothing in it to open.
func TestATableOfPlainColumnsOffersNoDocumentTree(t *testing.T) {
	session := dbtest.Open(t, dbtest.Postgres)
	dbtest.RunStatements(t, session,
		"drop table if exists plain_people",
		"create table plain_people (id serial primary key, name text, age integer)",
		"insert into plain_people (name, age) values ('ada', 36), ('grace', 45)",
	)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), "drop table if exists plain_people", 10, nil)
	})

	answered, err := session.RunQuery(context.Background(),
		"select id, name, age from plain_people", 100, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if present.HasDocumentColumn(answered.Columns, answered.Rows) {
		t.Error("a table of plain columns offers a tree")
	}
}
