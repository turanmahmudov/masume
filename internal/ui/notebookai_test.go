package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/notebook"
)

// reply is what the model writes when it is asked for a notebook: prose between the
// statements, one fenced block per statement, and a `:name` for a value of the reader.
const reply = "Three steps for the weekly review.\n\n" +
	"```sql\n-- countries by revenue\nselect country, sum(total) from orders " +
	"where placed_at >= :day group by country\n```\n\n" +
	"Then the funnel.\n\n" +
	"```sql\n-- funnel by step\nselect step, count(*) from sessions " +
	"where region = :region group by step\n```\n"

// A reply of several statements becomes a notebook of several cells, and the marks those
// statements bind become a parameter cell. A notebook whose marks had no cell would ask for
// every value at every run.
func TestBuildReplyNotebookHoldsEveryStatementAndItsMarks(t *testing.T) {
	book := app.BuildReplyNotebook(reply, "weekly review")
	kinds := []notebook.CellKind{}
	for _, cell := range book.Cells {
		kinds = append(kinds, cell.Kind)
	}
	wanted := []notebook.CellKind{
		notebook.CellParam, notebook.CellText, notebook.CellSQL,
		notebook.CellText, notebook.CellSQL,
	}
	if len(kinds) != len(wanted) {
		t.Fatalf("kinds: %v, wanted %v", kinds, wanted)
	}
	for at := range wanted {
		if kinds[at] != wanted[at] {
			t.Errorf("cell %d: %q, wanted %q", at+1, kinds[at], wanted[at])
		}
	}
	if book.Title != "weekly review" {
		t.Errorf("title: %q", book.Title)
	}
	values, _ := notebook.ReadParameters(book.Cells[0].Source)
	for _, name := range []string{"day", "region"} {
		if held, found := values[name]; !found || held != "" {
			t.Errorf("the parameter cell holds %q = %#v", name, held)
		}
	}
	if book.Cells[2].ID != "countries-by-revenue" {
		t.Errorf("the statement cell is named %q", book.Cells[2].ID)
	}
}

// The reply of a run that was asked for a notebook opens as one, and no cell of it runs.
func TestReplyOpensAsANotebook(t *testing.T) {
	model := buildOfflineModel(t, 110, 30)
	connection := model.Active()
	chat := connection.Chat
	chat.BuildsNotebook, chat.NotebookSubject = true, "weekly review"
	chat.Messages = append(chat.Messages,
		app.ChatMessage{Role: hist.ChatRoleUser, Content: "build a notebook"},
		app.ChatMessage{Role: hist.ChatRoleAssistant, Content: reply})

	model.Update(chatEventsMsg{
		ConnectionID: model.ActiveID(), Run: chat.Run,
		Events: []app.ChatEvent{{Run: chat.Run, Kind: app.ChatEnded}},
	})

	tab := connection.Active()
	if tab.Kind != app.TabNotebook || tab.Notebook == nil {
		t.Fatalf("the reply did not open as a notebook: %q", tab.Kind)
	}
	if tab.Notebook.CountCells() != 5 {
		t.Errorf("cells: %d", tab.Notebook.CountCells())
	}
	if !tab.Notebook.Dirty || tab.Notebook.Path != "" {
		t.Errorf("the notebook is saved already: %q", tab.Notebook.Path)
	}
	for at, cell := range tab.Notebook.Cells {
		if tab.ReadCellOutcome(cell).Kind != app.CellIdle {
			t.Errorf("cell %d ran", at+1)
		}
	}
	if chat.BuildsNotebook {
		t.Errorf("the chat still waits for a notebook")
	}
	if connection.Overlay.IsOpen() {
		t.Errorf("the chat panel is still open")
	}
}

// The request names what the notebook is to cover, and asks for one block per query. The
// system prompt asks for one block per answer, so a request that did not say so plainly
// would answer with one cell.
func TestNotebookRequestAsksForEveryQuery(t *testing.T) {
	written := strings.ReplaceAll(notebookRequest, "%s", "the weekly review")
	for _, wanted := range []string{
		"the weekly review", "one fenced sql block per query",
		"several fenced blocks", "-- name", ":name", "Run nothing.",
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the request holds no %q:\n%s", wanted, written)
		}
	}
}
