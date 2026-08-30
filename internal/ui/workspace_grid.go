package ui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/build"
	"github.com/turanmahmudov/masume/internal/query/result"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// GridShape is what the grid draws now: the rows on screen, their text, and the columns.
type GridShape struct {
	Columns []db.ResultColumn
	// Every row already read, as the server sent it.
	Rows [][]any
	// The rows the screen filter kept, as cell text.
	Text [][]string
	// The place of each kept row in the whole result, so an edit names the right row.
	RowIndexes []int
	Masked     map[int]bool
	// The name of each column with its sort mark, as the header draws it.
	Labels []string
	Widths []int
}

// buildGridShape returns what the grid draws for the result on screen.
func (model *Model) buildGridShape(connection *app.Connection, tab *app.Tab) GridShape {
	active := tab.Results.Active()
	if active == nil || active.State.Kind != app.QuerySucceeded {
		return GridShape{}
	}
	answered := active.State.Result

	key := model.buildTabKey(connection, tab)
	head := model.resolveGridHead(key, connection, tab, active)
	formatted := model.resolveGridText(key, tab, active, head.dataTypes, head.masked)
	text, indexes, widths := model.resolveGridShape(key, tab, formatted, head.labels)

	return GridShape{
		Columns: answered.Columns, Rows: answered.Rows, Text: text, RowIndexes: indexes,
		Masked: head.masked, Labels: head.labels,
		Widths: applyColumnWidths(widths, tab.ColumnWidths),
	}
}

// The widths a column is held between while its border is dragged: wide enough for a mark of
// its own, and no wider than the pane it is drawn in.
const (
	narrowestColumn = 3
	widestColumn    = 200
)

// applyColumnWidths lays the widths the reader set by hand over the widths the values asked
// for. A column the reader never dragged keeps the width of its widest value, and the widths
// the frame keeps for the result are left as they were.
func applyColumnWidths(widths []int, held map[int]int) []int {
	if len(held) == 0 {
		return widths
	}
	written := make([]int, len(widths))
	copy(written, widths)
	for index, width := range held {
		if index < 0 || index >= len(written) {
			continue
		}
		written[index] = clampColumnWidth(width)
	}
	return written
}

// clampColumnWidth holds a width a drag asked for between the two it is allowed.
func clampColumnWidth(width int) int {
	if width < narrowestColumn {
		return narrowestColumn
	}
	if width > widestColumn {
		return widestColumn
	}
	return width
}

// gridHead is the head of one result as the grid reads it: the sort the header marks, the
// columns whose values a mask hides, and the names, the types and the labels those are read
// from.
//
// It is kept because reading the sort tokenizes the whole statement in the editor and reading
// the mask matches every column name, and one frame asks for the shape of the grid from
// several places.
type gridHead struct {
	// Which statement of the run the head belongs to, and how many times it changed.
	result   int
	revision int
	// True while the values of a column whose name suggests a secret are shown.
	unmasked bool
	// The sort the tab laid, and the text the sort is read from where it laid none. A
	// server that cannot order a read marks nothing, which is the last of the three.
	sortKey    string
	editorText string
	sortsRead  bool

	names     []string
	dataTypes []string
	labels    []string
	masked    map[int]bool
}

// resolveGridHead returns the head of the result on screen, reading it only where the one it
// kept belongs to another result, another page of it, another masking or another sort.
func (model *Model) resolveGridHead(
	key tabKey, connection *app.Connection, tab *app.Tab, active *app.StatementResult,
) gridHead {
	sortsRead := connection.Session.Capabilities().SortsRead
	held, found := model.caches.readHead(key)
	if found && held.result == active.ID && held.revision == active.Revision &&
		held.unmasked == tab.Unmasked && held.sortsRead == sortsRead &&
		held.sortKey == buildSortKey(tab.Sort) && held.editorText == tab.Editor.Text {
		return held
	}

	result := active.State.Result
	sort := model.resolveGridSort(connection, tab)
	built := gridHead{
		result: active.ID, revision: active.Revision, unmasked: tab.Unmasked,
		sortKey: buildSortKey(tab.Sort), editorText: tab.Editor.Text, sortsRead: sortsRead,
		names:     make([]string, 0, len(result.Columns)),
		dataTypes: make([]string, 0, len(result.Columns)),
		labels:    make([]string, 0, len(result.Columns)),
	}
	for _, column := range result.Columns {
		built.names = append(built.names, column.Name)
		built.dataTypes = append(built.dataTypes, column.DataType)
		built.labels = append(built.labels,
			column.Name+describeSortMark(model.icons, sort, column.Name))
	}

	built.masked = map[int]bool{}
	if !tab.Unmasked {
		built.masked = present.FindMaskedColumns(built.names, present.DefaultMasking())
	}

	model.caches.keepHead(key, built)
	return built
}

// buildSortKey names one sort exactly. Every column name is written with its length before it,
// so two sorts whose names run together to the same text do not read as one.
func buildSortKey(sort []core.SortState) string {
	if len(sort) == 0 {
		return ""
	}
	var written strings.Builder
	for _, key := range sort {
		written.WriteString(strconv.Itoa(len(key.Column)))
		written.WriteByte(':')
		written.WriteString(key.Column)
		written.WriteString(string(key.Direction))
		written.WriteByte(';')
	}
	return written.String()
}

