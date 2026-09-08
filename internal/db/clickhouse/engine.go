package clickhouse

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Dialect writes SQL the way a ClickHouse reads it.
var Dialect = &query.Dialect{
	// The server reads a backtick name, a backslash escape and a hash comment, as MySQL does.
	Engine: core.EngineClickhouse, Syntax: syntax.FlavourMysql, SchemaWord: "database",
	StatementLanguage: "SQL", FenceTag: "sql", StatementHint: "select … from …",
	QuoteIdentifier: func(name string) string {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	},
	BuildPlaceholder: func(int) string { return "?" },
	CountExpression:  "count()",
	// The server takes no row lock a statement can ask for.
	RowLockClause: "",
	QuoteTextLiteral: func(text string) string {
		return "'" + strings.ReplaceAll(text, "'", "''") + "'"
	},
	// The server compares every type it stores, including an array and a map.
	CanCompareType: func(string) bool { return true },
	ColumnTypes: map[core.ColumnKind]string{
		core.KindText: "String", core.KindInteger: "Int64",
		core.KindNumber: "Decimal(38, 10)", core.KindBoolean: "Bool",
		core.KindTimestamp: "DateTime64(3)",
	},
	// The server numbers no column of its own, so a new table numbers its rows itself.
	IdentityColumn: "id UInt64",
	// A new table needs an engine, and a MergeTree needs the order it keeps its rows in. A
	// table with no column to order by keeps its rows in the order they were written.
	TableSuffix: func(keyColumn string) string {
		if keyColumn == "" {
			keyColumn = "tuple()"
		}
		return "engine = MergeTree\norder by " + keyColumn
	},
	// A write of one row is a mutation of the table.
	UpdateRow: func(_ *query.Dialect, target, assignments, predicate string) string {
		return fmt.Sprintf("alter table %s update %s where %s",
			target, assignments, predicate)
	},
	AddColumn: func(dialect *query.Dialect, target query.QualifiedName) string {
		return "alter table " + dialect.BuildQualifiedName(target) +
			"\n  add column new_column " + dialect.BuildColumnType(core.KindText) + ";"
	},
	RenameTable: func(dialect *query.Dialect, target query.QualifiedName, renamed string) string {
		return "rename table " + dialect.BuildQualifiedName(target) + "\n  to " +
			dialect.BuildQualifiedName(
				query.QualifiedName{Schema: target.Schema, Name: renamed}) + ";"
	},
	// A ClickHouse schema is a database, so a drop removes the database.
	DropSchema: func(dialect *query.Dialect, schema string) string {
		return "drop database " + dialect.QuoteIdentifier(schema) + ";"
	},
	DropTrigger: func(*query.Dialect, string, string, string) string {
		return "-- ClickHouse has no trigger"
	},
	// A function of the user carries no database, so a drop names it alone.
	DropRoutine: func(dialect *query.Dialect, _, name, _ string) string {
		return "drop function " + dialect.QuoteIdentifier(name) + ";"
	},
}

// Support is everything known about a ClickHouse before a connection exists.
var Support = db.EngineSupport{
	EngineInfo: core.ResolveEngineInfo(core.EngineClickhouse),
	Dialect:    Dialect,
	Language:   language.Mysql,
	Compose:    NewComposer(Dialect),
}
