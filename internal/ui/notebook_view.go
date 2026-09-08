package ui

import (
	"image/color"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
)

// The cell list stands where a query tab draws its editor. Each cell has a header row, and
// an open cell adds the rows of its body under it.

// The widths of one row of the cell list.
const (
	// cellKindWidth is wide enough for the longest kind name.
	cellKindWidth = 6
	// cellBodyIndent is the columns before the mark of a body row.
	cellBodyIndent = 5
	// cellOutcomeWidth is the room the report at the right of a header keeps.
	cellOutcomeWidth = 18
	// cellBodyLimit is how many rows of one cell the list draws.
	cellBodyLimit = 14
)

// The marks of the cell list.
const (
	cellFocusMark  = "▌"
	cellOpenMark   = "▾"
	cellFoldedMark = "▸"
	cellBodyMark   = "┆ "
)

// notebookRow is one drawn row of the list and the cell it belongs to.
type notebookRow struct {
	cell int
	text string
	// True for the header of a cell, which the cursor stands on.
	header bool
}

// renderNotebook draws the cells of a notebook tab.
func (model *Model) renderNotebook(
	connection *app.Connection, tab *app.Tab, width, height int,
) []string {
	theme := model.styles.Theme
	book := tab.Notebook
	prompt, asking := findPromptBar(connection, app.PromptCellName)
	focused := tab.Focus == app.PaneEditor && (asking || !connection.Overlay.IsOpen())
	inner := width - 2
	body := max(height-2, 1)

	promptRows := []string{}
	if asking {
		promptRows = model.renderPromptBar(prompt, inner)
		body = max(body-len(promptRows), 1)
	}

	rows := model.buildNotebookRows(connection, tab, inner, focused)
	// The list follows the focused cell, unless the wheel moved it away.
	offset := book.Offset
	if !book.Rolled {
		offset = resolveNotebookOffset(rows, book.Focused, book.Offset, body)
	}
	offset = clampOffset(offset, body, len(rows))
	book.Offset = offset

	written := make([]string, 0, body)
	for at := offset; at < len(rows) && len(written) < body; at++ {
		written = append(written, rows[at].text)
	}
	model.recordCellRows(rows, offset, len(written), width)
	for len(written) < body {
		written = append(written, "")
	}
	// The bar of the list is a cell of its own at the end of each row, and a drag of it
	// moves the list as the wheel does.
	written = model.drawScrollTrack(written, scrollView{
		offset: offset, rows: body, total: len(rows),
		moveTo: func(held int) tea.Cmd {
			book.Offset, book.Rolled = held, true
			return nil
		},
	}, firstPaneRow+1, model.editorLeft+1, inner, theme.Panel)
	written = append(written, promptRows...)

	return model.styles.RenderBoxRows(BoxOptions{
		Width: width, Height: height, Focused: focused,
		Title:       model.describeNotebookTitle(tab),
		Note:        model.describeNotebookNote(tab, inner),
		BottomTitle: model.describeNotebookBorder(book),
		BottomNote: model.styles.Muted().Background(theme.Panel).
			Render(describeNotebookPlace(book)),
		Lines: written, Ground: theme.Panel,
	})
}

// recordCellRows keeps which cell every row of the list belongs to, so a press reaches
// that cell.
func (model *Model) recordCellRows(rows []notebookRow, offset, drawn, width int) {
	cells := make([]int, 0, len(rows))
	for _, row := range rows {
		cells = append(cells, row.cell)
	}
	model.cellsOfRows = cells
	model.layout.cellRows = rowsHit{
		top: firstPaneRow + 1, count: drawn, offset: offset,
		from: model.editorLeft + 1, to: model.editorLeft + width - 2,
	}
}

// resolveNotebookOffset returns the first row drawn. The whole focused cell stays in view
// where it fits, and its header always does.
func resolveNotebookOffset(rows []notebookRow, focused, offset, body int) int {
	at, end := -1, -1
	for index, row := range rows {
		if row.cell != focused {
			continue
		}
		if row.header {
			at = index
		}
		end = index
	}
	if at < 0 {
		return offset
	}
	held := scrollTo(end, scrollTo(at, offset, body, len(rows)), body, len(rows))
	if at < held {
		return at
	}
	return held
}

