package ui

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/turanmahmudov/masume/internal/present"
)

// The frame every pane and card is drawn in. The box is written cell by cell rather than
// through the border of a style, because a title sits on the top border and a count on the
// bottom one, and both have to keep the frame in shape.
const (
	cornerTopLeft     = "╭"
	cornerTopRight    = "╮"
	cornerBottomLeft  = "╰"
	cornerBottomRight = "╯"
	borderHorizontal  = "─"
	borderVertical    = "│"
)

// BoxOptions says how one box is drawn.
type BoxOptions struct {
	// The width and the height of the whole box, border included.
	Width  int
	Height int
	// The title on the top border, and the count on the bottom one. Each is drawn only
	// where the caller wrote one.
	Title       string
	BottomTitle string
	// A note held against the right corner of a border, already styled, such as the key
	// that asks a model about the pane or where the caret stands. It is dropped where the
	// title leaves it no room.
	Note       string
	BottomNote string
	// True while the user is in this pane, which colours the frame.
	Focused bool
	// True for a box that asks about something that cannot be undone.
	Destructive bool
	// The lines of the body, already styled and already cut to the inner width.
	Lines []string
	// The ground the body stands on. The pane ground by default.
	Ground color.Color
}

// innerWidth returns the width the body of a box is drawn in.
func (options BoxOptions) innerWidth() int {
	width := options.Width - 2
	if width < 1 {
		return 1
	}
	return width
}

// innerHeight returns the rows the body of a box is drawn in.
func (options BoxOptions) innerHeight() int {
	height := options.Height - 2
	if height < 1 {
		return 1
	}
	return height
}

// RenderBox draws one box: the frame, the title on it, and the body inside.
func (styles *Styles) RenderBox(options BoxOptions) string {
	return strings.Join(styles.RenderBoxRows(options), "\n")
}

// RenderBoxRows draws one box as its rows. A caller that puts the box into a larger frame
// takes the rows, so the text of the box is written once instead of joined here and split
// again there.
func (styles *Styles) RenderBoxRows(options BoxOptions) []string {
	frame := styles.Theme.Border
	switch {
	case options.Destructive:
		frame = styles.Theme.Error
	case options.Focused:
		frame = styles.Theme.BorderFocus
	}
	ground := options.Ground
	if ground == nil {
		ground = styles.Theme.Panel
	}

	inner := options.innerWidth()
	side := paintText(frame, ground, borderVertical)
	opening := resolveOpening(nil, ground)

	lines := make([]string, 0, options.Height)
	lines = append(lines, styles.renderBorderRow(
		cornerTopLeft, cornerTopRight, options.Title, options.Note, inner, frame, ground))

	rows := options.innerHeight()
	var written strings.Builder
	for at := range rows {
		body := ""
		if at < len(options.Lines) {
			body = options.Lines[at]
		}
		written.Reset()
		// Laying the ground under the row opens it again after every reset the row
		// carries, so the room those openings take is counted here. A row grown to
		// less than it writes is copied again for every part of it.
		written.Grow(2*len(side) + len(body) + inner + 2*len(resetSequence) +
			(strings.Count(body, resetSequence)+2)*len(opening))
		written.WriteString(side)
		writePaddedOn(&written, body, inner, ground)
		written.WriteString(side)
		lines = append(lines, written.String())
	}

	lines = append(lines, styles.renderBorderRow(
		cornerBottomLeft, cornerBottomRight, options.BottomTitle, options.BottomNote,
		inner, frame, ground))
	return lines
}

// measureBorderNoteLeft returns the column a note of this width is drawn from, counted from
// the box. The note is held one cell of border in from the right corner.
func measureBorderNoteLeft(width, noteWidth int) int {
	return width - 2 - noteWidth
}

