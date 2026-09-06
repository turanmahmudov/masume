# AI chat

Each connection has its own AI chat. The AI chat uses that connection and ten database tools shared with the [MCP server](mcp.md).

## Opening the AI chat

These are the default keys. See [keys.md](keys.md) for other bindings.

| Key | Action |
| --- | --- |
| `Ctrl+I` | Open the AI chat without sending a question |
| `Alt+I` | Open the AI chat with the full editor buffer in the input field, with outer whitespace removed |
| `Ctrl+H` | Open the AI chat and immediately send a question about the editor error or failed check |
| `Ctrl+O` in the AI chat | Open the conversation list |

`Alt+I` does not send the input until submission. `Ctrl+H` sends immediately when the editor is not empty. The plan view also has an AI action that immediately sends the displayed raw plan.

Closing the AI chat panel does not stop a reply or reject a pending statement. The stop action cancels the reply, rejects a pending statement, and keeps the text already received. Cancellation does not undo a completed database operation.

In the AI chat, Enter submits a question; Shift+Enter or Alt+Enter inserts a newline. `Ctrl+X` stops the reply. `Ctrl+L` starts a new conversation. `Ctrl+O` lists stored conversations; `Ctrl+D` removes the selected conversation from that list. `Ctrl+J` inserts SQL from the last reply into the editor without execution.

## Configuration

```toml
[ai]
enabled              = true
default_provider     = "anthropic"
statement_timeout_ms = 30000

[ai.providers.anthropic]
model       = "claude-opus-5"
api_key_env = "ANTHROPIC_API_KEY"

[ai.providers.openai]
model       = "gpt-5"
api_key_env = "OPENAI_API_KEY"
```

`default_provider` is `anthropic` or `openai`. The starter config includes the environment variable names above.

`api_key_env` is the environment variable for the API key. `api_key` is a key stored directly in the config file and takes priority. A file with `api_key` contains a secret.

`base_url` is an optional gateway URL. `base_url_env` is the environment variable for that URL. The direct value takes priority. The gateway receives the API key and request content.

### Disabling the AI chat

```toml
[ai]
enabled = false
```

This disables the AI chat, its actions, and its interface elements. The disabled AI chat sends no provider requests. The config loader still reads the file, including configured API keys.

This setting does not disable MCP. `[mcp]` is separate, and external agents use their own providers and credentials.

## Request content

Opening the AI chat alone sends nothing. The first question includes system instructions, connection context, tool definitions, and the question.

| Context | Content |
| --- | --- |
| System instructions | The assistant role, answer format, and tool-use instructions |
| Dialect | The engine and statement language |
| Default namespace | The session's default schema or database |
| Other namespaces | Up to 300 distinct schema or database names from the loaded table catalog |
| Tools | The names, descriptions, and argument schemas of the ten tools |
| Profile instructions | `ai_instructions`, when set |

The generated catalog summary contains no table or column names. Instructions and editor text can contain those names. The model receives instructions to call `list_tables` and `describe_table` as needed. These instructions do not enforce tool use or require a minimum number of calls.

The prompt uses `Default schema or database` and `Other schemas or databases in the loaded catalog`. On PostgreSQL, these values are schema names within the connected database. They are not a list of PostgreSQL databases.

Each question can also include:

- The question text.
- Automatic editor context: the full buffer with outer whitespace removed, truncated to 4000 Unicode characters.
- The last execution error, when the editor is not empty and the last execution failed.
- Earlier user messages, assistant replies, and their saved editor contexts.
- The full raw plan when the plan action starts the question.

The 4000-character limit applies only to automatic editor text. It does not limit typed questions, the `Alt+I` input, error text, raw plans, or tool results.

An unchanged editor context is not attached again as a new message. The earlier context remains in the conversation and is sent again with its history.

Switching providers keeps the conversation. The next question sends the retained messages and contexts to the newly selected provider.

## Database tools

The panel displays tool steps during a reply. One question allows up to 25 provider rounds. Each round can request several tool calls.

| Tool | Result |
| --- | --- |
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

Tool results return to the provider in later rounds of the same reply. Later questions retain user and assistant text, but not a separate history of all tool calls and results.

`run_query` returns unmasked values. Grid masking does not remove data from AI requests. Undo statements can include old row values. Plans, defaults, constraints, and errors can also contain sensitive values.

MongoDB schema discovery reads up to 100 documents per collection description. The tool returns inferred field names and types, not the sampled documents themselves.

`validate_query` is a best-effort check, not proof that a statement is valid or safe. SQL adapters use preparation where available; MongoDB uses local diagnostics. Some unsupported checks and connection failures produce no diagnostic. An open transaction returns `checked: false`.

## Confirmation and limits

The AI chat asks before every `run_query` call, including reads. The AI chat does not use `confirm_writes` for these questions. It also asks before `explain_query` for a statement classified as a write.

`write_plan` adds a plan when the profile, engine, and statement support measurement. Available undo appears after execution, and `Alt+U` opens undo in the client. See [configuration.md](configuration.md#measuring-a-write).

The AI chat uses the profile's `mode` and database permissions. `[mcp] access`, `[mcp] row_limit`, and `[mcp] timeout_ms` do not apply to the AI chat.

The profile's `page_size` is the maximum returned rows for `run_query`. A tool argument can lower this limit. The row limit does not bound database work or changed rows.

`[ai] statement_timeout_ms` is the execution timeout for `run_query`. It does not cover provider requests, confirmation, schema calls, validation, explain calls, write-plan measurement, or undo capture. A profile timeout can apply separately to database operations. Cancellation can fail, and a statement can remain active after a timeout.

### Read-only protection

`mode = "read-only"` applies client checks to recognized writes. Engine protection differs:

| Engine | Additional protection |
| --- | --- |
| PostgreSQL family | A default read-only transaction setting on the main session |
| MySQL family, except TiDB | A read-only transaction setting on the main session |
| SQLite | Read-only opening for a disk file; no read-only file mode for `:memory:` |
| MongoDB | Client checks only; database permissions remain necessary |
| TiDB | An explicit read-only profile fails to connect |

These checks are not a sandbox. Database functions, extensions, and engine features can have effects beyond the statement's apparent operation.

`run_query` is not the only tool with possible effects. `explain_query` can execute a statement classified as a read when `analyze` is true. `plan_write` runs counts that evaluate the write predicate. Planning and validation can also invoke engine behavior.

The tools provide no general shell or file API. Database features can still reach files, networks, or other databases when database permissions allow access. Use database accounts with only the required permissions.

## Storage and sharing

The configured provider or gateway receives instructions, tool definitions, questions, editor contexts, retained messages, and tool results. Do not enable this sharing for data that must remain local.

The history file stores conversations and their editor contexts. It retains up to 50 conversations per profile and 100 messages per stored conversation. These storage limits do not cap the current conversation in memory.

`run_query` execution attempts also enter query history, including failures. `Ctrl+T` opens query history. History writes can fail.

`$XDG_STATE_HOME/masume/ai-chat.log` is a partial diagnostic log. Tool arguments and results are truncated after 500 Unicode characters. Questions and some events are not subject to that truncation. The log is not a complete request record or an audit log.

Rotation uses a 2,000,000-byte threshold and one `.1` backup. Logging and rotation are best effort. See [SECURITY.md](../SECURITY.md) for storage paths and protection limits.

## Caching and cost

Requests still transmit their content when caching is available. Anthropic requests include cache markers; OpenAI requests include a cache key. A cache hit and a lower price are not guaranteed. Provider prices and cache rules apply.
