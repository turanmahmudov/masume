package sqlite

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// readCatalog returns a catalog read as rows keyed by column name. It waits for its turn
// on the file like every other read: the pool opens one connection, so a catalog read that
// took it between the `begin` and the `commit` of a staged set would run inside that
// uncommitted transaction.
func (session *sqliteSession) readCatalog(
	ctx context.Context, statement string, params ...any,
) ([]map[string]any, error) {
	giveBack, waitErr := session.holdFile(ctx)
	if waitErr != nil {
		return nil, waitErr
	}
	defer giveBack()

	rows, err := session.file.QueryContext(ctx, statement, params...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	names, nameErr := rows.Columns()
	if nameErr != nil {
		return nil, nameErr
	}

	read := []map[string]any{}
	for rows.Next() {
		values, scanErr := db.ScanRow(rows, len(names))
		if scanErr != nil {
			return nil, scanErr
		}
		row := map[string]any{}
		for at, name := range names {
			row[name] = values[at]
		}
		read = append(read, row)
	}
	return read, rows.Err()
}

// listSchemas returns the file and everything attached to it, which is what SQLite
// calls a schema.
func (session *sqliteSession) listSchemas(ctx context.Context) []string {
	rows, err := session.readCatalog(
		ctx, "select name from pragma_database_list order by seq")
	if err != nil {
		return []string{mainSchema}
	}
	schemas := make([]string, 0, len(rows))
	for _, row := range rows {
		schemas = append(schemas, db.ReadAnyText(row["name"]))
	}
	if len(schemas) == 0 {
		return []string{mainSchema}
	}
	return schemas
}

func (session *sqliteSession) buildCatalogName(schema string) string {
	return session.Support.Dialect.BuildQualifiedName(
		query.QualifiedName{Schema: schema, Name: "sqlite_schema"})
}

func (session *sqliteSession) ListTables(ctx context.Context) ([]db.TableRef, error) {
	tables := []db.TableRef{}

	for _, schema := range session.listSchemas(ctx) {
		estimates := session.readRowEstimates(ctx, schema)
		rows, err := session.readCatalog(ctx, fmt.Sprintf(`
        select name, type
          from %s
         where type in ('table', 'view') and name not like 'sqlite_%%'
         order by name
      `, session.buildCatalogName(schema)))
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			name := db.ReadAnyText(row["name"])
			kind := db.RelationTable
			if db.ReadAnyText(row["type"]) == "view" {
				kind = db.RelationView
			}
			tables = append(tables, db.TableRef{
				Schema: schema, Name: name, Kind: kind, EstimatedRows: estimates[name],
			})
		}
	}
	return tables, nil
}

// readRowEstimates returns what ANALYZE last counted. Without it there is no estimate.
func (session *sqliteSession) readRowEstimates(
	ctx context.Context, schema string,
) map[string]int64 {
	table := session.Support.Dialect.BuildQualifiedName(
		query.QualifiedName{Schema: schema, Name: "sqlite_stat1"})
	rows, err := session.readCatalog(ctx, "select tbl, stat from "+table)
	if err != nil {
		return map[string]int64{}
	}

	estimates := map[string]int64{}
	for _, row := range rows {
		name := db.ReadAnyText(row["tbl"])
		if _, held := estimates[name]; held {
			continue
		}
		// The first word of the statistic is the row count. The rest is about the index.
		written := strings.Fields(db.ReadAnyText(row["stat"]))
		if len(written) > 0 {
			estimates[name] = db.ReadNonNegativeCount(written[0])
		}
	}
	return estimates
}

// triggerEvents name the writes a trigger can run for.
var triggerEvents = map[string]bool{"insert": true, "update": true, "delete": true}

// readTriggerEvents returns the write a trigger runs for, read out of the statement that
// made it. SQLite keeps that statement and no field of its own for the event, and the event
// always stands before the ON of the relation.
func readTriggerEvents(written string) string {
	for _, token := range syntax.ReadCodeTokens(written, syntax.FlavourStandard) {
		if !syntax.IsWordKind(token.Kind) {
			continue
		}
		if token.Text == "on" {
			return ""
		}
		if triggerEvents[token.Text] {
			return token.Text
		}
	}
	return ""
}

