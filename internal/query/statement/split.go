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

// closingWords follow `end` where they close a construct that opened no block of its own,
// such as `end if` and `end loop` inside a routine. `end case` closes the `case` that opened
// one, so the word after it opens nothing either.
var closingWords = map[string]bool{
	"if": true, "loop": true, "while": true, "repeat": true, "case": true,
}

// blockState counts the blocks one statement holds while the words of a buffer are read
// once. A trigger body and a stored routine hold their statements in a `begin` block, and
// the semicolons inside it end no statement.
type blockState struct {
	depth int
	// words is how many words the statement holds so far.
	words        int
	holdsBegin   bool
	previousWord string
	lastWord     string
}

// readWord takes one word of the statement.
func (state *blockState) readWord(text string) {
	switch {
	case state.previousWord == "end" && closingWords[text]:
		// `end if` and `end loop` close what opened no block. `end case` closed the
		// `case` already, so this word opens none either.
		if text != "case" {
			state.depth++
		}
	case text == "begin":
		// The first word of a statement is the `begin` of a transaction.
		if state.words > 0 {
			state.depth++
			state.holdsBegin = true
		}
	case text == "case":
		state.depth++
	case text == "end":
		if state.depth > 0 {
			state.depth--
		}
	}
	state.words++
	state.previousWord, state.lastWord = text, text
}

// endsStatement is true where a semicolon here ends the statement.
func (state *blockState) endsStatement() bool { return state.depth == 0 }

// keepsTerminator is true for a statement whose body is a `begin` block: `end;` closes the
// body, so the semicolon belongs to the statement.
func (state *blockState) keepsTerminator() bool {
	return state.holdsBegin && state.lastWord == "end"
}

// SplitStatementRanges splits at semicolons outside literals, comments, parentheses and
// blocks, preserving statement byte ranges. It reads the buffer once.
func SplitStatementRanges(sql string, flavour syntax.SyntaxFlavour) []StatementRange {
	ranges := []StatementRange{}
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

	start := 0
	walkStatementEnds(sql, flavour, func(at int, keepsTerminator bool) {
		end := at
		if keepsTerminator {
			end++
		}
		push(start, end)
		start = at + 1
	})
	push(start, len(sql))
	return ranges
}

// FindLastStatementEnd returns the offset after the last statement the text holds whole. A
// reader that takes a file a block at a time keeps the text after it for the next read.
func FindLastStatementEnd(sql string, flavour syntax.SyntaxFlavour) int {
	cut := 0
	walkStatementEnds(sql, flavour, func(at int, _ bool) { cut = at + 1 })
	return cut
}

// walkStatementEnds reads the buffer once and reports every semicolon that ends a statement,
// with whether that statement keeps the semicolon.
func walkStatementEnds(
	sql string, flavour syntax.SyntaxFlavour, onEnd func(at int, keepsTerminator bool),
) {
	tokens := syntax.ReadCodeTokens(sql, flavour)
	hits := syntax.FindKeywordsIn(tokens, []string{";"})
	state := blockState{}
	at := 0

	for _, token := range tokens {
		if at < len(hits) && token.Start == hits[at].Start {
			at++
			if !state.endsStatement() {
				continue
			}
			onEnd(token.Start, state.keepsTerminator())
			state = blockState{}
			continue
		}
		if syntax.IsWordKind(token.Kind) {
			state.readWord(token.Text)
		}
	}
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
