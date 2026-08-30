# Security

## Reporting a vulnerability

Do not open a public issue for a vulnerability.

Report it privately through [GitHub security advisories](https://github.com/turanmahmudov/masume/security/advisories/new). This is a small project with one maintainer, so expect a first reply within seven days rather than the same day. A confirmed report is fixed in a release, and the advisory credits the reporter unless they ask otherwise.

Do not put a password, a private host name, or real data in a report.

## Supported versions

Only the latest release gets fixes. There is no long-term branch.

## What masume stores

masume opens a database and keeps what it needs to do that. It sends nothing anywhere except to the servers in the config, and to the AI provider if one is set.

| Data | Path |
| --- | --- |
| Profiles, and a password if the file holds one | `$XDG_CONFIG_HOME/masume/config.toml` |
| History, saved queries, marks, open tabs | `$XDG_STATE_HOME/masume/history.sqlite` |
| Every agent call | `$XDG_STATE_HOME/masume/mcp.log` |
| Every chat request | `$XDG_STATE_HOME/masume/ai-chat.log` |

Be aware of what that adds up to. The history holds every statement that ran, so it holds any value written into one, and the two logs hold what an agent and a model received, which means the rows the tools returned. None of it is sent anywhere, and any of it can be deleted at any time. The state directory and every file in it are created for the owner alone, so another user of the same machine cannot read them. An export is written the same way, because it holds the rows of a read.

## Connections

- `auth = "prompt"` asks at connect and keeps the password in memory only.
- `auth = "command"` with `password_command` reads it from a password manager.
- `password_env` reads it from the environment.
- If a password must live in the file, run `chmod 600 ~/.config/masume/config.toml`.
- `env = "prod"` colours the title bar red.
- `mode = "read-only"` refuses every write. The setting that holds a session read-only cannot be turned off from the editor: a `SET` of a setting masume does not know counts as a write, and so does `begin read write`.
- `confirm_writes = "write"` asks before every write. `"delete"` asks before a delete only. Unset, the default follows `env`: `off` on dev, `delete` on test, `write` on prod.
- `sslmode = "verify-full"` checks the certificate and the host name. `"require"` encrypts but checks neither.
- `statement_timeout_ms` cancels a slow statement.

## Agents

An agent on the MCP server can read every table the connection can read.

- `[mcp] profiles` lists the profiles the server opens. Leave out any an agent should not reach.
- `[mcp] access` caps the whole server: `off`, `read-only`, `read-write` or `full`.
- `mcp` on a profile lowers that profile further. No profile can go above `access`.
- A profile served at `read-only` is opened read-only on the server, so a write the words of a statement did not show is refused too.
- When a profile asks about a write, the client puts the question through the agent of the user.
- Every call goes to `mcp.log`, so agent activity can be read back.

## AI chat

`[mcp]` does not govern the chat. It runs on the open connection and reads what that connection reads. `mode` and `confirm_writes` still apply, and it asks in the panel before it runs a write. A profile opened `read-only` is set read-only on the server at connect, so nothing on it can write.

The question, the schema the model read, and the rows the tools returned all go to the configured provider. Do not open the chat on a database whose rows must stay on the machine.

The client reads the API key from the environment by default.
