package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// buildPagedModel answers a model whose grid holds the first page of a longer result: the
// server said there are more rows, and the read can be paged.
func buildPagedModel(t *testing.T, rows int) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()

	values := make([][]any, 0, rows)
	for at := range rows {
		values = append(values, []any{int64(at + 1), fmt.Sprintf("customer %d", at+1)})
	}
	tab.Results.Start([]string{"select * from orders"}, 200)
	tab.Results.Succeed(0,
		db.ComposedRead{Text: "select * from orders", Display: "select * from orders",
			Pageable: true},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "id", DataType: "integer"},
				{Name: "customer", DataType: "text"},
			},
			Rows: values, Truncated: true,
		})
	// The frame records how many rows the pane draws, which is what the lookahead reads.
	model.View()
	return model, connection, tab
}

// pressGridKey runs one key of the grid and reports whether it asked the server for anything.
func pressGridKey(
	model *Model, connection *app.Connection, tab *app.Tab, action ActionID,
) bool {
	_, command := model.runGridAction(connection, tab, Match{Action: action})
	return command != nil
}

// The lookahead is half a pane, and never less than ten rows: a whole pane reads too early on
// a tall terminal, and less is too late for a fast scroll.
func TestResolveGridLookaheadIsHalfThePane(t *testing.T) {
	cases := []struct {
		paneRows int
		want     int
	}{
		{paneRows: 0, want: 10},
		{paneRows: 8, want: 10},
		{paneRows: 20, want: 10},
		{paneRows: 21, want: 10},
		{paneRows: 40, want: 20},
		{paneRows: 61, want: 30},
	}
	for _, held := range cases {
		if found := resolveGridLookahead(held.paneRows); found != held.want {
			t.Errorf("a pane of %d rows reads %d ahead, wanted %d",
				held.paneRows, found, held.want)
		}
	}
}

// A move towards the foot of the grid reads the next page before the cursor gets there, so a
// reader holding a cursor key down is not stopped at the end of the page.
func TestWalkingToTheFootOfTheGridReadsTheNextPage(t *testing.T) {
	const rows = 200
	model, connection, tab := buildPagedModel(t, rows)
	lookahead := resolveGridLookahead(model.layout.gridRows.count)

	tab.GridRow = rows - lookahead - 2
	if pressGridKey(model, connection, tab, ActionCursorDown) {
		t.Fatalf("a cursor at row %d of %d asked for the next page, %d rows early",
			tab.GridRow, rows, rows-tab.GridRow)
	}
	if tab.Results.Active().FetchingMore {
		t.Fatal("no page was asked for, and the result says one is being read")
	}

	if !pressGridKey(model, connection, tab, ActionCursorDown) {
		t.Fatalf("a cursor at row %d of %d, %d from the end, asked for nothing",
			tab.GridRow, rows, rows-tab.GridRow)
	}
	if !tab.Results.Active().FetchingMore {
		t.Error("the page was asked for and the result does not say one is being read")
	}
}

// A jump to the last row lands at the end, so it reads the page after it.
func TestJumpingToTheLastRowReadsTheNextPage(t *testing.T) {
	model, connection, tab := buildPagedModel(t, 200)
	if !pressGridKey(model, connection, tab, ActionCursorLastRow) {
		t.Error("a jump to the last row asked for nothing")
	}
}

// What decides the read is where the cursor lands, not which way it went: a page up that stays
// inside the lookahead still leaves the reader near the end.
func TestAMoveThatLandsNearTheFootReadsTheNextPage(t *testing.T) {
	const rows = 200
	model, connection, tab := buildPagedModel(t, rows)
	tab.GridRow = rows - 1

	if !pressGridKey(model, connection, tab, ActionCursorPageUp) {
		t.Fatalf("a page up to row %d of %d asked for nothing", tab.GridRow, rows)
	}
	if landed := rows - tab.GridRow; landed > resolveGridLookahead(model.layout.gridRows.count) {
		t.Errorf("the cursor landed %d rows from the end, outside the lookahead", landed)
	}
}

