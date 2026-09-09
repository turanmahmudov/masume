# Engines

Engine support and test coverage are separate. The tables describe implemented behavior, default capabilities, and the configured CI coverage.

## Support tiers

**Tier 1** engines have integration coverage in CI. The check workflow runs on pull requests, pushes to `master`, and calls from other workflows.

| Engine | Versions tested |
| --- | --- |
| PostgreSQL | 14 and 18 |
| MySQL | 8.0 and 8.4 |
| SQL Server | 2019 and 2022 |
| ClickHouse | 25.8 |
| MariaDB | 11 |
| MongoDB | 8, as a standalone server, with authentication, and as a replica set |
| SQLite | Temporary files without a server |

**Tier 2** engines share a tier 1 protocol and have engine-specific configuration or behavior. The repository has no real-server integration coverage for these services.

Tier 2 includes CockroachDB, TimescaleDB, Redshift, Neon, Supabase, TiDB, PlanetScale, and Aurora MySQL. Unit tests cover some engine-specific behavior. Protocol support is not a guarantee of service compatibility.

Engine problem reports should include the service, server version, statement, and error.

## Protocols

Engines in one protocol family share a driver. Catalogs, SQL features, permissions, plans, and hosted restrictions can differ.

| Protocol | Engines |
| --- | --- |
| PostgreSQL | PostgreSQL, CockroachDB, TimescaleDB, Redshift, Neon, Supabase |
| MySQL | MySQL, MariaDB, TiDB, PlanetScale, Aurora MySQL |
| SQLite | SQLite |
| TDS | SQL Server |
| ClickHouse native | ClickHouse |
| MongoDB wire | MongoDB |

## Capabilities by engine

Most capabilities are static defaults from `internal/core/engine.go`. The interface uses these flags for action availability. A flag does not guarantee server support or permission. An offered action can still fail.

The following table contains the default flags before deployment checks:

| Engine | Plans | Measures | Transactions | Cancels | Activity | Locks | Load | Sorts | Truncates | DDL |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| aurora-mysql | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| clickhouse | yes | no | no | yes | yes | no | yes | yes | yes | yes |
| cockroach | yes | yes | yes | no | no | no | no | yes | yes | yes |
| mariadb | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| mongodb | yes | yes | yes | no | yes | no | no | yes | no | no |
| mysql | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| neon | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| planetscale | yes | yes | yes | no | no | no | no | yes | yes | yes |
| postgres | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| redshift | yes | no | yes | yes | yes | no | no | yes | yes | yes |
| sqlite | yes | no | yes | no | no | no | no | yes | no | yes |
| sqlserver | yes | yes | yes | no | yes | yes | yes | yes | yes | yes |
| supabase | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| tidb | yes | yes | yes | yes | yes | no | no | yes | yes | yes |
| timescale | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |

- **Plans**: Query plans for supported statements.
- **Measures**: Execution measurements in supported plans.
- **Transactions**: Explicit begin, commit, and rollback operations.
- **Cancels**: Dedicated cancellation of the current query. An engine without it hides the cancel key, and the wheel of a running statement shows `this engine cannot stop a running statement`.
- **Activity**: Session or operation listing. Stopping another session also requires server permission.
- **Locks**: Blocking relationships between sessions.
- **Load**: Connection counts, limits, and server start time. Additional metrics depend on the engine.
- **Sorts**: Server-side sorting for supported reads.
- **Truncates**: `TRUNCATE` in the object menu.
- **DDL**: Object definition retrieval or generation.

MongoDB transaction and atomic staged-write flags depend on the deployment's `hello` response. Replica sets and sharded clusters support transactions; standalone servers do not. A standalone server applies staged changes separately, and earlier changes can remain after failure.

Other default flags are:

| Flag | Default |
| --- | --- |
| Plans every statement | CockroachDB only |
| Write previews | Every SQL engine except ClickHouse; no MongoDB |
| Read-only mode | Every engine except TiDB; MongoDB and SQL Server enforcement is client-only |
| Atomic staged changes | Every engine except ClickHouse, which holds no transaction; MongoDB adjusts this after connection |
| Statement statistics | No engine before connection; PostgreSQL-family sessions enable this after an extension check, SQL Server sessions after a permission check, and ClickHouse sessions after a check of its query log |

## Dashboard metrics

The dashboard omits unsupported panels. Activity, lock relationships, server load, and statement statistics are separate capabilities.

