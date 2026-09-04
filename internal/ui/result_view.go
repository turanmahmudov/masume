package ui

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// needsResult is what a view says while nothing has run.
const needsResult = "run a query first"

// The widths of the result pane.
const (
	// The gutter numbers each row of the result, so the footer can name where the cursor is.
	gutterGap = 1
	// The gap between two columns of the grid.
	columnGap = 2
	// The key label at the left of the statement row, and the count at its right.
	stripChrome = 14
)

// renderResultPane draws the grid, the detail views, and the strips above them.
func (model *Model) renderResultPane(
	connection *app.Connection, tab *app.Tab, width, height int,
) []string {
	theme := model.styles.Theme
	// A prompt drawn at the foot of this pane leaves the pane looking as it did, because the
	// reader is still working in it.
	prompt, asking := findPromptBar(
		connection, app.PromptSearch, app.PromptWhere, app.PromptGoToColumn)
	focused := tab.Focus == app.PaneResult && (asking || !connection.Overlay.IsOpen())
	inner := width - 2
	body := max(height-2, 1)

	// The views of a tab are read once for the frame, because each one asks the language
	// whether the server has a plan for the statement.
	views := tab.Views(connection.Session)
	drawn := app.ResolveDrawnView(views, tab.View)

	promptRows := []string{}
	if asking {
		promptRows = model.renderPromptBar(prompt, inner)
		body -= len(promptRows)
	}

	lines := []string{}

	// A result belongs to a statement, so the statements are the outer row and the views of
	// the one selected sit under it.
	statements := tab.Results.Results()
	if len(statements) > 1 {
		lines = append(lines, model.renderStatementStrip(
			tab, inner, model.layout.resultTop+1+len(lines)))
		body--
	}

	// A view is a way of looking at a result, so the strip waits for one: on the first
	// screen a reader sees, a row of views that all show nothing reads as an interface that
	// is already broken.
	if tab.Results.State().Kind != app.QueryIdle {
		lines = append(lines, model.renderViewStrip(views, drawn, inner,
			len(statements) > 1, model.layout.resultTop+1+len(lines)))
		body--
	}

	// The strip that names the sort and the filter belongs to every view that draws the
	// rows they shape. A reader who filtered in the tree has to be told what is hidden and
	// how to bring it back, the same way the grid tells them.
	banner := model.describeBanner(connection, tab)
	if DrawsResultRows(drawn) && banner != "" {
		lines = append(lines, model.renderBanner(tab, drawn, banner, inner,
			model.layout.resultTop+1+len(lines), model.editorLeft+1))
		body--
	}
	if body < 1 {
		body = 1
	}

	if drawn == app.ViewData {
		shape := model.buildGridShape(connection, tab)
		size, where := model.describeGridFooter(tab, shape)
		// The size and the place of the cursor stand on a row of their own under the
		// grid, so a wide result keeps them apart at both ends.
		if size != "" || where != "" {
			body--
		}
		// The first row of the rows on screen, so a press lands on the cell it looks
		// like. The header takes the row above them.
		model.layout.gridHeaderRow = model.layout.resultTop + 1 + len(lines)
		model.layout.gridRows.top = model.layout.gridHeaderRow + 1
		lines = append(lines, model.renderGrid(connection, tab, shape, inner, body)...)
		if size != "" || where != "" {
			lines = append(lines, model.styles.RenderStrip(theme.Background, inner,
				paintText(theme.Muted, theme.Background, size),
				paintText(theme.Muted, theme.Background, where)))
		}
	} else {
		// The document views name where the cursor stands, the way the grid does: a tree
		// of a thousand rows says nothing about which document is open without it.
		size, where := model.describeDocumentFooter(connection, tab, drawn)
		if size != "" || where != "" {
			body--
		}
		if body < 1 {
			body = 1
		}
		// Where the body of the view starts on the screen, so the bar it draws is
		// recorded at the rows it stands on.
		model.layout.detailTop = model.layout.resultTop + 1 + len(lines)
		lines = append(lines, model.renderDetailView(
			connection, tab, drawn, inner, body, model.layout.detailTop)...)
		if size != "" || where != "" {
			lines = append(lines, model.styles.RenderStrip(theme.Background, inner,
				paintText(theme.Muted, theme.Background, size),
				paintText(theme.Muted, theme.Background, where)))
		}
	}
	lines = append(lines, promptRows...)

	return model.styles.RenderBoxRows(BoxOptions{
		Width: width, Height: height, Focused: focused,
		Title: model.describeResultTitle(connection, tab, drawn),
		Lines: lines, Ground: theme.Panel,
	})
}

// describeResultTitle names what this pane shows: a relation, an object, or a statement, with
// the view after it.
func (model *Model) describeResultTitle(
	connection *app.Connection, tab *app.Tab, drawn app.ResultView,
) string {
	subject := "result"
	switch tab.Kind {
	case app.TabTable:
		subject = model.icons.Prefix(present.TableIcons[tab.Table.Kind]) +
			tab.Table.Schema + "." + tab.Table.Name
	case app.TabObject:
		subject = model.icons.Prefix(cfg.IconKind(tab.Object.Kind)) +
			tab.Object.Schema + "." + tab.Object.Name
	}

	// The view name after the subject, so `▦ public.orders · indexes` says whose indexes
	// these are.
	subtitle := string(drawn)
	if drawn == app.ViewData {
		subtitle = model.describeDataTitle(tab)
	}
	if drawn == app.ViewPlan && tab.ViewData.Kind == app.DataPlan {
		subtitle = "plan · estimated"
		if tab.ViewData.Plan.Analyzed {
			subtitle = "plan · analyzed"
		}
	}
	if subtitle == "" {
		return " " + subject + " "
	}
	return " " + subject + " · " + subtitle + " "
}

// describeDataTitle writes how far the run of the statement on screen got.
func (model *Model) describeDataTitle(tab *app.Tab) string {
	state := tab.Results.State()
	switch state.Kind {
	case app.QueryRunning:
		return "running…"
	case app.QueryFailed:
		return "failed"
	case app.QueryIdle:
		return ""
	}
	return present.FormatDuration(state.Result.Elapsed)
}

