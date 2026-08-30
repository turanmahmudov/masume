package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// markedHeadLabel is laid over the head that was kept. A read of the result writes the column
// names of the result, so the mark is gone wherever the head was read again.
const markedHeadLabel = "marked"

// markKeptGridHead lays the mark over the head the caches hold.
func markKeptGridHead(model *Model, key tabKey) {
	held, found := model.caches.readHead(key)
	if !found {
		return
	}
	held.labels = []string{markedHeadLabel}
	model.caches.keepHead(key, held)
}

// holdsMarkedGridHead is true while the head the caches hold is still the marked one.
func holdsMarkedGridHead(model *Model, key tabKey) bool {
	held, found := model.caches.readHead(key)
	return found && strings.Join(held.labels, "|") == markedHeadLabel
}

// The head of a result is read from several places in one frame, and reading it tokenizes the
// statement in the editor and matches every column name. Nothing changed, so it is kept.
func TestGridHeadIsKeptWhileNothingChanges(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	key := model.buildTabKey(connection, tab)

	first := model.resolveGridHead(key, connection, tab, tab.Results.Active())
	markKeptGridHead(model, key)
	for range 20 {
		model.buildGridShape(connection, tab)
	}
	if !holdsMarkedGridHead(model, key) {
		t.Error("the head was read again although nothing changed")
	}

	model.caches.keepHead(key, first)
	held := model.resolveGridHead(key, connection, tab, tab.Results.Active())
	if strings.Join(held.labels, "|") != strings.Join(first.labels, "|") {
		t.Errorf("the head that was kept names the columns %v, was %v",
			held.labels, first.labels)
	}
}

// One test per input the head is read from. A cache that misses one of these marks a sort the
// grid does not hold, or hides a column the reader asked to see.
func TestGridHeadIsReadAgainForEveryChange(t *testing.T) {
	cases := []struct {
		name   string
		change func(model *Model, connection *app.Connection, tab *app.Tab)
	}{
		{"another statement of the run is on screen", func(
			_ *Model, _ *app.Connection, tab *app.Tab,
		) {
			tab.Results.Start([]string{"select * from customers"}, 200)
			tab.Results.Succeed(0, db.ComposedRead{Text: "select * from customers"},
				db.QueryResult{
					Columns: []db.ResultColumn{{Name: "name", DataType: "text"}},
					Rows:    [][]any{{"ada"}},
				})
		}},
		{"the values of a masked column were shown", func(
			_ *Model, _ *app.Connection, tab *app.Tab,
		) {
			tab.Unmasked = true
		}},
		{"the grid laid a sort", func(_ *Model, _ *app.Connection, tab *app.Tab) {
			tab.Sort = []core.SortState{
				{Column: "customer", Direction: core.SortAscending},
			}
		}},
		{"the sort turned around", func(_ *Model, _ *app.Connection, tab *app.Tab) {
			tab.Sort = []core.SortState{
				{Column: "customer", Direction: core.SortDescending},
			}
		}},
		{"the statement in the editor changed", func(
			_ *Model, _ *app.Connection, tab *app.Tab,
		) {
			tab.Editor = app.NewEditorBuffer("select * from orders order by id", 0)
		}},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildGridModel(t)
			model.buildGridShape(connection, tab)
			key := model.buildTabKey(connection, tab)
			markKeptGridHead(model, key)

			held.change(model, connection, tab)
			model.buildGridShape(connection, tab)
			if holdsMarkedGridHead(model, key) {
				t.Error("the head was kept although " + held.name)
			}
		})
	}
}

// A sort of one column and a sort of another have to read apart, and so do two sorts whose
// column names run together to the same text.
func TestBuildSortKeyTellsSortsApart(t *testing.T) {
	cases := []struct {
		name string
		left []core.SortState
		held []core.SortState
	}{
		{
			name: "no sort and one sort",
			left: nil,
			held: []core.SortState{{Column: "id", Direction: core.SortAscending}},
		},
		{
			name: "the same column in the other direction",
			left: []core.SortState{{Column: "id", Direction: core.SortAscending}},
			held: []core.SortState{{Column: "id", Direction: core.SortDescending}},
		},
		{
			name: "two columns in the other order",
			left: []core.SortState{
				{Column: "id", Direction: core.SortAscending},
				{Column: "customer", Direction: core.SortAscending},
			},
			held: []core.SortState{
				{Column: "customer", Direction: core.SortAscending},
				{Column: "id", Direction: core.SortAscending},
			},
		},
		{
			name: "names that run together to the same text",
			left: []core.SortState{
				{Column: "ab", Direction: core.SortAscending},
				{Column: "c", Direction: core.SortAscending},
			},
			held: []core.SortState{
				{Column: "a", Direction: core.SortAscending},
				{Column: "bc", Direction: core.SortAscending},
			},
		},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			if buildSortKey(held.left) == buildSortKey(held.held) {
				t.Errorf("both sorts read as %q", buildSortKey(held.left))
			}
		})
	}
	// Two sorts that are the same read as the same key, or the head would be read again on
	// every frame.
	left := []core.SortState{{Column: "id", Direction: core.SortAscending}}
	held := []core.SortState{{Column: "id", Direction: core.SortAscending}}
	if buildSortKey(left) != buildSortKey(held) {
		t.Errorf("the same sort reads as %q and %q",
			buildSortKey(left), buildSortKey(held))
	}
}

// A cache that reads again and answers what it held before passes a test that only counts the
// reads, so the head itself is read: a sort marks the column it names, and a masked column is
// named as masked while its values are hidden.
func TestGridHeadCarriesWhatTheChangeAsksFor(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	model.buildGridShape(connection, tab)

	tab.Sort = []core.SortState{{Column: "customer", Direction: core.SortDescending}}
	held := model.resolveGridHead(model.buildTabKey(connection, tab), connection, tab,
		tab.Results.Active())
	if !strings.Contains(held.labels[1], "↓") {
		t.Errorf("the sorted column reads %q, without the mark of the sort", held.labels[1])
	}
	if strings.Contains(held.labels[0], "↓") || strings.Contains(held.labels[0], "↑") {
		t.Errorf("the column that is not sorted reads %q", held.labels[0])
	}

	tab.Sort = nil
	held = model.resolveGridHead(model.buildTabKey(connection, tab), connection, tab,
		tab.Results.Active())
	if strings.Contains(held.labels[1], "↓") {
		t.Errorf("the column still reads %q with the sort taken off", held.labels[1])
	}
}

// A column whose name suggests a secret is masked, and shown once the reader asks for it.
func TestGridHeadFollowsTheMasking(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	tab.Results.Start([]string{"select * from users"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select * from users"},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "id", DataType: "integer"},
				{Name: "password", DataType: "text"},
			},
			Rows: [][]any{{int64(1), "secret"}},
		})

	if !model.resolveGridHead(model.buildTabKey(connection, tab), connection, tab,
		tab.Results.Active()).masked[1] {
		t.Fatal("the password column is not masked")
	}
	if !strings.Contains(strings.Join(model.buildGridShape(connection, tab).Text[0], "|"),
		present.MaskedDisplay) {
		t.Error("the row does not hide the value of the masked column")
	}

	tab.Unmasked = true
	if model.resolveGridHead(model.buildTabKey(connection, tab), connection, tab,
		tab.Results.Active()).masked[1] {
		t.Error("the password column is still masked with its values asked for")
	}
	if !strings.Contains(strings.Join(model.buildGridShape(connection, tab).Text[0], "|"),
		"secret") {
		t.Error("the row does not show the value with the masking taken off")
	}
}
