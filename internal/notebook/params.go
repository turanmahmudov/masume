package notebook

import (
	"encoding/json"
	"sort"
	"strings"
)

// ReadParameters reads the `name = value` lines of a parameter cell. A value that is not
// JSON is kept as text. The names are lower case, as the binder wants them.
func ReadParameters(source string) (map[string]any, []string) {
	values := map[string]any{}
	problems := []string{}
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name, written, held := strings.Cut(trimmed, "=")
		if !held {
			problems = append(problems, "no value for "+trimmed)
			continue
		}
		values[strings.ToLower(strings.TrimSpace(name))] = readValue(strings.TrimSpace(written))
	}
	return values, problems
}

// readValue returns the value of one line: a JSON value, the text inside a pair of quotes,
// or the text as it was written. A reader of SQL writes 'paid' as readily as "paid", and
// both mean the same text.
func readValue(written string) any {
	var held any
	if err := json.Unmarshal([]byte(written), &held); err == nil {
		return held
	}
	if inside, quoted := cutQuotes(written); quoted {
		return inside
	}
	return written
}

// cutQuotes returns the text inside a matching pair of single or double quotes.
func cutQuotes(written string) (string, bool) {
	if len(written) < 2 {
		return written, false
	}
	held := written[0]
	if held != '\'' && held != '"' {
		return written, false
	}
	if written[len(written)-1] != held {
		return written, false
	}
	return written[1 : len(written)-1], true
}

// WriteParameters returns the lines of a parameter cell, one name each.
func WriteParameters(values map[string]any) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	lines := make([]string, 0, len(names))
	for _, name := range names {
		written, err := json.Marshal(values[name])
		if err != nil {
			continue
		}
		lines = append(lines, name+" = "+string(written))
	}
	return strings.Join(lines, "\n")
}

// DescribeParameters returns the values of a parameter cell in one line.
func DescribeParameters(source string) string {
	values, _ := ReadParameters(source)
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+" = "+describeValue(values[name]))
	}
	return strings.Join(parts, " · ")
}

// describeValue returns one value as a reader sees it, without JSON quotes.
func describeValue(value any) string {
	if text, isText := value.(string); isText {
		return text
	}
	written, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(written)
}
