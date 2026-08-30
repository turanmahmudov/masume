package statement

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// parameterMark is a `:name` parameter, and its place in the statement.
type parameterMark struct {
	name  string
	start int
	end   int
}

// readParameterMarks returns every `:name`, in order. A `::` is a cast, and the name
// must follow the colon directly.
func readParameterMarks(sql string) []parameterMark {
	tokens := []syntax.Token{}
	for _, token := range syntax.Tokenize(sql, syntax.FlavourStandard) {
		if token.Kind != syntax.TokenComment {
			tokens = append(tokens, token)
		}
	}

	marks := []parameterMark{}
	for index, token := range tokens {
		if token.Kind != syntax.TokenOperator || sql[token.Start:token.End] != ":" {
			continue
		}
		if index+1 >= len(tokens) {
			continue
		}
		next := tokens[index+1]
		if !syntax.IsWordKind(next.Kind) || next.Start != token.End {
			continue
		}
		marks = append(marks, parameterMark{
			name: sql[next.Start:next.End], start: token.Start, end: next.End,
		})
	}
	return marks
}

// FindQueryParameters returns the parameters of the statement, each one once, in the
// order written.
func FindQueryParameters(sql string) []string {
	seen := map[string]bool{}
	names := []string{}
	for _, mark := range readParameterMarks(sql) {
		key := strings.ToLower(mark.name)
		if seen[key] {
			continue
		}
		seen[key] = true
		names = append(names, mark.name)
	}
	return names
}

// ErrParameter marks a fault in the values a statement binds.
var ErrParameter = errors.New("parameter")

func newParameterError(format string, parts ...any) error {
	return fmt.Errorf("%w: %s", ErrParameter, fmt.Sprintf(format, parts...))
}

// rewriteParameterMarks replaces every mark by what `write` returns for its value.
func rewriteParameterMarks(
	sql string, values map[string]any, write func(any) string,
) (string, error) {
	var written strings.Builder
	cursor := 0

	for _, mark := range readParameterMarks(sql) {
		key := strings.ToLower(mark.name)
		value, held := values[key]
		if !held {
			return "", newParameterError("no value for :%s", mark.name)
		}
		written.WriteString(sql[cursor:mark.start])
		written.WriteString(write(value))
		cursor = mark.end
	}
	written.WriteString(sql[cursor:])
	return written.String(), nil
}

// BindQueryParameters binds every mark. A name written twice is bound twice with the
// same value, which both servers accept.
func BindQueryParameters(
	sql string, values map[string]any, dialect *query.Dialect, firstParamIndex int,
) (EffectiveStatement, error) {
	bound := query.NewBoundValues(dialect, firstParamIndex)
	written, err := rewriteParameterMarks(sql, values, bound.Bind)
	if err != nil {
		return EffectiveStatement{}, err
	}
	return EffectiveStatement{SQL: written, Params: bound.Params}, nil
}

// InlineQueryParameters writes the values into the statement, for the display and for
// the planner, never for an ordinary run.
func InlineQueryParameters(
	sql string, values map[string]any, dialect *query.Dialect,
) (string, error) {
	return rewriteParameterMarks(sql, values, func(value any) string {
		return build.RenderLiteral(value, dialect, "")
	})
}

// ResolveParameterValues keys the values in lower case. A name still in the statement
// keeps its value, and a name that is gone is dropped.
func ResolveParameterValues(names []string, current map[string]any) map[string]any {
	next := map[string]any{}
	for _, name := range names {
		key := strings.ToLower(name)
		if held, present := current[key]; present && held != nil {
			next[key] = held
			continue
		}
		next[key] = ""
	}
	return next
}

// BuildParameterForm writes the form the user fills in: the values as indented JSON.
// The names keep the order the statement writes them in, so the form reads in the
// order the user typed.
func BuildParameterForm(names []string, values map[string]any) string {
	lines := make([]string, 0, len(names))
	for _, name := range names {
		held, present := values[strings.ToLower(name)]
		if !present || held == nil {
			held = ""
		}
		key, keyErr := json.Marshal(name)
		value, valueErr := json.Marshal(held)
		if keyErr != nil || valueErr != nil {
			return "{}"
		}
		lines = append(lines, "  "+string(key)+": "+string(value))
	}
	if len(lines) == 0 {
		return "{}"
	}
	return "{\n" + strings.Join(lines, ",\n") + "\n}"
}

// ReadRowForm reads the form of a whole new row back. The names are the columns of the row,
// so they keep the case they were written in. A value keeps its JSON type: a number stays a
// number, and `null` stays a null.
func ReadRowForm(text string) (map[string]any, error) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || parsed == nil {
		return nil, newParameterError("the new row must be a JSON object")
	}
	return parsed, nil
}

// ReadParameterForm reads the form back. A value keeps its JSON type: a number stays
// a number, and `null` stays a null.
func ReadParameterForm(text string) (map[string]any, error) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || parsed == nil {
		return nil, newParameterError("the values must be a JSON object")
	}
	values := map[string]any{}
	for name, value := range parsed {
		values[strings.ToLower(name)] = value
	}
	return values, nil
}
