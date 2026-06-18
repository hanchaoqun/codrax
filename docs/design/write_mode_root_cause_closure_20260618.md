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

## 2026-06-18 RC-29 Source-Shape Provider Registry Design

User follow-up clarified that "actual-diff Python dynamic mapping boundary" can
sound Python-only. The current runtime already emits multi-language
actual-diff boundary events for Python, JS/TS, Ruby, Java/Kotlin, and Go, while
the SWE-bench owner-boundary AST audit remains Python-specific because
SWE-bench Lite is Python-centric. The remaining architecture gap is not coverage
for one language; it is the producer shape:

```text
actual diff lines + post-apply file bytes
  -> source-kind switch branches
  -> typed PatchEffectEvent rows
  -> PatchReview / ImpactAnalysis / P2 handoff
```

That shape works today, but every new language would add another branch in the
central function. RC-29 upgrades this to a deterministic provider registry:

- each provider declares extensions, line-shape rules, production-test scaffold
  rules, and optional file/hunk annotators;
- provider dispatch is by repo-relative path extension and post-apply file
  bytes, never by user request text, issue keywords, model rationale, or
  `<think>` output;
- provider output remains the existing typed `PatchEffectEvent` record consumed
  by PatchReview and controller follow-up logic;
- hard/soft severity stays in the existing event-code registry, so this refactor
  does not create a new hard gate or broaden approval prompts;
- future Rust/Swift/ArkTS/Cangjie providers can be added as registry entries
  with focused tests rather than editing source-shape control flow.

RC-29 tasks:

- [x] Add `patchEffectSourceProvider` with extension dispatch, line rules,
  production-test scaffold rule, and optional file/hunk annotators.
- [x] Move Python duplicate declaration and top-level owner-shape checks behind
  provider hooks without changing event codes or severity.
- [x] Move mapping/container and production scaffold rules into provider data.
- [x] Add registry coverage tests for all current providers and preserve
  existing multi-language event tests.
- [x] Run focused writeflow tests, related orchestrator/tool smoke where needed,
  and update the progress ledger before commit/push.

## 2026-06-18 RC-30 Official SWE-bench Result Summary Design

The 137-instance audit made one metric boundary explicit: non-empty prediction
export is compatibility, not correctness. Local authoritative verification and
typed manual audit are useful acceptance proxies, but the only official
SWE-bench pass rate is the harness `resolved` verdict. Current Codrax tooling can
call the official harness, but it does not normalize the harness JSON back into
a durable Codrax report. That forces humans to infer pass rate from terminal
output or to over-reuse local acceptance fields.

RC-30 adds a dependency-free summarizer that reads official harness JSON
artifacts only:

```text
official report.<run_id>.json or per-instance report.json
  -> normalized official_summary.json
  -> resolved/submitted, resolved/completed, resolved/total
```

Design constraints:

- do not import SWE-bench internals; installed harness versions can require a
  different Python runtime than Codrax's eval adapter;
- do not parse terminal prose or log narrative;
- do not merge local verifier/manual audit into official resolved metrics;
- keep subset-run and full-run denominators separate so a 3-instance smoke is
  not mislabeled as full benchmark accuracy;
- keep prediction export validation and official scoring as separate tools.

RC-30 tasks:

- [x] Add `eval/swebench/summarize_official_results.py` for run report and
  per-instance report aggregation.
- [x] Add unit tests covering run-report denominators, per-instance fallback,
  empty-patch/error accounting, and CLI JSON output.
- [x] Update `run_official_harness.sh` to pass `REPORT_DIR` through when the
  harness version supports it.
- [x] Update SWE-bench README with the official summary workflow and metric
  naming rules.
- [x] Run adapter/unit/smoke validation and update the progress ledger before
  commit/push.

## 2026-06-18 RC-31 Checkpoint Rewind Before Replan

The Claude Code-style online convergence target is edit -> observe -> repair,
not one large apply followed by late failure. Codrax already records
slice-level checkpoint metadata after apply, but before this batch a failed
verify followed by `replan_batch` could keep planning on the same dirty or
failed worktree state. That is a state-kernel gap: the next model turn may see
the failed side effects rather than the last typed applied checkpoint.

RC-31 makes rewind a deterministic EffectExecutor step before failed-verify
replan:

```text
active failed slice + checkpoint(commit, worktree)
  -> safe worktree boundary check
  -> git reset --hard <checkpoint commit>
  -> slice_restore event + restore attempt
  -> planner replan on a known typed checkpoint
```

Design constraints:

- The restore authority is structural only: active run/batch/slice,
  `WriteWorkflowCheckpoint.CommitSHA`, checkpoint worktree path, current
  `BusContext.WorktreePath`, and `worktreeBase`.
- The controller never reads user issue text, model rationale, stdout prose, or
  `<think>` output to decide whether to restore.
- The path gate allows only the current workflow worktree or a descendant of
  the controller worktree base. External directories are rejected and recorded
  as skipped progress.
- The reset uses the existing `worktree.ResetHard` helper rather than a new git
  runner. Main checkout HEAD and merge state remain untouched.
- Restore failure is fail-soft for automation smoothness: the workflow records a
  typed progress reason and continues into the existing replan path instead of
  prompting the user.
- Language-specific actual-diff signals remain separate from this batch.
  "Python dynamic mapping boundary" was never a Python-only architecture: the
  provider registry currently covers Python, JS/TS, Ruby, Java/Kotlin, and Go.
  Python has deeper duplicate/owner annotator hooks; other languages currently
  carry container/default-boundary and production-test-scaffold events, and can
  add richer AST/compiler providers incrementally.

RC-31 tasks:

- [x] Add durable `slice_restored` event normalization.
- [x] Add a safe checkpoint worktree resolver for current-worktree or
  worktree-base-descendant paths.
- [x] Wire checkpoint restore before `replan_batch` caused by failed verify.
- [x] Preserve bus/mutable repo roots after reset so planner/verifier consume
  the restored worktree.
- [x] Add real git-worktree regression for resetting a regressed worktree to a
  checkpoint commit.
- [x] Add external-directory rejection regression.
- [x] Run focused controller/types tests, related package regression, full
  regression, and update progress before commit/push.

## 2026-06-18 RC-32 Multi-language Verification Probe Coupling

RC-13 generalized bounded `verification_probes[]` runtime support from Python
to Python, JavaScript/Node, Ruby, and Go. The remaining proof-quality gap was
that the hard coupling validator still only checked Python probes. A JS/Ruby/Go
probe could therefore assert a copied local expression and pass without
importing or requiring the changed production module. That weakens local
verification exactly in the customer scenario where full project test runners
are unavailable.

RC-32 turns the coupling check into a deterministic language provider registry:

```text
changed production paths + probe.language + probe code import/require surface
  -> provider targets
  -> provider references
  -> changed-module coupling verdict
```

Design constraints:

- Hard routing consumes only typed plan state and code structure:
  `FileChange.Path`, source/test path role, `VerificationProbe.Language`, and
  import/require literals extracted from the probe code.
- It never reads user intent keywords, issue text, model summary/rationale,
  stdout prose, or `<think>` output.
- The validator is conservative: if there is no target for a provider or no
  probe in that provider language, it does not force a probe. Existing project
  suite verification remains available.
- Python keeps its existing public-package exception by adding repo-local
  public packages as targets.
- JavaScript/TypeScript targets come from changed source paths plus
  `package.json.name` when present; references come from `require(...)`,
  static `import`, and dynamic `import(...)`.
- Ruby targets come from changed `.rb` paths; references come from
  `require`, `require_relative`, and `load`.
- Go targets come from changed package directories plus the module path in
  `go.mod`; references come from Go import declarations.
- Future languages add a provider with `TargetProducer`, `ProbeRefs`, and
  `Covers`, rather than editing scheduler or prompt logic.

RC-32 tasks:

- [x] Replace Python-only probe coupling with `verificationProbeCouplingProvider`
  registry.
- [x] Preserve Python changed-module and public-package behavior.
- [x] Add JavaScript/TypeScript import/require/package-name coupling.
- [x] Add Ruby require/load coupling.
- [x] Add Go module import coupling.
- [x] Add emit-level regressions for JS/Ruby copied probes and accepted coupled
  probes.
- [x] Add Go module coupling regression without requiring a local Go compiler.
- [x] Update user guide Markdown/HTML and progress ledger.
- [x] Run focused `internal/tool`, related package regression, full regression,
  SWE adapter smoke, and diff check before commit/push.

## 2026-06-18 RC-33 Typed Plan Repair Retry Consumption

