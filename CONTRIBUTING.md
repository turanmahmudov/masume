# Contributing

This page covers builds, required checks, code style, and contributions.

## Before a change

- For a bug, open an issue with the steps that reproduce it.
- For a feature, open an issue and agree on the design first. A pull request without an issue can be declined.
- For a typo or a one-line fix, send the pull request directly.

Report a vulnerability as described in `SECURITY.md`, not in an issue.

## Setup

The project uses Go 1.27 and [mise](https://mise.jdx.dev) for tools and tasks. `mise.toml` contains the tool versions.

```sh
git clone https://github.com/turanmahmudov/masume.git
cd masume
mise install
mise run build
```

Integration tests, demo recordings, and screenshots need Docker. The standard quality gate needs no database server.

## The quality gate

Every change must pass this command. It needs no server:

```sh
mise run check
```

The gate checks gofmt formatting, import grouping, `go vet`, staticcheck, errcheck, deadcode, and known vulnerabilities through govulncheck. It also runs the full untagged test suite with the race detector. Formatting checks report errors without editing source files.

CI has separate lint, vulnerability, test, build, and integration jobs. The lint job checks formatting, imports, vet, staticcheck, and errcheck. CI does not run deadcode. The race suite runs on Linux and macOS.

The build job runs the binary's version and help commands on Linux. It also cross-builds Linux ARM64, macOS AMD64, and macOS ARM64.

You can also run the steps separately:

```sh
mise run fmt          # format the source and group the imports
mise run lint         # formatting, imports, vet, staticcheck, errcheck, deadcode
mise run test         # the test suite
mise run race         # the test suite with the race detector
mise run vuln         # govulncheck
mise run deadcode     # find unused code
mise run cover        # coverage per package and in total
```

A change to an engine must also pass the integration tests:

```sh
mise run test-integration-full   # start the servers, run the tests, stop the servers
```

`compose.yaml` defines servers on non-default localhost ports. Run `mise run servers-up`, then `mise run test-integration`, then `mise run servers-down`. The stop task removes the test volumes.

CI runs three integration combinations:

| Combination | PostgreSQL | MySQL protocol | MongoDB |
| --- | --- | --- | --- |
| Current | 18 | MySQL 8.4 | 8 |
| Oldest supported | 14 | MySQL 8.0 | 8 |
| MariaDB | 18 | MariaDB 11 | 8 |

Every combination includes MongoDB standalone, authenticated, and single-member replica-set deployments. SQLite tests use temporary files without a server.

This command selects the oldest-supported combination:

```sh
MASUME_TEST_POSTGRES_IMAGE=postgres:14-alpine \
MASUME_TEST_MYSQL_IMAGE=mysql:8.0 \
  mise run test-integration-full
```

## The demo recording and the screenshots

Both use the postgres container and the schema in `vhs/seed.sql`:

```sh
mise run demo    # vhs/demo.gif for the README
mise run shots   # the screenshots in vhs/shots
```

Both need Docker and install VHS, ttyd, and FFmpeg through mise when they run. `vhs/config.toml` contains the glyph settings, which need a Nerd Font. Rebuild captures when a change affects the displayed content.

## Code style

The whole tree follows one style. Read a file next to the one you change.

- New function and method names start with a verb, including accessors, private helpers, and named closures.
- Examples are `resolveRequestState`, `buildToolSchemas`, `findUnknownArgument`, and `getDialect`.
- Required interface methods keep their required names, including `Error`, `String`, `Len`, `Close`, and `Unwrap`.
- Existing repository interfaces retain their method names, including `Dialect` and `Capabilities`. Naming guidance does not require unrelated API renames.
- `find` can return no value. `get` returns a value or an error. There is no `OrFail` suffix.
- Names are full and descriptive. Keep meaningful prefixes and use domain names, such as `tableDetail`, instead of `data`.
- Comments are absent by default. Add a comment only for an external quirk that the code cannot express.
- Comments do not restate code, narrate edits, or justify choices. Existing comments remain unless the change requires an update.
- Written text uses simple English, short sentences, active voice, and one physical line per paragraph. Avoid em dashes and nested lists.
- Imports have three groups: standard library, external modules, then this module. `mise run fmt` groups imports.

## Tests

- A change in behaviour needs a test. The test must fail without the change.
- Use table-driven tests when there is more than one case.
- Spec tests live in `*_spec_test.go` files and use the `_test` package. They use only the exported API.
- `internal/ui/frame_safety_test.go` renders views and cards with hostile values. It checks screen width and control characters. Add new views and cards to that test.
- No test needs a server unless it is behind the `integration` build tag.

## User-visible changes

- A new action needs a key binding in every shipped preset. `TestEveryPresetBindsEveryAction` checks every preset against the action catalog.
- A new config key must be added to `config.example.toml` and to the page under `docs/` that covers it.
- For a screen change, include before and after frames in the pull request. `tmux capture-pane -p` writes a text frame.

## Commits and pull requests

- One commit per idea. Keep a rename or a move in its own commit.
- The subject is one imperative line under 72 characters, without a prefix or full stop: `read MariaDB statement plans`.
- The body contains facts about behavior and verification, without design rationale or conversation references.
- A rename updates every reference in the same commit.
- Fill in the pull request template. Tick only the boxes for checks you ran.

## License

A pull request releases its contribution under the Apache License 2.0. No CLA is required.
