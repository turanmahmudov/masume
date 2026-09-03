# Contributing

This page explains how to build masume, what a change must pass, and how the code is written.

## Before a change

- For a bug, open an issue with the steps that reproduce it.
- For a feature, open an issue and agree on the design first. A pull request without an issue can be declined.
- For a typo or a one-line fix, send the pull request directly.

Report a vulnerability as described in `SECURITY.md`, not in an issue.

## Setup

You need Go 1.27 or later, and [mise](https://mise.jdx.dev) for the tools and tasks.

```sh
git clone https://github.com/turanmahmudov/masume.git
cd masume
mise install
mise run build
```

Docker is needed only for the tests that use a real database server.

## The quality gate

Every change must pass this command. It needs no server:

```sh
mise run check
```

It runs gofmt, goimports, `go vet`, staticcheck, errcheck, govulncheck, and the full test suite with the race detector. CI runs the same steps as separate jobs, so a failure shows which step failed. The race suite runs on Linux and on macOS.

You can also run the steps separately:

```sh
mise run fmt          # format the source and group the imports
mise run lint         # fmt, imports, vet, staticcheck, errcheck
mise run test         # the test suite
mise run race         # the test suite with the race detector
mise run vuln         # govulncheck
mise run deadcode     # find unused code
mise run cover        # coverage per package and in total
```

A change to an engine must also pass the integration tests, which use a real server:

```sh
mise run test-integration-full   # start the servers, run the tests, stop the servers
```

`compose.yaml` defines those servers on ports that do not conflict with local servers. To control them manually, use `mise run servers-up` and `mise run servers-down`, then `mise run test-integration`.

CI runs this suite three times against different server versions. Set an image variable to reproduce one of those runs locally:

```sh
MASUME_TEST_POSTGRES_IMAGE=postgres:14-alpine \
MASUME_TEST_MYSQL_IMAGE=mariadb:11 \
  mise run test-integration-full
```

## The demo recording and the screenshots

Both use the postgres container and the schema in `vhs/seed.sql`:

```sh
mise run demo    # vhs/demo.gif for the README
mise run shots   # the screenshots in vhs/shots
```

Both need docker, and both install vhs, ttyd and ffmpeg through mise when they run. The glyphs come from `vhs/config.toml` and need a Nerd Font. Rebuild them when a change alters what they show.

## Code style

The whole tree follows one style. Read a file next to the one you change.

- **A method that does something starts with a verb.** This includes private helpers and closures: `resolveRequestState`, `buildToolSchemas`, `findUnknownArgument`.
- **An accessor is named after what it returns.** No `Get` prefix: `Dialect()` and `Capabilities()`, not `GetDialect()`. This follows Effective Go.
- **A method required by a standard interface keeps its name:** `Error`, `String`, `Len`, `Close`, `Unwrap`.
- **`find` can return nothing. `get` returns a value or fails.** There is no `OrFail` suffix.
- **Names are full.** Keep the prefix. Prefer a domain name over a generic one: `tableDetail`, not `data`.
- **Comments explain why, never what.** Add a comment only for what the code cannot express: a server quirk, or the reason a slower approach is correct.
- **Imports go in three groups:** the standard library, external modules, then this module. `mise run fmt` sorts them.

## Tests

- A change in behaviour needs a test. The test must fail without the change.
- Use table-driven tests when there is more than one case.
- Spec tests live in `*_spec_test.go` files and use the `_test` package. They use only the exported API.
- `internal/ui/frame_safety_test.go` renders every view and card with values that a terminal cannot print. It checks that each row is exactly the screen width and contains no control characters. Add new views and cards to it.
- No test needs a server unless it is behind the `integration` build tag.

## User-visible changes

- A new action needs a key binding in the default preset. `TestEveryPresetBindsEveryAction` checks this, so every action is reachable without the palette.
- A new config key must be added to `config.example.toml` and to the page under `docs/` that covers it.
- For a change to the screen, include a before and after frame in the pull request. `tmux capture-pane -p` writes a frame as text.

## Commits and pull requests

- One commit per idea. Keep a rename or a move in its own commit.
- The subject is one imperative line, under 72 characters, with no prefix and no full stop: `read the plan of a MariaDB statement`.
- The body explains why.
- A rename updates every reference in the same commit.
- Fill in the pull request template. Tick only the boxes for checks you ran.

## License

When you open a pull request, you agree to release the work under the Apache License 2.0. There is no CLA to sign.
