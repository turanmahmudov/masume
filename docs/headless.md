# Without a screen

`masume run` runs one statement and writes the result to stdout. It opens the connection the same way the client does, over the same profiles, and it keeps the same timeouts and the same access limits. Nothing is drawn, and the exit code is the answer for a caller that reads no output.

```sh
masume run -p shop-prod -f json 'select count(*) from orders'
```

## Arguments

```
masume run [TARGET] STATEMENT
masume run [TARGET] -e FILE
```

| Argument | Meaning |
| --- | --- |
| `TARGET` | A URL, a connection string, or a database file, read the same way as `masume TARGET`. With none, the connection comes from `--profile` or `$DATABASE_URL` |
| `-p`, `--profile NAME` | A profile of the config file |
| `-e`, `--execute FILE` | Read the statement from a file. A single `-` reads stdin |
| `-f`, `--format FORMAT` | `table` (the default), `csv`, `json` or `markdown` |
| `-l`, `--limit ROWS` | The rows to return. Without it, one page of the profile |
| `--param NAME=VALUE` | Bind `:NAME` in the statement. Repeat it for each parameter |
| `--explain` | Write the plan as JSON instead of running the statement |
| `-h`, `--help` | Write the arguments and exit |

The connection stands before the statement where both are written.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Every statement ran |
| `1` | The server refused a statement, or a parameter has no value |
| `2` | The connection could not be opened, or the arguments could not be read |
| `3` | The profile is read-only and the statement writes |

A read-only profile refuses a write in the client, before the statement reaches the server.

## Formats

`table` is for a person reading a terminal. Each column is as wide as its widest cell, and no line carries trailing spaces.

```
id  total_cents  status
--  -----------  ---------
1   4990         paid
2   1200         paid
3   99           cancelled
```

`csv`, `json` and `markdown` are the formats the export form writes. A file from a run and a file from the client are the same. A result with no rows still writes the shape of the result: `csv` writes its header and `json` writes an empty array.

A statement with no result set writes what it changed to stderr, `UPDATE 1` or `CREATE`, and nothing to stdout. The document on stdout holds the result and nothing else.

## Parameters

A statement binds `:name` marks, the same as in the client. The values are bound, never written into the statement as text, so a value from a script cannot be read as SQL.

```sh
masume run -p shop -f csv \
  --param day=2026-09-02 --param status=paid \
  'select id from orders where created_at::date = :day and status = :status'
```

`--explain` is the one exception: the values are written into the statement. A planner without them estimates for the wrong values, and a server does not plan a statement that still holds a placeholder.

## The plan

`--explain` writes the plan as JSON. A check in CI reads a number out of it, in place of the text of a server.

```sh
masume run -p shop --explain 'select * from orders where status = :s' --param s=paid \
  | jq '[.nodes[].selfMs // 0] | add'
```

Each node holds `depth`, `label`, `detail`, `estimatedRows`, `actualRows`, `selfMs`, `shareOfTotal`, `slowest` and `misestimated`. A count the server did not measure or estimate is `null`, never zero. The plan is measured only where the server can measure one and the statement changes nothing. `analyzed` is `true` for a measured plan.

## Passwords

The password comes from the profile: its own value, `password_env`, or the output of `password_command`. A profile with `auth = "prompt"` cannot be run without a screen. The run reports that and exits with 2.

## Several statements

A text or a file with several statements runs them in order and writes one result after another. A statement that fails stops the run. No later statement runs.

## The row count

A statement that bounds its own result returns every row it asks for. A statement that does not returns one page: `page_size` on the profile, 200 rows by default.

```sh
masume run -p shop 'select * from orders limit 250'    # 250 rows, set by the statement
masume run -p shop 'select * from orders'              # 200 rows, and a note that there are more
masume run -p shop --limit 5000 'select * from orders' # 5000 rows, set by the run
```

A `LIMIT`, a `FETCH FIRST`, or `.limit(n)` on a MongoDB find all bound a result. `--limit` bounds a run whose statement does not, and it wins over the page of the profile.

Where the page is all that came back and the result is longer, the run reports it on the error stream and exits `0`:

```
masume: the first 200 rows of a longer result; add a limit to the statement,
or --limit, to read more
```

The output stream holds the rows and nothing else.

### How the rows are held

A bounded result is read a batch at a time, `page_size` rows per batch. `csv` and `json` are written batch by batch, so a result larger than memory still reaches the stream. `table` and `markdown` are held whole before the first line is written, because both measure the widest cell of a column first; a run of many rows in those formats holds them all.

### A statement that changes something

A statement that writes is run one time and never twice, so its result is one read and not a stream. `update … returning` beyond one page writes what the read returned, reports that the rest was not read, and exits `1`. The output is a part of the result, not the whole:

```
masume: only the first 200 rows of a longer result: a statement that changes something
is never run twice
```

Pass `--limit` above the number of rows the statement returns to read them all in one read.

### Reading more by profile

Raising `page_size` on the profile moves the default for both the client and a run. A profile of its own keeps the client reading in small pages while a job reads a whole report:

```toml
[profile.shop-report]
engine    = "postgres"
host      = "db.internal"
database  = "shop"
user      = "reader"
mode      = "read-only"
page_size = 50000
```
