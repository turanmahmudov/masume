package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// The width the object tree asks for, which a narrow terminal cuts down.
const sidebarWidth = 36

// narrowestPaneWidth is the floor the editor and the result keep on a narrow terminal, below
// which a row of the grid holds nothing that can be read.
const narrowestPaneWidth = 20

// The rows the tab row takes, and the least the editor and the result each keep.
const (
	tabRowHeight  = 1
	minPaneHeight = 3
	// editorRows is the height the editor keeps while the result is on screen.
	editorRows = 8
)

// renderWorkspace draws the tab row, the tree, the editor and the result.
func (model *Model) renderWorkspace(height int) []string {
	connection := model.Active()
	if connection == nil {
		return nil
	}
	tab := connection.Active()

	treeWidth := 0
	if connection.SidebarVisible {
		treeWidth = present.PlanSidebarWidth(model.width, sidebarWidth)
	}
	paneWidth := max(model.width-treeWidth, narrowestPaneWidth)

	// A prompt that names the tab opens a field under the panes, in place of two of their
	// rows.
	naming := []string{}
	if prompt, asking := findPromptBar(connection, app.PromptTabName); asking {
		naming = model.renderPromptBar(prompt, model.width)
	}

	paneHeight := max(height-tabRowHeight-len(naming), minPaneHeight)

	// The editor and the result share the height. The editor keeps a third of it, and takes
	// the whole pane while the result is hidden.
	editorHeight, resultHeight := model.planPaneHeights(connection, tab, paneHeight)

	// Where each part is drawn, so a press of a button can be read as a press on a row,
	// a cell or a tab. The tab row is the first row under the title bar.
	model.layout = frameLayout{tabRow: tabRowIndex, treeFrom: 0, treeTo: treeWidth - 1}
	if editorHeight > 0 {
		model.layout.editorTop, model.layout.editorRows = firstPaneRow, editorHeight
	}
	if resultHeight > 0 {
		model.layout.resultTop = firstPaneRow + editorHeight
		model.layout.resultRows = resultHeight
	}

	// The inside of each pane is a block of its own, so a drag that began in one never
	// reaches the border of it or the rows of another.
	if treeWidth > 0 {
		model.layout.selectionBlocks = append(model.layout.selectionBlocks, blockRect{
			fromX: 1, toX: treeWidth - 2,
			fromY: firstPaneRow + 1, toY: firstPaneRow + paneHeight - 2,
		})
	}
	if editorHeight > 0 {
		model.layout.selectionBlocks = append(model.layout.selectionBlocks, blockRect{
			fromX: treeWidth + 1, toX: treeWidth + paneWidth - 2,
			fromY: firstPaneRow + 1, toY: firstPaneRow + editorHeight - 2,
		})
	}
	if resultHeight > 0 {
		model.layout.selectionBlocks = append(model.layout.selectionBlocks, blockRect{
			fromX: treeWidth + 1, toX: treeWidth + paneWidth - 2,
			fromY: firstPaneRow + editorHeight + 1,
			toY:   firstPaneRow + editorHeight + resultHeight - 2,
		})
	}

	// The pane opens where the tree ends, which the editor needs before it draws: the cell
	// of the caret is read from it, and so is the offset a press of the pointer lands on.
	model.editorLeft = treeWidth

	right := make([]string, 0, editorHeight+resultHeight)
	if editorHeight > 0 {
		right = append(right, model.renderEditor(connection, tab, paneWidth, editorHeight)...)
	}
	if resultHeight > 0 {
		right = append(right,
			model.renderResultPane(connection, tab, paneWidth, resultHeight)...)
	}

	middle := right
	if treeWidth > 0 {
		tree := model.renderTree(connection, tab, treeWidth, paneHeight)
		middle = joinSideBySide(tree, treeWidth, right, model.styles.Theme.Background)
	}

	frame := make([]string, 0, 1+len(middle)+len(naming))
	frame = append(frame, model.renderTabRow(connection, tab))
	frame = append(frame, middle...)
	frame = append(frame, naming...)

	// An overlay is drawn last, so it paints over the panes. A prompt drawn at the foot of a
	// pane is already part of the frame.
	if connection.Overlay.IsOpen() && !drawsPromptBar(connection.Overlay) {
		return model.renderOverlayOver(connection, tab, frame, height)
	}
	if popup, left, top := model.renderCompletionPopup(tab, height); popup != "" {
		return placeOver(frame, popup, left, top, model.styles.Theme.Background)
	}
	return frame
}

// firstPaneRow is the screen row the panes start on: the title bar takes the first, and the
// tab row the second.
const firstPaneRow = 2

// planPaneHeights returns the rows the editor and the result each take.
func (model *Model) planPaneHeights(
	connection *app.Connection, tab *app.Tab, height int,
) (int, int) {
	editorVisible := tab.EditorVisible()
	if !editorVisible {
		return 0, height
	}
	if !connection.ResultVisible {
		return height, 0
	}

	// The border between the two panes is dragged to set this, and the client opens with
	// the rows the editor takes by itself.
	editor := editorRows
	if connection.EditorHeight > 0 {
		editor = connection.EditorHeight
	}
	if editor > height-minPaneHeight {
		editor = height - minPaneHeight
	}
	if editor < minPaneHeight {
		editor = minPaneHeight
	}
	if editor >= height {
		return height, 0
	}
	return editor, height - editor
}

// The widths of one tab of the row.
const (
	tabChrome = 3
	// connectionDotWidth is the cells the mark of the health of a connection covers: the
	// mark and the blank after it.
	connectionDotWidth = 2
	// tabGapCells are the padding of the chip and the gap to the next tab, after the
	// close mark.
	tabGapCells = 2

	// tabRowIndex is the row of the screen the tabs are drawn on: the title bar takes the
	// one above it.
	tabRowIndex = 1
)