| Engines | Implemented metrics | Dependencies |
| --- | --- | --- |
| PostgreSQL, TimescaleDB, Neon, Supabase | Activity, locks, connections, connection limit, start time, transaction count, WAL bytes, temporary files, cache hits, replication lag | PostgreSQL statistics views, functions, and sufficient permissions |
| MySQL, MariaDB, Aurora MySQL | Activity, connections, connection limit, start time | `information_schema.processlist`, `performance_schema.global_status`, and `@@max_connections` |
| ClickHouse | Running statements, connections, connection limit, start time, statement statistics | `system.processes`, `system.metrics`, `system.server_settings`, and `system.query_log` |
| SQL Server | Activity, locks, connections, connection limit, start time, statement statistics | `sys.dm_exec_sessions`, `sys.dm_exec_requests`, `sys.dm_tran_locks`, `sys.dm_os_sys_info`, and the VIEW SERVER STATE permission |
| Redshift, TiDB | Activity only | The adapter's activity query and sufficient permissions |
| MongoDB | Current operations | `currentOp` and sufficient permissions |
| CockroachDB, PlanetScale, SQLite | No dashboard metrics | None |

PostgreSQL metrics use `pg_stat_activity`, `pg_locks`, `pg_stat_database`, WAL functions, and replication statistics. Replication lag appears only when the query returns a value. Cache hit rate needs recorded block reads or hits. Rates need successive counter samples.

PostgreSQL-family statement statistics need an available `pg_stat_statements` extension. masume checks the extension catalog when the session opens, except for engine variants without that catalog. The server must load the extension and permit access to its statistics. The panel contains call counts, mean execution time, total execution time, and returned rows.

ClickHouse statement statistics come from `system.query_log`, which the server must be configured to write. masume reads that table once when the session opens and offers the panel where the read succeeds. One row of the panel stands for one shape of statement, grouped by the hash the server normalizes it to.

SQL Server statement statistics come from `sys.dm_exec_query_stats`, which needs the VIEW SERVER STATE permission. masume reads that view once when the session opens and offers the panel where the read succeeds. SQL Server load metrics do not include the PostgreSQL counters, cache hit rate, or replication lag.

MySQL-family load metrics do not include PostgreSQL counters, cache hit rate, replication lag, or statement statistics. MariaDB's `performance_schema.global_status` table requires version 10.5.2 or later and an available Performance Schema. The adapter does not read MySQL lock relationships.

Static capability flags do not check every statistics view, extension setting, or permission. Missing dependencies can produce dashboard errors.

## Read-only access

The client refuses recognized writes for read-only profiles. PostgreSQL-family sessions also request server read-only mode. MySQL and MariaDB use `SET SESSION TRANSACTION READ ONLY`. SQLite opens existing files with `mode=ro`. ClickHouse uses `SET readonly = 2`, which refuses a write and still takes the settings the driver sends. MongoDB and SQL Server have client-only checks.

TiDB does not enforce the session read-only statement. An explicit TiDB profile with `mode = "read-only"` fails during connection.

MCP read-only access is separate from profile mode. For TiDB, MCP retains the profile mode and applies its client access policy. A writable TiDB profile can therefore open with MCP read-only access. An explicitly read-only TiDB profile still fails.

Client classification cannot guarantee that a read has no side effects. Database permissions remain separate from client access checks.

## Default port and TLS

| Engine | Port | Default `sslmode` |
| --- | --- | --- |
| aurora-mysql | 3306 | `prefer` |
| clickhouse | 9000 | unset; no TLS |
| cockroach | 26257 | `prefer` |
| mariadb | 3306 | `prefer` |
| mongodb | 27017 | unset; no TLS |
| mysql | 3306 | `prefer` |
| neon | 5432 | `require` |
| planetscale | 3306 | `require` |
| postgres | 5432 | `prefer` |
| redshift | 5439 | `require` |
| sqlite | none | none |
| sqlserver | 1433 | unset; the login only |
| supabase | 5432 | `require` |
| tidb | 4000 | `prefer` |
| timescale | 5432 | `prefer` |

The table contains effective defaults. Most PostgreSQL-family and MySQL-family defaults are unset internally and behave as `prefer`.

For PostgreSQL-family and MySQL-family engines, `allow` and `prefer` permit unencrypted fallback. `require` requires TLS without certificate verification. `verify-ca` checks the certificate chain; `verify-full` also checks the host name. Verification uses the system trust roots.

