// An end-to-end test of a notebook: it opens a real SQLite file through the real adapter,
// presses the keys a reader presses, and reads back the cells, the results and the file.
// SQLite needs no server, so this runs in the ordinary suite.
package ui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
)

// liveNotebook is the notebook the test opens: prose, values, statements, a chart, and a
// write that asks first.
const liveNotebook = "+++\ntitle = \"orders review\"\n\n[run]\n" +
	"transaction = \"autocommit\"\non_error = \"stop\"\n+++\n\n" +
	"# Orders review\n\n" +
	"```param id=values\nstatus = 'paid'\n```\n\n" +
	"```sql id=by-status\n-- orders by status\n" +
	"select status, count(*) as orders, sum(total_cents) as cents from orders " +
	"group by status order by orders desc\n```\n\n" +
	"```chart id=bars source=by-status label=status value=orders kind=bar\n```\n\n" +
	"```sql id=paid-orders\n-- paid orders\n" +
	"select id, total_cents from orders where status = :status order by id\n```\n\n" +
	"```sql id=hold-one write=confirm\n-- hold one order\n" +
	"update orders set status = 'held' where id = 3\n```\n"

// liveWorkspace is a model with one real connection on a fresh SQLite file.
type liveWorkspace struct {
	model      *Model
	connection *app.Connection
	directory  string
}

// buildLiveWorkspace opens a real connection on a new database of three orders.
func buildLiveWorkspace(t *testing.T, access cfg.AccessMode) liveWorkspace {
	t.Helper()
	directory := t.TempDir()
	// The state directory of the test is its own, so a save of a notebook never reaches
	// the state directory of the user running the suite.
	t.Setenv("XDG_STATE_HOME", filepath.Join(directory, "state"))
	path := filepath.Join(directory, "shop.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("the database file cannot be made: %v", err)
	}

	profile := cfg.Profile{
		Name: "shop", Engine: core.EngineSqlite, Database: path,
		AccessMode: cfg.AccessWrite, PageSize: cfg.DefaultPageSize,
	}
	opened := openLiveSession(t, profile)
	runLiveStatements(t, opened, `
		create table orders (id integer primary key, total_cents integer, status text);
		insert into orders (total_cents, status) values (4990, 'paid');
		insert into orders (total_cents, status) values (1200, 'paid');
		insert into orders (total_cents, status) values (99, 'cancelled');`)
	if err := opened.Close(); err != nil {
		t.Fatalf("the setup connection does not close: %v", err)
	}

	profile.AccessMode = access
	// The guard of a write is what the notebook has to go through, so the profile asks.
	profile.ConfirmWrites = cfg.ConfirmWrite
	profile.WritePlan = cfg.PlanUndo
	session := openLiveSession(t, profile)
	t.Cleanup(func() { _ = session.Close() })

	model := NewModel(loadedConfigForTest("tokyonight"), engines.CreateAdapters(), nil, nil)
	sized, _ := model.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	model = sized.(*Model)
	connection := app.NewConnection(session, nil, true)
	model.connections.open(connection)
	model.screen = ScreenWorking
	// The tree of a fresh connection is empty, and the cells want the width.
	connection.SidebarVisible = false
	held := liveWorkspace{model: model, connection: connection, directory: directory}
	// The catalog of the connection is read as it is on a connect, because a write plan
	// resolves its relation against it.
	held.pump(t, readCatalog(model.ActiveID(), session, quietCatalogRead))
	if len(connection.Catalog.Tables) == 0 {
		t.Fatalf("the catalog of the connection is empty")
	}
	return held
}

// openLiveSession opens one connection through the real adapter.
func openLiveSession(t *testing.T, profile cfg.Profile) db.Session {
	t.Helper()
	session, err := engines.CreateAdapters().Open(context.Background(), profile, "")
	if err != nil {
		t.Fatalf("the database cannot be opened: %v", err)
	}
	return session
}

// runLiveStatements lays out the schema of the test.
func runLiveStatements(t *testing.T, session db.Session, sql string) {
	t.Helper()
	for _, one := range session.Language().SplitStatements(sql) {
		if _, err := session.RunQuery(context.Background(), one, 100, nil); err != nil {
			t.Fatalf("the schema was not laid out: %v", err)
		}
	}
}

// press hands one key to the model and runs every command it answers with.
func (held *liveWorkspace) press(t *testing.T, key tea.KeyPressMsg) {
	t.Helper()
	_, command := held.model.Update(key)
	held.pump(t, command)
}

// pump runs a command and feeds every message it answers back into the model, until the
// commands run out.
func (held *liveWorkspace) pump(t *testing.T, command tea.Cmd) {
	t.Helper()
	queue := []tea.Cmd{command}
	for round := 0; round < liveRounds && len(queue) > 0; round++ {
		next := []tea.Cmd{}
		for _, one := range queue {
			if one == nil {
				continue
			}
			message := one()
			if message == nil {
				continue
			}
			if batch, is := message.(tea.BatchMsg); is {
				next = append(next, batch...)
				continue
			}
			// A wait for the clock would run for as long as the client does.
			if _, waits := message.(tickMsg); waits {
				continue
			}
			_, answered := held.model.Update(message)
			next = append(next, answered)
		}
		queue = next
	}
	// Every step draws, because a frame that cannot be drawn is a fault of its own.
	held.readFrame(t)
}

