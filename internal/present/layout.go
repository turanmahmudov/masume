package present

import (
	"math"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query/editor"
)

// The width limits of a tab label. A long batch shrinks its labels, so they stay inside
// the pane.
const (
	minLabelWidth = 8
	maxLabelWidth = 24
	// labelChrome is the padding on both sides plus the gap to the next label.
	labelChrome = 3
)

// PlanLabelWidth returns how wide each label of a strip may be drawn.
func PlanLabelWidth(count, available int) int {
	if count <= 0 {
		return 0
	}
	share := available/count - labelChrome
	if share < minLabelWidth {
		return minLabelWidth
	}
	if share > maxLabelWidth {
		return maxLabelWidth
	}
	return share
}

// The widths of a field list: a name and a type before each value.
const (
	fieldNameWidth = 24
	fieldTypeWidth = 16
	// Below these widths a name and a type say too little.
	minFieldNameWidth = 8
	minFieldTypeWidth = 6
	// The width kept for the value, so a narrow card still shows one.
	minFieldValueWidth = 12
)

// FieldColumnPlan holds the name column and the type column of a field list.
type FieldColumnPlan struct {
	Name int
	Type int
}

// PlanFieldColumns returns the widths of a field list. On a narrow card both the name and
// the type shrink together and the value keeps its width. The widths are planned here,
// because a column that shrinks on its own puts the rows out of line.
func PlanFieldColumns(available int) FieldColumnPlan {
	wanted := fieldNameWidth + fieldTypeWidth + minFieldValueWidth
	if available >= wanted {
		return FieldColumnPlan{Name: fieldNameWidth, Type: fieldTypeWidth}
	}

	nameSlack := fieldNameWidth - minFieldNameWidth
	typeSlack := fieldTypeWidth - minFieldTypeWidth
	// Each column gives up its own share, so both keep a readable width.
	taken := min(wanted-available, nameSlack+typeSlack)
	fromName := int(math.Round(float64(taken*nameSlack) / float64(nameSlack+typeSlack)))
	return FieldColumnPlan{
		Name: fieldNameWidth - fromName,
		Type: fieldTypeWidth - (taken - fromName),
	}
}

// WrapWords breaks a text into the rows it takes at a given width, on the spaces between
// its words. A word longer than the row is broken over several rows.
//
// A word may end on the last cell of a row only where the text ends with it. A word with
// more text after it needs room for the space that would follow it as well.
func WrapWords(text string, width int) []string {
	text = SafeText(text)
	if width <= 0 {
		return []string{text}
	}
	// The spaces a text opens with belong to it, so an indented line keeps its indent.
	indent := text[:len(text)-len(strings.TrimLeft(text, " "))]
	words := strings.Split(text[len(indent):], " ")
	rows := []string{}
	line, opening := "", indent
	for at, word := range words {
		room := width
		if at < len(words)-1 {
			room--
		}
		switch {
		case line == "":
			line = opening + word
			opening = ""
		case MeasureText(line)+1+MeasureText(word) <= room:
			line += " " + word
		default:
			rows = append(rows, line)
			line = word
		}
		for MeasureText(line) > width {
			head, tail := cutAtWidth(line, width)
			// A character wider than the row is all that is left, and it keeps the row
			// it was given rather than opening an empty one after it.
			if tail == "" {
				break
			}
			rows = append(rows, head)
			line = tail
		}
	}
	return append(rows, line)
}

// cutAtWidth splits a text at the last character that still fits in the width.
func cutAtWidth(text string, width int) (string, string) {
	used := 0
	for at, character := range text {
		cost := MeasureText(string(character))
		if used+cost > width {
			// A first character wider than the row would cut nothing, and the caller
			// would ask again with the same text. It goes on its own row instead.
			if at == 0 {
				end := len(string(character))
				return text[:end], text[end:]
			}
			return text[:at], text[at:]
		}
		used += cost
	}
	return text, ""
}

// CountWrappedRows returns the rows a word-wrapped text takes at a given width. It counts
// what WrapWords writes, because a caller sizes from this and then draws with that.
func CountWrappedRows(text string, width int) int {
	return len(WrapWords(text, width))
}

// The widths of the strip of views above the grid.
const (
	// viewChipChrome is the number, the space after it, and the padding on each side.
	viewChipChrome = 4
	// viewHintWidth is the `, . prev/next` hint at the far end of the strip.
	viewHintWidth = 15
)

