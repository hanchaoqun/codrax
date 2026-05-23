# Explorer Convergence Monotonicity

Date: 2026-05-24

Status: active design and implementation tracker.

## Goal

Make exploration completion monotonic and commercially stable:

- once a typed, accepted `emit_investigation_complete` closes an exploration
  facet, later equivalent/support-only work must not reopen, overwrite, or
  pollute the principal answer state;
- explicit multi-facet user requests still wait for the required sibling
  handoffs;
- rich grounded evidence summaries remain available downstream;
- no logic relies on matching raw user/model prose for hard decisions.

This is language-neutral. The implementation is in orchestration, typed
aggregate/evidence carriers, and fork merge policy. It does not inspect Go-only
syntax and should benefit every language supported by repo-map parsing
(Go/Java/Kotlin/ArkTS/TS/JS/Python/C/C++/Cangjie/proto/etc.) as well as
non-source observations such as git, logs, traces, command output, MCP, web,
external documents, and connector data.

## Code Audit Before Design

The current code already has most primitives. The work here is to tighten their
contracts and remove ambiguous merge behavior, not to create a new evidence
stack.

| Area | Current code | Finding |
| --- | --- | --- |
| Parallel dispatch | `internal/orchestrator/explore_parallel_dispatch.go` | `dispatchExploreWindowsParallel` creates forked mutable states, can cancel siblings after `exploreParallelResultConverged`, and already has typed wait rules via `parallelExploreMustWaitForSiblingHandoffs`. |
| Convergence predicate | `exploreParallelResultConverged` | Currently only checks `fork.IsInvestigationComplete()`. This trusts the tool's pre-complete gates, which is good, but it does not mark which lane won or constrain later merge. |
| Fork merge | `internal/types/context.go::MutableState.MergeExploreFork` | Merges evidence, Turn A artifacts, aggregate facts, retained closure state, phase1 ranking, and closure repair state. When the fork is complete, it clears pending reads/repairs. |
| Parallel result merge | `dispatchExploreWindowsParallel` | After a lane converges, other workers are canceled; however all already-finished non-error results are still merged in declaration order. A non-completed sibling that finished before cancellation can import support-tier evidence, aggregate facts, stage output, or pending state. |
| Aggregate merge | `internal/types/answer_aggregate_fact.go::MergeAnswerAggregateFacts` | Good member-set arbiter exists: compatible sets merge, stale subsets can demote, explicit principal buckets stay distinct. It cannot know which parallel lane's closure was the accepted answer boundary. |
| Accepted closure reuse | `internal/orchestrator/orchestrator.go::shouldAutoCompleteExploreWindowFromAcceptedClosure` | Already avoids redispatch when policy is soft/override and no pending validation/repair/read debt exists. It is intentionally conservative, but support-only reads can still make completion non-monotonic. |
| Tool closure | `internal/tool/emit_investigation_complete.go` | Accepted completion sets aggregate facts, result kind, closure reason, retains them, and clears pending reads/repairs. Downgrades do not flip completion. This is the right source-of-truth boundary. |
| Turn A artifact merge | `internal/agent/turn_a_merge.go` and `types.MergeExploreFork` | Keeps rich read/tool/evidence summaries across windows. This must remain intact; the fix must not drop model-authored summaries from the accepted lane. |

## Root Causes

### R1. Cancellation Exists, But Principal Merge Is Not Winner-Aware

Current flow:

1. lane A accepts `emit_investigation_complete`;
2. orchestrator calls `cancel()`;
3. some sibling lanes may have already returned non-error outputs;
4. merger sorts all results by window index and merges every non-error fork.

This can turn parallelism into duplicated work and can pollute downstream
principal facts with stale, broader, or partial sibling facts. This matches the
open gaps:

- `E20260522-G42`: equivalent architecture/diagram/type-relation lanes keep
  running/merging after enough evidence exists;
- `E20260522-G87`: an exact exhaustive closure is polluted by later/other
  lane aggregate facts;
- `E20260522-G55`: broader/late member sets can outrank narrower user-scope
  sets.

### R2. Accepted Closure Is Stable, But Later Support Debt Can Reopen Work

Accepted completion clears active pending reads/repairs at tool time and during
completed fork merge. But `shouldAutoCompleteExploreWindowFromAcceptedClosure`
still refuses auto-completion when any pending read/repair exists. That is
right for load-bearing obligations, but too broad for support/adjacent anchor
debt after a typed closure. This matches `E20260522-G38` and `E20260522-G145`.

### R3. Hybrid-Origin Work Needs Facet Ownership, Not More Lanes

For mixed VCS/log/trace/command + current-source questions, each lane can
independently search both origins. This duplicates cost and can blend
commit-local/diff facts with current-source line anchors. This matches
`E20260522-G152`. The long-term design should partition by typed origin/facet,
but the first safe step is to make accepted lane ownership explicit so sibling
output cannot silently become principal.

## Contract

### C1. Winning Closure Owns Principal State

When early convergence is allowed and a lane accepts
`emit_investigation_complete`, that lane becomes the winner for the current
parallel dispatch. The parent may merge the winning completed fork and any
other completed fork that also accepted completion. It must not merge a
non-completed sibling fork into principal state after a winner exists.

