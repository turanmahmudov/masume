package ai

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// The text sent to the model before the first question: its role, the tools it can call, and
// the structure of the connection. The content of the editor goes with the question, because
// it changes between questions and the provider caches the system prompt.

// maxEditorSQLChars is the length at which the content of the editor is truncated.
const maxEditorSQLChars = 4000

// EditorContext is the content of the editor, so a question can refer to "this query".
type EditorContext struct {
	SQL string
	// LastError is the error message of the server from the last run, if it failed.
	LastError string
}

// StatementLanguage is the statement language of the connected server, so the chat proposes
// a statement the server accepts and not SQL for a server without SQL.
type StatementLanguage struct {
	// Name is the name of the language, for example SQL.
	Name string
	// FenceTag is the tag of the fenced block that holds a proposed statement.
	FenceTag string
	// Example shows the form of one statement. It is empty for SQL.
	Example string
}

// describeStatementLanguage returns the first lines of the prompt. They are the only lines
// that are different between a server with SQL and a server with commands.
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
			", and nothing else in fences.",
	}
	if language.Example != "" {
		lines = append(lines, language.Example)
	}
	return lines
}

var systemPrompt = strings.Join([]string{
	"No table or column is named below, only the connected database and the names of any " +
		"others on the same server. Call list_tables to see what exists, in the connected " +
		"database or another one named below, and call describe_table before writing a query " +
		"against a table whose columns you have not already seen in this conversation.",
	"list_relationships shows how tables join, list_indexes and list_constraints show what a " +
		"table enforces, and get_table_ddl gives a whole table in one read. Reach for " +
		"whichever answers the question, rather than describe_table alone every time.",
	"Call validate_query on a query you are about to present, when you are not already " +
		"certain it is correct. It only checks the statement; it does not run it or return rows.",
	"Call explain_query to check a query for a missing index, a bad join order, or a wrong " +
		"estimate, before proposing it or when asked to make one faster. It only estimates by " +
		"default; ask for analyze to measure a read for real, which does not apply to a write.",
	"Call run_query only where the user asked for data, a count or a value, such as \"how " +
		"many orders are unpaid\". A request to write, fix or explain a query is answered with " +
		"the query in a fenced block and nothing run.",
	"The user is asked before a statement runs, and may say no. If they do, say so plainly " +
		"and do not ask again unless they bring it up.",
	"After a run, answer with the figures themselves. Never tell the user to look at a result " +
		"they cannot see.",
	"Only name a table, column, or database that a tool call confirmed. Never invent one, and " +
		"never present a guessed name as though it were confirmed.",
	"Do not ask the user to run a catalog query for you; call the tools yourself.",
	"If a tool call fails or a table still cannot be found afterward, say plainly that you " +
		"cannot see it, rather than assuming a layout from another database.",
	"The user may have a query already in the editor, sent with the question along with any " +
		"error it last failed with. Take it as the subject of a question like \"why does this " +
		"fail\" or \"optimize this\" unless they plainly mean something else.",
	"Keep prose short.",
}, "\n")

// describeProfileInstructions returns the rules the user set for this connection, for
// example a naming convention.
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
			"\n... (cut here; ask to see the rest if it matters)"
	}

	lines := []string{"The editor currently holds:", "```sql", capped, "```"}
	if context.LastError != "" {
		lines = append(lines, "", "It last failed with: "+context.LastError)
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
	// Instructions holds the rules the user set for this connection.
	Instructions string
}

// BuildChatSystemPrompt returns the same text for every question of one connection, so the
// provider can cache it.
func BuildChatSystemPrompt(source ChatPromptSource) string {
	schema := BuildSchemaContext(SchemaContextSource{
		DialectName: source.DialectName, DefaultSchema: source.DefaultSchema,
		Tables: source.Tables,
	})
	opening := strings.Join(describeStatementLanguage(source.Language), "\n")
	return opening + "\n" + systemPrompt + "\n\n" + schema +
		describeProfileInstructions(source.Instructions)
}