// liveRounds is how many rounds of commands one press may answer with.
const liveRounds = 60

// readFrame draws the frame and returns its rows, with the escapes taken off. Every row has
// to measure the width of the screen.
func (held *liveWorkspace) readFrame(t *testing.T) []string {
	t.Helper()
	rows := readFrameRows(held.model.View().Content)
	for at, row := range rows {
		if measured := present.MeasureText(row); measured != held.model.width {
			t.Errorf("row %d measures %d, wanted %d: %q",
				at, measured, held.model.width, row)
		}
	}
	return rows
}

// openLiveNotebook writes the notebook to a file and opens it the way the card does.
func (held *liveWorkspace) openLiveNotebook(t *testing.T, text string) *app.Tab {
	t.Helper()
	path := filepath.Join(held.directory, "review.masume.md")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("the notebook cannot be written: %v", err)
	}
	held.pump(t, readNotebookFile(held.model.ActiveID(), path, false))
	tab := held.connection.Active()
	if tab.Kind != app.TabNotebook {
		t.Fatalf("the file did not open as a notebook: %q", tab.Kind)
	}
	return tab
}

// readStatusOfOrder asks the server what one row holds now, so a write is read back
// through the server and not through the client that sent it.
func (held *liveWorkspace) readStatusOfOrder(t *testing.T, id int) string {
	t.Helper()
	answered, err := held.connection.Session.RunQuery(context.Background(),
		"select status from orders where id = "+strconv.Itoa(id), 10, nil)
	if err != nil {
		t.Fatalf("the row cannot be read: %v", err)
	}
	if len(answered.Rows) != 1 || len(answered.Rows[0]) != 1 {
		t.Fatalf("the read answered %d rows", len(answered.Rows))
	}
	return core.FormatCell(answered.Rows[0][0], "text")
}

// focusLiveCell moves the list to one cell with the keys of the list.
func (held *liveWorkspace) focusLiveCell(t *testing.T, tab *app.Tab, id string) {
	t.Helper()
	wanted := tab.Notebook.FindCellIndex(id)
	if wanted < 0 {
		t.Fatalf("the notebook holds no cell %q", id)
	}
	for tab.Notebook.Focused > wanted {
		held.press(t, tea.KeyPressMsg{Code: tea.KeyUp})
	}
	for tab.Notebook.Focused < wanted {
		held.press(t, tea.KeyPressMsg{Code: tea.KeyDown})
	}
}

// findLiveCell returns the cell of that id.
func findLiveCell(t *testing.T, tab *app.Tab, id string) *app.NotebookCell {
	t.Helper()
	at := tab.Notebook.FindCellIndex(id)
	if at < 0 {
		t.Fatalf("the notebook holds no cell %q", id)
	}
	return tab.Notebook.Cells[at]
}

// A notebook opens from a file without running a cell, runs every cell on one key, and
// answers the write of a cell with the question the profile asks for. This is the whole
// path a reader takes, so a break anywhere in it is a break of the feature.
func TestNotebookRunsEveryCellEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	// Opening runs nothing.
	for at, cell := range tab.Notebook.Cells {
		if tab.ReadCellOutcome(cell).Kind != app.CellIdle {
			t.Fatalf("cell %d ran on open", at+1)
		}
	}
	if tab.Notebook.Title != "orders review" {
		t.Errorf("title: %q", tab.Notebook.Title)
	}

	// The values of the parameter cell reach the statement that binds them.
	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if !held.connection.Overlay.IsOpen() {
		t.Fatalf("the write cell asked nothing")
	}
	if held.connection.Overlay.Kind != app.OverlayConfirm &&
		held.connection.Overlay.Kind != app.OverlayWritePlan {
		t.Fatalf("the card is %q", held.connection.Overlay.Kind)
	}
	held.press(t, tea.KeyPressMsg{Code: 'y', Text: "y"})

	for id, wanted := range map[string]int{"by-status": 2, "paid-orders": 2} {
		cell := findLiveCell(t, tab, id)
		outcome := tab.ReadCellOutcome(cell)
		if outcome.Kind != app.CellDone {
			t.Errorf("cell %q is %q: %s", id, outcome.Kind, outcome.Message)
			continue
		}
		if outcome.Rows != wanted {
			t.Errorf("cell %q answered %d rows, wanted %d", id, outcome.Rows, wanted)
		}
	}

	// The write ran, and the database holds what it wrote.
	written := findLiveCell(t, tab, "hold-one")
	if outcome := tab.ReadCellOutcome(written); outcome.Kind != app.CellDone {
		t.Errorf("the write cell is %q: %s", outcome.Kind, outcome.Message)
	}
	if status := held.readStatusOfOrder(t, 3); status != "held" {
		t.Errorf("order 3 is %q, wanted the status the write set", status)
	}

	// The chart draws the rows of its source cell.
	rows, problem := readCellChartRows(tab, notebook.ReadChart(
		notebook.Cell{Attrs: findLiveCell(t, tab, "bars").Attrs}))
	if problem != "" {
		t.Fatalf("the chart cannot draw: %s", problem)
	}
	if len(rows) != 2 {
		t.Errorf("the chart holds %d rows", len(rows))
	}
}

