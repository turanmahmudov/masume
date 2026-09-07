package notebook_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/notebook"
)

const sample = `+++
title = "Weekly revenue review"
profiles = ["shop"]
engine = "postgres"
mystery = 7

[run]
transaction = "single"
on_error = "continue"
+++

# Weekly revenue review

Revenue by country for the trailing week.

` + "```param" + `
day = "2026-09-01"
region = "EU"
` + "```" + `

` + "```sql id=countries-by-revenue" + `
-- countries by revenue
select c.country, sum(o.total) as revenue
from orders o join customers c on c.id = o.customer_id
where o.placed_at >= :day and c.region = :region
group by c.country
` + "```" + `

` + "```chart source=countries-by-revenue label=country value=revenue kind=bar" + `
` + "```" + `

` + "```sql id=refresh-revenue-daily write=confirm" + `
-- refresh revenue_daily
update revenue_daily set revenue = 0
` + "```" + `
`

// The front matter carries the title, the profiles and the run policy. A notebook that
// opened on the wrong policy would run a whole review outside the transaction it asked for.
func TestParseReadsTheFrontMatter(t *testing.T) {
	book := notebook.Parse(sample)
	if book.Title != "Weekly revenue review" {
		t.Errorf("title: %q", book.Title)
	}
	if len(book.Profiles) != 1 || book.Profiles[0] != "shop" {
		t.Errorf("profiles: %v", book.Profiles)
	}
	if book.Engine != "postgres" {
		t.Errorf("engine: %q", book.Engine)
	}
	if !book.Run.RunsInOneTransaction() {
		t.Errorf("transaction: %q", book.Run.Transaction)
	}
	if book.Run.StopsOnError() {
		t.Errorf("on error: %q", book.Run.OnError)
	}
	if len(book.Problems) != 0 {
		t.Errorf("problems: %v", book.Problems)
	}
}

// Every fence is a cell and the prose between two fences is a cell of its own, so a review
// reads as a document and not as a wall of comments.
func TestParseReadsEveryCell(t *testing.T) {
	book := notebook.Parse(sample)
	kinds := []notebook.CellKind{}
	for _, cell := range book.Cells {
		kinds = append(kinds, cell.Kind)
	}
	wanted := []notebook.CellKind{
		notebook.CellText, notebook.CellParam, notebook.CellSQL,
		notebook.CellChart, notebook.CellSQL,
	}
	if len(kinds) != len(wanted) {
		t.Fatalf("kinds: %v, want %v", kinds, wanted)
	}
	for at := range wanted {
		if kinds[at] != wanted[at] {
			t.Errorf("cell %d: %q, want %q", at+1, kinds[at], wanted[at])
		}
	}
	if book.Cells[2].ID != "countries-by-revenue" {
		t.Errorf("id: %q", book.Cells[2].ID)
	}
	if !book.Cells[4].AsksConfirmation() {
		t.Errorf("the last cell asks for no confirmation")
	}
	if book.CountWriteCells() != 1 {
		t.Errorf("write cells: %d", book.CountWriteCells())
	}
}

// A cell without an id is named after its first comment line, and two cells never share an
// id: a chart names its source cell by id, and two of one name would draw the wrong result.
func TestParseNamesEveryCellApart(t *testing.T) {
	book := notebook.Parse("```sql\n-- daily\nselect 1\n```\n\n" +
		"```sql\n-- daily\nselect 2\n```\n")
	if len(book.Cells) != 2 {
		t.Fatalf("cells: %d", len(book.Cells))
	}
	if book.Cells[0].ID != "daily" || book.Cells[1].ID != "daily-2" {
		t.Errorf("ids: %q and %q", book.Cells[0].ID, book.Cells[1].ID)
	}
}

// A save keeps a key this build does not know, so a notebook written by a newer build and
// opened by an older one loses nothing.
func TestWriteKeepsWhatItCannotRead(t *testing.T) {
	written := notebook.Write(notebook.Parse(sample))
	if !strings.Contains(written, "mystery = 7") {
		t.Errorf("the unknown key is gone:\n%s", written)
	}
	again := notebook.Parse(written)
	if len(again.Cells) != 5 {
		t.Errorf("cells after a round trip: %d", len(again.Cells))
	}
	if again.Title != "Weekly revenue review" || !again.Run.RunsInOneTransaction() {
		t.Errorf("the front matter changed: %q %q", again.Title, again.Run.Transaction)
	}
	if again.Cells[2].Source != notebook.Parse(sample).Cells[2].Source {
		t.Errorf("the statement changed:\n%q", again.Cells[2].Source)
	}
}