// A move that goes nowhere near the end reads nothing.
func TestMovesAwayFromTheFootReadNothing(t *testing.T) {
	cases := []struct {
		name   string
		action ActionID
		row    int
	}{
		{name: "the cursor steps down high up the result", action: ActionCursorDown, row: 20},
		{name: "the cursor steps up at the foot", action: ActionCursorUp, row: 20},
		{name: "the cursor pages up away from the foot", action: ActionCursorPageUp, row: 100},
		{name: "the cursor jumps to the first row", action: ActionCursorFirstRow, row: 199},
		{name: "the cursor steps left at the foot", action: ActionCursorLeft, row: 199},
		{name: "the cursor steps right at the foot", action: ActionCursorRight, row: 199},
	}
	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildPagedModel(t, 200)
			tab.GridRow = held.row
			if pressGridKey(model, connection, tab, held.action) {
				t.Error("the next page was asked for although " + held.name)
			}
		})
	}
}

// The wheel moves no cursor, so the row that approaches the end is the last one drawn.
func TestRollingToTheFootOfTheGridReadsTheNextPage(t *testing.T) {
	const rows = 200
	model, _, tab := buildPagedModel(t, rows)
	drawn := model.layout.gridRows.count
	lookahead := resolveGridLookahead(drawn)

	// The wheel over the result pane, with the foot of the grid still short of the end.
	over := tea.Mouse{X: model.width - 4, Y: model.layout.resultTop + 1}
	tab.GridRowOffset = rows - drawn - lookahead - 2
	if _, command := model.rollWheel(over, 1); command != nil {
		t.Fatalf("the wheel asked for the next page with the foot at row %d of %d",
			tab.GridRowOffset+drawn, rows)
	}

	tab.GridRowOffset = rows - drawn - lookahead
	_, command := model.rollWheel(over, 1)
	if command == nil {
		t.Fatalf("the wheel asked for nothing with the foot at row %d of %d",
			tab.GridRowOffset+drawn, rows)
	}
	if !tab.Results.Active().FetchingMore {
		t.Error("the page was asked for and the result does not say one is being read")
	}
}

// The wheel outside the result pane moves the tree, and reads no page.
func TestRollingTheTreeReadsNoPage(t *testing.T) {
	model, _, tab := buildPagedModel(t, 200)
	tab.GridRowOffset = 199
	over := tea.Mouse{X: 2, Y: model.layout.treeRows.top}
	if _, command := model.rollWheel(over, 1); command != nil {
		t.Error("the wheel over the tree asked for the next page")
	}
}

// One test per reason the next page may not be read on its own.
func TestTheGridReadsAheadOnlyWhenItMay(t *testing.T) {
	cases := []struct {
		name   string
		change func(tab *app.Tab)
		may    bool
	}{
		{name: "the server holds more rows", change: func(*app.Tab) {}, may: true},
		{
			name: "every row was read",
			change: func(tab *app.Tab) {
				held := tab.Results.Active()
				result := held.State.Result
				result.Truncated = false
				held.State = app.QueryState{Kind: app.QuerySucceeded, Result: result}
			},
		},
		{
			name: "the read cannot be paged",
			change: func(tab *app.Tab) {
				tab.Results.Active().Read.Pageable = false
			},
		},
		// A screen filter drops rows after the server has sent them. Reading ahead under
		// one would walk the whole relation for rows the filter then throws away.
		{
			name: "a screen filter keeps some values",
			change: func(tab *app.Tab) {
				tab.Screen = present.ScreenFilter{
					Values: map[int]map[string]bool{1: {"customer 1": true}},
				}
			},
		},
		{
			name: "a screen filter searches the rows",
			change: func(tab *app.Tab) {
				tab.Screen = present.ScreenFilter{Search: "customer"}
			},
		},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildPagedModel(t, 200)
			held.change(tab)
			if found := model.canPrefetchGrid(tab); found != held.may {
				t.Fatalf("reading ahead is %v although %s", found, held.name)
			}

			tab.GridRow = 198
			if pressGridKey(model, connection, tab, ActionCursorDown) != held.may {
				t.Errorf("the key asked for a page: %v, although %s", !held.may, held.name)
			}
		})
	}
}

