package clickhouse

import (
	"context"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

func (session *clickhouseSession) ListTables(ctx context.Context) ([]db.TableRef, error) {
	rows, _, err := session.readNamedRows(ctx, listTablesSQL)
	if err != nil {
		return nil, err
	}
	tables := make([]db.TableRef, 0, len(rows))
	for _, row := range rows {
		tables = append(tables, db.TableRef{
			Schema: db.ReadAnyText(row["schema"]), Name: db.ReadAnyText(row["name"]),
			Kind:          MapRelationKind(row["engine"]),
			EstimatedRows: db.ReadNonNegativeCount(row["estimated_rows"]),
		})
	}
	return tables, nil
}

func (session *clickhouseSession) ListRoles(ctx context.Context) ([]db.DbRole, error) {
	rows, _, err := session.readNamedRows(ctx, listRolesSQL)
	if err != nil {
		return nil, err
	}
	roles := make([]db.DbRole, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, db.DbRole{
			Name: db.ReadAnyText(row["name"]), Detail: db.ReadAnyText(row["detail"]),
		})
	}
	return roles, nil
}

// ListSchemaObjects returns the functions the user wrote. The server holds no sequence, no
// trigger and no type of the user.
func (session *clickhouseSession) ListSchemaObjects(
	ctx context.Context,
) ([]db.SchemaObject, error) {
	rows, _, err := session.readNamedRows(ctx, listFunctionsSQL)
	if err != nil {
		return nil, db.WrapDatabaseOperation("reading the functions", err)
	}
	objects := make([]db.SchemaObject, 0, len(rows))
	for _, row := range rows {
		name := db.ReadAnyText(row["name"])
		objects = append(objects, db.SchemaObject{
			// A function of the user belongs to the server, not to one database, and the
			// tree draws it under the database of the connection.
			Schema: session.Descriptor.DefaultSchema, Name: name,
			Kind: db.ObjectFunction, Detail: "function", Identity: name,
		})
	}
	return objects, nil
}

// ListRelationships returns none: the server holds no foreign key.
func (session *clickhouseSession) ListRelationships(
	context.Context,
) ([]db.Relationship, error) {
	return nil, nil
}

func (session *clickhouseSession) DescribeTable(
	ctx context.Context, table db.TableRef,
) (db.TableDetail, error) {
	rows, _, err := session.readNamedRows(
		ctx, describeColumnsSQL, table.Schema, table.Name)
	if err != nil {
		return db.TableDetail{}, err
	}

	columns := make([]db.ColumnDetail, 0, len(rows))
	for _, row := range rows {
		dataType := db.ReadAnyText(row["data_type"])
		kind := strings.ToUpper(db.ReadAnyText(row["default_kind"]))
		column := db.ColumnDetail{
			Name: db.ReadAnyText(row["name"]), DataType: dataType,
			Nullable:     ReadsNull(dataType),
			IsPrimaryKey: db.ReadNonNegativeCount(row["is_primary_key"]) == 1,
			// The server computes a MATERIALIZED and an ALIAS column itself.
			IsGenerated: kind == "MATERIALIZED" || kind == "ALIAS",
		}
		if written := db.ReadAnyText(row["default_value"]); written != "" && kind == "DEFAULT" {
			column.DefaultValue = written
			column.HasDefault = true
		}
		columns = append(columns, column)
	}
	// The server holds no foreign key, so a relation names none.
	return db.TableDetail{Table: table, Columns: columns}, nil
}

// ListIndexes returns the data-skipping indexes of a table, and the sorting key it keeps
// its rows in.
func (session *clickhouseSession) ListIndexes(
	ctx context.Context, table db.TableRef,
) ([]db.IndexDetail, error) {
	indexes := []db.IndexDetail{}
	keyRows, _, keyErr := session.readNamedRows(
		ctx, readSortingKeySQL, table.Schema, table.Name)
	if keyErr != nil {
		return nil, keyErr
	}
	if len(keyRows) > 0 {
		if written := db.ReadAnyText(keyRows[0]["sorting_key"]); written != "" {
			indexes = append(indexes, db.IndexDetail{
				Name: "sorting key", IsPrimary: true,
				Definition: "order by " + written,
			})
		}
	}

	rows, _, err := session.readNamedRows(ctx, listIndexesSQL, table.Schema, table.Name)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		name := db.ReadAnyText(row["name"])
		indexes = append(indexes, db.IndexDetail{
			Name: name,
			Definition: "index " + session.Support.Dialect.QuoteIdentifier(name) + " " +
				db.ReadAnyText(row["expr"]) + " type " + db.ReadAnyText(row["type_full"]) +
				" granularity " + db.ReadAnyText(row["granularity"]),
		})
	}
	return indexes, nil
}

// ListConstraints returns none: the server keeps the constraints of a table in the
// statement that made it, and in no catalog of its own.
func (session *clickhouseSession) ListConstraints(
	context.Context, db.TableRef,
) ([]db.ConstraintDetail, error) {
	return nil, nil
}

// BuildTableDDL returns the statement that made the relation, which the server writes
// itself.
func (session *clickhouseSession) BuildTableDDL(
	ctx context.Context, table db.TableRef,
) ([]string, error) {
	target := session.Support.Dialect.BuildQualifiedName(table.Qualified())
	rows, _, err := session.readNamedRows(ctx, showCreateSQL+target)
	if err != nil {
		return nil, err
	}
	return readDefinition(rows, table.Name, "statement"), nil
}

// BuildObjectDDL returns the statement that made the function.
func (session *clickhouseSession) BuildObjectDDL(
	ctx context.Context, object db.SchemaObject,
) ([]string, error) {
	rows, _, err := session.readNamedRows(ctx, listFunctionsSQL)
	if err != nil {
		return nil, err
	}
	held := []map[string]any{}
	for _, row := range rows {
		if db.ReadAnyText(row["name"]) == object.Name {
			held = append(held, row)
		}
	}
	return readDefinition(held, string(object.Kind)+" "+object.Name, "create_query"), nil
}

// readDefinition returns the stored statement of one object, split into its lines.
func readDefinition(rows []map[string]any, named, column string) []string {
	definition := ""
	if len(rows) > 0 {
		definition = strings.TrimSpace(db.ReadAnyText(rows[0][column]))
	}
	if definition == "" {
		return db.BuildMissingDefinition(named)
	}
	return strings.Split(definition, "\n")
}
