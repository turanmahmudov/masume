package build

import (
	"fmt"
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
)

// WriteTarget is the target table, result columns, key columns, and SQL dialect.
type WriteTarget struct {
	Table   query.QualifiedName
	Columns []query.ResultColumn
	// An empty list means no column identifies one row, so the whole row is the key.
	KeyColumns []string
	Dialect    *query.Dialect
}

// resolveBindValue converts a cell value to a parameter. DEFAULT requires literal SQL.
func resolveBindValue(value core.CellValue) (any, error) {
	switch value.Kind {
	case core.CellNull:
		return nil, nil
	case core.CellEmpty:
		return "", nil
	case core.CellText:
		return value.Text, nil
	}
	return nil, core.NewEditError("DEFAULT must be an SQL keyword, not a bound parameter")
}

func findColumnIndex(columns []query.ResultColumn, name string) int {
	lowered := strings.ToLower(name)
	for at, column := range columns {
		if strings.ToLower(column.Name) == lowered {
			return at
		}
	}
	return -1
}

// keyPredicate is a row predicate with parameters and a display summary.
type keyPredicate struct {
	text    string
	params  []any
	summary string
}

// FindIdentityColumns returns columns with comparable types for row matching.
func FindIdentityColumns(columns []query.ResultColumn, dialect *query.Dialect) []query.ResultColumn {
	kept := make([]query.ResultColumn, 0, len(columns))
	for _, column := range columns {
		if dialect.CanCompareType(column.DataType) {
			kept = append(kept, column)
		}
	}
	return kept
}

// resolveKeyColumnNames returns primary key columns or all comparable result columns.
func resolveKeyColumnNames(target WriteTarget) ([]string, error) {
	if len(target.KeyColumns) > 0 {
		return target.KeyColumns, nil
	}
	identity := FindIdentityColumns(target.Columns, target.Dialect)
	if len(identity) == 0 {
		return nil, core.NewEditError(
			"cannot identify a row: the table has no comparable columns")
	}
	names := make([]string, 0, len(identity))
	for _, column := range identity {
		names = append(names, column.Name)
	}
	return names, nil
}

// buildKeyPredicate builds a row predicate. Without a primary key, the caller must count matches before writing.
func buildKeyPredicate(
	target WriteTarget, row []any, firstParamIndex int,
) (keyPredicate, error) {
	dialect := target.Dialect
	clauses := []string{}
	bound := query.NewBoundValues(dialect, firstParamIndex)
	summary := []string{}

	names, err := resolveKeyColumnNames(target)
	if err != nil {
		return keyPredicate{}, err
	}

	for _, name := range names {
		index := findColumnIndex(target.Columns, name)
		if index == -1 {
			return keyPredicate{}, core.NewEditError(
				fmt.Sprintf("primary key column %q is not in the result", name))
		}
		var value any
		if index < len(row) {
			value = row[index]
		}
		if value == nil {
			clauses = append(clauses, dialect.QuoteIdentifier(name)+" is null")
			summary = append(summary, name+"="+core.NullText)
			continue
		}
		clauses = append(clauses, dialect.QuoteIdentifier(name)+" = "+bound.Bind(value))
		summary = append(summary, name+"="+core.FormatCell(value, ""))
	}

	return keyPredicate{
		text:    strings.Join(clauses, " and "),
		params:  bound.Params,
		summary: strings.Join(summary, ", "),
	}, nil
}

// buildRowsPredicate combines row predicates with OR for deletes and matching row counts.
func buildRowsPredicate(target WriteTarget, rows [][]any) (keyPredicate, error) {
	clauses := []string{}
	params := []any{}
	summary := []string{}

	for _, row := range rows {
		predicate, err := buildKeyPredicate(target, row, len(params)+1)
		if err != nil {
			return keyPredicate{}, err
		}
		params = append(params, predicate.params...)
		// Parenthesize each predicate when combining multiple rows.
		if len(rows) == 1 {
			clauses = append(clauses, predicate.text)
		} else {
			clauses = append(clauses, "("+predicate.text+")")
		}
		summary = append(summary, predicate.summary)
	}

	return keyPredicate{
		text:    strings.Join(clauses, " or "),
		params:  params,
		summary: strings.Join(summary, ", "),
	}, nil
}

// BuildRowCountStatement builds a count query for one row predicate.
func BuildRowCountStatement(target WriteTarget, row []any) (query.BoundStatement, error) {
	return BuildRowsCountStatement(target, [][]any{row})
}

// BuildRowsCountStatement builds a count query for the selected rows. The count must match the selection before writing.
func BuildRowsCountStatement(target WriteTarget, rows [][]any) (query.BoundStatement, error) {
	predicate, err := buildRowsPredicate(target, rows)
	if err != nil {
		return query.BoundStatement{}, err
	}
	named := target.Dialect.BuildQualifiedName(target.Table)
	return query.BoundStatement{
		SQL: fmt.Sprintf("select %s as matched from %s where %s",
			target.Dialect.CountExpression, named, predicate.text),
		Params: predicate.params,
		Description: fmt.Sprintf("count rows of %s where %s",
			target.Table.Name, predicate.summary),
	}, nil
}

