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

var describeForeignKeysSQL = `
  select c.conname as name,
         ` + sourceKeyColumnsSQL + ` as columns,
         tn.nspname as target_schema,
         tc.relname as target_table,
         ` + targetKeyColumnsSQL + ` as target_columns
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
         ` + targetKeyColumnsSQL + ` as target_columns
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

var listSchemaObjectsSQL = `
  select n.nspname as schema,
         p.proname  as name,
         'function' as kind,
         pg_get_function_result(p.oid) as detail,
         p.oid::text as identity
    from pg_proc p
    join pg_namespace n on n.oid = p.pronamespace
   where ` + buildSystemSchemaFilter("n.nspname") + `
     and p.prokind in ('f', 'p')
  union all
  select n.nspname, c.relname, 'sequence', '', c.oid::text
    from pg_class c
    join pg_namespace n on n.oid = c.relnamespace
   where c.relkind = 'S'
     and ` + buildSystemSchemaFilter("n.nspname") + `
  union all
  select n.nspname, t.typname, 'type',
         case t.typtype when 'e' then 'enum' when 'c' then 'composite' when 'd' then 'domain'
              else 'type' end,
         t.oid::text
    from pg_type t
    join pg_namespace n on n.oid = t.typnamespace
   where ` + buildSystemSchemaFilter("n.nspname") + `
     and t.typtype in ('e', 'c', 'd')
     and not exists (select 1 from pg_class c where c.oid = t.typrelid and c.relkind <> 'c')
  union all
  select n.nspname, tg.tgname, 'trigger', c.relname, tg.oid::text
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
  select pid,
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
