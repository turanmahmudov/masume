# Keys

Press `?` inside masume for the help screen, or `Ctrl+K` for the command palette, which finds an action by name. This page is the same list, written for the config file.

An action belongs to a scope, and the scope decides which pane handles the chord. Every action has a chord in the default preset, so nothing is reachable from the palette alone.

To rebind one, name it in `config.toml`. A line that is written replaces what the preset bound. A line left out keeps it.

```toml
[keys]
preset = "default"

[keys.global]
run-at-cursor = ["ctrl+r", "f5"]

[keys.grid]
sort-column = ["o"]
```

An action bound to `[]` has no chord until one is written.

`alt`, `meta` and `option` name the same modifier. The preset and this page write `alt`. The help screen writes `meta`. A space in a chord is a sequence of presses: `alt+p s` is Alt+P, then S.

## Cards

`[keys.dialog]`

| Action | Chord |
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

One key does both. `Alt+F` opens the `find` field, and the field itself offers the rest.

- Type a term and press Enter to mark every match. `F3` and `Shift+F3` step through them.
- Or press `Ctrl+R` instead of Enter. The field becomes `replace … with`, titled with the term that was typed, and the next text replaces **every** match in one step. The bar reports how many it wrote.

The term is typed once either way. `Ctrl+Z` takes the whole replace back in one step, however many matches it wrote.

Matching is plain text, so `id` matches `customer_id` too. The title names the term so it can be read before the replace runs.

## Editor

`[keys.editor]`

| Action | Chord |
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

| Action | Chord |
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

| Action | Chord |
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

| Action | Chord |
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

| Action | Chord |
| --- | --- |
| `ai-check-plan` | `i` |
| `copy-plan` | `y` |
| `toggle-raw-plan` | `r` |

## Object tree

`[keys.tree]`

| Action | Chord |
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
