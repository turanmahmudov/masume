# Keys

This page lists the default bindings and their configuration names. The [user guide](usage.md) describes the workflows.

Press `?` outside text entry for help. Press `Ctrl+K` for the command palette. Help shows the current bindings of every configurable action, including overrides. Keys a pane or a field handles itself, such as `Tab`, `Esc` and the Shift selection keys, are fixed and appear in help with their fixed names. The palette searches commands by name.

## Focus and notation

A scope is the pane or card where a binding applies. Cards and input fields handle their own keys. Unsupported engine actions are unavailable.

| Scope | Focus |
| --- | --- |
| `global` | The workspace, outside cards and prompts |
| `tree` | The object tree |
| `editor` | The SQL editor |
| `grid` | The Data grid |
| `document` | The result document tree |
| `plan` | The Plan view |
| `notebook` | The cell list of a notebook tab |
| `list` | Lists in cards, and scrolling in detail views |
| `dialog` | The active card, connection picker, or form |

Plain global keys type characters while the editor has focus. These include `?`, digits, brackets, braces, commas, periods, semicolons, and apostrophes. Use modified alternatives or the palette during text entry.

`return` is Enter. `digit` is any number from `1` through `9`. Uppercase letters require Shift: `F` differs from `f`.

`alt`, `meta`, and `option` are equivalent configuration modifiers. This reference and the help screen use Alt. Spaces separate key presses: `alt+p s` is Alt+P, then lowercase `s`. `C c` is uppercase C, then lowercase c.

`Ctrl+C` copies an editor or mouse text selection and clears the selection. Without a selection, `Ctrl+C` quits, with a connection-save question when applicable. `Esc` closes a card, dismisses completion, or clears a workspace selection.

`Tab` accepts a listed completion; otherwise, `Tab` moves focus. `Enter` inserts a newline in the editor. `Ctrl+V` pastes the last text copied inside masume. Use the terminal paste command for the operating system clipboard.

Legacy terminals can merge `Ctrl+I` with Tab, `Ctrl+H` with Backspace, and `Ctrl+M` with Enter. They can also merge `Ctrl+[` with Escape and `Ctrl+Shift+Z` with `Ctrl+Z`. Extended keyboard support depends on the terminal and any multiplexer. Use `Alt+Z` for editor redo, `Z` for grid redo, or the palette for affected commands.

## Rebinding

Add overrides to [config.toml](configuration.md#keys). Each entry replaces that action's preset bindings. Unlisted actions keep their defaults. `[]` removes a binding.

```toml
[keys]
preset = "default"

[keys.global]
refresh-objects = []
run-at-cursor = ["ctrl+r", "f5"]

[keys.grid]
sort-column = ["o"]
```

The example removes the default F5 refresh binding before assigning F5 to execution. The palette still offers refresh.

Every registered action has a default binding. Some palette operations have no registered action or binding; see [palette operations](usage.md#palette-operations).

## Cards

`[keys.dialog]`

One card returns only its own actions, so two rows of this table can carry the same key without a conflict.

| Action | Key |
| --- | --- |
| `accept-completion` | `tab` |
| `answer-no` | `n` |
| `answer-yes` | `y` |
| `apply-changes` | `ctrl+y` |
| `apply-step` | `return` |
| `chat-to-notebook` | `ctrl+g` |
| `close` | `escape` |
| `copy-value` | `ctrl+a` or `y` |
| `delete-connection` | `d` |
| `discard-changes` | `x` |
| `edit-connection` | `e` |
| `fold-row` | `left` |
| `insert-ai-sql` | `ctrl+j` |
| `keep-all-values` | `a` |
| `keep-only-value` | `o` |
| `leave-directory` | `left` |
| `list-secondary` | `ctrl+d` |
| `new-ai-chat` | `ctrl+l` |
| `new-connection` | `n` |
| `next-field` | `down` or `tab` |
| `next-row` | `right` |
| `next-turn` | `ctrl+n` |
| `next-value` | `right` |
| `open-directory` | `right` |
| `open-in-new-tab` | `alt+return` |
| `prettify-json` | `ctrl+f` |
| `previous-field` | `up` |
| `previous-row` | `left` |
| `previous-turn` | `ctrl+p` |
| `previous-value` | `left` |
| `replace-in-statement` | `ctrl+r` |
| `run-with-values` | `ctrl+r` |
| `save-cell` | `ctrl+s` |
| `save-form` | `ctrl+s` |
| `scroll-back` | `pageup` |
| `scroll-forward` | `pagedown` |
| `scroll-left` | `left` |
| `scroll-right` | `right` |
| `send-question` | `return` |
| `set-default` | `ctrl+d` |
| `set-empty` | `ctrl+e` |
| `set-null` | `ctrl+l` |
| `show-ai-chats` | `ctrl+o` |
| `step-back` | `escape` |
| `stop-ai-reply` | `ctrl+x` |
| `stop-session` | `x` |
| `test-connection` | `ctrl+t` |
| `toggle-value` | `space` |
| `unfold-row` | `right` |
| `use-keyring` | `tab` |
| `write-export` | `ctrl+s` |
| `write-newline` | `shift+return` or `alt+return` |

## Finding and replacing

See [editing SQL](usage.md#editing-sql) for search, replacement, completion, and text selection.

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
| `leave-cell` | `escape` |
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
| `new-notebook-tab` | `alt+b` or `alt+shift+n` |
| `new-query-tab` | `alt+n` |
| `next-connection` | `}` or `alt+right` |
| `next-page` | `ctrl+f` |
| `next-statement` | `'` |
| `next-tab` | `]` or `alt+down` |
| `next-view` | `.` |
| `notebook-run-policy` | `alt+o p` |
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
| `show-notebooks` | `alt+o n` |
| `write-notebook-report` | `alt+o r` |
| `show-palette` | `ctrl+k` |
| `show-saved` | `ctrl+q` |
| `show-themes` | `alt+o t` |
| `toggle-autocommit` | `ctrl+o` |
| `toggle-result` | `alt+d` |
| `toggle-sidebar` | `alt+s` |
| `undo-write` | `alt+u` |

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

The Tree view opens result values with fields or elements, including MongoDB documents and SQL JSON values. See [result views](usage.md#result-views).

| Action | Key |
| --- | --- |
| `clear-rewrites` | `c` |
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
| `pop-filter` | `u` |
| `search-columns` | `/` |
| `unfold-row` | `right` |

## Notebook

`[keys.notebook]`

The cell list takes no typed text, so single letters are free there. `Enter` puts the caret in the focused cell, and `Esc` brings it back to the list.

| Action | Key |
| --- | --- |
| `add-cell-above` | `a` |
| `add-cell-below` | `b` |
| `copy-cell` | `y` |
| `cursor-down` | `down` or `j` |
| `cursor-first-row` | `home` |
| `cursor-last-row` | `end` |
| `cursor-up` | `up` or `k` |
| `cut-cell` | `x` |
| `delete-cell` | `d d` |
| `edit-cell-source` | `return` |
| `mark-cell` | `space` |
| `move-cell-down` | `J` |
| `move-cell-up` | `K` |
| `name-cell` | `t` |
| `paste-cell` | `p` |
| `redo-cell-change` | `Z` |
| `run-cell` | `r` |
| `run-from-cell` | `R` |
| `run-marked-cells` | `m` |
| `set-cell-kind` | `c` |
| `toggle-cell-output` | `o` |
| `toggle-every-output` | `O` |
| `undo-cell-change` | `u` |

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
