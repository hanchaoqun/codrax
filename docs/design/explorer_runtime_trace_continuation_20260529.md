# Explorer Runtime / Trace Continuation Contract

Date: 2026-05-29

## Problem

Customer runtime / trace investigations can exceed one explorer ReAct dispatch. The default
dispatch cap is 20 iterations. When the explorer reaches that cap and still reports
`MissingFacts`, the scheduler requeues the explore node. The UI then renders a fresh
`探索 · 第 1 轮`, and the next prompt only receives a generic fact-retry hint such as
"read more discovered files".

The system does preserve structured progress through `TurnAArtifacts`, but the most useful
frontier for trace investigations often lives in accepted tool results and completion prose:
located line windows, timestamps, thread ids, event names, and the current "next hop" in a
wakeup / blocking chain. If the retry prompt does not make that continuation explicit, the model
can restart broad grep loops even though it already found the active frontier.

## Code Audit

- `internal/agent/agent.go`: `BaseAgent.Execute` obeys `AgentContext.MaxIterOverride`; default is 20.
- `internal/types/config.go`: default `AgentSettings.MaxIterations=20`, `ExplorerScaledIterMax=35`.
- `internal/orchestrator/orchestrator.go`: explorer iteration scaling currently only considers
  multiple sub-topics in the main dispatch path; fact retry is requeued by
  `requeueExploreWindowForFactRetry`.
- `internal/orchestrator/explore_parallel_dispatch.go`: parallel explorer scaling has a separate
  helper and must stay aligned with the main dispatch path.
- `internal/orchestrator/read_stage_retry.go`: transient network retries already build a durable
  continuation checkpoint from `TurnAArtifacts`; content/fact retries should reuse this pattern.
- `internal/agent/explorer.go`: `explorerDurableProgressContinuationActive` already switches the
  explorer into a checkpoint continuation instruction, but it only recognizes transient/prune hints.
- `internal/tool/builtin.go`: grep already supports `fixed_string`, `line_start`, `line_end`,
  streaming runtime artifacts to blob, and a first-line window hint. Large single-file log/trace
  searches still need multiple returned windows and stronger no-match guidance.

## Contract

1. Fact retry continuation is advisory only. It must not decide the answer, suppress model
   conclusions, or introduce new evidence.
2. Runtime/log/trace continuation must preserve origin. It must not force runtime observations into
   repo source `file:line` evidence.
3. Retry guidance must prefer already discovered line windows, timestamps, thread ids, event names,
   aggregate facts, read files, and accepted evidence before broad rediscovery.
4. Grep broad-result guidance must expose multiple line-window clusters when available. This is
   a generic line-number capability for code, configs, logs, traces, and external text artifacts;
   it must not assume systrace format.
5. Grep no-match guidance must teach line-order sensitivity and field splitting without changing
   the model's regex or silently rerunning a different search.
6. Explorer single-topic budget uplift may use typed request signals such as root-cause trace,
   runtime artifact, call chain, sequence diagram, and complexity. It must remain a budget hint,
   not a hard gate or sufficiency decision.
7. User-facing retry text should make continuation visible ("continue previous investigation") so
   a re-dispatch does not look like the system discarded state.

## Task List

- [x] T0. Record the incident, code audit, and contract in this document.
- [x] T1. Add a reusable content/fact retry continuation checkpoint built from preserved explorer
  progress, line-window hints, aggregate facts, and tool-result summaries.
- [x] T2. Teach the explorer durable-continuation path to recognize content/fact retry checkpoints.
- [x] T3. Localize runtime/log/trace fact-retry UX so users see that the pipeline is continuing the
  existing investigation, not restarting.
- [x] T4. Emit multiple grep line-window clusters for broad single-file text results.
- [x] T5. Strengthen runtime/log/trace no-match guidance with split-field and observed-order advice.
- [x] T6. Uplift complex single-topic explorer budgets using typed request shape, aligned across
  main and parallel explorer dispatch.
- [x] T7. Add targeted tests for continuation checkpoints, grep windows / no-match guidance, and
  single-topic budget scaling.
- [x] T8. Run targeted Go tests and update this document with verification results.

## Delivery Notes

2026-05-29:

- `requeueExploreWindowForFactRetry` now prepends an advisory continuation checkpoint before
  re-dispatching the explorer. The checkpoint is built from `TurnAArtifacts`, accepted aggregate
  facts, dispatch tool results, evidence counts, and preserved line-window hints.
- `explorerDurableProgressContinuationActive` recognizes the fact-retry checkpoint and reuses the
  existing checkpoint-continuation instruction instead of starting with a fresh breadth scan.
- Explore fact retry UX now uses a localized continuation message for root-cause/trace/chain-shaped
  requests.
- Grep broad-result output now keeps the existing first-match `line_window_hint` and adds a
  `line_windows=` cluster list when multiple returned windows are visible.
- Runtime/log/trace no-match guidance now explicitly recommends split-field literal probing and
  preserving observed line order before recombining regexes.
- Complex single-topic root-cause trace / call-chain / sequence requests can use the existing
  `ExplorerScaledIterMax` budget. Multi-topic scaling still goes through the same helper.

Validation:

- `go test ./internal/tool ./internal/orchestrator ./internal/agent` passed 2026-05-29.
- `go test ./...` passed 2026-05-29.

## Open Non-Goals

- No automatic semantic parsing of systrace, perfetto, logs, or custom trace formats in this batch.
- No forced retry, forced read, or hard gate based on grep counts or ranker scores.
- No truncation of model-authored content or replacement of model-generated conclusions.
