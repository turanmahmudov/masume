package postgres

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// The catalog reads of every PostgreSQL-protocol server. The engine entry names the
// schemas to leave out, so one statement serves every server of the family.

// buildSystemSchemaFilter writes the catalog schemas of the engine entry as a list to
// exclude.
func buildSystemSchemaFilter(column string) string {
	named := make([]string, 0, 2)
	for _, schema := range []string{"pg_catalog", "information_schema"} {
		named = append(named, "'"+schema+"'")
	}
	return column + " not in (" + strings.Join(named, ", ") + ")"
}

// buildOwnSchemaFilter writes the schemas the server creates for itself, which have a
// known prefix.
func buildOwnSchemaFilter(column string) string {
	written := make([]string, 0, 2)
	for _, prefix := range []string{"pg_toast", "pg_temp"} {
		written = append(written, column+" not like '"+prefix+"%'")
	}
	return strings.Join(written, "\n     and ")
}

var listTablesSQL = `
  select n.nspname as schema,
         c.relname  as name,
         c.relkind  as kind,
         c.reltuples::int8 as estimated_rows
    from pg_class c
    join pg_namespace n on n.oid = c.relnamespace
   where c.relkind in ('r', 'p', 'v', 'm')
     and ` + buildSystemSchemaFilter("n.nspname") + `
     and ` + buildOwnSchemaFilter("n.nspname") + `
   order by n.nspname, c.relname
`

const describeColumnsSQL = `
  select a.attname                                      as name,
         format_type(a.atttypid, a.atttypmod)           as data_type,
         not a.attnotnull                               as nullable,
         pg_get_expr(d.adbin, d.adrelid)                as default_value,
         coalesce(pk.is_primary_key, false)             as is_primary_key,
         a.attgenerated <> ''                           as is_generated,
         coalesce((select array_agg(e.enumlabel order by e.enumsortorder)
                     from pg_enum e
                    where e.enumtypid = a.atttypid), '{}')  as choices
    from pg_attribute a
    left join pg_attrdef d
      on d.adrelid = a.attrelid and d.adnum = a.attnum
    left join lateral (
      select true as is_primary_key
        from pg_constraint c
       where c.conrelid = a.attrelid
         and c.contype = 'p'
         and a.attnum = any (c.conkey)
    ) pk on true
   where a.attrelid = $1::regclass
     and a.attnum > 0
     and not a.attisdropped
   order by a.attnum
`

// buildKeyColumnsSQL joins the attribute numbers of a constraint back to names, which
// is how `pg_constraint` holds its key columns.
func buildKeyColumnsSQL(keyColumn, relationColumn string) string {
	return `(select array_agg(att.attname order by k.ord)
            from unnest(c.` + keyColumn + `) with ordinality k(attnum, ord)
            join pg_attribute att
              on att.attrelid = c.` + relationColumn + ` and att.attnum = k.attnum)`
}

var (
	sourceKeyColumnsSQL = buildKeyColumnsSQL("conkey", "conrelid")
	targetKeyColumnsSQL = buildKeyColumnsSQL("confkey", "confrelid")
)

// deleteRuleSQL writes the rule of a foreign key as text, out of the letter the catalog
// holds for it.
const deleteRuleSQL = `case c.confdeltype
           when 'c' then 'cascade' when 'n' then 'set null' when 'd' then 'set default'
           when 'r' then 'restrict' else 'no action' end`

var describeForeignKeysSQL = `
  select c.conname as name,
         ` + sourceKeyColumnsSQL + ` as columns,
         tn.nspname as target_schema,
         tc.relname as target_table,
         ` + targetKeyColumnsSQL + ` as target_columns,
         ` + deleteRuleSQL + ` as delete_rule
    from pg_constraint c
    join pg_class tc     on tc.oid = c.confrelid
    join pg_namespace tn on tn.oid = tc.relnamespace
   where c.conrelid = $1::regclass
     and c.contype = 'f'
   order by c.conname
`

var listRelationshipsSQL = `
  select c.conname as name,
         sn.nspname as schema,
         sc.relname as table,
         ` + sourceKeyColumnsSQL + ` as columns,
         tn.nspname as target_schema,
         tc.relname as target_table,
         ` + targetKeyColumnsSQL + ` as target_columns,
         ` + deleteRuleSQL + ` as delete_rule
    from pg_constraint c
    join pg_class sc     on sc.oid = c.conrelid
    join pg_namespace sn on sn.oid = sc.relnamespace
    join pg_class tc     on tc.oid = c.confrelid
    join pg_namespace tn on tn.oid = tc.relnamespace
   where c.contype = 'f'
     and ` + buildSystemSchemaFilter("sn.nspname") + `
   order by sn.nspname, sc.relname, c.conname
`