// resolveGridShape returns the rows the filter shows and the width of each column, measuring
// them only where the ones it kept were measured for another filter or another header.
//
// The widths are read from every cell of the page, so measuring them on every frame costs more
// than the whole of the rest of it.
func (model *Model) resolveGridShape(
	key tabKey, tab *app.Tab, formatted [][]string, labels []string,
) ([][]string, []int, []int) {
	screen := tab.Screen.Fingerprint()
	written := strings.Join(labels, "\x00")

	held, found := model.caches.readText(key)
	if found && held.shaped && held.screen == screen && held.labels == written {
		return held.shown, held.indexes, held.widths
	}

	text := make([][]string, 0, len(formatted))
	indexes := make([]int, 0, len(formatted))
	for at, row := range formatted {
		if !present.IsRowShown(row, tab.Screen) {
			continue
		}
		text = append(text, row)
		indexes = append(indexes, at)
	}
	// The label carries the sort mark, so a sorted column keeps room for it.
	widths := present.CalculateColumnWidths(labels, text)

	if found {
		held.shaped, held.screen, held.labels = true, screen, written
		held.shown, held.indexes, held.widths = text, indexes, widths
		model.caches.keepText(key, held)
	}
	return text, indexes, widths
}

// gridText is the rows of one result as the grid writes them, kept because writing every cell
// of a page costs more than the rest of the frame and the rows do not change between frames.
type gridText struct {
	// Which statement of the run the rows belong to, and how many times they changed.
	result   int
	revision int
	// True while the values of a column whose name suggests a secret are shown, because
	// that decides what a masked cell says.
	unmasked bool
	rows     [][]string

	// The shape drawn from those rows, kept for the same reason: the widths are measured
	// from every cell of the page, which costs more than everything else in the frame.
	//
	// The filter hides rows, and a sort mark widens a header, so the shape is kept only
	// while both are the ones it was measured for.
	shaped  bool
	screen  uint64
	labels  string
	shown   [][]string
	indexes []int
	widths  []int
}

// resolveGridText returns the rows as text, writing them only where the ones it kept belong
// to another result, another page of it, or another masking.
func (model *Model) resolveGridText(
	key tabKey, tab *app.Tab, active *app.StatementResult,
	dataTypes []string, masked map[int]bool,
) [][]string {
	held, found := model.caches.readText(key)
	if found && held.result == active.ID && held.revision == active.Revision &&
		held.unmasked == tab.Unmasked {
		return held.rows
	}
	written := present.FormatRows(active.State.Result.Rows, dataTypes, masked)
	model.caches.keepText(key, gridText{
		result: active.ID, revision: active.Revision, unmasked: tab.Unmasked, rows: written,
	})
	return written
}

// resolveGridSort returns the sort the header marks: the one the grid laid on, or the one the
// statement writes itself where the grid laid none. A server that cannot order a read marks
// nothing, because a mark would promise an order it cannot give.
func (model *Model) resolveGridSort(
	connection *app.Connection, tab *app.Tab,
) []core.SortState {
	if !connection.Session.Capabilities().SortsRead {
		return nil
	}
	if len(tab.Sort) > 0 {
		return tab.Sort
	}
	return statement.FindOrderByColumns(
		tab.Editor.Text, connection.Session.Dialect().Syntax)
}

// describeSortMark returns the mark a sorted column carries. A sort of several keys is
// numbered, because an arrow cannot show the order.
func describeSortMark(icons IconSet, sort []core.SortState, name string) string {
	for at, key := range sort {
		if key.Column != name {
			continue
		}
		arrow := " " + icons.Icon(cfg.IconSortUp)
		if key.Direction == core.SortDescending {
			arrow = " " + icons.Icon(cfg.IconSortDown)
		}
		if len(sort) == 1 {
			return arrow
		}
		return arrow + strconv.Itoa(at+1)
	}
	return ""
}

// minLookaheadRows is the smallest lookahead, so a page reaches a short pane before the reader
// needs it.
const minLookaheadRows = 10

// resolveGridLookahead returns how far above the last row read the next page is asked for.
// Half a pane: a whole pane reads too early on a tall terminal, and less is too late for a
// fast scroll.
func resolveGridLookahead(paneRows int) int {
	return max(minLookaheadRows, paneRows/2)
}

// canPrefetchGrid is true where the next page may be read without being asked for. A screen
// filter drops rows after the server has sent them, so reading ahead under one would walk the
// whole relation for rows the filter then throws away.
func (model *Model) canPrefetchGrid(tab *app.Tab) bool {
	return tab.Results.CanFetchMore() && tab.Screen.IsEmpty()
}

// approachGridEnd reads the next page where that row stands inside the lookahead of the last
// row read, so a reader who walks to the foot of the grid is not stopped at the end of the
// page. A key moves the cursor, so the caller names the row of the cursor; the wheel moves no
// cursor, so it names the last row on screen.
func (model *Model) approachGridEnd(
	connection *app.Connection, tab *app.Tab, position, rows int,
) tea.Cmd {
	if rows == 0 || !model.canPrefetchGrid(tab) {
		return nil
	}
	if position < rows-resolveGridLookahead(model.layout.gridRows.count) {
		return nil
	}
	return model.readMoreRows(connection, tab)
}

// approachDrawnGridEnd reads the next page where the foot of the grid stands inside the
// lookahead of the last row read. Nothing here moved the cursor: the wheel rolled, or a page
// arrived and made the grid longer, so the row that approaches the end is the last one drawn.
func (model *Model) approachDrawnGridEnd(
	connection *app.Connection, tab *app.Tab,
) tea.Cmd {
	if app.ResolveDrawnView(tab.Views(connection.Session), tab.View) != app.ViewData {
		return nil
	}
	return model.approachGridEnd(connection, tab,
		tab.GridRowOffset+model.layout.gridRows.count,
		len(model.buildGridShape(connection, tab).Text))
}

