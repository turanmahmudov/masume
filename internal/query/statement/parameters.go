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

// readParameterMarks finds :name parameters, excluding :: casts and adjacent object keys such as {a:true}.
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
		if index > 0 && closesObjectKey(tokens[index-1], token) {
			continue
		}
		marks = append(marks, parameterMark{
			name: sql[next.Start:next.End], start: token.Start, end: next.End,
		})
	}
	return marks
}

// FindQueryParameters returns unique parameters in statement order.
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

// closesObjectKey detects a colon directly after a word, string, quoted identifier, or number.
func closesObjectKey(previous syntax.Token, colon syntax.Token) bool {
	if previous.End != colon.Start {
		return false
	}
	return syntax.IsWordKind(previous.Kind) ||
		previous.Kind == syntax.TokenString ||
		previous.Kind == syntax.TokenQuoted ||
		previous.Kind == syntax.TokenNumber
}

// ErrParameter is the sentinel for parameter errors.
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

// BindQueryParameters binds each parameter occurrence separately, including repeated names.
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

// InlineQueryParameters substitutes literals for display and plan requests.
func InlineQueryParameters(
	sql string, values map[string]any, dialect *query.Dialect,
) (string, error) {
	return rewriteParameterMarks(sql, values, func(value any) string {
		return build.RenderLiteral(value, dialect, "")
	})
}

// ResolveParameterValues keeps current values for requested names, indexed by lowercase names.
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

// BuildParameterForm builds indented JSON in parameter order.
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

// FindFirstFormValue returns the first string value offset, or zero when the first value is not a string.
func FindFirstFormValue(written string) int {
	at := strings.Index(written, ": ")
	if at < 0 || at+2 >= len(written) || written[at+2] != '"' {
		return 0
	}
	return at + 3
}

// ReadRowForm parses a JSON object, preserving column name case and JSON value types.
func ReadRowForm(text string) (map[string]any, error) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || parsed == nil {
		return nil, newParameterError("the new row must be a JSON object")
	}
	return parsed, nil
}

// ReadParameterForm parses a JSON object with lowercase parameter names and preserved JSON value types.
func ReadParameterForm(text string) (map[string]any, error) {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || parsed == nil {
		return nil, newParameterError("parameter values must be a JSON object")
	}
	values := map[string]any{}
	for name, value := range parsed {
		values[strings.ToLower(name)] = value
	}
	return values, nil
}
