<h1 align="center">升目 masume</h1>

<h3 align="center">A database client for the terminal</h3>

<p align="center">
  <em>Open a database, read it, and give an agent the same catalog.</em>
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

Schemas, tables, views, functions, sequences, types, triggers and roles. Open a table for its data, columns, indexes, constraints, DDL and plan.

![The object tree](vhs/shots/01-object-tree.png)

### Diagram

A table and the tables a foreign key joins to it.

![An ER diagram of a table and the tables a foreign key joins to it](vhs/shots/07-er-diagram.png)

### Query

Syntax highlighting. Completion out of the catalog. Faults marked in the gutter before the statement runs.

![The SQL editor with the completion menu open](vhs/shots/08-completion.png)

### Results

Sort, filter, follow a foreign key, freeze a column, mask a column. An edit is staged where it is made. Review the whole set as SQL, then run it.

![A result grid](vhs/shots/09-result.png)

### Explain

Plans as a tree, estimated or measured.

![A query plan drawn as a tree](vhs/shots/10-plan.png)

### Agents

A chat in the client, and an MCP server over stdio. The chat uses the open connection. `masume --mcp` opens the profiles named in the config. Both use the same tools, under limits, and both ask before they write.

---

## Features

**Multi-engine support:** PostgreSQL, MySQL, SQLite, Redis and MongoDB, and the hosted services built on them

**MCP server:** `masume --mcp` serves named profiles over stdio, capped per profile and for the whole server

**AI chat:** ask about a statement, diagnose the error it returned, or check its plan, over Anthropic or OpenAI

**Completion from the catalog:** names as they are typed, and faults marked in the gutter before the statement runs

**Every view of a table:** data, columns, indexes, constraints, DDL, the plan, and an ER diagram

**Staged edits:** insert, edit, duplicate and delete rows, review it all as SQL, then run it or discard it

**A stack of filters:** filter by a value, by many values, or by a `WHERE`, then pop one off

**Follow a foreign key** to the row it points at

**Query plans** as a tree, estimated or measured, or raw

**Named parameters:** a statement with `:name` opens a card and asks for the values

**Server activity:** what other sessions are running, and stop one, on an engine that lists them

**Redis and MongoDB:** a tab takes Redis commands or the calls of the Mongo shell

**Export and copy:** CSV, JSON, Markdown, `INSERT` statements, a row as JSON, a column as an `IN` clause

**History and saved queries,** with the open tabs restored after a restart

**Transactions by hand:** autocommit off, then begin, commit, roll back

**Read-only profiles:** the session is set read-only on the server, so nothing on it can write

**Multiple themes,** or take the colours from the terminal and follow them as they change

**AI is optional:** `[ai] enabled = false` removes every AI feature, key and mention

---

## Install

There is no tagged release and no archive to download yet. Each command below builds the head of `master`.

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
masume --mcp                  serve named profiles to an agent over stdio
masume --mcp --profile=NAME   serve that one profile alone
masume --mcp --check          open every named profile once, report, and exit
masume --version              write the version and exit
```

The config file is `$XDG_CONFIG_HOME/masume/config.toml`. History is `$XDG_STATE_HOME/masume/history.sqlite`. See [docs/mcp.md](docs/mcp.md) for the MCP server.

## Status

Early. There is no tagged release yet, and the config file may still change shape before `v1`. Linux and macOS, amd64 and arm64. There is no Windows build.

The tier 1 engines run against a real server in CI on every push. The rest speak a protocol masume already supports, and rest on the unit tests. [docs/engines.md](docs/engines.md) says which is which, and what each engine cannot do. Read it before pointing masume at anything that matters.

## First connection

The first run writes a starter config if none is there. Run `masume`, press `Ctrl+N` for the picker, then `n` to add a connection. Or write it by hand:

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

`auth = "prompt"` asks at connect and keeps the password in memory only.

## Docs

| Page | About |
| --- | --- |
| [Configuration](docs/configuration.md) | Profiles, passwords, interface, limits |
| [Engines](docs/engines.md) | Support tiers and capabilities |
| [Keys](docs/keys.md) | Every action and its chord |
| [Themes](docs/themes.md) | The built-in themes, and writing one |
| [AI chat](docs/ai.md) | Providers, tools, what leaves the machine |
| [MCP server](docs/mcp.md) | Tools, limits, confirming a write |
| [Architecture](docs/architecture.md) | How the source is laid out |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