// The end can be reached again while a page read runs, and both reads would start at the same
// offset, so the second one is not sent.
func TestTheGridReadsOnePageAtATime(t *testing.T) {
	model, connection, tab := buildPagedModel(t, 200)
	tab.GridRow = 198
	if !pressGridKey(model, connection, tab, ActionCursorDown) {
		t.Fatal("the first approach of the end asked for nothing")
	}
	if pressGridKey(model, connection, tab, ActionCursorDown) {
		t.Error("a second page was asked for while the first one is being read")
	}
	if pressGridKey(model, connection, tab, ActionCursorUp) {
		t.Error("a page was asked for while one is being read")
	}
}

// The read runs on the server, so the footer says one is on its way.
func TestTheFooterSaysAPageIsBeingRead(t *testing.T) {
	model, connection, tab := buildPagedModel(t, 200)
	shape := model.buildGridShape(connection, tab)

	size, _ := model.describeGridFooter(tab, shape)
	if strings.Contains(size, "reading more") {
		t.Errorf("the footer reads %q with no page being read", size)
	}

	tab.Results.Active().FetchingMore = true
	size, _ = model.describeGridFooter(tab, shape)
	if !strings.Contains(size, "reading more") {
		t.Errorf("the footer reads %q while a page is being read", size)
	}
}

// A reader who asks for the next page and cannot have it is told why. The prefetch says
// nothing, because nobody asked it.
func TestAskingForAPageThatIsNotThereIsReported(t *testing.T) {
	model, connection, tab := buildPagedModel(t, 200)
	held := tab.Results.Active()
	result := held.State.Result
	result.Truncated = false
	held.State = app.QueryState{Kind: app.QuerySucceeded, Result: result}

	if _, command := model.fetchMore(connection, tab); command != nil {
		t.Error("a page was read although every row is already read")
	}
	if connection.Notice == nil {
		t.Fatal("the reader asked for a page that is not there and was told nothing")
	}

	connection.Notice = nil
	tab.GridRow = 199
	if pressGridKey(model, connection, tab, ActionCursorDown) {
		t.Error("the prefetch read a page although every row is already read")
	}
	if connection.Notice != nil {
		t.Errorf("the prefetch reported %q, and nobody asked it for a page",
			connection.Notice.Text)
	}
}

// pagingSession is a server that answers a second page, so the whole way from the scroll to
// the rows on screen can be walked.
type pagingSession struct {
	offlineSession
	// asked holds the window of every page read, in the order they were asked for.
	asked []db.ReadWindow
}

func (session *pagingSession) ReadPage(
	_ context.Context, _ db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	session.asked = append(session.asked, window)
	rows := make([][]any, 0, 40)
	for at := range 40 {
		place := window.Offset + at
		rows = append(rows, []any{int64(place + 1), fmt.Sprintf("customer %d", place+1)})
	}
	return db.QueryResult{
		Columns: []db.ResultColumn{
			{Name: "id", DataType: "integer"},
			{Name: "customer", DataType: "text"},
		},
		Rows: rows, Truncated: false,
	}, nil
}