// ViewStripPlan says how much of the view strip a pane has the width for.
type ViewStripPlan struct {
	// Named is false if only the numbers fit, and the name of the current view.
	Named     bool
	ShowsHint bool
}

func measureViewStrip(names []string, named bool, activeIndex int) int {
	total := 0
	for index, name := range names {
		shown := 0
		if named || index == activeIndex {
			shown = MeasureText(name)
		}
		total += viewChipChrome + shown
		if shown == 0 {
			total--
		}
	}
	return total
}

// PlanViewStrip drops the hint first, and the names of the other views second, so every
// view keeps a number to press in a narrow pane.
func PlanViewStrip(names []string, activeIndex, available int) ViewStripPlan {
	named := measureViewStrip(names, true, activeIndex)
	if named+viewHintWidth <= available {
		return ViewStripPlan{Named: true, ShowsHint: true}
	}
	if named <= available {
		return ViewStripPlan{Named: true}
	}
	numbered := measureViewStrip(names, false, activeIndex)
	return ViewStripPlan{ShowsHint: numbered+viewHintWidth <= available}
}

// minSidebarWidth is the narrowest object tree that still names something.
const minSidebarWidth = 16

// minPaneWidth is the width the editor and the result keep beside the tree, where the
// terminal has it.
const minPaneWidth = 32

// PlanSidebarWidth returns the width of the object tree. It shrinks with the terminal,
// because a tree of its full width would leave a narrow terminal no room for the panes
// beside it.
func PlanSidebarWidth(available, preferred int) int {
	room := max(available-minPaneWidth, minSidebarWidth)
	width := min(preferred, room, available)
	if width < 0 {
		return 0
	}
	return width
}

// ColumnPlanInput holds what deciding the visible columns needs.
type ColumnPlanInput struct {
	Widths []int
	// The columns that always draw at the left, whatever the window shows.
	Frozen       map[int]bool
	ColumnOffset int
	Available    int
	Gap          int
}

// ColumnPlan is the first column of the window, and how many columns it spans.
type ColumnPlan struct {
	WindowStart  int
	VisibleCount int
}

// PlanVisibleColumns decides which columns fit beside the frozen ones. Frozen columns
// always draw, and they take their width from the same total. The window spans over them
// without drawing them a second time.
func PlanVisibleColumns(input ColumnPlanInput) ColumnPlan {
	windowStart := max(input.ColumnOffset, 0)

	used := 0
	for index := range input.Frozen {
		if index < len(input.Widths) {
			used += input.Widths[index] + input.Gap
		}
	}

	visibleCount := 0
	drawn := 0
	for index := windowStart; index < len(input.Widths); index++ {
		if input.Frozen[index] {
			visibleCount++
			continue
		}
		used += input.Widths[index] + input.Gap
		if used > input.Available && drawn > 0 {
			break
		}
		visibleCount++
		drawn++
	}
	if visibleCount < 1 {
		visibleCount = 1
	}
	return ColumnPlan{WindowStart: windowStart, VisibleCount: visibleCount}
}

// ColumnHitInput holds what finding the column under a click needs.
type ColumnHitInput struct {
	Offset         int
	GutterWidth    int
	VisibleIndexes []int
	Widths         []int
	Gap            int
}

// IsOnFoldMarker is true where the click landed on the fold mark. Only the mark folds a
// row: a click on the name selects it.
func IsOnFoldMarker(offset, depth, indentPerLevel, markerWidth int) bool {
	start := depth * indentPerLevel
	return offset >= start && offset < start+markerWidth
}

// TabWindow is the tabs that draw, and where they start.
type TabWindow struct {
	Start int
	Count int
}

// tabEdgeWidth is the width kept at each end for the count of the tabs that did not fit.
const tabEdgeWidth = 10

// PlanVisibleTabs keeps the window still while the current tab is inside it, and then
// scrolls as little as it can.
func PlanVisibleTabs(widths []int, activeIndex, available, offset int) TabWindow {
	total := len(widths)
	if total == 0 {
		return TabWindow{}
	}
	spent := 0
	for _, width := range widths {
		spent += width
	}
	if spent <= available {
		return TabWindow{Count: total}
	}

	active := core.ClampIndex(activeIndex, total)
	start := min(active, core.ClampIndex(offset, total))

	countFrom := func(from int) int {
		used := 0
		fitted := 0
		for index := from; index < total; index++ {
			budget := available
			if from > 0 {
				budget -= tabEdgeWidth
			}
			if index < total-1 {
				budget -= tabEdgeWidth
			}
			used += widths[index]
			if used > budget && fitted > 0 {
				break
			}
			fitted++
		}
		if fitted < 1 {
			return 1
		}
		return fitted
	}

	fitted := countFrom(start)
	for active >= start+fitted && start < total-1 {
		start++
		fitted = countFrom(start)
	}
	return TabWindow{Start: start, Count: fitted}
}

