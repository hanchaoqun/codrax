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
| B3 | Done | Tighten accepted-closure auto-complete around support-only post-completion debt. Added typed helpers that keep unknown/exact pending reads blocking, while advisory repairs and known breadth/support pending reads no longer reopen exploration after an accepted closure. | `internal/orchestrator/orchestrator.go`, `internal/types/repair.go`, `internal/orchestrator/accepted_closure_monotonicity_test.go` | `go test ./internal/orchestrator -run 'TestShouldAutoCompleteExploreWindowFromAcceptedClosure|TestDispatchExploreWindowsParallel|TestParallelExploreAllowsEarlyConvergence'`; `go test ./internal/types -run 'Test.*Repair|Test.*PendingRead|TestMergeExploreFork_AcceptedCompletionClearsSiblingRepairs'` |
| B4 | Done | Add origin/facet partition follow-up for hybrid external+current-source questions. First design the typed lane ownership contract; then implement if code already has enough metadata. | orchestrator dispatch hints, answer intent contract, observation ledger | mixed VCS/log/trace/command evals |
| B5 | In progress | Rerun focused evals and refresh gap docs with before/after metrics: `qf_architecture`, `qf_diagram_pipeline`, `qf_type_relation_loop_controller`, `s5b`, `u7k`, and one small mechanism case. | eval results + gap docs | compare explorer_iters/read_file/midloop/finalizer_iters |

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
- 2026-05-24 B3: added `PendingReadBlocksAcceptedClosure` and
  `RepairBlocksAcceptedClosure`. The default remains conservative: unknown
  origins, `pre_complete.primary_anchor`, `required_file_hint_unread`, and
  `pre_complete.multi_path_anchor` still block. Known breadth/support origins
  such as `phase1_unread` and `chain_promotion.concrete_values_tracer*`, plus
  advisory repairs, no longer turn a valid accepted closure into another
  explorer round.
- 2026-05-24 B4 design after remote sync:
  - The latest remote change `384b287c` front-loads call-chain principal-span
    repairs. That solves a same-lane closure repair problem and should be
    reused as-is; it does not solve mixed-origin lane ownership.
  - Code audit found enough existing metadata for the first B4 slice:
    `CompileAnswerIntentContract` declares requested evidence origins,
    `ObservationLedgerInputFromBusContext` projects accepted current-source,
    VCS/diff, runtime/log/trace, command, MCP/web/connector-style records, and
    `shouldAutoCompleteExploreWindowFromAcceptedClosure` is the exact chokepoint
    where a prior accepted closure can skip later explore windows.
  - Gap: parallel dispatch already disables early cancellation for mixed
    current-source + non-source mechanism/trace/diagram/diagnostic/change-impact
    shapes, but serial or later-window auto-complete does not currently verify
    that the accepted closure actually contains each requested origin lane. A
    VCS/log/trace/command lane can therefore close first and skip the
    current-source sibling, or vice versa.
  - B4 typed lane ownership contract:
    - Required lanes come only from `AnswerIntentContract.Origins`, never raw
      user/model prose.
    - Coverage comes only from `ObservationLedger` records compiled from
      accepted evidence, aggregate facts, tool results, log/trace bundles, and
      MCP responses.
    - The first slice is origin-level, not output/facet-level. It blocks
      auto-complete only when the request is a typed mixed-origin sibling-handoff
      shape and at least one requested origin has no accepted ledger record.
      It does not add a hard rejection, does not rewrite answers, and does not
      create a new evidence carrier.
    - Output/facet partitioning remains a follow-up: once origin lanes are
      proven present, later work can map requested outputs/facets to each lane
      using the same `AnswerIntentContract` and claim-binding helpers. Do not
      infer facets from prose.
  - B4 task list:
    - [x] Add an orchestrator helper that returns missing requested origins for
      accepted-closure auto-complete by comparing `AnswerIntentContract` with
      `ObservationLedger`.
    - [x] Wire the helper into
      `shouldAutoCompleteExploreWindowFromAcceptedClosure` so missing mixed
      lanes dispatch the next window instead of being skipped.
    - [x] Add regression tests for mixed VCS/current-source and
      runtime/current-source auto-complete blocking, plus scalar/history and
      observation-only non-regressions.
    - [x] Run focused unit tests.
    - [ ] Continue B5 evals.
- 2026-05-24 B4 implementation:
  - `shouldAutoCompleteExploreWindowFromAcceptedClosure` now checks typed
    mixed-origin lane presence before treating an accepted closure as enough to
    skip later explore windows.
  - The check uses only `CompileAnswerIntentContract` and
    `ObservationLedgerInputFromBusContext` / `CompileObservationLedger`; it
    never scans the user request or model prose. Current-source coverage must
    have a current-checkout file span. VCS/log/trace/command/MCP-style lanes
    must carry a typed origin-specific observation record.
  - Regression coverage:
    - mixed VCS metadata/diff + current-source mechanism blocks until both
      lanes are present;
    - runtime artifact + current-source explanation blocks until current-source
      evidence is present;
    - observation-only runtime artifacts and scalar history lookups continue to
      auto-complete without opening source lanes.
  - Validation:
    - `go test ./internal/orchestrator -run 'TestAcceptedClosureAutoComplete|TestParallelExploreAllowsEarlyConvergence|TestDispatchExploreWindowsParallel_MixedOrigin'`
    - `go test ./internal/orchestrator ./internal/types`
- 2026-05-24 B5 first eval:
  - `read_combo_git_two_diffs_current_code-20260524-113636` ran to completion:
    `exit_code=0`, `analyzer_iters=3`, `explorer_iters=9`,
    `extractor_iters=1`, `finalizer_iters=1`, `midloop_inject=4`,
    no finalizer rewrite/reject.
  - The answer preserved both VCS and current-source content, but failed the
    case regex because the model did not visibly keep the requested labels
    `diff 线索 / 当前关键代码 / 作用 / 影响` under each commit. This is not a
    B4 early-convergence failure; it is a soft presentation-dimension prompt
    weakness. The finalizer prompt has been strengthened to ask for explicit
    per-subject dimension labels without adding a hard gate or system-generated
    table. Continue B5 with log/trace/command mixed cases after this commit.
  - Re-verification after the prompt-only dimension-label fix:
    `read_combo_git_two_diffs_current_code-20260524-114950` passed with
    `analyzer_iters=2`, `explorer_iters=8`, `extractor_iters=1`,
    `finalizer_iters=1`, `midloop_inject=3`, `tool_read_file=4`, and no
    finalizer reject/rewrite. The final answer visibly preserved the requested
    labels as `diff 线索与当前关键代码` and `作用与影响` under each commit while
    still keeping VCS diff evidence and current-source evidence in the same
    answer. This validates the B4 mixed-origin auto-complete guard for the
    VCS/current-source lane without introducing a deterministic supplement.
  - UX follow-up in the same batch: analysis-stage `emit_analysis` summaries
    now render typed requested answer dimensions and current-source explanation
    profile details in the REPL status output. This is presentation-only and
    does not feed model context or gates.
