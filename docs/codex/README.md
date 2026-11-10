# Codex harness

`codex` runs a session the way the [Codex CLI](https://github.com/openai/codex)
does: the model reads Codex's base instructions, the whole transcript is
replayed every step, and every tool output is bounded before it reaches the
model.

Pick it when creating a session (the harness dialog) or start one directly:

```sh
crush run --harness codex "Refactor the parser and prove the tests pass"
```

The harness is fixed for the life of the session, like every other one.

## What it changes

- **Instructions.** The base instruction text of Codex is appended after the
  system prompt: personality, AGENTS.md precedence, preamble messages,
  planning with `todos`, task execution, validation, ambition vs precision,
  final answer formatting, and the tool output limit. It is a port of
  `codex-rs/protocol/src/prompts/base_instructions/default.md` (openai/codex,
  Apache-2.0), rewritten for the tools Crush provides: `edit`, `multiedit` and
  `write` replace `apply_patch`, `todos` replaces `update_plan`, and Crush's
  permission modes replace the sandbox/approval injections.
- **Bounded observations.** A tool output longer than `max_output_bytes`
  reaches the model cut in the middle, the way Codex truncates:

  ```
  Warning: truncated output (original byte count: 37830)
  1
  ...
  …25949 bytes truncated…
  ...
  20000
  ```

  The model keeps the beginning and the end and the marker says how much was
  dropped. Nothing is removed from the session: the transcript on disk keeps
  every byte, only the prompt is bounded.
- **Status.** `crush_info` reports the harness, the cap and what the last step
  had to cut:

  ```
  [harness]
  name = codex
  cap_bytes = 12000
  truncated_outputs = 0
  removed_bytes = 0
  window = last step
  ```

## Configuration

```json
{
  "options": {
    "harness": {
      "codex": {
        "max_output_bytes": 24000
      }
    }
  }
}
```

The default is 12000 bytes: Codex stores 10 000 bytes of content plus the 20%
allowance it adds for the surrounding JSON. `0` or a negative value restores
the default.

## What is not ported

Codex carries a lot of machinery this harness leaves out on purpose:

- code mode (`exec`/`wait` wrappers and the JS sandbox),
- sandbox and approval-policy prompt injections (Crush has its own permission
  model),
- multi-agent tools and collaboration modes,
- hosted web search and plugin/memory tools,
- server-side and background compaction, and Codex's token accounting. Crush
  keeps the full transcript; when a session outgrows the window, summarize it.

## Layout

- `internal/harness/codex/harness.go` - name, status, transcript shaping.
- `internal/harness/codex/spec.md` - the instruction text appended to the
  system prompt. It mentions the output cap through `{{max_output_bytes}}`,
  which is filled in from the configuration.