// runGridAction returns the keys of the result grid.
func (model *Model) runGridAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (tea.Model, tea.Cmd) {
	shape := model.buildGridShape(connection, tab)
	rowCount := len(shape.Text)
	columnCount := len(shape.Columns)

	// A move of the cursor brings it back into view, whatever the wheel rolled to.
	switch match.Action {
	case ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown,
		ActionCursorFirstRow, ActionCursorLastRow:
		tab.GridRolled = false
	}

	switch match.Action {
	case ActionCursorUp:
		tab.GridRow = clamp(tab.GridRow-1, rowCount)
	case ActionCursorDown:
		tab.GridRow = clamp(tab.GridRow+1, rowCount)
	case ActionCursorPageUp:
		tab.GridRow = clamp(tab.GridRow-listPage, rowCount)
	case ActionCursorPageDown:
		tab.GridRow = clamp(tab.GridRow+listPage, rowCount)
	case ActionCursorFirstRow:
		tab.GridRow = 0
	case ActionCursorLastRow:
		tab.GridRow = clamp(rowCount-1, rowCount)
	case ActionCursorLeft:
		moveGridColumn(tab, columnCount, -1)
	case ActionCursorRight:
		moveGridColumn(tab, columnCount, 1)

	case ActionCountRows:
		return model.countRows(connection, tab)

	case ActionSortColumn, ActionAddSortColumn:
		return model.sortByColumn(connection, tab, shape, match.Action == ActionAddSortColumn)
	case ActionClearRewrites:
		if !tab.HasRewrite() && tab.Screen.IsEmpty() {
			return model, nil
		}
		tab.Sort, tab.Filter = nil, nil
		tab.Screen = present.NoScreenFilter()
		return model.runTabRead(connection, tab)
	case ActionPopFilter:
		if len(tab.Filter) == 0 {
			return model, nil
		}
		tab.Filter = tab.Filter[:len(tab.Filter)-1]
		return model.runTabRead(connection, tab)
	case ActionFilterByCell, ActionExcludeCell:
		return model.filterByCell(connection, tab, shape, match.Action == ActionExcludeCell)
	case ActionFilterWhere:
		written := ""
		for _, step := range tab.Filter {
			if step.Kind == core.FilterRaw {
				written = step.Text
			}
		}
		connection.Overlay = app.Overlay{
			Kind: app.OverlayPrompt, Prompt: app.PromptWhere, Title: "where",
			Hint:  "a predicate the server reads; empty clears it",
			Draft: app.NewEditorBuffer(written, len(written)),
		}
	case ActionSearchColumns:
		connection.Overlay = app.Overlay{
			Kind: app.OverlayPrompt, Prompt: app.PromptSearch, Title: "search",
			Hint:  "searches the rows on screen; empty clears it",
			Draft: app.NewEditorBuffer(tab.Screen.Search, len(tab.Screen.Search)),
		}
	case ActionGoToColumn:
		connection.Overlay = app.Overlay{
			Kind: app.OverlayPrompt, Prompt: app.PromptGoToColumn, Title: "column",
			Hint:  "the name of a column of this result",
			Draft: app.NewEditorBuffer("", 0),
		}
	case ActionFilterByValues:
		return model.askValueFilter(connection, tab, shape)

	case ActionFreezeColumns:
		return model.freezeColumns(connection, tab), nil
	case ActionToggleMasking:
		tab.Unmasked = !tab.Unmasked
		if tab.Unmasked {
			connection.Show("the hidden columns are shown")
		} else {
			connection.Show("the columns that hold a secret are hidden again")
		}

	case ActionViewCell:
		return model.viewCell(connection, tab, shape)
	case ActionOpenRow:
		if rowCount == 0 {
			return model, nil
		}
		connection.Overlay = app.Overlay{
			Kind: app.OverlayRowDetail,
			Window: app.RowWindow{
				Columns: shape.Columns, Rows: shape.Rows,
				Index: shape.RowIndexes[clamp(tab.GridRow, rowCount)],
			},
		}
	case ActionEditCell:
		return model.editCell(connection, tab, shape)
	case ActionToggleDelete:
		return model.toggleRowDelete(connection, tab, shape)
	case ActionDuplicateRow:
		return model.duplicateRow(connection, tab, shape)
	case ActionInsertRow:
		return model.insertRow(connection, tab, shape)
	case ActionReviewChanges:
		return model.reviewChanges(connection, tab)
	case ActionUndoChange:
		if !tab.UndoChange() {
			connection.Show("nothing to take back")
		}
	case ActionRedoChange:
		if !tab.RedoChange() {
			connection.Show("nothing to put back on")
		}
	case ActionFollowForeignKey:
		return model.followForeignKey(connection, tab, shape)
	case ActionCopyMenu:
		connection.Overlay = app.Overlay{
			Kind: app.OverlayCopyMenu, Title: " copy to clipboard ", Actions: buildCopyMenuActions(),
			Draft: app.NewEditorBuffer("", 0),
		}
	case ActionCopyCSV:
		return model.runCopy(connection, tab, copyResultCSV)
	case ActionCopyJSON:
		return model.runCopy(connection, tab, copyResultJSON)
	case ActionCopyMarkdown:
		return model.runCopy(connection, tab, copyResultMarkdown)
	case ActionCopyInserts:
		return model.runCopy(connection, tab, copyResultInsert)
	case ActionDiscardChanges:
		return model.requestDiscardChanges(connection, tab)
	case ActionOpenMenu:
		connection.Overlay = app.Overlay{
			Kind: app.OverlayActionMenu, Title: model.describeGridMenuTitle(tab, shape),
			Draft:   app.NewEditorBuffer("", 0),
			Actions: model.buildGridMenu(connection, tab, shape),
		}
	}

	// A move towards the foot of the grid reads the next page before the cursor gets
	// there. A move to the first row needs none, and a move between columns moves no row.
	switch match.Action {
	case ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown,
		ActionCursorLastRow:
		return model, model.approachGridEnd(connection, tab, tab.GridRow, rowCount)
	}
	return model, nil
}

