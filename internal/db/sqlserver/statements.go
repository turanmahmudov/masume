package sqlserver

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// systemSchemaList is the set of schemas the server holds for itself, as a statement reads it.
var systemSchemaList = func() string {
	named := make([]string, 0, len(core.ResolveEngineInfo(core.EngineSqlserver).SystemSchemas))
	for _, schema := range core.ResolveEngineInfo(core.EngineSqlserver).SystemSchemas {
		named = append(named, "'"+schema+"'")
	}
	return "(" + strings.Join(named, ", ") + ")"
}()

// The schemas of the connected database, including a schema that holds nothing.
var listSchemasSQL = `
  select s.name as [name]
    from sys.schemas s
   where lower(s.name) not in ` + systemSchemaList + `
   order by s.name
`

// The type of a column, written the way a statement of the user writes it.
const columnTypeExpression = `
         t.name + case
           when t.name in ('varchar', 'char', 'varbinary', 'binary')
             then '(' + case when c.max_length = -1 then 'max'
                             else cast(c.max_length as varchar(11)) end + ')'
           when t.name in ('nvarchar', 'nchar')
             then '(' + case when c.max_length = -1 then 'max'
                             else cast(c.max_length / 2 as varchar(11)) end + ')'
           when t.name in ('decimal', 'numeric')
             then '(' + cast(c.precision as varchar(11)) + ',' +
                  cast(c.scale as varchar(11)) + ')'
           when t.name in ('datetime2', 'datetimeoffset', 'time') and c.scale <> 7
             then '(' + cast(c.scale as varchar(11)) + ')'
           else '' end`

var listTablesSQL = `
  select s.name as [schema],
         o.name as [name],
         o.type as kind,
         coalesce((select sum(p.rows) from sys.partitions p
                    where p.object_id = o.object_id and p.index_id in (0, 1)), 0)
           as estimated_rows
    from sys.objects o
    join sys.schemas s on s.schema_id = o.schema_id
   where o.type in ('U', 'V')
     and o.is_ms_shipped = 0
     and lower(s.name) not in ` + systemSchemaList + `
   order by s.name, o.name
`

var describeColumnsSQL = `
  select c.name as [name],` + columnTypeExpression + ` as data_type,
         c.is_nullable as nullable,
         d.definition as default_value,
         case when k.column_id is null then 0 else 1 end as is_primary_key,
         case when c.is_computed = 1 or c.is_identity = 1 or t.name = 'timestamp'
              then 1 else 0 end as is_generated
    from sys.columns c
    join sys.types t on t.user_type_id = c.user_type_id
    left join sys.default_constraints d on d.object_id = c.default_object_id
    left join (select ic.column_id
                 from sys.indexes i
                 join sys.index_columns ic
                   on ic.object_id = i.object_id and ic.index_id = i.index_id
                where i.is_primary_key = 1 and i.object_id = object_id(@p1)) k
      on k.column_id = c.column_id
   where c.object_id = object_id(@p1)
   order by c.column_id
`

// The columns of a foreign key, read as one text of the names in key order.
const foreignKeyColumnsExpression = `
         (select string_agg(pc.name, ',')
                   within group (order by fkc.constraint_column_id)
            from sys.foreign_key_columns fkc
            join sys.columns pc on pc.object_id = fkc.parent_object_id
                               and pc.column_id = fkc.parent_column_id
           where fkc.constraint_object_id = fk.object_id) as columns,
         schema_name(rt.schema_id) as target_schema,
         rt.name as target_table,
         (select string_agg(rc.name, ',')
                   within group (order by fkc.constraint_column_id)
            from sys.foreign_key_columns fkc
            join sys.columns rc on rc.object_id = fkc.referenced_object_id
                               and rc.column_id = fkc.referenced_column_id
           where fkc.constraint_object_id = fk.object_id) as target_columns,
         lower(replace(fk.delete_referential_action_desc, '_', ' ')) as delete_rule`

const describeForeignKeysSQL = `
  select fk.name as [name],` + foreignKeyColumnsExpression + `
    from sys.foreign_keys fk
    join sys.tables rt on rt.object_id = fk.referenced_object_id
   where fk.parent_object_id = object_id(@p1)
   order by fk.name
`