// renderTabRow draws the row of open tabs above the workspace.
func (model *Model) renderTabRow(connection *app.Connection, active *app.Tab) string {
	theme := model.styles.Theme
	closable := len(connection.Tabs) > 1

	widths := make([]int, 0, len(connection.Tabs))
	for index, tab := range connection.Tabs {
		widths = append(widths, model.measureTab(tab, index, closable))
	}
	window := present.PlanVisibleTabs(
		widths, connection.ActiveIndex, model.width, connection.TabOffset)
	// The place the row was left is kept, so it moves only when it must.
	connection.TabOffset = window.Start

	written := []string{}
	// The row starts one column in, so a tab does not reach further left than the border
	// of the pane under it.
	at := 1
	// A mark with nothing behind it returns no press.
	model.layout.scrollTabsBack = columnHit{from: 1, to: 0}
	model.layout.scrollTabsOn = columnHit{from: 1, to: 0}
	if window.Start > 0 {
		mark := model.styles.Muted().Background(theme.Background).Render(
			" " + model.icons.Icon(cfg.IconStepBack) + " " +
				strconv.Itoa(window.Start) + " ")
		written = append(written, mark)
		model.layout.scrollTabsBack = columnHit{from: at, to: at + measureStyledWidth(mark) - 1}
		at += measureStyledWidth(mark)
	}

	hits := []tabHit{}
	for offset := 0; offset < window.Count; offset++ {
		index := window.Start + offset
		if index >= len(connection.Tabs) {
			break
		}
		drawn := model.renderTab(
			connection.Tabs[index], index, index == connection.ActiveIndex, closable)
		written = append(written, drawn)
		// The close mark is the two cells before the padding and the gap, so a press on
		// it closes the tab rather than opening it.
		hit := tabHit{index: index, from: at, to: at + measureStyledWidth(drawn) - 1}
		if closable {
			hit.closeTo = hit.to - tabGapCells
			hit.closeFrom = hit.closeTo - present.MeasureText(model.buildCloseMark()) + 1
		} else {
			hit.closeFrom, hit.closeTo = 1, 0
		}
		hits = append(hits, hit)
		at = hit.to + 1
	}
	model.layout.tabs = hits

	hiddenAfter := len(connection.Tabs) - window.Start - window.Count
	if hiddenAfter > 0 {
		mark := model.styles.Muted().Background(theme.Background).Render(
			" " + strconv.Itoa(hiddenAfter) + " " +
				model.icons.Icon(cfg.IconStepOn) + " ")
		written = append(written, mark)
		model.layout.scrollTabsOn = columnHit{from: at, to: at + measureStyledWidth(mark) - 1}
	} else {
		// The mark that opens a tab stands after the last one, where the mark that opens
		// one stands on the tabs of a browser.
		if glyph := model.icons.Icon(cfg.IconNewTab); glyph != "" {
			written = append(written,
				paintText(model.styles.IconColor(cfg.IconNewTab), theme.Background,
					" "+glyph+" "))
			model.recordButton(tabRowIndex, at+1, present.MeasureText(glyph),
				cfg.ScopeGlobal, ActionNewQueryTab)
		}
	}

	row := strings.Join(written, "")
	keys := model.buildTabHints(connection, active, closable)
	// The keys are shown only while every tab fits on screen.
	if window.Start == 0 && hiddenAfter == 0 &&
		model.width-measureStyledWidth(row)-1 >= measureKeyLine(keys) {
		return model.styles.RenderStrip(theme.Background, model.width, row,
			model.writeKeyLine(keys, theme.Background, tabRowIndex,
				model.width-1-measureKeyLine(keys)))
	}
	return model.styles.RenderStrip(theme.Background, model.width, row, "")
}

// measureTab returns how wide one tab of the row is drawn.
func (model *Model) measureTab(tab *app.Tab, index int, closable bool) int {
	close := ""
	if closable {
		close = model.buildCloseMark()
	}
	label := model.icons.Prefix(resolveTabIcon(tab)) + tab.Label()
	return present.MeasureText(
		strconv.Itoa(index+1)+" "+label+model.buildStagedMark(tab)+close) + tabChrome
}

// buildCloseMark returns the mark a tab and a row of the connection list draw to close
// themselves, with the blank that holds it off the name.
func (model *Model) buildCloseMark() string {
	glyph := model.icons.Icon(cfg.IconClose)
	if glyph == "" {
		return ""
	}
	return " " + glyph
}

// buildStagedMark marks a tab with staged work, as the status bar marks the active one.
func (model *Model) buildStagedMark(tab *app.Tab) string {
	if core.CountChanges(tab.Pending) > 0 {
		return " " + model.icons.Icon(cfg.IconDot)
	}
	return ""
}

// resolveTabIcon returns the glyph the tree uses for this kind, so the tab and the tree agree.
func resolveTabIcon(tab *app.Tab) cfg.IconKind {
	switch tab.Kind {
	case app.TabTable:
		return present.TableIcons[tab.Table.Kind]
	case app.TabObject:
		return cfg.IconKind(tab.Object.Kind)
	}
	return cfg.IconQuery
}

// renderTab draws one tab of the row.
func (model *Model) renderTab(tab *app.Tab, index int, active, closable bool) string {
	theme := model.styles.Theme
	ground := theme.Header
	if active {
		ground = theme.Accent
	}

	icon := resolveTabIcon(tab)
	numberInk, iconInk := theme.Muted, model.styles.IconColor(icon)
	labelInk, stagedInk := theme.Text, theme.Warning
	if active {
		numberInk, iconInk, labelInk, stagedInk =
			theme.OnAccent, theme.OnAccent, theme.OnAccent, theme.OnAccent
	}

	// The staged mark keeps its cell even when it is empty, and that cell is left out of the
	// measure, so every tab row puts a tab in the same column.
	mark := model.buildStagedMark(tab)
	if mark == "" {
		mark = " "
	}
	var written strings.Builder
	written.Grow(len(tab.Label()) + tabParts*cellEscapeBytes)
	writeTextOn(&written, numberInk, ground, " "+strconv.Itoa(index+1)+" ")
	writeTextOn(&written, iconInk, ground, model.icons.Prefix(icon))
	writeTextOn(&written, labelInk, ground, tab.Label())
	writeTextOn(&written, stagedInk, ground, mark)
	if closable {
		writeTextOn(&written, numberInk, ground, model.buildCloseMark())
	}
	writeBlanksOn(&written, ground, 1)
	writeBlanksOn(&written, theme.Background, 1)
	return written.String()
}

// tabParts is how many parts of their own colours one tab is written from.
const tabParts = 7

// buildTabHints returns the keys of the tab row, read from the registry so a rebound key
// shows here. A key the row cannot use is left out, and not named and refused.
func (model *Model) buildTabHints(
	connection *app.Connection, tab *app.Tab, many bool,
) *KeyLine {
	keys := model.sayKeys().bind(cfg.ScopeGlobal, ActionNewQueryTab, "new")
	if many {
		keys.bindPair(cfg.ScopeGlobal, ActionPreviousTab, ActionNextTab, "tab", " ").
			bind(cfg.ScopeGlobal, ActionActivateTab, "go")
	}
	if tab.Kind == app.TabQuery {
		keys.bind(cfg.ScopeGlobal, ActionNameTab, "name")
	}
	if many {
		keys.bind(cfg.ScopeGlobal, ActionCloseTab, "close")
	}
	return keys
}

