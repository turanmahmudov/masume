package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
)

// A run started beside the one going replaces only what the client keeps. The statements of
// the run before it are already with the server, so an INSERT asked for twice is written
// twice, and the second answer is dropped so nobody is told.
func TestATabThatIsRunningIsNotRunAgain(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()

	tab.Editor = app.NewEditorBuffer("insert into orders (customer) values ('ada')", 0)
	tab.Results.Start([]string{"insert into orders (customer) values ('ada')"}, 200)

	if !tab.Results.IsRunning() {
		t.Fatal("the tab does not read as running")
	}

	// Both ways of asking for a run are refused while one is going.
	for _, held := range []struct {
		name string
		run  func() (tea.Model, tea.Cmd)
	}{
		{"the whole buffer", func() (tea.Model, tea.Cmd) {
			return model.runWholeBuffer(connection, tab)
		}},
		{"the statement at the cursor", func() (tea.Model, tea.Cmd) {
			return model.runStatementAtCursor(connection, tab)
		}},
	} {
		connection.Notice = nil
		if _, command := held.run(); command != nil {
			t.Errorf("%s started a second run while one was going", held.name)
		}
		if connection.Notice == nil {
			t.Errorf("%s was refused without a word", held.name)
		}
	}

	// Once the run answered, the tab runs again.
	tab.Results.Succeed(0, db.ComposedRead{}, db.QueryResult{Command: "INSERT"})
	if tab.Results.IsRunning() {
		t.Fatal("the tab still reads as running after its statement answered")
	}
	if model.refuseSecondRun(connection, tab) {
		t.Error("a run was refused after the one before it answered")
	}
}
