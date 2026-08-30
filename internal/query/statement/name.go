package statement

import (
	"regexp"
	"strings"
)

var nameLine = regexp.MustCompile(`^\s*--\s?(.*)$`)

// FindQueryName returns the name of a query, which is the line comment at its
// start. It stays in the buffer, so it goes into the history and the saved queries,
// and survives a restart.
func FindQueryName(sql string) string {
	first := sql
	if before, _, ok := strings.Cut(sql, "\n"); ok {
		first = before
	}
	found := nameLine.FindStringSubmatch(first)
	if found == nil {
		return ""
	}
	return strings.TrimSpace(found[1])
}

// ApplyQueryName writes the name into the buffer, and replaces the one there. An
// empty name removes the line.
func ApplyQueryName(sql, name string) string {
	trimmed := strings.TrimSpace(name)
	lines := strings.Split(sql, "\n")
	body := lines
	if FindQueryName(sql) != "" {
		body = lines[1:]
	}
	if trimmed == "" {
		return strings.Join(body, "\n")
	}
	return strings.Join(append([]string{"-- " + trimmed}, body...), "\n")
}