// NeedsRowCountGuard is true for tables without a primary key.
func NeedsRowCountGuard(target WriteTarget) bool {
	return len(target.KeyColumns) == 0
}

// CellAssignment is one column of one row, and what the user chose for it.
type CellAssignment struct {
	ColumnIndex int
	Value       core.CellValue
}

// describeAssignedValue writes how a chosen value appears in the review overlay.
func describeAssignedValue(value core.CellValue) string {
	switch value.Kind {
	case core.CellNull:
		return core.NullText
	case core.CellEmpty:
		return "''"
	case core.CellDefault:
		return "DEFAULT"
	}
	return value.Text
}

// BuildUpdateStatement builds one update for all assigned cells in a row.
func BuildUpdateStatement(
	target WriteTarget, row []any, assignments []CellAssignment,
) (query.BoundStatement, error) {
	dialect := target.Dialect
	if len(assignments) == 0 {
		return query.BoundStatement{}, core.NewEditError("no column assignments")
	}

	clauses := []string{}
	bound := query.NewBoundValues(dialect, 1)
	summary := []string{}

	for _, assignment := range assignments {
		if assignment.ColumnIndex < 0 || assignment.ColumnIndex >= len(target.Columns) {
			return query.BoundStatement{}, core.NewEditError("column index is out of range")
		}
		column := target.Columns[assignment.ColumnIndex]
		assigned := dialect.QuoteIdentifier(column.Name)
		summary = append(summary, column.Name+" = "+describeAssignedValue(assignment.Value))

		if assignment.Value.Kind == core.CellDefault {
			clauses = append(clauses, assigned+" = default")
			continue
		}
		value, err := resolveBindValue(assignment.Value)
		if err != nil {
			return query.BoundStatement{}, err
		}
		clauses = append(clauses, assigned+" = "+bound.Bind(value))
	}

	predicate, err := buildKeyPredicate(target, row, len(bound.Params)+1)
	if err != nil {
		return query.BoundStatement{}, err
	}
	named := dialect.BuildQualifiedName(target.Table)

	return query.BoundStatement{
		SQL: dialect.BuildUpdateRow(
			named, strings.Join(clauses, ", "), predicate.text),
		Params: append(append([]any{}, bound.Params...), predicate.params...),
		Description: dialect.BuildUpdateRow(
			target.Table.Name, strings.Join(summary, ", "), predicate.summary),
	}, nil
}

// buildDeleteChunk removes a set of rows with one statement, one clause per row.
func buildDeleteChunk(target WriteTarget, rows [][]any) (query.BoundStatement, error) {
	dialect := target.Dialect
	predicate, err := buildRowsPredicate(target, rows)
	if err != nil {
		return query.BoundStatement{}, err
	}

	named := dialect.BuildQualifiedName(target.Table)
	description := fmt.Sprintf("delete %d rows from %s", len(rows), target.Table.Name)
	if len(rows) == 1 {
		description = fmt.Sprintf("delete from %s where %s",
			target.Table.Name, predicate.summary)
	}
	return query.BoundStatement{
		SQL:         fmt.Sprintf("delete from %s where %s", named, predicate.text),
		Params:      predicate.params,
		Description: description,
	}, nil
}

// BuildDeleteStatements builds deletes in batches within the parameter limit.
func BuildDeleteStatements(
	target WriteTarget, rows [][]any, maxParameters int,
) ([]query.BoundStatement, error) {
	chunks, err := buildDeleteChunks(target, rows, maxParameters)
	if err != nil {
		return nil, err
	}
	statements := []query.BoundStatement{}
	for _, chunk := range chunks {
		statement, err := buildDeleteChunk(target, chunk)
		if err != nil {
			return nil, err
		}
		statements = append(statements, statement)
	}
	return statements, nil
}

