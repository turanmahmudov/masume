package sqlserver

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// sqlserverIncomparableTypes is the set of types with no equality operator.
var sqlserverIncomparableTypes = map[string]bool{
	"text": true, "ntext": true, "image": true, "xml": true,
	"geography": true, "geometry": true,
}

// sqlserverBindLimit is the parameter limit of one statement.
const sqlserverBindLimit = 2100

// Dialect writes SQL the way a SQL Server reads it.
var Dialect = &query.Dialect{
	Engine: core.EngineSqlserver, Syntax: syntax.FlavourSqlserver, SchemaWord: "schema",
	StatementLanguage: "T-SQL", FenceTag: "sql", StatementHint: "select … from …",
	QuoteIdentifier: func(name string) string {
		return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
	},
	BuildPlaceholder: func(position int) string { return fmt.Sprintf("@p%d", position) },
	// count(*) returns an int, and count_big(*) a bigint.
	CountExpression: "count_big(*)",
	// A row lock is a table hint after the relation, not a trailing clause.
	RowLockClause: "",
	// An N prefix marks a literal the server reads as Unicode.
	QuoteTextLiteral: func(text string) string {
		return "N'" + strings.ReplaceAll(text, "'", "''") + "'"
	},
	CanCompareType: func(dataType string) bool {
		return !sqlserverIncomparableTypes[query.ReadBaseType(dataType)]
	},
	ColumnTypes: map[core.ColumnKind]string{
		core.KindText: "nvarchar(max)", core.KindInteger: "bigint",
		core.KindNumber: "decimal(38,10)", core.KindBoolean: "bit",
		core.KindTimestamp: "datetime2",
	},
	IdentityColumn: "id bigint identity(1,1) primary key",
	BindLimit:      sqlserverBindLimit,
	Paging: query.Paging{
		BuildWindow: func(limit, offset int) string {
			// The first page needs no window: the client stops reading at its own cap. A
			// window takes a sort, and a sort is refused in a statement that reads the next
			// value of a sequence.
			if offset <= 0 {
				return ""
			}
			return fmt.Sprintf("offset %d rows fetch next %d rows only", offset, limit)
		},
		// OFFSET follows a sort, and this one keeps the rows in the order the server reads them.
		EmptySort: "order by (select null)",
		// A subquery takes a sort only together with a window.
		SortKeeper: "offset 0 rows",
		BuildCappedRead: func(target string, rows int) string {
			return fmt.Sprintf("select top %d *\n  from %s;", rows, target)
		},
	},
	AddColumn: func(dialect *query.Dialect, target query.QualifiedName) string {
		return "alter table " + dialect.BuildQualifiedName(target) +
			"\n  add new_column " + dialect.BuildColumnType(core.KindText) + ";"
	},
	// The server renames a relation through a procedure, which takes the new name alone.
	RenameTable: func(dialect *query.Dialect, target query.QualifiedName, renamed string) string {
		return fmt.Sprintf("exec sp_rename %s, %s;",
			dialect.QuoteTextLiteral(target.Schema+"."+target.Name),
			dialect.QuoteTextLiteral(renamed))
	},
	DropSchema: func(dialect *query.Dialect, schema string) string {
		return "drop schema " + dialect.QuoteIdentifier(schema) + ";"
	},
	DropTrigger: func(dialect *query.Dialect, schema, name, _ string) string {
		return "drop trigger " +
			dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: name}) + ";"
	},
	DropRoutine: func(dialect *query.Dialect, schema, name, identity string) string {
		return fmt.Sprintf("drop %s %s;", ReadRoutineKind(identity),
			dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: name}))
	},
}

// Support is everything known about a SQL Server before a connection exists.
var Support = db.EngineSupport{
	EngineInfo: core.ResolveEngineInfo(core.EngineSqlserver),
	Dialect:    Dialect,
	Language:   language.Sqlserver,
	Compose:    db.NewSQLComposer(Dialect),
}
