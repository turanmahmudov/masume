package ui

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// What the editor writes and reads around the caret: the indent of a new line, the guides
// of the indent steps, and the bracket that closes the one at the caret.

// indentWidth is how many spaces one indent step has.
const indentWidth = 2

const (
	openingBrackets = "(["
	closingBrackets = ")]"
)

// readIndent returns the spaces or tabs the line opens with.
func readIndent(line string) string {
	for at := 0; at < len(line); at++ {
		if line[at] != ' ' && line[at] != '\t' {
			return line[:at]
		}
	}
	return line
}

// readLineAt returns the line the caret is on, and where that line starts in the text.
func readLineAt(text string, caret int) (string, int) {
	start := strings.LastIndexByte(text[:caret], '\n') + 1
	end := strings.IndexByte(text[start:], '\n')
	if end == -1 {
		return text[start:], start
	}
	return text[start : start+end], start
}

// countsUnclosed is true if the text of the line leaves a bracket open.
func countsUnclosed(written string) bool {
	depth := 0
	for _, character := range written {
		switch {
		case strings.ContainsRune(openingBrackets, character):
			depth++
		case strings.ContainsRune(closingBrackets, character):
			depth--
		}
	}
	return depth > 0
}

// OpenLineWithIndent opens a line below the caret. It keeps the indent, and adds one step if
// the line above left a bracket open. A closing bracket after the caret moves to its own line.
// It returns the new text and where the caret stands in it.
func OpenLineWithIndent(text string, offset int) (string, int) {
	caret := core.ClampWithin(offset, len(text))
	line, start := readLineAt(text, caret)

	// Only the indent before the caret is copied down.
	indent := readIndent(line)
	if caret-start < len(indent) {
		indent = indent[:caret-start]
	}
	written := line[:caret-start]
	stepped := indent
	if countsUnclosed(written) {
		stepped = indent + strings.Repeat(" ", indentWidth)
	}

	// A caret between the two halves of a pair moves the closing half down.
	closes := caret < len(text) &&
		strings.IndexByte(closingBrackets, text[caret]) != -1 && countsUnclosed(written)
	opened := "\n" + stepped
	if closes {
		opened = "\n" + stepped + "\n" + indent
	}
	return text[:caret] + opened + text[caret:], caret + 1 + len(stepped)
}

// countIndentSteps returns how many steps a line is indented, and whether it has any. A line
// that opens with a tab has none, because the buffer decides how wide a tab is and a guide
// under one would stand in the wrong cell.
func countIndentSteps(line string) (int, bool) {
	indent := readIndent(line)
	if strings.ContainsRune(indent, '\t') || len(indent) == len(line) {
		return 0, false
	}
	return len(indent) / indentWidth, true
}

// PlanIndentGuides returns the columns of the guides, line by line. A blank line takes the
// smaller indent of the blocks around it, so no guide leads nowhere.
func PlanIndentGuides(lines []string) [][]int {
	steps := make([]int, len(lines))
	known := make([]bool, len(lines))
	for at, line := range lines {
		steps[at], known[at] = countIndentSteps(line)
	}

	planned := make([][]int, len(lines))
	for at := range lines {
		depth := steps[at]
		if !known[at] {
			depth = min(
				readNeighbourSteps(steps, known, at, -1),
				readNeighbourSteps(steps, known, at, 1))
		}
		columns := make([]int, 0, depth)
		for level := 0; level < depth; level++ {
			columns = append(columns, level*indentWidth)
		}
		planned[at] = columns
	}
	return planned
}

// readNeighbourSteps returns the steps of the nearest line in that direction that has any.
func readNeighbourSteps(steps []int, known []bool, from, step int) int {
	for at := from + step; at >= 0 && at < len(steps); at += step {
		if known[at] {
			return steps[at]
		}
	}
	return 0
}

// BracketPair is the two halves of a bracket pair, as offsets into the text.
type BracketPair struct {
	Open  int
	Close int
}

// FindBracketPair returns the pair at the caret. The bracket under the caret comes first, and
// the one before it second, so a caret after a closing bracket still marks the pair. It
// reports nothing where there is no bracket, or where it is never closed.
func FindBracketPair(text string, offset int, ignored func(int) bool) (BracketPair, bool) {
	if ignored == nil {
		ignored = func(int) bool { return false }
	}
	caret := core.ClampWithin(offset, len(text))
	for _, at := range []int{caret, caret - 1} {
		if at < 0 || at >= len(text) || ignored(at) {
			continue
		}
		character := text[at]
		if strings.IndexByte(openingBrackets, character) != -1 {
			if closing, found := searchForwardBracket(text, at, character, ignored); found {
				return BracketPair{Open: at, Close: closing}, true
			}
		}
		if strings.IndexByte(closingBrackets, character) != -1 {
			if open, found := searchBackwardBracket(text, at, character, ignored); found {
				return BracketPair{Open: open, Close: at}, true
			}
		}
	}
	return BracketPair{}, false
}

func searchForwardBracket(
	text string, from int, open byte, ignored func(int) bool,
) (int, bool) {
	closing := closingBrackets[strings.IndexByte(openingBrackets, open)]
	depth := 0
	for at := from; at < len(text); at++ {
		if ignored(at) {
			continue
		}
		switch text[at] {
		case open:
			depth++
		case closing:
			depth--
			if depth == 0 {
				return at, true
			}
		}
	}
	return 0, false
}

func searchBackwardBracket(
	text string, from int, closing byte, ignored func(int) bool,
) (int, bool) {
	open := openingBrackets[strings.IndexByte(closingBrackets, closing)]
	depth := 0
	for at := from; at >= 0; at-- {
		if ignored(at) {
			continue
		}
		switch text[at] {
		case closing:
			depth++
		case open:
			depth--
			if depth == 0 {
				return at, true
			}
		}
	}
	return 0, false
}