RC-31/RC-32 strengthened the online state kernel and bounded proof surface, but
one JSON/tool-output repair gap remained: plan emit tools already attach a
typed `PlanRepairPack` in `ToolResult.Repair.Metadata`, while the controller
no-plan retry still mostly rendered the latest rejection `Summary` string back
to the planner. That left the model to mentally extract the important fields
from capped prose or the transparent `PLAN_REPAIR_PACK:` line, and it weakened
handoff for exact current bytes, failing field paths, retained partial plans,
and accepted enum repairs.

RC-33 makes repair-pack consumption deterministic:

```text
failed emit_change_plan / emit_plan_skeleton / emit_plan_change ToolResult
  -> ToolRepair.Code == write_plan_repair_pack
  -> Metadata["plan_repair_pack"] JSON
  -> normalized PlanRepairPack
  -> bounded retry hint rendered from typed fields
```

Design constraints:

- Hard routing still consumes only typed tool-result state: newest plan emit
  success clears older failures; newest failed plan emit can grant the bounded
  no-plan retry window.
- The controller does not parse `Summary`, `PLAN_REPAIR_PACK:` text, model
  rationale, user intent keywords, or `<think>` output to recover the repair
  pack.
- Summary text remains only a fallback soft hint when no typed repair metadata
  exists, preserving compatibility with older tool results.
- `internal/types` owns `PlanRepairToolCode`, `PlanRepairMetadataKey`, JSON
  normalization, and ToolResult extraction so tool/orchestrator code do not
  duplicate repair metadata keys.
- The rendered retry input is bounded and structured: `reason_code`,
  `failing_fields`, `failing_paths`, `accepted_enums`, current/expected bytes,
  `partial_plan_retained`, and `retry_instruction`.
- This is not a prompt hard route: validators, path policy, apply-pre gate, and
  verifier remain the only hard authorities.

RC-33 tasks:

- [x] Export typed PlanRepairPack metadata constants and ToolResult extraction
  helpers from `internal/types`.
- [x] Make plan emit tools attach metadata through the shared constants.
- [x] Add controller `planEmitRejectionView` and render retry hints from typed
  repair-pack fields before falling back to bounded summary prose.
- [x] Add tests proving metadata extraction rejects wrong repair codes / invalid
  payloads.
- [x] Add controller tests proving the retry hint consumes metadata and does not
  echo raw `PLAN_REPAIR_PACK:` summary text.
- [x] Run related package regression, full regression, SWE adapter tests/smoke,
  and diff check before commit/push.

## 2026-06-18 RC-34 Verification Probe Runtime Matrix

The follow-up language audit clarified two different surfaces:

- actual-diff mapping/container boundary signals already cover Python, JS/TS,
  Ruby, Java/Kotlin, and Go as soft semantic coverage obligations;
- bounded inline `verification_probes[]` currently execute only Python,
  JavaScript/Node, Ruby, and Go.

Before adding heavier JVM/Rust/Swift/ArkTS/Cangjie probe runtimes, RC-34 closes
the registry-drift gap inside the existing supported probe set. The same support
matrix must drive:

```text
runtime provider registry
  -> schema enum for emit_change_plan
  -> schema enum for emit_plan_skeleton
  -> schema enum for run_tests(dry_run verification_probe)
  -> validator supported-values list
  -> plan_repair_pack accepted_enums
```

Design constraints:

- This batch does not pretend Java/Kotlin inline probes are supported. JVM
  projects continue to use the project runner path (`mvn`/Gradle/JUnit XML) and
  actual-diff semantic obligations until a real JVM probe executor is designed.
- Schema, validator, and repair-pack values come from a typed runtime registry,
  not copied prompt text or ad hoc string lists.
- Aliases such as `node` and `golang` remain normalizer-only inputs; schemas
  expose canonical runtime names only.
- The registry is a soft tool-surface contract. Hard verification still comes
  from typed probe execution reports and project runner reports.

RC-34 tasks:

- [x] Convert verification probe language support from copied literals to a
  typed runtime spec registry with canonical language, aliases, and display
  description.
- [x] Render `emit_change_plan`, `emit_plan_skeleton`, and
  `run_tests.verification_probe` schema enums from the same registry.
- [x] Keep `supportedVerificationProbeLanguageSet()` feeding plan repair packs
  from the same registry.
- [x] Add schema consistency tests for all three tool surfaces.
- [x] Run related/full regression, SWE adapter smoke, and diff check before
  commit/push.

## 2026-06-18 RC-35 Root-Cause Coverage Dimensions

RC-27 made controller scheduling consume typed PatchReview/Impact coverage, and
RC-32/RC-34 strengthened bounded local proof. The remaining systemic gap is that
the compact PatchReview summary still collapses different root-cause dimensions
into one `semantic_uncovered` bucket. That makes SWE local acceptance and
controller handoff less explainable than the actual typed artifacts already are:
owner-symbol proof, behavior-contract proof, dependent-surface proof,
related-test proof, and actual-diff effect proof have different repair semantics
but the same coarse summary shape.

RC-35 adds a language-agnostic typed dimension layer:

```text
PatchEffect / ImpactObligation
  -> PatchReviewFinding{impact_kind, coverage_status}
  -> PatchReviewCoverageSummary.impact_kind_coverage[]
  -> WriteContextPack / controller repair queue / SWE local telemetry
```

Design constraints:

- `impact_kind` is copied from typed producers (`ImpactObligation.Kind` or
  deterministic patch-effect event producers). It is not inferred from user
  wording, issue text, model rationale, logs, `<think>`, or free-form finding
  messages.
- Existing `CoverageStatus` remains the hard signal. The dimension summary is a
  compact control/telemetry projection, not a new prose gate.
- Unknown legacy records degrade through exact typed finding-code aliases only
  for backward compatibility with durable artifacts. New producers should set
  `impact_kind` directly.
- SWE official prediction export remains unchanged; the adapter only records
  richer local acceptance/debug telemetry.

RC-35 tasks:

- [x] Add `PatchReviewFinding.impact_kind` and per-kind coverage summary fields
  to `PatchReviewCoverageSummary`.
- [x] Populate `impact_kind` in actual-diff effect findings and impact-obligation
  semantic coverage findings.
- [x] Render `impact_kind` into `WriteContextPack` so planner/verifier/controller
  views keep the owner-boundary dimension.
- [x] Let controller impact repair queue prefer `impact_kind` over legacy finding
  code mapping, while preserving existing exact-code fallback.
- [x] Export per-kind PatchReview telemetry from the SWE-bench adapter and cover
  merge/demotion behavior in adapter tests.
- [x] Run focused type/writeflow/orchestrator/adapter tests, related regression,
  full Go tests, SWE adapter smoke, and diff check before commit/push.

## 2026-06-18 RC-36 Impact Suite Queue Preservation

While checking the verifier target-selection path after RC-35, the deterministic
test-surface selector showed a narrower scheduling gap. `impactRunnerPlans` can
produce multiple related-test suite plans from typed `ImpactVerificationTarget`
rows, but `defaultRunnerPlansFromTestSurface` deduplicates queued plans by
working directory. When two related tests live under the same runner root, the
second suite can be dropped before `run_tests` observes it.

This is a system-level verification gap:

```text
ImpactVerificationTarget(test_surface: tests/test_a.py)
ImpactVerificationTarget(test_surface: tests/test_b.py)
  -> two impact runner plans
  -> default queue collapses by working_dir
  -> only one focused related test executes
```

RC-36 changes only the deterministic queue key for scheduler-owned priority
plans. Broad surface candidates continue to dedupe by working directory so
`run_tests {}` does not explode into duplicate full-suite executions. Typed
priority plans dedupe by runner/framework/working_dir/suite, preserving multiple
bounded related-test obligations from impact analysis.

Design constraints:

- No model prose, issue text, user keywords, or verifier narrative participate in
  target selection.
- The verifier still calls `run_tests {}`; the deterministic selector owns suite
  choice and escalation.
- Existing no-tests/dead-end escalation keeps using surface-candidate keys, so
  broad fallback behavior remains stable.

RC-36 tasks:

- [x] Add a runner-plan queue key that includes suite selectors for
  scheduler-owned priority plans.
- [x] Preserve multiple impact related-test suites in the default verification
  queue while still suppressing duplicate broad surface candidates.
- [x] Add regression coverage for two impact related tests in the same working
  directory.
- [x] Run focused `internal/tool` tests, related regression, full Go tests, SWE
  adapter smoke, and diff check before commit/push.

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

Language coverage note: the "dynamic mapping boundary" family is deliberately
language-aware rather than Python-specific. Current producers include Python
nested string-key direct mapping access, JavaScript/TypeScript nested
string-key direct access, Ruby nested symbol/string hash access, Java/Kotlin
chained string-key `Map.get`, and Go nested string-key map assignment. These
signals are all soft semantic coverage obligations that require typed
verification before being marked covered; they are not user-intent keyword
routes and they are not model-prose hard gates.

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

