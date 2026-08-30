package statement

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// WriteRisk is how much a statement risks, from none to the highest.
type WriteRisk string

// The four levels of risk. They are ordered, so one statement of a batch is enough
// to raise the whole batch.
const (
	RiskNone     WriteRisk = "none"
	RiskWrite    WriteRisk = "write"
	RiskDelete   WriteRisk = "delete"
	RiskEveryRow WriteRisk = "every-row"
)

var writeRisks = []WriteRisk{RiskNone, RiskWrite, RiskDelete, RiskEveryRow}

func indexOfRisk(risk WriteRisk) int {
	for at, candidate := range writeRisks {
		if candidate == risk {
			return at
		}
	}
	return 0
}

// ResolveStrongestRisk returns the highest risk of a set, because one statement is
// enough.
func ResolveStrongestRisk(risks []WriteRisk) WriteRisk {
	strongest := RiskNone
	for _, risk := range risks {
		if indexOfRisk(risk) > indexOfRisk(strongest) {
			strongest = risk
		}
	}
	return strongest
}

var destructiveKeywords = []string{"delete", "drop", "truncate"}

// routineKeywords name the objects whose body a server stores, and does not run.
var routineKeywords = map[string]bool{
	"procedure": true, "function": true, "trigger": true, "event": true,
}

// plainObjectKeywords name the objects of a CREATE or ALTER that have no body.
var plainObjectKeywords = map[string]bool{
	"table": true, "view": true, "materialized": true, "index": true, "schema": true,
	"database": true, "sequence": true, "type": true, "role": true, "user": true,
	"extension": true, "policy": true,
}

// writingOpeners name the words a statement starts with to do work instead of
// reading rows. Only the first word counts: `replace` is also a function, and
// `copy` and `call` can be column names.
var writingOpeners = map[string]bool{"copy": true, "refresh": true, "call": true, "do": true}

// destructiveOpeners name the words that remove data first. MySQL removes the
// conflicting row before it writes the new one.
var destructiveOpeners = map[string]bool{"replace": true}

// readingOpeners name the words that open a statement which returns rows, or touches only
// this session. Any other opening word is weighed as a write: a statement this client does
// not know may be one the server writes with, such as MySQL `LOAD DATA`, and a statement
// read as a read passes every check this client makes.
var readingOpeners = map[string]bool{
	"select": true, "with": true, "values": true, "table": true, "show": true,
	"describe": true, "desc": true, "explain": true, "lock": true, "pragma": true,
	"begin": true, "start": true, "commit": true, "rollback": true, "end": true,
	"savepoint": true, "release": true, "set": true, "reset": true, "use": true,
	"declare": true, "fetch": true, "close": true, "deallocate": true,
}

// serverSettingScopes name the words that carry a SET past this session and into the
// server, which every later connection then reads.
var serverSettingScopes = map[string]bool{"global": true, "persist": true, "persist_only": true}

// sessionScopes name the words that keep a SET inside this session or this transaction,
// and that stand between SET and the name of the setting.
var sessionScopes = map[string]bool{"session": true, "local": true}

// sessionSettings name the settings a SET or a RESET may touch and still count as a read.
// They change how the session answers and nothing else.
//
// A name that is not here is a write. A setting this client does not know may be one that
// lets a later read write, and two of them do exactly that: `default_transaction_read_only`
// in PostgreSQL and `transaction_read_only` in MySQL both turn a read-only session back
// into one that writes, and every statement after that reads as safe.
var sessionSettings = map[string]bool{
	// How a value is written back.
	"datestyle": true, "intervalstyle": true, "extra_float_digits": true,
	"bytea_output": true, "client_encoding": true, "timezone": true, "time_zone": true,
	"names": true, "character_set_client": true, "character_set_connection": true,
	"character_set_results": true, "collation_connection": true,
	"lc_monetary": true, "lc_numeric": true, "lc_time": true, "lc_messages": true,
	// What the connection is called, and how much the server says.
	"application_name": true, "client_min_messages": true,
	// How long the server gives a statement.
	"statement_timeout": true, "lock_timeout": true, "max_execution_time": true,
	"idle_in_transaction_session_timeout": true, "wait_timeout": true,
	"innodb_lock_wait_timeout": true,
	// Where an unqualified name is looked for.
	"search_path": true,
	// What the planner is given to work with.
	"work_mem": true, "jit": true, "random_page_cost": true, "cpu_tuple_cost": true,
	"effective_cache_size": true, "effective_io_concurrency": true,
	"group_concat_max_len": true, "sql_select_limit": true, "optimizer_switch": true,
	"profiling": true,
}

