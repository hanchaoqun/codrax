# Explore Lane Novelty And Throttling

Date: 2026-05-24

Status: T20 active implementation tracker.

## Goal

Reduce repeated exploration where several explorer workers keep digging the
same broad topic after a typed lane already has an owner. This is a performance
and UX fix, not an answer gate.

The system must not decide the answer for the model. Low novelty can only:

- lower scheduling priority;
- reduce budget for support/verification handoff lanes;
- add prompt guidance to verify/enrich instead of repeating the same search;
- improve telemetry.

Low novelty must never:

- hard-stop a principal lane;
- reject a model answer;
- delete model-authored evidence summaries;
- infer intent from raw user text or model prose.

## Code Audit

| Existing piece | Finding |
| --- | --- |
| `types.ExploreLanePlan` | Already derives typed lanes from `InvestigationPlan`, `AnswerIntentContract`, and `AnswerPresentationContract`. It does not inspect raw prose. |
| `scopeExploreLanePlansForWindows` | Already demotes exact duplicate ownership to `handoff=support`. This is a precise zero-novelty signal. |
| `BuildAgentContext` / explorer prompt | Already renders lane ownership and tells support/verify lanes to enrich or verify instead of repeating. |
| `applyExploreIterationScaling` | Can increase budget for multi-subtopic work, but does not reduce budget for support-only duplicate lanes. |
| `ObservationLedger` / aggregate facts | Rich enough for a future accepted-delta novelty ledger, but the first safe slice should not wait for a full new scheduler. |

## Design

### Slice 1: Exact Duplicate Handoff Budget Cap

If a worker's scoped `ExploreLanePlan` contains no principal `own` lane and
only support/verification/delay handoff lanes, that worker is not the principal
owner. It should receive a small exploration budget sufficient to:

- check whether it has a genuinely new typed observation;
- emit a concise support/verification handoff;
- avoid repeating the owner lane's producer/search loop.

This is safe because the principal owner lane still receives normal budget, and
explicit multi-facet waits remain protected by the existing typed handoff rules.

### Slice 2: Future Accepted-Delta Novelty Ledger

After Slice 1, add a ledger over accepted typed deltas:

- evidence identity: origin + source/file/payload + line/span + anchor;
- aggregate identity: kind + label + role + normalized members/support refs;
- observation identity: origin + source ref + span + row/payload refs.

The ledger can score whether a new worker/round produced new typed facts. It
must remain advisory. No hard gate may depend on fuzzy similarity, grep counts,
or prose overlap.

### Slice 2 Implementation: Same-Lane Low-Novelty Advisory

The first accepted-delta slice is implemented inside the explorer mid-loop,
after repair/completion-ready checks and before generic read-more nudges. It
compiles typed delta identities from:

- `EvidenceItem` stable IDs;
- retained `aggregate_facts` identities;
- `ObservationLedger` records compiled from accepted evidence, aggregate facts,
  tool results, log/perf bundles, and future MCP-compatible carriers.

When a lane already has typed facts and later navigation batches add no new
typed delta for two consecutive rounds, the explorer receives a concise
"emit the concrete new fact or close" hint. This is deliberately not a hard
stop, and it does not restrict principal lanes or rewrite evidence.

The navigation detector includes source tools and external-observation tools
(`git_log`, `git_show`, `git_diff`, `git_history_search`, and
`exec_command`). A first VCS/runtime/command observation counts as novelty, so
mixed history/current-source questions are not penalized for collecting the
external half of the answer.

During validation of `u7k-20260524-194928`, the prompt audit exposed a separate
mixed-origin contradiction: the initial prompt correctly told the explorer not
to convert VCS findings into fake `emit_evidence` file:line rows, but the
generic read-without-emit mid-loop hint still said all facts not passed through
`emit_evidence` are invisible. That generic wording caused the model to
re-anchor commit clues to design-document title lines. The fix is origin-aware:
read-without-emit hints now require `emit_evidence` only for current-checkout
source claims, while VCS/diff/log/trace/command/repo-index/external-document/
web/MCP/connector observations stay in `emit_investigation_complete.reason` and
`aggregate_facts`. The same origin-aware guard prevents read-without-emit
runtime tool-surface restriction from blocking VCS/command navigation in mixed
lanes.

