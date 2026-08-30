package mongo

import (
	"strconv"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Dialect says how a name and a value are written into a call. MongoDB has no SQL, so
// the rest of a dialect is for the object menu, which this engine does not offer.
var Dialect = &query.Dialect{
	Engine: core.EngineMongo, Syntax: syntax.FlavourStandard, SchemaWord: "database",
	StatementLanguage: "MongoDB shell calls", FenceTag: "js",
	StatementHint: "db.collection.find({…})",
	StatementExample: "One statement is one call of the shell, such as " +
		"`db.orders.find({status: \"new\"}).sort({total: -1}).limit(20)`, " +
		"`db.orders.aggregate([{$group: {_id: \"$status\", n: {$sum: 1}}}])` or " +
		"`db.getSiblingDB(\"shop\").orders.countDocuments({})`. " +
		"There is no SQL and no join: a document holds what a join would fetch, " +
		"or another collection is read by a second call.",
	// A collection is named after a dot, and a name of any other shape is named by a
	// call instead.
	NamesWithoutQuotes: isPlainName,
	QuoteIdentifier:    strconv.Quote,
	// A call carries its own arguments, so nothing is bound.
	BuildPlaceholder: func(int) string { return "?" },
	CountExpression:  "countDocuments()",
	QuoteTextLiteral: strconv.Quote,
	CanCompareType:   func(string) bool { return true },
	IdentityColumn:   IdentityField,
	DropSchema: func(_ *query.Dialect, schema string) string {
		return BuildStatementText(schema, "", "dropDatabase()")
	},
	DropTrigger: func(*query.Dialect, string, string, string) string {
		return "// mongodb keeps no trigger"
	},
	DropRoutine: func(*query.Dialect, string, string, string) string {
		return "// mongodb keeps no stored routine"
	},
}

// Support is everything known about a MongoDB server before a connection exists.
var Support = db.EngineSupport{
	EngineInfo: core.ResolveEngineInfo(core.EngineMongo),
	Dialect:    Dialect,
	Language:   Language,
	Compose:    NewComposer(Dialect),
}