const listRolesSQL = `
  select r.rolname as name,
         concat_ws(', ',
           case when r.rolsuper then 'superuser' end,
           case when r.rolcreatedb then 'createdb' end,
           case when r.rolcreaterole then 'createrole' end,
           case when r.rolreplication then 'replication' end,
           case when not r.rolcanlogin then 'nologin' end) as detail
    from pg_roles r
   where r.rolname not like 'pg\_%'
   order by r.rolname
`

// triggerEventsSQL writes the writes a trigger runs for, out of the bits the catalog holds
// for them. A trigger can name more than one.
const triggerEventsSQL = `concat_ws(', ',
           case when tg.tgtype & 4 > 0 then 'insert' end,
           case when tg.tgtype & 8 > 0 then 'delete' end,
           case when tg.tgtype & 16 > 0 then 'update' end,
           case when tg.tgtype & 32 > 0 then 'truncate' end)`

var listSchemaObjectsSQL = `
  select n.nspname as schema,
         p.proname  as name,
         'function' as kind,
         pg_get_function_result(p.oid) as detail,
         p.oid::text as identity,
         '' as events
    from pg_proc p
    join pg_namespace n on n.oid = p.pronamespace
   where ` + buildSystemSchemaFilter("n.nspname") + `
     and p.prokind in ('f', 'p')
  union all
  select n.nspname, c.relname, 'sequence', '', c.oid::text, ''
    from pg_class c
    join pg_namespace n on n.oid = c.relnamespace
   where c.relkind = 'S'
     and ` + buildSystemSchemaFilter("n.nspname") + `
  union all
  select n.nspname, t.typname, 'type',
         case t.typtype when 'e' then 'enum' when 'c' then 'composite' when 'd' then 'domain'
              else 'type' end,
         t.oid::text,
         ''
    from pg_type t
    join pg_namespace n on n.oid = t.typnamespace
   where ` + buildSystemSchemaFilter("n.nspname") + `
     and t.typtype in ('e', 'c', 'd')
     and not exists (select 1 from pg_class c where c.oid = t.typrelid and c.relkind <> 'c')
  union all
  select n.nspname, tg.tgname, 'trigger', c.relname, tg.oid::text,
         ` + triggerEventsSQL + `
    from pg_trigger tg
    join pg_class c     on c.oid = tg.tgrelid
    join pg_namespace n on n.oid = c.relnamespace
   where not tg.tgisinternal
     and ` + buildSystemSchemaFilter("n.nspname") + `
  order by 1, 3, 2
`

const listIndexesSQL = `
  select i.relname          as name,
         idx.indisunique    as is_unique,
         idx.indisprimary   as is_primary,
         pg_get_indexdef(idx.indexrelid) as definition
    from pg_index idx
    join pg_class i on i.oid = idx.indexrelid
   where idx.indrelid = $1::regclass
   order by idx.indisprimary desc, i.relname
`

const listConstraintsSQL = `
  select c.conname as name,
         c.contype as type,
         pg_get_constraintdef(c.oid) as definition
    from pg_constraint c
   where c.conrelid = $1::regclass
   order by c.contype, c.conname
`

const listActivitySQL = `
  select /*masume:dashboard*/ pid,
         coalesce(usename, '')           as usename,
         coalesce(application_name, '')  as application_name,
         coalesce(host(client_addr), '') as client_addr,
         coalesce(state, '')             as state,
         coalesce((extract(epoch from (now() - query_start)) * 1000)::int8, 0) as duration_ms,
         coalesce(query, '')             as query
    from pg_stat_activity
   where datname = current_database()
     and pid <> pg_backend_pid()
   order by state = 'active' desc, query_start nulls last
`

