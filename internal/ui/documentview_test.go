package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
)

// buildOrdersModel answers a model holding a result of documents, the shape a collection
// answers: an identity, plain fields, and the nested ones written so every type survives.
func buildOrdersModel(reporter testing.TB, rows int) *Model {
	reporter.Helper()
	model := buildOfflineModelFor(reporter, 160, 48)
	connection := model.Active()

	columns := []db.ResultColumn{
		{Name: "_id", DataType: core.DocumentTypeObjectID},
		{Name: "number", DataType: core.DocumentTypeString},
		{Name: "customer", DataType: core.DocumentTypeObject},
		{Name: "total", DataType: "mixed"},
	}
	values := make([][]any, 0, rows)
	for at := range rows {
		values = append(values, []any{
			strings.Repeat("0", 20) + string(rune('a'+at%26)) + "bcd",
			"ORD-" + string(rune('0'+at%10)),
			core.DocumentValue{Count: 2, Text: `{"city":"berlin",` +
				`"since":{"$date":{"$numberLong":"1600000000000"}}}`},
			int64(at),
		})
	}

	tab := connection.Active()
	tab.Editor = app.NewEditorBuffer(`db.orders.find({})`, 0)
	tab.Results.Start([]string{`db.orders.find({})`}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: `db.orders.find({})`, Pageable: true},
		db.QueryResult{Columns: columns, Rows: values})
	tab.Focus = app.PaneResult
	return model
}

// A result of documents offers the tree. A result of plain columns has nothing to open, so
// the strip does not carry a view that would draw the same rows again.
func TestTheTreeIsOfferedOnlyWhereAValueOpens(t *testing.T) {
	model := buildOrdersModel(t, 5)
	connection := model.Active()
	if !containsView(connection.Active().Views(connection.Session), app.ViewTree) {
		t.Error("a result of documents offers no tree")
	}

	plain := buildOfflineModelFor(t, 160, 48)
	held := plain.Active()
	plainTab := held.Active()
	plainTab.Results.Start([]string{"select * from people"}, 200)
	plainTab.Results.Succeed(0, db.ComposedRead{Text: "select * from people"},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "id", DataType: "integer"}, {Name: "name", DataType: "text"},
			},
			Rows: [][]any{{int64(1), "ada"}, {int64(2), "grace"}},
		})
	if containsView(plainTab.Views(held.Session), app.ViewTree) {
		t.Error("a result of plain columns offers a tree")
	}
}

func containsView(views []app.ResultView, wanted app.ResultView) bool {
	for _, view := range views {
		if view == wanted {
			return true
		}
	}
	return false
}

// The tree draws one row per document until a document is opened, so a result of many rows
// scrolls the way the grid does.
func TestTheTreeDrawsOneRowPerDocument(t *testing.T) {
	model := buildOrdersModel(t, 40)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree

	tree := model.buildDocumentTree(connection, tab)
	if tree.CountRows() != 40 {
		t.Fatalf("the tree draws %d rows, wanted one per document", tree.CountRows())
	}
	model.View()
}

// Opening a document with the key that opens the tree of the server shows its fields, and
// closing it puts them away again. It is the same gesture on the same kind of row.
func TestTheKeysOfTheTreeOpenAndCloseADocument(t *testing.T) {
	model := buildOrdersModel(t, 6)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()

	model.runDocumentTreeAction(connection, tab, Match{Action: ActionUnfoldRow})
	if held := model.buildDocumentTree(connection, tab).CountRows(); held != 10 {
		t.Fatalf("the tree draws %d rows, wanted the 6 documents and the 4 fields of one",
			held)
	}

	model.runDocumentTreeAction(connection, tab, Match{Action: ActionFoldRow})
	if held := model.buildDocumentTree(connection, tab).CountRows(); held != 6 {
		t.Fatalf("the tree draws %d rows after folding, wanted the 6 documents", held)
	}
}

