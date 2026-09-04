# Configuration

The configuration of one user is in one file: `$XDG_CONFIG_HOME/masume/config.toml`, which is `~/.config/masume/config.toml` on most systems. A repository can provide connections and queries of its own in a `.masume.toml` beside its code; see [The project file](#the-project-file). `config.example.toml` in the repository lists every key with its default value. Use it as a reference next to this page.

On the first run, masume creates a starter file if there is none. You can edit the file by hand, or let masume edit it. When you add or change a connection in the connection form, masume rewrites only that block and keeps everything else unchanged, including comments.

## The sections

| Section | Holds |
| --- | --- |
| [`[profile.NAME]`](#a-profile) | One connection. Any number of them |
| [`[secret.NAME]`](#a-secret-store) | One password store, named by any number of profiles |
| [`[ui]`](#interface) | Icons, theme and colours |
| [`[keys]`](#keys) | The key preset, and one table per scope of bindings |
| [`[ai]`](#ai) | Whether the chat is on, which provider, and the settings of each provider |
| [`[mcp]`](#mcp) | The profiles an agent reaches, and its access level |

A committed [`.masume.toml`](#the-project-file) holds `[profile.NAME]` and `[query.NAME]` only, and no section of this list.

Every key is optional unless the table marks it `required`. A wrong value never stops the app. The key is reported and the default is used, or the profile is skipped and the others still load.

Each kind of report appears in its own place:

| Report | Where it appears |
| --- | --- |
| A profile or a `[secret]` store | On stderr as the client starts, in `masume run` and in `masume --mcp`. The client also lists these reports under **Config problems** in the palette, `Ctrl+K` |
| `[ui]`, `[keys]` or `[ai]` | Under **Config problems** in the palette of the client. A run without a screen uses none of these three sections, and reports nothing about them |

A key masume does not know is ignored in silence, so check a key name against these tables if a setting seems to do nothing.

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
| `database` | required | The file path for SQLite. A leading `~` is expanded |
| `user` | required if the engine needs one | Ignored for SQLite. Optional for MongoDB |
| `auth` | `password`, or `command` if `password_command` is set, or `secret` if `secret` is set | The password source: `prompt`, `keyring`, `command`, `secret` or `password`. See [Passwords](#passwords) |
| `password` | | **Ignored.** No file masume reads carries a password. The key is reported and the profile asks for its password instead |
| `password_env` | | The environment variable that holds the password. Read when `auth` is `password` |
| `password_command` | | A shell command that prints the password on its first line. Read when `auth` is `command` |
| `secret` | | The `[secret]` store that holds the password. Read when `auth` is `secret` |
| `secret_ref` | | The reference inside that store, passed to its command as one quoted argument |
| `env` | `dev` | `dev`, `test` or `prod`. `prod` colours the title bar red |
| `mode` | `write` | `write` or `read-only` |
| `confirm_writes` | `off` on dev, `delete` on test, `write` on prod | `off`, `delete`, `write` or `agent`. `agent` is for an agent client that cannot show a question, see [mcp.md](mcp.md#a-client-that-cannot-ask) |
| `write_plan` | `off` on dev, `count` on test, `undo` on prod | `off`, `count` or `undo`. See [Measuring a write](#measuring-a-write) |
| `undo_rows` | `1000` | Rows a write plan reads to build an undo. `0` sets no limit |
| `sslmode` | unset, the same as `prefer`. `require` on the TLS-only engines: Redshift, Neon, Supabase and PlanetScale | `disable` never encrypts. `allow` and `prefer` try TLS and fall back to the clear. `require` encrypts and checks no certificate. `verify-ca` checks the certificate authority. `verify-full` also checks the host name on the certificate. An unknown name is reported and the profile is skipped |
| `statement_timeout_ms` | `0` | Time limit for one statement in milliseconds. `0` uses the server default |
| `keepalive_s` | `30` | Seconds between two connection checks. `0` disables the keepalive |
| `page_size` | `200` | Rows the grid loads per page, and rows one page of `masume run` holds. Must be above zero |
| `autocommit` | `true` | `false` keeps a transaction open until you commit or roll back |
| `command` | | A shell command run before the connection and stopped with it, for example an SSH tunnel. See [A command before the connection](#a-command-before-the-connection) |
| `wait_for_port` | | The port `command` must open before masume connects. Without it masume connects at once |
| `command_timeout` | `10` | Seconds to wait for `wait_for_port`. Must be above zero |
| `mcp` | the `[mcp]` level | The access level for agents on this profile: `off`, `read-only`, `read-write` or `full`. This key can only lower the `[mcp]` level, never raise it. A profile with `mode = "read-only"` stays at `read-only` whatever this key sets. See [mcp.md](mcp.md) |
| `description` | | One line, shown in the picker |
| `ai_instructions` | | Context about this database for the AI model, sent with every chat on this connection |

A profile that misses a required key is skipped and reported. The other profiles still load. A key masume does not know is ignored without a report. A wrong *value* is always reported.

## A connection on the command line

A connection given as the first argument needs no profile. masume reads three forms.

```sh
masume postgres://reader@db.internal:5432/shop?sslmode=verify-full
masume "host=db.internal port=5432 dbname=shop user=reader sslmode=require"
masume ./notes.db
```

| Form | Read as |
| --- | --- |
| A URL | The scheme selects the engine: `postgres`, `postgresql`, `mysql`, `mariadb`, `cockroachdb`, `redshift`, `mongodb` |
| A connection string | `key=value` pairs: `host`, `hostaddr`, `port`, `dbname`, `database`, `user`, `password`, `sslmode`. A value can be quoted with `'`. The engine is `postgres` |
| A file path | A SQLite file. The path needs an extension of `.db`, `.db3`, `.sqlite` or `.sqlite3`, or the file must be there already. `:memory:` opens a database that is never written |

A URL that names no database connects to the database the server itself defaults to: the name of the user on a PostgreSQL server, and `admin` on MongoDB. A MySQL server has no such default, so its URL must name a database.

Every setting the target does not carry takes the default of a new connection: `env = "dev"`, `mode = "write"`, `page_size = 200`.

masume asks for the password if the target carries none. The profile it builds is not written to the config file, and the picker lists it under the name of its database or its file. A name a profile of the config file already holds gets a number after it.

```sh
masume --profile shop-prod       # open one profile of the config file
```

## Databases in a container

```sh
masume --detect
```

`--detect` asks `docker` for the containers that run on this machine, and `podman` where there is no docker. Every database it finds becomes a row of the connection picker, before the profiles of the config file. Nothing is written to the config file.

A container is offered when both are true:

- Its image names a database masume supports. `postgres`, `postgis`, `pgvector`, `timescale`, `supabase`, `cockroach`, `mysql`, `percona`, `mariadb`, `tidb` and `mongo` are read from the image name, whatever the registry and the tag are. An image built on another one is read as itself, so `supabase/postgres` is Supabase and not PostgreSQL.
- It publishes the port that database listens on. A container that publishes no port is left out.

The user, the database and the password come from the environment of the container: `POSTGRES_USER`, `POSTGRES_DB` and `POSTGRES_PASSWORD`; `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE` and `MYSQL_ROOT_PASSWORD`, and the same names with a `MARIADB_` prefix; `MONGO_INITDB_ROOT_USERNAME`, `MONGO_INITDB_ROOT_PASSWORD` and `MONGO_INITDB_DATABASE`; `COCKROACH_USER`, `COCKROACH_PASSWORD` and `COCKROACH_DATABASE`. What the image itself defaults to is used where a variable is not set, so a `postgres` container with only `POSTGRES_DB` set still connects as the `postgres` user.

Each connection gets `env = "dev"` and `mode = "write"`. A container of a hosted service such as `supabase/postgres` gets `sslmode = "prefer"`, not the `require` of its engine. A container on this machine listens without TLS.

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
│ The password goes into the keyring, not the   │
│ file.                                         │
│                                               │
│ y save and quit · n quit without saving       │
└───────────────────────────────────────────────┘
```

The question names every such connection that was opened, one time each. `y` writes them and quits, `n` quits without them, and `Esc` returns to the client. The last line appears only where a connection holds a password. On a machine with no keyring the line says so, and the profile is written with `auth = "prompt"`. A profile the config file already holds is never offered, and neither is a connection that was listed but never opened.

To save a connection earlier, or under another name, or without its password, press `Ctrl+N` for the picker, then `e` to open the connection form and `Ctrl+S` to save it. A connection saved that way is not offered again when you quit.

A file that cannot be written keeps masume open, with the reason on the card.

## The project file

A repository can hold a `.masume.toml` next to its code. It provides the connections of the project and the statements the team keeps. A new person clones the repository, runs `masume`, and gets the development database with no onboarding document.

masume looks for the file in the directory it was started in, then in each directory above it, and reads the first one it finds. Every entry point reads it: the client, `masume run` and `masume --mcp`.

```toml
# .masume.toml, committed with the code

[profile.dev]
engine   = "postgres"
host     = "127.0.0.1"
port     = 5432
database = "shop"
user     = "shop"
env      = "dev"

[profile.staging]
engine           = "postgres"
host             = "staging.internal"
database         = "shop"
user             = "reader"
env              = "test"
mode             = "read-only"
password_command = "op read op://eng/shop-staging/password"

[query.recent-orders]
sql         = "select * from orders order by created_at desc limit 50"
description = "the newest 50 orders"

[query.stuck-jobs]
sql      = "select * from jobs where state = 'running' and started_at < now() - interval '1 hour'"
profiles = ["staging"]
```

### What the file can set

`[profile.NAME]` holds the server address and the connection guards. It cannot set anything that reaches a secret. These keys are refused, and the profile with them:

| Key | What it does |
| --- | --- |
| `password_command` | Runs a shell command on connect |
| `command` | Runs a shell command on connect |
| `password_env` | Reads a variable of your shell |
| `secret`, `secret_ref` | Reads a secret of your own store |

A profile that sets one of these is skipped, and the reason is reported.

`auth = "prompt"` and `auth = "keyring"` both work in a project file. For any other password source, open the connection form on the project profile with `e` and save it with `Ctrl+S`. The profile is then yours, in your own config file, and you set the password source there.

`[query.NAME]` holds one statement the whole team gets under `Ctrl+Q`:

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `sql` | string | required | The statement. A query without one is reported and skipped |
| `description` | string | | What the statement answers, shown in the card in place of its text |
| `profiles` | list of strings | every profile | The connections the statement is offered on |

`env`, `mode`, `confirm_writes` and `write_plan` are ordinary profile keys, and a project file sets them for everybody. `env = "prod"` colours the frame of the client on that connection.

No other section is read. A `[ui]`, `[keys]`, `[ai]` or `[mcp]` section in a project file is reported and ignored.

### What the user keeps

A profile in the config file replaces the project profile of the same name. A statement you saved under a name replaces the project statement of that name.

The connection picker marks a connection of the project file with `project`, and shows the file path. `d` does not remove such a connection. Edit the project file instead. `e` opens the connection form on it, and `Ctrl+S` writes it into your own config file, where it becomes yours and overrides the project one.

`Ctrl+Q` lists the statements of the project file with the ones you saved, sorted by name, and marks the project's with `project`. `Enter` loads one into the editor. `Ctrl+D` does not remove one; edit the project file instead.

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

`Enter` steps forward one stage at a time: the file is read, then what the import would do is reviewed, and only then are the rows written. `Esc` in the review goes back to the form, and a mapping can be changed without a second read of the file.

`Enter` on the `file` row opens the picker again, and the file can be changed after its columns were mapped. The row is also typed into, for a path that is quicker to paste than to walk to.

| Setting | Meaning |
| --- | --- |
| `file` | The path of the file, filled in by the picker. A leading `~` is expanded |
| `table` | The target table. A name with a dot in it carries its schema as well |
| `format` | `csv` or `json`. The name of the file chooses it, until you choose one yourself |
| `delimiter` | One character, for a CSV. A `.tsv` file gets a tab |
| `header` | `yes` reads the column names from the first row of a CSV. `no` maps the columns by position |
| `null as` | The text a CSV writes a value that is not there with. An empty field is one whatever this holds |

### The columns

Each column of the file is one row of the form, labelled with the kind of value it holds, and it steps through the columns of the table. A column of the file whose name a column of the table holds is mapped onto it by itself; any other is left out until you map it. A column the server fills itself is never offered.

The kinds are read from the first 200 rows: `integer`, `number`, `boolean`, `timestamp` and `text`. A number written with a zero in front of it, such as a postal code, is read as text. A number would drop the zero. A column of two kinds is read as the kind that holds both, and text holds anything.

For a table that is already there, a value is cast to the type of the column it goes into. For a table the import makes, the kinds of the file become its types: `bigint`, `numeric`, `boolean`, `timestamptz` and `text` on PostgreSQL, and the same idea in each server's own names.

### The review

The review gives the row count, lists the rows it cannot write, and shows the SQL:

```
3 of 4 rows into orders, 5 columns

1 row not written:
  line 5  total_cents: "n/a" is no whole number

insert into "public"."orders" (order_id, placed_at, total_cents, paid, note)
values (100241, '2026-02-11 09:03:00.000', 4990, true, 'first order'),
       (100242, '2026-02-12 00:00:00.000', 1200, false, null)
```

The dry run reads the whole file and reaches no server. Its answer is the same with or without an import after it. A row it refuses is left out of the import, and does not stop it.

The rows are written in batches of 1000 inside one transaction, so an import that fails part way leaves the table as it was, and the reason stays on the card.

An import does not yet read a Parquet file, upsert on a key, or read an encoding other than UTF-8.

## Measuring a write

`write_plan` measures one write before it runs and shows what it does, in place of the plain question. It applies to a single statement on PostgreSQL, MySQL and SQLite. The client reads the relation and the predicate out of the statement. Three writes keep the plain question: one that joins a second relation, one with its target behind an alias, and one that runs beside other statements.

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

The rows are counted on the server with the predicate of the write, not estimated. An update also lists the columns it assigns. `cascades` lists the triggers this write runs, and the foreign keys that follow a removed row into another relation. `blocked` lists the foreign keys that reject the delete while a row of theirs references these rows. The server answers such a write with an error.

`write_plan = "undo"` also builds the statements that reverse the write: an update per row for an update, an insert per row for a delete or a truncate. `Alt+U` runs them, all of them or none, after a question that shows what they undo. Only the last write of a connection keeps an undo, and a write the server refuses keeps none.

The rows of the undo are read inside the transaction of the write, and the read holds them until that transaction ends. A second session that writes to one of them waits for the commit. The undo then covers exactly the rows the write changed, with the values it overwrote. On a server without transactions the write runs without an undo, and the card says so.

Four writes get no undo: one on a relation with no primary key, one that assigns that key, an insert, and one that lands on more rows than `undo_rows`. The card gives the reason before the write runs. A write whose undo cannot be read at all does not run.

The chat and the MCP server measure a write the same way: the panel draws the plan with its question, and a statement an agent ran answers with an `undo` list of the statements that reverse it.

## Without a screen

`masume run` uses the same profiles as the client, and every limit a profile sets: `statement_timeout_ms`, `mode = "read-only"`, `page_size` and the pre-connect `command`. See [headless.md](headless.md).

## Passwords

`auth` is the password source. There are five.

| `auth` | Password source |
| --- | --- |
| `prompt` | The user, at connect time. masume keeps the password in memory only |
| `keyring` | The keyring of the operating system, where masume stored the password earlier |
| `command` | The shell command in `password_command` |
| `secret` | The `[secret]` store in `secret`, at the reference in `secret_ref` |
| `password` | The environment variable in `password_env` |

```toml
auth = "prompt"                      # asked at connect time, kept in memory only
```

```toml
auth = "keyring"                     # kept in the keyring of this machine
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

No file that masume reads carries a password. A `password` key is ignored, wherever it is written, and the client reports it:

```
profile "shop": a password in a file is ignored; type it once and tick
"remember in the keyring", or set password_env, password_command or a
[secret] store
```

The profile still opens, and asks for its password instead. Saving a profile removes a `password` line that a hand-edited file still holds.

### The keyring of the operating system

masume can keep a password in the keyring of the machine: secret-service over D-Bus on Linux, which is what GNOME Keyring and KWallet answer on, and the Keychain on macOS. Every entry masume writes carries the service name `masume` and the name of the profile, so they are easy to find in the tool of the desktop.

Nothing is stored without being asked. On a machine with a keyring, the password card carries a box:

```
╭─ password ────────────────────────────────────╮
│ connecting to shop-prod · prod                │
│ reader@db.internal:5432/shop                  │
│                                               │
│ ••••••••••••                                  │
│                                               │
│ [x] remember in the keyring                   │
│                                               │
│ Enter connect · Esc cancel · Tab keyring      │
╰───────────────────────────────────────────────╯
```

`Tab` ticks the box. The password reaches the keyring only after the server accepts it. masume then writes `auth = "keyring"` into the profile, and the next connection asks for nothing.

A profile that already reads the keyring opens with the box ticked. A keyring that lost its entry fills again on the next connection.

A connection given on the command line, as a URL with a password in it or one found by `--detect`, carries its password in memory only. Saving it puts that password in the keyring and writes the profile with `auth = "keyring"`. On a machine with no keyring the profile is written with `auth = "prompt"` instead, and the password is not kept.

Removing a connection with `d` removes its password from the keyring as well.

A machine with no keyring draws no box, and `auth = "keyring"` there asks the user instead of failing.

### A secret store

A store is declared once under `[secret.NAME]` and named by any number of profiles. masume knows no tool: `command` is a shell command that prints one secret, and `{{ref}}` is where the reference of the profile goes.

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `command` | string | required | The shell command that prints the secret on its first line. It must hold `{{ref}}` at least once. A store with no `command`, or a command with no `{{ref}}`, is reported and skipped |


```toml
[secret.work]
command = "op read {{ref}}"

[secret.infra]
command = "vault kv get -field=password {{ref}}"

[secret.local]
command = "sops -d --extract '[\"db\"][\"password\"]' {{ref}}"

[profile.shop-prod]
auth       = "secret"
secret     = "work"
secret_ref = "op://eng/shop-prod/password"

[profile.warehouse]
auth       = "secret"
secret     = "infra"
secret_ref = "secret/data/warehouse"
```

Naming a store is enough: a profile that sets `secret` and no `auth` reads that store.

The reference is passed as one quoted argument. A reference with a blank, a quote or a semicolon in it reaches the tool whole, and runs nothing of its own. One reference is one argument: a store command that needs two references belongs in a script the store calls.

A store whose command fails reports the exit code and the first line of its error. The same 30 second limit applies.

A profile that names a store the config file does not declare is skipped, with that reason.

Every `[secret]` store belongs to the config file of the user. A project file can neither declare a store nor name one.

MongoDB takes a user only when authentication is enabled. A server without authentication refuses a connection that sends credentials.

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
icons               = "plain"
theme               = "tokyonight"
hide_system_schemas = true
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `icons` | `plain` or `ascii` | `plain` | The glyph set the tree draws. `ascii` uses ASCII characters only, for a terminal or a font that draws the others wrongly. An unknown name is reported and `plain` is drawn |
| `theme` | string | `ayu-dark` | A built-in theme name, a file name in `themes/` without `.toml`, or `system` for the colours of the terminal. See [themes.md](themes.md) |
| `hide_system_schemas` | boolean | `true` | `false` shows `pg_catalog`, `information_schema` and the other system schemas in the tree. `h` in the tree toggles it for one session |

Four tables sit under `[ui]` and change colours without a theme file. Each one is applied over the selected theme:

| Table | Meaning |
| --- | --- |
| `[ui.icon_glyphs]` | The glyph for one kind of row, for example `table = "▦"`, which is where a Nerd Font glyph goes. A glyph set to `""` draws nothing for that kind. An unknown kind name is reported |
| `[ui.palette]` | Named colours, each a hex value. An entry of the two tables below can use a name from here in place of a hex value |
| `[ui.colors]` | One interface colour each, for example `accent` or `error`. [themes.md](themes.md) lists them all |
| `[ui.syntax]` | The colours of the editor: keywords, strings, numbers, comments |

`config.example.toml` lists every icon kind, every colour name and every token kind in full. [themes.md](themes.md) explains what each colour paints.

## Keys

```toml
[keys]
preset = "default"

[keys.global]
run-at-cursor = ["ctrl+r", "f5"]

[keys.grid]
copy-cell = "ctrl+y"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `preset` | string | `default` | The key set applied before your own bindings. `default` is the only one there is |

Every other entry of `[keys]` is a scope, and every entry of a scope is one action bound to one chord or to a list of chords. A list gives an action several keys. The eight scopes are:

| Scope | Where its keys apply |
| --- | --- |
| `global` | Everywhere, unless the focused pane binds the same chord |
| `tree` | The object tree on the left |
| `grid` | The result grid |
| `editor` | The query editor |
| `plan` | The query plan tree |
| `document` | The tree that shows result rows as documents |
| `list` | Any list inside a card: the history, the saved queries, the palette |
| `dialog` | A card that asks a question, and the connection picker |

`alt`, `meta` and `option` are three names for the same modifier. A binding with an unknown action or an unknown chord is reported and not applied. [keys.md](keys.md) lists every action and its default chord.

## AI

```toml
[ai]
enabled              = true
default_provider     = "anthropic"
statement_timeout_ms = 30000

[ai.providers.anthropic]
model       = "claude-opus-5"
api_key_env = "ANTHROPIC_API_KEY"
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `enabled` | boolean | `true` | `false` turns off every AI feature: the chat cannot be opened, no AI action is bound, and no AI element is drawn |
| `default_provider` | `anthropic` or `openai` | `anthropic` | The provider the chat starts on. The palette switches provider for one session. An unknown name is reported |
| `statement_timeout_ms` | integer above zero | `30000` | Time limit for one statement of the chat, in milliseconds. It is separate from `statement_timeout_ms` on a profile |

One table per provider, `[ai.providers.anthropic]` and `[ai.providers.openai]`:

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `model` | string | `claude-opus-5`, `gpt-5` | The model id the chat sends its requests to |
| `api_key` | string | | The key itself. It is then a secret in the file, so prefer `api_key_env` |
| `api_key_env` | string | | The environment variable that holds the key. Used when `api_key` is unset |
| `base_url` | string | | The address of a proxy or a gateway, in place of the provider address |
| `base_url_env` | string | | The environment variable that holds that address |

A table under `[ai.providers]` with an unknown name is reported and not read. See [ai.md](ai.md) for the data each provider receives.

## MCP

```toml
[mcp]
profiles   = ["shop-dev"]
access     = "read-only"
row_limit  = 500
timeout_ms = 30000
```

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `profiles` | list of strings | empty | The profiles an agent may connect to. Empty serves none. A profile that is not listed cannot be reached, even by name |
| `access` | `off`, `read-only`, `read-write` or `full` | `read-only` | The most an agent may do on any profile. `mcp` on a profile can lower it, never raise it. A profile with `mode = "read-only"` stays at `read-only` |
| `row_limit` | integer above zero | `500` | Rows one read returns, and the most a caller may ask for |
| `timeout_ms` | integer above zero | `30000` | Time limit for one statement of an agent, in milliseconds |

See [mcp.md](mcp.md) for the tools and for how a write is confirmed.

## Other files the client writes

| Path | Contains |
| --- | --- |
| `$XDG_CONFIG_HOME/masume/themes/` | Custom themes, one file each |
| `$XDG_STATE_HOME/masume/history.sqlite` | Query history, saved queries, marks, open tabs |
| `$XDG_STATE_HOME/masume/mcp.log` | Every MCP tool call |
| `$XDG_STATE_HOME/masume/ai-chat.log` | Every AI chat request |

`XDG_CONFIG_HOME` and `XDG_STATE_HOME` relocate all of them. Each state file is created with owner-only permissions.