// The widths of one row of the object tree.
const (
	// Wide enough for the longest type name a column row shows.
	treeDetailWidth = 12
	// The border, the padding on each side, and the gap before the detail.
	treeRowChrome = 4
)

// renderTree draws the object tree in the sidebar.
func (model *Model) renderTree(
	connection *app.Connection, tab *app.Tab, width, height int,
) []string {
	theme := model.styles.Theme
	focused := tab.Focus == app.PaneSidebar && !connection.Overlay.IsOpen()
	result := connection.BuildTree(time.Now())
	rows := result.Rows

	inner := width - 2
	body := height - 2
	if connection.Tree.Filtering {
		body--
	}
	if body < 1 {
		body = 1
	}

	// The open connections stand above the tree, so the user reads which server the
	// objects belong to and steps to another one. They take their rows before the tree is
	// held to what is left, or the last rows of the tree could not be reached.
	head := model.renderConnectionList(inner)
	body -= len(head)
	if body < 1 {
		body = 1
	}

	connection.Tree.Cursor = clamp(connection.Tree.Cursor, len(rows))
	connection.Tree.Offset = scrollFrom(connection.Tree.Cursor, connection.Tree.Offset,
		body, len(rows), connection.Tree.Rolled)

	lines := make([]string, 0, body+len(head))
	lines = append(lines, head...)
	// The first row inside the box, so a press lands on the row it looks like.
	bodyTop := firstPaneRow + 1
	model.layout.connections = rowsHit{
		top: bodyTop, count: model.countConnectionRows(),
		from: 0, to: width - 1,
	}
	// The close mark is the last two cells of the row, which starts inside the border.
	model.layout.closeConnectionFrom = inner - 1
	model.layout.closeConnectionTo = inner
	if len(rows) == 0 {
		reason := "nothing to show"
		if connection.Catalog.Loading {
			reason = spinnerFrame(model.spinnerAt) + " reading the objects…"
		}
		if connection.Catalog.Problem != "" {
			reason = connection.Catalog.Problem
		}
		lines = append(lines, " "+model.styles.Muted().Render(
			present.TruncateText(reason, inner-1)))
	}

	treeTop := bodyTop + len(lines)
	// The bar stands beside the rows of the tree, and a tree that fits in the pane gets none.
	thumb := buildScrollThumb(connection.Tree.Offset, body, len(rows))
	rowWidth := inner
	if len(thumb) > 0 {
		rowWidth--
	}
	guides := present.BuildTreeGuidesWithin(
		rows, connection.Tree.Offset, connection.Tree.Offset+body)
	for at := connection.Tree.Offset; at < len(rows) && len(lines) < body+len(head); at++ {
		line := model.renderTreeRow(rows[at], guides[at-connection.Tree.Offset],
			at == connection.Tree.Cursor, focused, rowWidth)
		if drawn := len(lines) - len(head); drawn < len(thumb) {
			line += model.styles.renderThumbCell(thumb[drawn], theme.Panel)
		}
		lines = append(lines, line)
	}
	model.layout.treeRows = rowsHit{
		top: treeTop, count: bodyTop + len(lines) - treeTop,
		offset: connection.Tree.Offset, from: 0, to: width - 1,
	}
	// The thumb of the tree is a cell of its own at the end of each row, so the track it
	// stands in is the column after the widest row.
	if len(thumb) > 0 {
		model.recordScrollbar(width-2, model.layout.treeRows.top,
			model.layout.treeRows.count, connection.Tree.Offset, len(rows),
			func(offset int) tea.Cmd {
				connection.Tree.Offset, connection.Tree.Rolled = offset, true
				return nil
			})
	}

	if connection.Tree.Filtering {
		lines = append(lines, model.renderTreeFilter(connection, inner))
	}

	// The counts on the bottom border stand for the filter, so a press on them opens it.
	// The border keeps its corner and one cell of border before the title.
	border := describeTreeBorder(
		result.Summary, strings.TrimSpace(connection.Tree.Filter),
		connection.Tree.FilterScope != "")
	// The title opens with a blank, so the key covers the words and not the blank.
	model.recordButton(firstPaneRow+height-1, treeBorderTitleColumn+1,
		min(present.MeasureText(strings.TrimSpace(border)), inner-3),
		cfg.ScopeTree, ActionFilterTree)

	return model.styles.RenderBoxRows(BoxOptions{
		Width: width, Height: height, Title: " explorer ", Focused: focused,
		BottomTitle: border, Lines: lines, Ground: theme.Panel,
	})
}

// visibleConnectionRows is how many connections the sidebar lists before it counts the
// rest on one line.
const visibleConnectionRows = 8

// renderConnectionList draws the open connections at the top of the sidebar, with the
// rule that parts them from the tree. One connection needs no list.
func (model *Model) renderConnectionList(width int) []string {
	if model.connections.count() == 0 {
		return nil
	}
	theme := model.styles.Theme

	shown := model.connections.count()
	hidden := 0
	if shown > visibleConnectionRows {
		shown = visibleConnectionRows - 1
		hidden = model.connections.count() - shown
	}

	lines := make([]string, 0, shown+2)
	for index := range shown {
		lines = append(lines, model.renderConnectionRow(
			model.connections.at(index), index, width))
	}
	if hidden > 0 {
		lines = append(lines, " "+model.styles.Muted().Render(
			present.TruncateText(fmt.Sprintf("+%d more", hidden), width-1)))
	}
	return append(lines, paintText(theme.Faint, theme.Panel, strings.Repeat("─", width)))
}

// countConnectionRows returns how many connections the sidebar lists, which is how many
// rows of it answer a press.
func (model *Model) countConnectionRows() int {
	if model.connections.count() == 0 {
		return 0
	}
	if model.connections.count() > visibleConnectionRows {
		return visibleConnectionRows - 1
	}
	return model.connections.count()
}

// resolveDotColor returns the colour of the dot beside a connection. A server that is down or
// coming back colours the dot instead of the environment, because that is the more urgent
// thing to read.
func (model *Model) resolveDotColor(connection *app.Connection) color.Color {
	switch connection.Health {
	case app.HealthDown:
		return model.styles.Theme.Danger
	case app.HealthReconnecting:
		return model.styles.Theme.Warning
	}
	return model.styles.EnvironmentColor(connection.Profile().Environment)
}