var listRelationshipsSQL = `
  select fk.name as [name],
         schema_name(pt.schema_id) as [schema],
         pt.name as [table],` + foreignKeyColumnsExpression + `
    from sys.foreign_keys fk
    join sys.tables pt on pt.object_id = fk.parent_object_id
    join sys.tables rt on rt.object_id = fk.referenced_object_id
   where lower(schema_name(pt.schema_id)) not in ` + systemSchemaList + `
   order by schema_name(pt.schema_id), pt.name, fk.name
`

const listIndexesSQL = `
  select i.name as [name],
         i.is_unique as is_unique,
         i.is_primary_key as is_primary,
         string_agg(c.name, ',') within group (order by ic.key_ordinal) as columns
    from sys.indexes i
    join sys.index_columns ic
      on ic.object_id = i.object_id and ic.index_id = i.index_id
    join sys.columns c on c.object_id = ic.object_id and c.column_id = ic.column_id
   where i.object_id = object_id(@p1)
     and i.type <> 0
     and ic.is_included_column = 0
   group by i.name, i.is_unique, i.is_primary_key, i.index_id
   order by i.is_primary_key desc, i.name
`

// A key constraint keeps its columns in the index it is built on. A check constraint keeps
// its own text, which the server writes back as it stored it.
const listConstraintsSQL = `
  select k.name as [name],
         case k.type when 'PK' then 'primary key' else 'unique' end as [type],
         null as check_clause,
         (select string_agg(c.name, ', ') within group (order by ic.key_ordinal)
            from sys.index_columns ic
            join sys.columns c on c.object_id = ic.object_id and c.column_id = ic.column_id
           where ic.object_id = k.parent_object_id
             and ic.index_id = k.unique_index_id
             and ic.is_included_column = 0) as columns,
         null as target_table,
         null as target_columns
    from sys.key_constraints k
   where k.parent_object_id = object_id(@p1)
  union all
  select fk.name, 'foreign key', null,
         (select string_agg(pc.name, ', ')
                   within group (order by fkc.constraint_column_id)
            from sys.foreign_key_columns fkc
            join sys.columns pc on pc.object_id = fkc.parent_object_id
                               and pc.column_id = fkc.parent_column_id
           where fkc.constraint_object_id = fk.object_id),
         schema_name(rt.schema_id) + '.' + rt.name,
         (select string_agg(rc.name, ', ')
                   within group (order by fkc.constraint_column_id)
            from sys.foreign_key_columns fkc
            join sys.columns rc on rc.object_id = fkc.referenced_object_id
                               and rc.column_id = fkc.referenced_column_id
           where fkc.constraint_object_id = fk.object_id)
    from sys.foreign_keys fk
    join sys.tables rt on rt.object_id = fk.referenced_object_id
   where fk.parent_object_id = object_id(@p1)
  union all
  select cc.name, 'check', 'check ' + cc.definition, null, null, null
    from sys.check_constraints cc
   where cc.parent_object_id = object_id(@p1)
`

// A routine of the user, with the type it returns. Parameter zero is the return value.
var listRoutinesSQL = `
  select schema_name(o.schema_id) as [schema],
         o.name as [name],
         case when o.type in ('P', 'PC') then 'procedure' else 'function' end as routine_kind,
         coalesce(type_name(r.user_type_id), '') as detail
    from sys.objects o
    left join sys.parameters r
      on r.object_id = o.object_id and r.parameter_id = 0
   where o.type in ('FN', 'IF', 'TF', 'FS', 'FT', 'P', 'PC')
     and o.is_ms_shipped = 0
     and lower(schema_name(o.schema_id)) not in ` + systemSchemaList + `
   order by schema_name(o.schema_id), o.name
`

var listSequencesSQL = `
  select schema_name(schema_id) as [schema],
         name as [name],
         coalesce(type_name(user_type_id), '') as detail
    from sys.sequences
   where lower(schema_name(schema_id)) not in ` + systemSchemaList + `
   order by schema_name(schema_id), name
`

var listTypesSQL = `
  select schema_name(schema_id) as [schema],
         name as [name],
         coalesce(type_name(system_type_id), '') as detail
    from sys.types
   where is_user_defined = 1
     and lower(schema_name(schema_id)) not in ` + systemSchemaList + `
   order by schema_name(schema_id), name
`

