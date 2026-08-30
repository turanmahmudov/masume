package ai

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// Everything the model is told before the first question: what it is, what it may call, and
// the shape of the connection. The contents of the editor go with the question instead,
// because they change between questions and the provider caches the system prompt.

// maxEditorSQLChars is the length above which the contents of the editor are cut.
const maxEditorSQLChars = 4000

// EditorContext is the contents of the editor, so a question can say "this query".
type EditorContext struct {
	SQL string
	// LastError is the message of the server from the last run, if it failed.
	LastError string
}

// StatementLanguage says what a statement of the connected server is written in, so the
// chat proposes one the server takes rather than SQL for a server that has none.
type StatementLanguage struct {
	// Name is what the language is called, such as SQL.
	Name string
	// FenceTag opens the fenced block a proposed statement is written in.
	FenceTag string
	// Example shows the shape of one statement, and is empty for SQL.
	Example string
}

// describeStatementLanguage writes the opening lines of the prompt, which are the only
// ones that differ between a server that takes SQL and one that takes a command.
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

// describeProfileInstructions writes the rules the user set for this connection, such as a
// naming convention.
func describeProfileInstructions(instructions string) string {
	trimmed := strings.TrimSpace(instructions)
	if trimmed == "" {
		return ""
	}
	return "\n\nInstructions the user set for this connection:\n" + trimmed
}

// DescribeEditorContext writes the contents of the editor, which are sent with the question.
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

// ChatPromptSource is what the chat of one connection is told, beside the prompt above.
type ChatPromptSource struct {
	DialectName string
	// Language says what a statement of this server is written in.
	Language      StatementLanguage
	DefaultSchema string
	Tables        []db.TableRef
	// Instructions holds the rules the user wrote for this connection.
	Instructions string
}

// BuildChatSystemPrompt writes the same text for every question of one connection, so the
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