// connectionRowChrome is the marker, the dot, the padding and the closing mark.
const connectionRowChrome = 8

// renderConnectionRow draws one open connection: the marker, the dot in the colour of its
// environment, its name, and the mark that closes it.
func (model *Model) renderConnectionRow(
	connection *app.Connection, index, width int,
) string {
	theme := model.styles.Theme
	active := index == model.connections.activeIndex()

	ground := theme.Panel
	nameInk := theme.Text
	markInk := theme.Muted
	marker := "  "
	if active {
		ground, nameInk, markInk, marker = theme.Header, theme.Accent, theme.Accent, "▸ "
	}

	profile := connection.Profile()
	name := present.TruncateText(profile.Name, width-connectionRowChrome)
	// Each part covers the cells its own text covers, so the gap is counted from those
	// widths rather than measured off the escapes of the row.
	gap := max(width-present.MeasureText(marker)-2-present.MeasureText(name)-2, 0)

	var written strings.Builder
	written.Grow(width + connectionRowParts*cellEscapeBytes)
	writeTextOn(&written, markInk, ground, marker)
	writeTextOn(&written, model.resolveDotColor(connection), ground,
		present.FitText(model.icons.Icon(cfg.IconDot), connectionDotWidth))
	writeTextOn(&written, nameInk, ground, name)
	writeBlanksOn(&written, ground, gap)
	writeTextOn(&written, markInk, ground, " ×")
	return written.String()
}

// connectionRowParts is how many parts of their own colours one row of the list is written
// from.
const connectionRowParts = 5

// treeBorderTitleColumn is where the title of a border starts: the corner takes the first
// cell, and one cell of border follows it.
const treeBorderTitleColumn = 2

// describeTreeBorder writes the filter and the counts on the bottom border, so they take no row.
func describeTreeBorder(summary present.TreeSummary, filter string, scoped bool) string {
	if filter != "" {
		here := ""
		if scoped {
			here = " here"
		}
		return " / " + filter + here + " · " +
			strconv.Itoa(summary.ShownSchemas) + " of " + strconv.Itoa(summary.TotalSchemas) + " "
	}
	counted := present.FormatCountOf(int64(summary.TotalSchemas), "schema", "schemas")
	if summary.HiddenSystemSchemas > 0 {
		return " " + counted + " · " + strconv.Itoa(summary.HiddenSystemSchemas) + " system hidden "
	}
	if summary.TotalSchemas > 0 {
		return " " + counted + " "
	}
	return ""
}

// renderTreeRow draws one row of the object tree: the guide, the fold mark, the glyph, the
// name, and the detail at the right.
func (model *Model) renderTreeRow(
	row present.TreeRow, guide string, selected, focused bool, width int,
) string {
	theme := model.styles.Theme
	highlighted := selected && focused

	ground := theme.Panel
	switch {
	case highlighted:
		ground = theme.Accent
	case selected:
		ground = theme.Header
	}

	marker := present.FitText("", treeMarkerWidth)
	if row.Expandable {
		kind := cfg.IconFoldClosed
		if row.Expanded {
			kind = cfg.IconFoldOpen
		}
		marker = present.FitText(model.icons.Icon(kind), treeMarkerWidth)
	}

	iconPrefix := ""
	if row.HasIcon {
		iconPrefix = model.icons.Prefix(row.Icon)
	}
	star := ""
	if row.Marked {
		star = model.icons.Icon(cfg.IconFavourites) + " "
	}
	detail := present.TruncateText(star+row.Detail, treeDetailWidth)

	// The name takes the width the rest of the row leaves.
	room := width - treeRowChrome - present.MeasureText(detail) -
		present.MeasureText(guide) - present.MeasureText(marker) -
		present.MeasureText(iconPrefix)
	label := present.TruncateText(row.Label, room)

	guideInk, iconInk, labelInk := theme.Faint, theme.Muted, theme.Text
	// The type of a column is read as often as its name, so both are drawn with a different
	// weight.
	detailInk := theme.Faint

	markInk := guideInk
	switch {
	case highlighted:
		guideInk, iconInk, labelInk, detailInk =
			theme.OnAccent, theme.OnAccent, theme.OnAccent, theme.OnAccent
		markInk = theme.OnAccent
	default:
		if row.HasIcon {
			iconInk = model.styles.IconColor(row.Icon)
		}
		if !row.Selectable {
			labelInk = theme.Muted
		}
		if row.Node.Kind == present.NodeColumn {
			detailInk = theme.Info
		}
	}

	// The parts each cover the cells their own text covers, so the row is written into one
	// buffer and the gap is counted from those widths rather than measured off the escapes.
	leftWidth := present.MeasureText(guide) + present.MeasureText(marker) +
		present.MeasureText(iconPrefix) + present.MeasureText(label)
	gap := max(width-2-leftWidth-present.MeasureText(detail), 0)

	written := strings.Builder{}
	written.Grow(width + treeRowParts*cellEscapeBytes)
	writeBlanksOn(&written, ground, 1)
	writeTextOn(&written, guideInk, ground, guide)
	writeTextOn(&written, markInk, ground, marker)
	writeTextOn(&written, iconInk, ground, iconPrefix)
	writeTextOn(&written, labelInk, ground, label)
	writeBlanksOn(&written, ground, gap)
	writeTextOn(&written, detailInk, ground, detail)
	writeBlanksOn(&written, ground, 1)
	return written.String()
}

// treeRowParts is how many parts of their own colours one row of the tree is written from.
const treeRowParts = 6

// renderTreeFilter draws the field the filter of the tree is typed into.
func (model *Model) renderTreeFilter(connection *app.Connection, width int) string {
	theme := model.styles.Theme
	placeholder := "filter"
	if connection.Tree.FilterScope != "" {
		placeholder = "filter this schema"
	}
	written := connection.Tree.Filter
	if written == "" {
		written = model.styles.Muted().Background(theme.Header).Render(placeholder)
	} else {
		written = paintText(theme.Text, theme.Header, written)
	}
	caret := paintOn(theme.Accent, " ")
	return lipgloss.NewStyle().Background(theme.Header).Width(width).Render(
		paintText(theme.Accent, theme.Header, " / ") +
			written + caret)
}

// scrollFrom returns the first row a scrolled view draws. A view the wheel moved keeps where
// it was rolled to, and its cursor may stand off screen. Every other move brings the cursor
// back into view.
func scrollFrom(cursor, offset, rows, count int, rolled bool) int {
	if rolled {
		return clampOffset(offset, rows, count)
	}
	return scrollTo(cursor, offset, rows, count)
}

