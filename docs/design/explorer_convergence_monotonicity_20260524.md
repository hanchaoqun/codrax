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
| B5 | Done | Rerun focused evals and refresh gap docs with before/after metrics: `qf_architecture`, `qf_diagram_pipeline`, `qf_type_relation_loop_controller`, `s5b`, `u7k`, mixed log/trace/command/current-source cases, and one small mechanism case. | eval results + gap docs | compare explorer_iters/read_file/midloop/finalizer_iters plus convergence-specific counters |
| B6 | Pending | Add a typed explore-lane ownership planner so parallel explorers do not all chase the same evidence lane. This is a pre-dispatch focus contract, not an answer override: hard decisions may only use typed unit/origin/facet state; model-authored conclusions remain authoritative when evidence is complete. | orchestrator/context/types as needed | focused unit tests + `u7k`, `read_combo_git_two_diffs_current_code`, log/trace/command mixed evals |

### B5 Audit Plan — 2026-05-24 second pass

This pass is an evidence-gathering audit, not a behavior change. It deliberately
does not add a new gate before the eval/log/code evidence proves a deterministic
system defect.

Focused cases:

- `qf_architecture`
- `qf_diagram_pipeline`
- `qf_type_relation_loop_controller`
- `s5b`
- `u7k`
- `read_combo_git_two_diffs_current_code`
- `read_combo_log_current_source_explanation`
- `read_combo_trace_current_source_explanation`
- `read_combo_command_current_source_explanation`

Metrics to record for every run:

- verdict, analyzer/explorer/extractor/finalizer iterations;
- `midloop_inject`, `explorer_dispatches`, `parallel_sibling_skips`,
  `mixed_origin_autocomplete_blocks`;
- `finalizer_rejects`, `finalizer_rewrites`, `semantic_quality_concerns`;
- any model-visible complaint that points to a system contract issue.

Code paths audited before running:

- `internal/orchestrator/explore_parallel_dispatch.go` winner-aware merge:
  after accepted early convergence, only the winning completed fork may merge
  principal state.
- `internal/orchestrator/orchestrator.go::shouldAutoCompleteExploreWindowFromAcceptedClosure`:
  accepted closures may skip later windows only when no typed mixed-origin lane
  is missing and no load-bearing repair/read debt remains.
- `internal/types/repair.go`: support/advisory debt is explicitly separated
  from load-bearing post-closure debt.
- `internal/types/observation_ledger_context.go` and
  `internal/types/observation_ledger.go`: mixed-origin lane coverage is
  compiled from accepted structured carriers, not raw user/model prose.

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
- 2026-05-24 B5 convergence audit design:
  - Do not add new hard gates before collecting focused convergence data. A
    model with full evidence may choose a valid answer structure that differs
    from a preferred system layout; the audit must classify that as presentation
    or model-choice evidence, not as a system override opportunity.
  - Add convergence-specific eval metrics:
    - non-winning parallel sibling skips after an accepted closure;
    - accepted-closure mixed-origin lane blocks;
    - finalizer document/patch rejects and rewrite renders;
    - analyzer/explorer/finalizer dispatch and iteration counts already
      emitted by `eval/run.sh`.
  - Add a focused audit runner that reuses `eval/run.sh` and
    `eval/runner_lib.sh` instead of creating another eval stack. The runner
    emits a markdown summary and is advisory-only. It may flag suspicious
    cases, but flags do not fail the product path and do not become gates.
  - Default focused case list:
    `qf_architecture`, `qf_diagram_pipeline`,
    `qf_type_relation_loop_controller`, `s5b`, `u7k`,
    `read_combo_git_two_diffs_current_code`,
    `read_combo_log_current_source_explanation`,
    `read_combo_trace_current_source_explanation`,
    `read_combo_command_current_source_explanation`.
  - B5 implementation tasks:
    - [x] Extend `eval/run.sh` metrics with convergence-specific counters.
    - [x] Add `eval/convergence_audit.sh` with `PARALLEL=2` default and a
      markdown summary.
    - [x] Smoke-test the runner on a narrow case set.
    - [ ] Run the focused batch, inspect flagged logs, and update this document
      with model-vs-system classification.
  - Smoke validation:
    `CASES="eval/cases/read_combo_command_current_source_explanation.case"
    PARALLEL=1 RUNS=1 TIMEOUT=1200 bash eval/convergence_audit.sh` passed.
    The run reported `finalizer_iters=1`, `finalizer_rejects=0`,
    `finalizer_rewrites=0`, `explorer_dispatches=1`, `midloop_inject=4`, and
    no advisory flags. This verifies the audit runner and new metrics without
    starting the full focused batch.