// A key that closes a node already closed steps out to the node that holds it, which is
// what a tree does everywhere.
func TestClosingAClosedFieldStepsOutToTheDocument(t *testing.T) {
	model := buildOrdersModel(t, 3)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()

	model.runDocumentTreeAction(connection, tab, Match{Action: ActionUnfoldRow})
	// Down onto the second field of the document, which holds nothing to open.
	model.runDocumentTreeAction(connection, tab, Match{Action: ActionCursorDown})
	model.runDocumentTreeAction(connection, tab, Match{Action: ActionCursorDown})
	if tab.TreeRow != 2 {
		t.Fatalf("the cursor is on row %d, wanted the second field", tab.TreeRow)
	}

	model.runDocumentTreeAction(connection, tab, Match{Action: ActionFoldRow})
	if tab.TreeRow != 0 {
		t.Errorf("the cursor is on row %d, wanted the document that holds the field",
			tab.TreeRow)
	}
}

// The type a row of the tree names is the type of the value in hand. A field a sample found
// under several types names none of them, and the tree still has one value to name.
func TestTheTreeNamesTheTypeOfTheValueInHand(t *testing.T) {
	model := buildOrdersModel(t, 3)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()
	model.runDocumentTreeAction(connection, tab, Match{Action: ActionUnfoldRow})

	held := map[string]string{}
	for _, node := range model.buildDocumentTree(connection, tab).ReadWindow(0, 5) {
		held[node.Key] = node.Type
	}
	if held["total"] != core.DocumentTypeLong {
		t.Errorf("the total is a %q, wanted the type of the value and not of the column",
			held["total"])
	}
	if held["_id"] != core.DocumentTypeObjectID {
		t.Errorf("the identity is a %q", held["_id"])
	}
}

// A value inside a document keeps the type the server stores. This is the reason the tree
// is worth drawing rather than reading the JSON in the cell.
func TestTheTreeKeepsTheTypeOfANestedValue(t *testing.T) {
	model := buildOrdersModel(t, 3)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()

	tab.Opened[present.BuildRowPath(0)] = true
	tab.Opened[present.BuildRowPath(0)+"\x1f"+"customer"] = true

	held := map[string]present.DocumentNode{}
	for _, node := range model.buildDocumentTree(connection, tab).ReadWindow(0, 8) {
		if node.Depth == 2 {
			held[node.Key] = node
		}
	}
	if held["since"].Type != core.DocumentTypeDate {
		t.Errorf("the date inside the document is a %q", held["since"].Type)
	}
	if held["since"].Value != "2020-09-13 12:26:40.000" {
		t.Errorf("the date reads %q", held["since"].Value)
	}
	if held["city"].Value != "berlin" {
		t.Errorf("the city reads %q", held["city"].Value)
	}
}

// A grid cell draws the shape of a document, because the text of one cut to the width of a
// column says only what its first field is called.
func TestAGridCellDrawsTheShapeOfADocument(t *testing.T) {
	model := buildOrdersModel(t, 3)
	connection := model.Active()
	tab := connection.Active()
	shape := model.buildGridShape(connection, tab)

	if shape.Text[0][2] != "{ 2 fields }" {
		t.Errorf("the cell draws %q, wanted the shape of the document", shape.Text[0][2])
	}
	// The column is as wide as the shape, and not as wide as the text behind it.
	if shape.Widths[2] > len("{ 2 fields }")+2 {
		t.Errorf("the column is %d cells wide, wanted the width of the shape",
			shape.Widths[2])
	}
}

// The cursor of the tree stays on the screen as it moves, the way the cursor of the grid
// does. A tree drawn without it would scroll away from the row the reader is on.
func TestTheTreeHoldsItsCursorOnTheScreen(t *testing.T) {
	model := buildOrdersModel(t, 400)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()

	for range 120 {
		model.runDocumentTreeAction(connection, tab, Match{Action: ActionCursorDown})
	}
	model.View()

	if tab.TreeRow != 120 {
		t.Fatalf("the cursor is on row %d, wanted row 120", tab.TreeRow)
	}
	if tab.TreeRow < tab.TreeRowOffset {
		t.Errorf("the cursor at %d is above the window at %d", tab.TreeRow, tab.TreeRowOffset)
	}
}