// plannerSettingPrefix opens the name of every planner switch, of which there are dozens
// and each one only decides how a read is run.
const plannerSettingPrefix = "enable_"

// holdsReadWritePhrase is true for the `read write` of a transaction mode, which takes a
// read-only session back to one that writes.
func holdsReadWritePhrase(tokens []syntax.CodeToken) bool {
	for at, token := range tokens {
		if token.Text != "write" || !syntax.IsWordKind(token.Kind) {
			continue
		}
		if before, present := syntax.TokenAt(tokens, at-1); present && before.Text == "read" {
			return true
		}
	}
	return false
}

// readOpeningWordInside returns the word the statement opens with, past the brackets a read
// may open with, as `(select 1) union (select 2)` does.
func readOpeningWordInside(tokens []syntax.CodeToken) string {
	for _, token := range tokens {
		if syntax.IsWordKind(token.Kind) {
			return token.Text
		}
		if token.Text != "(" {
			return ""
		}
	}
	return ""
}

// isSettingStatement is true for a SET, a RESET or a PRAGMA that only reads or only
// touches this session. `set global` reaches the server, a PRAGMA with a value written to
// it changes the file, and a setting this client does not know may be one that opens the
// session to writes.
func isSettingStatement(tokens []syntax.CodeToken, opening string) bool {
	if opening == "pragma" {
		return !syntax.IsOperatorAnywhere(tokens, "=")
	}
	if opening != "set" && opening != "reset" {
		return true
	}

	at := 1
	if scope, present := syntax.TokenAt(tokens, at); present {
		if serverSettingScopes[scope.Text] {
			return false
		}
		if sessionScopes[scope.Text] {
			at++
		}
	}

	named, present := syntax.TokenAt(tokens, at)
	if !present {
		return false
	}
	// A transaction mode names no setting. It is a read unless it asks to write, which
	// `set transaction read write` and `set session characteristics as transaction read
	// write` both do.
	if named.Text == "transaction" || named.Text == "characteristics" {
		return !holdsReadWritePhrase(tokens)
	}

	setting := named.Text
	// PostgreSQL writes one setting as two words.
	if setting == "time" {
		if next, follows := syntax.TokenAt(tokens, at+1); follows && next.Text == "zone" {
			setting = "timezone"
		}
	}
	return sessionSettings[setting] || strings.HasPrefix(setting, plannerSettingPrefix)
}

// definesRoutine is true where a routine body is stored, not run, so a DELETE in it
// removes nothing. Only MySQL reaches this, because a PostgreSQL body is a
// dollar-quoted string.
func definesRoutine(tokens []syntax.CodeToken) bool {
	opening := syntax.ReadOpeningWord(tokens)
	if opening != "create" && opening != "alter" {
		return false
	}
	for _, token := range tokens[1:] {
		if !syntax.IsWordKind(token.Kind) {
			continue
		}
		if routineKeywords[token.Text] {
			return true
		}
		if plainObjectKeywords[token.Text] {
			return false
		}
	}
	return false
}

// unqualifiedWriteOpeners name the words a statement starts with to change rows it
// may not have named.
var unqualifiedWriteOpeners = map[string]bool{"update": true, "delete": true, "truncate": true}

// lockClauseWords are the words of a locking clause between `for` and `update`, as
// in `for no key update`.
var lockClauseWords = map[string]bool{"no": true, "key": true}

// isCalledAsFunction is true where the word is a call, as MySQL `truncate(1.234, 2)`
// is. A keyword followed by a bracket names a function, not the statement.
func isCalledAsFunction(tokens []syntax.CodeToken, hit syntax.KeywordHit) bool {
	return syntax.IsOperator(tokens, hit.Index+syntax.CountKeywordTokens(hit.Keyword), "(")
}

// belongsToLockClause is true where the word belongs to `select … for update`, which
// locks and writes nothing.
func belongsToLockClause(tokens []syntax.CodeToken, hit syntax.KeywordHit) bool {
	for at := hit.Index - 1; at >= 0 && at >= hit.Index-3; at-- {
		token, present := syntax.TokenAt(tokens, at)
		text := ""
		if present {
			text = token.Text
		}
		if text == "for" {
			return true
		}
		if !lockClauseWords[text] {
			return false
		}
	}
	return false
}