## Task List

| Task | Status | Validation |
| --- | --- | --- |
| T20.1 Audit lane ownership and budget paths. | Done | This document |
| T20.2 Add support-only lane budget cap after existing multi-subtopic scaling. | Done | `go test ./internal/orchestrator -run TestApplyExploreLaneHandoffIterationCap` |
| T20.3 Document next accepted-delta novelty ledger work. | Done | Gap doc update |
| T20.4 Focused eval rerun on `u7k` and mixed runtime/source cases. | Done | `t20-collective-20260524-223835`: log/source and trace/source passed with one-turn finalizers; `u7k` product answer was good and the remaining failure was a brittle file-name assertion now widened to current stable implementation files. |
| T20.5 Add same-lane accepted-delta novelty advisory/telemetry. | In progress | unit tests done, including lane-scoped VCS/current-source protection; focused replay proves no finalizer churn but still shows early duplicate work (`log/source exp_it=30`, `trace/source exp_it=26`). |
| T20.6 Make read-without-emit hints origin-aware so VCS/log/trace/command observations are not forced into file:line `emit_evidence`. | Done | agent unit tests; rebuilt mixed VCS/current eval prompt audit proved origin-aware hint was injected and finalizer stayed one turn |
| T20.7 Surface per-worker lane labels in durable parallel scrollback. | Done | `go test ./internal/render ./internal/orchestrator -run 'TestRenderer_ParallelExplorerScrollbackShows|TestDispatchExploreWindowsParallel_ScopesLanePlanPerEvidenceWindow|TestScopeExploreLanePlansForWindows_DemotesExactDuplicateLane|TestApplyExploreLaneHandoffIterationCap'` |
| T20.8 Scope low-novelty streaks by typed origin and disable the hint when same-origin lanes cannot be mapped precisely. | Done | `go test ./internal/agent -run TestPostSameLaneLowNoveltySignal` |
| T20.9 Add collective typed-lane convergence for parallel explore. | Done | Orchestrator unit test covers canceling support siblings after required typed lane closures. Focused replay `t20-collective-20260524-223835` kept finalizer to one turn on log/source and trace/source; residual early duplicate digging remains under T20.5/T20.10 lane ownership. |
| T20.10 Keep tightly-coupled analyzer units in one dispatch window. | Done | Reuses `InvestigationCoupling` and `exploreWindowDispatchGroups`; tests cover runtime+current shared context, independent split, and user-bucket split. |

## Slice 1 Validation

Focused `u7k-20260524-191826` passed with a clean one-turn finalizer, but it
still reported `explorer_iters=66`, `midloop_inject=19`, and
`explorer_dispatches=0`.

This is an important boundary: the exact duplicate handoff cap is safe and
prevents support-only duplicate workers from consuming a full explorer budget,
but it does not solve same-lane deepening inside the main explorer path. The
next slice must therefore track accepted typed deltas within a lane and provide
soft "record or close" guidance when later rounds add no new evidence,
aggregate facts, or external observations.

The guidance remains advisory. It may shorten or steer repeated support work,
but it must not hard-stop a principal lane or reinterpret user/model intent.

## Slice 2 Validation

Focused rebuild validation:

- `u7k-20260524-200712` and `u7k-20260524-201412` both produced useful
  answers with commit clues and current-source implementation files. The
  finalizer accepted the first answer document without reject/rewrite.
- The prompt audit confirmed the mixed-origin read-without-emit hint now says
  VCS/diff/log/trace/command/repo-index/external-document/web/MCP/connector
  observations are first-class evidence carried through
  `emit_investigation_complete.reason` and `aggregate_facts`.
- The previous absolute wording, "anything not passed through `emit_evidence`
  is invisible", no longer appears on the mixed-origin path. It remains valid
  only for pure current-checkout source claims.
- The focused eval failure was a brittle case expectation: the answer separated
  the commit section and current implementation section instead of using the
  literal word `当前` on the same expected surface. The case expectation was
  changed to assert semantic coverage of `commit`, `scalar`, and the current
  implementation files rather than forcing a particular phrasing. Per validation
  policy, the good product answer was not rerun just to satisfy the old regex.