// A move towards the foot of the tree reads the next page before the cursor gets there, the
// way the grid does. A tree that did not would stop at the first page whatever the collection
// holds, and a reader would be told the read answered 200 documents.
func TestWalkingToTheFootOfTheTreeReadsTheNextPage(t *testing.T) {
	const rows = 200
	model := buildOrdersModel(t, rows)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	// The server holds more than the page that was read.
	tab.Results.Active().State.Result.Truncated = true
	model.View()

	lookahead := resolveGridLookahead(model.layout.documentRows)
	tab.TreeRow = rows - lookahead - 2
	if _, command := model.runDocumentTreeAction(
		connection, tab, Match{Action: ActionCursorDown}); command != nil {
		t.Fatalf("a cursor at row %d of %d asked for the next page, %d rows early",
			tab.TreeRow, rows, rows-tab.TreeRow)
	}

	tab.TreeRow = rows - lookahead
	_, command := model.runDocumentTreeAction(
		connection, tab, Match{Action: ActionCursorDown})
	if command == nil {
		t.Fatalf("a cursor at row %d of %d, %d from the end, asked for nothing",
			tab.TreeRow, rows, rows-tab.TreeRow)
	}
	if !tab.Results.Active().FetchingMore {
		t.Error("the page was asked for and the result does not say one is being read")
	}
}

// A page that arrives while the tree is drawn tops the tree up, so a pane taller than a page
// fills itself rather than waiting for the reader to scroll.
func TestAPageThatArrivesTopsUpTheTree(t *testing.T) {
	model := buildOrdersModel(t, 30)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	tab.Results.Active().State.Result.Truncated = true
	model.View()

	if model.approachDrawnResultEnd(connection, tab) == nil {
		t.Error("a tree shorter than the pane asked for no more rows")
	}
}

// A tree of every row the server holds asks for nothing, however far the cursor walks.
func TestAWholeTreeAsksForNoMoreRows(t *testing.T) {
	model := buildOrdersModel(t, 30)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()

	tab.TreeRow = 29
	if _, command := model.runDocumentTreeAction(
		connection, tab, Match{Action: ActionCursorDown}); command != nil {
		t.Error("a tree of every row asked for another page")
	}
}

// A document the reader opened is named by its place in the result, not by where it is drawn.
// A filter laid over the rows must leave it open, and leave it on the same document.
func TestAFilterLeavesTheOpenedDocumentsWhereTheyWere(t *testing.T) {
	model := buildOrdersModel(t, 12)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()

	// Open the document drawn fourth, and read the identity it shows.
	tab.TreeRow = 3
	model.runDocumentTreeAction(connection, tab, Match{Action: ActionUnfoldRow})
	opened, _ := model.findDocumentNode(connection, tab)
	if !opened.Open {
		t.Fatal("the document did not open")
	}

	// A filter that hides the rows above it moves where it is drawn, and not which one it is.
	tab.Screen = present.ApplySearchTerm(present.NoScreenFilter(), "ORD-3")
	model.View()

	held := model.buildDocumentTree(connection, tab).ReadWindow(0, 4)
	if len(held) == 0 {
		t.Fatal("the filter hid every row")
	}
	if !held[0].Open {
		t.Errorf("the document %q closed when the filter was laid", held[0].Key)
	}
	if held[0].Key != opened.Key {
		t.Errorf("the open document is now %q, wanted %q", held[0].Key, opened.Key)
	}
}

// A result read again is a different result, so the documents opened against the one before
// it are dropped rather than reopened on whatever now stands in their place.
func TestAResultReadAgainDropsTheOpenedDocuments(t *testing.T) {
	model := buildOrdersModel(t, 8)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()
	model.runDocumentTreeAction(connection, tab, Match{Action: ActionUnfoldRow})
	if len(tab.Opened) == 0 {
		t.Fatal("the document did not open")
	}

	model.placeResultCursor(connection, tab, []query.ResultColumn{{Name: "other"}})
	if len(tab.Opened) != 0 {
		t.Errorf("%d documents stayed open across a result of other columns", len(tab.Opened))
	}
	if tab.TreeRow != 0 || tab.TreeRowOffset != 0 {
		t.Errorf("the cursor stayed at row %d of the result before it", tab.TreeRow)
	}
}

