# Keys

Press `?` inside masume for the help screen, or `Ctrl+K` for the command palette, which searches actions by name. This page lists the same actions with the names used in the config file.

Every action belongs to a scope. The scope determines which pane handles the key. Every action has a key binding in the default preset, so every action is reachable without the palette.

To rebind an action, add it to `config.toml`. A line you write replaces the preset binding. Actions you do not list keep the preset binding.

```toml
[keys]
preset = "default"

[keys.global]
run-at-cursor = ["ctrl+r", "f5"]

[keys.grid]
sort-column = ["o"]
```

An action bound to `[]` has no key binding.

`alt`, `meta` and `option` are names for the same modifier. The preset and this page use `alt`. The help screen uses `meta`. A space in a binding means a sequence of key presses: `alt+p s` is Alt+P, then S.

## Cards

`[keys.dialog]`

| Action | Key |
| --- | --- |
| `answer-no` | `n` |
| `answer-yes` | `y` |
| `apply-changes` | `ctrl+y` |
| `close` | `escape` |
| `copy-value` | `ctrl+a` or `y` |
| `delete-connection` | `d` |
| `discard-changes` | `x` |
| `edit-connection` | `e` |
| `insert-ai-sql` | `ctrl+j` |
| `keep-all-values` | `a` |
| `keep-only-value` | `o` |
| `list-secondary` | `ctrl+d` |
| `new-ai-chat` | `ctrl+l` |
| `new-connection` | `n` |
| `next-turn` | `ctrl+n` |
| `open-in-new-tab` | `alt+return` |
| `prettify-json` | `ctrl+f` |
| `previous-turn` | `ctrl+p` |
| `replace-in-statement` | `ctrl+r` |
| `run-with-values` | `ctrl+r` |
| `save-cell` | `ctrl+s` |
| `save-form` | `ctrl+s` |
| `scroll-back` | `pageup` |
| `scroll-forward` | `pagedown` |
| `set-default` | `ctrl+d` |
| `set-empty` | `ctrl+e` |
| `set-null` | `ctrl+l` |
| `show-ai-chats` | `ctrl+o` |
| `stop-ai-reply` | `ctrl+x` |
| `test-connection` | `ctrl+t` |
| `toggle-value` | `space` |
| `write-export` | `ctrl+s` |

## Finding and replacing

One key does both. `Alt+F` opens the `find` field. From there:

- Type a search term and press Enter to highlight every match. `F3` and `Shift+F3` move between the matches.
- Or press `Ctrl+R` instead of Enter. The field changes to `replace … with`, and its title shows the search term. The text you enter next replaces **every** match in one step. The status bar reports the number of replacements.

In both cases you type the search term once. `Ctrl+Z` undoes the whole replace in one step, regardless of the number of matches.

Matching is plain text substring matching, so `id` also matches `customer_id`. The title shows the search term, so you can check it before the replace runs.

## Editor

`[keys.editor]`

| Action | Key |
| --- | --- |
| `caret-down` | `down` or `shift+down` |
| `caret-left` | `left` or `shift+left` |
| `caret-line-end` | `end` or `shift+end` |
| `caret-line-start` | `home` or `shift+home` |
| `caret-page-down` | `pagedown` or `shift+pagedown` |
| `caret-page-up` | `pageup` or `shift+pageup` |
| `caret-right` | `right` or `shift+right` |
| `caret-text-end` | `ctrl+end` or `ctrl+shift+end` |
| `caret-text-start` | `ctrl+home` or `ctrl+shift+home` |
| `caret-up` | `up` or `shift+up` |
| `caret-word-left` | `ctrl+left` or `ctrl+shift+left` |
| `caret-word-right` | `ctrl+right` or `ctrl+shift+right` |
| `comment-lines` | `alt+c` |
| `delete-back` | `backspace` |
| `delete-forward` | `delete` |
| `delete-word-back` | `ctrl+backspace` or `alt+backspace` |
| `delete-word-forward` | `ctrl+delete` or `alt+delete` |
| `find-in-statement` | `alt+f` |
| `format-sql` | `ctrl+d` |
| `indent-lines` | `alt+]` |
| `next-match` | `f3` |
| `next-problem` | `f8` |
| `open-line` | `return` |
| `outdent-lines` | `alt+[` |
| `paste-text` | `ctrl+v` |
| `previous-match` | `shift+f3` |
| `redo-edit` | `ctrl+shift+z` or `alt+z` |
| `select-all` | `ctrl+a` |
| `undo-edit` | `ctrl+z` |

## Global

`[keys.global]`

