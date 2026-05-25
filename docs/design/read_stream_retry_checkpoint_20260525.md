# Read-Mode Stream Retry Evidence Checkpoint

Date: 2026-05-25

## Problem

A read-mode exploration dispatch can make durable progress, then fail on a
stream-level transport error before `dispatchStage` returns cleanly. Existing
protection only covered the narrow case where `emit_investigation_complete` had
already produced an accepted closure. When the model had accepted evidence but
not yet closed the investigation, the scheduler requeued the whole explorer
window with the ordinary DAG hint. The next prompt looked like a fresh
investigation, so the model could restart broad `repo_map` / `grep` / `read_file`
work and lose the user's high-value progress in practice.

This is not a tool-call budget issue. It is a retry-continuation contract issue.

## Constraints

- Do not mark exploration complete merely because evidence exists. That would
  let the system decide sufficiency for the model.
- Do not inspect user prose or model narrative for control flow.
- Do not hard-block legitimate follow-up search.
- Preserve the model's ability to decide whether to continue reading or call
  `emit_investigation_complete`.
- Reuse existing state: `MutableState`, `TurnAArtifacts`, DAG window hints, and
  pre-prune checkpoint behavior.

## Design

Add a transient retry checkpoint for explore-stage stream errors that happen
after durable progress but before accepted closure.

The checkpoint is advisory and model-facing:

1. Detect durable progress from structured state only:
   - accepted / mutable evidence,
   - bus evidence, flow findings, answer chains, symbols,
   - accepted aggregate facts / closure state,
   - already captured read/tool results from the same dispatch.
2. Store a one-shot retry directive on the scheduler state.
3. On the next explorer dispatch, prepend that directive to the normal DAG
   window hint.
4. The directive says this is a continuation from a preserved checkpoint, not a
   fresh investigation. It asks the model to reuse accepted evidence and avoid
   repeating broad repo-wide scans unless the checkpoint is clearly missing the
   needed target. It may continue with narrow follow-up reads or close with
   `emit_investigation_complete`.

The accepted-closure fast path remains separate: if the model already emitted a
valid closure before the stream failure, the scheduler may safely advance. The
new path never advances by itself; it only improves the retry prompt.

## Remaining Gap Audit

- Pre-prune checkpoint: already implemented in `agent.go` and covered by unit
  tests. It preserves accepted structured state before raw tool history is
  elided.
- Analyze stage: usable IR after a transient failure already advances rather
  than reanalyzing.
- Extract stage: standalone transient retry is already separated from content
  retry budget.
- Finalize stage: accepted / recoverable finalizer output has its own transient
  preservation path.
- Open follow-up: measure how often explore transient retries install this
  checkpoint, and whether any later broad scan still occurs without new scoped
  rationale.

## Task List

- [x] Document root cause and safe boundary.
- [x] Add one-shot explore transient checkpoint hint to `graphState`.
- [x] Install the hint only for stream-level explore retry with structured
      progress and no accepted closure.
- [x] Prepend the hint to the next window hint without replacing normal DAG
      objectives.
- [x] Add regression tests for evidence-before-stream-stall retry continuation.
- [x] Run focused tests and full Go test suite.