// A run of one cell sends that cell only, and the result pane draws it. A run that sent
// the whole notebook would run a write the reader did not ask for.
func TestNotebookRunsOneCellEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	tab.Notebook.FocusCell(tab.Notebook.FindCellIndex("paid-orders"))
	tab.Editor = tab.Notebook.GetFocusedCell().Editor
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})

	if held.connection.Overlay.IsOpen() {
		t.Fatalf("one read asked a question: %q", held.connection.Overlay.Kind)
	}
	if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "paid-orders")); outcome.Rows != 2 {
		t.Errorf("the cell answered %d rows: %q %s", outcome.Rows, outcome.Kind, outcome.Message)
	}
	if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "by-status")); outcome.Kind != app.CellIdle {
		t.Errorf("another cell ran: %q", outcome.Kind)
	}
	if active := tab.Results.Active(); active == nil ||
		len(active.State.Result.Rows) != 2 {
		t.Errorf("the result pane draws another result")
	}
}

// A read-only profile refuses the write cell of a notebook, and the reads still run.
func TestNotebookRefusesAWriteOnAReadOnlyProfile(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessReadOnly)
	tab := held.openLiveNotebook(t, liveNotebook)

	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if held.connection.Overlay.IsOpen() {
		t.Fatalf("a read-only profile asked a question: %q", held.connection.Overlay.Kind)
	}
	if held.connection.Notice == nil ||
		!strings.Contains(held.connection.Notice.Text, "read-only") {
		t.Errorf("the notice is %v", held.connection.Notice)
	}
	for _, cell := range tab.Notebook.Cells {
		if tab.ReadCellOutcome(cell).Kind == app.CellDone {
			t.Errorf("a cell ran on a refused notebook")
			break
		}
	}
}

// A failed cell stops the run, and the cells after it do not run. The list moves to the
// cell that failed, so the reader is on the cell they have to fix.
func TestNotebookStopsAtAFailedCellEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t,
		"```sql id=first\n-- first\nselect 1 as held\n```\n\n"+
			"```sql id=missing\n-- missing\nselect * from no_such_table\n```\n\n"+
			"```sql id=third\n-- third\nselect 3 as held\n```\n")

	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})

	if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "first")); outcome.Kind != app.CellDone {
		t.Errorf("the first cell is %q", outcome.Kind)
	}
	if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "missing")); outcome.Kind != app.CellFailed {
		t.Errorf("the failed cell is %q", outcome.Kind)
	}
	if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "third")); outcome.Kind == app.CellDone {
		t.Errorf("the cell after the failure ran")
	}
	if tab.Notebook.Focused != tab.Notebook.FindCellIndex("missing") {
		t.Errorf("the list stands on cell %d, wanted the failed one",
			tab.Notebook.Focused+1)
	}
}

// A notebook whose policy continues on an error runs the cells after a failed one.
func TestNotebookContinuesAfterAFailedCellEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t,
		"+++\n[run]\non_error = \"continue\"\n+++\n\n"+
			"```sql id=missing\n-- missing\nselect * from no_such_table\n```\n\n"+
			"```sql id=second\n-- second\nselect 2 as held\n```\n")

	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})

	if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "missing")); outcome.Kind != app.CellFailed {
		t.Errorf("the first cell is %q", outcome.Kind)
	}
	if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "second")); outcome.Kind != app.CellDone {
		t.Errorf("the cell after the failure is %q: %s", outcome.Kind, outcome.Message)
	}
}

// One transaction over the whole notebook opens before the first cell and commits after the
// last one, so a run that holds a transaction open would leave the server waiting.
func TestNotebookRunsInOneTransactionEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t,
		"+++\n[run]\ntransaction = \"single\"\n+++\n\n"+
			"```sql id=one\n-- one\nselect 1 as held\n```\n\n"+
			"```sql id=two\n-- two\nselect 2 as held\n```\n")

	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})

	for _, id := range []string{"one", "two"} {
		if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, id)); outcome.Kind != app.CellDone {
			t.Errorf("cell %q is %q: %s", id, outcome.Kind, outcome.Message)
		}
	}
	if state := held.connection.Session.ReadTransactionState(); state != db.TransactionNone {
		t.Errorf("the transaction is %q after the run", state)
	}
	if tab.Notebook.HoldsTransaction {
		t.Errorf("the notebook still holds a transaction")
	}
}

// An edit of a cell is written to the file the notebook came from, and the file reads back
// as the same notebook.
func TestNotebookSavesWhatWasEditedEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	tab.Notebook.FocusCell(tab.Notebook.FindCellIndex("paid-orders"))
	cell := tab.Notebook.GetFocusedCell()
	tab.Editor = cell.Editor
	cell.Editor.SetText("-- paid orders\nselect id from orders where status = :status")
	tab.Notebook.Dirty = true

	held.press(t, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if tab.Notebook.Dirty {
		t.Errorf("the notebook is still marked as edited")
	}

	written, err := os.ReadFile(tab.Notebook.Path)
	if err != nil {
		t.Fatalf("the file cannot be read: %v", err)
	}
	again := notebook.Parse(string(written))
	if len(again.Cells) != len(tab.Notebook.Cells) {
		t.Fatalf("the file holds %d cells, wanted %d",
			len(again.Cells), len(tab.Notebook.Cells))
	}
	edited, found := findParsedCell(again, "paid-orders")
	if !found || !strings.Contains(edited.Source, "select id from orders") {
		t.Errorf("the edit is not in the file: %q", edited.Source)
	}
	if again.Title != "orders review" || again.Run.OnError != notebook.ErrorStop {
		t.Errorf("the front matter changed: %q %q", again.Title, again.Run.OnError)
	}
	if _, held := findParsedCell(again, "hold-one"); !held {
		t.Errorf("the write cell is gone")
	}
}

