package ui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
)

// buildScrollingModel answers a model whose grid holds far more rows than the pane shows, so
// its bar draws a thumb that can be taken hold of.
func buildScrollingModel(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 140, 30)
	connection := model.Active()
	tab := connection.Active()

	rows := make([][]any, 0, 200)
	for at := range 200 {
		rows = append(rows, []any{int64(at), "row"})
	}
	tab.Results.Start([]string{"select * from orders"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select * from orders"}, db.QueryResult{
		Columns: []db.ResultColumn{
			{Name: "id", DataType: "integer"}, {Name: "name", DataType: "text"},
		},
		Rows: rows,
	})
	tab.Focus = app.PaneResult
	return model, connection, tab
}

// findGridScrollbar answers the bar of the grid, which is the only one a workspace with no
// card open draws over a result.
func findGridScrollbar(t *testing.T, model *Model) scrollHit {
	t.Helper()
	if len(model.layout.scrollbars) == 0 {
		t.Fatal("the frame drew no scroll bar at all")
	}
	return model.layout.scrollbars[0]
}

// The track a bar records has to be the cells the thumb was drawn in, or a press lands on a
// column that holds nothing.
func TestTheTrackOfABarStandsWhereItsThumbIsDrawn(t *testing.T) {
	model, _, _ := buildScrollingModel(t)
	frame := strings.Split(model.render(), "\n")
	bar := findGridScrollbar(t, model)

	found := 0
	for at := bar.top; at < bar.top+bar.rows && at < len(frame); at++ {
		switch cutRowText(frame[at], bar.column, bar.column) {
		case thumbFull, thumbUpper, thumbLower:
			found++
		}
	}
	if found == 0 {
		t.Errorf("the track runs from row %d over %d rows in column %d and holds no thumb",
			bar.top, bar.rows, bar.column)
	}
}

// A press on the track brings the thumb to the pointer, so the foot of the track shows the
// last rows and the head of it the first.
func TestAPressOnTheTrackBringsTheThumbToThePointer(t *testing.T) {
	model, _, tab := buildScrollingModel(t)
	model.render()
	bar := findGridScrollbar(t, model)

	model.readMouse(tea.MouseClickMsg{
		X: bar.column, Y: bar.top + bar.rows - 1, Button: tea.MouseLeft,
	})
	if tab.GridRowOffset != len(tab.Results.Active().State.Result.Rows)-bar.rows {
		t.Errorf("a press at the foot of the track answers offset %d, wanted the last rows",
			tab.GridRowOffset)
	}

	// A frame is drawn between one press and the next, so the bar knows where its thumb
	// stands now.
	model.render()
	bar = findGridScrollbar(t, model)
	model.readMouse(tea.MouseClickMsg{X: bar.column, Y: bar.top, Button: tea.MouseLeft})
	if tab.GridRowOffset != 0 {
		t.Errorf("a press at the head of the track answers offset %d", tab.GridRowOffset)
	}
}

// A drag of the thumb follows the pointer, and holds while the pointer wanders off the track.
func TestADragOfTheThumbFollowsThePointer(t *testing.T) {
	model, _, tab := buildScrollingModel(t)
	model.render()
	bar := findGridScrollbar(t, model)

	next, _ := model.readMouse(tea.MouseClickMsg{
		X: bar.column, Y: bar.top, Button: tea.MouseLeft,
	})
	model = next.(*Model)
	if !model.drag.holds(dragScrollbar) {
		t.Fatal("a press on the track began no drag")
	}

	// The pointer stands away from the column of the bar, and the drag holds.
	next, _ = model.readMouseMotion(tea.MouseMotionMsg{
		X: 4, Y: bar.top + bar.rows/2, Button: tea.MouseLeft,
	})
	model = next.(*Model)
	middle := tab.GridRowOffset
	if middle <= 0 {
		t.Errorf("a drag to the middle of the track answers offset %d", middle)
	}

	next, _ = model.readMouseRelease(tea.MouseReleaseMsg{X: 4, Y: bar.top + bar.rows/2})
	model = next.(*Model)
	if model.drag.holds(dragScrollbar) {
		t.Error("the drag outlived the release")
	}
	if tab.GridRowOffset != middle {
		t.Errorf("the release moved the rows from %d to %d", middle, tab.GridRowOffset)
	}
}

