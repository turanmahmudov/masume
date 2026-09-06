package statement

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// SelectSource is a relation named by a statement.
type SelectSource struct {
	Schema    string
	HasSchema bool
	Name      string
}

// multiSourceKeywords is the set of clauses excluded from editable single-table queries.
var multiSourceKeywords = []string{
	"join", "union", "intersect", "except", "group by", "having", "distinct",
}

// fromEnders is the set of clauses after FROM in a simple SELECT.
var fromEnders = []string{"where", "order by", "limit", "offset", "fetch", "window", "for"}

// readTableReference parses one table reference with an optional alias, excluding subqueries and multiple tables.
func readTableReference(sql string, tokens []syntax.CodeToken, from, to int) (SelectSource, bool) {
	if from < 0 || to > len(tokens) || from >= to {
		return SelectSource{}, false
	}
	clause := tokens[from:to]
	cursor := 0

	first, isName := syntax.ReadIdentifier(sql, clause, cursor)
	if !isName {
		return SelectSource{}, false
	}
	cursor++

	source := SelectSource{Name: first}
	if syntax.IsOperator(clause, cursor, ".") {
		qualified, isQualified := syntax.ReadIdentifier(sql, clause, cursor+1)
		if !isQualified {
			return SelectSource{}, false
		}
		source = SelectSource{Schema: first, HasSchema: true, Name: qualified}
		cursor += 2
	}

	if token, present := syntax.TokenAt(clause, cursor); present && token.Text == "as" {
		cursor++
	}
	if cursor < len(clause) {
		if _, isAlias := syntax.ReadIdentifier(sql, clause, cursor); isAlias {
			cursor++
		}
	}
	if cursor != len(clause) {
		return SelectSource{}, false
	}
	return source, true
}

// TableReference is a relation a statement reads, with its alias.
type TableReference struct {
	SelectSource
	Alias    string
	HasAlias bool
	// The table name byte range in the buffer.
	Start int
	End   int
}

// readReferenceAt parses a table reference and returns the next token index.
func readReferenceAt(sql string, tokens []syntax.CodeToken, index int) (TableReference, int, bool) {
	opening, present := syntax.TokenAt(tokens, index)
	if !present || !syntax.IsNameToken(tokens, index) {
		return TableReference{}, index, false
	}
	first, isName := syntax.ReadIdentifier(sql, tokens, index)
	if !isName {
		return TableReference{}, index, false
	}

	cursor := index + 1
	reference := TableReference{
		Name: first, Start: opening.Start, End: opening.End,
	}

	if syntax.IsOperator(tokens, cursor, ".") {
		qualified, isQualified := syntax.ReadIdentifier(sql, tokens, cursor+1)
		if !isQualified {
			return TableReference{}, index, false
		}
		named, _ := syntax.TokenAt(tokens, cursor+1)
		reference.Schema = first
		reference.HasSchema = true
		reference.Name = qualified
		reference.End = named.End
		cursor += 2
	}

	if token, hasToken := syntax.TokenAt(tokens, cursor); hasToken && token.Text == "as" {
		alias, isAlias := syntax.ReadIdentifier(sql, tokens, cursor+1)
		if isAlias {
			reference.Alias = alias
			reference.HasAlias = true
			cursor += 2
		}
	} else if syntax.IsNameToken(tokens, cursor) {
		alias, isAlias := syntax.ReadIdentifier(sql, tokens, cursor)
		if isAlias {
			reference.Alias = alias
			reference.HasAlias = true
			cursor++
		}
	}

	return reference, cursor, true
}

// readCteDefinition parses name [(columns)] AS [NOT] [MATERIALIZED] (...) and returns the next definition index.
func readCteDefinition(tokens []syntax.CodeToken, index int) (string, int, bool) {
	named, present := syntax.TokenAt(tokens, index)
	if !present || !syntax.IsNameToken(tokens, index) {
		return "", 0, false
	}
	name := syntax.UnquoteIdentifier(named.Text)

	cursor := syntax.SkipBracketGroup(tokens, index+1)
	if token, hasToken := syntax.TokenAt(tokens, cursor); !hasToken || token.Text != "as" {
		return name, -1, true
	}
	cursor++
	if token, hasToken := syntax.TokenAt(tokens, cursor); hasToken && token.Text == "not" {
		cursor++
	}
	if token, hasToken := syntax.TokenAt(tokens, cursor); hasToken && token.Text == "materialized" {
		cursor++
	}

	cursor = syntax.SkipBracketGroup(tokens, cursor)
	if !syntax.IsOperator(tokens, cursor, ",") {
		return name, -1, true
	}
	return name, cursor + 1, true
}

// FindCteNames returns lowercase CTE names from the statement.
func FindCteNames(sql string, flavour syntax.SyntaxFlavour) map[string]bool {
	tokens := syntax.ReadCodeTokens(sql, flavour)
	names := map[string]bool{}

	for _, hit := range syntax.FindKeywordsAnywhere(tokens, []string{"with"}) {
		index := hit.Index + 1
		if token, present := syntax.TokenAt(tokens, index); present && token.Text == "recursive" {
			index++
		}
		for index >= 0 {
			name, next, read := readCteDefinition(tokens, index)
			if !read {
				break
			}
			names[strings.ToLower(name)] = true
			index = next
		}
	}
	return names
}

// relationKeywords is the set of clauses followed by table references.
var relationKeywords = []string{"from", "join", "update", "insert into"}

// FindTableReferences returns table references with their aliases.
func FindTableReferences(sql string, flavour syntax.SyntaxFlavour) []TableReference {
	tokens := syntax.ReadCodeTokens(sql, flavour)
	references := []TableReference{}

	for _, hit := range syntax.FindKeywordsAnywhere(tokens, relationKeywords) {
		index := hit.Index + syntax.CountKeywordTokens(hit.Keyword)
		for {
			reference, next, read := readReferenceAt(sql, tokens, index)
			if !read {
				break
			}
			references = append(references, reference)
			// Only a FROM clause lists more than one relation, separated by commas.
			if hit.Keyword != "from" || !syntax.IsOperator(tokens, next, ",") {
				break
			}
			index = next + 1
		}
	}
	return references
}

// FindSingleSelectTable parses a simple SELECT source, excluding joins, grouping, set operations, and subquery sources.
func FindSingleSelectTable(sql string, flavour syntax.SyntaxFlavour) (SelectSource, bool) {
	statement := strings.TrimRight(strings.TrimSpace(sql), "; \t\r\n")
	tokens := syntax.ReadCodeTokens(statement, flavour)
	if syntax.ReadOpeningWord(tokens) != "select" {
		return SelectSource{}, false
	}
	if len(syntax.FindKeywordsIn(tokens, multiSourceKeywords)) > 0 {
		return SelectSource{}, false
	}

	fromHits := syntax.FindKeywordsIn(tokens, []string{"from"})
	if len(fromHits) != 1 {
		return SelectSource{}, false
	}

	start := fromHits[0].Index + 1
	end := len(tokens)
	for _, hit := range syntax.FindKeywordsIn(tokens, fromEnders) {
		if hit.Index >= start {
			end = hit.Index
			break
		}
	}
	return readTableReference(statement, tokens, start, end)
}
