package core

import (
	"slices"
	"strings"
)

// Engine is the name of one database server the client supports.
type Engine string

// The supported servers. Each one has an entry in the registry below.
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

// Engines lists every engine, in the order used by the docs.
var Engines = []Engine{
	EnginePostgres, EngineMysql, EngineSqlite, EngineRedis,
	EngineCockroach, EngineTimescale, EngineRedshift, EngineNeon, EngineSupabase,
	EngineMariadb, EngineTidb, EnginePlanetscale, EngineAuroraMysql,
	EngineMongo,
}

// DefaultEngine is the engine used when a profile does not name one.
const DefaultEngine = EnginePostgres

// Family is the protocol of a server. It selects the adapter that connects to the
// server and the SQL dialect used for it.
type Family string

// The five protocols used by the fourteen engines.
const (
	FamilyPostgres Family = "postgres"
	FamilyMysql    Family = "mysql"
	FamilySqlite   Family = "sqlite"
	FamilyRedis    Family = "redis"
	FamilyMongo    Family = "mongo"
)

// Capabilities lists the operations a server supports.
type Capabilities struct {
	PlansStatement bool
	MeasuresPlan   bool
	// Most servers report a syntax error if you ask for the plan of a DROP.
	PlansEveryStatement bool
	HasServerSessions   bool
	// True if the server reports which of its sessions wait for a lock another one holds.
	ReportsLockWaits bool
	// True if the server reports the load it is under.
	ReportsServerLoad bool
	// True if the server keeps a count of the statements it has run. No engine sets this:
	// the connection answers it once it is open.
	ReportsStatementStats bool
	// A cancel needs a second connection to the same server.
	CancelsRunningQuery bool
	HasTransactions     bool
	// A key store returns a scan in its own key order only.
	SortsRead      bool
	TruncatesTable bool
	WritesDDL      bool
	// True if a connection can be opened read-only. TiDB cannot: it accepts the
	// statement but does not apply it.
	TakesReadOnlyMode bool
	// True if the staged changes of the grid are applied all together or not at all.
	// This is not the same as HasTransactions: Redis has no transaction the user
	// controls but still applies a staged set inside a MULTI, and a standalone MongoDB
	// has neither.
	AppliesChangesTogether bool
}

// EngineInfo holds the properties of a server that are known before a connection
// exists. The query tier adds the dialect and the language.
type EngineInfo struct {
	Engine        Engine
	Family        Family
	Capabilities  Capabilities
	DefaultPort   int
	OpensFile     bool
	NeedsUser     bool
	NeedsPassword bool
	// True if the password belongs to the server and not to a named user, so a profile
	// without a user can still have a password. Redis is the only one: `requirepass`
	// has no user. Every other server checks the password against a user, so a profile
	// without a user cannot use a password.
	PasswordWithoutUser bool
	URLSchemes          []string
	DefaultSSLMode      SSLMode
	// The full names of the schemas this server reserves for itself.
	SystemSchemas []string
	// The name prefixes of the schemas this server creates for itself.
	SystemSchemaPrefixes []string
}

// The catalog schemas of every PostgreSQL-protocol server.
var postgresCatalogSchemas = []string{"pg_catalog", "information_schema"}

// The name prefixes of the schemas a PostgreSQL server creates for itself.
var postgresOwnPrefixes = []string{"pg_toast", "pg_temp"}

// The databases every MySQL-protocol server reserves for itself.
var mysqlSystemSchemas = []string{"mysql", "information_schema", "performance_schema", "sys"}

var postgresCapabilities = Capabilities{
	PlansStatement:      true,
	MeasuresPlan:        true,
	PlansEveryStatement: false,
	HasServerSessions:   true,
	ReportsLockWaits:    true,
	ReportsServerLoad:   true,
	CancelsRunningQuery: true,
	HasTransactions:     true,
	SortsRead:           true,
	TruncatesTable:      true,
	WritesDDL:           true,
	TakesReadOnlyMode:   true,

	AppliesChangesTogether: true,
}

