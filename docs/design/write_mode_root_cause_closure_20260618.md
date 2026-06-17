# Write Mode Root-Cause Closure And SWE-bench Recovery Plan

Date: 2026-06-18
Branch: codex/cc-like-swe-gap-20260618
Status: active delivery plan

## Inputs Reviewed

- `/Users/han/opt/cc_like.md`
- `docs/design/write_mode_claude_code_online_convergence_architecture_20260617.md`
- `docs/design/swebench_manual_audit_20260618.md`
- `internal/agent/write_controller.go`
- `internal/orchestrator/write_controller_scheduler.go`
- `internal/writeflow/patch_review.go`
- `internal/writeflow/impact/engine.go`
- `internal/writeflow/convention/learner.go`
- `internal/types/impact_obligation.go`
- `internal/types/impact_analysis.go`
- `internal/types/convention_graph.go`
- `eval/swebench/run_codrax_swebench.py`

## Current State

Codrax has already moved well past the old batch-only write mode:

- controller-first durable workflow exists;
- `WorkflowExecutionView`, transition validation, approval execution, and
  observation authority are typed;
- `WriteContextPack` carries prioritized P0-P3 handoff;
- `PatchReviewRecord` reviews actual applied patch effects, not only plan
  prose;
- `ImpactAnalysisResult` and `ImpactObligationSet` exist and are derived from
  `ChangePlan`, actual `PatchEffectRecord`, and repomap graph relations;
- `ConventionGraph` exists as an inspectable soft artifact derived from
  exploration evidence and repository graph signals;
- SWE-bench adapter exports harness-consumable predictions and local telemetry.

The 137-instance manual audit changed the quality picture:

| Signal | Count | Rate | Interpretation |
| --- | ---: | ---: | --- |
| Non-empty patch | 130 / 137 | 94.9% | Export works; not correctness. |
| Local authoritative verify pass | 18 / 137 | 13.1% | Useful but weak and sometimes false-positive. |
| Strict manual audit pass | 30 / 137 | 21.9% | Current conservative internal correctness floor. |
| Definite manual fail | 28 / 137 | 20.4% | Wrong-layer, incomplete, empty, or contradicted. |
| Unknown | 79 / 137 | 57.7% | Needs official harness/deeper execution. |

## Root Cause Assessment

The low pass rate is not mainly a patch-export problem. It is also not one
single parser, prompt, or language-specific issue.

The dominant system gaps are:

1. **Root-cause localization is often too shallow.**
   The model can patch the symptom site or caller adapter instead of the owner
   boundary. Manual fail examples include wrong-layer fixes where local verify
   passed but the behavioral owner remained wrong.
2. **Impact analysis exists but is not yet a scheduling authority.**
   `ImpactAnalysisResult` emits verification targets, yet missing dependent,
   changed-symbol, or behavior-contract coverage is mostly telemetry. It does
   not consistently force re-explore/replan while budget remains.
3. **Patch Critic is real-diff aware but its findings are under-consumed.**
   `ReviewAppliedPatchSemantic` creates typed findings from actual diff,
   impact obligations, and convention graph. SWE/local acceptance previously
   ignored those findings when computing local correctness proxy.
4. **Verification can prove too little.**
   Local project environments are often partial. Passing a narrow probe or weak
   local check can be useful evidence, but it cannot prove owner-boundary
   correctness without changed-symbol/contract/dependent coverage.
5. **Convention learning is present but still advisory.**
   The graph is inspectable and evidence-backed, which is good, but it has not
   yet become a stable contributor to patch review and replan priorities.

## Commercial Design Direction

Do not add issue-specific checks. Do not route on user intent keywords, model
`<think>`, summaries, rationales, or manual notes.

The system should converge around this typed loop:

```text
symptom -> typed localization pack -> bounded plan -> actual diff ->
impact obligations -> patch review -> focused verify -> observation authority ->
finish | re-explore | replan | split | block
```

