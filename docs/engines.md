# Engines

Not all engines have the same test coverage. The support tiers below describe the coverage of each one. Read the tiers first, then the capability table.

## Support tiers

**Tier 1** engines are tested against a real server on every change. They are reliable.

| Engine | Versions tested |
| --- | --- |
| PostgreSQL | 14 and 18 |
| MySQL | 8.0 and 8.4 |
| MariaDB | 11 |
| MongoDB | 8, as a standalone server, with authentication, and as a replica set |
| Redis | 8 |
| SQLite | Against a temporary file, so no server is involved |

**Tier 2** engines use the protocol of a tier 1 engine. masume adapts the catalog queries, the capabilities and the plan parser to each service. None of them is tested against a real server, so problems are found only through user reports. The tier 2 engines are CockroachDB, TimescaleDB, Redshift, Neon, Supabase, TiDB, PlanetScale and Aurora MySQL.

Open an engine problem issue if a tier 2 engine gets something wrong.

## Protocols

Engines that share a protocol behave the same way. This is why a hosted service works when the engine it is based on works.

| Protocol | Engines |
| --- | --- |
| PostgreSQL | PostgreSQL, CockroachDB, TimescaleDB, Redshift, Neon, Supabase |
| MySQL | MySQL, MariaDB, TiDB, PlanetScale, Aurora MySQL |
| SQLite | SQLite |
| RESP | Redis |
| MongoDB wire | MongoDB |

## Capabilities by engine

masume queries the capabilities of the server and hides unsupported actions. An action that the engine does not support has no key binding and is not shown in menus. So the interface never offers an action that the server refuses.

| Engine | Plans | Measures | Transactions | Cancels | Activity | Sorts | Truncates | DDL |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| aurora-mysql | yes | yes | yes | yes | yes | yes | yes | yes |
| cockroach | yes | yes | yes | no | no | yes | yes | yes |
| mariadb | yes | yes | yes | yes | yes | yes | yes | yes |
| mongodb | yes | yes | yes | no | yes | yes | no | no |
| mysql | yes | yes | yes | yes | yes | yes | yes | yes |
| neon | yes | yes | yes | yes | yes | yes | yes | yes |
| planetscale | yes | yes | yes | no | no | yes | yes | yes |
| postgres | yes | yes | yes | yes | yes | yes | yes | yes |
| redis | no | no | no | no | yes | no | no | no |
| redshift | yes | no | yes | yes | yes | yes | yes | yes |
| sqlite | yes | no | yes | no | no | yes | no | yes |
| supabase | yes | yes | yes | yes | yes | yes | yes | yes |
| tidb | yes | yes | yes | yes | yes | yes | yes | yes |
| timescale | yes | yes | yes | yes | yes | yes | yes | yes |

- **Plans** - the server can return the query plan of a statement.
- **Measures** - the plan can include measured execution times.
- **Transactions** - begin, commit and roll back by hand.
- **Cancels** - a running statement can be cancelled.
- **Activity** - other sessions can be listed, and one can be stopped.
- **Sorts** - the grid can sort a query result.
- **Truncates** - `TRUNCATE` is offered in the object menu.
- **DDL** - the server can return the `CREATE` statement of an object.

The MongoDB row depends on the deployment. MongoDB supports transactions on a replica set or a sharded cluster, and not on a standalone server. masume reports the capabilities of the connected deployment.

TiDB accepts `SET SESSION TRANSACTION READ ONLY` but does not enforce it. On a profile opened `read-only`, this client still refuses writes, but the server itself does not enforce the read-only session.

## Default port and TLS

| Engine | Port | Default `sslmode` |
| --- | --- | --- |
| aurora-mysql | 3306 | `prefer` |
| cockroach | 26257 | `prefer` |
| mariadb | 3306 | `prefer` |
| mongodb | 27017 | `prefer` |
| mysql | 3306 | `prefer` |
| neon | 5432 | `require` |
| planetscale | 3306 | `require` |
| postgres | 5432 | `prefer` |
| redis | 6379 | `prefer` |
| redshift | 5439 | `require` |
| sqlite | none | none |
| supabase | 5432 | `require` |
| tidb | 4000 | `prefer` |
| timescale | 5432 | `prefer` |

`prefer` tries TLS first and falls back to an unencrypted connection if the server does not support TLS. `require` always uses TLS and never falls back. See [configuration.md](configuration.md) for `verify-ca` and `verify-full`.

## Redis

Redis has no SQL. A query tab accepts Redis commands, one per line:

```
SET user:1 "a name"
GET user:1
```

The tree is built by scanning the key space, so it shows the state at the time of the last scan. A key written after that scan appears after the next scan. masume knows the Redis command set and marks unknown commands.

A Redis profile can have a password without a user, because the server setting `requirepass` does not use a user name.

## MongoDB

A query tab accepts MongoDB shell syntax:

```js
db.orders.find({status: "new"}).sort({total: -1})
```

The parser accepts the shell form of a document as well as strict extended JSON. So `ObjectId("…")`, unquoted keys, single quotes and `/pattern/i` all work.

A collection has no schema, so the columns of a result are the fields found in a sample of its documents. The type of a column is the type found in the sample, or `mixed` if the sample contained more than one type. A staged edit is written using the `_id` of its row.

Set a user in a MongoDB profile only when the server has authentication enabled. A server without authentication refuses a connection that sends credentials.

## Choosing the engine

Set `engine` in the profile. masume selects the driver, the default port, the catalog queries and the plan parser based on it.

```toml
[profile.shop]
engine = "postgres"
```

See [configuration.md](configuration.md) for the other profile keys.
