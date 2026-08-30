package ui

import (
	"image/color"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/app"
)

// hoverKind says what the pointer stands on, and how the frame marks it.
type hoverKind string

const (
	// hoverNothing is the pointer on a cell that returns no press.
	hoverNothing hoverKind = ""
	// hoverRow marks a whole row a press would take: a tab, a row of a list, a row of the
	// grid, a chip of a strip.
	hoverRow hoverKind = "row"
	// hoverKey marks a word a press would run, which keeps its own colours and takes a
	// rule under it.
	hoverKey hoverKind = "key"
)

// hoverTarget is what the pointer stands on: the cells it covers, and how they are marked.
// The whole of it is compared, so a move that stays on the same thing costs no frame.
type hoverTarget struct {
	kind     hoverKind
	row      int
	from, to int
}

// insetBy pulls the mark in from each end, so a row that spans a whole pane does not mark the
// border of the pane itself.
func (target hoverTarget) insetBy(cells int) hoverTarget {
	if !target.isSomething() {
		return target
	}
	target.from += cells
	target.to -= cells
	return target
}

// isSomething is true where the pointer stands on a thing a press would take.
func (target hoverTarget) isSomething() bool {
	return target.kind != hoverNothing && target.to >= target.from
}

// underlineSequence draws a rule under a cell, which is what marks a word a press would run.
const underlineSequence = "\x1b[4m"

// paintHover marks what the pointer stands on. A row takes the ground of the header, and a
// word a press would run takes a rule under it, which is the smallest mark that says the
// word is a key without printing a border around every key in the client. Only the ground of
// a cell is laid, so a row keeps the colours of its own parts.
func (model *Model) paintHover(frame string) string {
	target := model.frame.hover
	if !target.isSomething() {
		return frame
	}
	rows := strings.Split(frame, "\n")
	if target.row < 0 || target.row >= len(rows) {
		return frame
	}
	cells := mapCells(rows[target.row])
	if target.kind == hoverKey {
		// A key keeps its own colours and takes a rule under it, which is the smallest
		// mark that says the word is a key.
		for at := max(target.from, 0); at <= target.to && at < len(cells); at++ {
			cells[at].sgr += underlineSequence
		}
		rows[target.row] = writeCells(cells)
		return strings.Join(rows, "\n")
	}

	ground, ink := model.styles.resolveHoverColors()
	mark := buildSgr(ink, ground)
	for at := max(target.from, 0); at <= target.to && at < len(cells); at++ {
		cells[at].sgr = mark
	}
	rows[target.row] = writeCells(cells)
	return strings.Join(rows, "\n")
}

// resolveHover returns what the pointer stands on. The parts of the frame are read in the
// order a press reads them, so a card wins the cells of the pane under it.
func (model *Model) resolveHover(x, y int) hoverTarget {
	if x < 0 || y < 0 || x >= model.width || y >= model.height {
		return hoverTarget{}
	}
	// A drag belongs to what it began on, so nothing is marked while one runs. The mark
	// would follow the pointer over the cells the drag covers and read as a second thing
	// happening at once.
	if model.isDragging() {
		return hoverTarget{}
	}
	// Every key a renderer drew as a word is drawn over the rows, so it is read first.
	for _, held := range model.layout.buttons {
		if y == held.row && x >= held.from && x <= held.to {
			return hoverTarget{kind: hoverKey, row: held.row, from: held.from, to: held.to}
		}
	}
	switch model.screen {
	case ScreenPickingProfile:
		return resolveRowHover(
			model.layout.pickerRows, len(model.profiles), model.picker.cursor, x, y)
	case ScreenEditingConnection:
		return resolveRowHover(
			model.layout.formRows, model.countFormFields(), noFilledRow, x, y)
	case ScreenWorking:
		return model.resolveWorkspaceHover(x, y)
	}
	return hoverTarget{}
}

// countFormFields returns how many rows the connection form shows.
func (model *Model) countFormFields() int {
	if model.form == nil {
		return 0
	}
	return len(model.form.Shown())
}

// resolveRowHover returns the row of a block the pointer stands on, and nothing where the row
// under it holds no item.
//
// The row named by filled is the one already drawn on a ground of its own, such as the row
// under the cursor. It takes no mark: its ink was chosen against that ground, and laying
// another one under it would leave the ink unreadable. A block whose rows are never filled
// names none of them, with a row of minus one.
func resolveRowHover(block rowsHit, items, filled, x, y int) hoverTarget {
	row, found := block.holds(x, y)
	if !found || row >= items || row == filled {
		return hoverTarget{}
	}
	return hoverTarget{kind: hoverRow, row: y, from: block.from, to: block.to}
}

// noFilledRow says that no row of a block is drawn on a ground of its own, so every row it
// holds takes the mark of the pointer.
const noFilledRow = -1

