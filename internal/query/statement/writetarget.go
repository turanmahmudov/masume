package statement

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Write target parsing for row counts before execution.

// WriteKind is the write operation.
type WriteKind string

// The kinds of write a plan is built for.
const (
	WriteInsert   WriteKind = "insert"
	WriteUpdate   WriteKind = "update"
	WriteDelete   WriteKind = "delete"
	WriteTruncate WriteKind = "truncate"
)

// WriteTarget is a parsed write with its target table, predicate, and assigned columns.
type WriteTarget struct {
	Kind  WriteKind
	Table SelectSource
	// The WHERE clause text without the keyword.
	Where    string
	HasWhere bool
	// The columns a SET assigns.
	Columns []string
}

// whereEnders name the clauses that end a WHERE.
var whereEnders = []string{"returning", "order by", "limit", "offset", "fetch"}

// joiningKeywords is the set of clauses excluded from single-table write measurement.
var joiningKeywords = []string{"from", "using", "join", "select", "with"}

func countsJoins(tokens []syntax.CodeToken) int {
	return len(syntax.FindKeywordsIn(tokens, joiningKeywords))
}

// readWriteRelation parses a target table and rejects aliases.
func readWriteRelation(
	sql string, tokens []syntax.CodeToken, index int,
) (SelectSource, int, bool) {
	reference, next, read := readReferenceAt(sql, tokens, index)
	if !read || reference.HasAlias {
		return SelectSource{}, 0, false
	}
	return reference.SelectSource, next, true
}

func readWhereClause(sql string, tokens []syntax.CodeToken) (string, bool) {
	hits := syntax.FindKeywordsIn(tokens, []string{"where"})
	if len(hits) != 1 {
		return "", false
	}
	first, present := syntax.TokenAt(tokens, hits[0].Index+1)
	if !present {
		return "", false
	}

	end := len(sql)
	for _, hit := range syntax.FindKeywordsIn(tokens, whereEnders) {
		if hit.Start > hits[0].Start {
			end = hit.Start
			break
		}
	}
	if first.Start >= end {
		return "", false
	}
	return strings.TrimSpace(sql[first.Start:end]), true
}

// readAssignedColumns reads assigned columns after SET and top-level commas.
func readAssignedColumns(sql string, tokens []syntax.CodeToken, from, to int) []string {
	columns := []string{}
	depth := 0
	expects := true

	for at := from; at < to && at < len(tokens); at++ {
		switch {
		case syntax.IsOperator(tokens, at, "("):
			depth++
		case syntax.IsOperator(tokens, at, ")"):
			depth--
		case depth == 0 && syntax.IsOperator(tokens, at, ","):
			expects = true
		case expects && depth == 0:
			name, read := syntax.ReadIdentifier(sql, tokens, at)
			if read {
				columns = append(columns, name)
			}
			expects = false
		}
	}
	return columns
}

func readUpdateTarget(sql string, tokens []syntax.CodeToken) (WriteTarget, bool) {
	table, next, read := readWriteRelation(sql, tokens, 1)
	if !read {
		return WriteTarget{}, false
	}
	hits := syntax.FindKeywordsIn(tokens, []string{"set"})
	if len(hits) != 1 || hits[0].Index != next {
		return WriteTarget{}, false
	}

	end := len(tokens)
	for _, hit := range syntax.FindKeywordsIn(
		tokens, append([]string{"where"}, whereEnders...)) {
		if hit.Index > hits[0].Index {
			end = hit.Index
			break
		}
	}

	target := WriteTarget{
		Kind:    WriteUpdate,
		Table:   table,
		Columns: readAssignedColumns(sql, tokens, hits[0].Index+1, end),
	}
	target.Where, target.HasWhere = readWhereClause(sql, tokens)
	return target, len(target.Columns) > 0
}

func readDeleteTarget(sql string, tokens []syntax.CodeToken) (WriteTarget, bool) {
	if token, present := syntax.TokenAt(tokens, 1); !present || token.Text != "from" {
		return WriteTarget{}, false
	}
	table, _, read := readWriteRelation(sql, tokens, 2)
	if !read {
		return WriteTarget{}, false
	}
	target := WriteTarget{Kind: WriteDelete, Table: table}
	target.Where, target.HasWhere = readWhereClause(sql, tokens)
	return target, true
}

func readTruncateTarget(sql string, tokens []syntax.CodeToken) (WriteTarget, bool) {
	at := 1
	if token, present := syntax.TokenAt(tokens, at); present && token.Text == "table" {
		at++
	}
	// Require one table without trailing tokens.
	table, next, read := readWriteRelation(sql, tokens, at)
	if !read || next != len(tokens) {
		return WriteTarget{}, false
	}
	return WriteTarget{Kind: WriteTruncate, Table: table}, true
}

func readInsertTarget(sql string, tokens []syntax.CodeToken) (WriteTarget, bool) {
	if token, present := syntax.TokenAt(tokens, 1); !present || token.Text != "into" {
		return WriteTarget{}, false
	}
	table, _, read := readWriteRelation(sql, tokens, 2)
	if !read {
		return WriteTarget{}, false
	}
	return WriteTarget{Kind: WriteInsert, Table: table}, true
}

// ReadWriteTarget parses supported single-statement writes. UPDATE and DELETE joins and target aliases are unsupported.
func ReadWriteTarget(sql string, flavour syntax.SyntaxFlavour) (WriteTarget, bool) {
	if len(SplitStatements(sql, flavour)) > 1 {
		return WriteTarget{}, false
	}
	written := strings.TrimRight(strings.TrimSpace(sql), "; \t\r\n")
	tokens := syntax.ReadCodeTokens(written, flavour)
	switch syntax.ReadOpeningWord(tokens) {
	case "update":
		if countsJoins(tokens) > 0 {
			return WriteTarget{}, false
		}
		return readUpdateTarget(written, tokens)
	case "delete":
		// The FROM of the delete itself is the one join word it may carry.
		if countsJoins(tokens) > 1 {
			return WriteTarget{}, false
		}
		return readDeleteTarget(written, tokens)
	case "truncate":
		if countsJoins(tokens) > 0 {
			return WriteTarget{}, false
		}
		return readTruncateTarget(written, tokens)
	case "insert":
		return readInsertTarget(written, tokens)
	}
	return WriteTarget{}, false
}