Rationale: if early convergence is allowed, the typed request shape has already
said sibling handoff is not required. A sibling that did not close is at most
supporting context. Keeping it out of principal merge prevents stale aggregate
facts and forced-read debt from outliving the accepted boundary.

### C2. Explicit Multi-Facet Obligations Still Wait

Early convergence remains disabled for typed shapes that require sibling
handoffs:

- explicit user buckets / declared count / completeness obligation;
- exhaustive enumeration and relation member-set handoffs;
- required diagram contract;
- active change-impact or field-value profiles;
- mixed-origin mechanism/trace/diagram/diagnostic/change-impact/comparison/
  enumeration/absence outputs where current source and a non-source origin are
  both required.

These rules already live in `parallelExploreMustWaitForSiblingHandoffs` and
must stay typed-only.

### C3. Evidence Richness Is Preserved From Accepted Lanes

Do not discard or compress evidence summaries from the winning completed fork.
The accepted lane's `TurnAArtifacts`, evidence items, aggregate facts, flow
findings, read files, tool results, and closure reason remain the downstream
source for extractor/finalizer prompts.

Non-winning, non-completed siblings are not principal. Later batches may add an
explicit support-only merge lane, but it must be opt-in typed telemetry and must
not create required answer members, closure facts, repair debt, or visible
system supplements.

### C4. Post-Completion Reads Are Enrichment, Not Re-closure, Unless Typed
Load-Bearing

After a valid accepted closure, support/adjacent forced reads should be handled
as deterministic enrichment or a localized boundary note. They must not ask the
model to re-close the same investigation. Reopening exploration after closure
requires a precise typed reason: missing explicit bucket/facet, missing required
diagram, missing exact requested current-source anchor, unresolved change-impact
target, etc.

### C5. No Hard Decisions From Raw Prose

Every wait/cancel/demote decision in this design comes from structured state:
request model, answer contract, aggregate facts, result kind, closure flags,
pending read/repair directive metadata, and observation origins. Raw user text,
model thoughts, and free-form closure prose can be displayed or summarized, but
cannot be parsed for hard control flow.

## Implementation Plan

| Batch | Status | Task | Code / docs | Validation |
| --- | --- | --- | --- | --- |
| B0 | In progress | Land this design and task tracker. | `docs/design/explorer_convergence_monotonicity_20260524.md`, `gap_architecture_retriage_20260523.md` | doc review |
| B1 | Done | Make parallel result merge winner-aware: when early convergence fires, merge only the winning completed fork; skip non-winning siblings and log why. Preserve existing full merge when early convergence is disabled. | `internal/orchestrator/explore_parallel_dispatch.go`, tests | `go test ./internal/orchestrator -run 'TestDispatchExploreWindowsParallel|TestParallelExploreAllowsEarlyConvergence'` |
| B2 | Done | Add regression tests proving non-winning partial forks cannot import aggregate facts, StageOutput, or pending repair debt after a winning closure; explicit enumeration/bucket/diagram/mixed-origin waits still merge siblings. | orchestrator/type tests | targeted unit tests |
| B3 | Pending | Tighten accepted-closure auto-complete around support-only post-completion debt. Audit pending read/repair directive origins and add a typed helper that distinguishes load-bearing from enrichment/advisory. | `internal/orchestrator/orchestrator.go`, `internal/types/repair.go`, `internal/types/evidence_closure.go` | forced-read/reconcile unit tests |
| B4 | Pending | Add origin/facet partition follow-up for hybrid external+current-source questions. First design the typed lane ownership contract; then implement if code already has enough metadata. | orchestrator dispatch hints, answer intent contract, observation ledger | mixed VCS/log/trace/command evals |
| B5 | Pending | Rerun focused evals and refresh gap docs with before/after metrics: `qf_architecture`, `qf_diagram_pipeline`, `qf_type_relation_loop_controller`, `s5b`, `u7k`, and one small mechanism case. | eval results + gap docs | compare explorer_iters/read_file/midloop/finalizer_iters |

## Safety Checklist

- Do not weaken `emit_investigation_complete` pre-complete gates.
- Do not infer convergence from free-form text.
- Do not suppress required sibling handoffs for explicit user structure.
- Do not remove accepted-lane rich summaries.
- Do not create new evidence carriers when `ObservationLedger`,
  `AnswerAggregateFact`, `TurnAArtifacts`, and `EvidenceClosure` already cover
  the data.
- Keep single-threaded / non-parallel behavior unchanged.

## Progress Log

- 2026-05-24 B0: audited current code paths for parallel dispatch, fork merge,
  accepted closure reuse, aggregate fact merge, and Turn A artifact merge.
  Root cause is a missing winner-aware merge boundary, not lack of parallel
  dispatch.
- 2026-05-24 B1/B2: implemented winner-aware parallel result merge. When
  early convergence is allowed and one lane accepts completion, non-winning
  sibling forks are skipped instead of merged into parent state. Regression
  guard proves a partial sibling cannot leak `StageReport`, repair debt,
  `TurnAArtifacts.AcceptedAggregateFacts`, or stable aggregate members after
  the winning closure. Existing tests still prove explicit enumeration and
  mixed-origin mechanism questions wait for sibling handoffs.