// clampOffset holds an offset inside the rows there are to show.
func clampOffset(offset, rows, count int) int {
	if count == 0 || rows <= 0 {
		return 0
	}
	if offset > count-rows {
		offset = count - rows
	}
	if offset < 0 {
		return 0
	}
	return offset
}

// scrollTo returns the offset that keeps the cursor on screen, and moves as little as it can.
func scrollTo(cursor, offset, rows, count int) int {
	offset = clampOffset(offset, rows, count)
	if count == 0 || rows <= 0 {
		return 0
	}
	// A cursor before the window pulls it back, and one past the window pushes it on.
	// Neither may answer a row that is not there.
	if cursor < offset {
		return clampOffset(cursor, rows, count)
	}
	if cursor >= offset+rows {
		return clampOffset(cursor-rows+1, rows, count)
	}
	return offset
}

// renderEditor draws the SQL editor: the line numbers, the coloured text, and the caret.
func (model *Model) renderEditor(
	connection *app.Connection, tab *app.Tab, width, height int,
) []string {
	theme := model.styles.Theme
	// The field of a search opens at the foot of this pane, and the pane keeps the look of
	// a pane the reader is working in while it stands there.
	prompt, asking := findPromptBar(connection, app.PromptFind, app.PromptReplace)
	focused := tab.Focus == app.PaneEditor && (asking || !connection.Overlay.IsOpen())
	inner := width - 2
	body := max(height-2, 1)

	promptRows := []string{}
	if asking {
		promptRows = model.renderPromptBar(prompt, inner)
		body -= len(promptRows)
		if body < 1 {
			body = 1
		}
	}

	lines := tab.Editor.Lines()
	placeholder := tab.Editor.Text == ""
	caretLine, caretColumn := tab.Editor.CaretPosition()

	// The fault on screen takes a row of its own under the statement, so the message and
	// the key that fixes it are read where the fault is.
	faults := model.findDiagnostics(connection, tab)
	shown, hasFault := findShownFault(faults, tab.Editor.Text, caretLine)
	if hasFault {
		body--
		if body < 1 {
			body = 1
		}
	}
	// The pane follows the caret unless the wheel moved it, so a statement longer than the
	// pane can be read without the caret coming along.
	offset := tab.EditorRowOffset
	if !tab.EditorRolled {
		offset = scrollTo(caretLine, tab.EditorRowOffset, body, len(lines))
	}
	offset = clampOffset(offset, body, len(lines))
	tab.EditorRowOffset = offset

	// The gutter numbers each line, so a fault can be pointed at.
	gutterWidth := max(present.MeasureText(strconv.Itoa(len(lines)))+1, 4)

	// The statement keeps a blank column on its right, so a line as wide as the pane still
	// shows where it ends.
	textWidth := max(inner-gutterWidth-1, 1)
	// A line wider than the pane is moved left, and only as far as it must be, so the text
	// before the caret stays in view while the caret walks along a long line.
	columnOffset := scrollTo(
		caretColumn, tab.EditorColumnOffset, textWidth, measureLongestLine(lines)+1)
	tab.EditorColumnOffset = columnOffset

	// The cell of the caret on the screen, which the completion popup is placed from.
	model.caretRow = tabRowHeight + 1 + (caretLine - offset)
	model.caretColumn = model.editorLeft + 1 + gutterWidth + caretColumn - columnOffset

	// Where the text of the statement is drawn, so a press of the pointer can be read as an
	// offset in the buffer.
	model.layout.editorTextLeft = model.editorLeft + 1 + gutterWidth
	model.layout.editorTextTop = firstPaneRow + 1
	model.layout.editorTextWidth = textWidth
	model.layout.editorTextRows = body
	model.layout.editorFirstLine = offset
	model.layout.editorColumnOffset = columnOffset

	// The selection is drawn as a ground under the colours of the tokens, so what Shift and
	// the arrows took is read on the screen and not only by the keys that copy it.
	selectFrom, selectTo := tab.Editor.SelectionRange()
	selects := tab.Editor.HasSelection()
	lineFrom := 0
	for at := 0; at < offset && at < len(lines); at++ {
		lineFrom += len(lines[at]) + 1
	}

	highlights := model.buildEditorHighlights(connection, tab, lines, faults)
	// A line with a fault is marked in the gutter, so a long statement says where it
	// is wrong without reading the border.
	faulty := findFaultyLines(tab, faults)
	written := make([]string, 0, body)
	for at := offset; at < len(lines) && len(written) < body; at++ {
		// The gutter of the line the caret is on takes a ground of its own.
		gutterGround := theme.Panel
		if at == caretLine && focused {
			gutterGround = theme.Zebra
		}
		sign := paintOn(gutterGround, " ")
		if faulty[at] {
			sign = paintText(theme.Error, gutterGround, model.describeProblemSign())
		}
		// The number of the line the caret is on is drawn in the accent, as the number of
		// the row the cursor is on is drawn in the grid.
		numberInk := theme.Faint
		if at == caretLine && focused {
			numberInk = theme.Accent
		}
		number := sign + paintText(numberInk, gutterGround, buildGutterText(strconv.Itoa(at+1), gutterWidth-1))

		drawn := codeLine{
			text: lines[at], spans: highlights[at], width: textWidth,
			columnOffset: columnOffset, caretColumn: caretColumn,
			showCaret: at == caretLine && focused,
		}
		if selects {
			drawn.selectFrom, drawn.selectTo = resolveLineSelection(
				selectFrom-lineFrom, selectTo-lineFrom, len(lines[at]))
		}
		text := model.renderCodeLine(drawn)
		if placeholder && at == 0 {
			text = paintText(theme.Muted, theme.Panel,
				present.FitText(describeEditorHint(connection), textWidth))
		}
		written = append(written,
			number+text+paintOn(gutterGround, " "))
		lineFrom += len(lines[at]) + 1
	}
	for len(written) < body {
		written = append(written, "")
	}
	if hasFault {
		written = append(written, model.renderFaultRow(shown, faults, tab, inner,
			firstPaneRow+1+len(written)))
	}
	written = append(written, promptRows...)

	return model.styles.RenderBoxRows(BoxOptions{
		Width: width, Height: height, Focused: focused,
		Title:       model.describeEditorTitle(tab, len(faults)),
		Note:        model.renderEditorAiKey(connection, tab, hasFault, width),
		BottomTitle: model.describeEditorBorder(connection),
		BottomNote: model.styles.Muted().Background(theme.Panel).
			Render(describeEditorPlace(tab)),
		Lines: written, Ground: theme.Panel,
	})
}