// Scrolling to the foot of the grid reads the next page and puts its rows under the ones
// already there. This walks the whole way: the key, the command it answers, the message the
// server sends back, and the rows on screen after it.
func TestScrollingToTheFootLoadsMoreRows(t *testing.T) {
	const rows = 200
	model, connection, tab := buildPagedModel(t, rows)
	session := &pagingSession{offlineSession: *connection.Session.(*offlineSession)}
	connection.Session = session

	tab.GridRow = rows - 1
	held, command := model.runGridAction(connection, tab, Match{Action: ActionCursorDown})
	model = held.(*Model)
	if command == nil {
		t.Fatal("the cursor reached the last row and asked for no page")
	}

	message := command()
	page, held2 := message.(pageReadMsg)
	if !held2 {
		t.Fatalf("the command answered %T, not a page", message)
	}
	if len(session.asked) != 1 {
		t.Fatalf("the server was asked for %d pages, wanted 1", len(session.asked))
	}
	if session.asked[0].Offset != rows {
		t.Errorf("the page was asked for at offset %d, wanted %d",
			session.asked[0].Offset, rows)
	}
	if session.asked[0].Limit != tab.Results.Active().PageSize {
		t.Errorf("the page was asked for with a limit of %d, wanted %d",
			session.asked[0].Limit, tab.Results.Active().PageSize)
	}

	held, _ = model.Update(page)
	model = held.(*Model)

	active := connection.Active().Results.Active()
	if len(active.State.Result.Rows) != rows+40 {
		t.Fatalf("the grid holds %d rows, wanted %d", len(active.State.Result.Rows), rows+40)
	}
	// The rows of the page stand under the ones already read, in the order the server sent.
	if first := active.State.Result.Rows[rows][1]; first != "customer 201" {
		t.Errorf("the first row of the page reads %v, wanted customer 201", first)
	}
	if active.FetchingMore {
		t.Error("the page arrived and the result still says one is being read")
	}
	// The server sent the last of the rows, so nothing is read ahead after this.
	if active.State.Result.Truncated {
		t.Error("the server sent every row and the result still says there are more")
	}
	if model.canPrefetchGrid(connection.Active()) {
		t.Error("the grid would read ahead although every row is now read")
	}

	// The reader can now walk into the rows that arrived, and the grid draws them. The
	// window still stands where the cursor was, so the walk is what brings them into it.
	held, _ = model.runGridAction(connection, connection.Active(), Match{Action: ActionCursorLastRow})
	model = held.(*Model)
	if tab.GridRow != rows+40-1 {
		t.Fatalf("the last row of the grid is %d, wanted %d", tab.GridRow, rows+40-1)
	}
	if !strings.Contains(model.render(), "customer 240") {
		t.Error("the last row of the page that arrived is not drawn")
	}
}

// A page that fails is reported, and the rows already read stay.
func TestAFailedPageKeepsTheRowsAlreadyRead(t *testing.T) {
	const rows = 200
	model, connection, tab := buildPagedModel(t, rows)

	tab.GridRow = rows - 1
	if !pressGridKey(model, connection, tab, ActionCursorDown) {
		t.Fatal("the cursor reached the last row and asked for no page")
	}

	held, _ := model.Update(pageReadMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID,
		Index: tab.Results.ActiveIndex(), ResultID: tab.ReadActiveResultID(),
		Problem: "the server went away",
	})
	model = held.(*Model)

	active := connection.Active().Results.Active()
	if len(active.State.Result.Rows) != rows {
		t.Errorf("the grid holds %d rows after a failed page, wanted %d",
			len(active.State.Result.Rows), rows)
	}
	if active.FetchingMore {
		t.Error("the page failed and the result still says one is being read")
	}
	if connection.Notice == nil || !strings.Contains(connection.Notice.Text, "went away") {
		t.Error("the failure of the page was not reported")
	}
	// The reader can try again, because the rows are still short of the whole result.
	if !model.canPrefetchGrid(connection.Active()) {
		t.Error("the grid will not read ahead again after a page failed")
	}
}

