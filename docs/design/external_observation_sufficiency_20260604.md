# External Observation Sufficiency and Source-Sidecar Noise Control

Date: 2026-06-04

## Problem

Recent representative evals showed that MCP / runtime external observations can
now reach the answer pipeline with correct typed line/row facts, but explorer may
still read current-source files as incidental sidecar support after the external
facts already answer the user's question. The final content can be correct, but
the extra source reads cost latency and can pollute final-answer context with
unrelated implementation files.

This is not an MCP-only issue. The same pattern can occur for typed log rows,
trace_query output, connector rows, external docs, web resources, or command
measurements when the current request is about the external observation itself
and not about current-checkout implementation.

## Root Cause

- `ObservationLedger` correctly separates external observations from current
  source citations.
- `TurnRouteHint` and `ExternalObservationPolicy` already provide typed routing
  context, and `CurrentSourceLaneDecision` separates required source evidence
  from optional source exploration.
- Explorer close-readiness is still source-shaped in key places. In particular,
  `postCompletionReadySignal` requires successful `emit_evidence`, which is a
  current-source evidence tool, before it marks a lane close-ready.
- External observations can therefore be rich enough for an answer, but the
  explorer receives no early typed signal saying: "source is optional here; close
  instead of reading sidecar source."

## Red Lines

- Do not infer user intent from Go keyword scans over the raw request.
- Do not hard-gate on model-authored prose.
- Do not suppress source analysis when a typed current-source lane is required.
- Do not change code/write/log/trace mixed scenarios where current-source proof
  is explicitly requested or structurally required.
- Do not make large/broad external outputs automatically sufficient; the signal
  must require typed, addressable observations.

## System Design

Add a typed `ExternalObservationSufficiency` assessment that consumes only:

- `ObservationLedger` records compiled from accepted producer outputs.
- `RequestModel.CurrentSourceLaneDecision()`.
- `TurnRouteHint` from the structured REPL/CLI classifier.

The assessment returns `sufficient_for_answer` only when all are true:

- Current source is not required by typed request state.
- The turn is externally originated or the request has an external observation
  artifact reference/runtime artifact lane.
- The accepted observations are non-current-source records.
- Records are addressable through line, row, JSON pointer, selector,
  time-window, payload/rowset/page/resource refs, or trace support refs.
- The candidate set is small enough to be a direct answer surface; broad output
  must continue through normal investigation.

Explorer then uses this typed result in two places:

1. Initial guidance:
   - If external observation is first-class and source is optional, tell the
     model that typed external rows may be enough and source reads are optional
     verification, not required sidecars.
2. Mid-loop close-ready:
   - Before source-shaped completion readiness, if sufficiency is satisfied and
     there are no source-required blockers, issue a bounded hint to call
     `emit_investigation_complete` with the external facts and boundary.

Finalizer coverage guards remain unchanged and continue to protect typed
selector values in the final answer.

## Task List

### Batch 1: Design and Ledger

- [x] Record the issue, root cause, design, red lines, and task list.

### Batch 2: Typed Sufficiency Primitive

- [ ] Add `ExternalObservationSufficiency` types/helpers in `internal/types`.
- [ ] Cover MCP typed rows, runtime trace_query records, and mixed source-required
  negative cases with unit tests.
- [ ] Ensure the helper consumes typed structures only.

### Batch 3: Explorer Integration

- [ ] Cache the current dispatch's MCP responses in explorer for mid-loop checks.
- [ ] Add an external-observation close-ready signal before source-shaped
  completion readiness.
- [ ] Add prompt guidance for external-observation-first, source-optional
  dispatches.
- [ ] Keep explicit source/mixed requests on the existing source lane.

### Batch 4: Eval and Guardrails

- [ ] Add/adjust focused tests for MCP-only line facts, MCP+source, and
  trace/log+source mixed cases.
- [ ] Rerun `mcp_typed_line` and manually review both tool usage and final answer.
- [ ] Rerun at least one mixed external+source case to confirm source analysis is
  still preserved.

## Expected Outcome

External observation questions with sufficient typed rows should close without
reading unrelated current-source files. Mixed code/log/trace/source questions
should still use current-source evidence normally. The behavior is driven by
typed lane/sufficiency state, not by keyword matching or case-specific patches.