// runPlanAction returns the keys of the plan view, which is drawn in place of the grid.
func (model *Model) runPlanAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (tea.Model, tea.Cmd) {
	switch match.Action {
	case ActionToggleRawPlan:
		tab.RawPlan = !tab.RawPlan
		tab.DetailOffset = 0
	case ActionCopyPlan:
		if tab.ViewData.Kind != app.DataPlan {
			return model, nil
		}
		connection.Show("the plan is on the clipboard")
		return model, model.keepOnClipboard(tab.ViewData.Plan.Raw)
	case ActionAiCheckPlan:
		return model.askAiToCheckPlan(connection, tab)
	}
	return model, nil
}

// sortByColumn sorts by the column under the cursor, and toggles the direction.
func (model *Model) sortByColumn(
	connection *app.Connection, tab *app.Tab, shape GridShape, add bool,
) (tea.Model, tea.Cmd) {
	if tab.GridColumn < 0 || tab.GridColumn >= len(shape.Columns) {
		return model, nil
	}
	name := shape.Columns[tab.GridColumn].Name

	// The sort the user acts on is the one the header marks, which is the order the rows
	// are in: the one the grid laid on, or the one the statement writes itself. Starting
	// from the grid alone would throw the order of the statement away on the first press.
	tab.Sort = core.ApplySortColumn(model.resolveGridSort(connection, tab), name, add)
	return model.runTabRead(connection, tab)
}

// filterByCell filters by the value under the cursor, or excludes it.
func (model *Model) filterByCell(
	connection *app.Connection, tab *app.Tab, shape GridShape, exclude bool,
) (tea.Model, tea.Cmd) {
	value, column, found := model.findCellUnderCursor(tab, shape)
	if !found {
		return model, nil
	}
	tab.Filter = append(tab.Filter, core.BuildCellFilter(column.Name, value, exclude))
	return model.runTabRead(connection, tab)
}

// findCellUnderCursor returns the value and the column the cursor stands on.
func (model *Model) findCellUnderCursor(
	tab *app.Tab, shape GridShape,
) (any, db.ResultColumn, bool) {
	if len(shape.Text) == 0 || len(shape.Columns) == 0 {
		return nil, db.ResultColumn{}, false
	}
	row := shape.RowIndexes[clamp(tab.GridRow, len(shape.Text))]
	columnIndex := clamp(tab.GridColumn, len(shape.Columns))
	if row < 0 || row >= len(shape.Rows) || columnIndex >= len(shape.Rows[row]) {
		return nil, db.ResultColumn{}, false
	}
	return shape.Rows[row][columnIndex], shape.Columns[columnIndex], true
}

// buildColumnOrder returns the order the columns are drawn and walked in: the frozen ones
// first, then the ones the window scrolls.
func buildColumnOrder(tab *app.Tab, count int) []int {
	frozen := []int{}
	scrolling := []int{}
	for index := range count {
		if tab.Frozen[index] {
			frozen = append(frozen, index)
			continue
		}
		scrolling = append(scrolling, index)
	}
	return append(frozen, scrolling...)
}

// moveGridColumn steps the cursor through the order the columns are drawn in, so a freeze
// moves the walk with the columns.
func moveGridColumn(tab *app.Tab, count, step int) {
	order := buildColumnOrder(tab, count)
	if len(order) == 0 {
		return
	}
	place := -1
	for at, index := range order {
		if index == tab.GridColumn {
			place = at
			break
		}
	}
	if place == -1 {
		tab.GridColumn = order[0]
		return
	}
	tab.GridColumn = order[clamp(place+step, len(order))]
}

// freezeColumns keeps the column under the cursor on screen, whatever the window shows.
func (model *Model) freezeColumns(connection *app.Connection, tab *app.Tab) tea.Model {
	if tab.Frozen[tab.GridColumn] {
		delete(tab.Frozen, tab.GridColumn)
		return model
	}
	if tab.Frozen == nil {
		tab.Frozen = map[int]bool{}
	}
	tab.Frozen[tab.GridColumn] = true
	return model
}

// viewCell opens the cell under the cursor full size.
func (model *Model) viewCell(
	connection *app.Connection, tab *app.Tab, shape GridShape,
) (tea.Model, tea.Cmd) {
	value, column, found := model.findCellUnderCursor(tab, shape)
	if !found {
		return model, nil
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayCell, Cell: app.CellTarget{Column: column, Value: value},
	}
	return model, nil
}

// refusesEdit is true where the result cannot be written to, and reports why in the bar. Every
// key that stages a change asks this first.
func refusesEdit(connection *app.Connection, tab *app.Tab) bool {
	if tab.Target.Editable {
		return false
	}
	connection.Show(tab.Target.Reason)
	return true
}

// editCell stages an edit of the cell under the cursor.
func (model *Model) editCell(
	connection *app.Connection, tab *app.Tab, shape GridShape,
) (tea.Model, tea.Cmd) {
	if refusesEdit(connection, tab) {
		return model, nil
	}
	value, column, found := model.findCellUnderCursor(tab, shape)
	if !found {
		return model, nil
	}
	if problem := tab.Target.FindColumnProblem(column.Name); problem != "" {
		connection.Show(problem)
		return model, nil
	}

	rowIndex := shape.RowIndexes[clamp(tab.GridRow, len(shape.Text))]
	columnIndex := clamp(tab.GridColumn, len(shape.Columns))
	initial := present.FormatForViewer(value, column.DataType)
	// A value staged against the cell is what is edited further, not what the server sent.
	if staged, held := tab.Pending.Edits[core.BuildEditKey(rowIndex, columnIndex)]; held {
		initial = core.DescribeCellValue(staged.Value)
	}
	if initial == core.NullText {
		initial = ""
	}

	connection.Overlay = buildCellEditor(app.Overlay{
		Kind: app.OverlayCellEdit,
		Cell: app.CellTarget{
			Column: column, RowIndex: rowIndex, ColumnIndex: columnIndex,
			Choices: tab.Target.FindColumnChoices(column.Name),
		},
	}, initial)
	return model, nil
}

