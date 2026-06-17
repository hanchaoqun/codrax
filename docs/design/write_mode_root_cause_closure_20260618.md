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
   - Keep expanding actual-diff line feature events through a language-provider
     registry. Current mapping/container boundary coverage includes Python,
     JS/TS, Ruby, Java/Kotlin, and Go; further languages should add typed
     providers, not ad hoc branch logic.
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

## 2026-06-18 RC-9 SWE-bench SymPy Regression

Run directory:
`/private/tmp/codrax-swebench-rc9-sympy-20260618-022926`

Result:

- `predictions.jsonl` was non-empty and official-harness dry-run consumable.
- The workflow completed after multiple online replan/verify rounds.
- Local typed verify passed, but local acceptance stayed blocked by
  `patch_review_semantic_uncovered:behavior_contract_without_verify_coverage`.
- Manual audit still failed: the final patch accepted `Array([]).shape == ()`
  and did not recover the reference `(0,)` shape semantics.

New root cause:

- RC-9 gave the system a typed comparator lane, but the analyzer did not use it.
- Worse, the analyzer converted an ungrounded exact value into P0 behavior:
  `Array([]).shape expected=()`, even though `()` was not present in the issue
  evidence and no comparator was attached.
- Planner then faithfully generated a probe for the wrong P0 contract. This is
  not a planner/probe execution bug; it is a write-analysis quality gate gap.

## 2026-06-18 RC-10 Exact Contract Grounding Gate

Design:

- Add a typed `WriteAnalysisIR` quality gate after `emit_write_analysis`
  succeeds and before the IR reaches controller/planner.
- For required expected behavior contracts with exact operators
  `equals | not_equals | returns`, accept the contract only when:
  - the exact `expected` value appears verbatim in `raw_request`; or
  - the contract carries a typed comparator whose exact expected value is
    grounded by a non-empty `evidence_ref` or by `comparator.expected`
    appearing verbatim in `raw_request`.
- If the gate rejects, clear the IR and reuse the existing
  `AnalyzerRetryHint` retry surface so the write analyzer re-emits corrected
  structured JSON.
- If the analyzer fails the bounded retry, install the conservative fallback IR
  instead of leaking an ungrounded exact P0 contract downstream.

Why this is generalized:

- It does not parse user intent keywords or model prose.
- It does not know anything about SymPy, Matrix, Array, or shapes.
- It enforces a schema-level principle: exact values are hard planning signals
  only when backed by exact evidence or a typed comparator.
- A comparator object by itself is not enough. A comparator subject that appears
  in the request proves only that the comparison surface exists; it does not
  prove the exact expected value. This prevents the analyzer from satisfying the
  schema by inventing values behind a subject-only comparator.

Expected effect:

- Wrong exact expectations like `shape == ()` stop becoming verifier targets
  just because the model guessed them.
- Comparator-framed bugs get a second chance to emit a comparator contract
  before planning starts.
- If the analyzer still cannot ground the exact value, downstream planning uses
  the original request/fallback context rather than a false P0 invariant.

Pre-tightening SWE-bench regression:

- Run directory:
  `/private/tmp/codrax-swebench-rc10-sympy-20260618-024245`.
- The workflow produced a non-empty, official-harness-consumable prediction and
  no longer silently finished with a passing weak invariant.
- It still exposed a second analyzer-quality gap: the model emitted a
  comparator object whose subject was not grounded in the request or evidence,
  then the follow-up patch failed verification with a syntax/duplicate-function
  edit. RC-10 therefore tightened comparator acceptance to require
  `evidence_ref` or verbatim raw-request grounding.

Post-tightening SWE-bench regression:

- Run directory:
  `/private/tmp/codrax-swebench-rc10b-sympy-20260618-025637`.
- The workflow produced a non-empty, official-harness-consumable prediction but
  ended `workflow_status=blocked` with
  `workflow_latest_progress=write_controller/progress/verify_retry_budget_exhausted`.
- This is the correct product-side failure mode for this batch: it did not
  claim local correctness after repeated verification failures. The final
  verify evidence was a typed Python probe exception at `ndim_array.py:485`
  (`IndexError` during empty-array iteration).
