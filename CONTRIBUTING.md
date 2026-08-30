# Contributing

How to build masume, what a change must pass, and how the code is written.

## Before a change

- For a bug, open an issue with the steps that show it.
- For a feature, open an issue and agree the shape first. A pull request with no issue behind it may be turned down.
- For a typo or a one line fix, send the pull request.

Report a vulnerability through `SECURITY.md`, not an issue.

## Set up

Go 1.27 or later, and [mise](https://mise.jdx.dev) for the tools and tasks.

```sh
git clone https://github.com/turanmahmudov/masume.git
cd masume
mise install
mise run build
```

Docker is needed only for the tests that use a real server.

## The gate

Every change must pass this, and it needs no server:

```sh
mise run check
```

That is gofmt, goimports, `go vet`, staticcheck, errcheck, govulncheck, and the whole suite under the race detector. CI runs the same set, split into separate jobs so a red tick names the one that failed, with the race suite run on Linux and on macOS.

The parts also run alone:

```sh
mise run fmt          # format the source and group the imports
mise run lint         # fmt, imports, vet, staticcheck, errcheck
mise run test         # the suite
mise run race         # the suite, with the race detector
mise run vuln         # govulncheck
mise run deadcode     # dead code
mise run cover        # coverage per package and in total
```

A change to an engine must also pass the tests that use a real server:

```sh
mise run test-integration-full   # start the servers, run the tests, stop them
```

`compose.yaml` holds those servers, on ports no local server uses. To drive them by hand, use `mise run servers-up` and `mise run servers-down`, then `mise run test-integration`.

CI runs this suite three times, against different server versions. Override an image to reproduce one of those runs locally:

```sh
MASUME_TEST_POSTGRES_IMAGE=postgres:14-alpine \
MASUME_TEST_MYSQL_IMAGE=mariadb:11 \
  mise run test-integration-full
```

## The recording and the still frames

Both are built against the postgres container and the schema in `vhs/seed.sql`:

```sh
mise run demo    # vhs/demo.gif for the README
mise run shots   # the still frames in vhs/shots
```

Each needs docker, and installs vhs, ttyd and ffmpeg through mise as it runs. The glyphs come from `vhs/config.toml` and need a Nerd Font. Rebuild them when a change alters what they show.

## Code style

The whole tree follows one style. Read a file next to the one being changed.

- **A method that does something starts with a verb.** This covers private helpers and closures: `resolveRequestState`, `buildToolSchemas`, `findUnknownArgument`.
- **An accessor is named for what it returns.** No `Get` prefix: `Dialect()`, `Capabilities()`, not `GetDialect()`. This is what Effective Go asks for.
- **A method a standard interface requires keeps its name:** `Error`, `String`, `Len`, `Close`, `Unwrap`.
- **`find` may return nothing. `get` returns or fails.** There is no `OrFail` suffix.
- **Names are full.** Keep the prefix. Prefer a domain name over a generic one: `tableDetail`, not `data`.
- **Comments say why, never what.** Add one only for what the code cannot say: a server quirk, or why a slower path is right.
- **Imports go in three groups:** the standard library, outside modules, then this module. `mise run fmt` sorts them.

## Tests

- A change to behaviour needs a test. The test must fail without the change.
- Use table-driven tests for more than one case.
- Spec tests live in `*_spec_test.go` and use the `_test` package. They reach the code only through what it exports.
- `internal/ui/frame_safety_test.go` draws every view and card with values a terminal cannot print. It checks that each row is exactly the screen width and carries no control character. Add new views and cards to it.
- No test needs a server unless it sits behind the `integration` build tag.

## Changes a user sees

- A new action needs a chord in the default preset. `TestEveryPresetBindsEveryAction` enforces this, so nothing is reachable from the palette alone.
- A new config key belongs in `config.example.toml` and the page under `docs/` that covers it.
- For a change to the frame, put the before and after in the pull request. `tmux capture-pane -p` writes a frame as text.

## Commits and pull requests

- One commit per idea. Keep a rename or a move in its own commit.
- The subject is one imperative line, under 72 characters. No prefix, no full stop: `read the plan of a MariaDB statement`.
- The body says why.
- A rename updates every reference in the same commit.
- Fill in the pull request template. Tick only the boxes that were run.

## License

Opening a pull request agrees to release the work under the Apache License 2.0. No CLA, nothing to sign.