// minCellEditorRows is the rows the card gives a cell with one line or with no value.
const minCellEditorRows = 6

// buildCellEditor fills in what the card of a cell editor needs: the list to pick from with
// the value already held marked, or a field with the value in it. The rows of the content are
// read here, so the card does not grow while the user types.
func buildCellEditor(overlay app.Overlay, initial string) app.Overlay {
	if len(overlay.Cell.Choices) > 0 {
		overlay.ContentRows = len(overlay.Cell.Choices)
		for at, choice := range overlay.Cell.Choices {
			if choice == initial {
				overlay.List.Cursor = at
			}
		}
		return overlay
	}
	overlay.Draft = app.NewEditorBuffer(initial, 0)
	overlay.ContentRows = max(strings.Count(initial, "\n")+1, minCellEditorRows)
	return overlay
}

// toggleRowDelete marks the row under the cursor for delete, or takes the mark off.
func (model *Model) toggleRowDelete(
	connection *app.Connection, tab *app.Tab, shape GridShape,
) (tea.Model, tea.Cmd) {
	if refusesEdit(connection, tab) {
		return model, nil
	}
	if len(shape.Text) == 0 {
		return model, nil
	}
	rowIndex := shape.RowIndexes[clamp(tab.GridRow, len(shape.Text))]
	if !tab.StageChange(func(pending *core.PendingChanges) {
		if pending.DeletedRows[rowIndex] {
			delete(pending.DeletedRows, rowIndex)
			return
		}
		pending.DeletedRows[rowIndex] = true
	}) {
		connection.ShowError(describeStageRefusal(tab))
	}
	return model, nil
}

// duplicateRow stages a new row with the values of the one under the cursor, without its
// primary key.
func (model *Model) duplicateRow(
	connection *app.Connection, tab *app.Tab, shape GridShape,
) (tea.Model, tea.Cmd) {
	if refusesEdit(connection, tab) {
		return model, nil
	}
	if len(shape.Text) == 0 {
		return model, nil
	}
	rowIndex := shape.RowIndexes[clamp(tab.GridRow, len(shape.Text))]

	keys := map[string]bool{}
	for _, name := range tab.Target.KeyColumns {
		keys[strings.ToLower(name)] = true
	}
	generated := map[string]bool{}
	for _, column := range tab.Target.Columns {
		if column.IsGenerated {
			generated[strings.ToLower(column.Name)] = true
		}
	}

	values := map[string]any{}
	for at, column := range shape.Columns {
		lowered := strings.ToLower(column.Name)
		if keys[lowered] || generated[lowered] {
			continue
		}
		if at < len(shape.Rows[rowIndex]) {
			values[column.Name] = shape.Rows[rowIndex][at]
		}
	}

	if !tab.StageChange(func(pending *core.PendingChanges) {
		pending.Inserts = append(pending.Inserts, values)
	}) {
		connection.ShowError(describeStageRefusal(tab))
		return model, nil
	}
	connection.Show("a copy of the row is staged")
	return model, nil
}

// insertRow opens the form a new row is written into, as JSON.
func (model *Model) insertRow(
	connection *app.Connection, tab *app.Tab, shape GridShape,
) (tea.Model, tea.Cmd) {
	if refusesEdit(connection, tab) {
		return model, nil
	}

	form := map[string]any{}
	names := []string{}
	for _, column := range tab.Target.Columns {
		// The server fills a generated column itself, and it fills a column with a
		// default where the row names no value.
		if column.IsGenerated || column.HasDefault {
			continue
		}
		form[column.Name] = ""
		names = append(names, column.Name)
	}
	written := statement.BuildParameterForm(names, form)

	connection.Overlay = buildCellEditor(app.Overlay{
		Kind: app.OverlayCellEdit,
		Cell: app.CellTarget{
			RowIndex: app.WholeRow, ColumnIndex: app.WholeRow,
			Column: db.ResultColumn{Name: "new row", DataType: "json"},
		},
	}, written)
	return model, nil
}

// reviewChanges shows the staged work as the statements that will run.
func (model *Model) reviewChanges(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	if core.CountChanges(tab.Pending) == 0 {
		connection.Show("nothing is staged")
		return model, nil
	}
	changes, err := model.buildChanges(connection, tab)
	if err != nil {
		connection.ShowError(db.DescribeError(err))
		return model, nil
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayChanges, Title: " review the changes ", Changes: changes,
	}
	return model, nil
}

// followForeignKey opens the row a foreign key points at, in the tab of its table.
func (model *Model) followForeignKey(
	connection *app.Connection, tab *app.Tab, shape GridShape,
) (tea.Model, tea.Cmd) {
	value, column, found := model.findCellUnderCursor(tab, shape)
	if !found {
		return model, nil
	}
	target, points := build.FindForeignKeyTarget(tab.Target.ForeignKeys, column.Name)
	if !points {
		connection.Show(column.Name + " points at no other table")
		return model, nil
	}
	table, known := connection.Catalog.FindTable(target.Schema, target.Table)
	if !known {
		connection.Show("the catalog does not hold " + target.Table)
		return model, nil
	}

	preview := connection.Session.Composer().ComposeRelationRead(
		table, core.ReadRewrite{}).Display
	opened := connection.OpenTable(table, preview)
	opened.Filter = []core.FilterStep{core.BuildCellFilter(target.Column, value, false)}
	return model.runTabRead(connection, opened)
}