// findParsedCell returns the cell of that id in a notebook read from a file.
func findParsedCell(book notebook.Notebook, id string) (notebook.Cell, bool) {
	for _, cell := range book.Cells {
		if cell.ID == id {
			return cell, true
		}
	}
	return notebook.Cell{}, false
}

// The tabs of a profile are stored and restored, so a notebook comes back with its cells
// and without its results.
func TestNotebookRestoresItsCellsEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)
	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	held.press(t, tea.KeyPressMsg{Code: 'y', Text: "y"})

	snapshot := held.connection.BuildWorkspaceSnapshot()
	again := app.NewConnection(held.connection.Session, nil, true)
	again.RestoreTabs(snapshot, func(db.TableRef) string { return "" })

	restored := again.Active()
	if restored.Kind != app.TabNotebook || restored.Notebook == nil {
		t.Fatalf("the notebook did not come back: %q", restored.Kind)
	}
	if restored.Notebook.CountCells() != tab.Notebook.CountCells() {
		t.Errorf("cells: %d, wanted %d",
			restored.Notebook.CountCells(), tab.Notebook.CountCells())
	}
	if restored.Notebook.Path != tab.Notebook.Path {
		t.Errorf("path: %q", restored.Notebook.Path)
	}
	for at, cell := range restored.Notebook.Cells {
		if restored.ReadCellOutcome(cell).Kind != app.CellIdle {
			t.Errorf("cell %d came back with a result", at+1)
		}
	}
}

// A notebook comes back on the cell the list stood on, with the cells that were folded away
// still folded, so a reopened notebook reads as it was left.
func TestNotebookRestoresTheFocusAndTheFoldsEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	held.focusLiveCell(t, tab, "paid-orders")
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})
	tab.Focus = app.PaneResult
	held.press(t, tea.KeyPressMsg{Code: 's', Text: "s"})
	tab.Focus = app.PaneEditor
	held.press(t, tea.KeyPressMsg{Code: 'o', Text: "o"})
	if !findLiveCell(t, tab, "paid-orders").Folded {
		t.Fatalf("the cell did not fold")
	}
	if held.connection.Overlay.IsOpen() {
		t.Fatalf("a sort of a cell asked something: %q", held.connection.Overlay.Kind)
	}

	snapshot := held.connection.BuildWorkspaceSnapshot()
	again := app.NewConnection(held.connection.Session, nil, true)
	again.RestoreTabs(snapshot, func(db.TableRef) string { return "" })
	restored := again.Active()

	if restored.Notebook.Focused != tab.Notebook.Focused {
		t.Errorf("the list came back on cell %d, wanted %d",
			restored.Notebook.Focused+1, tab.Notebook.Focused+1)
	}
	at := restored.Notebook.FindCellIndex("paid-orders")
	if at < 0 || !restored.Notebook.Cells[at].Folded {
		t.Errorf("the folded cell came back open")
	}
	if len(restored.Sort) == 0 {
		t.Errorf("the sort of the focused cell did not come back")
	}
}

// The grid of one cell keeps its own cursor, its frozen columns and its staged changes, so
// the view of one cell is not the view of another.
func TestNotebookKeepsTheViewOfEveryCellEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	held.focusLiveCell(t, tab, "paid-orders")
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})
	tab.Focus = app.PaneResult
	held.press(t, tea.KeyPressMsg{Code: tea.KeyDown})
	held.press(t, tea.KeyPressMsg{Code: 'z', Text: "z"})
	row, frozen := tab.GridRow, len(tab.Frozen)
	if row == 0 || frozen == 0 {
		t.Fatalf("the grid stands on row %d with %d frozen columns", row, frozen)
	}

	tab.Focus = app.PaneEditor
	held.focusLiveCell(t, tab, "by-status")
	if tab.GridRow != 0 || len(tab.Frozen) != 0 {
		t.Errorf("the next cell took the view of another cell: row %d, %d frozen",
			tab.GridRow, len(tab.Frozen))
	}

	held.focusLiveCell(t, tab, "paid-orders")
	if tab.GridRow != row || len(tab.Frozen) != frozen {
		t.Errorf("the cell lost its view: row %d, %d frozen", tab.GridRow, len(tab.Frozen))
	}
}

