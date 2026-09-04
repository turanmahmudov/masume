package sqlite

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Dialect writes SQL the way SQLite reads it. What it calls a schema is an
// attached database, and `main` is the file itself.
var Dialect = &query.Dialect{
	Engine: core.EngineSqlite, Syntax: syntax.FlavourStandard, SchemaWord: "database",
	StatementLanguage: "SQL", FenceTag: "sql", StatementHint: "select … from …",
	QuoteIdentifier: func(name string) string {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	},
	BuildPlaceholder: func(int) string { return "?" },
	CountExpression:  "count(*)",
	QuoteTextLiteral: func(text string) string {
		return "'" + strings.ReplaceAll(text, "'", "''") + "'"
	},
	// SQLite compares any two values it stores, whatever the column type is.
	CanCompareType: func(string) bool { return true },
	BindLimit:      32766,
	ColumnTypes: map[core.ColumnKind]string{
		core.KindText: "text", core.KindInteger: "integer", core.KindNumber: "real",
		// SQLite holds neither a boolean nor a timestamp of its own: a boolean is a
		// number of zero or one, and a timestamp is the text of its own form.
		core.KindBoolean: "integer", core.KindTimestamp: "text",
	},
	IdentityColumn: "id integer primary key autoincrement",
	// A SQLite database is a file, and DETACH is the only way to release one.
	DropSchema: func(dialect *query.Dialect, schema string) string {
		return "detach database " + dialect.QuoteIdentifier(schema) + ";"
	},
	DropTrigger: func(dialect *query.Dialect, schema, name, _ string) string {
		return "drop trigger " +
			dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: name}) + ";"
	},
	DropRoutine: func(*query.Dialect, string, string, string) string {
		return "-- sqlite keeps no stored routine"
	},
}

// Support is everything known about a SQLite file before it is opened.
var Support = db.EngineSupport{
	EngineInfo: core.ResolveEngineInfo(core.EngineSqlite),
	Dialect:    Dialect,
	Language:   language.SQL,
	Compose:    db.NewSQLComposer(Dialect),
}
