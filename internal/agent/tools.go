package agent

import (
	"context"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/result"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// Everything a model may ask a connection for. Only the last one writes.

// The limits of a listing. A pattern narrows a long list rather than a higher number.
const (
	maxRelationshipsListed = 300
	maxTablesListed        = 200
)

// buildNamePattern reads a pattern a caller wrote: the text anywhere in the name, with `*`
// for any run of characters. The case is not read.
func buildNamePattern(written string) (*regexp.Regexp, error) {
	escaped := regexp.QuoteMeta(written)
	// The star is the one part of the pattern that is not literal.
	return regexp.Compile("(?i)" + strings.ReplaceAll(escaped, `\*`, ".*"))
}

// resolveDatabase returns the schema a call names, or the one the connection opened on.
func resolveDatabase(deps ToolDeps, named string) string {
	if named != "" {
		return named
	}
	return deps.Session.Describe().DefaultSchema
}

// listSchemaNames returns every schema the relations sit in, in name order.
func listSchemaNames(tables []db.TableRef) []string {
	seen := map[string]bool{}
	names := []string{}
	for _, table := range tables {
		if !seen[table.Schema] {
			seen[table.Schema], names = true, append(names, table.Schema)
		}
	}
	slices.Sort(names)
	return names
}

// resolveTableInput returns the relation a name from the model stands for. The second answer
// is the problem to report where there is no such relation.
func resolveTableInput(deps ToolDeps, table, database string) (db.TableRef, map[string]any) {
	source := statement.SelectSource{Name: table}
	if database != "" {
		source.Schema, source.HasSchema = database, true
	}
	if before, after, ok := strings.Cut(table, "."); ok {
		source.Name = after
		if database == "" {
			source.Schema, source.HasSchema = before, true
		}
	}

	found, known := db.FindTableByName(
		deps.Tables(), source, deps.Session.Describe().DefaultSchema)
	if !known {
		return db.TableRef{}, map[string]any{
			"error": `no table named "` + table + `" in "` +
				resolveDatabase(deps, source.Schema) + `"`,
			"hint": "call list_tables first to see what is actually there",
		}
	}
	return found, nil
}

// tableInputFields are the arguments of a tool that takes one relation name.
var tableInputFields = []field{
	{
		name: "table", kind: kindString, required: true,
		description: "The table name, unqualified, or written as database.table.",
	},
	{
		name: "database", kind: kindString,
		description: "The database the table is in, if not the connected one " +
			"and not already in `table`.",
	},
}

// defineTableReadTool builds a tool that returns one thing about one relation. Only the read
// differs between them.
func defineTableReadTool(
	name, description string,
	readDetail func(
		ctx context.Context, deps ToolDeps, table db.TableRef,
	) (map[string]any, error),
) ToolDefinition {
	return ToolDefinition{
		Name: name, Description: description, InputSchema: buildSchema(tableInputFields),
		Call: func(ctx context.Context, deps ToolDeps, input map[string]any) any {
			read, problem := readInput(tableInputFields, input)
			if problem != "" {
				return refuseInput(problem)
			}
			named, _ := readText(read, "table")
			database, _ := readText(read, "database")
			found, missing := resolveTableInput(deps, named, database)
			if missing != nil {
				return missing
			}
			answered, err := readDetail(ctx, deps, found)
			if err != nil {
				return map[string]any{"error": db.DescribeError(err)}
			}
			answer := map[string]any{"table": found.Schema + "." + found.Name}
			maps.Copy(answer, answered)
			return answer
		},
	}
}

// refuseInput returns what is wrong with a call, as a text the model reads.
func refuseInput(problem string) map[string]any {
	return map[string]any{
		"error": problem, "hint": "read the schema of this tool and call it again",
	}
}

var listTablesFields = []field{
	{
		name: "database", kind: kindString,
		description: "The database to list. Defaults to the connected database.",
	},
	{
		name: "pattern", kind: kindString,
		description: "Only the tables whose name matches: the text anywhere in the name, " +
			"with * for any run of characters. Case is not read. \"order\" finds " +
			"order_item, and \"*_log\" finds audit_log.",
	},
	{
		name: "limit", kind: kindInteger, positive: true,
		description: "The most to answer with. 200 by default.",
	},
}

var listTables = ToolDefinition{
	Name: "list_tables",
	Description: "List the tables in a database this connection can see. Defaults to the " +
		"connected database. Call this to check a table exists, or to look inside a " +
		"database named only in the schema summary. On a database with many tables, " +
		"pass a pattern rather than reading the whole list.",
	InputSchema: buildSchema(listTablesFields),
	Call: func(_ context.Context, deps ToolDeps, input map[string]any) any {
		read, problem := readInput(listTablesFields, input)
		if problem != "" {
			return refuseInput(problem)
		}
		database, _ := readText(read, "database")
		pattern, hasPattern := readText(read, "pattern")
		limit, hasLimit := readCount(read, "limit")

		schema := resolveDatabase(deps, database)
		inDatabase := []db.TableRef{}
		for _, table := range deps.Tables() {
			if table.Schema == schema {
				inDatabase = append(inDatabase, table)
			}
		}
		if len(inDatabase) == 0 {
			return map[string]any{
				"error":          `no tables found in "` + schema + `"`,
				"knownDatabases": listSchemaNames(deps.Tables()),
			}
		}

		matching := inDatabase
		if hasPattern {
			matcher, err := buildNamePattern(pattern)
			if err != nil {
				return refuseInput(`the pattern "` + pattern + `" cannot be read`)
			}
			matching = []db.TableRef{}
			for _, table := range inDatabase {
				if matcher.MatchString(table.Name) {
					matching = append(matching, table)
				}
			}
			if len(matching) == 0 {
				return map[string]any{
					"error": `no table in "` + schema + `" matches "` + pattern + `"`,
					"hint":  "list without a pattern to see what is there",
				}
			}
		}

		most := maxTablesListed
		if hasLimit {
			most = limit
		}
		listed := matching
		if len(listed) > most {
			listed = listed[:most]
		}

		described := make([]map[string]any, 0, len(listed))
		for _, table := range listed {
			held := map[string]any{"name": table.Name, "kind": string(table.Kind)}
			// Only where the server has an estimate. Nothing is sent instead of a zero.
			if table.EstimatedRows > 0 {
				held["estimatedRows"] = table.EstimatedRows
			}
			described = append(described, held)
		}
		answer := map[string]any{"database": schema, "tables": described}
		if len(matching) > len(listed) {
			answer["truncatedBy"] = len(matching) - len(listed)
			answer["hint"] = "pass a pattern to narrow this, rather than raising the limit"
		}
		return answer
	},
}

func describeColumnForModel(column db.ColumnDetail) map[string]any {
	// A column with no default is written as null, not as an empty text, because an empty
	// text is a default a column can have.
	var defaultValue any
	if column.HasDefault {
		defaultValue = column.DefaultValue
	}
	described := map[string]any{
		"name":       column.Name,
		"type":       column.DataType,
		"nullable":   column.Nullable,
		"primaryKey": column.IsPrimaryKey,
		"default":    defaultValue,
	}
	// The values of an enum column. Without them the model has to read them from the
	// server to write `where status = 'shipped'`.
	if len(column.Choices) > 0 {
		described["choices"] = column.Choices
	}
	return described
}

func describeForeignKeyForModel(key query.ForeignKey) map[string]any {
	return map[string]any{
		"columns":       key.Columns,
		"targetTable":   key.TargetSchema + "." + key.TargetTable,
		"targetColumns": key.TargetColumns,
	}
}

// The server is asked every time, and its answer also goes to the tree. A cached column list
// read before an ALTER is worse than one more request.
var describeTable = defineTableReadTool(
	"describe_table",
	"Get the columns, types, and foreign keys of one table, in the connected database or "+
		"one named by list_tables. Call this before writing a query whenever a table's "+
		"columns are not already known; never guess a column name.",
	func(ctx context.Context, deps ToolDeps, table db.TableRef) (map[string]any, error) {
		detail, err := deps.Session.DescribeTable(ctx, table)
		if err != nil {
			return nil, err
		}
		if deps.MarkTableDescribed != nil {
			deps.MarkTableDescribed(table, detail)
		}
		columns := make([]map[string]any, 0, len(detail.Columns))
		for _, column := range detail.Columns {
			columns = append(columns, describeColumnForModel(column))
		}
		keys := make([]map[string]any, 0, len(detail.ForeignKeys))
		for _, key := range detail.ForeignKeys {
			keys = append(keys, describeForeignKeyForModel(key))
		}
		return map[string]any{"columns": columns, "foreignKeys": keys}, nil
	},
)

var listIndexes = defineTableReadTool(
	"list_indexes",
	"List the indexes of one table: their names, whether they are unique or the primary "+
		"key, and their definition. Call this for a question about how a table is indexed, "+
		"or before suggesting one that may already exist.",
	func(ctx context.Context, deps ToolDeps, table db.TableRef) (map[string]any, error) {
		found, err := deps.Session.ListIndexes(ctx, table)
		if err != nil {
			return nil, err
		}
		described := make([]map[string]any, 0, len(found))
		for _, index := range found {
			described = append(described, map[string]any{
				"name": index.Name, "unique": index.IsUnique,
				"primary": index.IsPrimary, "definition": index.Definition,
			})
		}
		return map[string]any{"indexes": described}, nil
	},
)

var listConstraints = defineTableReadTool(
	"list_constraints",
	"List the constraints of one table: primary key, foreign keys, unique, check, and "+
		"exclusion, each with its definition. Call this to explain why a write would be "+
		"rejected, or what a table enforces beyond its column types.",
	func(ctx context.Context, deps ToolDeps, table db.TableRef) (map[string]any, error) {
		found, err := deps.Session.ListConstraints(ctx, table)
		if err != nil {
			return nil, err
		}
		described := make([]map[string]any, 0, len(found))
		for _, constraint := range found {
			described = append(described, map[string]any{
				"name": constraint.Name, "kind": string(constraint.Kind),
				"definition": constraint.Definition,
			})
		}
		return map[string]any{"constraints": described}, nil
	},
)

var getTableDDL = defineTableReadTool(
	"get_table_ddl",
	"Get the CREATE TABLE statement for one table, exactly as the server would write it: "+
		"every column, key, and constraint together. Useful for a full picture in one read, "+
		"instead of describe_table plus list_constraints.",
	func(ctx context.Context, deps ToolDeps, table db.TableRef) (map[string]any, error) {
		lines, err := deps.Session.BuildTableDDL(ctx, table)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ddl": strings.Join(lines, "\n")}, nil
	},
)

func describeRelationshipForModel(relationship db.Relationship) map[string]any {
	return map[string]any{
		"from":          relationship.Schema + "." + relationship.Table,
		"columns":       relationship.Columns,
		"to":            relationship.TargetSchema + "." + relationship.TargetTable,
		"targetColumns": relationship.TargetColumns,
	}
}

// touchesTable is true where the key starts at this relation or points at it.
func touchesTable(relationship db.Relationship, table db.TableRef) bool {
	starts := relationship.Schema == table.Schema && relationship.Table == table.Name
	points := relationship.TargetSchema == table.Schema &&
		relationship.TargetTable == table.Name
	return starts || points
}

var listRelationshipsFields = []field{
	{
		name: "table", kind: kindString,
		description: "Limit to the relationships that touch this table, unqualified or " +
			"database.table.",
	},
	{
		name: "database", kind: kindString,
		description: "The database the table is in, if `table` is given and not qualified.",
	},
}

var listRelationships = ToolDefinition{
	Name: "list_relationships",
	Description: "List the foreign keys of the connected database, both directions. Pass a " +
		"table to see only the relationships that touch it. Call this to find how to join " +
		"two tables, or what references a table before changing it.",
	InputSchema: buildSchema(listRelationshipsFields),
	Call: func(ctx context.Context, deps ToolDeps, input map[string]any) any {
		read, problem := readInput(listRelationshipsFields, input)
		if problem != "" {
			return refuseInput(problem)
		}
		named, hasName := readText(read, "table")
		database, _ := readText(read, "database")

		target, wanted := db.TableRef{}, false
		if hasName {
			found, missing := resolveTableInput(deps, named, database)
			if missing != nil {
				return missing
			}
			target, wanted = found, true
		}

		all, err := deps.Session.ListRelationships(ctx)
		if err != nil {
			return map[string]any{"error": db.DescribeError(err)}
		}
		matching := all
		if wanted {
			matching = []db.Relationship{}
			for _, relationship := range all {
				if touchesTable(relationship, target) {
					matching = append(matching, relationship)
				}
			}
		}
		listed := matching
		if len(listed) > maxRelationshipsListed {
			listed = listed[:maxRelationshipsListed]
		}
		described := make([]map[string]any, 0, len(listed))
		for _, relationship := range listed {
			described = append(described, describeRelationshipForModel(relationship))
		}
		answer := map[string]any{"relationships": described}
		if len(matching) > len(listed) {
			answer["truncatedBy"] = len(matching) - len(listed)
		}
		return answer
	},
}

func describePlanRowForModel(row result.PlanRow) map[string]any {
	// A count the server did not measure or estimate is written as null, not as a zero, and
	// not left out.
	var estimatedRows, actualRows, selfMs any
	if row.Node.HasEstimatedRows {
		estimatedRows = row.Node.EstimatedRows
	}
	if row.Node.HasActualRows {
		actualRows = row.Node.ActualRows
	}
	if row.Node.HasSelfMs {
		selfMs = row.Node.SelfMs
	}
	return map[string]any{
		"depth":         row.Depth,
		"label":         row.Node.Label,
		"detail":        row.Node.Detail,
		"estimatedRows": estimatedRows,
		"actualRows":    actualRows,
		"selfMs":        selfMs,
		"shareOfTotal":  row.Share,
		"slowest":       row.Slowest,
		"misestimated":  row.Misestimated,
	}
}

var explainQueryFields = []field{
	{
		name: "sql", kind: kindString, required: true,
		description: "The exact statement to plan.",
	},
	{
		name: "analyze", kind: kindBoolean,
		description: "Run the statement to measure it for real, instead of only " +
			"estimating. Only honoured for a statement that only reads; a write is always " +
			"estimated, never run, whatever this says.",
	},
}

var explainQuery = ToolDefinition{
	Name: "explain_query",
	Description: "Get the query plan for a statement: the order tables are read in, which " +
		"indexes are used, and where the row count or the time goes. Call this to check a " +
		"query for a missing index, a bad join order, or a wildly wrong estimate, before " +
		"proposing it or when asked to make one faster.",
	InputSchema: buildSchema(explainQueryFields),
	Call: func(ctx context.Context, deps ToolDeps, input map[string]any) any {
		read, problem := readInput(explainQueryFields, input)
		if problem != "" {
			return refuseInput(problem)
		}
		sql, _ := readText(read, "sql")
		askedToAnalyze, _ := readFlag(read, "analyze")

		// What the server cannot do at all is answered before the user is asked about a
		// statement that would never be sent.
		if !deps.Session.Capabilities().PlansStatement {
			return map[string]any{
				"error": db.DescribeError(db.NewUnsupportedError("plan a statement")),
			}
		}

		statements := deps.Session.Language().SplitStatements(sql)
		risk := language.ResolveBatchRisk(statements, deps.Session.Language())
		canAnalyze := risk == statement.RiskNone
		if !canAnalyze {
			if refusal := deps.Runner.AskToRun(ctx, risk, statements); refusal != "" {
				return map[string]any{"error": refusal}
			}
		}

		willAnalyze := askedToAnalyze && canAnalyze
		plan, err := deps.Session.ExplainQuery(ctx, sql, willAnalyze)
		if err != nil {
			return map[string]any{"error": db.DescribeError(err)}
		}

		nodes := []map[string]any{}
		for _, row := range result.FlattenPlan(plan) {
			nodes = append(nodes, describePlanRowForModel(row))
		}
		answer := map[string]any{
			"analyzed": plan.Analyzed,
			"summary":  result.DescribePlanCost(plan),
			"nodes":    nodes,
		}
		if askedToAnalyze && !canAnalyze {
			answer["note"] = "asked to analyze, but this statement writes, so only the " +
				"estimate is shown; nothing ran"
		}
		return answer
	},
}

var validateQueryFields = []field{
	{
		name: "sql", kind: kindString, required: true,
		description: "The exact statement to check.",
	},
}

var validateQuery = ToolDefinition{
	Name: "validate_query",
	Description: "Check whether a statement parses and its names resolve, without running " +
		"it: no rows come back either way. Call this on a query before presenting it, to " +
		"catch a wrong column or a syntax mistake yourself.",
	InputSchema: buildSchema(validateQueryFields),
	Call: func(ctx context.Context, deps ToolDeps, input map[string]any) any {
		read, problem := readInput(validateQueryFields, input)
		if problem != "" {
			return refuseInput(problem)
		}
		sql, _ := readText(read, "sql")

		if deps.Session.ReadTransactionState() != db.TransactionNone {
			return map[string]any{
				"checked": false,
				"reason":  "a transaction is open; validation only runs outside one",
			}
		}
		found, faulty := deps.Session.CheckStatement(ctx, sql)
		if !faulty {
			return map[string]any{"checked": true, "problem": nil}
		}
		// The offset is written as null where the server placed the fault nowhere.
		var offset any
		if found.HasOffset {
			offset = found.Offset
		}
		return map[string]any{"checked": true, "problem": map[string]any{
			"message": found.Message, "offset": offset,
		}}
	},
}

// Definitions returns everything a caller may ask one connection. Only the last one writes.
func Definitions() []ToolDefinition {
	return []ToolDefinition{
		listTables, describeTable, listIndexes, listConstraints, getTableDDL,
		listRelationships, validateQuery, explainQuery, runQuery,
	}
}
