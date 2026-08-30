# What this changes

<!-- One paragraph. What masume does after this change that it did not do before. -->

Closes #

## Why

<!-- The problem this solves. Link the issue that reports it. -->

## How it was checked

<!-- Say what you ran and what it answered. Please do not tick a box you did not run. -->

- [ ] `mise run check` is green.
- [ ] `mise run test-integration-full` is green, if this change touches an engine.
- [ ] There is a test that fails without this change.

## What the screen shows

<!--
If a user sees this change, put the frame before and the frame after here.
`tmux capture-pane -p` writes a frame as text, and a screenshot works too.
Remove any value you cannot share.
-->

## Anything the reviewer should know

<!--
Worth calling out, if it applies:
- A new dependency, and why it is needed.
- A new config key. It belongs in `config.example.toml` too.
- A new action. It needs a chord in the default preset, or a test will fail.
-->

- [ ] Nothing here holds a password, a private host name, or real data.
