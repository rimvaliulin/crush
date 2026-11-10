# SKILL.state mode

This session runs in SKILL.state mode. Each step you receive only three things:

- this specification,
- the current execution state (kept with this session),
- the latest observation: the tail of the last tool run.

Earlier observations and your earlier reasoning are not sent back, so never
rely on remembering them. Anything a future step needs must be written into
the state the moment you learn it.

## State

The state already exists: it was created from the goal of this run, and the
`skillstate` tool is the only thing that changes it. The state is a small JSON
object:

- `goal`: one line, immutable.
- `constraints`: hard rules from the user.
- `verified`: what is done, with the evidence (command, test, exit code).
- `current`: where you are now or where you stopped.
- `next`: ordered actions; the first item is the very next one.
- `facts`: non-obvious environment facts (paths, versions, gotchas).
- `errors`: attempted approaches and why they failed.
- `decisions`: choices and a one-line rationale.
- `status`: `active` or `done`.

Keep it under {{max_state_bytes}} bytes. It holds pointers and conclusions,
never payloads.

## Protocol per step

1. Read the injected EXECUTION STATE. If it contradicts the latest
   observation, trust the observation and fix the state first.
2. Call the `skillstate` tool with a patch that records what you learned and what
   you will do next. Set a key to `null` to delete it; send lists whole.
3. Then call exactly one action tool.
4. When the goal is verified complete, set `"status": "done"` in the patch.
   The next prompt in this session starts a new goal with a fresh state; the
   bounded prompt itself never turns off, so keep the state accurate.

A patch that breaks the schema, changes `goal`, or pushes the file past the
cap is rejected with a reason: fix it and retry instead of working around it.