// ListSchemaObjects returns the triggers, which are the only SQLite object that is
// neither a relation nor an index.
func (session *sqliteSession) ListSchemaObjects(ctx context.Context) ([]db.SchemaObject, error) {
	objects := []db.SchemaObject{}

	for _, schema := range session.listSchemas(ctx) {
		rows, err := session.readCatalog(ctx, fmt.Sprintf(`
        select name, tbl_name, sql
          from %s
         where type = 'trigger'
         order by name
      `, session.buildCatalogName(schema)))
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			name := db.ReadAnyText(row["name"])
			objects = append(objects, db.SchemaObject{
				Schema: schema, Name: name, Kind: db.ObjectTrigger,
				Detail:   db.ReadAnyText(row["tbl_name"]),
				Events:   readTriggerEvents(db.ReadAnyText(row["sql"])),
				Identity: schema + "." + name,
			})
		}
	}
	return objects, nil
}

func (session *sqliteSession) DescribeTable(
	ctx context.Context, table db.TableRef,
) (db.TableDetail, error) {
	rows, err := session.readCatalog(ctx, listSqliteColumnsSQL, table.Name, table.Schema)
	if err != nil {
		return db.TableDetail{}, err
	}

	columns := make([]db.ColumnDetail, 0, len(rows))
	for _, row := range rows {
		column := db.ColumnDetail{
			Name:         db.ReadAnyText(row["name"]),
			DataType:     strings.ToLower(db.ReadAnyText(row["data_type"])),
			Nullable:     db.ReadNonNegativeCount(row["not_null"]) == 0,
			IsPrimaryKey: db.ReadNonNegativeCount(row["pk"]) > 0,
			IsGenerated:  generatedMarks[db.ReadNonNegativeCount(row["hidden"])],
		}
		if row["default_value"] != nil {
			column.DefaultValue = db.ReadAnyText(row["default_value"])
			column.HasDefault = true
		}
		columns = append(columns, column)
	}

	keys, keyErr := session.readForeignKeys(ctx, table.Schema, table.Name)
	if keyErr != nil {
		return db.TableDetail{}, keyErr
	}
	return db.TableDetail{Table: table, Columns: columns, ForeignKeys: keys}, nil
}

// foreignKeyRows are the columns of one foreign key. The server reports one row per
// column.
type foreignKeyRows struct {
	targetTable   string
	deleteRule    query.DeleteRule
	columns       []string
	targetColumns []string
	// True if the key names no column of the target, so its primary key is used.
	needsKeyColumns bool
}

func (session *sqliteSession) readForeignKeys(
	ctx context.Context, schema, table string,
) ([]db.ForeignKey, error) {
	rows, err := session.readCatalog(ctx, listSqliteForeignKeysSQL, table, schema)
	if err != nil {
		return nil, err
	}

	grouped := map[int64]*foreignKeyRows{}
	order := []int64{}
	for _, row := range rows {
		id := db.ReadNonNegativeCount(row["id"])
		group, held := grouped[id]
		if !held {
			group = &foreignKeyRows{
				targetTable: db.ReadAnyText(row["target_table"]),
				deleteRule:  query.ParseDeleteRule(db.ReadAnyText(row["delete_rule"])),
			}
			grouped[id] = group
			order = append(order, id)
		}
		group.columns = append(group.columns, db.ReadAnyText(row["column_name"]))
		if row["target_column"] == nil {
			group.needsKeyColumns = true
			continue
		}
		group.targetColumns = append(group.targetColumns, db.ReadAnyText(row["target_column"]))
	}

	keys := make([]db.ForeignKey, 0, len(order))
	for _, id := range order {
		group := grouped[id]
		targetColumns := group.targetColumns
		if group.needsKeyColumns {
			held, keyErr := session.readKeyColumns(ctx, schema, group.targetTable)
			if keyErr != nil {
				return nil, keyErr
			}
			targetColumns = held
		}
		keys = append(keys, db.ForeignKey{
			// SQLite gives a key no name, so its number is used.
			Name: fmt.Sprintf("fk_%d", id), Columns: group.columns,
			// A key never points outside its own database.
			TargetSchema: schema, TargetTable: group.targetTable, TargetColumns: targetColumns,
			DeleteRule: group.deleteRule,
		})
	}
	return keys, nil
}

// readKeyColumns returns the primary key, for a foreign key that names the target but
// not its columns.
func (session *sqliteSession) readKeyColumns(
	ctx context.Context, schema, table string,
) ([]string, error) {
	rows, err := session.readCatalog(ctx, listSqliteColumnsSQL, table, schema)
	if err != nil {
		return nil, err
	}
	keyed := []map[string]any{}
	for _, row := range rows {
		if db.ReadNonNegativeCount(row["pk"]) > 0 {
			keyed = append(keyed, row)
		}
	}
	sort.SliceStable(keyed, func(left, right int) bool {
		return db.ReadNonNegativeCount(keyed[left]["pk"]) < db.ReadNonNegativeCount(keyed[right]["pk"])
	})
	names := make([]string, 0, len(keyed))
	for _, row := range keyed {
		names = append(names, db.ReadAnyText(row["name"]))
	}
	return names, nil
}

