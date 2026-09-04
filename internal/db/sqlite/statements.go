package sqlite

import (
	_ "modernc.org/sqlite"
)

const listSqliteColumnsSQL = `
  select name, type as data_type, "notnull" as not_null, dflt_value as default_value,
         pk, hidden
    from pragma_table_xinfo(?, ?)
   order by cid
`

const listSqliteForeignKeysSQL = `
  select id, "table" as target_table, "from" as column_name, "to" as target_column,
         on_delete as delete_rule
    from pragma_foreign_key_list(?, ?)
   order by id, seq
`

const listSqliteIndexesSQL = `
  select name, "unique" as is_unique, origin
    from pragma_index_list(?, ?)
   order by (origin = 'pk') desc, name
`

const listSqliteIndexColumnsSQL = `
  select name from pragma_index_info(?, ?) where name is not null order by seqno
`
