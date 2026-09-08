package clickhouse

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// readCommands is the set of commands that answer with rows.
var readCommands = map[string]bool{
	"select": true, "with": true, "show": true, "describe": true, "desc": true,
	"explain": true, "exists": true, "check": true,
}

// plannedCommands is the set of commands the server plans without running.
var plannedCommands = map[string]bool{"select": true, "with": true}

// ReadsRows is true for a command whose answer is a result set.
func ReadsRows(command string) bool {
	return readCommands[strings.ToLower(command)]
}

// PlansStatement is true for a command the server plans, which is how a statement is
// checked without running it.
func PlansStatement(command string) bool {
	return plannedCommands[strings.ToLower(command)]
}

// relationKindByEngine reads the engine `system.tables` names for a view.
var relationKindByEngine = map[string]db.RelationKind{
	"view": db.RelationView, "materializedview": db.RelationMaterializedView,
	"livewview": db.RelationView, "windowview": db.RelationView,
}

// MapRelationKind reads the kind of a relation out of the engine it runs on. Every other
// engine holds the rows of a table itself.
func MapRelationKind(engine any) db.RelationKind {
	kind, known := relationKindByEngine[strings.ToLower(db.ReadAnyText(engine))]
	if !known {
		return db.RelationTable
	}
	return kind
}

// nullablePrefix opens the type of a column that takes a null.
const nullablePrefix = "nullable("

// ReadsNull is true for a column type that takes a null.
func ReadsNull(dataType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(dataType)), nullablePrefix)
}

// byteArrayType is the one type the driver hands over as raw bytes.
const byteArrayType = "array(uint8)"

// rowReader reads the values of one row, in the shapes this driver returns them.
type rowReader struct {
	// True for a column whose value stays bytes.
	binary []bool
}

// readRow reads one row as plain values.
func (reader rowReader) readRow(rows *sql.Rows) ([]any, error) {
	return db.ScanRow(rows, len(reader.binary), reader.binary...)
}

// buildClickhouseColumns reads the columns of a result with the reader of its values.
func buildClickhouseColumns(rows *sql.Rows) ([]db.ResultColumn, rowReader, error) {
	names, err := rows.Columns()
	if err != nil {
		return nil, rowReader{}, err
	}
	if len(names) == 0 {
		return nil, rowReader{}, nil
	}
	types, typeErr := rows.ColumnTypes()
	if typeErr != nil {
		return nil, rowReader{}, typeErr
	}

	columns := make([]db.ResultColumn, 0, len(names))
	reader := rowReader{binary: make([]bool, 0, len(names))}
	for at, name := range names {
		dataType := ""
		if at < len(types) && types[at] != nil {
			dataType = types[at].DatabaseTypeName()
		}
		if name == "" {
			// The server names no column of a computed expression.
			name = "column" + strconv.Itoa(at+1)
		}
		columns = append(columns, db.ResultColumn{Name: name, DataType: dataType})
		reader.binary = append(reader.binary,
			strings.ToLower(dataType) == byteArrayType)
	}
	return columns, reader, nil
}

// readClickhouseRows reads a result as rows of plain values, with its columns.
func readClickhouseRows(rows *sql.Rows, cap int) ([][]any, []db.ResultColumn, error) {
	columns, reader, err := buildClickhouseColumns(rows)
	if err != nil || columns == nil {
		return nil, nil, err
	}

	read := [][]any{}
	for rows.Next() {
		values, scanErr := reader.readRow(rows)
		if scanErr != nil {
			return nil, nil, scanErr
		}
		read = append(read, values)
		if cap >= 0 && len(read) >= cap {
			break
		}
	}
	return read, columns, rows.Err()
}