func (session *sqliteSession) ListRelationships(ctx context.Context) ([]db.Relationship, error) {
	relationships := []db.Relationship{}

	for _, schema := range session.listSchemas(ctx) {
		rows, err := session.readCatalog(ctx, fmt.Sprintf(`
        select name
          from %s
         where type = 'table' and name not like 'sqlite_%%'
         order by name
      `, session.buildCatalogName(schema)))
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			table := db.ReadAnyText(row["name"])
			keys, keyErr := session.readForeignKeys(ctx, schema, table)
			if keyErr != nil {
				return nil, keyErr
			}
			for _, key := range keys {
				relationships = append(relationships,
					db.Relationship{ForeignKey: key, Schema: schema, Table: table})
			}
		}
	}
	return relationships, nil
}

func (session *sqliteSession) ListIndexes(
	ctx context.Context, table db.TableRef,
) ([]db.IndexDetail, error) {
	rows, err := session.readCatalog(ctx, listSqliteIndexesSQL, table.Name, table.Schema)
	if err != nil {
		return nil, err
	}
	written, statementErr := session.readIndexStatements(ctx, table)
	if statementErr != nil {
		return nil, db.WrapDatabaseOperation("reading the indexes", statementErr)
	}

	indexes := make([]db.IndexDetail, 0, len(rows))
	for _, row := range rows {
		name := db.ReadAnyText(row["name"])
		isPrimary := db.ReadAnyText(row["origin"]) == "pk"
		definition, held := written[name]
		if !held {
			built, buildErr := session.buildIndexDefinition(ctx, table, name, isPrimary)
			if buildErr != nil {
				return nil, buildErr
			}
			definition = built
		}
		indexes = append(indexes, db.IndexDetail{
			Name: name, IsUnique: db.ReadNonNegativeCount(row["is_unique"]) == 1,
			IsPrimary: isPrimary, Definition: definition,
		})
	}
	return indexes, nil
}

// readIndexStatements returns the statement that made each index. SQLite keeps it for
// all but its own.
func (session *sqliteSession) readIndexStatements(
	ctx context.Context, table db.TableRef,
) (map[string]string, error) {
	rows, err := session.readCatalog(ctx, fmt.Sprintf(`
        select name, sql
          from %s
         where type = 'index' and tbl_name = ? and sql is not null
      `, session.buildCatalogName(table.Schema)), table.Name)
	if err != nil {
		return nil, err
	}
	written := map[string]string{}
	for _, row := range rows {
		written[db.ReadAnyText(row["name"])] = db.ReadAnyText(row["sql"])
	}
	return written, nil
}

// buildIndexDefinition writes the key an index belongs to. An index the server made
// itself has no statement, and a CREATE with that name would be refused.
func (session *sqliteSession) buildIndexDefinition(
	ctx context.Context, table db.TableRef, name string, isPrimary bool,
) (string, error) {
	columns, err := session.readIndexColumns(ctx, table.Schema, name)
	if err != nil {
		return "", err
	}
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, session.Support.Dialect.QuoteIdentifier(column))
	}
	written := strings.Join(quoted, ", ")
	if isPrimary {
		return "primary key (" + written + ")", nil
	}
	return "unique (" + written + ")", nil
}

func (session *sqliteSession) readIndexColumns(
	ctx context.Context, schema, index string,
) ([]string, error) {
	rows, err := session.readCatalog(ctx, listSqliteIndexColumnsSQL, index, schema)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, db.ReadAnyText(row["name"]))
	}
	return names, nil
}