// askValueFilter asks which values of the column stay on screen.
func (model *Model) askValueFilter(
	connection *app.Connection, tab *app.Tab, shape GridShape,
) (tea.Model, tea.Cmd) {
	if tab.GridColumn < 0 || tab.GridColumn >= len(shape.Columns) {
		return model, nil
	}
	// The counts are of the rows already read, before this filter hides any of them.
	names := make([]string, 0, len(shape.Columns))
	dataTypes := make([]string, 0, len(shape.Columns))
	for _, column := range shape.Columns {
		names = append(names, column.Name)
		dataTypes = append(dataTypes, column.DataType)
	}
	everyRow := present.FormatRows(shape.Rows, dataTypes, shape.Masked)
	values := present.CountColumnValues(everyRow, tab.GridColumn)

	kept := map[string]bool{}
	held, filtered := tab.Screen.Values[tab.GridColumn]
	for _, value := range values {
		if !filtered || held[value.Value] {
			kept[value.Value] = true
		}
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayValueFilter, Title: " " + names[tab.GridColumn] + " ",
		Values: values, Kept: kept,
		Cell: app.CellTarget{ColumnIndex: tab.GridColumn},
	}
	return model, nil
}

// The ids of the copy menu, so a typo is a build error rather than a row that copies nothing.
const (
	copyCell           = "cell"
	copyRowJSON        = "row-json"
	copyResultCSV      = "result-csv"
	copyResultJSON     = "result-json"
	copyResultMarkdown = "result-markdown"
	copyResultInsert   = "result-insert"
	copyColumnIn       = "column-in"
)

// buildCopyMenuActions returns what the copy menu offers.
func buildCopyMenuActions() []app.MenuAction {
	return []app.MenuAction{
		{ID: copyCell, Label: "Cell", Detail: "the selected value"},
		{ID: copyRowJSON, Label: "Row as JSON", Detail: "the selected row"},
		{ID: copyResultCSV, Label: "Result as CSV", Detail: "every loaded row"},
		{ID: copyResultJSON, Label: "Result as JSON", Detail: "every loaded row"},
		{ID: copyResultMarkdown, Label: "Result as Markdown", Detail: "every loaded row"},
		{ID: copyResultInsert, Label: "Result as INSERT", Detail: "every loaded row"},
		{
			ID: copyColumnIn, Label: "Column as IN clause",
			Detail: "the selected column, ready for a where",
		},
	}
}

// buildGridMenu returns what the menu offers on the row and the cell under the cursor.
func (model *Model) buildGridMenu(
	connection *app.Connection, tab *app.Tab, shape GridShape,
) []app.MenuAction {
	capabilities := connection.Session.Capabilities()
	hasRow := len(shape.Text) > 0
	editable := tab.Target.Editable

	// Each entry runs the grid action of the same id, so the menu offers what a key
	// also reaches. An action that does not fit the row or the cell is left out. Copy
	// is always offered, so the menu is never empty on a result with no rows.
	offered := []struct {
		id      ActionID
		label   string
		detail  string
		needs   bool
		harmful bool
	}{
		{ActionViewCell, "View value", "the value under the cursor", hasRow, false},
		{ActionEditCell, "Edit value", "the value under the cursor", hasRow && editable, false},
		{ActionFilterByCell, "Filter by value", "keep rows that match", hasRow, false},
		{ActionExcludeCell, "Exclude value", "drop rows that match", hasRow, false},
		{ActionFollowForeignKey, "Follow foreign key", "open the row it points to", hasRow, false},
		{ActionFilterByValues, "Filter by values", "choose which values stay", hasRow, false},
		{ActionSortColumn, "Sort by column", "order the read by it", capabilities.SortsRead, false},
		{
			ActionAddSortColumn, "Add column to sort", "order by it as well",
			capabilities.SortsRead, false,
		},
		{ActionOpenRow, "Open row", "every field of the row", hasRow, false},
		{ActionCopyMenu, "Copy", "the cell, row, or result", true, false},
		{ActionInsertRow, "Insert row", "add a new row", editable, false},
		{
			ActionDuplicateRow, "Duplicate row", "copy the row without its key",
			hasRow && editable, false,
		},
		{ActionToggleDelete, "Delete row", "remove the row", hasRow && editable, true},
	}

	actions := make([]app.MenuAction, 0, len(offered))
	for _, entry := range offered {
		if !entry.needs ||
			!AnswersFor(capabilities, FindActionCapability(cfg.ScopeGrid, entry.id)) {
			continue
		}
		actions = append(actions, app.MenuAction{
			ID: string(entry.id), Label: entry.label, Detail: entry.detail,
			Chord:       model.registry.FormatActionChords(cfg.ScopeGrid, entry.id),
			Destructive: entry.harmful,
		})
	}
	return actions
}

// describeGridMenuTitle names the row and the column the menu opens on, so the entries
// are read against the cell they act on.
func (model *Model) describeGridMenuTitle(tab *app.Tab, shape GridShape) string {
	if len(shape.Text) == 0 {
		return " the result "
	}
	row := shape.RowIndexes[clamp(tab.GridRow, len(shape.Text))]
	written := " row " + present.FormatCount(int64(row+1))
	if tab.GridColumn < len(shape.Columns) {
		written += " · " + shape.Columns[tab.GridColumn].Name
	}
	return written + " "
}

