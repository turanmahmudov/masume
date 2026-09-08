package language

import (
	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// sqlLanguage provides shared SQL processing with dialect-specific tokenization.
type sqlLanguage struct{ flavour syntax.SyntaxFlavour }

func (language sqlLanguage) Tokenize(text string) []syntax.Token {
	return syntax.Tokenize(text, language.flavour)
}

func (language sqlLanguage) SplitStatements(text string) []string {
	return statement.SplitStatements(text, language.flavour)
}

func (language sqlLanguage) SplitStatementRanges(text string) []statement.StatementRange {
	return statement.SplitStatementRanges(text, language.flavour)
}

func (language sqlLanguage) ReadStatementAtOffset(text string, offset int) string {
	return statement.ReadStatementAtOffset(text, offset, language.flavour)
}

func (language sqlLanguage) FormatStatement(text string) string {
	return statement.FormatStatement(text, language.flavour)
}

// LineComment returns the two dashes SQL comments a line out with.
func (language sqlLanguage) LineComment() string {
	return "--"
}

func (language sqlLanguage) FindLocalDiagnostics(
	text string, knowledge editor.SchemaKnowledge,
) []editor.Diagnostic {
	return editor.FindLocalDiagnostics(text, knowledge, language.flavour)
}

// ResolveWriteRisk returns the highest statement risk in the buffer.
func (language sqlLanguage) ResolveWriteRisk(text string) statement.WriteRisk {
	statements := language.SplitStatements(text)
	risks := make([]statement.WriteRisk, 0, len(statements))
	for _, written := range statements {
		risks = append(risks, statement.ResolveWriteRisk(written, language.flavour))
	}
	return statement.ResolveStrongestRisk(risks)
}

// HoldsRowLimit detects LIMIT or FETCH. OFFSET alone does not limit the result size.
func (language sqlLanguage) HoldsRowLimit(text string) bool {
	return statement.HoldsRowLimit(text, language.flavour)
}

func (language sqlLanguage) ChangesCatalog(text string) bool {
	return statement.ChangesCatalog(text, language.flavour)
}

func (language sqlLanguage) CanExplain(text string) bool {
	return statement.CanExplain(text, language.flavour)
}

func (language sqlLanguage) BuildCompletions(
	prefix string, sources editor.CompletionSources, context editor.CompletionContext,
) []editor.Completion {
	return editor.BuildCompletions(prefix, sources, context)
}

// SQL is the shared language with standard SQL tokenization.
var SQL Language = sqlLanguage{flavour: syntax.FlavourStandard}

// Mysql is the same language, read the way MySQL reads a buffer.
var Mysql Language = sqlLanguage{flavour: syntax.FlavourMysql}

// Sqlserver is the same language, read the way a SQL Server reads a buffer.
var Sqlserver Language = sqlLanguage{flavour: syntax.FlavourSqlserver}