// buildNotebookRows returns every row of the list, header and body.
func (model *Model) buildNotebookRows(
	connection *app.Connection, tab *app.Tab, inner int, focused bool,
) []notebookRow {
	book := tab.Notebook
	rows := make([]notebookRow, 0, len(book.Cells)*4)
	for at, cell := range book.Cells {
		// A blank row stands between one cell and the next, so the head of a cell reads
		// apart from the body of the cell above it.
		if at > 0 {
			rows = append(rows, notebookRow{cell: at - 1})
		}
		onCell := at == book.Focused && focused
		rows = append(rows, notebookRow{
			cell: at, header: true,
			text: model.renderCellHeader(tab, cell, at, inner, onCell),
		})
		if cell.Folded {
			continue
		}
		for _, line := range model.buildCellBody(connection, tab, cell, inner) {
			rows = append(rows, notebookRow{cell: at, text: line})
		}
	}
	return rows
}

// renderCellHeader draws the row of one cell: its state, its number, its kind, its name
// and what its last run answered.
func (model *Model) renderCellHeader(
	tab *app.Tab, cell *app.NotebookCell, at, inner int, focused bool,
) string {
	theme := model.styles.Theme
	ground := theme.Panel
	if focused {
		ground = theme.Zebra
	}

	lead := paintOn(ground, " ")
	if focused {
		lead = paintText(theme.Accent, ground, cellFocusMark)
	}
	outcome := tab.ReadCellOutcome(cell)
	mark := paintText(model.readOutcomeInk(outcome.Kind), ground,
		describeOutcomeMark(outcome.Kind))
	number := paintText(theme.Muted, ground,
		present.FitText(strconv.Itoa(at+1), 3))
	if cell.Marked {
		number = paintText(theme.Accent, ground,
			present.FitText("*"+strconv.Itoa(at+1), 3))
	}
	kind := paintText(theme.Info, ground,
		present.FitText(string(cell.Kind), cellKindWidth))
	fold := paintText(theme.Faint, ground, cellFoldedMark)
	if !cell.Folded {
		fold = paintText(theme.Faint, ground, cellOpenMark)
	}

	report := model.describeCellOutcome(cell, outcome, ground)
	titleWidth := max(inner-cellBodyIndent-cellKindWidth-cellOutcomeWidth-4, 4)
	titleInk := theme.Text
	if focused {
		titleInk = theme.Accent
	}
	title := paintText(titleInk, ground,
		present.FitText(present.TruncateText(cell.BuildTitle(), titleWidth), titleWidth))

	return lead + mark + paintOn(ground, " ") + number + paintOn(ground, " ") + kind +
		title + paintOn(ground, "  ") + report + paintOn(ground, " ") + fold
}

// describeOutcomeMark returns the glyph of one state of a cell.
func describeOutcomeMark(kind app.CellOutcomeKind) string {
	switch kind {
	case app.CellDone:
		return "✓"
	case app.CellFailed:
		return "✗"
	case app.CellRunning:
		return "•"
	case app.CellStale:
		return "~"
	}
	return " "
}

// readOutcomeInk returns the colour of one state of a cell.
func (model *Model) readOutcomeInk(kind app.CellOutcomeKind) color.Color {
	theme := model.styles.Theme
	switch kind {
	case app.CellDone:
		return theme.Success
	case app.CellFailed:
		return theme.Error
	case app.CellRunning:
		return theme.Accent
	case app.CellStale:
		return theme.Warning
	}
	return theme.Faint
}

// describeCellOutcome returns the report at the right of a header row.
func (model *Model) describeCellOutcome(
	cell *app.NotebookCell, outcome app.CellOutcome, ground color.Color,
) string {
	theme := model.styles.Theme
	ink, text := theme.Muted, ""
	switch {
	case outcome.Kind == app.CellFailed:
		ink, text = theme.Error, "failed"
	case outcome.Kind == app.CellRunning:
		ink, text = theme.Accent, "running"
	case outcome.Kind == app.CellStale:
		ink, text = theme.Warning, "stale · run again"
	case outcome.Kind == app.CellDone:
		ink = theme.Success
		text = present.FormatCountOf(int64(outcome.Rows), "row", "rows")
	case cell.Kind == notebook.CellText:
		text = "prose"
	case cell.Kind == notebook.CellParam:
		text = "values"
	case cell.Kind == notebook.CellChart:
		text = "from " + describeChartSource(cell)
	case cell.AsksConfirmation():
		ink, text = theme.Warning, "write · asks first"
	default:
		text = "not run"
	}
	if outcome.Kind == app.CellDone && cell.AsksConfirmation() {
		ink, text = theme.Warning, "write · "+text
	}
	fitted := present.FitText(
		present.TruncateText(text, cellOutcomeWidth), cellOutcomeWidth)
	// The report is held against the right of the row.
	return paintText(ink, ground, fitted)
}