ClickHouse differs: the native protocol does not negotiate, so unset and `allow` and `prefer` connect without encryption. `require` encrypts without certificate verification, and `verify-ca` and `verify-full` verify it. An encrypted ClickHouse listens on a port of its own, which is 9440 by default.

SQL Server differs: unset and `allow` and `prefer` encrypt the login and send the rest of the session unencrypted. `disable` encrypts nothing. `require` encrypts the whole session without certificate verification, and `verify-ca` and `verify-full` verify it.

MongoDB differs: unset or `disable` uses no TLS. Explicit `allow`, `prefer`, and `require` require TLS without certificate verification and have no unencrypted fallback. MongoDB also supports `verify-ca` and `verify-full`.

See [configuration.md](configuration.md) for profile settings. Connection targets do not forward native URL options; see [headless.md](headless.md#connection-targets).


## MongoDB

A query tab accepts a subset of MongoDB shell calls. The client is not a JavaScript runtime or a complete `mongosh` implementation.

```js
db.orders.find({status: "new"}).sort({total: -1})
```

The parser accepts extended JSON, unquoted document keys, single-quoted strings, comments, trailing commas, and regular expressions such as `/pattern/i`.

Supported value helpers are `ObjectId`, `ISODate`, `Date`, `NumberLong`, `NumberInt`, `NumberDouble`, `NumberDecimal`, and `UUID`. General JavaScript variables, loops, and function evaluation are unsupported.

`db.getSiblingDB("name")` selects a database for a statement. `db.getCollection("name")` selects a collection with a quoted name. Statements end at a top-level semicolon or newline. Open documents and continuation lines starting with `.` can span lines.

### Supported calls

The adapter executes these database calls:

| Call | Supported arguments |
| --- | --- |
| `runCommand`, `adminCommand` | One command document; `adminCommand` uses the admin database |
| `getCollectionNames` | No filter or options |
| `createCollection` | Collection name only |
| `dropDatabase` | No options |

The adapter executes these collection calls:

| Call | Supported arguments and behavior |
| --- | --- |
| `find`, `findOne` | Filter, optional projection, and the supported chains below |
| `aggregate` | Pipeline array only |
| `countDocuments`, `count` | Filter only; both use `CountDocuments` |
| `estimatedDocumentCount` | No options |
| `distinct` | Field name and optional filter |
| `getIndexes` | No options |
| `insertOne`, `insertMany`, `insert` | Document or document array; the value shape selects single or multiple insertion |
| `updateOne`, `updateMany`, `replaceOne` | Filter and update or replacement only |
| `deleteOne`, `deleteMany` | Filter only |
| `remove` | Filter and optional boolean or `{justOne: true}` |
| `findOneAndUpdate`, `findOneAndReplace` | Filter and update or replacement; returns the original document |
| `findOneAndDelete` | Filter; returns the removed document |
| `createIndex` | Key document and optional `name`, `unique`, and `sparse` settings |
| `dropIndex` | Index name |
| `drop` | No options |

Find chains apply `sort`, `projection`, `limit`, and `skip`. The parser accepts `pretty`, `toArray`, `batchSize`, `hint`, `allowDiskUse`, and `collation`, but execution ignores these chains. Other find chains fail.

Most methods ignore extra arguments and chains instead of rejecting them. Unsupported options are not forwarded. For example, update options such as `upsert` and find-and-update options such as `returnDocument` have no effect. Aggregate options such as `allowDiskUse` also have no effect. Index options other than `name`, `unique`, and `sparse` are ignored.

Command documents passed to `runCommand` or `adminCommand` reach the server as documents. Their replies remain command documents; cursor replies are not automatically exhausted. Client access checks still apply.

Completion and syntax highlighting include more methods than execution supports. Calls such as `bulkWrite`, `update`, `save`, and `createIndexes` are not implemented as shell methods.

Plans support `find`, `aggregate`, `count`, `countDocuments`, and `distinct`. Shell `.explain()` chaining is not implemented. Use the interface plan action or headless `--explain`.

### Documents and access

Collection metadata samples up to 100 documents. Result columns come from the returned documents; streaming columns come from the first batch. A column with different non-null types is `mixed`. Staged edits use the row's `_id`.

Streaming omits fields first encountered after the first batch and reports an error. Headless output can already be incomplete at that point. See [headless.md](headless.md#memory-and-streaming).

Set a user only for authenticated MongoDB connections. With a user, the adapter supplies credentials; without a user, it supplies none. Authentication settings beyond the profile fields are not available through native URL query options.

## SQL Server

masume connects to SQL Server 2016 and later, and to Azure SQL Database, over TDS. The connection opens one database, and the relations of that database appear under their schemas. `dbo` is the default schema of most logins.

A page after the first is taken with `OFFSET` and `FETCH NEXT`, which the server reads after a sort only. Such a page of a read with no sort of its own gets `ORDER BY (SELECT NULL)`, which keeps the rows in the order the server returns them. That order is not guaranteed between pages, so sort a read whose pages must line up. The first page takes no window at all, and the client caps the rows as it reads them, so a read the server refuses to sort still runs: `select next value for` is one. The generated `SELECT` of the object menu caps its rows with `TOP` instead.

The statement separator is the semicolon. `GO` is a separator of the command-line tools and not of the server, so a buffer that holds one fails. The server also takes `CREATE VIEW`, `CREATE FUNCTION`, `CREATE PROCEDURE` and `CREATE TRIGGER` as the first statement of a batch only, so each one needs a tab or a cell of its own.

An identity column and a computed column both refuse a value from the client, so the row form leaves them out. A rename goes through `sp_rename`, which the object menu writes.

A write with an `OUTPUT` clause answers with rows, and masume shows them in place of a count. The server refuses `OUTPUT` without `INTO` on a table that has an enabled trigger, so such a write needs `OUTPUT INTO` or a disabled trigger.

Statement diagnostics use `sys.dm_exec_describe_first_result_set`, which compiles the statement and returns the fault as a row. A statement with a `:name` parameter is checked once the parameters have values.

An estimated plan comes from `SET SHOWPLAN_ALL ON` and a measured plan from `SET STATISTICS PROFILE ON`. Both carry estimated and counted rows per step; neither carries a time per step.

The dashboard stops another session with `KILL`, which ends the session and its transaction. T-SQL has no statement that stops one statement of another session, so the activity list offers no cancel, and neither does the interface for a statement of this connection: `Ctrl+X` is unavailable on a SQL Server connection. Set `statement_timeout_ms` to bound a statement instead. The client stops such a statement through the driver, and it opens the connection again afterwards, because a stopped statement leaves the connection unusable. A transaction is lost with that connection, and the client says so.

The server has no read-only session, so a read-only profile is enforced by this client alone. It also has no materialized view; an indexed view appears as a view.

## ClickHouse

masume connects to ClickHouse over its native protocol, on port 9000 by default. A ClickHouse database is a schema, and the connected one is the default. The object tree draws that database alone.

The protocol takes one statement per call, so a buffer of several statements runs one at a time and answers with the result of the last one. Such a buffer binds no values: a `:name` parameter belongs to a buffer that holds one statement.

The server holds no transaction of the user, so begin, commit and rollback are refused, and staged changes are applied one after another. A change that fails leaves the changes before it in place, and the report names the change that failed.

A staged edit of a row is written as `ALTER TABLE … UPDATE`, which is a mutation of the table. The session sets `mutations_sync = 1`, so the server finishes the mutation before it answers and the grid reads the row back as it now stands. A staged delete is written as `DELETE FROM`. A write reports no row count: the server counts the rows of a mutation nowhere the client can read.

The server has no foreign key and keeps the constraints of a table in the statement that made it, so the relation panes show neither, and the diagram draws no relationship. The sorting key of a table appears as its primary index, and the data-skipping indexes appear beside it. A materialized view appears as one, and the table it keeps its rows in is hidden.

A new table needs an engine, so the object menu writes `ENGINE = MergeTree` with the order the table keeps its rows in. An import writes the same, ordered by the first column of the file. The server numbers no column of its own, so a new table carries a plain `UInt64` key.

`EXPLAIN` returns the plan of a read, as one step per line. No plan carries a measurement of a run, so the pane shows the estimate alone. Statement diagnostics use `EXPLAIN PLAN`, which reads every name of a read without running it; a statement that is not a read is not checked.

`KILL QUERY` stops one statement, and both stop actions of the activity list use it: a statement of this server belongs to no session that can be closed. The server names a statement with a text of its own, so the list of this client counts its rows instead, and a stop acts on the row that was listed.

## Choosing the engine

`engine` is the profile's database engine. The engine entry contains the driver, default port, capabilities, and engine-specific behavior. The default engine is `postgres`.

```toml
[profile.shop]
engine = "postgres"
```

See [configuration.md](configuration.md) for the other profile keys.