// buildDeleteChunks groups rows within the parameter limit for delete and count queries.
func buildDeleteChunks(
	target WriteTarget, rows [][]any, maxParameters int,
) ([][][]any, error) {
	if maxParameters <= 0 {
		maxParameters = target.Dialect.ResolveBindLimit()
	}
	chunks := [][][]any{}
	chunk := [][]any{}
	bound := 0

	for _, row := range rows {
		predicate, err := buildKeyPredicate(target, row, 1)
		if err != nil {
			return nil, err
		}
		cost := len(predicate.params)
		if len(chunk) > 0 && bound+cost > maxParameters {
			chunks = append(chunks, chunk)
			chunk, bound = nil, 0
		}
		chunk = append(chunk, row)
		bound += cost
	}
	if len(chunk) > 0 {
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// BuildInsertStatement builds an INSERT from a column-value map. Omitted columns use server defaults.
func BuildInsertStatement(
	table query.QualifiedName, values map[string]any, dialect *query.Dialect,
) (query.BoundStatement, error) {
	if len(values) == 0 {
		return query.BoundStatement{}, core.NewEditError("give at least one column a value")
	}

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)

	target := dialect.BuildQualifiedName(table)
	quoted := make([]string, 0, len(names))
	placeholders := make([]string, 0, len(names))
	bound := query.NewBoundValues(dialect, 1)
	for _, name := range names {
		quoted = append(quoted, dialect.QuoteIdentifier(name))
		placeholders = append(placeholders, bound.Bind(values[name]))
	}

	return query.BoundStatement{
		SQL: fmt.Sprintf("insert into %s (%s) values (%s)",
			target, strings.Join(quoted, ", "), strings.Join(placeholders, ", ")),
		Params: bound.Params,
		Description: fmt.Sprintf("insert into %s (%s)",
			table.Name, strings.Join(names, ", ")),
	}, nil
}

// ChangeStatement is a staged write with an optional row count check.
type ChangeStatement struct {
	Statement query.BoundStatement
	// Guard is the count query for a table without a primary key. The count must equal Expect before writing.
	Guard *query.BoundStatement
	// Expect is the rows the user chose, which the guard has to match exactly.
	Expect int
}

// buildGuard builds a count query for a keyless table, or returns nil for a table with a primary key.
func buildGuard(target WriteTarget, rows [][]any) (*query.BoundStatement, error) {
	if !NeedsRowCountGuard(target) {
		return nil, nil
	}
	guard, err := BuildRowsCountStatement(target, rows)
	if err != nil {
		return nil, err
	}
	return &guard, nil
}

// BuildChangeStatements builds inserts, updates, then deletes. Deleted rows skip updates. Keyless updates and deletes include row count checks.
func BuildChangeStatements(
	target WriteTarget, rows [][]any, pending core.PendingChanges,
) ([]ChangeStatement, error) {
	statements := []ChangeStatement{}

	// Inserts require no row count check.
	for _, values := range pending.Inserts {
		statement, err := BuildInsertStatement(target.Table, values, target.Dialect)
		if err != nil {
			return nil, err
		}
		statements = append(statements, ChangeStatement{Statement: statement})
	}

	// Group cell assignments by row.
	byRow := map[int][]CellAssignment{}
	order := []int{}
	for _, edit := range core.SortedEdits(pending) {
		if pending.DeletedRows[edit.RowIndex] {
			continue
		}
		if edit.RowIndex < 0 || edit.RowIndex >= len(rows) {
			continue
		}
		if _, held := byRow[edit.RowIndex]; !held {
			order = append(order, edit.RowIndex)
		}
		byRow[edit.RowIndex] = append(byRow[edit.RowIndex],
			CellAssignment{ColumnIndex: edit.ColumnIndex, Value: edit.Value})
	}

	for _, rowIndex := range order {
		statement, err := BuildUpdateStatement(target, rows[rowIndex], byRow[rowIndex])
		if err != nil {
			return nil, err
		}
		guard, err := buildGuard(target, [][]any{rows[rowIndex]})
		if err != nil {
			return nil, err
		}
		statements = append(statements,
			ChangeStatement{Statement: statement, Guard: guard, Expect: 1})
	}

	deleted := [][]any{}
	for _, rowIndex := range core.SortedDeletedRows(pending) {
		if rowIndex >= 0 && rowIndex < len(rows) {
			deleted = append(deleted, rows[rowIndex])
		}
	}
	removals, err := BuildDeleteChangeStatements(
		target, deleted, target.Dialect.ResolveBindLimit())
	if err != nil {
		return nil, err
	}
	return append(statements, removals...), nil
}

// BuildDeleteChangeStatements builds batched deletes with a row count check for each keyless batch.
func BuildDeleteChangeStatements(
	target WriteTarget, rows [][]any, maxParameters int,
) ([]ChangeStatement, error) {
	chunks, err := buildDeleteChunks(target, rows, maxParameters)
	if err != nil {
		return nil, err
	}
	statements := make([]ChangeStatement, 0, len(chunks))
	for _, chunk := range chunks {
		statement, err := buildDeleteChunk(target, chunk)
		if err != nil {
			return nil, err
		}
		guard, err := buildGuard(target, chunk)
		if err != nil {
			return nil, err
		}
		statements = append(statements, ChangeStatement{
			Statement: statement, Guard: guard, Expect: len(chunk),
		})
	}
	return statements, nil
}

// FindForeignKeyTarget returns the referenced column for a grid column.
func FindForeignKeyTarget(
	foreignKeys []query.ForeignKey, columnName string,
) (query.ForeignKeyTarget, bool) {
	lowered := strings.ToLower(columnName)
	for _, key := range foreignKeys {
		for position, column := range key.Columns {
			if strings.ToLower(column) != lowered {
				continue
			}
			if position >= len(key.TargetColumns) {
				continue
			}
			return query.ForeignKeyTarget{
				Schema: key.TargetSchema, Table: key.TargetTable,
				Column: key.TargetColumns[position],
			}, true
		}
	}
	return query.ForeignKeyTarget{}, false
}