// renderStatementStrip draws the statements of a batch, and which of them the pane draws.
func (model *Model) renderStatementStrip(tab *app.Tab, width, row int) string {
	theme := model.styles.Theme
	statements := tab.Results.Results()
	labelWidth := present.PlanLabelWidth(len(statements), width-stripChrome)

	written := []string{}
	// The strip opens at the first column inside the border of the pane.
	left := model.editorLeft + 1
	chips := make([]chipHit, 0, len(statements))
	for at, entry := range statements {
		ground := theme.Header
		ink := theme.Text
		if at == tab.Results.ActiveIndex() {
			ground = theme.Success
			ink = model.styles.InkOn(theme.Success)
		}
		chip := paintText(ink, ground, " "+present.TruncateText(entry.Label, labelWidth)+" ")
		chips = append(chips, chipHit{
			index: at, row: row, from: left, to: left + measureStyledWidth(chip) - 1,
		})
		left += measureStyledWidth(chip) + 1
		written = append(written, chip+
			paintOn(theme.Header, " "))
	}
	model.layout.statementChips = chips

	keys := model.sayKeys().
		say(strconv.Itoa(tab.Results.ActiveIndex()+1)+" of "+strconv.Itoa(len(statements))).
		bindPair(cfg.ScopeGlobal, ActionPreviousStatement, ActionNextStatement,
			"prev/next", " ")
	// The keys are held against the right end of the strip, which opens at the first
	// column inside the border of the pane.
	counted := model.writeKeyLine(keys, theme.Header, row,
		model.editorLeft+width-measureKeyLine(keys))
	return model.styles.RenderStrip(
		theme.Header, width, strings.Join(written, ""), counted)
}

// renderViewStrip draws the row of views under the result title, with the key that steps
// through them.
func (model *Model) renderViewStrip(
	views []app.ResultView, drawn app.ResultView, width int, indented bool, row int,
) string {
	theme := model.styles.Theme
	// Each named chip carries the glyph of its view before its name, so the plan is made
	// against a name that is as wide as the chip will be. A set that draws no glyph leaves
	// the chip the width of its name.
	names := make([]string, 0, len(views))
	activeIndex := 0
	for at, view := range views {
		names = append(names, string(view)+model.describeViewGlyph(view))
		if view == drawn {
			activeIndex = at
		}
	}

	// The strip drops the hint first, and the names of the other views second, so every view
	// keeps a number to press in a narrow pane.
	room := width
	indent := ""
	if indented {
		indent = " └ "
		room -= present.MeasureText(indent)
	}
	plan := present.PlanViewStrip(names, activeIndex, room)

	written := []string{}
	// The strip opens at the first column inside the border of the pane, after the indent
	// that ties it to the statement above it.
	left := model.editorLeft + 1 + present.MeasureText(indent)
	chips := make([]chipHit, 0, len(views))
	if indented {
		written = append(written, paintText(theme.Muted, theme.Background, indent))
	}
	for at, view := range views {
		active := view == drawn
		ground := theme.Background
		numberInk := theme.Muted
		nameInk := theme.Text
		if active {
			ground = theme.Accent
			numberInk, nameInk = theme.OnAccent, theme.OnAccent
		}

		chip := paintText(numberInk, ground, " "+strconv.Itoa(at+1))
		if plan.Named || active {
			// The glyph of the view stands between its number and its name, so a strip
			// that dropped the names still reads as a row of views.
			if glyph := model.describeViewGlyph(view); glyph != "" {
				iconInk := model.styles.IconColor(viewIcons[view])
				if active {
					iconInk = theme.OnAccent
				}
				chip += paintText(iconInk, ground, glyph)
			}
			chip += paintText(nameInk, ground, " "+string(view))
		}
		chip += paintOn(ground, " ")
		chips = append(chips, chipHit{
			index: at, row: row, from: left, to: left + measureStyledWidth(chip) - 1,
		})
		left += measureStyledWidth(chip)
		written = append(written, chip)
	}
	model.layout.viewChips = chips

	hint := ""
	if len(views) > 1 && plan.ShowsHint {
		keys := model.sayKeys().bindPair(
			cfg.ScopeGlobal, ActionPreviousView, ActionNextView, "prev/next", " ")
		// The strip keeps one blank column at its right end, so the keys stop before it.
		hint = model.writeKeyLine(keys, theme.Background, row,
			model.editorLeft+width-measureKeyLine(keys))
	}
	// The chips carry their own padding, so the strip adds none at the left.
	strip := strings.Join(written, "")
	gap := max(width-measureStyledWidth(strip)-measureStyledWidth(hint)-1, 0)
	ground := lipgloss.NewStyle().Background(theme.Background)
	return padStyledOn(truncateStyled(
		strip+ground.Render(strings.Repeat(" ", gap))+hint+ground.Render(" "), width),
		width, theme.Background)
}

// describeViewGlyph returns the glyph a view carries on its chip, with the blank before it,
// and nothing where the set draws no glyph for it.
func (model *Model) describeViewGlyph(view app.ResultView) string {
	glyph := model.icons.Icon(viewIcons[view])
	if glyph == "" {
		return ""
	}
	return " " + glyph
}

// viewIcons names the glyph each view carries on its chip, so a reader picks a view by its
// mark as well as by its name.
var viewIcons = map[app.ResultView]cfg.IconKind{
	app.ViewTree:        cfg.IconFolder,
	app.ViewData:        cfg.IconTable,
	app.ViewFields:      cfg.IconColumn,
	app.ViewColumns:     cfg.IconColumn,
	app.ViewIndexes:     cfg.IconIndex,
	app.ViewConstraints: cfg.IconPrimaryKey,
	app.ViewDDL:         cfg.IconQuery,
	app.ViewPlan:        cfg.IconPlan,
	app.ViewStatistics:  cfg.IconSequence,
}

