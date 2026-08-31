package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// buildDocumentModel answers a model holding a page of documents the shape a collection has:
// an identity, plain fields, and the nested ones written as extended JSON. A document is
// where the cost of writing and measuring a page shows, because one field of it is a
// paragraph rather than a word.
func buildDocumentModel(reporter testing.TB, rows int) *Model {
	reporter.Helper()
	model := buildOfflineModelFor(reporter, 160, 48)
	connection := model.Active()

	tab := connection.Active()
	tab.Editor = app.NewEditorBuffer(`db.getSiblingDB("shop").orders.find({})`, 0)
	tab.Results.Start([]string{`db.getSiblingDB("shop").orders.find({})`}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: `db.getSiblingDB("shop").orders.find({})`,
		Pageable: true},
		db.QueryResult{Columns: buildDocumentColumns(), Rows: buildDocumentRows(0, rows)})
	tab.Focus = app.PaneResult
	return model
}

func buildDocumentColumns() []db.ResultColumn {
	names := []string{"_id", "number", "status", "total", "customer", "lines", "note"}
	types := []string{"objectId", "string", "string", "double", "object", "array", "string"}
	columns := make([]db.ResultColumn, 0, len(names))
	for at, name := range names {
		columns = append(columns, db.ResultColumn{Name: name, DataType: types[at]})
	}
	return columns
}

// buildDocumentRows answers rows numbered from this one on, so a page reads as the one after
// the rows already drawn.
func buildDocumentRows(from, count int) [][]any {
	statuses := []string{"new", "paid", "packed", "sent", "returned"}
	rows := make([][]any, 0, count)
	for at := from; at < from+count; at++ {
		rows = append(rows, []any{
			fmt.Sprintf("%024x", at),
			fmt.Sprintf("ORD-%d", 100000+at),
			statuses[at%len(statuses)],
			float64(at%90000) / 100,
			fmt.Sprintf(`{"name":"customer %d","address":{"city":"berlin","zip":"%d"}}`,
				at, 10000+at%9000),
			fmt.Sprintf(`[{"sku":"sku-%d","quantity":2},{"sku":"sku-%d","quantity":1}]`,
				at%5000, (at+1)%5000),
			strings.Repeat(fmt.Sprintf("a note about order %d. ", at), 8),
		})
	}
	return rows
}

// The rows arrive a page at a time, and a page leaves the rows before it as they were. The
// grid folds the page into what it wrote and measured for those rows, so the shape it draws
// has to be the one it would have built from every row at once.
func TestTheGridFoldsAPageIntoTheShapeItHas(t *testing.T) {
	for _, held := range []struct {
		name   string
		screen present.ScreenFilter
	}{
		{"with nothing hidden", present.NoScreenFilter()},
		{"with a search hiding rows", present.ScreenFilter{
			Values: map[int]map[string]bool{}, Search: "sku-7"}},
		{"with a column filtered", present.ScreenFilter{
			Values: map[int]map[string]bool{2: {"sent": true, "new": true}}}},
	} {
		t.Run(held.name, func(t *testing.T) {
			paged := buildDocumentModel(t, 300)
			pagedTab := paged.Active().Active()
			pagedTab.Screen = held.screen
			// The first page is drawn, then the next one lands and is drawn again.
			paged.buildGridShape(paged.Active(), pagedTab)
			pagedTab.Results.AppendRows(0, db.QueryResult{Rows: buildDocumentRows(300, 300)})
			folded := paged.buildGridShape(paged.Active(), pagedTab)

			atOnce := buildDocumentModel(t, 600)
			atOnceTab := atOnce.Active().Active()
			atOnceTab.Screen = held.screen
			wanted := atOnce.buildGridShape(atOnce.Active(), atOnceTab)

			if len(folded.Rows) != len(wanted.Rows) {
				t.Fatalf("folding left %d rows, wanted %d", len(folded.Rows), len(wanted.Rows))
			}
			if !reflect.DeepEqual(folded.Widths, wanted.Widths) {
				t.Errorf("folding measured %v, wanted %v", folded.Widths, wanted.Widths)
			}
			if !reflect.DeepEqual(folded.RowIndexes, wanted.RowIndexes) {
				t.Errorf("folding kept %d rows on screen, wanted %d",
					len(folded.RowIndexes), len(wanted.RowIndexes))
			}
			if !reflect.DeepEqual(folded.Text, wanted.Text) {
				t.Error("folding wrote different text for the rows on screen")
			}
		})
	}
}

// A filter laid after a page landed is measured over every row again, because the rows it
// hides are not the ones the shape was built for.
func TestTheGridShapesAgainWhenTheFilterChanges(t *testing.T) {
	model := buildDocumentModel(t, 300)
	tab := model.Active().Active()
	model.buildGridShape(model.Active(), tab)

	tab.Screen = present.ScreenFilter{Values: map[int]map[string]bool{2: {"sent": true}}}
	filtered := model.buildGridShape(model.Active(), tab)

	if len(filtered.Text) == 0 || len(filtered.Text) >= 300 {
		t.Fatalf("%d rows are on screen, wanted the ones one status keeps", len(filtered.Text))
	}
	for at, row := range filtered.Text {
		if row[2] != "sent" {
			t.Fatalf("row %d on screen reads %q, wanted the filtered status", at, row[2])
		}
	}
}

// A result read again is a different result, so nothing written for the one before it is
// kept: the rows of a re-read are written from the start.
func TestTheGridWritesEveryRowAgainAfterAReread(t *testing.T) {
	model := buildDocumentModel(t, 300)
	connection := model.Active()
	tab := connection.Active()
	model.buildGridShape(connection, tab)

	tab.Results.Succeed(0, db.ComposedRead{Text: "db.orders.find({})", Pageable: true},
		db.QueryResult{Columns: buildDocumentColumns(), Rows: buildDocumentRows(900, 40)})
	written := model.buildGridShape(connection, tab)

	if len(written.Text) != 40 {
		t.Fatalf("%d rows are on screen, wanted the 40 the re-read answered", len(written.Text))
	}
	if written.Text[0][1] != "ORD-100900" {
		t.Errorf("the first row reads %q, wanted the first of the re-read", written.Text[0][1])
	}
}
