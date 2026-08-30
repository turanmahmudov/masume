# Themes

Press `Alt+O` then `T` to see the themes that ship with masume and pick one. There are eleven: Tokyo Night, Catppuccin Mocha and Latte, Gruvbox Dark and Light, Rosé Pine and Dawn, Nord, Dracula, Ayu Dark and Solarized Dark.

To set one in the config instead:

```toml
[ui]
theme = "tokyonight"
```

The name is the file name without `.toml`. Ayu Dark is the fallback: every other theme inherits the keys it does not set.

## Following the terminal

```toml
[ui]
theme = "system"
```

masume takes its colours from the terminal: the background, the foreground and the sixteen palette colours.

It keeps following them. Change the terminal theme while masume is open and it changes with it, within about two seconds.

## A custom theme

A custom theme is a TOML file in `$XDG_CONFIG_HOME/masume/themes/`. The file name without `.toml` is the name to set under `[ui]`. A file of the same name as a shipped theme replaces it.

```toml
title = "My Theme"
appearance = "dark"
extends = "tokyonight"

[palette]
ink  = "#c0caf5"
blue = "#7aa2f7"

[colors]
background   = "#16161e"
panel        = "#1a1b26"
border_focus = "blue"
text         = "ink"
accent       = "blue"
```

`title` is the name the picker shows. `appearance` is `dark` or `light`. `extends` takes every colour of a shipped theme first, so a file only has to name what it changes.

`[palette]` holds names for reuse. A colour can point at a palette entry or at another colour by that name: `border_focus = "blue"` or `border_focus = "accent"`. A palette entry must be a hex value, not a name.

## The colour names

| Name | Used for |
| --- | --- |
| `background` | Behind everything |
| `panel` | A pane or a card |
| `header` | The row of column names |
| `zebra` | Every second row of the grid |
| `border` | A pane border |
| `border_focus` | The border of the focused pane |
| `selection` | A selected row or a drag. Mixed from `panel` and `text` if omitted |
| `text` | Normal text |
| `muted` | A hint or a label |
| `faint` | A line number or a rule |
| `accent` | The main highlight |
| `accent_alt` | A second highlight |
| `accent_warm` | A third highlight |
| `on_accent` | Text on an accent background. Chosen for contrast if omitted |
| `info` | A note |
| `success` | A statement that worked |
| `warning` | A warning |
| `danger` | A destructive action |
| `error` | A failure |
| `env_dev` | The title bar on dev. Follows `success` if omitted |
| `env_test` | The title bar on test. Follows `warning` if omitted |
| `env_prod` | The title bar on prod. Follows `danger` if omitted |

To change a few colours without writing a theme file, set them under `[ui.colors]` in the config. They lay over whichever theme is active.

## Syntax

`[syntax]` styles the editor. Each kind is a table of its own. Ayu Dark holds the rules every other theme inherits if it writes none.

```toml
[syntax]
keyword    = { fg = "accent", bold = true }
comment    = { fg = "muted", italic = true }
parameter  = { fg = "danger" }
problem    = { fg = "error", underline = true }
bracket    = { fg = "on_accent", bg = "accent_alt" }
guide      = { bg = "header" }
match      = { fg = "on_accent", bg = "accent_warm" }
```

`fg` and `bg` take a hex value or a colour name. `bold`, `italic` and `underline` are flags. `link` copies another highlight first, then the rest of the keys apply.

| Kind | Marks |
| --- | --- |
| `keyword` | `SELECT`, `FROM`, and the rest |
| `type` | A type name |
| `string` | A quoted string |
| `comment` | A comment |
| `number` | A number |
| `identifier` | A name |
| `quoted` | A quoted identifier |
| `operator` | An operator |
| `parameter` | A `:name` mark |
| `problem` | A fault the scanner found |
| `bracket` | The bracket at the caret, and the one that closes it |
| `guide` | The indent step of a line |
| `match` | What a search of the statement found |
