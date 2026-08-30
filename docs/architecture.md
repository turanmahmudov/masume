# Architecture

masume is one binary with two front ends, and `cmd/masume` decides which one to start before it starts anything else. `internal/ui` draws the screen for a person; `internal/mcp` serves the protocol for an agent.

Neither front end owns a connection. Both reach a server through `internal/db`, and both write to the same history in `internal/hist`, which is why a statement an agent ran shows up in `Ctrl+T`.

## The packages

| Package | Holds |
| --- | --- |
| `internal/core` | What every tier above reads: the text form of a value, the engine list, JSON that keeps field order |
| `internal/cfg` | The config file: profiles, interface settings, key presets, themes, the pre-connect command |
| `internal/db` | The interface to a server: what one open connection provides |
| `internal/db/<engine>` | One driver per protocol, and a flavour per hosted service |
| `internal/db/engines` | The one place that pairs an engine with its driver and its capabilities |
| `internal/query` | The dialect: how a name is quoted, how a placeholder is written |
| `internal/query/syntax` | The lexer: tokens, keywords, top-level keyword search |
| `internal/query/statement` | What a statement is and does: split, kind, write risk, references, paging, sorting |
| `internal/query/editor` | Completion and the faults found without the server |
| `internal/query/build` | Statements the client writes: edits, filters, the object menu templates |
| `internal/query/result` | The plan reader, and the export and copy formats |
| `internal/query/language` | Reading a buffer that is not SQL |
| `internal/present` | Where things sit at a given width, how a value is drawn, and the ER diagram |
| `internal/app` | The state of the client: screens, tabs, connections, chats |
| `internal/ui` | Drawing: the theme, the keys, the screens and the panes |
| `internal/agent` | The nine tools, shared by the chat and the MCP server |
| `internal/ai` | One provider protocol each, for Anthropic and OpenAI |
| `internal/mcp` | The JSON-RPC server, `list_profiles`, and the access policy |
| `internal/hist` | The SQLite file: history, saved queries, marks, open tabs |

Nothing under `internal/query` or `internal/present` opens a socket. They take text and return text, which is why most of the tests need no server.

## One call at a time

A driver that speaks one socket refuses a second call while the first still holds it. `pgx` returns `conn busy`, and `go-sql-driver` drops the connection.

The screens read on their own goroutines. A refresh of the tree can ask for the columns of every open relation at once. Each connection therefore holds a queue in `internal/db/callqueue.go`, and a second call waits its turn.

The user connection and the catalog connection hold separate queues. A read of the tree never waits behind a query in the editor.

## Drawing

There is no layout pass. The frame is written directly, and the work goes into not repeating any of it: the escapes for a colour pair are built once and kept, a drawn line is measured by its plain runs so only the runs that are not plain reach the width library, and the rows of a result are written once and held until the result, the page or the masking changes.

Nothing a terminal acts on ever reaches the frame. `present.SafeText` replaces a control character with a space and a byte that is no text with a replacement mark. Every measure, cut, pad and wrap reads that text, so what is measured is what is drawn.

`internal/ui/frame_safety_test.go` holds the client to it. It draws a result of hostile values through every view and card. Each row must be exactly the screen width, and carry nothing a terminal acts on.

## Tests

Spec tests sit next to the code in `*_spec_test.go` and use the `_test` package. They reach a package only through what it exports.

Tests that need a real server sit behind the `integration` build tag and read connection URLs from the environment. `compose.yaml` starts those servers. Nothing in Go knows a container is involved.

See [../CONTRIBUTING.md](../CONTRIBUTING.md) for how to run them.