// renderBorderRow draws one border row, with a title written into it where there is one.
func (styles *Styles) renderBorderRow(
	left, right, title, note string, inner int, frame, ground color.Color,
) string {
	if title == "" && note == "" {
		return paintText(frame, ground, left+strings.Repeat(borderHorizontal, inner)+right)
	}

	// The title starts one cell in, so the corner keeps its shape.
	written := present.TruncateText(title, max(0, inner-2))
	titleWidth := present.MeasureText(written)

	// The note is held against the other corner, and is dropped whole where the title
	// leaves it no run of border between them. It carries its own colours, so a key drawn
	// on a border reads as the key it is.
	noteWidth := measureStyledWidth(note)
	tail := inner - 1 - titleWidth - noteWidth - 1
	if noteWidth == 0 || tail < 1 || noteWidth > inner-2-titleWidth {
		note, noteWidth = "", 0
		tail = inner - 1 - titleWidth
	}
	if tail < 0 {
		tail = 0
	}

	drawn := paintText(frame, ground, left+borderHorizontal) +
		paintText(styles.Theme.Accent, ground, written) +
		paintText(frame, ground, strings.Repeat(borderHorizontal, tail))
	if noteWidth > 0 {
		drawn += note + paintText(frame, ground, borderHorizontal)
	}
	return drawn + paintText(frame, ground, right)
}

// padStyledOn fills the line out to the width on that ground, and lays the ground under
// every part of the line that sets none of its own.
func padStyledOn(line string, width int, ground color.Color) string {
	var written strings.Builder
	written.Grow(len(line) + width)
	writePaddedOn(&written, line, width, ground)
	return written.String()
}

// writePaddedOn writes what padStyledOn returns into the builder, so a row that is padded and
// then framed is built once rather than built, padded, framed and copied at each step.
func writePaddedOn(dst *strings.Builder, line string, width int, ground color.Color) {
	used := measureStyledWidth(line)
	if used > width {
		dst.WriteString(truncateStyled(paintGround(line, ground), width))
		return
	}
	writeGroundOn(dst, line, ground)
	// A line that fills the row exactly needs no fill.
	if used < width {
		writeBlanksOn(dst, ground, width-used)
	}
}

// resetSequence is what a style writes when it ends, which drops the ground with it.
const resetSequence = "\x1b[m"

// paintGround lays a ground under a line: it opens with the ground, and opens it again
// after every reset, so a part of the line that sets no ground of its own is drawn on
// the ground of the card rather than the ground of the terminal.
func paintGround(line string, ground color.Color) string {
	var written strings.Builder
	written.Grow(len(line) + len(resetSequence))
	writeGroundOn(&written, line, ground)
	return written.String()
}

// writeGroundOn writes what paintGround returns into the builder.
func writeGroundOn(dst *strings.Builder, line string, ground color.Color) {
	if line == "" {
		return
	}
	opening := resolveOpening(nil, ground)
	if opening == "" {
		dst.WriteString(line)
		return
	}
	dst.WriteString(opening)
	for {
		at := strings.Index(line, resetSequence)
		if at < 0 {
			break
		}
		dst.WriteString(line[:at+len(resetSequence)])
		dst.WriteString(opening)
		line = line[at+len(resetSequence):]
	}
	dst.WriteString(line)
	dst.WriteString(resetSequence)
}

