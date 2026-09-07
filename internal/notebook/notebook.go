// Package notebook reads and writes a notebook file: a front matter block and an ordered
// list of cells. It holds no connection and draws nothing.
package notebook

import (
	"strings"
	"unicode"
)

// FileSuffix is the ending of a notebook file.
const FileSuffix = ".masume.md"

// CellKind is the content of one cell.
type CellKind string

// The kinds of cell a notebook holds.
const (
	// CellSQL holds statements of the engine.
	CellSQL CellKind = "sql"
	// CellText holds prose. It runs nothing.
	CellText CellKind = "md"
	// CellParam holds the values of the `:name` marks the other cells bind.
	CellParam CellKind = "param"
	// CellChart draws the result of another cell.
	CellChart CellKind = "chart"
	// CellOther is a fence this build does not know. It is kept as it was written.
	CellOther CellKind = "other"
)

// Attribute is one `name=value` pair of a fence.
type Attribute struct {
	Name  string
	Value string
}

// Cell is one cell of a notebook.
type Cell struct {
	ID   string
	Kind CellKind
	// The text between the fences, or the prose of a text cell.
	Source string
	// The whole fence line of a cell of an unknown kind.
	Fence string
	// The pairs of the fence, in the order they were written.
	Attrs []Attribute
}

// FindAttr returns the value of that pair, and false where the cell has none.
func (cell Cell) FindAttr(name string) (string, bool) {
	for _, attr := range cell.Attrs {
		if attr.Name == name {
			return attr.Value, true
		}
	}
	return "", false
}

// AsksConfirmation is true for a cell whose fence asks for one more question before a write.
func (cell Cell) AsksConfirmation() bool {
	value, held := cell.FindAttr("write")
	return held && value == "confirm"
}

// RunsStatements is true for a cell the run sends to the server.
func (cell Cell) RunsStatements() bool {
	return cell.Kind == CellSQL
}

// The values of the transaction policy.
const (
	TransactionAutocommit = "autocommit"
	TransactionSingle     = "single"
)

// The values of the error policy.
const (
	ErrorStop     = "stop"
	ErrorContinue = "continue"
)

// RunPolicy is the transaction and the error policy of one notebook.
type RunPolicy struct {
	Transaction string
	OnError     string
}

// RunsInOneTransaction is true while the whole notebook runs inside one transaction.
func (policy RunPolicy) RunsInOneTransaction() bool {
	return policy.Transaction == TransactionSingle
}

// StopsOnError is true while the first failure ends the run.
func (policy RunPolicy) StopsOnError() bool {
	return policy.OnError != ErrorContinue
}

// Notebook is one notebook file.
type Notebook struct {
	Title string
	// The profiles the notebook is offered on. An empty list offers it on every one.
	Profiles []string
	// The engine the notebook was written for. A mismatch warns.
	Engine string
	Run    RunPolicy
	// The front matter as it was written, so a key this build does not know survives a save.
	FrontMatter string
	Cells       []Cell
	// What could not be read, one line each.
	Problems []string
}

// CountWriteCells returns how many cells the fence marks as a write.
func (book Notebook) CountWriteCells() int {
	count := 0
	for _, cell := range book.Cells {
		if cell.AsksConfirmation() {
			count++
		}
	}
	return count
}

// DefaultPolicy is the policy a notebook without a `[run]` block gets.
func DefaultPolicy() RunPolicy {
	return RunPolicy{Transaction: TransactionAutocommit, OnError: ErrorStop}
}

// BuildSlug returns the text as a cell id: lower case, words joined by hyphens.
func BuildSlug(text string) string {
	var written strings.Builder
	hyphen := false
	for _, letter := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(letter) || unicode.IsDigit(letter):
			written.WriteRune(letter)
			hyphen = false
		case written.Len() > 0 && !hyphen:
			written.WriteRune('-')
			hyphen = true
		}
	}
	return strings.Trim(written.String(), "-")
}
