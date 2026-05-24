# Explore Lane Ownership And Eval Telemetry Contract

Date: 2026-05-24

Status: Batch design started. This document tracks the T16 telemetry fix and
the T11/B6 typed explore-lane ownership work. It intentionally excludes local
small-model-only gaps unless the same root cause also affects remote models.

## Why This Batch Exists

The focused convergence audit reported 8/9 PASS. The only red case,
`read_combo_git_two_diffs_current_code`, was not a finalizer failure: the
finalizer accepted in one turn. The red signal had two causes:

1. `eval/run.sh` and sweep helpers counted retry/reject strings by grepping the
   entire log. Because this case itself investigates source/eval code that
   contains strings such as `finalizer_rejects` and `finalizer_rewrites`, the
   metrics were content-contaminated.
2. The case regex required `diff/current-source` wording in a narrow shape,
   while the answer expressed the right dimensions across Markdown table cells.

The same audit also confirmed a real product gap: parallel explorer windows can
all deep-dive the same broad topic. `u7k` reached `explorer_iters=67` and
`midloop_inject=24` even though finalizer converged in one turn. Current code
splits exploration by DAG evidence siblings; it does not yet assign exclusive
ownership for typed evidence lanes such as VCS history, VCS diff, current
source, runtime log, trace, command measurement, MCP/web/connector, or
cross-repo index observations.

## Code Audit

### Existing Reusable Pieces

- `internal/types/investigation_plan.go`
  already derives `InvestigationPlan` and `InvestigationUnit` from typed
  analyzer surfaces. It distinguishes user answer buckets from analyzer
  decomposition and does not parse raw prose.
- `internal/types/answer_evidence_origin.go`,
  `AnswerIntentContract`, `AnswerPresentationContract`, and
  `ObservationLedger` already carry typed origins and requested outputs. Do not
  build a parallel evidence stack.
- `internal/orchestrator/dag_node_dispatch.go`
  already splits multi-evidence windows into focused windows. The missing layer
  is lane ownership across those windows, not another sibling splitter.
- `internal/orchestrator/explore_parallel_dispatch.go`
  already emits typed parallel lifecycle events and performs winner-aware merge
  after accepted closure. It is the natural place to attach lane hints and exact
  overlap handling.
- `internal/context/builder.go`
  already passes `ExploreDispatchKey` and `ExploreDispatchKind` into
  `AgentContext`, so lane hints can be added there without broad prompt
  plumbing.
- `internal/render/parallel_activity.go`,
  `internal/render/status_messages.go`, and `internal/render/renderer.go`
  already have a parallel activity model, lane ordinal labels, and localized
  `调查单元` wording. UX should extend this model rather than print new ad-hoc
  status rows.
- `eval/telemetry` already has a structured log collector; however, it also has
  a few broad string checks for finalizer reject/rewrite text. The fix is to
  narrow both telemetry and shell eval counters to control-line semantics.
- Remote update `519c0668 fix: widen source inventory evidence repair` already
  broadened source-inventory repair in `internal/tool/emit_evidence.go` and
  `source_inventory_reconcile.go`. This batch must not duplicate that path.

### Current Gaps

- `eval/run.sh::count_pattern`, `eval/parallel_all.sh`, and
  `eval/parallel_priority.sh` count finalizer rejects from whole-log regexes.
  They can misread source snippets, prompts, or model answers as system
  control events.
- `eval/telemetry/main.go` uses typed `TOOLRESULT ... ok=false` matching for
  some counts, but still increments rejects/rewrites for unscoped Chinese
  render text anywhere in a log line. That is safer than shell grep, but still
  content-contaminable in copied customer snippets.
- `splitExploreWindowForDispatch` and `dispatchExploreWindowsParallel` know
  window identity only as node ids. A worker can start with a VCS/window focus
  and widen into current-source work that another worker also owns.
- `EventParallelDispatchStart` exposes only `ParallelUnitIDs`, so the REPL can
  honestly show “第 N 路 / 并行 N 路,” but not the human-friendly evidence lane
  such as “历史差异” or “当前源码.” The UI should not show internal ids.

## Architecture Contract

### Telemetry

- Eval metrics must count control events, not answer content.
- Tool reject metrics count only diagnostic lines like
  `[diag finalizer] ... TOOLRESULT emit_answer_document ok=false`.
- Render rewrite metrics count only `[render]` lines with `⟳ 4/4 ...`.
- Contract metrics count only `[diag finalizer]` contract-check / repair-plan
  lines, not quoted validation text in prompts or answers.
- Expectation regexes can remain case-specific, but they must be reviewed as
  eval expectations. Product code must not be changed to satisfy a brittle regex
  when the answer already preserves the requested information.

### Explore Lane Ownership

Add `ExploreLanePlan` as a derived, side-effect-free typed view. It reuses:

- `InvestigationPlan` units;
- `AnswerIntentContract.Origins`;
- `AnswerPresentationContract` requested dimensions;
- precise evidence/facet/support contracts;
- accepted `ObservationLedger` / aggregate evidence carriers.