// truncateStyled cuts a styled line to the width, keeping the escapes that colour it.
func truncateStyled(line string, width int) string {
	if measureStyledWidth(line) <= width {
		return line
	}
	// A style cuts every row of a block, and checks its border, its margin and its padding
	// on the way. One row needs only the cut.
	if strings.IndexByte(line, '\n') < 0 {
		return ansi.Truncate(line, width, "")
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// RenderStrip draws a one-row strip with one item at each end, which is what the title bar,
// the status bar and the banner are.
func (styles *Styles) RenderStrip(ground color.Color, width int, left, right string) string {
	room := max(width-2, 1)

	leftWidth := measureStyledWidth(left)
	rightWidth := measureStyledWidth(right)
	if leftWidth+rightWidth+1 > room {
		// The item on the left gives way first, because the one on the right is the count.
		left = truncateStyled(left, max(0, room-rightWidth-1))
		leftWidth = measureStyledWidth(left)
	}
	gap := max(room-leftWidth-rightWidth, 0)
	// Each blank carries the ground itself: an item ends with a reset, and a blank
	// after it would otherwise fall back to the ground of the terminal.
	var written strings.Builder
	written.Grow(len(left) + len(right) + gap + 3*cellEscapeBytes)
	writeBlanksOn(&written, ground, 1)
	written.WriteString(left)
	writeBlanksOn(&written, ground, gap)
	written.WriteString(right)
	writeBlanksOn(&written, ground, 1)
	return written.String()
}

// joinSideBySide puts one block of rows beside another, row by row. Joining through the layout
// tier reads both blocks, measures every row of them and lays them out again; the tree and the
// panes already carry the rows of the body, each row as wide as its own block, so the join only
// has to fill a row that came out short.
func joinSideBySide(left []string, leftWidth int, right []string, ground color.Color) []string {
	rows := max(len(left), len(right))

	joined := make([]string, 0, rows)
	var written strings.Builder
	for at := range rows {
		row := ""
		if at < len(left) {
			row = left[at]
		}
		beside := ""
		if at < len(right) {
			beside = right[at]
		}
		missing := leftWidth - measureStyledWidth(row)
		if missing <= 0 && beside == "" {
			joined = append(joined, row)
			continue
		}
		written.Reset()
		written.Grow(len(row) + len(beside) + max(0, missing) + cellEscapeBytes)
		written.WriteString(row)
		writeBlanksOn(&written, ground, missing)
		written.WriteString(beside)
		joined = append(joined, written.String())
	}
	return joined
}

// centerRows puts the rows of a block in the middle of an area of that many rows, with blank
// rows over and under it. A pane centres each of its own rows and stands on the ground of a
// panel, so this leaves the rows as they were drawn.
func centerRows(block []string, height int) []string {
	lines := make([]string, 0, height)
	for range halfRoundedUp(height - len(block)) {
		lines = append(lines, "")
	}
	lines = append(lines, block...)
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines[:height]
}

// CenterOn puts a block in the middle of an area of that size, on the ground of the screen.
func (styles *Styles) CenterOn(block string, width, height int) string {
	return strings.Join(styles.CenterRowsOn(block, width, height), "\n")
}

// CenterRowsOn returns what CenterOn returns, as its rows.
func (styles *Styles) CenterRowsOn(block string, width, height int) []string {
	ground := styles.Theme.Background
	rows := strings.Split(block, "\n")

	blockWidth := 0
	for _, row := range rows {
		if measured := measureStyledWidth(row); measured > blockWidth {
			blockWidth = measured
		}
	}
	left := halfRoundedUp(width - blockWidth)
	top := halfRoundedUp(height - len(rows))

	blank := padStyledOn("", width, ground)
	lead := padStyledOn("", left, ground)

	placed := make([]string, 0, height)
	for range top {
		placed = append(placed, blank)
	}
	for _, row := range rows {
		placed = append(placed, padStyledOn(lead+row, width, ground))
	}
	for len(placed) < height {
		placed = append(placed, blank)
	}
	return placed
}

// halfRoundedUp returns half a gap, with the odd cell kept. A gap of nine leaves five over
// a card and four under it.
func halfRoundedUp(gap int) int {
	if gap <= 0 {
		return 0
	}
	return gap - gap/2
}

// placeOver draws a card over a frame at that cell, so a popup paints over the panes
// under it without moving anything. The rows and the cells outside the card are kept.
func placeOver(rows []string, card string, left, top int, ground color.Color) []string {
	for offset, drawn := range strings.Split(card, "\n") {
		at := top + offset
		if at < 0 || at >= len(rows) {
			continue
		}
		rows[at] = overlayRow(rows[at], drawn, left, ground)
	}
	return rows
}

// overlayRow writes the card row into the frame row at that cell, and keeps what the
// card does not cover on either side.
func overlayRow(row, drawn string, left int, ground color.Color) string {
	head, rest, covered := cutRow(row, left)
	// A row of the frame that ends before the card carries the ground of the screen
	// up to it, so no cell is left with the ground of the terminal.
	if missing := left - covered; missing > 0 {
		head += paintOn(ground, strings.Repeat(" ", missing))
	}
	_, tail, _ := cutRow(rest, measureStyledWidth(drawn))
	return head + drawn + tail
}

// cutRow splits a drawn row at a cell, and returns the first cells of it, the rest of it, and
// how many cells the first half covers. The escapes that stood open at the cut are written
// again in front of the rest, so each half carries the colours of every cell in it.
//
// The row is walked in runs, as it is measured: a run of plain bytes is one cell each, and
// the library is asked only about a run that is not plain, and only about the one run the cut
// falls inside. Placing a card over the frame cuts every row the card covers, so the walk
// costs the frame more than anything else it does.
func cutRow(row string, cells int) (string, string, int) {
	if cells <= 0 {
		return "", row, 0
	}
	head := strings.Builder{}
	head.Grow(len(row))
	// The escapes that are open at the cut. A reset closes every one of them, and any
	// other escape lays over the ones before it.
	open := strings.Builder{}

	covered, at := 0, 0
	for at < len(row) && covered < cells {
		if row[at] == 0x1b {
			width := measureEscape(row[at:])
			escape := row[at : at+width]
			head.WriteString(escape)
			if escape == resetSequence {
				open.Reset()
			} else {
				open.WriteString(escape)
			}
			at += width
			continue
		}

		start := at
		plain := isPlainByte(row[at])
		for at < len(row) && row[at] != 0x1b && isPlainByte(row[at]) == plain {
			at++
		}
		run := row[start:at]
		width := len(run)
		if !plain {
			width = measureOddRun(run)
		}
		if covered+width <= cells {
			head.WriteString(run)
			covered += width
			continue
		}

		// The cut falls inside this run.
		wanted := cells - covered
		if plain {
			head.WriteString(run[:wanted])
			return head.String(), open.String() + run[wanted:] + row[at:], cells
		}
		taken := ansi.Truncate(run, wanted, "")
		head.WriteString(taken)
		// A character of two cells the cut runs through is drawn by neither half, so a
		// blank stands in its place and both halves measure what they should.
		if missing := wanted - lipgloss.Width(taken); missing > 0 {
			head.WriteString(strings.Repeat(" ", missing))
		}
		return head.String(),
			open.String() + ansi.TruncateLeft(run, wanted, "") + row[at:], cells
	}

	if at >= len(row) {
		return head.String(), "", covered
	}
	return head.String(), open.String() + row[at:], covered
}

// isPlainByte is true for a byte that is one cell of its own.
func isPlainByte(held byte) bool {
	return held >= 0x20 && held < 0x7f
}

// truncateCells returns the first cells of a row, with its colours kept.
func truncateCells(row string, cells int) string {
	head, _, _ := cutRow(row, cells)
	return head
}

// dropCells returns the row from that cell on, with its colours kept.
func dropCells(row string, cells int) string {
	_, tail, _ := cutRow(row, cells)
	return tail
}

// resolveThumbSpan returns the half cells of the track the thumb covers: where it starts and
// how long it is. The track is measured in half cells, because a list far taller than the
// view gives a thumb shorter than one row. It reports false where the view holds every row it
// has, which draws no bar at all.
func resolveThumbSpan(offset, viewport, total int) (int, int, bool) {
	if viewport <= 0 || total <= viewport {
		return 0, 0, false
	}
	track := viewport * 2
	size := min(max(track*viewport/total, 1), track)
	room := total - viewport
	start := 0
	if room > 0 {
		start = halfUpDivide(offset*(track-size), room)
	}
	return start, size, true
}

// resolveTrackOffset returns the first row a view shows when the top of its thumb stands on
// that row of the track. It is the step the other way from resolveThumbSpan, so a drag of the
// thumb and the bar it draws agree. A row outside the run of the thumb is held at its ends,
// so a press or a drag past either end of the track reaches that end of the view.
func resolveTrackOffset(row, viewport, total int) int {
	_, size, drawn := resolveThumbSpan(0, viewport, total)
	if !drawn {
		return 0
	}
	// The top of the thumb runs over the half cells the thumb leaves in the track, which is
	// this many whole rows of it.
	last := (viewport*2 - size) / 2
	if last < 1 {
		if row > 0 {
			return clampOffset(total-viewport, viewport, total)
		}
		return 0
	}
	row = min(max(row, 0), last)
	return clampOffset(halfUpDivide(row*(total-viewport), last), viewport, total)
}

// scrollView is what a bar stands for: how far the view has scrolled, how many rows it shows,
// how many rows it holds, and how to move it.
type scrollView struct {
	offset int
	rows   int
	total  int
	// moveTo writes the first row the view shows, and returns what the move asks for, such
	// as the next page of a result the drag reached the end of. It writes through the tab,
	// the connection or the card the bar belongs to, each of which outlives the frame that
	// drew it.
	moveTo func(offset int) tea.Cmd
}

// drawScrollTrack writes the thumb of a bar over the last cell of each row, and records where
// the track was drawn so a press or a drag of the thumb moves the view. The rows are the body
// of a pane or a card: `top` is the screen row of the first of them, and `left` the screen
// column of the first cell of one.
func (model *Model) drawScrollTrack(
	lines []string, view scrollView, top, left, width int, ground color.Color,
) []string {
	if width < 1 || len(lines) == 0 {
		return lines
	}
	thumb := buildScrollThumb(view.offset, view.rows, view.total)
	if len(thumb) == 0 {
		return lines
	}
	for at, glyph := range thumb {
		if at >= len(lines) || glyph == "" {
			continue
		}
		lines[at] = model.styles.paintThumbColumn(lines[at], glyph, width-1, ground)
	}
	model.recordScrollbar(left+width-1, top, min(view.rows, len(lines)),
		view.offset, view.total, view.moveTo)
	return lines
}

// recordScrollbar keeps where a bar was drawn, so a press or a drag of its thumb moves the
// view it stands for.
func (model *Model) recordScrollbar(
	column, top, rows, offset, total int, moveTo func(int) tea.Cmd,
) {
	if rows < 1 || moveTo == nil {
		return
	}
	model.layout.scrollbars = append(model.layout.scrollbars, scrollHit{
		column: column, top: top, rows: rows,
		offset: offset, total: total, moveTo: moveTo,
	})
}

// The glyphs a scroll bar draws. The bar is measured in half cells, so a long list gives a
// thumb shorter than one row.
const (
	thumbFull  = "█"
	thumbUpper = "▀"
	thumbLower = "▄"
)

// buildScrollThumb returns the glyph each row of a scroll bar draws, and an empty string for
// a row the thumb does not reach. A list that fits gets no bar at all.
func buildScrollThumb(offset, viewport, total int) []string {
	start, size, drawn := resolveThumbSpan(offset, viewport, total)
	if !drawn {
		return nil
	}

	glyphs := make([]string, viewport)
	for at := range glyphs {
		cellStart, cellEnd := at*2, at*2+2
		from := max(start, cellStart)
		to := min(start+size, cellEnd)
		switch covered := to - from; {
		case covered >= 2:
			glyphs[at] = thumbFull
		case covered <= 0:
		case from == cellStart:
			glyphs[at] = thumbUpper
		default:
			glyphs[at] = thumbLower
		}
	}
	return glyphs
}

// halfUpDivide returns the quotient rounded to the nearer whole number, with a half rounded
// up.
func halfUpDivide(top, bottom int) int {
	if bottom == 0 {
		return 0
	}
	return (top*2 + bottom) / (bottom * 2)
}

// paintOverStart writes a mark over the first cells of a drawn line, keeping the rest.
func (styles *Styles) paintOverStart(line, text string, ground color.Color) string {
	mark := paintText(styles.Theme.Muted, ground, text)
	return mark + dropCells(line, present.MeasureText(text))
}

// paintOverEnd writes a mark over the last cells of a drawn line, keeping the rest.
func (styles *Styles) paintOverEnd(line, text string, ground color.Color) string {
	keep := max(measureStyledWidth(line)-present.MeasureText(text), 0)
	mark := paintText(styles.Theme.Muted, ground, text)
	return truncateStyled(line, keep) + mark
}

// renderThumbCell draws one cell of a scroll bar that stands beside the rows it measures,
// rather than over the last cell of them.
func (styles *Styles) renderThumbCell(glyph string, ground color.Color) string {
	if glyph == "" {
		glyph = " "
	}
	return paintText(styles.Theme.Border, ground, glyph)
}

// paintThumbColumn writes one glyph of a scroll bar over the cell at that column of a line.
func (styles *Styles) paintThumbColumn(
	line, glyph string, column int, ground color.Color,
) string {
	if glyph == "" {
		return line
	}
	tail := max(measureStyledWidth(line)-column-1, 0)
	mark := paintText(styles.Theme.Border, ground, glyph)
	return truncateStyled(padStyledOn(line, column, ground), column) + mark +
		padStyledOn("", tail, ground)
}

// styledCell is one cell of a drawn line: the text it holds, and the escapes that colour it.
type styledCell struct {
	sgr  string
	text string
}

// mapCells reads a drawn line as its cells, each with the escapes in force where it stands.
// A cell wider than one column keeps its width, so the count of cells is the width of the
// line.
func mapCells(line string) []styledCell {
	cells := []styledCell{}
	sgr := ""
	for at := 0; at < len(line); {
		if line[at] == '\x1b' {
			end := strings.IndexByte(line[at:], 'm')
			if end == -1 {
				break
			}
			sequence := line[at : at+end+1]
			if sequence == resetSequence || sequence == "\x1b[0m" {
				sgr = ""
			} else {
				sgr += sequence
			}
			at += end + 1
			continue
		}
		next := at + 1
		for next < len(line) && line[next] >= 0x80 && line[next] < 0xc0 {
			next++
		}
		text := line[at:next]
		cells = append(cells, styledCell{sgr: sgr, text: text})
		// A cell two columns wide takes the column beside it, which draws nothing.
		for filler := 1; filler < measureStyledWidth(text); filler++ {
			cells = append(cells, styledCell{sgr: sgr})
		}
		at = next
	}
	return cells
}

// writeCells writes the cells back as one line, with a run of cells that share their escapes
// written once.
func writeCells(cells []styledCell) string {
	written := strings.Builder{}
	sgr := ""
	for _, cell := range cells {
		if cell.sgr != sgr {
			if sgr != "" {
				written.WriteString(resetSequence)
			}
			written.WriteString(cell.sgr)
			sgr = cell.sgr
		}
		written.WriteString(cell.text)
	}
	if sgr != "" {
		written.WriteString(resetSequence)
	}
	return written.String()
}

// buildSgr returns the escapes that paint a cell in these colours, without the reset that
// closes a rendered string.
func buildSgr(ink, ground color.Color) string {
	return resolveOpening(ink, ground)
}

// paintCaretOverStart draws the caret over the first cell of a drawn line, keeping the rest.
func paintCaretOverStart(line string, ground, ink color.Color) string {
	cells := mapCells(line)
	if len(cells) == 0 {
		return lipgloss.NewStyle().Background(ground).Foreground(ink).Render(" ")
	}
	return lipgloss.NewStyle().Background(ground).Foreground(ink).Render(cells[0].text) +
		dropCells(line, 1)
}
