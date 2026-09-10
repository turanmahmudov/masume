package dump_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/dump"
)

// A SQLite file is a server this test can open without a container, so the whole round trip
// runs here: a real catalog, a real dump, and the file run back into the database.

// openSqlite returns a session on a fresh SQLite file that holds three orders.
func openSqlite(t *testing.T) db.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	// The adapter refuses a path with no file, so the file is made before it opens.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	session, err := engines.CreateAdapters().Open(ctx, cfg.Profile{
		Name: "shop", Engine: core.EngineSqlite, Database: path,
		AccessMode: cfg.AccessWrite, PageSize: cfg.DefaultPageSize,
	}, "")
	if err != nil {
		t.Fatalf("the file cannot be opened: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	for _, statement := range []string{
		`create table orders (id integer primary key, status text, note text)`,
		`create index orders_status on orders (status)`,
		`insert into orders (id, status, note) values (1, 'open', 'first')`,
		`insert into orders (id, status, note) values (2, 'sent', null)`,
		`insert into orders (id, status, note) values (3, 'open', 'a '';'' note')`,
	} {
		if _, err := session.RunQuery(ctx, statement, 10, nil); err != nil {
			t.Fatalf("cannot run %q: %v", statement, err)
		}
	}
	return session
}

// readNotes returns the note of every order, by id.
func readNotes(t *testing.T, session db.Session) map[int64]string {
	t.Helper()
	answered, err := session.RunQuery(context.Background(),
		"select id, note from orders order by id", 100, nil)
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	held := map[int64]string{}
	for _, row := range answered.Rows {
		held[db.ReadNonNegativeCount(row[0])] = db.ReadAnyText(row[1])
	}
	return held
}

func TestSqliteDumpRunsBackIntoTheFile(t *testing.T) {
	session := openSqlite(t)
	schema := session.Describe().DefaultSchema

	path := filepath.Join(t.TempDir(), "shop.sql")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	report, writeErr := dump.Write(context.Background(), session, dump.Options{
		Schema: schema, Content: dump.ContentAll, DropsFirst: true,
	}, file)
	if closeErr := file.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if writeErr != nil {
		t.Fatalf("the dump answered %v", writeErr)
	}
	if report.Tables != 1 || report.Rows != 3 {
		t.Fatalf("report: %+v, want one table and three rows", report)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(string(written)), "create index") {
		t.Errorf("the dump holds no index:\n%s", written)
	}

	// The dump drops the table it makes, so the same file rebuilds the database it came
	// from, rows and index and all.
	run, runErr := dump.RunFile(context.Background(), session, path,
		session.Dialect().Syntax)
	if runErr != nil {
		t.Fatalf("the restore answered %v after %d statements", runErr, run.Statements)
	}
	notes := readNotes(t, session)
	if len(notes) != 3 || notes[3] != "a ';' note" {
		t.Fatalf("orders: %v, want the three rows of the dump", notes)
	}
}