| Action | Key |
| --- | --- |
| `activate-tab` | `alt+digit` |
| `ai-fix-error` | `ctrl+h` |
| `begin-transaction` | `ctrl+b` |
| `cancel-query` | `ctrl+x` |
| `close-connection` | `ctrl+w` |
| `close-tab` | `alt+w` |
| `commit-transaction` | `ctrl+l` |
| `explain` | `ctrl+e` |
| `explain-analyze` | `ctrl+y` |
| `export-csv` | `ctrl+s` |
| `export-json` | `ctrl+g` |
| `focus-editor` | `alt+p e` |
| `focus-next-pane` | `tab` |
| `focus-previous-pane` | `shift+tab` |
| `focus-result` | `alt+p r` |
| `focus-sidebar` | `alt+p s` |
| `name-tab` | `alt+t` |
| `new-query-tab` | `alt+n` |
| `next-connection` | `}` or `alt+right` |
| `next-page` | `ctrl+f` |
| `next-statement` | `'` |
| `next-tab` | `]` or `alt+down` |
| `next-view` | `.` |
| `open-picker` | `ctrl+n` |
| `previous-connection` | `{` or `alt+left` |
| `previous-statement` | `;` |
| `previous-tab` | `[` or `alt+up` |
| `previous-view` | `,` |
| `refresh-objects` | `f5` |
| `reopen-tab` | `alt+shift+w` |
| `reveal-sql` | `alt+e` |
| `rollback-transaction` | `ctrl+u` |
| `run-at-cursor` | `ctrl+r` |
| `run-batch` | `alt+r` |
| `save-query` | `ctrl+p` |
| `select-view` | `digit` |
| `send-to-ai` | `alt+i` |
| `show-activity` | `alt+o a` |
| `show-ai-chat` | `ctrl+i` |
| `show-help` | `?` |
| `show-history` | `ctrl+t` |
| `show-palette` | `ctrl+k` |
| `show-saved` | `ctrl+q` |
| `show-themes` | `alt+o t` |
| `toggle-autocommit` | `ctrl+o` |
| `toggle-result` | `alt+d` |
| `toggle-sidebar` | `alt+s` |

## Grid

`[keys.grid]`

| Action | Key |
| --- | --- |
| `add-sort-column` | `S` |
| `clear-rewrites` | `c` |
| `copy-csv` | `C c` |
| `copy-inserts` | `C i` |
| `copy-json` | `C j` |
| `copy-markdown` | `C m` |
| `copy-menu` | `y` |
| `count-rows` | `t` |
| `cursor-down` | `down` |
| `cursor-first-row` | `home` |
| `cursor-last-row` | `end` |
| `cursor-left` | `left` |
| `cursor-page-down` | `pagedown` |
| `cursor-page-up` | `pageup` |
| `cursor-right` | `right` |
| `cursor-up` | `up` |
| `discard-changes` | `X` |
| `duplicate-row` | `D` |
| `edit-cell` | `e` |
| `exclude-cell` | `x` |
| `filter-by-cell` | `f` |
| `filter-by-values` | `F` |
| `filter-where` | `w` |
| `follow-foreign-key` | `g` |
| `freeze-columns` | `z` |
| `go-to-column` | `a` |
| `insert-row` | `n` |
| `open-menu` | `m` |
| `open-row` | `return` |
| `pop-filter` | `u` |
| `redo-change` | `ctrl+shift+z` or `Z` |
| `review-changes` | `p` |
| `search-columns` | `/` |
| `sort-column` | `s` |
| `toggle-delete` | `d` |
| `toggle-masking` | `M` |
| `undo-change` | `ctrl+z` |
| `view-cell` | `v` |

## Lists

`[keys.list]`

| Action | Key |
| --- | --- |
| `choose-row` | `return` |
| `cursor-down` | `down` |
| `cursor-first-row` | `home` |
| `cursor-last-row` | `end` |
| `cursor-page-down` | `pagedown` |
| `cursor-page-up` | `pageup` |
| `cursor-up` | `up` |

## Plan

`[keys.plan]`

| Action | Key |
| --- | --- |
| `ai-check-plan` | `i` |
| `copy-plan` | `y` |
| `toggle-raw-plan` | `r` |

## Document tree

`[keys.document]`

The document tree shows the rows of a result as documents. It is available wherever a value has fields or elements: a document of a MongoDB collection, or a `json` or `jsonb` column of a SQL database.

| Action | Key |
| --- | --- |
| `copy-path` | `shift+y` |
| `copy-value` | `y` |
| `count-rows` | `t` |
| `cursor-down` | `down` |
| `cursor-first-row` | `home` |
| `cursor-last-row` | `end` |
| `cursor-page-down` | `pagedown` |
| `cursor-page-up` | `pageup` |
| `cursor-up` | `up` |
| `fold-row` | `left` |
| `open-node` | `return` |
| `search-columns` | `/` |
| `unfold-row` | `right` |

## Object tree

`[keys.tree]`

| Action | Key |
| --- | --- |
| `cursor-down` | `down` |
| `cursor-first-row` | `home` |
| `cursor-last-row` | `end` |
| `cursor-page-down` | `pagedown` |
| `cursor-page-up` | `pageup` |
| `cursor-up` | `up` |
| `describe-table` | `i` |
| `filter-tree` | `/` |
| `fold-row` | `left` |
| `object-menu` | `m` |
| `open-in-new-tab` | `o` |
| `open-node` | `return` |
| `toggle-favourite` | `f` |
| `toggle-system-schemas` | `h` |
| `unfold-row` | `right` |
