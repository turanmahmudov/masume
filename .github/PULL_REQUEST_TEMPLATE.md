# What this changes

<!-- One paragraph. What masume does after this change that it did not do before. -->

Closes #

## Why

<!-- The problem this solves. Link the issue that reports it. -->

## How it was checked

<!-- List what you ran and the result. Please do not tick a box for a check you did not run. -->

- [ ] `mise run check` passes.
- [ ] `mise run test-integration-full` passes, if this change touches an engine.
- [ ] There is a test that fails without this change.

## What the screen shows

<!--
If this change is visible on screen, include the frame before and after.
`tmux capture-pane -p` writes a frame as text. A screenshot works too.
Remove any value you cannot share.
-->

## Notes for the reviewer

<!--
Mention these if they apply:
- A new dependency, and why it is needed.
- A new config key. Add it to `config.example.toml` too.
- A new action. It needs a key binding in the default preset, otherwise a test fails.
-->

- [ ] Nothing here contains a password, a private host name, or real data.
