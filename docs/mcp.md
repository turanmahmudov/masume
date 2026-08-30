# MCP server

`masume --mcp` serves the profiles named in the config to an agent over JSON-RPC 2.0, one message per line, on stdin and stdout. An agent gets those connections under limits set in the same file. The MCP process opens its own connections. It does not share the ones the screen has open.

```sh
masume --mcp                    # serve every profile named under [mcp] profiles
masume --mcp --profile=shop     # serve one profile
masume --mcp --check            # open every named profile once, report, exit
```

A profile is connected the first time an agent calls a tool against it, not when the server starts.

## Nothing is served until it is named

This is the part to read first. `[mcp] profiles` is empty by default, and an empty list means **no profile is served**. A fresh install serves an agent nothing at all.

```toml
[mcp]
profiles = ["shop"]
```

`masume --mcp --check` reports what is open and why the rest is not.

## Registering it

### Claude Code

`--scope user` registers it once for every project:

```sh
claude mcp add --scope user masume -- masume --mcp
```

### opencode

`mcp` in `opencode.json`. The global file is `~/.config/opencode/opencode.json`, and a project file of the same name overrides it:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "masume": {
      "type": "local",
      "command": ["masume", "--mcp"],
      "enabled": true
    }
  }
}
```

### Cursor, Claude Desktop, and the rest

The same server as JSON. Cursor reads `~/.cursor/mcp.json`, and Claude Desktop reads its own config file:

```json
{
  "mcpServers": {
    "masume": { "command": "masume", "args": ["--mcp"] }
  }
}
```

### One profile per server

Serve one profile alone when the agent should not reach the rest:

```sh
claude mcp add --scope user masume-shop -- masume --mcp --profile=shop
```

Where one server holds more than one profile, every tool takes a `profile` argument. With `--profile=NAME`, that argument is dropped and every call uses that profile.

## The tools

| Tool | Returns |
| --- | --- |
| `list_profiles` | The connections this server opens, and what may be run on each |
| `list_tables` | The tables of a database, by pattern |
| `describe_table` | The columns, types and foreign keys of a table |
| `list_indexes` | The indexes of a table |
| `list_constraints` | The constraints of a table |
| `get_table_ddl` | The `CREATE TABLE` statement of a table |
| `list_relationships` | The foreign keys into and out of a table |
| `validate_query` | Whether a statement parses and its names resolve, without running it |
| `explain_query` | The plan of a statement, estimated or measured |
| `run_query` | The rows of a statement |

`list_profiles` is the one extra tool the MCP server has over the [chat](ai.md). Call it first. `run_query` is the only tool that can write, and the only one that returns rows of data. The rest read the catalog, parse a statement, or read a plan.

## What the agent is told, and what it is not

No table and no column is named up front. The agent is handed the tool definitions, the dialect, the name of the connected database, and the names of the other databases the connection can see. Everything else it has to ask for.

An agent therefore cannot reach:

- A profile that is not named under `[mcp] profiles`.
- A database the profile does not connect to.
- A row of any table, until `run_query` runs and its access level allows it.
- The file system, the network, or the config file.

## Limits

Two settings limit an agent, and the lower one wins.

```toml
[mcp]
profiles   = ["shop"]      # empty means no profile is served
access     = "read-only"   # off, read-only, read-write, full
row_limit  = 500           # the rows one read returns
timeout_ms = 30000         # how long one read may take
```

| `access` | An agent may |
| --- | --- |
| `off` | Nothing |
| `read-only` | Read rows and the catalog |
| `read-write` | Also `INSERT`, `UPDATE`, and `CREATE`, `ALTER`, `GRANT` or `REVOKE` an object |
| `full` | Also `DELETE`, `DROP` and `TRUNCATE` |

`access` defaults to `read-only`, so an agent reads and nothing more until the file says otherwise.

The level is weighed against what the statement would do, not against the word it opens with. These cases matter:

- **A write with no `WHERE` counts as the highest risk.** `update orders set paid = true` touches every row, so it needs `full` even though a qualified `UPDATE` needs only `read-write`.
- **A statement that creates a routine is a write,** whatever the body of the routine does later.
- **A MongoDB `runCommand` is weighed by the command in the document,** not by the call, so `db.runCommand({dropDatabase: 1})` needs `full`.
- **A Redis script counts as the highest risk.** `EVAL` can call any command, so it needs `full`. `EVAL_RO`, `EVALSHA_RO` and `FCALL_RO` are reads, because the server holds them to reads.
- **A `SET` or a `RESET` of a setting masume does not know is a write.** `set search_path`, `set time zone` and the timeouts are reads. `set default_transaction_read_only = off` is not, because a read that follows it could write. `begin read write` is a write for the same reason.
- **An executable comment is read as the statement it holds.** MySQL runs `/*! … */` and MariaDB also runs `/*M! … */`, so a DELETE written inside one is weighed as a DELETE.

A profile can lower itself, and never raise itself:

```toml
[profile.shop-prod]
mcp = "read-only"
```

A profile served at `read-only` also connects read-only. masume sets the session read-only on the server, so a write the words of a statement did not show is refused as well, such as a `SELECT` of a function that writes. A server that holds no read-only session of its own, TiDB among them, keeps only the check this client makes.

`mode = "read-only"` on a profile is stronger than any of this. It applies to every connection, not only to the ones an agent opens.

## Confirming a write

Where a profile has `confirm_writes` set, the server does not run the write on its own. It puts the question through the agent with `elicitation/create`, and waits for the answer.

An agent whose client does not support elicitation cannot run such a write.

## What it records

Every call goes to `$XDG_STATE_HOME/masume/mcp.log`. `tail -f` on it shows what an agent is asking for while it works.

Statements that ran also go into the history the screens read, so `Ctrl+T` shows anything an agent ran.

The log holds the rows the tools returned. See [../SECURITY.md](../SECURITY.md).

## Trying it by hand

The protocol is one JSON object per line, so a pipe is enough:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | masume --mcp
```
