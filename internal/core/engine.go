package core

import (
	"slices"
	"strings"
)

// Engine names one server the client knows.
type Engine string

// The servers the client knows. Each one has an entry in the registry below.
const (
	EnginePostgres    Engine = "postgres"
	EngineMysql       Engine = "mysql"
	EngineSqlite      Engine = "sqlite"
	EngineRedis       Engine = "redis"
	EngineCockroach   Engine = "cockroach"
	EngineTimescale   Engine = "timescale"
	EngineRedshift    Engine = "redshift"
	EngineNeon        Engine = "neon"
	EngineSupabase    Engine = "supabase"
	EngineMariadb     Engine = "mariadb"
	EngineTidb        Engine = "tidb"
	EnginePlanetscale Engine = "planetscale"
	EngineAuroraMysql Engine = "aurora-mysql"
	EngineMongo       Engine = "mongodb"
)

// Engines lists every engine, in the order the docs name them.
var Engines = []Engine{
	EnginePostgres, EngineMysql, EngineSqlite, EngineRedis,
	EngineCockroach, EngineTimescale, EngineRedshift, EngineNeon, EngineSupabase,
	EngineMariadb, EngineTidb, EnginePlanetscale, EngineAuroraMysql,
	EngineMongo,
}

// DefaultEngine is the engine used where a profile names none.
const DefaultEngine = EnginePostgres

// Family is the protocol a server speaks, which decides the adapter that opens
// it and the SQL written for it.
type Family string

// The five protocols behind the fourteen engines.
const (
	FamilyPostgres Family = "postgres"
	FamilyMysql    Family = "mysql"
	FamilySqlite   Family = "sqlite"
	FamilyRedis    Family = "redis"
	FamilyMongo    Family = "mongo"
)

// Capabilities names what a server either returns for or does not.
type Capabilities struct {
	PlansStatement bool
	MeasuresPlan   bool
	// Most servers report a syntax error when asked for the plan of a DROP.
	PlansEveryStatement bool
	HasServerSessions   bool
	// Cancelling needs a second connection to the same server.
	CancelsRunningQuery bool
	HasTransactions     bool
	// A key store returns a scan in its own key order only.
	SortsRead      bool
	TruncatesTable bool
	WritesDDL      bool
	// True where a connection can be opened read-only. TiDB is the one that cannot: it
	// takes the statement and does nothing under it.
	TakesReadOnlyMode bool
	// True where the staged work of the grid lands whole or not at all. This is not the
	// same as HasTransactions: Redis holds no transaction the user drives and still runs
	// a staged set inside a MULTI, and a standalone MongoDB holds neither.
	AppliesChangesTogether bool
}

// EngineInfo holds the facts about a server that are known before any connection
// exists. The dialect and the language are joined to it in the query tier.
type EngineInfo struct {
	Engine        Engine
	Family        Family
	Capabilities  Capabilities
	DefaultPort   int
	OpensFile     bool
	NeedsUser     bool
	NeedsPassword bool
	// True where a password belongs to the server rather than to a named user, so a
	// profile that names no user can still have one. Redis is the one: `requirepass`
	// names nobody. Every other server checks a password against a user, so a profile
	// that names none has nothing to give a password to.
	PasswordWithoutUser bool
	URLSchemes          []string
	DefaultSSLMode      SSLMode
	// The schemas this server keeps for itself, named in full.
	SystemSchemas []string
	// The prefixes of the schemas this server creates for itself.
	SystemSchemaPrefixes []string
}

// The catalog schemas every PostgreSQL-protocol server has.
var postgresCatalogSchemas = []string{"pg_catalog", "information_schema"}

// The prefixes of the schemas a PostgreSQL server creates for itself.
var postgresOwnPrefixes = []string{"pg_toast", "pg_temp"}

// The databases every MySQL-protocol server keeps for itself.
var mysqlSystemSchemas = []string{"mysql", "information_schema", "performance_schema", "sys"}

var postgresCapabilities = Capabilities{
	PlansStatement:      true,
	MeasuresPlan:        true,
	PlansEveryStatement: false,
	HasServerSessions:   true,
	CancelsRunningQuery: true,
	HasTransactions:     true,
	SortsRead:           true,
	TruncatesTable:      true,
	WritesDDL:           true,
	TakesReadOnlyMode:   true,

	AppliesChangesTogether: true,
}

var mysqlCapabilities = postgresCapabilities