// A press on the thumb itself takes hold of it where it was pressed, so the rows do not jump
// under the pointer before the drag has moved at all.
func TestAPressOnTheThumbLeavesTheRowsWhereTheyStand(t *testing.T) {
	model, _, tab := buildScrollingModel(t)
	tab.GridRowOffset, tab.GridRolled = 90, true
	model.render()
	bar := findGridScrollbar(t, model)

	start, _, drawn := resolveThumbSpan(bar.offset, bar.rows, bar.total)
	if !drawn {
		t.Fatal("the bar drew no thumb")
	}
	model.readMouse(tea.MouseClickMsg{
		X: bar.column, Y: bar.top + start/2, Button: tea.MouseLeft,
	})
	if moved := tab.GridRowOffset - 90; moved < -2 || moved > 2 {
		t.Errorf("a press on the thumb moved the rows from 90 to %d", tab.GridRowOffset)
	}
}

// The row a thumb is drawn on and the offset a drag of it answers are two steps of the same
// sum, so a drag that puts the thumb back where it stood has to answer the offset it stood
// for. The track holds fewer rows than the view holds, so the answer is that offset rounded
// to the nearest row of the track.
func TestTheOffsetOfATrackAndTheRowOfItsThumbAgree(t *testing.T) {
	for _, held := range []struct{ viewport, total int }{
		{10, 11}, {10, 20}, {10, 200}, {10, 4000}, {3, 5000}, {40, 41}, {2, 900},
	} {
		room := held.total - held.viewport
		// One row of the track is two half cells, and the thumb runs over the half cells
		// the track has left over, so that is how far apart two rows of it stand.
		_, size, _ := resolveThumbSpan(0, held.viewport, held.total)
		step := max(2*room/max(held.viewport*2-size, 1), 1)
		for offset := 0; offset <= room; offset += max(room/17, 1) {
			start, _, drawn := resolveThumbSpan(offset, held.viewport, held.total)
			if !drawn {
				t.Fatalf("a view of %d rows in %d drew no thumb", held.total, held.viewport)
			}
			answered := resolveTrackOffset(start/2, held.viewport, held.total)
			if answered < offset-step || answered > offset+step {
				t.Errorf("a view of %d rows in %d at offset %d answers %d, off by more than %d",
					held.total, held.viewport, offset, answered, step)
			}
		}
	}
}

// Every offset a track can answer is a row the view holds, whatever row of it the pointer
// is on and however far the thumb was taken hold of from its top.
func TestATrackNeverAnswersARowTheViewHasNot(t *testing.T) {
	const viewport, total = 12, 300
	for row := -8; row < viewport+8; row++ {
		answered := resolveTrackOffset(row, viewport, total)
		if answered < 0 || answered > total-viewport {
			t.Errorf("row %d answers offset %d, outside 0..%d",
				row, answered, total-viewport)
		}
	}
	if answered := resolveTrackOffset(0, 1, total); answered != 0 {
		t.Errorf("a track of one row answers %d", answered)
	}
}

// findThumbRows answers the rows of one column that hold a piece of a scroll thumb.
func findThumbRows(frame []string, column int) []int {
	rows := []int{}
	for at, line := range frame {
		cells := []rune(stripStyles(line))
		if column >= len(cells) {
			continue
		}
		switch string(cells[column]) {
		case thumbFull, thumbUpper, thumbLower:
			rows = append(rows, at)
		}
	}
	return rows
}

