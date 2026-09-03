# Security

## Reporting a vulnerability

Do not open a public issue for a vulnerability.

Report it privately through [GitHub security advisories](https://github.com/turanmahmudov/masume/security/advisories/new). This is a small project with one maintainer, so a first reply can take up to seven days. A confirmed report is fixed in a release. The advisory credits the reporter unless they ask otherwise.

Do not include a password, a private host name, or real data in a report.

## Supported versions

Only the latest release receives fixes. There is no long-term support branch.

## What masume stores

masume stores only what it needs to connect to a database and to keep its own history. It sends data only to the database servers in the config, and to the AI provider if one is configured.

| Data | Path |
| --- | --- |
| Profiles, and a password if you store one in the file | `$XDG_CONFIG_HOME/masume/config.toml` |
| Query history, saved queries, marks, open tabs | `$XDG_STATE_HOME/masume/history.sqlite` |
| Every MCP tool call | `$XDG_STATE_HOME/masume/mcp.log` |
| Every AI chat request | `$XDG_STATE_HOME/masume/ai-chat.log` |

Be aware of what these files contain. The history holds every statement that ran, including any literal values in the statement text. The two logs hold everything an agent or a model received, including the rows the tools returned. None of it is sent anywhere, and you can delete any of it at any time. The state directory and every file in it are created with owner-only permissions, so other users on the same machine cannot read them. Export files get the same permissions, because they contain query results.

## Connections

- `auth = "prompt"` asks for the password at connect time and keeps it in memory only.
- `auth = "command"` with `password_command` reads the password from a password manager.
- `password_env` reads the password from an environment variable.
- If a password must be stored in the file, run `chmod 600 ~/.config/masume/config.toml`.
- `env = "prod"` colours the title bar red.
- `mode = "read-only"` refuses every write. The session is set read-only on the server, and this cannot be undone from the editor. A `SET` of a setting that masume does not recognize is treated as a write. `begin read write` is treated as a write too.
- `confirm_writes = "write"` asks before every write. `"delete"` asks before a delete only. When unset, the default depends on `env`: `off` on dev, `delete` on test, `write` on prod.
- `sslmode = "verify-full"` checks the certificate and the host name. `"require"` encrypts but checks neither.
- `statement_timeout_ms` cancels a statement that runs longer than the limit.

## Agents

An agent connected to the MCP server can read every table that the connection can read.

- `[mcp] profiles` lists the profiles the server exposes. Leave out every profile an agent must not access.
- `[mcp] access` sets the maximum access level for the whole server: `off`, `read-only`, `read-write` or `full`.
- `mcp` on a profile lowers the access level for that profile. No profile can exceed `access`.
- A profile served at `read-only` is also opened read-only on the server. So a write that is not visible in the statement text, for example a function that writes, is refused too.
- When a profile requires confirmation for a write, the server sends the confirmation question to the user through the agent.
- Every call is logged to `mcp.log`, so you can review what an agent did.

## AI chat

`[mcp]` does not apply to the chat. The chat uses the current connection and can read whatever that connection can read. `mode` and `confirm_writes` still apply, and the chat asks for confirmation in its panel before it runs a write. A profile opened `read-only` is set read-only on the server at connect time, so writes are impossible.

The question, the schema information the model requested, and the rows the tools returned are all sent to the configured provider. Do not use the chat on a database whose data must not leave your machine.

By default the client reads the API key from an environment variable.
