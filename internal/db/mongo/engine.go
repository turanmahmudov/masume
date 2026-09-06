package mongo

import (
	"strconv"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Dialect is the MongoDB name and value syntax configuration.
var Dialect = &query.Dialect{
	Engine: core.EngineMongo, Syntax: syntax.FlavourStandard, SchemaWord: "database",
	StatementLanguage: "MongoDB shell calls", FenceTag: "js",
	StatementHint: "db.collection.find({…})",
	StatementExample: "One statement is one shell call chain, such as " +
		"`db.orders.find({status: \"new\"}).sort({total: -1}).limit(20)`, " +
		"`db.orders.aggregate([{$group: {_id: \"$status\", n: {$sum: 1}}}])` or " +
		"`db.getSiblingDB(\"shop\").orders.countDocuments({})`. " +
		"Use MongoDB shell calls, not SQL. " +
		"Read related data from embedded documents or another collection.",
	// Plain collection names follow a dot. Other names use getCollection.
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
		return "// MongoDB triggers are unsupported"
	},
	DropRoutine: func(*query.Dialect, string, string, string) string {
		return "// MongoDB stored routines are unsupported"
	},
}

// Support is everything known about a MongoDB server before a connection exists.
var Support = db.EngineSupport{
	EngineInfo: core.ResolveEngineInfo(core.EngineMongo),
	Dialect:    Dialect,
	Language:   Language,
	Compose:    NewComposer(Dialect),
}
