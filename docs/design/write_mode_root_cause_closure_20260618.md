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

## 2026-06-18 SWE-bench Lite Smoke Audit

Run directory:
`/private/tmp/codrax-swebench-rc4-providers-20260618-011240`

The adapter produced three non-empty predictions and validated that
`predictions.jsonl` is consumable by the official SWE-bench harness command.
This is an export/format signal only; it is not functional correctness.

| Instance | Export | Local typed verdict | Manual audit | Root-cause note |
| --- | --- | --- | --- | --- |
| `django__django-14534` | non-empty | failed verify + semantic coverage unverified | fail | The owner site was found, but the patch used `attrs["id"]` instead of a nullable lookup. It misses the no-auto-id boundary represented by the reference test. |
| `pytest-dev__pytest-11143` | non-empty | audit-blocked due semantic coverage unverified | pass | The actual diff implements the reference behavior. This is a false negative from missing targeted verification/coverage proof in a partial local environment. |
| `sympy__sympy-23117` | non-empty | audit-blocked + workflow still in semantic follow-up | fail | The patch addresses the empty iterable symptom but misses the adjacent mutable index behavior covered by the reference test patch. |

Strict manual pass for this focused smoke set is `1 / 3 = 33.3%`.

New conclusions:

- Provider/config forwarding was an evaluation harness gap: running a Codrax
  binary from an isolated worktree must be able to forward the real
  `providers.yaml`; otherwise every instance can fail before exercising write
  mode.
- The low manual pass rate is mixed:
  - root-cause localization can be too shallow for adjacent behavioral
    obligations;
  - impact obligations are still not strong enough to force "changed A implies
    check B" closure before export;
  - local verification is often unavailable because project dependencies are
    incomplete, so unavailable verify cannot be a hard product gate;
  - patch review is correctly conservative for incomplete patches, but it also
    false-negatives correct patches when the system lacks a targeted proof.
- Export compatibility, local typed acceptance, and manual correctness must stay
  separate metrics. Non-empty patch rate cannot be presented as functional pass
  rate.

## 2026-06-18 RC-7 SWE-bench Lite Smoke Audit

Run directory:
`/private/tmp/codrax-swebench-rc7-20260618-014254`

Again the adapter produced three non-empty, official-harness-consumable
predictions. Strict manual pass remained `1 / 3 = 33.3%`:

| Instance | Export | Local typed verdict | Manual audit | Root-cause note |
| --- | --- | --- | --- | --- |
| `django__django-14534` | non-empty | failed verify + semantic coverage unverified | fail | The patch improved from direct nested access to `.get("id", "")`, but the required no-auto-id boundary expects `None` (`.get("id")`). |
| `pytest-dev__pytest-11143` | non-empty | audit-blocked due semantic coverage unverified | pass | The patch covers the non-string docstring crash; it is broader than the reference patch but preserves the intended guard. |
| `sympy__sympy-23117` | non-empty | failed verify timeout | fail | Online replan improved localization from `_scan_iterable_shape` to `_loop_size`, but the final patch still uses shape `()` instead of the reference `(0,)` and misses adjacent mutable-index behavior. |

New deterministic gap:

- The verifier ran a scoped pre-suite verification probe for SymPy, and the
  probe passed. It still escalated to full `pytest` solely because the probe did
  not carry every required `contract_ref`
  (`verification_probe_missing_required_contract_ref`). That converted useful
  scoped proof into a five-minute timeout and left the workflow in progress.
- Missing `contract_ref` coverage should remain a typed confidence downgrade and
  handoff signal. It must not be a standalone reason to run an expensive project
  suite after a passing scoped probe. Real escalation should remain tied to
  failed probes, plan-touched test/spec paths, missing changed-symbol coupling,
  or explicit runner policy.

## 2026-06-18 RC-8 SymPy Verification Check

Run directory:
`/private/tmp/codrax-swebench-rc8-sympy-20260618-020754`

The SymPy single-instance rerun validated the selector fix:

- The workflow produced a non-empty official-harness-consumable prediction.
- The verifier no longer escalated a passed scoped probe into full `pytest`
  solely because of `verification_probe_missing_required_contract_ref`.
- The workflow completed with local `verify_status=passed`.
- The missing contract refs remained visible as
  `prediction_confidence_downgrade_reason=verification_probe_missing_required_contract_ref`
  and patch-review semantic coverage remained unverified.

Manual correctness still failed: the final patch made `Array([])` locally pass
for the system's scoped invariant but still did not match the reference behavior
(`shape == (0,)`) and did not include the adjacent mutable-index fix from the
reference patch. This is a root-cause/invariant-extraction gap: the workflow can
now iterate online, but it can accept an under-specified invariant when the issue
mentions a comparator behavior ("Matrix([]) works") and no deterministic probe
forces that comparator into the expected contract.

Next root-cause project:

- Add typed comparative-observation obligations during write analysis /
  exploration when the issue or runtime evidence presents an observed failing
  expression alongside a working comparator. The model may propose the
  comparator, but control must consume only typed probes and evidence refs.
- Project comparator probes into P1/P2 context and verifier probes with explicit
  contract refs, so local pass means "matches observed comparator semantics",
  not merely "does not throw".
- Keep this as a generalized phenomenon-to-invariant closure lane, not a
  SWE-bench/SymPy-specific rule.

## 2026-06-18 RC-9 Comparator Invariant Closure Design

The RC-8 SymPy rerun proved that the online loop can stop too early when the
local probe proves an under-specified invariant. The failing system shape is
general:

```text
observed failing subject + known working/contrasting reference
  -> plan proves only "the subject no longer crashes"
  -> verifier accepts a weaker invariant than the issue requires
```

This is not Python-specific. It appears in collection shape bugs, CLI parity
bugs, API compatibility bugs, serializer/deserializer parity bugs, and
multi-language container/default-boundary fixes. The previous actual-diff
line-shape work is also not Python-only: it now covers Python, JS/TS, Ruby,
Java/Kotlin, and Go mapping/container boundary shapes as soft semantic coverage
events.

RC-9 adds a typed comparator lane to `WriteBehaviorContract`:

- `comparator.subject`: grounded working or contrasting reference surface;
- `comparator.operator`: typed observable operator;
- `comparator.expected`: comparator observable value/behavior;
- `comparator.relation`: `same_as | consistent_with | contrasts_with |
  regression_baseline`;
- `comparator.evidence_ref`: issue/log/file evidence for the comparator.

Rules:

- The model may emit comparator facts through `emit_write_analysis`, but
  controller/planner/verifier consume only the typed JSON fields.
- Hard routing still does not parse user intent keywords, model rationale,
  `<think>`, summaries, or issue prose.
- Comparator context is persistent handoff: it is rendered into
  `WriteContextPack` so planner and verifier see the same P0/P1 contract view.
- Planner prompt guidance is soft: when a referenced contract carries
  comparator context, probes should exercise both the changed subject and the
  comparator relationship instead of only checking no-crash.

Expected effect:

- For comparator-framed issues, a locally passing probe is more likely to prove
  the actual required relationship.
- Missing comparator coverage remains visible through existing
  `contract_refs`/verification-confidence telemetry instead of forcing broad
  suites or silently counting the patch as correct.
- This is a schema and handoff upgrade, not a one-off SWE-bench rule.

## Progress Ledger

