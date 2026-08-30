// Package language reads the buffer of a tab without the server: coloured, split,
// formatted, checked and completed.
package language

import (
	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Language says how the buffer of a tab is read without the server: coloured,
// split, formatted, checked and completed. SQL is one language. Another server has
// its own, and the code above does not know which one it uses.
type Language interface {
	// Tokenize returns the tokens of the buffer, which the editor colours.
	Tokenize(text string) []syntax.Token
	// SplitStatements returns the statements a buffer holds, in run order.
	SplitStatements(text string) []string
	// SplitStatementRanges returns the same, with the place of each one.
	SplitStatementRanges(text string) []statement.StatementRange
	// ReadStatementAtOffset returns the statement the caret is inside.
	ReadStatementAtOffset(text string, offset int) string
	// FormatStatement writes the buffer again, one clause per line.
	FormatStatement(text string) string
	// LineComment returns the mark that comments out the rest of a line, or an empty text
	// where the language has none.
	LineComment() string
	// FindLocalDiagnostics returns the faults that can be found without the server.
	FindLocalDiagnostics(text string, knowledge editor.SchemaKnowledge) []editor.Diagnostic
	// ResolveWriteRisk weighs the statement, for the confirmation.
	ResolveWriteRisk(text string) statement.WriteRisk
	// ChangesCatalog is true if the statement makes the catalog of this client stale.
	ChangesCatalog(text string) bool
	// CanExplain is true if the server can plan the statement.
	CanExplain(text string) bool
	// BuildCompletions returns what could be typed next.
	BuildCompletions(
		prefix string, sources editor.CompletionSources, context editor.CompletionContext,
	) []editor.Completion
}

// ResolveBatchRisk returns the risk of a set, which is the highest risk in it.
func ResolveBatchRisk(statements []string, language Language) statement.WriteRisk {
	risks := make([]statement.WriteRisk, 0, len(statements))
	for _, written := range statements {
		risks = append(risks, language.ResolveWriteRisk(written))
	}
	return statement.ResolveStrongestRisk(risks)
}
