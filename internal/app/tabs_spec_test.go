package app_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
)

// openConnection answers a connection on a SQLite file, which needs no server.
func openConnection(t *testing.T) *app.Connection {
	t.Helper()
	session := openSqliteSession(t)
	return app.NewConnection(session, nil, true)
}

// A new connection opens with one tab, which is the invariant every reader of Active() leans
// on: a connection on screen always has a tab.
func TestANewConnectionOpensWithOneTab(t *testing.T) {
	connection := openConnection(t)
	if len(connection.Tabs) != 1 {
		t.Fatalf("a new connection holds %d tabs, wanted 1", len(connection.Tabs))
	}
	if connection.Active() == nil {
		t.Error("a new connection has no tab on screen")
	}
}

// Walking the tree must not leave a row of identical tabs behind, so a relation already open
// is brought forward rather than opened again.
func TestOpenTableBringsForwardTheTabThatHoldsIt(t *testing.T) {
	connection := openConnection(t)
	orders := db.TableRef{Schema: "public", Name: "orders", Kind: db.RelationTable}

	first := connection.OpenTable(orders, "select * from orders")
	count := len(connection.Tabs)

	again := connection.OpenTable(orders, "select * from orders")
	if again != first {
		t.Error("the relation was opened in a second tab")
	}
	if len(connection.Tabs) != count {
		t.Errorf("the connection holds %d tabs, wanted the %d it had", len(connection.Tabs), count)
	}
	if connection.Active() != first {
		t.Error("the tab that holds the relation was not brought forward")
	}
}

// Two filters of one relation are compared side by side, so a second tab on it can be asked
// for on purpose.
func TestOpenTableInNewTabAsksForASecondTabOnPurpose(t *testing.T) {
	connection := openConnection(t)
	orders := db.TableRef{Schema: "public", Name: "orders", Kind: db.RelationTable}

	first := connection.OpenTable(orders, "select * from orders")
	second := connection.OpenTableInNewTab(orders, "select * from orders")

	if first == second {
		t.Error("the second tab is the first one")
	}
	if connection.Active() != second {
		t.Error("the new tab was not brought to the front")
	}
}

// An empty tab is always a new one, so a reader can open several to write in. A statement takes
// the place of a blank tab rather than leaving it behind.
func TestOpenQueryTabTakesThePlaceOfABlankTab(t *testing.T) {
	connection := openConnection(t)
	// The connection opens with one blank tab, and a statement takes its place.
	count := len(connection.Tabs)
	connection.OpenQueryTab("select 1")
	if len(connection.Tabs) != count {
		t.Errorf("a statement made %d tabs, wanted the blank one used",
			len(connection.Tabs)-count+1)
	}

	// An empty tab is a new one, because it is asked for to write in.
	connection.OpenQueryTab("")
	if len(connection.Tabs) != count+1 {
		t.Errorf("an empty tab did not open: the connection holds %d", len(connection.Tabs))
	}
}

// The last tab cannot be closed, because a connection on screen always has one.
func TestCloseTabKeepsTheLastOne(t *testing.T) {
	connection := openConnection(t)
	connection.CloseTab(0)
	if len(connection.Tabs) != 1 {
		t.Errorf("the last tab was closed: the connection holds %d", len(connection.Tabs))
	}
	if connection.Active() == nil {
		t.Error("the connection has no tab on screen")
	}
}

func TestCloseTabHoldsAnIndexOutsideTheRow(t *testing.T) {
	connection := openConnection(t)
	connection.OpenQueryTab("")
	count := len(connection.Tabs)

	for _, index := range []int{-1, count, count + 50} {
		connection.CloseTab(index)
	}
	if len(connection.Tabs) != count {
		t.Errorf("an index outside the row closed a tab: %d left of %d",
			len(connection.Tabs), count)
	}
}

// The tab closed last can be opened again with what it held, so a close by mistake costs
// nothing.
func TestReopenTabBringsBackTheOneClosedLast(t *testing.T) {
	connection := openConnection(t)
	connection.OpenQueryTab("select 1")
	connection.OpenQueryTab("select 2")

	// Close the tab that holds the second statement.
	index := connection.ActiveIndex
	closed := connection.Tabs[index].Editor.Text
	connection.CloseTab(index)

	if !connection.ReopenTab() {
		t.Fatal("the tab was not opened again")
	}
	if held := connection.Active().Editor.Text; held != closed {
		t.Errorf("the tab came back holding %q, wanted %q", held, closed)
	}
}

func TestReopenTabAnswersNothingWhereNoneWasClosed(t *testing.T) {
	connection := openConnection(t)
	if connection.ReopenTab() {
		t.Error("a tab came back although none was closed")
	}
}