// A write of one cell goes through the write plan of the profile, which measures it and
// keeps the undo of the rows it changed. A notebook that skipped the plan would write
// without one.
func TestNotebookMeasuresAWriteOfOneCellEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	tab.Notebook.FocusCell(tab.Notebook.FindCellIndex("hold-one"))
	tab.Editor = tab.Notebook.GetFocusedCell().Editor
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})

	if held.connection.Overlay.Kind != app.OverlayWritePlan {
		t.Fatalf("the card is %q, wanted the write plan", held.connection.Overlay.Kind)
	}
	if held.connection.Overlay.Plan.Rows != 1 {
		t.Errorf("the plan measured %d rows", held.connection.Overlay.Plan.Rows)
	}
	held.press(t, tea.KeyPressMsg{Code: 'y', Text: "y"})

	if held.connection.Undo == nil {
		t.Fatalf("the write kept no undo")
	}
	if status := held.readStatusOfOrder(t, 3); status != "held" {
		t.Errorf("order 3 is %q", status)
	}

	// The undo of the write puts the row back, once the question is answered.
	held.press(t, tea.KeyPressMsg{Code: 'u', Mod: tea.ModAlt})
	if held.connection.Overlay.Kind != app.OverlayConfirm {
		t.Fatalf("the undo asked nothing: %q", held.connection.Overlay.Kind)
	}
	held.press(t, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if status := held.readStatusOfOrder(t, 3); status != "cancelled" {
		t.Errorf("order 3 is %q after the undo", status)
	}
}

// The sort and the filter of the grid belong to the cell the reader sorted. A notebook that
// kept them on the tab would wrap the statement of the next cell in them, and read a column
// that result does not hold.
func TestNotebookKeepsTheSortOfOneCellOffTheNext(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	// The list moves with its own keys, because the keys are what carries the sort of a
	// cell over to it and back.
	held.focusLiveCell(t, tab, "by-status")
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})

	// The reader sorts the result of that cell.
	tab.Focus = app.PaneResult
	held.press(t, tea.KeyPressMsg{Code: 's', Text: "s"})
	if len(tab.Sort) == 0 {
		t.Fatalf("the grid did not sort the result")
	}

	// The next cell runs, and its statement is its own.
	tab.Focus = app.PaneEditor
	held.focusLiveCell(t, tab, "paid-orders")
	if len(tab.Sort) != 0 {
		t.Errorf("the next cell brought the sort of another cell: %#v", tab.Sort)
	}
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})

	outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "paid-orders"))
	if outcome.Kind != app.CellDone {
		t.Fatalf("the next cell is %q: %s", outcome.Kind, outcome.Message)
	}
	if active := tab.Results.Active(); active != nil &&
		strings.Contains(active.Read.Text, `order by "status"`) {
		t.Errorf("the next cell ran with the sort of another cell: %q", active.Read.Text)
	}

	// The cell that was sorted brings its sort back when the list returns to it.
	held.focusLiveCell(t, tab, "by-status")
	if len(tab.Sort) == 0 {
		t.Errorf("the sorted cell lost its sort")
	}
}

// A sort of one cell re-runs that cell alone: it binds the values of the parameter cells
// without asking again, and the cells it did not run say their rows are gone rather than
// pointing at the rows of another statement.
func TestNotebookSortRunsOnlyThatCellEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	held.press(t, tea.KeyPressMsg{Code: 'y', Text: "y"})
	for _, id := range []string{"by-status", "paid-orders"} {
		if tab.ReadCellOutcome(findLiveCell(t, tab, id)).Kind != app.CellDone {
			t.Fatalf("cell %q did not run", id)
		}
	}

	held.focusLiveCell(t, tab, "paid-orders")
	tab.Focus = app.PaneResult
	held.press(t, tea.KeyPressMsg{Code: 's', Text: "s"})

	if held.connection.Overlay.IsOpen() {
		t.Fatalf("the sort asked for something: %q", held.connection.Overlay.Kind)
	}
	sorted := findLiveCell(t, tab, "paid-orders")
	if outcome := tab.ReadCellOutcome(sorted); outcome.Kind != app.CellDone {
		t.Errorf("the sorted cell is %q: %s", outcome.Kind, outcome.Message)
	}
	if active := tab.Results.Active(); active == nil ||
		!strings.Contains(active.Read.Text, "order by") {
		t.Errorf("the sort did not reach the statement")
	}
	// The cell that is not in this run says so, and points at no result of it.
	other := findLiveCell(t, tab, "by-status")
	if outcome := tab.ReadCellOutcome(other); outcome.Kind != app.CellStale {
		t.Errorf("the cell outside the run is %q, wanted stale", outcome.Kind)
	}
	if other.FirstResult >= 0 {
		t.Errorf("the cell outside the run points at result %d", other.FirstResult)
	}
}

// A cell added and given a kind holds no statement of the cell before it, and the keys of
// the list reach every cell.
func TestNotebookAddsAndRunsANewCellEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	tab.Notebook.FocusCell(tab.Notebook.CountCells() - 1)
	tab.Editor = tab.Notebook.GetFocusedCell().Editor
	count := tab.Notebook.CountCells()

	held.press(t, tea.KeyPressMsg{Code: 'b', Text: "b"})
	if held.connection.Overlay.Kind != app.OverlayActionMenu {
		t.Fatalf("the kind menu did not open: %q", held.connection.Overlay.Kind)
	}
	held.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	if tab.Notebook.CountCells() != count+1 {
		t.Fatalf("cells: %d, wanted %d", tab.Notebook.CountCells(), count+1)
	}
	if !tab.Notebook.Editing {
		t.Fatalf("the new cell does not hold the caret")
	}

	// The text is written into the cell, and the last letter is typed, so the path a
	// keypress takes is the path of this cell as well. Each keypress schedules the check
	// of the statement, and the test runs that wait itself, so it types one letter.
	tab.Editor.SetText("select 7 as hel")
	held.press(t, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if tab.Editor.Text != "select 7 as held" {
		t.Fatalf("the cell holds %q", tab.Editor.Text)
	}
	held.press(t, tea.KeyPressMsg{Code: tea.KeyEscape})
	held.press(t, tea.KeyPressMsg{Code: tea.KeyEscape})
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})

	outcome := tab.ReadCellOutcome(tab.Notebook.GetFocusedCell())
	if outcome.Kind != app.CellDone || outcome.Rows != 1 {
		t.Errorf("the new cell is %q with %d rows: %s",
			outcome.Kind, outcome.Rows, outcome.Message)
	}
}

