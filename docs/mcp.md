# MCP server

`masume --mcp` provides database tools to an external agent over standard input and output. The protocol is JSON-RPC 2.0, with one message per line.

The process opens its own database connections. It does not share connections with a running terminal client. Each profile connects on its first database tool call.

```sh
masume --mcp                    # Serve allowed profiles
masume --mcp --profile=shop     # Serve one allowed profile
masume --mcp --check            # Check allowed profiles and exit
```

Normal protocol output goes to stdout. Startup reports and errors go to stderr, usually with the prefix `masume mcp: `. Check result lines and some argument help lack that prefix.

## Allowed profiles

`[mcp] profiles` is empty by default. An empty list exposes no profile.

```toml
[mcp]
profiles = ["shop"]
```

`--profile=shop` restricts the server to `shop`; it does not bypass the allowed list or access settings. Connection tools then omit the `profile` argument. Without this option, every connection tool requires that argument, even when only one profile is allowed.

`list_profiles` returns only profiles with effective access above `off`. The result includes names, engines, targets, configured databases, environments, descriptions, and access levels. Password-prompt requirements can appear as `unreachable`.

`--check` attempts connections and table discovery for those same profiles. It does not test disabled profiles or validate every operation. It returns status 1 when no profile is available or a checked connection fails. A successful check returns status 0.

### Config files

The server reads the global config and the nearest `.masume.toml` above its working directory, including that directory. A global profile replaces a project profile with the same name as a whole. Other project profiles join the profile list.

Only the global config supplies `[mcp]` and `[ai]` settings. A project file cannot add itself to `[mcp] profiles`. An allowed profile name can still match a project profile from the server's working directory.

The working directory belongs to the process launched by the agent client. Check that directory and the resolved profile names before registration. Project profiles permit `auth = "keyring"`. Keyring entries use the service `masume` and profile name only, without a host or project identifier.

The server cannot display a password prompt. Available sources include environment variables, keyring entries, password commands, and named secret stores. Missing credentials that require a terminal prompt leave the profile unavailable. A literal profile `password` in a config file is ignored.

`[ai] enabled = false` disables only the AI chat. It does not disable MCP. MCP does not use the AI chat's provider credentials.

## Registering the server

### Claude Code

`--scope user` registers the server for every project:

```sh
claude mcp add --scope user masume -- masume --mcp
```

### opencode

The server entry belongs under `mcp` in `opencode.json`. The global file is `~/.config/opencode/opencode.json`. Project settings merge with global settings and override matching keys.

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

Cursor reads `~/.cursor/mcp.json`. Claude Desktop uses its own config file. These clients use this registration form:

```json
{
  "mcpServers": {
    "masume": { "command": "masume", "args": ["--mcp"] }
  }
}
```

### One profile per server

```sh
claude mcp add --scope user masume-shop -- masume --mcp --profile=shop
```

`list_profiles` and `--check` then cover that profile alone, subject to the access settings.

## Tools and results

| Tool | Result |
| --- | --- |
| `list_profiles` | Allowed profiles and their effective access levels |
| `list_tables` | Table or collection names, kinds, and available row estimates |
| `describe_table` | Columns or fields, types, defaults, choices, and foreign keys where available |
| `list_indexes` | Index names and definitions |
| `list_constraints` | Constraint names and definitions |
| `get_table_ddl` | Table or collection creation statements |
| `list_relationships` | Foreign keys into and out of tables |
| `validate_query` | Best-effort statement diagnostics |
| `explain_query` | An estimated or analyzed plan |
| `plan_write` | Available row counts, assigned columns, trigger names, foreign-key effects, and undo information |
| `run_query` | Execution status, returned rows, and optional undo statements |

`list_profiles` is the only tool absent from the [AI chat](ai.md).

`initialize` returns protocol information, server information, and tool capabilities. It does not send the AI chat's system prompt, namespace summary, or profile `ai_instructions`. `tools/list` returns tool definitions. The agent must request profile and catalog information through tools.

