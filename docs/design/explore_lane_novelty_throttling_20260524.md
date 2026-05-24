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

## Task List

| Task | Status | Validation |
| --- | --- | --- |
| T20.1 Audit lane ownership and budget paths. | Done | This document |
| T20.2 Add support-only lane budget cap after existing multi-subtopic scaling. | Done | `go test ./internal/orchestrator -run TestApplyExploreLaneHandoffIterationCap` |
| T20.3 Document next accepted-delta novelty ledger work. | Done | Gap doc update |
| T20.4 Focused eval rerun on `u7k` and mixed runtime/source cases. | In progress | `u7k-20260524-191826` |
| T20.5 Add same-lane accepted-delta novelty advisory/telemetry. | Planned | focused `u7k`, log/source, trace/source, VCS/current reruns |

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