var engineRegistry = map[Engine]EngineInfo{
	EnginePostgres: {
		Engine: EnginePostgres, Family: FamilyPostgres, Capabilities: postgresCapabilities,
		DefaultPort: 5432, NeedsUser: true, NeedsPassword: true,
		URLSchemes:    []string{"postgres", "postgresql"},
		SystemSchemas: postgresCatalogSchemas, SystemSchemaPrefixes: postgresOwnPrefixes,
	},
	EngineCockroach: {
		Engine: EngineCockroach, Family: FamilyPostgres,
		Capabilities: withPostgres(func(capabilities *Capabilities) {
			// The server plans a schema change too: `explain drop table` returns a plan.
			capabilities.PlansEveryStatement = true
			// The server names a session with its own string, not the number in
			// `pg_stat_activity`, and it has no `pg_cancel_backend`.
			capabilities.HasServerSessions = false
			capabilities.CancelsRunningQuery = false
		}),
		DefaultPort: 26257, NeedsUser: true, NeedsPassword: true,
		URLSchemes:           []string{"cockroachdb"},
		SystemSchemas:        append(append([]string{}, postgresCatalogSchemas...), "pg_extension", "crdb_internal"),
		SystemSchemaPrefixes: postgresOwnPrefixes,
	},
	EngineTimescale: {
		Engine: EngineTimescale, Family: FamilyPostgres, Capabilities: postgresCapabilities,
		DefaultPort: 5432, NeedsUser: true, NeedsPassword: true,
		SystemSchemas:        postgresCatalogSchemas,
		SystemSchemaPrefixes: append(append([]string{}, postgresOwnPrefixes...), "_timescaledb_", "timescaledb_"),
	},
	EngineRedshift: {
		Engine: EngineRedshift, Family: FamilyPostgres,
		// EXPLAIN only estimates. There is no ANALYZE that runs and measures.
		Capabilities: withPostgres(func(capabilities *Capabilities) { capabilities.MeasuresPlan = false }),
		DefaultPort:  5439, NeedsUser: true, NeedsPassword: true,
		URLSchemes: []string{"redshift"},
		// The cluster returns over TLS only.
		DefaultSSLMode:       SSLRequire,
		SystemSchemas:        append(append([]string{}, postgresCatalogSchemas...), "pg_internal", "catalog_history"),
		SystemSchemaPrefixes: postgresOwnPrefixes,
	},
	EngineNeon: {
		Engine: EngineNeon, Family: FamilyPostgres, Capabilities: postgresCapabilities,
		DefaultPort: 5432, NeedsUser: true, NeedsPassword: true, DefaultSSLMode: SSLRequire,
		SystemSchemas: postgresCatalogSchemas, SystemSchemaPrefixes: postgresOwnPrefixes,
	},
	EngineSupabase: {
		Engine: EngineSupabase, Family: FamilyPostgres, Capabilities: postgresCapabilities,
		DefaultPort: 5432, NeedsUser: true, NeedsPassword: true, DefaultSSLMode: SSLRequire,
		SystemSchemas: append(append([]string{}, postgresCatalogSchemas...),
			"auth", "storage", "realtime", "graphql", "graphql_public", "extensions",
			"vault", "supabase_functions", "supabase_migrations", "pgbouncer", "net", "cron"),
		SystemSchemaPrefixes: postgresOwnPrefixes,
	},
	EngineMysql: {
		Engine: EngineMysql, Family: FamilyMysql, Capabilities: mysqlCapabilities,
		DefaultPort: 3306, NeedsUser: true, NeedsPassword: true,
		URLSchemes: []string{"mysql"}, SystemSchemas: mysqlSystemSchemas,
	},
	EngineMariadb: {
		Engine: EngineMariadb, Family: FamilyMysql, Capabilities: mysqlCapabilities,
		DefaultPort: 3306, NeedsUser: true, NeedsPassword: true,
		URLSchemes: []string{"mariadb"}, SystemSchemas: mysqlSystemSchemas,
	},
	EngineTidb: {
		Engine: EngineTidb, Family: FamilyMysql,
		// The server takes `set session transaction read only` and does nothing under it.
		Capabilities: withMysql(func(capabilities *Capabilities) {
			capabilities.TakesReadOnlyMode = false
		}),
		DefaultPort: 4000, NeedsUser: true, NeedsPassword: true,
		SystemSchemas: append(append([]string{}, mysqlSystemSchemas...), "metrics_schema"),
	},
	EnginePlanetscale: {
		Engine: EnginePlanetscale, Family: FamilyMysql,
		Capabilities: withMysql(func(capabilities *Capabilities) {
			capabilities.HasServerSessions = false
			capabilities.CancelsRunningQuery = false
		}),
		DefaultPort: 3306, NeedsUser: true, NeedsPassword: true, DefaultSSLMode: SSLRequire,
		SystemSchemas: mysqlSystemSchemas,
	},
	EngineAuroraMysql: {
		Engine: EngineAuroraMysql, Family: FamilyMysql, Capabilities: mysqlCapabilities,
		DefaultPort: 3306, NeedsUser: true, NeedsPassword: true,
		SystemSchemas: mysqlSystemSchemas,
	},
	EngineSqlite: {
		Engine: EngineSqlite, Family: FamilySqlite,
		Capabilities: Capabilities{
			// SQLite plans a statement, but measures nothing.
			PlansStatement: true,
			MeasuresPlan:   false,
			// A file has only the session that opened it, and the driver reads it in
			// the thread that draws.
			HasServerSessions:   false,
			CancelsRunningQuery: false,
			HasTransactions:     true,
			SortsRead:           true,
			// SQLite empties a table by deleting every row.
			TruncatesTable:         false,
			WritesDDL:              true,
			TakesReadOnlyMode:      true,
			AppliesChangesTogether: true,
		},
		// A file is opened, not reached, so there is no port and no URL scheme.
		DefaultPort: 0, OpensFile: true,
	},
	EngineMongo: {
		Engine: EngineMongo, Family: FamilyMongo,
		Capabilities: Capabilities{
			// The server explains a find and an aggregate, and measures either one.
			PlansStatement: true,
			MeasuresPlan:   true,
			// A write command is not one the server explains.
			PlansEveryStatement: false,
			// currentOp lists every running operation, and killOp ends one.
			HasServerSessions: true,
			// The driver cancels through the context, and no second connection finds
			// the operation id of the call it would kill.
			CancelsRunningQuery: false,
			// A replica set and a sharded cluster both hold a transaction. A standalone
			// server holds none, and the session reports what the deployment it reached
			// actually answered.
			HasTransactions: true,
			// A find takes a sort, so the server orders the page.
			SortsRead: true,
			// A collection is emptied by a delete of every document, not by a command
			// of its own.
			TruncatesTable: false,
			// Every statement is a command, so the object menu has no SQL to write.
			WritesDDL: false,
			// The server holds no read-only session, so this client refuses the write.
			TakesReadOnlyMode: true,
			// A replica set applies a staged set inside a transaction. A standalone
			// server holds none, and the session says so once it knows what it reached.
			AppliesChangesTogether: true,
		},
		// A user is what turns authentication on: a server that has it turned off
		// refuses a connection that carries one, so the profile may name none. A
		// profile that does name one is asked for its password like any other server.
		NeedsPassword: true,
		DefaultPort:   27017,
		URLSchemes:    []string{"mongodb"},
		SystemSchemas: []string{"admin", "config", "local"},
	},
	EngineRedis: {
		Engine: EngineRedis, Family: FamilyRedis,
		Capabilities: Capabilities{
			// A command says what it does, so the server plans nothing.
			PlansStatement: false,
			MeasuresPlan:   false,
			// CLIENT LIST lists every connection, and CLIENT KILL ends one.
			HasServerSessions: true,
			// Redis runs one command at a time and returns before the next.
			CancelsRunningQuery: false,
			// MULTI queues the commands and returns nothing until EXEC.
			HasTransactions: false,
			// A SCAN returns the keys in the order of the server.
			SortsRead: false,
			// A prefix is only a name pattern, so no command empties it.
			TruncatesTable: false,
			WritesDDL:      false,
			// The server holds no read-only session, so this client refuses the write.
			TakesReadOnlyMode: true,
			// MULTI holds no transaction the user drives, and it does run a staged set
			// as one.
			AppliesChangesTogether: true,
		},
		DefaultPort: 6379, URLSchemes: []string{"redis", "rediss"},
		PasswordWithoutUser: true,
	},
}

