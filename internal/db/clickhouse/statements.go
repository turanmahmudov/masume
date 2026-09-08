package clickhouse

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// systemSchemaList is the set of databases the server holds for itself, as a statement
// reads it. The standard views stand in one database under two names.
var systemSchemaList = func() string {
	named := []string{"'INFORMATION_SCHEMA'"}
	for _, schema := range core.ResolveEngineInfo(core.EngineClickhouse).SystemSchemas {
		named = append(named, "'"+schema+"'")
	}
	return "(" + strings.Join(named, ", ") + ")"
}()

// A materialized view keeps its rows in a table of its own, which the server names with a
// leading dot. That table belongs to the view and not to the user.
var listTablesSQL = `
  select database as ` + "`schema`" + `,
         name as name,
         engine as engine,
         coalesce(total_rows, 0) as estimated_rows
    from system.tables
   where database not in ` + systemSchemaList + `
     and name not like '.inner%'
   order by database, name
`

const describeColumnsSQL = `
  select name as name,
         type as data_type,
         default_kind as default_kind,
         default_expression as default_value,
         is_in_primary_key as is_primary_key
    from system.columns
   where database = ? and table = ?
   order by position
`

const listIndexesSQL = `
  select name as name,
         type_full as type_full,
         expr as expr,
         granularity as granularity
    from system.data_skipping_indices
   where database = ? and table = ?
   order by name
`

// The sorting key of a table, which the server keeps the rows of a table in.
const readSortingKeySQL = `
  select sorting_key as sorting_key,
         primary_key as primary_key
    from system.tables
   where database = ? and name = ?
`

var listFunctionsSQL = `
  select name as name,
         create_query as create_query
    from system.functions
   where origin != 'System'
   order by name
`

const listRolesSQL = `
  select name as name, 'user' as detail from system.users
   union all
  select name as name, 'role' as detail from system.roles
   order by name
`

// The definition of a relation, as the statement that made it.
const showCreateSQL = `show create table `

// A running statement of the server, with the client that sent it. The server names each
// one with a text id.
const listActivitySQL = `
  select query_id as query_id,
         user as user,
         concat(client_name, if(client_name = '', '', ' ')) as client,
         toString(address) as address,
         if(is_cancelled, 'cancelled', 'active') as state,
         toUInt64(elapsed * 1000) as duration_ms,
         query as query
    from system.processes
   where query_id != queryID()
   order by elapsed desc
`

const readServerLoadSQL = `
  select (select value from system.metrics where metric = 'TCPConnection') as connections,
         (select toUInt64OrZero(value) from system.server_settings
           where name = 'max_connections') as max_connections,
         toUInt64(uptime()) as uptime_seconds
`

// The statements the server logged, the slowest by mean time first. One row stands for
// every run of one shape of statement.
const listSlowStatementsSQL = `
  select any(query) as query,
         count() as calls,
         toUInt64(avg(query_duration_ms)) as mean_ms,
         toUInt64(sum(query_duration_ms)) as total_ms,
         toUInt64(sum(read_rows)) as rows_read
    from system.query_log
   where type = 'QueryFinish'
   group by normalized_query_hash
   order by mean_ms desc
   limit ?
`

// killQuerySQL stops one statement of the server, and waits for it to end.
const killQuerySQL = `kill query where query_id = ? sync`
