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

## Follow-up: Runtime Text Search Tooling Gaps

Customer log excerpt at 2026-05-29 16:44 exposed a second-order gap after the
continuation work:

- The model searched an explicit `.systrace` file with `file_type=config` and
  `context_lines=3`. The search still ran, but those parameters are redundant for
  a concrete runtime artifact and make broad first-pass scans heavier.
- The broad grep result correctly emitted `line_window_hint` and `line_windows`,
  but the first search only matched a thread name, so the windows were from an
  earlier timestamp. The tool guidance needs to push models toward the narrowest
  timestamp / thread / event literal first.
- The model then switched to `exec_command grep ... | grep ... | head`, which is
  a reasonable deterministic fallback for numeric windows, but it did not include
  `-n`, so the result could not be directly converted into `read_file` line
  windows or line-scope evidence.
- `exec_command` returned `exit status 1` for a grep pipeline with no matches.
  For grep, that often means "zero matches", not a broken tool. The result should
  explain this and suggest split-field / line-number-preserving recovery.

### Follow-up Contract

1. Explicit single-file runtime/log/trace grep should emit a soft parameter
   advisory when `file_type`, `include`, or broad `context_lines` are present.
   The tool must not silently rewrite the model's query.
2. Runtime/log/trace grep guidance for deterministic commands must say to
   preserve original line numbers (`grep -n` or equivalent) before `read_file`
   grounding.
3. `exec_command` may append search-shape advisories when a shell grep pipeline
   targets a large text artifact and either exits 1 or returns output without
   line numbers. This is advisory only; it must not change the command, success
   semantics, raw output, or answer sufficiency.

### Follow-up Tasks

- [x] T9. Add runtime/log/trace grep parameter advisory for redundant
  `file_type`/`include` and expensive broad `context_lines`.
- [x] T10. Update grep deterministic-command hints to require line-number
  preservation before evidence grounding.
- [x] T11. Add `exec_command` grep-pipeline advisories for no-match exit 1 and
  line-number-free runtime/log/trace output.
- [x] T12. Add targeted tests for code-safe advisory behavior.

2026-05-29 follow-up delivery:

- Grep runtime artifact broad/no-match output now emits `artifact_search_advisory`
  for explicit single-file artifact searches that include redundant
  `file_type`/`include` filters or broad `context_lines`.
- Grep's deterministic-command guidance now explicitly requires preserving
  original line numbers, for example with `grep -n`, before `read_file`
  grounding.
- `exec_command` appends advisory lines for grep pipelines that target runtime
  text artifacts and either exit 1 or return output without original line
  numbers. The command is not rewritten and the success/failure status is not
  changed.
- Added targeted tests in `internal/tool/builtin_test.go`; `go test
  ./internal/tool` passed 2026-05-29.
- `go test ./...` passed 2026-05-29.

## Follow-up: Large Runtime Artifact Search Recovery

Customer log excerpts after the T9-T12 delivery exposed one more model-facing
ambiguity in the same subsystem. The trace file was present and grep-capable,
but the model first called `grep(path=".", files_only=true)`. The Go-native
directory scanner correctly skipped the very large `.systrace` file under its
broad directory safety cap, returned no matches from the scanned subset, and
listed the skipped path. The model interpreted that as "the trace file is too
large to grep" and switched to `read_file(offset=0)`, which is the worst
navigation shape for a 1.5M-line runtime artifact.

### Root Cause

- The native directory-scan no-match text collapsed two distinct facts into one
  sentence: "the scanned subset had no matches" and "some large files were not
  scanned". For runtime/log/trace artifacts, that ambiguity can make a model
  treat a directory-scan safety skip as a capability failure.
- The result named skipped paths but did not provide a concrete next tool call
  shape. The correct recovery is explicit single-file grep on the skipped
  artifact, not reading the artifact from the beginning.
- The read-without-emit nudge is origin-aware, but when the just-read file is a
  runtime/log/trace header or broad initial page it still starts from an
  evidence-materialization frame. That can distract the model from the better
  next step: narrow by timestamp/thread/event, then read the returned line
  window.
- `exec_command` already reminds models to preserve line numbers. It still lacks
  a soft warning for broad runtime grep alternation (`A\|B`, `A|B`) that returns
  early unrelated rows before the intended time window.

### Recovery Contract

1. Directory-scan large-file skips are not absence proof. The tool result must
   say the scanned subset was searched, skipped candidates were not searched,
   and explicit single-file grep remains supported.
2. Runtime/log/trace skipped candidates should include a concrete `next_call`
   example with `path` set to the skipped artifact, `files_only=false`, and
   `context_lines=0`. This is guidance only; the model still chooses the
   pattern.
3. The system must not silently rerun grep with different parameters, rewrite
   regexes, or force evidence emission from non-target runtime artifact pages.
4. Runtime/log/trace `read_file` pages that look like broad/header reads should
   receive a continuation/search-shape nudge, not an evidence-pressure nudge.
5. Runtime grep pipelines with broad OR shapes should get a soft advisory to
   use conjunctive/numeric filtering while preserving original line numbers.
   This must not affect code/config searches.

### Follow-up Tasks

- [x] T13. Split native grep skipped-large no-match output into
  `searched_subset_no_matches`, skipped candidates, and explicit single-file
  recovery guidance.
- [x] T14. Add concrete runtime/log/trace `next_call` guidance for skipped large
  candidates, including "do not read_file from offset 0" wording.
- [x] T15. Make explorer read-without-emit hints runtime-header-aware so broad
  runtime artifact pages steer back to targeted grep/read windows.
- [x] T16. Add `exec_command` soft advisory for runtime grep pipelines that use
  broad OR alternation and return line-numbered output.
- [x] T17. Add targeted tests covering T13-T16 and run focused plus full Go test
  suites.

2026-05-29 large-artifact recovery delivery:

- Native grep directory-scan no-match output now separates
  `searched_subset_no_matches=true` from `skipped_large_candidates=...`; skipped
  large files are explicitly described as unsearched candidates, not absence
  proof.
- Runtime/log/trace skipped candidates now include a concrete
  `next_call=grep(path=..., pattern="<one exact timestamp/thread/event literal>",
  files_only=false, context_lines=0)` and explicitly warn not to start with
  `read_file` from offset 0.
- Explorer read-without-emit guidance detects runtime/log/trace header / broad
  first-page reads and steers the model back to targeted single-file grep or
  line-number-preserving deterministic filters. Mixed code/runtime read batches
  keep the normal source-evidence materialization nudge.
- `exec_command` now appends a soft advisory when a runtime/log/trace grep
  pipeline uses broad OR alternation; the command output and success semantics
  are not changed.
- Targeted `internal/tool` and `internal/agent` tests passed 2026-05-29.
- `go test ./...` passed 2026-05-29.