// Every bar the frame records has to stand where its thumb was drawn. The panes and the cards
// each work their place out for themselves, and one cell out means a press lands on a column
// that holds nothing.
func TestEveryBarStandsWhereItsThumbIsDrawn(t *testing.T) {
	// The card of a row draws a bar only where the columns do not fit in it, which is what
	// this case is here to check.
	columns := make([]db.ResultColumn, 0, 40)
	values := make([]any, 0, 40)
	for at := range 40 {
		columns = append(columns, db.ResultColumn{
			Name: "column" + strconv.Itoa(at), DataType: "text"})
		values = append(values, "a value of this column")
	}

	for _, held := range []struct {
		name    string
		overlay app.Overlay
	}{
		{"no card at all", app.Overlay{}},
		{"the help", app.Overlay{Kind: app.OverlayHelp}},
		{"the card of a row", app.Overlay{
			Kind: app.OverlayRowDetail,
			Window: app.RowWindow{
				Columns: columns, Rows: [][]any{values, values}, Index: 0,
			},
		}},
		{"the viewer of a cell", app.Overlay{
			Kind: app.OverlayCell,
			Cell: app.CellTarget{
				Column: db.ResultColumn{Name: "note", DataType: "text"},
				Value:  strings.Repeat("a line of the value\n", 40),
			},
		}},
	} {
		t.Run(held.name, func(t *testing.T) {
			model, connection, _ := buildScrollingModel(t)
			held.overlay.Draft = app.NewEditorBuffer("", 0)
			connection.Overlay = held.overlay

			frame := strings.Split(model.render(), "\n")
			if len(model.layout.scrollbars) == 0 {
				t.Fatal("the frame recorded no bar at all")
			}
			for _, bar := range model.layout.scrollbars {
				rows := findThumbRows(frame, bar.column)
				if len(rows) == 0 {
					t.Errorf("the bar in column %d holds no thumb at all", bar.column)
					continue
				}
				for _, row := range rows {
					if row < bar.top || row >= bar.top+bar.rows {
						t.Errorf("the bar runs from row %d over %d rows and its thumb is drawn on row %d",
							bar.top, bar.rows, row)
					}
				}
			}
		})
	}
}

// The conversation of the chat draws its bar beside the rows rather than over the last cell
// of them, so its own place is checked as well.
func TestTheBarOfTheChatStandsWhereItsThumbIsDrawn(t *testing.T) {
	model := buildChattingModel(t, 30)
	frame := strings.Split(model.render(), "\n")

	found := false
	for _, bar := range model.layout.scrollbars {
		rows := findThumbRows(frame, bar.column)
		if len(rows) == 0 {
			continue
		}
		found = true
		for _, row := range rows {
			if row < bar.top || row >= bar.top+bar.rows {
				t.Errorf("the bar runs from row %d over %d rows and its thumb is drawn on row %d",
					bar.top, bar.rows, row)
			}
		}
	}
	if !found {
		t.Fatal("the conversation drew no thumb the frame records")
	}
}

// A drag of the bar of the chat moves the conversation, and leaves it following the newest
// row only where the drag reached the foot of the track.
func TestADragOfTheChatBarStopsItFollowing(t *testing.T) {
	model := buildChattingModel(t, 30)
	model.render()
	connection := model.Active()
	if len(model.layout.scrollbars) == 0 {
		t.Fatal("the conversation drew no bar")
	}
	bar := model.layout.scrollbars[len(model.layout.scrollbars)-1]

	model.readMouse(tea.MouseClickMsg{X: bar.column, Y: bar.top, Button: tea.MouseLeft})
	if connection.Chat.Offset != 0 {
		t.Errorf("a press at the head of the track answers offset %d", connection.Chat.Offset)
	}
	if connection.Chat.Follow {
		t.Error("the conversation still follows its newest row")
	}
}

