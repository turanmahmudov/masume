package mysql

import (
	"strings"
)

// The name is lowered before the comparison, because TiDB reports these in capitals
// and compares a schema name byte for byte.
var mysqlSystemSchemaList = func() string {
	named := make([]string, 0, 4)
	for _, schema := range []string{"mysql", "information_schema", "performance_schema", "sys"} {
		named = append(named, "'"+schema+"'")
	}
	return "(" + strings.Join(named, ", ") + ")"
}()

var listMysqlTablesSQL = `
  select table_schema            as ` + "`schema`" + `,
         table_name              as name,
         table_type              as kind,
         coalesce(table_rows, 0) as estimated_rows
    from information_schema.tables
   where lower(table_schema) not in ` + mysqlSystemSchemaList + `
   order by table_schema, table_name
`

const describeMysqlColumnsSQL = `
  select column_name    as name,
         column_type    as data_type,
         is_nullable    as nullable,
         column_default as default_value,
         column_key     as column_key,
         extra          as extra
    from information_schema.columns
   where table_schema = ? and table_name = ?
   order by ordinal_position
`

const describeMysqlForeignKeysSQL = `
  select k.constraint_name as name,
         group_concat(k.column_name order by k.ordinal_position) as columns,
         k.referenced_table_schema as target_schema,
         k.referenced_table_name   as target_table,
         group_concat(k.referenced_column_name order by k.ordinal_position) as target_columns,
         min(r.delete_rule) as delete_rule
    from information_schema.key_column_usage k
    join information_schema.referential_constraints r
      on r.constraint_schema = k.constraint_schema
     and r.constraint_name   = k.constraint_name
     and r.table_name        = k.table_name
   where k.table_schema = ?
     and k.table_name = ?
     and k.referenced_table_name is not null
   group by k.constraint_name, k.referenced_table_schema, k.referenced_table_name
   order by k.constraint_name
`

var listMysqlRelationshipsSQL = `
  select k.constraint_name as name,
         k.table_schema    as ` + "`schema`" + `,
         k.table_name      as ` + "`table`" + `,
         group_concat(k.column_name order by k.ordinal_position) as columns,
         k.referenced_table_schema as target_schema,
         k.referenced_table_name   as target_table,
         group_concat(k.referenced_column_name order by k.ordinal_position) as target_columns,
         min(r.delete_rule) as delete_rule
    from information_schema.key_column_usage k
    join information_schema.referential_constraints r
      on r.constraint_schema = k.constraint_schema
     and r.constraint_name   = k.constraint_name
     and r.table_name        = k.table_name
   where k.referenced_table_name is not null
     and lower(k.table_schema) not in ` + mysqlSystemSchemaList + `
   group by k.table_schema, k.table_name, k.constraint_name,
            k.referenced_table_schema, k.referenced_table_name
   order by k.table_schema, k.table_name, k.constraint_name
`

// MySQL has no roles apart from its users, so the grantees are listed.
const listMysqlRolesSQL = `
  select grantee as name,
         group_concat(distinct privilege_type order by privilege_type separator ', ') as detail
    from information_schema.user_privileges
   group by grantee
   order by grantee
`

var listMysqlRoutinesSQL = `
  select routine_schema              as ` + "`schema`" + `,
         routine_name                as name,
         lower(routine_type)         as routine_type,
         coalesce(dtd_identifier, '') as detail
    from information_schema.routines
   where lower(routine_schema) not in ` + mysqlSystemSchemaList + `
   order by routine_schema, routine_name
`

var listMysqlTriggersSQL = `
  select trigger_schema     as ` + "`schema`" + `,
         trigger_name       as name,
         event_object_table as detail,
         lower(event_manipulation) as events
    from information_schema.triggers
   where lower(trigger_schema) not in ` + mysqlSystemSchemaList + `
   order by trigger_schema, trigger_name
`

const listMysqlIndexesSQL = `
  select index_name as name,
         max(non_unique) as non_unique,
         group_concat(column_name order by seq_in_index) as columns
    from information_schema.statistics
   where table_schema = ? and table_name = ?
   group by index_name
   order by (index_name = 'PRIMARY') desc, index_name
`

const listMysqlConstraintsSQL = `
  select tc.constraint_name as name,
         tc.constraint_type as type,
         cc.check_clause    as check_clause,
         (select group_concat(k.column_name order by k.ordinal_position separator ', ')
            from information_schema.key_column_usage k
           where k.constraint_schema = tc.constraint_schema
             and k.constraint_name = tc.constraint_name
             and k.table_schema = tc.table_schema
             and k.table_name = tc.table_name) as columns,
         (select concat(max(k.referenced_table_schema), '.', max(k.referenced_table_name))
            from information_schema.key_column_usage k
           where k.constraint_schema = tc.constraint_schema
             and k.constraint_name = tc.constraint_name
             and k.table_schema = tc.table_schema
             and k.table_name = tc.table_name
             and k.referenced_table_name is not null) as target_table,
         (select group_concat(k.referenced_column_name order by k.ordinal_position separator ', ')
            from information_schema.key_column_usage k
           where k.constraint_schema = tc.constraint_schema
             and k.constraint_name = tc.constraint_name
             and k.table_schema = tc.table_schema
             and k.table_name = tc.table_name
             and k.referenced_table_name is not null) as target_columns
    from information_schema.table_constraints tc
    left join information_schema.check_constraints cc
      on cc.constraint_schema = tc.constraint_schema
     and cc.constraint_name = tc.constraint_name
   where tc.table_schema = ? and tc.table_name = ?
   order by tc.constraint_type, tc.constraint_name
`

// The load the server itself is carrying. The connection count and the uptime are status
// variables and the limit is a system variable, so one statement reads all three out of
// the tables that hold them rather than through three SHOW commands.
const readMysqlServerLoadSQL = `
  select (select cast(variable_value as unsigned)
            from performance_schema.global_status
           where variable_name = 'Threads_connected')        as connections,
         @@max_connections                                   as max_connections,
         (select cast(variable_value as unsigned)
            from performance_schema.global_status
           where variable_name = 'Uptime')                    as uptime_seconds
`

const listMysqlActivitySQL = `
  select id                        as pid,
         coalesce(user, '')        as user,
         coalesce(db, '')          as db,
         coalesce(host, '')        as host,
         coalesce(command, '')     as command,
         coalesce(state, '')       as state,
         coalesce(time, 0) * 1000  as duration_ms,
         coalesce(info, '')        as query
    from information_schema.processlist
   where id <> connection_id()
   order by command <> 'Sleep' desc, time desc
`