// A fence inside a cell would close it early, so a longer fence is written around it.
func TestWriteHoldsAFenceInsideACell(t *testing.T) {
	book := notebook.Notebook{Cells: []notebook.Cell{{
		ID: "one", Kind: notebook.CellText, Source: "```\nheld\n```",
	}}}
	again := notebook.Parse(notebook.Write(book))
	if len(again.Cells) != 1 || !strings.Contains(again.Cells[0].Source, "held") {
		t.Errorf("cells: %d, first: %q", len(again.Cells), again.Cells[0].Source)
	}
}

// An unknown fence is kept as it was written, so a chart of a newer build survives a save
// by an older one.
func TestParseKeepsAnUnknownFence(t *testing.T) {
	book := notebook.Parse("```mermaid theme=dark\ngraph TD;\n```\n")
	if len(book.Cells) != 1 || book.Cells[0].Kind != notebook.CellOther {
		t.Fatalf("cells: %v", book.Cells)
	}
	written := notebook.Write(book)
	if !strings.Contains(written, "```mermaid theme=dark") {
		t.Errorf("the fence changed:\n%s", written)
	}
}

// The values of a parameter cell are JSON, so a number stays a number and text with a comma
// stays one value.
func TestReadParametersKeepsTheTypeOfEveryValue(t *testing.T) {
	values, problems := notebook.ReadParameters(
		"day = \"2026-09-01\"\nlimit = 10\nname = plain text\n# a comment\n")
	if len(problems) != 0 {
		t.Errorf("problems: %v", problems)
	}
	if values["day"] != "2026-09-01" {
		t.Errorf("day: %#v", values["day"])
	}
	if held, isNumber := values["limit"].(float64); !isNumber || held != 10 {
		t.Errorf("limit: %#v", values["limit"])
	}
	if values["name"] != "plain text" {
		t.Errorf("name: %#v", values["name"])
	}
}

// A reader of SQL writes a value in SQL quotes, and the quotes are no part of the value. A
// value that kept them would bind text no row holds, and the run would answer no rows.
func TestReadParametersTakesTheTextInsideQuotes(t *testing.T) {
	for _, held := range []struct {
		line string
		want any
	}{
		{"status = 'paid'", "paid"},
		{`status = "paid"`, "paid"},
		{"status = paid", "paid"},
		{"status = ''", ""},
		{"day = '2026-09-01'", "2026-09-01"},
		{"limit = 10", float64(10)},
		{"flag = true", true},
		{"held = 'unclosed", "'unclosed"},
	} {
		values, problems := notebook.ReadParameters(held.line)
		if len(problems) != 0 {
			t.Errorf("%q reports %v", held.line, problems)
		}
		name, _, _ := strings.Cut(held.line, " ")
		if got := values[name]; got != held.want {
			t.Errorf("%q binds %#v, wanted %#v", held.line, got, held.want)
		}
	}
}

// A run policy the file cannot hold is reported and the default stands, so a typo does not
// open a transaction over a whole notebook.
func TestParseReportsAnUnknownPolicy(t *testing.T) {
	book := notebook.Parse("+++\n[run]\ntransaction = \"maybe\"\n+++\n")
	if len(book.Problems) == 0 {
		t.Fatalf("no problem reported")
	}
	if book.Run.Transaction != notebook.TransactionAutocommit {
		t.Errorf("transaction: %q", book.Run.Transaction)
	}
}

// A chart reads its source cell and its two columns off the fence.
func TestReadChartReadsTheFence(t *testing.T) {
	book := notebook.Parse(sample)
	spec := notebook.ReadChart(book.Cells[3])
	if spec.Source != "countries-by-revenue" {
		t.Errorf("source: %q", spec.Source)
	}
	if spec.Label != "country" || spec.Value != "revenue" {
		t.Errorf("columns: %q and %q", spec.Label, spec.Value)
	}
	if spec.Shape != notebook.ChartBar {
		t.Errorf("shape: %q", spec.Shape)
	}
}

// A name becomes a file name, so a title with spaces still opens on every file system.
func TestBuildSlugHoldsOnlyWordsAndHyphens(t *testing.T) {
	for _, held := range []struct{ text, want string }{
		{"Weekly revenue review", "weekly-revenue-review"},
		{"  order backfill!  ", "order-backfill"},
		{"2026/09/01 audit", "2026-09-01-audit"},
		{"", ""},
	} {
		if built := notebook.BuildSlug(held.text); built != held.want {
			t.Errorf("slug of %q: %q, want %q", held.text, built, held.want)
		}
	}
}