// A notebook that was never saved asks for a name, and the name it takes writes the file
// into the notebook directory of the user.
func TestNotebookSavesANewNotebookEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)

	held.press(t, tea.KeyPressMsg{Code: 'b', Mod: tea.ModAlt})
	tab := held.connection.Active()
	if tab.Kind != app.TabNotebook || tab.Notebook.Path != "" {
		t.Fatalf("the key opened %q with the path %q", tab.Kind, tab.Notebook.Path)
	}

	tab.Notebook.GetFocusedCell().Editor.SetText("-- every order\nselect count(*) from orders")
	held.press(t, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if held.connection.Overlay.Prompt != app.PromptNotebookName {
		t.Fatalf("the save asked for no name: %q", held.connection.Overlay.Prompt)
	}
	// The field of a prompt takes the text a key at a time, and it schedules no check.
	for _, letter := range "orders review" {
		held.press(t, tea.KeyPressMsg{Code: letter, Text: string(letter)})
	}
	held.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})

	if tab.Notebook.Path == "" {
		t.Fatalf("the notebook was not written")
	}
	if !strings.HasSuffix(tab.Notebook.Path, "orders-review"+notebook.FileSuffix) {
		t.Errorf("the file is %q", tab.Notebook.Path)
	}
	if tab.Notebook.Origin != notebook.OriginPersonal {
		t.Errorf("the origin is %q", tab.Notebook.Origin)
	}
	written, err := os.ReadFile(tab.Notebook.Path)
	if err != nil {
		t.Fatalf("the file cannot be read: %v", err)
	}
	again := notebook.Parse(string(written))
	if again.Title != "orders review" || len(again.Cells) != 1 {
		t.Errorf("the file holds %q and %d cells", again.Title, len(again.Cells))
	}
	// The file of a notebook holds no result of one.
	if strings.Contains(string(written), "count(*)") &&
		strings.Contains(string(written), "| ") {
		t.Errorf("the file holds a table of rows:\n%s", written)
	}
}

// The marked cells run, and no other cell runs with them.
func TestNotebookRunsTheMarkedCellsEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	held.focusLiveCell(t, tab, "paid-orders")
	held.press(t, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	held.focusLiveCell(t, tab, "by-status")
	held.press(t, tea.KeyPressMsg{Code: 'm', Text: "m"})

	if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "paid-orders")); outcome.Kind != app.CellDone {
		t.Errorf("the marked cell is %q: %s", outcome.Kind, outcome.Message)
	}
	if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "by-status")); outcome.Kind != app.CellIdle {
		t.Errorf("a cell that was not marked ran: %q", outcome.Kind)
	}
	if outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "hold-one")); outcome.Kind != app.CellIdle {
		t.Errorf("the write cell ran: %q", outcome.Kind)
	}
}

// A `:name` with no value in a parameter cell opens the form the editor opens, and the run
// goes on with the value it takes.
func TestNotebookAsksForAMissingValueEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t,
		"```sql id=by-status\n-- by status\n"+
			"select id from orders where status = :status order by id\n```\n")

	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if held.connection.Overlay.Kind != app.OverlayParameters {
		t.Fatalf("the run asked for no value: %q", held.connection.Overlay.Kind)
	}
	if len(held.connection.Overlay.Names) != 1 ||
		held.connection.Overlay.Names[0] != "status" {
		t.Errorf("the form asks for %v", held.connection.Overlay.Names)
	}

	// The form holds JSON, and the value of the mark is written into it.
	held.connection.Overlay.Draft = app.NewEditorBuffer(`{"status": "paid"}`, 0)
	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})

	outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "by-status"))
	if outcome.Kind != app.CellDone || outcome.Rows != 2 {
		t.Errorf("the cell is %q with %d rows: %s",
			outcome.Kind, outcome.Rows, outcome.Message)
	}
}

