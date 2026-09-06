package ai

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// The system prompt contains the role, tools, and connection metadata. Each question includes the current editor text separately.

// maxEditorSQLChars is the length at which the content of the editor is truncated.
const maxEditorSQLChars = 4000

// EditorContext is the content of the editor, so a question can refer to "this query".
type EditorContext struct {
	SQL string
	// LastError is the error message of the server from the last run, if it failed.
	LastError string
}

// StatementLanguage is the connected server language and code block format.
type StatementLanguage struct {
	// Name is the language, for example SQL.
	Name string
	// FenceTag is the tag of the fenced block that holds a proposed statement.
	FenceTag string
	// Example is a sample statement. It is empty for SQL.
	Example string
}

// describeStatementLanguage builds the language-specific prompt instructions.
func describeStatementLanguage(language StatementLanguage) []string {
	name := language.Name
	if name == "" {
		name = "SQL"
	}
	tag := language.FenceTag
	if tag == "" {
		tag = "sql"
	}
	lines := []string{
		"You are a database assistant built into a terminal database client.",
		"Answer questions about the connected database and, when asked for a query, write " +
			"correct " + name + " for its server.",
		"Put a proposed query in exactly one fenced code block, opened with ```" + tag +
			". Do not use fences for other text.",
	}
	if language.Example != "" {
		lines = append(lines, language.Example)
	}
	return lines
}

var systemPrompt = strings.Join([]string{
	"The catalog summary below lists database and schema names, without tables or columns. " +
		"Call list_tables to find tables in the default database or schema, or another listed database or schema. " +
		"Before writing a query, call describe_table for each table whose columns are unknown in this conversation.",
	"list_relationships returns foreign keys. list_indexes and list_constraints return indexes and constraints. " +
		"get_table_ddl returns a table definition in one response. " +
		"Choose the tool that answers the question; do not always use describe_table alone.",
	"Call validate_query on a query you are about to present, when you are not already " +
		"certain it is correct. It only checks the statement; it does not run it or return rows.",
	"Call explain_query to check for missing indexes, inefficient joins, or inaccurate estimates before proposing or optimizing a query. " +
		"The default is an estimated plan. Use analyze for execution measurements of eligible read-only statements, never writes.",
	"Call run_query only when the user requests data, a count, or a value, such as \"how many orders are unpaid\". " +
		"For requests to write, fix, or explain a query, return the query in a fenced block without execution.",
	"The user is asked before a statement runs, and may say no. If they do, say so plainly " +
		"and do not ask again unless they bring it up.",
	"After execution, include the result values in the answer. Do not refer the user to results outside the chat.",
	"Only name a table, column, or database that a tool call confirmed. Never invent one, and " +
		"never present a guessed name as though it were confirmed.",
	"Do not ask the user to run a catalog query for you; call the tools yourself.",
	"If a tool fails or cannot find a table, state that the metadata is unavailable. " +
		"Do not assume the schema matches another database.",
	"The user may have a query already in the editor, sent with the question along with any " +
		"error it last failed with. Take it as the subject of a question like \"why does this " +
		"fail\" or \"optimize this\" unless they plainly mean something else.",
	"Keep prose short.",
}, "\n")

// describeProfileInstructions returns the configured connection instructions.
func describeProfileInstructions(instructions string) string {
	trimmed := strings.TrimSpace(instructions)
	if trimmed == "" {
		return ""
	}
	return "\n\nInstructions the user set for this connection:\n" + trimmed
}

// DescribeEditorContext returns the content of the editor, which is sent with the question.
func DescribeEditorContext(context EditorContext) string {
	trimmed := strings.TrimSpace(context.SQL)
	if trimmed == "" {
		return ""
	}
	capped := trimmed
	if len([]rune(capped)) > maxEditorSQLChars {
		capped = string([]rune(capped)[:maxEditorSQLChars]) +
			"\n... (truncated; request the remaining text if needed)"
	}

	lines := []string{"Current editor statement:", "```sql", capped, "```"}
	if context.LastError != "" {
		lines = append(lines, "", "Last execution error: "+context.LastError)
	}
	return strings.Join(lines, "\n")
}

// ChatPromptSource holds the connection data added to the prompt above.
type ChatPromptSource struct {
	DialectName string
	// Language is the statement language of this server.
	Language      StatementLanguage
	DefaultSchema string
	Tables        []db.TableRef
	// Instructions is the configured connection guidance.
	Instructions string
}

// BuildChatSystemPrompt builds the system prompt from connection metadata and instructions.
func BuildChatSystemPrompt(source ChatPromptSource) string {
	schema := BuildSchemaContext(SchemaContextSource{
		DialectName: source.DialectName, DefaultSchema: source.DefaultSchema,
		Tables: source.Tables,
	})
	opening := strings.Join(describeStatementLanguage(source.Language), "\n")
	return opening + "\n" + systemPrompt + "\n\n" + schema +
		describeProfileInstructions(source.Instructions)
}
