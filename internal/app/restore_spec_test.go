package app_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
)

// buildPreview stands in for the read a restored table tab opens with.
func buildPreview(table db.TableRef) string {
	return "select * from " + table.Name
}

func TestBuildWorkspaceSnapshotHoldsEveryKindOfTab(t *testing.T) {
	connection := openConnection(t)
	connection.Tabs = []*app.Tab{
		app.NewQueryTab(1, "select 1"),
		app.NewTableTab(2, db.TableRef{Schema: "public", Name: "orders"}, "select * from orders"),
		app.NewObjectTab(3, db.SchemaObject{Schema: "public", Name: "order_totals"}),
	}
	connection.ActiveIndex = 2

	snapshot := connection.BuildWorkspaceSnapshot()
	if len(snapshot.Tabs) != 3 {
		t.Fatalf("the snapshot holds %d tabs, wanted 3", len(snapshot.Tabs))
	}
	if snapshot.ActiveIndex != 2 {
		t.Errorf("the snapshot puts the cursor on %d, wanted 2", snapshot.ActiveIndex)
	}
	for at, wanted := range []string{"query", "table", "object"} {
		if snapshot.Tabs[at].Kind != wanted {
			t.Errorf("tab %d is %q, wanted %q", at, snapshot.Tabs[at].Kind, wanted)
		}
	}
	if snapshot.Tabs[0].SQL != "select 1" {
		t.Errorf("the query tab kept %q", snapshot.Tabs[0].SQL)
	}
	if snapshot.Tabs[1].Name != "orders" {
		t.Errorf("the table tab kept %q", snapshot.Tabs[1].Name)
	}
}

func TestRestoreTabsOpensWhatTheSnapshotHeld(t *testing.T) {
	written := openConnection(t)
	written.Tabs = []*app.Tab{
		app.NewQueryTab(1, "select id from orders"),
		app.NewTableTab(2, db.TableRef{Schema: "public", Name: "orders"}, "select * from orders"),
	}
	written.ActiveIndex = 1
	snapshot := written.BuildWorkspaceSnapshot()

	read := openConnection(t)
	read.RestoreTabs(snapshot, buildPreview)

	if len(read.Tabs) != 2 {
		t.Fatalf("restored %d tabs, wanted 2", len(read.Tabs))
	}
	if read.ActiveIndex != 1 {
		t.Errorf("the cursor is on %d, wanted 1", read.ActiveIndex)
	}
	if read.Tabs[0].Editor.Text != "select id from orders" {
		t.Errorf("the query tab came back as %q", read.Tabs[0].Editor.Text)
	}
	if read.Tabs[1].Kind != app.TabTable || read.Tabs[1].Table.Name != "orders" {
		t.Errorf("the table tab came back as %v on %q",
			read.Tabs[1].Kind, read.Tabs[1].Table.Name)
	}
}

func TestRestoreTabsKeepsTheOneEmptyTabWhereNothingWasSaved(t *testing.T) {
	connection := openConnection(t)
	before := connection.Tabs[0]

	connection.RestoreTabs(hist.SavedWorkspace{}, buildPreview)

	if len(connection.Tabs) != 1 {
		t.Fatalf("holds %d tabs, wanted the one it opened with", len(connection.Tabs))
	}
	if connection.Tabs[0] != before {
		t.Error("replaced the tab it opened with, and there was nothing to restore")
	}
}

func TestRestoreTabsSettlesACursorOutsideTheTabsItRead(t *testing.T) {
	for _, saved := range []int{-1, 7} {
		connection := openConnection(t)
		connection.RestoreTabs(hist.SavedWorkspace{
			Tabs:        []hist.SavedTab{{Kind: "query", SQL: "select 1"}},
			ActiveIndex: saved,
		}, buildPreview)

		if connection.ActiveIndex != 0 {
			t.Errorf("a saved cursor of %d settled on %d, wanted 0",
				saved, connection.ActiveIndex)
		}
		if connection.Active() == nil {
			t.Errorf("a saved cursor of %d left no tab on screen", saved)
		}
	}
}