Remaining open convergence work: the same-lane advisory fired but did not fully
eliminate long exploration in `u7k`; that stays under T20.5. A separate
`condition` + `line_range` evidence-repair strictness issue was observed during
the same replay and is tracked as future evidence-repair hardening, not as a
T20.6 prompt conflict.

## Slice 4 Lane-Scoped Novelty Ledger

Status: Done for the code-level guard; focused eval replay still pending.

The first same-lane ledger was global inside an explorer dispatch. That was safe
for ordinary single-origin runs, but mixed-origin questions exposed a subtle
failure mode: accepted VCS/log/command facts could make later current-source
navigation look like "no novelty" even though the worker had moved to a
different evidence lane. That would be a soft hint rather than a hard gate, but
it is still a system nudge based on the wrong lane and therefore violates the
same architectural red line.

The ledger now scopes accepted-delta accounting by typed evidence origin:

- current-source navigation counts only current-source accepted deltas;
- VCS history/diff navigation counts only VCS accepted deltas;
- command navigation counts command-measurement deltas unless the tool summary
  carries a structured `evidence_origin=` tag such as `vcs_metadata`;
- repo-map/list-files navigation can count current-source and cross-repo-index
  deltas;
- observation ledger records keep their own origin.

This is still advisory only. If no origin can be derived from structured tool
metadata, the old all-lane behavior is preserved rather than guessing from
prose.

## Slice 5 Origin-Scoped Streak And Ambiguity Guard

Status: Done for the code-level guard; focused eval replay still pending.

The first low-novelty implementation filtered accepted deltas by evidence
origin, but its "two rounds without novelty" streak was still global inside the
explorer dispatch. That created a subtle red-line risk: a VCS history lane could
consume one no-novelty round, and a later current-source lane could inherit that
streak even though it was investigating a different typed evidence origin.

The streak is now keyed by the typed origin scope used for the current
navigation batch. A VCS no-novelty round and a current-source no-novelty round
do not accumulate into one warning. Additionally, if the scoped
`ExploreLanePlan` contains multiple lanes with the same origin but different
ownership keys, the evaluator does not emit the low-novelty hint for that
origin. The evidence items currently do not carry a precise investigation-unit
key, so the system cannot safely decide which same-origin lane owns a fact.
Suppressing the hint is intentionally conservative and keeps the mechanism in
the "soft guidance only" lane.

This still does not hard-stop exploration, does not rewrite evidence, and does
not infer duplicate topics from raw prose or similarity.

## Slice 6 Collective Typed-Lane Convergence

Status: Done for post-closure scheduling; early duplicate lane ownership remains
under T20.5/T20.10.

Focused replay on 2026-05-24 showed that the correctness path is mostly healthy:
mixed VCS/current-source, log/current-source, trace/current-source, and
command/current-source answers reached the finalizer in one turn without
document rejects. The remaining cost problem is orchestration-level: when
`parallelExploreMustWaitForSiblingHandoffs` disables single-winner early
convergence, the scheduler currently waits for every parallel window, including
support-only duplicate windows, even after every required typed lane owner has
accepted closure.

The fix must stay typed and conservative:

- compute required lane ownership keys from `ExploreLanePlan` after per-window
  scoping and duplicate demotion;
- mark a key covered only when that window's fork accepted
  `emit_investigation_complete`;
- cancel remaining workers only after all required owner keys are covered;
- merge only the accepted owner windows after collective convergence, so
  partial support-only repairs and stale aggregate facts do not leak into the
  parent state;
- do nothing when there is no precise lane plan, when the required set is empty,
  or when a hard shape such as exhaustive enumeration still lacks an accepted
  owner closure.

This is scheduling only. It does not inspect raw user text, does not compare
model prose, does not reject answers, and does not change any finalizer contract.

Validation after remote `e477ce42`:

- `go test ./internal/orchestrator ./internal/agent ./internal/types -run
  'Test(DispatchExploreWindowsParallel|ParallelExploreAllowsEarlyConvergence|AcceptedClosureAutoComplete|CompileExploreLanePlan|PostSameLaneLowNoveltySignal)'`
  passed.
