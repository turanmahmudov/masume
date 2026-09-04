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
	// A backslash starts an escape in a MySQL literal, so it is doubled too. Only a
	// literal rendered for a reader passes through here, never a value a run binds.
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

// BuildSupport joins the facts of a server that speaks the MySQL protocol to this
// dialect and language, so each of those engines names only what it differs in.
func BuildSupport(engine core.Engine) db.EngineSupport {
	return db.EngineSupport{
		EngineInfo: core.ResolveEngineInfo(engine),
		Dialect:    Dialect,
		Language:   language.Mysql,
		Compose:    db.NewSQLComposer(Dialect),
	}
}