## RC-18: Verify Coverage Persistence

The RC-17 Django smoke confirmed that probe confidence is now present in
`ChangeReport`, but the final `ChangePlan` JSON still showed stale
unverified coverage for the same contracts and symbols. Runtime mutable state
was updated by `syncMutablePlanStatusAfterVerify`, but the updated plan snapshot
was not persisted to `.codrax/plans/<plan>.json`.

Impact:

- SWE adapter and any durable consumer can read stale coverage from disk.
- Resume can rehydrate a plan that lost post-verify coverage projection.
- Controller/handoff evidence can diverge between in-memory state and durable
  artifacts.

Design:

- Make verify post-processing durable: after status and coverage projection,
  persist the current `ChangePlan` snapshot.
- Keep report persistence unchanged; `ChangeReport` remains the authoritative
  verify result, while `ChangePlan` carries the latest lifecycle projection.
- Add a regression that exercises `syncMutablePlanStatusAfterVerify` with a real
  `PlanStore` path and asserts the on-disk JSON contains verified coverage.
- Keep this as deterministic artifact persistence only; no prompt or natural
  language routing changes.

Task list:

- [x] Record RC-18 durable coverage persistence gap.
- [x] Persist coverage-updated plan snapshots after verify sync.
- [x] Add focused persistence regression.
- [x] Run focused orchestrator tests, related packages, full regression, update
  progress, commit, and push.

Verification:

- `go test ./internal/orchestrator -run 'TestSyncMutablePlanStatusAfterVerify'`
- `go test ./internal/orchestrator ./internal/tool ./internal/types ./internal/writeflow`
- `go test ./internal/agent ./internal/repl ./internal/operation`
- `go test ./...`
- `make test`

RC-18 smoke follow-up:

- Run directory:
  `/private/tmp/codrax-swebench-rc18-django-20260618-052359`
- Instance: `django__django-14534`
- Export compatibility: 1/1 harness dry-run validated, non-empty patch.
- `ChangeReport.verification_confidence` is now present for probe reports when
  probes pass, and coverage status is persisted for post-verify unavailable
  attempts.
- Local acceptance still failed:
  `prediction_audit_block_reason=final_plan_test_only_exported_source_patch`,
  `prediction_confidence_downgrade_reason=make_target_missing`.
- New system gap: after replan/impact-repair, the final durable plan can be a
  test-only repair/unverified plan while the adapter exports an earlier source
  patch. The export should be tied to a coherent typed delivery artifact rather
  than mixing final-plan audit status with a previous source diff.

Follow-up RC-19 candidate:

- Define a `WriteDeliveryCandidate` artifact that explicitly binds:
  `plan_id`, source patch diff/fingerprint, optional test patch diff,
  verification report id/status, patch review summary, and confidence summary.
- SWE adapter and user-facing final status should consume the same delivery
  candidate instead of independently guessing from latest plan plus available
  source diff.
- When the latest active plan is test-only but source patch comes from an older
  plan, mark the candidate as `incoherent` unless a typed relation says the
  test-only plan is a validation-only follow-up for the same source patch.
- Keep the rule typed-only: plan file roles, patch effect path roles,
  workflow batch attempts, diff fingerprints, report status, and explicit
  delivery-candidate links.

## 2026-06-18 RC-19 Delivery Candidate Coherence

The RC-18 Django smoke showed that "latest durable plan" is not necessarily the
same thing as "source patch being exported". That is a system-level delivery
artifact gap, not a Django-specific condition:

```text
applied source batch -> applied validation/test-only batch -> final plan is test-only
  -> adapter exports cumulative source diff
  -> local acceptance reads final plan audit/report
  -> source patch and audit authority diverge
```

Design:

- Build a typed delivery candidate from persisted workflow attempts, plan JSON,
  report JSON, exported diff paths, and path-role classification.
- Preserve final-plan telemetry as `final_plan_*`; use `delivery_*` as the
  local-acceptance/export authority.
- Allow a later test-only validation batch only through an explicit typed
  `source_plan_with_later_test_followup` relation.
- Mark candidates `incoherent` when exported source paths are not owned by any
  applied source plan.
- Merge PatchReview summaries across all applied source-owner plans instead of
  reading only the latest plan.
- Keep all routing on typed artifacts; no issue text, stdout prose, model
  rationale, or user-intent keywords influence candidate coherence.

Task list:

- [x] Add plan-path lookup helpers and source/test path projection helpers.
- [x] Add `workflow_applied_plan_summaries`.
- [x] Add `build_write_delivery_candidate` with status/reason/relation fields.
- [x] Add source-owner PatchReview summary merging.
- [x] Switch prediction confidence/local acceptance to delivery candidate facts.
- [x] Preserve final-plan telemetry separately.
- [x] Document new `delivery_*` fields.
- [x] Add focused adapter tests for test-only follow-up coherence and missing
  source-owner incoherence.
- [x] Run adapter tests, Python compile, and local SWE-bench adapter smoke.

Verification:

- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`
- `bash eval/swebench/smoke_local.sh`

Post-fix Lite smoke:

- Run directory:
  `/private/tmp/codrax-swebench-rc20-astropy-20260618-055213`
- Instance: `astropy__astropy-12907`
- Export compatibility: 1/1 harness dry-run validated, non-empty patch.
- Patch size dropped from the RC-19 4.2 MB polluted export to 504 bytes.
- Exported paths: `astropy/modeling/separable.py`.
- `delivery_candidate_status=coherent`,
  `delivery_candidate_relation=source_plan_owns_exported_patch`.
- Remaining failure is verify-environment related:
  `verification_probe_import_error` because the historical Astropy checkout
  could not import without built extension modules and `extension_helpers` was
  missing during `build_ext --inplace`.

## 2026-06-18 RC-20 Typed-Owned Prediction Export

The first post-RC-19 Lite smoke (`astropy__astropy-12907`) exported a
harness-consumable prediction, but the patch was 4.2 MB and included generated
environment/build artifacts under `cextern/wcslib`. RC-19 correctly marked the
delivery candidate incoherent because those exported paths were not owned by
applied source plans, but the official prediction export still contained the
noise. The system gap is broader than Astropy:

```text
environment preparation or editable install mutates generated files
  -> cumulative git diff includes generated artifacts
  -> prediction export emits all non-test changed paths
  -> non-empty patch metrics and manual audit are polluted
```

Design:

- Restrict prediction export to a typed allowlist derived from applied workflow
  plan paths; include test paths only when `--include-test-patches` is active.
- Fall back to final plan paths only when no workflow attempts are available.
- Treat an explicit empty allowlist as "export nothing"; do not fall back to
  all git diff paths.
- Record dropped generated/unowned paths in `dropped_unowned_patch_paths`.
- Keep test-patch dropping separate in `dropped_test_patch_paths`.
- Keep delivery candidate and local acceptance on the same exported path set.
- Do not use repo-specific filenames, issue text, stdout prose, or model
  rationale to decide export eligibility.

Task list:

- [x] Add `export_allowed_patch_paths` from workflow applied plans/final plan.
- [x] Extend `export_patch_between` / `export_patch` with an explicit typed
  allowlist and `dropped_unowned_patch_paths`.
- [x] Wire process results to `export_allowed_patch_paths` and
  `dropped_unowned_patch_paths`.
- [x] Add a real git-diff regression proving generated artifacts are filtered
  while owned source paths export.
- [x] Update adapter README and this ledger.
- [x] Run adapter unit tests, Python compile, and local SWE-bench adapter smoke.

Verification:

- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`
- `bash eval/swebench/smoke_local.sh`

## 2026-06-18 RC-21 PyProject Build Requirement Parsing

RC-20's Astropy rerun exposed an environment-preparation gap: the adapter was
executed under Python 3.9, where `tomllib` is unavailable. Because the adapter
did not try `tomli` or a fallback parser, `[build-system].requires` was not
read from `pyproject.toml`; build helpers such as `extension-helpers` were not
installed before editable/build-ext verification setup.

This is not an Astropy-specific fix. Many historical SWE/Python customer
repositories rely on `pyproject.toml` build-system requirements even when the
outer evaluation interpreter is older than Python 3.11.

Design:

- Prefer stdlib `tomllib` when available.
- Fall back to `tomli` when installed.
- If no TOML library is available, parse only the typed
  `[build-system].requires` string-array field with a narrow TOML-subset
  parser.
- Install parsed build requirements through the existing best-effort pip path.
- Ensure legacy `setuptools.dep_util` compatibility before the
  `--no-build-isolation` editable retry for setup.py projects.
