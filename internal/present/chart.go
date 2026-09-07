package present

import (
	"slices"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
)

// A chart draws one column of a result against another. The rows are read once, and the
// screen and the report both draw the same rows.

// ChartRow is one row of a chart.
type ChartRow struct {
	Label string
	Value float64
	// The value as the result held it.
	Written string
}

// BuildChartRows returns the rows of a chart, and the reason a result cannot be drawn.
func BuildChartRows(
	columns []query.ResultColumn, rows [][]any, label, value string, sortsByValue bool,
) ([]ChartRow, string) {
	labelAt := FindColumnIndex(columns, label)
	valueAt := FindColumnIndex(columns, value)
	if valueAt < 0 {
		if value == "" {
			return nil, "this chart names no value column"
		}
		return nil, "the result has no column named " + value
	}

	built := make([]ChartRow, 0, len(rows))
	for index, row := range rows {
		held := ChartRow{Label: strconv.Itoa(index + 1)}
		if labelAt >= 0 && labelAt < len(row) {
			held.Label = SafeText(
				core.CollapseWhitespace(core.FormatCell(row[labelAt], "")))
		}
		if valueAt >= len(row) {
			continue
		}
		number, isNumber := ReadChartValue(row[valueAt])
		if !isNumber {
			continue
		}
		held.Value = number
		held.Written = SafeText(core.FormatCell(row[valueAt], ""))
		built = append(built, held)
	}
	if len(built) == 0 {
		return nil, "the result holds no number to draw"
	}
	if sortsByValue {
		slices.SortStableFunc(built, func(left, right ChartRow) int {
			switch {
			case left.Value > right.Value:
				return -1
			case left.Value < right.Value:
				return 1
			}
			return 0
		})
	}
	return built, ""
}

// FindColumnIndex returns the column of that name, and -1 where the result has none.
func FindColumnIndex(columns []query.ResultColumn, name string) int {
	if name == "" {
		return -1
	}
	for at, column := range columns {
		if strings.EqualFold(column.Name, name) {
			return at
		}
	}
	return -1
}

// ChartBounds is the range a chart draws over: the lowest value, the highest, and the
// share of the bar the zero line stands at.
type ChartBounds struct {
	Lowest  float64
	Highest float64
	// HoldsNegative is true where a value is below zero, and the bars draw from a line
	// inside the bar rather than from its left edge.
	HoldsNegative bool
}

// ReadChartBounds returns the range of the rows.
func ReadChartBounds(rows []ChartRow) ChartBounds {
	held := ChartBounds{}
	for at, row := range rows {
		if at == 0 {
			held.Lowest, held.Highest = row.Value, row.Value
			continue
		}
		held.Lowest = min(held.Lowest, row.Value)
		held.Highest = max(held.Highest, row.Value)
	}
	held.HoldsNegative = held.Lowest < 0
	return held
}

// BuildChartCells returns how many cells of a bar are filled and where they start. A chart
// of values below zero draws from the zero line, so a bar to the left of it is a loss.
func BuildChartCells(value float64, bounds ChartBounds, width int) (int, int) {
	width = max(width, 0)
	if !bounds.HoldsNegative {
		return 0, resolveBarCells(value, bounds.Highest, width)
	}
	span := bounds.Highest - bounds.Lowest
	if span <= 0 {
		return 0, 0
	}
	zero := resolveBarCells(-bounds.Lowest, span, width)
	if value < 0 {
		from := zero - resolveBarCells(-value, span, width)
		return max(from, 0), zero - max(from, 0)
	}
	return zero, min(resolveBarCells(value, span, width), width-zero)
}

// resolveBarCells returns the cells a value fills of a whole, rounded to the nearest.
func resolveBarCells(value, of float64, width int) int {
	if of <= 0 {
		return 0
	}
	return max(min(int(value/of*float64(width)+0.5), width), 0)
}

// BuildChartBars returns one bar per row as plain text.
func BuildChartBars(rows []ChartRow, labelWidth, barWidth int) []string {
	bounds := ReadChartBounds(rows)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		from, filled := BuildChartCells(row.Value, bounds, barWidth)
		bar := strings.Repeat(" ", from) + strings.Repeat(meterFilled, filled) +
			strings.Repeat(meterEmpty, max(barWidth-from-filled, 0))
		lines = append(lines,
			FitText(TruncateText(row.Label, labelWidth-1), labelWidth)+bar+"  "+row.Written)
	}
	return lines
}

// ReadChartValue reads one cell of a result as a number.
func ReadChartValue(value any) (float64, bool) {
	switch held := value.(type) {
	case nil:
		return 0, false
	case float64:
		return held, true
	case float32:
		return float64(held), true
	case int:
		return float64(held), true
	case int32:
		return float64(held), true
	case int64:
		return float64(held), true
	case uint64:
		return float64(held), true
	case bool:
		if held {
			return 1, true
		}
		return 0, true
	case string:
		return readChartNumber(held)
	case []byte:
		return readChartNumber(string(held))
	}
	return readChartNumber(core.FormatCell(value, ""))
}

// readChartNumber reads text as a number.
func readChartNumber(written string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(written), 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

// FormatChartValue returns one number as a reader sees it.
func FormatChartValue(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
