package mysql

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Dialect writes SQL the way every MySQL-protocol server reads it.
var Dialect = &query.Dialect{
	Engine: core.EngineMysql, Syntax: syntax.FlavourMysql, SchemaWord: "database",
	StatementLanguage: "SQL", FenceTag: "sql", StatementHint: "select … from …",
	QuoteIdentifier: func(name string) string {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	},
	BuildPlaceholder: func(int) string { return "?" },
	CountExpression:  "count(*)",
	RowLockClause:    " for update",
	// MySQL literals escape backslashes and quotes. Executed values use bound parameters.
	// MySQL reads bytes as a hexadecimal literal.
	RenderBytes: func(hex string) string { return "x'" + hex + "'" },
	QuoteTextLiteral: func(text string) string {
		return "'" + strings.ReplaceAll(strings.ReplaceAll(text, `\`, `\\`), "'", "''") + "'"
	},
	// MySQL compares every type it stores, including its own `json`.
	CanCompareType: func(string) bool { return true },
	ColumnTypes: map[core.ColumnKind]string{
		core.KindText: "text", core.KindInteger: "bigint", core.KindNumber: "decimal(38,10)",
		core.KindBoolean: "boolean", core.KindTimestamp: "datetime",
	},
	IdentityColumn: "id bigint auto_increment primary key",
	// A MySQL definition names no database, so a dump of one database selects it first.
	SelectSchema: func(dialect *query.Dialect, schema string) string {
		return "use " + dialect.QuoteIdentifier(schema) + ";"
	},
	// A MySQL schema is a database, so a drop removes the database.
	DropSchema: func(dialect *query.Dialect, schema string) string {
		return "drop database " + dialect.QuoteIdentifier(schema) + ";"
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

// Support is everything known about a MySQL server before a connection exists.
var Support = db.EngineSupport{
	EngineInfo: core.ResolveEngineInfo(core.EngineMysql),
	Dialect:    Dialect,
	Language:   language.Mysql,
	Compose:    db.NewSQLComposer(Dialect),
}

// BuildSupport combines engine metadata with the MySQL dialect and language.
func BuildSupport(engine core.Engine) db.EngineSupport {
	return db.EngineSupport{
		EngineInfo: core.ResolveEngineInfo(engine),
		Dialect:    Dialect,
		Language:   language.Mysql,
		Compose:    db.NewSQLComposer(Dialect),
	}
}