Hard gates consume only typed artifacts:

- `ChangePlan`
- `PatchEffectRecord`
- `ImpactAnalysisResult`
- `PatchReviewRecord`
- `ChangeReport`
- `WorkflowExecutionView`
- `WriteContextPack`
- deterministic risk/permission records

Model prose stays visible and transparent but cannot drive control.

## P0 Delivery Tasks

1. **Patch-review local acceptance boundary**
   - Export `plan_patch_review_*` fields from the SWE-bench adapter.
   - Treat typed patch-review hard errors and unverified semantic coverage as
     local acceptance blockers.
   - Do not block official prediction export.
   - Do not parse manual notes, issue text, stdout, or model prose.

2. **Patch-review feedback into controller**
   - Promote high-signal semantic coverage findings into P2 context pack rows.
   - When budget remains and findings are unverified, prefer replan/re-explore
     over finish.
   - Keep this behind typed transition validation and observation authority.

3. **Root-cause coverage score**
   - Derive a small typed score from `ImpactAnalysisResult.VerificationTargets`
     and `PatchReviewRecord.Findings`.
   - Track changed-symbol, behavior-contract, dependent-surface, and related
     test coverage separately.
   - Use it as confidence/replan guidance, not as prose-driven routing.

## P1 Delivery Tasks

4. **Impact obligations become a first-class repair queue**
   - Convert uncovered obligations into bounded follow-up slices.
   - Preserve source evidence refs and priority.
   - Avoid broad re-investigation unless the obligation has no owner path or
     symbol.

5. **Convention graph review expansion**
   - Add convention mismatch findings only when a convention node is tied to
     the active path/symbol with evidence.
   - Keep style findings advisory unless coupled with structural or semantic
     coverage failures.

6. **Verifier target selection from impact**
   - Let deterministic TestSurface prefer related tests and changed symbols
     from `ImpactVerificationTarget`.
   - Keep verifier tool input as `{}`; the model does not choose broad suites.

## P2 Delivery Tasks

7. **Official harness scoring lane**
   - Run fixed SWE-bench Lite groups through official harness and report
     `resolved/total`.
   - Keep fair-run history isolation enabled only for SWE-bench.

8. **Language-wide owner-boundary producers**
   - Extend actual-diff line feature events beyond Python using existing
     repomap features where available.
   - The output remains typed patch effect events, not natural-language
     keyword rules.

9. **UX automation**
   - Surface one next-action card from `WorkflowExecutionView`.
   - Keep `/workflow` and `/plan` as audit/recovery commands, not required
     routine usage.

## Progress Ledger

| Batch | Status | Notes |
| --- | --- | --- |
| RC-0 | complete | Reviewed `cc_like.md`, current write-mode typed harness, impact/convention/patch-review code, and 137-instance manual audit. Root cause classified as root-cause localization plus impact/verify closure, not patch export. |
| RC-1 | complete | Implemented patch-review local acceptance boundary in the SWE-bench adapter. `results.jsonl` now exports `plan_patch_review_*` typed fields, and patch-review hard errors plus unverified semantic coverage block local acceptance telemetry while official prediction export remains unchanged. Verification: adapter unit tests and Python compile pass. |
| RC-2 | pending | Feed patch-review semantic findings into controller context/replan policy. |
| RC-3 | pending | Add root-cause coverage score and focused impact repair queue. |
| RC-4 | pending | Extend verifier target selection from impact targets and run official harness groups. |

## Acceptance Criteria

- SWE local acceptance no longer counts a patch as pass when typed
  `PatchReviewRecord` says the actual diff has a hard error or unverified
  semantic coverage.
- Official prediction export remains unchanged and harness-consumable.
- Runtime hard gates continue to read typed artifacts only.
- No prompt red-line changes: no keyword routing over user intent or model
  prose.
- Current read/log/trace/data/operation/computer paths remain untouched.
- All new eval fields and docs distinguish export compatibility from
  functional correctness.
