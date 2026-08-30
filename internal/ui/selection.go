package ui

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/present"
)

// What the pointer was dragged over. A terminal reports the drag; this keeps where it began
// and where it stands, and the frame is painted over at the end.

// screenSelection is the two cells a drag runs between, in reading order.
type screenSelection struct {
	// fromX and fromY are where the drag began, and toX and toY where it stands now.
	fromX, fromY int
	toX, toY     int
	// True while a button is held, so a release keeps what was selected.
	dragging bool
	// True once the drag covered more than the cell it began on.
	held bool
	// The block the drag began in, which it never reaches outside of.
	block blockRect
	// True where the drag began in a block of its own.
	bounded bool
}

// order returns the two ends in reading order: the earlier one first.
func (selection screenSelection) order() (int, int, int, int) {
	if selection.fromY < selection.toY ||
		(selection.fromY == selection.toY && selection.fromX <= selection.toX) {
		return selection.fromX, selection.fromY, selection.toX, selection.toY
	}
	return selection.toX, selection.toY, selection.fromX, selection.fromY
}

// coversRow returns whether the selection reaches this row at all. The paint asks this first,
// because reading a row into cells costs one cell per column.
func (selection screenSelection) coversRow(row int) bool {
	_, fromY, _, toY := selection.order()
	if row < fromY || row > toY {
		return false
	}
	if selection.bounded && (row < selection.block.fromY || row > selection.block.toY) {
		return false
	}
	return true
}

// spanOnRow returns the cells of one row the selection covers, and whether it covers any.
// A selection over several rows takes the whole of every row between its ends, held inside
// the block the drag began in.
func (selection screenSelection) spanOnRow(row, width int) (int, int, bool) {
	fromX, fromY, toX, toY := selection.order()
	if row < fromY || row > toY {
		return 0, 0, false
	}
	left, right := 0, width-1
	if selection.bounded {
		left, right = selection.block.fromX, selection.block.toX
		if row < selection.block.fromY || row > selection.block.toY {
			return 0, 0, false
		}
		if right > width-1 {
			right = width - 1
		}
	}

	from, to := left, right
	if row == fromY && fromX > from {
		from = fromX
	}
	if row == toY && toX < to {
		to = toX
	}
	if from > to {
		return 0, 0, false
	}
	return from, to, true
}

// paintSelection lays the selection over the frame. The cells keep their text and take the
// ground of the selection, so what is under the pointer is read as one block.
func (model *Model) paintSelection(frame string) string {
	if !model.selection.held {
		return frame
	}
	theme := model.styles.Theme
	sgr := buildSgr(theme.Text, theme.Border)

	rows := strings.Split(frame, "\n")
	for row, line := range rows {
		// Mapping a row costs a cell per column, so a row the selection never reaches
		// is left alone.
		if !model.selection.coversRow(row) {
			continue
		}
		cells := mapCells(line)
		from, to, covers := model.selection.spanOnRow(row, len(cells))
		if !covers {
			continue
		}
		for at := from; at <= to && at < len(cells); at++ {
			cells[at].sgr = sgr
		}
		rows[row] = writeCells(cells)
	}
	return strings.Join(rows, "\n")
}

// readSelectedText returns the text the drag covers, one line per row, or nothing where the
// drag covered only blanks. The blanks at the end of a row are padding, not content.
func (model *Model) readSelectedText(frame string) string {
	if !model.selection.held {
		return ""
	}
	written := []string{}
	for row, line := range strings.Split(frame, "\n") {
		if !model.selection.coversRow(row) {
			continue
		}
		// The paint reads display cells, so the copy reads them too. A wide glyph takes
		// two columns and one rune, and counting runes would cover a different span.
		cells := mapCells(line)
		from, to, covers := model.selection.spanOnRow(row, len(cells))
		if !covers {
			continue
		}
		if to >= len(cells) {
			to = len(cells) - 1
		}
		taken := strings.Builder{}
		for at := from; at <= to; at++ {
			taken.WriteString(cells[at].text)
		}
		written = append(written, strings.TrimRight(taken.String(), " "))
	}
	text := strings.TrimRight(strings.Join(written, "\n"), " \t\n")
	return text
}

// describeCopied writes what a copy of the selection took.
func describeCopied(text string) string {
	lines := strings.Count(text, "\n") + 1
	characters := int64(len([]rune(text)))
	many := present.FormatCountOf(characters, "character", "characters")
	if lines > 1 {
		return "copied " + many + " over " + present.FormatCount(int64(lines)) + " lines"
	}
	return "copied " + many
}
