# Configuration

All configuration is in one file: `$XDG_CONFIG_HOME/masume/config.toml`, which is `~/.config/masume/config.toml` on most systems. `config.example.toml` in the repository lists every key with its default value. Use it as a reference next to this page.

On the first run, masume creates a starter file if there is none. You can edit the file by hand, or let masume edit it. When you add or change a connection in the connection form, masume rewrites only that block and keeps everything else unchanged, including comments.

## A profile

One profile is one connection. The name after `profile.` is the name shown in the picker.

```toml
[profile.shop]
engine   = "postgres"
host     = "127.0.0.1"
port     = 5432
database = "shop"
user     = "ada"
auth     = "prompt"
env      = "dev"
mode     = "write"
```

| Key | Default | Meaning |
| --- | --- | --- |
| `engine` | `postgres` | See [engines.md](engines.md) for the list |
| `host` | required | The form defaults to `127.0.0.1`. Ignored for SQLite |
| `port` | per engine | The default port of the engine |
| `database` | required | The file path for SQLite. The database number for Redis. A leading `~` is expanded |
| `user` | required if the engine needs one | Ignored for SQLite. Optional for Redis and MongoDB |
| `auth` | `password`, or `command` if `password_command` is set | `password`, `command` or `prompt` |
| `env` | `dev` | `dev`, `test` or `prod`. `prod` colours the title bar red |
| `mode` | `write` | `write` or `read-only` |
| `confirm_writes` | `off` on dev, `delete` on test, `write` on prod | `off`, `delete` or `write` |
| `sslmode` | `prefer`, or `require` for engines that only accept TLS | `disable`, `allow`, `prefer`, `require`, `verify-ca`, `verify-full` |
| `statement_timeout_ms` | `0` | Time limit for one statement in milliseconds. `0` uses the server default |
| `keepalive_s` | `30` | Seconds between two connection checks. `0` disables the keepalive |
| `page_size` | `200` | Rows the grid loads per page. Must be above zero |
| `autocommit` | `true` | `false` keeps a transaction open until you commit or roll back |
| `mcp` | | The access level for agents on this profile. See [mcp.md](mcp.md) |
| `description` | | One line, shown in the picker |
| `ai_instructions` | | Context about this database for the AI model |

A profile that misses a required key is skipped and reported. The other profiles still load.

## Passwords

Keep the password out of the file.

```toml
auth = "prompt"                      # asked at connect time, kept in memory only
```

```toml
auth             = "command"
password_command = "pass db/shop"    # read from a password manager
```

```toml
auth         = "password"
password_env = "PGPASSWORD"          # read from an environment variable
```

A password command has 30 seconds to print its first line. The rest of its output is ignored.

You can store a password in the file with `password`, but then the file contains a secret. Run `chmod 600 ~/.config/masume/config.toml` if you do.

Redis takes a password without a user, because the server setting `requirepass` has no user. MongoDB takes a user only when authentication is enabled. A server without authentication refuses a connection that sends credentials.

## A command before the connection

A profile can run a command before the connection opens, for example an SSH tunnel. masume stops the command when the connection closes, so no tunnel process is left running.

```toml
[profile.shop-tunnel]
engine          = "postgres"
host            = "127.0.0.1"
port            = 15432
database        = "shop"
user            = "reader"
auth            = "prompt"
command         = "ssh -N -L 15432:db.internal:5432 jump.example.com"
command_timeout = 10
wait_for_port   = 15432
```

masume waits until `wait_for_port` accepts a connection, then connects. It waits at most `command_timeout` seconds.

## Interface

```toml
[ui]
icons               = "plain"    # plain or ascii
theme               = "system"   # see themes.md
hide_system_schemas = true       # false shows pg_catalog and other system schemas
```

`[ui.icon_glyphs]` sets the glyph for each kind of row. A glyph set to `""` disables that glyph. `[ui.colors]` overrides theme colours without a theme file. `config.example.toml` lists both in full.

## Keys

```toml
[keys]
preset = "default"

[keys.global]
run-at-cursor = ["ctrl+r", "f5"]
```

Every action has a default key binding, and every binding can be changed. `alt`, `meta` and `option` are names for the same modifier. See [keys.md](keys.md).

## AI and MCP

`[ai] enabled = false` disables every AI feature. `[mcp] profiles` is empty by default, so no profile is served. See [ai.md](ai.md) and [mcp.md](mcp.md).

## Other files the client writes

| Path | Contains |
| --- | --- |
| `$XDG_CONFIG_HOME/masume/themes/` | Custom themes, one file each |
| `$XDG_STATE_HOME/masume/history.sqlite` | Query history, saved queries, marks, open tabs |
| `$XDG_STATE_HOME/masume/mcp.log` | Every MCP tool call |
| `$XDG_STATE_HOME/masume/ai-chat.log` | Every AI chat request |

`XDG_CONFIG_HOME` and `XDG_STATE_HOME` relocate all of them. Each state file is created with owner-only permissions.
