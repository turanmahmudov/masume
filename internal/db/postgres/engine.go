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

// postgresIncomparableTypes is the set of types without equality operators. jsonb supports equality.
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
	// PostgreSQL reads bytes as a string of the hexadecimal form.
	RenderBytes: func(hex string) string { return `'\x` + hex + `'` },
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
	// A PostgreSQL array is written as `{…}` inside a string, and each element that holds
	// a comma, a quote or a backslash is quoted inside it.
	RenderList: func(dialect *query.Dialect, texts []string) string {
		written := make([]string, 0, len(texts))
		for _, text := range texts {
			if text == "NULL" {
				written = append(written, text)
				continue
			}
			held := strings.ReplaceAll(text, `\`, `\\`)
			written = append(written, `"`+strings.ReplaceAll(held, `"`, `\"`)+`"`)
		}
		return dialect.QuoteTextLiteral("{" + strings.Join(written, ",") + "}")
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

// BuildSupport combines engine metadata with the PostgreSQL dialect and language.
func BuildSupport(engine core.Engine) db.EngineSupport {
	return db.EngineSupport{
		EngineInfo: core.ResolveEngineInfo(engine),
		Dialect:    Dialect,
		Language:   language.SQL,
		Compose:    db.NewSQLComposer(Dialect),
	}
}
