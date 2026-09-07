package ui

import (
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
)

// A chart cell reads the result of another cell of the same notebook. It holds no rows of
// its own, so it draws nothing until that cell has run.

// The widths of one row of a chart.
const (
	chartLabelWidth = 16
	chartValueWidth = 12
	// chartRowLimit is how many bars one chart draws.
	chartRowLimit = 12
)

// buildChartLines returns the rows a chart cell draws, already styled.
func (model *Model) buildChartLines(
	tab *app.Tab, cell *app.NotebookCell, width int,
) []string {
	theme := model.styles.Theme
	spec := notebook.ReadChart(notebook.Cell{Attrs: cell.Attrs})
	lines := []string{paintText(theme.Muted, theme.Panel,
		present.FitText(notebook.DescribeChart(spec), width))}

	points, problem := readCellChartRows(tab, spec)
	if problem != "" {
		return append(lines, paintText(theme.Warning, theme.Panel,
			present.FitText(problem, width)))
	}
	points = capChartRows(points, spec.Top)
	if spec.Shape == notebook.ChartLine {
		return append(lines, model.renderChartLine(points, width)...)
	}
	return append(lines, model.renderChartBars(points, width)...)
}

// readCellChartRows returns the rows of the source cell, and why the chart cannot draw them.
func readCellChartRows(
	tab *app.Tab, spec notebook.ChartSpec,
) ([]present.ChartRow, string) {
	if spec.Source == "" {
		return nil, "this chart names no source cell"
	}
	at := tab.Notebook.FindCellIndex(spec.Source)
	if at < 0 {
		return nil, "no cell named " + spec.Source
	}
	source := tab.Notebook.Cells[at]
	if tab.ReadCellOutcome(source).Kind != app.CellDone {
		return nil, "run cell " + strconv.Itoa(at+1) + " first"
	}
	held := tab.Results.ResultAt(source.FirstResult)
	if held == nil {
		return nil, "cell " + strconv.Itoa(at+1) + " returned no rows"
	}
	return present.BuildChartRows(held.State.Result.Columns, held.State.Result.Rows,
		spec.Label, spec.Value, spec.SortsByValue)
}

// capChartRows returns the first rows of a chart, where its fence caps them.
func capChartRows(points []present.ChartRow, top int) []present.ChartRow {
	if top <= 0 || top >= len(points) {
		return points
	}
	return points[:top]
}

// renderChartBars draws one bar per row. A chart of values below zero draws them from the
// zero line, so a loss reads as a bar on the other side of it.
func (model *Model) renderChartBars(points []present.ChartRow, width int) []string {
	theme := model.styles.Theme
	bounds := present.ReadChartBounds(points)
	barWidth := max(width-chartLabelWidth-chartValueWidth-2, 4)

	lines := make([]string, 0, len(points)+1)
	for at, point := range points {
		if at >= chartRowLimit {
			lines = append(lines, paintText(theme.Faint, theme.Panel,
				present.FitText(present.FormatCount(int64(len(points)-chartRowLimit))+
					" more rows", width)))
			break
		}
		from, filled := present.BuildChartCells(point.Value, bounds, barWidth)
		ink := theme.Accent
		if point.Value < 0 {
			ink = theme.Error
		}
		lines = append(lines,
			paintText(theme.Text, theme.Panel,
				present.FitText(present.TruncateText(point.Label, chartLabelWidth-1),
					chartLabelWidth))+
				paintText(theme.Faint, theme.Panel, strings.Repeat("·", from))+
				paintText(ink, theme.Panel, strings.Repeat("▇", filled))+
				paintText(theme.Faint, theme.Panel,
					strings.Repeat("░", max(barWidth-from-filled, 0)))+
				paintText(theme.AccentAlt, theme.Panel,
					present.FitText("  "+point.Written, chartValueWidth+2)))
	}
	return lines
}

// renderChartLine draws every value in one row of blocks.
func (model *Model) renderChartLine(points []present.ChartRow, width int) []string {
	theme := model.styles.Theme
	values := make([]float64, 0, len(points))
	for _, point := range points {
		values = append(values, point.Value)
	}
	lowest, highest := values[0], values[0]
	for _, value := range values {
		lowest, highest = min(lowest, value), max(highest, value)
	}
	return []string{
		paintText(theme.Accent, theme.Panel,
			present.FitText(present.BuildSparkline(values), width)),
		paintText(theme.Muted, theme.Panel, present.FitText(
			points[0].Label+" … "+points[len(points)-1].Label+
				"   low "+present.FormatChartValue(lowest)+
				"   high "+present.FormatChartValue(highest), width)),
	}
}