Tool answers use text content. Successful answers generally contain JSON text. Important result fields are:

- A nonempty `error` field sets the MCP result's `isError: true`.
- Tool failures outside the shared handler can return plain error text with `isError: true`.
- A denied `run_query` returns `ran: false` and `reason`, without `isError: true`.
- An execution failure returns `ran: true` and `error`. `ran: true` means execution was attempted, not that execution succeeded.
- Available undo statements appear in `undo`. An unavailable undo can have `undo_reason`, but that field is optional.
- Invalid protocol requests and unknown tool names can return JSON-RPC errors instead of tool results.

`validate_query` is a best-effort check. SQL adapters use preparation where available; MongoDB uses local diagnostics. `checked: true` with no problem is not proof of validity. Some unsupported checks and connection failures produce no diagnostic. An open transaction returns `checked: false`.

## Notebook tools

`list_notebooks` returns the notebooks of the project and of the user: the name, the title, the origin, the cell count and how many cells write. `read_notebook` returns one notebook with its run policy and every cell: id, kind, title and text.

Both tools are read-only and run no cell. A notebook whose front matter names profiles is listed only where one of those profiles is served. There is no `run_notebook`: an agent runs a cell by sending its text through `run_query`, where the access level and the confirmation apply. See the [notebook guide](notebooks.md).

## Access limits

```toml
[mcp]
profiles   = ["shop"]
access     = "read-only"
row_limit  = 500
timeout_ms = 30000
```

`access` is the maximum MCP access level. The default is `read-only`.

| `access` | Allowed statement classes |
| --- | --- |
| `off` | No profile tools |
| `read-only` | Statements classified as reads, plus catalog tools |
| `read-write` | Also ordinary writes, including `INSERT`, filtered `UPDATE`, `CREATE`, `ALTER`, `GRANT`, and `REVOKE` |
| `full` | Also `DELETE`, `DROP`, `TRUNCATE`, and writes classified as affecting every row |

The classifier examines statement structure, but cannot prove all database effects. Examples include:

- An `UPDATE` without `WHERE` needs `full`, even when the statement changes no rows.
- Creating a routine is a write, regardless of its body.
- MongoDB `runCommand` uses the command inside its document for classification.
- Unrecognized `SET` and `RESET` settings are writes. Recognized settings such as `search_path`, time zones, and timeouts can be reads.
- Disabling read-only transactions and `BEGIN READ WRITE` are writes.
- MySQL and MariaDB executable comments use the statement inside the comment for classification.

A profile can lower the global access level:

```toml
[profile.shop-prod]
mcp = "read-only"
```

`mode = "read-only"` also limits effective MCP access to read-only. This profile mode applies outside MCP too.

For effective read-only access, MCP requests a read-only connection when the engine supports that profile mode:

- PostgreSQL-family adapters set default read-only transactions on the main session.
- MySQL-family adapters, except TiDB, set read-only transactions on the main session.
- SQLite opens disk files read-only. The `:memory:` database has client checks without a read-only file mode.
- MongoDB uses client checks, without a server read-only session setting.
- TiDB keeps client classification checks when only MCP access is read-only. An explicit `mode = "read-only"` profile fails to connect.

These settings are not a sandbox. Database permissions remain necessary, including restrictions on functions, extensions, files, networks, and other databases.

`run_query` is not the only tool with possible effects. `explain_query` with `analyze: true` executes statements classified as reads. Statements classified as writes receive only estimated plans, subject to access and confirmation checks.

`plan_write` runs counts that evaluate the write predicate. Functions in a predicate can have side effects. Validation and planning can invoke engine behavior without a `run_query` call.

`row_limit` is the maximum returned rows for `run_query`. A call can request fewer rows. This limit does not bound changed rows, database work, catalog results, or plan results.

`timeout_ms` is the execution timeout for `run_query`, including reads and writes. It does not cover connection setup, confirmation, catalog calls, validation, explain calls, write-plan measurement, or undo capture. A profile timeout can apply separately to database operations. Cancellation can fail, and a statement can remain active after a timeout.

