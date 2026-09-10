# Without a screen

`masume run` runs statements without a screen and writes results to stdout. The command uses the configured profiles, connection commands, timeouts, and read-only checks.

```sh
masume run -p shop-prod -f json 'select count(*) from orders'
```

## Arguments

```text
masume run [TARGET] STATEMENT
masume run [TARGET] -e FILE
```

| Argument | Meaning |
| --- | --- |
| `TARGET` | A supported URL, keyword connection string, or SQLite path. Without a target, use `--profile` or `$DATABASE_URL` |
| `-p`, `--profile NAME` | A profile from the config file or the project file |
| `-e`, `--execute FILE` | The statement file. A single `-` reads stdin |
| `-f`, `--format FORMAT` | `table` by default, or `csv`, `json`, or `markdown` |
| `-l`, `--limit ROWS` | A positive output cap per statement, including statements with their own limit |
| `--param NAME=VALUE` | A string value for `:NAME`. Repeat for each parameter |
| `--explain` | A JSON plan, with execution measurements for eligible reads |
| `-h`, `--help` | Print help and exit |

The target precedes the statement when both are positional arguments. A target and `--profile` cannot appear together.

`-h` takes priority over the positional arguments and prints the help. An unknown option takes priority over `-h` and exits with code 2.

Only one `-e` file is used. Repeated `-e` options use the last file. SQL that starts with `--` must come from a file or stdin. The argument parser has no `--` separator.

### Connection targets

Supported URL schemes are `postgres`, `postgresql`, `cockroachdb`, `redshift`, `mysql`, `mariadb`, `sqlserver`, `mssql`, `clickhouse`, and `mongodb`. Other engines need a profile or a keyword connection string with `engine`.

URLs support one host. Multi-host URLs and `mongodb+srv` are unsupported. The parser reads credentials, host, port, database, and `sslmode`, `ssl-mode`, or `sslMode`. Other query options are ignored and are not forwarded to the driver. This includes native options such as `authSource`, `replicaSet`, `tls`, and `connect_timeout`.

Keyword connection strings default to PostgreSQL. The accepted keys are `engine`, `host`, `hostaddr`, `port`, `dbname`, `database`, `user`, `password`, and `sslmode`. `hostaddr` is an alias for `host`; `dbname` is an alias for `database`. Unknown keys are refused. See [configuration.md](configuration.md) for connection configuration.

SQLite files must already exist. Recognized extensions are `.db`, `.db3`, `.sqlite`, and `.sqlite3`; other paths need a SQLite header. `:memory:` opens a temporary database for that run.

## Writes and access

Headless runs do not use `confirm_writes`, `write_plan`, or undo capture. The profile's `autocommit` setting does not start a transaction. SQL transactions require explicit `BEGIN`, `COMMIT`, or `ROLLBACK` statements.

A read-only profile refuses recognized writes before execution. PostgreSQL also sets `default_transaction_read_only`; MySQL and MariaDB set the session transaction mode to read-only. SQLite opens existing files with `mode=ro`. MongoDB has client checks only, without a server read-only session.

