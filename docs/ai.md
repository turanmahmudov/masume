# AI chat

Each connection has its own chat. The chat reads the database through the same nine tools that the [MCP server](mcp.md) provides to an agent. The chat cannot access a database that is not connected.

| Key | Opens |
| --- | --- |
| `Ctrl+I` | The chat |
| `Alt+I` | The chat, with the current statement in the input field |
| `Ctrl+H` | The chat, with a question about the error of the current statement |
| `Ctrl+O` | An earlier conversation |

## Disabling it

```toml
[ai]
enabled = false
```

This disables the feature completely. The chat cannot be opened, no AI action has a key binding, and nothing about AI is shown: not in the title bar, not in the query pane, not in the help screen, and not in the command palette. Nothing is sent to a provider, and no API key is read.

## Configuration

```toml
[ai]
enabled              = true          # false disables every AI feature
default_provider     = "anthropic"   # anthropic or openai
statement_timeout_ms = 30000         # time limit for a statement that the model runs

[ai.providers.anthropic]
model       = "claude-opus-5"
api_key_env = "ANTHROPIC_API_KEY"

[ai.providers.openai]
model       = "gpt-5"
api_key_env = "OPENAI_API_KEY"
```

masume reads the API key from the environment variable named in `api_key_env`. The starter config file already names `ANTHROPIC_API_KEY` and `OPENAI_API_KEY`. You can store a key in the file with `api_key`, but then the file contains a secret.

`base_url` and `base_url_env` point masume at a gateway instead of the provider.

## What the model receives before any question

This is the complete list. No table and no column is named.

| Sent | Example |
| --- | --- |
| Its role, and how to answer | "You are a database assistant built into a terminal database client" |
| The dialect | `Dialect: PostgreSQL` |
| The name of the connected database | `Connected database: shop` |
| The names of the other databases on the connection, up to a limit | `Other databases this connection can also see, named only: analytics, staging` |
| The nine tool definitions | The name, description and arguments of each tool |
| `ai_instructions` from the profile, if set | "every amount is in cents" |

The model is told that no table or column has been named. It must call `list_tables` to find out what exists, and `describe_table` before it writes a query against a table it has not seen.

So a first question costs at least one tool call. The schema is not uploaded in advance. The model reads only the parts it requests.

## What is sent with each question

- The question you typed.
- The contents of the editor in a fenced code block, truncated at 4000 characters. This is what makes "why does this fail" and "optimize this" work.
- The error of the last run, if the statement failed.

## What the model reads while it answers

Every read is a tool call. Each call is shown in the panel as a step, so you can see what the model read before it answered. For one question the model can make up to twenty-five tool calls.

| Tool | Reads |
| --- | --- |
| `list_tables` | The tables of a database, filtered by a pattern |
| `describe_table` | The columns, types and foreign keys of a table |
| `list_indexes` | The indexes of a table |
| `list_constraints` | The constraints of a table |
| `get_table_ddl` | The `CREATE TABLE` statement of a table |
| `list_relationships` | The foreign keys into and out of a table |
| `validate_query` | Whether a statement parses and its names resolve. It does not run the statement |
| `explain_query` | The plan of a statement. Estimated, or measured if the model asks for analyze |
| `run_query` | The rows returned by a statement |

`run_query` is the only tool that can write, and the only tool that returns table data.

## What the model cannot access

- Any database that is not connected. The chat runs on the one connection it belongs to.
- Any row of any table, until `run_query` returns one.
- The file system, the network, and the config file. The nine tools above are the complete interface.
- A write on a profile opened `read-only`. masume sets the session read-only on the server at connect time, so the server itself refuses the write.

## Before it writes

The chat asks for confirmation before it runs a statement that writes. The question appears in the chat panel, and the statement runs only after you answer yes.

`mode` and `confirm_writes` on the profile apply to the chat in the same way as to a statement typed in the editor. A profile opened `read-only` is set read-only on the server, so the chat cannot write to it at all.

`[mcp] access` does **not** apply to the chat. That setting is for the MCP server only. The chat uses the current connection, and reads what that connection can read.

## What is sent to the provider

Three things are sent to Anthropic or OpenAI: the question, everything the tools returned, and the contents of the editor.

Everything the tools returned includes table rows, once `run_query` has run. Do not use the chat on a database whose data must not leave your machine.

Every request is written to `$XDG_STATE_HOME/masume/ai-chat.log`, so you can review what was sent. Conversations are stored in the history file, and `Ctrl+O` reopens them. Statements that the model ran go into the same history that the screens read, so `Ctrl+T` shows them.

## Cost

A follow-up question in the same conversation is cheaper than the first. The system prompt is the same for every question on one connection, so the provider caches it. The provider is also asked to cache the results of earlier tool calls, so they are not sent again.
