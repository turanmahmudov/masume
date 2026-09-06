package build

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
)

// Predicate is a filter as SQL for one server, with its bind values.
type Predicate struct {
	Text   string
	Params []any
}

// RenderLiteral formats a value as an SQL literal.
func RenderLiteral(value any, dialect *query.Dialect, dataType string) string {
	switch held := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", held)
	case float32:
		return strconv.FormatFloat(float64(held), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(held, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(held)
	}
	return dialect.QuoteTextLiteral(core.FormatCell(value, dataType))
}

// writeStep builds one SQL predicate. bind is the parameter binder or literal formatter.
func writeStep(step core.FilterStep, dialect *query.Dialect, bind func(any) string) string {
	if step.Kind == core.FilterRaw {
		return step.Text
	}
	column := dialect.QuoteIdentifier(step.Column)
	switch step.Test {
	case core.FilterIsNull:
		return column + " is null"
	case core.FilterIsNotNull:
		return column + " is not null"
	case core.FilterEquals:
		return column + " = " + bind(step.Value)
	case core.FilterDiffers:
		return column + " <> " + bind(step.Value)
	}
	return ""
}

func joinSteps(steps []core.FilterStep, dialect *query.Dialect, bind func(any) string) string {
	written := make([]string, 0, len(steps))
	for _, step := range steps {
		text := writeStep(step, dialect, bind)
		if text != "" {
			written = append(written, text)
		}
	}
	return strings.Join(written, " and ")
}

// ComposeFilter builds a filter with parameters numbered from firstParamIndex.
func ComposeFilter(steps []core.FilterStep, dialect *query.Dialect, firstParamIndex int) *Predicate {
	if len(steps) == 0 {
		return nil
	}
	bound := query.NewBoundValues(dialect, firstParamIndex)
	text := joinSteps(steps, dialect, bound.Bind)
	return &Predicate{Text: text, Params: bound.Params}
}

// InlineFilter builds a filter with inline values for display.
func InlineFilter(steps []core.FilterStep, dialect *query.Dialect) *Predicate {
	if len(steps) == 0 {
		return nil
	}
	return &Predicate{Text: joinSteps(steps, dialect, func(value any) string {
		return RenderLiteral(value, dialect, "")
	})}
}
