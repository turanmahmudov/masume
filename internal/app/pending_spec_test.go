package app_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
)

// An inserted row is a map, so a snapshot that shared it would let a later edit write into
// the state an undo goes back to, and the undo would restore the edit it was meant to drop.
func TestUndoDoesNotCarryAnEditToAnInsertedRow(t *testing.T) {
	tab := app.NewQueryTab(1, "select * from orders")

	tab.StageChange(func(pending *core.PendingChanges) {
		pending.Inserts = append(pending.Inserts, map[string]any{"name": "first"})
	})
	tab.StageChange(func(pending *core.PendingChanges) {
		pending.Inserts[0]["name"] = "second"
	})

	if !tab.UndoChange() {
		t.Fatal("the undo did nothing")
	}
	if len(tab.Pending.Inserts) != 1 {
		t.Fatalf("the undo left %d inserted rows, wanted 1", len(tab.Pending.Inserts))
	}
	if held := tab.Pending.Inserts[0]["name"]; held != "first" {
		t.Errorf("the undo restored %q, wanted the row as it stood before the edit", held)
	}
}

func TestUndoDoesNotCarryAChangeToACell(t *testing.T) {
	tab := app.NewQueryTab(1, "select * from orders")
	key := core.BuildEditKey(0, 0)

	tab.StageChange(func(pending *core.PendingChanges) {
		pending.Edits[key] = core.CellEdit{
			Value: core.CellValue{Kind: core.CellText, Text: "first"},
		}
	})
	tab.StageChange(func(pending *core.PendingChanges) {
		pending.Edits[key] = core.CellEdit{
			Value: core.CellValue{Kind: core.CellText, Text: "second"},
		}
		pending.DeletedRows[3] = true
	})

	if !tab.UndoChange() {
		t.Fatal("the undo did nothing")
	}
	if held := tab.Pending.Edits[key].Value.Text; held != "first" {
		t.Errorf("the undo restored %q, wanted the value as it stood", held)
	}
	if tab.Pending.DeletedRows[3] {
		t.Error("the undo kept a row that was marked for deletion after it")
	}
}

// A redo puts back exactly what the undo took off, and the rows must not be shared with the
// staged work either.
func TestRedoPutsTheInsertedRowBackAsItWas(t *testing.T) {
	tab := app.NewQueryTab(1, "select * from orders")

	tab.StageChange(func(pending *core.PendingChanges) {
		pending.Inserts = append(pending.Inserts, map[string]any{"name": "first"})
	})
	tab.StageChange(func(pending *core.PendingChanges) {
		pending.Inserts[0]["name"] = "second"
	})
	if !tab.UndoChange() {
		t.Fatal("the undo did nothing")
	}
	if !tab.RedoChange() {
		t.Fatal("the redo did nothing")
	}
	if held := tab.Pending.Inserts[0]["name"]; held != "second" {
		t.Errorf("the redo restored %q, wanted the edit back", held)
	}
}

// A run of several statements reads several relations, and the grid keeps one set of staged
// work. Work staged against one result and applied while another is on screen would write
// the rows of the second through the relation of the first.
func TestStagedWorkStaysWithTheResultItWasStagedAgainst(t *testing.T) {
	tab := app.NewQueryTab(1, "select * from orders; select * from customers")
	tab.Results.Start([]string{"select * from orders", "select * from customers"}, 100)

	if !tab.StageChange(func(pending *core.PendingChanges) {
		pending.DeletedRows[0] = true
	}) {
		t.Fatal("the first change was refused")
	}
	if tab.HoldsChangesOfAnotherResult() {
		t.Error("the work reads as belonging to another result while it is on screen")
	}

	// The second statement of the same run is a different relation.
	tab.Results.SelectResult(1)
	if !tab.HoldsChangesOfAnotherResult() {
		t.Error("the work of the first result reads as the work of the second")
	}
	if tab.StageChange(func(pending *core.PendingChanges) {
		pending.DeletedRows[1] = true
	}) {
		t.Error("a change was staged against a second result while work stands on the first")
	}

	// Back on the result it was staged against, the work is its own again.
	tab.Results.SelectResult(0)
	if tab.HoldsChangesOfAnotherResult() {
		t.Error("the work does not read as the work of the result it was staged against")
	}

	// Once it is thrown away, the other result takes staged work of its own.
	tab.DiscardChanges()
	tab.Results.SelectResult(1)
	if !tab.StageChange(func(pending *core.PendingChanges) {
		pending.DeletedRows[1] = true
	}) {
		t.Error("the second result took no work of its own after the first was discarded")
	}
}

// The answer of an apply throws the staged work away. A change staged while that apply was
// still with the server would go with it, and the reader would never be told.
func TestNothingIsStagedWhileTheWorkIsBeingWritten(t *testing.T) {
	tab := app.NewQueryTab(1, "select * from orders")
	tab.Results.Start([]string{"select * from orders"}, 100)

	if !tab.StageChange(func(pending *core.PendingChanges) {
		pending.DeletedRows[0] = true
	}) {
		t.Fatal("the first change was refused")
	}

	tab.Applying = true
	if tab.StageChange(func(pending *core.PendingChanges) {
		pending.DeletedRows[1] = true
	}) {
		t.Error("a change was staged while the work was with the server")
	}

	tab.Applying = false
	if !tab.StageChange(func(pending *core.PendingChanges) {
		pending.DeletedRows[1] = true
	}) {
		t.Error("a change was refused after the apply answered")
	}
}