// describeBanner writes the rewrites the grid laid on, with or without a request to the server.
func (model *Model) describeBanner(connection *app.Connection, tab *app.Tab) string {
	shape := model.buildGridShape(connection, tab)
	names := make([]string, 0, len(shape.Columns))
	for _, column := range shape.Columns {
		names = append(names, column.Name)
	}

	parts := []string{}
	if written := tab.RewriteSummary(connection.Session); written != "" {
		parts = append(parts, written)
	}
	if written := present.DescribeScreenFilter(tab.Screen, names); written != "" {
		parts = append(parts, written)
	}
	return strings.Join(parts, " · ")
}

// DrawsResultRows is true for a view that draws the rows the read answered, rather than
// something about them. Those views carry the strip that names the sort and the filter, and
// a read run again lands back on the one the reader was working in.
func DrawsResultRows(drawn app.ResultView) bool {
	return drawn == app.ViewData || drawn == app.ViewTree
}

// resolveRewriteScope returns the scope the keys of the banner are bound in, which is the
// scope of the view the banner stands over.
func resolveRewriteScope(drawn app.ResultView) cfg.KeyScope {
	if drawn == app.ViewTree {
		return cfg.ScopeDocument
	}
	return cfg.ScopeGrid
}

// renderBanner draws the strip that names the sort and the filter, and the keys that remove one.
func (model *Model) renderBanner(
	tab *app.Tab, drawn app.ResultView, banner string, width, row, left int,
) string {
	theme := model.styles.Theme
	ink := model.styles.InkOn(theme.Warning)
	style := lipgloss.NewStyle().Foreground(ink).Background(theme.Warning)

	// The keys are the ones of the view the banner stands over, so the strip names the
	// chord that works where the reader is and a press on it runs the right action.
	scope := resolveRewriteScope(drawn)
	clear := model.registry.FormatActionChordCompact(scope, ActionClearRewrites) + " clear"
	keys, dropped := clear, ""
	if len(tab.Filter) > 1 {
		dropped = model.registry.FormatActionChordCompact(scope, ActionPopFilter) +
			" drop the last"
		keys = dropped + " · " + clear
	}

	// The keys stand against the right end of the strip, so each one is recorded from
	// there and a press on the word removes what it names.
	at := left + width - 1 - present.MeasureText(keys)
	if dropped != "" {
		model.recordButton(row, at, present.MeasureText(dropped), scope, ActionPopFilter)
		at += present.MeasureText(dropped) + 3
	}
	model.recordButton(row, at, present.MeasureText(clear), scope, ActionClearRewrites)

	return model.styles.RenderStrip(
		theme.Warning, width,
		style.Render(model.icons.Prefix(cfg.IconBanner)+banner), style.Render(keys))
}

// renderGrid draws the header, the rows and the cursor of the result grid.
func (model *Model) renderGrid(
	connection *app.Connection, tab *app.Tab, shape GridShape, width, height int,
) []string {
	theme := model.styles.Theme
	state := tab.Results.State()

	// A read the server can be told to stop names the key that stops it, and one it cannot
	// names none.
	stop := model.sayKeys()
	if AnswersFor(connection.Session.Capabilities(), NeedsCancelsRunning) {
		stop.bindCompact(cfg.ScopeGlobal, ActionCancelQuery, "stop")
	}

	switch state.Kind {
	case app.QueryIdle:
		return model.renderEmptyState(width, height, "no result yet", []Hint{
			{
				Key:   model.registry.FormatActionChords(cfg.ScopeTree, ActionOpenNode),
				Label: "open the table the cursor is on",
			},
			{
				Key:   model.registry.FormatActionChords(cfg.ScopeGlobal, ActionRunAtCursor),
				Label: "run the statement in the editor",
			},
		})
	case app.QueryRunning:
		return model.renderWaitingBlock(waitBlock{
			label: "running", since: model.findRunStart(tab), stop: stop,
			top: model.layout.gridHeaderRow, left: model.editorLeft + 1,
		}, width, height)
	case app.QueryFailed:
		return model.wrapMessage(state.Message, width, height, theme.Error)
	}
	if len(shape.Columns) == 0 {
		return model.renderEmptyState(width, height,
			"the statement returned no rows", []Hint{{Label: model.describeOutcome(state)}})
	}

	// The gutter numbers each row of the result, so the footer can name where the cursor is.
	total := len(shape.Rows)
	gutterWidth := present.MeasureText(strconv.Itoa(total)) + gutterGap
	if total == 0 {
		gutterWidth = 0
	}

	rows := max(height-1, 1)
	tab.GridRow = clamp(tab.GridRow, len(shape.Text))
	tab.GridColumn = clamp(tab.GridColumn, len(shape.Columns))
	tab.GridRowOffset = scrollFrom(
		tab.GridRow, tab.GridRowOffset, rows, len(shape.Text), tab.GridRolled)

	plan := model.followColumnCursor(tab, shape, width-gutterWidth)
	visible := model.resolveVisibleColumns(tab, plan, len(shape.Columns))
	hidden := countHiddenColumns(tab, plan, len(shape.Columns))

	lines := []string{model.renderGridHeader(tab, shape, visible, hidden, gutterWidth, width)}
	for at := tab.GridRowOffset; at < len(shape.Text) && len(lines) <= rows; at++ {
		lines = append(lines, model.renderGridRow(
			tab, shape, visible, at, gutterWidth, width))
	}
	model.recordGridColumns(shape, visible, gutterWidth, width, len(lines)-1, tab.GridRowOffset)
	for len(lines) < height {
		lines = append(lines, "")
	}
	// The bar stands over the last cell of each row, so a result taller than the pane
	// says how much of it is on screen. The header takes the first row, so the track of the
	// bar starts under it.
	model.drawScrollTrack(lines[1:], scrollView{
		offset: tab.GridRowOffset, rows: rows, total: len(shape.Text),
		// A drag that reaches the foot of a result the server has more of asks for the
		// next page, as a roll of the wheel to the same place does.
		moveTo: func(offset int) tea.Cmd {
			tab.GridRowOffset, tab.GridRolled = offset, true
			return model.approachDrawnGridEnd(connection, tab)
		},
	}, model.layout.gridRows.top, model.editorLeft+1, width, theme.Panel)
	return lines
}