- 2026-05-24 B5 second focused audit:
  - Command:
    `PARALLEL=2 RUNS=1 TIMEOUT=1500 bash eval/convergence_audit.sh`.
  - Summary:
    `eval/convergence_audit_summary.md`, sweep
    `20260524-152143`.
  - Results:
    - PASS: `qf_architecture`, `qf_diagram_pipeline`,
      `qf_type_relation_loop_controller`, `s5b`, `u7k`,
      `read_combo_log_current_source_explanation`,
      `read_combo_trace_current_source_explanation`,
      `read_combo_command_current_source_explanation`.
    - FAIL: `read_combo_git_two_diffs_current_code`.
  - Metrics snapshot:
    - `qf_architecture`: `explorer_iters=7`, `midloop_inject=3`,
      `finalizer_iters=1`, one semantic advisory. This is not a hard gate; the
      reviewer found the answer sufficient but noted prose could quote more
      identifiers.
    - `qf_diagram_pipeline`: `explorer_iters=50`, `midloop_inject=16`,
      `finalizer_iters=1`. Root cost is repeated read-without-emit and
      evidence-repair hints in a diagram explanation, not finalizer churn.
    - `s5b`: `explorer_dispatches=2`, `explorer_iters=31`,
      `midloop_inject=16`, `finalizer_iters=1`. The first closure lacked
      several package entry-point anchors and triggered `⟳ 2/4 正在补充关键信息`.
      The second pass converged; this is coverage-cost, not answer rewrite.
    - `u7k`: `explorer_iters=67`, `midloop_inject=24`,
      `finalizer_iters=1`. Parallel lanes repeatedly investigated the same
      scalar-history/current-source chain from different directions. The answer
      was accepted in one finalizer turn, so the system gap is pre-dispatch
      focus ownership, not finalizer validation.
    - `read_combo_git_two_diffs_current_code`: `explorer_iters=7`,
      `parallel_sibling_skips=7`, `mixed_origin_autocomplete_blocks=7`,
      `finalizer_iters=1`, but the case failed its explicit
      `diff/current-source` regex. Inspection shows the model had the right
      VCS diff and current-source evidence and produced a good table, but the
      deterministic supplemental source-anchor table at the end shifted the
      visible surface away from the requested wording. The convergence counters
      also show that mixed-origin lane waiting worked, but focus ownership is
      still noisy.
    - `read_combo_log_current_source_explanation`: `explorer_iters=27`,
      `midloop_inject=17`, `finalizer_iters=1`. The answer correctly separated
      LLM stream timeout from contract violation, but evidence repair loops
      around source anchors inflated exploration.
    - `read_combo_trace_current_source_explanation`: `explorer_iters=24`,
      `midloop_inject=7`, `finalizer_iters=1`. Runtime artifact + current
      source lanes both reached finalizer.
    - `read_combo_command_current_source_explanation`: `explorer_iters=10`,
      `midloop_inject=7`, `finalizer_iters=1`. Command measurement and current
      source lanes reached finalizer.
  - Root-cause classification:
    - No evidence of finalizer retry loops in the passing cases. The current
      finalizer path usually accepts the first structured answer.
    - The dominant cost is explorer focus drift: multiple parallel windows can
      be launched with the same broad objective and then independently chase
      the same topic, especially history + current-source chains.
    - B4 prevents accepted closures from skipping missing mixed-origin lanes,
      but it does not assign exclusive ownership of those lanes before dispatch.
      That leaves duplicate work even when correctness is preserved.
  - Some mid-loop hints are valuable repair signals, but repeated
    read-without-emit / evidence-repair hints on the same semantic lane are a
    symptom that the fork was not given a narrow enough contract at launch.

#### Per-case forensics and gap clustering

The B5 audit must be read case-by-case, because the single red row is not a
runtime finalizer failure.

