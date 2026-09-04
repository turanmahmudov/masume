# Themes

Press `Alt+O` then `T` to see the built-in themes and select one. There are eleven: Tokyo Night, Catppuccin Mocha and Latte, Gruvbox Dark and Light, Rosé Pine and Dawn, Nord, Dracula, Ayu Dark and Solarized Dark.

To set one in the config instead:

```toml
[ui]
theme = "tokyonight"
```

The name is the file name without `.toml`. Ayu Dark is the base theme. Every other theme inherits from it the keys it does not define.

## Using the terminal colours

```toml
[ui]
theme = "system"
```

masume uses the colours of the terminal: the background, the foreground and the sixteen palette colours.

It keeps them in sync. If you change the terminal theme while masume is running, masume updates within about two seconds.

## A custom theme

A custom theme is a TOML file in `$XDG_CONFIG_HOME/masume/themes/`. The file name without `.toml` is the name you set under `[ui]`. A file with the same name as a built-in theme replaces that theme.

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

`title` is the name shown in the picker. `appearance` is `dark` or `light`. `extends` copies every colour of a built-in theme first. The file then defines only what it changes.

`[palette]` holds named colours for reuse. A colour can refer to a palette entry or to another colour by name: `border_focus = "blue"` or `border_focus = "accent"`. A palette entry must be a hex value, not a name.

## The colour names

| Name | Used for |
| --- | --- |
| `background` | The background of the whole screen |
| `panel` | A pane or a card |
| `header` | The row of column names |
| `zebra` | Every second row of the grid |
| `border` | A pane border |
| `border_focus` | The border of the focused pane |
| `selection` | A selected row or a drag selection. Mixed from `panel` and `text` if not set |
| `text` | Normal text |
| `muted` | A hint or a label |
| `faint` | A line number or a separator line |
| `accent` | The main highlight |
| `accent_alt` | A second highlight |
| `accent_warm` | A third highlight |
| `on_accent` | Text on an accent background. Chosen for contrast if not set |
| `info` | An informational message |
| `success` | A statement that succeeded |
| `warning` | A warning |
| `danger` | A destructive action |
| `error` | A failure |
| `env_dev` | The title bar on dev. Same as `success` if not set |
| `env_test` | The title bar on test. Same as `warning` if not set |
| `env_prod` | The title bar on prod. Same as `danger` if not set |

To change a few colours without a theme file, set them under `[ui.colors]` in the config. They override the active theme.

## Syntax highlighting

`[syntax]` styles the editor. Each token kind has its own table. Ayu Dark defines the rules that every other theme inherits when it defines none.

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

`fg` and `bg` take a hex value or a colour name. `bold`, `italic` and `underline` are flags. `link` copies another highlight first, then the other keys apply on top.

| Kind | Applies to |
| --- | --- |
| `keyword` | `SELECT`, `FROM`, and other keywords |
| `type` | A type name |
| `string` | A quoted string |
| `comment` | A comment |
| `number` | A number |
| `identifier` | A name |
| `quoted` | A quoted identifier |
| `operator` | An operator |
| `parameter` | A `:name` placeholder |
| `problem` | An error found by the scanner |
| `bracket` | The bracket at the caret, and its matching bracket |
| `guide` | The indent guide of a line |
| `match` | A search match in the statement |
