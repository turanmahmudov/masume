# MCP server

`masume --mcp` exposes the profiles listed in the config to an AI agent. It speaks JSON-RPC 2.0, one message per line, on stdin and stdout. The agent gets those connections with the access limits set in the same file. The MCP process opens its own connections. It does not share the connections of a running client.

```sh
masume --mcp                    # serve every profile listed under [mcp] profiles
masume --mcp --profile=shop     # serve one profile
masume --mcp --check            # connect to every listed profile once, print a report, exit
```

A profile connects the first time an agent calls a tool on it, not when the server starts.

## No profile is served until it is listed

Read this section first. `[mcp] profiles` is empty by default, and an empty list means **no profile is served**. A fresh install exposes nothing to an agent.

```toml
[mcp]
profiles = ["shop"]
```

`masume --mcp --check` reports which profiles are connected and why the others are not.

## Registering the server

### Claude Code

`--scope user` registers it once for every project:

```sh
claude mcp add --scope user masume -- masume --mcp
```

### opencode

Add it under `mcp` in `opencode.json`. The global file is `~/.config/opencode/opencode.json`. A project file with the same name overrides it:

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

### Cursor, Claude Desktop, and others

The same server as JSON. Cursor reads `~/.cursor/mcp.json`. Claude Desktop reads its own config file:

```json
{
  "mcpServers": {
    "masume": { "command": "masume", "args": ["--mcp"] }
  }
}
```

### One profile per server

Serve a single profile when the agent must not access the others:

```sh
claude mcp add --scope user masume-shop -- masume --mcp --profile=shop
```

When one server serves more than one profile, every tool takes a `profile` argument. With `--profile=NAME`, that argument is removed and every call uses that profile.

## The tools

| Tool | Returns |
| --- | --- |
| `list_profiles` | The connections this server serves, and the access level of each |
| `list_tables` | The tables of a database, filtered by a pattern |
| `describe_table` | The columns, types and foreign keys of a table |
| `list_indexes` | The indexes of a table |
| `list_constraints` | The constraints of a table |
| `get_table_ddl` | The `CREATE TABLE` statement of a table |
| `list_relationships` | The foreign keys into and out of a table |
| `validate_query` | Whether a statement parses and its names resolve, without running it |
| `explain_query` | The plan of a statement, estimated or measured |
| `plan_write` | What a write would do, measured without running it |
| `run_query` | The rows returned by a statement |

`list_profiles` is the one tool that the MCP server has and the [chat](ai.md) does not. Call it first. `run_query` is the only tool that can write, and the only tool that returns table data. The other tools read the catalog, parse a statement, or read a plan.

## What the agent is told, and what it is not

No table and no column is named in advance. The agent receives the tool definitions, the dialect, the name of the connected database, and the names of the other databases on the connection. It must request everything else.

An agent cannot access:

- A profile that is not listed under `[mcp] profiles`.
- A database that the profile does not connect to.
- A row of any table, until `run_query` runs and the access level allows it.
- The file system, the network, or the config file.

## Access limits

Two settings limit an agent, and the lower one applies.

```toml
[mcp]
profiles   = ["shop"]      # empty means no profile is served
access     = "read-only"   # off, read-only, read-write, full
row_limit  = 500           # maximum rows one read returns
timeout_ms = 30000         # time limit for one read
```

| `access` | An agent can |
| --- | --- |
| `off` | Do nothing |
| `read-only` | Read rows and the catalog |
| `read-write` | Also `INSERT` and `UPDATE`, and `CREATE`, `ALTER`, `GRANT` or `REVOKE` an object |
| `full` | Also `DELETE`, `DROP` and `TRUNCATE` |

`access` defaults to `read-only`, so an agent can only read unless the config says otherwise.

The level is checked against the effect of the statement, not against its first keyword. These cases matter:

