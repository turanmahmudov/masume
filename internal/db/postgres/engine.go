package postgres

import (
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// postgresIncomparableTypes are the types PostgreSQL has no equality for, so a
// comparison is an error and not a false. `jsonb` is another type, and it compares.
var postgresIncomparableTypes = map[string]bool{"json": true, "xml": true}

// Dialect writes SQL the way every PostgreSQL-protocol server reads it.
var Dialect = &query.Dialect{
	Engine: core.EnginePostgres, Syntax: syntax.FlavourStandard, SchemaWord: "schema",
	StatementLanguage: "SQL", FenceTag: "sql", StatementHint: "select … from …",
	QuoteIdentifier: func(name string) string {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	},
	BuildPlaceholder: func(position int) string { return fmt.Sprintf("$%d", position) },
	CountExpression:  "count(*)::int8",
	RowLockClause:    " for update",
	// A backslash is a plain character in a Postgres literal, so only a quote is doubled.
	QuoteTextLiteral: func(text string) string {
		return "'" + strings.ReplaceAll(text, "'", "''") + "'"
	},
	CanCompareType: func(dataType string) bool {
		return !postgresIncomparableTypes[query.ReadBaseType(dataType)]
	},
	ColumnTypes: map[core.ColumnKind]string{
		core.KindText: "text", core.KindInteger: "bigint", core.KindNumber: "numeric",
		core.KindBoolean: "boolean", core.KindTimestamp: "timestamptz",
	},
	IdentityColumn: "id bigserial primary key",
	DropSchema: func(dialect *query.Dialect, schema string) string {
		return "drop schema " + dialect.QuoteIdentifier(schema) + " restrict;"
	},
	DropTrigger: func(dialect *query.Dialect, schema, name, table string) string {
		target := dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: table})
		return "drop trigger " + dialect.QuoteIdentifier(name) + " on " + target + ";"
	},
	// ROUTINE covers a function and a procedure. DROP FUNCTION refuses a procedure.
	DropRoutine: func(dialect *query.Dialect, schema, name, _ string) string {
		return "drop routine " +
			dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: name}) + ";"
	},
}

// Support is everything known about a PostgreSQL server before a connection exists.
var Support = db.EngineSupport{
	EngineInfo: core.ResolveEngineInfo(core.EnginePostgres),
	Dialect:    Dialect,
	Language:   language.SQL,
	Compose:    db.NewSQLComposer(Dialect),
}

// BuildSupport joins the facts of a server that speaks the PostgreSQL protocol to this
// dialect and language, so each of those engines names only what it differs in.
func BuildSupport(engine core.Engine) db.EngineSupport {
	return db.EngineSupport{
		EngineInfo: core.ResolveEngineInfo(engine),
		Dialect:    Dialect,
		Language:   language.SQL,
		Compose:    db.NewSQLComposer(Dialect),
	}
}