// Copying takes the value under the cursor, and a document is copied whole rather than as the
// shape the row draws in its place.
func TestCopyingTakesTheValueUnderTheCursor(t *testing.T) {
	model := buildOrdersModel(t, 4)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()
	model.runDocumentTreeAction(connection, tab, Match{Action: ActionUnfoldRow})

	// The third field of the document is the one that holds a document of its own.
	tab.TreeRow = 3
	model.runDocumentTreeAction(connection, tab, Match{Action: ActionCopyValue})
	if !strings.Contains(model.clipboard, "berlin") {
		t.Errorf("the clipboard holds %q, wanted the document the field holds", model.clipboard)
	}
	if strings.Contains(model.clipboard, "fields }") {
		t.Errorf("the clipboard holds %q, which is the shape and not the document",
			model.clipboard)
	}

	// A value inside the document is copied as the value, without its wrapper.
	tab.Opened[present.BuildRowPath(0)+"\x1f"+"customer"] = true
	tab.TreeRow = 4
	model.runDocumentTreeAction(connection, tab, Match{Action: ActionCopyValue})
	if model.clipboard != "berlin" {
		t.Errorf("the clipboard holds %q, wanted the value alone", model.clipboard)
	}
}

// Copying the path takes the name of the field, so a reader who found a value deep in a
// document can write a filter for it.
func TestCopyingThePathTakesTheNameOfTheField(t *testing.T) {
	model := buildOrdersModel(t, 4)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()
	model.runDocumentTreeAction(connection, tab, Match{Action: ActionUnfoldRow})
	tab.Opened[present.BuildRowPath(0)+"\x1f"+"customer"] = true

	tab.TreeRow = 4
	model.runDocumentTreeAction(connection, tab, Match{Action: ActionCopyPath})
	if model.clipboard != "customer.city" {
		t.Errorf("the clipboard holds %q, wanted the name of the field", model.clipboard)
	}
}

// A press moves the cursor of the tree to the row it landed on, the way a press moves the
// cursor of the grid. A second press on the same row opens it.
func TestAPressMovesTheCursorOfTheTree(t *testing.T) {
	model := buildOrdersModel(t, 20)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()

	block := model.layout.documentRowsHit
	if block.count == 0 {
		t.Fatal("the tree recorded no rows to press")
	}
	model.readMouse(tea.MouseClickMsg{
		X: block.from, Y: block.top + 4, Button: tea.MouseLeft,
	})
	if tab.TreeRow != 4 {
		t.Fatalf("the cursor is on row %d, wanted the row the press landed on", tab.TreeRow)
	}
	if len(tab.Opened) != 0 {
		t.Error("one press opened a document, wanted it to move the cursor only")
	}

	// The second press on the same row opens it.
	model.readMouse(tea.MouseClickMsg{
		X: block.from, Y: block.top + 4, Button: tea.MouseLeft,
	})
	if len(tab.Opened) != 1 {
		t.Errorf("a second press left %d documents open, wanted the one pressed",
			len(tab.Opened))
	}
}

// A press below the last row of the tree lands on nothing, rather than on the last row.
func TestAPressBelowTheTreeLandsOnNothing(t *testing.T) {
	model := buildOrdersModel(t, 3)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()

	block := model.layout.documentRowsHit
	model.readMouse(tea.MouseClickMsg{
		X: block.from, Y: block.top + 10, Button: tea.MouseLeft,
	})
	if tab.TreeRow != 0 {
		t.Errorf("the cursor moved to row %d for a press below the rows", tab.TreeRow)
	}
}

