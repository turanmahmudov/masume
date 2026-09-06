// Package language provides local statement tokenization, splitting, formatting, diagnostics, and completion.
package language

import (
	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Language is the interface for local statement processing.
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
	// LineComment returns the line comment prefix, or an empty string when unsupported.
	LineComment() string
	// FindLocalDiagnostics returns the faults that can be found without the server.
	FindLocalDiagnostics(text string, knowledge editor.SchemaKnowledge) []editor.Diagnostic
	// ResolveWriteRisk classifies statement risk for confirmation.
	ResolveWriteRisk(text string) statement.WriteRisk
	// HoldsRowLimit is true for statements with a result limit.
	HoldsRowLimit(text string) bool
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
