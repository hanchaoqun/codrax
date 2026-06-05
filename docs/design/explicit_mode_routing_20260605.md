# Explicit Mode Routing

## Problem

`--mode` and `/mode` currently mix two different concepts:

- user task lane selection: automatic classification, code analysis,
  local computer operation, read-only data processing, and code writing;
- internal write workflow phase: plan, apply, verify.

That makes it hard for users to bypass a classifier mistake safely. It also
forces operation and data requests through one more model classification round
even when the user already knows the intended lane.

## Design

Introduce a user-facing mode axis:

```text
auto | code | operation | data | write
```

Keep the existing orchestrator `PipelineMode` (`read`, `plan`, `apply`,
`verify`) as the internal write-phase axis. It is not user-facing task routing
anymore.

CLI:

```bash
codrax --mode auto -r "..."
codrax --mode code -r "..."
codrax --mode operation -r "..."
codrax --mode data -r "..."
codrax --mode write --write-phase plan -r "..."
codrax --mode write --write-phase apply --plan-file .codrax/plans/p.json --auto-apply
codrax --mode write --write-phase verify --plan-file .codrax/plans/p.json
```

REPL:

```text
/mode auto
/mode code
/mode operation
/mode data
/mode write

/code <question>
/op <task>
/data <data task>
/write <change request>
```

`/mode ...` is sticky for subsequent turns. The one-shot slash commands bypass
only the classifier for that turn and do not mutate sticky mode.

## Red Lines

- Explicit mode bypasses only the classifier.
- It must not bypass operation risk policy, high-risk approval, critical
  denial, data sandboxing, write_enabled, write planning, apply approval, or
  verification.
- No hard gate may parse user prose or model prose keywords. The only hard
  signal is the explicit CLI flag or slash command enum.
- Code, log, trace, mixed source/external-observation, operation, data, and
  write paths must remain separated by typed route policy.
- The internal orchestrator `PipelineMode` remains the write-phase enum to
  avoid reworking stable plan/apply/verify scheduling.

## Task Ledger

- [x] Write this design and task ledger.
- [x] Add typed user-facing mode enum and helpers.
- [x] Rework CLI `--mode` to accept `auto|code|operation|data|write`.
- [x] Add CLI `--write-phase plan|apply|verify` and map it to internal
      `PipelineMode` only when `--mode=write`.
- [x] Make single-shot CLI construct typed route policies directly for
      explicit `code|operation|data|write`.
- [x] Rework REPL sticky `/mode` to use the user-facing mode axis.
- [x] Add REPL one-shot `/code`, `/op`, `/data`, `/write`.
- [x] Update REPL banner and help text.
- [x] Refresh README and user guide without historical process notes.
- [x] Add tests for CLI mode parsing, REPL sticky modes, one-shot slash modes,
      and safety gates.
- [x] Run focused and full tests, then push.
