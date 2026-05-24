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
| T20.4 Focused eval rerun on `u7k` and mixed runtime/source cases. | In progress | `u7k-20260524-191826`; rebuilt `u7k-20260524-200712` / `u7k-20260524-201412` showed good answers and no finalizer churn; remaining mixed runtime/source breadth pending |
| T20.5 Add same-lane accepted-delta novelty advisory/telemetry. | In progress | unit tests done; focused `u7k`, log/source, trace/source, VCS/current reruns pending |
| T20.6 Make read-without-emit hints origin-aware so VCS/log/trace/command observations are not forced into file:line `emit_evidence`. | Done | agent unit tests; rebuilt mixed VCS/current eval prompt audit proved origin-aware hint was injected and finalizer stayed one turn |
| T20.7 Surface per-worker lane labels in durable parallel scrollback. | Done | `go test ./internal/render ./internal/orchestrator -run 'TestRenderer_ParallelExplorerScrollbackShows|TestDispatchExploreWindowsParallel_ScopesLanePlanPerEvidenceWindow|TestScopeExploreLanePlansForWindows_DemotesExactDuplicateLane|TestApplyExploreLaneHandoffIterationCap'` |

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