A TiDB profile with `mode = "read-only"` fails to open, with exit `2`. MCP read-only access has a separate client-only fallback for TiDB. See [engines.md](engines.md#read-only-access).

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | The run completed, possibly with an intentional cap or a default read cap |
| `1` | Statement, parameter, plan, or output failure; an empty batch; or incomplete write results without `--limit` |
| `2` | Argument, input file, password, connection command, or connection failure |
| `3` | The profile's read-only check refused a write |

Exit `1` does not prove that a write failed. A write can succeed before output fails or its returned rows exceed the default cap. Do not automatically retry a write after a nonzero exit. Check the database state first.

An explicit `--limit` makes truncation return `0`, including truncation of write results. Diagnostics and truncation notices go to stderr. Check the exit status before using the output as a complete result.

## Formats

`table` uses spaces between columns and measures each column's widest cell. `markdown` writes a pipe-separated table without measuring column widths. Both formats replace newlines inside cells with spaces.

```text
id  total_cents  status
--  -----------  ---------
1   4990         paid
2   1200         paid
3   99           cancelled
```

CSV uses commas, a header, LF endings, and quoting as needed. Null and empty string values both produce empty fields. Newlines remain inside quoted fields.

CSV formula guarding is enabled. Non-numeric fields starting with `=`, `+`, `-`, `@`, tab, or carriage return receive a leading apostrophe. Plain numbers remain unchanged. Headless runs have no options for these CSV settings.

JSON output is an array of records. A result without rows produces an empty array. CSV writes the available column header even without rows. An empty MongoDB result has no discovered columns.

| JSON value | Form |
| --- | --- |
| Top-level record keys | Sorted by byte value, so uppercase names come first; repeated column names receive suffixes such as `_2` |
| Native numbers and booleans | JSON numbers and booleans |
| Driver-specific decimals | Text, with the driver's decimal precision |
| Null | `null` |
| Arrays and document values | Embedded JSON when the value has a recognized structure |
| Newline inside text | The escape `\n` |
| Other driver values | Formatted text |

MongoDB output contains columns from documents, not a lossless BSON dump. Object IDs become hexadecimal strings, and decimal values become text.

A statement without a result set writes its command and available affected count to stderr, such as `UPDATE 1`. Such statements write nothing to stdout.

## Parameters

Parameter names are case-insensitive. Every CLI parameter value is a string. `42`, `true`, and `null` do not become numbers, booleans, or null values. The database can apply conversions required by the statement.

Repeated parameter names use the last supplied value.

SQL execution uses driver parameters. MongoDB parameters become quoted inline values. Explain parameters also become quoted inline values through the engine dialect.

```sh
masume run -p shop -f csv \
  --param day=2026-09-02 --param status=paid \
  'select id from orders where created_at::date = :day and status = :status'
```

Typed MongoDB values must appear in the statement, for example `ObjectId("507f1f77bcf86cd799439011")` or `{quantity: 42}`.

## Plans

`--explain` always writes JSON, regardless of `--format`. Eligible reads execute when the engine supports measured plans. This is not a dry run. Reads can take locks or call functions with side effects.

Recognized writes receive an estimated plan only, when the engine supports that statement. Read-only checks still apply before planning.

```sh
masume run -p shop --explain --param s=paid \
  'select * from orders where status = :s' \
  | jq '[.nodes[].selfMs // 0] | add'
```

The top-level fields are `analyzed`, `summary`, and `nodes`. `analyzed` is `true` for a measured plan.

Each node contains `depth`, `label`, `detail`, `estimatedRows`, `actualRows`, `selfMs`, `shareOfTotal`, `slowest`, and `misestimated`. Missing estimates or measurements are `null`.

Use one statement per explain run. Several statements produce consecutive JSON objects, without an enclosing array, unless `--format json` refuses the batch first.

## Passwords

A `password` value in a TOML profile is ignored and produces a warning. Passwords supplied in connection targets remain supported.

| Authentication | Headless source |
| --- | --- |
| `auth = "password"` | `password_env`, or a password supplied in the connection target |
| `auth = "command"` | The first stdout line from `password_command` |
| `auth = "secret"` | The first stdout line from the configured `[secret.NAME]` command |
| `auth = "keyring"` | The existing keyring entry for the profile name |
| `auth = "prompt"` | Unsupported when the connection requires a prompt |

Commands have no stdin and have a 30-second timeout. Missing required passwords, missing keyring entries, and password resolution errors return `2`. SQLite needs no password. MongoDB can omit the user when authentication is disabled.

## Several statements

A statement argument or file can contain several statements. Headless runs execute statements in order on one session and stop at the first failure. Later statements do not run.

A batch is not automatically atomic. Earlier writes can remain committed after a later failure. Supported SQL engines accept explicit transaction statements within the batch. Separate invocations do not share a session or transaction.

For example, a transaction file can contain:

```sql
BEGIN;
UPDATE orders SET status = 'paid' WHERE id = 42;
INSERT INTO order_events (order_id, event) VALUES (42, 'paid');
COMMIT;
```

Run the file with `masume run -p shop -e payment.sql`. Engine DDL and nontransactional table restrictions still apply.

`--format json` refuses several statements with exit `1`, before executing any statement. Other formats write consecutive results, with a separate header for each tabular result. Empty input or SQL containing only comments returns `1`.

## Row limits

Without `--limit`, a read with a recognized limit returns all rows within that limit. Other reads return at most `page_size` rows, which defaults to 200.

Recognized limits include SQL `LIMIT`, `FETCH FIRST`, MongoDB `.limit(n)`, and `findOne()`. A MongoDB pipeline's `$limit` stage alone does not select the streaming path.

```sh
masume run -p shop 'select * from orders limit 250'
masume run -p shop 'select * from orders'
masume run -p shop --limit 100 'select * from orders limit 250'
```

The first command returns up to 250 rows. The second returns one profile page. The third returns at most 100 rows, despite the SQL limit.

`--limit` applies separately to each statement and disables batch streaming. A default read cap returns `0` and a notice when more rows exist. An explicit cap also returns `0` and a notice.

### Memory and streaming

Batch streaming applies only to recognized reads with their own limit and without CLI `--limit`. The batch size is `page_size`. CSV and JSON write each batch as the batch arrives.

Table and Markdown output buffer the whole returned result before writing. Other reads and all writes use one capped read before output.

MongoDB streaming columns are the fields in the first batch. Fields first encountered later are omitted from the output. The driver reports those fields as an error after reading the cursor.

Such a MongoDB run returns `1`. CSV can already contain incomplete records, and JSON can lack its closing bracket. Buffered table and Markdown output remains unwritten. Cursor and output errors can also leave partial output.

### Write results

A write executes once and never uses the streaming path. A write such as `UPDATE ... RETURNING` can succeed while returning more rows than the cap.

Without `--limit`, truncated write results return `1`. An explicit `--limit` makes the same truncation return `0`. A larger `--limit` can hold the complete returned result in one read. The output cap does not limit affected rows.

### Report profile

`page_size` is the default cap for the interface and headless runs. This example uses an environment password:

```toml
[profile.shop-report]
engine       = "postgres"
host         = "db.internal"
database     = "shop"
user         = "reader"
auth         = "password"
password_env = "SHOP_REPORT_PASSWORD"
mode         = "read-only"
page_size    = 50000
```

`SHOP_REPORT_PASSWORD` must be available in the process environment.

## Notebooks

`masume nb run FILE` runs a notebook file. The profile, the timeouts, the read-only check and the exit codes are the ones above. A write cell needs `--allow-writes`, because a run without a screen has no confirmation, no write plan and no undo.

```
masume nb run reports/revenue-review.masume.md -p shop --param day=2026-09-01 -f markdown
```

`--only CELL` runs one cell by id. `--explain` writes a JSON plan of every statement and runs none of them. `markdown` writes the whole notebook with the rows of every cell. See the [notebook guide](notebooks.md#without-a-screen).

## Dump and restore

`masume dump` writes a schema as SQL and `masume restore` runs such a file back into a server. Both use the profiles, the connection commands, the timeouts and the exit codes above.

```sh
masume dump -p shop --schema public --drop shop.sql
masume dump -p shop - | gzip > shop.sql.gz
masume restore -p shop-staging shop.sql
```

```text
masume dump [TARGET] FILE
masume restore [TARGET] FILE
```

| Argument | Meaning |
| --- | --- |
| `FILE` | The dump file. A single `-` writes stdout, and a restore reads stdin |
| `-p`, `--profile NAME` | A profile from the config file or the project file |
| `-s`, `--schema NAME` | The schema to dump. Without it, the default schema of the connection |
| `-t`, `--table NAME` | One table, as `name` or `schema.name`. Repeat for more, and the objects are left out |
| `-c`, `--content WHAT` | `schema and rows` by default, or `schema only` or `rows only` |
| `--drop` | Write a `DROP … IF EXISTS` for everything the dump makes |
| `-h`, `--help` | Print help and exit |

A dump holds the types, sequences and functions of the schema, then its tables and their rows, then the views over them, then its triggers. Every table stands after the tables its foreign keys name. Roles, grants and owners are not written. A dump only reads, so a read-only profile writes one.

Neither command runs on an engine that reports no definitions, such as MongoDB, which holds another language.

A restore runs each statement on its own, in file order, with no wrapping transaction. It stops at the first failure, reports the statement that failed and how many ran before it, and exits with code 1. A read-only profile exits with code 3 and sends nothing.

## Config and history

`masume run` reads the config and project files without writing either file. A run without a config file does not create the starter file. The terminal client and `masume --detect` can create that file.

Headless runs do not record query history in `history.sqlite`.