// renderEditorAiKey draws the one key the top border of the editor offers the model, and
// records the cells it covers so a press runs it. Only one is ever drawn, and which one it is
// follows what the statement is in: a run that failed is explained, a statement the scanner
// marked is diagnosed, an empty editor is written for, and a statement that stands is asked
// about.
func (model *Model) renderEditorAiKey(
	connection *app.Connection, tab *app.Tab, hasFault bool, width int,
) string {
	keys := model.buildEditorAiKey(tab, hasFault)
	if keys.isEmpty() {
		return ""
	}
	// The key keeps a blank on each side, as every title of a border does, so it does not
	// run into the border beside it. The note stands one cell of border in from the right
	// corner of the box, and the box starts where the editor pane does.
	ground := model.styles.Theme.Panel
	blank := paintOn(ground, " ")
	left := model.editorLeft +
		measureBorderNoteLeft(width, measureKeyLine(keys)+2) + 1
	return blank + model.writeKeyLine(keys, ground, firstPaneRow, left) + blank
}

// buildEditorAiKey returns the one key the editor offers the model now.
func (model *Model) buildEditorAiKey(tab *app.Tab, hasFault bool) *KeyLine {
	keys := model.sayKeys()
	switch {
	case findLastRunError(tab) != "":
		return keys.bindIcon(
			cfg.ScopeGlobal, ActionAiFixError, cfg.IconAi, "explain the failure")
	case hasFault:
		return keys.bindIcon(
			cfg.ScopeGlobal, ActionAiFixError, cfg.IconAi, "diagnose this")
	case strings.TrimSpace(tab.Editor.Text) == "":
		return keys.bindIcon(
			cfg.ScopeGlobal, ActionShowAiChat, cfg.IconAi, "ask for a query")
	}
	return keys.bindIcon(cfg.ScopeGlobal, ActionSendToAi, cfg.IconAi, "ask about this")
}

// resolveLineSelection returns the columns of one line a selection covers, given where the
// selection starts and ends relative to the start of the line. The end reaches one past the
// text where the selection carries the line break as well.
func resolveLineSelection(from, to, length int) (int, int) {
	if from < 0 {
		from = 0
	}
	if to > length+1 {
		to = length + 1
	}
	if to <= from {
		return 0, 0
	}
	return from, to
}

// measureLongestLine returns the bytes of the longest line, which is how far the pane can
// be moved left.
func measureLongestLine(lines []string) int {
	longest := 0
	for _, line := range lines {
		if len(line) > longest {
			longest = len(line)
		}
	}
	return longest
}

// findShownFault returns the fault the pane reports: the one on the line of the caret, or
// the first one.
func findShownFault(
	faults []editor.Diagnostic, text string, caretLine int,
) (editor.Diagnostic, bool) {
	if len(faults) == 0 {
		return editor.Diagnostic{}, false
	}
	for _, fault := range faults {
		if present.ResolvePosition(text, fault.Start).Line-1 == caretLine {
			return fault, true
		}
	}
	return faults[0], true
}

// editorPaneName is what the pane calls itself. Not every server this client opens has SQL,
// so it names what is written in it and not the language that writes it.
const editorPaneName = "query"

// describeEditorTitle names the pane, and what the list or the scan found. The list shows
// its place and its keys in the title, to save a row.
func (model *Model) describeEditorTitle(tab *app.Tab, faults int) string {
	if total := len(tab.Completion.Candidates); total > 0 {
		return " " + editorPaneName + " · " + strconv.Itoa(tab.Completion.Selected+1) + "/" +
			strconv.Itoa(total) + " · ↑↓ Tab take · Esc "
	}
	if faults == 0 {
		return " " + editorPaneName + " "
	}
	return " " + editorPaneName + " · " +
		present.FormatCountOf(int64(faults), "problem", "problems") + " "
}

// renderFaultRow writes the fault on screen: where it is, what it says, and the keys that
// reach the rest of them.
func (model *Model) renderFaultRow(
	shown editor.Diagnostic, faults []editor.Diagnostic, tab *app.Tab, inner, row int,
) string {
	theme := model.styles.Theme
	at := present.ResolvePosition(tab.Editor.Text, shown.Start)
	said := model.writeProblemSign() + strconv.Itoa(at.Line) + ":" + strconv.Itoa(at.Column) +
		"  " + strings.ReplaceAll(shown.Message, "\n", " ")

	// The key that asks the model about the fault stands on the border of the pane, where
	// the one key the editor offers the model always stands, so the row names the count of
	// the faults and the key that steps through them.
	counted := ""
	if len(faults) > 1 {
		place := 1
		for index, fault := range faults {
			if fault == shown {
				place = index + 1
				break
			}
		}
		counted = strconv.Itoa(place) + "/" + strconv.Itoa(len(faults)) + " " +
			model.registry.FormatActionChordCompact(cfg.ScopeEditor, ActionNextProblem)
	}
	right := model.styles.Muted().Render(counted)

	room := max(inner-2-measureStyledWidth(right), 0)

	// The fault itself steps to the next one, so the words work as the key they name.
	left := model.editorLeft + 1
	model.recordButton(row, left+1, min(present.MeasureText(said), room),
		cfg.ScopeEditor, ActionNextProblem)

	return paintOn(theme.Panel, " ") +
		padStyledOn(model.styles.Error().Render(present.TruncateText(said, room)),
			inner-2-measureStyledWidth(right), theme.Panel) + right +
		paintOn(theme.Panel, " ")
}

// describeEditorHint returns what an empty pane says it takes, which is the shape of one
// statement of this server.
func describeEditorHint(connection *app.Connection) string {
	if hint := connection.Session.Dialect().StatementHint; hint != "" {
		return hint
	}
	return "select … from …"
}

// describeProblemSign returns the mark a fault carries. A set that draws no glyph for it
// leaves the words of the fault to say what it is.
func (model *Model) describeProblemSign() string {
	return model.icons.Icon(cfg.IconProblem)
}

// writeProblemSign writes the mark of a fault with the blank after it, or nothing where the
// set draws no glyph for one.
func (model *Model) writeProblemSign() string {
	return model.icons.Prefix(cfg.IconProblem)
}

// findFaultyLines returns the lines the scanner found a fault on. The faults are read once for
// the whole frame and handed on, because finding them tokenizes the buffer and walks the
// catalog.
func findFaultyLines(tab *app.Tab, faults []editor.Diagnostic) map[int]bool {
	faulty := map[int]bool{}
	for _, found := range faults {
		faulty[present.ResolvePosition(tab.Editor.Text, found.Start).Line-1] = true
	}
	return faulty
}

