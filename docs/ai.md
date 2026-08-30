# AI chat

Each connection gets its own chat, and that chat reads its database through the same nine tools the [MCP server](mcp.md) hands to an agent. The chat cannot reach a database that is not open.

| Chord | Opens |
| --- | --- |
| `Ctrl+I` | The chat |
| `Alt+I` | The chat, with the current statement in the field |
| `Ctrl+H` | The chat, asking what is wrong with the current statement |
| `Ctrl+O` | An earlier conversation |

## Turning it off

```toml
[ai]
enabled = false
```

That removes the feature rather than hiding it. The chat cannot be opened, no AI action holds a key, and nothing about AI is drawn or offered: not in the title bar, not on the query pane, not in the help screen, not in the command palette. Nothing is sent to a provider, and no API key is read.

## Setting it up

```toml
[ai]
enabled              = true          # false removes every AI feature
default_provider     = "anthropic"   # anthropic or openai
statement_timeout_ms = 30000         # how long a statement the model runs may take

[ai.providers.anthropic]
model       = "claude-opus-5"
api_key_env = "ANTHROPIC_API_KEY"

[ai.providers.openai]
model       = "gpt-5"
api_key_env = "OPENAI_API_KEY"
```

masume reads the key from the environment named in `api_key_env`. A first-run starter file already names `ANTHROPIC_API_KEY` and `OPENAI_API_KEY`. A key can be written into the file with `api_key`, but then the file holds a secret.

`base_url` and `base_url_env` point masume at a gateway rather than at the provider.

## What the model is told before any question

This is the whole of it. No table and no column is named.

| Sent | Example |
| --- | --- |
| What it is, and how to answer | "You are a database assistant built into a terminal database client" |
| The dialect | `Dialect: PostgreSQL` |
| The database the connection opened, by name | `Connected database: shop` |
| The other databases the connection can see, names only, up to a cap | `Other databases this connection can also see, named only: analytics, staging` |
| The nine tool definitions | name, description and arguments of each |
| `ai_instructions` from the profile, if any | "every amount is in cents" |

The model is told plainly that no table or column has been named, and that it has to call `list_tables` to find out what exists and `describe_table` before writing a query against a table it has not already seen.

So a first question costs at least one tool call. The schema is not uploaded, and the model reads only the parts it asks for.

## What goes with each question

- The question that was typed.
- The contents of the editor, in a fenced block, cut at 4000 characters. This is what makes "why does this fail" and "optimize this" work.
- The error of the last run, if the statement failed.

## What the model reads while it answers

Every read is a tool call, and each one is named in the panel as a step, so what it read is visible before it answered. For one question the model may call the nine tools up to twenty-five times.

| Tool | Reads |
| --- | --- |
| `list_tables` | The tables of a database, by pattern |
| `describe_table` | The columns, types and foreign keys of a table |
| `list_indexes` | The indexes of a table |
| `list_constraints` | The constraints of a table |
| `get_table_ddl` | The `CREATE TABLE` statement of a table |
| `list_relationships` | The foreign keys into and out of a table |
| `validate_query` | Whether a statement parses and its names resolve. It does not run it |
| `explain_query` | The plan of a statement. Estimated, or measured if it asks for analyze |
| `run_query` | The rows of a statement |

`run_query` is the only one that writes, and the only one that returns rows of data.

## What the model cannot reach

- Any database that is not open. The chat runs on the one connection it belongs to.
- Any row of any table, until `run_query` returns one.
- The file system, the network, and the config file. The nine tools above are the whole surface.
- A write on a profile opened `read-only`. masume sets the session read-only on the server at connect, so the server itself refuses it.

## Before it writes

The chat asks before it runs a statement that writes. The question stands in the chat panel, and the statement runs only after a yes.

`mode` and `confirm_writes` on the profile apply to the chat exactly as they apply to a statement typed in the editor. A profile opened `read-only` is set read-only on the server, so the chat cannot write on it at all.

`[mcp] access` does **not** govern the chat. That setting is for the MCP server. The chat runs on the connection that is already open, and reads what that connection reads.

## What leaves the machine

Three things go to Anthropic or OpenAI: the question, whatever the tools returned, and the contents of the editor.

Whatever the tools returned includes rows, once `run_query` has run. Do not open the chat on a database whose rows must stay on the machine.

Every request is written to `$XDG_STATE_HOME/masume/ai-chat.log`, so what was sent can be read back. Conversations are kept in the history file and reopen from `Ctrl+O`. Statements the model ran go into the same history the screens read, so `Ctrl+T` shows them.

## Cost

A follow-up question in the same conversation is cheaper than the first. The system prompt is identical for every question of one connection, so the provider caches it, and the provider is asked to keep what the earlier tool calls returned rather than being sent it again.