// runCopy returns a row of the copy menu, and puts the text on the clipboard.
func (model *Model) runCopy(
	connection *app.Connection, tab *app.Tab, id string,
) (tea.Model, tea.Cmd) {
	shape := model.buildGridShape(connection, tab)
	if len(shape.Columns) == 0 {
		connection.Show("there is nothing to copy")
		return model, nil
	}
	dialect := connection.Session.Dialect()

	var written string
	switch id {
	case copyCell:
		value, column, found := model.findCellUnderCursor(tab, shape)
		if !found {
			return model, nil
		}
		written = present.FormatForViewer(value, column.DataType)
	case copyRowJSON:
		if len(shape.Text) == 0 {
			return model, nil
		}
		rowIndex := shape.RowIndexes[clamp(tab.GridRow, len(shape.Text))]
		names := result.BuildRecordKeys(shape.Columns)
		values := map[string]any{}
		for at, name := range names {
			if at < len(shape.Rows[rowIndex]) {
				values[name] = core.FormatCell(shape.Rows[rowIndex][at], shape.Columns[at].DataType)
			}
		}
		written = statement.BuildParameterForm(names, values)
	case copyResultCSV:
		writer := result.CreateExportWriter(result.ExportCSV, result.DefaultCSVOptions())
		written = writer.Begin(shape.Columns) +
			writer.WriteRows(shape.Rows, shape.Columns) + writer.End()
	case copyResultJSON:
		writer := result.CreateExportWriter(result.ExportJSON, result.DefaultCSVOptions())
		written = writer.Begin(shape.Columns) +
			writer.WriteRows(shape.Rows, shape.Columns) + writer.End()
	case copyResultMarkdown:
		written = result.BuildMarkdown(shape.Columns, shape.Rows)
	case copyResultInsert:
		if !tab.Target.Editable && tab.Kind != app.TabTable {
			connection.Show("this result comes from more than one relation")
			return model, nil
		}
		written = result.BuildInsertScript(
			shape.Columns, shape.Rows, tab.Target.Table.Qualified(), dialect)
	case copyColumnIn:
		written = result.BuildInClause(shape.Columns, shape.Rows, tab.GridColumn, dialect)
	}

	if written == "" {
		connection.Show("there is nothing to copy")
		return model, nil
	}
	connection.Show("copied " + present.FormatCountOf(
		int64(len(strings.Split(written, "\n"))), "line", "lines"))
	return model, model.keepOnClipboard(written)
}

// openExport asks for the file and the options rather than writing at once.
func (model *Model) openExport(
	connection *app.Connection, tab *app.Tab, format result.ExportFormat,
) (tea.Model, tea.Cmd) {
	active := tab.Results.Active()
	if active == nil || active.State.Kind != app.QuerySucceeded {
		connection.Show("run a statement first")
		return model, nil
	}

	label := tab.Label()
	if tab.Kind == app.TabTable {
		label = tab.Table.Name
	}
	// The file is offered in the directory the client was started in, named after the
	// relation and the moment, so two exports of one relation never collide.
	stamp := time.Now().Format("060102150405")
	path := result.BuildExportFilename(label, format, stamp)
	if directory, err := os.Getwd(); err == nil {
		path = filepath.Join(directory, path)
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayExport, Title: " export ",
		Export: app.ExportRequest{
			Path: path, Format: format, CSV: result.DefaultCSVOptions(),
			RowCount: len(active.State.Result.Rows),
		},
		Draft: app.NewEditorBuffer(path, len(path)),
	}
	return model, nil
}

// writeExport writes the whole read to a file, one batch at a time, so a large relation is
// never held whole.
func (model *Model) writeExport(
	connection *app.Connection, tab *app.Tab, overlay app.Overlay,
) (tea.Model, tea.Cmd) {
	active := tab.Results.Active()
	if active == nil {
		return model, nil
	}

	if problem := FindExportProblem(overlay); problem != "" {
		connection.ShowError(problem)
		return model, nil
	}
	if overlay.Export.WholeRead {
		if problem := model.findRereadProblem(connection, tab); problem != "" {
			connection.ShowError(problem)
			return model, nil
		}
	}
	path := core.ExpandHomePath(strings.TrimSpace(overlay.Export.Path))
	// An existing file is never written over without a yes.
	if _, err := os.Stat(path); err == nil {
		connection.Overlay = app.Overlay{
			Kind: app.OverlayConfirm, Title: " write over the file ",
			Body: path + " is already there.",
			Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
				if !confirmed {
					return nil
				}
				connection.Overlay = app.Overlay{}
				return carryAnswer(model.startExport(connection, tab, path, overlay))
			}},
		}
		return model, nil
	}

	connection.Overlay = app.Overlay{}
	return model, model.startExport(connection, tab, path, overlay)
}

// findRereadProblem returns why the statement of this result cannot be read again, and
// nothing where it can. Reading every row asks the server for the statement a second time,
// and a statement that writes would write a second time with it.
func (model *Model) findRereadProblem(connection *app.Connection, tab *app.Tab) string {
	active := tab.Results.Active()
	if active == nil || active.Source == "" {
		return ""
	}
	if connection.Session.Language().ResolveWriteRisk(active.Source) == statement.RiskNone {
		return ""
	}
	return "this statement writes, so only the rows loaded so far can be exported"
}