// describeEditorPlace writes where the caret stands, or how much stands selected, which the
// bottom border of the pane carries against its right corner.
func describeEditorPlace(tab *app.Tab) string {
	if tab.Editor.HasSelection() {
		selected := tab.Editor.Selection()
		if lines := strings.Count(selected, "\n") + 1; lines > 1 {
			return " " + present.FormatCountOf(int64(lines), "line", "lines") + " selected "
		}
		return " " + strconv.Itoa(utf8.RuneCountInString(selected)) + " selected "
	}
	// The place is read the way the fault row reads one, so the two never disagree about
	// which cell of which line the caret is on.
	at := present.ResolvePosition(tab.Editor.Text, tab.Editor.Caret)
	return " " + strconv.Itoa(at.Line) + ":" + strconv.Itoa(at.Column) + " "
}

// describeEditorBorder names the key on the border between this pane and the result.
func (model *Model) describeEditorBorder(connection *app.Connection) string {
	said := "full height"
	if !connection.ResultVisible {
		said = "show the result"
	}
	return " " + model.registry.FormatActionChordCompact(
		cfg.ScopeGlobal, ActionToggleResult) + " " + said + " "
}

// findLocalDiagnostics returns the faults the scanner found in the buffer.
func (model *Model) findLocalDiagnostics(
	connection *app.Connection, tab *app.Tab,
) []editor.Diagnostic {
	return connection.Session.Language().FindLocalDiagnostics(
		tab.Editor.Text, model.buildSchemaKnowledge(connection, tab))
}

// editorFaults holds the faults of one buffer, and the buffer and the catalog they were found
// in. Finding them tokenizes the whole buffer and checks every relation it names against the
// catalog, and neither the buffer nor the catalog changes between most frames.
type editorFaults struct {
	found   bool
	text    string
	catalog app.CatalogFingerprint
	faults  []editor.Diagnostic
}

// resolveLocalDiagnostics returns the faults the scanner found, reading the buffer only where
// the faults it kept were found in another buffer or against another catalog.
func (model *Model) resolveLocalDiagnostics(
	connection *app.Connection, tab *app.Tab,
) []editor.Diagnostic {
	catalog := connection.FingerprintCatalog()
	key := model.buildTabKey(connection, tab)
	held, kept := model.caches.readFaults(key)
	if kept && held.found && held.text == tab.Editor.Text && held.catalog == catalog {
		return held.faults
	}

	faults := model.findLocalDiagnostics(connection, tab)
	model.caches.keepFaults(key, editorFaults{
		found: true, text: tab.Editor.Text, catalog: catalog, faults: faults,
	})
	return faults
}

// findDiagnostics returns the faults of the buffer: the ones the scanner found, and the
// ones the server named where the scan found none.
func (model *Model) findDiagnostics(
	connection *app.Connection, tab *app.Tab,
) []editor.Diagnostic {
	if found := model.resolveLocalDiagnostics(connection, tab); len(found) > 0 {
		return found
	}
	if tab.Served.SQL == tab.Editor.Text {
		return tab.Served.Found
	}
	return nil
}

// buildSchemaKnowledge returns what this tab knows of the catalog. Nothing is reported until
// the catalog is read, because an empty catalog knows nothing.
func (model *Model) buildSchemaKnowledge(
	connection *app.Connection, tab *app.Tab,
) editor.SchemaKnowledge {
	// The same columns as the list offers, by name only, which is all the check needs.
	byQualifier := map[string][]string{}
	for qualifier, columns := range model.buildCompletionColumns(connection, tab) {
		names := make([]string, 0, len(columns))
		for _, column := range columns {
			names = append(names, column.Name)
		}
		byQualifier[qualifier] = names
	}

	return editor.SchemaKnowledge{
		Loaded: len(connection.Catalog.Tables) > 0,
		IsKnownTable: func(reference statement.TableReference) bool {
			_, known := model.findTableByName(connection, reference.SelectSource)
			return known
		},
		ColumnsByQualifier: byQualifier,
	}
}

// lineHighlight is one span of one line to colour.
type lineHighlight struct {
	kind  HighlightKind
	start int
	end   int
}

// collectLineHighlights returns the spans to colour, one list per line. The editor colours
// one line at a time, so a token over a line break is cut.
func collectLineHighlights(text string, tokens []syntax.Token) map[int][]lineHighlight {
	byLine := map[int][]lineHighlight{}
	line := 0
	lineStart := 0
	cursor := 0

	advanceTo := func(offset int) {
		for cursor < offset && cursor < len(text) {
			if text[cursor] == '\n' {
				line++
				lineStart = cursor + 1
			}
			cursor++
		}
	}

	for _, token := range tokens {
		advanceTo(token.Start)
		spanStart := token.Start
		for index := token.Start; index < token.End && index < len(text); index++ {
			if text[index] != '\n' {
				continue
			}
			if index > spanStart {
				byLine[line] = append(byLine[line], lineHighlight{
					kind:  HighlightKind(token.Kind),
					start: spanStart - lineStart, end: index - lineStart,
				})
			}
			advanceTo(index + 1)
			spanStart = index + 1
		}
		advanceTo(token.End)
		if token.End > spanStart {
			byLine[line] = append(byLine[line], lineHighlight{
				kind:  HighlightKind(token.Kind),
				start: spanStart - lineStart, end: token.End - lineStart,
			})
		}
	}
	return byLine
}

// buildSQLLineHighlights returns the spans to colour in a read-only statement, such as the
// definition of a relation. It reads the SQL scanner rather than the language of the open
// buffer, because a definition is SQL whatever the buffer holds.
func buildSQLLineHighlights(text string) map[int][]lineHighlight {
	return collectLineHighlights(text, syntax.Tokenize(text, syntax.FlavourStandard))
}

