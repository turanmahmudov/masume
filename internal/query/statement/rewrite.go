package statement

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// EffectiveStatement is a statement ready to run, with its bind values.
type EffectiveStatement struct {
	SQL    string
	Params []any
}

// StatementParts is a statement split around its ORDER BY and trailing clauses.
type StatementParts struct {
	Body       string
	OrderBy    string
	HasOrderBy bool
	Tail       string
	Terminator string
}

var splitKeywords = []string{"order by", "limit", "offset", "fetch"}

// SplitStatement separates ORDER BY from trailing LIMIT, OFFSET, or FETCH clauses.
func SplitStatement(sql string, flavour syntax.SyntaxFlavour) StatementParts {
	withoutTerminator := strings.TrimRight(sql, " \t\r\n")
	terminator := ""
	statement := withoutTerminator
	if strings.HasSuffix(withoutTerminator, ";") {
		terminator = ";"
		statement = strings.TrimRight(withoutTerminator[:len(withoutTerminator)-1], " \t\r\n")
	}

	hits := syntax.FindTopLevelKeywords(statement, splitKeywords, flavour)
	tailStart := len(statement)
	for _, hit := range hits {
		if hit.Keyword != "order by" {
			tailStart = hit.Start
			break
		}
	}

	orderByStart := -1
	for _, hit := range hits {
		if hit.Keyword == "order by" {
			orderByStart = hit.Start
		}
	}

	tail := strings.TrimSpace(statement[tailStart:])
	if orderByStart != -1 && orderByStart < tailStart {
		return StatementParts{
			Body:       strings.TrimRight(statement[:orderByStart], " \t\r\n"),
			OrderBy:    strings.TrimSpace(statement[orderByStart:tailStart]),
			HasOrderBy: true,
			Tail:       tail,
			Terminator: terminator,
		}
	}
	return StatementParts{
		Body:       strings.TrimRight(statement[:tailStart], " \t\r\n"),
		Tail:       tail,
		Terminator: terminator,
	}
}

func joinParts(parts []string, terminator string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "\n") + terminator
}

// Subquery aliases for generated wrappers.
const (
	filterAlias = "masume_filter"
	pageAlias   = "masume_page"
	countAlias  = "masume_count"
)

// wrapStatement builds a subquery. A newline before the closing parenthesis terminates any trailing line comment.
func wrapStatement(inner, alias string) string {
	return "select * from (\n" + inner + "\n) as " + alias
}

func buildSortKeys(sort []core.SortState, dialect *query.Dialect) string {
	keys := make([]string, 0, len(sort))
	for _, key := range sort {
		keys = append(keys, dialect.QuoteIdentifier(key.Column)+" "+string(key.Direction))
	}
	return strings.Join(keys, ", ")
}

// ApplyOrderBy replaces the top-level sort with keys in the supplied order.
func ApplyOrderBy(sql string, sort []core.SortState, dialect *query.Dialect) string {
	parts := SplitStatement(sql, dialect.Syntax)
	if len(sort) == 0 {
		return joinParts([]string{parts.Body, parts.Tail}, parts.Terminator)
	}
	return joinParts(
		[]string{parts.Body, "order by " + buildSortKeys(sort, dialect), parts.Tail},
		parts.Terminator)
}

// FindOrderByColumns returns simple column sort keys, or nil for unsupported expressions.
func FindOrderByColumns(sql string, flavour syntax.SyntaxFlavour) []core.SortState {
	parts := SplitStatement(sql, flavour)
	if !parts.HasOrderBy {
		return nil
	}

	tokens := syntax.ReadCodeTokens(parts.OrderBy, flavour)
	if len(tokens) < 2 || tokens[0].Text != "order" || tokens[1].Text != "by" {
		return nil
	}

	keys := []core.SortState{}
	index := 2
	for index < len(tokens) {
		column, isName := syntax.ReadIdentifier(parts.OrderBy, tokens, index)
		if !isName {
			return nil
		}
		index++

		direction := core.SortAscending
		if next, present := syntax.TokenAt(tokens, index); present {
			if next.Text == "asc" || next.Text == "desc" {
				direction = core.SortDirection(next.Text)
				index++
			}
		}
		keys = append(keys, core.SortState{Column: column, Direction: direction})

		if _, present := syntax.TokenAt(tokens, index); !present {
			break
		}
		// Reject expressions and NULLS clauses.
		if !syntax.IsOperator(tokens, index, ",") {
			return nil
		}
		index++
	}
	return keys
}