var mysqlCapabilities = withPostgres(func(capabilities *Capabilities) {
	// Which session waits for a lock is in performance_schema, which this client does not
	// read yet.
	capabilities.ReportsLockWaits = false
})

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
			// The server also plans a schema change: `explain drop table` returns a plan.
			capabilities.PlansEveryStatement = true
			// The server identifies a session with its own string, not the number in
			// `pg_stat_activity`, and it has no `pg_cancel_backend`.
			capabilities.HasServerSessions = false
			capabilities.CancelsRunningQuery = false
			capabilities.ReportsLockWaits = false
			capabilities.ReportsServerLoad = false
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
		Capabilities: withPostgres(func(capabilities *Capabilities) {
			// EXPLAIN only estimates. There is no ANALYZE that measures the query.
			capabilities.MeasuresPlan = false
			// The cluster reports its locks and its load through its own `stv_` tables.
			capabilities.ReportsLockWaits = false
			capabilities.ReportsServerLoad = false
		}),
		DefaultPort: 5439, NeedsUser: true, NeedsPassword: true,
		URLSchemes: []string{"redshift"},
		// The cluster accepts a TLS connection only.
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
		Capabilities: withMysql(func(capabilities *Capabilities) {
			// The server accepts `set session transaction read only` but does not apply
			// it.
			capabilities.TakesReadOnlyMode = false
			// The status variables of the server are its own, not the ones MySQL reports.
			capabilities.ReportsServerLoad = false
		}),
		DefaultPort: 4000, NeedsUser: true, NeedsPassword: true,
		SystemSchemas: append(append([]string{}, mysqlSystemSchemas...), "metrics_schema"),
	},
	EnginePlanetscale: {
		Engine: EnginePlanetscale, Family: FamilyMysql,
		Capabilities: withMysql(func(capabilities *Capabilities) {
			capabilities.HasServerSessions = false
			capabilities.CancelsRunningQuery = false
			capabilities.ReportsServerLoad = false
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
			// SQLite plans a statement, but it does not measure the run.
			PlansStatement: true,
			MeasuresPlan:   false,
			// A file has only the session that opened it, and the driver reads it in
			// the thread that draws the screen.
			HasServerSessions:   false,
			CancelsRunningQuery: false,
			HasTransactions:     true,
			SortsRead:           true,
			// SQLite empties a table with a delete of every row.
			TruncatesTable:         false,
			WritesDDL:              true,
			TakesReadOnlyMode:      true,
			AppliesChangesTogether: true,
		},
		// A file is opened locally, so there is no port and no URL scheme.
		DefaultPort: 0, OpensFile: true,
	},
	EngineMongo: {
		Engine: EngineMongo, Family: FamilyMongo,
		Capabilities: Capabilities{
			// The server explains a find and an aggregate, and it can measure both.
			PlansStatement: true,
			MeasuresPlan:   true,
			// The server does not explain a write command.
			PlansEveryStatement: false,
			// currentOp lists every running operation, and killOp stops one.
			HasServerSessions: true,
			// The driver cancels through the context. A second connection cannot find
			// the operation id of the call it would stop.
			CancelsRunningQuery: false,
			// A replica set and a sharded cluster support a transaction. A standalone
			// server does not, and the session reports what the connected deployment
			// supports.
			HasTransactions: true,
			// A find accepts a sort, so the server sorts the page.
			SortsRead: true,
			// A collection is emptied with a delete of every document. There is no
			// separate command.
			TruncatesTable: false,
			// Every statement is a command, so the object menu has no SQL to generate.
			WritesDDL: false,
			// The server has no read-only session, so this client blocks the write.
			TakesReadOnlyMode: true,
			// A replica set applies a staged set inside a transaction. A standalone
			// server cannot, and the session reports this after it connects.
			AppliesChangesTogether: true,
		},
		// A user name enables authentication. A server with authentication off refuses
		// a connection that sends one, so the profile can omit the user. A profile that
		// does name a user asks for a password like any other server.
		NeedsPassword: true,
		DefaultPort:   27017,
		URLSchemes:    []string{"mongodb"},
		SystemSchemas: []string{"admin", "config", "local"},
	},
	EngineRedis: {
		Engine: EngineRedis, Family: FamilyRedis,
		Capabilities: Capabilities{
			// A command states the operation, so the server makes no plan.
			PlansStatement: false,
			MeasuresPlan:   false,
			// CLIENT LIST lists every connection, and CLIENT KILL stops one.
			HasServerSessions: true,
			// Redis runs one command at a time and completes it before the next one.
			CancelsRunningQuery: false,
			// MULTI queues the commands and returns no result before EXEC.
			HasTransactions: false,
			// A SCAN returns the keys in the order of the server only.
			SortsRead: false,
			// A prefix is only a name pattern, so no command deletes it.
			TruncatesTable: false,
			WritesDDL:      false,
			// The server has no read-only session, so this client blocks the write.
			TakesReadOnlyMode: true,
			// MULTI is not a transaction the user controls, but it does apply a staged
			// set as one unit.
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

// ResolveEngineInfo returns the properties of that engine. An unknown name gives the
// properties of the default engine, so a caller never reads a port of zero. The name is
// validated when the profile is built, and an error is reported there.
func ResolveEngineInfo(engine Engine) EngineInfo {
	info, known := engineRegistry[engine]
	if !known {
		return engineRegistry[DefaultEngine]
	}
	return info
}

// ListEngineInfo returns the properties of every engine, in the order of Engines.
func ListEngineInfo() []EngineInfo {
	listed := make([]EngineInfo, 0, len(Engines))
	for _, engine := range Engines {
		listed = append(listed, engineRegistry[engine])
	}
	return listed
}

// FindEngine parses the text as an engine name.
func FindEngine(written string) (Engine, bool) {
	return FindAllowed(Engines, strings.ToLower(strings.TrimSpace(written)))
}

// HoldsSystemSchema is true if the server reserves that schema for itself.
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

// OpensFile is true for an engine that opens a local file. It has no host, port or user.
func OpensFile(engine Engine) bool {
	return ResolveEngineInfo(engine).OpensFile
}

// NeedsUser is true if the engine connects as a named user.
func NeedsUser(engine Engine) bool {
	return ResolveEngineInfo(engine).NeedsUser
}

// ResolveDefaultPort returns the default port of the engine.
func ResolveDefaultPort(engine Engine) int {
	return ResolveEngineInfo(engine).DefaultPort
}
