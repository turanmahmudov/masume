package sqlserver

import (
	"database/sql"
	"strconv"
	"strings"

	mssql "github.com/microsoft/go-mssqldb"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// relationKindByObjectType reads the one-letter type `sys.objects` stores.
var relationKindByObjectType = map[string]db.RelationKind{
	"u": db.RelationTable, "v": db.RelationView,
}

// constraintKinds read the constraint type the catalog reports.
var constraintKinds = map[string]db.ConstraintKind{
	"primary key": db.ConstraintPrimaryKey, "foreign key": db.ConstraintForeignKey,
	"unique": db.ConstraintUnique, "check": db.ConstraintCheck,
}

// identifierWidth is the byte width of a uniqueidentifier value.
const identifierWidth = 16

// MapRelationKind reads the type of a relation.
func MapRelationKind(code any) db.RelationKind {
	kind, known := relationKindByObjectType[strings.ToLower(
		strings.TrimSpace(db.ReadCatalogText(code)))]
	if !known {
		return db.RelationTable
	}
	return kind
}

// MapConstraintKind reads the kind of a constraint.
func MapConstraintKind(code any) db.ConstraintKind {
	kind, known := constraintKinds[strings.ToLower(db.ReadAnyText(code))]
	if !known {
		return db.ConstraintCheck
	}
	return kind
}

// readFlag reads a `bit` of the catalog, which the driver hands over as a boolean and a
// computed one as a number.
func readFlag(value any) bool {
	if held, isFlag := value.(bool); isFlag {
		return held
	}
	return db.ReadNonNegativeCount(value) == 1
}

// HoldsOutputClause is true for a statement with a top-level OUTPUT clause, the one form of
// a write that answers with rows.
func HoldsOutputClause(sql string, flavour syntax.SyntaxFlavour) bool {
	return len(syntax.FindTopLevelKeywords(sql, []string{"output"}, flavour)) > 0
}

// ReadForeignKey reads a foreign key of a catalog row.
func ReadForeignKey(row map[string]any) db.ForeignKey {
	return db.ForeignKey{
		Name: db.ReadAnyText(row["name"]), Columns: db.SplitCommaList(row["columns"]),
		TargetSchema:  db.ReadAnyText(row["target_schema"]),
		TargetTable:   db.ReadAnyText(row["target_table"]),
		TargetColumns: db.SplitCommaList(row["target_columns"]),
		DeleteRule:    query.ParseDeleteRule(db.ReadAnyText(row["delete_rule"])),
	}
}

// rowReader reads the values of one row, in the shapes this driver returns them.
type rowReader struct {
	// True for a column whose value stays bytes.
	binary []bool
	// True for a uniqueidentifier column, which the driver hands over as 16 bytes.
	guid []bool
}

// readRow reads one row as plain values.
func (reader rowReader) readRow(rows *sql.Rows) ([]any, error) {
	values, err := db.ScanRow(rows, len(reader.binary), reader.binary...)
	if err != nil {
		return nil, err
	}
	for at, value := range values {
		if reader.guid[at] {
			values[at] = readGUID(value)
		}
	}
	return values, nil
}

// readGUID writes a uniqueidentifier the way the server writes it.
func readGUID(value any) any {
	held, isBytes := value.([]byte)
	if !isBytes || len(held) != identifierWidth {
		return value
	}
	identifier := mssql.UniqueIdentifier{}
	if identifier.Scan(held) != nil {
		return value
	}
	return identifier.String()
}

// buildSqlserverColumns reads the columns of a result with the reader of its values.
func buildSqlserverColumns(rows *sql.Rows) ([]db.ResultColumn, rowReader, error) {
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
	reader := rowReader{binary: make([]bool, 0, len(names)), guid: make([]bool, 0, len(names))}
	for at, name := range names {
		dataType := ""
		if at < len(types) && types[at] != nil {
			dataType = strings.ToLower(types[at].DatabaseTypeName())
		}
		if name == "" {
			// The server names no column of a computed expression.
			name = "column" + strconv.Itoa(at+1)
		}
		columns = append(columns, db.ResultColumn{Name: name, DataType: dataType})
		guid := dataType == "uniqueidentifier"
		reader.guid = append(reader.guid, guid)
		reader.binary = append(reader.binary, guid || db.IsBinaryColumnType(dataType))
	}
	return columns, reader, nil
}

// readSqlserverRows reads a result as rows of plain values, with its columns.
func readSqlserverRows(rows *sql.Rows, cap int) ([][]any, []db.ResultColumn, error) {
	columns, reader, err := buildSqlserverColumns(rows)
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

// readLastResultSet returns the last result set of a batch, which is the one a statement of
// the user answers with.
func readLastResultSet(rows *sql.Rows, cap int) ([][]any, []db.ResultColumn, error) {
	read, columns, err := readSqlserverRows(rows, cap)
	if err != nil {
		return nil, nil, err
	}
	for rows.NextResultSet() {
		next, nextColumns, nextErr := readSqlserverRows(rows, cap)
		if nextErr != nil {
			return nil, nil, nextErr
		}
		read, columns = next, nextColumns
	}
	return read, columns, nil
}
