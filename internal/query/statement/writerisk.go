package statement

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// WriteRisk is the statement risk classification.
type WriteRisk string

// Risk levels in ascending order.
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

// ResolveStrongestRisk returns the highest risk in a set.
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

// writingOpeners is the set of opening keywords classified as writes.
var writingOpeners = map[string]bool{"copy": true, "refresh": true, "call": true, "do": true}

// destructiveOpeners is the set of destructive opening keywords. MySQL REPLACE deletes conflicting rows before insertion.
var destructiveOpeners = map[string]bool{"replace": true}

// readingOpeners is the set of opening keywords eligible for read-only classification. Unknown opening keywords are writes.
var readingOpeners = map[string]bool{
	"select": true, "with": true, "values": true, "table": true, "show": true,
	"describe": true, "desc": true, "explain": true, "lock": true, "pragma": true,
	"begin": true, "start": true, "commit": true, "rollback": true, "end": true,
	"savepoint": true, "release": true, "set": true, "reset": true, "use": true,
	"declare": true, "fetch": true, "close": true, "deallocate": true,
}

// serverSettingScopes is the set of server-wide SET scopes.
var serverSettingScopes = map[string]bool{"global": true, "persist": true, "persist_only": true}

// sessionScopes is the set of session-local SET scopes.
var sessionScopes = map[string]bool{"session": true, "local": true}

// sessionSettings is the allowlist for read-only SET and RESET classification, in addition to planner switches.
//
// PostgreSQL default_transaction_read_only and MySQL transaction_read_only can permit writes and are excluded.
var sessionSettings = map[string]bool{
	// Value formatting and encoding.
	"datestyle": true, "intervalstyle": true, "extra_float_digits": true,
	"bytea_output": true, "client_encoding": true, "timezone": true, "time_zone": true,
	"names": true, "character_set_client": true, "character_set_connection": true,
	"character_set_results": true, "collation_connection": true,
	"lc_monetary": true, "lc_numeric": true, "lc_time": true, "lc_messages": true,
	// Session name and message level.
	"application_name": true, "client_min_messages": true,
	// Statement and session timeouts.
	"statement_timeout": true, "lock_timeout": true, "max_execution_time": true,
	"idle_in_transaction_session_timeout": true, "wait_timeout": true,
	"innodb_lock_wait_timeout": true,
	// Schema search path.
	"search_path": true,
	// Planner and query settings.
	"work_mem": true, "jit": true, "random_page_cost": true, "cpu_tuple_cost": true,
	"effective_cache_size": true, "effective_io_concurrency": true,
	"group_concat_max_len": true, "sql_select_limit": true, "optimizer_switch": true,
	"profiling": true,
}

// plannerSettingPrefix is the prefix accepted for planner switches.
const plannerSettingPrefix = "enable_"

// settingFunctions change a server setting from inside a reading statement.
var settingFunctions = map[string]bool{"set_config": true}

// isAllowedSetting is true for a setting a read-only statement may change.
func isAllowedSetting(setting string) bool {
	return sessionSettings[setting] || strings.HasPrefix(setting, plannerSettingPrefix)
}

// changesGuardedSetting is true where a setting function changes a setting outside the allowlist.
func changesGuardedSetting(tokens []syntax.CodeToken) bool {
	for at, token := range tokens {
		if !syntax.IsWordKind(token.Kind) || !settingFunctions[token.Text] {
			continue
		}
		if !syntax.IsOperator(tokens, at+1, "(") {
			continue
		}
		named, present := syntax.TokenAt(tokens, at+2)
		if !present || named.Kind != syntax.TokenString {
			return true
		}
		if !isAllowedSetting(strings.Trim(named.Text, "'\"")) {
			return true
		}
	}
	return false
}

// holdsReadWritePhrase detects a READ WRITE transaction mode.
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

// readOpeningWordInside returns the first word after any opening parentheses.
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

// isSettingStatement checks SET, RESET, and PRAGMA forms eligible for read-only classification.
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
	// Transaction modes with READ WRITE are classified as writes.
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
	return isAllowedSetting(setting)
}

// definesRoutine detects stored routine definitions. MySQL bodies contain code tokens; PostgreSQL bodies commonly use dollar-quoted strings.
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

// unqualifiedWriteOpeners is the set of writes checked for a missing WHERE clause.
var unqualifiedWriteOpeners = map[string]bool{"update": true, "delete": true, "truncate": true}

// lockClauseWords is the optional words between FOR and UPDATE in a locking clause.
var lockClauseWords = map[string]bool{"no": true, "key": true}

// isCalledAsFunction detects a following parenthesis, as in MySQL truncate(1.234, 2).
func isCalledAsFunction(tokens []syntax.CodeToken, hit syntax.KeywordHit) bool {
	return syntax.IsOperator(tokens, hit.Index+syntax.CountKeywordTokens(hit.Keyword), "(")
}

// belongsToLockClause detects UPDATE in a SELECT locking clause.
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

// keepStatementHits excludes function calls and locking clauses.
func keepStatementHits(tokens []syntax.CodeToken, hits []syntax.KeywordHit) []syntax.KeywordHit {
	kept := make([]syntax.KeywordHit, 0, len(hits))
	for _, hit := range hits {
		if !isCalledAsFunction(tokens, hit) && !belongsToLockClause(tokens, hit) {
			kept = append(kept, hit)
		}
	}
	return kept
}

// readActingWord returns the opening keyword or the first top-level write keyword after WITH.
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

// isUnqualifiedWrite detects TRUNCATE and UPDATE or DELETE without a top-level WHERE.
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

// ResolveWriteRisk classifies statement risk from tokens. Unqualified writes have the highest risk.
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
	// Empty or comment-only input has no write risk.
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
	if changesGuardedSetting(tokens) {
		return RiskWrite
	}
	// READ WRITE transaction modes are classified as writes.
	if transactionOpeners[acting] && holdsReadWritePhrase(tokens) {
		return RiskWrite
	}
	return RiskNone
}

type riskVerb struct{ one, many string }

var riskVerbs = map[WriteRisk]riskVerb{
	RiskNone:   {"is classified as read-only", "are classified as read-only"},
	RiskWrite:  {"writes to the database", "write to the database"},
	RiskDelete: {"removes data", "remove data"},
	RiskEveryRow: {
		"has no WHERE clause and may affect every row",
		"include a write without a WHERE clause that may affect every row",
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

// BuildConfirmation builds a confirmation with the profile, environment, risk, and statement text.
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