func withPostgres(change func(*Capabilities)) Capabilities {
	capabilities := postgresCapabilities
	change(&capabilities)
	return capabilities
}

func withMysql(change func(*Capabilities)) Capabilities {
	capabilities := mysqlCapabilities
	change(&capabilities)
	return capabilities
}

// ResolveEngineInfo returns the facts of that engine, and the facts of the default engine
// for a name nothing knows, so a caller never reads a port of zero. A name is checked where
// a profile is built, which is where a bad one is reported.
func ResolveEngineInfo(engine Engine) EngineInfo {
	info, known := engineRegistry[engine]
	if !known {
		return engineRegistry[DefaultEngine]
	}
	return info
}

// ListEngineInfo returns the facts of every engine, in the order they are named.
func ListEngineInfo() []EngineInfo {
	listed := make([]EngineInfo, 0, len(Engines))
	for _, engine := range Engines {
		listed = append(listed, engineRegistry[engine])
	}
	return listed
}

// FindEngine reads this text as an engine name.
func FindEngine(written string) (Engine, bool) {
	return FindAllowed(Engines, strings.ToLower(strings.TrimSpace(written)))
}

// HoldsSystemSchema is true where the server keeps that schema for itself.
func (info EngineInfo) HoldsSystemSchema(schema string) bool {
	name := strings.ToLower(schema)
	if slices.Contains(info.SystemSchemas, name) {
		return true
	}
	for _, prefix := range info.SystemSchemaPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// OpensFile is true for an engine that opens a file, which has no host, port or user.
func OpensFile(engine Engine) bool {
	return ResolveEngineInfo(engine).OpensFile
}

// NeedsUser is true if the engine connects as a named user.
func NeedsUser(engine Engine) bool {
	return ResolveEngineInfo(engine).NeedsUser
}

// ResolveDefaultPort returns the port the engine listens on.
func ResolveDefaultPort(engine Engine) int {
	return ResolveEngineInfo(engine).DefaultPort
}
