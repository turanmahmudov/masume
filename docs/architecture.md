# Architecture

masume is one binary with three front ends. `cmd/masume` parses arguments, builds connection targets, and starts the selected front end.

`internal/ui` is the terminal interface. `internal/mcp` is the MCP server. `internal/headless` runs statements without a screen and returns an exit code.

Front ends create and retain sessions through `internal/db`. Drivers hold the database connections. The terminal interface and MCP record query history through `internal/hist`; headless runs do not.

## Packages

| Package | Contents |
| --- | --- |
| `internal/core` | Shared values, engine properties, and JSON with field order |
| `internal/cfg` | Profiles, settings, keys, themes, project configuration, connection targets, and pre-connect commands |
| `internal/secret` | System keyring access and availability checks |
| `internal/db` | Session interfaces, connection wrappers, call queues, and shared database types |
| `internal/db/<engine>` | Protocol drivers and engine-specific behavior |
| `internal/db/engines` | Engine support registry and adapter creation |
| `internal/db/dbtest` | Shared integration fixtures and server connections from environment variables |
| `internal/query` | SQL dialects, identifiers, placeholders, and result types |
| `internal/query/syntax` | SQL tokens, keywords, and top-level searches |
| `internal/query/statement` | Statement splitting, classification, write risk, references, paging, and sorting |
| `internal/query/editor` | Completion and local diagnostics |
| `internal/query/build` | Generated SQL for edits, filters, and object actions |
| `internal/query/result` | Plans, exports, and copy formats |
| `internal/query/language` | Shared language interface and SQL implementation |
| `internal/present` | Layout, value formatting, safe text, and ER diagrams |
| `internal/app` | Application state, tabs, connections, and chats |
| `internal/ui` | Rendering, themes, keys, screens, and event handling |
| `internal/agent` | Tools shared by chat and MCP |
| `internal/ai` | Anthropic and OpenAI clients |
| `internal/mcp` | JSON-RPC server, profile listing, sessions, and access policy |
| `internal/load` | Import sampling, type detection, mapping, dry runs, and generated statements |
| `internal/writeplan` | Write previews, affected rows, cascades, and undo statements |
| `internal/detect` | Database discovery through Docker or Podman |
| `internal/headless` | Statement batches, output formats, row caps, and exit codes |
| `internal/hist` | SQLite storage for history, saved queries, tabs, favorites, recent schemas, chats, and catalog cache |

The language interface covers SQL and MongoDB syntax. The MongoDB implementation is in `internal/db/mongo`.

`internal/query` and `internal/present` do not open network connections. These packages process text and typed data, including tokens, diagnostics, plans, columns, rows, and layout structures. Export writers can write to supplied streams.

## Sessions

`internal/db/engines` selects an adapter for each profile. PostgreSQL-family engines share the PostgreSQL adapter; MySQL-family engines share the MySQL adapter. Engine variants provide their catalog behavior, capabilities, and plan handling.

The adapter layer wraps sessions with reconnect handling, statement timeouts, and read-only checks. Front ends add their own execution behavior. Headless batches do not use interface write confirmations, write previews, undo capture, or automatic transactions.

## Call queues

`internal/db/callqueue.go` permits one call at a time through each queue. Calls wait for the queue or stop when their context ends.

PostgreSQL and MySQL use separate main and catalog connections with separate queues. Catalog reads can run while an editor query holds the main connection. Server locks and resources can still delay either connection.

SQLite has one connection and a shared queue for user and catalog work. Catalog reads can wait for editor queries. MongoDB uses the driver's client and separate transaction synchronization.

## Rendering

The renderer writes frames directly. It caches color escape codes and rendered result rows. Result, page, and masking changes invalidate the relevant row cache.

`present.SafeText` replaces control characters with spaces and invalid bytes with replacement characters. Text measurement, cutting, padding, and wrapping use the safe text helpers. The renderer adds its own terminal escape sequences.

`internal/ui/frame_safety_test.go` renders hostile values through views and cards. The tests check row widths and control characters in rendered content.

## Tests

Spec tests use `*_spec_test.go` files and external `_test` packages. These tests exercise exported APIs. Other tests can use the package itself.

Server tests use the `integration` build tag and connection URLs from environment variables. `compose.yaml` provides the local test servers. `internal/db/dbtest` provides shared test setup.

See [CONTRIBUTING.md](../CONTRIBUTING.md) for the quality gate and integration matrix.
