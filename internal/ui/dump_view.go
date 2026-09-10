package ui

import (
	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/present"
)

// The dump card contains the file and what the dump holds. A restore shows the picker
// first, then the file it runs.

// dumpLabelWidth is the column the mark and the name of a field share.
const dumpLabelWidth = 18

// renderDump draws the card of a dump: the picker, or the form.
func (model *Model) renderDump(overlay app.Overlay, width int) string {
	if overlay.Dump.Stage == app.DumpPick {
		return model.renderDumpPicker(overlay, width)
	}
	return model.renderDumpForm(overlay, width)
}

// renderDumpPicker draws the directory the file is chosen out of.
func (model *Model) renderDumpPicker(overlay app.Overlay, width int) string {
	inner := width - present.CardChrome
	lines := model.renderFilePicker(model.ActiveID(), inner)
	if overlay.Notice != "" {
		lines = append(lines, "", model.styles.Error().Render(
			present.TruncateText(overlay.Notice, inner)))
	}

	keys := model.buildCardKeys(app.OverlayDump, keyScene{overlay: overlay})
	text := present.TruncateText(keys.buildText(), width-4)
	model.recordCardBody()
	lines = model.appendCardKeyRow(lines, keys, text, cardBodyRow, cardBodyColumn)
	model.rememberCardKeys(keys)
	return model.renderCard(buildDumpTitle(overlay.Dump), width, lines, plainCard)
}

// renderDumpForm draws one row per setting, and the line that says what a run does.
func (model *Model) renderDumpForm(overlay app.Overlay, width int) string {
	fields := BuildDumpFields(overlay)
	valueWidth := max(width-present.CardChrome-dumpLabelWidth, 8)

	model.layout.formChoices = nil
	lines := make([]string, 0, len(fields)+4)
	for at, field := range fields {
		focused := at == overlay.Field
		marker := "  "
		labelStyle := model.styles.Muted()
		if focused {
			marker = present.FitText(model.icons.Icon(cfg.IconField), fieldMarkerWidth)
			labelStyle = model.styles.Accent()
		}

		value := field.Value
		written := model.styles.Muted().Render(
			present.TruncateText(describeFieldValue(field), valueWidth))
		switch {
		case len(field.Choices) > 0:
			written = model.renderChoiceField(value, valueWidth, at,
				cardBodyRow+at, cardBodyColumn+dumpLabelWidth, focused)
		case focused:
			written = model.renderField(
				app.NewEditorBuffer(value, len(value)), valueWidth, FieldLook{
					Ground: model.styles.Theme.Header, Ink: model.styles.Theme.Text,
					Focused: true, Placeholder: field.Label,
				})
		}
		lines = append(lines, labelStyle.Render(marker+
			fitFieldLabel(field.Label, dumpLabelWidth-present.MeasureText(marker)))+written)
	}

	note := model.styles.Muted().Render(
		present.TruncateText(describeDumpRun(overlay.Dump), width-4))
	if overlay.Dump.Running {
		note = model.renderProgress(overlay.Dump.Progress, width-4)
	}
	lines = append(lines, "", note)
	// The problem line is always counted, so the card keeps its height.
	text := FindDumpProblem(overlay)
	if text == "" {
		text = overlay.Notice
	}
	lines = append(lines, model.styles.Error().Render(
		present.TruncateText(text, width-4)))

	keys := model.buildCardKeys(app.OverlayDump, keyScene{overlay: overlay})
	keyRow := present.TruncateText(keys.buildText(), width-4)
	model.recordCardBody()
	lines = model.appendCardKeyRow(lines, keys, keyRow, cardBodyRow, cardBodyColumn)
	model.rememberCardKeys(keys)
	model.layout.formRows = rowsHit{
		top: model.layout.cardBodyTop, count: len(fields),
		from: model.layout.cardBodyLeft - 1, to: model.layout.cardBodyLeft + width - 4,
	}
	return model.renderCard(buildDumpTitle(overlay.Dump), width, lines, plainCard)
}

// describeDumpRun says what a run of the card does.
func describeDumpRun(held app.DumpRequest) string {
	if held.Mode == app.DumpRestore {
		return "every statement of the file runs on this connection, one at a time"
	}
	return "the tables are read a batch of rows at a time and written as SQL"
}

// describeDumpStep describes what Enter does.
func describeDumpStep(overlay app.Overlay) string {
	if overlay.Dump.Running {
		if overlay.Dump.Mode == app.DumpRestore {
			return "running…"
		}
		return "writing…"
	}
	if overlay.Dump.Mode == app.DumpRestore {
		return "run the file"
	}
	return "write the dump"
}

// buildDumpTitle names the card: what it does, and the target of a dump.
func buildDumpTitle(held app.DumpRequest) string {
	if held.Mode == app.DumpRestore {
		if held.Path == "" {
			return " restore "
		}
		return " restore " + present.TruncateText(baseName(held.Path), 40) + " "
	}
	if held.Target == "" {
		return " dump "
	}
	return " dump " + present.TruncateText(held.Target, 40) + " "
}
