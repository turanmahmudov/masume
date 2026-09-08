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

// Database tools for models.

// Default listing limits.
const (
	maxRelationshipsListed = 300
	maxTablesListed        = 200
)

// buildNamePattern builds a case-insensitive substring pattern with * wildcards.
func buildNamePattern(written string) (*regexp.Regexp, error) {
	escaped := regexp.QuoteMeta(written)
	// The star is the only part of the pattern that is not literal.
	return regexp.Compile("(?i)" + strings.ReplaceAll(escaped, `\*`, ".*"))
}

// resolveDatabase returns the schema of the call, or the schema of the connection.
func resolveDatabase(deps ToolDeps, named string) string {
	if named != "" {
		return named
	}
	return deps.Session.Describe().DefaultSchema
}

// listSchemaNames returns every schema that contains a table, sorted by name.
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

// resolveTableInput resolves a table name or returns an error response.
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
			"hint": "call list_tables to check available tables",
		}
	}
	return found, nil
}

// tableInputFields are the arguments of a tool that takes one table name.
var tableInputFields = []field{
	{
		name: "table", kind: kindString, required: true,
		description: "The table name, unqualified, or written as database.table.",
	},
	{
		name: "database", kind: kindString,
		description: "The table database or schema, if different from the connection default " +
			"and absent from `table`.",
	},
}

// defineTableReadTool builds a table metadata tool.
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

// refuseInput returns the error of a call as a text for the model.
func refuseInput(problem string) map[string]any {
	return map[string]any{
		"error": problem, "hint": "check the tool input schema and correct the arguments",
	}
}

var listTablesFields = []field{
	{
		name: "database", kind: kindString,
		description: "The database or schema to list. Defaults to the connection default.",
	},
	{
		name: "pattern", kind: kindString,
		description: "A case-insensitive substring pattern, " +
			"with * for any sequence of characters. \"order\" matches " +
			"order_item, and \"*_log\" finds audit_log.",
	},
	{
		name: "limit", kind: kindInteger, positive: true,
		description: "Maximum tables to return. Default: 200.",
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
				return refuseInput(`invalid pattern: "` + pattern + `"`)
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
					"hint":  "omit the pattern to list available tables",
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
			// Omit unavailable or zero estimates.
			if table.EstimatedRows > 0 {
				held["estimatedRows"] = table.EstimatedRows
			}
			described = append(described, held)
		}
		answer := map[string]any{"database": schema, "tables": described}
		if len(matching) > len(listed) {
			answer["truncatedBy"] = len(matching) - len(listed)
			answer["hint"] = "use a more specific pattern instead of increasing the limit"
		}
		return answer
	},
}

func describeColumnForModel(column db.ColumnDetail) map[string]any {
	// A missing default is null. An empty string is a valid default.
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
	// Include allowed enum values.
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

// Refresh table metadata from the server on each call.
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
	"Get a CREATE TABLE statement from the table metadata available through this connection. "+
		"Call this for the table definition in one response "+
		"instead of separate describe_table and list_constraints calls.",
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

// touchesTable is true if the key starts at this table or refers to it.
func touchesTable(relationship db.Relationship, table db.TableRef) bool {
	starts := relationship.Schema == table.Schema && relationship.Table == table.Name
	points := relationship.TargetSchema == table.Schema &&
		relationship.TargetTable == table.Name
	return starts || points
}

var listRelationshipsFields = []field{
	{
		name: "table", kind: kindString,
		description: "Limit to foreign keys from or to this table, unqualified or " +
			"database.table.",
	},
	{
		name: "database", kind: kindString,
		description: "The database the table is in, if `table` is given and not qualified.",
	},
}

var listRelationships = ToolDefinition{
	Name: "list_relationships",
	Description: "List foreign keys in the connected database. Pass a " +
		"table to list foreign keys from or to that table. Call this to find joins " +
		"or check references before changing a table.",
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
	// Unavailable counts and times are null.
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
		description: "Request execution measurements instead of estimates. " +
			"Only statements classified as read-only are eligible. " +
			"Other statements receive estimates without execution, regardless of this value.",
	},
}

var explainQuery = ToolDefinition{
	Name: "explain_query",
	Description: "Get a query plan with table scan order, indexes, row counts, and execution times when available. " +
		"Call this to check for missing indexes, inefficient joins, or inaccurate estimates " +
		"before proposing a query or when asked to improve query performance.",
	InputSchema: buildSchema(explainQueryFields),
	Call: func(ctx context.Context, deps ToolDeps, input map[string]any) any {
		read, problem := readInput(explainQueryFields, input)
		if problem != "" {
			return refuseInput(problem)
		}
		sql, _ := readText(read, "sql")
		askedToAnalyze, _ := readFlag(read, "analyze")

		// Check plan support before requesting permission.
		if !deps.Session.Capabilities().PlansStatement {
			return map[string]any{
				"error": db.DescribeError(db.NewUnsupportedError("plan a statement")),
			}
		}

		statements := deps.Session.Language().SplitStatements(sql)
		risk := language.ResolveBatchRisk(statements, deps.Session.Language())
		canAnalyze := risk == statement.RiskNone
		if !canAnalyze {
			permission := deps.Runner.AskToRun(ctx, PurposePlan, risk, statements)
			if permission.Refusal != "" {
				return map[string]any{"error": permission.Refusal}
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
			answer["note"] = "analysis was requested, but this statement is not classified as read-only; " +
				"the plan contains estimates only, without statement execution"
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
	Description: "Check statement syntax and name resolution without execution. " +
		"No result rows are returned. Call this before presenting a query to check " +
		"for invalid column names and syntax errors.",
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
				"reason":  "validation requires no active transaction",
			}
		}
		found, faulty := deps.Session.CheckStatement(ctx, sql)
		if !faulty {
			return map[string]any{"checked": true, "problem": nil}
		}
		// The offset is written as null if the server gave no position for the error.
		var offset any
		if found.HasOffset {
			offset = found.Offset
		}
		return map[string]any{"checked": true, "problem": map[string]any{
			"message": found.Message, "offset": offset,
		}}
	},
}

// Definitions returns the database tools available to callers.
func Definitions() []ToolDefinition {
	return []ToolDefinition{
		listTables, describeTable, listIndexes, listConstraints, getTableDDL,
		listRelationships, validateQuery, explainQuery, planWrite, runQuery,
	}
}
