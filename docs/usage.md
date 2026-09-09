# User guide

This guide covers the interactive terminal client. The [key reference](keys.md) lists every default binding and its focus scope.

## First connection

Run `masume` to open the connection picker. Select a profile with Up and Down, then press Enter. `n` opens a new connection form; `e` edits the selected connection. In the form, `Ctrl+T` tests the connection and `Ctrl+S` saves the profile.

An explicit target or `$DATABASE_URL` can open a connection directly. See [connection targets](configuration.md#a-connection-on-the-command-line), [container detection](configuration.md#databases-in-a-container), and [passwords](configuration.md#passwords).

A connection without restored tabs starts with an empty query tab and focus in the object tree. `Tab` and `Shift+Tab` move between visible panes. `Alt+P s` focuses the tree, `Alt+P e` focuses the editor, and `Alt+P r` focuses results.

In the tree, arrows move, expand, and collapse nodes. `Enter` on a table opens its rows or reuses its existing tab. `o` opens another tab for that table. `i` opens the Columns view. `Enter` on a column inserts its qualified name into a query editor.

`/` filters the tree; Enter keeps the filter, and Esc clears the active filter entry. A filter started inside a schema searches that schema. `f` toggles a favourite, `h` toggles system schemas, and `F5` refreshes the catalog.

`m` opens the object menu. Available entries include SQL templates, table changes, imports, and an ER diagram. SQL templates enter the editor without execution. Available actions depend on the object and engine.

## Editing SQL

`Alt+N` opens a new query tab with editor focus. `Alt+E` opens a table read as a query. In a query tab, `Alt+E` writes the grid sort and server filters into the SQL.

Type SQL or use the terminal paste command. `Ctrl+V` only pastes text last copied inside masume, not arbitrary operating system clipboard text.

Completion appears while typing. Up and Down select a candidate; Tab accepts the candidate. Enter always inserts a newline, including while completion is open. Esc dismisses completion. Without completion, Tab changes panes.

Shift with movement keys extends the selection. `Ctrl+A` selects all editor text. `Ctrl+C` copies and clears the selection; without a selection, `Ctrl+C` quits, and asks first about staged changes and open transactions. `Ctrl+D` formats SQL, `Alt+C` toggles line comments, and `Alt+[` or `Alt+]` changes indentation.

`Alt+F` opens Find. Enter applies the search; `F3` and `Shift+F3` select the next or previous match. Matching uses plain substrings, including matches inside longer names.

For replacement, type the search term in Find, then press `Ctrl+R`. Type the replacement and press Enter. Replacement changes every match. One editor undo reverses the whole replacement.

Search and replacement cover the whole editor buffer, including other statements, comments and strings. Matching normally ignores case. If lowercase conversion changes the text's byte length, matching uses the original case.

`F8` moves to the next reported SQL problem. Editor undo is `Ctrl+Z`; redo is `Ctrl+Shift+Z` or `Alt+Z`.

## Running SQL

`Ctrl+R` runs the selected text, or the statement at the caret when no text is selected. A selection can contain several statements. `Alt+R` runs the entire buffer. On a table tab, either command reads the table again.

Statements run in order, with one result each. The first failure stops the batch; later statements do not run. A batch is not automatically atomic. Without a transaction, earlier successful writes can remain after a later failure.

The result pane receives focus after editor execution. `;` and `'` select previous and next statement results outside text entry. The numbered result strip also accepts mouse clicks.

Named parameters such as `:customer_id` open a JSON value form before execution. `Ctrl+R` submits the values; Esc cancels. Each statement with parameters has its own form. No statement runs until all parameter forms are complete.

Read-only profiles refuse writes. Other profiles can ask for confirmation or show a write plan. Review the SQL and affected rows before accepting. See [write guards](configuration.md#a-profile) and [write plans](configuration.md#measuring-a-write).

`Ctrl+X` requests query cancellation and stops a running export. Cancellation support depends on the engine. `Ctrl+C` is not the query cancellation command.

## Transactions

Transaction commands apply to the active connection, including its other tabs. They require engine transaction support.

| Command | Key |
| --- | --- |
| Begin a transaction | `Ctrl+B` |
| Commit the transaction | `Ctrl+L` |
| Roll back the transaction | `Ctrl+U` |
| Toggle autocommit for this connection | `Ctrl+O` |

Begin a transaction before running statements that must remain uncommitted. With autocommit off, masume begins a transaction before execution when none is active. Execution leaves that transaction open for an explicit commit or rollback. The profile setting `autocommit = false` starts the connection in this mode.

This applies to editor queries, table reads, grid writes, undo, whole-query export and AI chat `run_query`. Imports manage their own transactions and refuse an existing transaction. Catalog discovery and other AI chat tools do not automatically begin transactions. Headless and MCP execution have separate transaction behavior.

Changing autocommit does not commit or roll back an existing transaction. Commit or roll back explicitly. Engine restrictions still apply, including statements that implicitly commit.

## Result views

The result strip lists available views. Outside text entry, `1` through `9` select views by position; comma and period select adjacent views. View positions depend on the tab, result, and engine.

Table tabs offer Data, Columns, Indexes, Constraints, DDL, and Plan where supported. Query results offer Data, Fields, and Plan where supported. Statements without result columns show Statistics, including affected rows and execution time.

Tree appears when a result contains documents or structured values. Left and Right collapse and expand nodes; Enter opens a node. `y` copies the value; `Shift+Y` copies its path. `/`, `u`, and `c` have the same search and reset behavior as the grid.

In Data, arrows move between cells. `v` opens the full cell value; Enter opens the full row. `g` follows a foreign key. `a` finds a column by name. `z` freezes or unfreezes the current column at the left.

Foreign-key navigation filters by the selected column only. For a composite foreign key, add filters for the remaining key columns.

Grid masking hides values in columns with sensitive names. `M` toggles masking. Masks affect only the grid display. Cell viewers, row viewers, document trees, structured copies, and exports expose the original values.

## Sorting and filters

Server operations run the read again. Screen filters only hide loaded rows. Neither operation changes the editor text; `Alt+E` exposes server rewrites as SQL.

| Key | Operation |
| --- | --- |
| `s` | Sort by the current column; repeat to change direction |
| `S` | Add or change a column in the multi-column sort |
| `f` | Add a server filter equal to the current cell |
| `x` | Add a server filter excluding the current cell |
| `w` | Enter a server predicate; replace the previous raw predicate |
| `u` | Remove the last server filter and rerun |
| `F` | Select current-column values to keep on screen |
| `/` | Search loaded rows on screen |
| `c` | Clear grid sorting, all server filters, and all screen filters; rerun |

`f` and `x` add filters to a stack. `w` preserves those cell filters but replaces the existing raw predicate. An empty `w` entry removes the raw predicate. `u` removes only the last server filter; sorting remains. Its rerun also clears screen filters. With no server filters, `u` does nothing. `c` does not remove clauses already written in the SQL.

A server filter searches the whole statement, not the rows already fetched. A `LIMIT` written in the SQL applies to the filtered rows, so the grid can show rows the unfiltered statement does not return.

In the `F` card, Space toggles a value, `o` keeps only the selected value, and `a` keeps all values. Enter applies the selection. Counts cover loaded rows before screen filtering. An empty `/` entry clears the screen search.

**Sorting, server filtering, and rerunning discard staged grid edits without asking.** The status bar reports the discarded changes. These operations also clear screen filters. Apply staged changes before these operations.

## Loading rows

`page_size` is the rows per page; the default is 200. Moving or scrolling near the loaded end fetches more rows automatically. The footer separates loaded rows, total rows when known, and rows shown after screen filtering.

An active screen filter disables automatic paging in both Data and Tree. `Ctrl+F` explicitly fetches another page, including under a screen filter. `t` requests a server count where supported. Counting does not load every row.

## Editing rows

Grid writes require a writable profile, a single identifiable table, loaded column metadata, and a primary key. Updates and deletes require every primary-key column in the result. Joins and other results without a single table cannot be edited. Views and materialized views are not editable through the grid. Generated columns cannot be edited.

| Key | Staged operation |
| --- | --- |
| `e` | Edit the current cell |
| `d` | Mark or unmark the current row for deletion |
| `D` | Duplicate the row without primary-key or generated columns |
| `n` | Enter a new row as JSON |
| `p` | Review staged SQL |
| `X` | Ask to discard all staged changes |

The cell editor offers choices for known enum and boolean columns. `Ctrl+S` stages the value. `Ctrl+L` stages NULL, `Ctrl+E` stages an empty value, and `Ctrl+D` stages DEFAULT. `Ctrl+F` formats JSON in a JSON cell editor. These card keys do not run their global actions.

Staging does not write to the database. The right of the status bar counts the staged changes and names the key of the review, such as `● 3 staged · p to review`. In the review card, `Ctrl+Y` applies changes, `x` discards changes, and Esc returns without applying. A failed apply retains staged changes. Engines without atomic staged writes ask before applying multiple changes individually; a failure can leave earlier changes written.

Staged changes belong to one statement result. Return to that result before applying, or discard the staged changes.

### Three undo operations

| Operation | Effect |
| --- | --- |
| Editor `Ctrl+Z` | Reverses text edits only; redo with `Ctrl+Shift+Z` or `Alt+Z` |
| Grid `Ctrl+Z` | Reverses staged changes only; redo with `Ctrl+Shift+Z` or `Z` |
| Global `Alt+U` | Asks to execute reverse SQL retained by a write plan |

Neither editor undo nor grid undo reverses an executed database write. `Alt+U` requires an available write-plan undo; imports and staged grid writes do not create one. Transaction rollback is separate from all three operations. See [write-plan limits](configuration.md#measuring-a-write).

## Copy and export

`y` opens the grid copy menu. Available forms are cell, row JSON, result CSV, result JSON, result Markdown, result INSERT statements, and column IN clause. `C c`, `C j`, `C m`, and `C i` copy whole loaded results directly. These sequences start with uppercase C.

Result and column copies include every loaded row, including rows hidden by screen filters. They use original values, ignore masks, and do not apply staged edits. Copies never fetch additional rows. INSERT copies require a table target.

`Ctrl+S` opens CSV file export; `Ctrl+G` opens JSON file export. Up and Down move between fields; Left and Right change choices. Set the path and options, then press `Ctrl+S` to write. Existing files require overwrite confirmation.

| Row scope | Behavior |
| --- | --- |
| `loaded so far` | Writes loaded rows without another server read; the default |
| `every row` | Reruns the result's read and streams all returned rows |

Every-row export preserves the executed read, including its server filters, sorting, and SQL limits. The second read can return newer data. Statements classified as writes permit loaded-row export only. Screen filters and masks affect neither export scope.

CSV options include delimiter, header, quoting, line endings, NULL text, and formula guarding. Defaults are comma, header enabled, as-needed quoting, LF, empty NULL text, and formula guarding enabled. CSV clipboard copies use these defaults.

Formula guarding prefixes risky text with an apostrophe. Risky prefixes are `=`, `+`, `-`, `@`, tab, and carriage return; plain numbers remain unchanged.

Default CSV output cannot distinguish NULL from empty text. A custom `null as` value changes the NULL output. Import treats empty CSV fields as NULL even with a custom marker; see [import NULL handling](configuration.md#importing-a-file).

Result JSON exports and copies preserve JSON nulls and native numeric and boolean values. Cell and row-menu copies use display text; row-menu JSON is not the typed result JSON export.

## Importing files

Select a table in the object tree, press `m`, and choose Import a file. The schema menu imports into a new table. Select a file, adjust its format and column mapping, then press Enter for review. Enter from review starts the import. Esc returns from review to the form.

The review validates file values locally; the review does not test database constraints or permissions. Rejected rows are omitted. Import refuses an active transaction. Commit or roll back before importing.

Import has its own review and transaction. Import does not use `write_plan` or create an `Alt+U` undo. See [import configuration](configuration.md#importing-a-file) for formats, sampled types, NULL handling, mappings, and limits.

## Query plans

`Ctrl+E` requests an estimated plan. `Ctrl+Y` requests an analyzed plan where supported. Analysis executes a read to measure the read. For a statement classified as a write, masume downgrades analysis to an estimate without executing the write.

In Plan, `r` toggles the raw server plan and `y` copies the raw plan. `i` requests AI analysis when AI is enabled. See [engine support](engines.md) and [AI data handling](ai.md).

## Tabs and history

`Alt+Up` and `Alt+Down` switch tabs; `Alt+1` through `Alt+9` select tabs directly. `Alt+T` names a query tab with a first-line comment, and titles a notebook tab. Table and object tabs keep their object names.

`Alt+W` closes a tab; `Alt+Shift+W` reopens the last closed tab during the session. Closing a tab with staged edits offers apply, discard, or cancel. The last tab stays open. `Ctrl+W` closes the connection; multiple tabs or staged changes require confirmation.

`Ctrl+N` returns to the connection picker. `Alt+Left` and `Alt+Right` switch open connections. `Alt+S` toggles the sidebar; `Alt+D` toggles the result pane.

`Ctrl+P` saves the query under a name. On a notebook tab, `Ctrl+P` writes the notebook file. `Ctrl+Q` opens saved queries; `Ctrl+T` opens query history. Type to filter either list. Enter replaces the current query text; `Alt+Enter` loads a new query tab. On a table or object tab, either choice opens a query tab. Loading does not execute SQL.

`Ctrl+D` removes a selected personal saved query. Project queries require changes to the [project file](configuration.md#the-project-file).

Restoration uses stored tab identities, query text, notebook text, active tab, caret, sort, and server filters. Results, staged edits, transaction state, screen filters, column widths, and frozen columns are not restored. Restored query tabs do not execute automatically. Restored table and object tabs read data when first shown.

## Notebooks

`Alt+B` opens a notebook: an ordered list of cells over one connection, stored as a Markdown file. `Alt+O n` lists the notebooks of the project and of the user. Opening one runs no cell.

The cell list stands where a query tab draws its editor. Up and Down move between cells, `Enter` puts the caret in the focused cell, and `Esc` brings it back to the list. `b` adds a cell and asks for its kind. `r` runs the focused cell, `R` runs it and the cells below it, and `Alt+R` runs every cell. `Alt+O r` writes a report of the rows every cell answered.

Each cell keeps the view of its own result: the view, the cursor, the frozen columns, the sort and the filter. A reopened notebook comes back on the cell it was left on, with the cells that were folded still folded. See the [notebook guide](notebooks.md) for the cell kinds, the run policy, the file format and `masume nb run`.

## Server activity

`Alt+O a` opens Server activity. The dashboard refreshes about every two seconds. The dashboard shows sessions and available load, blocking, and slow-statement panels.

Panels depend on engine support and server permissions. MySQL load values require access to `performance_schema`; MySQL blocking and slow-statement panels are unavailable. PostgreSQL slow statements require a working `pg_stat_statements` extension, detected at connection time. Slow statements are ordered by mean execution time, highest first, within the current database. Unsupported or failed optional panels are omitted.

Up and Down select a session. Enter replaces the current query text with that session's SQL; `Alt+Enter` opens a new query tab. Neither command executes SQL. On a non-query tab, Enter also opens a query tab.

`x` asks to cancel the selected session's statement. `Ctrl+D` asks to terminate the session and its connection. Confirm either operation before execution. Left collapses the optional panels; Right expands the panels.

SQL Server has no statement that stops one statement of another session, so `x` reports the action as unsupported. `Ctrl+D` runs `KILL`, which ends the session and its transaction.

ClickHouse lists running statements instead of sessions. Both stop actions run `KILL QUERY`, which ends the statement and not the connection. The number of a row is the row of the list, because the server names a statement with a text of its own.

MongoDB lists operations instead of SQL sessions. Both stop actions use `killOp`; neither action terminates the connection.

## Palette operations

`Ctrl+K` opens the searchable command palette. Type a command name, select a row, and press Enter. Commands require their target state, such as a result for copying.

Palette-only operations include Reload the theme files and AI provider selection. Ask AI: explain this query, Ask AI: optimize this query and Ask AI: build a notebook are also palette-only operations. Config problems appears when configuration reports exist. These operations have no default direct binding.

`Alt+O t` opens the theme picker. Movement previews themes, Enter saves the selection, and Esc cancels. See [themes](themes.md).

`Ctrl+I` opens AI chat. `Alt+I` fills the chat input with the editor buffer without sending. `Ctrl+H` immediately requests help with an editor error or failed check. Terminal collisions can affect these keys; the palette offers alternatives. See the [AI guide](ai.md) for chat controls, providers, write confirmation, and shared data.

## Mouse controls

- Scroll the cell list of a notebook with the wheel or the bar at its right. Click a row to move to that cell; click it again to open it.
- Click a pane to focus the pane. Click tabs, connections, statement results, and view labels to select each target.
- Click a tree row to select the row; double-click opens the row. Click the fold marker to expand or collapse.
- Click a grid cell to select the cell; double-click a row to open row details.
- Click a column header to sort; Shift-click adds the column to the sort. Sorting discards staged edits.
- Drag a column edge to resize; double-click the edge to reset the width.
- Drag the divider between the editor and the result to resize both panes. Either side of it works: the foot of the editor and the head of the result. Click the foot of the editor without dragging to toggle results.
- Drag the right border of the object tree to set its width. The tree keeps at least 16 columns and leaves the pane beside it at least 32.
- Drag a scrollbar or turn the wheel to scroll. The wheel leaves the cursor in place; keyboard movement returns to the cursor.
- In the editor, click for the caret, double-click for a word, or triple-click for a line. Drag to select text.
- Drag other displayed text for a screen selection. `Ctrl+C` copies the selected display text.
- Right-click objects, cells, headers, tabs, connections, or the editor for context menus.
- Middle-click a tab, or click its close mark, to close the tab. Normal close confirmation still applies.
- Click displayed key hints and menu entries to activate the corresponding action.

## Troubleshooting

- A key types text: check focus and the [scope rules](keys.md#focus-and-notation). A card can have a different action for the same key.
- A modified key fails: check terminal and multiplexer support. Use a listed alternative, the palette, or [rebind the action](keys.md#rebinding).
- Clipboard paste differs from expected text: use terminal paste for the operating system clipboard. Clipboard writes also require terminal support.
- Rows appear missing: check screen filters, server filters, SQL limits, and the loaded-row count. Use `Ctrl+F` for another page.
- Grid editing is refused: check the status reason, primary key, selected columns, relation kind, and profile access mode.
- Connection or configuration fails: inspect the reported error and Config problems in the palette. See [configuration](configuration.md) and [engine limits](engines.md).
- Symbols or colors are unreadable: select ASCII icons or another [theme](themes.md) in the [interface configuration](configuration.md#interface).
- For execution without a TUI, see [headless usage](headless.md). For agent access, see [MCP](mcp.md).
