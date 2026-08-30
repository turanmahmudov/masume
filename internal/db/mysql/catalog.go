package mysql

import (
	"context"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

func (session *mysqlSession) ListTables(ctx context.Context) ([]db.TableRef, error) {
	rows, _, err := session.readNamedRows(ctx, listMysqlTablesSQL)
	if err != nil {
		return nil, err
	}
	tables := make([]db.TableRef, 0, len(rows))
	for _, row := range rows {
		kind, known := relationKindByTableType[db.ReadAnyText(row["kind"])]
		if !known {
			kind = db.RelationTable
		}
		tables = append(tables, db.TableRef{
			Schema: db.ReadAnyText(row["schema"]), Name: db.ReadAnyText(row["name"]),
			Kind: kind, EstimatedRows: db.ReadNonNegativeCount(row["estimated_rows"]),
		})
	}
	return tables, nil
}

func (session *mysqlSession) ListRoles(ctx context.Context) ([]db.DbRole, error) {
	rows, _, err := session.readNamedRows(ctx, listMysqlRolesSQL)
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

func (session *mysqlSession) ListSchemaObjects(ctx context.Context) ([]db.SchemaObject, error) {
	routineRows, _, err := session.readNamedRows(ctx, listMysqlRoutinesSQL)
	if err != nil {
		return nil, db.WrapDatabaseOperation("reading the routines", err)
	}
	triggerRows, _, triggerErr := session.readNamedRows(ctx, listMysqlTriggersSQL)
	if triggerErr != nil {
		return nil, db.WrapDatabaseOperation("reading the triggers", triggerErr)
	}

	objects := make([]db.SchemaObject, 0, len(routineRows)+len(triggerRows))
	for _, row := range routineRows {
		schema := db.ReadAnyText(row["schema"])
		name := db.ReadAnyText(row["name"])
		kind := RoutineFunction
		detail := db.ReadAnyText(row["detail"])
		if db.ReadAnyText(row["routine_type"]) == "procedure" {
			kind = RoutineProcedure
			detail = "procedure"
		}
		objects = append(objects, db.SchemaObject{
			Schema: schema, Name: name, Kind: db.ObjectFunction, Detail: detail,
			Identity: BuildRoutineIdentity(kind, schema, name),
		})
	}
	for _, row := range triggerRows {
		schema := db.ReadAnyText(row["schema"])
		name := db.ReadAnyText(row["name"])
		objects = append(objects, db.SchemaObject{
			Schema: schema, Name: name, Kind: db.ObjectTrigger,
			Detail: db.ReadAnyText(row["detail"]), Identity: schema + "." + name,
		})
	}
	return objects, nil
}

// BuildObjectDDL asks MySQL for the definition of the object, which it prints itself.
func (session *mysqlSession) BuildObjectDDL(
	ctx context.Context, object db.SchemaObject,
) ([]string, error) {
	target := session.Support.Dialect.BuildQualifiedName(
		query.QualifiedName{Schema: object.Schema, Name: object.Name})
	statement := "show create trigger " + target
	if object.Kind != db.ObjectTrigger {
		statement = "show create " +
			string(ReadRoutineKind(object.Identity)) + " " + target
	}

	rows, _, err := session.readNamedRows(ctx, statement)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return db.BuildMissingDefinition(string(object.Kind) + " " + object.Name), nil
	}
	definition := FindDefinition(rows[0])
	if definition == "" {
		return db.BuildMissingDefinition(string(object.Kind) + " " + object.Name), nil
	}
	return strings.Split(definition, "\n"), nil
}

func (session *mysqlSession) DescribeTable(
	ctx context.Context, table db.TableRef,
) (db.TableDetail, error) {
	columnRows, _, err := session.readNamedRows(
		ctx, describeMysqlColumnsSQL, table.Schema, table.Name)
	if err != nil {
		return db.TableDetail{}, err
	}
	keyRows, _, keyErr := session.readNamedRows(
		ctx, describeMysqlForeignKeysSQL, table.Schema, table.Name)
	if keyErr != nil {
		return db.TableDetail{}, keyErr
	}

	columns := make([]db.ColumnDetail, 0, len(columnRows))
	for _, row := range columnRows {
		dataType := db.ReadAnyText(row["data_type"])
		column := db.ColumnDetail{
			Name: db.ReadAnyText(row["name"]), DataType: dataType,
			Nullable:     strings.EqualFold(db.ReadAnyText(row["nullable"]), "YES"),
			IsPrimaryKey: strings.EqualFold(db.ReadAnyText(row["column_key"]), "PRI"),
			// MySQL marks a computed column in EXTRA as "VIRTUAL GENERATED" or "STORED
			// GENERATED". An auto-increment column is not one, because it takes a value.
			IsGenerated: strings.Contains(strings.ToUpper(db.ReadAnyText(row["extra"])), "GENERATED"),
			Choices:     ReadEnumChoices(dataType),
		}
		if row["default_value"] != nil {
			column.DefaultValue = db.ReadAnyText(row["default_value"])
			column.HasDefault = true
		}
		columns = append(columns, column)
	}

	keys := make([]db.ForeignKey, 0, len(keyRows))
	for _, row := range keyRows {
		keys = append(keys, readMysqlForeignKey(row))
	}
	return db.TableDetail{Table: table, Columns: columns, ForeignKeys: keys}, nil
}

func (session *mysqlSession) ListRelationships(ctx context.Context) ([]db.Relationship, error) {
	rows, _, err := session.readNamedRows(ctx, listMysqlRelationshipsSQL)
	if err != nil {
		return nil, err
	}
	relationships := make([]db.Relationship, 0, len(rows))
	for _, row := range rows {
		relationships = append(relationships, db.Relationship{
			ForeignKey: readMysqlForeignKey(row),
			Schema:     db.ReadAnyText(row["schema"]), Table: db.ReadAnyText(row["table"]),
		})
	}
	return relationships, nil
}

func (session *mysqlSession) ListIndexes(
	ctx context.Context, table db.TableRef,
) ([]db.IndexDetail, error) {
	rows, _, err := session.readNamedRows(ctx, listMysqlIndexesSQL, table.Schema, table.Name)
	if err != nil {
		return nil, err
	}
	indexes := make([]db.IndexDetail, 0, len(rows))
	for _, row := range rows {
		name := db.ReadAnyText(row["name"])
		isPrimary := name == "PRIMARY"
		isUnique := db.ReadNonNegativeCount(row["non_unique"]) == 0
		indexes = append(indexes, db.IndexDetail{
			Name: name, IsUnique: isUnique, IsPrimary: isPrimary,
			Definition: RenderIndexDefinition(
				table, name, isPrimary, isUnique, row["columns"], session.Support.Dialect),
		})
	}
	return indexes, nil
}

func (session *mysqlSession) ListConstraints(
	ctx context.Context, table db.TableRef,
) ([]db.ConstraintDetail, error) {
	rows, _, err := session.readNamedRows(
		ctx, listMysqlConstraintsSQL, table.Schema, table.Name)
	if err != nil {
		return nil, err
	}
	constraints := make([]db.ConstraintDetail, 0, len(rows))
	for _, row := range rows {
		kind, known := mysqlConstraintKinds[strings.ToUpper(db.ReadAnyText(row["type"]))]
		if !known {
			kind = db.ConstraintCheck
		}
		constraints = append(constraints, db.ConstraintDetail{
			Name: db.ReadAnyText(row["name"]), Kind: kind,
			Definition: RenderConstraintDefinition(kind, row),
		})
	}
	return constraints, nil
}

// BuildTableDDL asks MySQL for the CREATE of a relation of any kind, which it writes
// itself.
func (session *mysqlSession) BuildTableDDL(
	ctx context.Context, table db.TableRef,
) ([]string, error) {
	word := "view"
	if table.Kind == db.RelationTable {
		word = "table"
	}
	target := session.Support.Dialect.BuildQualifiedName(table.Qualified())
	rows, _, err := session.readNamedRows(ctx, "show create "+word+" "+target)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return db.BuildMissingDefinition(table.Name), nil
	}
	definition := FindDefinition(rows[0])
	if definition == "" {
		return db.BuildMissingDefinition(table.Name), nil
	}
	return strings.Split(definition+";", "\n"), nil
}