// A closed tab keeps the page it read, so a connection holds only the last few of them. A
// session that opens and closes tabs all day would otherwise hold every page it ever drew.
func TestCloseTabHoldsOnlyTheLastFewClosedTabs(t *testing.T) {
	connection := openConnection(t)
	for at := range 200 {
		connection.OpenQueryTab(fmt.Sprintf("select %d", at))
		connection.CloseTab(connection.ActiveIndex)
	}

	reopened := 0
	for connection.ReopenTab() {
		reopened++
	}
	if reopened != 10 {
		t.Errorf("%d tabs came back, wanted the last 10", reopened)
	}
}

// The tabs that come back are the ones closed last, in the order they were closed.
func TestReopenTabWalksBackFromTheTabClosedLast(t *testing.T) {
	connection := openConnection(t)
	for at := range 20 {
		connection.OpenQueryTab(fmt.Sprintf("select %d", at))
		connection.CloseTab(connection.ActiveIndex)
	}

	for at := 19; at >= 10; at-- {
		if !connection.ReopenTab() {
			t.Fatalf("the tab holding %d did not come back", at)
		}
		wanted := fmt.Sprintf("select %d", at)
		if held := connection.Active().Editor.Text; held != wanted {
			t.Fatalf("the tab came back holding %q, wanted %q", held, wanted)
		}
	}
	if connection.ReopenTab() {
		t.Error("a tab came back although only the last 10 are held")
	}
}

// Stepping through the tabs wraps, so the row can be walked in one direction.
func TestStepTabWrapsAtEachEnd(t *testing.T) {
	connection := openConnection(t)
	for range 2 {
		connection.OpenQueryTab("")
	}
	count := len(connection.Tabs)
	if count != 3 {
		t.Fatalf("the connection holds %d tabs, wanted 3", count)
	}

	connection.ActivateTab(count - 1)
	connection.StepTab(1)
	if connection.ActiveIndex != 0 {
		t.Errorf("stepping past the last tab landed on %d, wanted the first",
			connection.ActiveIndex)
	}

	connection.StepTab(-1)
	if connection.ActiveIndex != count-1 {
		t.Errorf("stepping back from the first landed on %d, wanted the last",
			connection.ActiveIndex)
	}
}

func TestActivateTabHoldsAnIndexOutsideTheRow(t *testing.T) {
	connection := openConnection(t)
	connection.OpenQueryTab("")

	for _, index := range []int{-5, 50} {
		connection.ActivateTab(index)
		if connection.ActiveIndex < 0 || connection.ActiveIndex >= len(connection.Tabs) {
			t.Errorf("an index of %d left the active tab at %d of %d",
				index, connection.ActiveIndex, len(connection.Tabs))
		}
	}
}

func TestIndexOfTabAnswersMinusOneForATabItHasNot(t *testing.T) {
	connection := openConnection(t)
	held := connection.Active()

	if at := connection.IndexOfTab(held.ID); at != connection.ActiveIndex {
		t.Errorf("the tab on screen reads as %d, wanted %d", at, connection.ActiveIndex)
	}
	if at := connection.IndexOfTab(9999); at != -1 {
		t.Errorf("a tab the connection has not reads as %d, wanted -1", at)
	}
}

// A report leaves the bar on its own, and an error stays longer because the user may have to
// act on it.
func TestShowAndShowErrorPutAReportInTheBar(t *testing.T) {
	connection := openConnection(t)

	connection.Show("read 3 rows")
	if connection.Notice == nil || connection.Notice.Tone != app.NoticeInfo {
		t.Errorf("the report reads %+v, wanted one of the plain tone", connection.Notice)
	}

	connection.ShowError("the server refused it")
	if connection.Notice == nil || connection.Notice.Tone != app.NoticeError {
		t.Errorf("the report reads %+v, wanted one of the error tone", connection.Notice)
	}
	if connection.Notice.ReadLife() <= app.NoticeLife {
		t.Error("an error leaves the bar as fast as a plain report")
	}
}

// openSqliteSession opens a session on a file of its own, so a test of the state needs no
// server and no stub of the port.
func openSqliteSession(t *testing.T) db.Session {
	t.Helper()
	path := t.TempDir() + "/shop.db"
	if err := writeEmptyFile(path); err != nil {
		t.Fatalf("cannot make the database file: %v", err)
	}
	session, err := engines.CreateAdapters().Open(testContext(), cfg.Profile{
		Name: "test", Engine: core.EngineSqlite, Database: path,
		AccessMode: cfg.AccessWrite, PageSize: 100,
	}, "")
	if err != nil {
		t.Fatalf("cannot open the file: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func writeEmptyFile(path string) error {
	return os.WriteFile(path, nil, 0o600)
}

func testContext() context.Context { return context.Background() }
