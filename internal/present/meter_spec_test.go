package present_test

import (
	"math"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/present"
)

// The plan view draws the share of the run one node took as a bar of eight cells, and the
// dashboard draws a count against its limit the same way. One meter serves both, so the two
// views never disagree about what a filled cell means.
func TestBuildMeterFillsTheCellsTheValueCovers(t *testing.T) {
	for _, held := range []struct {
		name  string
		value float64
		of    float64
		width int
		want  string
	}{
		{"nothing of the whole", 0, 1, 8, "░░░░░░░░"},
		{"the whole of it", 1, 1, 8, "▇▇▇▇▇▇▇▇"},
		{"half of it", 0.5, 1, 8, "▇▇▇▇░░░░"},
		{"a count against its limit", 84, 100, 10, "▇▇▇▇▇▇▇▇░░"},
		{"a limit that is reached", 100, 100, 10, "▇▇▇▇▇▇▇▇▇▇"},
		// The cells round to the nearest, so a value just over half a cell fills it and
		// one just under leaves it empty.
		{"just over half a cell", 0.07, 1, 8, "▇░░░░░░░"},
		{"just under half a cell", 0.05, 1, 8, "░░░░░░░░"},
		// A value the server reports above its own limit, which happens while a limit is
		// being lowered, fills the bar rather than running past its end.
		{"more than the whole", 120, 100, 10, "▇▇▇▇▇▇▇▇▇▇"},
		{"less than nothing", -5, 100, 10, "░░░░░░░░░░"},
		// A whole of zero is no share at all, and dividing by it would report every bar
		// as full.
		{"a whole of zero", 5, 0, 8, "░░░░░░░░"},
		{"a whole below zero", 5, -1, 8, "░░░░░░░░"},
		{"no width at all", 0.5, 1, 0, ""},
		{"a width below zero", 0.5, 1, -4, ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			if built := present.BuildMeter(held.value, held.of, held.width); built != held.want {
				t.Errorf("the meter reads %q, wanted %q", built, held.want)
			}
		})
	}
}

// A meter stands in a column of a fixed width, so it must measure that width whatever it
// holds. A bar one cell short breaks the border of the card it is drawn in.
func TestBuildMeterAlwaysMeasuresItsWidth(t *testing.T) {
	for _, width := range []int{1, 4, 8, 10, 24} {
		for step := -1; step <= 21; step++ {
			value := float64(step) / 20.0
			built := present.BuildMeter(value, 1, width)
			if measured := present.MeasureText(built); measured != width {
				t.Errorf("%v of 1 at width %d measures %d: %q",
					value, width, measured, built)
			}
		}
	}
}

// A share the app never means to draw still reaches the meter where a server reports a run
// time of zero, and it must answer a bar rather than panic or a row of the wrong width.
func TestBuildMeterAnswersAValueThatIsNoNumber(t *testing.T) {
	for _, held := range []struct {
		name  string
		value float64
	}{
		{"no number", math.NaN()},
		{"without limit", math.Inf(1)},
		{"below every limit", math.Inf(-1)},
	} {
		t.Run(held.name, func(t *testing.T) {
			built := present.BuildMeter(held.value, 1, 8)
			if present.MeasureText(built) != 8 {
				t.Errorf("the meter reads %q, which is not eight cells", built)
			}
			if strings.Trim(built, "▇░") != "" {
				t.Errorf("the meter reads %q, which holds more than its cells", built)
			}
		})
	}
}