// resolveWorkspaceHover returns what the pointer stands on over the workspace.
func (model *Model) resolveWorkspaceHover(x, y int) hoverTarget {
	connection := model.Active()
	if connection == nil {
		return hoverTarget{}
	}
	layout := model.layout
	overlay := connection.Overlay
	if overlay.IsOpen() {
		// The two answers of a question are both drawn on a ground of their own.
		if _, onChip := findChip(layout.overlayChips, x, y); onChip {
			return hoverTarget{}
		}
		if target := resolveRowHover(layout.formRows, layout.formRows.count,
			noFilledRow, x, y); target.isSomething() {
			return target
		}
		return resolveRowHover(layout.overlayRows,
			model.overlayRowCount(connection, overlay), overlay.List.Cursor, x, y)
	}

	tab := connection.Active()
	if target := resolveRowHover(layout.completionRows, len(tab.Completion.Candidates),
		tab.Completion.Selected, x, y); target.isSomething() {
		return target
	}
	// A block of the workspace draws one row per item, so the rows it recorded are the
	// items it holds and the count of them returns for both.
	if y == layout.tabRow {
		return resolveTabHover(layout.tabs, connection.ActiveIndex, x, y)
	}
	// The rows of the tree pane reach both of its borders, so the mark is pulled in from
	// each end and the border keeps its shape.
	if target := resolveRowHover(layout.connections, model.connections.count(),
		noFilledRow, x, y); target.isSomething() {
		return target.insetBy(1)
	}
	if target := resolveRowHover(layout.treeRows, layout.treeRows.count,
		filledRowOf(connection.Tree.Cursor-layout.treeRows.offset,
			tab.Focus == app.PaneSidebar), x, y); target.isSomething() {
		return target.insetBy(1)
	}
	if target := resolveChipHover(layout.statementChips, tab.Results.ActiveIndex(),
		x, y); target.isSomething() {
		return target
	}
	if target := resolveChipHover(layout.viewChips,
		findViewIndex(tab.Views(connection.Session), tab.ActiveView(connection.Session)),
		x, y); target.isSomething() {
		return target
	}
	if y == layout.gridHeaderRow && tab.View == app.ViewData {
		return resolveColumnHover(layout.gridColumns,
			filledRowOf(tab.GridColumn, tab.Focus == app.PaneResult), x, y)
	}
	return resolveRowHover(layout.gridRows, layout.gridRows.count,
		filledRowOf(tab.GridRow, tab.Focus == app.PaneResult), x, y)
}

// buildSpanHover returns the mark for a run of cells, and nothing where the item is the one
// already drawn on a ground of its own.
func buildSpanHover(index, filled, row, from, to int) hoverTarget {
	if index == filled {
		return hoverTarget{}
	}
	return hoverTarget{kind: hoverRow, row: row, from: from, to: to}
}

// resolveChipHover returns the chip of a strip the pointer stands on.
func resolveChipHover(chips []chipHit, filled, x, y int) hoverTarget {
	for _, held := range chips {
		if y == held.row && x >= held.from && x <= held.to {
			return buildSpanHover(held.index, filled, y, held.from, held.to)
		}
	}
	return hoverTarget{}
}

// resolveTabHover returns the tab the pointer stands on, on the row the tabs are drawn.
func resolveTabHover(tabs []tabHit, filled, x, y int) hoverTarget {
	for _, held := range tabs {
		if x >= held.from && x <= held.to {
			return buildSpanHover(held.index, filled, y, held.from, held.to)
		}
	}
	return hoverTarget{}
}

// resolveColumnHover returns the column the pointer stands on, on a row it is known to be on.
func resolveColumnHover(columns []columnHit, filled, x, y int) hoverTarget {
	for _, held := range columns {
		if x >= held.from && x <= held.to {
			return buildSpanHover(held.index, filled, y, held.from, held.to)
		}
	}
	return hoverTarget{}
}

// filledRowOf returns the row drawn on a ground of its own, and none where the pane it belongs
// to does not hold the keyboard: a cursor of a pane that is not focused is drawn quietly, so
// the mark of the pointer reads against it.
func filledRowOf(cursor int, focused bool) int {
	if !focused {
		return noFilledRow
	}
	return cursor
}

// findViewIndex returns which of the views offered is this one.
func findViewIndex(offered []app.ResultView, drawn app.ResultView) int {
	for at, view := range offered {
		if view == drawn {
			return at
		}
	}
	return noFilledRow
}

// isDragging is true while a press is being dragged, whichever thing it took hold of.
func (model *Model) isDragging() bool {
	return model.drag.running() || model.selection.dragging
}

// keyFlashWait is how long a key stays lit after a press. It is one frame of the wheel, which
// is long enough to be seen and short enough that it never looks like a state of its own.
const keyFlashWait = 120 * time.Millisecond

// paintPressedKey lights the key a press landed on. A key that ran something and gave nothing
// back reads as a key that was never pressed, so the press itself is answered.
func (model *Model) paintPressedKey(frame string) string {
	if !model.frame.isFlashing() {
		return frame
	}
	held := model.frame.pressed
	// A key that ran something may have taken its own row off the frame, and the cells it
	// covered belong to whatever was drawn there instead. The light is dropped then: the
	// frame that answered the press is the answer the reader wanted.
	if !model.holdsKeyStill(held) {
		return frame
	}
	rows := strings.Split(frame, "\n")
	if held.row < 0 || held.row >= len(rows) || held.to < held.from {
		return frame
	}
	theme := model.styles.Theme
	sgr := buildSgr(theme.OnAccent, theme.Accent)

	cells := mapCells(rows[held.row])
	for at := held.from; at <= held.to && at < len(cells); at++ {
		if at >= 0 {
			cells[at].sgr = sgr
		}
	}
	rows[held.row] = writeCells(cells)
	return strings.Join(rows, "\n")
}

// resolveHoverColors returns the ground and the ink a marked row takes: the same pair the row
// under the cursor is drawn in. The pointer and the keyboard mark a row the same way, so a row
// reads the same whichever of the two reached it, and every part of the row keeps the weight
// and the colour it has under the cursor.
func (styles *Styles) resolveHoverColors() (color.Color, color.Color) {
	return styles.Theme.Accent, styles.Theme.OnAccent
}

// holdsKeyStill is true where the frame still draws this key in the cells it was pressed in.
func (model *Model) holdsKeyStill(held buttonHit) bool {
	for _, drawn := range model.layout.buttons {
		if drawn.row == held.row && drawn.from == held.from && drawn.to == held.to &&
			drawn.action == held.action {
			return true
		}
	}
	return false
}