// recordGridColumns keeps where each column was drawn, so a press lands on the cell it
// looks like. The pane starts after the object tree, and the gutter holds the row numbers.
func (model *Model) recordGridColumns(
	shape GridShape, visible []int, gutterWidth, width, drawn, offset int,
) {
	// The strip starts at the first column inside the border of the pane, every row opens
	// with a blank, and the gutter of row numbers follows it.
	left := model.editorLeft + 2 + gutterWidth
	columns := make([]columnHit, 0, len(visible))
	edges := make([]columnHit, 0, len(visible))
	for _, index := range visible {
		room := shape.Widths[index] + columnGap
		columns = append(columns, columnHit{index: index, from: left, to: left + room - 1})
		// The gap after a column is the border between it and the next one, and a drag on
		// it sets how wide the column is.
		edges = append(edges, columnHit{
			index: index, from: left + room - columnGap, to: left + room - 1,
		})
		left += room
	}
	model.layout.gridColumns = columns
	model.layout.columnEdges = edges
	model.layout.gridRows.count = drawn
	model.layout.gridRows.offset = offset
	model.layout.gridRows.from = model.editorLeft + 1
	model.layout.gridRows.to = model.editorLeft + width
}

// followColumnCursor plans the columns that fit, and moves the window until the column under
// the cursor is one of them. A window that keeps the cursor outside would leave the reader
// with a cursor they cannot see.
func (model *Model) followColumnCursor(
	tab *app.Tab, shape GridShape, available int,
) present.ColumnPlan {
	planColumns := func() present.ColumnPlan {
		return present.PlanVisibleColumns(present.ColumnPlanInput{
			Widths: shape.Widths, Frozen: tab.Frozen, ColumnOffset: tab.GridColumnOffset,
			Available: available, Gap: columnGap,
		})
	}

	plan := planColumns()
	if tab.Frozen[tab.GridColumn] {
		return plan
	}
	// A column wider than the pane can leave the cursor outside the window it was moved to,
	// so the window is moved again, and never more times than there are columns.
	for step := 0; step <= len(shape.Widths); step++ {
		switch {
		case tab.GridColumn < plan.WindowStart:
			tab.GridColumnOffset = tab.GridColumn
		case tab.GridColumn >= plan.WindowStart+plan.VisibleCount:
			tab.GridColumnOffset = tab.GridColumn - plan.VisibleCount + 1
		default:
			return plan
		}
		plan = planColumns()
	}
	return plan
}

// hiddenColumns is how many columns stand outside the window at each edge, which the header
// counts for the reader.
type hiddenColumns struct {
	left  int
	right int
}

// countHiddenColumns counts the columns the window leaves out. A frozen column is always
// drawn, so it is never counted.
func countHiddenColumns(tab *app.Tab, plan present.ColumnPlan, count int) hiddenColumns {
	countBetween := func(from, to int) int {
		hidden := 0
		for index := max(0, from); index < to; index++ {
			if !tab.Frozen[index] {
				hidden++
			}
		}
		return hidden
	}
	return hiddenColumns{
		left:  countBetween(0, plan.WindowStart),
		right: countBetween(plan.WindowStart+plan.VisibleCount, count),
	}
}

func (model *Model) resolveVisibleColumns(
	tab *app.Tab, plan present.ColumnPlan, count int,
) []int {
	windowEnd := plan.WindowStart + plan.VisibleCount
	visible := []int{}
	for _, index := range buildColumnOrder(tab, count) {
		if tab.Frozen[index] || (index >= plan.WindowStart && index < windowEnd) {
			visible = append(visible, index)
		}
	}
	return visible
}

// renderGridHeader draws the names of the columns, with the sort mark on the ones
// sorted by. The name of the column under the cursor is drawn on the accent, and a
// frozen column takes the second accent, so both are told apart at a glance.
func (model *Model) renderGridHeader(
	tab *app.Tab, shape GridShape, visible []int, hidden hiddenColumns,
	gutterWidth, width int,
) string {
	theme := model.styles.Theme
	ground := theme.Header
	focused := tab.Focus == app.PaneResult

	// Every name lays its own ground, so the header needs no second pass to ground it, and
	// the cells it covers are counted as they are written rather than measured again.
	written := strings.Builder{}
	written.Grow(width + (len(visible)+1)*cellEscapeBytes)
	used := 1 + gutterWidth
	writeBlanksOn(&written, ground, used)

	for _, index := range visible {
		label := shape.Columns[index].Name
		if index < len(shape.Labels) {
			label = shape.Labels[index]
		}

		highlighted := index == tab.GridColumn && focused
		ink, cell := theme.Accent, ground
		switch {
		case highlighted:
			ink, cell = theme.OnAccent, theme.Accent
		case tab.Frozen[index]:
			ink = theme.AccentAlt
		}

		// The gap to the next column carries the ground of this one, so the highlight
		// over a name is as wide as the highlight over its values.
		opening := resolveBoldOpening(ink, cell)
		written.WriteString(opening)
		written.WriteString(present.FitText(label, shape.Widths[index]))
		writeBlanks(&written, columnGap)
		written.WriteString(resetSequence)
		used += shape.Widths[index] + columnGap
	}

	row := written.String()
	if used > width {
		row = padStyledOn(truncateStyled(row, width), width, ground)
	} else if used < width {
		row += paintBlanks(ground, width-used)
	}
	return model.paintHiddenColumnMarks(row, hidden, gutterWidth, ground)
}

