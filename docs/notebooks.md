# Notebooks

A notebook is a fourth kind of tab: an ordered list of cells over one connection. The cells share one parameter set and one run policy. A notebook is a Markdown file, so it reads outside masume and diffs in a pull request.

The [key reference](keys.md#notebook) lists the bindings of the cell list.

## Opening a notebook

`Alt+B` opens an empty notebook, and so does `Alt+Shift+N` where the terminal reports the Shift. `Alt+O n` opens the notebooks card, which lists the notebooks of the project and of the user with the directory each one is kept in. Enter opens the row in this tab, `Alt+Enter` in a new tab, `n` opens an empty notebook, `e` renames the file, and `d` deletes it after a question. The filter field also matches the place a notebook is kept, so `project` lists the notebooks of the team.

`masume ./review.masume.md` opens a notebook file by path, together with the connection the arguments name.

Opening a notebook runs no cell. Every cell is marked `not run` until a run key is pressed.

## The cell list and the cell editor

A notebook tab has two panes: the cell list where a query tab draws its editor, and the result pane under it. The list shows one row per cell with its state, its number, its kind, its name and what its last run answered. An open cell draws its text under that row.

Up and Down move between cells. `Enter` puts the caret in the focused cell, and `Esc` brings it back to the list. Inside a cell every editor key works: completion, formatting, comments, find and replace, and the local checks.

The result pane draws the result of the focused cell, with the views, the sort, the filters and the row editing of a query tab. A sort and a filter belong to the cell they were set on: the next cell brings its own, and the cell that was sorted brings its sort back. `o` folds one cell, `O` folds every cell.

The wheel moves the list, and a drag of the bar at its right does the same. The list stays where the wheel left it until the focus moves. A press on a row moves to the cell that row belongs to, and a second press on the same cell opens it.

`b` and `a` add a cell below or above. Both ask for the kind first, and the cell opens for writing with the kind it takes. `c` changes the kind of a cell that already stands. `t` names the cell, which writes a comment on its first line. `d d` deletes the cell, and `u` undoes a change of the list. `K` and `J` move the focused cell. `y`, `x` and `p` copy, cut and paste a cell.

A statement cell is completed and checked against the catalog. A prose cell and a parameter cell are neither: no completion, no problem row, and no colour of a statement.

## Kinds of cell

| Kind | Holds | Runs |
| --- | --- | --- |
| `sql` | One or more statements of the engine | Yes, in order, one result per statement |
| `md` | Prose | No |
| `param` | The values every cell binds | Binds values |
| `chart` | A source cell, a label column and a value column | Draws the result of the source cell |

A statement cell takes its name from its first `--` comment line. A cell with three statements shows three numbered results.

A parameter cell holds one `name = value` line per parameter. Every `:name` mark of every cell binds from these values. A mark without a value opens the same form a query tab opens.

A value in single quotes, in double quotes, or in no quotes at all is text, and the quotes are no part of it. A number is a number and `true` is a flag.

```
day = '2026-09-01'
region = "EU"
status = paid
limit = 100
paid = true
```

A chart cell draws the result of another cell, and holds no text of its own. A value below zero draws from the zero line, so a loss reads as a bar on the other side of it.

To draw one: run the statement cell first, then add a cell with `b`, choose `chart`, and the form of the chart opens. It opens on the source cell above it and on the first two columns of its result, one that is no number as the label and the first number as the value. Up and Down move between the rows of the form, Left and Right change the value of a row, `Ctrl+S` applies it and `Esc` closes it. `Enter` on a chart cell opens the same form again.

| Row | Values |
| --- | --- |
| source cell | Every statement cell of the notebook |
| label column | The columns of the result of the source cell, or the row number |
| value column | The columns of the result of the source cell |
| shape | `bar` draws one bar per row; `line` draws every value in one row of blocks |
| order | The order of the result, or the value, highest first |
| rows | Every row, or the first 5, 10, 20 or 50 |

The columns offered are the columns the source cell returned, so a chart names no column that result does not hold. A source cell that has not run offers none, and the form says which cell to run.

## One cell inside another

A statement cell can name another cell in place of a relation:

```sql
-- the total of the paid orders
select count(*) as orders, sum(total_cents) as cents from {{cell:paid-orders}} as paid
```

`{{cell:id}}` expands to the statement of that cell in parentheses, before the values of the `:name` marks are bound. It carries no rows: the statement of the named cell runs again inside this one. A reference to a cell that is not there, a reference that names itself, and a chain more than eight cells deep all stop the run and say which reference it is. A cell of MongoDB cannot name another cell.

## Running cells

| Key | Runs |
| --- | --- |
| `r` | The focused cell |
| `R` | The focused cell and every cell below it |
| `Alt+R` | Every cell |
| `space` then `m` | The marked cells |
| `Ctrl+R` | The focused cell, or the selection inside it |
| `Ctrl+X` | Stops the run. A stopped run sends no further cell, whatever the error policy says |

A run is the batch runner of a query tab with a longer plan: one result per statement, in order, with the run identity, the cancellation and the query history of any other run. Every write goes through the same guard. A read-only profile refuses a write, `confirm_writes` still asks, and `write_plan` still measures. See [measuring a write](configuration.md#measuring-a-write).

`Alt+O p` sets the run policy of this notebook.

| Setting | Values | Default | What it does |
| --- | --- | --- | --- |
| Transaction | `autocommit`, `single` | `autocommit` | `single` begins one transaction before the first cell and commits after the last one. A failure leaves the transaction open, and `Ctrl+L` and `Ctrl+U` close it. |
| On error | `stop`, `continue` | `stop` | `stop` ends the run at the first failure. `continue` runs the cells after a failed one. |

A notebook that names another engine reports the difference and runs.

The store of a tab holds the results of one run, so a run of one cell replaces the rows of the cells it did not run. Those cells are marked `stale · run again`, and point at no result of the new run.

## The file

A notebook is Markdown with a TOML front matter block and fenced code cells. Every fence with a known language is a cell, and the prose between two fences is a text cell. An unknown fence and an unknown front matter key are kept as they were written, so a notebook of a newer build loses nothing in an older one.

````
+++
title = "Weekly revenue review"
profiles = ["shop", "shop-prod"]
engine = "postgres"

[run]
transaction = "single"
on_error = "stop"
+++

# Weekly revenue review

Revenue by country for the trailing week. Every statement cell binds :day and :region.

```param
day = "2026-09-01"
region = "EU"
```

```sql id=countries-by-revenue
-- countries by revenue
select c.country, count(*) as orders, sum(o.total) as revenue
from orders o join customers c on c.id = o.customer_id
where o.placed_at >= :day and c.region = :region
group by c.country order by revenue desc
```

```chart source=countries-by-revenue label=country value=revenue kind=bar
```

```sql id=refresh-revenue-daily write=confirm
-- refresh revenue_daily
update revenue_daily set revenue = 0
```
````

`profiles` is the profiles the card offers the notebook on. It opens no connection of its own. `write=confirm` on a fence adds a question before that cell; it removes none.

## A report of the rows

`Alt+O r` writes a report of the notebook: its prose, every statement, the rows each cell answered, and every chart as a block of text. It goes to the notebook file with a Markdown ending, or to a path typed into the field. A masked column stays masked. Files are created with mode `0600`.

This is the one place inside the app where the rows of a notebook reach a file. `Ctrl+S` and `Ctrl+G` still export the result of the focused cell as CSV or JSON.

## Where notebooks live

| Kind | Location |
| --- | --- |
| Project | `<project root>/.masume/notebooks/*.masume.md`, beside the nearest `.masume.toml` |
| Personal | `$XDG_STATE_HOME/masume/notebooks/*.masume.md`, beside the history file |
| Extra | The directories of `[notebooks] paths` in the config file |
| Any path | `masume ./one-off.masume.md`, or a path typed into the save field |

`Ctrl+P` writes the notebook. A notebook that was never saved asks for a name, and a name without a directory is written to the project directory where there is a project file, and to the personal directory otherwise. Files are created with mode `0600` in a directory of mode `0700`.

A notebook file holds statements, prose and parameter defaults. It never holds result rows. The open tabs of a profile keep the text of every notebook, so an unsaved notebook survives a restart, and no result is stored.

## From the AI chat

**Ask AI: build a notebook** in the palette asks what the notebook is to cover, then asks the model for it. The reply opens as a notebook the moment it arrives: its prose becomes text cells, every statement it wrote becomes a statement cell, and the `:name` marks those statements bind become a parameter cell at the top, one line each, for the reader to fill in. The notebook opens unsaved, and no cell runs.

The request asks the model for one fenced block per query, and for a `-- name` line on each, so every cell arrives named. A reply that holds no statement reports that and opens nothing.

`Ctrl+J` in the chat inserts the statement of the last reply as a new cell under the focused one. `Ctrl+G` in the chat turns the whole conversation into a notebook: the prose of every turn becomes a text cell, and every statement the model wrote becomes a statement cell.

## Without a screen

`masume nb run FILE` runs a notebook and writes the results. It follows the rules of [masume run](headless.md): the profile, the timeouts and the read-only check all apply.

```
masume nb run .masume/notebooks/revenue-review.masume.md \
  -p shop --param day=2026-09-01 -f markdown > review.md
```

| Argument | What it does |
| --- | --- |
| `-p`, `--profile NAME` | The profile of the run |
| `-f`, `--format FORMAT` | `table`, `csv`, `json` or `markdown` |
| `-l`, `--limit ROWS` | The output cap per statement |
| `--param NAME=VALUE` | A value over the value of a parameter cell |
| `--only CELL` | Runs only the cell of that id; repeat for each cell |
| `--explain` | Writes a JSON plan of every statement, and runs none of them |
| `--allow-writes` | Runs the cells that write |

A run without a screen has no write confirmation, no write plan and no undo, so a write cell needs `--allow-writes`. Exit codes match the [headless table](headless.md#exit-codes): `1` for a failed cell, `3` for a write on a read-only profile.

`markdown` writes the whole notebook as one report: the prose, the statements, the rows of every cell as tables, and the charts as blocks of text. This is the one place results are written to disk. A masked column stays masked.

## For an agent

The [MCP server](mcp.md) carries two read-only notebook tools: `list_notebooks` and `read_notebook`. They return names, titles, cell titles and statement text. There is no `run_notebook`. An agent runs a cell by sending its text through `run_query`, where the access level and the confirmation apply.
