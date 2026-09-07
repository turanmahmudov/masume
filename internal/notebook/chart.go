package notebook

import (
	"strconv"
	"strings"
)

// A chart cell draws the result of another cell. It holds no data of its own: the source
// cell, the two columns and the shape are pairs of its fence.

// ChartShape is how a chart draws its values.
type ChartShape string

// The shapes a chart cell draws.
const (
	// ChartBar draws one bar per row.
	ChartBar ChartShape = "bar"
	// ChartLine draws every value in one row of blocks.
	ChartLine ChartShape = "line"
)

// ChartSpec is what a chart cell draws.
type ChartSpec struct {
	// The id of the cell whose result the chart reads.
	Source string
	Label  string
	Value  string
	Shape  ChartShape
	// True while the rows are sorted by the value, largest first.
	SortsByValue bool
	// Top is how many rows the chart draws. Zero draws every row it is given.
	Top int
}

// ReadChart returns what the fence of a chart cell asks for.
func ReadChart(cell Cell) ChartSpec {
	spec := ChartSpec{Shape: ChartBar}
	if source, held := cell.FindAttr("source"); held {
		spec.Source = source
	}
	if label, held := cell.FindAttr("label"); held {
		spec.Label = label
	}
	if value, held := cell.FindAttr("value"); held {
		spec.Value = value
	}
	if kind, held := cell.FindAttr("kind"); held && ChartShape(kind) == ChartLine {
		spec.Shape = ChartLine
	}
	if sort, held := cell.FindAttr("sort"); held {
		spec.SortsByValue = sort == "desc"
	}
	if top, held := cell.FindAttr("top"); held {
		if count, err := strconv.Atoi(top); err == nil && count > 0 {
			spec.Top = count
		}
	}
	return spec
}

// RowNumberLabel is what a chart draws in place of a label column.
const RowNumberLabel = "row number"

// DescribeChart returns the settings of a chart cell in one line.
func DescribeChart(spec ChartSpec) string {
	label := spec.Label
	if label == "" {
		label = RowNumberLabel
	}
	parts := []string{
		"source " + describeOrNothing(spec.Source), "label " + label,
		"value " + describeOrNothing(spec.Value), string(spec.Shape),
	}
	if spec.SortsByValue {
		parts = append(parts, "value order")
	}
	if spec.Top > 0 {
		parts = append(parts, "top "+strconv.Itoa(spec.Top))
	}
	return strings.Join(parts, " · ")
}

// orNothing returns the text, or a mark for an empty one.
func describeOrNothing(text string) string {
	if text == "" {
		return "not set"
	}
	return text
}

// NameChart returns the name of a chart cell: its two columns and its shape.
func NameChart(spec ChartSpec) string {
	if spec.Value == "" {
		return "chart, not set"
	}
	if spec.Label == "" {
		return spec.Value + " by " + RowNumberLabel + " · " + string(spec.Shape)
	}
	return spec.Value + " by " + spec.Label + " · " + string(spec.Shape)
}
