package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// buildCompletionColumns returns the columns of the relations the statement reads,
// keyed in lower case under the name of each relation and under any alias it takes.
func (model *Model) buildCompletionColumns(
	connection *app.Connection, tab *app.Tab,
) map[string][]editor.CompletionColumn {
	byQualifier := map[string][]editor.CompletionColumn{}
	flavour := connection.Session.Dialect().Syntax

	for _, reference := range statement.FindTableReferences(tab.Editor.Text, flavour) {
		table, found := model.findTableByName(connection, reference.SelectSource)
		if !found {
			continue
		}
		state, read := connection.Catalog.Details[present.BuildTableID(table)]
		if !read || state.Kind != present.DetailReady {
			continue
		}
		columns := make([]editor.CompletionColumn, 0, len(state.Detail.Columns))
		for _, column := range state.Detail.Columns {
			// A key column says so, because the name alone does not.
			detail := column.DataType
			if column.IsPrimaryKey {
				detail += " pk"
			}
			columns = append(columns, editor.CompletionColumn{
				Name: column.Name, Detail: detail,
			})
		}
		byQualifier[strings.ToLower(reference.Name)] = columns
		if reference.HasAlias {
			byQualifier[strings.ToLower(reference.Alias)] = columns
		}
	}
	return byQualifier
}

// buildCompletionSources returns everything the catalog and the result offer the caret.
func (model *Model) buildCompletionSources(
	connection *app.Connection, tab *app.Tab,
) editor.CompletionSources {
	schemas, tables, functions := connection.CompletionSources()

	columns := []editor.CompletionColumn{}
	if held := tab.Results.Active(); held != nil && held.State.Kind == app.QuerySucceeded {
		for _, column := range held.State.Result.Columns {
			columns = append(columns, editor.CompletionColumn{
				Name: column.Name, Detail: column.DataType,
			})
		}
	}

	return editor.CompletionSources{
		Schemas: schemas, Tables: tables, Functions: functions, Columns: columns,
		ColumnsByQualifier: model.buildCompletionColumns(connection, tab),
	}
}

// refreshCompletion builds the list for the caret.
func (model *Model) refreshCompletion(connection *app.Connection, tab *app.Tab) {
	list := &tab.Completion
	if tab.Focus != app.PaneEditor || connection.Overlay.IsOpen() || list.Dismissed {
		list.Close()
		return
	}

	text := tab.Editor.Text
	offset := tab.Editor.Caret
	prefix := editor.ReadPrefix(text, offset)
	found := connection.Session.Language().BuildCompletions(
		prefix, model.buildCompletionSources(connection, tab),
		editor.CompletionContext{
			AllowQualified: !editor.IsUpdateSetTarget(text, offset),
			// Read from the start of the word, because the text before it decides
			// what may follow.
			NamePosition: editor.ResolveNamePosition(text, offset-len(prefix)),
		})

	list.Candidates = found
	list.Selected = 0
}

// acceptCompletion writes the marked candidate in place of the word under the caret.
func (model *Model) acceptCompletion(connection *app.Connection, tab *app.Tab) {
	chosen, found := tab.Completion.Chosen()
	if !found {
		return
	}
	written, caret := editor.ApplyCompletion(
		tab.Editor.Text, tab.Editor.Caret, chosen, connection.Session.Dialect())
	tab.Editor.SetText(written)
	tab.Editor.Caret = caret
	tab.Completion.Close()
}

// completionRows is how many suggestions the popup shows at once.
const completionRows = 6

// completionChrome is the border and the padding of the popup.
const completionChrome = 2

// renderCompletionPopup draws the suggestions over whatever stands under the editor.
// The pane cannot paint over its neighbour, so the workspace places it on the frame.
func (model *Model) renderCompletionPopup(tab *app.Tab, height int) (string, int, int) {
	list := &tab.Completion
	model.layout.completionRows = rowsHit{}
	if !list.IsListing() {
		return "", 0, 0
	}
	theme := model.styles.Theme

	widest := 0
	for _, candidate := range list.Candidates {
		if measured := measureCompletionRow(candidate); measured > widest {
			widest = measured
		}
	}
	width := min(widest+completionChrome, model.width)

	shownRows := min(len(list.Candidates), completionRows)
	popupHeight := shownRows + completionChrome

	// The window follows the marked row, because the list is longer than the popup.
	start := 0
	if len(list.Candidates) > completionRows {
		start = core.ClampWithin(
			list.Selected-completionRows/2, len(list.Candidates)-completionRows)
	}

	lines := make([]string, 0, shownRows)
	for at := start; at < start+shownRows && at < len(list.Candidates); at++ {
		lines = append(lines, model.renderCompletionRow(
			list.Candidates[at], at == list.Selected, width-completionChrome))
	}

	// Below the caret where there is room, and above it where there is not. A popup
	// past the bottom of the screen would show one row only.
	top := model.caretRow + 1
	if top+popupHeight > height {
		top = model.caretRow - popupHeight
	}
	if top < 0 {
		top = 0
	}
	left := core.ClampWithin(model.caretColumn, model.width-width)

	// The rows of the popup, so a press takes the candidate it lands on. The box is placed
	// on the frame of the workspace and the title bar is put over it afterwards, so a row of
	// the screen is one more than a row of the frame. The border of the box takes its first
	// row and its first column.
	model.layout.completionRows = rowsHit{
		top: top + 1 + titleBarRows, count: shownRows, offset: start,
		from: left + 1, to: left + width - 2,
	}

	return model.styles.RenderBox(BoxOptions{
		Width: width, Height: popupHeight, Lines: lines, Ground: theme.Header,
	}), left, top
}

// measureCompletionRow returns the width of one row: the name, the kind and the detail.
func measureCompletionRow(candidate editor.Completion) int {
	detail := ""
	if candidate.Detail != "" {
		detail = " · " + candidate.Detail
	}
	return present.MeasureText(candidate.Text) + present.MeasureText(string(candidate.Kind)) +
		present.MeasureText(detail) + 4
}

// renderCompletionRow draws one suggestion: the name at the left, and what it is at the
// right. The kind tells a column from a table of the same name.
func (model *Model) renderCompletionRow(
	candidate editor.Completion, marked bool, width int,
) string {
	theme := model.styles.Theme
	ground := theme.Header
	if marked {
		ground = theme.Accent
	}

	detail := ""
	if candidate.Detail != "" {
		detail = " · " + candidate.Detail
	}
	kindInk := model.styles.CompletionKindColor(candidate.Kind)
	nameInk, detailInk := theme.Text, theme.Faint
	if marked {
		nameInk, kindInk, detailInk = theme.OnAccent, theme.OnAccent, theme.OnAccent
	}

	right := lipgloss.NewStyle().Foreground(kindInk).Background(ground).
		Render(string(candidate.Kind)) +
		lipgloss.NewStyle().Foreground(detailInk).Background(ground).Render(detail)
	// The blank column on each side of the row is not the name, so the room is measured
	// after it.
	inner := width - 2
	room := inner - present.MeasureText(string(candidate.Kind)) -
		present.MeasureText(detail) - 1
	name := lipgloss.NewStyle().Foreground(nameInk).Background(ground).
		Render(present.TruncateText(candidate.Text, room))

	gap := max(inner-present.MeasureText(present.TruncateText(candidate.Text, room))-
		present.MeasureText(string(candidate.Kind))-present.MeasureText(detail), 1)
	pad := lipgloss.NewStyle().Background(ground).Render(" ")
	return pad + name +
		lipgloss.NewStyle().Background(ground).Render(strings.Repeat(" ", gap)) + right + pad
}
