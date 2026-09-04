<h1 align="center">升目 masume</h1>

<h3 align="center">A database client for the terminal</h3>

<p align="center">
  <em>Browse and query a database in the terminal. Let an AI agent use the same connections.</em>
</p>

<p align="center">
  <a href="https://github.com/turanmahmudov/masume/actions/workflows/check.yml"><img src="https://github.com/turanmahmudov/masume/actions/workflows/check.yml/badge.svg" alt="check"></a>
  <img src="https://img.shields.io/badge/go-1.27+-00ADD8.svg?logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/license-Apache--2.0-green.svg" alt="License">
</p>

<p align="center">
  <code>go install github.com/turanmahmudov/masume/cmd/masume@master</code>
</p>

<p align="center">
  <img src="vhs/demo.gif" alt="masume" />
</p>

---

### Browse

The object tree lists schemas, tables, views, functions, sequences, types, triggers and roles. Select a table to see its data, columns, indexes, constraints, DDL and query plan.

![The object tree](vhs/shots/01-object-tree.png)

### Diagram

An ER diagram shows a table and the tables it is linked to by foreign keys.

![An ER diagram of a table and its related tables](vhs/shots/07-er-diagram.png)

### Query

The editor has syntax highlighting and autocompletion based on the database catalog. Errors are marked in the gutter before you run the statement.

![The SQL editor with the completion menu open](vhs/shots/08-completion.png)

### Results

Sort, filter, follow a foreign key, freeze a column, or mask a column. Edits are staged in the grid. Nothing is written until you review the changes as SQL and run them.

![A result grid](vhs/shots/09-result.png)

### Explain

Query plans are displayed as a tree, with estimated or measured costs.

![A query plan drawn as a tree](vhs/shots/10-plan.png)

### Agents

masume has a built-in AI chat and an MCP server over stdio. The chat uses the current connection. `masume --mcp` connects to the profiles listed in the config. Both use the same tools with the same access limits, and both ask for confirmation before a write.

---

## Features

**Multiple engines:** PostgreSQL, MySQL, SQLite, Redis and MongoDB, plus hosted services based on them

**MCP server:** `masume --mcp` exposes selected profiles to an agent over stdio, with an access level per profile and for the whole server

**AI chat:** ask about a statement, its error, or its query plan. Supports Anthropic and OpenAI

**Autocompletion from the catalog:** table and column names are suggested as you type. Errors are marked in the gutter before the statement runs

**All table details:** data, columns, indexes, constraints, DDL, query plan, and an ER diagram

**Staged edits:** insert, edit, duplicate and delete rows. Review the changes as SQL, then run or discard them

**Filters:** filter by one value, by several values, or by a `WHERE` clause. Filters stack, and one key removes the last one

**Follow a foreign key** to the referenced row

**Query plans** as a tree with estimated or measured costs, or as raw text

**Named parameters:** a statement with `:name` placeholders opens a form for the values

**Server dashboard:** `Alt+O A` opens an operations view that refreshes every two seconds: the other sessions and what they are running, connections against the limit, transactions and write ahead log per second, the cache hit rate, replication lag, and the sessions waiting for a lock drawn as a tree of who waits for whom. With `pg_stat_statements` installed it also lists the statements the server spends the most time in. `Enter` opens a session's statement in a tab, `x` stops it, `Ctrl+D` ends the session

**Redis and MongoDB:** a query tab accepts Redis commands or MongoDB shell syntax

**Export and copy:** CSV, JSON, Markdown, `INSERT` statements, one row as JSON, or one column as an `IN` clause

**Import:** a CSV or JSON file into a table, or into a table the import makes. A file picker offers the files it can read, types are read from the file, columns are mapped by name, and a dry run reports the rows that cannot be written before any of them are

**Query history and saved queries.** Open tabs are restored after a restart

**Project connections and shared queries:** a `.masume.toml` committed next to the code provides the connections and the queries of a project, found from the working directory upward. Clone the repository, run `masume`, and the development database is in the picker. Your own config file overrides it

**Write plans:** before a write runs, masume counts the rows it lands on, and lists the columns it assigns, the relations it reaches through a trigger, and the foreign keys that refuse it. It also reads the rows the write changes, inside the transaction of that write, and `Alt+U` undoes it afterwards. The chat and an agent over MCP are measured the same way, and an agent is handed the undo with its result

**Manual transactions:** disable autocommit, then begin, commit or roll back

**Passwords never reach the config file:** a `password` key in any file masume reads is ignored and reported. `auth = "keyring"` keeps the password in the keyring of the machine, and masume offers to put a typed password there once the server accepts it. `auth = "secret"` reads one reference out of a store you declare, so 1Password, Bitwarden, Vault, SOPS, `pass` or a script of your own all work through the same two lines

**Read-only profiles:** the session is set read-only on the server, so writes are impossible

**Eleven built-in themes,** or use the terminal colours. When the terminal theme changes, masume updates

**AI is optional:** `[ai] enabled = false` disables all AI features and hides them from the interface

---

## Install

There is no tagged release yet, so there are no prebuilt binaries. Each command below builds the latest commit on `master`.

- **Go 1.27 or later:** `go install github.com/turanmahmudov/masume/cmd/masume@master`
- **mise:** `mise use -g "go:github.com/turanmahmudov/masume/cmd/masume@master"`