- Keep environment setup observational; failures continue to produce
  predictions and should downgrade local verification confidence rather than
  block official patch export.

Task list:

- [x] Add `tomli` fallback import.
- [x] Add `parse_pyproject_build_requires_fallback`.
- [x] Reuse existing `install_pyproject_build_requires` flow.
- [x] Move legacy `setuptools.dep_util` compatibility before the
  no-build-isolation retry.
- [x] Add pyproject build-system parsing regressions.
- [x] Run adapter unit tests, Python compile, and local adapter smoke.
- [x] Re-run Astropy Lite smoke and record remaining host-compiler limitation.

Verification:

- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`
- `bash eval/swebench/smoke_local.sh`
- RC-21 Astropy rerun:
  `/private/tmp/codrax-swebench-rc21-astropy-20260618-055836`

RC-21 Astropy result:

- `pyproject_build_requires` now includes `extension-helpers`.
- `install_pyproject_build_requires` succeeds.
- Prediction remains clean and harness-consumable: 504-byte patch touching only
  `astropy/modeling/separable.py`.
- Verification still fails locally because extension compilation reaches a host
  compiler/header limit: `fatal error: 'Python.h' file not found`. This is a
  separate verify-sandbox capability gap, not patch-export or root-cause
  localization failure.

## 2026-06-18 RC-22 PatchReview Acceptance Severity Split

The three-instance RC-22 Lite run
(`/private/tmp/codrax-swebench-rc22-3-20260618-060627`) produced 3/3 non-empty
harness-consumable predictions and exposed a local-acceptance severity gap:

- Django remained a true hard block: the patch used direct
  `self.data['attrs']['id']` access and PatchReview surfaced
  `python_nested_string_key_direct_access_added`.
- Pytest looked functionally correct but local verification was unavailable due
  generated-version import setup (`_pytest._version`).
- SymPy passed its typed verification probe for `Array([])`, `Array([[]])`, and
  `Array([[], []])`, and the patch looked functionally correct, but local
  acceptance still failed because broad impact surfaces remained unverified.

The system gap is not that impact telemetry is useless; it is that not every
unverified impact obligation should be a hard local blocker after target
behavior and changed symbols have typed verification coverage.

Design:

- Keep hard local blockers for:
  - missing behavior-contract coverage;
  - missing changed-symbol coverage;
  - actual-diff semantic/structural boundary events such as nested direct
    mapping access and production test scaffolding.
- Treat broad dependent-surface and related-test-surface gaps as confidence
  downgrades when the primary typed verifier passed.
- Preserve telemetry in `plan_patch_review_semantic_unverified_telemetry_codes`.
- Keep all routing on PatchReview typed finding codes and coverage statuses; no
  issue text, model summary, or stdout prose drives acceptance.

Task list:

- [x] Narrow `PATCH_REVIEW_LOCAL_BLOCKER_CODES` to true local hard blockers.
- [x] Filter persisted `coverage_summary.block_reason` through the local
  blocker registry.
- [x] Add `patch_review_confidence_downgrade_reason`.
- [x] Export `plan_patch_review_semantic_unverified_telemetry_codes`.
- [x] Add regressions for actual-diff blocker preservation and
  dependent-surface downgrade.

Verification:

- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`

Post-change validation notes:

- Recomputing the RC-22 three-instance SymPy plan with the updated adapter turns
  `dependent_surface_without_verify_coverage` / `related_test_surface_unverified`
  into `patch_review_semantic_unverified_telemetry_codes` and produces
  `prediction_confidence_downgrade_reason=patch_review_semantic_unverified:dependent_surface_without_verify_coverage`
  instead of a hard block.
- A fresh single SymPy rerun
  (`/private/tmp/codrax-swebench-rc22-sympy-20260618-062458`) exposed the next
  verifier policy gap: the probe passed, but an intermediate plan touched tests,
  so verifier escalated to full pytest, timed out after 5 minutes, and local
  acceptance failed with `plan_touches_test_path`. RC-23 should make verifier
  escalation and local hard failure read the final typed delivery candidate
  and exported path ownership, not stale or unexported intermediate test-path
  touches.

## 2026-06-18 RC-23 Probe-Primary Suite Timeout Policy

The fresh SymPy rerun after RC-22 showed a verifier escalation gap in core
write mode, not just the SWE adapter:

```text
verification_probe passes target behavior and changed symbols
  -> plan touches a test/spec path during repair
  -> verifier continues to project suite
  -> full pytest times out after 5 minutes
  -> ChangeReport becomes failed even though the typed behavior probe passed
```

This is different from a real red test. A suite timeout/resource cap after an
authoritative probe pass is local verification capacity debt; it should lower
confidence, preserve command evidence, and continue the online workflow instead
of making the source patch look functionally failed.

Design:

- Keep the existing rule that a passed probe continues to the project suite when
  the plan touches test/spec paths.
- Preserve hard failure when that continued suite executes and reports real test
  failures.
- When the continued suite exits by verifier infrastructure/resource exhaustion
  (`timeout`, `oom`, `cpu_limit`) and the only continuation reason is
  `plan_touches_test_path`, restore the probe report as the authoritative local
  behavior verdict.
- Attach a `VerificationConfidenceRecord` with
  `category=project_runner`, `status=unavailable`, and reason such as
  `project_suite_timeout_after_probe_pass`.
- Keep executed command rows for both the continuation decision and the suite
  timeout so handoff/debug traces remain transparent.
- Do not read model prose, issue text, or stdout narratives for the decision.

Task list:

- [x] Record the pre-suite continuation reason in `run_tests`.
- [x] Add probe-primary resource-exhaustion conversion for timeout/OOM/CPU.
- [x] Add a focused regression where probe passes, suite times out, and local
  verification remains passed with a confidence warning.
- [x] Preserve existing regression where probe passes but the project suite
  reports a real failure, which remains failed.
- [x] Run focused `internal/tool`, related write packages, and adapter tests.

Verification:

- `go test ./internal/tool -run 'TestRunTestsVerificationProbePass(ContinuesProjectSuiteWhenPlanTouchesTests|DowngradesProjectSuiteTimeoutWhenPlanTouchesTests|SkipsProjectSuiteWhenOnlyContractRefsMissing)'`
- `go test ./internal/tool -run 'TestRunTestsVerificationProbe'`
- `go test ./internal/tool`
- `go test ./internal/orchestrator ./internal/types ./internal/writeflow`
- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`

Post-change validation notes:

- RC-23 SymPy rerun:
  `/private/tmp/codrax-swebench-rc23-sympy-20260618-064126`
- The run did not reproduce the RC-22 timeout shape. It first hit real
  behavior-probe failures, replanned twice, then produced a 1228-byte official
  prediction patch touching only `sympy/tensor/array/ndim_array.py`.
- The final local verifier verdict was passed:
  `verify_status=passed`, `verify_passed=true`, `verify_test_count=1`.
- This is useful negative evidence for RC-23: real probe failures still drove
  replan and were not softened by the new resource-exhaustion policy.
- The run exposed the next gap: the adapter marked local acceptance failed with
  `patch_review_semantic_unverified:changed_symbol_without_probe_coverage` even
  though the final plan's normalized patch-review local blocker was empty and
  verification passed. The cause is stale/intermediate source-owner plan review
  being combined into the delivery candidate instead of letting the final
  verified delivery report dominate.

## 2026-06-18 RC-24 Delivery PatchReview Authority

The RC-23 SymPy run exposed a second-order online convergence bug. The workflow
did the right thing operationally:

```text
bad first patch -> typed probe failure -> replan
  -> bad second patch -> typed probe failure -> replan
  -> final source patch -> typed verifier passes
```

But the SWE-bench adapter still combined PatchReview summaries from every
source-owner plan as if all attempts were equally authoritative. That made a
stale proof gap from a failed intermediate plan block the final prediction:

```text
final verify_status=passed
final_plan_patch_review_block_reason=""
combined plan_patch_review_block_reason=
  patch_review_semantic_unverified:changed_symbol_without_probe_coverage