// describeChartSource returns the cell a chart draws from.
func describeChartSource(cell *app.NotebookCell) string {
	if source, held := cell.FindAttr("source"); held {
		return present.SafeText(source)
	}
	return "no source cell"
}

// buildCellBody returns the rows under a header: the source, the prose, or the chart.
func (model *Model) buildCellBody(
	connection *app.Connection, tab *app.Tab, cell *app.NotebookCell, inner int,
) []string {
	theme := model.styles.Theme
	width := max(inner-cellBodyIndent-len(cellBodyMark), 4)
	prefix := paintOn(theme.Panel, strings.Repeat(" ", cellBodyIndent)) +
		paintText(theme.Faint, theme.Panel, cellBodyMark)

	lines := model.buildCellLines(connection, tab, cell, width)
	rows := make([]string, 0, len(lines)+1)
	for at, line := range lines {
		if at >= cellBodyLimit {
			rows = append(rows, prefix+paintText(theme.Faint, theme.Panel,
				present.FormatCount(int64(len(lines)-cellBodyLimit))+" more lines"))
			break
		}
		rows = append(rows, prefix+line)
	}
	return rows
}

// buildCellLines returns the body of one cell, already styled.
func (model *Model) buildCellLines(
	connection *app.Connection, tab *app.Tab, cell *app.NotebookCell, width int,
) []string {
	theme := model.styles.Theme
	switch cell.Kind {
	case notebook.CellChart:
		return model.buildChartLines(tab, cell, width)
	case notebook.CellText, notebook.CellOther:
		lines := []string{}
		for _, line := range strings.Split(cell.Editor.Text, "\n") {
			lines = append(lines, paintText(theme.Muted, theme.Panel,
				present.FitText(present.SafeText(present.TruncateText(line, width)), width)))
		}
		return lines
	}

	text := cell.Editor.Text
	if strings.TrimSpace(text) == "" {
		empty := "empty"
		if chord := model.registry.FormatFirstActionChord(
			cfg.ScopeNotebook, ActionEditCellSource); chord != "" {
			empty += "; press " + chord + " to edit"
		}
		return []string{paintText(theme.Faint, theme.Panel, present.FitText(empty, width))}
	}
	source := strings.Split(text, "\n")
	spans := collectLineHighlights(text, connection.Session.Language().Tokenize(text))
	lines := make([]string, 0, len(source))
	for at, line := range source {
		lines = append(lines, model.renderCodeLine(codeLine{
			text: line, spans: spans[at], width: width,
		}))
	}
	return lines
}

// describeNotebookTitle returns the title on the top border of the cell list.
func (model *Model) describeNotebookTitle(tab *app.Tab) string {
	book := tab.Notebook
	written := " notebook · " + tab.NotebookName()
	written = present.SafeText(written)
	if book.Origin != "" && book.Path != "" {
		written += " · " + string(book.Origin)
	}
	if book.Path == "" {
		written += " · unsaved"
	}
	return written + " "
}

// describeNotebookNote returns the note at the right of the top border: where the cursor
// stands, and whether the file holds what is on screen.
func (model *Model) describeNotebookNote(tab *app.Tab, inner int) string {
	theme := model.styles.Theme
	book := tab.Notebook
	written := "cell " + strconv.Itoa(book.Focused+1) + "/" +
		strconv.Itoa(book.CountCells())
	note := model.styles.Muted().Background(theme.Panel).Render(written)
	if book.Dirty {
		note += paintText(theme.Warning, theme.Panel, " ● edited")
	}
	if present.MeasureText(written) > inner {
		return ""
	}
	return note
}

// describeNotebookBorder returns the title on the bottom border of the cell list.
func (model *Model) describeNotebookBorder(book *app.Notebook) string {
	return " " + describeTransactionPolicy(book.Run.Transaction) + " · " +
		book.Run.OnError + " on error "
}

// describeNotebookPlace returns what the foot of the border says about the focused cell.
func describeNotebookPlace(book *app.Notebook) string {
	if book.Editing {
		return "editing"
	}
	if marked := len(book.ListMarkedCells()); marked > 0 {
		return present.FormatCount(int64(marked)) + " marked"
	}
	return string(book.GetFocusedCell().Kind)
}

// describeTransactionPolicy returns the transaction policy as a reader reads it.
func describeTransactionPolicy(held string) string {
	if held == notebook.TransactionSingle {
		return "one transaction"
	}
	return notebook.TransactionAutocommit
}