// A drag that reaches the foot of a result the server has more of asks for the next page, as
// a roll of the wheel to the same place does. Without this the reader is stopped at the end
// of the page they dragged to and nothing says why.
func TestADragToTheFootOfAResultReadsTheNextPage(t *testing.T) {
	// One page is asked for at a time, so each way of reaching the foot is read on a model
	// of its own.
	pressed, pressedBar := buildPageableModel(t)
	if _, command := pressed.readMouse(tea.MouseClickMsg{
		X: pressedBar.column, Y: pressedBar.top + pressedBar.rows - 1,
		Button: tea.MouseLeft,
	}); command == nil {
		t.Error("a press at the foot of the track asked for nothing")
	}

	dragged, draggedBar := buildPageableModel(t)
	dragged.readMouse(tea.MouseClickMsg{
		X: draggedBar.column, Y: draggedBar.top, Button: tea.MouseLeft,
	})
	if _, command := dragged.readMouseMotion(tea.MouseMotionMsg{
		X: draggedBar.column, Y: draggedBar.top + draggedBar.rows - 1,
		Button: tea.MouseLeft,
	}); command == nil {
		t.Error("a drag to the foot of the track asked for nothing")
	}
}

// buildPageableModel answers a model whose grid holds one page of a result the server has
// more of, with the bar of that grid.
func buildPageableModel(t *testing.T) (*Model, scrollHit) {
	t.Helper()
	model, _, tab := buildScrollingModel(t)

	rows := make([][]any, 0, 200)
	for at := range 200 {
		rows = append(rows, []any{int64(at), "row"})
	}
	tab.Results.Start([]string{"select * from orders"}, 200)
	tab.Results.Succeed(0,
		db.ComposedRead{Text: "select * from orders", Pageable: true},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "id", DataType: "integer"}, {Name: "name", DataType: "text"},
			},
			Rows: rows, Truncated: true,
		})
	if !tab.Results.CanFetchMore() {
		t.Fatal("the result reports it cannot be paged")
	}
	model.render()
	return model, findGridScrollbar(t, model)
}

// A bar of a view the server has no more of asks for nothing, so a drag over a whole result
// reads the server no further.
func TestADragOverAWholeResultReadsNothingMore(t *testing.T) {
	model, _, _ := buildScrollingModel(t)
	model.render()
	bar := findGridScrollbar(t, model)

	if _, command := model.readMouse(tea.MouseClickMsg{
		X: bar.column, Y: bar.top + bar.rows - 1, Button: tea.MouseLeft,
	}); command != nil {
		t.Error("a whole result asked the server for more")
	}
}

// The statistics view and the definition view are drawn in the same pane at the same width, so
// the bar of each stands in the same column. One of them a column further out would draw its
// thumb outside the pane, and record a track a press never lands on.
func TestTheStatisticsBarStandsWhereTheDefinitionBarDoes(t *testing.T) {
	model := buildOfflineModel(t, 140, 30)
	tab := model.Active().Active()

	counted := make([]app.Statistic, 0, 40)
	lines := make([]string, 0, 40)
	for at := range 40 {
		counted = append(counted, app.Statistic{Label: "rows", Value: strconv.Itoa(at)})
		lines = append(lines, "select "+strconv.Itoa(at))
	}

	model.layout.scrollbars = nil
	model.renderStatistics(tab, counted, 60, 10)
	if len(model.layout.scrollbars) != 1 {
		t.Fatalf("the statistics view recorded %d bars", len(model.layout.scrollbars))
	}
	statistics := model.layout.scrollbars[0]

	model.layout.scrollbars = nil
	model.renderLines(tab, lines, 60, 10, true)
	if len(model.layout.scrollbars) != 1 {
		t.Fatalf("the definition view recorded %d bars", len(model.layout.scrollbars))
	}
	definition := model.layout.scrollbars[0]

	if statistics.column != definition.column {
		t.Errorf("the statistics bar stands in column %d and the definition bar in column %d",
			statistics.column, definition.column)
	}
}
