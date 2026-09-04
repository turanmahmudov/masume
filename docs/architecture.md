# Architecture

masume is one binary with three front ends. `cmd/masume` reads the arguments and decides which one to start, and it builds the profile of a connection given on the command line. `internal/ui` is the terminal user interface. `internal/mcp` is the MCP server for AI agents. `internal/headless` is `masume run`, which draws nothing and answers with an exit code.

No front end owns a connection. Each one accesses a server through `internal/db`, and the two that keep a history write to the same one through `internal/hist`. This is why a statement run by an agent appears in the history under `Ctrl+T`.

## The packages

| Package | Contains |
| --- | --- |
| `internal/core` | Shared basics: the text form of a value, the engine list, JSON that keeps field order |
| `internal/cfg` | The config file: profiles, interface settings, key presets, themes, the pre-connect command. Also the URL, connection string and file path read from the command line |
| `internal/db` | The connection interface: what one open connection provides |
| `internal/db/<engine>` | One driver per protocol, and one variant per hosted service |
| `internal/db/engines` | The one place that maps an engine to its driver and its capabilities |
| `internal/query` | The dialect: how a name is quoted, how a placeholder is written |
| `internal/query/syntax` | The lexer: tokens, keywords, top-level keyword search |
| `internal/query/statement` | Statement analysis: splitting, kind, write risk, references, paging, sorting |
| `internal/query/editor` | Autocompletion, and the errors that can be detected without a server |
| `internal/query/build` | Statements the client generates: edits, filters, the object menu templates |
| `internal/query/result` | The plan parser, and the export and copy formats |
| `internal/query/language` | Parsing a buffer that is not SQL |
| `internal/present` | Layout at a given width, value formatting, and the ER diagram |
| `internal/app` | The application state: screens, tabs, connections, chats |
| `internal/ui` | Rendering: the theme, the keys, the screens and the panes |
| `internal/agent` | The nine tools shared by the chat and the MCP server |
| `internal/ai` | One provider client each, for Anthropic and OpenAI |
| `internal/mcp` | The JSON-RPC server, `list_profiles`, and the access policy |
| `internal/load` | Reading a data file into an import: sampling, kinds, mapping, the dry run, the statements |
| `internal/detect` | The databases running in a container on this machine, read from docker or podman |
| `internal/headless` | `masume run`: one statement, one format, one exit code, no screen |
| `internal/hist` | The SQLite file: history, saved queries, marks, open tabs |

Nothing under `internal/query` or `internal/present` opens a network connection. These packages take text and return text. This is why most of the tests need no server.

## One call at a time

A driver that uses one socket refuses a second call while the first call is still running. `pgx` returns `conn busy`, and `go-sql-driver` drops the connection.

The screens read on their own goroutines. A refresh of the tree can request the columns of every open relation at the same time. To handle this, each connection has a queue in `internal/db/callqueue.go`, and a second call waits until the first one finishes.

The user connection and the catalog connection have separate queues. A read of the tree never waits for a query from the editor.

## Rendering

There is no layout pass. The frame is written directly, and the optimization effort goes into avoiding repeated work. The escape codes for a colour pair are built once and cached. A line is measured by its plain runs, so only the runs with non-ASCII text go to the width library. The rows of a result are rendered once and cached until the result, the page or the masking changes.

No control sequences reach the frame. `present.SafeText` replaces a control character with a space, and an invalid byte with a replacement character. Every measure, cut, pad and wrap operation works on that text, so what is measured is what is drawn.

`internal/ui/frame_safety_test.go` enforces this. It renders a result with hostile values through every view and card. Each row must be exactly the screen width and must contain no control sequences.

## Tests

Spec tests sit next to the code in `*_spec_test.go` files and use the `_test` package. They use only the exported API of a package.

Tests that need a real server are behind the `integration` build tag and read connection URLs from the environment. `compose.yaml` starts those servers. No Go code knows that a container is involved.

See [../CONTRIBUTING.md](../CONTRIBUTING.md) for how to run them.
