# Configuration

All configuration is in one file: `$XDG_CONFIG_HOME/masume/config.toml`, which is `~/.config/masume/config.toml` on most systems. `config.example.toml` in the repository lists every key with its default value. Use it as a reference next to this page.

On the first run, masume creates a starter file if there is none. You can edit the file by hand, or let masume edit it. When you add or change a connection in the connection form, masume rewrites only that block and keeps everything else unchanged, including comments.

## A profile

One profile is one connection. The name after `profile.` is the name shown in the picker.

```toml
[profile.shop]
engine   = "postgres"
host     = "127.0.0.1"
port     = 5432
database = "shop"
user     = "ada"
auth     = "prompt"
env      = "dev"
mode     = "write"
```

| Key | Default | Meaning |
| --- | --- | --- |
| `engine` | `postgres` | See [engines.md](engines.md) for the list |
| `host` | required | The form defaults to `127.0.0.1`. Ignored for SQLite |
| `port` | per engine | The default port of the engine |
| `database` | required | The file path for SQLite. The database number for Redis. A leading `~` is expanded |
| `user` | required if the engine needs one | Ignored for SQLite. Optional for Redis and MongoDB |
| `auth` | `password`, or `command` if `password_command` is set | `password`, `command` or `prompt` |
| `env` | `dev` | `dev`, `test` or `prod`. `prod` colours the title bar red |
| `mode` | `write` | `write` or `read-only` |
| `confirm_writes` | `off` on dev, `delete` on test, `write` on prod | `off`, `delete`, `write` or `agent`. `agent` is for an agent client that cannot show a question, see [mcp.md](mcp.md#a-client-that-cannot-ask) |
| `write_plan` | `off` on dev, `count` on test, `undo` on prod | `off`, `count` or `undo`. See [Measuring a write](#measuring-a-write) |
| `undo_rows` | `1000` | Rows a write plan reads to build an undo. `0` sets no limit |
| `sslmode` | `prefer`, or `require` for engines that only accept TLS | `disable`, `allow`, `prefer`, `require`, `verify-ca`, `verify-full` |
| `statement_timeout_ms` | `0` | Time limit for one statement in milliseconds. `0` uses the server default |
| `keepalive_s` | `30` | Seconds between two connection checks. `0` disables the keepalive |
| `page_size` | `200` | Rows the grid loads per page, and rows one page of `masume run` holds. Must be above zero |
| `autocommit` | `true` | `false` keeps a transaction open until you commit or roll back |
| `mcp` | | The access level for agents on this profile. See [mcp.md](mcp.md) |
| `description` | | One line, shown in the picker |
| `ai_instructions` | | Context about this database for the AI model |

A profile that misses a required key is skipped and reported. The other profiles still load.

## A connection on the command line

A connection given as the first argument needs no profile. masume reads three forms.

```sh
masume postgres://reader@db.internal:5432/shop?sslmode=verify-full
masume "host=db.internal port=5432 dbname=shop user=reader sslmode=require"
masume ./notes.db
```

| Form | Read as |
| --- | --- |
| A URL | The scheme selects the engine: `postgres`, `postgresql`, `mysql`, `mariadb`, `cockroachdb`, `redshift`, `redis`, `rediss`, `mongodb` |
| A connection string | `key=value` pairs: `host`, `hostaddr`, `port`, `dbname`, `database`, `user`, `password`, `sslmode`. A value can be quoted with `'`. The engine is `postgres` |
| A file path | A SQLite file. The path needs an extension of `.db`, `.db3`, `.sqlite` or `.sqlite3`, or the file must be there already. `:memory:` opens a database that is never written |

A URL that names no database connects to the database the server itself defaults to: the name of the user on a PostgreSQL server, database `0` on Redis, and `admin` on MongoDB. A MySQL server has no such default, so its URL must name a database.

`rediss://` connects with TLS and verifies the certificate. Every other setting the target does not carry takes the default of a new connection: `env = "dev"`, `mode = "write"`, `page_size = 200`.

masume asks for the password if the target carries none. The profile it builds is not written to the config file, and the picker lists it under the name of its database or its file. A name a profile of the config file already holds gets a number after it, so the two rows can be told apart.

```sh
masume --profile shop-prod       # open one profile of the config file
```

## Databases in a container

```sh
masume --detect
```

`--detect` asks `docker` for the containers that run on this machine, and `podman` where there is no docker. Every database it finds becomes a row of the connection picker, before the profiles of the config file. Nothing is written to the config file.

A container is offered when both are true:

- Its image names a database masume supports. `postgres`, `postgis`, `pgvector`, `timescale`, `supabase`, `cockroach`, `mysql`, `percona`, `mariadb`, `tidb`, `redis`, `valkey` and `mongo` are read from the image name, whatever the registry and the tag are. An image built on another one is read as itself, so `supabase/postgres` is Supabase and not PostgreSQL.
- It publishes the port that database listens on. A container that publishes no port cannot be reached from this machine, so it is left out.

The user, the database and the password come from the environment of the container: `POSTGRES_USER`, `POSTGRES_DB` and `POSTGRES_PASSWORD`; `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE` and `MYSQL_ROOT_PASSWORD`, and the same names with a `MARIADB_` prefix; `MONGO_INITDB_ROOT_USERNAME`, `MONGO_INITDB_ROOT_PASSWORD` and `MONGO_INITDB_DATABASE`; `REDIS_PASSWORD`; `COCKROACH_USER`, `COCKROACH_PASSWORD` and `COCKROACH_DATABASE`. What the image itself defaults to is used where a variable is not set, so a `postgres` container with only `POSTGRES_DB` set still connects as the `postgres` user.

Each connection gets `env = "dev"` and `mode = "write"`. A container of a hosted service such as `supabase/postgres` gets `sslmode = "prefer"` instead of the `require` its engine defaults to, because a container on this machine listens without TLS.

`--detect` exits with 1 if neither docker nor podman is on the path, if the tool reports an error, or if no container runs a database. It takes no connection of its own, so `--detect` with a URL or with `--profile` is an error.

With no argument at all, masume opens `$DATABASE_URL` if the shell exports it. An argument on the command line is opened instead of the variable.

## Keeping a connection that is in no file

A connection opened from the command line, or found by `--detect`, is in no config file. masume asks about it when you quit:

```
┌─ save connection ─────────────────────────────┐
│ Write "shop" to the config file?              │
│                                               │
│ shop  postgres@db.internal:5432/shop          │
│                                               │
│ The password is written to the file as well.   │
│                                               │
│ y save and quit · n quit without saving       │
└───────────────────────────────────────────────┘
```

The question names every such connection that was opened, one time each. `y` writes them and quits, `n` quits without them, and `Esc` returns to the client. The last line appears only where a connection holds a password, because that password goes into the file with it. A profile the config file already holds is never offered, and neither is a connection that was listed but never opened.

To save a connection earlier, or under another name, or without its password, press `Ctrl+N` for the picker, then `e` to open the connection form and `Ctrl+S` to save it. A connection saved that way is not offered again when you quit.

A file that cannot be written keeps masume open with the reason, so the report does not disappear with the client.

## Importing a file

The object menu on a table, `m` in the tree, offers **Import a file…**. The same entry on a schema imports into a table the import makes.

The card opens on a file picker of the directory masume was started in. `↑↓` moves, `→` opens a directory, `←` goes to the one above, and `Enter` chooses. Only the files an import can read are offered; any other is drawn dimmed and cannot be chosen.

Choosing a file reads it and shows the form, which asks how the file is read and maps the columns:

```
┌─ import orders.csv · 4 rows ──────────────────────────────┐
│ ▸ file                  ./orders.csv                      │
│   table                 public.orders                     │
│   format                csv                               │
│   delimiter             ,                                 │
│   header                yes                               │
│   null as               \N                                │
│   order_id integer      order_id                          │
│   placed_at timestamp   placed_at                         │
│   coupon text           (skip)                            │
│ ↑↓ field · ← → change · Enter review · Esc cancel         │
└───────────────────────────────────────────────────────────┘
```

`Enter` steps forward one stage at a time: the file is read, then what the import would do is reviewed, and only then are the rows written. `Esc` in the review goes back to the form, so a mapping can be changed without the file being read again.

`Enter` on the `file` row opens the picker again, so the file can be changed after its columns were mapped. The row is also typed into, for a path that is quicker to paste than to walk to.

| Setting | Meaning |
| --- | --- |
| `file` | The path of the file, filled in by the picker. A leading `~` is expanded |
| `table` | Where the rows go. A name with a dot in it names its schema as well |
| `format` | `csv` or `json`. The name of the file chooses it, until you choose one yourself |
| `delimiter` | One character, for a CSV. A `.tsv` file gets a tab |
| `header` | Whether the first row of a CSV names the columns. A file without one names them by position |
| `null as` | The text a CSV writes a value that is not there with. An empty field is one whatever this holds |

### The columns

Each column of the file is one row of the form, labelled with the kind of value it holds, and it steps through the columns of the table. A column of the file whose name a column of the table holds is mapped onto it by itself; any other is left out until you map it. A column the server fills itself is never offered.

The kinds are read from the first 200 rows: `integer`, `number`, `boolean`, `timestamp` and `text`. A number written with a zero in front of it, such as a postal code, is read as text, because a number would drop the zero. A column of two kinds is read as the kind that holds both, and text holds anything.

For a table that is already there, a value is cast to the type of the column it goes into. For a table the import makes, the kinds of the file become its types: `bigint`, `numeric`, `boolean`, `timestamptz` and `text` on PostgreSQL, and the same idea in each server's own names.

### The review

The review says how many rows would be written, lists the rows that cannot be, and shows the SQL:

```
3 of 4 rows into orders, 5 columns

1 row not written:
  line 5  total_cents: "n/a" is no whole number

insert into "public"."orders" (order_id, placed_at, total_cents, paid, note)
values (100241, '2026-02-11 09:03:00.000', 4990, true, 'first order'),
       (100242, '2026-02-12 00:00:00.000', 1200, false, null)
```

The dry run reads the whole file and reaches no server, so its answer is the same whether the import runs after it or not. A row it refuses is left out of the import rather than stopping it, because you have already read how many there are.

The rows are written in batches of 1000 inside one transaction, so an import that fails part way leaves the table as it was, and the reason stays on the card.

Not yet: an upsert on a key, Parquet, and an encoding other than UTF-8.

## Measuring a write

`write_plan` measures one write before it runs and shows what it does, in place of the plain question. It applies to a single statement on PostgreSQL, MySQL and SQLite: the client reads the relation and the predicate out of the statement, so a write that joins a second relation, that names its target through an alias, or that runs beside other statements keeps the plain question.

```
╭─ write plan · shop-prod ──────────────────────────────────────────────╮
│ delete from orders where status = 'open'                              │
│                                                                       │
│ rows      12,904 of 48,210 in orders · 26.8%  ▇▇▇░░░░░░░░░            │
│ cascades  order_lines · on delete cascade · 4,201 rows                │
│ blocked   order_notes · on delete restrict · 8 rows reference these   │
│ undo      12,904 rows read with the write  Alt+U after it ran         │
│ commit    the write and its undo run in one transaction               │
│                                                                       │
│ y run · n cancel · Esc cancel                                         │
╰───────────────────────────────────────────────────────────────────────╯
```

The rows are counted on the server with the predicate of the write, not estimated. An update also lists the columns it assigns. `cascades` names the triggers this write runs and the foreign keys that follow a removed row into another relation. `blocked` names the foreign keys that reject the delete while a row of theirs references these rows, which is a write the server answers with an error.

`write_plan = "undo"` also builds the statements that reverse the write: an update per row for an update, an insert per row for a delete or a truncate. `Alt+U` runs them, all of them or none, after a question that names what they undo. Only the last write of a connection keeps an undo, and a write the server refuses keeps none.

The rows of the undo are read inside the transaction of the write, and the read holds them until that transaction ends. A second session that writes to one of them waits for the commit, so the undo covers exactly the rows the write changed, with the values it overwrote. On a server without transactions the write runs without an undo, and the card says so.

There is no undo where the relation has no primary key, where the write assigns that key, where it is an insert, or where it lands on more rows than `undo_rows`. The card says which of these it is before the write runs. Where the undo cannot be read at all, nothing is written: the answer was given to a plan that promised one.

The chat and the MCP server measure a write the same way: the panel draws the plan with its question, and a statement an agent ran answers with an `undo` list of the statements that reverse it.

## Without a screen

`masume run` uses the same profiles as the client, and every limit a profile sets: `statement_timeout_ms`, `mode = "read-only"`, `page_size` and the pre-connect `command`. See [headless.md](headless.md).

## Passwords

Keep the password out of the file.

```toml
auth = "prompt"                      # asked at connect time, kept in memory only
```

```toml
auth             = "command"
password_command = "pass db/shop"    # read from a password manager
```

```toml
auth         = "password"
password_env = "PGPASSWORD"          # read from an environment variable
```

A password command has 30 seconds to print its first line. The rest of its output is ignored.

You can store a password in the file with `password`, but then the file contains a secret. Run `chmod 600 ~/.config/masume/config.toml` if you do.

Redis takes a password without a user, because the server setting `requirepass` has no user. MongoDB takes a user only when authentication is enabled. A server without authentication refuses a connection that sends credentials.

## A command before the connection

A profile can run a command before the connection opens, for example an SSH tunnel. masume stops the command when the connection closes, so no tunnel process is left running.

```toml
[profile.shop-tunnel]
engine          = "postgres"
host            = "127.0.0.1"
port            = 15432
database        = "shop"
user            = "reader"
auth            = "prompt"
command         = "ssh -N -L 15432:db.internal:5432 jump.example.com"
command_timeout = 10
wait_for_port   = 15432
```

masume waits until `wait_for_port` accepts a connection, then connects. It waits at most `command_timeout` seconds.

## Interface

```toml
[ui]
icons               = "plain"    # plain or ascii
theme               = "system"   # see themes.md
hide_system_schemas = true       # false shows pg_catalog and other system schemas
```

`[ui.icon_glyphs]` sets the glyph for each kind of row. A glyph set to `""` disables that glyph. `[ui.colors]` overrides theme colours without a theme file. `config.example.toml` lists both in full.

## Keys

```toml
[keys]
preset = "default"

[keys.global]
run-at-cursor = ["ctrl+r", "f5"]
```

Every action has a default key binding, and every binding can be changed. `alt`, `meta` and `option` are names for the same modifier. See [keys.md](keys.md).

## AI and MCP

`[ai] enabled = false` disables every AI feature. `[mcp] profiles` is empty by default, so no profile is served. See [ai.md](ai.md) and [mcp.md](mcp.md).

## Other files the client writes

| Path | Contains |
| --- | --- |
| `$XDG_CONFIG_HOME/masume/themes/` | Custom themes, one file each |
| `$XDG_STATE_HOME/masume/history.sqlite` | Query history, saved queries, marks, open tabs |
| `$XDG_STATE_HOME/masume/mcp.log` | Every MCP tool call |
| `$XDG_STATE_HOME/masume/ai-chat.log` | Every AI chat request |

`XDG_CONFIG_HOME` and `XDG_STATE_HOME` relocate all of them. Each state file is created with owner-only permissions.
