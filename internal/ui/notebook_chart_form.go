package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
)

// A chart cell holds no text of its own, so it is written through a form: the cell it
// reads, the two columns it draws, the shape and the order. The columns offered are the
// columns of the result of the source cell, so a chart never names a column that result
// does not hold.

// chartFormLabelWidth is the width of the names of the rows of the form.
const chartFormLabelWidth = 16

// chartRowNumberChoice is the label of a chart that numbers its rows.
const chartRowNumberChoice = notebook.RowNumberLabel

// The two orders a chart draws its rows in.
const (
	chartFileOrder  = "result order"
	chartValueOrder = "value, highest first"
)

// openChartForm opens the form of the focused chart cell.
func (model *Model) openChartForm(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	cell := tab.Notebook.GetFocusedCell()
	spec := notebook.ReadChart(notebook.Cell{Attrs: cell.Attrs})
	shape := string(spec.Shape)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayChart,
		Chart: app.ChartRequest{
			Cell: cell.ID, Source: spec.Source, Label: spec.Label, Value: spec.Value,
			Shape: shape, SortsByValue: spec.SortsByValue, Top: spec.Top,
		},
		Draft: app.NewEditorBuffer("", 0),
	}
	return model, nil
}

// BuildChartFields returns the rows of the form of a chart cell.
func (model *Model) BuildChartFields(tab *app.Tab, overlay app.Overlay) []DialogField {
	held := overlay.Chart
	label := held.Label
	if label == "" {
		label = chartRowNumberChoice
	}
	order := chartFileOrder
	if held.SortsByValue {
		order = chartValueOrder
	}
	columns := model.listSourceColumns(tab, held.Source)
	return []DialogField{
		{
			Key: "source", Label: "source cell", Value: held.Source,
			Choices: listStatementCells(tab),
		},
		{
			Key: "label", Label: "label column", Value: label,
			Choices: append([]string{chartRowNumberChoice}, columns...),
		},
		{Key: "value", Label: "value column", Value: held.Value, Choices: columns},
		{
			Key: "shape", Label: "shape", Value: held.Shape,
			Choices: []string{string(notebook.ChartBar), string(notebook.ChartLine)},
		},
		{
			Key: "order", Label: "order", Value: order,
			Choices: []string{chartFileOrder, chartValueOrder},
		},
		{
			Key: "top", Label: "rows", Value: describeChartTop(held.Top),
			Choices: chartTopChoices,
		},
	}
}

// listStatementCells returns the id of every cell a chart can read.
func listStatementCells(tab *app.Tab) []string {
	ids := []string{}
	for _, cell := range tab.Notebook.Cells {
		if cell.RunsStatements() {
			ids = append(ids, cell.ID)
		}
	}
	return ids
}

// listSourceColumns returns the columns of the result of the source cell, and nothing where
// that cell has not run.
func (model *Model) listSourceColumns(tab *app.Tab, source string) []string {
	at := tab.Notebook.FindCellIndex(source)
	if at < 0 {
		return nil
	}
	cell := tab.Notebook.Cells[at]
	if tab.ReadCellOutcome(cell).Kind != app.CellDone {
		return nil
	}
	held := tab.Results.ResultAt(cell.FirstResult)
	if held == nil {
		return nil
	}
	names := make([]string, 0, len(held.State.Result.Columns))
	for _, column := range held.State.Result.Columns {
		names = append(names, present.SafeText(column.Name))
	}
	return names
}

// FindChartProblem returns why the form cannot be applied, and nothing where it can.
func (model *Model) FindChartProblem(tab *app.Tab, overlay app.Overlay) string {
	held := overlay.Chart
	if held.Source == "" {
		if len(listStatementCells(tab)) == 0 {
			return "no statement cell to read"
		}
		return "choose the cell the chart reads"
	}
	at := tab.Notebook.FindCellIndex(held.Source)
	if at < 0 {
		return "no cell named " + held.Source
	}
	if tab.ReadCellOutcome(tab.Notebook.Cells[at]).Kind != app.CellDone {
		return "run cell " + strconv.Itoa(at+1) + " for its columns"
	}
	if held.Value == "" {
		return "choose the column the chart draws"
	}
	return ""
}

// StepChartField moves the cursor of the form through its rows.
func StepChartField(overlay *app.Overlay, step int) {
	overlay.Field = clamp(overlay.Field+step, chartFieldCount)
}

// chartFieldCount is how many rows the form holds.
const chartFieldCount = 6

// chartEveryRow is the choice of a chart that draws every row it is given.
const chartEveryRow = "every row"

// chartTopChoices are the row caps the form offers.
var chartTopChoices = []string{chartEveryRow, "5", "10", "20", "50"}

// describeChartTop returns the row cap as the form shows it.
func describeChartTop(top int) string {
	if top <= 0 {
		return chartEveryRow
	}
	return strconv.Itoa(top)
}