// BuildRelationSQL builds a table query without paging.
func BuildRelationSQL(
	table query.QualifiedName, predicate *build.Predicate, sort []core.SortState, dialect *query.Dialect,
) EffectiveStatement {
	lines := []string{"select *", "  from " + dialect.BuildQualifiedName(table)}
	if predicate != nil && strings.TrimSpace(predicate.Text) != "" {
		lines = append(lines, " where "+strings.TrimSpace(predicate.Text))
	}
	if len(sort) > 0 {
		lines = append(lines, " order by "+buildSortKeys(sort, dialect))
	}
	statement := EffectiveStatement{SQL: strings.Join(lines, "\n")}
	if predicate != nil {
		statement.Params = predicate.Params
	}
	return statement
}

// ApplyWhere wraps the query with a filter and moves trailing limit clauses outside. ORDER BY remains inside the subquery.
func ApplyWhere(sql, predicate string, flavour syntax.SyntaxFlavour) string {
	trimmed := strings.TrimSpace(predicate)
	parts := SplitStatement(sql, flavour)
	if trimmed == "" || parts.Body == "" {
		return sql
	}
	inner := joinInner(parts.Body, parts.OrderBy)
	return joinParts(
		[]string{wrapStatement(inner, filterAlias), "where " + trimmed, parts.Tail},
		parts.Terminator)
}

func joinInner(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "\n")
}

// BuildEffectiveSQL applies the grid filter and sort to a statement.
func BuildEffectiveSQL(
	sql string, predicate *build.Predicate, sort []core.SortState, dialect *query.Dialect,
) EffectiveStatement {
	filtered := sql
	if predicate != nil {
		filtered = ApplyWhere(sql, predicate.Text, dialect.Syntax)
	}
	ordered := filtered
	if len(sort) > 0 {
		ordered = ApplyOrderBy(filtered, sort, dialect)
	}
	statement := EffectiveStatement{SQL: ordered}
	if predicate != nil {
		statement.Params = predicate.Params
	}
	return statement
}

// BuildCountSQL wraps a query with a count, excluding ORDER BY and preserving trailing limit clauses.
func BuildCountSQL(sql string, dialect *query.Dialect) string {
	parts := SplitStatement(sql, dialect.Syntax)
	if parts.Body == "" {
		return sql
	}
	inner := joinInner(parts.Body, parts.Tail)
	return fmt.Sprintf("select %s as total from (\n%s\n) as %s%s",
		dialect.CountExpression, inner, countAlias, parts.Terminator)
}

// ApplyPaging adds a page limit and offset, using a subquery when trailing limit clauses exist.
func ApplyPaging(sql string, limit, offset int, flavour syntax.SyntaxFlavour) string {
	parts := SplitStatement(sql, flavour)
	if parts.Body == "" {
		return sql
	}
	window := fmt.Sprintf("limit %d", limit)
	if offset > 0 {
		window = fmt.Sprintf("limit %d offset %d", limit, offset)
	}

	if parts.Tail == "" {
		return joinParts([]string{parts.Body, parts.OrderBy, window}, parts.Terminator)
	}
	// ORDER BY can reference columns absent from the projection.
	inner := joinInner(parts.Body, parts.OrderBy, parts.Tail)
	return joinParts([]string{wrapStatement(inner, pageAlias), window}, parts.Terminator)
}

// DescribeRewrite summarizes the grid sort and filter.
func DescribeRewrite(sort []core.SortState, filter string) string {
	parts := []string{}
	if len(sort) > 0 {
		keys := make([]string, 0, len(sort))
		for _, key := range sort {
			keys = append(keys, key.Column+" "+string(key.Direction))
		}
		parts = append(parts, "order by "+strings.Join(keys, ", "))
	}
	if strings.TrimSpace(filter) != "" {
		parts = append(parts, "where "+strings.TrimSpace(filter))
	}
	return strings.Join(parts, " · ")
}
