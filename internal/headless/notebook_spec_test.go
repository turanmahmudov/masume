// A functional test of a notebook run: it opens a real SQLite file through the real adapter
// and reads what the run wrote.
package headless_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/headless"
	"github.com/turanmahmudov/masume/internal/notebook"
)

const bookText = "+++\ntitle = \"orders\"\n\n[run]\ntransaction = \"autocommit\"\n" +
	"on_error = \"stop\"\n+++\n\n" +
	"Every paid order of the shop.\n\n" +
	"```param\nstatus = \"paid\"\n```\n\n" +
	"```sql id=paid-orders\n-- paid orders\n" +
	"select id, total_cents from orders where status = :status order by id\n```\n\n" +
	"```chart source=paid-orders label=id value=total_cents kind=bar\n```\n"

// runNotebookText runs one notebook and returns the exit code, what was written, and what
// was reported.
func runNotebookText(
	t *testing.T, options headless.NotebookOptions, text string,
) (int, string, string) {
	t.Helper()
	out, reported := &bytes.Buffer{}, &bytes.Buffer{}
	options.Out, options.Err = out, reported
	if options.Format == "" {
		options.Format = headless.FormatTable
	}
	options.Notebook = notebook.Parse(text)
	code := headless.RunNotebook(
		context.Background(), engines.CreateAdapters(), options)
	return code, out.String(), reported.String()
}

// A notebook run writes the rows of every statement cell and binds the values of the
// parameter cells, so a script gets the same rows the screen draws.
func TestRunNotebookWritesTheRowsOfEveryCell(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runNotebookText(t, headless.NotebookOptions{
		Options: headless.Options{Profile: profile},
	}, bookText)
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if !strings.Contains(written, "total_cents") {
		t.Errorf("the run wrote %q, wanted the column names", written)
	}
	lines := strings.Split(strings.TrimRight(written, "\n"), "\n")
	if len(lines) != 4 {
		t.Errorf("the run wrote %d lines, wanted a head, a rule and the two paid orders: %q",
			len(lines), written)
	}
	if !strings.Contains(reported, "paid orders") {
		t.Errorf("the report is %q, wanted the name of the cell", reported)
	}
}

// A value of the command line replaces the value of a parameter cell, so one notebook runs
// for a day or a status the file does not name.
func TestRunNotebookTakesTheValueOfTheCommandLine(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runNotebookText(t, headless.NotebookOptions{
		Options: headless.Options{
			Profile: profile, Params: map[string]any{"status": "cancelled"},
		},
	}, bookText)
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if !strings.Contains(written, "3   99") {
		t.Errorf("the run wrote %q, wanted the cancelled order", written)
	}
}

// A run without a screen can confirm no write, so a write cell is refused until the caller
// says so on the command line.
func TestRunNotebookRefusesAWriteCellWithoutTheFlag(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	held := "```sql id=hold write=confirm\nupdate orders set status = 'held'\n```\n"

	code, _, reported := runNotebookText(t, headless.NotebookOptions{
		Options: headless.Options{Profile: profile},
	}, held)
	if code != headless.CodeStatement {
		t.Fatalf("the run answered %d, wanted %d", code, headless.CodeStatement)
	}
	if !strings.Contains(reported, "--allow-writes") {
		t.Errorf("the report is %q, wanted the flag that runs it", reported)
	}

	code, _, reported = runNotebookText(t, headless.NotebookOptions{
		Options: headless.Options{Profile: profile}, AllowWrites: true,
	}, held)
	if code != headless.CodeOK {
		t.Fatalf("the run with the flag answered %d and said %q", code, reported)
	}
}

// A read-only profile refuses a write cell whatever the flags say.
func TestRunNotebookRefusesAWriteOnAReadOnlyProfile(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessReadOnly)

	code, _, reported := runNotebookText(t, headless.NotebookOptions{
		Options: headless.Options{Profile: profile}, AllowWrites: true,
	}, "```sql id=hold\nupdate orders set status = 'held'\n```\n")
	if code != headless.CodeRefused {
		t.Fatalf("the run answered %d, wanted %d", code, headless.CodeRefused)
	}
	if !strings.Contains(reported, "read-only") {
		t.Errorf("the report is %q, wanted the refusal", reported)
	}
}

// The Markdown report holds the prose, the statements and the rows of every cell, and the
// chart of a cell as a block of text.
func TestRunNotebookWritesAMarkdownReport(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)

	code, written, reported := runNotebookText(t, headless.NotebookOptions{
		Options: headless.Options{Profile: profile, Format: headless.FormatMarkdown},
	}, bookText)
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	for _, wanted := range []string{
		"Every paid order of the shop.", "## paid orders", "```sql",
		"| id | total_cents |", "▇",
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the report holds no %q:\n%s", wanted, written)
		}
	}
}

// A cell of `--only` runs and the cells beside it do not, so one cell of a long notebook
// can be run from a script.
func TestRunNotebookRunsOnlyTheCellItIsAskedFor(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	text := bookText + "```sql id=every-order\n-- every order\nselect count(*) from orders\n```\n"

	code, written, reported := runNotebookText(t, headless.NotebookOptions{
		Options: headless.Options{Profile: profile}, Only: []string{"every-order"},
	}, text)
	if code != headless.CodeOK {
		t.Fatalf("the run answered %d and said %q", code, reported)
	}
	if strings.Contains(reported, "paid orders") {
		t.Errorf("the report names a cell that was not asked for: %q", reported)
	}
	if !strings.Contains(written, "3") {
		t.Errorf("the run wrote %q, wanted the count of the orders", written)
	}
}

// A notebook that continues on an error runs the cells after a failed one, and still
// answers with the code of a failure.
func TestRunNotebookContinuesAfterAFailedCell(t *testing.T) {
	profile := buildDatabase(t, cfg.AccessWrite)
	text := "+++\n[run]\non_error = \"continue\"\n+++\n\n" +
		"```sql id=missing\n-- a table that is not there\nselect * from no_such_table\n```\n\n" +
		"```sql id=orders\n-- every order\nselect count(*) as held from orders\n```\n"

	code, written, reported := runNotebookText(t, headless.NotebookOptions{
		Options: headless.Options{Profile: profile},
	}, text)
	if code != headless.CodeStatement {
		t.Fatalf("the run answered %d, wanted %d", code, headless.CodeStatement)
	}
	if !strings.Contains(written, "held") {
		t.Errorf("the run wrote %q, wanted the cell after the failed one", written)
	}
	if !strings.Contains(reported, "fail") {
		t.Errorf("the report is %q, wanted the failed cell", reported)
	}
}
