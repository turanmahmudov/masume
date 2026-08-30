package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
)

func TestScrollFromKeepsWhereTheWheelRolledTo(t *testing.T) {
	// Ten rows in a window of four. The cursor stands on the first row.
	for _, held := range []struct {
		name           string
		cursor, offset int
		rolled         bool
		wanted         int
	}{
		{"a cursor in view keeps the offset", 2, 1, false, 1},
		{"a cursor over the window pulls it up", 0, 5, false, 0},
		{"a cursor under the window pulls it down", 9, 0, false, 6},
		{"a rolled view keeps its offset", 0, 5, true, 5},
		{"a rolled view stops at the last rows", 0, 40, true, 6},
		{"a rolled view stops at the first row", 0, -3, true, 0},
	} {
		answered := scrollFrom(held.cursor, held.offset, 4, 10, held.rolled)
		if answered != held.wanted {
			t.Errorf("%s: answers %d, wanted %d", held.name, answered, held.wanted)
		}
	}
}

func TestScrollFromAnswersNothingForAnEmptyList(t *testing.T) {
	if answered := scrollFrom(0, 3, 4, 0, true); answered != 0 {
		t.Errorf("an empty list answers %d", answered)
	}
	if answered := scrollFrom(0, 3, 0, 10, true); answered != 0 {
		t.Errorf("a window of no rows answers %d", answered)
	}
}

// The tree draws its rows under the list of open connections, so the rows it can show are
// fewer than the pane is tall. An offset held to the pane instead would leave the last rows
// of the tree out of reach.
func TestScrollFromReachesTheLastRowOfAShortenedPane(t *testing.T) {
	const rows, header, count = 27, 2, 45
	body := rows - header

	// The wheel rolls past the end, and the draw holds it to the rows it can show.
	answered := scrollFrom(0, count, body, count, true)
	if last := answered + body; last != count {
		t.Errorf("the last row drawn is %d of %d", last, count)
	}
}

// The wheel moves the offset of a detail view without a floor of its own, and that offset is
// handed to scrollTo as the cursor as well. An answer of a row that is not there once made
// the frame read past the end of the rows it drew.
func TestScrollToAnswersARowThatIsThere(t *testing.T) {
	for _, held := range []struct {
		name           string
		cursor, offset int
		rows, count    int
		wanted         int
	}{
		{"rolled past the top", -5, -5, 10, 53, 0},
		{"rolled far past the top", -400, -400, 10, 53, 0},
		{"rolled past the bottom", 400, 400, 10, 53, 43},
		{"a cursor before the window", 2, 20, 10, 53, 2},
		{"a cursor past the window", 30, 0, 10, 53, 21},
		{"a window taller than the rows", 0, 0, 80, 53, 0},
	} {
		answered := scrollTo(held.cursor, held.offset, held.rows, held.count)
		if answered != held.wanted {
			t.Errorf("%s: answers %d, wanted %d", held.name, answered, held.wanted)
		}
		if answered < 0 || answered > held.count {
			t.Errorf("%s: answers %d, which is not a row of %d", held.name, answered, held.count)
		}
	}
}

// Every view of the result that is not the grid holds no cursor, so the keys of a list move
// the rows it shows. The draw holds the offset to the rows there are, so a step past the end
// is answered with the last of them.
func TestScrollDetailViewMovesTheRowsShown(t *testing.T) {
	tab := &app.Tab{}
	for _, held := range []struct {
		action ActionID
		wanted int
	}{
		{ActionCursorDown, 1},
		{ActionCursorDown, 2},
		{ActionCursorPageDown, 2 + listPage},
		{ActionCursorUp, 1 + listPage},
		{ActionCursorPageUp, 1},
		{ActionCursorFirstRow, 0},
		{ActionCursorUp, 0},
		{ActionCursorLastRow, detailLinesFloor},
	} {
		if !scrollDetailView(tab, Match{Action: held.action}) {
			t.Fatalf("%s did not belong to the view", held.action)
		}
		if tab.DetailOffset != held.wanted {
			t.Errorf("%s answers %d, wanted %d", held.action, tab.DetailOffset, held.wanted)
		}
	}
	if scrollDetailView(tab, Match{Action: ActionCopyValue}) {
		t.Error("a key of its own belonged to the view")
	}
}