```

This is a system-level mismatch between online edit-run-observe convergence and
batch-style audit aggregation. The fix is not to ignore intermediate plans:
early plans can still introduce real actual-diff structural/boundary risks that
remain present in the exported patch. The fix is to split PatchReview blockers
by authority:

- **Effect/structural blockers** from any exported source-owner plan remain hard
  blockers. Examples: path/scope errors, generated production test scaffolding,
  direct dynamic mapping/container boundary events.
- **Proof-only blockers** (`behavior_contract_without_verify_coverage`,
  `changed_symbol_without_probe_coverage`) are authoritative only on the final
  report plan after a coherent delivery candidate has a passed typed verifier
  report. The final report proves the cumulative worktree; stale proof gaps from
  failed intermediate attempts should become confidence telemetry.
- If delivery is incoherent, verifier did not pass, or no report authority exists,
  the previous conservative source-owner aggregation stays in force.
- The decision consumes only typed plan IDs, delivery status, verify status,
  PatchReview finding codes, and coverage statuses. No model prose, issue text,
  stdout narrative, or keyword route is used.

Task list:

- [x] Add a proof-blocker code registry separate from actual-diff effect
  blockers in the SWE-bench adapter.
- [x] Add a delivery PatchReview combiner that accepts report-authority plan IDs
  and demotes stale non-authority proof blockers to telemetry only.
- [x] Keep actual-diff/structural blockers from any source-owner plan as hard
  local blockers.
- [x] Export enough typed telemetry to explain which proof authority was used.
- [x] Add adapter regressions for stale proof-blocker demotion and actual-diff
  blocker preservation.
- [x] Recompute the RC-23 SymPy result through the adapter and verify it becomes
  low-confidence passed/unblocked rather than audit-blocked.

Verification:

- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`

Post-change validation notes:

- Recomputing the RC-23 SymPy artifacts through the new combiner produces
  `prediction_verdict=predicted_passed_low_confidence`,
  `blocks_local_acceptance=false`, and no `audit_block_reason`.
- Stale proof gaps remain visible as
  `plan_patch_review_semantic_unverified_telemetry_codes`, including
  `changed_symbol_without_probe_coverage`.
- The confidence downgrade remains
  `verification_probe_missing_required_contract_ref`, so the adapter does not
  overstate local certainty.

## 2026-06-18 RC-25 Probe Reference Enrichment

RC-24 intentionally kept the RC-23 SymPy delivery as low confidence because the
final plan's probe proved behavior but did not carry `contract_refs`. That is
correct for already-recorded artifacts, but it exposes a future-runtime gap:
model-emitted JSON routinely omits optional metadata even when the typed plan
context makes the reference unambiguous. Making every planner prompt carry this
burden creates model mindshare tax and repeats the same repair hint across
stages.

Design:

- Add a deterministic post-normalization enrichment step before approval,
  fingerprinting, persistence, apply, and verify.
- If a plan has exactly one verification probe and exactly one required
  non-observed behavior contract, fill `verification_probes[0].contract_refs`
  when the model omitted it.
- If `changed_symbol_refs` is empty, fill it from the single contract's
  `subject` when available; otherwise fill a transparent `path:<repo-rel>`
  reference when the plan has exactly one non-test target path.
- Preserve all explicit refs exactly as emitted.
- Do not guess when there are multiple probes or multiple required contracts.
- Keep verifier/report confidence logic unchanged; enrichment only improves the
  typed input facts it consumes.
- Apply the same helper to single-shot `emit_change_plan` and multi-round
  `emit_plan_skeleton` so large plans do not drift.
- Hard gates continue to read typed artifacts only. No user keywords, issue text,
  model prose, stdout, or `<think>` output participates.

Task list:

- [x] Add shared `enrichVerificationProbeRefs` helper.
- [x] Wire it into `emit_change_plan`.
- [x] Wire it into `emit_plan_skeleton`.
- [x] Add regressions for single-probe enrichment and multi-contract no-guess.
- [x] Run focused tool tests and related package tests.

Verification:

- `go test ./internal/tool -run 'TestEmitChangePlan_(PersistsBehaviorContractsAndProbeRefs|EnrichesSingleProbeRefsFromTypedPlanContext|AcceptsPartialRequiredContractProbeCoverage|DoesNotGuessProbeRefsWhenMultipleRequiredContracts)'`
- `go test ./internal/tool -run 'TestVerificationConfidenceRecordsFromProbeReport'`
- `go test ./internal/tool`

## 2026-06-18 RC-26 Hard/Soft Probe Coverage Authority

RC-25 removed low-ambiguity metadata omissions, but the real RC-23 SymPy
artifact still has multiple expected outcomes:

```text
hard required contracts: []
soft/fallback required contracts:
  no-error-array-empty, outcome-1, outcome-2, outcome-3, outcome-4
```

The old SWE adapter collapsed both categories into
`verification_probe_missing_required_contract_ref`, while runtime confidence
used only hard-required contracts. That mismatch made the reason code look like
a hard proof gap even when the actual problem was weaker: the probe passed a
local behavior check but did not explicitly cover every natural-language
expected outcome such as "existing suite passes".

Design:

- Keep hard-required probe coverage semantics aligned with
  `types.IsHardRequiredWriteBehaviorContract`:
  non-observed, non-fallback, required contracts with typed exact/existence/
  raise/return style operators.
- Project non-hard required contracts, including fallback expected outcomes and
  `satisfies` contracts, into a separate confidence lane:
  `category=probe_soft_contract_refs`.
- Missing hard refs still use
  `verification_probe_missing_required_contract_ref`.
- Missing soft/fallback refs use
  `verification_probe_missing_soft_contract_ref`.
- Satisfied soft/fallback refs use
  `verification_probe_soft_contract_ref_covered`.
- Adapter fallback logic mirrors runtime's hard/soft split for historical
  artifacts that lack `verification_confidence`.
- No prompt, issue text, stdout prose, or keyword matching participates; only
  typed contract fields, typed probe refs, report status, and command/test
  suites are consumed.

Task list:

- [x] Add runtime soft-required contract confidence records for probe-only pass.
- [x] Keep hard-required changed-symbol coupling unchanged.
- [x] Add adapter hard/soft contract helpers matching runtime semantics.
- [x] Update adapter confidence fallback to return soft-specific reason.
- [x] Add Go and adapter regressions.
- [x] Recompute RC-23 SymPy artifact: reason becomes
  `verification_probe_missing_soft_contract_ref`, with no hard-required
  contracts.

Verification:

- `go test ./internal/tool -run 'TestVerificationConfidenceRecordsFromProbeReport|TestRunPlanVerificationProbesAttachesConfidenceToProbeReport'`
- `go test ./internal/tool`
- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`
- `eval/swebench/smoke_local.sh`

## 2026-06-18 RC-27 Impact Follow-up Authority

The RC-23 SymPy workflow revealed a state-machine authority bug after the
functional patch was locally verified:

```text
batch-1 verify passed tests_passed
batch-1 impact_obligation_followup_requested
batch-1-impact-repair ready_to_plan
run status in_progress
```

The final plan's changed-symbol and actual-diff guard/call-site coverage had
already been marked `verified`. The remaining uncovered items were broad graph
fan-out (`dependent`, `dependency`, `test_surface`) plus a generic
`effect_followup` target from the patch-effect obligation projection. Those
signals are useful confidence telemetry and handoff context, but they do not
identify a bounded code change that must occur after a passed local verifier.

Root cause:

- `normalizeControllerTypedStateDecision` asks
  `impactObligationRepairFollowupNeeded` before finishing a completed
  non-failed batch.
- `impactObligationRepairFollowupNeeded` treated any selected uncovered
  `ImpactVerificationTarget` or semantic `PatchReviewFinding` as an executable
  repair obligation.
- `selectImpactRepairQueueItems` therefore gave graph-wide inferred targets the
  same scheduling authority as hard proof gaps (`changed_symbol`,
  `behavior_contract`) and actual-diff boundary events.

Design:

- Split impact queue items into:
  - **Executable follow-up authority**: hard proof obligations that can be
    closed by a bounded replan or explicit probe (`changed_symbol`,
    `behavior_contract`) and actual-diff boundary events registered in
    `PatchReviewEffectUnknownCoverage` such as dynamic mapping/default-boundary
    signals across Python, JavaScript/TypeScript, Ruby, Java/Kotlin, and Go.
  - **Confidence telemetry**: broad graph fan-out (`dependent`,
    `dependency`, `test_surface`, `changed_file`) and generic patch-effect
    follow-up rows without an uncovered boundary event. These stay in
    `PatchReview`, `ImpactAnalysis`, reports, and P2 context, but cannot append
    a new code batch after a non-failed verifier verdict.
- Keep hard/soft routing structural:
  - consume typed `kind`, `code`, `coverage_status`, and `source`;
  - do not read user issue text, model rationale/summary, stdout prose, or
    natural-language success criteria.
- Preserve RC-14's useful behavior:
  - missing hard changed-symbol or behavior-contract proof still appends one
    bounded follow-up;
  - actual-diff dynamic boundary unknown coverage still appends a bounded
    follow-up;
  - the one-shot recursion guard remains workflow-wide.
- Improve terminal semantics:
  - when only confidence telemetry remains after a passed verifier, the
    controller can finish the run with low-confidence evidence rather than
    leaving the durable workflow `in_progress`.

Task list:

- [x] Replayed the RC-23 SymPy artifact and confirmed the appended batch was
  caused by soft graph/effect telemetry after the real changed surface verified.
- [x] Add typed `impactRepairQueueItemRequiresFollowup` authority classifier.
- [x] Filter `selectImpactRepairQueueItems` through the authority classifier.
- [x] Add regressions for passed verifier + soft-only telemetry finishing.
- [x] Add regressions for passed verifier + actual-diff boundary unknown still
  appending a bounded follow-up.
- [x] Run focused controller/writeflow tests and adapter smoke.

Verification:

- `go test ./internal/orchestrator -run 'TestNormalizeControllerTypedStateDecision(VerifiedButUndercoveredAppendsImpactRepair|VerifiedSoftTelemetryOnlyFinishes|VerifiedActualDiffBoundaryAppendsFollowup|ImpactRepairWithoutPathExploresFirst|SemanticPatchReviewFollowupRunsOnce|SemanticPatchReviewDoesNotRecurse)'`
- `go test ./internal/writeflow ./internal/types -run 'Test.*(PatchReview|Impact|Coverage|Effect)'`
- `go test ./internal/orchestrator ./internal/writeflow ./internal/types ./internal/tool`
- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`
- `eval/swebench/smoke_local.sh`
- `git diff --check`

