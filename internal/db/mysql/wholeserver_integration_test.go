//go:build integration

// An integration test: it reads a real MySQL. The server is started outside this code and
// named through MASUME_TEST_MYSQL. Nothing here knows how it was started.
package mysql_test

import (
	"context"
	"slices"
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/db/engines"
)

// emptyDatabase is a database of these tests that holds no relation.
const emptyDatabase = "masume_empty"

// openWholeServer returns a session of a profile that names no database, which is what a
// MySQL connection without one reads.
func openWholeServer(t *testing.T) db.Session {
	t.Helper()
	profile, password := dbtest.BuildProfile(t, dbtest.MySQL)
	profile.Database = ""

	session, err := engines.CreateAdapters().Open(context.Background(), profile, password)
	if err != nil {
		t.Fatalf("a profile without a database does not open: %v", db.DescribeError(err))
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// A MySQL connection reads every database of the server, so a profile that names none draws
// them all, including a database that holds no relation.
func TestAProfileWithoutADatabaseListsEveryDatabase(t *testing.T) {
	session := openWholeServer(t)
	ctx := context.Background()

	if _, err := session.RunQuery(
		ctx, "create database if not exists "+emptyDatabase, dbtest.ReadEverything, nil,
	); err != nil {
		t.Fatalf("the database was not created: %v", db.DescribeError(err))
	}
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			ctx, "drop database if exists "+emptyDatabase, dbtest.ReadEverything, nil)
	})

	schemas, err := session.ListSchemas(ctx)
	if err != nil {
		t.Fatalf("the database list answered %v", db.DescribeError(err))
	}
	if !slices.Contains(schemas, emptyDatabase) {
		t.Errorf("the list reads %v, without the database that holds nothing", schemas)
	}
	if !slices.Contains(schemas, "shop") {
		t.Errorf("the list reads %v, without the database of the tests", schemas)
	}
	// The system databases belong to the server, and the tree draws none of them.
	for _, held := range []string{"mysql", "information_schema", "performance_schema", "sys"} {
		if slices.Contains(schemas, held) {
			t.Errorf("the list holds %q, which the server keeps for itself", held)
		}
	}
}

// A profile that names a database reads that database alone, so the tree of it holds no
// relation of another database.
func TestAProfileWithADatabaseListsThatDatabaseAlone(t *testing.T) {
	whole := openWholeServer(t)
	ctx := context.Background()
	if _, err := whole.RunQuery(
		ctx, "create database if not exists "+emptyDatabase, dbtest.ReadEverything, nil,
	); err != nil {
		t.Fatalf("the database was not created: %v", db.DescribeError(err))
	}
	t.Cleanup(func() {
		_, _ = whole.RunQuery(
			ctx, "drop database if exists "+emptyDatabase, dbtest.ReadEverything, nil)
	})

	session := openShop(t)
	schemas, err := session.ListSchemas(ctx)
	if err != nil {
		t.Fatalf("the database list answered %v", db.DescribeError(err))
	}
	if len(schemas) != 1 || schemas[0] != "shop" {
		t.Errorf("the list reads %v, wanted the database of the profile alone", schemas)
	}

	tables, tableErr := session.ListTables(ctx)
	if tableErr != nil {
		t.Fatalf("the relation list answered %v", db.DescribeError(tableErr))
	}
	for _, table := range tables {
		if table.Schema != "shop" {
			t.Errorf("the relation %s.%s is outside the database of the profile",
				table.Schema, table.Name)
		}
	}
}