## Confirming a write

MCP uses the profile's `confirm_writes` setting after its access check. Statements classified as reads need no confirmation.

| `confirm_writes` | Confirmation requirement |
| --- | --- |
| `off` | None |
| `delete` | Deletes, destructive statements, and writes classified as affecting every row |
| `write` | Every statement classified as a write |
| `agent` | Every classified write, with token support for clients without elicitation |

When unset, the defaults are `off` on `dev`, `delete` on `test`, and `write` on `prod`.

A client with elicitation support receives `elicitation/create` when confirmation is required. An accepted answer must contain `confirm: true`. The answer timeout is 120 seconds. A refusal or timeout leaves the statement unrun.

`write_plan` adds available measurements to the question. Measurement is best effort and does not prove the complete effects of a write. See [configuration.md](configuration.md#measuring-a-write).

When undo is available, `run_query` returns reversal statements read within the write transaction. The server does not execute those statements automatically. Undo statements can contain old row values.

## Clients without elicitation

A client without elicitation cannot run a statement that requires confirmation under `write` or `delete`. Statements that need no confirmation remain available.

`confirm_writes = "agent"` permits a plan token when the client cannot use elicitation. Token issuance requires:

- `write_plan` enabled on the profile.
- An engine with write-plan support.
- One recognized write statement with a target found in the connection catalog.
- A supported target form, such as a simple `UPDATE`, `DELETE`, `TRUNCATE`, or recognized `INSERT`.
- No unsupported target alias or multi-target form. Joined updates and deletes are not measured.

MongoDB has no write-plan support. Unsupported statements return `measured: false` without a token. A measured plan can still have missing counts or incomplete effect information.

An example configuration for token confirmation is:

```toml
[mcp]
profiles = ["shop"]
access = "full"

[profile.shop]
engine = "postgres"
host = "127.0.0.1"
database = "shop"
user = "writer"
auth = "password"
password_env = "SHOP_PASSWORD"
mode = "write"
confirm_writes = "agent"
write_plan = "count"
```

The password must be available in `SHOP_PASSWORD`. `full` permits destructive statement classes as well as ordinary writes.

The agent flow is:

1. Call `plan_write` with the statement.
2. Present the plan and request human approval.
3. After approval, call `run_query` with the returned `token` as `plan_token`.

masume does not verify that a human saw the plan or approved the statement. A token is not proof of human consent.

A token belongs to one profile and statement within one server process. It is single-use and expires after ten minutes. Matching ignores outer whitespace after statement splitting; changes inside the statement do not match. A mismatched token remains available for its original statement.

A token stores no row snapshot and does not lock the planned rows. Data can change before execution. The access check still applies when the token returns.

A client with elicitation support receives no token. A supplied token is ignored, and required confirmation still uses elicitation. Profiles with `write` or `delete` issue no tokens.

## Data and logs

The external agent receives tool results and can send those results to its provider or gateway. The external client has its own storage and sharing rules.

Results are unmasked. Schema definitions, plans, errors, rows, and undo statements can contain sensitive values. MongoDB collection descriptions read up to 100 documents and return inferred field names and types.

`$XDG_STATE_HOME/masume/mcp.log` is a partial diagnostic log. Tool arguments and results are truncated after 500 Unicode characters. Errors and other events can be longer.

Rotation uses a 2,000,000-byte threshold and one `.1` backup. Logging and rotation are best effort, not an audit record. The initialization log reports elicitation support, but its refusal text does not describe the `agent` token exception.

`run_query` execution attempts also enter the shared query history, including failures. `Ctrl+T` opens that history in the terminal client. Other tool operations do not all enter query history, and history writes can fail.

See [SECURITY.md](../SECURITY.md) for credential sources, project restrictions, and local storage protection.

## Testing manually

The protocol accepts one JSON object per line:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | masume --mcp
```