## 2026-06-18 RC-28 Multi-language BuildError Changed-line Attribution

The shared `PreflightDiagnostic` / changed-line filter was intentionally
language-neutral, but only the Python static-name producer was wired through
that filter. This leaves a system-level false-negative class for other
languages: a native project runner can parse `BuildErrors[]` from javac, tsc,
rustc, Go, Node, Ruby, C/C++, Swift, Cangjie, or similar output, but a
pre-existing error on an untouched line still looks like an authoritative
post-apply build failure.

Current code already has the right reusable pieces:

- `parseBuildErrors` produces typed `BuildError{file,line,column,message}`
  rows for many toolchains.
- `writeflow.FilterPreflightDiagnosticsToChangedLines` already decides whether
  a typed file:line diagnostic intersects the current `ChangePlan` changed-line
  surface.
- `ChangeReport.VerificationStatus` and typed `FailureKind` already separate
  failed code evidence from unavailable local verification.

Design:

- Add a typed `preexisting_build_failure` failure kind whose
  `VerificationStatus` is `unavailable`.
- During `run_tests` report finalization, scope `BuildErrors[]` against the
  active `ChangePlan`:
  - if any build error lacks a precise path/line match, keep normal
    `build_failure` (fail-closed);
  - if any build error intersects a changed line, keep normal
    `build_failure`;
  - if all build errors are precise and outside changed lines, preserve them as
    P2 handoff evidence but downgrade the primary verifier verdict to
    `preexisting_build_failure`.
- Keep routing structural:
  - only typed `BuildError` file/line/column and typed `ChangePlan` changed-line
    surfaces participate;
  - no user-intent keywords, model prose, stdout summaries, or language-specific
    issue text are used for the decision.
- Preserve handoff:
  - build-error rows remain in `ChangeReport.TestResults[].BuildErrors`;
  - a typed `VerificationDiagnostic` records the downgrade reason;
  - controller/planner/verifier can still consume P2 context, but the workflow
    does not chase unrelated repository debt as a code defect.

Task list:

- [x] Add `FailureKindPreexistingBuildFailure` and mark it unavailable.
- [x] Add a `run_tests` report qualifier that scopes all structured
  `BuildErrors[]` through the changed-line filter.
- [x] Keep fail-closed behavior for unstructured build failures, missing line
  numbers, unmatched paths, and imprecise plan surfaces.
- [x] Add unit tests for outside-line downgrade and changed-line retention.
- [x] Run related `types`, `tool`, `orchestrator`, and SWE adapter smoke.

Verification:

- `go test ./internal/tool -run 'Test(QualifyChangeReport_QualifiesBuildErrorPathsFromRunnerRoot|ScopeBuildFailureReportToChangedLines|QualifyChangeReport|MergeChangeReports|RenderTestSummary)'`
- `go test ./internal/types -run 'TestChangeReportNormalizeVerificationStatus|TestChangeReportEnsureVerificationStatusBackfillsNoTestsFailureKind'`
- `go test ./internal/orchestrator -run 'TestNormalizeControllerTypedStateDecision.*Unverified|Test.*Preexisting|Test.*Verify'`
- `go test ./internal/tool ./internal/types ./internal/orchestrator ./internal/writeflow`
- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`
- `go test ./...`
- `eval/swebench/smoke_local.sh`
- `git diff --check`

## RC-37: Convention Evidence Projection

### Gap

`ConventionGraph` now persists repository-local style/mechanism/relationship
signals and `PatchReview` can emit advisory convention findings, but the
finding only carried generic `convention_surface_available` metadata. The
actual convention summary, source stage, and line span stayed on the graph node.
That made downstream planner/verifier/controller views know that convention
context existed, without giving them enough typed evidence to decide what to
inspect next.

This is a handoff-consumption gap rather than a new hard-gate opportunity.
Repository convention text is a noisy signal and must remain soft guidance. The
fix must not route on user intent keywords, model rationale, or natural-language
summaries. It should only preserve already-produced typed evidence fields.

### Design

- Extend `PatchReviewFinding` with generic evidence projection fields:
  `source_stage`, `line_start`, `line_end`, and `context_summary`.
- Populate those fields from normalized `ConventionNode` values when
  `patchReviewConventionFindings` emits advisory convention findings.
- Render those fields through `WriteContextPack` as a separate
  `patch_review_evidence` item so planner/verifier/controller Top-N views can
  consume convention evidence without overloading the verdict-oriented
  `patch_review_finding` row or reopening the whole graph.
- Keep convention findings `coverage_status=advisory`; they must never become
  hard blockers by themselves.
- Keep all hard routing on typed enums/booleans already present in
  `PatchReviewCoverageSummary` and controller scheduling. `context_summary` is
  handoff text only.

### Tasks

- [x] Add normalized evidence projection fields to `PatchReviewFinding`.
- [x] Copy `ConventionNode` summary/source-stage/line span into advisory
  convention patch-review findings.
- [x] Render the fields in `WriteContextPack` patch-review evidence rows.
- [x] Add regressions for normalize, semantic review, and context-pack
  projection.

### Verification

- `go test ./internal/types -run 'TestNormalizePatchReviewRecord|TestWriteContextPackFromChangePlanCarriesPatchEffect' -count=1`
- `go test ./internal/writeflow -run 'TestReviewAppliedPatchSemanticAddsConventionFindingAsAdvisory' -count=1`
- `go test ./internal/types ./internal/writeflow ./internal/orchestrator -count=1`
- `go test ./...`
- `make test`
- `eval/swebench/smoke_local.sh`
- `git diff --check`

## RC-38: Multi-Language Probe-Only Adapter Authority

### Gap

RC-13 and RC-17 generalized core verification probes from Python-only to typed
`verification_probe/<language>` suites, but the SWE-bench adapter's local
confidence helper still recognized only `verification_probe/python`. That leaves
the runtime and evaluation consumer out of sync:

```text
JS/Ruby/Go verification probe passes
  -> ChangeReport carries typed verification_probe/<language> suite
  -> adapter does not classify the report as probe-only
  -> contract/source-context confidence checks are skipped
  -> manual audit and local confidence telemetry diverge from runtime evidence
```

This is a typed consumer bug, not a language-specific prompt issue. The fix must
consume the report's structured `suite` field only. It must not inspect user
request prose, model rationale, or issue text.

### Design

- Add an adapter helper that accepts any schema-shaped
  `verification_probe/<non-empty-language>` suite and rejects untyped or nested
  variants such as `verification_probe`, `verification_probe/`, and
  `verification_probe/python/extra`.
- Keep the existing guard that a probe-only confidence path cannot include a
  successful non-probe executed command.
- Preserve project-suite behavior: mixed probe plus project test results are not
  probe-only and should not trigger probe-only downgrade logic.
- Keep this as audit/confidence telemetry only; official prediction export
  remains unchanged.

### Tasks

- [x] Replace the adapter's Python-only suite equality with a typed
  `verification_probe/<language>` classifier.
- [x] Add unit coverage for JavaScript, Ruby, and Go probe-only suites.
- [x] Add rejection coverage for untyped, malformed, and mixed project suites.

### Verification

- `python3 -m unittest eval.swebench.run_codrax_swebench_test.PredictionConfidenceTests`
- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`
- `eval/swebench/smoke_local.sh`
- `go test ./...`
- `make test`
- `git diff --check`

## RC-39: Node Impact Related-Test Selector

### Gap

