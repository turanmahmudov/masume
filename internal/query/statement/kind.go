package statement

import (
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// readStarts are the words a statement can start with and still only read.
var readStarts = map[string]bool{"select": true, "with": true, "table": true, "values": true}

// IsPageable is true if the client can read the statement one page at a time.
func IsPageable(sql string, flavour syntax.SyntaxFlavour) bool {
	tokens := syntax.ReadCodeTokens(sql, flavour)
	opening := syntax.ReadOpeningWord(tokens)
	if opening == "" || !readStarts[opening] {
		return false
	}
	return len(syntax.FindKeywordsAnywhere(tokens, WriteKeywords)) == 0
}

// plannedStarts are the words a server plans. A server plans the rows a statement
// reads, but not a statement that defines an object or grants a right.
var plannedStarts = map[string]bool{
	"select": true, "with": true, "table": true, "values": true,
	"insert": true, "update": true, "delete": true, "merge": true, "replace": true,
}

// plannedCreateObjects are the objects a CREATE fills from a query.
var plannedCreateObjects = []string{"table", "materialized view"}

// definesFromQuery is true for `create table t as select …` and for a materialized view.
func definesFromQuery(tokens []syntax.CodeToken) bool {
	if syntax.ReadOpeningWord(tokens) != "create" {
		return false
	}
	asHits := syntax.FindKeywordsIn(tokens, []string{"as"})
	if len(asHits) == 0 {
		return false
	}
	objectHits := syntax.FindKeywordsIn(tokens, plannedCreateObjects)
	return len(objectHits) > 0 && objectHits[0].Start < asHits[0].Start
}

// CanExplain is true if the server can plan the statement.
func CanExplain(sql string, flavour syntax.SyntaxFlavour) bool {
	tokens := syntax.ReadCodeTokens(sql, flavour)
	opening := syntax.ReadOpeningWord(tokens)
	if opening == "" {
		return false
	}
	return plannedStarts[opening] || definesFromQuery(tokens)
}
