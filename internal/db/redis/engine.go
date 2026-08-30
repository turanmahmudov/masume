package redis

import (
	"regexp"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// plainKey matches a key Redis reads back exactly as it is written.
var plainKey = regexp.MustCompile(`^[\w:.-]+$`)

// Dialect says how a key is written into a command. Redis has no SQL, so the
// rest of a dialect is for the object menu, which Redis has not.
var Dialect = &query.Dialect{
	Engine: core.EngineRedis, Syntax: syntax.FlavourStandard, SchemaWord: "database",
	StatementLanguage: "Redis commands", FenceTag: "redis",
	StatementHint: "GET key",
	StatementExample: `One statement is one command on its own line, such as ` +
		"`GET order:1` or `SCAN 0 MATCH order:* COUNT 500`. There is no SQL.",
	// A key holds characters SQL would quote, so the key rule is its own.
	NamesWithoutQuotes: plainKey.MatchString,
	// A key is quoted like a command line argument, not like SQL.
	QuoteIdentifier: func(name string) string {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(name, `\`, `\\`), `"`, `\"`) + `"`
	},
	// A command holds its own arguments, so nothing is bound.
	BuildPlaceholder: func(int) string { return "?" },
	CountExpression:  "DBSIZE",
	QuoteTextLiteral: func(text string) string {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(text, `\`, `\\`), `"`, `\"`) + `"`
	},
	CanCompareType: func(string) bool { return true },
	IdentityColumn: "key",
	DropSchema:     func(*query.Dialect, string) string { return "FLUSHDB" },
	DropTrigger: func(*query.Dialect, string, string, string) string {
		return "# redis keeps no trigger"
	},
	DropRoutine: func(*query.Dialect, string, string, string) string {
		return "# redis keeps no stored routine"
	},
}

// Support is everything known about a Redis server before a connection exists.
var Support = db.EngineSupport{
	EngineInfo: core.ResolveEngineInfo(core.EngineRedis),
	Dialect:    Dialect,
	Language:   Language,
	Compose:    NewComposer(Dialect),
}
