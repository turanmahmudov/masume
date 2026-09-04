package ui

import (
	"image/color"
	"strings"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// The card that says what a write does before it runs.

const (
	// writePlanLabelWidth is the column the values of the plan start in.
	writePlanLabelWidth = 10
	writePlanMeterWidth = 12
	// writePlanShareOfConcern is the share of a relation above which the count is drawn as
	// a fault rather than as a number.
	writePlanShareOfConcern = 0.25
	// writePlanStatementRows is how many rows of the statement the card draws. The rest is
	// counted, so the answers stay on the card whatever the statement is.
	writePlanStatementRows = 8
)

// renderWritePlan draws the plan and the two answers.
func (model *Model) renderWritePlan(overlay app.Overlay, width int) string {
	inner := max(width-present.CardChrome, 1)
	plan := overlay.Plan

	lines := model.renderWritePlanStatement(plan.SQL, inner)
	lines = append(lines, "")
	lines = append(lines, model.renderWritePlanLines(plan, inner)...)

	yes, no := model.renderWritePlanAnswers()
	lines = append(lines, "", yes+"  "+no)

	keys := model.sayKeys().
		bind(cfg.ScopeDialog, ActionAnswerYes, "run").
		bind(cfg.ScopeDialog, ActionAnswerNo, "cancel").
		bind(cfg.ScopeDialog, ActionClose, "cancel")
	model.recordCardBody()
	model.recordAnswerChips(
		len(lines)-1, measureStyledWidth(yes), measureStyledWidth(no))
	return model.renderTextCard(overlay.Kind, overlay.Title, width, lines, keys,
		len(lines), destructiveCard)
}

func (model *Model) renderWritePlanAnswers() (string, string) {
	theme := model.styles.Theme
	yes := paintText(model.styles.InkOn(theme.Error), theme.Error, "  "+
		model.registry.FormatActionChords(cfg.ScopeDialog, ActionAnswerYes)+" run  ")
	no := paintText(theme.Text, theme.Header, "  "+
		model.registry.FormatActionChords(cfg.ScopeDialog, ActionAnswerNo)+" cancel  ")
	return yes, no
}

// renderWritePlanStatement draws the write, wrapped so the predicate is read in full.
func (model *Model) renderWritePlanStatement(sql string, inner int) []string {
	theme := model.styles.Theme
	wrapped := []string{}
	for line := range strings.SplitSeq(present.SafeLines(sql), "\n") {
		wrapped = append(wrapped, present.WrapWords(line, inner)...)
	}

	// The colours are read off the text as drawn, so a wrapped line keeps them.
	highlights := buildSQLLineHighlights(strings.Join(wrapped, "\n"))
	shown := min(len(wrapped), writePlanStatementRows)

	lines := make([]string, 0, shown+1)
	for at := range shown {
		lines = append(lines, model.renderCodeLineOn(theme.Panel, codeLine{
			text: wrapped[at], spans: highlights[at], width: inner}))
	}
	if len(wrapped) > shown {
		lines = append(lines, padStyledOn(paintText(theme.Muted, theme.Panel,
			"… "+present.FormatCountOf(int64(len(wrapped)-shown), "line", "lines")+" more"),
			inner, theme.Panel))
	}
	return lines
}

// writePlanLine is one line of the plan. A line that follows another of the same label
// leaves the label out.
type writePlanLine struct {
	label string
	value string
	ink   color.Color
	// Drawn after the value, already styled.
	trailer string
}

func (model *Model) renderWritePlanLines(plan writeplan.Plan, inner int) []string {
	rows := []writePlanLine{model.buildWritePlanRowsLine(plan)}
	if columns, named := writeplan.DescribeColumns(plan); named {
		rows = append(rows, writePlanLine{
			label: writeplan.LabelColumns, value: columns, ink: model.styles.Theme.Text,
		})
	}
	rows = append(rows, model.buildWritePlanCascadeLines(plan)...)
	rows = append(rows, model.buildWritePlanBlockerLines(plan)...)
	rows = append(rows, model.buildWritePlanUndoLine(plan), writePlanLine{
		label: writeplan.LabelCommit, value: writeplan.DescribeCommit(plan),
		ink: model.styles.Theme.Muted,
	})

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, model.renderWritePlanLine(row, inner))
	}
	return lines
}

func (model *Model) buildWritePlanRowsLine(plan writeplan.Plan) writePlanLine {
	theme := model.styles.Theme
	row := writePlanLine{
		label: writeplan.LabelRows, value: writeplan.DescribeRows(plan), ink: theme.Text,
	}
	if !plan.HasRows {
		row.ink = theme.Warning
		return row
	}

	share, held := plan.ReadShare()
	if !held {
		return row
	}
	if plan.NamesEveryRow() || share >= writePlanShareOfConcern {
		row.ink = theme.Error
	}
	row.trailer = paintText(row.ink, theme.Panel,
		"  "+present.BuildMeter(share, 1, writePlanMeterWidth))
	return row
}

func (model *Model) buildWritePlanCascadeLines(plan writeplan.Plan) []writePlanLine {
	rows := make([]writePlanLine, 0, len(plan.Cascades))
	for at, cascade := range plan.Cascades {
		label := writeplan.LabelCascades
		if at > 0 {
			label = ""
		}
		rows = append(rows, writePlanLine{
			label: label, value: writeplan.DescribeCascade(cascade),
			ink: model.styles.Theme.Warning,
		})
	}
	return rows
}

// buildWritePlanBlockerLines returns one line per relation that blocks the write, drawn as
// a fault because the server rejects the write while one of them references it.
func (model *Model) buildWritePlanBlockerLines(plan writeplan.Plan) []writePlanLine {
	rows := make([]writePlanLine, 0, len(plan.Blockers))
	for at, blocker := range plan.Blockers {
		label := writeplan.LabelBlocked
		if at > 0 {
			label = ""
		}
		rows = append(rows, writePlanLine{
			label: label, value: writeplan.DescribeBlocker(blocker),
			ink: model.styles.Theme.Error,
		})
	}
	return rows
}

func (model *Model) buildWritePlanUndoLine(plan writeplan.Plan) writePlanLine {
	theme := model.styles.Theme
	row := writePlanLine{
		label: writeplan.LabelUndo, value: writeplan.DescribeUndo(plan.Undo), ink: theme.Text,
	}
	if !plan.Undo.Kept {
		row.ink = theme.Muted
		return row
	}
	row.trailer = paintText(theme.Muted, theme.Panel, "  "+
		model.registry.FormatActionChords(cfg.ScopeGlobal, ActionUndoWrite)+" after it ran")
	return row
}

func (model *Model) renderWritePlanLine(row writePlanLine, inner int) string {
	theme := model.styles.Theme
	label := present.FitText(row.label, writePlanLabelWidth)
	room := max(inner-writePlanLabelWidth-measureStyledWidth(row.trailer), 1)

	written := paintText(theme.Muted, theme.Panel, label)
	written += paintText(row.ink, theme.Panel, present.TruncateText(row.value, room))
	return padStyledOn(written+row.trailer, inner, theme.Panel)
}
