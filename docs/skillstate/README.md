# SKILL.state sessions

Skillstate mode keeps the prompt of every step bounded. Instead of replaying the
whole transcript, each step receives only three things:

- the specification (the system prompt plus the SKILL.state protocol),
- the current execution state,
- the latest observation: the tail of the conversation since the last
  assistant message (tool calls, their results and any prompt queued since).

The prompt stops growing with the execution horizon: a hundred tool calls cost
the same tokens as one. Everything a later step needs must therefore be
written into the state the moment it is learned.

## The mode belongs to the session

A session is either a SKILL.state session or a normal one, and the choice is
made once, when the session is created. There is no switch to flip later: a
SKILL.state session keeps a bounded prompt for its whole life, and a normal
session never becomes one. Deleting the session is the only way to end the
mode.

Create one with:

- the `New Session` command in the TUI command palette (`ctrl+n`), which first
  asks which harness the session should run;
- `crush run --harness skillstate "..."`, which creates a fresh session.

The flag only applies to a new session: with `--session` or `--continue` it is
an error, because the running session already has its harness.

A session without a chosen harness runs the default one. It is a harness like
any other (`harness.DefaultName`, `internal/harness/default.go`): it appends
nothing to the system prompt, keeps the whole transcript and owns no tools.
`--harness default` asks for it by name.

SKILL.state is one harness among possibly several. A harness is a named
implementation of `internal/harness.Harness`, and SKILL.state's own lives in
`internal/harness/skillstate`: it owns the specification, the per-session
state, the shape of the step prompt and the tools the model sees.
A session stores only the harness name in `sessions.harness`, so adding a
harness (its own package, an entry in `harness.Catalog`, a line in
`app.New`) never touches the session, proto, backend or agent layers, and the
palette does not grow with the number of harnesses: the new-session dialog
lists the catalog.

The execution state lives next to the conversation it belongs to: one row per
session in the Crush database (`.crush/crush.db` by default, table
`skill_state`).

Crush creates the row on the session's first turn, taking `goal` from that
turn's prompt, and leaves it alone afterwards: the row is the model's memory
and nothing overwrites it. Deleting the session deletes its state.

`status` is `active` while the goal runs and `done` once it is verified.
A `done` goal makes the next prompt in that session a new goal with a fresh
state; the bounded prompt itself never turns off.

A created state looks like this:

```json
{
  "status": "active",
  "goal": "one line, immutable",
  "constraints": ["hard rules from the user"],
  "verified": {},
  "current": "where the run stands now",
  "next": ["the very next action"],
  "facts": {},
  "errors": {}
}
```

A stored state that does not parse is never replaced. The step stays bounded
and receives an `EXECUTION STATE is unavailable` note with the reason, so a
corrupt row cannot silently turn the mode back into a full transcript.

`decisions` is optional. Objects (`verified`, `facts`, `errors`, `decisions`)
map a short key to a short value; `constraints` and `next` are lists of
strings. The compact form of the state, the one every step receives, must stay
under the configured cap: store pointers and conclusions, never payloads.

The cap defaults to 4096 bytes and is set per harness:

```json
{
  "options": {
    "harness": {
      "skillstate": {
        "max_state_bytes": 8192
      }
    }
  }
}
```

`0` or a negative value restores the default. The specification sent to the
model and the rejection message both quote the cap actually enforced, so a
raised limit does not leave the model guessing. The cap applies to patches:
the initial state, seeded from the prompt's goal, is stored as-is, so a cap
smaller than the goal makes every patch fail instead of truncating the goal.

## The `skillstate` tool

The tool is attached only to SKILL.state sessions; a normal session never
sees it. It takes a `patch` object and:

- merges objects key by key, so untouched keys survive,
- deletes a key whose patch value is `null`,
- replaces lists and strings whole,
- rejects a patch that breaks the schema, changes `goal`, or pushes the state
  past the cap, leaving the row untouched and returning the reason,
- writes the result in one transaction, so a crash cannot leave half a state.

Patches are serialised by the database: the read-modify-write cycle runs in an
immediate transaction with a busy timeout, so parallel tool calls in one step
and a second Crush instance on the same data directory cannot lose an update.
There is no lock file to clean up after a crash.

## What a step looks like

1. The model reads the injected `EXECUTION STATE` block.
2. It calls `skillstate` with what it learned and what it will do next.
3. It calls exactly one action tool.
4. When the goal is verified, it sets `"status": "done"`.

The next step reads the new state, so a fact written in step 1 of a hundred is
still there in step 100, at a constant token cost.

## When not to use it

Skillstate mode trades recall for a bounded prompt. Work that needs to reread
long output, compare many files line by line, or hold a large diff in view is
better served by a normal session, where the transcript is the memory.
