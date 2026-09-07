package notebook

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// A statement cell can name another cell in place of a relation. The reference expands to
// the statement of that cell in parentheses, before the values of the `:name` marks are
// bound. It carries no rows: the statement of the named cell runs again inside this one.

// referenceMark matches one `{{cell:id}}` reference.
var referenceMark = regexp.MustCompile(`\{\{\s*cell:\s*([^}\s]+)\s*\}\}`)

// referenceDepth is how many references deep one statement may reach.
const referenceDepth = 8

// ErrReference is the sentinel for a reference that cannot be expanded.
var ErrReference = errors.New("cell reference")

// HoldsReference is true for a statement that names another cell.
func HoldsReference(source string) bool {
	return referenceMark.MatchString(source)
}

// ExpandReferences replaces every `{{cell:id}}` with the statement of that cell, wrapped in
// parentheses. Sources holds the statement of every cell a reference may name.
func ExpandReferences(source string, sources map[string]string) (string, error) {
	return expandReferences(source, sources, nil)
}

// expandReferences expands one statement, with the chain of cells it was reached through.
func expandReferences(
	source string, sources map[string]string, chain []string,
) (string, error) {
	if !HoldsReference(source) {
		return source, nil
	}
	if len(chain) >= referenceDepth {
		return "", fmt.Errorf("%w: %s reaches more than %d cells deep",
			ErrReference, strings.Join(chain, " → "), referenceDepth)
	}

	var failure error
	written := referenceMark.ReplaceAllStringFunc(source, func(match string) string {
		id := referenceMark.FindStringSubmatch(match)[1]
		if held := findIndexOfName(chain, id); held >= 0 {
			failure = fmt.Errorf("%w: %s names itself",
				ErrReference, strings.Join(append(chain[held:], id), " → "))
			return match
		}
		held, found := sources[id]
		if !found {
			failure = fmt.Errorf("%w: no statement cell is named %s", ErrReference, id)
			return match
		}
		inside, err := expandReferences(held, sources, append(chain, id))
		if err != nil {
			failure = err
			return match
		}
		return "(" + cutLeadingComments(inside) + ")"
	})
	if failure != nil {
		return "", failure
	}
	return written, nil
}

// indexOfName returns where this id stands in the chain, and -1 where it does not.
func findIndexOfName(chain []string, id string) int {
	for at, held := range chain {
		if held == id {
			return at
		}
	}
	return -1
}

// cutLeadingComments returns the statement without the comment lines that open it. The
// first line of a cell names it, and a name inside the parentheses of a subquery is noise.
func cutLeadingComments(source string) string {
	lines := strings.Split(strings.TrimSpace(source), "\n")
	at := 0
	for at < len(lines) {
		trimmed := strings.TrimSpace(lines[at])
		if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
			break
		}
		at++
	}
	return strings.TrimSpace(strings.Join(lines[at:], "\n"))
}