- The run exposed one more precise rule: subject-only comparator grounding is
  insufficient. The plan carried `comparator.subject=Matrix([])`, which appears
  in the request, but still invented `shape = (), flat_list = []` as the exact
  expected value. RC-10 now rejects this shape unless the value itself appears
  in `raw_request`, appears as grounded `comparator.expected`, or has an
  `evidence_ref`.

Final RC-10c SWE-bench regression:

- Run directory:
  `/private/tmp/codrax-swebench-rc10c-sympy-20260618-030808`.
- The workflow produced a non-empty, official-harness-consumable prediction and
  local acceptance correctly failed:
  `prediction_verdict=predicted_failed_verify`,
  `prediction_audit_block_reason=patch_review_semantic_uncovered:behavior_contract_without_verify_coverage`.
- The final patch changed the empty iterable return to `(0,)`, closer to the
  upstream expectation, but project verification still failed
  (`pytest exitcode=1`, 9 passed / 10 failed), so the system did not claim a
  functional pass.
- New remaining gap for the next batch: exact-looking values can still appear
  inside `operator=satisfies` natural-language expected text. This must not be
  fixed by scanning prose for shape/tuple keywords. The generalized fix is a
  schema/consumer rule: only exact typed fields/operators can become hard
  verifier targets; `satisfies` expected text is soft guidance unless the
  analyzer emits a separate typed exact-value artifact with evidence.

## 2026-06-18 RC-11 Hard/Soft Contract Separation

Design:

- Preserve the wide `RequiredWriteBehaviorContractIDs` view for audit and
  backward-compatible inspection.
- Add `HardRequiredWriteBehaviorContractIDs` and
  `IsHardRequiredWriteBehaviorContract` as the verifier/control view.
- A contract is hard-required only when it is:
  - `required=true`;
  - not `polarity=observed`;
  - not `source=expected_outcome_fallback`;
  - and its operator is a typed exact/verifiable operator
    (`equals`, `not_equals`, `contains`, `not_contains`, `exists`,
    `not_exists`, `raises`, `not_raises`, `returns`).
- `operator=satisfies` remains visible and useful, but is soft guidance. Its
  natural-language `expected` text cannot create a hard missing-contract gate.
- `WriteContextPack` promotes only hard-required contracts to P0. Soft required
  contracts remain in planner/verifier handoff with `soft_required=true`.
- Planner/controller prompt rendering now emits `hard_required=true` or
  `soft_required=true` instead of the ambiguous `required=true`.
- `run_tests` verification confidence now computes missing/covered contract
  refs from the hard-required view only. A probe can still cite a soft contract,
  but that citation does not satisfy or fail a hard coverage gate.

Why this is generalized:

- It does not inspect or regex-match model-generated expected prose.
- It does not special-case SymPy, shapes, tuples, Python, or SWE-bench.
- It pushes the hard/soft distinction into typed contract semantics and
  deterministic consumers.

Expected effect:

- Models can still carry broad behavior text for planning, but local confidence
  and controller handoff no longer treat that prose as an exact oracle.
- When exact values matter, the analyzer must use a typed exact operator and
  pass RC-10 grounding; otherwise the behavior remains soft until verified by
  project tests or explicit probes.

RC-11 SWE-bench smoke:

- Run directory:
  `/private/tmp/codrax-swebench-rc11-sympy-20260618-031804`.
- Prediction export remained non-empty and official-harness consumable.
- Local acceptance correctly failed:
  `prediction_verdict=predicted_failed_verify`,
  `prediction_confidence_downgrade_reason=local_verification_failed`.
- The workflow did not claim correctness; verifier evidence reported
  `AssertionError: Expected shape=(), got (0,)`.
- The run exposed a narrower typed-quality gap: a hard operator such as
  `not_raises` can still carry a compound natural-language expected payload.
  That is not a `satisfies` issue; it means RC-10 grounding should apply to all
  hard operators, not only `equals | not_equals | returns`.

## 2026-06-18 RC-12 All Hard Operators Grounding

Design:

- Extend the post-`emit_write_analysis` quality gate from
  `equals | not_equals | returns` to every hard operator:
  `equals`, `not_equals`, `contains`, `not_contains`, `exists`, `not_exists`,
  `raises`, `not_raises`, `returns`.
- Keep the same evidence rule: a non-empty exact `expected` payload must be
  verbatim in `raw_request` or grounded by comparator evidence.
- Leave `operator=satisfies` soft and out of the hard grounding gate.

Why this is generalized:

