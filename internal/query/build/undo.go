package build

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
)

// Undo statements use bound parameters for execution and inline values for display.

// writeValue is a parameter binder or literal formatter.
type writeValue func(value any) string

func renderStoredValue(value any, dialect *query.Dialect) string {
	if value == nil {
		return "null"
	}
	return RenderLiteral(value, dialect, "")
}

// writeKeyMatch builds a row predicate with IS NULL for null key values.
func writeKeyMatch(target WriteTarget, row []any, write writeValue) (string, error) {
	names, err := resolveKeyColumnNames(target)
	if err != nil {
		return "", err
	}

	clauses := make([]string, 0, len(names))
	for _, name := range names {
		index := findColumnIndex(target.Columns, name)
		if index == -1 || index >= len(row) {
			return "", core.NewEditError(
				fmt.Sprintf("key column %q is missing from the captured row", name))
		}
		quoted := target.Dialect.QuoteIdentifier(name)
		if row[index] == nil {
			clauses = append(clauses, quoted+" is null")
			continue
		}
		clauses = append(clauses, quoted+" = "+write(row[index]))
	}
	return strings.Join(clauses, " and "), nil
}

func writeUndoUpdate(
	target WriteTarget, row []any, columns []int, write writeValue,
) (string, error) {
	if len(columns) == 0 {
		return "", core.NewEditError("no columns to restore")
	}

	clauses := make([]string, 0, len(columns))
	for _, at := range columns {
		if at < 0 || at >= len(target.Columns) || at >= len(row) {
			return "", core.NewEditError("column index is out of range")
		}
		clauses = append(clauses,
			target.Dialect.QuoteIdentifier(target.Columns[at].Name)+" = "+write(row[at]))
	}

	match, err := writeKeyMatch(target, row, write)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("update %s set %s where %s",
		target.Dialect.BuildQualifiedName(target.Table),
		strings.Join(clauses, ", "), match), nil
}

// BuildUndoUpdate builds a parameterized update to restore the selected columns.
func BuildUndoUpdate(
	target WriteTarget, row []any, columns []int,
) (query.BoundStatement, error) {
	bound := query.NewBoundValues(target.Dialect, 1)
	written, err := writeUndoUpdate(target, row, columns, bound.Bind)
	if err != nil {
		return query.BoundStatement{}, err
	}
	names := make([]string, 0, len(columns))
	for _, at := range columns {
		names = append(names, target.Columns[at].Name)
	}
	return query.BoundStatement{
		SQL: written, Params: bound.Params,
		Description: fmt.Sprintf("restore %s in one row of %s",
			strings.Join(names, ", "), target.Table.Name),
	}, nil
}

// BuildShownUndoUpdate builds the undo update with inline values for display.
func BuildShownUndoUpdate(target WriteTarget, row []any, columns []int) (string, error) {
	return writeUndoUpdate(target, row, columns, func(value any) string {
		return renderStoredValue(value, target.Dialect)
	})
}

func writeUndoInsert(target WriteTarget, row []any, write writeValue) (string, error) {
	if len(target.Columns) == 0 || len(row) < len(target.Columns) {
		return "", core.NewEditError("the captured row has missing columns or values")
	}

	quoted := make([]string, 0, len(target.Columns))
	values := make([]string, 0, len(target.Columns))
	for at, column := range target.Columns {
		quoted = append(quoted, target.Dialect.QuoteIdentifier(column.Name))
		values = append(values, write(row[at]))
	}
	return fmt.Sprintf("insert into %s (%s) values (%s)",
		target.Dialect.BuildQualifiedName(target.Table),
		strings.Join(quoted, ", "), strings.Join(values, ", ")), nil
}

// BuildUndoInsert builds a parameterized insert to restore a deleted row.
func BuildUndoInsert(target WriteTarget, row []any) (query.BoundStatement, error) {
	bound := query.NewBoundValues(target.Dialect, 1)
	written, err := writeUndoInsert(target, row, bound.Bind)
	if err != nil {
		return query.BoundStatement{}, err
	}
	return query.BoundStatement{
		SQL: written, Params: bound.Params,
		Description: "restore one deleted row in " + target.Table.Name,
	}, nil
}

// BuildShownUndoInsert builds the undo insert with inline values for display.
func BuildShownUndoInsert(target WriteTarget, row []any) (string, error) {
	return writeUndoInsert(target, row, func(value any) string {
		return renderStoredValue(value, target.Dialect)
	})
}