`ImpactAnalysisResult.VerificationTargets` can already express "changed source A
should verify related test B" as typed `kind=test_surface` obligations. The
deterministic selector then converts those obligations into priority runner
plans before the broad default queue. That worked for Python, Go, and Ruby, but
Node/JS/TS targets were still rejected as unsupported even though `run_tests`
already supports Node/Jest/Vitest and `TestSurface` detects `package.json`.

This undercuts the non-Python "改 A 查 B" workflow:

```text
ImpactVerificationTarget(test_surface: src/widget.test.ts)
  -> TestSurface has node/package.json candidate
  -> impact selector rejects node as unsupported
  -> default queue runs broad npm test or misses the targeted related test
```

### Design

- Allow Node candidates to consume typed related-test paths from
  `ImpactVerificationTarget.RelatedPath`.
- Accept only repo-relative JavaScript/TypeScript-family file selectors:
  `.js`, `.jsx`, `.mjs`, `.cjs`, `.ts`, `.tsx`, `.mts`, `.cts`.
- Keep unsafe paths and non-test/non-code selectors rejected by the existing
  boundary checks plus the new extension guard.
- Adjust Node command construction so file selectors are passed as positional
  Jest/Vitest filters, while ordinary test-name selectors still use `-t`.
- Keep routing structural: only typed TestSurface candidates and typed related
  paths participate; no issue text, user keywords, model rationale, or stdout
  prose drives the decision.

### Tasks

- [x] Add Node to the impact suite-capable runner set.
- [x] Map JS/TS-family related test files to Node runner-plan suites.
- [x] Render Node file suites as positional npm test selectors instead of
  `-t` test-name patterns.
- [x] Add regressions for positive Node impact queueing, unsafe/unsupported
  selector rejection, and command construction.

### Verification

- `go test ./internal/tool -run 'TestImpactRunnerPlansFromChangePlan(TargetsNodeRelatedTest|RejectsUnsafeOrUnsupportedTargets)|TestBuildRunCommandForPlan_NodeFileSuiteUsesPositionalSelector' -count=1`
- `go test ./internal/tool -count=1`
- `go test ./...`
- `make test`
- `eval/swebench/smoke_local.sh`
- `git diff --check`

## RC-40: Go No-Test Source Compile Fallback

### Gap

RC-28 made native runner `BuildErrors[]` language-neutral, and RC-39 improved
typed related-test selection for Node. A remaining non-Python verify gap was
the no-test branch for Go:

```text
ChangePlan touches cmd/app/main.go
  -> TestSurface detects Go runner
  -> runnerHasNoTestWork("go") == true when no *_test.go files exist
  -> run_tests emitted synthetic NoTestsRunners/pass-with-warning
  -> compile errors in changed production Go could be missed until manual audit
```

That is a systemic false-confidence problem, not a SWE-bench case issue. Go's
standard `go test` is already the correct deterministic harness: even with no
test files, it compiles the package and emits typed file:line diagnostics on
compile failure. Codrax should use that harness automatically instead of asking
the user to know when to run a manual compile command.

### Design

- Extend the existing syntax/source fallback dispatcher with a Go source-compile
  provider.
- Keep the behavior typed and deterministic:
  - the trigger is runner language + `ChangePlan.target_paths` extension +
    filesystem test-surface state;
  - the command is the standard Go toolchain, not model prose or user keyword
    matching;
  - diagnostics continue through `parseBuildErrors`,
    `qualifyChangeReport`, and `scopeBuildFailureReportToChangedLines`.
- Run Go compile fallback only in the no-test-work branch. When Go tests exist,
  the normal `go test -json` project runner already compiles and tests the
  package, so an extra preflight would duplicate work.
- Scope execution to package directories containing plan-touched `.go` files
  under the runner root instead of broad `./...`, reducing blast radius and
  avoiding unrelated packages where possible.
- If `go` is missing and there are no tests, keep the existing customer-friendly
  unavailable/pass-with-warning semantics; missing local toolchains are not code
  failures.

### Tasks

- [x] Add Go `.go` source-compile fallback provider for no-test-work paths.
- [x] Preserve Go project-runner behavior when tests exist.
- [x] Parse Go compile failures into `BuildErrors[]` and reuse changed-line
  attribution.
- [x] Add regressions for successful no-test Go compile, compile-error failure,
  and end-to-end `run_tests({})` no-test failure.
- [x] Update user guide Markdown/HTML and this progress ledger.

### Verification

- `go test ./internal/tool -run 'TestRunGoCompileFallback|TestRunTestsEmptyParamsGoNoTestsCompileFailure|TestSyntaxCheckExtensions_OnlySupportedRunners' -count=1`
- `go test ./internal/tool -count=1`
- `go test ./...`
- `make test`
- `eval/swebench/smoke_local.sh`
- `git diff --check`

## RC-41: TypeScript No-Test Compile Fallback

### Gap

Node's no-test fallback intentionally uses `node --check` only for JavaScript
family files. That avoids feeding TypeScript syntax to Node, but it leaves a
TypeScript-only package with no test files in a weak state:

```text
ChangePlan touches src/widget.ts
  -> TestSurface detects Node/package.json
  -> runnerHasNoTestWork("node") == true
  -> node syntax fallback sees no .js/.mjs/.cjs files
  -> synthetic no_tests/unavailable can hide tsc-detectable compile errors
```

This is the same class as RC-40: the system should use the language's standard
bounded compiler proof when no tests exist, instead of asking the user to know
which command to run.

### Design

- Keep project-test behavior unchanged. If Jest/Vitest tests exist, the normal
  runner remains the authority.
- Extend only the no-test source fallback for `runner=node` to include
  TypeScript-family plan files.
- For `.ts/.tsx/.mts/.cts` files, prefer repo-local `node_modules/.bin/tsc`;
  otherwise use `tsc` from `PATH`. Missing `tsc` stays pass-with-warning /
  unavailable, not a code failure.
- Run `tsc --noEmit --pretty false` from the runner root when `tsconfig.json`
  exists; otherwise pass the plan-touched TypeScript files explicitly.
- Parse TypeScript diagnostics through the existing `parseBuildErrors` and
  route them through the same `qualifyChangeReport` /
  `scopeBuildFailureReportToChangedLines` path.
- Do not use issue text, user keywords, model rationale, stdout prose, or
  case-specific identifiers for routing.

### Tasks

- [x] Add a no-test source-check extension set that includes TypeScript for
  Node without enabling broad TS preflight before ordinary project tests.
- [x] Add TypeScript `tsc --noEmit --pretty false` fallback inside the Node
  no-test source checker.
- [x] Add regressions for TypeScript compile diagnostics and end-to-end
  `run_tests({})` no-test TS failure.
- [x] Update user guide Markdown/HTML and this ledger.

### Verification

