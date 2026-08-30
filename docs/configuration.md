# Configuration

Everything lives in one file: `$XDG_CONFIG_HOME/masume/config.toml`, which is `~/.config/masume/config.toml` on most machines. `config.example.toml` in the repository lists every key with its default. Open it next to this page.

The first run writes a starter file if none is there. Edit the file by hand, or let masume edit it. When a connection is added or changed in the form, masume rewrites only that block and leaves everything else where it was, comments included.

## A profile

One profile is one connection. The name after `profile.` is what the picker lists.

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

| Key | Default | Means |
| --- | --- | --- |
| `engine` | `postgres` | See [engines.md](engines.md) for the list |
| `host` | required | The form starts at `127.0.0.1`. Not read for SQLite |
| `port` | per engine | The default port of the engine |
| `database` | required | The file for SQLite. The number for Redis. A leading `~` expands |
| `user` | required where the engine needs one | Not read for SQLite. Optional for Redis and MongoDB |
| `auth` | `password`, or `command` if `password_command` is set | `password`, `command` or `prompt` |
| `env` | `dev` | `dev`, `test` or `prod`. `prod` colours the title bar red |
| `mode` | `write` | `write` or `read-only` |
| `confirm_writes` | `off` on dev, `delete` on test, `write` on prod | `off`, `delete` or `write` |
| `sslmode` | `prefer`, or `require` where the engine only answers over TLS | `disable`, `allow`, `prefer`, `require`, `verify-ca`, `verify-full` |
| `statement_timeout_ms` | `0` | Milliseconds one statement may run. `0` leaves the timeout to the server |
| `keepalive_s` | `30` | Seconds between two checks that the server responds. `0` sends no keepalive |
| `page_size` | `200` | Rows the grid reads per page. A value must be above zero |
| `autocommit` | `true` | `false` holds a transaction open by hand |
| `mcp` | | Caps this profile for agents. See [mcp.md](mcp.md) |
| `description` | | One line, shown in the picker |
| `ai_instructions` | | What the model should know about this database |

A profile that omits a required key is skipped and reported, and the rest still load.

## Passwords

Keep the password out of the file.

```toml
auth = "prompt"                      # asked at connect, held in memory only
```

```toml
auth             = "command"
password_command = "pass db/shop"    # read from a password manager
```

```toml
auth         = "password"
password_env = "PGPASSWORD"          # read from the environment
```

A password command has 30 seconds to print the first line. The rest of its output is ignored.

A password can be written in the file with `password`, but then the file holds a secret. Run `chmod 600 ~/.config/masume/config.toml` if it does.

Redis takes a password with no user: the server `requirepass` names nobody. MongoDB takes a user only where authentication is on. A server without it refuses a connection that carries one.

## A command before the connection

A profile can run a command before the connection opens, such as an SSH tunnel. masume stops it when the connection closes, so no tunnel is left behind.

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

masume waits for `wait_for_port` to accept a connection before it connects, for up to `command_timeout` seconds.

## Interface

```toml
[ui]
icons               = "plain"    # plain or ascii
theme               = "system"   # see themes.md
hide_system_schemas = true       # false lists pg_catalog and the rest
```

`[ui.icon_glyphs]` sets the glyph for each kind of row. A glyph written as `""` turns that one off. `[ui.colors]` lays colours over the theme without writing a theme file. `config.example.toml` lists both in full.

## Keys

```toml
[keys]
preset = "default"

[keys.global]
run-at-cursor = ["ctrl+r", "f5"]
```

Every action has a default chord, and every one of them can be rebound. `alt`, `meta` and `option` name the same modifier. See [keys.md](keys.md).

## AI and MCP

`[ai] enabled = false` removes every AI feature. `[mcp] profiles` is empty by default, so no profile is served. See [ai.md](ai.md) and [mcp.md](mcp.md).

## Where else the client writes

| Path | Holds |
| --- | --- |
| `$XDG_CONFIG_HOME/masume/themes/` | Custom themes, one file each |
| `$XDG_STATE_HOME/masume/history.sqlite` | History, saved queries, marks, open tabs |
| `$XDG_STATE_HOME/masume/mcp.log` | Every agent call |
| `$XDG_STATE_HOME/masume/ai-chat.log` | Every chat request |

`XDG_CONFIG_HOME` and `XDG_STATE_HOME` move them all. Each state file is created for the owner alone.
