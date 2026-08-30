package ui

import (
	"fmt"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// buildGridModel answers a model whose tab holds a result of a few rows.
func buildGridModel(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()

	columns := []db.ResultColumn{
		{Name: "id", DataType: "integer"},
		{Name: "customer", DataType: "text"},
	}
	rows := [][]any{
		{int64(1), "ada"},
		{int64(2), "grace"},
		{int64(3), "a much longer customer name than the rest"},
	}
	tab.Results.Start([]string{"select * from orders"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select * from orders"},
		db.QueryResult{Columns: columns, Rows: rows})
	return model, connection, tab
}

// The shape is kept between frames, because the widths are read from every cell of the page.
// Nothing changed, so the same rows and widths are answered.
func TestBuildGridShapeIsKeptWhileNothingChanges(t *testing.T) {
	model, connection, tab := buildGridModel(t)

	first := model.buildGridShape(connection, tab)
	if len(first.Text) != 3 {
		t.Fatalf("the grid draws %d rows, wanted 3", len(first.Text))
	}
	for range 20 {
		held := model.buildGridShape(connection, tab)
		if len(held.Text) != len(first.Text) {
			t.Fatalf("the grid drew %d rows and then %d", len(first.Text), len(held.Text))
		}
	}
}

// A filter hides rows, so the shape has to be measured again: the rows drawn are fewer and a
// column may no longer need its widest cell.
func TestBuildGridShapeFollowsAFilter(t *testing.T) {
	model, connection, tab := buildGridModel(t)

	before := model.buildGridShape(connection, tab)
	wide := before.Widths[1]

	// Keep only the two short names, so the longest cell of the column is hidden.
	tab.Screen = present.ScreenFilter{Values: map[int]map[string]bool{
		1: {"ada": true, "grace": true},
	}}

	after := model.buildGridShape(connection, tab)
	if len(after.Text) != 2 {
		t.Fatalf("the filter left %d rows, wanted 2", len(after.Text))
	}
	if after.Widths[1] >= wide {
		t.Errorf("the column is %d cells wide with the longest row hidden, was %d",
			after.Widths[1], wide)
	}
	// The places of the rows kept point at the rows of the whole result.
	for _, at := range after.RowIndexes {
		if at < 0 || at >= 3 {
			t.Errorf("a row kept points at %d of the three read", at)
		}
	}
}

// Two filters that keep a different set of values must not answer one shape, which is what the
// banner alone could not tell apart.
func TestBuildGridShapeFollowsAFilterOfAnotherSetOfValues(t *testing.T) {
	model, connection, tab := buildGridModel(t)

	tab.Screen = present.ScreenFilter{Values: map[int]map[string]bool{1: {"ada": true}}}
	first := model.buildGridShape(connection, tab)

	tab.Screen = present.ScreenFilter{Values: map[int]map[string]bool{1: {"grace": true}}}
	second := model.buildGridShape(connection, tab)

	if len(first.Text) != 1 || len(second.Text) != 1 {
		t.Fatalf("the filters left %d and %d rows, wanted one each",
			len(first.Text), len(second.Text))
	}
	if first.Text[0][1] == second.Text[0][1] {
		t.Errorf("both filters drew %q, wanted the row each one keeps", first.Text[0][1])
	}
}

// A sort mark widens the header of its column, so the shape is measured again. A server that
// cannot order a read marks nothing, so the mark is drawn only where it can.
func TestBuildGridShapeFollowsTheSortMark(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	// A server that cannot order a read marks nothing, so this one can.
	connection.Session = &sortingSession{}
	model.buildGridShape(connection, tab)

	tab.Sort = []core.SortState{{Column: "id", Direction: core.SortAscending}}
	after := model.buildGridShape(connection, tab)

	if after.Labels[0] == "id" {
		t.Errorf("the header reads %q with a sort on it, wanted the mark", after.Labels[0])
	}
}

// A page of more rows changes the widest cell, so the widths are measured again rather than
// cutting a value the page now holds.
func TestBuildGridShapeFollowsMoreRowsArriving(t *testing.T) {
	model, connection, tab := buildGridModel(t)

	// A short page first, so the column has room to grow before it reaches its cap.
	short, connectionShort, tabShort := buildNarrowGridModel(t)
	before := short.buildGridShape(connectionShort, tabShort)
	wide := before.Widths[0]

	tabShort.Results.AppendRows(0, db.QueryResult{
		Rows: [][]any{{"a longer value than the page held"}},
	})

	after := short.buildGridShape(connectionShort, tabShort)
	if len(after.Text) != 2 {
		t.Fatalf("the grid draws %d rows after a page arrived, wanted 2", len(after.Text))
	}
	if after.Widths[0] <= wide {
		t.Errorf("the column is %d cells wide with a longer value in it, was %d",
			after.Widths[0], wide)
	}

	_ = model
	_ = connection
	_ = tab
}

// sortingSession answers a server that can order a read, so the header carries a sort mark.
type sortingSession struct {
	offlineSession
}

func (session *sortingSession) Capabilities() core.Capabilities {
	return core.Capabilities{SortsRead: true}
}

// buildNarrowGridModel answers a tab holding one short value, so a column can still grow.
func buildNarrowGridModel(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	tab.Results.Start([]string{"select * from held"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select * from held"}, db.QueryResult{
		Columns: []db.ResultColumn{{Name: "v", DataType: "text"}},
		Rows:    [][]any{{"ada"}},
	})
	return model, connection, tab
}

// Showing the values of a masked column changes what every cell of it reads, so the widths
// follow.
func TestBuildGridShapeFollowsTheMasking(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()

	tab.Results.Start([]string{"select * from users"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select * from users"}, db.QueryResult{
		Columns: []db.ResultColumn{{Name: "password", DataType: "text"}},
		Rows:    [][]any{{"a secret nobody should read from a screen"}},
	})

	masked := model.buildGridShape(connection, tab)
	tab.Unmasked = true
	shown := model.buildGridShape(connection, tab)

	if masked.Text[0][0] == shown.Text[0][0] {
		t.Errorf("the cell reads %q both masked and shown", masked.Text[0][0])
	}
}

// A second result of the same run is its own shape, so stepping between them never draws the
// rows of the other.
func TestBuildGridShapeFollowsTheResultOnScreen(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()

	tab.Results.Start([]string{"select 1", "select 2"}, 200)
	for at, name := range []string{"first", "second"} {
		tab.Results.Succeed(at, db.ComposedRead{Text: fmt.Sprintf("select %d", at+1)},
			db.QueryResult{
				Columns: []db.ResultColumn{{Name: "held", DataType: "text"}},
				Rows:    [][]any{{name}},
			})
	}

	tab.Results.SelectResult(0)
	first := model.buildGridShape(connection, tab)
	tab.Results.SelectResult(1)
	second := model.buildGridShape(connection, tab)

	if first.Text[0][0] == second.Text[0][0] {
		t.Errorf("both results drew %q", first.Text[0][0])
	}
}
