package postgres

// Flavour holds the parts each PostgreSQL-protocol server does differently.
// They share the wire, the catalog and the SQL, so they share one adapter. The rest is
// here, one entry per server.
type Flavour struct {
	// PostgreSQL takes options in brackets, CockroachDB takes none, and Redshift
	// measures nothing.
	BuildExplainPrefix func(analyze bool) string
	// The statement that opens a session where the server refuses every write.
	ReadOnlyStatement string
	// The function that stops another session. The name differs per server.
	BuildCancelFunction func(terminate bool) string
}

const postgresReadOnlyStatement = "set default_transaction_read_only = on"

func buildPostgresCancelFunction(terminate bool) string {
	if terminate {
		return "pg_terminate_backend"
	}
	return "pg_cancel_backend"
}

// FlavourStandard is PostgreSQL itself, which the other flavours differ from.
var FlavourStandard = Flavour{
	BuildExplainPrefix: func(analyze bool) string {
		if analyze {
			return "explain (ANALYZE, BUFFERS, COSTS)"
		}
		return "explain (COSTS)"
	},
	ReadOnlyStatement:   postgresReadOnlyStatement,
	BuildCancelFunction: buildPostgresCancelFunction,
}

// FlavourCockroach writes its own plan, and takes no options in brackets.
var FlavourCockroach = Flavour{
	BuildExplainPrefix: func(analyze bool) string {
		if analyze {
			return "explain analyze"
		}
		return "explain"
	},
	ReadOnlyStatement:   postgresReadOnlyStatement,
	BuildCancelFunction: buildPostgresCancelFunction,
}

// FlavourRedshift plans without measuring, so it is only asked for the estimate.
var FlavourRedshift = Flavour{
	BuildExplainPrefix:  func(bool) string { return "explain" },
	ReadOnlyStatement:   postgresReadOnlyStatement,
	BuildCancelFunction: buildPostgresCancelFunction,
}

// MysqlFlavour holds the parts each MySQL-protocol server does differently. They share