// keepStatementHits returns the hits that are the statement itself, not a call and
// not a lock.
func keepStatementHits(tokens []syntax.CodeToken, hits []syntax.KeywordHit) []syntax.KeywordHit {
	kept := make([]syntax.KeywordHit, 0, len(hits))
	for _, hit := range hits {
		if !isCalledAsFunction(tokens, hit) && !belongsToLockClause(tokens, hit) {
			kept = append(kept, hit)
		}
	}
	return kept
}

// readActingWord returns the word the statement acts with: the opening one, or the
// first one after a CTE list.
func readActingWord(tokens []syntax.CodeToken) string {
	opening := syntax.ReadOpeningWord(tokens)
	if opening != "with" {
		return opening
	}
	openers := make([]string, 0, len(unqualifiedWriteOpeners))
	for word := range unqualifiedWriteOpeners {
		openers = append(openers, word)
	}
	hits := syntax.FindKeywordsIn(tokens, openers)
	if len(hits) == 0 {
		return opening
	}
	return hits[0].Keyword
}

// isUnqualifiedWrite is true for a write without a top-level WHERE. A WHERE in
// brackets belongs to a subquery.
func isUnqualifiedWrite(tokens []syntax.CodeToken) bool {
	acting := readActingWord(tokens)
	if !unqualifiedWriteOpeners[acting] {
		return false
	}
	if acting == "truncate" {
		return true
	}
	return len(syntax.FindKeywordsIn(tokens, []string{"where"})) == 0
}

// ResolveWriteRisk weighs the statement. DELETE, DROP and TRUNCATE are one group,
// and a write without a WHERE ranks higher.
func ResolveWriteRisk(sql string, flavour syntax.SyntaxFlavour) WriteRisk {
	tokens := syntax.ReadCodeTokens(sql, flavour)
	opening := syntax.ReadOpeningWord(tokens)
	finds := func(keywords []string) bool {
		return len(keepStatementHits(tokens, syntax.FindKeywordsAnywhere(tokens, keywords))) > 0
	}

	// Creating a routine is a write, whatever its body does later.
	if definesRoutine(tokens) {
		return RiskWrite
	}
	if isUnqualifiedWrite(tokens) {
		return RiskEveryRow
	}
	if destructiveOpeners[opening] {
		return RiskDelete
	}
	if finds(destructiveKeywords) {
		return RiskDelete
	}
	if finds(WriteKeywords) {
		return RiskWrite
	}
	if finds([]string{"create", "alter", "grant", "revoke"}) {
		return RiskWrite
	}
	if writingOpeners[opening] || syntax.SelectsIntoTarget(tokens) {
		return RiskWrite
	}
	// A buffer of nothing but comments runs nothing.
	if len(tokens) == 0 {
		return RiskNone
	}
	acting := readOpeningWordInside(tokens)
	if !readingOpeners[acting] {
		return RiskWrite
	}
	if !isSettingStatement(tokens, acting) {
		return RiskWrite
	}
	// `begin read write` and `start transaction read write` open a transaction that
	// writes, whatever the connection was set read-only for.
	if transactionOpeners[acting] && holdsReadWritePhrase(tokens) {
		return RiskWrite
	}
	return RiskNone
}

type riskVerb struct{ one, many string }

var riskVerbs = map[WriteRisk]riskVerb{
	RiskNone:   {"reads only", "read only"},
	RiskWrite:  {"writes to the database", "write to the database"},
	RiskDelete: {"removes data", "remove data"},
	RiskEveryRow: {
		"names no rows, so it lands on every row",
		"name no rows, so they land on every row",
	},
}

// DescribeRisk writes what a statement of this risk does, for a question or a refusal.
func DescribeRisk(risk WriteRisk, count int) string {
	verb := riskVerbs[risk]
	if count == 1 {
		return verb.one
	}
	return verb.many
}

// Confirmation is the question asked before a write runs.
type Confirmation struct {
	Title string
	Body  string
}

// BuildConfirmation writes the question. The profile and the environment are named,
// because the risk depends on them.
func BuildConfirmation(
	profileName, environment string, risk WriteRisk, statements []string,
) Confirmation {
	counted := "This statement"
	if len(statements) != 1 {
		counted = fmt.Sprintf("These %d statements", len(statements))
	}
	return Confirmation{
		Title: "confirm on " + profileName,
		Body: fmt.Sprintf("%s %s on %s.\n\n%s", counted,
			DescribeRisk(risk, len(statements)), environment, strings.Join(statements, ";\n")),
	}
}

// WriteKeywords are the words that change rows.
var WriteKeywords = []string{"insert", "update", "delete", "merge"}