func (session *sqliteSession) ListConstraints(
	ctx context.Context, table db.TableRef,
) ([]db.ConstraintDetail, error) {
	quote := session.Support.Dialect.QuoteIdentifier
	constraints := []db.ConstraintDetail{}

	keyColumns, err := session.readKeyColumns(ctx, table.Schema, table.Name)
	if err != nil {
		return nil, err
	}
	if len(keyColumns) > 0 {
		constraints = append(constraints, db.ConstraintDetail{
			Name: "primary key", Kind: db.ConstraintPrimaryKey,
			Definition: "primary key (" + db.JoinQuoted(keyColumns, quote) + ")",
		})
	}

	indexRows, indexErr := session.readCatalog(
		ctx, listSqliteIndexesSQL, table.Name, table.Schema)
	if indexErr != nil {
		return nil, indexErr
	}
	for _, row := range indexRows {
		if db.ReadAnyText(row["origin"]) != "u" {
			continue
		}
		name := db.ReadAnyText(row["name"])
		columns, columnErr := session.readIndexColumns(ctx, table.Schema, name)
		if columnErr != nil {
			return nil, columnErr
		}
		constraints = append(constraints, db.ConstraintDetail{
			Name: name, Kind: db.ConstraintUnique,
			Definition: "unique (" + db.JoinQuoted(columns, quote) + ")",
		})
	}

	keys, keyErr := session.readForeignKeys(ctx, table.Schema, table.Name)
	if keyErr != nil {
		return nil, keyErr
	}
	for _, key := range keys {
		constraints = append(constraints, db.ConstraintDetail{
			Name: key.Name, Kind: db.ConstraintForeignKey,
			Definition: fmt.Sprintf("foreign key (%s) references %s (%s)",
				db.JoinQuoted(key.Columns, quote), quote(key.TargetTable),
				db.JoinQuoted(key.TargetColumns, quote)),
		})
	}

	created, statementErr := session.readRelationStatement(ctx, table)
	if statementErr != nil {
		return nil, statementErr
	}
	for at, clause := range readCheckClauses(created, session.Support.Dialect.Syntax) {
		constraints = append(constraints, db.ConstraintDetail{
			Name: fmt.Sprintf("check_%d", at+1), Kind: db.ConstraintCheck, Definition: clause,
		})
	}
	return constraints, nil
}

// readCheckClauses reads each CHECK from the CREATE TABLE text, because SQLite has none
// in its catalog.
func readCheckClauses(createSQL string, flavour syntax.SyntaxFlavour) []string {
	tokens := syntax.ReadCodeTokens(createSQL, flavour)
	clauses := []string{}

	for _, hit := range syntax.FindKeywordsAnywhere(tokens, []string{"check"}) {
		opening := hit.Index + 1
		if !syntax.IsOperator(tokens, opening, "(") {
			continue
		}
		after := syntax.SkipBracketGroup(tokens, opening)
		closing, present := syntax.TokenAt(tokens, after-1)
		if !present {
			continue
		}
		start, hasStart := syntax.TokenAt(tokens, opening)
		if !hasStart {
			continue
		}
		clauses = append(clauses, "check "+createSQL[start.Start:closing.End])
	}
	return clauses
}

func (session *sqliteSession) readRelationStatement(
	ctx context.Context, table db.TableRef,
) (string, error) {
	rows, err := session.readCatalog(ctx, fmt.Sprintf(
		"select sql from %s where name = ? and sql is not null",
		session.buildCatalogName(table.Schema)), table.Name)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return db.ReadAnyText(rows[0]["sql"]), nil
}

// BuildTableDDL returns the statement that made the relation, which SQLite keeps, so
// nothing is rebuilt.
func (session *sqliteSession) BuildTableDDL(
	ctx context.Context, table db.TableRef,
) ([]string, error) {
	created, err := session.readRelationStatement(ctx, table)
	if err != nil {
		return nil, db.WrapDatabaseOperation("reading the create statement", err)
	}
	if created == "" {
		return db.BuildMissingDefinition(table.Name), nil
	}

	lines := strings.Split(created+";", "\n")
	written, statementErr := session.readIndexStatements(ctx, table)
	if statementErr != nil {
		return nil, statementErr
	}
	statements := make([]string, 0, len(written))
	for _, statement := range written {
		statements = append(statements, statement)
	}
	slices.Sort(statements)
	if len(statements) > 0 {
		lines = append(lines, "")
		for _, statement := range statements {
			lines = append(lines, statement+";")
		}
	}
	return lines, nil
}

func (session *sqliteSession) BuildObjectDDL(
	ctx context.Context, object db.SchemaObject,
) ([]string, error) {
	rows, err := session.readCatalog(ctx, fmt.Sprintf(
		"select sql from %s where name = ? and sql is not null",
		session.buildCatalogName(object.Schema)), object.Name)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return db.BuildMissingDefinition(string(object.Kind) + " " + object.Name), nil
	}
	written := db.ReadAnyText(rows[0]["sql"])
	if written == "" {
		return db.BuildMissingDefinition(string(object.Kind) + " " + object.Name), nil
	}
	return strings.Split(written+";", "\n"), nil
}