// The sessions that wait for a lock, and the session that holds the one they wait for.
// pg_blocking_pids answers the holders of one waiter without joining pg_locks to itself.
// The mode reported is the one the holder was granted on the relation the waiter asked for.
// A lock that is not on a relation leaves both the mode and the relation empty.
const listLockWaitsSQL = `
  select /*masume:dashboard*/ waiting.pid                 as blocked_pid,
         coalesce(waiting.query, '') as blocked_query,
         coalesce((extract(epoch from (now() - waiting.state_change)) * 1000)::int8, 0)
                                     as waiting_ms,
         coalesce(held.mode, '')     as mode,
         coalesce(held.relname, '')  as relation,
         holder.pid                  as blocking_pid,
         coalesce(holder.query, '')  as blocking_query,
         coalesce((extract(epoch from (now() - holder.query_start)) * 1000)::int8, 0)
                                     as blocking_ms
    from pg_stat_activity waiting
    cross join lateral unnest(pg_blocking_pids(waiting.pid)) as blocker(pid)
    join pg_stat_activity holder on holder.pid = blocker.pid
    left join lateral (
           select granted_lock.mode, relation.relname
             from pg_locks asked
             join pg_locks granted_lock
               on granted_lock.pid = holder.pid
              and granted_lock.granted
              and granted_lock.relation is not distinct from asked.relation
             left join pg_class relation on relation.oid = asked.relation
            where asked.pid = waiting.pid
              and not asked.granted
            order by asked.relation nulls last
            limit 1
         ) held on true
   where waiting.wait_event_type = 'Lock'
     and waiting.datname = current_database()
   order by waiting.state_change, waiting.pid, holder.pid
`

// The load the server itself is carrying: how many connections it holds, how many it allows,
// and when it started. The count covers every database of the server.
const readServerLoadSQL = `
  select /*masume:dashboard*/ (select count(*) from pg_stat_activity)  as connections,
         current_setting('max_connections')::int8 as max_connections,
         pg_postmaster_start_time()               as started_at,
         (select sum(xact_commit + xact_rollback)::int8 from pg_stat_database) as transactions,
         (select sum(temp_files)::int8 from pg_stat_database)            as temp_files,
         (select sum(blks_hit)::int8   from pg_stat_database)            as blocks_hit,
         (select sum(blks_read)::int8  from pg_stat_database)            as blocks_read,
         -- A standby cannot be asked where its own write ahead log stands, so the count
         -- is read on the server that writes one.
         case when pg_is_in_recovery() then null
              else pg_wal_lsn_diff(pg_current_wal_lsn(), '0/0')::int8
         end as wal_bytes,
         -- A standby reports how far behind the server that feeds it is. Any other server
         -- reports the worst of the standbys it feeds, and nothing where it feeds none.
         case when pg_is_in_recovery()
              then extract(epoch from (now() - pg_last_xact_replay_timestamp()))::float8
              else (select extract(epoch from max(replay_lag))::float8 from pg_stat_replication)
         end as replication_lag_s
`

// DashboardMark is written into every statement the dashboard runs, so the panel of slow
// statements can leave its own reads out of what it draws.
//
// The server counts a statement by the shape of its parse tree, and a comment is not part of
// that shape. A reader who runs the same statement byte for byte apart from the mark shares
// a row with it.
const DashboardMark = "masume:dashboard"

// The statements the server spent the most time in, the slowest by mean first. The count
// belongs to the whole server, so it is narrowed to this database.
const listSlowStatementsSQL = `
  select /*masume:dashboard*/ query,
         calls,
         mean_exec_time::float8  as mean_ms,
         total_exec_time::float8 as total_ms,
         rows                    as rows_returned
    from pg_stat_statements
   where dbid = (select oid from pg_database where datname = current_database())
     and calls > 0
     -- The dashboard reads the server every two seconds, so its own reads would fill the
     -- panel it draws. Every one of them carries the mark below and nothing else does,
     -- so a reader's own statement about the statistics is still counted.
     and query not like '%' || $2 || '%' 
   order by mean_exec_time desc
   limit $1
`

// The statements that print the definition of each kind of object Postgres describes
// itself.
var postgresObjectDDL = map[db.SchemaObjectKind]string{
	db.ObjectFunction: "select pg_get_functiondef($1::oid) as ddl",
	db.ObjectTrigger:  "select pg_get_triggerdef($1::oid) as ddl",
	db.ObjectSequence: "select format('create sequence %s.%s start %s increment %s;', " +
		"quote_ident(schemaname), quote_ident(sequencename), coalesce(start_value, 1), " +
		"increment_by) as ddl from pg_sequences where schemaname || '.' || sequencename = " +
		"(select n.nspname || '.' || c.relname from pg_class c " +
		"join pg_namespace n on n.oid = c.relnamespace where c.oid = $1::oid)",
	db.ObjectType: "select format('create type %s.%s as enum (%s);', quote_ident(n.nspname), " +
		"quote_ident(t.typname), string_agg(quote_literal(e.enumlabel), ', ' " +
		"order by e.enumsortorder)) as ddl from pg_type t " +
		"join pg_namespace n on n.oid = t.typnamespace " +
		"left join pg_enum e on e.enumtypid = t.oid where t.oid = $1::oid " +
		"group by n.nspname, t.typname",
}