// The wheel moves the rows of the tree and leaves the cursor where it is, the way it does
// over the grid. The rows must not spring back to the cursor on the next frame.
func TestTheWheelMovesTheRowsOfTheTreeAndNotTheCursor(t *testing.T) {
	model := buildOrdersModel(t, 300)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	model.View()

	for range 6 {
		model.readMouseWheel(tea.MouseWheelMsg{
			X: model.layout.documentRowsHit.from,
			Y: model.layout.documentRowsHit.top, Button: tea.MouseWheelDown,
		})
	}
	model.View()

	if tab.TreeRowOffset == 0 {
		t.Fatal("the wheel moved no rows")
	}
	if tab.TreeRow != 0 {
		t.Errorf("the wheel moved the cursor to row %d, wanted it left alone", tab.TreeRow)
	}
}

// A filter laid in the tree is named on the strip over it, with the keys that take it off.
// A reader who hid rows and was told nothing would think the collection had lost them.
func TestTheTreeNamesTheFilterOverIt(t *testing.T) {
	model := buildOrdersModel(t, 20)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree

	if held := model.describeBanner(connection, tab); held != "" {
		t.Errorf("a tree with nothing laid over it named %q", held)
	}
	if !DrawsResultRows(app.ViewTree) {
		t.Fatal("the tree draws no strip for the filter over it")
	}

	tab.Screen = present.ApplySearchTerm(present.NoScreenFilter(), "ORD-3")
	banner := model.describeBanner(connection, tab)
	if !strings.Contains(banner, "ORD-3") {
		t.Errorf("the strip reads %q, wanted the term the reader searched for", banner)
	}

	// The strip names the keys of the view it stands over, so the chord it shows works.
	if resolveRewriteScope(app.ViewTree) != cfg.ScopeDocument {
		t.Error("the strip over the tree names the keys of another view")
	}
	if model.registry.FormatActionChordCompact(
		cfg.ScopeDocument, ActionClearRewrites) == "" {
		t.Error("the tree binds no key to clear the filter, and the strip offers one")
	}
}

// The key the strip names takes the filter off and reads the relation again.
func TestClearingTheFilterFromTheTreeTakesItOff(t *testing.T) {
	model := buildOrdersModel(t, 20)
	connection := model.Active()
	tab := connection.Active()
	tab.View = app.ViewTree
	tab.Screen = present.ApplySearchTerm(present.NoScreenFilter(), "ORD-3")
	model.View()

	model.runDocumentTreeAction(connection, tab, Match{
		Action: ActionClearRewrites, Scope: cfg.ScopeDocument,
	})
	if !tab.Screen.IsEmpty() {
		t.Errorf("the filter stayed on: %q", tab.Screen.Search)
	}
}

// A sort or a filter run from the tree reads the relation again, and must land back in the
// tree. A read that threw the reader into the grid every time would make the view unusable
// for anything but looking.
func TestAReadRunAgainLandsBackInTheTree(t *testing.T) {
	for _, held := range []struct {
		name  string
		view  app.ResultView
		wants app.ResultView
	}{
		{"the tree stays", app.ViewTree, app.ViewTree},
		{"the grid stays", app.ViewData, app.ViewData},
		// A view that draws something other than the rows steps aside for them.
		{"the columns step aside", app.ViewColumns, app.ViewData},
		{"the ddl steps aside", app.ViewDDL, app.ViewData},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := resolveRowView(held.view); answered != held.wants {
				t.Errorf("a read run again from %q lands on %q, wanted %q",
					held.view, answered, held.wants)
			}
		})
	}
}

// Drawing the tree deep into a result must not cost what drawing the rows above it would.
// A folded document is one row, so the rows the reader opened are the only ones read.
func BenchmarkDrawTheDocumentTree(b *testing.B) {
	for _, rows := range []int{1000, 100000} {
		b.Run(strings.Repeat("", 0)+formatRowCount(rows), func(b *testing.B) {
			model := buildOrdersModel(b, rows)
			tab := model.Active().Active()
			tab.View = app.ViewTree
			tab.TreeRow = rows - 1
			model.View()

			b.ReportAllocs()
			for b.Loop() {
				held, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
				model = held.(*Model)
				model.View()
			}
		})
	}
}

func formatRowCount(rows int) string {
	return present.FormatCount(int64(rows))
}
