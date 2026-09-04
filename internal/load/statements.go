package load

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
)

// The statements one import runs: the table where it makes one, and an insert per batch of
// rows. Every value is bound.

// BuildCreateTable returns the statement that makes the table the import writes into, in
// the types of the server it is written for.
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

// writeInsert returns one insert of the rows given. `write` decides whether a value is bound
// to the statement or written into it.
func writeInsert(
	plan Plan, rows [][]any, dialect *query.Dialect, write func(any) string,
) (string, error) {
	mapped := plan.ListMappedColumns()
	if len(mapped) == 0 {
		return "", failValue("no column of the file is written")
	}
	if len(rows) == 0 {
		return "", failValue("there is no row to write")
	}

	quoted := make([]string, 0, len(mapped))
	for _, mapping := range mapped {
		quoted = append(quoted, dialect.QuoteIdentifierIfNeeded(mapping.Target))
	}

	groups := make([]string, 0, len(rows))
	for _, row := range rows {
		if len(row) != len(mapped) {
			return "", failValue(
				"a row holds %d values and the import writes %d columns",
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

// BuildInsert returns one insert of the rows given, with every value bound. The rows are
// already cast to the kind of their column.
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

// BuildShownInsert returns the insert with its values written in, for the review to show.
// It is never run.
func BuildShownInsert(plan Plan, rows [][]any, dialect *query.Dialect) (string, error) {
	return writeInsert(plan, rows, dialect, func(value any) string {
		// A value that is not there is written as the server reads it.
		if value == nil {
			return "null"
		}
		return build.RenderLiteral(value, dialect, "")
	})
}

// BuildRows returns the values of the rows given, cast to the kind of the column each one is
// written into. A value the column cannot hold stops the batch.
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

// DescribeStatements returns the statements of the import as the review shows them: the
// table where one is made, and the first insert with its values written in.
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

// describedRows is how many rows the review writes out.
const describedRows = 5