| Batch | Status | Notes |
| --- | --- | --- |
| RC-0 | complete | Reviewed `cc_like.md`, current write-mode typed harness, impact/convention/patch-review code, and 137-instance manual audit. Root cause classified as root-cause localization plus impact/verify closure, not patch export. |
| RC-1 | complete | Implemented patch-review local acceptance boundary in the SWE-bench adapter. `results.jsonl` now exports `plan_patch_review_*` typed fields, and patch-review hard errors plus unverified semantic coverage block local acceptance telemetry while official prediction export remains unchanged. Verification: adapter unit tests and Python compile pass. |
| RC-2 | complete | Verified existing runtime support: `WriteContextPackFromChangePlan` projects patch-review findings into P2 handoff, and `normalizeControllerTypedStateDecision` appends one bounded semantic-review batch when a completed unverified batch has uncovered semantic patch-review findings. Focused Go tests cover follow-up creation and no-recursion behavior. |
| RC-3 | complete | Added typed `PatchReviewCoverageSummary` to `PatchReviewRecord`; controller semantic follow-up and SWE-bench adapter consume the normalized summary instead of re-scanning ad hoc findings. Adapter now exports `plan_patch_review_coverage_verdict`. Verification: focused Go tests, adapter unit tests, Python compile, and diff check pass. |
| RC-4 | complete | Added impact-aware verifier target selection for default `run_tests({})`: typed `ImpactAnalysis.VerificationTargets` / `ImpactObligations` with `kind=test_surface` and safe existing `related_path` now become priority runner plans for supported selector-capable runners before the generic TestSurface queue. Executed command provenance records `impact_test_surface`; explicit model runner/suite choices are unchanged. Verification: `internal/tool` full package, controller/writeflow/types focused tests, adapter unit tests, and diff check pass. |
| RC-5 | complete | Fixed SWE-bench smoke provider forwarding so isolated Codrax binaries can receive `providers.yaml` via `PROVIDERS_PATH`, `CODRAX_PROVIDERS`, or `--providers`. A three-instance Lite smoke produced non-empty harness-consumable predictions; manual audit found 1 strict pass, 2 incomplete patches. Next batches should target obligation closure and targeted proof generation, not broader patch export. |
| RC-6 | complete | Added actual-diff line text capture to `PatchEffectHunk` and a Python source-shape event for newly added nested string-key direct mapping access. Patch review consumes it as a soft semantic coverage finding with `coverage_status=unknown`, so dynamic-language absent-key/default boundaries reach P2 handoff and bounded semantic follow-up without becoming a hard gate. Verification: focused writeflow/types/orchestrator tests and full `go test ./...` pass. |
| RC-7 | complete | Generalized RC-6 from a Python-only producer into language-aware actual-diff line-shape producers. JS/TS nested string-key access, Ruby nested hash-key access, Java/Kotlin chained string-key map `get`, and Go nested map assignment now emit soft semantic coverage events from typed diff text. These are advisory/coverage obligations, not hard gates, and do not read user intent or model prose. Verification: related writeflow/types/orchestrator tests and full `go test ./...` pass. |
| RC-8 | complete | Verifier selector tightening: a passed scoped pre-suite verification probe no longer escalates to a full project suite solely because required `contract_refs` are missing. The missing refs remain typed `VerificationConfidence` downgrade/handoff evidence; expensive suite escalation is reserved for failed probes, touched tests/specs, missing changed-symbol coupling, or explicit policy. Verification: focused `internal/tool` tests, related tool/types/orchestrator tests, and full `go test ./...` pass. |
| RC-9 | complete | Comparator invariant closure: `WriteBehaviorContract` now carries an optional typed comparator baseline; `emit_write_analysis` validates comparator operator/relation enums; `WriteContextPack` renders comparator fields for controller/planner/verifier consumers; analyzer/planner prompts provide soft guidance to fill and verify comparator contracts without hard-routing on prose. Verification: focused `internal/types` + `internal/tool`, related `internal/skill`, and full `go test ./...` pass. |

## Acceptance Criteria

- SWE local acceptance no longer counts a patch as pass when typed
  `PatchReviewRecord` says the actual diff has a hard error or unverified
  semantic coverage.
- Official prediction export remains unchanged and harness-consumable.
- Runtime hard gates continue to read typed artifacts only.
- No prompt red-line changes: no keyword routing over user intent or model
  prose.
- Comparator-framed bug reports can carry a grounded working/contrasting
  reference through write analysis, context handoff, plan probes, and verifier
  confidence without relying on natural-language re-interpretation.
- Current read/log/trace/data/operation/computer paths remain untouched.
- All new eval fields and docs distinguish export compatibility from
  functional correctness.