- `eval/results/t20-collective-20260524-223835` showed one-turn finalizers and
  zero finalizer rejects/rewrites for log/source and trace/source mixed cases.
- `u7k` produced a useful answer with commit lineage and current-code scalar
  chain; the failure was a brittle eval expectation that required older file
  names instead of the current stable files used by the answer.
- After remote `81ac9726`, package tests still passed. The remote change is
  scoped to repo-map/source-inventory projection and grouped lens output, so it
  does not alter this scheduling contract; a future focused eval can measure
  whether the improved scoped lens reduces source-inventory exploration breadth.
- Residual gap: log/source and trace/source still begin duplicate early
  exploration before any owner lane has a typed closure. The next batch should
  add pre-dispatch lane ownership / novelty budget, not a harder finalizer or
  supplement gate.

## Slice 7 Coupling-Aware Pre-Dispatch Ownership

Status: Done for typed shared/sequential unification. Focused eval confirms the
change reduces early duplicate exploration on runtime+current-source cases.

The remaining `t20-collective-20260524-223835` cost is early duplication: before
any worker can close a typed lane, log/trace + current-source questions may
launch several analyzer sub-topic windows that all grep/read the same small set
of performance/log parsing files. This is not a finalizer or validation problem.

Design:

- reuse the existing `InvestigationCoupling` typed field; do not infer coupling
  from raw user text or model prose;
- classify external runtime artifact + current-source verification as
  `shared_context`, because the runtime observation and current source
  explanation are two facets of the same answer, not independent user buckets;
- keep analyzer-decomposition windows with `shared_context` or `sequential`
  coupling unified in `exploreWindowDispatchGroups`;
- preserve splitting for explicit user buckets / comparative partitions and for
  independent sub-topics, so loose multi-question requests still benefit from
  parallel exploration;
- this is scheduling-only. It does not mark evidence complete, does not decide
  the final answer, and does not drop accepted rich summaries.

Validation:

- unit tests for runtime+current-source shared coupling and unified dispatch:
  `TestCompileInvestigationPlan_RuntimeArtifactCurrentVerificationIsSharedContext`
  and
  `TestExploreWindowDispatchGroups_RuntimeCurrentSharedContextStaysUnified`;
- regression tests proving ordinary independent sub-topics still split and user
  buckets remain distinct:
  `TestExploreWindowDispatchGroups_OrdinaryMultiTopicStillSplits` and
  `TestExploreWindowDispatchGroups_UserBucketsStillSplitDespiteSharedSignals`.
- focused replay `eval/results/t20-coupled-20260524-230105`:
  - `read_combo_log_current_source_explanation`: PASS, finalizer one turn,
    `finalizer_rejects=0`, `finalizer_rewrites=0`, `explorer_iters=9`.
    Previous collective replay was `explorer_iters=30`, so this removes the
    early duplicate fan-out without changing answer validation.
  - `read_combo_trace_current_source_explanation`: PASS, finalizer one turn,
    `finalizer_rejects=0`, `finalizer_rewrites=0`, `explorer_iters=7`.
    Previous collective replay was `explorer_iters=26`.
  - Both cases used one explorer dispatch group (`explorer_dispatches=1`),
    proving the shared-context pre-dispatch grouping is active. The log case
    still has one non-blocking semantic reviewer concern about deeper call-chain
    explanation; track that as a T11/T17 answer-quality follow-up, not as a
    scheduling or finalizer-gate regression.

## Slice 3 UX Validation

Parallel explorer scrollback now keeps the existing ordinal shape while adding
localized per-worker lane labels when the scheduler has an exact lane plan. For
example, a mixed VCS/current-source worker renders as:

`探索 · 第 2 路（历史差异、当前源码） · 第 5 轮`

This is render-only telemetry. It is populated from `ExploreLanePlan.Labels()`
on `EventParallelDispatchUnitStart`, stored in the renderer's parallel activity
state, and never fed back into scheduling, gates, prompts, or answer content.
If a worker has no scoped lane labels, the old `第 N 路` surface remains
unchanged.