It must not inspect raw user text, model free prose, model thoughts, or grep
frequency. It must not decide the final answer. It only scopes exploration.

Each `ExploreLane` has:

- `id`: stable short key;
- `label`: localized display label seed, derived from typed origin/facet;
- `origin`: e.g. `current_source`, `vcs_metadata`, `vcs_diff`,
  `runtime_artifact`, `command_measurement`, `external_document`, `web`, `mcp`,
  `connector`, `cross_repo_index`;
- `unit_id`: optional `InvestigationUnit.ID`;
- `facet_ids` and `dimension_labels`;
- `role`: `principal`, `support`, or `verification`;
- `coupling`: copied from `InvestigationPlan`;
- `handoff_policy`: `own`, `support`, `verify`, or `delay`.

Exact same `(origin, unit_id, facet_id, dimension_label)` ownership cannot be
principal-owned by two active workers. Exact overlap can demote a sibling to
support/verification or delay it. Similarity heuristics are noisy and may only
drive soft logs or future telemetry.

## UX Contract

- The live dock keeps the current shape: “并行 N 路 · M 个调查单元.”
- When lane labels are available, append a compact localized segment such as
  `证据通道：历史差异、当前源码` or `evidence lanes: VCS diff, current source`.
- Scrollback keeps the already-fixed lane ordinal:
  `探索 · 第 2 路 · 第 10 轮`.
- Do not show internal node ids (`n1_evidence_t0`) or raw enum names as the
  primary UX surface.
- If labels overflow, show the first three plus `等 N 类`. This follows the
  existing status-line brevity pattern and avoids a noisy dashboard.
- UX is informational only. It must not become a gate and must not imply a lane
  has completed unless typed completion state says so.

## Batch Task List

| Batch | Status | Task | Code Areas | Validation |
| --- | --- | --- | --- | --- |
| L0 | Done | Remote update checked and fast-forwarded. New source-inventory repair is acknowledged as existing infrastructure, not duplicated. | git / docs | `git merge --ff-only origin/main` |
| L1 | Done | Harden eval finalizer counters and telemetry parser so content cannot fake finalizer rejects/retries. | `eval/runner_lib.sh`, `eval/run.sh`, `eval/parallel_all.sh`, `eval/parallel_priority.sh`, `eval/telemetry` | `bash eval/runner_lib_test.sh`; `go test ./eval/telemetry`; old failed mixed-VCS log now recomputes `finalizer_rejects=0`, `finalizer_rewrites=0`; case regex now matches existing answer surface |
| L2 | Pending | Add typed `ExploreLanePlan` / `ExploreLane` derived from existing request/intent/presentation/origin contracts. | `internal/types`, tests | unit tests for VCS+source, log+source, trace+source, command+source, user buckets, shared architecture |
| L3 | Pending | Thread lane hints through `BusContext` / `AgentContext` / prompt builder without replacing `ExploreDispatchKey`. | `internal/types/context.go`, `internal/context/builder.go`, explorer prompt | prompt/context tests |
| L4 | Pending | Add scheduler exact-overlap handling. Exact typed duplicate ownership becomes support/verification/delay; no raw-text similarity. | `internal/orchestrator/dag_node_dispatch.go`, `internal/orchestrator/explore_parallel_dispatch.go` | parallel dispatch unit tests |
| L5 | Pending | Add UX lane labels to parallel dispatch events and dock/status rendering. | `internal/render/event.go`, `internal/render/parallel_activity.go`, `internal/render/status_messages.go`, renderer tests | dock/output stream tests in zh/en |
| L6 | Pending | Rerun focused convergence evals and update gap docs with before/after metrics. | eval results + docs | 9-case convergence audit; compare `explorer_iters`, `midloop_inject`, false finalizer counters |

### L1 Implementation Notes

- `eval/runner_lib.sh` now provides `eval_count_finalizer_rejects` and
  `eval_count_finalizer_rewrites`. They count only `[diag finalizer]`
  `phase=toolresult` lines and `[render]` retry/reject lines.
- `eval/run.sh`, `eval/parallel_all.sh`, and `eval/parallel_priority.sh`
  consume the shared helpers instead of each keeping an independent broad
  regex.
- `eval/telemetry` uses the same control-line boundary. Its tests include a
  log where the answer/source text quotes `TOOLRESULT emit_answer_document
  ok=false`, `成文校验未通过`, and `答案待完善`; those quoted strings no longer
  count as finalizer failures.
- `read_combo_git_two_diffs_current_code.case` now accepts `Diff` as well as
  `diff`. The case still requires VCS/current-source/impact dimensions; this
  only removes casing brittleness.

## Guardrails

- Do not let lane ownership drop model-authored rich summaries. Non-owner
  evidence may still merge as support/enrichment when it carries accepted,
  lane-novel evidence.
- Do not conflate user buckets with analyzer subtopics. User buckets stay
  principal partitions; analyzer subtopics are investigation decomposition.
- Do not create hard blocks from noisy similarity, rank score, grep hit count,
  or answer wording.
- Do not system-rewrite model tables. Any system addition must be append-only,
  localized, and clearly marked.