// paintHiddenColumnMarks writes how many columns stand outside the window at each edge of the
// header. A mark is laid over the row rather than set inside it, because a mark in the flow
// would take cells from the columns and move every column with them.
func (model *Model) paintHiddenColumnMarks(
	line string, hidden hiddenColumns, gutterWidth int, ground color.Color,
) string {
	if hidden.left > 0 {
		// The mark stands over the gutter, which the header leaves blank. A count wider than
		// the gutter is dropped, so it does not cover the first name.
		counted := model.icons.Icon(cfg.IconStepBack) + strconv.Itoa(hidden.left)
		if present.MeasureText(counted) > gutterWidth {
			counted = model.icons.Icon(cfg.IconStepBack)
		}
		line = model.styles.paintOverStart(line, counted, ground)
	}
	if hidden.right > 0 {
		line = model.styles.paintOverEnd(line,
			strconv.Itoa(hidden.right)+model.icons.Icon(cfg.IconStepOn), ground)
	}
	return line
}

// waitBlock says what a pane draws while it waits: the label of the wait, when it began, and
// the key that stops it where the wait can be stopped.
type waitBlock struct {
	label string
	since time.Time
	// The key that stops the wait, and where the block lands on the screen, so a press on
	// the word runs it.
	stop      *KeyLine
	top, left int
}

// renderWaitingBlock draws the wheel of a wait in the middle of the pane, where an empty pane
// draws what it waits for, so a wait reads the same way whichever view is waiting. A wait that
// can be stopped names the key beside the wheel, and a press on the word stops it.
func (model *Model) renderWaitingBlock(wait waitBlock, width, height int) []string {
	written := model.renderThinkingLine(
		wait.label, wait.since, model.styles.Theme.Panel)
	said := ""
	if !wait.stop.isEmpty() {
		said = "  " + wait.stop.buildText()
	}

	wheel := measureStyledWidth(written)
	left := max((width-wheel-present.MeasureText(said))/2, 0)
	if said != "" {
		written += paintOn(model.styles.Theme.Panel, "  ") +
			model.writeKeyLine(wait.stop, model.styles.Theme.Panel,
				wait.top+halfRoundedUp(height-1), wait.left+left+wheel+2)
	}

	return centerRows([]string{strings.Repeat(" ", left) + written}, height)
}

// findRunStart returns when the statement on screen began to run.
func (model *Model) findRunStart(tab *app.Tab) time.Time {
	if active := tab.Results.Active(); active != nil {
		return active.StartedAt
	}
	return time.Time{}
}

// renderGridRow draws one row of the grid: the gutter, then each cell of the window.
func (model *Model) renderGridRow(
	tab *app.Tab, shape GridShape, visible []int, at, gutterWidth, width int,
) string {
	theme := model.styles.Theme
	rowIndex := shape.RowIndexes[at]
	focused := tab.Focus == app.PaneResult
	onCursor := at == tab.GridRow
	deleted := tab.Pending.DeletedRows[rowIndex]

	// The row under the cursor and a row staged for removal both stand out from the
	// zebra stripe the rest of the grid carries.
	ground := theme.Panel
	if at%2 == 1 {
		ground = theme.Zebra
	}
	if deleted || (onCursor && focused) {
		ground = theme.Header
	}

	gutterInk := theme.Muted
	if onCursor && focused {
		gutterInk = theme.Accent
	}
	// The row grows a cell at a time, so it is built in one buffer rather than joined
	// again for every column. Every cell lays its own ground, so the row needs no second
	// pass to ground it, and the cells it covers are counted as they are written rather
	// than measured again at the end.
	written := strings.Builder{}
	written.Grow(width + (len(visible)+1)*cellEscapeBytes)
	used := 1 + gutterWidth
	writeTextOn(&written, gutterInk, ground,
		" "+buildGutterText(strconv.Itoa(rowIndex+1), gutterWidth))

	for _, index := range visible {
		cell := ""
		if index < len(shape.Text[at]) {
			cell = shape.Text[at][index]
		}

		// A staged edit is drawn as the value it will write, so the grid shows what
		// will run. Naming a cell to look one up costs more than the whole of the rest
		// of the cell, so a grid with nothing staged asks nothing.
		staged := false
		if len(tab.Pending.Edits) > 0 {
			if edit, held := tab.Pending.Edits[core.BuildEditKey(rowIndex, index)]; held {
				cell = core.DescribeCellValue(edit.Value)
				if cell == "" {
					cell = "''"
				}
				staged = true
			}
		}

		// Every cell sets its own ground, otherwise the cursor highlight bleeds across
		// the rest of the row. The column of the cursor carries the ground of the row of
		// the cursor, so the two cross on the cell and a wide grid says where the cursor
		// is without a search.
		onColumn := index == tab.GridColumn && focused
		highlighted := onCursor && onColumn
		cellGround := ground
		switch {
		case highlighted:
			cellGround = theme.BorderFocus
		case onColumn && !deleted:
			cellGround = theme.Header
		}

		ink := theme.Text
		switch {
		case highlighted:
			ink = theme.OnAccent
		case deleted:
			ink = theme.Error
		case staged:
			ink = theme.Success
		case cell == present.NullDisplay:
			ink = theme.Muted
		}

		writeTextOn(&written, ink, cellGround, present.FitText(cell, shape.Widths[index]))
		writeBlanksOn(&written, cellGround, columnGap)
		used += shape.Widths[index] + columnGap
	}
	// A plan that keeps the columns inside the pane leaves nothing to cut, so the cut is
	// kept for the row that overflows it.
	if used > width {
		return padStyledOn(truncateStyled(written.String(), width), width, ground)
	}
	writeBlanksOn(&written, ground, width-used)
	return written.String()
}

// buildGutterText writes a row number into the gutter, which holds it against its right
// edge and keeps one cell of air before the first column.
func buildGutterText(label string, gutterWidth int) string {
	if gutterWidth == 0 {
		return ""
	}
	room := max(gutterWidth-1-present.MeasureText(label), 0)
	return strings.Repeat(" ", room) + label + " "
}

// describeOutcome writes what a statement with no result set did.
func (model *Model) describeOutcome(state app.QueryState) string {
	if state.Result.HasAffected {
		return present.FormatRowCount(state.Result.Affected) + " affected · " +
			present.FormatDuration(state.Result.Elapsed)
	}
	return present.FormatDuration(state.Result.Elapsed)
}