// StepChartChoice steps the value of the row under the cursor.
func (model *Model) StepChartChoice(tab *app.Tab, overlay *app.Overlay, step int) {
	fields := model.BuildChartFields(tab, *overlay)
	field := fields[clamp(overlay.Field, len(fields))]
	if len(field.Choices) == 0 {
		return
	}
	next := stepThroughChoices(field.Choices, field.Value, step)

	switch field.Key {
	case "source":
		overlay.Chart.Source = next
		// The columns of another cell are other columns, so the two the form held are
		// dropped rather than kept over a result that does not hold them.
		overlay.Chart.Label, overlay.Chart.Value = "", ""
	case "label":
		overlay.Chart.Label = ""
		if next != chartRowNumberChoice {
			overlay.Chart.Label = next
		}
	case "value":
		overlay.Chart.Value = next
	case "shape":
		overlay.Chart.Shape = next
	case "order":
		overlay.Chart.SortsByValue = next == chartValueOrder
	case "top":
		overlay.Chart.Top = 0
		if count, err := strconv.Atoi(next); err == nil {
			overlay.Chart.Top = count
		}
	}
}

// stepThroughChoices returns the choice after this one, and wraps at the ends. A value that
// is none of the choices steps to the first one.
func stepThroughChoices(choices []string, held string, step int) string {
	at := 0
	for index, choice := range choices {
		if choice == held {
			at = index
		}
	}
	count := len(choices)
	return choices[((at+step)%count+count)%count]
}

// applyChartForm writes the form onto the cell it was opened for.
func (model *Model) applyChartForm(
	connection *app.Connection, tab *app.Tab, overlay app.Overlay,
) (tea.Model, tea.Cmd) {
	if problem := model.FindChartProblem(tab, overlay); problem != "" {
		connection.ShowError(problem)
		return model, nil
	}
	at := tab.Notebook.FindCellIndex(overlay.Chart.Cell)
	if at < 0 {
		connection.Overlay = app.Overlay{}
		return model, nil
	}
	cell := tab.Notebook.Cells[at]
	cell.Attrs = buildChartAttrs(overlay.Chart, cell.Attrs)
	cell.Folded = false
	tab.Notebook.Dirty = true
	connection.Overlay = app.Overlay{}
	connection.Show("chart of " + overlay.Chart.Value + " from cell " +
		strconv.Itoa(tab.Notebook.FindCellIndex(overlay.Chart.Source)+1))
	return model, nil
}

// buildChartAttrs returns the pairs of the fence of a chart cell, and keeps every pair the
// form does not hold.
func buildChartAttrs(held app.ChartRequest, kept []notebook.Attribute) []notebook.Attribute {
	own := map[string]bool{
		"source": true, "label": true, "value": true, "kind": true, "sort": true,
	}
	attrs := []notebook.Attribute{{Name: "source", Value: held.Source}}
	if held.Label != "" {
		attrs = append(attrs, notebook.Attribute{Name: "label", Value: held.Label})
	}
	attrs = append(attrs,
		notebook.Attribute{Name: "value", Value: held.Value},
		notebook.Attribute{Name: "kind", Value: held.Shape})
	if held.SortsByValue {
		attrs = append(attrs, notebook.Attribute{Name: "sort", Value: "desc"})
	}
	if held.Top > 0 {
		attrs = append(attrs,
			notebook.Attribute{Name: "top", Value: strconv.Itoa(held.Top)})
	}
	for _, attr := range kept {
		if !own[attr.Name] {
			attrs = append(attrs, attr)
		}
	}
	return attrs
}

// renderChartForm draws the form of a chart cell.
func (model *Model) renderChartForm(tab *app.Tab, overlay app.Overlay, width int) string {
	fields := model.BuildChartFields(tab, overlay)
	valueWidth := max(width-present.CardChrome-chartFormLabelWidth, 8)

	model.layout.formChoices = nil
	lines := make([]string, 0, len(fields)+3)
	for at, field := range fields {
		focused := at == clamp(overlay.Field, len(fields))
		marker := "  "
		labelStyle := model.styles.Muted()
		if focused {
			marker = present.FitText(model.icons.Icon(cfg.IconField), fieldMarkerWidth)
			labelStyle = model.styles.Accent()
		}
		written := model.styles.Muted().Render(
			present.TruncateText(describeFieldValue(field), valueWidth))
		if len(field.Choices) > 0 {
			written = model.renderChoiceField(field.Value, valueWidth, at,
				cardBodyRow+at, cardBodyColumn+chartFormLabelWidth, focused)
		}
		lines = append(lines, labelStyle.Render(marker+fitFieldLabel(
			field.Label, chartFormLabelWidth-present.MeasureText(marker)))+written)
	}

	// The problem line is always counted, so the card keeps its height.
	lines = append(lines, model.styles.Error().Render(
		present.TruncateText(model.FindChartProblem(tab, overlay), width-4)))

	keys := model.buildCardKeys(app.OverlayChart, keyScene{overlay: overlay})
	text := present.TruncateText(keys.buildText(), width-4)
	model.recordCardBody()
	lines = model.appendCardKeyRow(lines, keys, text, cardBodyRow, cardBodyColumn)
	model.rememberCardKeys(keys)
	model.layout.formRows = rowsHit{
		top: model.layout.cardBodyTop, count: len(fields),
		from: model.layout.cardBodyLeft - 1, to: model.layout.cardBodyLeft + width - 4,
	}
	return model.renderCard(" chart cell ", width, lines, plainCard)
}
