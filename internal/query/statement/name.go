package statement

import (
	"regexp"
	"strings"
)

var nameLine = regexp.MustCompile(`^\s*--\s?(.*)$`)

// FindQueryName returns the query name from the first line comment.
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

// ApplyQueryName replaces the initial query name comment. An empty name removes the comment.
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