- **A write without `WHERE` is treated as the highest risk.** `update orders set paid = true` affects every row, so it needs `full`. An `UPDATE` with a `WHERE` needs only `read-write`.
- **A statement that creates a routine is a write,** regardless of what the routine body does later.
- **A MongoDB `runCommand` is checked by the command in the document,** not by the call. So `db.runCommand({dropDatabase: 1})` needs `full`.
- **A Redis script is treated as the highest risk.** `EVAL` can call any command, so it needs `full`. `EVAL_RO`, `EVALSHA_RO` and `FCALL_RO` are reads, because the server restricts them to reads.
- **A `SET` or `RESET` of a setting that masume does not recognize is a write.** `set search_path`, `set time zone` and the timeouts are reads. `set default_transaction_read_only = off` is a write, because a read after it could write. `begin read write` is a write for the same reason.
- **An executable comment is checked as the statement inside it.** MySQL executes `/*! … */` and MariaDB also executes `/*M! … */`, so a DELETE inside one is checked as a DELETE.

A profile can lower its own level, but never raise it:

```toml
[profile.shop-prod]
mcp = "read-only"
```

A profile served at `read-only` also connects read-only. masume sets the session read-only on the server, so a write that is not visible in the statement text is refused too, for example a `SELECT` of a function that writes. A server without a read-only session mode, such as TiDB, is protected only by the check in this client.

`mode = "read-only"` on a profile is stronger than all of the above. It applies to every connection, not only to the connections an agent opens.

## Confirming a write

When a profile has `confirm_writes` set, the server does not run the write on its own. It sends the confirmation question to the agent with `elicitation/create`, and waits for the answer.

An agent whose client does not support elicitation cannot run such a write. The first line of the log says which kind of client is connected:

```
> initialize claude-code: can ask its user
> initialize some-agent: cannot ask its user, so a write that confirms cannot run
```

`write_plan` on the profile measures the write first, and the question then carries what it lands on: the rows it was counted at, the columns it assigns, the relations it reaches through a trigger or a foreign key, and whether the write can be undone. See [configuration.md](configuration.md#measuring-a-write).

Where the plan kept an undo, the answer of `run_query` carries an `undo` list: the statements that reverse the write, read inside its own transaction. They are not run. An agent can report them, and a person can run them.

## A client that cannot ask

Elicitation is optional, and several agent clients do not implement it. On one of those, a profile with `confirm_writes` set can run no write at all: masume has no way to reach you.

`confirm_writes = "agent"` is for that case, and only for that case. **A client that can show a dialog is always shown the dialog**, whatever the agent sends: masume issues no token to such a client, and refuses one that arrives anyway. The token is a way to reach you where nothing else can, never a way around the question.

On a client that cannot be asked it works like this:

1. The agent calls `plan_write` with the statement. masume measures it and answers with the plan and a `token`. Nothing is written.
2. The agent shows you the plan and asks, in its own words.
3. If you agree, it calls `run_query` with that `token` as `plan_token`, and the write runs without a dialog.

A token is bound to one statement on one connection, is taken one time, and goes stale after ten minutes. A statement that differs by one character does not match it. A profile set to `write` or `delete` issues no token at all, and neither does any profile on a client that shows dialogs.

This is weaker than a dialog: your yes reaches masume through the model. It is stronger than `confirm_writes = "off"`, which asks nothing and shows nothing. Use it where the client cannot be asked, and `write` everywhere else.

`plan_write` works on every client. On one that shows a dialog it is still useful: the agent can read the plan before it decides to ask at all.

## Logging

Every call is written to `$XDG_STATE_HOME/masume/mcp.log`. Run `tail -f` on it to watch the requests of an agent while it works.

Statements that ran also go into the history that the client reads, so `Ctrl+T` shows every statement an agent ran.

The log contains the rows that the tools returned. See [../SECURITY.md](../SECURITY.md).

## Testing it manually

The protocol is one JSON object per line, so a pipe is enough:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | masume --mcp
```
