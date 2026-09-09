package sqlserver

import (
	"context"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// ListSchemas returns the schemas of the connected database, including one that holds
// nothing. The catalog views of the server are of one database, so the list is of that
// database.
func (session *sqlserverSession) ListSchemas(ctx context.Context) ([]string, error) {
	rows, _, err := session.readNamedRows(ctx, listSchemasSQL)
	if err != nil {
		return nil, err
	}
	schemas := make([]string, 0, len(rows))
	for _, row := range rows {
		if name := db.ReadAnyText(row["name"]); name != "" {
			schemas = append(schemas, name)
		}
	}
	return schemas, nil
}

func (session *sqlserverSession) ListTables(ctx context.Context) ([]db.TableRef, error) {
	rows, _, err := session.readNamedRows(ctx, listTablesSQL)
	if err != nil {
		return nil, err
	}
	tables := make([]db.TableRef, 0, len(rows))
	for _, row := range rows {
		tables = append(tables, db.TableRef{
			Schema: db.ReadAnyText(row["schema"]), Name: db.ReadAnyText(row["name"]),
			Kind:          MapRelationKind(row["kind"]),
			EstimatedRows: db.ReadNonNegativeCount(row["estimated_rows"]),
		})
	}
	return tables, nil
}

func (session *sqlserverSession) ListRoles(ctx context.Context) ([]db.DbRole, error) {
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

func (session *sqlserverSession) ListSchemaObjects(
	ctx context.Context,
) ([]db.SchemaObject, error) {
	routineRows, _, err := session.readNamedRows(ctx, listRoutinesSQL)
	if err != nil {
		return nil, db.WrapDatabaseOperation("reading the routines", err)
	}
	sequenceRows, _, sequenceErr := session.readNamedRows(ctx, listSequencesSQL)
	if sequenceErr != nil {
		return nil, db.WrapDatabaseOperation("reading the sequences", sequenceErr)
	}
	typeRows, _, typeErr := session.readNamedRows(ctx, listTypesSQL)
	if typeErr != nil {
		return nil, db.WrapDatabaseOperation("reading the types", typeErr)
	}
	triggerRows, _, triggerErr := session.readNamedRows(ctx, listTriggersSQL)
	if triggerErr != nil {
		return nil, db.WrapDatabaseOperation("reading the triggers", triggerErr)
	}

	objects := make([]db.SchemaObject, 0,
		len(routineRows)+len(sequenceRows)+len(typeRows)+len(triggerRows))
	for _, row := range routineRows {
		schema := db.ReadAnyText(row["schema"])
		name := db.ReadAnyText(row["name"])
		kind := RoutineFunction
		detail := db.ReadAnyText(row["detail"])
		if db.ReadAnyText(row["routine_kind"]) == string(RoutineProcedure) {
			kind = RoutineProcedure
			detail = string(RoutineProcedure)
		}
		objects = append(objects, db.SchemaObject{
			Schema: schema, Name: name, Kind: db.ObjectFunction, Detail: detail,
			Identity: BuildRoutineIdentity(kind, schema, name),
		})
	}
	objects = append(objects,
		readPlainObjects(sequenceRows, db.ObjectSequence)...)
	objects = append(objects, readPlainObjects(typeRows, db.ObjectType)...)
	for _, row := range triggerRows {
		schema := db.ReadAnyText(row["schema"])
		name := db.ReadAnyText(row["name"])
		objects = append(objects, db.SchemaObject{
			Schema: schema, Name: name, Kind: db.ObjectTrigger,
			Detail: db.ReadAnyText(row["detail"]), Events: db.ReadAnyText(row["events"]),
			Identity: schema + "." + name,
		})
	}
	return objects, nil
}

// readPlainObjects reads the objects of one kind that carry a schema, a name and a detail.
func readPlainObjects(
	rows []map[string]any, kind db.SchemaObjectKind,
) []db.SchemaObject {
	objects := make([]db.SchemaObject, 0, len(rows))
	for _, row := range rows {
		schema := db.ReadAnyText(row["schema"])
		name := db.ReadAnyText(row["name"])
		objects = append(objects, db.SchemaObject{
			Schema: schema, Name: name, Kind: kind,
			Detail: db.ReadAnyText(row["detail"]), Identity: schema + "." + name,
		})
	}
	return objects
}

func (session *sqlserverSession) DescribeTable(
	ctx context.Context, table db.TableRef,
) (db.TableDetail, error) {
	target := session.Support.Dialect.BuildQualifiedName(table.Qualified())
	columnRows, _, err := session.readNamedRows(ctx, describeColumnsSQL, target)
	if err != nil {
		return db.TableDetail{}, err
	}
	keyRows, _, keyErr := session.readNamedRows(ctx, describeForeignKeysSQL, target)
	if keyErr != nil {
		return db.TableDetail{}, keyErr
	}

	columns := make([]db.ColumnDetail, 0, len(columnRows))
	for _, row := range columnRows {
		column := db.ColumnDetail{
			Name: db.ReadAnyText(row["name"]), DataType: db.ReadAnyText(row["data_type"]),
			Nullable:     readFlag(row["nullable"]),
			IsPrimaryKey: readFlag(row["is_primary_key"]),
			// A computed column and an identity column both refuse a value of the client.
			IsGenerated: readFlag(row["is_generated"]),
		}
		if row["default_value"] != nil {
			column.DefaultValue = db.ReadAnyText(row["default_value"])
			column.HasDefault = true
		}
		columns = append(columns, column)
	}

	keys := make([]db.ForeignKey, 0, len(keyRows))
	for _, row := range keyRows {
		keys = append(keys, ReadForeignKey(row))
	}
	return db.TableDetail{Table: table, Columns: columns, ForeignKeys: keys}, nil
}

func (session *sqlserverSession) ListRelationships(
	ctx context.Context,
) ([]db.Relationship, error) {
	rows, _, err := session.readNamedRows(ctx, listRelationshipsSQL)
	if err != nil {
		return nil, err
	}
	relationships := make([]db.Relationship, 0, len(rows))
	for _, row := range rows {
		relationships = append(relationships, db.Relationship{
			ForeignKey: ReadForeignKey(row),
			Schema:     db.ReadAnyText(row["schema"]), Table: db.ReadAnyText(row["table"]),
		})
	}
	return relationships, nil
}

func (session *sqlserverSession) ListIndexes(
	ctx context.Context, table db.TableRef,
) ([]db.IndexDetail, error) {
	target := session.Support.Dialect.BuildQualifiedName(table.Qualified())
	rows, _, err := session.readNamedRows(ctx, listIndexesSQL, target)
	if err != nil {
		return nil, err
	}
	indexes := make([]db.IndexDetail, 0, len(rows))
	for _, row := range rows {
		name := db.ReadAnyText(row["name"])
		isPrimary := readFlag(row["is_primary"])
		isUnique := readFlag(row["is_unique"])
		indexes = append(indexes, db.IndexDetail{
			Name: name, IsUnique: isUnique, IsPrimary: isPrimary,
			Definition: db.RenderIndexDefinition(
				table, name, isPrimary, isUnique, row["columns"], session.Support.Dialect),
		})
	}
	return indexes, nil
}

func (session *sqlserverSession) ListConstraints(
	ctx context.Context, table db.TableRef,
) ([]db.ConstraintDetail, error) {
	target := session.Support.Dialect.BuildQualifiedName(table.Qualified())
	rows, _, err := session.readNamedRows(ctx, listConstraintsSQL, target)
	if err != nil {
		return nil, err
	}
	constraints := make([]db.ConstraintDetail, 0, len(rows))
	for _, row := range rows {
		kind := MapConstraintKind(row["type"])
		constraints = append(constraints, db.ConstraintDetail{
			Name: db.ReadAnyText(row["name"]), Kind: kind,
			Definition: db.RenderConstraintDefinition(kind, row),
		})
	}
	return constraints, nil
}

// BuildTableDDL returns the statement of a view as the server stored it, and builds the
// statement of a table from the catalog.
func (session *sqlserverSession) BuildTableDDL(
	ctx context.Context, table db.TableRef,
) ([]string, error) {
	if table.Kind == db.RelationView || table.Kind == db.RelationMaterializedView {
		return session.readObjectDefinition(ctx, table.Name,
			session.Support.Dialect.BuildQualifiedName(table.Qualified()))
	}
	detail, err := session.DescribeTable(ctx, table)
	if err != nil {
		return nil, db.WrapDatabaseOperation("reading the columns", err)
	}
	indexes, indexErr := session.ListIndexes(ctx, table)
	if indexErr != nil {
		return nil, db.WrapDatabaseOperation("reading the indexes", indexErr)
	}
	constraints, constraintErr := session.ListConstraints(ctx, table)
	if constraintErr != nil {
		return nil, db.WrapDatabaseOperation("reading the constraints", constraintErr)
	}
	return db.RenderTableDDL(detail, indexes, constraints, session.Support.Dialect), nil
}

// BuildObjectDDL returns the statement that made the object, which the server keeps for a
// routine, a trigger and a view.
func (session *sqlserverSession) BuildObjectDDL(
	ctx context.Context, object db.SchemaObject,
) ([]string, error) {
	target := query.QualifiedName{Schema: object.Schema, Name: object.Name}
	return session.readObjectDefinition(ctx, string(object.Kind)+" "+object.Name,
		session.Support.Dialect.BuildQualifiedName(target))
}

// readObjectDefinition reads the stored statement of one object back.
func (session *sqlserverSession) readObjectDefinition(
	ctx context.Context, named, target string,
) ([]string, error) {
	rows, _, err := session.readNamedRows(ctx, readObjectDefinitionSQL, target)
	if err != nil {
		return nil, err
	}
	definition := ""
	if len(rows) > 0 {
		definition = strings.TrimSpace(db.ReadAnyText(rows[0]["definition"]))
	}
	if definition == "" {
		return db.BuildMissingDefinition(named), nil
	}
	return strings.Split(definition, "\n"), nil
}
