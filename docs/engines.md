# Engines

The engines on this page do not all get the same test coverage. The support tiers below say how much each one gets. Read them first, then the capability table under them.

## Support tiers

**Tier 1** is tested against a real server on every change, so these are the engines to trust.

| Engine | Versions tested |
| --- | --- |
| PostgreSQL | 14 and 18 |
| MySQL | 8.0 and 8.4 |
| MariaDB | 11 |
| MongoDB | 8, standalone, with authentication, and as a replica set |
| Redis | 8 |
| SQLite | Against a temporary file, so no server is involved |

**Tier 2** speaks the protocol of a tier 1 engine, and masume tunes the catalog reads, the capabilities and the plan reader to each service. None of them is tested against a real server, so they rest on reports: CockroachDB, TimescaleDB, Redshift, Neon, Supabase, TiDB, PlanetScale and Aurora MySQL.

File an engine report if a tier 2 engine gets something wrong.

## Protocols

Engines that share a protocol behave the same way, which is why a hosted service works as soon as the engine it was built on does.

| Protocol | Engines |
| --- | --- |
| PostgreSQL | PostgreSQL, CockroachDB, TimescaleDB, Redshift, Neon, Supabase |
| MySQL | MySQL, MariaDB, TiDB, PlanetScale, Aurora MySQL |
| SQLite | SQLite |
| RESP | Redis |
| MongoDB wire | MongoDB |

## Capabilities by engine

masume asks the server what it can do, then hides what it cannot. An action the engine does not support has no key and is not offered, so nothing on screen promises something the server will refuse.

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

- **Plans** - the server can explain a statement.
- **Measures** - the plan carries the times the server measured.
- **Transactions** - begin, commit and roll back by hand.
- **Cancels** - a running statement can be stopped.
- **Activity** - other sessions can be listed, and one can be stopped.
- **Sorts** - the grid can order a read.
- **Truncates** - `TRUNCATE` is offered in the object menu.
- **DDL** - the server can write the `CREATE` statement of an object.

The MongoDB row depends on the deployment. MongoDB holds a transaction on a replica set or a sharded cluster, and none at all on a standalone server. masume reports what the deployment answered.

TiDB takes `SET SESSION TRANSACTION READ ONLY` and does nothing under it. A profile opened `read-only` is still refused by this client, but the server itself will not hold the session read-only.

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

`prefer` tries TLS and falls back to the clear if the server refuses it. `require` encrypts and never falls back. See [configuration.md](configuration.md) for `verify-ca` and `verify-full`.

## Redis

Redis has no SQL. A tab takes commands, one to a line:

```
SET user:1 "a name"
GET user:1
```

The tree is built by scanning the key space, so it shows the result of the last scan. A write made since then is not in it until the next scan. masume knows the command set, and marks a command it does not know.

A Redis profile may name a password with no user. The server `requirepass` names nobody.

## MongoDB

A tab takes the calls of the shell:

```js
db.orders.find({status: "new"}).sort({total: -1})
```

The reader takes the shell form of a document as well as strict extended JSON, so `ObjectId("…")`, a bare key, a single quote and `/pattern/i` all read.

A collection keeps no schema, so the columns of a read are the fields a sample of its documents holds. The type of a column is the one type the sample saw, or `mixed` where it saw more than one. A staged edit is written by the `_id` of its row.

Name a user in a MongoDB profile only where the server has authentication turned on. A server without it refuses a connection that carries one.

## Choosing the engine

Set `engine` in the profile. masume picks the driver, the default port, the catalog reads and the plan reader from it.

```toml
[profile.shop]
engine = "postgres"
```

See [configuration.md](configuration.md) for the rest of a profile.