// renderEmptyState draws a pane with nothing in it: what it waits for, and the key for it.
func (model *Model) renderEmptyState(width, height int, title string, keys []Hint) []string {
	// The block stands in the middle of the pane. A line in the top corner of a tall
	// pane looks like a leftover, not like a hint.
	block := []string{model.styles.Muted().Render(present.TruncateText(title, width-2))}
	if len(keys) > 0 {
		// Two blank rows stand between the title and the keys.
		block = append(block, "", "")
		for _, hint := range keys {
			block = append(block,
				model.styles.Accent().Render(present.PadText(hint.Key, emptyStateKeyWidth))+
					model.styles.Faint().Render(hint.Label))
		}
	}

	// The title stands on its own, and the keys line up as one block under it, so both
	// sit in the middle of the pane. A title is a line of text, which keeps the odd cell
	// on its right; the keys are a block, which keeps it on its left.
	widest := 0
	for _, line := range block[1:] {
		if measured := measureStyledWidth(line); measured > widest {
			widest = measured
		}
	}
	centre := func(line string, left int) string {
		if left < 0 {
			left = 0
		}
		return strings.Repeat(" ", left) + line
	}

	drawn := make([]string, 0, len(block))
	drawn = append(drawn, centre(block[0], (width-measureStyledWidth(block[0]))/2))
	for _, line := range block[1:] {
		drawn = append(drawn, centre(line, halfRoundedUp(width-widest)))
	}
	return centerRows(drawn, height)
}

// emptyStateKeyWidth is the key column of an empty pane, wide enough for the longest
// chord these panes name.
const emptyStateKeyWidth = 10