// A pane taller than a page fills itself: each page that arrives and still leaves the foot of
// the grid near the end is followed by the next one, until the server runs out of rows.
func TestPagesFollowEachOtherUntilTheGridIsFull(t *testing.T) {
	model, connection, tab := buildPagedModel(t, 40)
	session := &pagingSession{offlineSession: *connection.Session.(*offlineSession)}
	connection.Session = session

	// The foot of the drawn grid sits at the end of the forty rows read, so the first page
	// is asked for at once.
	tab.GridRowOffset = 40 - model.layout.gridRows.count
	held, command := model.rollWheel(
		tea.Mouse{X: model.width - 4, Y: model.layout.resultTop + 1}, 1)
	model = held.(*Model)
	if command == nil {
		t.Fatal("the wheel at the foot of a short result asked for no page")
	}

	// Every page the command answers is fed back, as the loop of the program would.
	for at := 0; command != nil && at < 8; at++ {
		message := command()
		page, isPage := message.(pageReadMsg)
		if !isPage {
			t.Fatalf("the command answered %T, not a page", message)
		}
		held, command = model.Update(page)
		model = held.(*Model)
	}
	if command != nil {
		t.Fatal("the pages did not stop coming")
	}

	// The server answered a page that was not truncated, so the reading stopped there.
	active := connection.Active().Results.Active()
	if len(session.asked) != 1 {
		t.Errorf("the server was asked for %d pages, wanted 1", len(session.asked))
	}
	if len(active.State.Result.Rows) != 80 {
		t.Errorf("the grid holds %d rows, wanted 80", len(active.State.Result.Rows))
	}
	if active.FetchingMore {
		t.Error("the pages stopped and the result still says one is being read")
	}
}

// A page that brought no rows is not followed by another, or the same offset would be asked
// for again and again.
func TestAnEmptyPageIsNotFollowed(t *testing.T) {
	model, connection, tab := buildPagedModel(t, 200)
	tab.GridRow = 199
	if !pressGridKey(model, connection, tab, ActionCursorDown) {
		t.Fatal("the cursor reached the last row and asked for no page")
	}

	// The server says there are more rows and sends none of them.
	held, command := model.Update(pageReadMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID, Index: tab.Results.ActiveIndex(),
		ResultID: tab.ReadActiveResultID(),
		Result:   db.QueryResult{Rows: [][]any{}, Truncated: true},
	})
	if command != nil {
		t.Error("a page that brought no rows was followed by another")
	}
	if held.(*Model).Active().Active().Results.Active().FetchingMore {
		t.Error("the empty page left the result saying one is being read")
	}
}

// A tab that ran again holds another result at the same position. A page of the run before
// it belongs to nothing, and added to the result that took its place it would show the rows
// of one statement under the rows of another.
func TestAPageOfARunThatWasReplacedIsDropped(t *testing.T) {
	const rows = 200
	model, connection, tab := buildPagedModel(t, rows)

	tab.GridRow = rows - 1
	if !pressGridKey(model, connection, tab, ActionCursorDown) {
		t.Fatal("the cursor reached the last row and asked for no page")
	}
	stale := tab.ReadActiveResultID()

	// The tab runs something else while the page is still on its way.
	tab.Results.Start([]string{"select * from customers"}, 100)
	tab.Results.Succeed(0, db.ComposedRead{}, db.QueryResult{
		Columns: []db.ResultColumn{{Name: "id"}}, Rows: [][]any{{int64(1)}},
	})
	if fresh := tab.ReadActiveResultID(); fresh == stale {
		t.Fatal("the run that followed took the id of the one before it")
	}

	if _, command := model.Update(pageReadMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID, Index: 0, ResultID: stale,
		Result: db.QueryResult{Rows: [][]any{{int64(99)}}},
	}); command != nil {
		t.Error("a page of the run before this one asked for another")
	}

	active := connection.Active().Results.Active()
	if len(active.State.Result.Rows) != 1 {
		t.Errorf("the new result holds %d rows, wanted the one it read",
			len(active.State.Result.Rows))
	}
}

// A count of a run that was replaced belongs to nothing either, and written into the result
// that took its place it would report a size that result never had.
func TestACountOfARunThatWasReplacedIsDropped(t *testing.T) {
	model, connection, tab := buildPagedModel(t, 200)
	stale := tab.ReadActiveResultID()

	tab.Results.Start([]string{"select * from customers"}, 100)
	tab.Results.Succeed(0, db.ComposedRead{}, db.QueryResult{
		Columns: []db.ResultColumn{{Name: "id"}}, Rows: [][]any{{int64(1)}},
	})

	model.Update(countedMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID, Index: 0, ResultID: stale,
		Total: 9999, HasTotal: true,
	})

	active := connection.Active().Results.Active()
	if active.HasTotalRows {
		t.Errorf("the new result took the count of the run before it: %d", active.TotalRows)
	}
}