| Case | Result | Forensic finding | Code-correlated gap |
| --- | --- | --- | --- |
| `qf_architecture` | PASS | Finalizer accepted in one turn. The only concern is a semantic-quality advisory: the answer is sufficient but could quote more identifiers such as `IsTerminal()`. | Advisory/reviewer output can still create generic supplement text. Keep it soft and localized; do not turn this into a hard rewrite. Relevant paths: semantic-quality reviewer + answer supplement rendering. |
| `qf_diagram_pipeline` | PASS | Correct diagram answer, but `explorer_iters=50` and `midloop_inject=16`. Workers repeatedly hit read-without-emit and evidence-repair hints while pursuing the same broad architecture target. | Parallel dispatch is sibling-based (`internal/orchestrator/dag_node_dispatch.go`) rather than lane-owned. Need typed lane ownership before launching workers. |
| `qf_type_relation_loop_controller` | PASS | Correct relation coverage after typed relation work, including log/perf/write/sub-explorer implementations. A residual weak-support caveat is low value. | Support-strength caveats should be specific advisory telemetry, not generic visible noise. Lower priority than lane ownership. |
| `s5b` | PASS | First exploration close was rejected/reopened because package entry anchors were incomplete and one closure emitted `aggregate_facts` as the wrong JSON shape. Second pass converged; finalizer still one turn. | Closure schema compatibility and structured member-set repair still cost an explorer retry. Relevant paths: `emit_investigation_complete.aggregate_facts`, explorer closure evaluator. |
| `u7k` | PASS | Worst cost case: `explorer_iters=67`, `midloop_inject=24`. Logs show several parallel workers separately chasing the same scalar-history/current-source chain. | Highest product ROI: add `ExploreLanePlan` ownership keyed by typed origin/facet/dimension/investigation unit; keep B4 missing-lane wait as safety net. |
| `read_combo_git_two_diffs_current_code` | FAIL | The answer preserved both commits, VCS diff facts, current-source anchors,作用/影响 in a table, and finalizer accepted in one turn. The FAIL came from the case regex being too literal/case/line sensitive. The reported `finalizer_rejects=7` / `finalizer_rewrites=7` are false positives: `eval/run.sh::count_pattern` greps the whole log and counted the literal strings inside the requested source/eval content, not finalizer control lines. | Two eval-harness gaps: control metrics must be scoped to structured render/diag lines, and answer expectations for Markdown tables should be semantic/case-insensitive enough to accept `Diff 线索` + `当前关键代码` in separate cells. Product note: deterministic source-anchor supplement was append-only but may be visually noisy. |
| `read_combo_log_current_source_explanation` | PASS | Good mixed log+source answer; it distinguishes first-byte timeout from contract failure. `explorer_iters=27`, `midloop_inject=17` show repair cost, not finalizer churn. | Runtime artifact + current-source lanes need the same ownership/scoping contract as VCS+source. |
| `read_combo_trace_current_source_explanation` | PASS | Good mixed trace+source answer with runtime artifact and current-source evidence. Moderate repair cost. | Same typed lane ownership applies, but urgency is lower than `u7k` and log/source. |
| `read_combo_command_current_source_explanation` | PASS | Good command measurement + source answer. Command-origin fact and source anchor both reached finalizer. | Minor source-attribution/evidence-repair cost; the existing ObservationLedger path is sound. |

Priority order after clustering:

1. **P0 telemetry correctness:** fix eval control counters so answer/source text
   cannot masquerade as finalizer rejects or rewrites. Without this, every
   later ROI decision can be skewed by false failures.
2. **P1 typed lane ownership:** implement B6 so parallel explorers own distinct
   `(origin, facet, dimension, investigation unit)` lanes before dispatch.
   This addresses `u7k`, `qf_diagram_pipeline`, and mixed log/trace/command
   over-exploration with one architecture change.
3. **P2 schema-native repair:** close remaining native-array / JSON-string
   slips for analyzer `source_inventory_profile` and
   `emit_investigation_complete.aggregate_facts` through the shared tool-param
   compatibility layer, not per-tool bespoke parsing.
4. **P3 evidence-repair throttling:** after lane ownership, dedupe repeated
   read-without-emit / evidence-repair hints within the same lane and preserve
   accepted rich summaries instead of reopening broad exploration.
5. **P4 visible noise cleanup:** generic caveats and deterministic supplements
   should stay append-only, localized, and only visible when they materially
   improve the answer. They must not replace model tables or become a reason
   to rewrite.

### B6 Systemic Fix Candidate — Typed Explore Lane Ownership

Problem statement:

Parallel exploration currently splits by DAG evidence sibling windows and uses
winner-aware merge after completion. This avoids stale sibling pollution, but it
does not prevent several active workers from pursuing the same broad evidence
theme. In mixed VCS/log/trace/command + current-source questions, one worker can
start from VCS and another from current source, yet both may recursively expand
into the same scalar/history/current-source chain. The result is high
`explorer_iters` and many mid-loop repair hints even when finalizer converges in
one turn.

Systemic solution:

Add a typed pre-dispatch lane ownership planner. The planner derives lanes from
structured state only:

- `InvestigationPlan` units and coupling (`user_bucket`,
  `analyzer_decomposition`, sequential/comparative/shared-context);
- `AnswerIntentContract.Origins` (`current_source`, `vcs_metadata`,
  `vcs_diff`, `runtime_artifact`, `command_measurement`, `external_document`,
  `web`, `mcp`, `connector`, `cross_repo_index`);
- requested answer dimensions from typed `AnswerPresentationContract`;
- explicit facet/support requirements already present in `AnswerContract` and
  evidence plan nodes.

It must not scan the user question text, model prose, or model thoughts. It must
not decide the answer. It only scopes which worker owns which evidence lane.

Proposed lane contract:

- A lane has: `id`, `origin`, `facet_ids`, `dimension_labels`,
  `investigation_unit_id`, `role` (`principal`, `support`, `verification`),
  `coupling`, and `handoff_policy`.
- A dispatch window receives a lane hint saying what it owns and what it should
  not widen into unless it first emits the owned evidence or a structured
  `need_handoff` completion.
- At most one active worker owns the same `(origin, facet, dimension, unit)`
  key. Sibling workers with overlapping keys are demoted to
  `verification/support` or delayed until the owner finishes.
- Novelty is computed from typed ledger records: an owner has made progress
  only when it adds new accepted evidence/aggregate/tool records for its lane.
  Low novelty can trigger soft guidance or scheduling deprioritization, never a
  hard answer rewrite.
- B4's missing-lane auto-complete check remains the downstream safety net; B6
  reduces duplicated work before the fork starts.

Expected benefits:

- `u7k`-style history/current-source questions should stop launching several
  workers that all chase the same scalar chain.
- Mixed log/trace/command/source questions keep all required lanes, but each
  lane has a clear owner.
- User bucket / comparative questions still get separate owners per bucket, so
  genuinely different user questions are not collapsed.
- Single-repo, multi-repo, code, git, runtime artifact, web/MCP/connector
  sources all use the same origin/facet lane abstraction.

Risks and guardrails:

- Do not over-partition ordinary architecture explanations. If typed state only
  says "shared context" without distinct origins/facets/dimensions, keep the
  current behavior.
- Do not block model exploration because a lane label looks semantically
  similar. Similarity and frequency are noisy; they can only drive soft
  scheduling preferences or telemetry.
- Do not drop rich summaries from non-owner forks if they contain accepted,
  lane-novel evidence. They may merge as support/enrichment, not as competing
  principal state.

B6 task list:

- [ ] Code audit: identify every place that currently builds parallel explore
      windows or hints (`splitExploreWindowForDispatch`,
      `dispatchExploreWindowsParallel`, `runExploreAgentOnFork`,
      `BuildAgentContext`, investigation-plan projection).
- [ ] Add a `types.ExploreLanePlan` / `ExploreLane` derived view that reuses
      `InvestigationPlan`, `AnswerIntentContract`, `AnswerPresentationContract`,
      and `ObservationLedger`; do not create a duplicate evidence carrier.
- [ ] Thread lane hints into `AgentContext` and explorer prompts as scoped
      guidance. The prompt must say the lane is an ownership focus, not an
      answer constraint.
- [ ] Add scheduler-side overlap handling: exact typed lane-key conflicts are
      support/verification/delay candidates; no raw text similarity.
- [ ] Add tests:
      - VCS diff + current-source mechanism creates two distinct owners;
      - log/trace + current-source keeps runtime and source owners separate;
      - command measurement + source keeps count and mechanism lanes separate;
      - user buckets remain separate principal owners;
      - ordinary architecture explanation with shared context is unchanged;
      - non-owner accepted evidence can still merge as support.
- [ ] Re-run focused evals: `u7k`,
      `read_combo_git_two_diffs_current_code`,
      `read_combo_log_current_source_explanation`,
      `read_combo_trace_current_source_explanation`,
      `read_combo_command_current_source_explanation`, plus one user-bucket
      multi-question case.