- `go test ./internal/tool -run 'TestRunNodeCheckFallback_TypeScript|TestRunTestsEmptyParamsNodeTypeScriptNoTestsCompileFailure|TestSourceCheckExtensionsForNoTestWork' -count=1`
- `go test ./internal/tool -count=1`
- `go test ./...`
- `make test`
- `eval/swebench/smoke_local.sh`
- `git diff --check`

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
| RC-18 | complete | Verify coverage persistence: `syncMutablePlanStatusAfterVerify` now persists the coverage-updated `ChangePlan` snapshot after post-verify projection, and focused regression proves on-disk PatchReview/Impact coverage matches mutable state. Verification: focused orchestrator persistence test, related packages, full `go test ./...`, and `make test` pass. |
| RC-18 smoke | complete | One-instance Django smoke after RC-18 exported a non-empty harness-consumable prediction and proved probe confidence is present in reports. It also exposed the next gap: final test-only repair plan and exported source patch can diverge, producing `final_plan_test_only_exported_source_patch`. |
| RC-19 | complete | Delivery candidate coherence: SWE-bench adapter now binds exported source paths to applied source-owner plans, accepts later test-only validation only through an explicit typed relation, merges PatchReview across source-owner plans, and drives local acceptance from `delivery_*` facts instead of final-plan drift. Verification: adapter unit tests, Python compile, and local SWE-bench adapter smoke pass. |
| RC-20 | complete | Typed-owned prediction export: post-RC-19 Lite smoke exposed 4.2 MB generated build artifacts in the official prediction. Export now uses workflow/final-plan typed path ownership as an allowlist, records unowned drops separately from test-patch drops, and never falls back from an explicit empty allowlist to all git diff paths. Verification: adapter unit tests, Python compile, and local SWE-bench adapter smoke pass. |
| RC-21 | complete | Pyproject build requirement parsing: adapter now uses `tomllib`, `tomli`, or a narrow `[build-system].requires` fallback so Python 3.9 eval runs still install typed build helpers such as `extension-helpers`; legacy `setuptools.dep_util` compatibility now runs before no-build-isolation editable retry. Astropy rerun confirms build requirements are installed and prediction export remains clean; local verify still hits host `Python.h` compiler-header limits, tracked as the next verify-sandbox gap. |
| RC-22 | complete | PatchReview acceptance severity split: local hard blockers now stay reserved for missing behavior/changed-symbol proof and actual-diff boundary/structural risks; broad dependent/test-surface impact gaps become confidence downgrade telemetry when target typed verification passes. SymPy RC-22 audit now recomputes as low-confidence verified instead of hard-blocked, while Django's nested direct mapping access remains blocked. |
| RC-23 | complete | Probe-primary suite timeout policy: core `run_tests` now preserves a passed verification probe as the authoritative local behavior verdict when the only suite continuation reason is `plan_touches_test_path` and the continued project suite hits timeout/OOM/CPU verifier capacity limits. Real red suite failures still fail. The report retains continuation and timeout command evidence plus a `project_runner` confidence warning. SymPy RC-23 did not re-hit the timeout path; it confirmed real probe failures still replan and exposed the next delivery-candidate review ownership gap. |
| RC-24 | complete | Delivery PatchReview authority: SWE local acceptance now separates proof-only blockers from actual-diff/effect blockers when a coherent delivery candidate has a passed report. Stale proof blockers from intermediate attempts become telemetry, while actual-diff boundary blockers from any source-owner plan remain hard. Recomputing RC-23 SymPy now yields `predicted_passed_low_confidence` instead of audit-blocked. |
| RC-25 | complete | Probe reference enrichment: single-probe, single-required-contract plans now auto-fill omitted `contract_refs`, and fill `changed_symbol_refs` from contract subject or a transparent single-source `path:<repo-rel>` fallback. Explicit refs are preserved, multi-contract plans are not guessed, and both single-shot plus skeleton planning paths share the helper. |
| RC-26 | complete | Hard/soft probe coverage authority: runtime and SWE adapter now distinguish hard-required contract coverage from soft/fallback expected-outcome coverage. Historical RC-23 SymPy recomputes to `verification_probe_missing_soft_contract_ref` with no hard-required gaps, preserving low confidence without overstating the failure as a hard required contract omission. |
| RC-27 | complete | Impact follow-up authority: controller scheduling now keeps broad graph/effect telemetry as low-confidence handoff after a passed verifier, while hard proof gaps and actual-diff boundary events can still append one bounded follow-up. Regression coverage locks both sides, and local SWE-bench adapter smoke remains harness-consumable. |
| RC-28 | complete | Multi-language BuildError changed-line attribution: structured compiler diagnostics from native runners now reuse the existing changed-line authority. Errors proven outside current patch lines become `preexisting_build_failure` unavailable handoff evidence; changed-line, unstructured, pathless, or imprecise diagnostics remain fail-closed `build_failure`. User guide Markdown/HTML and full regression evidence were updated. |
| RC-29 | complete | Source-shape provider registry: actual-diff mapping/container boundary and production-test scaffold producers are now declared by deterministic language providers instead of central source-kind switches. Python duplicate/top-level owner checks moved behind provider hooks, current Python/JS/TS/Ruby/Java/Kotlin/Go event behavior is preserved, and registry coverage tests lock unique extension ownership plus required typed rules. Verification: focused writeflow provider tests and related `internal/writeflow ./internal/orchestrator ./internal/types ./internal/tool` tests pass. |
| RC-30 | complete | Official SWE-bench result summary: added dependency-free `summarize_official_results.py` to normalize official harness run reports or per-instance reports into explicit `resolved/submitted`, `resolved/completed`, and `resolved/total` metrics. The wrapper now passes `REPORT_DIR`, README documents the official scoring flow, and tests cover denominator handling plus CLI JSON output without importing SWE-bench. Verification: system Python and eval-venv tests, adapter unit suite, local SWE-bench smoke, and diff check pass. |
| RC-31 | complete | Deterministic checkpoint rewind before failed-verify replan: controller now restores the active slice worktree to its typed checkpoint commit before planner replan, records `slice_restored` metadata, rejects external checkpoint paths, and leaves main checkout untouched. This closes the online-convergence state-kernel gap where repair could plan on dirty failed side effects. Verification: focused controller/types restore tests, related orchestrator/types/worktree regression, full `go test ./...`, and diff check pass. |
| RC-32 | complete | Multi-language verification-probe coupling: the copied-implementation hard gate now uses providers for Python, JavaScript/TypeScript, Ruby, and Go. JS/Ruby/Go probes must import/require the changed production module when a same-language target is present, while Python public-package behavior is preserved. This strengthens bounded local proof in missing-test-runner environments without parsing prose or adding user approvals. Verification: focused tool coupling tests, related tool regression, full `go test ./...`, SWE adapter unit/smoke, and diff check pass. |
| RC-33 | complete | Typed plan repair retry consumption: `PlanRepairPack` metadata constants and extraction helpers now live in `internal/types`; controller no-plan retry renders bounded structured repair fields from `ToolResult.Repair.Metadata` instead of asking the planner to mine capped rejection prose. Verification: focused types/tool/orchestrator tests, related package regression, full `go test ./...`, SWE adapter unit/smoke, and diff check pass. |
| RC-34 | complete | Verification probe runtime matrix: existing Python/JavaScript/Ruby/Go inline probe support now has a single typed runtime registry that feeds schema enums, validator supported-values, and plan repair accepted-enums. JVM inline probes remain explicit future work rather than an implied capability. Verification: focused schema/runtime tests, related package regression, full `go test ./...`, SWE adapter unit/smoke, and diff check pass. |
| RC-35 | complete | Root-cause coverage dimensions: PatchReview findings now carry typed `impact_kind`, coverage summary reports per-kind owner/contract/dependent/test/effect buckets, context pack renders the dimension for planner/verifier/controller handoff, controller repair queue prefers typed impact kind over legacy code fallback, and SWE adapter exports per-kind local telemetry without changing official predictions. Verification: focused type/writeflow/orchestrator/adapter tests, related package regression, full `go test ./...`, `make test`, SWE adapter smoke, and diff check pass. |
| RC-36 | complete | Impact suite queue preservation: scheduler-owned priority runner plans now dedupe by runner/framework/working_dir/suite, so multiple typed related-test obligations in the same working directory survive into the default `run_tests {}` queue, while broad surface candidates still dedupe by working directory. Verification: focused impact selector tests, full `internal/tool`, full `go test ./...`, `make test`, SWE adapter smoke, and diff check pass. |
| RC-37 | complete | Convention evidence projection: advisory convention patch-review findings now carry the source stage, line span, and context summary from the typed `ConventionGraph` node, and context packs render those fields as separate `patch_review_evidence` items for downstream consumers without overloading verdict rows. Verification: focused types/writeflow tests, related packages, full `go test ./...`, `make test`, SWE adapter smoke, and diff check pass. |
| RC-38 | complete | Multi-language probe-only adapter authority: SWE-bench local confidence now treats any typed `verification_probe/<language>` suite as probe-only evidence and rejects malformed/mixed suites, aligning adapter consumption with the core runtime matrix. Verification: focused/all adapter unit tests, Python compile, SWE adapter smoke, full `go test ./...`, `make test`, and diff check pass. |
| RC-39 | complete | Node impact related-test selector: typed ImpactAnalysis related-test targets can now prioritize JS/TS-family Node suites, with Node file selectors rendered as positional Jest/Vitest filters. Verification: focused Node impact/command tests, full `internal/tool`, full `go test ./...`, `make test`, SWE adapter smoke, and diff check pass. |
| RC-40 | complete | Go no-test source compile fallback: plan-touched Go packages with no `_test.go` files now run a bounded `go test -json` compile check instead of synthetic no-tests pass. Verification: focused Go fallback tests, full `internal/tool`, full `go test ./...`, `make test`, SWE adapter smoke, and diff check pass. |
| RC-41 | complete | TypeScript no-test source compile fallback: plan-touched TS files in Node packages now use `tsc --noEmit --pretty false` when available instead of synthetic no-tests pass. Verification: focused TS fallback tests, full `internal/tool`, full `go test ./...`, `make test`, SWE adapter smoke, and diff check pass. |

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
- No-test Go source changes compile automatically through the standard Go
  toolchain before being marked unavailable/pass-with-warning.
- No-test TypeScript source changes compile automatically through repo-local or
  PATH `tsc --noEmit --pretty false` when available.
- Uncovered typed impact obligations become a bounded repair queue before
  finish, instead of remaining passive telemetry after local verifier success.
- Current read/log/trace/data/operation/computer paths remain untouched.
- All new eval fields and docs distinguish export compatibility from
  functional correctness.
