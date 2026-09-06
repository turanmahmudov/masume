# Engines

Engine support and test coverage are separate. The tables describe implemented behavior, default capabilities, and the configured CI coverage.

## Support tiers

**Tier 1** engines have integration coverage in CI. The check workflow runs on pull requests, pushes to `master`, and calls from other workflows.

| Engine | Versions tested |
| --- | --- |
| PostgreSQL | 14 and 18 |
| MySQL | 8.0 and 8.4 |
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
| MongoDB wire | MongoDB |

## Capabilities by engine

Most capabilities are static defaults from `internal/core/engine.go`. The interface uses these flags for action availability. A flag does not guarantee server support or permission. An offered action can still fail.

The following table contains the default flags before deployment checks:

| Engine | Plans | Measures | Transactions | Cancels | Activity | Locks | Load | Sorts | Truncates | DDL |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| aurora-mysql | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| cockroach | yes | yes | yes | no | no | no | no | yes | yes | yes |
| mariadb | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| mongodb | yes | yes | yes | no | yes | no | no | yes | no | no |
| mysql | yes | yes | yes | yes | yes | no | yes | yes | yes | yes |
| neon | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| planetscale | yes | yes | yes | no | no | no | no | yes | yes | yes |
| postgres | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| redshift | yes | no | yes | yes | yes | no | no | yes | yes | yes |
| sqlite | yes | no | yes | no | no | no | no | yes | no | yes |
| supabase | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| tidb | yes | yes | yes | yes | yes | no | no | yes | yes | yes |
| timescale | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |

- **Plans**: Query plans for supported statements.
- **Measures**: Execution measurements in supported plans.
- **Transactions**: Explicit begin, commit, and rollback operations.
- **Cancels**: Dedicated cancellation of the current query.
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
| Write previews | Every SQL engine; no MongoDB |
| Read-only mode | Every engine except TiDB; MongoDB enforcement is client-only |
| Atomic staged changes | Every engine; MongoDB adjusts this after connection |
| Statement statistics | No engine before connection; PostgreSQL-family sessions can enable this after an extension check |

## Dashboard metrics

The dashboard omits unsupported panels. Activity, lock relationships, server load, and statement statistics are separate capabilities.

| Engines | Implemented metrics | Dependencies |
| --- | --- | --- |
| PostgreSQL, TimescaleDB, Neon, Supabase | Activity, locks, connections, connection limit, start time, transaction count, WAL bytes, temporary files, cache hits, replication lag | PostgreSQL statistics views, functions, and sufficient permissions |
| MySQL, MariaDB, Aurora MySQL | Activity, connections, connection limit, start time | `information_schema.processlist`, `performance_schema.global_status`, and `@@max_connections` |
| Redshift, TiDB | Activity only | The adapter's activity query and sufficient permissions |
| MongoDB | Current operations | `currentOp` and sufficient permissions |
| CockroachDB, PlanetScale, SQLite | No dashboard metrics | None |

PostgreSQL metrics use `pg_stat_activity`, `pg_locks`, `pg_stat_database`, WAL functions, and replication statistics. Replication lag appears only when the query returns a value. Cache hit rate needs recorded block reads or hits. Rates need successive counter samples.

PostgreSQL-family statement statistics need an available `pg_stat_statements` extension. masume checks the extension catalog when the session opens, except for engine variants without that catalog. The server must load the extension and permit access to its statistics. The panel contains call counts, mean execution time, total execution time, and returned rows.

MySQL-family load metrics do not include PostgreSQL counters, cache hit rate, replication lag, or statement statistics. MariaDB's `performance_schema.global_status` table requires version 10.5.2 or later and an available Performance Schema. The adapter does not read MySQL lock relationships.

Static capability flags do not check every statistics view, extension setting, or permission. Missing dependencies can produce dashboard errors.

## Read-only access

The client refuses recognized writes for read-only profiles. PostgreSQL-family sessions also request server read-only mode. MySQL and MariaDB use `SET SESSION TRANSACTION READ ONLY`. SQLite opens existing files with `mode=ro`; MongoDB has client-only checks.

TiDB does not enforce the session read-only statement. An explicit TiDB profile with `mode = "read-only"` fails during connection.

MCP read-only access is separate from profile mode. For TiDB, MCP retains the profile mode and applies its client access policy. A writable TiDB profile can therefore open with MCP read-only access. An explicitly read-only TiDB profile still fails.

Client classification cannot guarantee that a read has no side effects. Database permissions remain separate from client access checks.

## Default port and TLS

| Engine | Port | Default `sslmode` |
| --- | --- | --- |
| aurora-mysql | 3306 | `prefer` |
| cockroach | 26257 | `prefer` |
| mariadb | 3306 | `prefer` |
| mongodb | 27017 | unset; no TLS |
| mysql | 3306 | `prefer` |
| neon | 5432 | `require` |
| planetscale | 3306 | `require` |
| postgres | 5432 | `prefer` |
| redshift | 5439 | `require` |
| sqlite | none | none |
| supabase | 5432 | `require` |
| tidb | 4000 | `prefer` |
| timescale | 5432 | `prefer` |

The table contains effective defaults. Most PostgreSQL-family and MySQL-family defaults are unset internally and behave as `prefer`.

For PostgreSQL-family and MySQL-family engines, `allow` and `prefer` permit unencrypted fallback. `require` requires TLS without certificate verification. `verify-ca` checks the certificate chain; `verify-full` also checks the host name. Verification uses the system trust roots.

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

## Choosing the engine

`engine` is the profile's database engine. The engine entry contains the driver, default port, capabilities, and engine-specific behavior. The default engine is `postgres`.

```toml
[profile.shop]
engine = "postgres"
```

See [configuration.md](configuration.md) for the other profile keys.
