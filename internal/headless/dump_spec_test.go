// A functional test of the whole dump and of the whole restore: it opens a real SQLite file
// through the real adapter and reads what each run wrote.
package headless_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/dump"
	"github.com/turanmahmudov/masume/internal/headless"
)

// runDump writes a dump of that database and answers the exit code and what it reported.
func runDump(t *testing.T, options headless.DumpOptions) (int, string, string) {
	t.Helper()
	out, reported := &bytes.Buffer{}, &bytes.Buffer{}
	options.Out, options.Err = out, reported
	code := headless.RunDump(context.Background(), engines.CreateAdapters(), options)
	return code, out.String(), reported.String()
}

// runRestore runs that file and answers the exit code and what it reported.
func runRestore(t *testing.T, options headless.RestoreOptions) (int, string) {
	t.Helper()
	out, reported := &bytes.Buffer{}, &bytes.Buffer{}
	options.Out, options.Err = out, reported
	code := headless.RunRestore(context.Background(), engines.CreateAdapters(), options)
	return code, reported.String()
}

func TestDumpWritesTheFileAndTheRestoreRunsIt(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	path := filepath.Join(t.TempDir(), "shop.sql")

	code, _, reported := runDump(t, headless.DumpOptions{
		Options: headless.Options{Profile: profile},
		Path:    path,
		Dump:    dump.Options{Content: dump.ContentAll, DropsFirst: true},
	})
	if code != headless.CodeOK {
		t.Fatalf("the dump answered %d: %s", code, reported)
	}
	if !strings.Contains(reported, "dumped 1 table") {
		t.Errorf("report: %q, want the counts of the dump", reported)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "insert into") {
		t.Fatalf("the file holds no row:\n%s", written)
	}

	// The dump drops what it makes, so it runs on the database it came from.
	code, reported = runRestore(t, headless.RestoreOptions{
		Options: headless.Options{Profile: profile}, Path: path,
	})
	if code != headless.CodeOK {
		t.Fatalf("the restore answered %d: %s", code, reported)
	}
	if !strings.Contains(reported, "ran ") {
		t.Errorf("report: %q, want the count of the run", reported)
	}

	answered, _, _ := runOptions(t, headless.Options{
		Profile: profile, Statement: "select count(*) from orders", Format: headless.FormatCSV,
	})
	if answered != headless.CodeOK {
		t.Fatalf("the read answered %d", answered)
	}
}

func TestDumpWritesToTheOutputStream(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runDump(t, headless.DumpOptions{
		Options: headless.Options{Profile: profile},
		Path:    "-",
		Dump:    dump.Options{Content: dump.ContentSchema},
	})
	if code != headless.CodeOK {
		t.Fatalf("the dump answered %d: %s", code, reported)
	}
	if !strings.Contains(written, "-- masume dump") ||
		!strings.Contains(strings.ToLower(written), "create table") {
		t.Fatalf("the stream holds no dump:\n%s", written)
	}
	if strings.Contains(written, "insert into") {
		t.Errorf("a schema-only dump holds rows:\n%s", written)
	}
}

func TestRestoreReadsTheInputStream(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, reported := runRestore(t, headless.RestoreOptions{
		Options: headless.Options{Profile: profile}, Path: "-",
		In: strings.NewReader("create table notes (id integer primary key);\n"),
	})
	if code != headless.CodeOK {
		t.Fatalf("the restore answered %d: %s", code, reported)
	}
	if !strings.Contains(reported, "ran 1 statement") {
		t.Errorf("report: %q, want the count of the run", reported)
	}
}

func TestRestoreStopsAtTheStatementThatFailed(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	path := filepath.Join(t.TempDir(), "broken.sql")
	if err := os.WriteFile(path, []byte(
		"create table notes (id integer primary key);\nselect from;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, reported := runRestore(t, headless.RestoreOptions{
		Options: headless.Options{Profile: profile}, Path: path,
	})
	if code != headless.CodeStatement {
		t.Fatalf("the restore answered %d, want %d: %s", code, headless.CodeStatement, reported)
	}
	if !strings.Contains(reported, "statement 2 failed") ||
		!strings.Contains(reported, "1 statement ran before it") {
		t.Errorf("report: %q, want the statement that failed and the count", reported)
	}
}

func TestRestoreIsRefusedByAReadOnlyProfile(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessReadOnly)
	path := filepath.Join(t.TempDir(), "shop.sql")
	if err := os.WriteFile(path, []byte("create table notes (id integer);\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, reported := runRestore(t, headless.RestoreOptions{
		Options: headless.Options{Profile: profile}, Path: path,
	})
	if code != headless.CodeRefused {
		t.Fatalf("the restore answered %d, want %d", code, headless.CodeRefused)
	}
	if !strings.Contains(reported, "read-only") {
		t.Errorf("report: %q, want the refusal", reported)
	}
}

func TestRestoreReportsAMissingFile(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, reported := runRestore(t, headless.RestoreOptions{
		Options: headless.Options{Profile: profile},
		Path:    filepath.Join(t.TempDir(), "gone.sql"),
	})
	if code != headless.CodeConnection {
		t.Fatalf("the restore answered %d, want %d", code, headless.CodeConnection)
	}
	if !strings.Contains(reported, "gone.sql") {
		t.Errorf("report: %q, want the file it could not read", reported)
	}
}

// A read-only profile dumps, because a dump only reads.
func TestDumpRunsOnAReadOnlyProfile(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessReadOnly)

	code, written, reported := runDump(t, headless.DumpOptions{
		Options: headless.Options{Profile: profile}, Path: "-",
		Dump: dump.Options{Content: dump.ContentAll},
	})
	if code != headless.CodeOK {
		t.Fatalf("the dump answered %d: %s", code, reported)
	}
	if !strings.Contains(written, "insert into") {
		t.Errorf("the dump holds no row:\n%s", written)
	}
}