// wrapMessage draws a message over several rows, so a long error is read whole. It opens
// with the mark a fault carries in the editor, and the rows under the first line up with it,
// so a failure reads the same way in both panes.
func (model *Model) wrapMessage(
	message string, width, height int, ink color.Color,
) []string {
	ground := model.styles.Theme.Panel
	style := lipgloss.NewStyle().Foreground(ink).Background(ground)
	written := model.writeProblemSign()
	sign := paintText(ink, ground, written)
	indent := paintBlanks(ground, present.MeasureText(written))
	wrapped := lipgloss.NewStyle().
		Width(max(width-2-present.MeasureText(written), 1)).Render(message)

	lines := []string{}
	for line := range strings.SplitSeq(wrapped, "\n") {
		if len(lines) >= height {
			break
		}
		opening := indent
		if len(lines) == 0 {
			opening = sign
		}
		lines = append(lines, " "+opening+style.Render(line))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

// resolveViewContent returns what the view drawn shows. The statistics of a statement and the
// fields of a result are read from the result itself, so they are built for the frame that
// draws them rather than asked for.
func (model *Model) resolveViewContent(tab *app.Tab, drawn app.ResultView) app.PaneContent {
	switch drawn {
	case app.ViewTree:
		return app.PaneContent{Kind: app.DataTree}
	case app.ViewStatistics:
		return app.PaneContent{Kind: app.DataStatistics, Statistics: buildStatistics(tab)}
	case app.ViewFields:
		active := tab.Results.Active()
		if active == nil || active.State.Kind != app.QuerySucceeded {
			return app.PaneContent{Kind: app.DataIdle, Reason: needsResult}
		}
		return app.PaneContent{
			Kind: app.DataResultColumns, ResultColumns: active.State.Result.Columns,
		}
	}
	return tab.ViewData
}

// renderDetailView draws the view that is not the grid: the columns, the indexes, the
// constraints, the DDL, the statistics or the plan.
func (model *Model) renderDetailView(
	connection *app.Connection, tab *app.Tab, drawn app.ResultView, width, height, top int,
) []string {
	theme := model.styles.Theme
	content := model.resolveViewContent(tab, drawn)

	switch content.Kind {
	case app.DataTree:
		return model.renderDocumentTree(connection, tab, width, height)
	case app.DataLoading:
		return model.renderWaitingBlock(waitBlock{
			label: "reading", since: content.StartedAt,
		}, width, height)
	case app.DataIdle:
		return model.renderEmptyState(width, height, content.Reason, nil)
	case app.DataFailed:
		return model.wrapMessage(content.Message, width, height, theme.Error)
	case app.DataDDL:
		return model.renderLines(tab, content.Lines, width, height, true)
	case app.DataStatistics:
		return model.renderStatistics(tab, content.Statistics, width, height)
	case app.DataPlan:
		return model.renderPlan(tab, content.Plan, width, height, top)
	case app.DataColumns:
		return model.renderTable(tab, model.buildColumnRows(content.Columns), width, height)
	case app.DataResultColumns:
		return model.renderTable(tab,
			model.buildResultColumnRows(content.ResultColumns), width, height)
	case app.DataIndexes:
		return model.renderTable(tab, model.buildIndexRows(content.Indexes), width, height)
	case app.DataConstraints:
		return model.renderTable(tab,
			model.buildConstraintRows(content.Constraints), width, height)
	}
	return model.renderEmptyState(width, height, "nothing to show", nil)
}

// detailTable is the header and the rows of a read-only table.
type detailTable struct {
	Headers []string
	Rows    [][]string
	// True for a row the relation is keyed by, whose name carries the second accent.
	Accented []bool
}

// detailGap is the gap between one column of a detail table and the next.
const detailGap = 2

// buildColumnRows writes the columns of a relation as a table.
func (model *Model) buildColumnRows(columns []db.ColumnDetail) detailTable {
	table := detailTable{Headers: []string{"column", "type", "null", "default"}}
	for _, column := range columns {
		nullable := "no"
		if column.Nullable {
			nullable = "yes"
		}
		table.Rows = append(table.Rows, []string{
			markKeyRow(column.IsPrimaryKey) + column.Name, column.DataType, nullable,
			column.DefaultValue,
		})
		table.Accented = append(table.Accented, column.IsPrimaryKey)
	}
	return table
}

// keyRowMark stands before the name of a row the server keys a relation by.
const keyRowMark = "◆ "

// markKeyRow writes the mark before a name, and the room it takes before every other
// name, so the names of a table line up.
func markKeyRow(keyed bool) string {
	if keyed {
		return keyRowMark
	}
	return "  "
}

// buildResultColumnRows writes the columns of a result as a table.
func (model *Model) buildResultColumnRows(columns []query.ResultColumn) detailTable {
	table := detailTable{Headers: []string{"#", "column", "type"}}
	for at, column := range columns {
		table.Rows = append(table.Rows, []string{
			strconv.Itoa(at + 1), column.Name, column.DataType,
		})
	}
	return table
}

// buildIndexRows writes the indexes of a relation as a table.
func (model *Model) buildIndexRows(indexes []db.IndexDetail) detailTable {
	table := detailTable{Headers: []string{"index", "unique", "definition"}}
	for _, index := range indexes {
		unique := "no"
		if index.IsUnique {
			unique = "yes"
		}
		table.Rows = append(table.Rows, []string{
			markKeyRow(index.IsPrimary) + index.Name, unique, index.Definition,
		})
		table.Accented = append(table.Accented, index.IsPrimary)
	}
	return table
}

// buildConstraintRows writes the constraints of a relation as a table.
func (model *Model) buildConstraintRows(constraints []db.ConstraintDetail) detailTable {
	table := detailTable{Headers: []string{"constraint", "kind", "definition"}}
	for _, constraint := range constraints {
		table.Rows = append(table.Rows, []string{
			"  " + constraint.Name, string(constraint.Kind), constraint.Definition,
		})
	}
	return table
}

// renderTable draws a read-only table, with its columns measured from their content.
func (model *Model) renderTable(
	tab *app.Tab, table detailTable, width, height int,
) []string {
	if len(table.Rows) == 0 {
		return model.renderEmptyState(width, height, "there are none", nil)
	}

	theme := model.styles.Theme
	widths := present.PlanDetailColumns(table.Headers, table.Rows, width-2, detailGap)
	rows := max(height-1, 1)
	tab.DetailOffset = clampOffset(tab.DetailOffset, rows, len(table.Rows))

	gap := strings.Repeat(" ", detailGap)
	var header strings.Builder
	header.WriteString(paintOn(theme.Header, " "))
	for at, name := range table.Headers {
		header.WriteString(paintText(theme.Accent, theme.Header, present.FitText(name, widths[at])+gap))
	}
	lines := []string{padStyledOn(header.String(), width, theme.Header)}

	for at := tab.DetailOffset; at < len(table.Rows) && len(lines) <= rows; at++ {
		ground := theme.Panel
		if at%2 == 1 {
			ground = theme.Zebra
		}
		// The name of a row the relation is keyed by carries the second accent.
		nameInk := theme.Text
		if at < len(table.Accented) && table.Accented[at] {
			nameInk = theme.AccentAlt
		}

		var written strings.Builder
		written.WriteString(paintOn(ground, " "))
		for column, cell := range table.Rows[at] {
			if column >= len(widths) {
				break
			}
			ink := theme.Text
			if column == 0 {
				ink = nameInk
			}
			written.WriteString(paintText(ink, ground, present.FitText(cell, widths[column])+gap))
		}
		lines = append(lines, padStyledOn(written.String(), width, ground))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

// statisticLabelWidth is the column the label of a statistic takes, so every value lines up.
const statisticLabelWidth = 14

// renderStatistics draws what a statement with no result set did: the label of each line in
// the muted ink, and the line that reports what changed marked.
func (model *Model) renderStatistics(
	tab *app.Tab, held []app.Statistic, width, height int,
) []string {
	theme := model.styles.Theme
	if len(held) == 0 {
		return model.renderEmptyState(width, height, "there is nothing to show", nil)
	}
	tab.DetailOffset = clampOffset(tab.DetailOffset, height, len(held))

	room := max(width-1-statisticLabelWidth, 1)
	lines := make([]string, 0, height)
	for at := tab.DetailOffset; at < len(held) && len(lines) < height; at++ {
		line := held[at]
		written := present.TruncateText(line.Value, room)
		value := paintText(theme.Text, theme.Panel, written)
		if line.Leading {
			value = model.styles.Bold(theme.AccentAlt, theme.Panel).Render(written)
		}
		lines = append(lines, paintOn(theme.Panel, " ")+
			paintText(theme.Muted, theme.Panel,
				present.PadText(line.Label, statisticLabelWidth))+value)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return model.drawScrollTrack(lines, scrollView{
		offset: tab.DetailOffset, rows: height, total: len(held),
		moveTo: func(offset int) tea.Cmd { tab.DetailOffset = offset; return nil },
	}, model.layout.detailTop, model.editorLeft+1, width, theme.Panel)
}

// renderLines draws a block of text, coloured as SQL where it is a definition.
func (model *Model) renderLines(
	tab *app.Tab, held []string, width, height int, asSQL bool,
) []string {
	theme := model.styles.Theme
	if len(held) == 0 {
		return model.renderEmptyState(width, height, "there is nothing to show", nil)
	}
	tab.DetailOffset = clampOffset(tab.DetailOffset, height, len(held))

	// A definition is SQL, so it is coloured the way the editor colours a statement.
	highlights := map[int][]lineHighlight{}
	if asSQL {
		highlights = buildSQLLineHighlights(strings.Join(held, "\n"))
	}

	lines := make([]string, 0, height)
	for at := tab.DetailOffset; at < len(held) && len(lines) < height; at++ {
		lines = append(lines, paintOn(theme.Panel, " ")+
			model.renderCodeLine(codeLine{
				text: held[at], spans: highlights[at], width: width - 1}))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	// The bar stands over the last cell of each row, so a definition taller than the pane
	// says how much of it is on screen.
	return model.drawScrollTrack(lines, scrollView{
		offset: tab.DetailOffset, rows: height, total: len(held),
		moveTo: func(offset int) tea.Cmd { tab.DetailOffset = offset; return nil },
	}, model.layout.detailTop, model.editorLeft+1, width, theme.Panel)
}

// The widths of the plan tree: the label, the row counts, the time of a node alone,
// and the line under a node that says what it read.
const (
	planLabelWidth  = 46
	planRowsWidth   = 28
	planTimeWidth   = 10
	planDetailWidth = 100
	// planBarWidth is the bar that draws the share of the run one node took, and
	// wholeShare is the whole that share is measured against.
	planBarWidth = 8
	wholeShare   = 1.0
)

// renderPlan draws the plan as a tree, or as the server sent it. The strip over it
// says what the plan cost and which keys read it another way.
func (model *Model) renderPlan(
	tab *app.Tab, plan query.QueryPlan, width, height int, row int,
) []string {
	theme := model.styles.Theme
	said := "the raw plan"
	if tab.RawPlan {
		said = "back to the tree"
	}
	keys := model.sayKeys().
		bindCompact(cfg.ScopePlan, ActionToggleRawPlan, said).
		bindCompact(cfg.ScopePlan, ActionCopyPlan, "copy").
		bindCompact(cfg.ScopePlan, ActionAiCheckPlan, "ask ai")
	// The strip opens at the first column inside the border of the pane, and its keys are
	// held against the right end of it.
	left := model.editorLeft + 1
	written := model.writeKeyLine(keys, theme.Header, row, left+width-1-measureKeyLine(keys))
	lines := []string{model.styles.RenderStrip(theme.Header, width,
		model.buildPlanCostLine(plan, row, left+1), written)}

	body := height - 1
	if tab.RawPlan {
		return append(lines,
			model.renderLines(tab, strings.Split(plan.Raw, "\n"), width, body, false)...)
	}

	// A node with a detail takes a second row, so the rows are built first and then
	// the window is taken from them.
	drawn := []string{}
	for at, row := range result.FlattenPlan(plan) {
		drawn = append(drawn, model.renderPlanRow(plan, row, at, width))
		if row.Node.Detail != "" {
			drawn = append(drawn, model.renderPlanDetail(row, at, width))
		}
	}

	tab.DetailOffset = clampOffset(tab.DetailOffset, body, len(drawn))
	for at := tab.DetailOffset; at < len(drawn) && len(lines) < height; at++ {
		lines = append(lines, drawn[at])
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

// buildPlanCostLine writes what the plan cost. An estimate is one key away from the real
// times, and the key is the one bound now, drawn as a key a press runs.
func (model *Model) buildPlanCostLine(plan query.QueryPlan, row, left int) string {
	theme := model.styles.Theme
	cost := paintText(theme.AccentAlt, theme.Header, result.DescribePlanCost(plan))
	if plan.Analyzed || !plan.Measurable {
		return cost
	}
	keys := model.sayKeys().
		bindCompact(cfg.ScopeGlobal, ActionExplainAnalyze, "for actual times")
	if keys.isEmpty() {
		return cost
	}
	measured := present.MeasureText(result.DescribePlanCost(plan) + hintSeparator)
	return cost + paintText(theme.Faint, theme.Header, hintSeparator) +
		model.writeKeyLine(keys, theme.Header, row, left+measured)
}

// renderPlanRow draws one node of the plan: its label, the rows it answered, its own
// time, and the share of the run it took.
func (model *Model) renderPlanRow(
	plan query.QueryPlan, row result.PlanRow, at, width int,
) string {
	theme := model.styles.Theme
	ground := theme.Panel
	if at%2 == 1 {
		ground = theme.Zebra
	}

	labelInk := theme.Text
	switch {
	case row.Slowest:
		labelInk = theme.Error
	case row.Depth == 0:
		labelInk = theme.AccentAlt
	}

	indent := strings.Repeat("  ", row.Depth)
	branch := "└ "
	if row.Depth == 0 {
		branch = ""
	}
	label := paintText(labelInk, ground, present.TruncateText(indent+branch+row.Node.Label, planLabelWidth))

	countInk := theme.Muted
	if row.Misestimated {
		countInk = theme.Warning
	}
	timeInk := theme.Muted
	if row.Slowest {
		timeInk = theme.Error
	}
	right := paintText(countInk, ground, present.FitText(describePlanRowCounts(row.Node), planRowsWidth)) +
		paintText(timeInk, ground, present.PadText(describeSelfMs(row.Node), planTimeWidth))
	if plan.Analyzed {
		barInk := theme.Accent
		if row.Slowest {
			barInk = theme.Error
		}
		right += paintText(barInk, ground,
			present.BuildMeter(row.Share, wholeShare, planBarWidth))
	}

	return model.styles.RenderStrip(ground, width, label, right)
}

// renderPlanDetail draws the line under a node that says what it read.
func (model *Model) renderPlanDetail(row result.PlanRow, at, width int) string {
	ground := model.styles.Theme.Panel
	if at%2 == 1 {
		ground = model.styles.Theme.Zebra
	}
	written := strings.Repeat("  ", row.Depth) + "  " + row.Node.Detail
	return padStyledOn(paintText(model.styles.Theme.Muted, ground, " "+present.TruncateText(written, planDetailWidth)),
		width, ground)
}

// describePlanRowCounts writes the rows a node answered against the rows it expected. A
// dash stands where the server gave no estimate, which is not an estimate of zero.
func describePlanRowCounts(node query.PlanNode) string {
	estimated := "—"
	if node.HasEstimatedRows {
		estimated = present.FormatCount(int64(node.EstimatedRows + 0.5))
	}
	if !node.HasActualRows {
		return "est " + estimated
	}
	return present.FormatCount(int64(node.ActualRows)) + " of est " + estimated
}

// describeSelfMs writes the time of one node alone, and nothing where the server
// measured none, which is not the same as no time.
func describeSelfMs(node query.PlanNode) string {
	if !node.HasSelfMs {
		return ""
	}
	return strconv.FormatFloat(node.SelfMs, 'f', 1, 64) + " ms"
}