var listTriggersSQL = `
  select schema_name(t.schema_id) as [schema],
         tr.name as [name],
         t.name as detail,
         (select string_agg(lower(replace(te.type_desc, '_', ' ')), ', ')
            from sys.trigger_events te
           where te.object_id = tr.object_id) as events
    from sys.triggers tr
    join sys.tables t on t.object_id = tr.parent_id
   where tr.is_ms_shipped = 0
     and lower(schema_name(t.schema_id)) not in ` + systemSchemaList + `
   order by schema_name(t.schema_id), tr.name
`

const listRolesSQL = `
  select name as [name],
         lower(replace(type_desc, '_', ' ')) as detail
    from sys.database_principals
   where is_fixed_role = 0
     and name not like '##%'
   order by name
`

// The definition of an object, as the server stored the statement that made it.
const readObjectDefinitionSQL = `
  select object_definition(object_id(@p1)) as definition
`

// The statement the server compiles without running it. A fault comes back as a row.
const describeStatementSQL = `
  select error_number, error_message
    from sys.dm_exec_describe_first_result_set(@p1, null, 0)
   where error_number is not null
`

const listActivitySQL = `
  select s.session_id as pid,
         coalesce(s.login_name, '') as [user],
         coalesce(s.program_name, '') as application_name,
         coalesce(c.client_net_address, '') as client_address,
         coalesce(r.status, s.status) as state,
         coalesce(r.total_elapsed_time, 0) as duration_ms,
         coalesce(t.text, '') as query
    from sys.dm_exec_sessions s
    left join sys.dm_exec_connections c on c.session_id = s.session_id
    left join sys.dm_exec_requests r on r.session_id = s.session_id
    outer apply sys.dm_exec_sql_text(r.sql_handle) t
   where s.session_id <> @@spid
     and s.is_user_process = 1
   order by case when r.session_id is null then 1 else 0 end,
            coalesce(r.total_elapsed_time, 0) desc
`

// A session that waits for a lock, with the session that holds it. A locked object is
// named where the wait is on one, and named by its kind of resource otherwise.
const listLockWaitsSQL = `
  select r.session_id as blocked_pid,
         coalesce(t.text, '') as blocked_query,
         r.wait_time as waiting_ms,
         coalesce(r.wait_type, '') as mode,
         coalesce(case when l.resource_type = 'OBJECT'
                       then object_name(l.resource_associated_entity_id)
                       else lower(l.resource_type) end, '') as relation,
         r.blocking_session_id as blocking_pid,
         coalesce(bt.text, '') as blocking_query,
         coalesce(br.total_elapsed_time, 0) as blocking_ms
    from sys.dm_exec_requests r
    outer apply sys.dm_exec_sql_text(r.sql_handle) t
    left join sys.dm_exec_requests br on br.session_id = r.blocking_session_id
    outer apply sys.dm_exec_sql_text(br.sql_handle) bt
    outer apply (select top 1 tl.resource_type, tl.resource_associated_entity_id
                   from sys.dm_tran_locks tl
                  where tl.request_session_id = r.session_id
                    and tl.request_status = 'WAIT') l
   where r.blocking_session_id <> 0
   order by r.wait_time desc
`

const readServerLoadSQL = `
  select (select count_big(*) from sys.dm_exec_sessions where is_user_process = 1)
           as connections,
         cast(@@max_connections as bigint) as max_connections,
         (select sqlserver_start_time from sys.dm_os_sys_info) as started_at
`

// The statement text of a counter row covers one statement of the batch it ran in, so the
// offsets of that statement cut it out. An end offset of -1 marks the last statement.
const listSlowStatementsSQL = `
  select top (@p1)
         coalesce(substring(t.text, (qs.statement_start_offset / 2) + 1,
           case when qs.statement_end_offset = -1
                then 4000
                else (qs.statement_end_offset - qs.statement_start_offset) / 2 + 1 end),
           '') as query,
         qs.execution_count as calls,
         qs.total_elapsed_time as total_us,
         qs.total_elapsed_time / qs.execution_count as mean_us,
         qs.total_rows as rows_back
    from sys.dm_exec_query_stats qs
    outer apply sys.dm_exec_sql_text(qs.sql_handle) t
   order by mean_us desc
`