// minDetailWidth is the width below which a column of a detail table says too little.
const minDetailWidth = 4

// PlanDetailColumns measures the columns of a read-only table from their content and then
// fits them to the pane. A fixed width would cut a definition short on a wide screen, and
// run past the border on a narrow one. The extra width is taken from the widest column
// first, so a name column keeps its names and a definition column gives way.
func PlanDetailColumns(headers []string, rows [][]string, available, gap int) []int {
	widths := make([]int, len(headers))
	for index, header := range headers {
		widest := MeasureText(header)
		for _, row := range rows {
			if index < len(row) {
				if measured := MeasureText(row[index]); measured > widest {
					widest = measured
				}
			}
		}
		widths[index] = widest
	}

	spent := func() int {
		total := 0
		for _, width := range widths {
			total += width + gap
		}
		return total
	}

	for spent() > available {
		widest := 0
		for index, width := range widths {
			if width > widths[widest] {
				widest = index
			}
		}
		if widths[widest] <= minDetailWidth {
			break
		}
		widths[widest]--
	}
	return widths
}

// The width limits of a result column.
const (
	minColumnWidth = 4
	maxColumnWidth = 28
)

// CalculateColumnWidths measures each result column from its header and its cells.
func CalculateColumnWidths(headers []string, rows [][]string) []int {
	widths := make([]int, 0, len(headers))
	for index, header := range headers {
		widest := MeasureText(header)
		for _, row := range rows {
			if index < len(row) {
				if measured := MeasureText(row[index]); measured > widest {
					widest = measured
				}
			}
		}
		if widest < minColumnWidth {
			widest = minColumnWidth
		}
		if widest > maxColumnWidth {
			widest = maxColumnWidth
		}
		widths = append(widths, widest)
	}
	return widths
}

// CardChrome is the rows the border and the padding of a card take.
const CardChrome = 4

// ScreenMargin is the gap a card of a screen leaves to the edge of the terminal.
const ScreenMargin = 8

// ResolveCardWidth returns the width of a card in cells. The screen width is the limit,
// because a wider card draws over everything beside it.
func ResolveCardWidth(widest, narrowest, available int) int {
	width := max(min(widest, available-ScreenMargin), narrowest)
	if width > available {
		width = available
	}
	if width < 1 {
		return 1
	}
	return width
}

// AlignCardHeight returns a height that leaves an even number of rows above and below. An
// odd number cannot be shared evenly, and the half row rounds the last row of the card off
// the screen, so the card takes that row instead.
func AlignCardHeight(height, available int) int {
	if (available-height)%2 == 0 {
		return height
	}
	if height < available {
		return height + 1
	}
	return height - 1
}

// TextPosition is a line and a column, counted from one.
type TextPosition struct {
	Line   int
	Column int
}

// ResolvePosition returns the line and column of an offset in the text.
func ResolvePosition(text string, offset int) TextPosition {
	capped := core.ClampWithin(offset, len(text))
	before := text[:capped]
	lastBreak := strings.LastIndexByte(before, '\n')
	return TextPosition{
		Line:   strings.Count(before, "\n") + 1,
		Column: capped - lastBreak,
	}
}

// LineSpan is a span cut to its first line, in columns from zero.
type LineSpan struct {
	Line  int
	Start int
	End   int
}

// ResolveLineSpan returns the span of a fault on its first line. A span over several lines
// is marked on the first line only.
func ResolveLineSpan(text string, diagnostic editor.Diagnostic) LineSpan {
	at := ResolvePosition(text, diagnostic.Start)
	lineStart := diagnostic.Start - (at.Column - 1)
	lineEnd := len(text)
	if broke := strings.IndexByte(text[core.ClampWithin(diagnostic.Start, len(text)):], '\n'); broke != -1 {
		lineEnd = core.ClampWithin(diagnostic.Start, len(text)) + broke
	}

	start := at.Column - 1
	end := min(diagnostic.End, lineEnd)
	end -= lineStart
	if end < start+1 {
		end = start + 1
	}
	return LineSpan{Line: at.Line - 1, Start: start, End: end}
}