// The notebooks card lists the notebooks of the user and opens the row it stands on.
func TestNotebooksCardOpensARowEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	directory := notebook.ResolvePersonalDirectory()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("the notebook directory cannot be made: %v", err)
	}
	path := filepath.Join(directory, "review"+notebook.FileSuffix)
	if err := os.WriteFile(path, []byte(liveNotebook), 0o600); err != nil {
		t.Fatalf("the notebook cannot be written: %v", err)
	}

	held.press(t, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	held.press(t, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if held.connection.Overlay.Kind != app.OverlayNotebooks {
		t.Fatalf("the card is %q", held.connection.Overlay.Kind)
	}
	if len(held.connection.Overlay.Notebooks) != 1 {
		t.Fatalf("the card lists %d notebooks", len(held.connection.Overlay.Notebooks))
	}
	listed := held.connection.Overlay.Notebooks[0]
	if listed.Name != "review" || listed.Writes != 1 || listed.Cells != 6 {
		t.Errorf("the row says %q, %d writes, %d cells",
			listed.Name, listed.Writes, listed.Cells)
	}

	held.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})
	tab := held.connection.Active()
	if tab.Kind != app.TabNotebook || tab.Notebook.Path != path {
		t.Fatalf("the row opened %q with the path %q", tab.Kind, tab.Notebook.Path)
	}
	for at, cell := range tab.Notebook.Cells {
		if tab.ReadCellOutcome(cell).Kind != app.CellIdle {
			t.Errorf("cell %d ran on open", at+1)
		}
	}
}

// A cell that names another cell runs with the statement of that cell inside it, and none
// of its rows. A reference that carried rows would be a copy of a result the server no
// longer holds.
func TestNotebookRunsACellReferenceEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t,
		"```sql id=paid\n-- paid orders\n"+
			"select id, total_cents from orders where status = 'paid'\n```\n\n"+
			"```sql id=total\n-- the total of the paid orders\n"+
			"select count(*) as orders, sum(total_cents) as cents "+
			"from {{cell:paid}} as paid\n```\n")

	held.focusLiveCell(t, tab, "total")
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})

	outcome := tab.ReadCellOutcome(findLiveCell(t, tab, "total"))
	if outcome.Kind != app.CellDone {
		t.Fatalf("the cell is %q: %s", outcome.Kind, outcome.Message)
	}
	active := tab.Results.Active()
	if active == nil || len(active.State.Result.Rows) != 1 {
		t.Fatalf("the cell answered no row")
	}
	if held := core.FormatCell(active.State.Result.Rows[0][0], ""); held != "2" {
		t.Errorf("the reference counted %q rows, wanted 2", held)
	}
	if strings.Contains(active.Read.Text, "{{cell:") {
		t.Errorf("the reference reached the server: %q", active.Read.Text)
	}
	// The text of the cell keeps the reference as it was written.
	if !strings.Contains(findLiveCell(t, tab, "total").Editor.Text, "{{cell:paid}}") {
		t.Errorf("the reference is gone from the cell")
	}
}

// A reference that names no cell, or names itself, stops the run and says which reference
// it is, rather than sending text the server cannot read.
func TestNotebookRefusesABrokenReferenceEndToEnd(t *testing.T) {
	for _, one := range []struct {
		name   string
		text   string
		wanted string
	}{
		{
			name:   "a cell that is not there",
			text:   "```sql id=total\nselect * from {{cell:missing}} as held\n```\n",
			wanted: "no statement cell is named missing",
		},
		{
			name:   "a cell that names itself",
			text:   "```sql id=loop\nselect * from {{cell:loop}} as held\n```\n",
			wanted: "names itself",
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			held := buildLiveWorkspace(t, cfg.AccessWrite)
			tab := held.openLiveNotebook(t, one.text)
			held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})

			if held.connection.Notice == nil ||
				!strings.Contains(held.connection.Notice.Text, one.wanted) {
				t.Errorf("the notice is %v, wanted %q", held.connection.Notice, one.wanted)
			}
			for _, cell := range tab.Notebook.Cells {
				if tab.ReadCellOutcome(cell).Kind == app.CellDone {
					t.Errorf("a cell ran with a broken reference")
				}
			}
		})
	}
}

// Two notebooks on one connection keep their own cells and their own results, because the
// run of a tab is stamped with that tab.
func TestTwoNotebooksOnOneConnectionEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	first := held.openLiveNotebook(t,
		"```sql id=one\n-- one\nselect id from orders where status = 'paid'\n```\n")
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if outcome := first.ReadCellOutcome(findLiveCell(t, first, "one")); outcome.Rows != 2 {
		t.Fatalf("the first notebook answered %d rows", outcome.Rows)
	}

	// A second notebook opens in a tab of its own.
	second := held.connection.OpenNotebookInNewTab(
		notebook.Parse("```sql id=two\n-- two\nselect id from orders\n```\n"),
		"", notebook.OriginPersonal)
	second.Focus = app.PaneEditor
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})

	if outcome := second.ReadCellOutcome(findLiveCell(t, second, "two")); outcome.Rows != 3 {
		t.Errorf("the second notebook answered %d rows", outcome.Rows)
	}
	// The first notebook keeps what it read, because its results are its own.
	if outcome := first.ReadCellOutcome(findLiveCell(t, first, "one")); outcome.Rows != 2 {
		t.Errorf("the first notebook now answers %d rows", outcome.Rows)
	}
}

// A run refuses a second run of the same notebook, so two runs never write into one store.
func TestNotebookRefusesASecondRunEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)

	// The store of the tab is put into the running state, as one statement of a run does.
	tab.Results.Start([]string{"select 1"}, 100)
	if !tab.Results.IsRunning() {
		t.Fatalf("the store is not running")
	}
	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})

	if held.connection.Notice == nil ||
		!strings.Contains(held.connection.Notice.Text, "a query is running") {
		t.Errorf("the notice is %v", held.connection.Notice)
	}
}