// buildEditorHighlights returns everything drawn over the statement: the colour of every
// token, the guide of every indent step, the bracket at the caret with the one that closes
// it, and the marks of the faults. A mark is planned before the colour, because the first
// span over a cell is the one that is kept.
func (model *Model) buildEditorHighlights(
	connection *app.Connection, tab *app.Tab, lines []string, faults []editor.Diagnostic,
) map[int][]lineHighlight {
	term := resolveFindTerm(connection, tab)
	text := tab.Editor.Text
	byLine := map[int][]lineHighlight{}
	// A span over several lines is marked on its first line only.
	add := func(kind HighlightKind, start, end int) {
		span := present.ResolveLineSpan(text, editor.Diagnostic{Start: start, End: end})
		if span.Line < 0 || span.Line >= len(lines) {
			return
		}
		byLine[span.Line] = append(byLine[span.Line], lineHighlight{
			kind: kind, start: span.Start, end: span.End,
		})
	}

	tokens := connection.Session.Language().Tokenize(text)
	if pair, found := FindBracketPair(
		text, tab.Editor.Caret, buildCoveredReader(tokens)); found {
		add(BracketStyle, pair.Open, pair.Open+1)
		add(BracketStyle, pair.Close, pair.Close+1)
	}
	for _, fault := range faults {
		add(ProblemStyle, fault.Start, fault.End)
	}
	// What a search found is marked over the colour of the token it stands in, so a match
	// inside a name or a string is seen as readily as one between them.
	for _, start := range tab.Editor.FindMatches(term) {
		add(MatchStyle, start, start+len(term))
	}
	for at, columns := range PlanIndentGuides(lines) {
		for _, column := range columns {
			if at < len(lines) {
				byLine[at] = append(byLine[at], lineHighlight{
					kind: GuideStyle, start: column, end: column + 1,
				})
			}
		}
	}

	// The colour of every token is read from the tokens above, because tokenizing the
	// buffer again for them is the most expensive thing the pane does.
	for line, spans := range collectLineHighlights(text, tokens) {
		byLine[line] = append(byLine[line], spans...)
	}
	return byLine
}

// buildCoveredReader returns a reader that is true for an offset inside a string, a comment
// or a quoted name, so a bracket in one of those is not paired.
func buildCoveredReader(tokens []syntax.Token) func(int) bool {
	covered := []syntax.Token{}
	for _, token := range tokens {
		switch token.Kind {
		case syntax.TokenString, syntax.TokenComment, syntax.TokenQuoted:
			covered = append(covered, token)
		}
	}
	return func(at int) bool {
		for _, token := range covered {
			if at >= token.Start && at < token.End {
				return true
			}
		}
		return false
	}
}

// codeLine is one line of code the editor draws: the text, the highlights over it, and
// where the caret and the selection fall in it. Every column counts bytes of the line, as
// the buffer counts them.
type codeLine struct {
	text  string
	spans []lineHighlight
	width int
	// The first column drawn, so a line wider than the pane is moved left.
	columnOffset int
	// The column the caret stands in, drawn only while showCaret is true.
	caretColumn int
	showCaret   bool
	// The columns the selection covers. The end stands one past the text of the line where
	// the selection carries the line break as well.
	selectFrom, selectTo int
}

// holdsColumn is true where the selection covers that column of the line.
func (drawn codeLine) holdsColumn(at int) bool {
	return at >= drawn.selectFrom && at < drawn.selectTo
}

// renderCodeLine draws one line of the editor, coloured by the highlights of the theme.
func (model *Model) renderCodeLine(drawn codeLine) string {
	return model.renderCodeLineOn(model.styles.Theme.Panel, drawn)
}

// renderCodeLineOn draws one line of code on a ground of its own, which a block of code inside
// a card carries.
func (model *Model) renderCodeLineOn(ground color.Color, drawn codeLine) string {
	theme := model.styles.Theme
	// The line holds what the user typed or what the server sent, so what is drawn from
	// it is made safe first. The buffer keeps the text as it stands.
	line := present.SafeText(drawn.text)

	// The escapes each byte of the line opens with, held as a place in a short list rather
	// than as a style of its own: a style for every byte of a block of code costs more than
	// the whole of the rest of the frame. Each kind is opened twice, once on the ground of
	// the block and once on the ground of a selection, and one place reads in both lists.
	// The first span over a cell wins, so a mark planned before the colour of the token
	// stays on top.
	plain := []string{resolveOpening(theme.Text, ground)}
	picked := []string{resolveOpening(theme.Text, theme.Selection)}
	kinds := make([]HighlightKind, 0, len(drawn.spans))
	resolveMark := func(kind HighlightKind) int32 {
		for at, held := range kinds {
			if held == kind {
				return int32(at) + 1
			}
		}
		kinds = append(kinds, kind)
		plain = append(plain, model.styles.resolveSyntaxOpening(kind, ground))
		picked = append(picked, model.styles.resolveSyntaxOpening(kind, theme.Selection))
		return int32(len(kinds))
	}

	marks := make([]int32, len(line))
	taken := make([]bool, len(line))
	for _, span := range drawn.spans {
		mark := resolveMark(span.kind)
		for at := max(span.start, 0); at < span.end && at < len(line); at++ {
			if taken[at] {
				continue
			}
			marks[at], taken[at] = mark, true
		}
	}

	// The caret is drawn on the cell it stands on, so the pane needs no cursor of its own
	// and the token under it keeps its colour. It covers every byte of the character it
	// stands on, or the escapes would be written into the middle of one.
	caret := resolveOpening(theme.OnAccent, theme.Accent)
	onCaret := make([]bool, len(line))
	if drawn.showCaret && drawn.caretColumn >= 0 && drawn.caretColumn < len(line) {
		_, width := utf8.DecodeRuneInString(line[drawn.caretColumn:])
		for at := drawn.caretColumn; at < drawn.caretColumn+max(width, 1) && at < len(line); at++ {
			onCaret[at] = true
		}
	}

	written := strings.Builder{}
	written.Grow(len(line) + (len(drawn.spans)+2)*cellEscapeBytes)
	for at := max(drawn.columnOffset, 0); at < len(line); {
		end := at + 1
		for end < len(line) && marks[end] == marks[at] && onCaret[end] == onCaret[at] &&
			drawn.holdsColumn(end) == drawn.holdsColumn(at) {
			end++
		}
		opening := plain[marks[at]]
		switch {
		case onCaret[at]:
			opening = caret
		case drawn.holdsColumn(at):
			opening = picked[marks[at]]
		}
		writeOpenedText(&written, opening, line[at:end])
		at = end
	}
	// The caret past the last character takes a cell of its own, and so does the line break
	// a selection over more than one line carries.
	switch {
	case drawn.showCaret && drawn.caretColumn >= len(line):
		writeOpenedText(&written, caret, " ")
	case drawn.selectTo > len(line) && len(line) >= max(drawn.columnOffset, 0):
		writeOpenedText(&written, picked[0], " ")
	}
	return padStyledOn(truncateStyled(written.String(), drawn.width), drawn.width, ground)
}
