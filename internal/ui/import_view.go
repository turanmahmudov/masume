package ui

import (
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/load"
	"github.com/turanmahmudov/masume/internal/present"
)

// The card the import draws: the form while the file and the mapping are set, and the
// review of what the import would do before anything is written.

// importLabelWidth is the width of the label of one row.
const importLabelWidth = 30

// importedProblemRows is how many refused rows the review lists. The rest are counted.
const importedProblemRows = 5

// renderImport draws the card of an import: the picker, the form, or the review.
func (model *Model) renderImport(overlay app.Overlay, width int) string {
	switch overlay.Import.Stage {
	case app.ImportPick:
		return model.renderImportPicker(overlay, width)
	case app.ImportReview:
		return model.renderImportReview(overlay, width)
	}
	return model.renderImportForm(overlay, width)
}

// renderImportPicker draws the directory the file is chosen out of.
func (model *Model) renderImportPicker(overlay app.Overlay, width int) string {
	inner := width - present.CardChrome
	lines := model.renderFilePicker(model.ActiveID(), inner)
	if overlay.Notice != "" {
		lines = append(lines, "", model.styles.Error().Render(
			present.TruncateText(overlay.Notice, inner)))
	}

	keys := model.sayKeys().name("↑↓", "file").
		name("→", "open the directory").name("←", "the one above").
		name("Enter", "choose").name("Esc", "cancel")
	said := present.TruncateText(keys.buildText(), width-4)
	model.recordCardBody()
	lines = model.appendCardKeyRow(lines, keys, said, cardBodyRow, cardBodyColumn)
	model.rememberCardKeys(keys)
	return model.renderCard(buildImportTitle(overlay.Import), width, lines, plainCard)
}

// renderImportForm draws one row per setting and one row per column of the file.
func (model *Model) renderImportForm(overlay app.Overlay, width int) string {
	fields := BuildImportFields(overlay)
	valueWidth := max(width-present.CardChrome-importLabelWidth, 8)

	model.layout.formChoices = nil
	lines := make([]string, 0, len(fields)+3)
	for at, field := range fields {
		focused := at == overlay.Field
		marker := "  "
		labelStyle := model.styles.Muted()
		if focused {
			marker = present.FitText(model.icons.Icon(cfg.IconField), fieldMarkerWidth)
			labelStyle = model.styles.Accent()
		}

		value := field.Value
		written := model.styles.Muted().Render(present.TruncateText(value, valueWidth))
		switch {
		case len(field.Choices) > 0:
			written = model.renderChoiceField(value, valueWidth, at,
				cardBodyRow+at, cardBodyColumn+importLabelWidth, focused)
		case focused:
			written = model.renderField(
				app.NewEditorBuffer(value, len(value)), valueWidth, FieldLook{
					Ground: model.styles.Theme.Header, Ink: model.styles.Theme.Text,
					Focused: true, Placeholder: field.Label,
				})
		}
		lines = append(lines, labelStyle.Render(marker+
			fitFieldLabel(field.Label, importLabelWidth-present.MeasureText(marker)))+written)
	}

	// The problem line is always counted, so the card keeps its height.
	said := FindImportProblem(overlay, model.readActiveDialect())
	if said == "" {
		said = overlay.Notice
	}
	lines = append(lines, model.styles.Error().Render(
		present.TruncateText(said, width-4)))

	keys := model.sayKeys().name("↑↓", "field").name("← →", "change").
		name("Enter", describeImportStep(overlay.Import)).
		bind(cfg.ScopeDialog, ActionClose, "cancel")
	keyRow := present.TruncateText(keys.buildText(), width-4)
	model.recordCardBody()
	lines = model.appendCardKeyRow(lines, keys, keyRow, cardBodyRow, cardBodyColumn)
	model.rememberCardKeys(keys)
	model.layout.formRows = rowsHit{
		top: model.layout.cardBodyTop, count: len(fields),
		from: model.layout.cardBodyLeft - 1, to: model.layout.cardBodyLeft + width - 4,
	}
	return model.renderCard(buildImportTitle(overlay.Import), width, lines, plainCard)
}

// describeImportStep describes the next step of the import.
func describeImportStep(held app.ImportRequest) string {
	if held.Running {
		return "reading…"
	}
	if held.Stage == app.ImportFile {
		return "read the file"
	}
	return "review"
}

// buildImportTitle names the card: the file being read, and the rows it holds once they
// have been counted.
func buildImportTitle(held app.ImportRequest) string {
	if held.Plan.Path == "" {
		return " import "
	}
	title := " import " + present.TruncateText(baseName(held.Plan.Path), 40)
	if held.Plan.CreatesTable {
		title += " · new table"
	}
	if held.Report.Rows > 0 {
		title += " · " + present.FormatRowCount(int64(held.Report.Rows))
	} else if len(held.Plan.Sample.Rows) > 0 && held.Plan.Sample.More {
		title += " · " + strconv.Itoa(len(held.Plan.Sample.Rows)) + "+ rows"
	}
	return title + " "
}

// baseName returns the name of the file without the directories in front of it.
func baseName(path string) string {
	if at := strings.LastIndex(path, "/"); at != -1 {
		return path[at+1:]
	}
	return path
}

// renderImportReview draws what the import would do: the rows it would write, the rows it
// cannot, and the SQL that would run.
func (model *Model) renderImportReview(overlay app.Overlay, width int) string {
	held := overlay.Import
	inner := width - present.CardChrome

	lines := []string{
		model.styles.Ink().Render(present.TruncateText(DescribeImportSummary(held), inner)),
		model.styles.Muted().Render(present.TruncateText(
			held.Plan.Path+" → "+describeImportTable(held), inner)),
		"",
	}

	if held.Report.Refused > 0 {
		lines = append(lines, model.styles.Error().Render(present.TruncateText(
			present.FormatRowCount(int64(held.Report.Refused))+
				" not written:", inner)))
		for at, problem := range held.Report.Problems {
			if at >= importedProblemRows {
				break
			}
			lines = append(lines, model.styles.Muted().Render(present.TruncateText(
				"  line "+strconv.Itoa(problem.Line)+"  "+
					describeRowProblem(problem), inner)))
		}
		lines = append(lines, "")
	}

	for _, statement := range held.Statements {
		for line := range strings.SplitSeq(statement, "\n") {
			lines = append(lines, model.styles.Faint().Render(
				present.TruncateText(line, inner)))
		}
		lines = append(lines, "")
	}

	if overlay.Notice != "" {
		lines = append(lines, model.styles.Error().Render(
			present.TruncateText(overlay.Notice, inner)), "")
	}

	keys := model.sayKeys().
		name("Enter", describeImportRun(held)).
		name("Esc", "back to the form")
	said := present.TruncateText(keys.buildText(), width-4)
	model.recordCardBody()
	lines = model.appendCardKeyRow(lines, keys, said, cardBodyRow, cardBodyColumn)
	model.rememberCardKeys(keys)
	return model.renderCard(buildImportTitle(held), width, lines, plainCard)
}

// describeRowProblem returns one refused row as the review lists it.
func describeRowProblem(problem load.RowProblem) string {
	if problem.Column == "" {
		return problem.Reason
	}
	return problem.Column + ": " + problem.Reason
}

// describeImportRun describes what running the import writes.
func describeImportRun(held app.ImportRequest) string {
	if held.Running {
		return "writing…"
	}
	written := held.Report.Rows - held.Report.Refused
	return "write " + present.FormatRowCount(int64(written))
}