// The report of a notebook holds its prose, its statements and the rows every cell
// answered. It is the one file the rows of a notebook are written to.
func TestNotebookWritesAReportEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t, liveNotebook)
	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	held.press(t, tea.KeyPressMsg{Code: 'y', Text: "y"})

	held.press(t, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if held.connection.Overlay.Prompt != app.PromptNotebookReport {
		t.Fatalf("the report asked for no file: %q", held.connection.Overlay.Prompt)
	}
	path := held.connection.Overlay.Draft.Text
	if !strings.HasSuffix(path, ".md") {
		t.Fatalf("the report goes to %q", path)
	}
	held.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the report cannot be read: %v", err)
	}
	report := string(written)
	for _, wanted := range []string{
		"# orders review", "# Orders review", "**Parameters**", "## orders by status",
		"| status | orders | cents |", "```sql", "▇",
	} {
		if !strings.Contains(report, wanted) {
			t.Errorf("the report holds no %q:\n%s", wanted, report)
		}
	}
	// The notebook file itself never holds a row, and the report does.
	if !strings.Contains(report, "| paid |") {
		t.Errorf("the report holds no row of the result:\n%s", report)
	}
	_ = tab
}

// A chart of values below zero draws them from the zero line, so a loss reads as a bar on
// the other side of it.
func TestNotebookChartHoldsValuesBelowZeroEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t,
		"```sql id=change\n-- change by day\n"+
			"select 'mon' as day, -40 as change union all "+
			"select 'tue', 80 union all select 'wed', -10 order by day\n```\n\n"+
			"```chart id=bars source=change label=day value=change kind=bar\n```\n")

	held.focusLiveCell(t, tab, "change")
	held.press(t, tea.KeyPressMsg{Code: 'r', Text: "r"})

	rows, problem := readCellChartRows(tab, notebook.ReadChart(
		notebook.Cell{Attrs: findLiveCell(t, tab, "bars").Attrs}))
	if problem != "" {
		t.Fatalf("the chart cannot draw: %s", problem)
	}
	bounds := present.ReadChartBounds(rows)
	if !bounds.HoldsNegative || bounds.Lowest != -40 || bounds.Highest != 80 {
		t.Fatalf("the bounds are %#v", bounds)
	}
	// A value below zero starts left of the zero line, and one above it starts at the line.
	from, filled := present.BuildChartCells(-40, bounds, 12)
	if from != 0 || filled == 0 {
		t.Errorf("a loss draws from %d for %d cells", from, filled)
	}
	above, cells := present.BuildChartCells(80, bounds, 12)
	if above == 0 || cells == 0 {
		t.Errorf("a gain draws from %d for %d cells", above, cells)
	}
}

// The card renames a notebook file, and the tab that holds it follows the new name.
func TestNotebooksCardRenamesAFileEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	directory := notebook.ResolvePersonalDirectory()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("the notebook directory cannot be made: %v", err)
	}
	path := filepath.Join(directory, "review"+notebook.FileSuffix)
	if err := os.WriteFile(path, []byte(liveNotebook), 0o600); err != nil {
		t.Fatalf("the notebook cannot be written: %v", err)
	}
	held.pump(t, readNotebookFile(held.model.ActiveID(), path, false))
	tab := held.connection.Active()

	held.press(t, tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt})
	held.press(t, tea.KeyPressMsg{Code: 'n', Text: "n"})
	held.press(t, tea.KeyPressMsg{Code: 'e', Text: "e"})
	if held.connection.Overlay.Prompt != app.PromptNotebookRename {
		t.Fatalf("the rename asked for no name: %q", held.connection.Overlay.Prompt)
	}
	held.connection.Overlay.Draft = app.NewEditorBuffer("weekly review", 0)
	held.press(t, tea.KeyPressMsg{Code: tea.KeyEnter})

	renamed := filepath.Join(directory, "weekly-review"+notebook.FileSuffix)
	if _, err := os.Stat(renamed); err != nil {
		t.Fatalf("the file was not renamed: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("the file it was renamed from is still there")
	}
	if tab.Notebook.Path != renamed {
		t.Errorf("the tab holds %q", tab.Notebook.Path)
	}
}

// A run the reader stopped runs no further cell, whatever the error policy of the notebook
// says. A stopped run that went on would send statements after the reader asked it to stop.
func TestNotebookStopEndsTheRunEndToEnd(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	tab := held.openLiveNotebook(t,
		"+++\n[run]\non_error = \"continue\"\n+++\n\n"+
			"```sql id=first\n-- first\nselect 1 as held\n```\n\n"+
			"```sql id=second\n-- second\nselect 2 as held\n```\n")

	// The reader stops the run, and the statement of the moment fails with the cancel.
	tab.Notebook.Stopped = true
	if continuesAfterFailure(tab) {
		t.Errorf("a stopped run still continues after a failure")
	}
	// A run that starts again clears the stop.
	held.press(t, tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if tab.Notebook.Stopped {
		t.Errorf("the stop of the last run is still held")
	}
	if !continuesAfterFailure(tab) {
		t.Errorf("the policy of this notebook does not continue after a failure")
	}
	for _, id := range []string{"first", "second"} {
		if tab.ReadCellOutcome(findLiveCell(t, tab, id)).Kind != app.CellDone {
			t.Errorf("cell %q did not run", id)
		}
	}
}