// startExport writes the export to a file, one batch at a time, so a large relation is
// never held whole.
func (model *Model) startExport(
	connection *app.Connection, tab *app.Tab, path string, overlay app.Overlay,
) tea.Cmd {
	active := tab.Results.Active()
	session := connection.Session
	id := model.ActiveID()
	read := active.Read
	loaded := active.State.Result
	wholeRead := overlay.Export.WholeRead
	format, options := overlay.Export.Format, overlay.Export.CSV

	ctx, stop := context.WithCancel(context.Background())
	connection.BeginExport(stop)

	return func() tea.Msg {
		defer stop()
		fail := func(reason string) tea.Msg {
			return exportWrittenMsg{ConnectionID: id, Path: path, Problem: reason}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fail(err.Error())
		}
		// Written beside the file and moved over it at the end, so a read or a write that
		// fails part way leaves the file that was there whole.
		file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.part")
		if err != nil {
			return fail(err.Error())
		}
		temporaryPath := file.Name()
		// The file holds the rows of the database, so it stays readable by its owner
		// alone, the way the history file and the logs do. A reader who wants to hand it
		// on can widen it themselves.
		if modeErr := file.Chmod(0o600); modeErr != nil {
			_ = file.Close()
			_ = os.Remove(temporaryPath)
			return fail(modeErr.Error())
		}
		dropTemporary := func() {
			_ = file.Close()
			_ = os.Remove(temporaryPath)
		}

		writer := result.CreateExportWriter(format, options)
		wroteHeader := false
		writeBatch := func(rows [][]any, columns []db.ResultColumn) error {
			if !wroteHeader {
				if _, headerErr := file.WriteString(writer.Begin(columns)); headerErr != nil {
					return headerErr
				}
				wroteHeader = true
			}
			_, rowErr := file.WriteString(writer.WriteRows(rows, columns))
			return rowErr
		}

		written := int64(len(loaded.Rows))
		if wholeRead {
			// Every row means the whole read, which only the server holds, so the
			// statement is read again a batch at a time.
			streamed, streamErr := session.StreamQuery(ctx,
				read.Text, read.Params, result.ExportProgressRows, writeBatch)
			if streamErr != nil {
				dropTemporary()
				return fail(db.DescribeError(streamErr))
			}
			written = streamed
		} else if len(loaded.Rows) > 0 {
			// The rows loaded so far are already in hand, so the server is not asked
			// again. A statement that writes is read once and never twice.
			if batchErr := writeBatch(loaded.Rows, loaded.Columns); batchErr != nil {
				dropTemporary()
				return fail(batchErr.Error())
			}
		}
		// A read of no rows still writes a file that reads back: a JSON array that is
		// empty, and a CSV of its header alone.
		if !wroteHeader {
			if _, headerErr := file.WriteString(writer.Begin(loaded.Columns)); headerErr != nil {
				dropTemporary()
				return fail(headerErr.Error())
			}
		}
		if _, endErr := file.WriteString(writer.End()); endErr != nil {
			dropTemporary()
			return fail(endErr.Error())
		}
		// The close is reported. A write can fail here and nowhere earlier, when the disk
		// fills or a mount returns late, and the user would otherwise read that the export
		// is whole while the file is short.
		if closeErr := file.Close(); closeErr != nil {
			_ = os.Remove(temporaryPath)
			return fail(closeErr.Error())
		}
		if renameErr := os.Rename(temporaryPath, path); renameErr != nil {
			_ = os.Remove(temporaryPath)
			return fail(renameErr.Error())
		}
		return exportWrittenMsg{ConnectionID: id, Path: path, Rows: written}
	}
}

// goToColumn moves the cursor of the grid to the column of that name.
func (model *Model) goToColumn(
	connection *app.Connection, tab *app.Tab, name string,
) (tea.Model, tea.Cmd) {
	shape := model.buildGridShape(connection, tab)
	wanted := strings.TrimSpace(name)
	if wanted == "" {
		return model, nil
	}
	for at, column := range shape.Columns {
		if present.MatchesText(column.Name, wanted) {
			tab.GridColumn = at
			return model, nil
		}
	}
	connection.Show("no column matches " + wanted)
	return model, nil
}

// describeGridFooter writes the size of the result on the left, and the row the cursor is on
// at the right.
func (model *Model) describeGridFooter(tab *app.Tab, shape GridShape) (string, string) {
	active := tab.Results.Active()
	if active == nil || active.State.Kind != app.QuerySucceeded {
		return "", ""
	}
	result := active.State.Result

	size := present.FormatResultSize(
		len(result.Rows), result.Truncated, active.TotalRows, active.HasTotalRows)
	// The count runs on the server, so the wheel turns where its answer will stand.
	if active.Counting {
		size = present.FormatCount(int64(len(result.Rows))) + " of " +
			spinnerFrame(model.spinnerAt) + " rows"
	}
	// The page read runs on the server, so the wheel turns where the rows will arrive.
	if active.FetchingMore {
		size += " · " + spinnerFrame(model.spinnerAt) + " reading more…"
	}
	if !tab.Screen.IsEmpty() {
		size += " · " + strconv.Itoa(len(shape.Text)) + " shown"
	}

	where := ""
	if len(shape.Text) > 0 {
		// The footer names the result row, so it agrees with the gutter.
		row := shape.RowIndexes[clamp(tab.GridRow, len(shape.Text))]
		where = "row " + present.FormatCount(int64(row+1))
		if tab.GridColumn < len(shape.Columns) {
			// A result of more than one column says which of them the cursor is on, because
			// the ones outside the window are counted at the edges of the header and not
			// named there.
			if len(shape.Columns) > 1 {
				where += " · col " + strconv.Itoa(tab.GridColumn+1) +
					"/" + strconv.Itoa(len(shape.Columns))
			}
			column := shape.Columns[tab.GridColumn]
			where += " · " + column.Name + " " + present.AbbreviateDataType(column.DataType)
		}
	}
	return size, where
}