- It rejects typed-field misuse, not issue prose.
- It does not inspect shape syntax, exception names, Python stack traces, or
  SWE-bench metadata.
- It forces hard operators to carry precise, grounded operands, while broad
  behavior descriptions stay in soft `satisfies`.

Expected effect:

- `not_raises expected=ValueError` is accepted when `ValueError` appears in the
  request/log.
- `not_raises expected="does not raise and returns shape=()"` is rejected unless
  that exact expected payload is grounded, forcing the analyzer to split it into
  a grounded exception contract plus a soft or separately grounded value
  contract.

RC-12 SWE-bench smoke:

- Run directory:
  `/private/tmp/codrax-swebench-rc12-sympy-20260618-033120`.
- Prediction export remained non-empty and official-harness consumable:
  `patch_bytes=974`, `validated 1 prediction(s); empty_patch=0`.
- Local acceptance correctly failed:
  `local_acceptance_verdict=fail`,
  `prediction_verdict=predicted_failed_verify`,
  `prediction_confidence_downgrade_reason=verification_probe_exception`,
  `plan_patch_review_coverage_verdict=unverified`.
- Workflow status remained non-complete after failed verification:
  `workflow_status=in_progress`,
  `workflow_latest_progress=write_controller/progress/plan_batch_interrupted_after_applied_patch`.
- Verifier evidence stayed typed and concrete:
  `IndexError: list index out of range` at `sympy/tensor/array/ndim_array.py`.
- Conclusion: RC-12 did not make this SymPy patch functionally correct, but it
  preserved the commercial acceptance boundary: exported patches stay harness
  compatible while failed local typed verification prevents false pass claims.

## 2026-06-18 RC-13 Multi-language Verification Probe Runtime

Gap:

- Actual-diff mapping/container boundary signals are already language-aware
  (Python, JS/TS, Ruby, Java/Kotlin, Go), but the bounded
  `verification_probes[]` executor still accepts only `language=python`.
- This creates a proof asymmetry: non-Python write tasks can use project
  runners, but cannot easily attach a tiny local behavior probe when the
  project suite is missing, slow, flaky, or too broad.
- The gap affects the Claude-Code-style online loop because `Edit -> Run ->
  Observe -> Repair` needs a cheap scoped observation channel across common
  languages, not only Python.

Design:

- Keep `verification_probes[]` as the single typed small-proof lane. Do not add
  ad hoc shell-command probes or parse natural-language `acceptance_tests`.
- Add a deterministic runtime registry with canonical languages:
  `python`, `javascript`, `ruby`, and `go`.
- Continue to accept only bounded inline source, repo-relative `working_dir`,
  short timeout, optional `expected_stdout`, `contract_refs`, and
  `changed_symbol_refs`.
- Runtime behavior:
  - Python keeps the existing structured wrapper and project-root cwd promotion.
  - JavaScript runs through `node` with base64 source injection.
  - Ruby runs through `ruby` with base64 source injection.
  - Go writes the probe to `.codrax/tmp/verification-probes` and runs
    `go run` from the safe resolved working directory, then deletes the temp
    file. Keeping the file under the repo runtime directory preserves Go
    `internal/` import rules without writing source-tree probe files.
- Failure-signal validation remains structural code validation, not issue
  keyword routing:
  - no `expected_stdout` requires a language-appropriate non-zero failure
    surface (`assert`/`raise`, `throw`/`process.exit`, `raise`/`fail`,
    `panic`/`os.Exit`, etc.);
  - these checks scan probe source bytes only and never inspect user intent,
    model rationale, summaries, or `<think>`.
- Missing runtime binaries map to typed `runner_missing`; syntax/import/runtime
  errors remain typed probe outcomes and flow through existing verifier
  confidence, context pack, and controller replan logic.

Why this is generalized:

- It extends an existing typed abstraction instead of creating per-language
  prompt branches.
- The registry gives future Java/Kotlin/Rust/Swift/ArkTS/Cangjie providers a
  single place to plug in once a bounded inline-probe story is safe.
- It preserves current read/log/trace/data/operation paths and all write hard
  gates: control still consumes typed enums, paths, exit codes, timeouts,
  stdout fragments, and refs.

Task list:

- [x] Update plan schemas and planner guidance to advertise supported probe
  languages.
- [x] Replace Python-only normalization with a canonical
  supported-language registry and aliases.