### From source

```sh
git clone https://github.com/turanmahmudov/masume.git
cd masume
mise run install
```

## Usage

```
masume                        open the client
masume run STATEMENT          run one statement, write the result, and exit
masume URL                    open one connection, for example postgres://you@host/shop
masume FILE                   open one SQLite file, for example ./notes.db
masume DSN                    open one connection string, for example "host=db dbname=shop"
masume --profile NAME         open one profile of the config file
masume --detect               list the databases running in a container on this machine
masume --mcp                  run the MCP server for the configured profiles
masume --mcp --profile=NAME   run the MCP server for one profile
masume --mcp --check          connect to every configured profile once, print a report, and exit
masume --version              print the version and exit
```

A connection given on the command line needs no profile in the config file. With no argument, masume opens `$DATABASE_URL` if the shell exports it.

```sh
masume postgres://reader@db.internal:5432/shop?sslmode=verify-full
masume "host=db.internal dbname=shop user=reader"
masume ./notes.db
masume --profile shop-prod
masume --detect
```

`--detect` asks docker, or podman where there is no docker, for the containers that run on this machine. A container whose image is a database masume supports, and which publishes the port that database listens on, becomes a row in the connection picker. The user, the database and the password come from the environment of the container, so most local containers open with one `Enter` and nothing typed.

masume asks for the password if the connection carries none. The connection is not written to the config file, so masume offers to write it when you quit. Answer `y` to save it as a profile and `n` to quit without it. To save it earlier, or under another name, press `Ctrl+N` for the picker, then `e` and `Ctrl+S`.

### Without a screen

`masume run` runs one statement and writes the result to stdout, over the same profiles, timeouts and access limits as the client. This is how a Makefile, a container or a CI job uses masume.

```sh
masume run -p shop-prod -f json 'select count(*) from orders'
masume run -p shop -e ./reports/daily.sql --param day=2026-09-02
masume run -p shop --explain 'select * from orders where status = :status' --param status=paid
masume run ./notes.db -f csv 'select * from notes limit 100000' > notes.csv
echo 'select 1' | masume run -p shop -e -
```

Formats are `table` (the default), `csv`, `json` and `markdown`. A statement without a limit of its own returns one page, `page_size` on the profile, the same as the client; the run reports on stderr that the result is longer. A statement that bounds itself, with a `LIMIT`, returns every row it asks for, read a batch at a time, and a result larger than memory still reaches the stream. `--limit ROWS` bounds a run whose statement carries no limit. The exit codes are: `0` every statement ran, `1` the server refused one, `2` the connection could not be opened, `3` the profile is read-only and the statement writes. See `masume run --help` and [docs/headless.md](docs/headless.md).

### For a team

A repository can hold a `.masume.toml` next to its code, with the connections of the project and the queries the team keeps:

```toml
[profile.dev]
engine   = "postgres"
host     = "127.0.0.1"
database = "shop"
user     = "shop"
env      = "dev"

[query.recent-orders]
sql         = "select * from orders order by created_at desc limit 50"
description = "the newest 50 orders"
```

masume reads the first such file it finds, starting in the working directory and walking up. The connections appear in the picker marked `project`, and the queries appear under `Ctrl+Q`. A profile of your own config file replaces a project profile of the same name.

A project file holds the server address. It holds no way to reach a secret: `password`, `password_command`, `password_env`, `command`, `secret` and `secret_ref` are all refused there. `auth = "prompt"` and `auth = "keyring"` both work. A project file also cannot set your theme, your keys, your icons, or the profiles an agent reaches. See [docs/configuration.md](docs/configuration.md).

The config file is `$XDG_CONFIG_HOME/masume/config.toml`. The history file is `$XDG_STATE_HOME/masume/history.sqlite`. See [docs/mcp.md](docs/mcp.md) for the MCP server.

## Status

The project is in an early stage. There is no tagged release yet, and the config file format can change before `v1`. It builds on Linux and macOS, for amd64 and arm64. There is no Windows build.

The tier 1 engines are tested against a real server in CI on every push. The other engines use the protocol of a tier 1 engine and are covered by unit tests only. [docs/engines.md](docs/engines.md) lists the tiers and the limitations of each engine. Read it before you use masume on a production database.

## First connection

The quickest first connection is a URL on the command line:

```sh
masume postgres://ada@127.0.0.1:5432/shop
```

For a connection you open again, write a profile. On the first run, masume creates a starter config file if there is none. Run `masume`, press `Ctrl+N` to open the connection picker, then press `n` to add a connection. Or write the profile by hand:

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

`auth = "prompt"` asks for the password at connect time and keeps it in memory only.

## Docs

| Page | About |
| --- | --- |
| [Configuration](docs/configuration.md) | Every config key, its type, its default and what it does |
| [Engines](docs/engines.md) | Support tiers and capabilities |
| [Keys](docs/keys.md) | Every action and its key binding |
| [Themes](docs/themes.md) | Built-in themes, and how to write a custom one |
| [AI chat](docs/ai.md) | Providers, tools, what is sent to the provider |
| [MCP server](docs/mcp.md) | Tools, limits, confirming a write |
| [Without a screen](docs/headless.md) | `masume run` for scripts and CI |
| [Architecture](docs/architecture.md) | How the source is organized |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