func TestRestoreTabsMarksOnlyTheTabsThatHaveToReadAgain(t *testing.T) {
	connection := openConnection(t)
	connection.RestoreTabs(hist.SavedWorkspace{Tabs: []hist.SavedTab{
		{Kind: "query", SQL: "select 1"},
		{Kind: "table", Schema: "public", Name: "orders"},
		{Kind: "object", Schema: "public", Name: "order_totals"},
	}}, buildPreview)

	if connection.TakeUnread(connection.Tabs[0]) {
		t.Error("a query tab was marked to read, and it holds its own statement")
	}
	for _, at := range []int{1, 2} {
		if !connection.TakeUnread(connection.Tabs[at]) {
			t.Errorf("tab %d was not marked to read what it describes", at)
		}
		// The mark is taken the first time, so a second ask answers nothing.
		if connection.TakeUnread(connection.Tabs[at]) {
			t.Errorf("tab %d stayed marked after it was read", at)
		}
	}
}

func TestRestoreTabsOpensATableTabWithThePreviewItWasGiven(t *testing.T) {
	connection := openConnection(t)
	connection.RestoreTabs(hist.SavedWorkspace{Tabs: []hist.SavedTab{
		{Kind: "table", Schema: "public", Name: "orders"},
	}}, buildPreview)

	if held := connection.Tabs[0].Editor.Text; held != "select * from orders" {
		t.Errorf("the table tab opened with %q", held)
	}
}

func TestRestoreTabsLaysTheSortAndTheFilterBackOn(t *testing.T) {
	connection := openConnection(t)
	connection.RestoreTabs(hist.SavedWorkspace{Tabs: []hist.SavedTab{{
		Kind: "query", SQL: "select id from orders",
		State: hist.SavedTabState{Caret: 7},
	}}}, buildPreview)

	tab := connection.Tabs[0]
	if tab.Editor.Caret != 7 {
		t.Errorf("the caret came back at %d, wanted 7", tab.Editor.Caret)
	}
	if tab.Editor.Anchor != 7 {
		t.Errorf("the anchor came back at %d, wanted it on the caret", tab.Editor.Anchor)
	}
}

func TestRestoreTabsIgnoresACaretOutsideTheStatementItRead(t *testing.T) {
	connection := openConnection(t)
	connection.RestoreTabs(hist.SavedWorkspace{Tabs: []hist.SavedTab{{
		Kind: "query", SQL: "select 1",
		State: hist.SavedTabState{Caret: 500},
	}}}, buildPreview)

	if caret := connection.Tabs[0].Editor.Caret; caret > len("select 1") {
		t.Errorf("the caret came back at %d, outside the statement", caret)
	}
}

func TestASnapshotOfRestoredTabsRoundTrips(t *testing.T) {
	first := openConnection(t)
	first.Tabs = []*app.Tab{
		app.NewQueryTab(1, "select 1"),
		app.NewTableTab(2, db.TableRef{Schema: "public", Name: "orders"}, "preview"),
	}
	first.ActiveIndex = 1

	second := openConnection(t)
	second.RestoreTabs(first.BuildWorkspaceSnapshot(), buildPreview)
	again := second.BuildWorkspaceSnapshot()

	wanted := first.BuildWorkspaceSnapshot()
	if len(again.Tabs) != len(wanted.Tabs) {
		t.Fatalf("the second snapshot holds %d tabs, wanted %d",
			len(again.Tabs), len(wanted.Tabs))
	}
	if again.ActiveIndex != wanted.ActiveIndex {
		t.Errorf("the cursor moved to %d over a round trip", again.ActiveIndex)
	}
	for at := range wanted.Tabs {
		if again.Tabs[at].Kind != wanted.Tabs[at].Kind {
			t.Errorf("tab %d changed kind to %q", at, again.Tabs[at].Kind)
		}
		if again.Tabs[at].Name != wanted.Tabs[at].Name {
			t.Errorf("tab %d changed name to %q", at, again.Tabs[at].Name)
		}
	}
}
