package load

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
)

// An import creates a table if necessary, then inserts rows in batches with bound values.

// BuildCreateTable returns a CREATE TABLE statement with engine-specific column types.
func BuildCreateTable(plan Plan, dialect *query.Dialect) string {
	mapped := plan.ListMappedColumns()
	lines := make([]string, 0, len(mapped))
	for _, mapping := range mapped {
		lines = append(lines, "  "+dialect.QuoteIdentifierIfNeeded(mapping.Target)+
			" "+dialect.BuildColumnType(mapping.Kind))
	}
	return fmt.Sprintf("create table %s (\n%s\n)",
		dialect.BuildQualifiedName(plan.Table), strings.Join(lines, ",\n"))
}

// writeInsert returns one INSERT statement. The write callback is the value renderer or parameter binder.
func writeInsert(
	plan Plan, rows [][]any, dialect *query.Dialect, write func(any) string,
) (string, error) {
	mapped := plan.ListMappedColumns()
	if len(mapped) == 0 {
		return "", failValue("no source columns are mapped")
	}
	if len(rows) == 0 {
		return "", failValue("no rows to insert")
	}

	quoted := make([]string, 0, len(mapped))
	for _, mapping := range mapped {
		quoted = append(quoted, dialect.QuoteIdentifierIfNeeded(mapping.Target))
	}

	groups := make([]string, 0, len(rows))
	for _, row := range rows {
		if len(row) != len(mapped) {
			return "", failValue(
				"the row has %d values; expected %d mapped columns",
				len(row), len(mapped))
		}
		written := make([]string, 0, len(row))
		for _, value := range row {
			written = append(written, write(value))
		}
		groups = append(groups, "("+strings.Join(written, ", ")+")")
	}

	return fmt.Sprintf("insert into %s (%s)\nvalues %s",
		dialect.BuildQualifiedName(plan.Table),
		strings.Join(quoted, ", "), strings.Join(groups, ",\n       ")), nil
}

// BuildInsert returns an INSERT statement with bound values. The input rows already have their target types.
func BuildInsert(
	plan Plan, rows [][]any, dialect *query.Dialect,
) (query.BoundStatement, error) {
	bound := query.NewBoundValues(dialect, 1)
	written, err := writeInsert(plan, rows, dialect, bound.Bind)
	if err != nil {
		return query.BoundStatement{}, err
	}
	return query.BoundStatement{
		SQL: written, Params: bound.Params,
		Description: fmt.Sprintf("insert %d rows into %s", len(rows), plan.Table.Name),
	}, nil
}

// BuildShownInsert returns an INSERT preview with literal values. The preview is never executed.
func BuildShownInsert(plan Plan, rows [][]any, dialect *query.Dialect) (string, error) {
	return writeInsert(plan, rows, dialect, func(value any) string {
		if value == nil {
			return "null"
		}
		return build.RenderLiteral(value, dialect, "")
	})
}

// BuildRows converts mapped values to their target types. A conversion error stops the batch.
func BuildRows(plan Plan, rows []Row) ([][]any, error) {
	mapped := plan.ListMappedColumns()
	indexes := plan.buildSourceIndexes()

	built := make([][]any, 0, len(rows))
	for _, row := range rows {
		values := make([]any, 0, len(mapped))
		for _, mapping := range mapped {
			at, held := indexes[mapping.Source]
			if !held || at >= len(row.Values) {
				values = append(values, nil)
				continue
			}
			value, err := CastValue(row.Values[at], mapping.Kind)
			if err != nil {
				return nil, failValue("line %d, %s: %s",
					row.Line, mapping.Source, err.Error())
			}
			values = append(values, value)
		}
		built = append(built, values)
	}
	return built, nil
}

// DescribeStatements returns the optional CREATE TABLE statement and an INSERT preview.
func DescribeStatements(plan Plan, dialect *query.Dialect) ([]string, error) {
	written := []string{}
	if plan.CreatesTable {
		written = append(written, BuildCreateTable(plan, dialect))
	}

	// Only a row the import would write is shown.
	shown := make([]Row, 0, describedRows)
	for _, row := range plan.Sample.Rows {
		if len(shown) >= describedRows {
			break
		}
		if plan.HoldsWritableRow(row, len(plan.Sample.Columns)) {
			shown = append(shown, row)
		}
	}
	if len(shown) == 0 {
		return written, nil
	}

	values, err := BuildRows(plan, shown)
	if err != nil {
		return nil, err
	}
	shownInsert, err := BuildShownInsert(plan, values, dialect)
	if err != nil {
		return nil, err
	}
	return append(written, shownInsert), nil
}

// describedRows is the maximum rows in an INSERT preview.
const describedRows = 5
