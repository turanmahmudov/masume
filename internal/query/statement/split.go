package statement

import (
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// StatementRange is one statement of a buffer, and its place in it.
type StatementRange struct {
	Text  string
	Start int
	End   int
}

// SplitStatementRanges splits at semicolons outside literals, comments, and parentheses, preserving statement byte ranges.
func SplitStatementRanges(sql string, flavour syntax.SyntaxFlavour) []StatementRange {
	hits := syntax.FindTopLevelKeywords(sql, []string{";"}, flavour)
	ranges := []StatementRange{}
	start := 0

	push := func(from, to int) {
		if from > to {
			return
		}
		slice := sql[from:to]
		leading := len(slice) - len(strings.TrimLeft(slice, " \t\r\n\v\f"))
		text := strings.TrimSpace(slice)
		if text != "" && syntax.HoldsCode(text, flavour) {
			ranges = append(ranges, StatementRange{
				Text: text, Start: from + leading, End: from + leading + len(text),
			})
		}
	}

	for _, hit := range hits {
		push(start, hit.Start)
		start = hit.Start + 1
	}
	push(start, len(sql))
	return ranges
}

// SplitStatements returns the statements a buffer holds, in run order.
func SplitStatements(sql string, flavour syntax.SyntaxFlavour) []string {
	ranges := SplitStatementRanges(sql, flavour)
	statements := make([]string, 0, len(ranges))
	for _, one := range ranges {
		statements = append(statements, one.Text)
	}
	return statements
}

// ReadStatementAtOffset returns the statement at or before the caret, or the first statement when the caret precedes all statements.
func ReadStatementAtOffset(sql string, offset int, flavour syntax.SyntaxFlavour) string {
	ranges := SplitStatementRanges(sql, flavour)
	if len(ranges) <= 1 {
		return strings.TrimSpace(sql)
	}
	for _, one := range ranges {
		if offset >= one.Start && offset <= one.End {
			return one.Text
		}
	}
	for _, one := range slices.Backward(ranges) {
		if one.End < offset {
			return one.Text
		}
	}
	return ranges[0].Text
}