- [x] Add JavaScript, Ruby, and Go probe executors with bounded timeout,
  expected-stdout handling, typed command evidence, and no source-tree probe
  files.
- [x] Add unit tests for language normalization, failure-signal validation,
  successful probes, failing probes, and missing runtime behavior where
  practical.
- [x] Update `docs/user_guide.md` and `docs/user_guide.html`.
- [x] Run focused tests, full `go test ./...`, and one SWE-bench smoke to
  ensure the Python path did not regress.

RC-13 SWE-bench smoke:

- Run directory:
  `/private/tmp/codrax-swebench-rc13-sympy-20260618-035454`.
- Prediction export remained non-empty and official-harness consumable:
  `patch_bytes=988`, `validated 1 prediction(s); empty_patch=0`.
- Python verification-probe path remained operational:
  `plan_verification_probe_count=1`, `verify_status=passed`.
- Local acceptance correctly stayed blocked because proof coverage was still
  incomplete:
  `local_acceptance_verdict=fail`,
  `prediction_verdict=predicted_audit_blocked`,
  `prediction_audit_block_reason=patch_review_semantic_uncovered:changed_symbol_without_probe_coverage`,
  `prediction_confidence_downgrade_reason=verification_probe_missing_required_contract_ref`,
  `plan_patch_review_coverage_verdict=unverified`.
- Conclusion: RC-13 did not relax correctness claims. It widened the bounded
  local-proof lane while preserving patch-review and confidence downgrades.

## 2026-06-18 RC-14 Impact Obligation Repair Queue

Gap:

- `ImpactAnalysisResult`, `ImpactObligationSet`, and actual-diff
  `PatchReviewRecord` are already typed and persisted, but controller
  scheduling still treats uncovered impact mostly as telemetry unless local
  verification is unavailable.
- A locally passing verifier can still leave typed coverage gaps such as
  `changed_symbol_without_probe_coverage` or
  `behavior_contract_without_verify_coverage`. That is exactly the
  SWE-bench failure mode where a patch proves "something passed" while the
  owner boundary or dependent surface remains unproved.
- The previous semantic follow-up path appended one broad review batch only
  after an unverified verifier outcome. It did not make impact obligations a
  deterministic repair queue and did not trigger after a passed but
  under-covered local verifier result.

Design:

- Add a deterministic `Impact Obligation Repair Queue` in the controller
  normalization layer.
- The queue reads only typed artifacts:
  - `ImpactAnalysisResult.VerificationTargets`;
  - `PatchReviewRecord.Findings`;
  - typed `coverage_status`, `kind`, `path`, `related_path`,
    `subject_symbol`, `contract_ref`, and `evidence_ref`.
- When the active batch has a terminal non-failed verify attempt
  (`passed` or typed `unverified`) and uncovered semantic/impact coverage
  remains, the controller overrides terminal/interruption actions into one
  bounded repair step:
  - `append_batch` when the uncovered obligations carry concrete paths;
  - `explore_code` first when the obligation has no path and needs bounded
    localization before planning.
- The repair batch carries:
  - deterministic ID `<active-batch>-impact-repair`;
  - expected/explore paths from typed obligation paths;
  - success criteria derived from obligation IDs/codes;
  - typed exploration requirements when the selected obligations lack a
    concrete path.
- Existing semantic-review follow-up is subsumed by this queue. The old reason
  remains recognized as already-requested state for durable-run compatibility,
  but no public legacy engine surface is reintroduced.
- Hard routing still does not parse user intent, model rationale,
  `<think>`, issue text, stdout prose, or manual audit notes. The model can
  consume the appended batch goal and context pack as soft planning guidance;
  the decision to append is deterministic.

Task list:

- [x] Add impact repair item selection and stable priority ordering.
- [x] Route finish/replan/interruption decisions to `append_batch` or
  `explore_code` when typed uncovered impact remains after a non-failed
  verifier attempt.
- [x] Preserve one-shot behavior to avoid recursive repair loops while keeping
  evidence visible in `WriteContextPack`.
- [x] Cover passed-verifier-but-undercovered and unverified-undercovered
  regressions with controller tests.
- [x] Run focused orchestrator tests, full `go test ./...`, `make test`, then
  refresh this ledger and push.

Verification:

- `go test ./internal/orchestrator -run 'TestNormalizeControllerTypedStateDecision(SemanticPatchReview|VerifiedButUndercovered|ImpactRepair|LegacySemantic|RunnerMissing|ReplanAfter)'`
- `go test ./internal/orchestrator ./internal/writeflow ./internal/types`
- `go test ./internal/tool ./internal/agent`
- `go test ./...`
- `make test`

## RC-14 SWE Smoke Follow-Up: Coverage Projection Authority

After RC-14, a three-instance SWE-bench Lite smoke produced harness-consumable
non-empty predictions for:

- `django__django-14534`
- `pytest-dev__pytest-11143`
- `sympy__sympy-23117`

Artifacts:

- Run directory:
  `/private/tmp/codrax-swebench-rc14-20260618-041330`
- Predictions:
  `/private/tmp/codrax-swebench-rc14-20260618-041330/predictions.jsonl`
- Results:
  `/private/tmp/codrax-swebench-rc14-20260618-041330/results.jsonl`

Audit summary:

- Export compatibility: 3/3 predictions validated by the official SWE-bench
  harness dry-run path and none were empty.
- Strict commercial manual pass: 0/3.
- `django__django-14534`: local verifier passed, but the actual patch changed
  `id_for_label` to direct-index `self.data["attrs"]["id"]`. Patch review
  later reported
  `patch_review_semantic_uncovered:python_nested_string_key_direct_access_added`
  and `verification_probe_missing_required_contract_ref`, but the runtime
  workflow had already allowed controller `finish`.
- `pytest-dev__pytest-11143`: RC-14 correctly appended an impact-repair batch,
  but the repair polluted production source with test scaffolding. The core
  null-guard fix was plausible; commercial quality remained blocked.
- `sympy__sympy-23117`: RC-14 correctly avoided a false pass and kept replanning
  after failed verification, but the repair loop produced structurally poor
  edits such as a duplicate method and did not converge before wall time.

Root-cause classification:

- Not a patch-export problem. The official prediction path is consumable.
- Not simply a missing planner prompt. The controller consumed typed artifacts,
  but the verify coverage projector over-promoted actual-diff semantic findings
  from `unknown` / `unverified` to `verified` after an unrelated local pass.
- This is a system-level authority gap: verification projection must only
  certify the target it has typed evidence for. A narrow probe or unrelated
  suite pass cannot certify every actual-diff boundary event.

RC-15 design:

- Keep `ObservationAuthorityFailed` and `ObservationAuthorityUnverified`
  able to downgrade all coverage to failed/unavailable states.
- For `ObservationAuthorityVerified`, promote only findings whose typed
  evidence is actually covered:
  - changed-symbol coverage reads `VerificationConfidence` symbol refs;
  - behavior-contract coverage reads `VerificationConfidence` contract refs;
  - dependent/test-surface coverage requires a matching typed
    `TestResult.Suite`;
  - actual-diff unknown coverage events stay uncovered until an explicit typed
    verifier signal exists.
- Use the existing PatchReview language event registry as the source of truth;
  do not duplicate Python/JS/TS/Ruby/JVM/Go event strings in controller logic.
- Keep the rule typed-only: no user intent keywords, no issue prose, no model
  rationale, no `<think>`, no stdout summary parsing.

Task list:

- [x] Export a `writeflow` helper for actual-diff unknown coverage event codes.
- [x] Add typed path coverage to `verifyCoverageConfidence` from passing
  `ChangeReport.TestResult.Suite` values.
- [x] Preserve multi-language actual-diff unknown coverage findings after
  unrelated local pass.
- [x] Verify scoped test-surface findings only when the matching suite passed.
- [x] Run focused controller/writeflow tests, full regression, update progress,
  commit, and push.

Verification:

- `go test ./internal/orchestrator -run 'TestApplyVerifyCoverageToChangePlan|TestSyncMutablePlanStatusAfterVerifyMarksCoverage'`
- `go test ./internal/writeflow -run 'TestAnnotatePatchEffect|TestReviewAppliedPatch'`
- `go test ./internal/orchestrator ./internal/writeflow ./internal/types`
- `go test ./internal/tool ./internal/agent`
- `go test ./...`
- `make test`

## RC-16: Structural Patch Quality Events

RC-14/RC-15 smoke also exposed two patch-quality gaps that are not solved by
better planning prose:

- A repair batch can add test scaffolding into a production source path.
- A repair batch can add a duplicate declaration that silently shadows an
  existing declaration before the verifier catches the downstream symptom.

These are system-level actual-diff defects. They should enter the same typed
PatchEffect -> PatchReview -> ImpactAnalysis -> WriteContextPack -> bounded
repair flow as parser errors and semantic coverage events.

Design:

- Reuse `PatchEffectRecord` and `SourcePathRole`.
- Produce structural events from the applied diff and post-apply file bytes:
  - `python_duplicate_symbol_added` is a hard structural event for an added
    Python declaration that duplicates another declaration in the same owner
    scope.
  - `production_test_scaffold_added` is a soft semantic coverage event when an
    added test-shaped declaration appears in a production path. It is not a
    global hard block because customers may intentionally edit test helpers,
    but it must be visible to controller/planner/verifier as uncovered quality
    debt.
- Keep the detector structural:
  - path role comes from `ClassifySourcePathRole`;
  - declaration/test-shape comes from language-specific source-shape providers;
  - duplicate Python ownership comes from indentation/declaration structure;
  - no user intent keywords, issue prose, model rationale, stdout summary, or
    `<think>` are parsed for routing.
- Keep room for language expansion without duplicating the policy layer:
  `production_test_scaffold_added` is language-neutral, while duplicate symbol
  detection starts with Python because that is where silent shadowing is common
  and Go/JVM/TS are normally caught by compiler/parser lanes.

Task list:

- [x] Add RC-16 design and task ledger.
- [x] Add `python_duplicate_symbol_added` actual-diff structural event and
  hard PatchReview consumption.
- [x] Add language-provider `production_test_scaffold_added` event for
  production paths and soft unknown PatchReview consumption.
- [x] Add focused PatchEffect/PatchReview tests.
- [x] Run focused writeflow tests, related write packages, full regression,
  update progress, commit, and push.

Verification:

- `go test ./internal/writeflow -run 'TestAnnotatePatchEffect(PythonDuplicateSymbol|ProductionTestScaffold|PythonTopLevelSelf|PythonNested|MultiLanguage)'`
- `go test ./internal/writeflow`
- `go test ./internal/orchestrator ./internal/writeflow ./internal/types`
- `go test ./internal/tool ./internal/agent`
- `go test ./...`
- `make test`

## RC-17: Verification Confidence Authority

The RC-16 SWE smoke showed that a passing verification probe can still leave
final plan coverage unverified when `ChangeReport.verification_confidence` is
missing. In `django__django-14534`, the final probe explicitly carried
`contract_refs=["outcome-1", "outcome-2", "outcome-3", "outcome-4"]` and
`changed_symbol_refs`, yet the report persisted no confidence records, so
coverage projection could not mark those specific obligations covered.

Root cause:

- `run_tests.finishReport` derives confidence from
  `ctx.Mutable.ChangePlan()`. Some verify paths have the probe report but not a
  reliable mutable plan snapshot at the final install point.
- `reportPassedOnlyByVerificationProbes` still recognized only
  `verification_probe/python`, which undercuts the RC-13 multi-language probe
  runtime.

Design:

- Attach verification confidence directly when `runPlanVerificationProbes`
  creates the probe-only report, using the same plan snapshot that supplied the
  probes.
- Keep `finishReport` as an idempotent enrichment layer; merged confidence
  records are deduplicated.
- Generalize probe-only detection to any typed `verification_probe/<language>`
  suite.
- Keep the authority typed-only: confidence comes from `ChangePlan`
  `behavior_contracts`, `verification_probes[].contract_refs`,
  `verification_probes[].changed_symbol_refs`, and `ChangeReport` test/command
  fields.

Task list:

- [x] Record RC-17 smoke finding and root cause.
- [x] Attach confidence to probe reports at creation time.
- [x] Generalize probe-only confidence to non-Python probe suites.
- [x] Add focused confidence regression tests.
- [x] Run focused tool tests, related packages, full regression, update
  progress, commit, and push.

Verification:

- `go test ./internal/tool -run 'Test(VerificationConfidenceRecordsFromProbeReport|RunPlanVerificationProbesAttachesConfidence|RunTestsVerificationProbePass)'`
- `go test ./internal/tool ./internal/orchestrator ./internal/types ./internal/agent`
- `go test ./internal/writeflow ./internal/writeflow/impact`
- `go test ./...`
- `make test`

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
| RC-10 | complete | Exact contract grounding gate: post-emit write-analysis quality check rejects required exact behavior contracts whose expected value is neither verbatim in `raw_request` nor backed by grounded comparator evidence, then retries through the existing AnalyzerRetryHint surface. Subject-only comparators no longer ground exact values. Verification: focused orchestrator tests, related orchestrator/skill/types/tool tests, full `go test ./...`, `make test`, and SymPy SWE-bench smoke export checks pass; RC-10c still fails local functional verification, correctly reported as failed/unverified rather than a pass. |
| RC-11 | complete | Hard/soft contract separation: added hard-required behavior-contract helpers; context handoff promotes only hard contracts to P0; planner/controller rendering distinguishes `hard_required=true` from `soft_required=true`; verifier contract-ref confidence uses only hard-required contracts. Verification: focused `internal/types`, `internal/tool`, and `internal/agent`; full `go test ./...`; `make test`; RC-11 SymPy smoke export passed and local correctness correctly failed. |
| RC-12 | complete | All hard operators grounding: write-analysis quality gate now applies grounding to `contains`, `not_contains`, `exists`, `not_exists`, `raises`, and `not_raises` in addition to equals/not_equals/returns. Verification: focused orchestrator tests, related orchestrator/skill/types/tool/agent tests, full `go test ./...`, `make test`, and RC-12 SymPy SWE-bench smoke export passed; local correctness correctly failed with typed verifier evidence instead of a false pass. |
| RC-13 | complete | Multi-language verification probe runtime: generalized the Python-only bounded probe lane to a typed provider registry for Python, JavaScript/Node, Ruby, and Go; updated schemas, planner guidance, user docs, and tests. Verification: focused multi-language probe tests, full `internal/tool`, related types/skill/orchestrator/agent tests, full `go test ./...`, `make`, and RC-13 SymPy SWE-bench smoke passed export compatibility while local acceptance remained correctly audit-blocked. |
| RC-14 | complete | Impact obligation repair queue: controller now schedules one bounded impact-repair follow-up when typed impact/patch-review coverage remains uncovered after a non-failed verifier attempt, including the previously missed "local verify passed but changed-symbol/contract coverage is still unverified" case. Path-backed obligations append a scoped repair batch; pathless obligations trigger bounded exploration first. Verification: focused controller regressions, related write packages, full `go test ./...`, and `make test` pass. |
| RC-15 | complete | Coverage projection authority: post-RC-14 SWE smoke showed actual-diff unknown coverage findings could be over-promoted to verified after unrelated local pass. The fix keeps verified projection target-specific: changed symbols and contracts use confidence refs, scoped surfaces use matching passed suites, and multi-language actual-diff unknown events remain uncovered until explicitly proven. Verification: focused controller/writeflow regressions, related packages, full `go test ./...`, and `make test` pass. |
| RC-16 | complete | Structural patch quality events: added `python_duplicate_symbol_added` as a hard actual-diff PatchReview event, and `production_test_scaffold_added` as a language-provider soft semantic coverage event for production paths. These events are generated from applied diff lines, post-apply file bytes, and `SourcePathRole`; no user/model prose drives routing. Verification: focused writeflow tests, related packages, full `go test ./...`, and `make test` pass. |
| RC-17 | complete | Verification confidence authority: probe-only reports now attach contract/symbol confidence from the same plan snapshot that supplied the probes, and probe-only confidence recognizes all `verification_probe/<language>` suites instead of Python only. Verification: focused confidence tests, related packages, full `go test ./...`, and `make test` pass. |

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
- Ungrounded exact expected values cannot become P0 write behavior contracts;
  they must be verbatim-evidenced, comparator-grounded, softened, or removed on
  analyzer retry.
- Natural-language `satisfies` expected text stays soft; hard exact verifier
  targets require typed exact fields/operators with evidence.
- `verification_probes[]` provide bounded local behavior checks for common
  non-Python projects without requiring project-wide test runners or shell
  command probes.
- Uncovered typed impact obligations become a bounded repair queue before
  finish, instead of remaining passive telemetry after local verifier success.
- Current read/log/trace/data/operation/computer paths remain untouched.
- All new eval fields and docs distinguish export compatibility from
  functional correctness.
