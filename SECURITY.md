# Security

## Reporting a vulnerability

Do not open a public issue for a vulnerability. Report vulnerabilities privately through [GitHub security advisories](https://github.com/turanmahmudov/masume/security/advisories/new).

A first reply can take up to seven days. Confirmed vulnerabilities receive fixes in a release. The advisory credits the reporter unless the reporter requests otherwise.

Reports must not contain passwords, private host names, or real data.

## Supported versions

Before the first tagged release, fixes target `master`. After releases begin, only the latest release receives fixes. There is no long-term support branch.

## Stored data

| Data | Location |
| --- | --- |
| Global profiles, settings, secret commands, and optional AI API keys | `$XDG_CONFIG_HOME/masume/config.toml` |
| Project profiles and saved queries | The nearest `.masume.toml` in or above the working directory |
| Query history, saved queries, tabs, editor buffers, marks, recent schemas, catalog cache, AI chats and their editor contexts | `$XDG_STATE_HOME/masume/history.sqlite` |
| SQLite state files | `history.sqlite-wal` and `history.sqlite-shm` beside the history file |
| Partial MCP diagnostics | `$XDG_STATE_HOME/masume/mcp.log` and `mcp.log.1` |
| Partial AI chat diagnostics | `$XDG_STATE_HOME/masume/ai-chat.log` and `ai-chat.log.1` |
| Remembered database passwords | The operating system keyring, under service `masume` and the profile name |
| Exported query results | The selected export path |

Without XDG overrides, the config directory is `~/.config/masume` and the state directory is `~/.local/state/masume`.

History can contain statement literals, errors, filter values, and unsent editor text. Stored chats can contain returned data in assistant replies and editor contexts. The catalog cache contains table, object, and role information.

AI chat storage retains up to 50 conversations per profile and 100 messages per conversation. These limits do not cap the active conversation in memory. Deletion does not guarantee secure erasure from SQLite files, backups, providers, or external clients.

masume creates new state directories with mode `0700`. It applies mode `0600` to the history file and existing WAL and SHM files when opening history. New config files and logs use mode `0600`. Terminal exports also use mode `0600`.

Existing directory permissions do not automatically become restrictive. Log permission changes are best effort. These permissions are not encryption and do not protect against the same operating system user or an administrator.

### Diagnostic logs

The AI and MCP logs are partial diagnostics, not complete request records or audit logs. Tool arguments and results are truncated after 500 Unicode characters. Questions, errors, and some other events can be longer.

Logs can contain unmasked values, statement text, errors, and fragments of returned rows or undo statements. A truncated result can still contain secrets.

Rotation uses a 2,000,000-byte threshold and one `.1` backup. Large individual entries and concurrent processes can exceed this threshold. masume ignores logging and rotation failures.

`run_query` execution attempts enter query history, including failed attempts. Refused calls and other database tool operations do not all enter query history. History writes are also best effort.

## Credentials

A literal database `password` in a profile config is ignored. masume does not save database passwords in profile files. Existing literal passwords remain secrets in those files even when ignored.

| Source | Behavior |
| --- | --- |
| `auth = "prompt"` | Request a password in the terminal client |
| `auth = "password"` with `password_env` | Read the password from the environment variable |
| `auth = "keyring"` | Read the password under service `masume` and the profile name |
| `auth = "command"` with `password_command` | Run a shell command and read the first output line |
| `auth = "secret"` with `secret` and `secret_ref` | Run the named store command and read the first output line |

The terminal client can prompt when a required environment or keyring password is unavailable. MCP and headless execution have no terminal prompt fallback. Command and store failures report errors.

The password prompt can store a successful password in the keyring when requested. Without storage, the password remains in process memory for connection use.

Keyring entries have no host, database, username, or project identifier. Profiles with the same name can reuse the same password, including profiles in different projects.

AI credentials have different storage rules. `api_key` stores an AI API key directly in the global config. `api_key_env` is the environment variable for the key; a direct key takes priority. Restrict access to config files, environment variables, secret commands, and backups.

### Project files

masume reads the nearest `.masume.toml` from the working directory upward. The file supplies profiles and queries. Global profiles replace project profiles with the same name as whole profiles; other project profiles remain available.

Project profiles cannot contain `command`, `password_command`, `password_env`, `secret`, or `secret_ref`. masume skips profiles with those keys. Project files cannot supply global AI, MCP, interface, or secret-store settings.

Project profiles can use `auth = "keyring"` and change connection targets. A keyring password can therefore reach a target from a project file. Review project profiles before connecting.

MCP loads project profiles from its process working directory. The global MCP list contains profile names, not fixed project paths or host identities. An allowed name can match a project profile unless a global profile replaces that name.

## Database protection

masume is not a sandbox. Statement classification and confirmation cannot prove that an operation has no side effects. Use database accounts with only the required permissions.

`mode = "read-only"` rejects recognized writes in the client. Additional protection depends on the engine:

| Engine | Additional protection |
| --- | --- |
| PostgreSQL family | Default read-only transactions on the main session |
| MySQL family, except TiDB | Read-only transactions on the main session |
| SQLite | Read-only opening for disk files; client checks alone for `:memory:` |
| MongoDB | Client checks only; no server read-only session setting |
| TiDB | Explicit read-only profiles fail to connect |

MCP requests read-only mode for effective read-only access when the engine supports that profile mode. On TiDB, MCP read-only access alone uses client classification without a server read-only session.

Database functions, extensions, and engine features can reach files, networks, or other databases within their permissions. A statement classified as a read can have side effects. Server read-only settings also have engine-specific limits.

`run_query` is not the only tool with possible effects. `EXPLAIN ANALYZE` executes statements classified as reads. Write-plan counts evaluate predicates, including functions. Validation and planning can also invoke engine behavior.

`validate_query` is a best-effort check, not a safety check. Missing diagnostics do not prove validity or prevent execution errors.

Profile timeouts depend on engine and operation support. AI and MCP execution timeouts apply to `run_query`, not every database tool or confirmation wait. Cancellation can fail or leave a statement active. A timeout does not prove that a write had no effect.

TLS behavior depends on the engine and `sslmode`. Verify certificate and host checks for the selected engine in [engines.md](docs/engines.md).

## Confirmation

The editor and MCP use `confirm_writes` for statements classified as writes. `off` asks nothing. `delete` asks for deletes, destructive statements, and writes classified as affecting every row. `write` asks for every classified write.

Unset defaults are `off` on `dev`, `delete` on `test`, and `write` on `prod`. Environment colors are visual labels, not access restrictions.

The AI chat asks before every `run_query`, including reads, regardless of `confirm_writes`. It also asks before explain calls classified as writes. Read analyze calls and other tool operations do not all require confirmation.

Headless runs do not use write confirmation, write plans or undo. A nonzero exit can follow a successful write with failed output. See [headless execution](docs/headless.md#writes-and-access).

MCP sends required confirmation through client elicitation when available. Without elicitation, required confirmation fails unless `confirm_writes = "agent"` accepts a valid plan token.

A plan token requires supported write-plan measurement and is single-use, profile-bound, statement-bound, and valid for ten minutes. Statement matching ignores outer whitespace after splitting. The token stores no row snapshot and proves no human approval. A client with elicitation support ignores supplied tokens and still receives required confirmation.

Write plans and undo are incomplete for some statements and engine features. Available undo statements can contain old row values. See [mcp.md](docs/mcp.md) for token requirements and result fields.

## Data sharing

The AI chat sends content to the selected provider or configured gateway. A gateway receives the API key and request content. Sent content can include:

- System instructions, profile `ai_instructions`, connection namespaces, and tool definitions.
- Questions, previous user messages, assistant replies, and retained editor contexts.
- Automatic editor text, truncated to 4000 Unicode characters, and the last execution error.
- Full question text, including the full editor buffer prefilled by `Alt+I` after outer whitespace removal.
- Full raw plans from the plan action.
- Tool results, including schema definitions, errors, unmasked rows, and undo statements with old row values.

`Ctrl+I` opens the AI chat without sending. `Alt+I` prefills the input without sending. `Ctrl+H` immediately sends an error-help question when the editor is not empty. The plan action also sends immediately.

The editor limit applies only to automatic editor text. It does not limit question text, errors, plans, or tool results. Switching providers keeps the conversation and sends retained messages and contexts on the next question.

MongoDB schema discovery reads up to 100 documents per collection description. The tool returns inferred field names and types. It does not return the sampled documents.

Grid masking does not redact AI or MCP results. Caching does not prevent transmission. Provider storage, retention, and prices follow provider rules.

The external MCP client receives tool results and can share those results with its own provider or gateway. MCP initialization does not include the AI chat's system prompt or schema context.

`[ai] enabled = false` disables the AI chat only. `[mcp]` settings are separate. Neither setting erases stored conversations or data already sent.

Configured password commands, secret stores, and connection commands can also start processes and contact external services. Network traffic is not limited to database servers and AI providers.

See [ai.md](docs/ai.md) for AI chat behavior and [mcp.md](docs/mcp.md) for agent access limits.
