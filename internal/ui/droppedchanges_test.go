package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// stageCellEdits stages an edit on the first cell of that many rows.
func stageCellEdits(tab *app.Tab, rows int) {
	for at := range rows {
		row := at
		tab.StageChange(func(pending *core.PendingChanges) {
			pending.Edits[core.BuildEditKey(row, 0)] = core.CellEdit{
				RowIndex: row, ColumnIndex: 0,
				Value: core.CellValue{Kind: core.CellText, Text: "typed"},
			}
		})
	}
}

// succeedRead puts a result of a few rows into the tab, as a read of the relation would.
func succeedRead(tab *app.Tab) {
	tab.Results.Succeed(0,
		db.ComposedRead{Text: "select * from orders", Display: "select * from orders"},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "id", DataType: "integer"},
				{Name: "customer", DataType: "text"},
			},
			Rows: [][]any{{int64(1), "ada"}, {int64(2), "grace"}, {int64(3), "hedy"}},
		})
}

// buildTableTabModel answers a model whose tab is bound to a relation, which is the tab a
// reader edits cells in.
func buildTableTabModel(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	table := db.TableRef{Schema: "public", Name: "orders", Kind: db.RelationTable}
	connection.Catalog.Tables = []db.TableRef{table}
	tab := connection.OpenTable(table, "select * from public.orders")
	tab.Results.Start([]string{"select * from public.orders"}, 200)
	succeedRead(tab)
	return model, connection, tab
}

// A staged change names a row by its place in the result. A run puts other rows in those
// places, so the change goes with them and the reader is told. Silence reads as the client
// losing what they typed.
func TestRunningAStatementReportsTheChangesItDropped(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	tab.Editor = app.NewEditorBuffer("select * from orders", 0)
	tab.Results.Start([]string{"select * from orders"}, 200)
	succeedRead(tab)
	stageCellEdits(tab, 2)

	model.execute(connection, tab, []string{"select * from orders"})

	if core.CountChanges(tab.Pending) != 0 {
		t.Errorf("the run left %d changes staged", core.CountChanges(tab.Pending))
	}
	if connection.Notice == nil {
		t.Fatal("the run dropped the staged changes and reported nothing")
	}
	if !strings.Contains(connection.Notice.Text, "2 staged changes were dropped") {
		t.Errorf("the run reported %q", connection.Notice.Text)
	}
}

// A read of the relation replaces the rows just as a statement does, so it drops the staged
// work too. A change that outlived the read would name a row the reader never chose: the
// changes are applied through the place of the row in the result, so the rows that came in
// their place would be written instead.
func TestReadingTheRelationAgainReportsTheChangesItDropped(t *testing.T) {
	model, connection, tab := buildTableTabModel(t)
	stageCellEdits(tab, 3)

	model.runTabRead(connection, tab)

	if core.CountChanges(tab.Pending) != 0 {
		t.Errorf("the read left %d changes staged, which name rows it replaced",
			core.CountChanges(tab.Pending))
	}
	if connection.Notice == nil {
		t.Fatal("the read dropped the staged changes and reported nothing")
	}
	if !strings.Contains(connection.Notice.Text, "3 staged changes were dropped") {
		t.Errorf("the read reported %q", connection.Notice.Text)
	}
}

// Sorting a relation reads it again in another order, so the same rows stand in other places.
// Work staged before the sort would be written to whichever row landed in its place.
func TestSortingARelationDropsTheStagedChanges(t *testing.T) {
	model, connection, tab := buildTableTabModel(t)
	stageCellEdits(tab, 1)

	shape := model.buildGridShape(connection, tab)
	model.sortByColumn(connection, tab, shape, false)

	if core.CountChanges(tab.Pending) != 0 {
		t.Errorf("the sort left %d changes staged, which name places the rows have left",
			core.CountChanges(tab.Pending))
	}
	if connection.Notice == nil ||
		!strings.Contains(connection.Notice.Text, "1 staged change was dropped") {
		t.Error("the sort dropped the staged change and did not report it")
	}
}

// A run with nothing staged says nothing about changes, or every run would report on work
// that was never there.
func TestARunWithNothingStagedReportsNoDroppedChanges(t *testing.T) {
	for _, held := range []struct {
		name string
		run  func(model *Model, connection *app.Connection, tab *app.Tab)
	}{
		{
			name: "a statement of the reader",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				model.execute(connection, tab, []string{"select * from orders"})
			},
		},
		{
			name: "a read of the relation",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				model.runTabRead(connection, tab)
			},
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildTableTabModel(t)
			connection.Notice = nil
			held.run(model, connection, tab)
			if connection.Notice != nil &&
				strings.Contains(connection.Notice.Text, "dropped") {
				t.Errorf("%s reported %q with nothing staged",
					held.name, connection.Notice.Text)
			}
		})
	}
}

// Every path that replaces a result comes through one place, so none of them can keep staged
// work that names the rows it replaced.
func TestNoResultIsReplacedWithStagedWorkLeftOnIt(t *testing.T) {
	cases := []struct {
		name string
		run  func(model *Model, connection *app.Connection, tab *app.Tab)
	}{
		{
			name: "the reader runs a statement",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				model.execute(connection, tab, []string{"select * from orders"})
			},
		},
		{
			name: "the relation is read again",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				model.runTabRead(connection, tab)
			},
		},
		{
			name: "a column is sorted",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				shape := model.buildGridShape(connection, tab)
				model.sortByColumn(connection, tab, shape, false)
			},
		},
		{
			name: "a rewrite is cleared",
			run: func(model *Model, connection *app.Connection, tab *app.Tab) {
				tab.Filter = []core.FilterStep{{Kind: core.FilterRaw, Text: "id > 1"}}
				model.runGridAction(connection, tab, Match{Action: ActionClearRewrites})
			},
		},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildTableTabModel(t)
			stageCellEdits(tab, 2)
			held.run(model, connection, tab)
			if left := core.CountChanges(tab.Pending); left != 0 {
				t.Errorf("%d changes outlived the result they name after %s",
					left, held.name)
			}
		})
	}
}
