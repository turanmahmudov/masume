# Themes

`Alt+O`, then lowercase `t`, opens the theme picker. The picker includes seventeen built-in themes, custom themes, and `System`.

Selecting a theme applies the theme and saves `[ui] theme` in the user configuration file. A save error appears in the client. [Usage](usage.md) covers the client controls.

Dark: Ayu Dark, Tokyo Night, Catppuccin Mocha, Gruvbox Dark, Dracula, Nord, One Dark, Monokai, GitHub Dark, Rosé Pine, Solarized Dark.

Light: Catppuccin Latte, GitHub Light, One Light, Gruvbox Light, Solarized Light, Rosé Pine Dawn.

The same setting is available in the configuration file:

```toml
[ui]
theme = "tokyonight"
```

The theme name is the file name without `.toml`. Ayu Dark is the default theme and the fallback parent.

## Using the terminal colours

```toml
[ui]
theme = "system"
```

masume uses the terminal background, foreground and sixteen palette colours.

masume checks terminal colours about every two seconds. Colour updates require terminal support for colour queries.

## A custom theme

A custom theme is a TOML file in `$XDG_CONFIG_HOME/masume/themes/`, normally `~/.config/masume/themes/`. The file name without `.toml` is the `[ui] theme` value. A custom file with a built-in name replaces that theme. `system` is reserved; masume reports and ignores `system.toml`.

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

`title` is the picker title, with the file name as the default. `appearance` is `dark` or `light`. An absent appearance inherits from the parent, with `dark` as the final fallback.

`extends` is the parent theme name. The parent can be a built-in or custom theme, but not `system`. An absent parent uses `ayu-dark`, except in `ayu-dark` itself. Child values override inherited palette entries, colours and syntax properties. Missing parents and inheritance cycles produce reports. The inheritance chain includes at most eight themes.

`[palette]` contains named hex colours. Palette values cannot reference other names. A colour can reference a palette entry or another colour, such as `border_focus = "blue"` or `border_focus = "accent"`.

## The colour names

| Name | Used for |
| --- | --- |
| `background` | The background of the whole screen |
| `panel` | A pane or a card |
| `header` | The row of column names |
| `zebra` | Every second row of the grid |
| `border` | A pane border |
| `border_focus` | The border of the focused pane |
| `selection` | A selected row or drag selection. Derived from `panel` and `text` when absent from the resolved theme |
| `text` | Normal text |
| `muted` | A hint or a label |
| `faint` | A line number or a separator line |
| `accent` | The main highlight |
| `accent_alt` | A second highlight |
| `accent_warm` | A third highlight |
| `on_accent` | Text on an accent background. Derived for contrast when absent from the resolved theme |
| `info` | An informational message |
| `success` | A statement that succeeded |
| `warning` | A warning |
| `danger` | A destructive action |
| `error` | A failure |
| `env_dev` | The development title bar. Defaults to `success` when absent from the resolved theme |
| `env_test` | The test title bar. Defaults to `warning` when absent from the resolved theme |
| `env_prod` | The production title bar. Defaults to `danger` when absent from the resolved theme |

`[ui.palette]`, `[ui.colors]` and `[ui.syntax]` in the user configuration override the selected theme. These overrides also apply after a theme change.

## Syntax highlighting

`[syntax]` contains editor highlight rules. Each token kind has a table. Missing properties inherit from the parent theme.

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

`fg` and `bg` are hex values or colour names. `bold`, `italic` and `underline` are boolean flags. `link` is another token kind. A linked rule replaces the inherited rule, then applies its own properties over the linked style.

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
