# Write Mode Root-Cause Closure And SWE-bench Recovery Plan

Date: 2026-06-18
Branch: main
Status: active delivery plan

## Inputs Reviewed

- `/Users/han/opt/cc_like.md`
- `docs/design/write_mode_claude_code_online_convergence_architecture_20260617.md`
- `docs/design/swebench_manual_audit_20260618.md`
- `docs/design/swebench_historical_patch_audit_20260618.md`
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

The latest 137-instance historical audit changed the quality picture again.
The new report is a conservative oracle-assisted post-hoc patch audit over the
latest local run per instance. It is not an official SWE-bench score and does
not feed oracle data back into Codrax.

| Signal | Count | Rate | Interpretation |
| --- | ---: | ---: | --- |
| Non-empty patch | 130 / 137 | 94.9% | Export works; not correctness. |
| Current local-acceptance fields | 0 / 137 | 0.0% | Historical rows are not evaluable under the current local acceptance schema. |
| Oracle-assisted theoretical pass | 60 / 137 | 43.8% | Patch touches the expected source surface with strong similarity or typed verify overlap. |
| Oracle-assisted theoretical fail | 43 / 137 | 31.4% | Empty, wrong source surface, weak semantic overlap, or typed verify failed. |
| Unknown | 34 / 137 | 24.8% | Needs official harness/deeper execution/manual semantic review. |

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
4. **Verification can prove too little, and old final answers are not typed.**
   Local project environments are often partial. Passing a narrow probe or weak
   local check can be useful evidence, but it cannot prove owner-boundary
   correctness without changed-symbol/contract/dependent coverage. Historical
   SWE artifacts have `codrax.out` logs but no stable typed final-answer
   artifact, so answer-quality audit is log-level spot checking rather than a
   durable contract.
5. **Convention learning is present but still advisory.**
   The graph is inspectable and evidence-backed, which is good, but it has not
   yet become a stable contributor to patch review and replan priorities.

### Priority Update: Shared Localization Owner Anchors

The upstream "read/write shared typed localization owner/evidence anchor"
gap is a P0 prerequisite for every later SWE correctness improvement because
wrong source-owner localization makes verifier, patch critic, and impact
analysis reason over the wrong surface.

This gap is partially implemented by RC-97 / RC-98 / RC-104, but it is not
fully closed as a scheduling authority:

- RC-97 introduced `SourceLocalizationAnchor` as a shared read/write typed
  artifact, distinguishing read-file observation from grounded owner/support
  evidence and projecting those anchors into write context packs.
- RC-98 filtered broad deterministic evidence so `concrete_value` and similar
  rows remain available as evidence but cannot satisfy owner localization
  unless they carry typed defining authority.
- RC-104 preserves prior owner/supporting/scope anchors through write plan
  localization review and downstream context packs.

The remaining P0 gap is now explicitly queued as RC-107: read and write need
one shared typed owner/evidence-anchor authority that influences exploration,
extraction, planning, replan repair slices, and final answer/report projection
before broader target-file hints. This must be solved through typed artifacts
and deterministic consumers, not user-keyword or model-prose routing.

Current sorted queue:

1. **RC-107: shared owner-anchor scheduling authority.** Promote typed
   localization owner/evidence anchors into read/write scheduling, repair, and
   report projection so planning avoids wrong source surfaces earlier.
2. **RC-109: verification environment/probe unavailable authority.** Source
   checkout build-extension import errors and unavailable generated make targets
   should remain typed environment/probe-unavailable evidence, not source-code
   failure loops or local hard blockers when the patch effect is otherwise
   coherent.
3. **Broader SWE re-run and audit.** Continue Lite spot runs after each system
   fix, keeping official harness compatibility separate from functional
   correctness.

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

## 2026-06-18 RC-37 Schema-driven Tool JSON Object-fragment Repair

The SWE-bench logs repeatedly showed `emit_change_plan` failures with
`strict_decode_remap] string-carrier field "changes" kind=array`. Inspection of
real planner params showed a provider/tool-call serialization artifact:

```json
"changes": "[{\"path\":\"a.py\", ...}, \"path\":\"b.py\", ...}]"
```

The second object lost its opening `{`. Existing tolerance could repair normal
JSON-stringified arrays and answer-document-specific object fragments, but it
was not generalized to write-plan object arrays. Asking the model to retry burns
planning rounds and violates the desired Claude Code-like principle: the
deterministic harness should absorb mechanical transport mistakes when the
schema proves the intended structure.

RC-37 moves this into `internal/toolparam`:

```text
tool-call params + live JSON schema
  -> array field whose item schema is object-shaped
  -> stringified array decode fails
  -> delimiter scanner sees a top-level array item starting with a declared
     item property key followed by colon
  -> insert missing object opener
  -> JSON decode succeeds
  -> existing recursive schema normalizer fixes nested carriers
```

Design constraints:

- The repair is schema-driven. It inserts an object opener only when the next
  token is one of `items.properties` for that array's declared object schema.
- It never reads user request keywords, issue text, model summary/rationale,
  `<think>`, or stdout prose.
- It never invents field values, drops unknown fields, fills missing required
  fields, or relaxes `emit_change_plan` validators.
- The repaired candidate must pass normal JSON decoding before it is accepted.
- Nested fields still flow through the existing recursive normalizer, so
  examples like `depends_on:"[\"a.py\"]"` become native arrays without a
  tool-specific path.

RC-37 tasks:

- [x] Add schema-key-driven object-fragment repair for JSON-stringified arrays
  in `internal/toolparam`.
- [x] Keep the answer-document legacy repair path intact, but make write-plan
  arrays rely on the shared schema normalizer.
- [x] Add normalizer regressions proving object fragments are repaired only
  when the fragment key is declared by the item schema.
- [x] Add `emit_change_plan` regression using the real SWE-style
  `changes:"[{...}, \"path\":...}]"` shape.
- [x] Update architecture/user docs to clarify that `tool_param_compat` also
  covers schema-proven object-fragment arrays.
- [x] Run focused tool/toolparam tests, full regression, diff check, commit, and
  push.

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

## RC-42: Source-Check Provider Registry

### Gap

RC-40 and RC-41 deliberately expanded non-Python source verification, but the
implementation still left three parallel routing surfaces:

```text
syntaxCheckExtensions(runner)
sourceCheckExtensionsForNoTestWork(runner)
runSyntaxCheckFallback(runner)
```

That shape invites future case-by-case drift: a new language can be added to
one switch but missed in another, or a no-test compile fallback can accidentally
be enabled as a pre-project-test blocker. The capability is now important enough
to be represented as a typed registry, similar to the verification-probe runtime
and actual-diff source-shape provider registries.

### Design

- Introduce a deterministic `sourceCheckProvider` registry with:
  - `runner`;
  - ordinary pre-project-test extensions;
  - no-test-work fallback extensions;
  - whether the provider may run before the normal project runner;
  - the provider execution function.
- Keep hard routing structural:
  - runner enum from `TestSurface` / `run_tests` params;
  - `ChangePlan.target_paths` extension filtering;
  - compiler/parser output.
- Preserve existing behavior:
  - Python, JS, Ruby still run before project tests;
  - Go remains no-test only because `go test` already compiles during normal
    project verification;
  - TypeScript remains Node no-test only through the Node provider.
- Add drift tests so registry rows, extension surfaces, and dispatcher behavior
  stay coherent.

### Tasks

- [x] Replace parallel source-check switches with a single provider registry.
- [x] Preserve Python/Node/Ruby/Go behavior and TypeScript no-test-only
  semantics.
- [x] Add provider-registry drift tests for uniqueness and dispatch coverage.
- [x] Run focused/source-check regression, full regression, SWE smoke, and diff
  check.

### Verification

- `go test ./internal/tool -run 'Test(SourceCheckProviderRegistry|SyntaxCheckExtensions|RunNodeCheckFallback_TypeScript|RunGoCompileFallback|RunTestsEmptyParams(NodeTypeScript|GoNoTests))' -count=1`
- `go test ./internal/tool -count=1`
- `go test ./...`
- `make test`
- `eval/swebench/smoke_local.sh`
- `git diff --check`

## 2026-06-18 RC-43 Impact Selector Language Coverage Design

RC-39 closed JS/TS related-test targeting, and RC-40/41/42 closed source-check
fallback drift. The remaining executor gap is that typed
`ImpactVerificationTarget{kind=test_surface, related_path=...}` still behaves
like a Python/Go/Node-first feature even though `run_tests` already supports
Java, Ruby, Rust, Swift, and other runners. This weakens the cc_like
`Execute -> Observe -> Repair` loop for non-Python repositories: the controller
can know "changed A, related test B exists", but verifier default `{}` may fall
back to a broader suite or a less relevant runner instead of the smallest typed
proof surface.

RC-43 expands only deterministic path-to-selector conversion for existing
runners:

```text
ImpactAnalysis related_path
  -> safe repo-relative path resolver
  -> TestSurface candidate with typed test signal
  -> runner-native selector
  -> priority runnerPlan before default suite
```

Supported additions:

- Java Maven/Gradle: `src/test/java/com/acme/FooTest.java` becomes
  `com.acme.FooTest`; Kotlin Gradle test files under `src/test/kotlin` use the
  same class selector through the existing Java runner lane.
- Rust: top-level integration tests `tests/foo.rs` become
  `cargo test --test foo`; inline `#[cfg(test)]` module selection stays
  unsupported until a typed Rust symbol/test index exists.
- Swift Package Manager: `Tests/.../FooTests.swift` becomes
  `swift test --filter FooTests`.
- Ruby remains file-path based; Python/Go/Node behavior is preserved.

This is not a prompt or keyword route. The conversion consumes only
repo-relative paths, runner candidates, and file extensions. Unsupported shapes
return no priority plan and continue through existing TestSurface fallback and
handoff telemetry.

RC-43 tasks:

- [x] Extend Impact related-test priority plans to Java, Kotlin-on-Gradle,
  Rust integration tests, and Swift Package tests.
- [x] Normalize Java/Kotlin path selectors before Maven/Gradle command
  construction.
- [x] Normalize Rust integration-test path selectors to `cargo test --test`.
- [x] Normalize Swift test-file path selectors to `swift test --filter`.
- [x] Add focused regressions proving priority queue order and command
  rendering.

## 2026-06-18 RC-44 Verify Source-Compile Fallback Reuse Design

RC-43 widened the targeted test selector path. The next observed executor gap
is the no-test branch: plan emit already has dry-build checks for Java, Kotlin,
Swift, and Rust, but verify could still treat Java/Swift source changes with no
test files as synthetic no-tests evidence. That is a weaker online convergence
signal than the system already knows how to produce.

RC-44 reuses the existing compile-check shape in verify-time source-check
providers:

```text
no test work for runner
  -> plan-touched source extension set
  -> source-check provider
  -> build diagnostics with parseable file/line evidence
  -> failed verify only when attributable
```

Policy boundary:

- Missing tools, missing manifests, or compile output without parseable source
  diagnostics remain pass-with-warning/unavailable. Customer environments often
  lack full toolchains, and delivery should not hard-block on that alone.
- Parseable build errors become `FailureKindBuildFailure` with structured
  `BuildErrors[]`, so changed-line scoping and P2 handoff continue to work.
- The provider dispatch consumes only runner id, repo-relative target paths,
  manifest files, tool availability, and parser output. No user intent keyword
  or model prose controls routing.

RC-44 tasks:

- [x] Share Java Maven/Gradle compile command selection between plan dry-build
  and verify source fallback.
- [x] Register Java/Kotlin no-test source fallback in the source-check provider
  registry.
- [x] Register Swift Package no-test source fallback in the provider registry.
- [x] Preserve environment-missing outcomes as pass-with-warning rather than
  hard blockers.
- [x] Add focused regressions for Java compile fallback success/failure, Swift
  build fallback, and provider registry drift.

## 2026-06-18 RC-45 User Guide Language-Coverage Sync

RC-43 and RC-44 expanded implementation coverage, but the user-facing guide
still described the older Python/Node/Ruby/Go source-check list. That creates
avoidable usage friction: users can reasonably read the docs as saying
Java/Kotlin/Swift source changes remain synthetic no-tests paths, or that the
actual-diff "dynamic mapping boundary" family is Python-only.

RC-45 is a documentation-only commercial UX closure:

- Update `docs/user_guide.md` and `docs/user_guide.html` so source compile
  fallback lists Java/Kotlin Maven/Gradle compile, bounded `kotlinc`, and Swift
  Package `swift build --skip-build` alongside Python, Node, Ruby, and Go.
- Explicitly document that actual-diff mapping/container boundary signals are
  language-provider events covering Python, JS/TS, Ruby, Java/Kotlin, and Go.
- Preserve the hard/soft boundary: these docs describe typed implementation
  behavior only; no prompt routing, user-intent keyword matching, or model
  prose parsing is introduced.
- Keep read/log/trace/data/operation/computer surfaces untouched.

## 2026-06-18 RC-46 Java Verification Probe Runtime

RC-45 clarified that source-check and actual-diff coverage are multi-language,
but the bounded runtime proof lane still had a concrete gap: `verification_probes[]`
could run Python, JavaScript, Ruby, and Go, while Java/Kotlin source changes had
only compile-level fallback and project runner coverage. That leaves a common
customer scenario under-proved: a Java bug report may have no runnable JUnit
suite locally, but a tiny behavior assertion against the changed class is still
valuable online evidence.

RC-46 adds Java to the same typed verification-probe registry rather than adding
a prompt branch:

```text
ChangePlan.verification_probes[].language=java
  -> registry enum / alias javac
  -> failure-signal validator
  -> temporary CodraxVerificationProbe.java
  -> javac compile with repo/source/classpath candidates
  -> java -ea execution
  -> ChangeReport TestResult + ExecutedCommands
```

Commercial constraints:

- Java probe code may be a main-method body or a full `CodraxVerificationProbe`
  class. Imports are preserved when wrapping body code.
- Missing JDK remains `runner_missing/unavailable`, not a product-code failure.
  Compile diagnostics from the temporary probe itself are `parser_error`, so a
  broken probe does not trigger incorrect product-code repair.
- Same-language production Java changes now participate in the copied-probe
  hard gate: a Java probe must import or type-reference the changed production
  class, including static imports, instead of testing a copied local expression.
- Hard routing consumes only typed probe language enums, repo-relative paths,
  Java package/import declarations, executable failure signals, and process exit
  status. No user-intent keywords or model prose affect the decision.

RC-46 tasks:

- [x] Register Java/JDK in the verification-probe runtime provider matrix and
  schema enums.
- [x] Add Java executable failure-signal validation.
- [x] Execute Java probes through bounded `javac` + `java -ea` with temp source
  cleanup and structured command evidence.
- [x] Map Java compile failure, missing JDK, timeout/OOM/CPU outcomes to typed
  `ChangeReport` failure kinds.
- [x] Extend copied-implementation coupling to Java production classes,
  package imports, wildcard imports, static imports, and simple type refs.
- [x] Update planner soft guidance, Markdown/HTML user docs, and this ledger.
- [x] Add focused tests for schema/runtime registry, fake JDK execution,
  compile parser_error, and Java coupling.

## 2026-06-18 RC-47 Owner-Boundary Runtime Signals Design

RC-45/RC-46 closed the "is actual-diff dynamic mapping Python-only?" question:
mapping/container boundary events are already provider-backed for Python,
JS/TS, Ruby, Java/Kotlin, and Go. The remaining owner-boundary gap is different:
the SWE-bench adapter still owns a Python-only audit helper for
caller-return adapters, conditionally suppressed diagnostics, and external
private-state synchronization. Those signals currently downgrade eval
confidence after export, but they cannot help live write mode re-explore or
replan because they are not emitted as runtime `PatchEffectEvent` records.

RC-47 moves this class of evidence into the same actual-diff provider pipeline:

```text
actual unified diff + post-apply file bytes
  -> language source-shape provider
  -> owner-boundary/workaround hunk signal
  -> PatchEffectEvent
  -> PatchReview semantic coverage unknown
  -> P2 context pack + bounded impact repair queue
```

Design constraints:

- signals are generated only from typed artifacts: repo-relative paths, actual
  diff hunk line text, post-apply source bytes, source path role, and provider
  registry metadata;
- no user request keywords, issue prose, model rationale, `<think>` text, or
  manual audit notes drive control flow;
- findings are soft semantic-coverage unknowns, not apply blockers. They should
  request proof/replan while budget remains, but they must not increase routine
  approval friction;
- event codes are generic enough to be consumed by controller and adapter
  without language branches, while provider rules remain language-aware where
  syntax differs;
- the SWE-bench adapter may continue exporting its audit fields for historical
  comparison, but runtime PatchReview becomes the authoritative producer for
  live write-mode scheduling.

RC-47 tasks:

- [x] Add provider-backed hunk signals for caller-return wrapper adapters,
  newly guarded diagnostic calls, and external private-state assignment
  workarounds.
- [x] Cover Python plus analogous JS/TS, Ruby, Java/Kotlin syntax shapes where
  the source-code signal is precise enough; skip languages whose local syntax
  cannot be identified without noisy inference.
- [x] Register the new event codes as PatchReview soft semantic-coverage
  unknowns so they flow into context packs and bounded repair scheduling.
- [x] Add focused actual-diff tests proving the findings are generated from
  source hunks and remain non-hard-blocking.
- [x] Update this ledger with verification evidence and keep the eval adapter's
  historical fields clearly labeled as audit telemetry.

## 2026-06-18 RC-48 Plan Localization Retry Design

The controller already persists `plan_context_coverage` items that compare the
source files a `ChangePlan` edits against prior typed localization context from
write analysis and exploration. That evidence currently behaves mostly as
telemetry for recovery/audit/SWE-bench adapter reporting. It does not
consistently give the planner one immediate chance to correct a plan that edits
source files outside the evidence-backed localization set.

This is a root-cause gap, not a prompt wording gap. Several manual-audit
failures were not empty patches or parser failures; they were plausible
symptom-site fixes where the plan skipped the owner boundary that exploration
had already hinted at. The right generalized response is to make localization
coverage a typed planning observation:

```text
prior WriteContextPack localization paths
  + current ChangePlan source paths
  -> plan_context_coverage
  -> one bounded planner retry when source paths are missing prior localization
  -> persist the same coverage context for controller/planner/verifier
```

Design constraints:

- this is not a hard gate. Legitimate newly discovered files can still proceed
  after one retry, and high-risk approval remains governed by the existing risk
  engine;
- routing consumes only typed context items and ChangePlan paths. It must not
  parse issue text, model rationale, manual audit notes, stdout prose, or
  `<think>`;
- test-only paths are excluded from the localization miss calculation, matching
  the existing adapter and context-pack semantics;
- the retry hint should be precise and short: preserve evidence-backed paths,
  explain any new source path with direct read/repomap evidence, or split the
  new owner investigation into a later batch;
- existing read/log/trace/data/operation/computer paths remain untouched.

RC-48 tasks:

- [x] Add a controller-side helper that derives missing source localization
  paths from `WriteContextPackFromPlanContextCoverage`.
- [x] Invoke it in `runControllerPlanBatch` as one bounded retry before accepting
  a fresh low/medium-risk plan.
- [x] Scope it to existing workflow/context packs and exclude the current plan's
  self-generated context so it cannot satisfy itself.
- [x] Add regressions for missing localization retry, covered source plan
  acceptance, no-prior-context no retry, and test-only path exclusion.
- [x] Update this ledger and run focused/full regression before commit/push.

## 2026-06-18 RC-49 Official Harness Dry-Run Import Check Design

The RC-48 targeted SWE-bench smoke produced three non-empty predictions and a
dry-run official harness command, but a follow-up import check exposed an
evaluation-environment gap: the local `eval/results/swebench/.venv` was Python
3.9, while the installed `swebench` package uses syntax that requires Python
3.10+. `run_official_harness.sh` only checked `importlib.util.find_spec`, so a
dry-run could claim "official harness command" even though the selected Python
could not actually import `swebench.harness.run_evaluation`.

This is not a write-mode correctness bug, but it directly affects the
"predictions + official harness consumable" acceptance boundary. The adapter
should distinguish three facts:

```text
prediction JSONL validates
  != official harness module is importable
  != official resolved/total was executed
```

RC-49 tightens the eval harness wrapper without making the dependency-free
local fake smoke heavier:

- actual non-dry official runs must import `swebench.harness.run_evaluation`,
  not merely locate a package name;
- Lite dry-runs should check harness import by default so "can consume" means
  the configured Python can load the official entrypoint;
- dependency-free local smoke can opt out and remain a command-shape smoke;
- error messages must name the Python executable/version and point at the
  documented Python 3.10+ venv setup instead of failing with a later TypeError.

RC-49 tasks:

- [x] Add a reusable import check in `run_official_harness.sh` for
  `swebench.harness.run_evaluation`.
- [x] Keep `DRY_RUN=1` command-only behavior available, but make
  `smoke_lite.sh` enable the import check by default.
- [x] Update SWE-bench README with the stronger dry-run semantics and Python
  version caveat.
- [x] Validate the wrapper with the existing command-only path and a Python
  3.11 SWE-bench venv.
- [x] Record the targeted RC-48 SWE smoke artifacts and findings in this ledger.

RC-48 targeted SWE-bench smoke artifacts:

- Workdir: `/private/tmp/codrax-swe-rc48-20260618-095217`
- Predictions: `/private/tmp/codrax-swe-rc48-20260618-095217/predictions.jsonl`
- Results: `/private/tmp/codrax-swe-rc48-20260618-095217/results.jsonl`
- Instances: `django__django-11742`, `mwaskom__seaborn-3190`,
  `pydata__xarray-4248`
- Export result: 3/3 non-empty predictions, `validate_predictions.py` passed,
  and RC-49 Python 3.11 import-check dry-run produced an official harness
  command for the same predictions.

Manual audit notes from gold-patch comparison:

- `django__django-11742`: Codrax exported a source patch and even appended an
  impact-repair batch, but local verification was unavailable with
  `parser_error/make_target_missing`. The patch adds a separate
  `_check_max_length_for_choices()` and uses error id `fields.E180`, while the
  gold fix integrates with `_check_choices()` and id `fields.E009`. Treat as
  still failed/blocked, not pass.
- `mwaskom__seaborn-3190`: Codrax patch is plausibly close for NumPy boolean
  extrema but narrower than the gold `map(float, axis.convert_units(...))`
  normalization. Local verify passed only as low confidence because probe
  contract refs were soft/missing. Treat as unknown/low-confidence until
  official harness.
- `pydata__xarray-4248`: Codrax added `attrs["units"]` formatting, while the
  gold fix routes through `_data._repr_inline_()` so array-like unit wrappers
  can render themselves. Treat as likely wrong-owner/incomplete despite local
  low-confidence verify.

System implication: RC-47/RC-48 improved observability and online repair
behavior, but local correctness still depends on stronger behavior-contract
proof. The next root-cause batches should target contract/probe generation and
owner-boundary proof rather than patch export.

## RC-50: Verification Proof Obligation Follow-up

Targeted RC-48 smoke exposed a recurring low-confidence shape:

- the workflow can produce a non-empty official patch and a local passed report;
- the report still carries typed `verification_confidence[]` records such as
  `verification_probe_missing_soft_contract_ref`;
- the SWE adapter correctly downgrades confidence, but the controller does not
  yet treat the missing proof as an online convergence obligation.

This is a system gap, not an adapter scoring issue. In Claude-Code-like online
convergence terms, the loop observes "the patch ran, but the proof is
incomplete" and should schedule a bounded `Edit/Run/Observe` follow-up that
adds or tightens proof near the actual change. Leaving the signal as passive
telemetry makes SWE-bench manual audit depend on lucky probes instead of a
durable proof-closure mechanism.

Design constraints:

- Hard logic must consume only typed `ChangeReport.VerificationConfidence`
  records and typed plan fields. No user-intent keywords, no model rationale,
  no prose summary parsing.
- Missing proof is not the same as a failing behavior. It must append one
  bounded proof follow-up before finish, not mark the current patch failed or
  trigger unbounded replan.
- Existing impact/patch-review follow-up stays authoritative for actual-diff
  and changed-symbol coverage. RC-50 extends the same queue with
  verification-confidence proof obligations instead of creating a parallel
  scheduler.
- The follow-up must preserve P2 handoff evidence: reason code, contract refs,
  changed-symbol refs, source, and current batch context all travel through
  `SuccessCriteria`.
- Recursion must be impossible: once a proof-obligation follow-up has been
  requested in the run ledger, the controller finishes with low-confidence
  caveats instead of appending another proof batch.

Implementation plan:

- Add `verificationConfidenceRepairQueueItem` projection into the existing
  impact repair queue.
- Consume these categories when status is `missing`:
  - `probe_soft_contract_refs` with concrete `contract_refs`;
  - `probe_contract_refs` with concrete hard `contract_refs`;
  - `probe_changed_symbol` with concrete `changed_symbol_refs` when present.
- Map them to typed repair kinds `behavior_contract` or `changed_symbol`, with
  `source=verification_confidence` and criteria preserving the original
  `reason_code`.
- Scope expected paths from plan-changed source paths when no more precise
  related path exists, so the follow-up remains small and normally does not
  require extra exploration.
- Extend the one-shot recursion guard to cover both impact and proof follow-up
  progress reasons.
- Add focused controller tests:
  - passed/unverified batch plus missing soft contract ref appends one proof
    follow-up batch;
  - criteria include the reason code and missing contract ref;
  - missing proof records without refs stay as telemetry and do not append
    blind batches;
  - a prior proof follow-up reason prevents recursion.
- Run focused orchestrator tests, related write packages, full Go regression,
  and diff check.

## RC-51: Durable Proof Criteria And Probe Ref Binding

RC-50 closed the controller scheduling gap: a passed batch with missing typed
proof now appends a proof follow-up. The RC-50 three-instance Lite smoke then
showed the next system gap:

- `pydata__xarray-4248` appended `batch-1-proof-repair` from
  `verification_proof_followup`, applied and verified that proof batch, and
  exported a harness-consumable patch.
- The final `ChangeReport` still carried
  `verification_probe_missing_soft_contract_ref`.
- Manual plan/report inspection showed the proof-repair plan's
  `verification_probes[]` still omitted `contract_refs`, even though the
  controller-generated proof criteria contained the missing refs.
- After proof-repair verify failed once, the replan prompt retained the failure
  summary but not the original typed proof obligation list as durable batch
  state.

This is not an Xarray-specific problem. It is a durable handoff gap between
controller-observed proof obligations and planner-emitted probe metadata:
proof obligations currently exist as one-round planning text, not as durable
batch fields that downstream deterministic code can consume.

Design:

- Extend `types.WriteWorkflowBatch` with durable controller-owned fields:
  `purpose`, `expected_paths`, and `success_criteria`.
- When controller appends/splits/replans batches, persist these fields from the
  typed `writeflow.WriteBatchPlan`. Resume and replan must therefore retain the
  same proof obligations across failed proof attempts.
- Add deterministic proof-follow-up probe-ref binding after plan emit:
  - authorized only when the active durable batch has
    `purpose=verification_proof_followup` or
    `purpose=impact_and_verification_proof_followup`, and the run ledger
    contains `verification_proof_followup_requested`;
  - reads only the controller-owned `success_criteria` rows generated by RC-50
    and the plan's typed `BehaviorContracts`;
  - extracts concrete `contract_ref=` tokens whose IDs exist in the plan
    behavior contracts;
  - if the proof-follow-up plan has exactly one verification probe, append
    missing refs to that probe's `contract_refs`;
  - if the probe lacks `changed_symbol_refs`, reuse the existing conservative
    `path:<single-production-target>` fallback only when exactly one
    production target path exists.
- Ordinary multi-contract plans keep the existing red line: the tool layer
  still does not guess refs for multiple contracts outside controller-owned
  proof-follow-up batches.
- Persist the enriched plan snapshot before approval/apply so verifier,
  workflow store, adapter export, and manual audit all consume the same typed
  artifact.

RC-51 tasks:

- [x] Add durable batch `purpose`, `expected_paths`, and `success_criteria`
  fields plus normalization tests.
- [x] Persist those fields when workflow batches are created/updated from
  `WriteBatchPlan`.
- [x] Add proof-follow-up ref extraction from durable batch criteria.
- [x] Add post-plan enrichment for authorized proof-follow-up plans.
- [x] Prove ordinary multi-probe plan behavior remains unchanged.
- [x] Add focused controller tests for first proof plan and failed-proof replan.
- [x] Run focused/related/full Go tests, SWE local smoke, and a small Lite
  rerun evidence check.

Implementation:

- `types.WriteWorkflowBatch` now persists `purpose`, `expected_paths`, and
  `success_criteria`; normalization trims and dedupes them.
- `writeflow.ApplyWorkflowDecisionToRun` stores those fields for
  `plan_batch`, `append_batch`, `split_batch`, and `replan_batch`.
- `HydrateWriteWorkflowDecisionFromRun` restores them from the durable active
  batch when a later controller action only supplies the batch ID.
- `runControllerPlanBatch` enriches exactly one verification probe for
  authorized proof-follow-up batches by reading controller-owned
  `success_criteria` rows and validating `contract_ref=` IDs against typed
  `BehaviorContracts`.
- Multi-probe plans and plans without the proof-follow-up ledger reason remain
  unchanged.

Verification:

- Focused tests:
  `TestEnrichProofFollowupPlanProbeRefsBindsDurableContractRefs`,
  `TestEnrichProofFollowupPlanProbeRefsRequiresAuthorizedDurableBatch`,
  `TestEnrichProofFollowupPlanProbeRefsDoesNotGuessAcrossMultipleProbes`.
- Related tests:
  `go test ./internal/orchestrator ./internal/types ./internal/writeflow ./internal/tool -count=1`.
- Full regression:
  `go test ./...`, `make test`, `git diff --check`, and
  `eval/swebench/smoke_local.sh`.
- Lite rerun:
  `pydata__xarray-4248` at
  `/private/tmp/codrax-swe-rc51-xarray-20260618-110606` generated a non-empty
  prediction, passed prediction validation, and produced an official
  harness-consumable command. The run did not re-enter the proof-follow-up path
  because online replan found a directly passing batch; focused tests cover the
  proof-follow-up binding itself.

## RC-52: Dispatch-Interrupted Terminalization

The RC-51 Lite rerun exposed a separate user-facing state gap: after online
replan, `pydata__xarray-4248` reached a typed passed verifier result and
exported a coherent patch, but the workflow persisted as `in_progress` because
the final controller dispatch was interrupted after the batch had already
recorded a terminal verify verdict.

This is a state-machine/UX gap, not a Python-specific one. A completed batch
should not require another successful model turn merely to write the run-level
terminal state when deterministic typed artifacts already prove completion.

Design:

- On controller dispatch cancellation/interruption, first check whether all
  workflow batches already have terminal typed statuses.
- Run the existing typed finish normalization on a cloned run so pending
  deterministic follow-ups, such as proof/impact repair batches, can still
  block terminalization.
- If the normalized action remains `finish`, complete the run through the same
  completion aggregator used by budget terminalization and write an explicit
  progress reason `controller_dispatch_interrupted_after_complete`.
- If the cloned normalization would append a follow-up batch, leave the real
  run unchanged and keep the existing applied-patch-interrupted guidance.

RC-52 tasks:

- [x] Add dispatch-interrupted terminalization helper.
- [x] Wire it before applied-patch-interrupted guidance on cancellation.
- [x] Add regression proving verified completed runs finish without another
  model turn.
- [x] Add regression proving pending typed proof follow-up is not swallowed.

## RC-53: Local Acceptance Confidence Boundary

The post-RC-52 aggregate audit over 137 latest SWE-bench result rows showed
that `local_acceptance_verdict=pass/source=local_verify` could still be emitted
for `prediction_verdict=predicted_passed_low_confidence`. The concrete
RC-51 `pydata__xarray-4248` rerun had:

- `verify_status=passed`;
- `prediction_verdict=predicted_passed_low_confidence`;
- `prediction_local_confidence=unknown`;
- `prediction_confidence_downgrade_reason=patch_review_semantic_unverified:...`;
- `local_acceptance_verdict=pass`.

That muddles the user's requested pass-rate denominator. A local verifier pass
with typed confidence downgrades is valuable evidence, but it is not the same
as an authoritative local pass. Otherwise dashboards that combine
"authoritative local verify passed + typed manual audit passed" overstate
correctness and hide the reason manual audit is still needed.

Design:

- Official SWE-bench prediction export remains unchanged.
- `prediction_verdict` and `prediction_local_confidence` remain the primary
  typed confidence fields.
- The internal `local_acceptance_verdict` proxy counts `local_verify` pass only
  when `verify_status=passed` and there is no
  `prediction_confidence_downgrade_reason`.
- A typed manual audit `pass` may accept a low-confidence local verify pass,
  but cannot override failed local verification or hard local audit blockers.
- Free-form manual notes remain audit-only; no natural-language text drives
  acceptance.

RC-53 tasks:

- [x] Thread `confidence_downgrade_reason` into
  `local_acceptance_verdict`.
- [x] Keep high-confidence local verifier pass unchanged.
- [x] Add tests that low-confidence verifier passes stay `unknown` without
  manual audit.
- [x] Add tests that explicit manual pass can accept low-confidence verifier
  evidence but still cannot override hard blockers/failed verify.
- [x] Update SWE-bench README so dashboards do not call missing manual audit
  rows a manual pass rate.

## RC-54: Codrax Results Summary Denominator Guard

The same aggregate audit also showed that many "latest 137" rows came from
older smoke runs with missing current fields. A one-off grep or ad hoc Python
script can therefore mix:

- current rows where `prediction_local_confidence` and
  `local_acceptance_verdict/source` are meaningful;
- old-schema rows where those fields are absent;
- official prediction export shape;
- high-confidence local verifier pass;
- low-confidence verifier pass;
- typed manual audit pass/fail/unknown.

That is a system-level eval observability gap. It does not change write-mode
runtime, but it directly affects the user's pass-rate question and can lead
engineers to optimize the wrong layer.

Design:

- Add a dependency-free Codrax `results.jsonl` summarizer separate from the
  official harness summarizer.
- Consume only typed adapter fields; do not parse issue text, model prose,
  terminal logs, or manual notes.
- Report separate denominators for:
  - non-empty patch;
  - high-confidence local verifier pass;
  - low-confidence verifier pass;
  - local acceptance pass/fail/unknown;
  - typed manual audit recorded/pass/fail/unknown;
  - local audit blockers;
  - recorded local-verify pass rows that fail the current high-confidence
    boundary;
  - missing current core fields.
- Keep official `resolved/*` metrics in `summarize_official_results.py`.

RC-54 tasks:

- [x] Add `eval/swebench/summarize_codrax_results.py`.
- [x] Add unit tests for confidence/manual/schema-missing summaries and CLI
  JSON output.
- [x] Update SWE-bench README with the local summary command and denominator
  warnings.

## RC-55: Multi-Run Codrax Result Summary

RC-54 covers one `results.jsonl`, but the user's 137-instance question spans
many historical smoke directories. Without first-class multi-file support,
engineers still need ad hoc scripts to choose the latest row per instance and
will keep mixing old and new schema rows.

Design:

- Allow `summarize_codrax_results.py` to read multiple `--results-jsonl` files
  and/or `--results-glob` patterns.
- Preserve source path, line, and file mtime as audit metadata for each row.
- Add explicit `--dedupe latest-by-file-mtime` for "latest per instance"
  summaries. This is an audit/reporting choice, not a hidden default.
- Keep `--dedupe none` as the default so raw run summaries remain literal.
- Surface `input_row_count`, `input_results_paths`, `dedupe_mode`, duplicate
  IDs, and source locations for rows missing current core fields.

RC-55 tasks:

- [x] Add multi-file/glob loading.
- [x] Add latest-by-file-mtime instance de-duplication.
- [x] Add tests for dedupe selection and CLI multi-file summary.
- [x] Update SWE-bench README with the multi-run command.

## RC-56: Primary Failure Reason Authority

The RC-55 multi-run summary made one telemetry defect visible. Some Django smoke
rows had real Python unittest failures, but the aggregate
`FailureReasonCode`/SWE confidence reason surfaced `make_target_missing`
because a secondary docs/extras Makefile candidate also reported an unavailable
target. That misclassified the root cause as environment/test-surface
availability even though the authoritative verifier verdict was red tests.

This is a system-level observation-authority gap, not a Django-specific case:
`mergeChangeReports` already promotes the primary `FailureKind` by severity, but
it used every child report's reason code as the aggregate primary reason. That
lets low-authority unavailable runner noise overwrite the reason consumed by
controller handoff, REPL status, and SWE-bench local confidence.

Design:

- Keep all child evidence in `FailureSummary`, `TestResults`,
  `NoTestsRunners`, and verification diagnostics.
- Bind aggregate `FailureReasonCode` to child reports whose
  `FailureKind` matches the final aggregate `FailureKind`.
- If the final aggregate has no typed failure kind, preserve the previous
  all-reason fallback for legacy reports.
- Do not parse terminal prose, issue text, user intent, or model rationale.
  The gate consumes only typed `FailureKind`, `FailureReasonCode`, and
  executed-command reason codes.

RC-56 tasks:

- [x] Add a typed primary-reason helper for `mergeChangeReports`.
- [x] Add regressions proving unavailable Makefile reasons do not become the
  primary reason when red tests are the aggregate failure.
- [x] Preserve single parser-error reason backfill behavior.
- [x] Update this progress ledger.

## RC-57: Typed SWE Failure-Cause Taxonomy

After RC-56, the local results summary still required humans to read raw reason
strings and infer whether low manual/audit pass rate came from localization
mistakes, proof metadata gaps, actual-diff semantic gaps, local environment
limits, workflow state, or export issues. That slows the feedback loop and
invites case-by-case diagnosis.

This is an observability gap rather than a runtime routing gap. The right fix is
not to parse issue text, terminal logs, or model rationale. The adapter already
emits typed fields (`prediction_verdict`, `verify_status`,
`verify_failure_kind`, local acceptance, audit reason codes, and confidence
reason codes). The summarizer should project those typed fields into a stable
triage taxonomy.

Design:

- Add per-row `result_cause_category`/family projection inside
  `summarize_codrax_results.py`.
- Prefer high-authority typed verdict fields over reason strings. For example,
  `verify_failure_kind=tests_failed` classifies as
  `verify_red_tests_or_build` even if an older row still carries a secondary
  `make_target_missing` confidence reason.
- Keep reason strings as enum-like evidence for examples/top reasons; do not
  inspect logs, summaries, issue text, model prose, or manual notes.
- Report category counts, family counts, top typed reasons, and small examples
  with source file/line metadata when available.

RC-57 tasks:

- [x] Add typed cause category/family projection to the Codrax results
  summarizer.
- [x] Add regressions for red-test-vs-environment precedence, proof gaps,
  actual-diff gaps, probe authoring gaps, accepted rows, manual audit rows, and
  empty-patch export.
- [x] Document the local triage taxonomy in the SWE-bench README.
- [x] Update this progress ledger.

## RC-58: Auto Approval Refresh After Deterministic Plan Enrichment

A fresh `pydata__xarray-4248` Lite run after RC-57 exercised the intended
online loop:

```
explore -> plan -> apply -> verify failed -> checkpoint restore -> replan
-> apply -> verify -> impact repair -> apply -> verify -> proof repair
```

The run then blocked with `approval_authority_invalid` and exported an empty
prediction even though earlier source-owner plans had applied and local verify
had passed. The active proof-repair plan was low/medium risk and had
`approval.action=auto_execute`, but controller-owned proof-ref binding appended
`verification_probes[].contract_refs`/`changed_symbol_refs` after the plan
post-hook had already stamped the approval fingerprint. Because
`PlanFingerprint` correctly includes verification probes, the approval record
became stale and Auto Pilot asked for approval in a non-interactive eval lane.

This is a system-level low-friction approval gap:

- The deterministic controller may mutate apply-relevant plan payload after
  planner emission.
- Those mutations must either run before approval stamping or refresh the
  auto approval record when the refreshed risk policy still allows
  auto-execute.
- High/manual/deny paths must remain conservative; stale manual approvals still
  require the user.

Design:

- Add a controller helper that refreshes stale `auto_execute` approvals after
  deterministic controller mutations.
- The helper consumes only structured plan payload, current approval record,
  typed risk assessment, and policy decision.
- It refuses missing/tampered/manual/denied approval records and refuses to
  upgrade any plan whose fresh policy decision is not `auto_execute`.
- Wire it into proof-follow-up probe-ref binding, the current mutation point
  that changes `verification_probes[]` after plan approval.

RC-58 tasks:

- [x] Add `refreshAutoApprovalAfterDeterministicPlanMutation`.
- [x] Refresh approval after proof-follow-up probe-ref enrichment.
- [x] Add regression proving proof-ref enrichment changes the plan fingerprint
  but leaves a valid `auto_allowed` approval view.
- [x] Re-run the xarray Lite instance to verify predictions export no longer
  becomes empty for this approval-staleness path.
- [x] Update this progress ledger.

## RC-59: Applied-But-Unobserved Dispatch Interruption Completion Verify

The RC-58 xarray rerun crossed the approval boundary and produced a non-empty
prediction, but the workflow still ended `in_progress` with:

```text
workflow_latest_progress_reason_code=controller_dispatch_interrupted_after_applied_patch
plan_status=applied_pending_verify
verify_status=<missing>
```

The active proof-repair batch had applied its patch, then the next
`write_controller` dispatch returned `context canceled` before the controller
could ask for `verify_batch`. This is an online-convergence state-kernel gap:
the system performed Edit but failed to deterministically run Observe before
surfacing the interruption. It is not a patch-export issue and should not be
fixed by special-casing the xarray instance.

Design:

- Reuse the existing budget-completion verify lane as a generic
  "applied-pending-observe" helper.
- Trigger the helper when controller dispatch is interrupted and typed workflow
  attempts show the active batch's latest successful apply has no later verify.
- Respect explicit user/global cancellation: if the orchestrator cancel token is
  set, return the interruption immediately and do not run extra work.
- If the completion verify passes or is typed unavailable/no-tests/runner-missing,
  terminalize the batch/run with the same typed completion semantics used by the
  budget lane.
- If the completion verify fails, persist normal P2 verify evidence and let the
  caller preserve the applied-patch interruption guidance for later resume.
- Keep hard logic on typed workflow attempts, typed reports, and cancel-token
  state only; do not inspect model prose, issue text, logs, or user keywords.

RC-59 tasks:

- [x] Factor `runBudgetCompletionVerify` through shared
  `runAppliedPendingCompletionVerify`.
- [x] Add `runDispatchInterruptedCompletionVerify` and call it before publishing
  applied-patch interruption guidance.
- [x] Add regression proving dispatch `context.Canceled` after apply runs one
  bounded verify and completes a green batch.
- [x] Add regression proving explicit cancel token does not run completion
  verify.
- [x] Update this progress ledger.

RC-59 smoke:

- Reran `pydata__xarray-4248` at
  `/private/tmp/codrax-swe-rc59-xarray-20260618-122015`.
- Prediction export stayed non-empty (`patch_bytes=626`) and official harness
  import/dry-run accepted the predictions file.
- The workflow no longer stopped at `applied_pending_verify` with missing
  `verify_status`; it completed with two typed verify attempts.
- New remaining gap: local verifier was unavailable (`parser_error` from
  `unittest` loader import errors and missing `make check`), while local audit
  blocked on `changed_symbol_without_probe_coverage`. The generated patch also
  showed planner synthesis risk around an existing formatting column variable,
  so the next fix must improve proof-follow-up scheduling rather than pretend
  non-empty export implies correctness.

## RC-60: Proof Coverage Follow-Up Must Require Typed Probes

The RC-59 smoke showed that `changed_symbol_without_probe_coverage` was being
queued as a generic impact repair. That made the next batch look like "repair
the code again" instead of "prove the changed symbol through a bounded typed
probe." This is a controller scheduling taxonomy gap:

- `changed_symbol_without_probe_coverage` and
  `behavior_contract_without_verify_coverage` are proof obligations even when
  they originate from `PatchReviewRecord`, not only when they originate from
  `VerificationConfidenceRecord`.
- A proof follow-up batch without `verification_probes[]` cannot close the
  missing proof class; applying it as a normal code plan risks blind extra
  edits when the local project runner is unavailable.
- The hard gate can be typed and generic: batch purpose
  `verification_proof_followup` / `impact_and_verification_proof_followup` plus
  `ChangePlan.VerificationProbes`, never user prose or model rationale.

Design:

- Reclassify typed proof-coverage PatchReview codes as proof follow-up queue
  items.
- Add `verification_probe_required=true` to proof success criteria so the
  planner sees a structured obligation.
- During proof follow-up planning, retry once with a bounded planning hint if
  the emitted plan lacks `verification_probes[]`; after that, fail loud instead
  of applying a code-only proof plan.
- Bind `symbol=` success criteria into the emitted probe's
  `changed_symbol_refs` when the probe omitted refs, preserving typed evidence
  priority for verifier coverage projection.
- Keep ordinary impact/effect follow-ups unchanged, and keep custom
  non-proof `ImpactKind=changed_symbol` events classified as impact repairs.

RC-60 tasks:

- [x] Treat `changed_symbol_without_probe_coverage` and
  `behavior_contract_without_verify_coverage` as verification proof follow-up
  items independent of source.
- [x] Add proof criteria marker `verification_probe_required=true`.
- [x] Add proof-follow-up planning gate requiring `verification_probes[]`, with
  one structured retry hint.
- [x] Enrich proof follow-up probe `changed_symbol_refs` from typed
  `symbol=` criteria before falling back to path-level refs.
- [x] Add regressions for proof classification and proof-plan probe
  requirement.

RC-60 smoke:

- Reran `pydata__xarray-4248` at
  `/private/tmp/codrax-swe-rc60-xarray-20260618-123244`.
- The first online slice applied a partial formatting fix, failed the bounded
  `units_repr_positive` probe with typed assertion evidence, restored the
  applied checkpoint, replanned, and applied a smaller second patch.
- Final prediction export was non-empty (`patch_bytes=987`) and the official
  SWE-bench harness dry-run accepted the generated predictions file.
- Final workflow reached `complete`, `verify_status=passed`, and
  `verify_test_count=2`; the passed verification surface was two bounded
  `verification_probe/python` probes, while the broader discovered unittest and
  pytest surfaces remained skipped/telemetry.
- Manual audit: the patch is functionally plausible for the requested repr
  behavior because it appends `attrs["units"]` as `, in <units>` for both
  coordinates and data variables and expands the first-column width to avoid
  truncating the unit text. Confidence remains low rather than authoritative:
  related project tests were not executed, PatchReview still reports
  `related_test_surface_unverified` as telemetry, and the change may have
  layout-width side effects that require convention/impact proof instead of a
  single xarray-specific rule.
- Architecture takeaway: the online `Edit -> Observe -> Replan -> Observe`
  kernel is now functioning for this case. The remaining gap is proof-quality
  confidence escalation from bounded probes plus related convention/test
  evidence, not patch export or a Python-only boundary signal.

## RC-61: Probe-Pass Should Continue Concrete Impact Test Surfaces

RC-60 showed that the online loop can recover from a failing bounded probe, but
the final confidence stayed low because `related_test_surface_unverified`
remained telemetry even when the `ChangePlan.ImpactAnalysis` contained concrete
related test paths:

- `impactRunnerPlansFromChangePlan` already maps typed related tests to
  runner-native suites across Python, Go, Node, Ruby, Java/Kotlin, Rust, and
  Swift.
- `run_tests` already queues these impact runner plans before broad default
  TestSurface plans for normal verification.
- The pre-suite `verification_probes[]` fast path returns immediately after a
  pass unless the plan touched tests or the probes omit changed-symbol refs.
  That skips concrete impact test surfaces and leaves low-confidence evidence
  even though the next action is bounded and deterministic.

This is a proof-quality scheduling gap, not an xarray-specific test rule. The
right behavior is:

- If bounded probes pass and the queued runner plan list contains
  `impact_test_surface` entries, continue to those scoped impact plans.
- Trim the continuation queue to the impact plans for that verify call so a
  concrete related-test proof does not escalate into a broad project suite.
- A real red related test remains `failed` and drives normal online replan.
- Timeout/OOM/CPU/parser/runner infrastructure after a passed probe remains a
  typed confidence warning, preserving the probe pass as the local behavior
  verdict instead of turning partial customer infrastructure into a hard
  blocker.
- Hard logic reads only `ChangePlan.ImpactAnalysis`,
  `ImpactVerificationTarget`, `TestSurface`, runner plan provenance, and typed
  execution status. It must not inspect user prose, issue text, model
  rationale, or `<think>`.

RC-61 tasks:

- [x] Add `impact_related_test_surface` as a pre-suite probe continuation
  reason when queued runner plans include concrete impact test surfaces.
- [x] Trim the post-probe continuation queue to impact runner plans for that
  reason.
- [x] Preserve the existing `plan_touches_test_path` and
  `verification_probe_missing_changed_symbol_ref` behavior.
- [x] Extend probe-primary infrastructure downgrade to
  `impact_related_test_surface`.
- [x] Add regressions proving a passing probe continues a failing scoped impact
  test, and a timed-out scoped impact test becomes confidence warning rather
  than hard failure.
- [x] Prefer the most precise same-depth impact runner candidate so Python
  pytest file selectors beat unittest fallback when both are available.
- [x] Render unittest directory selectors as `unittest discover -s <dir>` so
  scoped related-test directories do not become zero-test module invocations.
- [x] Run focused `internal/tool`, related write packages, full Go regression,
  `make test`, diff check, and a targeted SWE Lite rerun.

RC-61 smoke:

- Reran `pydata__xarray-4248` at
  `/private/tmp/codrax-swe-rc61-xarray-20260618-125247`.
- Final prediction export was non-empty (`patch_bytes=1248`) and the official
  SWE-bench harness dry-run accepted the generated predictions file.
- The workflow failed the first bounded probe, restored the checkpoint,
  replanned, and then passed the bounded probe plus two concrete impact test
  surfaces:
  `pytest xarray/tests/test_formatting.py` and
  `pytest xarray/tests/test_formatting_html.py`.
- `verify_status=passed`, `verify_test_count=32`, and final PatchReview
  coverage moved from the RC-60 `unverified` state to `verified`; no final
  semantic-unverified telemetry codes remained.
- Manual audit: this patch is stronger than RC-60 because it adjusts both
  `summarize_variable` and `_get_col_items`, so column-width calculation
  accounts for units before formatting. The scoped xarray formatting tests are
  exactly the related surfaces the impact engine derived, so local functional
  confidence is materially higher than probe-only verification.
- New remaining gap for the next batch: the adapter still exported a stale
  top-level `prediction_confidence_downgrade_reason` from earlier
  `plan_patch_review_*` telemetry even though `final_plan_patch_review_*` was
  verified. This is a result-authority aggregation gap, not a write runtime or
  patch correctness gap.

## 2026-06-18 RC-62 PatchReview Confidence Authority

RC-61 proved the runtime can now continue from bounded probes into concrete
impact test surfaces. The next gap is in SWE-bench result aggregation: final
PatchReview coverage can become verified, while the top-level prediction
confidence downgrade still reads stale delivery/source-owner telemetry from
earlier online batches.

This is a typed result-authority gap:

- `plan_patch_review_*` is useful audit telemetry because it preserves
  source-owner history and stale proof gaps;
- `final_plan_patch_review_*` is the report-plan actual-diff authority after a
  coherent delivery has a typed passed verifier report;
- hard actual-diff/effect blockers from any source owner must still block local
  acceptance through `plan_patch_review_block_reason`;
- proof-quality confidence must not keep reading stale telemetry after the
  final report-plan PatchReview proves the cumulative patch.

Design:

- Add a deterministic `patch_review_confidence_authority_summary()` selector.
- Inputs are only typed summaries plus `delivery_candidate_status` and
  `verify_status`; no issue text, model rationale, stdout prose, or keyword
  matching participates.
- If delivery is `coherent`, local verify is `passed`, and the final report-plan
  PatchReview summary has any structured signal, confidence consumes the final
  summary.
- Otherwise confidence falls back to the delivery/source-owner summary.
- Keep exporting both plan-level and final-level telemetry for manual audit and
  dashboards.
- Add `patch_review_confidence_authority_source` to result rows so future
  dashboards can explain whether confidence came from `final_plan` or
  `delivery`.

Tasks:

- [x] Implement the typed confidence-authority selector in the SWE-bench adapter.
- [x] Route `prediction_confidence_downgrade_reason` through the selected
  authority summary.
- [x] Preserve delivery hard blockers and existing official prediction export.
- [x] Add unit coverage for final-report authority and no-final-signal fallback.
- [x] Re-run adapter tests, compile, focused RC-61 xarray recompute, and update
  smoke evidence.

Verification:

- `python3 -m unittest eval.swebench.run_codrax_swebench_test`
- `python3 -m py_compile eval/swebench/run_codrax_swebench.py
  eval/swebench/run_codrax_swebench_test.py`
- `eval/swebench/smoke_local.sh`

RC-61 artifact recompute:

- Input artifact:
  `/private/tmp/codrax-swe-rc61-xarray-20260618-125247/results.jsonl`.
- Old top-level confidence downgrade:
  `patch_review_semantic_unverified:call_site_touched`.
- Typed selector result under RC-62 code:
  `selected_source=final_plan`,
  `selected_coverage_verdict=verified`,
  `new_patch_review_confidence_downgrade_reason=""`.
- This proves the stale-confidence reporting gap is closed for the passed RC-61
  trajectory without weakening delivery hard blockers or official prediction
  export.

RC-62 fresh xarray smoke:

- Reran `pydata__xarray-4248` at
  `/private/tmp/codrax-swe-rc62-xarray-20260618-135000`.
- Prediction export remained non-empty (`patch_bytes=931`) and official
  SWE-bench harness dry-run accepted the generated predictions file.
- This fresh trajectory did not reach the stale-confidence path because local
  verify correctly failed: `verify_status=failed`, `verify_test_count=32`,
  `prediction_verdict=predicted_failed_verify`,
  `local_acceptance_verdict=fail/source=local_audit_block`.
- Failure evidence: scoped impact tests reported `16 passed, 2 failed`; the
  patch inserted units into the main formatter but did not preserve existing
  xarray diff repr spacing/dtype expectations. This is a runtime convergence
  quality signal, not an adapter confidence-authority regression.
- New follow-up candidate: repeated checkpoint-restore replan eventually
  produced a narrower but still failing patch. The controller already preserved
  typed failed-test evidence and blocked rather than exporting a local pass; a
  future batch should improve failure-evidence compression into replan prompts
  and/or patch critic guidance so online convergence reaches the RC-61-quality
  fix more consistently.

## 2026-06-18 RC-63 Typed Failure-Signal Handoff

RC-62 fresh xarray smoke proved the runtime now blocks bad patches through
scoped impact tests, but it also exposed a convergence-quality gap: failed
test evidence survives as long `failure_detail` text and failure-summary
previews, while the next planner turn lacks a compact typed delta such as
assertion, suite, first file:line, and high-signal assertion lines.

This is a system-level handoff gap, not an xarray-specific bug:

- online convergence depends on `Edit -> Observe -> Repair` loops carrying the
  newest observation as the lead repair constraint;
- current `VerifyFailureHandoff` carries failed rows and artifacts, but not a
  normalized failure signal that can be ranked, deduped, and rendered within a
  small planner Top-N budget;
- `WriteContextPackFromChangeReport` renders `failed_test` as test id plus
  raw detail, which asks the model to parse runner prose repeatedly;
- the existing `ExtractFailureSignal` helper lived under orchestrator, so it
  could not serve durable `types` handoff without copy/paste drift.

Design:

- Move the shared runner-output signal extractor to `internal/types`, leaving a
  thin orchestrator wrapper for existing callers.
- Add `types.TestFailureSignal` as soft replan guidance:
  `assertion_id / suite / kind / location / signal / expected / actual`.
- Generate signals only from typed `TestResult` rows and runner output; no user
  intent keywords, issue text, model prose, or free-form rationale drives hard
  logic.
- Attach bounded `failure_signals[]` to `VerifyFailureHandoff`.
- Project `failure_signal` P2 items into `WriteContextPack` before raw
  `failed_test` rows, so planner/controller/verifier limited views preserve the
  compact repair signal.
- Render `failure_signal` rows in the planner's direct failed-verify handoff
  section before long failing-test details.
- Keep original `failed_test`, `failure_summary_blob_ref`, and artifact refs as
  fallback context for cases where the compact signal is insufficient.

Tasks:

- [x] Add shared failure-signal extractor and typed signal shape.
- [x] Preserve existing orchestrator `ExtractFailureSignal` API through a thin
  wrapper.
- [x] Add `VerifyFailureHandoff.FailureSignals`.
- [x] Add `failure_signal` to `WriteContextPack` and planner limited-view
  priority.
- [x] Render direct planner handoff failure signals before raw failing tests.
- [x] Update planner prompt guidance to prefer compact failure signals as soft
  evidence.
- [x] Run focused types/agent/orchestrator tests, full Go regression, and
  update smoke/eval evidence.

Verification:

- `go test ./internal/types -run 'Test(ExtractTestFailureSignal|BuildVerifyFailureHandoff|WriteContextPackFromChangeReport|WriteContextPackPlanner)' -count=1`
- `go test ./internal/orchestrator -run TestExtractFailureSignal -count=1`
- `go test ./internal/agent -run 'TestBuildVerifyFailureHandoffSection|TestPlannerWriteContextPack|TestChangePlanSkill|TestPlanner' -count=1`
- `go test ./internal/types ./internal/agent ./internal/orchestrator ./internal/tool ./internal/writeflow -count=1`
- `go test ./...`
- `make test`

RC-63 smoke:

- Reran `pydata__xarray-4248` at
  `/private/tmp/codrax-swe-rc63-xarray-20260618-142300`.
- Prediction export was non-empty (`patch_bytes=942`) and official SWE-bench
  harness dry-run accepted the generated predictions file.
- The first verify failed on bounded probes with concrete assertion evidence:
  expected `x, in metres` / `rainfall, in mm`, but output was truncated to
  `x, in m...` / `rainfal...`.
- After replan, the workflow completed with `verify_status=passed`,
  `verify_test_count=32`, `prediction_verdict=predicted_passed`,
  `prediction_confidence_downgrade_reason=""`,
  `patch_review_confidence_authority_source=final_plan`, and
  `local_acceptance_verdict=pass/source=local_verify`.
- Manual audit: the final patch adjusts formatter column width before
  `pretty_print`, preserving existing formatting tests while exposing units.
  This is materially better than the RC-62 fresh run, where failed-test evidence
  led to a narrower patch that broke diff repr tests.

## 2026-06-18 RC-64 Adjacent Duplicate Inserted Block Critic

The second RC-63 smoke (`sympy__sympy-18199`) produced a non-empty,
harness-consumable patch but inserted the exact same `if a % p == 0` block
twice in adjacent lines. Codrax correctly kept the row low-confidence
(`prediction_verdict=predicted_passed_low_confidence`,
`local_acceptance_verdict=unknown`), but the actual diff critic did not flag
the duplicate block as a structural patch defect.

This is a general Patch Critic gap:

- duplicate adjacent inserted code blocks are visible in the applied diff, so
  they should be caught deterministically before relying on local probes;
- the signal is language-agnostic and belongs in `PatchEffect`, not SWE-bench
  post-processing;
- the hard gate must read only structured diff lines and file roles, not issue
  text, model summaries, or keywords from the user request;
- to avoid false positives, the rule should be conservative: same hunk,
  adjacent duplicate, at least three nonblank normalized lines, and at least one
  real code line.

Design:

- Add `duplicate_inserted_block_added` as a `PatchEffectEvent`.
- Detect only adjacent duplicate added blocks inside one hunk.
- Treat the event as a PatchReview structural error/hard block.
- Preserve existing soft actual-diff events and language-provider hooks.

Tasks:

- [x] Add the duplicate inserted block detector in `patch_effect.go`.
- [x] Register the event as a hard PatchReview event.
- [x] Add focused tests proving adjacent duplicate blocks hard-block review.
- [x] Run focused writeflow tests, related regressions, full Go regression, and
  update smoke/eval evidence.

Verification:

- `go test ./internal/writeflow -run 'TestAnnotatePatchEffect(DuplicateInsertedBlock|PythonDuplicateSymbol|PythonTopLevelSelfMethod|ProductionTestScaffold|PythonNestedStringKeyAccess)|TestReviewAppliedPatch' -count=1`
- `go test ./internal/writeflow ./internal/types ./internal/orchestrator -count=1`
- `go test ./...`
- `make test`

RC-64 smoke:

- Reran `sympy__sympy-18199` at
  `/private/tmp/codrax-swe-rc64-sympy18199-20260618-143900`.
- Prediction export remained non-empty (`patch_bytes=537`) and official
  SWE-bench harness dry-run accepted the generated predictions file.
- This trajectory did not reproduce the duplicate adjacent block; the patch
  inserted one guard:
  `if a % p == 0: return [0] if all_roots else 0`.
- Local verdict remained conservative: `verify_status=unavailable`,
  `prediction_verdict=predicted_unverified`,
  `prediction_confidence_downgrade_reason=make_target_missing`,
  `local_acceptance_verdict=unknown`.
- Manual audit: the patch is plausible for the prime-modulus zero-residue case
  but remains unproven locally and likely incomplete for the broader historical
  issue. The important system evidence is that Codrax no longer labels it a
  functional pass without authoritative verify; duplicate-block hard blocking is
  locked by focused PatchEffect/PatchReview tests.

## 2026-06-18 RC-65 Structured Plan Repair Relocation And Canceled Run Terminalization

RC-65 three-instance SWE Lite smoke started at
`/private/tmp/codrax-swe-rc65-three-20260618-151000` exposed a workflow-level
gap before any new source patch was produced:

- `django__django-11742` located the correct source/test surfaces for the
  `Field.max_length` + `choices` check, then repeatedly failed
  `emit_change_plan`. The decisive validator error was an `old_text_mismatch`
  where a test-file edit body was placed inside the production file's
  `changes[].edits[]`. The existing repair pack carried exact current bytes, but
  it did not use the already-typed target/test paths from `WriteAnalysisIR`,
  exploration request, handoff, or context pack to identify that the supplied
  `old_text` uniquely belonged to another candidate file. The planner retried
  until write wall-time expired and exported an empty patch.
- The same Django row recorded `workflow_status=in_progress` even though the
  controller had already emitted `plan_batch_canceled` after wall-time expiry.
  That leaves `/workflow show`, resume semantics, and eval attribution in a
  contradictory state: no patch was produced, but the durable run still looks
  active.
- `mwaskom__seaborn-3190` is running under the pre-RC-65 binary and exposed a
  separate candidate gap: the applied boolean-scale fix can satisfy a narrow
  typed probe, while the full `Plot(...).add(so.Bar())._plot()` verifier probe
  fails on host dependency drift (`pandas` no longer has
  `mode.use_inf_as_na`). This should be treated as a future environment
  attribution / verifier-baseline issue, not folded into the structured repair
  batch.

Root-cause classification:

- The Django failure is not primarily exploration accuracy. The planner had
  scoped files and behavioral intent. The failure is a structural plan-emission
  repair gap: typed validation detected the bad edit, but the repair artifact
  lacked a deterministic cross-file relocation candidate derived from typed
  path evidence.
- The workflow status issue is a state-kernel gap: canceled plan batch terminal
  semantics were optimized for manual resume, but in Auto Pilot/eval they create
  stale active runs and ambiguous empty-patch rows.

Design:

- Extend `PlanRepairCurrentBytes` with
  `relocation_candidates[] { path, start_line, end_line, source }`.
- On structured-edit `old_text_mismatch`, inspect only typed candidate paths:
  `ChangePlan` paths, `WriteAnalysisIR` scope anchors / prescan / phase targets,
  `WriteExplorationRequest.candidate_paths`, `WriteExplorationHandoff`
  target/test/evidence refs, and context-pack evidence refs.
- Read no broad repository glob and do not parse user request text, model
  summary, rationale, or `<think>`. If the supplied old text uniquely matches
  exactly one candidate file/line range, attach that relocation candidate to the
  repair pack. Ambiguous or absent matches remain ordinary `old_text_mismatch`.
- Render relocation candidates in the controller retry hint so the planner
  receives a bounded typed edit-location repair instead of prose-only retry
  advice.
- Treat plan-batch cancellation after wall-time/context cancellation as a
  terminal blocked workflow run when no applied-patch interruption guidance owns
  the state. Preserve `plan_batch_canceled` as the reason code so user-visible
  status and eval telemetry stay transparent.

Tasks:

- [x] Add relocation-candidate fields to `PlanRepairCurrentBytes` and normalize
  them.
- [x] Add structured-edit relocation candidate discovery over typed candidate
  paths only.
- [x] Copy relocation candidates into `write_plan_repair_pack` metadata and
  render them in controller no-plan retry hints.
- [x] Mark canceled plan-batch workflows as blocked terminal runs while
  preserving `plan_batch_canceled` evidence.
- [x] Add focused tests for wrong-path `old_text` relocation, retry-hint
  rendering, and canceled workflow terminalization.
- [x] Run full Go regression, `make test`, `make`, and `git diff --check`.
- [x] Rerun the Django RC-65 reproducer with the rebuilt binary after this
  batch lands, then decide whether RC-66 should address dependency-drift
  verifier attribution from the Seaborn smoke.

Verification so far:

- `go test ./internal/tool -run 'TestEmitChangePlan_OldTextMismatch|TestEmitPlanChange_FinalizeOldTextMismatch'`
- `go test ./internal/orchestrator -run 'TestLastPlanEmitRejectionView|TestRunWriteControllerWorkflow_CanceledPlanRecordsCanceledReason'`
- `go test ./internal/types -run TestPlanRepairPack`
- `go test ./internal/tool ./internal/types ./internal/orchestrator`
- `go test ./...`
- `make test`
- `make`
- `git diff --check`

Reproducer evidence:

- Pre-fix three-instance smoke:
  `/private/tmp/codrax-swe-rc65-three-20260618-151000`
  recorded `django__django-11742` as `status=empty_patch`,
  `empty_patch_reason=write_wall_time_empty_patch`,
  `workflow_status=in_progress`, and
  `workflow_latest_progress_reason_code=plan_batch_canceled`.
- Rebuilt targeted Django rerun:
  `/private/tmp/codrax-swe-rc65-django-rerun-20260618-140600`
  recorded `status=predicted`, `patch_bytes=1408`,
  `workflow_status=complete`, and validated one non-empty
  harness-consumable prediction. This closes the RC-65 empty-export and stale
  active-run symptoms.
- The same targeted rerun still reports `verify_status=failed` with
  `prediction_verdict=predicted_failed_verify`. That is deliberately not
  counted as RC-65 functional correctness. The remaining typed evidence belongs
  to the next verifier/coverage-attribution line: scoped test selection,
  environment baseline attribution, and proof authority.

## 2026-06-18 RC-66 Command-Derived Failure Reason Attribution

The RC-65 Django rerun closed empty export and stale workflow state, but its
local verifier row exposed a typed attribution bug:

- the actual report had `FailureKind=tests_failed` because many Django
  `tests/runtests.py ...` invocations failed after the patch;
- later default `make check` attempts were unavailable and emitted typed
  `ExecutedCommand{outcome=parser_error, reason_code=make_target_missing}`;
- final report installation backfilled `FailureReasonCode` from all command
  evidence, so a secondary unavailable runner became the primary
  `tests_failed` reason.

This is not a Django-specific issue. Any multi-runner verification aggregate can
mix red tests, parser/startup errors, missing runners, zero-test selectors, and
resource limits. The primary reason must come from the same typed failure kind as
the selected primary `FailureKind`; secondary runner evidence should remain in
`ExecutedCommands`, `VerificationDiagnostics`, `NoTestsRunners`, and summaries.

Design:

- Keep the existing `FailureKind` precedence model.
- Convert command-derived reason codes into `failureReasonCandidate{kind,codes}`
  using only `ExecutedCommand.Outcome`, `Runner`, and bounded typed
  `ReasonCode`.
- When `ChangeReport.FailureReasonCode` is missing, merge/report installation
  selects command-derived reason codes whose derived kind matches the primary
  report kind.
- Preserve verification-probe runtime exceptions as `tests_failed`, while
  probe authoring/import errors, parser errors, missing runners, zero-tests, and
  resource limits keep their own typed kinds.
- Do not parse command output, test stdout, user text, model rationale, or
  `<think>` content.

Tasks:

- [x] Add typed command->failure-kind mapping for failure-reason candidates.
- [x] Update aggregate report merge to append command-derived candidates with
  their own derived kind instead of the enclosing report kind.
- [x] Update final report installation backfill to filter command reasons by
  the report's primary `FailureKind`.
- [x] Add focused tests for red-test reports polluted by `make_target_missing`
  command evidence and for preserving verification-probe runtime failures.
- [x] Run related/full regressions, refresh progress, commit, and push.

Verification so far:

- `go test ./internal/tool -run 'TestMergeChangeReports|TestFailureReasonCodeFromExecutedCommandsForKind'`
- `go test ./internal/tool`
- `go test ./...`
- `make test`
- `git diff --check`

## 2026-06-18 RC-67 Runner-Native Impact Test Selectors

The same Django evidence exposed a second verifier automation gap after RC-66:
the impact runner queue selected dozens of `tests/**/__init__.py` paths as
`test_surface` targets and rendered them as Django runner labels such as
`absolute_url_overrides.__init__`. These package marker files are typed path
artifacts, but they are not runnable test surfaces. They create zero-test churn
and noisy verifier summaries, and they slow online convergence before the
planner receives the truly useful failure evidence.

This is a runner-selector contract problem, not a Django issue patch:

- impact analysis can legitimately discover related files and inferred test
  surface paths;
- each runner/provider must decide which related paths are runnable selectors;
- selectors must be rendered in the runner's native shape rather than passing
  filesystem paths blindly.

Design:

- Keep impact analysis broad enough to preserve evidence; filter unsupported
  selectors only at the runner-provider boundary.
- Treat Python package marker `__init__.py` files as non-runnable impact test
  selectors for Python test runners.
- For `python/django`, convert test file paths through the existing
  `djangoSuiteSelector` helper so `tests/app/tests.py` becomes `app.tests` and
  `tests/app/test_case.py` becomes `app.test_case`.
- Leave pytest, unittest, Go, Node, Java/Kotlin, Ruby, Rust, and Swift existing
  selector contracts unchanged except for the generic Python package-marker
  filter.
- Do not parse issue prose, model output, stdout summaries, or user intent.

Tasks:

- [x] Add Python package-marker filtering to `impactSuiteForCandidate`.
- [x] Reuse existing Django suite-label normalization for impact runner plans.
- [x] Add focused tests that ignore Django `__init__.py` targets and preserve
  runnable Django labels.
- [x] Run related/full regressions, refresh progress, commit, and push.

Verification so far:

- `go test ./internal/tool -run 'TestImpactRunnerPlans'`
- `go test ./internal/tool`
- `go test ./...`
- `make test`
- `git diff --check`

## 2026-06-18 RC-68 Observation Authority Red-Test Precedence

The RC-67 targeted Django smoke at
`/private/tmp/codrax-swe-rc67-django-20260618-142500` validated RC-66/RC-67 and
then exposed a state-machine authority bug:

- prediction export stayed non-empty and harness-consumable;
- verify executed the correct Django selector
  `invalid_models_tests.test_ordinary_fields` instead of dozens of
  `__init__` selectors;
- `FailureReasonCode` was no longer polluted by `make_target_missing`;
- however the report also carried secondary unavailable make runners, so
  `DeriveObservationAuthorityFromReport` classified the mixed report as
  unverified/no-tests before checking the red Django failures;
- the batch was terminalized as `complete/accepted_failed`, and a later
  controller `replan_batch` decision was rejected because the workflow was
  already complete.

This is a generic observation-authority ordering gap. A report can contain
multiple typed signals: red tests, build failures, no-tests runners, parser
errors, missing make targets, resource limits, and probe confidence warnings.
Only unavailable-only reports should finish as unverified. Any report with a
typed failed verification status must keep `ObservationAuthorityFailed` and
route to replan/block according to budget.

Design:

- Keep explicit runner-missing/parser-error/no-tests-only reports as
  `ObservationAuthorityUnverified`.
- Promote `VerificationStatusFailed` before generic `NoTestsRunners` caveats,
  so mixed red-test + unavailable-runner reports remain failures.
- Preserve legacy empty `Passed=false + NoTestsRunners` behavior because
  `ChangeReport.NormalizeVerificationStatus` still classifies reports with no
  test rows as unavailable.
- Do not inspect failure detail text, command stdout, model prose, user intent,
  or `<think>` content.

Tasks:

- [x] Reorder observation authority to prefer typed failed verification over
  secondary no-tests evidence.
- [x] Add observation-authority and verify-outcome tests for mixed red-test +
  no-tests reports.
- [x] Run related/full regressions, rerun targeted Django smoke, refresh
  progress, commit, and push.

Verification so far:

- `go test ./internal/writeflow -run 'TestDeriveObservationAuthorityFromReport|TestClassifyVerifyAttemptOutcome'`
- `go test ./internal/writeflow ./internal/orchestrator ./internal/types`
- `go test ./...`
- `make test`
- `git diff --check`
- Targeted Django smoke:
  `/private/tmp/codrax-swe-rc68-django-20260618-143400`

Smoke evidence:

- The first failed Django suite now left the workflow in
  `batch-1:ready_to_plan` with
  `checkpoint_restored_before_replan`, proving red-test observations no longer
  terminalize as unverified completion.
- The run generated a non-empty harness-consumable prediction
  (`patch_bytes=1530`).
- The final row still ended `workflow_status=in_progress` with
  `plan_batch_interrupted_after_applied_patch`; final verify failed because a
  model-generated verification probe called `Field.check()` on an unbound
  Django field, causing `AttributeError: 'NoneType' object has no attribute
  'endswith'`. That is a separate probe-construction/budget-terminalization gap
  for the next batch, not an RC-68 regression.

## 2026-06-18 RC-69 Typed Interruption Source And Applied-Patch Terminalization

The RC-68 Django smoke proved red verification now routes to replan, but the
final run still exported `workflow_status=in_progress` with
`plan_batch_interrupted_after_applied_patch`. The controller had already
preserved the applied patch and recorded failed verification evidence, yet the
durable workflow status stayed ambiguous because all `context.Canceled` values
were treated as resumable operator cancellation.

This is a state-kernel gap, not a Django-specific repair. Online convergence has
at least two different interruption sources:

- operator cancellation (`Ctrl+C`, `/cancel`, explicit stop): preserve the
  applied patch and keep a resumable workflow;
- deterministic write wall-time cancellation: fail loud with a terminal
  `blocked` workflow/batch after preserving recovery refs and typed evidence,
  both when the patch has already been applied and when the controller is
  interrupted after planning but before the next executable decision.

The implementation must not parse the cancellation reason string. RC-69 adds a
typed `CancelSource` to `CancelToken` and marks the write-mode deadline timer as
`write_deadline`; REPL and public `Cancel(reason)` keep the default `user`
source. Applied-patch interruption handling then consumes the typed source to
choose resumable-vs-blocked state.

Tasks:

- [x] Add `CancelSource` to the orchestrator cancellation token and preserve
  first-source-wins semantics.
- [x] Mark the write-mode wall-clock timer with `CancelSourceWriteDeadline`
  while keeping read mode and public user cancellation behavior unchanged.
- [x] In controller plan/replan interruption after applied work, block the
  workflow for typed write-deadline cancellation and preserve resumable state
  for typed user/unknown cancellation.
- [x] In controller dispatch interruption after a bounded plan has been produced
  but not yet applied, block the workflow for typed write-deadline
  cancellation instead of leaving `planned/in_progress`.
- [x] Add focused tests for cancel-source recording, user cancel after applied
  patch, write-deadline cancel after applied patch, and write-deadline cancel
  after plan-before-apply.
- [x] Run focused, related, full regressions, rerun the targeted Django smoke if
  time allows, refresh this ledger, commit, and push.

Design constraints:

- Hard routing reads only the typed cancel source and workflow/plan/report
  records.
- No user-intent keywords, model prose, stdout narrative, issue text, or
  `<think>` content participates in the gate.
- Read/log/trace/data/operation/computer modes remain untouched; the deadline
  source is set only by the existing write-mode timer.

Verification:

- `go test ./internal/orchestrator -run 'TestCancelToken_(FirstReasonWins|FirstSourceWins)|TestRunWriteControllerWorkflow_(DispatchWriteDeadlineAfterPlanBlocksRun|DoesNotRecoverCanceledControllerDispatch|ReplanCancelReportsAppliedPatch|ReplanWriteDeadlineAfterAppliedPatchBlocksRun|ReplanFailureAfterAppliedPatchBlocksRun|CanceledPlanRecordsCanceledReason)' -count=1`
- `go test ./internal/orchestrator ./internal/writeflow ./internal/types -count=1`
- `go test ./...`
- `make test`
- `make`
- `git diff --check`

SWE smoke:

- First targeted Django rerun with the pre-dispatch-deadline fix exposed the
  second edge: deadline after replacement plan production but before apply left
  `workflow_in_progress_empty_patch`.
- After adding generic typed `write_deadline` dispatch terminalization, the
  targeted Django rerun at
  `/private/tmp/codrax-swe-rc69b-django-20260618-150800` produced
  `patch_bytes=1740`, `workflow_status=blocked`,
  `workflow_latest_progress_reason_code=plan_batch_interrupted_after_applied_patch_blocked`,
  `prediction_verdict=predicted_failed_verify`, and an official-harness
  consumable prediction. This proves export/status no longer collapses to an
  empty in-progress patch when deadline interrupts online convergence.
- Functional correctness is still not achieved for `django__django-11742`: the
  final patch fails local verification. The remaining system gaps are planner
  edit-location affordance (repeated `old_text`/line-anchor retries) and
  verification-probe fixture/lifecycle quality for framework objects. Those are
  separate follow-up batches, not RC-69 state-kernel regressions.

## 2026-06-18 RC-70 Local Structured-Edit Anchor Relocation

The RC-69 Django smoke still spent several planner turns correcting
`emit_change_plan` structured edits:

- the model supplied the right `old_text` but a nearby stale line number;
- existing structured-edit validation could auto-relocate only when the
  submitted `old_text` was unique across the whole file;
- common anchors such as `return []` or `break` are not globally unique, so the
  planner had to re-emit several times using repair-pack prose and line
  arithmetic.

This is not a prompt issue and should not be fixed by keyword matching model
output. It is a deterministic edit affordance gap: when the intended anchor can
be proven from current bytes in a small local window, the tool should compile
the edit directly.

Design:

- Extend structured-edit normalization with same-file, local-window relocation:
  if `old_text` mismatches the submitted line/range, look for an exact
  `old_text` range within a bounded window around the submitted start line.
- Accept relocation only when there is exactly one match in that local window.
  Ambiguous or missing matches stay rejected with the existing typed
  `old_text_mismatch` repair pack.
- Apply the same local-window proof to replace/delete and insert_before/after.
- Keep existing whole-file unique relocation as a second chance; the local
  window handles common repeated anchors without broadening to unrelated file
  regions.
- Hard routing reads only current file bytes, edit kind, line numbers, and exact
  `old_text`; no user issue text, model prose, stdout narrative, or `<think>`
  content can trigger relocation.

Tasks:

- [x] Add a bounded helper for local unique old_text range lookup.
- [x] Use it before whole-file unique relocation for replace/delete.
- [x] Use it before whole-file unique relocation for insert_before/after and
  keep insert_before/after positioning relative to the relocated old_text range.
- [x] Add focused tests for repeated global anchors with one local match,
  off-by-one insert anchors, and ambiguous local matches that still reject.
- [x] Run focused, related, full regressions, refresh this ledger, commit, and
  push.

Verification:

- Focused structured-edit and `emit_change_plan` relocation tests pass.
- Related `internal/tool`, `internal/types`, and `internal/orchestrator`
  regressions pass.
- Full `go test ./...` passes.

## 2026-06-18 RC-71 Verification Probe Top-Level Exception Boundary

RC-69/RC-70 Django follow-up kept one verifier-quality gap open: a bounded
`verification_probes[]` check can fail because the probe itself constructed an
unrealistic framework fixture or lifecycle, not because the patched product
code is wrong. Before this batch, Python probes treated only top-level
`NameError` as probe authoring/infrastructure. Other top-level exceptions such
as `AttributeError`, `TypeError`, `ValueError`, or `FileNotFoundError` could be
classified as `tests_failed`, causing the controller to repair product code from
probe-fixture noise.

This is a typed observation-boundary issue, not a prompt issue:

- if a Python probe traceback contains only `<codrax_verification_probe>` frames
  and no product-code frame, the exception is probe-authored and produces an
  unavailable local verification signal;
- if the traceback enters an actual repo/product file, the same exception family
  remains a real `tests_failed` verification failure;
- AssertionError and explicit non-zero SystemExit remain behavior failures;
- import and syntax errors keep their existing parser/unavailable semantics.

Design constraints:

- The hard gate reads only the structured probe wrapper status:
  `outcome`, exception type, exit code, and `probe_top_level`.
- No user issue text, model rationale, stdout prose, manual audit notes, or
  `<think>` content participates in classification.
- The output remains existing typed `ChangeReport`,
  `ExecutedCommand.ReasonCode`, `VerificationDiagnostic`, and
  `VerificationConfidence` records, so handoff/REPL/eval consumers do not need
  bespoke parsing.

Tasks:

- [x] Add `verification_probe_top_level_exception` for Python probe exceptions
  with `probe_top_level=true`.
- [x] Classify top-level probe exceptions as `parser_error` /
  `VerificationStatus=unavailable` with `probe_authoring` diagnostics.
- [x] Preserve product-code-frame exceptions as `tests_failed` and
  `verification_probe_exception`.
- [x] Add focused tests for top-level probe authoring failure, product-frame
  runtime failure, existing import errors, and NameError diagnostics.
- [x] Run related/full regressions, refresh this ledger with verification
  evidence, commit, and push.

Verification:

- Focused probe-boundary tests pass.
- `go test ./internal/tool -count=1` passes.
- `go test ./internal/tool ./internal/types ./internal/orchestrator -count=1`
  passes.
- Full `go test ./...` passes.

## 2026-06-18 RC-72 Actual-Diff Nested Collection Exclusion Boundary

The post-RC-71 targeted run for `django__django-11742` proved that the online
state kernel and export path now work:

- workflow status reached `complete`;
- local verifier reported `passed`;
- the prediction patch was non-empty and official-harness dry-run consumable;
- checkpoint restore/replan happened after earlier red observations instead of
  restarting from scratch.

Manual audit still found a likely semantic miss. The final patch checks flat
choices for `max_length`, but it sets a flatness flag to false and skips the
entire grouped/nested choices branch. The generated probes proved only a
flat-value invariant plus a too-weak "grouped choices no error" invariant. That
is a root-cause/proof-boundary gap, not an export, scheduler, or Python-only
runtime gap.

This batch adds a generalized actual-diff event:

```text
actual added source hunk
  -> language provider sees nested-collection shape check
  -> nearby branch exclusion action appears in validation/handling context
  -> PatchEffectEvent(code=nested_collection_branch_exclusion_added)
  -> PatchReview semantic coverage unknown / effect_followup
  -> P2 context + bounded follow-up can require targeted nested-collection proof
```

Language scope:

- This is not Python-only. The event is declared through the existing
  source-shape provider registry for Python, JavaScript, TypeScript, Ruby,
  Java, Kotlin, and Go.
- Python remains the first SWE-bench evidence source because SWE-bench Lite is
  Python-centric, but the architecture is language-provider based.
- Deeper provider strength is intentionally uneven: Python already has
  duplicate/top-level owner annotators; other languages currently carry precise
  line/hunk shape events and compile/runner verification paths. Future AST or
  compiler-backed providers should extend the registry rather than add
  scheduler branches.

Design constraints:

- The producer reads only actual diff hunks and post-apply source bytes.
- It does not inspect user issue text, model rationale, stdout narrative,
  manual audit notes, `<think>` content, or natural-language summaries.
- The event is soft semantic coverage, not a hard blocker and not an approval
  trigger. It lowers confidence and hands a typed proof obligation to the
  controller/planner/verifier loop.
- The event is intentionally generic: it does not mention choices, Django,
  `max_length`, or a specific repository. It covers the broader pattern "new
  validation/handling code detects nested collections but excludes that branch
  from the new behavior".

Tasks:

- [x] Add provider-owned nested collection shape-check, branch-exclusion, and
  validation-signal rules for Python, JS/TS, Ruby, Java/Kotlin, and Go.
- [x] Emit `nested_collection_branch_exclusion_added` from actual diff hunk
  structure when a bounded nearby exclusion action follows a nested collection
  shape check in validation/handling context.
- [x] Register the event as PatchReview soft semantic coverage with
  `coverage_status=unknown` and `impact_kind=effect_followup`.
- [x] Add multi-language provider tests proving the event is not Python-only.
- [x] Add a handled-branch regression proving nested collection expansion does
  not emit the exclusion event.

Verification:

- Focused nested-collection/provider PatchEffect tests pass.
- `go test ./internal/writeflow -count=1` passes.
- `go test ./internal/writeflow ./internal/types ./internal/orchestrator -count=1` passes.
- Full `go test ./...` passes.
- `make test`, `make`, and `git diff --check` pass.

## 2026-06-18 RC-73 Actual-Diff Unreachable Body Hard Gate

The post-RC-72 `django__django-11742` rerun produced a useful split signal:

- prediction export stayed non-empty and official-harness dry-run consumable;
- workflow correctly restored checkpoints and replanned after red verification;
- local acceptance became `fail` instead of a weak pass;
- the final exported patch was still structurally bad.

The bad patch was not the earlier nested-collection exclusion shape. It inserted
a full new body under an already existing Python method and left the previous
method body in place:

```text
def _check_max_length_covers_choices(self):
    ...new logic...
    return []
    if self.max_length is None:
        ...
```

Existing `duplicate_inserted_block_added` only detects adjacent duplication
inside newly added lines. It cannot catch a new function-body return that makes
pre-existing same-function code unreachable. That is a Patch Critic gap over
the actual final source shape.

RC-73 adds a Python provider hard event:

```text
actual added line number + post-apply Python source bytes
  -> added return statement at function-body base indent
  -> later non-comment statement still exists in the same function body
  -> PatchEffectEvent(code=python_unreachable_body_after_added_return, severity=error)
  -> PatchReview hard block
```

Design constraints:

- The gate reads only `PatchEffectHunk.AddedLineNumbers`, post-apply file bytes,
  and deterministic Python indentation/`def` structure.
- It does not inspect user issue text, model rationale, stdout narrative,
  manual audit notes, `<think>` content, or finding prose.
- It is intentionally structural: an added `return` nested under a guard does
  not trigger when following function-body statements remain reachable.
- This batch implements Python because the current failure is Python and Codrax
  already has Python hunk annotators. The provider model remains the extension
  point for JS/TS/Ruby/JVM/Go equivalent unreachable-code producers.

Tasks:

- [x] Add `python_unreachable_body_after_added_return` PatchEffect hard event.
- [x] Detect only function-body base-indent added returns that leave later
  same-function statements unreachable.
- [x] Register the event as a PatchReview hard blocker.
- [x] Add regression for the RC-72 shape: added return before existing method
  body hard-blocks.
- [x] Add regression proving nested/guarded returns do not mark following
  function code unreachable.

Verification:

- Focused Python unreachable-return PatchEffect tests pass.
- `go test ./internal/writeflow -count=1` passes.
- `go test ./internal/writeflow ./internal/types ./internal/orchestrator -count=1` passes.
- Full `go test ./...` passes.
- `make test`, `make`, and `git diff --check` pass.

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
| RC-42 | complete | Source-check provider registry: source-check extensions, no-test extensions, before-runner policy, and dispatch now share one typed registry to avoid future language drift. Verification: focused provider-registry tests, full `internal/tool`, full `go test ./...`, `make test`, SWE adapter smoke, and diff check pass. |
| RC-43 | complete | Impact selector language coverage: typed related-test targets now prioritize Java/Kotlin, Rust integration-test, and Swift Package test selectors in addition to existing Python/Go/Node/Ruby coverage. Runner command construction normalizes Java/Kotlin path selectors to class selectors, Rust integration paths to `cargo test --test`, and Swift paths to `swift test --filter`, all from structured paths rather than prose. Verification: focused selector/command regressions, full `internal/tool`, full `go test ./...`, `make test`, SWE adapter smoke, and diff check pass. |
| RC-44 | complete | Verify source-compile fallback reuse: Java/Kotlin and Swift no-test source changes now use typed source-check providers instead of synthetic no-tests when plan-touched files exist. Java reuses the Maven/Gradle compile command selector shared with plan dry-build; Kotlin uses bounded `kotlinc` when available; Swift uses `swift build --skip-build`. Missing tools/manifests or unparseable environment output stay pass-with-warning, while parseable build diagnostics fail verify with structured `BuildErrors[]`. |
| RC-45 | complete | User-facing guide sync: Markdown and HTML now document Java/Kotlin/Swift source compile fallback and clarify that actual-diff mapping/container boundary signals are multi-language provider events, not Python-only logic. Verification: diff check and focused `internal/tool` regressions pass. |
| RC-46 | complete | Java verification probe runtime: bounded `verification_probes[]` now support Java through the typed runtime registry, with `javac` compile, `java -ea` execution, missing-JDK unavailable semantics, Java production-class coupling, packaged full-source probes, and soft docs/prompt sync. Verification: focused runtime/coupling tests, full `internal/tool`, related packages, `go test ./...`, `make test`, and diff check pass. |
| RC-47 | complete | Owner-boundary runtime signals: actual-diff source providers now emit typed soft semantic-coverage events for caller-return wrapper adapters, newly guarded diagnostic calls, and external private-state writes across precise Python/JS/TS/Ruby/Java/Kotlin shapes, while Go keeps only the precise return-wrapper shape. PatchReview registers the generic event codes as unknown coverage so they reach P2 handoff and bounded repair scheduling instead of staying SWE-bench adapter-only audit telemetry. Verification: focused writeflow owner-boundary/registry tests and related package regression pass. |
| RC-48 | complete | Plan localization retry: controller planning now consumes typed prior-localization context and gives one bounded retry when a fresh source plan edits paths outside P0/P1 evidence-backed anchors. The helper excludes plan-authored context and test-only paths, skips no-prior-context cases, and preserves legal new owner discovery after the bounded retry. Verification: focused types/orchestrator localization tests, related package regression, `go test ./...`, `make test`, and diff check pass. |
| RC-49 | complete | Official harness dry-run import check: the harness wrapper now imports `swebench.harness.run_evaluation` for non-dry runs and for Lite dry-runs by default, while command-only dry-run remains available. README documents Python 3.10+ and the `SWEBENCH_CHECK_OFFICIAL_IMPORT` / `CHECK_HARNESS_IMPORT` knobs. Targeted RC-48 smoke exported 3/3 non-empty predictions and exposed continued correctness gaps; Python 3.9 import-check fails clearly, Python 3.11 import-check dry-run passes, and dependency-free `smoke_local.sh` still passes. |
| RC-50 | complete | Verification proof obligation follow-up: controller now promotes typed missing `verification_confidence` proof refs into the existing bounded follow-up queue. Local passes with incomplete soft/hard contract probe proof append one scoped `proof-repair` batch when concrete refs exist, preserve reason/refs/source in success criteria, skip blind no-ref records, and stop after one proof follow-up. Verification: focused controller regressions and related write packages pass. |
| RC-50 smoke | complete | Reran three Lite instances (`django__django-11742`, `mwaskom__seaborn-3190`, `pydata__xarray-4248`) at `/private/tmp/codrax-swe-rc50-20260618-102834`: predictions validated 3/3 non-empty and official harness import/dry-run succeeded. Django remained failed/unverified (`make_target_missing`); Seaborn blocked after failed verify and no-change replan; Xarray triggered RC-50 `batch-1-proof-repair` but still ended low-confidence because final probe metadata lacked the missing soft `contract_refs`, motivating RC-51. |
| RC-51 | complete | Durable proof criteria and probe-ref binding: controller-owned batch purpose/expected paths/success criteria now persist across append/split/replan/hydration, and authorized proof-follow-up plans deterministically bind criteria `contract_ref=` IDs to exactly one verification probe after validating against typed behavior contracts. Multi-probe and unauthorized plans remain unchanged. Verification: focused controller/type/writeflow regressions, related packages, `go test ./...`, `make test`, `git diff --check`, and SWE local smoke pass. |
| RC-51 smoke | complete | Single Lite rerun `pydata__xarray-4248` generated a non-empty harness-consumable prediction at `/private/tmp/codrax-swe-rc51-xarray-20260618-110606`; local verify passed and `verify_confidence_reason_codes` was empty. The run did not exercise proof-follow-up because online replan converged directly, but it exposed RC-52: workflow status could remain `in_progress` after a typed passed batch when the final controller dispatch was interrupted. |
| RC-52 | complete | Dispatch-interrupted terminalization: if every batch already has typed terminal status and finish normalization does not request a follow-up batch, controller cancellation after local verify now completes the run deterministically with `controller_dispatch_interrupted_after_complete`. A cloned normalization preserves proof/impact follow-up red lines. Verification: focused orchestrator regressions pass; full regression evidence is recorded with RC-51 implementation verification. |
| RC-53 | complete | Local acceptance confidence boundary: SWE-bench adapter no longer counts low-confidence verifier passes as `local_acceptance_verdict=pass/source=local_verify`; only no-downgrade `verify_status=passed` is high-confidence local acceptance, while explicit typed manual pass can accept low-confidence evidence. README now separates high-confidence local verifier pass, low-confidence local verifier pass, typed manual audit, and official score. |
| RC-54 | complete | Codrax results summary denominator guard: added dependency-free `summarize_codrax_results.py` so local SWE-bench dashboards can report non-empty patch, high-confidence local verifier pass, low-confidence verifier pass, typed manual audit, local blockers, local-verify confidence mismatches from older rows, and missing current core fields separately from official harness `resolved/*`. |
| RC-55 | complete | Multi-run Codrax result summary: `summarize_codrax_results.py` now accepts multiple result files or globs, can explicitly dedupe to the latest row per `instance_id` by file mtime, and reports input row count/path/source-line metadata so 137-instance summaries no longer require ad hoc scripts. |
| RC-56 | complete | Primary failure reason authority: aggregate verify reports now bind `FailureReasonCode` to the final primary `FailureKind`, so secondary unavailable runner signals such as `make_target_missing` remain visible as evidence but no longer overwrite red test/build/resource failure attribution. |
| RC-57 | complete | Typed SWE failure-cause taxonomy: local Codrax result summaries now group rows into typed cause categories/families such as verification proof, implementation/localization, patch semantics, environment, workflow state, probe generation, export, and accepted/manual-audit buckets. This gives low-pass-rate analysis a stable denominator without parsing logs, issue prose, or model output. |
| RC-58 | complete | Auto approval refresh after deterministic enrichment: a fresh xarray Lite run showed proof-follow-up probe-ref binding made an auto-executable plan's approval fingerprint stale, causing `approval_authority_invalid` and empty export. Controller-owned deterministic plan mutations now refresh stale auto approvals only when the fresh typed risk policy still allows `auto_execute`; manual/denied/tampered paths remain conservative. Rerunning `pydata__xarray-4248` produced a 1318-byte non-empty prediction and crossed the prior approval boundary; the official harness dry-run still needs a Python 3.10+ eval venv, while the next observed runtime gap is applied-but-unobserved interruption leaving `verify_status` absent. |
| RC-59 | complete | Applied-but-unobserved interruption closure: controller dispatch `context.Canceled` after a successful apply now runs one bounded typed completion verify before surfacing interruption, unless the orchestrator cancel token was explicitly set. The implementation reuses the budget-completion verify semantics and records `controller_dispatch_completion_verify`; green/unavailable verifier outcomes terminalize the run, while real verify failures persist normal evidence and preserve resumable applied-patch guidance. Verification: focused controller regressions and diff check pass; full regression evidence follows this batch. |
| RC-59 smoke | complete | Reran `pydata__xarray-4248` at `/private/tmp/codrax-swe-rc59-xarray-20260618-122015`: prediction export is non-empty and official harness dry-run accepts it. Workflow is now `complete` with typed verify attempts instead of missing `verify_status`; local verification remains unavailable (`parser_error`), and audit blocks on `changed_symbol_without_probe_coverage`, motivating RC-60 proof-follow-up scheduling. |
| RC-60 | complete | Proof follow-up scheduling: typed proof coverage codes from PatchReview now route to `verification_proof_followup`, criteria mark `verification_probe_required=true`, proof batches retry once then fail-loud if the plan omits `verification_probes[]`, and typed `symbol=` criteria enrich probe `changed_symbol_refs`. Ordinary impact repairs remain unchanged. Verification: focused controller regressions, related packages, full `go test ./...`, `make test`, and diff check pass. |
| RC-60 smoke | complete | Reran `pydata__xarray-4248` at `/private/tmp/codrax-swe-rc60-xarray-20260618-123244`: the workflow failed the first bounded probe, restored checkpoint, replanned, then passed two typed verification probes and exported a non-empty harness-consumable prediction. Manual audit marks the patch plausible but low-confidence because related project test surfaces remain telemetry rather than executed proof. |
| RC-61 | complete | Probe-pass continuation now runs concrete impact related-test surfaces instead of skipping them, trims that continuation to impact runner plans, preserves probe-primary infra downgrades, fixes unittest directory selector rendering, and prefers precise pytest file selectors over unittest fallback. Verification: focused `internal/tool`, related packages, full `go test ./...`, `make test`, diff check, and xarray RC-61 SWE smoke pass. |
| RC-61 smoke | complete | Reran `pydata__xarray-4248` at `/private/tmp/codrax-swe-rc61-xarray-20260618-125247`: prediction export was non-empty, official harness dry-run accepted it, verify executed the bounded probe plus scoped `xarray/tests/test_formatting.py` and `xarray/tests/test_formatting_html.py`, `verify_test_count=32`, and final PatchReview coverage was `verified`. A stale adapter confidence downgrade from earlier plan-level telemetry remains as RC-62 candidate. |
| RC-62 | complete | PatchReview confidence authority selector now makes coherent passed deliveries consume final report-plan PatchReview for confidence while preserving delivery/source-owner hard blockers and telemetry. Adapter unit tests, Python compile, local adapter smoke, and RC-61 xarray artifact recompute pass; a fresh xarray smoke correctly failed local acceptance after scoped impact tests caught a narrower bad patch, exposing a separate replan-quality follow-up. |
| RC-63 | complete | Typed failure-signal handoff now projects failed verify observations into compact assertion/location/signal rows in `VerifyFailureHandoff` and `WriteContextPack`, renders them before raw failing-test details in planner replan prompts, and keeps all routing tied to typed report verdicts. Focused tests, related package tests, full `go test ./...`, and `make test` pass. |
| RC-63 smoke | complete | Reran `pydata__xarray-4248`: first verify failed with typed probe evidence about truncated unit labels, replan completed, final verify passed 32 scoped tests, final PatchReview was verified, and local acceptance became `pass/source=local_verify`. |
| RC-64 | complete | Adjacent duplicate inserted block PatchEffect detection now hard-blocks structural duplicate source additions. Focused writeflow tests, related regressions, full `go test ./...`, and `make test` pass. A fresh SymPy smoke did not reproduce the duplicate block and stayed `predicted_unverified` because local proof was unavailable, preserving conservative acceptance semantics. |
| RC-65 | complete | Structured plan repair now carries unique typed relocation candidates for wrong-path `old_text_mismatch` and controller retry hints render them; canceled plan batches now terminalize as blocked with `plan_batch_canceled`. Focused, related, full `go test ./...`, `make test`, `make`, and `git diff --check` pass. Targeted Django rerun moved the pre-fix `empty_patch` / stale `in_progress` run to a non-empty harness-consumable prediction with `workflow_status=complete`; local verify remains failed and is tracked as the next verifier/coverage-attribution gap. |
| RC-66 | complete | Command-derived failure reasons now carry typed failure-kind attribution, so secondary unavailable runner signals such as `make_target_missing` cannot become the primary reason for a red `tests_failed` report. Focused tests, full `internal/tool`, full `go test ./...`, `make test`, and `git diff --check` pass. |
| RC-67 | complete | Impact runner plans now filter Python package-marker `__init__.py` paths and render Django related test paths as runner-native labels via the existing `djangoSuiteSelector`; focused selector tests, full `internal/tool`, full `go test ./...`, `make test`, and `git diff --check` pass. |
| RC-68 | complete | Observation authority now classifies mixed red-test plus secondary no-tests/unavailable evidence as failed/replan instead of unverified/finish. Focused writeflow tests, related packages, full `go test ./...`, `make test`, and `git diff --check` pass. Targeted Django smoke confirmed failed suite evidence now restores checkpoint and replans instead of terminalizing as unverified; the run exposed a follow-up probe-construction/interruption gap. |
| RC-69 | complete | Typed cancellation source now distinguishes public user cancellation from write-mode wall-clock deadline cancellation. The write deadline terminalizes controller dispatch interruption as blocked both after an applied failed patch and after a plan-before-apply interruption, while user/unknown cancellation preserves resumability. Focused/related/full regressions, `make test`, `make`, and diff check pass. Targeted Django smoke now exports a non-empty harness-consumable prediction with `workflow_status=blocked` instead of `workflow_in_progress_empty_patch`; local correctness still fails and is tracked as planner edit-affordance/probe-fixture follow-up. |
| RC-70 | complete | Structured edit compilation now relocates repeated `old_text` anchors only when current same-file bytes prove exactly one match inside a bounded local window around the submitted line. This reduces `emit_change_plan` retry loops for nearby stale line numbers without reading prompt prose, user issue text, stdout narrative, or `<think>`. Focused relocation tests, related packages, and full `go test ./...` pass. |
| RC-71 | complete | Python verification probes now separate probe-authored top-level exceptions from product-code runtime failures using the wrapper's typed `probe_top_level` status. Top-level probe exceptions become unavailable probe-authoring diagnostics; product-frame exceptions remain red `tests_failed`. Focused probe-boundary tests, related packages, and full `go test ./...` pass. |
| RC-72 | complete | Actual-diff nested collection branch exclusion signal is now a multi-language provider event, not a Python-only or Django-specific patch. Newly skipped nested collection branches become soft semantic coverage obligations with `impact_kind=effect_followup`, preserving automation while requiring bounded proof/replan before high-confidence local acceptance. Verification: focused provider tests, writeflow package, related packages, full `go test ./...`, `make test`, `make`, and diff check pass. |
| RC-73 | complete | Python actual-diff Patch Critic now flags added function-body returns that leave later same-function statements unreachable. This closes the RC-72 Django smoke gap where replan prepended a new method body before the old body, local acceptance failed correctly, but PatchReview lacked a structural hard event for the exported bad patch. Verification: focused Python provider tests, writeflow package, related packages, full `go test ./...`, `make test`, `make`, and diff check pass. |
| RC-74 | complete | Plan path-state validation now rejects typed `ChangePlan` mismatches before apply: `create` for an existing file/directory, `modify`/`patch`/`rename` source for a missing or directory path, rename destination collisions, repo-boundary escapes, and directory deletes. This closes the RC-78 Django partial-apply/stale-approval class where an existing test file was planned as `create`, the source file applied, test-file create failed, and the workflow blocked with an empty exported patch. Verification: focused `emit_change_plan` path-state tests and full `internal/tool` package pass. |
| RC-75 | complete | Verify scoped selector handoff now persists `ExecutedCommand.Suite` and falls back to that command-level selector when failure rows are multiple concrete cases under the same prior suite. This closes the RC-79 Django regression where replan verify inherited runner/framework/cwd but an empty suite widened to all 13040 tests, surfacing unrelated host-version failures. Verification: focused handoff inheritance tests and tool regression pass. |
| RC-81 | complete | Applied-patch interruption policy now keeps failed-verify repair lanes resumable when typed handoff and recovery refs are present, even under `write_deadline`. Non-cancel planner failures and no-evidence deadline interruptions still block. Focused scheduler tests, related packages, full `go test ./...`, `make`, diff check, and targeted Django RC-81b SWE smoke pass the state-kernel objective: non-empty prediction, `workflow_status=in_progress`, `batch.status=ready_to_plan`, and latest progress `plan_batch_interrupted_after_applied_patch_resumable`. |
| RC-82 | complete | PatchReview hard/error diagnostics are now a replacement-patch-only repair lane. A passed functional probe or no-change sentinel cannot clear structural actual-diff failures such as unreachable code; the planner removes `run_tests` for the whole typed replacement lane, forcing bounded source reads plus a replacement ChangePlan or continued needs-replan state. Controller dispatch write-deadline interruptions after a failed-verify repair plan is persisted stay resumable instead of becoming terminal blocked. Focused tests, related packages, full `go test ./...`, `make`, diff check, and targeted Django RC-83 SWE smoke pass the intended state-kernel behavior: non-empty prediction, `workflow_status=complete`, `verify_status=passed`, and harness-shaped predictions JSONL. |
| RC-83 | complete | Impact related-test precision and coverage aliasing: moved package-marker/generic-stem filtering from only runner selection into the repomap provider boundary, and made coverage projection consume typed executed command suite selectors as normalized path/module aliases across Python/Django and Java-class style runners. Focused/related/full Go regressions and build pass. Targeted Django smoke exported a non-empty harness-shaped prediction and exposed RC-84 stale source-owner provenance, which is now fixed in the adapter. |
| RC-84 | complete | Restore-aware SWE delivery provenance: the SWE adapter now reads typed workflow `checkpoint_restored_before_replan` progress events and excludes applied source plans before the latest restore in that batch, so rolled-back PatchReview hard errors cannot pollute final delivery audit. Adapter regressions pass and old RC-84 artifact recomputes to a single final source-owner plan. |
| RC-96 | complete | SWE final-report projection now uses the typed delivery candidate's primary source plan path instead of undefined post-processing locals, eliminating false `status=error` rows after Codrax has already exported a non-empty patch. The RC-95 three-instance rerun validates 3/3 non-empty predictions and official harness import/dry-run consumption, while manual audit keeps the broader exploration/localization gap open for subsequent typed-localization work. |
| RC-97 | complete | Shared source-localization owner anchors: introduced a read/write typed `SourceLocalizationAnchor` contract so read-mode Turn A distinguishes read-file observation from grounded line-backed owner evidence, write-mode context packs carry typed anchor objects, and plan localization gates prefer precise anchors over broad target-file lists without parsing model prose or stdout. Focused/related tests and full `go test ./...` pass. |
| RC-98 | complete | Anchor relevance filter: post-RC97 SWE smoke showed broad deterministic `concrete_value` evidence can become supporting localization anchors even when it is not issue-owner evidence. Anchor generation now keeps broad deterministic facts as evidence refs but prevents them from satisfying owner-localization gates unless they carry typed `context_role=defining` authority. Focused/related/full Go regressions pass. |
| RC-99 | complete | Coherent delivery proof aggregation: final reports now aggregate typed verification proof artifacts across completed current workflow batches, so proof-follow-up batches can satisfy missing contract/symbol refs without parsing verifier prose or historical attempts. Focused type/orchestrator tests, related write packages, full `go test ./...`, and `make` pass. |
| RC-100 | complete | Planner materialization convergence: active workflow expected paths and write-analysis scope anchors now count as typed localization material, suppressing broad rediscovery and allowing one bounded post-`run_tests` structured emit window before the hard cap. Focused planner tests, related write/read-isolated packages, full `go test ./...`, and `make` pass. |
| RC-101 | complete | Patch Critic quality-shape expansion: production-source non-ASCII comment and nearby duplicate-assignment events are now emitted from typed diff/file bytes only and consumed as PatchReview soft/unknown coverage. Focused, related, full Go regressions and build pass. |
| RC-102 | complete | Docstring/text-region executable insertion guard: Python provider now emits `python_docstring_section_executable_added` as a PatchReview hard event when added executable-looking statements disrupt structured docstring section shape. Focused, related, full Go regressions and build pass. |
| RC-103 | complete | Verify-failure repair read affordance: split typed failed-verify repair planning from initial handoff synthesis so a bounded read/search window remains available after build/test failure evidence, without reopening ordinary exec or broad exploration. Focused planner tests, related write/read packages, full `go test ./...`, `make`, and `git diff --check` pass. |
| RC-104 | complete | Shared localization owner/evidence anchor closure: preserve prior owner/supporting/scope anchors through write plan localization review and downstream context packs, so later controller/planner/verifier consumers see typed anchor evidence instead of only path sets. Focused types tests, related write/read consumers, full `go test ./...`, `make`, and `git diff --check` pass. |
| RC-105 | complete | Cumulative PatchEffect owned-path boundary: limit cumulative actual-diff review to durable applied plan-owned paths, so verify/build generated artifacts do not become plan hard blockers or patch-review scope evidence. Focused/related/full Go regressions, `make`, and `git diff --check` pass. |
| RC-106 | complete | Same-batch source-owner relation after test-only replan: restore-aware delivery lineage now preserves/export earlier source-owner plans when a later test-only replan verifies the batch without re-editing production code, while still excluding stale pre-restore plans when no precise restored checkpoint relation exists. Focused Go and SWE adapter regressions pass. |
| RC-107 | complete | Shared typed localization owner/evidence scheduling authority: RC107-A added ranked owner-anchor views and planner consumption; RC107-B preserves `owner_symbol` across read/write typed handoff artifacts; RC107-C records selected owner-anchor IDs on plans and projects plan/handoff owner anchors into final reports; RC107-D infers owner symbols from typed `source_path:symbol` evidence subjects; RC107-E adds owner-depth critique for path-covered but owner/evidence-missing plans; RC107-F writes planner read observations back into durable localization anchors; RC107-G makes controller planning/exploration seeds prefer typed owner/evidence anchors before broad expected paths; RC107-H stamps read-mode final answers with typed owner anchors and renders a compact localization supplement; RC107-I projects unresolved owner-anchor gaps into typed final reports and residual risks. |
| RC-108 | complete | Apply checkpoint owned-path boundary: RC106 smoke showed generated build artifacts can enter the apply commit itself before cumulative review. Apply checkpoint commits now stage only typed plan-owned paths instead of `git add -A`, preventing unowned generated files from becoming PatchEffect hard blockers. Full regressions pass; RC108 smoke produced 3/3 non-empty predictions and no generated-path PatchEffect blocker. |
| RC-109 | complete | Verification environment/probe unavailable authority: typed unavailable reason-code helpers, report normalization, observation authority, and `run_tests` aggregation now classify dependency/probe unavailable evidence as unverified instead of product-code failure when there is no primary red source failure. Focused/full regressions and RC109 SWE smoke passed. |
| RC-110 | complete | Auditable partial final reports: persist typed final reports for non-terminal workflows once apply/verify evidence exists, add `workflow_nonterminal` residual risk, export final-report owner-gap telemetry in the SWE adapter, and reran the failed Django spot case to confirm failed-verify `in_progress` deliveries no longer fall back to prose/log audit. Focused Go/Python tests, full `go test ./...`, `make`, diff check, prediction validation, and official harness dry-run pass. |
| RC-111 | in progress | Shared localization owner/evidence pre-plan authority: RC111-A added a shared typed `LocalizationRequirement` projection, write pre-plan/exploration/replan consumption, plan-context persistence, read-source projection tests, and hygiene coverage proving plan narrative/prose does not drive requirements. RC111-B adds a one-shot pre-apply bounded read-only exploration when an open owner-localization requirement remains. Remaining work is runtime read-mode owner-discovery final handoff and vague-symptom measurement. |
| RC-112 | complete | SWE delivery PatchReview authority: adapter acceptance now scopes actual-diff/effect hard blockers to the typed primary delivery source plan, while proof blockers still use the typed report-plan authority. Stale source-owner PatchReview findings remain explicit non-authoritative telemetry and no longer block or downgrade final delivery when the exported patch comes from a later clean source plan. |
| RC-113 | complete | Failed-test assertion preservation: verifier failure handoff now promotes the failed test's typed file:line assertion into a temporary replan-only protected test contract. Replans may still fix production code or add tests, but deleting/replacing the exact failed assertion line triggers the existing bounded test-contract critique instead of silently weakening the local regression oracle. Python traceback `File "...", line N` locations now feed the shared failure-signal parser. |
| RC-114 | complete | Verifier worktree path normalization: RC113 smoke showed traceback locations can point at `.codrax/worktrees/<trace>/...`, which made failed-test protection classify the location as a Codrax artifact instead of a repo test. The critic now maps that deterministic worktree prefix back to repo-relative paths before path-role classification. |
| RC-115 | complete | Protected-test critic hard closure: the typed test-contract critic now retries once with a structured hint, then blocks/fails loud if the next plan still weakens protected regression or failed-verifier assertion lines. This prevents a model from acknowledging `preserve_failed_test_assertion` in prose while still applying the weakened test patch. |
| RC-116 | complete | Protected-oracle source-only repair lane: RC115 correctly blocks repeated protected-test weakening, but Django smoke showed the workflow then exports an empty patch instead of spending one more bounded attempt on implementation-side alternatives. The scheduler now adds a deterministic second retry lane: after the first protected-test hint is ignored, it forces one source-only/protected-path-forbidden replan before the final block, and preserves inner-loop durable progress when returning to the controller. |
| RC-117 | complete | Optional follow-up terminal normalization: RC116 Django smoke generated a coherent, source-only, locally verified patch, but a later proof/impact obligation follow-up left the workflow `blocked` and downgraded local acceptance. Interrupted optional follow-up batches now complete as `unverified` when all primary source batches are already complete, the follow-up has no applied work, and there is no typed failure handoff. |

## 2026-06-18 RC-74 Plan Path-State Pre-Apply Gate

- Evidence:
  - SWE-bench Lite targeted run `django__django-11742` at
    `/private/tmp/codrax-swe-rc78-django-20260618-rc78-django-cumulative`
    ended with `workflow_status=blocked`, `plan_status=partially_applied`,
    `prediction_verdict=empty_patch`, and
    `prediction_audit_block_reason=workflow_blocked_empty_patch`.
  - The plan combined a valid production patch with
    `kind=create` for `tests/model_fields/test_charfield.py`, but that test
    file already existed in the checkout. Apply wrote the production file,
    rejected the test-file create, and the next controller pass hit a stale
    approval boundary before cumulative patch review could converge.
- Generalized rule:
  - `emit_change_plan` validates current path state before any apply/export
    path. The gate consumes only typed `ChangePlan` fields
    (`kind`, `path`, `new_path`) plus repo-relative filesystem state under
    `BusContext.RepoRoot`.
  - `create` must target an absent path. `modify` and `patch` must target an
    existing regular file. `rename` source must be an existing regular file and
    destination must be absent. `delete` stays idempotent for missing paths but
    refuses directories.
  - Boundary escapes, stat failures, and directory/file mismatches return a
    structured `PlanRepairPack`, so planner retry can change the typed kind or
    path without mining rejection prose.
- Prompt and hard-gate hygiene:
  - No user issue keywords, model rationale, natural-language summary,
    stdout/stderr narrative, or visible `<think>` text participates in the
    decision.
  - This is a plan-time hard gate that prevents partial apply; it does not
    change read/log/trace/data/operation/computer modes or approval policy.
- Task list:
  - [x] Add `validatePlanPathStateWithRepair` to the shared
    `emit_change_plan` full-content validation chain.
  - [x] Cover create-existing, modify/patch-missing, and rename-destination
    collision regressions.
  - [x] Update existing probe tests with realistic current-file fixtures now
    that source changes must reference existing files.
  - [x] Document the path-state hard gate in architecture and user guides,
    including the synchronized HTML guide.
- Verification:
  - Focused path-state tests passed:
    `go test ./internal/tool -run 'TestEmitChangePlan_(RejectsCreateForExistingFile|RejectsModifyAndPatchForMissingFile|RejectsRenameDestinationExisting|RepairsStringWrappedChangesArray)' -count=1`.
  - Tool package regression passed:
    `go test ./internal/tool -count=1`.

## 2026-06-18 RC-75 Command-Level Verify Suite Handoff

- Evidence:
  - SWE-bench Lite targeted run `django__django-11742` at
    `/private/tmp/codrax-swe-rc79-django-20260618-rc79-django-path-state`
    confirmed RC-74: prediction export became non-empty (`patch_bytes=2257`)
    instead of the prior empty patch, and the first failed verify correctly
    restored the checkpoint and replanned.
  - The next failure was a verify-scope regression. The first verify executed
    `python3 tests/runtests.py invalid_models_tests.test_ordinary_fields -v 1`
    and produced two concrete failures inside that module. The following verify
    inherited only `runner=python`, `framework=django`, and `working_dir=.`;
    the command-level suite was not persisted, so `suite=""` widened to
    `python3 tests/runtests.py -v 1` and ran 13040 tests, surfacing unrelated
    Python 3.11/host-version failures in mail and validators.
- Generalized rule:
  - `ExecutedCommand` is now the durable authority for the exact selector that
    was executed. It carries `suite` alongside runner/framework/cwd/command.
  - Scope inheritance first uses a single reusable failing-test suite. If
    failure rows are multiple concrete cases but the prior command has a real
    reusable suite selector, the next verify inherits the command-level suite.
  - Empty suite is not treated as a scoped selector. Synthetic/aggregate labels
    such as `unittest`, `verification_probe/*`, `runner_missing`, `py_compile`,
    `build`, and `make-test` remain evidence only and are never converted into
    a runner selector.
- Prompt and hard-gate hygiene:
  - The decision consumes only typed `VerifyFailureHandoff.Executed`,
    `ExecutedCommand.Suite`, and `TestResult.Suite`. It does not parse command
    strings, runner output, user issue text, model prose, or `<think>`.
  - The change is runner-general: it applies to any runner that records a real
    suite selector, not only Django/Python.
- Task list:
  - [x] Add `suite` to `types.ExecutedCommand`.
  - [x] Populate `ExecutedCommand.Suite` from each `runnerPlan.Suite` for real
    execution, syntax preflight, syntax fallback, synthetic no-tests, and
    suite-continuation rows.
  - [x] Teach verify scope inheritance to fall back to command-level suite
    when failing-test suites are dispersed.
  - [x] Add focused regression for dispersed failure rows under one command
    suite.
  - [x] Update architecture and user guides, including synchronized HTML.
- Verification:
  - Focused inheritance test passed:
    `go test ./internal/tool -run 'TestRunTests(InheritsScopedSuiteFromVerifyFailureHandoff|DoesNotInventSuiteForAmbiguousFailureHandoff|InheritsCommandSuiteWhenFailureRowsAreDispersed|DoesNotInheritScopeAcrossAmbiguousExecutedCommands)' -count=1`.
  - Targeted RC-80 Django rerun at
    `/private/tmp/codrax-swe-rc80-django-20260618-rc80-django-command-suite`
    exported a non-empty prediction (`patch_bytes=2863`) and verified the
    selector fix in live logs: after the first failure,
    `run_tests` inherited
    `suite="invalid_models_tests.test_ordinary_fields"` and executed
    `python3 tests/runtests.py invalid_models_tests.test_ordinary_fields -v 1`.
    The run no longer executed the RC-79 all-suite
    `python3 tests/runtests.py -v 1` / 13040-test path. Final local
    acceptance still failed because PatchReview correctly found
    `python_duplicate_symbol_added` in a generated test file; that is tracked
    as a planner/patch-quality follow-up, not a selector handoff failure.

## 2026-06-18 RC-76 Proof Follow-up Probe-only Lane

- Evidence:
  - The Claude-Code-like online convergence target is `Edit -> Run -> Observe`
    in small slices, but not every observed gap needs another source edit. A
    typed proof gap such as `verification_probe_missing_soft_contract_ref` or
    `verification_probe_missing_changed_symbol_ref` can mean "rerun or relabel
    the bounded probe against the already-applied worktree", not "invent a new
    file diff".
  - Current `emit_change_plan` still rejected `changes: []` outside the older
    failed-verify no-change sentinel. Therefore controller-owned
    `verification_proof_followup` batches were forced through the source-edit
    ChangePlan lane, which can pressure the planner into no-op structured edits
    or harmless-looking doc/comment patches just to satisfy the non-empty
    `changes[]` schema.
  - RC-76 focused SWE rerun confirmed the adjacent mixed-obligation shape:
    `pydata__xarray-4248` generated a non-empty prediction, but the
    `impact_and_verification_proof_followup` batch added only verification
    comments to `xarray/core/formatting.py`. The first planning rejection log
    already showed the model understood this was proof-only work, but the
    schema pressure still pushed it toward a production comment patch.
- Generalized rule:
  - Ordinary write plans still require at least one file change.
  - A probe-only plan is accepted only when all of these typed conditions hold:
    write mode, plan stage, active durable workflow batch purpose is
    `verification_proof_followup` or `impact_and_verification_proof_followup`,
    the run ledger contains `verification_proof_followup_requested`, and the
    payload includes at least one valid `verification_probes[]` entry.
  - Accepted probe-only plans use the existing internal
    `PlanStatusNoChangeRequired` sentinel and carry durable batch
    `expected_paths` into `target_paths`, but they do not enqueue source file
    applies.
  - For proof-follow-up batches that do produce a ChangePlan with source
    changes, deterministic acceptance rejects production patches that only add
    comments or blank lines. The retry hint instructs the planner to either
    emit a real executable source/test repair or use the probe-only lane.
  - Pure `verification_proof_followup` batches cannot modify production source
    unless the active batch has a typed verification-failure handoff. A missing
    proof record means "observe the already-applied worktree"; a failed typed
    probe can still become a normal executable repair through the existing
    failure handoff lane. Mixed `impact_and_verification_proof_followup`
    batches keep their source-repair lane, but still cannot add comment-only
    production patches.
  - Controller scheduling recognizes this plan as a proof lane and moves the
    active batch directly to `verifying`; if the controller later emits
    `finish`, `apply_plan`, `replan_batch`, or another delaying action before a
    verdict, deterministic normalization overrides it to `verify_batch`.
- Prompt and hard-gate hygiene:
  - No user issue keywords, model rationale, summary prose, stdout/stderr text,
    or `<think>` content drives the decision.
  - The hard authorization reads only typed workflow state, batch purpose,
    progress reason code, and schema-validated `verification_probes[]`.
  - The comment-only guard reads only typed ChangePlan changes and diff line
    prefixes for production paths; it does not parse issue text, model
    rationale, or output prose.
  - This separates proof metadata/observe work from source-edit work without
    weakening the stable code-change path.
- Task list:
  - [x] Add a controller-authorized proof-follow-up probe-only sentinel to
    `emit_change_plan`.
  - [x] Keep ordinary probe-only/empty-change plans rejected.
  - [x] Teach controller plan scheduling to mark proof-probe-only plans as
    `verifying` instead of applying them.
  - [x] Add deterministic normalize/recovery fallback that forces
    `verify_batch` before terminal or replanning actions.
  - [x] Reject proof-follow-up production patches that only add comments or
    blank lines, and retry once toward executable repair or probe-only proof.
  - [x] Reject pure proof-follow-up production source edits when there is no
    typed verify-failure handoff for that batch.
  - [x] Add focused tool and controller regressions.
- Verification:
  - Focused tests added:
    `TestEmitChangePlan_ProofFollowupProbeOnlyPlanAccepted`,
    `TestEmitChangePlan_ProbeOnlyPlanRejectedOutsideProofFollowup`,
    `TestRunControllerPlanBatch_ProofFollowupAcceptsProbeOnlyPlan`,
    `TestRunControllerPlanBatch_PureProofFollowupRetriesProductionSourcePatchToProbeOnly`,
    `TestRunControllerPlanBatch_PureProofFollowupAllowsProductionRepairWithFailureHandoff`,
    `TestRunControllerPlanBatch_ProofFollowupRetriesCommentOnlyProductionPatchToProbeOnly`, and
    `TestNormalizeControllerTypedStateDecisionProofProbeOnlyPlanVerifiesBeforeFinish`.
  - Focused RC-76 SWE rerun:
    `/private/tmp/codrax-swe-rc76-xarray-20260618-proof-probe-only`
    exported a non-empty prediction and generated an official harness command;
    it also exposed the mixed-obligation comment-only production patch that
    this RC now guards.
  - Follow-up RC-76b SWE rerun:
    `/private/tmp/codrax-swe-rc76b-xarray-20260618-proof-probe-only`
    exported a non-empty prediction and generated an official harness command.
    The exported patch no longer contained verification comments, but it still
    showed a pure proof batch could make additional executable production
    source edits without a typed failed probe. The pure-proof source-edit guard
    closes that state-machine gap.
  - Final RC-76c SWE rerun:
    `/private/tmp/codrax-swe-rc76c-xarray-20260618-proof-source-boundary`
    exported a non-empty prediction and generated an official harness command.
    The first batch applied one production patch, then the controller appended
    `verification_proof_followup`; that proof batch produced
    `proof_probe_only_plan_ready`, was normalized to `verify_batch`, and added
    no production source diff. Local acceptance remained `fail` because the
    local project environment made verification unavailable and typed
    changed-symbol proof stayed uncovered; that is intentionally not counted as
    functional SWE success.
  - Full verification for this batch is recorded in the commit that lands this
    RC.

## 2026-06-18 RC-77 WriteAnalysisIR Contract Quarantine

- Evidence:
  - RC-76c `pydata__xarray-4248` showed that low SWE quality is not only a
    verifier dependency problem. The first write-analyzer attempt produced
    exact but under-grounded output fragments and was correctly rejected. The
    second attempt produced a useful `WriteAnalysisIR` with raw request,
    scope anchors, phase proposal, and behavior contracts, but one auxiliary
    `not_raises` contract carried an ungrounded prose payload. The orchestrator
    rejected the entire IR and installed fallback analysis, losing all
    structured behavior contracts and phase proposal evidence.
  - The resulting ChangePlan had `behavior_contracts=null`; probes checked only
    broad substring presence (`in metres`, `in degC`) and did not carry the
    stricter output-shape contract (`name, in units (dims) dtype`) forward.
- Generalized rule:
  - Keep the existing first-attempt retry: hard exact operators still require a
    value present verbatim in `raw_request` or a grounded comparator/evidence
    ref.
  - On the final write-analysis attempt, do not degrade the whole IR when the
    failure is limited to ungrounded exact behavior contracts. Deterministically
    quarantine those contracts by converting their operator to `satisfies` and
    tagging their source as quality-repaired soft guidance.
  - Valid contracts, scope anchors, constraints, expected outcomes, risk, and
    phase proposal remain intact. Ungrounded exact values never become P0 hard
    gates; they become soft planning/verification guidance only.
- Prompt and hard-gate hygiene:
  - The repair reads only typed `WriteAnalysisIR` fields and verbatim
    `raw_request` substring grounding. It does not parse issue intent keywords,
    model rationale, natural-language summaries, stdout/stderr, or visible
    `<think>` text.
  - This is a unified structural repair layer for model tool JSON, not a
    case-specific patch for xarray.
- Task list:
  - [x] Add deterministic `WriteAnalysisIR` quality repair that softens only
    ungrounded exact behavior contracts.
  - [x] Keep retry semantics for first-attempt under-grounded contracts.
  - [x] Use repaired IR instead of fallback on final-attempt partial contract
    quality failures.
  - [x] Add orchestrator regressions proving useful IR fields survive partial
    contract quarantine.
  - [x] Re-run focused xarray SWE smoke to confirm ChangePlan carries
    behavior contracts again.
- Verification:
  - Focused tests added:
    `TestRepairWriteAnalysisIRQualitySoftensOnlyUngroundedExactContracts` and
    `TestRunWriteAnalyzePhaseRepairsFinalAttemptUngroundedContractInsteadOfFallback`.
  - Related package verification passed:
    `go test ./internal/orchestrator ./internal/tool ./internal/types ./internal/writeflow -count=1`.
  - Focused RC-77 SWE rerun:
    `/private/tmp/codrax-swe-rc77-xarray-20260618-ir-contract-quarantine`
    exported a non-empty prediction (`patch_bytes=686`) and generated an
    official harness command. The final ChangePlan carried five
    `behavior_contracts` instead of `null`, proving the useful IR survived a
    partial contract quality issue.
  - The same run exposed the next candidate gap: the appended
    `verification_proof_followup` batch spent too long re-reading and
    re-questioning the already-applied worktree, then ended
    `plan_batch_canceled`. That is tracked separately as proof-follow-up
    planning budget/direct-synthesis work; RC-77 intentionally does not mix
    that scheduler refinement into the IR repair batch.

## 2026-06-18 RC-78 Proof Follow-up Materialization Lane

- Evidence:
  - RC-77 restored behavior contracts, but the follow-up
    `verification_proof_followup` batch spent hundreds of seconds re-reading
    the already-applied worktree. The planner saw the newly applied code,
    questioned whether the feature was already implemented, attempted a dry-run
    probe that failed due missing local dependencies, then continued reading.
    The batch finally ended `plan_batch_canceled` without a proof plan.
  - Existing planner code already has a materialization-only tool surface after
    typed exploration handoff: once read budget is exhausted, it narrows to
    `run_tests` dry-run probes and structured emit tools. Pure proof follow-up
    batches lacked the typed signal that activates this lane because they are
    not exploration handoffs.
  - RC-78 first smoke (`/private/tmp/codrax-swe-rc78-xarray-20260618-proof-materialization`)
    showed a second boundary bug: the first proof-follow-up dispatch narrowed
    to four tools, but the model emitted a production-source patch; the typed
    `old_text_mismatch` repair path then reopened `read_file`/`grep` and pulled
    the proof batch back into investigation mode. The run was stopped after the
    stall was proven in logs.
- Generalized rule:
  - A controller-owned pure `verification_proof_followup` batch is not a new
    investigation. It is a materialization step over an already-applied
    worktree: emit `changes: []` plus `verification_probes[]`, or fail fast
    with a typed plan-emission error.
  - The planner should activate the existing handoff/materialization surface
    from the first iteration when the durable workflow active batch purpose is
    `verification_proof_followup` and the run ledger contains
    `verification_proof_followup_requested`.
  - A pure proof-follow-up remains materialization-only even after a structured
    emit validator rejection. The structured emit repair layer may reopen exact
    reads for ordinary handoff planning, but proof-only batches must recover by
    emitting a probe-only plan (`changes: []`, `verification_probes[]`) or by
    failing fast for controller redispatch.
  - Mixed `impact_and_verification_proof_followup` is not narrowed this way
    because it may still need executable repair.
- Prompt and hard-gate hygiene:
  - The hard routing reads only typed workflow run fields: active batch id,
    batch purpose, and progress reason code. No user keywords, issue prose,
    model rationale, summaries, stdout/stderr, or `<think>` output are parsed.
  - This reuses the existing planner materialization surface and does not
    introduce a separate proof planner or duplicated tool policy.
- Task list:
  - [x] Teach planner handoff detection to treat authorized pure proof
    follow-up batches as materialization work.
  - [x] Give pure proof follow-up a zero read budget so schema narrows on the
    first planner iteration.
  - [x] Keep pure proof follow-up on the materialization surface after
    `emit_change_plan`/structured emit rejection; do not reopen read tools from
    the generic repair layer.
  - [x] Render a typed proof-materialization prompt section from the durable
    active workflow batch, so the model sees the proof-only shape without any
    user-intent keyword routing.
  - [x] Add planner tool-surface regression proving read tools are removed but
    `run_tests` and emit tools remain.
  - [x] Re-run focused proof-follow-up tests and relevant package tests.
- Verification:
  - Focused planner tests passed:
    `TestPlannerFilterToolSchemas_PureProofFollowupMaterializesImmediately`,
    `TestPlannerFilterToolSchemas_PureProofFollowupKeepsMaterializationAfterEmitReject`,
    `TestPlannerBuildInitialInstruction_RendersProofFollowupMaterializationSection`,
    `TestPlannerFilterToolSchemas_MixedProofImpactFollowupKeepsReadBudget`,
    `TestPlannerFilterToolSchemas_HandoffSynthesisExhaustsReadBudget`, and
    `TestPlannerFilterToolSchemas_StructuredEmitRepairKeepsReadTools`.
  - Focused controller proof-lane tests passed:
    `TestRunControllerPlanBatch_ProofFollowupAcceptsProbeOnlyPlan`,
    `TestRunControllerPlanBatch_PureProofFollowupRetriesProductionSourcePatchToProbeOnly`,
    `TestRunControllerPlanBatch_PureProofFollowupAllowsProductionRepairWithFailureHandoff`,
    `TestRunControllerPlanBatch_ProofFollowupRetriesCommentOnlyProductionPatchToProbeOnly`, and
    `TestNormalizeControllerTypedStateDecisionProofProbeOnlyPlanVerifiesBeforeFinish`.
  - RC-78b focused SWE smoke
    (`/private/tmp/codrax-swe-rc78b-xarray-20260618-proof-materialization`)
    produced a non-empty prediction (`patch_bytes=691`) and an official harness
    command. The proof batch completed quickly instead of stalling:
    `batch-1-proof-repair` emitted `plan-1781785487524167000-78149` with
    `changes=[]`, `verification_probes=2`, and workflow progress
    `proof_probe_only_plan_ready`.
- Remaining gap exposed by RC-78b:
  - The adapter still reports `local_acceptance_verdict=fail` with
    `prediction_audit_block_reason=patch_review_semantic_unverified:changed_symbol_without_probe_coverage`
    when local verification is unavailable (`parser_error`,
    `make_target_missing`, `unittest_loader_import_error`). This is no longer
    a proof-planning stall; it is a follow-up acceptance/proof accounting gap:
    proof probes that cannot execute in the local environment should remain
    conservative for SWE scoring, but the workflow should carry typed
    unavailable-verifier evidence and avoid re-requesting the same proof batch.

## 2026-06-18 RC-79 Validation Follow-up Artifact Discovery

- Evidence:
  - RC-78b generated a proof-only ChangePlan, but the SWE adapter still
    reported the earlier source plan as `plan_id`/`delivery_report_plan_id`.
    Root cause: `find_latest_change_plan` accepted only artifacts whose
    `changes` field was a JSON list. Probe-only/no-change plans are valid
    ChangePlans but serialize with `changes: null` and
    `verification_probes[]`, so artifact discovery dropped them.
  - After allowing probe-only plans into discovery, RC-79 smoke correctly
    surfaced the validation follow-up plan/report, but prediction audit briefly
    reported `final_plan_exported_source_drift`: the final plan was
    validation-only, while the exported patch was owned by the earlier applied
    source plan. The adapter already computed a coherent delivery source-owner
    view but the drift check still consumed final-plan paths.
  - RC-79b fixed that metadata binding: `delivery_relation` became
    `source_plan_with_later_validation_followup`,
    `delivery_report_plan_id` pointed at the proof-only plan, and
    `delivery_source_plan_covers_exported_source_patch=true`. The prediction
    remained conservative for the real blocker:
    `patch_review_semantic_unverified:changed_symbol_without_probe_coverage`
    because verification was unavailable.
- Generalized rule:
  - Durable ChangePlan discovery must accept either source/test edits
    (`changes[]`) or typed verification materialization (`verification_probes[]`
    with no changes). Final-plan/report metadata must reflect the latest
    durable plan, even when patch export is owned by an earlier source plan.
  - Local prediction drift audit should consume the coherent delivery view
    (source-owner plan paths and coverage) rather than the final plan's empty
    changes when the final plan is a validation-only follow-up.
  - A follow-up batch that completes with typed unavailable verification and no
    failed-verification handoff is terminal for the current workflow turn. The
    controller must not append another follow-up from the same unverifiable
    observation; it should finish with `accept_unverified`.
- Prompt and hard-gate hygiene:
  - All decisions read typed artifacts: ChangePlan JSON shape, workflow
    attempts, batch purpose/status, report status/reason codes, delivery
    source-owner paths, and patch-review records. No user keywords, model
    narrative, stdout prose, or `<think>` text controls routing.
- Task list:
  - [x] Broaden SWE adapter ChangePlan discovery to include no-change
    probe-only plans via `verification_probes[]`.
  - [x] Mark delivery candidates whose final plan is validation-only and bind
    their report as the report authority while preserving source-owner patch
    ownership.
  - [x] Use delivery source-owner coverage for local drift audit when delivery
    is coherent.
  - [x] Add controller terminal override for proof/impact follow-up batches
    completed as unverified due verification infrastructure, with no failure
    handoff.
  - [x] Add Python adapter tests and focused controller tests for these paths.
- Verification:
  - Python adapter tests passed:
    `python3 -m unittest eval.swebench.run_codrax_swebench_test`.
  - Focused controller tests passed:
    `TestNormalizeControllerTypedStateDecisionProofFollowupUnverifiedDoesNotAppendImpact`,
    `TestNormalizeControllerTypedStateDecisionProofFollowupDoesNotRecurse`,
    `TestNormalizeControllerTypedStateDecisionSemanticPatchReviewAppendsFollowup`,
    `TestNormalizeControllerTypedStateDecisionVerifiedButUndercoveredAppendsImpactRepair`,
    `TestNormalizeControllerTypedStateDecisionMissingSoftProofAppendsProofFollowup`, and
    `TestCompleteDispatchInterruptedRunIfAllBatchesCompleteDoesNotSwallowProofFollowup`.
  - RC-79b focused SWE smoke produced a non-empty prediction
    (`patch_bytes=1076`), coherent delivery metadata, final validation-only
    report binding, and no `final_plan_exported_source_drift`.
  - RC-79c focused SWE smoke
    (`/private/tmp/codrax-swe-rc79c-xarray-20260618-followup-terminal`)
    rebuilt the Go binary and confirmed the controller terminal override:
    workflow progress contained `followup_unverified_terminal_overridden` and
    final reason `accept_unverified_followup_without_failure_evidence`, with no
    `workflow_transition_rejected` retry after the follow-up batch completed
    unverified. Prediction export remained harness-consumable and conservative:
    `prediction_verdict=predicted_audit_blocked` with real remaining blocker
    `patch_review_semantic_unverified:behavior_contract_without_verify_coverage`.

## 2026-06-18 RC-81 Resumable Failed-Verify Repair Interruption

- Evidence:
  - RC-80 Django smoke
    (`/private/tmp/codrax-swe-rc80-django-20260618-rc80-django-command-suite`)
    fixed the prior all-suite selector widening, but the final run still ended
    `workflow_status=blocked` with
    `workflow_latest_progress_reason_code=plan_batch_interrupted_after_applied_patch_blocked`.
  - The durable workflow already had an applied recovery ref, failed
    post-apply verification evidence, `VerifyFailureHandoff`, and an active
    replan edge. The replanner read the exact local duplicate-symbol cause, but
    the write-mode wall-clock deadline canceled the planning dispatch before a
    replacement `ChangePlan` was emitted.
  - RC-69 intentionally made write-deadline cancellation terminal to avoid
    ambiguous `in_progress` / empty-patch exports. Live RC-80 evidence shows
    that rule was too broad for Claude-Code-like online convergence: an
    already-applied failed-verify repair with typed evidence is resumable, not
    permanently blocked.
- Generalized rule:
  - Hard routing reads only typed state: cancellation source enum, active
    workflow batch, latest verify attempt status, `VerifyFailureHandoff`, and
    applied `ChangePlan` recovery metadata.
  - Non-cancel planner failures after an applied patch still fail loud and mark
    the run blocked.
  - Write-deadline cancellation without failed-verify handoff remains allowed
    to terminalize ambiguous/no-evidence states.
  - Write-deadline cancellation with an applied patch, failed verify attempt,
    and matching `VerifyFailureHandoff` preserves `run.status=in_progress` and
    `batch.status=ready_to_plan`, records
    `plan_batch_interrupted_after_applied_patch_resumable`, and lets the next
    write turn auto-resume from the typed failure evidence.
- Prompt and hard-gate hygiene:
  - No branch parses error strings, user task text, model rationale, planner
    prose, stdout/stderr narrative, or visible `<think>`. The cancellation
    reason text remains user-facing only.
  - The change is controller-state semantics only; read/log/trace/data,
    operation, and computer modes are untouched.
- Task list:
  - [x] Add a typed resumability predicate for applied-patch interruption:
    canceled error + active failed verify + matching verify-failure handoff.
  - [x] Change `appliedPatchInterruptedShouldBlock` to keep that state
    resumable even when the cancellation source is `write_deadline`.
  - [x] Record a distinct progress reason for resumable applied repair
    interruption.
  - [x] Apply the same predicate to controller-dispatch interruption after an
    applied patch so the fallback `controller_dispatch_write_deadline` block
    cannot overwrite a typed resumable repair.
  - [x] Update scheduler tests so deadline-interrupted failed-verify repair is
    `in_progress` / `ready_to_plan`, while ordinary replacement planning
    failure remains blocked.
- Verification:
  - Focused scheduler tests passed:
    `go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_Replan(WriteDeadlineAfterAppliedPatchStaysResumable|FailureAfterAppliedPatchBlocksRun|InterruptedAfterAppliedPatchPreservesAppliedPlan)' -count=1`
    and
    `go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_ControllerDispatchWriteDeadlineBlocks|TestRunWriteControllerWorkflow_ControllerDispatchInterruptedAfterAppliedPatch|TestRunWriteControllerWorkflow_VerifyFailureCanReplanSameBatch' -count=1`.
  - Related packages passed:
    `go test ./internal/orchestrator ./internal/writeflow ./internal/types -count=1`.
  - Full regression/build checks passed: `go test ./...`, `make`, and
    `git diff --check`.
  - The first RC-81 smoke at
    `/private/tmp/codrax-swe-rc81-django-20260618-resumable-repair` failed
    before write mode because the smoke command omitted `--providers`; the log
    stopped at `providers.yaml: llm.default.provider is required` and produced
    no workflow, so it is not system-behavior evidence.
  - Targeted RC-81b Django smoke at
    `/private/tmp/codrax-swe-rc81b-django-20260618-resumable-repair` used the
    provider config, exported a non-empty prediction (`patch_bytes=1971`),
    and proved the state-kernel fix:
    `workflow_status=in_progress`,
    `batch-1.status=ready_to_plan`, and
    `workflow_latest_progress_reason_code=plan_batch_interrupted_after_applied_patch_resumable`.
    The prediction remains functionally failed, correctly, because PatchReview
    found `python_unreachable_body_after_added_return` in
    `django/db/models/fields/__init__.py`; this is a separate planner
    patch-quality gap, not an interruption-state regression.

## 2026-06-18 RC-82 PatchReview Hard Failure Replacement Lane

- Evidence:
  - RC-81b correctly kept the failed-verify repair resumable, but the next
    blocker showed a deeper convergence gap. The latest typed report carried
    `VerificationDiagnostic{source=patch_review, category=structural,
    severity=error, outcome=failed,
    reason_code=python_unreachable_body_after_added_return}`.
  - The replanner treated that structural actual-diff failure like an ordinary
    functional test failure. It spent many turns running `run_tests` planner
    probes, reasoned that behavior was correct, then tried a no-op
    `emit_change_plan` for the same source file. The no-op was rejected, and
    the run hit the write deadline before a replacement patch was emitted.
  - This is a system-level authority gap: runtime/functional probes can prove
    behavior, but they cannot clear a typed PatchReview hard error over the
    actual diff. The repair lane must distinguish "tests failed" from
    "patch reviewer rejected the diff shape".
- Generalized rule:
  - `VerifyFailureHandoff.Diagnostics` is the authority. When any diagnostic
    has `source=patch_review`, `severity=error`, and failed outcome, the prior
    verify failure requires a replacement patch.
  - `no_change_required` sentinels are denied for that state even if planner
    probes pass.
  - The planner removes `run_tests` for the whole typed replacement lane. Before
    exact-read budget is exhausted it may still use bounded source-read tools to
    synthesize the replacement; after the budget is exhausted the tool surface
    keeps only structured plan emit tools. Functional probes remain available
    for ordinary red tests and proof follow-up batches.
  - If controller dispatch is interrupted by the write wall-clock deadline after
    a failed-verify repair plan has already been persisted, the run stays
    `in_progress` and the active batch stays `planned`. This is a durable-state
    resume condition, not a correctness pass.
  - The policy is language-general: it reads typed PatchReview diagnostic
    fields, not Python/Django reason strings, model prose, runner output, or
    user issue text.
- Prompt and hard-gate hygiene:
  - The hard gate lives in `writeflow.QualifyNoChangeReplanSentinel` and the
    planner schema filter. Both consume typed artifacts only.
  - The visible `<think>` log remains untouched; transparency is preserved.
  - No read/log/trace/data/operation/computer mode code is changed.
- Task list:
  - [x] Add shared
    `writeflow.VerifyFailureRequiresReplacementPatch(*VerifyFailureHandoff)`.
  - [x] Deny no-change replan sentinel when a patch-review hard/error
    diagnostic is present.
  - [x] Narrow planner materialization tool surface by removing `run_tests`
    when the same typed state is active.
  - [x] Remove `run_tests` before planner materialization too, so structural
    actual-diff repair cannot burn early turns on functional probes.
  - [x] Keep controller dispatch write-deadline interruptions resumable when a
    replacement repair plan is already attached to a failed-verify batch.
  - [x] Add focused writeflow, planner, and scheduler tests.
- Verification:
  - Focused tests passed:
    `go test ./internal/writeflow -run TestQualifyNoChangeReplanSentinel -count=1`
    and
    `go test ./internal/agent -run 'TestPlannerFilterToolSchemas_(HandoffSynthesisExhaustsReadBudget|PatchReviewHardFailureRequiresReplacementPatch|PureProofFollowupMaterializesImmediately|StructuredEmitRepairReadBudgetNarrows)' -count=1`.
  - Focused scheduler tests passed:
    `go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_DispatchWriteDeadlineAfter(PlanBlocksRun|RepairPlanStaysResumable)|TestRunWriteControllerWorkflow_(ControllerDispatchWriteDeadlineBlocks|ControllerDispatchInterruptedAfterAppliedPatch|VerifyFailureCanReplanSameBatch)' -count=1`.
  - Related and full regressions passed:
    `go test ./internal/agent ./internal/writeflow ./internal/orchestrator ./internal/types -count=1`,
    `go test ./...`, `make`, and `git diff --check`.
  - Targeted SWE smoke passed for `django__django-11742` at
    `/private/tmp/codrax-swe-rc83-django-20260618-repair-plan-resume`:
    `status=predicted`, `patch_bytes=2359`, `workflow_status=complete`,
    `verify_status=passed`, `delivery_candidate_status=coherent`, and
    `predictions.jsonl` contains the official harness-shaped
    `instance_id/model_name_or_path/model_patch` row. This is not reported as
    official SWE-bench resolved; Docker harness scoring remains the authority.

## 2026-06-18 RC-83 Impact Related-Test Precision And Coverage Aliases

- Evidence:
  - RC-83 Django smoke reached `verify_status=passed`, but post-verify local
    confidence remained weaker than the runtime evidence because impact targets
    still contained broad related-test obligations produced before runner-level
    filtering. The runner queue could drop package-marker paths such as
    `tests/**/__init__.py`, but those paths could still survive in
    `ImpactAnalysis`, `PatchReview`, P2 handoff, and coverage ledgers.
  - The same run also showed selector/coverage mismatch: a real executed typed
    command can be `suite=invalid_models_tests.test_ordinary_fields`, while the
    impact target is stored as
    `tests/invalid_models_tests/test_ordinary_fields.py`. Without a shared
    selector-to-path projection, controller coverage sees "a test passed" but
    cannot close the exact related-test obligation.
- Generalized rule:
  - Related-test discovery is tightened at the graph-provider boundary. For
    package-marker source files (`__init__.py`, `index.*`, `mod.rs`, `lib.rs`,
    etc.), the semantic parent stem is used instead of the generic file stem.
    Cross-directory package-marker test files are not emitted as inferred
    related tests. Candidate matching uses normalized path tokens and bounded
    deterministic ranking, not issue text, model prose, stdout, or manual notes.
  - Coverage projection now records aliases from structured
    `ChangeReport.TestResults[].Suite` and successful
    `ChangeReport.ExecutedCommands[].Suite`. Module selectors are mapped to
    exact comparable path aliases only when the selector itself carries typed
    test-shape structure, for example Django/Python module selectors and
    Java/Kotlin class-test selectors.
  - The fix is multi-language in shape: Python/Django module selectors can
    cover `tests/.../*.py`, while Java-style class selectors can cover
    `src/test/java/...Test.java` and Kotlin equivalents. Unsupported or
    ambiguous labels remain telemetry, not hard coverage.
- Prompt and hard-gate hygiene:
  - No prompt routing changes are required. The controller and coverage gates
    consume typed paths, suite fields, command outcomes, and exit codes only.
  - Visible `<think>` transparency is untouched.
  - Read/log/trace/data/operation/computer modes are not modified.
- Task list:
  - [x] Replace raw `strings.Contains(sourceStem(test), sourceStem(source))`
    related-test matching with semantic source stems, package-marker filtering,
    token matching, deterministic ranking, and a bounded candidate cap.
  - [x] Add provider regression for package-marker source files so unrelated
    `tests/**/__init__.py` files do not become inferred obligations.
  - [x] Add coverage aliases from passing test results and successful typed
    executed commands.
  - [x] Add regression coverage for Django/Python command selectors and
    Java-class selectors closing exact related-test obligations.
  - [x] Run focused regressions, related package tests, full `go test ./...`,
    `make`, `git diff --check`, and one targeted SWE smoke.
- Verification:
  - Focused tests passed:
    `go test ./internal/writeflow/impact -run TestGraphProvider -count=1`
    and
    `go test ./internal/orchestrator -run 'TestApplyVerifyCoverageToChangePlan(VerifiesScopedTestSurfaceCoverage|CoversModuleSelectorFromExecutedCommand|DoesNotCoverUnrelatedModuleSelector|CoversJavaClassSelectorFromExecutedCommand)' -count=1`.
  - Related and full regressions passed:
    `go test ./internal/writeflow/impact ./internal/orchestrator ./internal/tool -count=1`,
    `go test ./...`, `make`, and `git diff --check`.
  - Targeted SWE smoke for `django__django-11742` at
    `/private/tmp/codrax-swe-rc84-django-20260618-impact-coverage` exported a
    non-empty harness-shaped prediction (`patch_bytes=2447`,
    `workflow_status=complete`, `verify_status=passed`). The run also exposed
    RC-84: adapter delivery audit still included rolled-back source-owner plans
    before checkpoint restore, producing a stale
    `patch_review_error:python_unreachable_body_after_added_return` block.

## 2026-06-18 RC-84 Restore-Aware Delivery Source Ownership

- Evidence:
  - The RC-83 smoke's workflow ledger showed two
    `checkpoint_restored_before_replan` events in `batch-1`. The durable
    attempts still contained three `apply status=applied` source plans because
    attempts are append-only audit history.
  - The adapter's `workflow_applied_plan_ids()` treated every historical
    applied attempt as final source ownership. As a result, the first rolled
    back plan's `python_unreachable_body_after_added_return` hard PatchReview
    finding polluted the final delivery summary even though the exported patch
    came from the later source plan and the workflow completed after a proof
    follow-up.
- Generalized rule:
  - Workflow attempts remain append-only history. Delivery provenance must
    reconstruct final worktree ownership from typed attempts plus typed restore
    progress.
  - For each batch, a `checkpoint_restored_before_replan` progress event
    invalidates earlier applied attempts in that batch. Applied plans after the
    latest restore remain source owners; later proof-only/test-only batches can
    still validate those owners.
  - Timestamp parsing accepts Go-style RFC3339 values with variable fractional
    seconds, normalizing the fractional component before comparison.
- Prompt and hard-gate hygiene:
  - The adapter consumes only typed workflow progress/attempt fields and plan
    JSON artifacts. It does not parse controller prose, model rationale,
    visible `<think>`, test output, issue text, or manual notes.
  - Controller runtime behavior is unchanged; this batch fixes SWE export/audit
    accounting without affecting read/log/trace/data/operation/computer modes.
- Task list:
  - [x] Add typed restore cutoff extraction from workflow progress ledger.
  - [x] Filter `workflow_applied_plan_ids()` by latest per-batch restore
    timestamp.
  - [x] Normalize variable-length fractional RFC3339 timestamps before Python
    parsing.
  - [x] Add adapter regression where two stale applied source plans are rolled
    back and only the final source plan owns the exported patch.
  - [x] Recompute the old RC-83 smoke artifact with the new adapter logic:
    `source_owner_plan_ids=['plan-1781791223887175000-16315']`,
    `hard_block=False`, and no stale `patch_review_error` block.
- Verification:
  - `python3 -m unittest eval.swebench.run_codrax_swebench_test -v` passed.
  - The old RC-83 artifact recompute confirms restore-aware owner selection.

## 2026-06-18 RC-85 Durable Verify-Failure Handoff Retention

- Evidence:
  - Targeted SWE smoke for `pydata__xarray-4248` at
    `/private/tmp/codrax-swe-rc85-xarray-20260618-impact-provenance` exported a
    non-empty harness-shaped prediction, but local confidence correctly stayed
    failed: `workflow_status=in_progress`, `verify_status=failed`, and
    `workflow_latest_progress=plan_batch_interrupted_after_applied_patch_resumable`.
  - The typed verify report contained the actionable failure:
    `text_repr_with_units` still expected full unit text such as
    `x, in metres`, while the patch produced truncated output such as
    `x, in m...`.
  - The durable workflow context pack for the active batch did not retain
    `verify_failure`, `failed_test`, or `failure_signal` items after later plan
    context synchronization. Inspection showed only plan/effect/review/impact
    context. The root cause was `upsertWorkflowRunContextPack()` replacing an
    existing same-key pack (`merged|batch-1`) with the later pack, so the
    append-only workflow ledger kept attempts but the consumer-facing handoff
    view lost unresolved P2 evidence.
- Generalized rule:
  - Workflow context packs are evidence ledgers. Same-key upsert means "merge
    this newer projection into the existing scoped handoff", not "replace all
    earlier evidence for this batch".
  - Replacement is unsafe because replan, plan-context coverage, impact, and
    verify-failure projections can all share the same durable batch key while
    representing different typed facts needed by the next consumer.
  - The merge preserves pack identity for stable storage/recovery while
    deduplicating items through `NormalizeWriteContextPack`; stale cross-batch
    evidence is still filtered by `ForScope()` before sync.
- Prompt and hard-gate hygiene:
  - No prompt change is required. The fix consumes and preserves only typed
    `WriteContextPack` items, batch IDs, slice IDs, and normalized item
    fingerprints.
  - It does not inspect issue text, SWE-bench IDs, model rationale, summaries,
    visible `<think>`, stdout prose, or manual audit notes.
  - Read/log/trace/data/operation/computer mode paths remain unchanged.
- Task list:
  - [x] Change workflow context same-key upsert from replacement to
    normalized merge while preserving durable pack identity.
  - [x] Add regression coverage where a scoped verify failure pack is synced,
    then a later scoped plan pack is synced for the same active batch, and the
    planner view sees both the unresolved failure and the new target file.
  - [x] Keep stale completed-batch failure evidence out of the active batch via
    the existing scope filter regression.
- Verification:
  - Focused regression passed:
    `go test ./internal/orchestrator -run 'TestSyncCurrentWriteContextPackToRun(KeepsStaleBatchEvidenceOutOfActiveBatch|MergesPriorVerifyFailureEvidence)' -count=1`.

## 2026-06-18 RC-86 Durable Context Pack Capacity Governance

- Evidence:
  - After RC-85, a fresh targeted SWE smoke for `pydata__xarray-4248` at
    `/private/tmp/codrax-swe-rc86-xarray-20260618-context-retention` improved
    materially: `workflow_status=complete`, `verify_status=passed`,
    `prediction_verdict=predicted_passed`, `prediction_local_confidence=high`,
    and `patch_bytes=1733`.
  - The workflow shows the desired online convergence shape:
    first plan/apply/verify failed, controller restored the checkpoint, replanned
    the same batch, applied the second plan, and verified successfully.
  - A follow-up audit of the final durable merged context pack still found a
    capacity issue. The pack contained 96 items, exactly
    `writeContextPackMaxItems`, but did not retain the prior failed verify rows.
    The consumer view layer already had planner must-carry logic for verify
    failures, but the pack normalization layer had already truncated later items
    by insertion order, so the view layer never saw those rows.
- Generalized rule:
  - Durable pack normalization must be an evidence-governance layer, not FIFO
    truncation. P0 safety/context items and typed verify-failure rows must be
    eligible to displace ordinary lower-value context when the persisted pack is
    full.
  - The rule is typed and language-neutral: it reads priority, source stage,
    item kind, and normalized item identity. It does not inspect issue text,
    model prose, output logs, SWE-bench IDs, or manual notes.
  - Consumer Top-N views still do their own budgeted rendering; the new
    pack-level rule only ensures critical typed rows survive long enough to be
    available to controller/planner/verifier views and recovery/audit tooling.
- Prompt and hard-gate hygiene:
  - No prompt change is required. `<think>` transparency remains intact.
  - No new hard gate is introduced; this is durable evidence retention for
    existing typed artifacts.
  - Read/log/trace/data/operation/computer mode paths remain unchanged.
- Task list:
  - [x] Remove insertion-order early break from `NormalizeWriteContextPack`.
  - [x] Add pack-level bounded selection that retains P0 items and
    verify-failure must-carry items by replacing ordinary non-P0 context when
    the pack is full.
  - [x] Keep durable pack size bounded at `writeContextPackMaxItems`.
  - [x] Add regression coverage for a full pack where late
    `verify_failure`/`failed_test` rows displace ordinary P2 impact context.
- Verification:
  - Focused regression passed:
    `go test ./internal/types -run 'Test(WriteContextPackPlannerLimitedViewRetainsVerifyFailureLane|NormalizeWriteContextPackRetainsLateVerifyFailureWhenPackFull|WriteContextPackBudgetedViewRetainsSafetyAndFailure)' -count=1`.
  - Related and full regressions passed:
    `go test ./internal/types ./internal/orchestrator -count=1`,
    `go test ./...`, `make`, and `git diff --check`.
  - Follow-up SWE smoke for `pydata__xarray-4248` at
    `/private/tmp/codrax-swe-rc87-xarray-20260618-context-pack-cap` confirms
    the full path:
    `workflow_status=complete`, `verify_status=passed`,
    `prediction_verdict=predicted_passed`, `prediction_local_confidence=high`,
    `patch_bytes=1700`, and final durable
    `wf-1781793569872758000-34553-batch-1-merged.json` remained bounded at 96
    items while retaining one `verify_failure`, one `failed_test`, and one
    `failure_signal` from the prior failed verify after successful replan.

## 2026-06-18 RC-87 Source-Owner Export For Validation Follow-ups

- Evidence:
  - A fresh three-instance historical-fail smoke at
    `/private/tmp/codrax-swe-rc88-crossfail-20260618` covered different failure
    families:
    `astropy__astropy-14365`, `astropy__astropy-6938`, and
    `sympy__sympy-13177`.
  - `astropy__astropy-14365` now produces a high-confidence local pass:
    `workflow_status=complete`, `verify_status=passed`,
    `prediction_verdict=predicted_passed`, and `patch_bytes=617`.
  - `sympy__sympy-13177` still correctly exports a source patch but remains
    local-audit blocked: `verify_status=unavailable`,
    `prediction_verdict=predicted_audit_blocked`, and
    `plan_patch_review_block_reason=patch_review_semantic_unverified:changed_symbol_without_probe_coverage`.
  - `astropy__astropy-6938` exposed a new adapter/export gap. The source plan
    correctly fixed the owner line in `astropy/io/fits/fitsrec.py`:
    `output_field = output_field.replace(...)`. The later proof-repair plan
    was validation-only and failed due local environment/probe infrastructure
    (`verification_probe_module_not_found`, `ModuleNotFoundError: No module
    named 'numpy'`, unrelated WCS/cextern build loader errors). Because the
    adapter exported from the final proof-only plan, the official prediction
    became an empty patch even though the typed workflow still contained an
    applied source-owner plan and the source-owner diff was coherent.
- Generalized rule:
  - Prediction export and local acceptance are separate. A failed or
    unavailable validation follow-up should lower/block local confidence, but
    it must not erase a typed applied source patch that the official harness can
    score.
  - When the final durable plan has no source changes and workflow attempts
    contain applied source plans after restore cutoffs, the adapter selects the
    latest typed applied source-owner plan for `model_patch` export. The final
    proof/test report remains the verification authority for local confidence.
  - Selection consumes only typed workflow attempts and ChangePlan artifacts:
    plan IDs, applied statuses, source path lists, restore-aware provenance, and
    repository refs. It does not inspect issue text, model prose, stdout,
    manual notes, SWE-bench IDs, or `<think>` output.
- Prompt and hard-gate hygiene:
  - Runtime controller behavior is unchanged. This is an evaluation/export
    boundary fix.
  - Local acceptance still fails or downgrades for typed proof failures,
    unavailable verification, and patch-review unverified coverage. Non-empty
    export remains separate from functional correctness.
  - Read/log/trace/data/operation/computer mode paths remain unchanged.
- Task list:
  - [x] Add `select_prediction_export_plan()` to choose the final source
    ChangePlan for export when the final durable plan is probe-only or
    validation-only.
  - [x] Record `export_plan_id`, `export_plan_path`, and
    `export_plan_selection_reason` in `results.jsonl`.
  - [x] Keep export path allowlisting bound to typed applied plan paths so setup
    artifacts remain dropped.
  - [x] Add regression coverage where a final proof-only plan exports the
    prior applied source plan's ref instead of an empty patch.
- Verification:
  - Focused adapter regression passed:
    `python3 -m unittest eval.swebench.run_codrax_swebench_test.ExportPatchPolicyTests.test_validation_followup_exports_latest_applied_source_plan -v`.
  - Python syntax check passed:
    `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/run_codrax_swebench_test.py`.
  - Recomputing the existing `astropy__astropy-6938` RC-88 artifact with the
    new selector chooses `plan-1781794367301721000-38231` with
    `export_plan_selection_reason=workflow_latest_applied_source_plan`,
    produces `patch_bytes=514`, and exports only
    `astropy/io/fits/fitsrec.py`. Local confidence remains blocked by the typed
    proof/verification failure evidence.
  - Full rerun for `astropy__astropy-6938` at
    `/private/tmp/codrax-swe-rc89-astropy6938-export-source-owner` confirms the
    end-to-end adapter boundary:
    `status=predicted`, `patch_bytes=523`,
    `prediction_verdict=predicted_audit_blocked`,
    `workflow_status=blocked`,
    `prediction_audit_block_reason=patch_review_semantic_unverified:changed_symbol_without_probe_coverage`,
    `delivery_candidate_status=coherent`, and
    `exported_patch_source_paths=['astropy/io/fits/fitsrec.py']`. The exported
    patch is harness-consumable while local acceptance remains failed.
  - New follow-up candidate from the rerun: when proof-repair fails due missing
    environment dependencies such as `numpy`, the controller can still spend a
    later batch on source edits that remain unverified. That is a runtime proof
    policy gap, not an export/provenance gap, and should be handled separately.

## 2026-06-18 RC-88 Verifier Unavailable Reason Authority

- Evidence:
  - The `astropy__astropy-6938` export rerun showed the source-owner export
    boundary was fixed, but runtime convergence could still over-react after a
    proof-repair verification failed in a partial local environment.
  - Individual typed reports carried verifier/tooling unavailability reasons
    such as `verification_probe_module_not_found` and import-loader failures.
    During aggregation, however, a broader `build_failure` surface could become
    the primary `FailureKind` while the unavailable reason was dropped. The
    controller then saw "build failure" rather than "local verifier unavailable"
    and could spend later budget on source replans even though the observed
    failure was dependency/loader availability, not proof of a bad patch.
- Generalized rule:
  - `FailureKind` describes the broad failure surface; `FailureReasonCode`
    carries the typed authority needed to distinguish code failure from local
    verifier unavailability.
  - If the aggregate primary kind is `build_failure` but it has no same-kind
    structured reason, and all available carried reason codes are typed
    verifier/tooling unavailability codes, the report normalizes to
    `verification_status=unavailable`.
  - A real build reason such as `typescript_compile_check_failed` remains
    `failed` and still drives replan. Red tests also keep unavailable secondary
    reasons out of the primary verdict.
- Prompt and hard-gate hygiene:
  - No prompt or model instruction changed. The fix consumes only typed
    `ChangeReport.FailureKind`, `FailureReasonCode`, `ExecutedCommand`
    reason codes, and normalized `VerificationStatus`.
  - The control plane does not inspect issue text, SWE IDs, stdout prose,
    manual audit notes, model rationale, summaries, or visible `<think>` logs.
  - Read/log/trace/data/operation/computer mode paths remain unchanged.
- Task list:
  - [x] Add a shared `FailureReasonCodeIndicatesVerificationUnavailable`
    helper in `internal/types` for report normalization and controller
    observation classification.
  - [x] Teach `run_tests` aggregation to retain typed unavailable/probe-import
    reason codes when an unreasoned build surface is the primary kind.
  - [x] Teach `ObservationAuthority` to classify
    `build_failure + unavailable reason` as `unverified`/finish-with-caveat
    instead of replan.
  - [x] Add regressions proving real build failures and red tests still fail.
- Verification:
  - Focused regressions cover `ChangeReport` normalization, `run_tests`
    aggregation, and `ObservationAuthority` classification.
  - This RC is a runtime proof-policy classification fix; official SWE
    prediction export remains separate and harness-consumable.

## 2026-06-18 RC-89 Local SWE Summary Current-Schema Denominator Guard

- Evidence:
  - Running the local Codrax summarizer over historical
    `eval/results/swebench/**/results.jsonl` with
    `--dedupe latest-by-file-mtime` produced 137 unique rows and preserved the
    known export signal: `non_empty_patch=130/137 (94.89%)`.
  - The same summary reported `local_acceptance_pass=0/137`, but every row was
    missing the current local-acceptance fields
    `local_acceptance_verdict`, `local_acceptance_source`, and
    `manual_audit_verdict`. That means the historical artifacts are not
    evaluable under the current acceptance schema; treating absent fields as
    `0% pass` would understate current capability and pollute the low-pass-rate
    root-cause analysis.
- Generalized rule:
  - Official SWE-bench pass rate remains only `resolved/total` from the
    official harness.
  - Local Codrax acceptance metrics must carry their own denominator. Rows that
    lack current core fields can still support non-empty export and typed
    cause-family telemetry, but they cannot support current
    `local_acceptance` pass/fail percentages.
  - The summarizer must fail loud on demand for current dashboards rather than
    silently mixing old-schema artifacts with current-run acceptance fields.
- Prompt and hard-gate hygiene:
  - This is an eval observability fix only. It reads JSON field presence and
    typed adapter fields.
  - It does not inspect issue text, model prose, terminal logs, manual audit
    notes, stdout narrative, or visible `<think>` output.
- Task list:
  - [x] Add current-core completeness counts and missing-field counts to
    `summarize_codrax_results.py`.
  - [x] Add `local_acceptance_evaluable_instances` and `*_evaluable` rates so
    absent current fields are not interpreted as failures.
  - [x] Add `--require-current-core` to make current local-acceptance
    dashboards fail loud when rows are stale.
  - [x] Update SWE-bench README with current-core denominator guidance.
  - [x] Add unit coverage for legacy rows and CLI fail-loud behavior.
- Verification:
  - Focused summarizer tests and Python compile cover the new denominator
    fields and fail-loud flag.
  - Re-running the 137-row historical summary now exposes
    `current_core=0/137`, making the current local-acceptance pass ratio
    explicitly unevaluable from those artifacts.

## 2026-06-18 RC-90 Historical SWE Patch And Answer Audit

- Evidence:
  - Added and ran `eval/swebench/audit_historical_results.py` over all
    historical `eval/results/swebench/*/results.jsonl` files with
    `--dedupe latest-by-file-mtime`.
  - The audit consumed 278 result rows and selected 137 latest unique
    instances. It loaded the public SWE-bench Lite dataset only for post-hoc
    oracle comparison; this is not fair-run input and not an official score.
  - Outputs:
    - `docs/design/swebench_historical_patch_audit_20260618.md`
    - `docs/design/swebench_historical_patch_audit_20260618.jsonl`
  - Conservative result: theoretical pass 60/137 (43.8%), fail 43/137
    (31.4%), unknown 34/137 (24.8%).
  - Failure buckets: wrong source surface 13, weak semantic overlap 12, typed
    verify failed 11, empty patch 7.
  - Every audited instance has `codrax.out`, but none has a typed
    final-answer/report artifact. The audit stores log tails for human review,
    but production eval must not treat terminal prose as the final answer
    contract.
- Generalized rule:
  - Non-empty patch remains export compatibility, not correctness.
  - Official SWE-bench pass rate remains only official harness
    `resolved/total`.
  - Local theoretical patch audit may use oracle patches after the fact, but
    the verdict must stay out of Codrax runtime routing and fair-run execution.
  - Typed verifier failures override oracle similarity in the audit; unavailable
    local verification can still leave rows pass/unknown depending on source
    overlap and patch similarity.
  - Final-answer quality needs a typed artifact containing patch intent,
    changed contracts, executed verification, observed failures, residual risk,
    and exported patch fingerprint.
- System gaps exposed:
  - Root-cause exploration/localization still fails often enough to produce
    non-empty patches against the wrong owner surface.
  - Verify-passed rows with weak or wrong oracle overlap show that local
    verification can be too narrow and should feed impact/patch-critic
    confidence rather than stand alone.
  - Handoff success must be measured by whether observations change subsequent
    source surfaces and repair slices, not by context-pack existence.
  - Historical final answers are not auditable as typed deliverables.
- Task list:
  - [x] Add a reproducible historical audit tool that consumes local typed
    result fields, predictions, and optional oracle patches.
  - [x] Add unit tests for diff parsing, source/test classification, empty
    patch failure, source-surface mismatch, high oracle overlap, and typed
    verifier-failure precedence.
  - [x] Generate the full 137-instance Markdown and JSONL audit artifacts.
  - [x] Document the audit command and metric boundary in the SWE-bench README.
  - [ ] Future batch: persist a typed final-answer/report artifact from each
    write-mode SWE run so answer quality can be audited without scraping
    `codrax.out`.
  - [ ] Future batch: connect weak/wrong source-surface audit buckets to
    impact-obligation re-explore/replan triggers using typed patch review and
    context-pack evidence, not oracle data.
- Verification:
  - `python3 -m unittest eval.swebench.audit_historical_results_test -v`
  - `python3 -m py_compile eval/swebench/audit_historical_results.py
    eval/swebench/audit_historical_results_test.py`
  - Audit run with eval venv and SWE-bench Lite dataset completed over 137
    unique instances.

## 2026-06-18 RC-91 Typed Final Delivery Report

- Gap:
  - RC-90 proved that historical SWE instances have durable `prediction.json`,
    `result.json`, `ChangePlan`, `ChangeReport`, workflow, context packs, and
    `codrax.out`, but no stable typed final-answer artifact.
  - Free-form terminal logs are intentionally transparent to the user, including
    visible model thinking, but they are not a safe audit contract. Downstream
    eval and customer review need a typed surface for what was delivered, what
    was verified, what remains risky, and which evidence refs survived handoff.
- Design:
  - Add a lightweight `WriteFinalReport` artifact beside the plan/report pair:
    `<plan-id>.final.json`.
  - The artifact is a deterministic projection from existing typed artifacts:
    `WriteWorkflowRun`, active `ChangePlan`, `ChangeReport`,
    `PatchReviewRecord`, `ImpactAnalysisResult`, `WriteContextPack`, and
    workflow completion. It never parses user intent keywords, model rationale,
    summary prose, terminal logs, or `<think>` text.
  - The report is not a new scheduler authority. Existing hard gates remain
    `ObservationAuthority`, approval/risk policy, patch review hard blocks, and
    workflow transition validation.
  - Required fields:
    - schema/kind/generated_at
    - run/workflow/batch/plan/report refs
    - completion verdict/reason/source
    - changed source/test paths and patch fingerprint/effect refs
    - verification status/failure kind/reason/test count/command count
    - patch-review coverage verdict/reason codes/block reason
    - impact target counts by coverage status and uncovered target examples
    - Top P0-P3 handoff evidence refs
    - residual risk reason codes, derived from typed unverified/failed coverage
- SWE adapter consumption:
  - Read `<plan-id>.final.json` when present.
  - Emit `final_report_path`, `final_report_present`,
    `final_report_completion_verdict`, `final_report_verification_status`,
    `final_report_patch_review_verdict`, `final_report_residual_risk_codes`,
    and `final_report_handoff_evidence_refs` into `results.jsonl`.
  - Keep old rows evaluable as `final_report_present=false`; do not infer
    final-answer quality from `codrax.out` when the typed artifact is absent.
- Task list:
  - [x] Add `types.WriteFinalReport` plus normalize/build/write/load helpers.
  - [x] Persist `<plan-id>.final.json` whenever a controller run reaches
    `complete` or `blocked`, and at plan-mode terminal completion.
  - [x] Add orchestrator tests proving terminal controller paths write a final
    report, and non-terminal paths do not.
  - [x] Add SWE adapter fields and unit tests.
  - [x] Update historical audit tooling to prefer `WriteFinalReport` over log
    tails for future runs while keeping old log-tail fallback for legacy rows.
  - [x] Run focused tests, `go test ./...`, `make`, Python eval tests, update
    this ledger, commit, and push.
- Prompt/hard-gate hygiene:
  - This batch changes no model prompt routing.
  - The final report consumes only typed artifacts and enum/boolean fields.
  - Terminal log text stays visible for user transparency but remains outside
    hard logic.
- Implementation notes:
  - Added `internal/types/write_final_report.go` as the single schema/build/load
    point.
  - `persistWriteWorkflowRun` now emits terminal `.final.json` reports from
    typed workflow/plan/report state; non-terminal runs do not emit the artifact.
  - SWE adapter result rows expose `final_report_*` fields, and historical
    audit prefers the typed artifact when present while preserving legacy
    `codrax.out` tail fallback.
- Verification:
  - `go test ./internal/types ./internal/orchestrator ./internal/writeflow -run
    'Test(BuildWriteFinalReport|WriteFinalReport|WriteOutputKind|PersistWriteWorkflowRun|RunWriteControllerWorkflow|WorkflowRunState)'`
  - `python3 -m unittest eval.swebench.run_codrax_swebench_test
    eval.swebench.audit_historical_results_test
    eval.swebench.summarize_codrax_results_test -v`
  - `python3 -m py_compile eval/swebench/run_codrax_swebench.py
    eval/swebench/run_codrax_swebench_test.py
    eval/swebench/audit_historical_results.py
    eval/swebench/audit_historical_results_test.py
    eval/swebench/summarize_codrax_results.py
    eval/swebench/summarize_codrax_results_test.py`
  - `go test ./...`
  - `make`

## 2026-06-18 RC-92 Shared Source Localization Review

- Gap:
  - Controller planning already has one typed online correction: when a
    `ChangePlan` edits production source paths outside prior P0/P1 localization
    context, `runControllerPlanBatch` retries planning once with a bounded
    localization critique.
  - That signal is still split across helper functions, planner hints, context
    pack rows, and SWE adapter telemetry. It is not a durable first-class
    artifact on read-mode Turn A handoff, `ChangePlan`, final delivery reports,
    or downstream audit summaries.
  - This makes post-hoc diagnosis difficult: reviewers can see that context
    packs exist, but cannot reliably answer whether the final patch surface was
    supported by earlier localization evidence, missing it, or had no prior
    localization context at all.
- Design:
  - Add `SourceLocalizationReview` under `internal/types` as a shared read/write
    artifact. The builder consumes only typed paths and evidence refs:
    `TurnAArtifacts.ReadFiles`, `EvidenceItem.Source`, `WriteContextPack` P0/P1
    `scope_anchor`/`target_file`/`evidence_ref`, and `ChangePlan` source paths.
  - Status is an enum: `unknown`, `observed`, `supported`, `weak`, `missing`.
    Reason codes are typed strings such as
    `read_turn_a_source_observed`,
    `plan_source_paths_supported_by_prior_context`,
    `plan_source_paths_missing_prior_context`, and
    `plan_source_paths_without_prior_context`.
  - Tests/docs/fixtures/examples are excluded from production-source missing
    path decisions through existing `ClassifySourcePathRole`; this is
    language-neutral and not Python-specific.
  - Hard control remains typed. The existing planner retry keeps using
    `WritePlanSourcePathsOutsidePriorContext`; no user intent keyword, model
    rationale, summary prose, `<think>` text, or terminal log text can route
    the workflow.
  - `WriteFinalReport` and SWE adapter rows should expose the localization
    review so customers can distinguish export compatibility, local verify, and
    owner-boundary localization quality.
- Task list:
  - [x] Add `SourceLocalizationReview` schema, normalization, clone/merge, and
    builders for read Turn A and write plan context coverage.
  - [x] Store the review on `TurnAArtifacts` and preserve it through
    Set/Get/Fork/Merge defensive-copy paths without touching the read scheduler
    byte-identity red line.
  - [x] Store the review on `ChangePlan` during plan context pack attachment
    and emit review rows into `WriteContextPackFromChangePlan`.
  - [x] Add localization fields to `WriteFinalReport` and the SWE adapter.
  - [x] Add focused Go/Python tests and run full regression before commit/push.
- Implementation notes:
  - Added `internal/types/source_localization_review.go` as the shared
    normalization/building point.
  - `TurnAArtifacts` now carries a read-side localization review derived from
    read files and evidence refs, and write exploration projection preserves it
    before planner handoff.
  - `attachPlanContextPackToWorkflowRun` stamps `ChangePlan.LocalizationReview`
    from existing P0/P1 context-pack paths before rendering the plan pack.
  - `WriteFinalReport.plan.localization` and SWE `final_report_localization_*`
    / `plan_localization_*` fields expose the signal for audit.
  - `WriteFinalReport.residual_risks` now includes
    `source_localization_weak` / `source_localization_missing` when the final
    plan's production source paths lack strong prior localization support.
- Verification:
  - `go test ./internal/types -run
    'Test(SourceLocalization|TurnAArtifacts|WriteContextPackFromChangePlanCarriesSourceLocalization|BuildWriteFinalReport|WriteFinalReport)'`
  - `go test ./internal/orchestrator -run
    'Test(AttachPlanContextPack|RunControllerPlanBatch_.*Localization|RunWriteControllerWorkflow|PersistWriteWorkflowRun)'`
  - `go test ./internal/agent -run
    'Test(TurnA|Explorer|Planner.*Localization|WriteExploration|Extractor)'`
  - `python3 -m unittest eval.swebench.run_codrax_swebench_test -v`
  - `go test ./...`
  - `make`
- Prompt/hard-gate hygiene:
  - This batch adds no prompt keyword routing.
  - Planner hints remain advisory text generated from typed missing path arrays.
  - Future controller actions may consume `SourceLocalizationReview` directly;
    this batch first makes the signal durable and auditable.

## 2026-06-18 RC-93 Verify Evidence Width And Failure Handoff Audit

- Gap re-check:
  - The original finding was accurate for old historical runs: a local
    `verify passed` line could be narrower than the functional owner boundary.
  - Current code no longer treats verifier prose as authority. The typed path is
    `ChangeReport.NormalizeVerificationStatus` +
    `VerificationConfidenceRecord` + `ImpactAnalysisResult.VerificationTargets`
    + `PatchReviewRecord.CoverageSummary` + workflow observation authority.
  - SWE local confidence already downgrades probe-only passes when changed
    source paths lack prior typed context; project-runner passes are not
    downgraded purely for context absence, because many customer environments
    can only provide partial runners and useful code delivery should not be
    blocked by missing dependency setup.
- Existing handoff/repair-slice evidence:
  - Failed post-apply verify calls `BuildVerifyFailureHandoff` and persists
    diff/surface artifact refs before returning the batch to `ready_to_plan`.
  - `planner.buildVerifyFailureHandoffSection` renders that typed carrier as
    the leading replan section, so the next plan sees failed tests,
    diagnostics, commands, and artifact refs instead of restarting broad
    exploration.
  - `selectImpactRepairQueueItems` derives bounded proof/impact follow-up
    batches from patch-review findings, impact targets, and verification
    confidence rows.
  - Tests covering this include
    `TestRunWriteControllerWorkflow_VerifyFailureHandoffSurvivesIntoReplan`,
    `TestNormalizeControllerTypedStateDecisionVerifiedButUndercoveredAppendsImpactRepair`,
    proof-follow-up source-edit rejection/allowance tests, and context-pack
    verify-failure scoping tests.
- Incremental fix completed in RC-92/RC-93:
  - Source localization risk now appears in `WriteFinalReport.residual_risks`.
    This keeps final delivery transparent when code may be usable but
    localization evidence was weak or missing.
- Decision:
  - Do not add another scheduler gate in this batch. The current hard-routing
    inputs already use typed artifacts; adding a new broad block on weak
    localization would overfit SWE oracle-style audits and would risk customer
    cases where local dependency/runners are incomplete.
  - Next meaningful code batch, if needed after fresh SWE runs, should target a
    typed `VerificationProofProfile` artifact that summarizes runner strength,
    probe coverage, impact-target coverage, and localization status in one
    normalized record. It should consume the existing artifacts above rather
    than parse logs or model prose.
- Task list:
  - [x] Re-audit observation authority, patch-review, impact repair, and
    verify-failure handoff code paths.
  - [x] Confirm failure handoff survives reset into replan and is cleared after
    green verify by existing tests.
  - [x] Confirm proof-follow-up and impact repair batches consume typed
    coverage gaps instead of broad re-exploration.
  - [x] Surface weak/missing localization as final residual risk.
  - [x] Leave project-runner pass semantics stable; continue downgrading
    probe-only pass when typed context coverage is missing.

## 2026-06-19 RC-94 Verification Proof Profile

- Gap:
  - Verification strength is currently recoverable, but spread across
    `ChangeReport.VerificationStatus`, `VerificationConfidence`,
    `PatchReviewRecord.CoverageSummary`, `ImpactAnalysis.VerificationTargets`,
    and `SourceLocalizationReview`.
  - This makes final delivery/eval consumers re-assemble proof strength in
    parallel, increasing model/operator mental load and making "verify passed
    but proof narrow" harder to audit.
- Design:
  - Add `VerificationProofProfile` as a typed projection from existing typed
    artifacts only.
  - Status enum: `unknown / unavailable / failed / weak / adequate / strong`.
  - Runner evidence enum:
    `none / unavailable / project_runner / verification_probe /
    syntax_fallback / mixed`.
  - The profile records reason codes, confidence reason codes, test/command
    counts, probe/project/syntax command counts, impact target coverage counts,
    patch-review verdict, and localization status.
  - A passed project runner with verified impact/localization and no weak
    confidence is `strong`.
  - A passed bounded probe can be `adequate` when coupled to contracts/symbols,
    or `weak` when typed confidence, impact, patch-review, or localization rows
    show missing proof.
  - Failed/unavailable verification remains explicit. Missing customer
    toolchains stay unavailable/warning evidence, not a code-failure hard gate.
- Task list:
  - [x] Add `VerificationProofProfile` type, normalization, builder, and tests.
  - [x] Project the profile into `WriteFinalReport.proof`.
  - [x] Add proof-profile residual risk rows for weak/unavailable/failed proof.
  - [x] Export `final_report_proof_*` fields in SWE adapter results.
  - [x] Update SWE README and this ledger.
- Prompt/hard-gate hygiene:
  - No prompt routing changes.
  - No keyword matching over user intent, issue text, model prose, stdout text,
    terminal logs, or `<think>`.
  - The profile consumes only typed report/plan artifacts and enum fields.
- Verification:
  - `go test ./internal/types -run
    'Test(BuildVerificationProofProfile|BuildWriteFinalReport|WriteFinalReport)'`
  - `python3 -m py_compile eval/swebench/run_codrax_swebench.py
    eval/swebench/run_codrax_swebench_test.py`
  - `python3 -m unittest eval.swebench.run_codrax_swebench_test -v`
  - `go test ./...`
  - `make`

## 2026-06-19 RC-95 Proof Follow-up No-op State Closure

- Fresh SWE-bench Lite smoke evidence:
  - Targeted run directory:
    `eval/results/swebench/lite-smoke-20260619-localization-proof-3`.
  - Instance `pytest-dev__pytest-5227` reached
    `batch-1-obligation-repair` with purpose
    `impact_and_verification_proof_followup`.
  - The source plan had already applied the intended
    `src/_pytest/logging.py` change, while post-apply verification was
    `unverified/parser_error`, not a typed code failure.
  - Planner then repeatedly tried to produce a patch against bytes that were
    already in the desired state; emit validation correctly rejected no-op
    edits, but the workflow lacked a clear typed materialization path for
    "already applied, proof still needed".
- Gap:
  - The existing `no_change_required` sentinel was available for
    verify-failure replans and pure proof-follow-up probe plans, but mixed
    `impact_and_verification_proof_followup` batches without a typed failure
    handoff still exposed read/repair tools and the skeleton emit path.
  - That gave the model too many choices and let a proof batch degrade into
    fake source-edit attempts.
- Design:
  - Treat any controller-authorized verification proof follow-up with no active
    typed code-failure handoff as a proof-materialization-only batch.
  - Expose only `run_tests` dry-run probes and `emit_change_plan`; hide
    `emit_plan_skeleton` / `emit_plan_change` for that materialization lane
    because skeletons cannot usefully represent "no source edit".
  - Allow mixed proof/impact follow-up to regain bounded source-read/patch
    tools only after a typed `VerifyFailureHandoff` proves code still needs
    repair.
  - Keep ordinary write plans unchanged: `changes[]` remains required unless
    workflow typed state authorizes a proof/no-change sentinel.
- Task list:
  - [x] Broaden planner proof-materialization detection from pure
    `verification_proof_followup` to typed proof follow-ups without active
    code-failure handoff.
  - [x] Narrow proof-materialization tool surface to `run_tests` plus
    `emit_change_plan`.
  - [x] Update `emit_change_plan` and `emit_plan_skeleton` schema reminders so
    authorized proof batches can emit `changes: []` with
    `verification_probes[]`.
  - [x] Add skeleton empty-change fallback to the same probe-only typed plan,
    while preserving non-empty file metadata for normal plans.
  - [x] Extend no-op structured-edit repair diagnostics for proof-follow-up
    batches so models are routed to typed probe-only plans, not comments or
    whitespace patches.
  - [x] Add regressions for pure proof, mixed proof without failure, and mixed
    proof with typed failure handoff.
- Prompt/hard-gate hygiene:
  - Hard behavior reads only typed workflow progress, active batch purpose, and
    `VerifyFailureHandoff`.
  - No user-intent keyword matching, no issue-text matching, no parsing model
    rationale/summary/`<think>`, and no stdout prose routing.
  - Prompt/tool text is only soft guidance and schema repair; it does not
    decide safety or completion.
- Verification:
  - `go test ./internal/agent ./internal/tool ./internal/orchestrator -run
    'TestPlannerFilterToolSchemas_(PureProofFollowup|MixedProofImpact)|TestPlannerBuildInitialInstruction_RendersProofFollowup|TestEmit(ChangePlan|PlanSkeleton).*Proof|NoChange|ProofFollowup|TestRunControllerPlanBatch_.*Proof|TestRunControllerPlanBatch_NoChange'`
  - `go test ./...`
  - `make`
  - `git diff --check`
  - SWE rerun followed up by RC-96.

## 2026-06-19 RC-96 SWE Adapter Final-Report Projection Fix

- Fresh SWE-bench Lite smoke evidence:
  - Targeted run directory:
    `eval/results/swebench/lite-smoke-20260619-rc95-localization-proof-3`.
  - Instances: `pytest-dev__pytest-5227`, `django__django-14534`, and
    `sympy__sympy-18199`.
  - All three exported non-empty predictions:
    `pytest-dev__pytest-5227` 512 bytes, `django__django-14534` 416 bytes,
    and `sympy__sympy-18199` 594 bytes.
  - `eval/swebench/validate_predictions.py` accepted the predictions file with
    `empty_patch=0`.
  - Official SWE-bench harness import/dry-run accepted the generated command
    with Python 3.11 and the installed `swebench` package.
- Gap:
  - The adapter post-processing path looked up the final report through
    undefined locals named `final_plan_path` and `primary_plan_path`.
  - That made result rows print `status=error patch_bytes=0` even when
    `predictions.jsonl` contained a valid non-empty patch. The false error
    polluted SWE dashboards and made root-cause/localization analysis look
    worse or different than the actual Codrax delivery artifact.
- Design:
  - Use the already-selected typed delivery candidate as the authority for
    final-report projection.
  - Preserve the delivery candidate's `primary_source_plan_path` and pass it to
    `first_final_report_for_plans` together with the delivery report plan,
    current plan, and export plan.
  - Keep official prediction export unchanged; this is telemetry/report
    normalization only.
- Manual audit:
  - `pytest-dev__pytest-5227` changed the default logging format in
    `src/_pytest/logging.py`; the patch is plausible and local verification
    stayed `unverified/parser_error`.
  - `django__django-14534` changed `BoundWidget.id_for_label` to return
    `self.data["attrs"].get("id")`; post-hoc oracle comparison confirms this
    matches the historical fix, but local visible tests without SWE
    `test_patch` still failed. This is a SWE-local-verification nuance, not a
    reason to weaken customer verification globally.
  - `sympy__sympy-18199` added the zero-residue nthroot branch and completed
    with the RC-95 proof follow-up represented as a probe-only
    `no_change_required` plan.
- Remaining architecture conclusion:
  - This RC does not close the full exploration/localization gap.
  - Completed layers now prevent several downstream false positives and false
    negatives: typed localization retry, delivery-source provenance,
    restore-aware provenance, patch critic, proof follow-up materialization,
    and adapter final-report projection.
  - The open systemic work is still upstream: read/write localization must
    produce stronger typed owner/evidence anchors so plans avoid unrelated
    source surfaces before patch review needs to intervene.
- Task list:
  - [x] Replace undefined final-report projection locals with typed delivery
    candidate paths.
  - [x] Add regression coverage that delivery candidates preserve
    `primary_source_plan_path`.
  - [x] Re-run adapter compile and unit suite.
  - [x] Validate the RC-95 three-instance predictions file.
  - [x] Run official harness import/dry-run consumption check.
  - [x] Record that exploration/localization is only partially closed and
    remains a system-level follow-up.
- Prompt/hard-gate hygiene:
  - No prompt changes.
  - No keyword matching over user intent, issue text, model prose, logs,
    stdout, SWE IDs, manual notes, or `<think>`.
  - The fix consumes typed delivery-candidate metadata and filesystem paths
    only.
- Verification:
  - `python3 -m py_compile eval/swebench/run_codrax_swebench.py
    eval/swebench/run_codrax_swebench_test.py`
  - `python3 -m unittest eval.swebench.run_codrax_swebench_test -v`
  - `PYTHON=/opt/homebrew/bin/python3.11 DRY_RUN=1
    CHECK_HARNESS_IMPORT=1
    PREDICTIONS_PATH=eval/results/swebench/lite-smoke-20260619-rc95-localization-proof-3/predictions.jsonl
    eval/swebench/run_official_harness.sh`

## 2026-06-19 RC-97 Shared Source-Localization Owner Anchors

- Current code audit:
  - Read mode already persists `TurnAArtifacts.SourceLocalization` from
    `ReadFiles` plus grounded evidence.
  - Write mode already stamps `ChangePlan.LocalizationReview` and retries once
    when a source plan edits paths outside prior P0/P1 localization context.
  - `WriteContextPackFromExplorationHandoff` still projects broad
    `target_file` rows from every read file, and the plan gate treats those
    rows like owner evidence when no finer artifact exists.
  - Therefore the system can know "a file was read" but cannot reliably
    distinguish that from "this file has line-backed owner evidence". Wrong
    localization can survive until PatchReview or verifier failure.
- Architecture:
  - Add `SourceLocalizationAnchor` as a typed artifact shared by read and
    write:
    - `path`, `role`, `source_stage`, `kind`, `strength`,
      `evidence_ref`, `subject`, `anchor_symbol`, `reason_code`.
    - `kind` examples: `read_file`, `grounded_evidence`,
      `recovered_evidence`, `scope_anchor`.
    - `strength` examples: `observed`, `supporting`, `owner`.
  - `SourceLocalizationReviewFromTurnA` derives anchors from structured
    read-mode facts:
    - production `read_file` rows become `observed` anchors.
    - grounded/recovered line-backed evidence in production paths becomes
      `owner` anchors with evidence refs.
    - auxiliary paths remain auxiliary and cannot satisfy production owner
      localization.
  - `WriteExplorationHandoff` carries the normalized localization review.
  - `WriteContextPack` projects each anchor as a `localization_anchor` item
    with a typed `localization_anchor` sub-object. Prompt text remains
    advisory; hard gates consume the sub-object.
  - `writeContextCoveragePriorPaths` prefers typed owner/supporting
    localization anchors when they exist, plus explicit write-analysis scope
    anchors. It falls back to legacy target/evidence rows only when no typed
    localization anchor is available, preserving stable simple flows.
- Prompt and hard-gate hygiene:
  - No user-intent keyword matching.
  - No parsing model summary/rationale/`<think>`.
  - No stdout/stderr prose routing.
  - The only new hard signal is a schema-owned typed struct produced from
    deterministic path roles and grounded evidence metadata.
- Task list:
  - [x] Add `SourceLocalizationAnchor` types, normalization, cloning, and
    merge support.
  - [x] Derive anchors in `SourceLocalizationReviewFromTurnA`.
  - [x] Carry localization review through `WriteExplorationHandoff`.
  - [x] Project typed `localization_anchor` context items.
  - [x] Make prior-path coverage prefer typed owner/supporting anchors while
    retaining fallback behavior when absent.
  - [x] Add read/write type tests for grounded evidence anchors, read-file-only
    weak anchors, auxiliary exclusion, handoff projection, and fallback.
  - [x] Run focused tests, related package tests, full regression if touched
    surfaces warrant it, then SWE-bench Lite spot validation.
- Implementation:
  - Added `SourceLocalizationAnchor` with typed `kind`, `strength`, `role`,
    optional evidence ref, and owner symbol metadata.
  - Read-mode Turn A localization now emits `observed` anchors for production
    `read_file` paths and `owner` / `supporting` anchors for grounded,
    recovered, or line-backed evidence. Auxiliary test/docs/fixture evidence
    remains auxiliary and cannot satisfy production owner localization.
  - `WriteExplorationHandoff` persists the source localization review, and
    `WriteContextPackFromExplorationHandoff` projects each anchor as a
    `localization_anchor` item with a typed sub-object.
  - `writeContextCoveragePriorPaths` now uses typed owner/supporting anchors
    when anchor artifacts exist; broad `target_file` / `evidence_ref` fallback
    is retained only for legacy/no-anchor runs. Observed read-file anchors do
    not satisfy owner localization.
- Verification:
  - `go test ./internal/types -run
    'TestSourceLocalization|TestWriteExplorationHandoffCarriesSourceLocalizationAnchors|TestWriteContextPackFromExplorationHandoffProjectsLocalizationAnchors|TestWritePlanSourcePathsOutsidePriorContext|TestWriteContextPackFromPlanContextCoverage'
    -count=1`
  - `go test ./internal/types ./internal/orchestrator ./internal/agent -run
    'Test(SourceLocalization|WriteContext|WriteExploration|RunControllerPlanBatch_Localization|AttachPlanContextPack|Planner)'
    -count=1`
  - `go test ./internal/types ./internal/tool -count=1`
  - `go test ./...`
- Acceptance criteria:
  - Read mode still returns the same answer pipeline and keeps L1 scheduler
    untouched.
  - Rich read-mode exploration produces durable typed localization anchors.
  - Write-mode planning can distinguish broad read observation from
    evidence-backed owner support without natural-language parsing.
  - Existing simple write paths without exploration anchors continue to use the
    legacy fallback rather than failing closed.

### RC-97 SWE Smoke Evidence And RC-98 Follow-up

- Post-RC97 SWE-bench Lite smoke:
  - Workdir:
    `eval/results/swebench/lite-smoke-20260619-rc97-localization-anchor-3`.
  - Instances: `pytest-dev__pytest-5227`, `django__django-14534`, and
    `sympy__sympy-18199`.
  - Predictions: 3/3 non-empty; validator passed with `empty_patch=0`;
    official harness import/dry-run accepted the generated command.
  - Result rows:
    - Pytest: `status=predicted`, `patch_bytes=512`, workflow complete,
      verify unavailable, localization supported by `src/_pytest/logging.py`.
    - Django: `status=predicted`, `patch_bytes=416`, workflow complete,
      verify unavailable, localization supported by `django/forms/boundfield.py`.
    - SymPy: `status=predicted`, `patch_bytes=563`, workflow complete,
      verify passed, localization supported by `sympy/ntheory/residue_ntheory.py`.
- Audit:
  - Pytest and Django used write-analysis scope anchors; no read-side
    localization anchors were present, which is acceptable for direct
    scope-localized requests.
  - SymPy persisted `localization_anchor` items, proving the new handoff path
    works, but the anchors came from broad deterministic `concrete_value`
    evidence in `sympy/codegen/ast.py` rather than the final owner path.
  - The plan stayed safe because the write-analysis scope anchor still covered
    `sympy/ntheory/residue_ntheory.py`, but the broad deterministic anchors are
    too noisy to satisfy owner localization in future no-scope cases.
- RC-98 task list:
  - [x] Let `SourceLocalizationReviewFromTurnA` keep deterministic
    `concrete_value` / `dataflow_path` rows as evidence refs.
  - [x] Prevent those broad deterministic rows from becoming
    owner/supporting localization anchors unless they carry a typed
    `context_role=defining` authority.
  - [x] Preserve LLM-emittable grounded/recovered evidence anchors.
  - [x] Add regression coverage for deterministic concrete-value evidence not
    satisfying owner localization.
- RC-98 implementation:
  - `sourceLocalizationAnchorFromEvidence` now requires either
    LLM-emittable evidence kind or `context_role=defining` before a line-backed
    evidence row can become an owner/supporting localization anchor.
  - Deterministic-only broad rows still remain in `EvidenceRefs`, preserving
    downstream handoff richness without letting them become hard routing facts.
- RC-98 verification:
  - `go test ./internal/types -run
    'TestSourceLocalization|TestWriteExplorationHandoffCarriesSourceLocalizationAnchors|TestWriteContextPackFromExplorationHandoffProjectsLocalizationAnchors|TestWritePlanSourcePathsOutsidePriorContext|TestWriteContextPackFromPlanContextCoverage'
    -count=1`
  - `go test ./internal/types ./internal/tool ./internal/orchestrator
    ./internal/agent -count=1`
  - `go test ./...`
  - `make`
- Post-fix SWE spot:
  - Reran `sympy__sympy-18199` at
    `eval/results/swebench/lite-smoke-20260619-rc98-anchor-filter-sympy`.
  - Prediction validation passed with `empty_patch=0`; official harness
    import/dry-run accepted the generated command.
  - Result row: `status=predicted`, `patch_bytes=549`, workflow complete,
    `verify_status=passed`, `prediction_verdict=predicted_passed_low_confidence`.
  - Context audit: `localization_anchor` rows now point to
    `sympy/ntheory/residue_ntheory.py`; broad `sympy/codegen/ast.py`
    deterministic evidence no longer appears as localization anchors.

## 2026-06-19 RC-99 Coherent Delivery Proof Aggregation

- Evidence:
  - Post-RC98 `sympy__sympy-18199` produced a non-empty patch on the correct
    source owner path and local verification passed.
  - The source plan's report covered `outcome-1`, `outcome-2`,
    `outcome-3`, and `zero-is-nthpow-residue`, while the proof follow-up
    report covered the previously missing `outcome-4` and
    `zero-root-included`.
  - The terminal final report still marked proof `weak` because
    `BuildWriteFinalReport` projected only one `ChangePlan + ChangeReport`.
    A pure proof-follow-up batch can therefore satisfy the missing typed
    obligations but look weak in isolation because it does not re-mention the
    already-covered source-plan contracts.
- Root cause:
  - The online controller now works like an edit/run/observe loop, but the
    durable final audit artifact still has a single-batch proof lens.
  - This is not an eval-adapter issue and not a prompt issue. It is a typed
    artifact aggregation boundary: final proof should represent the coherent
    terminal delivery chain, not the last batch only.
- Design:
  - Add a cumulative proof helper that consumes typed `ChangeReport`
    `VerificationConfidenceRecord` rows from the terminal workflow's completed
    current batches.
  - Keep hard failure/unavailable authority conservative. The helper may remove
    `verification_probe_missing_*` reason codes only when typed satisfied
    records cover the same contract/symbol references elsewhere in the same
    coherent delivery chain.
  - Do not parse stdout/stderr, model prose, issue text, final answer text, or
    SWE oracle data.
  - Do not load historical attempts. Use each completed batch's current
    `PlanID` / report artifact so rolled-back or superseded attempts do not
    strengthen or weaken final proof.
  - Probe-only plans may have `changes: []`; they must still contribute report
    confidence even when `LoadChangePlanFromFile` refuses an empty plan.
- Task list:
  - [x] Add typed `VerificationProofArtifact` / cumulative proof aggregation
    helpers in `internal/types`.
  - [x] Extend `WriteFinalReportInput` so callers can pass coherent proof
    artifacts while preserving the single-plan default.
  - [x] Teach `persistWriteFinalReportIfTerminal` to collect completed-batch
    current report artifacts from the workflow store/report directory.
  - [x] Add unit coverage for source-plan + proof-follow-up union removing
    resolved missing soft/hard contract reasons while retaining unresolved
    reasons and real failed/unavailable states.
  - [ ] Recompute or rerun the RC98 SymPy spot and verify the final report no
    longer falsely downgrades cumulative proof.
  - [x] Run focused, related, full Go regressions and push.
- Implementation:
  - `VerificationProofProfile` now has cumulative metadata and can merge typed
    `VerificationConfidenceRecord` evidence across a coherent workflow proof
    chain.
  - Only resolvable probe missing reason codes are removed, and only when the
    same contract/symbol refs are covered by typed satisfied records elsewhere
    in the same chain.
  - `WriteFinalReportInput` accepts `ProofArtifacts`; the single-plan default
    is preserved for callers that do not pass extra artifacts.
  - `persistWriteFinalReportIfTerminal` collects each terminal completed
    batch's current plan/report artifact, ignoring historical attempts and
    accepting report-only proof batches whose plan file may have `changes: []`.
- Verification:
  - `go test ./internal/types -run
    'TestBuild(CumulativeVerificationProofProfile|WriteFinalReport)' -count=1`
  - `go test ./internal/orchestrator -run
    'TestPersistWriteWorkflowRunTerminal(WritesFinalReport|AggregatesCompletedBatchProofReports)|TestPersistWriteWorkflowRunNonTerminalDoesNotWriteFinalReport'
    -count=1`
  - `go test ./internal/types ./internal/orchestrator ./internal/tool
    ./internal/writeflow -count=1`
  - `go test ./...`
  - `make`

## 2026-06-19 RC-100 Planner Materialization Convergence Boundary

- Evidence:
  - RC-99 SymPy spot with fair git-history isolation and prepared Python env
    reached `batch-1:ready_to_plan` with typed scope anchors and
    `expected_paths=["sympy/ntheory/residue_ntheory.py"]`.
  - The planner repeatedly read the same localized source and attempted a
    planning-stage dry-run probe, but never called a structured plan emit tool.
  - The workflow blocked as `workflow_blocked_no_plan`, producing an empty
    prediction despite enough typed localization and a small, well-bounded
    source owner surface.
- Root cause:
  - Planner materialization mode was activated only by exploration handoff or
    proof-followup state. A batch seeded directly from `WriteAnalysisIR`
    expected paths remained in broad investigation mode, so each retry could
    spend its entire turn rediscovering already-known code.
  - When a typed planning-stage `run_tests` probe happened near the soft
    iteration cap, the planner loop stopped immediately instead of allowing one
    bounded post-probe structured emit turn.
- Design:
  - Treat active workflow batch `ExpectedPaths` and `WriteAnalysisIR`
    `ScopeAnchors` as typed localization material for planner
    materialization. These are structured artifacts, not user/prose keyword
    matches.
  - Keep a small exact-byte read window, then narrow the planner tool surface
    to structured plan emit tools plus typed dry-run probes.
  - If materialization-mode `run_tests` is called at the soft cap, allow the
    bounded hard-cap recovery window so the next turn can consume typed probe
    results and emit a plan. Repeated probes still stop at the hard cap.
  - Continue rendering model `<think>` for transparency; no control logic
    reads it.
- Task list:
  - [x] Add workflow-seed localization counting from active batch
    `ExpectedPaths` and `WriteAnalysisIR.ScopeAnchors`.
  - [x] Use that count in planner handoff/materialization activation and read
    budget calculation.
  - [x] Suppress generic keyword-based likely-file rediscovery when typed
    workflow paths already localize the batch.
  - [x] Allow one bounded post-`run_tests` materialization turn before hard cap.
  - [x] Add planner focused tests for workflow expected-path materialization and
    post-probe emit-window behavior.
- Verification:
  - `go test ./internal/agent -run
    'TestPlanner(ShouldStop_SoftCapAllowsMaterializationRunTestsFollowup|BuildInitialInstruction_WorkflowExpectedPathsSuppressInvestigationSeed|BuildInitialInstruction_ExplorationHandoffSuppressesInvestigationSeed|ShouldStop_SoftCapAllowsTypedEmitRepairRead|ShouldStop_TypedEmitRepairStillHardCaps)'
    -count=1`
  - `go test ./internal/agent ./internal/orchestrator ./internal/types
    ./internal/tool ./internal/writeflow -count=1`

## 2026-06-19 RC-101 Actual-Diff Patch Quality Shape

- Evidence:
  - After RC-100, `sympy__sympy-18199` generated a non-empty
    harness-consumable prediction and local verifier passed.
  - Manual inspection found the patch was functionally plausible, but it added
    a non-ASCII Chinese comment to upstream Python source and duplicated the
    already-present `a, n, p = as_int(a), as_int(n), as_int(p)` assignment a few
    lines later.
  - The final report was already low-confidence because patch review/impact
    proof stayed weak, but Patch Critic did not name these concrete quality
    risks, so future replans had no precise typed evidence for this class.
- Root cause:
  - Patch Critic has actual-diff structural and semantic events, but the
    current shape library still misses small localized quality regressions:
    repository-inconsistent source comments and nearby duplicate statements.
  - This is not Python-specific. The missing abstraction is a language-provider
    source-quality event producer over actual added lines plus post-apply file
    bytes.
- Design:
  - Emit `non_ascii_source_comment_added` when an added production-source line is
    comment-only and contains non-ASCII runes. This is a soft semantic coverage
    warning, not a hard block, because some repositories intentionally use
    localized comments.
  - Emit `nearby_duplicate_statement_added` when an added production-source
    assignment exactly duplicates a nearby pre-existing assignment in the final
    file. This is also soft/unknown coverage: it should invite a bounded
    cleanup/replan while not blocking a customer delivery whose functionality is
    otherwise proven.
  - Use only repo-relative paths, `SourcePathRole`, parsed diff added line
    numbers/text, and post-apply file bytes. Do not read user intent keywords,
    issue text, stdout narrative, model rationale, final answer prose, or
    `<think>`.
  - Keep the events language-neutral and provider-gated so future language
    providers can refine comment/statement rules without central prompt logic.
- Task list:
  - [x] Record the RC-101 gap and implementation plan in this delivery ledger.
  - [x] Add actual-diff quality-shape producers to `PatchEffectRecord`.
  - [x] Register the new events as PatchReview soft/unknown coverage findings.
  - [x] Add focused regression coverage for non-ASCII source comments and
    nearby duplicate assignment statements.
  - [x] Run focused, related, full Go regressions and push.
- Verification:
  - `go test ./internal/writeflow -run
    'TestAnnotatePatchEffect(NonASCIISourceCommentWarns|NearbyDuplicateStatementWarns|PythonAddedReturnBeforeExistingBodyHardBlocks|DuplicateInsertedBlockHardBlocks|MultiLanguageLineShapeWarnings|OwnerBoundaryWarnings)'
    -count=1`
  - `go test ./internal/writeflow ./internal/types ./internal/orchestrator
    ./internal/tool -count=1`
  - `go test ./...`
  - `make`

## 2026-06-19 RC-102 Python Docstring Section Executable Insertions

- Evidence:
  - Current run `eval/results/swebench/lite-smoke-20260619-rc102-current-3`
    exported three non-empty predictions and official harness dry-run accepted
    the predictions JSONL.
  - `sympy__sympy-18199` exported a patch that inserted:
    `if a % p == 0: ... return 0` inside the `nthroot_mod()` docstring,
    between the `Parameters` title and its underline. The patch is not
    executable product code even though the user-visible behavior request needs
    runtime logic.
  - Local verification was unavailable (`make_target_missing` plus probe
    syntax/unavailable evidence), so the bad patch survived as
    `predicted_unverified`. Existing PatchEffect events were empty for the
    affected file.
- Root cause:
  - Actual-diff Patch Critic catches many source-shape defects, but it does not
    distinguish executable-looking added statements in non-executable text
    regions such as docstrings.
  - This is not an issue-specific SymPy rule. It is a source-region authority
    gap: a patch can target the right owner file and still land the behavior
    code in a non-runtime region.
- Design:
  - Add a Python provider hook that computes triple-quoted string/docstring line
    spans from post-apply file bytes.
  - Emit a hard `python_docstring_section_executable_added` event only when an
    added executable-looking Python statement is inside such a span and disrupts
    structured docstring section shape, i.e. the inserted block sits before the
    next non-added section underline. This keeps ordinary doctest/code-block
    documentation examples from becoming hard failures.
  - The event uses only actual diff line numbers/text, `SourcePathRole`, and
    post-apply source bytes. It does not read user intent, issue text, stdout,
    model rationale, final answer prose, or `<think>`.
  - PatchReview consumes the event as a structural hard block, reusing the
    existing replacement-patch repair lane and retry budget.
- Task list:
  - [x] Record the three-instance smoke evidence and RC-102 design.
  - [x] Add the Python docstring section executable event producer.
  - [x] Register the event as a PatchReview structural hard event.
  - [x] Add focused tests for the SymPy-style section disruption and a safe
    docstring code-example non-event.
  - [x] Run focused, related, full Go regressions, rebuild, commit, and push.
- Verification:
  - `go test ./internal/writeflow -run
    'TestAnnotatePatchEffectPythonDocstring(SectionExecutableHardBlocks|CodeExampleDoesNotHardBlock)'
    -count=1`
  - `go test ./internal/writeflow ./internal/orchestrator ./internal/types
    ./internal/tool -count=1`
  - `go test ./...`
  - `make`

## 2026-06-19 RC-103 Verify-failure Repair Read Affordance

- Evidence:
  - The RC-102 three-instance smoke showed `django__django-14534` entered
    repeated failed-verify restore/replan loops. After successful patch
    application and red verification evidence, the planner eventually attempted
    an unavailable `grep` call because the handoff synthesis read window had
    already been consumed and the schema had narrowed to materialization tools.
- Gap:
  - Initial handoff synthesis and failed-verify repair shared one read budget.
    That is too coarse for online convergence: a build/test failure can point to
    a neighboring owner symbol, fixture, selector, or invariant that was not
    necessary for the first patch. The planner needs a small repair-local
    read/search affordance, but it must not regain ordinary command execution or
    broad exploratory loops.
- Design:
  - Add a dedicated typed `verifyFailureRepairActive` planner state derived only
    from `VerifyFailureHandoff`; proof-only follow-up batches remain
    materialization-only.
  - Preserve planner hard gates: `exec_command`, `apply_patch`, and verifier
    output tools stay blocked in StagePlan; the new affordance exposes only the
    existing typed repository read/search tools (`read_file`, `grep`,
    `list_files`, `repo_map`).
  - Run the failed-verify repair read window only after the ordinary handoff
    synthesis budget is exhausted, then narrow back to plan emit plus typed
    dry-run probe tools. This keeps Claude-Code-style online edit/observe/repair
    behavior without reverting to batch-wide rediscovery.
  - Emit a distinct hint key
    `planner.verify-failure-repair-tool-surface` when the model calls a read
    tool after the repair budget has closed, so retry hints are precise and do
    not depend on model prose.
- Task list:
  - [x] Record the RC-103 gap and implementation plan.
  - [x] Add a bounded verify-failure repair read/search budget to the planner
    evaluator state machine.
  - [x] Keep proof-only follow-up and structured emit repair semantics separate.
  - [x] Add focused regression tests for repair-window retention, narrowing, and
    unavailable-tool hint routing.
  - [x] Run focused, related, full Go regressions, rebuild, commit, and push.
- Verification:
  - `go test ./internal/agent -run
    'TestPlanner(FilterToolSchemas_VerifyFailureRepair|Observe_VerifyFailureRepair|FilterToolSchemas_StructuredEmitRepair)'
    -count=1`
  - `go test ./internal/agent ./internal/orchestrator ./internal/types
    ./internal/tool ./internal/writeflow -count=1`
  - `go test ./...`
  - `make`

## 2026-06-19 RC-104 Localization Anchor Consumption Closure

- Evidence:
  - The post-RC102/RC103 SWE audit still classifies wrong-source-surface
    localization as the largest upstream correctness gap. RC97/RC98 introduced
    `SourceLocalizationAnchor` production and path-level plan localization
    gates, but the write plan review compressed prior localization evidence into
    `SupportedPaths` / `MissingPaths`.
  - That path-only compression loses the consumer-critical distinction between
    owner anchor, supporting evidence anchor, scope anchor, and merely observed
    read-file rows. Downstream context packs could tell that a plan path was
    supported, but not which typed anchor supported it or which evidence ref
    carried the line-backed owner signal.
- Gap:
  - `SourceLocalizationReviewFromWritePlanContext` consumed prior context as a
    path set. `WriteContextPackFromChangePlan` rendered the resulting review
    summary and missing paths, but did not project matched prior anchors from
    the review into typed context items.
  - The system already had the right artifact type; the missing piece was
    end-to-end consumption and preservation across the plan boundary. Rebuilding
    another localization model would duplicate RC97/RC98.
- Design:
  - Add a typed prior-localization anchor extractor for `WriteContextPack`
    arrays. It reads only P0/P1 `localization_anchor` structs and explicit
    `scope_anchor` rows, skips plan-authored packs and test paths, and returns
    normalized `SourceLocalizationAnchor` values.
  - Keep routing semantics unchanged: `WritePlanSourcePathsOutsidePriorContext`
    still uses the existing path coverage logic. The new anchor view enriches
    the review and downstream handoff; it does not introduce user-intent
    keyword gates or prompt-prose parsing.
  - When a plan path is covered by prior context, attach the matched
    owner/supporting/scope anchors to `ChangePlan.LocalizationReview.Anchors`.
    Observed read-file anchors still do not satisfy localization coverage.
  - Project `LocalizationReview.Anchors` back into the change-plan context pack
    as typed `localization_anchor` rows with evidence refs preserved, so
    controller/planner/verifier Top-N views can consume anchor strength,
    symbol, line ref, and stage without mining summary text.
- Task list:
  - [x] Audit current RC97/RC98 localization anchor production and consumption.
  - [x] Record RC104 design and sorted priority in this ledger.
  - [x] Add typed prior-anchor extraction from write context packs.
  - [x] Carry matched prior anchors into plan localization reviews.
  - [x] Project plan localization anchors into downstream context packs.
  - [x] Add focused tests for owner/scope anchor carry-through and observed
    anchor exclusion.
  - [x] Run related/full regressions, rebuild, commit, and push.
- Verification:
  - `go test ./internal/types -run
    'TestSourceLocalizationReviewFromWritePlanContext|TestWriteContextPackFromChangePlanCarriesSourceLocalizationReview|TestWritePlanSourcePathsOutsidePriorContext'
    -count=1`
  - `go test ./internal/types ./internal/orchestrator ./internal/agent
    ./internal/writeflow -count=1`
  - `go test ./...`
  - `make`

## 2026-06-19 RC-105 Cumulative PatchEffect Owned-path Boundary

- Evidence:
  - The post-RC104 three-instance SWE-bench Lite smoke wrote results under
    `eval/results/swebench/lite-smoke-20260619-024215`.
  - `validate_predictions.py` accepted 3 prediction rows and official harness
    import/dry-run accepted the JSONL. This proves export compatibility, not
    functional correctness.
  - `astropy__astropy-12907` and `astropy__astropy-14182` exported clean
    non-empty source patches, but the workflow blocked on
    `patch_effect_path_outside_plan_scope`.
  - Manual audit showed the cumulative PatchEffect included verify/build
    generated tracked artifacts such as `cextern/wcslib/*` in addition to the
    owned source file. The official exported patch did not contain those files.
- Gap:
  - Per-apply `PatchEffectRecord` already captures the single applied commit.
    The pollution came from cumulative review using `git diff base..HEAD`
    across the whole worktree after verify/build side effects.
  - Treating those generated artifacts as hard patch-review scope evidence can
    block a valid source patch, and it makes controller repair reason over
    files the plan never owned.
- Design:
  - Keep cumulative review, because it is needed when a later test-only proof
    batch must review an earlier source patch.
  - Limit cumulative diff capture to a deterministic allowlist derived from
    durable applied `ChangePlan` artifacts: `target_paths`, `applied_paths`,
    `changes[].path`, and `changes[].new_path`.
  - Normalize allowlist entries as repo-relative pathspecs and reject absolute,
    parent-escaping, or git pathspec-magic paths before passing them to git.
  - If no durable owned paths exist, skip cumulative review rather than falling
    back to whole-worktree diff. This fails narrow and preserves the typed
    boundary.
  - Generated verify/build artifacts may remain telemetry elsewhere, but they
    cannot become cumulative actual-diff hard blockers unless a typed plan
    explicitly owned them.
- Task list:
  - [x] Add a path-limited worktree diff helper with repo-relative path
    normalization and unsafe-path rejection.
  - [x] Derive cumulative review owned paths from durable applied plan
    artifacts.
  - [x] Switch cumulative PatchEffect source to
    `workflow_cumulative_owned_diff`.
  - [x] Add worktree regression for excluding unowned generated artifacts.
  - [x] Add controller regression proving cumulative follow-up keeps source
    evidence while ignoring verify-generated artifacts.
  - [x] Run focused, related, full Go regressions, rebuild, diff check, commit,
    and push.
- Verification:
  - `go test ./internal/worktree -run
    'TestCaptureRangePatchForPaths|TestCaptureRangePatchCapturesCumulativeDiff'
    -count=1`
  - `go test ./internal/orchestrator -run
    'TestAppendCumulativePatchReviewFollowupCoversSourcePatchAfterTestOnlyPlan'
    -count=1`
  - `go test ./internal/worktree ./internal/orchestrator ./internal/writeflow
    ./internal/types -count=1`
  - `go test ./...`
  - `make`
  - `git diff --check`

## 2026-06-19 RC-106 Same-batch Source-owner Relation After Test-only Replan

- Evidence:
  - In the same RC104 smoke, `astropy__astropy-14365` reached workflow
    `complete` with typed verify passed, but prediction export was empty.
  - The earlier plan modified `astropy/io/ascii/qdp.py` and its test. Verify
    failed on the test assertion. After checkpoint restore, a later replan
    changed only `astropy/io/ascii/tests/test_qdp.py` and passed.
  - The final delivery candidate treated the terminal test-only plan as the
    whole delivery, losing the source-owner relation to the earlier source
    patch in the same coherent batch.
- Design:
  - Use typed workflow attempts, checkpoint restore events, plan IDs,
    PatchReview records, and final verify reports to build a coherent delivery
    chain.
  - A later test-only replan can prove an earlier source-owner plan only when
    the earlier source patch remains in the worktree lineage after restore and
    has no current actual-diff hard blocker.
  - Export/report source-owner patches from that coherent chain; do not parse
    model prose, final summaries, issue text, or logs.
  - If restore events prove the earlier source patch was rolled back, exclude it
    as RC-84 already requires.
  - Distinguish two restore meanings through typed slice events:
    progress-ledger restore cutoffs without precise restored plan metadata still
    exclude older attempts, while `slice_restored` events that point to
    `plan_id` or `refs/codrax/applied/<plan_id>` retain that restored plan in
    the active delivery lineage.
  - Final reports now include a typed `delivery` summary so normal write-mode
    users and auditors can see the source-owner plan relation even when the
    terminal plan is test-only.
- Task list:
  - [x] Audit delivery candidate construction in core final report and
    `eval/swebench/run_codrax_swebench.py`.
  - [x] Add a typed delivery relation such as
    `source_plan_with_later_same_batch_test_replan`.
  - [x] Preserve source-owner plan export when terminal verification is
    test-only but proves the coherent source patch.
  - [x] Add adapter/core regressions for source+test initial plan, failed
    assertion, restore, test-only repair, passed verify, non-empty export.
  - [x] Run focused, related, full regressions and commit/push this batch.
  - [ ] Run a fresh Lite smoke after the RC-106 code batch is on `main`.
- Implementation:
  - `eval/swebench/run_codrax_swebench.py` now keeps restored source-owner plan
    ids in `workflow_applied_plan_ids()` when typed `slice_restored` events
    prove that the checkpoint was restored into the active lineage.
  - `BuildWriteFinalReport` now carries `delivery`, a typed summary containing
    final/report/source-owner plan ids, source/test paths, relation, and
    test-only/validation-only terminal semantics.
  - `persistWriteFinalReportIfTerminal` computes that delivery summary from
    durable workflow attempts, restored slice events, and durable `ChangePlan`
    artifacts.
- Verification so far:
  - `eval/results/swebench/.venv/bin/python
    eval/swebench/run_codrax_swebench_test.py`
  - `go test ./internal/orchestrator -run
    'TestPersistWriteWorkflowRunTerminalWritesFinalReport|TestPersistWriteWorkflowRunFinalReportPreservesRestoredSourceOwnerForTestOnlyReplan|TestPersistWriteWorkflowRunTerminalAggregatesCompletedBatchProofReports'
    -count=1`
  - `go test ./internal/types -run
    'TestBuildWriteFinalReport|TestWriteOutputKindIncludesFinalReport'
    -count=1`
  - `go test ./internal/orchestrator ./internal/types ./internal/writeflow
    ./internal/worktree -count=1`
  - `go test ./...`
  - `make`
  - `git diff --check`
  - Offline recompute of
    `eval/results/swebench/lite-smoke-20260619-024215/instances/astropy__astropy-14365`
    now selects `plan-1781809191030045000-69938` as source-owner export,
    exports `astropy/io/ascii/qdp.py`, and reports relation
    `source_plan_with_later_same_batch_test_replan`.

## 2026-06-19 RC-108 Apply Checkpoint Owned-path Boundary

- Evidence:
  - Fresh RC106 smoke at
    `eval/results/swebench/lite-smoke-20260619-rc106-3` produced 3/3
    harness-consumable non-empty predictions and official harness dry-run/import
    accepted the JSONL.
  - The smoke also showed all three workflows blocked locally after apply/replan
    with `patch_effect_path_outside_plan_scope:cextern/wcslib/C/fitshdr.c`.
  - RC105 already limited cumulative PatchEffect review to durable plan-owned
    paths, so this new evidence means the generated artifact entered the apply
    checkpoint commit itself before cumulative review.
- Gap:
  - `worktree.CommitChanges` uses `git add -A`, which stages every dirty,
    deleted, and untracked file in the worktree.
  - In SWE/customer project environments, editable installs, partial builds, or
    verification probes can leave generated files dirty before or during the
    apply stage. Those files are not plan-owned but can become part of the
    applied ref and actual PatchEffect, causing hard PatchReview blockers and
    confusing repair loops.
- Design:
  - Keep `CommitChanges` for legacy callers and tests that intentionally want a
    whole-worktree checkpoint.
  - Add `CommitChangesForPaths`, which resets the index and stages only a
    typed repo-relative path allowlist. It rejects parent escapes and git
    pathspec-magic inputs through the same structural path normalizer as
    path-limited diff capture.
  - Wire write apply checkpoint commits to `CommitChangesForPaths` using
    deterministic plan-owned paths: `target_paths`, `applied_paths`,
    `changes[].path`, and `changes[].new_path`.
  - Empty owned-path lists create an empty checkpoint commit and never fall
    back to whole-worktree `git add -A`.
  - This is language-agnostic and does not inspect user wording, issue text,
    model rationale, logs, stdout prose, or `<think>` output.
- Task list:
  - [x] Add path-limited apply checkpoint commit helper in `internal/worktree`.
  - [x] Switch write apply post-hook checkpoint commits to typed plan-owned
    paths.
  - [x] Add worktree regression proving unowned generated files remain
    uncommitted.
  - [x] Add apply post-hook regression proving applied PatchEffect commits only
    plan-owned files.
  - [x] Run related/full regressions, rebuild, commit, push, and rerun a fresh
    SWE Lite smoke.
- Verification:
  - `go test ./internal/worktree -run
    'TestCommitChangesForPaths|TestCommitChanges_RoundTrip' -count=1`
  - `go test ./internal/orchestrator -run
    'TestApplyPostHookCheckpointCommitKeepsOnlyPlanOwnedPaths|TestPersistVerifyFailureEvidence'
    -count=1`
  - `go test ./internal/worktree ./internal/orchestrator ./internal/types
    ./internal/writeflow -count=1`
  - `go test ./...`
  - `make`
  - `git diff --check`
  - Fresh smoke:
    `WORKDIR=eval/results/swebench/lite-smoke-20260619-rc108-3
    SWEBENCH_SMOKE_LIMIT=3 SWEBENCH_FAIL_ON_EMPTY_PATCH=0
    SWEBENCH_REQUIRE_NONEMPTY_PATCH=0 SWEBENCH_FAIL_ON_INSTANCE_ERROR=0
    CODRAX_BIN=/Users/han/opt/codrax/codrax
    eval/swebench/smoke_lite.sh`
- RC108 smoke result:
  - Predictions: 3/3 non-empty; official harness dry-run/import accepted
    `eval/results/swebench/lite-smoke-20260619-rc108-3/predictions.jsonl`.
  - `grep` over plan artifacts found no
    `patch_effect_path_outside_plan_scope:cextern` generated-path blocker.
  - `astropy__astropy-14365` reached `workflow_status=complete`,
    `verify_status=passed`, `prediction_verdict=predicted_passed`, and
    `prediction_local_confidence=high`.
  - Remaining gaps are no longer generated-path PatchEffect pollution:
    `astropy__astropy-12907` failed local verification due Astropy source
    checkout extension import error; `astropy__astropy-14182` completed
    unverified with `make_target_missing` plus probe import errors and stayed
    audit-blocked on proof coverage.

## 2026-06-19 RC-109 Complete Verification Environment/probe Unavailable Authority

- Evidence:
  - RC108 smoke shows the write pipeline now gets past apply PatchEffect
    pollution and reaches proof/verification surfaces.
  - `astropy__astropy-12907` still becomes `predicted_failed_verify` because a
    bounded probe imports Astropy from an unbuilt source checkout and raises the
    project-standard extension-build ImportError.
  - `astropy__astropy-14182` completes unverified with `make_target_missing`
    and `verification_probe_import_error`, while local acceptance still blocks
    on changed-symbol proof coverage.
- Gap:
  - The verifier already has typed unavailable states, but source-checkout
    extension import errors and generated/native make target gaps are not yet
    consistently promoted to environment/probe-unavailable authority before
    they drive source replan or hard local failure.
  - Probe import failures need a repair/selection layer that can choose a more
    local import surface or classify the project environment as unavailable
    without asking the model to infer from traceback prose.
- Design:
  - Add one shared `ChangeReport` evidence authority that can answer "are all
    red rows explained by typed verifier/probe/environment unavailability?" so
    `run_tests`, controller observation authority, final reports, and eval
    adapters do not each invent their own interpretation.
  - Add typed environment-unavailable classifiers for common source-checkout
    build-extension boundaries using structured exception class/module/path
    frames and existing environment diagnostics, not natural-language matching
    over model summaries.
  - Normalize make target unavailable records so runner/working-dir/target are
    structured; avoid malformed synthetic targets such as
    `make@cextern/wcslib::make` becoming proof authority.
  - When PatchEffect is coherent and verifier evidence is environment/probe
    unavailable, prefer `unverified` completion and proof telemetry over source
    replan loops, unless a typed product-code frame/changed-line diagnostic
    proves the patch caused the failure.
  - Keep local acceptance conservative: unavailable proof is not a pass, but it
    should not masquerade as a functional failure or cause broad source edits.
- Task list:
  - [x] Audit verification probe failure normalization and command-derived
    unavailable reason aggregation.
  - [x] Add shared `ChangeReport` unavailable-evidence authority for probe
    import failures, unavailable command evidence, and qualified make rows.
  - [x] Add typed classifier for generated make dependency/include file
    unavailability.
  - [x] Normalize TestSurface candidate IDs embedded in `run_tests.suite` into
    typed runner/framework/working_dir/suite parameters before execution.
  - [x] Preserve source-checkout build-extension import errors as typed
    `verification_probe_import_error` unavailable evidence when the probe layer
    emits that structured reason.
  - [x] Normalize make target unavailable command fields and prevent malformed
    runner/target concatenation.
  - [x] Update observation authority so coherent patch + unavailable proof
    produces unverified completion instead of source-code failure loops.
  - [x] Add focused verifier/tool/writeflow regressions.
  - [x] Run related/full regressions, rebuild, commit, push, and rerun Lite
    smoke.
- Verification:
  - `go test ./internal/types -run
    'TestChangeReportNormalizeVerificationStatus|TestChangeReportEnsureVerificationStatusBackfillsNoTestsFailureKind'
    -count=1`
  - `go test ./internal/writeflow -run
    'TestDeriveObservationAuthorityFromReport|TestDeriveObservationAuthorityFromAttempt|TestClassifyVerifyAttemptOutcome'
    -count=1`
  - `go test ./internal/tool -run
    'TestRunTestsVerificationProbeProductNameErrorIsTestFailure|Test(ParseMakeOutput_(MissingTargetIsUnavailable|PythonModuleMissingIsUnavailable|DependencyFileMissingIsUnavailable|DependencyFileMissingWithoutNoRuleIsUnavailable)|NormalizeRunTestsCandidateSuiteSelector)'
    -count=1`
  - `go test ./internal/types ./internal/writeflow ./internal/tool
    ./internal/orchestrator -count=1`
  - `go test ./...`
  - `make`
  - `git diff --check`
  - Fresh smoke:
    `WORKDIR=eval/results/swebench/lite-smoke-20260619-rc109-3
    SWEBENCH_SMOKE_LIMIT=3 SWEBENCH_FAIL_ON_EMPTY_PATCH=0
    SWEBENCH_REQUIRE_NONEMPTY_PATCH=0 SWEBENCH_FAIL_ON_INSTANCE_ERROR=0
    CODRAX_BIN=/Users/han/opt/codrax/codrax
    eval/swebench/smoke_lite.sh`
- RC109 smoke result:
  - Predictions: 3/3 non-empty; official harness import/dry-run command
    accepted
    `eval/results/swebench/lite-smoke-20260619-rc109-3/predictions.jsonl`.
  - `astropy__astropy-12907` and `astropy__astropy-14182` now complete with
    `verify_status=unavailable` instead of false functional failure; terminal
    reports carry typed reasons
    `verification_probe_import_error,make_dependency_file_missing,make_target_missing`.
  - `astropy__astropy-14365` remains `workflow_status=complete`,
    `verify_status=passed`, `prediction_verdict=predicted_passed`, and
    `prediction_local_confidence=high`.
  - Remaining blockers are proof/audit confidence and source-owner scheduling,
    not verifier unavailable being misread as product-code failure. During
    `astropy__astropy-14182`, `plan_batch` also spent more than three minutes
    before progressing; record this as a follow-up UX/perf budget signal, not a
    correctness blocker.

## 2026-06-19 RC-107 In Progress P0 Shared Typed Localization Owner/evidence Scheduling Authority

Priority: **next systemic batch after RC-109 lands**. This is the main upstream
gap behind wrong-source-surface SWE patches and must not be displaced by
verifier-only hardening unless a regression blocks mainline stability.

- Evidence:
  - The user-selected follow-up explicitly calls out that upstream typed
    localization owner/evidence anchors are still the main unfinished gap:
    plans can still reach wrong source surfaces before patch critic or verifier
    gets a chance to correct them.
  - RC97/RC98/RC104 created and preserved anchor artifacts, but they do not yet
    form one shared read/write scheduling authority across exploration,
    extraction, planning, replan, and final report consumption.
- Gap:
  - Read mode can discover rich evidence and write mode can carry context
    packs, but controller/planner still often see path sets or broad hints
    before an explicit ranked owner-anchor view.
  - Handoff quality must be measured by whether later patch surface changes in
    response to owner evidence, not merely by whether a context pack exists.
- Design:
  - Define a single typed owner-anchor view consumed by read finalization,
    write controller, planner, replan repair, verifier, PatchReview/Impact, and
    final delivery report.
  - Anchor priority is structural: owner/support/scope/observed kind, evidence
    refs, line span, symbol, path role, source stage, confidence, and consumer.
    No hard gate may inspect user-intent keywords, model rationale, summaries,
    or stdout narrative.
  - Exploration should expand from weak/missing owner anchors; planning should
    prefer owner anchors over broad target files; replan should keep failed
    evidence near the owner slice; final answer/report should expose which
    anchors justified the chosen patch surface.
  - Existing `SourceLocalizationAnchor`, `WriteContextPack`, repomap,
    `ImpactAnalysisResult`, and `PatchReviewRecord` are reused rather than
    introducing a parallel localization system.
- Task list:
  - [x] Audit current anchor producers and consumers across read analyze,
    explore, extract/finalize, write analysis, context pack projection,
    controller decisions, planner hints, replan repair, verifier confidence,
    PatchReview/Impact, and final reports.
  - [x] Add a normalized ranked `OwnerAnchorView` helper in shared types.
  - [x] Preserve read-mode `EvidenceItem.OwnerSymbol` through
    `WriteExplorationEvidenceRef`, `SourceLocalizationAnchor`,
    `OwnerAnchorViewItem`, context pack render/dedupe, and final evidence-ref
    identity without changing L1 read scheduler byte identity.
  - [x] Project selected write-plan and handoff owner anchors into typed final
    report fields for audit.
  - [x] Project read-mode extraction/finalization owner evidence into read
    final-answer owner-anchor fields beyond the write handoff/report layer.
  - [x] Make write planner context-pack view and handoff-material budgeting
    consume `OwnerAnchorView` before broad target-file hints.
  - [x] Record chosen owner-anchor IDs on each `ChangePlan` when the controller
    attaches plan context.
  - [x] Make write controller/replan repair consume `OwnerAnchorView` before broad
    target-file hints when choosing follow-up repair/explore slices.
  - [x] Add handoff tests proving rich read evidence survives to write planning
    through typed artifacts.
  - [x] Add tests proving owner-anchor evidence changes the selected
    source-owner surface rather than only being retained.
  - [x] Add final report fields that show chosen owner anchors for audit.
  - [x] Add final report fields that show unresolved owner-anchor gaps for audit.
  - [ ] Run read-mode isolation tests, write-mode focused/related/full
    regressions, SWE Lite smoke, and manual patch-surface audit.
- RC107-A implementation notes:
  - Added `OwnerAnchorView` / `OwnerAnchorViewItem` as the first shared typed
    ranking view over `SourceLocalizationAnchor`, scoped `WriteContextPack`, and
    `WriteExplorationHandoff` evidence.
  - Planner context-pack rendering now ranks strong `localization_anchor` items
    before broad P1 `target_file` / `evidence_ref` rows and preserves strong
    owner/support anchors in limited and bounded pack views.
  - Planner handoff-material detection now counts strong read/write
    `SourceLocalizationReview` anchors even when no broad target-file list is
    present, while keeping write-analysis-only scope anchors from activating
    exploration handoff synthesis.
  - Prompt change is soft guidance only: `localization_anchor` /
    source-localization rows join existing planner coverage guidance, but hard
    gates still consume typed artifacts.
  - Focused verification:
    `go test ./internal/types -run
    'Test(OwnerAnchorView|WriteContextPackPlanner.*Owner|WriteContextPackPlannerLimitedViewRetainsVerifyFailureLane|WriteContextPackBudgetedViewRetainsSafetyAndFailure)'
    -count=1` and `go test ./internal/agent -run
    'TestPlannerHandoffSynthesisReadBudget|TestPlannerWriteContextPack'
    -count=1`.
  - Related/full verification:
    `go test ./internal/types ./internal/agent ./internal/orchestrator
    -count=1`, `go test ./...`, `make`, and `git diff --check`.
  - SWE Lite smoke:
    `WORKDIR=eval/results/swebench/lite-smoke-20260619-rc107a-3
    SWEBENCH_SMOKE_LIMIT=3 SWEBENCH_FAIL_ON_EMPTY_PATCH=0
    SWEBENCH_REQUIRE_NONEMPTY_PATCH=0 SWEBENCH_FAIL_ON_INSTANCE_ERROR=0
    CODRAX_BIN=/Users/han/opt/codrax/codrax
    eval/swebench/smoke_lite.sh`
  - RC107-A smoke result:
    predictions were 3/3 non-empty and official harness import/dry-run command
    was accepted. Patch surfaces stayed on plausible source owners:
    `astropy/modeling/separable.py`, `astropy/io/ascii/rst.py`, and
    `astropy/io/ascii/qdp.py`. Manual audit: all three patches are
    theoretically aligned with their issue behavior; `astropy__astropy-14365`
    reached high-confidence local pass, while `astropy__astropy-12907` and
    `astropy__astropy-14182` remain local-audit blocked by typed unavailable
    Astropy checkout/build verification reasons rather than wrong source
    surface.
  - Newly exposed RC107-B gap: context packs now carry and rank localization
    anchors, but many producer-side anchors are still `supporting` with broad
    local symbols such as branch calls or helper invocations instead of
    line-backed owner/member anchors. The next batch must improve read-mode
    exploration/extraction owner attribution and project those owner-strength
    anchors through the same `OwnerAnchorView`; otherwise write mode can rank
    anchors, but it cannot create stronger evidence than read/explore produced.
- RC107-B implementation notes:
  - The first concrete owner-attribution leak was in the typed handoff schema,
    not in a single SWE case: `EvidenceItem.OwnerSymbol` existed, but
    `WriteExplorationEvidenceRef`, `SourceLocalizationAnchor`, and
    `OwnerAnchorViewItem` only preserved `Subject` and `AnchorSymbol`. This let
    planner-visible anchors degrade to local tokens such as `if`, `super`, or
    `split` even when read-mode evidence had an owner/member symbol.
  - Added durable `owner_symbol` fields to those shared artifacts and threaded
    them through `SourceLocalizationReviewFromTurnA`, exploration handoff,
    context pack rendering/stable IDs/dedupe keys, owner-anchor ranking, and
    final evidence-ref identity. Duplicate context/evidence merges now retain a
    later owner symbol when the earlier row only had a local anchor token.
  - Hard routing remains typed-only. The new field is copied from structured
    evidence and normalized structs; no user intent keyword, model rationale,
    stdout narrative, or `<think>` prose participates in the decision.
  - Focused verification:
    `go test ./internal/types -run
    'Test(SourceLocalization|OwnerAnchorView|WriteContextPack.*Localization|WriteContextPackPlanner.*Owner|WriteContextPackPlannerLimitedViewRetainsOwnerAnchor)'
    -count=1`
  - Related verification:
    `go test ./internal/types ./internal/agent ./internal/orchestrator -run
    'Test(WriteExplorationHandoff|PlannerHandoffSynthesisReadBudget|PlannerWriteContextPack|RunController.*Localization|RunController.*Handoff|WriteContext|SourceLocalization)'
    -count=1`
- RC107-C implementation notes:
  - Added `ChangePlan.OwnerAnchors` as lifecycle/audit metadata excluded from
    `PlanFingerprint`. The controller stamps it from the typed
    `SourceLocalizationReview` inside `attachPlanContextPackToWorkflowRun`, so
    plans now record which prior owner/support anchors justified the source
    surface.
  - Added final-report `plan.owner_anchors` and `handoff.owner_anchors`. The
    former answers which anchors this plan consumed; the latter answers which
    owner anchors were available from accumulated context packs. Both are
    normalized `OwnerAnchorViewItem` rows with stable IDs and typed evidence
    refs.
  - This still leaves direct replan-slice selection from `OwnerAnchorView` as a
    remaining RC107 task; this batch records the evidence deterministically and
    gives later controller decisions a typed consumer surface.
  - Focused verification:
    `go test ./internal/types -run
    'Test(OwnerAnchorView|BuildWriteFinalReport|WriteFinalReport)' -count=1`
    and `go test ./internal/orchestrator -run
    'TestAttachPlanContextPackToWorkflowRun.*|TestPersistWriteWorkflowRunTerminalWritesFinalReport'
    -count=1`.
- RC107-C SWE smoke/audit:
  - Ran
    `WORKDIR=/Users/han/opt/codrax/eval/results/swebench/lite-smoke-20260619-rc107c-3
    SWEBENCH_SMOKE_LIMIT=3 SWEBENCH_FAIL_ON_INSTANCE_ERROR=0
    SWEBENCH_FAIL_ON_EMPTY_PATCH=0 SWEBENCH_REQUIRE_NONEMPTY_PATCH=0
    CODRAX_BIN=/Users/han/opt/codrax/codrax eval/swebench/smoke_lite.sh`.
  - Result: 3/3 non-empty predictions and official harness import/dry-run
    command accepted. Local summary:
    `current_core=3/3`, `non_empty_patch=3/3`, `high_conf_local_verify=1/3`,
    `final_report=3/3`.
  - Manual patch audit:
    `astropy__astropy-12907` changed `astropy/modeling/separable.py` from
    constant `1` to `right` in `_cstack`, theoretically aligned with expected
    separability behavior but locally unverified due typed environment/probe
    unavailable evidence.
    `astropy__astropy-14182` changed `astropy/io/ascii/rst.py` to thread
    `header_rows`, theoretically aligned but locally unverified for the same
    environment/probe reasons.
    `astropy__astropy-14365` changed QDP line-type regex compilation to
    `re.IGNORECASE`, reached `verify_status=passed` and high local confidence.
  - Newly exposed RC107-D gap: the final-report owner-anchor fields are present
    and exported, but `owner_symbol` remained empty for several real anchors.
    The evidence had typed `subject=source_path:symbol` rows such as
    `astropy/modeling/separable.py:separability_matrix`, while the local
    `anchor_symbol` was a statement token such as `if` or `np.where`. The next
    producer-side fix should infer owner symbols from this exact typed
    `source_path:symbol` shape and feed both read-mode handoff and write-mode
    localization, without parsing user intent or model prose.
- RC107-D implementation notes:
  - Added a conservative typed owner fallback in shared types: when
    `EvidenceItem.OwnerSymbol` is empty but `EvidenceItem.Subject` exactly
    starts with the normalized `EvidenceItem.Source` path (or basename)
    followed by `:` / `::`, the suffix is accepted as `owner_symbol` only if it
    is a compact identifier-shaped symbol. This consumes structured evidence
    fields only; summaries, rationale, stdout, issue text, and `<think>` are
    ignored.
  - `SourceLocalizationReviewFromTurnA` and
    `WriteExplorationHandoffFromTurnA` now use the same helper, so read-mode
    TurnA artifacts and write-mode context packs see one consistent owner
    projection.
  - Focused/related verification:
    `go test ./internal/types -run
    'Test(SourceLocalizationReviewFromTurnA.*Owner|WriteExplorationHandoffFromTurnAInfersOwner|OwnerAnchorView|BuildWriteFinalReport)'
    -count=1`
    and `go test ./internal/types ./internal/agent ./internal/orchestrator -run
    'Test(SourceLocalization|WriteExplorationHandoff|TurnA|AnswerDocument|Explorer|AttachPlanContextPack)'
    -count=1`.
- RC107-D SWE smoke/audit:
  - Ran
    `WORKDIR=/Users/han/opt/codrax/eval/results/swebench/lite-smoke-20260619-rc107d-3
    SWEBENCH_SMOKE_LIMIT=3 SWEBENCH_FAIL_ON_INSTANCE_ERROR=0
    SWEBENCH_FAIL_ON_EMPTY_PATCH=0 SWEBENCH_REQUIRE_NONEMPTY_PATCH=0
    CODRAX_BIN=/Users/han/opt/codrax/codrax eval/swebench/smoke_lite.sh`
    after commit `4577db8f`.
  - Result: 3/3 non-empty predictions and official harness import/dry-run
    command accepted. Local summary:
    `current_core=3/3`, `non_empty_patch=3/3`, `high_conf_local_verify=1/3`,
    `final_report=3/3`.
  - Manual patch audit remained theoretically aligned for all three patches:
    `_cstack` now writes the right separability matrix block, RST now threads
    `header_rows`, and QDP parsing compiles the line-type regex with
    `re.IGNORECASE`. The first two remain locally unverified because the
    checked-out Astropy environment is dependency/probe unavailable; the third
    reached `verify_status=passed`.
  - Owner-anchor audit: `astropy__astropy-14182` now exports plan owner symbols
    `RST.__init__`, `RST.write`, and
    `SimpleRSTHeader.get_fixedwidth_params`, proving the typed
    `source_path:symbol` fallback is working. `astropy__astropy-12907` and
    `astropy__astropy-14365` still export only scope anchors because the run's
    prior context contains no evidence refs at all, only write-analysis
    scope paths. This is a distinct producer/scheduler gap: path coverage is
    being treated as sufficient localization depth.
- RC107-E owner-depth critique:
  - Added a typed owner-depth distinction to `WriteContextPack` coverage:
    `plan_context_coverage` still records path coverage, while
    `plan_context_owner_depth` and `plan_context_owner_gap_path` report
    production source paths that are covered only by broad path context and
    lack a P0/P1 typed owner/evidence localization anchor.
  - Added `WritePlanSourcePathsWithoutOwnerAnchor`, derived only from
    normalized `WriteContextPack`, `SourceLocalizationAnchor`, and
    `ChangePlan` paths. Scope anchors intentionally do not satisfy owner-depth
    evidence; grounded evidence refs, owner symbols, subjects, or anchor
    symbols do.
  - The controller plan loop now performs one bounded retry with a
    `Typed owner localization depth critique` hint when a plan edits a
    path-covered source file without owner/evidence anchors. This is soft
    guidance and audit context, not an apply-risk denial: after the single retry
    the workflow can still deliver low/medium-risk code with residual
    localization risk recorded. The retry is intentionally lower priority than
    off-scope high-risk, protected-test-contract, and missing-localization
    critiques, so it does not stack extra plan rounds after those stronger
    structural corrections have already fired.
  - Remaining follow-up after RC107-F: direct controller/replan slice selection
    from `OwnerAnchorView` and read final-answer projection still need separate
    batches.
- RC107-E SWE smoke/audit:
  - Ran
    `WORKDIR=/Users/han/opt/codrax/eval/results/swebench/lite-smoke-20260619-rc107e-12907
    INSTANCE_ID=astropy__astropy-12907 SWEBENCH_SMOKE_LIMIT=1
    SWEBENCH_FAIL_ON_INSTANCE_ERROR=0 SWEBENCH_FAIL_ON_EMPTY_PATCH=0
    SWEBENCH_REQUIRE_NONEMPTY_PATCH=0 CODRAX_BIN=/Users/han/opt/codrax/codrax
    eval/swebench/smoke_lite.sh` after commit `94802602`.
  - Result: 1/1 non-empty prediction and official harness import/dry-run
    command accepted. Local summary:
    `current_core=1/1`, `non_empty_patch=1/1`, `final_report=1/1`,
    `high_conf_local_verify=0/1` because the Astropy checkout still reports
    typed dependency/probe unavailable evidence.
  - Final-report owner audit improved from the RC107-D run:
    `final_report_plan_owner_symbols=["separability_matrix"]` and
    `final_report_handoff_owner_symbols=["Model._calculate_separability_matrix",
    "separability_matrix"]`. The selected patch remains the expected
    `_cstack` matrix-block repair (`1` -> `right`) and the prediction remains
    conservatively `predicted_audit_blocked` due unavailable verification proof,
    not due patch emptiness or harness incompatibility.
- RC107-F planner observation writeback:
  - `read_file` now attaches a typed `ObservationRecord` side channel for
    successful current-source reads: repo-relative path, line window, raw blob
    ref, producer, grounding status, and current-source origin. The visible
    read_file summary is unchanged.
  - Added `WriteContextPackFromPlannerToolResults`, which consumes only
    `ToolResult.Observations` and projects planner/replan current-source
    observations into a durable `planner-observation` context pack. It does not
    parse tool summaries, model rationale, or user prose.
  - The write controller synchronizes this pack into the active durable
    workflow after each plan dispatch and before localization/no-plan critiques.
    A read_file observation is intentionally recorded as
    `SourceLocalizationAnchorReadFile` / `observed`, not owner proof; future
    stronger current-source producers can upgrade to supporting/owner anchors
    only through typed fields.
  - Focused verification:
    `go test ./internal/tool ./internal/types ./internal/orchestrator -run
    'Test(ReadFile_ResolvesAgainstRepoRoot|WriteContextPackFromPlannerToolResults|SyncPlannerObservationContextPack|RunControllerPlanBatch_.*Localization|WriteContextPackFromPlanContextCoverage)'
    -count=1`.
- RC107-G controller owner-anchor repair-slice selection:
  - Added `OwnerAnchorCandidatePaths` and `OwnerAnchorEvidenceRequirements` as
    shared typed projections over `OwnerAnchorView`. They accept only strong
    non-scope owner/evidence anchors as first-class repair/explore candidates;
    broad scope/expected paths remain fallback input and observed `read_file`
    rows still do not satisfy owner proof.
  - `seedControllerBatchPlanningHint` and
    `seedControllerBatchExplorationContext` now consume the same ranked owner
    view before broad `ExpectedPaths`. This changes the typed
    `WriteExplorationRequest.CandidatePaths` order and deterministic evidence
    requirements, so planner, exploration, and structured-edit repair lanes see
    owner-localized paths first without parsing prompt prose.
  - Hard routing remains typed-only: the projection reads
    `SourceLocalizationAnchor`, `OwnerAnchorViewItem`, evidence refs, path role,
    strength, kind, source stage, batch/slice scope, and consumer visibility.
    It does not read user-intent keywords, model summaries/rationale,
    `<think>`, stdout narrative, or manual audit notes.
  - Focused verification:
    `go test ./internal/types ./internal/orchestrator -run
    'Test(OwnerAnchorCandidatePaths|SeedControllerBatchContextPrefersOwnerAnchors)'
    -count=1`.
  - RC107-G SWE smoke/audit:
    ran
    `WORKDIR=/Users/han/opt/codrax/eval/results/swebench/lite-smoke-20260619-rc107g-3
    SWEBENCH_SMOKE_LIMIT=3 SWEBENCH_FAIL_ON_INSTANCE_ERROR=0
    SWEBENCH_FAIL_ON_EMPTY_PATCH=0 SWEBENCH_REQUIRE_NONEMPTY_PATCH=0
    CODRAX_BIN=/Users/han/opt/codrax/codrax eval/swebench/smoke_lite.sh`.
    Result: 3/3 non-empty predictions, official harness prediction validation
    passed, and the official harness command was emitted successfully.
    Manual patch-surface audit:
    `astropy__astropy-12907` patched
    `astropy/modeling/separable.py` `_cstack` from scalar `1` to the right
    matrix block; `astropy__astropy-14182` patched
    `astropy/io/ascii/rst.py` `RST.__init__` / `RST.write` header-row
    handling; `astropy__astropy-14365` patched
    `astropy/io/ascii/qdp.py` line-type regex compilation with
    `re.IGNORECASE`. These source surfaces are theoretically aligned with the
    issue behavior.
  - RC107-G smoke exposed one adjacent UX/state gap, not an owner-localization
    regression: `astropy__astropy-14365` reached a verified source batch, then
    appended a proof-only follow-up; the write wall-clock deadline interrupted
    before that proof follow-up could verify, leaving the run `blocked` despite
    a coherent verified source patch. The generalized fix completes such
    proof-only/no-change follow-ups as `unverified` when all non-active source
    batches already have typed completion verdicts and no verify-failure
    handoff exists. Ordinary deadline-after-plan and failed-verify repair
    deadline behavior remains unchanged.
  - Additional focused verification for the deadline hardening:
    `go test ./internal/orchestrator -run
    'TestRunWriteControllerWorkflow_DispatchWriteDeadline(AfterPlanBlocksRun|AfterRepairPlanStaysResumable|ProofFollowupCompletesUnverified)'
    -count=1`.
  - Post-fix single-instance confirmation:
    `WORKDIR=/Users/han/opt/codrax/eval/results/swebench/lite-smoke-20260619-proof-deadline-14365
    INSTANCE_ID=astropy__astropy-14365 SWEBENCH_SMOKE_LIMIT=1
    SWEBENCH_FAIL_ON_INSTANCE_ERROR=0 SWEBENCH_FAIL_ON_EMPTY_PATCH=0
    SWEBENCH_REQUIRE_NONEMPTY_PATCH=0 CODRAX_BIN=/Users/han/opt/codrax/codrax
    eval/swebench/smoke_lite.sh` produced one non-empty prediction, passed
    prediction validation, and finished with `workflow_status=complete`,
    `verify_status=passed`, `prediction_verdict=predicted_passed`, and high
    local confidence. The final patch remained the same aligned QDP
    `re.IGNORECASE` source fix; no proof-only deadline block recurred.
- RC107-H read final-answer owner-anchor projection:
  - Added internal `AnswerDocumentV2.read_owner_anchors` as a deterministic
    projection from read-mode `TurnAArtifacts.SourceLocalization`. The field is
    stamped in the unified answer-document persist path, so full emits and patch
    emits share one code path.
  - The LLM-facing `emit_answer_document` schema is unchanged. Models do not
    emit this field, and hard routing never reads model prose, summaries,
    rationale, `<think>`, stdout, or user keywords. The projection consumes only
    typed `SourceLocalizationAnchor`, `OwnerAnchorViewItem`, evidence refs,
    path role, kind, and strength.
  - The final answer renderer appends a compact
    `系统补充：源码定位锚点核对` / `source-localization anchors` table when
    strong owner/evidence anchors exist. Observed `read_file` anchors and
    broad scope anchors are intentionally filtered out so a file read cannot
    masquerade as owner proof.
  - Focused verification:
    `go test ./internal/tool ./internal/agent -run
    'TestApplyAndPersistMutation_StampsReadOwnerAnchorsFromTurnA|TestAnswerDocumentEvaluator_ParseOutput_.*ReadOwnerAnchor'
    -count=1`.
  - Related/full verification: `go test ./internal/types ./internal/tool
    ./internal/agent -run
    'Test(ApplyAndPersistMutation|AnswerDocumentEvaluator_ParseOutput|AnswerDocumentV2Patch|OwnerAnchor|NormalizeOwnerAnchor|SourceLocalization|Clone|clone)'
    -count=1`, `git diff --check`, `go test ./...`, and `make`.
  - Read-mode smoke: `./codrax --repo . --branch main --request
    "哪个agent可以调用subagent？" --pipeline-max-steps 60 --log-level info`
    answered `AgentExplorer` and rendered the typed
    `系统补充：源码定位锚点核对` table with owner anchors for
    `internal/agent/registry.go`, `internal/agent/subagent.go`, and
    `internal/tool/propose_sub_agents.go`.
- RC107-I final-report owner-gap audit:
  - Added `WriteFinalPlanSummary.owner_anchor_gaps[]` with typed rows
    `{path, reason_code, required_evidence, source}`. Rows are derived only
    from persisted workflow context packs plus the plan's typed production
    source paths via `WritePlanSourcePathsWithoutOwnerAnchor`; no terminal
    logs, model prose, summaries, rationale, `<think>`, or user keywords
    participate.
  - Added residual risk `source_owner_anchor_missing` as a compact audit
    signal when unresolved owner-depth gaps remain at delivery time. This is
    advisory confidence metadata, not an apply/verify gate.
  - Focused verification:
    `go test ./internal/types -run 'TestBuildWriteFinalReport.*Owner' -count=1`.

## 2026-06-19 RC-110 SWE Spot Run And Partial Final-Report Audit

Post-RC107 spot run:

```bash
eval/results/swebench/.venv/bin/python eval/swebench/run_codrax_swebench.py \
  --dataset-name SWE-bench/SWE-bench_Lite --split test \
  --instance-id django__django-14534 \
  --instance-id pytest-dev__pytest-5227 \
  --instance-id sympy__sympy-23117 \
  --workdir eval/results/swebench/lite-smoke-20260619-rc107-post-owner-gap-3 \
  --codrax-bin ./codrax --settings eval/swebench/codrax_swebench.yaml \
  --providers providers.yaml --max-steps 80 --codrax-timeout 1800 \
  --codrax-progress-interval 60 --prepare-python-env --isolate-git-history
```

Verification:

- Prediction schema: `eval/swebench/validate_predictions.py ... --require-nonempty-patch`
  validated 3 predictions, 0 empty patches.
- Official harness dry-run command construction passed with
  `eval/swebench/run_official_harness.sh` and `DRY_RUN=1`.
- Oracle-assisted audit artifact:
  `docs/design/swebench_rc107_post_owner_gap_audit_20260619.md` /
  `.jsonl`; theoretical audit pass 2/3, fail 1/3.

Per-instance outcome:

| instance | exported patch | local/typed verdict | audit result | key signal |
| --- | ---: | --- | --- | --- |
| `django__django-14534` | 416 bytes | `predicted_failed_verify` | fail | Workflow stayed `in_progress` after failed verify and exported patch had no typed final report; audit had to fall back to `codrax.out`. |
| `pytest-dev__pytest-5227` | 512 bytes | `predicted_unverified` | pass | Patch overlapped oracle source/token surface but final report correctly exposed `verification_unavailable`, patch-review unverified test surface, and `source_owner_anchor_missing`. |
| `sympy__sympy-23117` | 1133 bytes | `predicted_passed_low_confidence` | pass | Typed obligation follow-up appended a repair batch and verified locally, but final report retained low-confidence residuals: dependent surface unverified, weak localization, and owner-anchor gap. |

New system gap:

- Terminal-only final-report persistence is too narrow. A write workflow can
  export a useful or at least auditable patch while remaining `in_progress`
  after failed verify/replan interruption. Without a typed final report, SWE
  audit and routine users fall back to terminal prose/log tails, which violates
  the handoff/audit contract.

RC110 design:

- Persist `WriteFinalReport` whenever the workflow is terminal **or** has
  auditable typed delivery evidence: existing `ChangeReport`, applied/failed
  verification plan status, apply/verify attempts, apply/verify refs, or
  apply/verify slice events.
- Keep pure planned/pending non-terminal workflows report-free, preserving
  stable plan-only paths and avoiding noise before any delivery evidence exists.
- Add residual risk `workflow_nonterminal` when a partial final report is
  emitted for an in-progress run.
- Project `plan.owner_anchor_gaps[]` from final reports into SWE results as
  `final_report_plan_owner_gap_*` telemetry so batch analysis does not need to
  reopen the final JSON.
- Hard routing remains typed-only: run status, plan status, `ChangeReport`,
  workflow attempts/events, and final-report fields. No user keywords, model
  rationale, `<think>`, terminal logs, or manual audit prose influence product
  control flow.

RC110 task list:

- [x] Replace terminal-only final-report persistence with auditable-state
  persistence.
- [x] Keep pending/planned non-terminal runs from writing final reports.
- [x] Add `workflow_nonterminal` residual risk for partial final reports.
- [x] Add orchestrator tests for pending non-terminal suppression and
  verify-failed partial final report emission.
- [x] Export final-report owner-gap telemetry in the SWE adapter and update
  adapter tests/docs.
- [x] Rerun the Django failed-verify instance after RC110 to confirm a partial
  final report is present even when workflow remains non-terminal.

RC110 rerun evidence:

- Targeted command:
  `eval/swebench/run_codrax_swebench.py --instance-id django__django-14534 --workdir eval/results/swebench/lite-smoke-20260619-rc110-django-partial-final ... --prepare-python-env --isolate-git-history`.
- Prediction validation: `validate_predictions.py ... --require-nonempty-patch`
  validated 1 prediction, 0 empty patches.
- Official harness dry-run command construction passed with `DRY_RUN=1`.
- During the non-terminal repair loop, typed final reports were persisted for
  `run_status=in_progress` plans with `plan_status=verify_failed` and residual
  risk `workflow_nonterminal`; no audit needed `codrax.out` prose fallback.
- The final adapter row was harness-consumable:
  `status=predicted`, `patch_bytes=420`,
  `prediction_verdict=predicted_passed_low_confidence`,
  `workflow_status=complete`, `verify_status=passed`,
  `final_report_present=true`.
- The selected delivery-candidate final report still carried the partial-run
  audit risk `workflow_nonterminal`, plus
  `source_owner_anchor_missing`; this is intentional because the adapter audits
  the actual delivered patch plan rather than the later terminal no-change plan.
  It keeps the upstream RC111 localization gap visible even when the workflow
  reaches `complete`.

## 2026-06-19 RC-111 Queued P0: Shared Localization Owner/Evidence Pre-Plan Authority

The selected unresolved upstream gap is now explicitly ordered as the next P0
root-cause repair after RC110:

> read/write shared typed localization owner/evidence anchors must let planning
> avoid the wrong source surface before edit generation, not only report the gap
> after delivery.

Why this stays P0:

- Historical SWE audits still show non-empty patches on source surfaces that
  weakly overlap or do not overlap the oracle patch, even when later reporting
  can expose `source_owner_anchor_missing`.
- RC97/RC104/RC107 added the typed anchor contract, handoff preservation,
  ranked planner views, read-mode final-answer stamping, and final-report gap
  telemetry. Those close audit visibility, but not the upstream control loop:
  vague symptom reports still need a deterministic owner-discovery obligation
  before broad path hints can dominate planning.
- This must be shared by read and write modes. If only write mode learns better
  localization, read-mode answers can still drop the same owner/evidence signal
  before handoff; if only read mode learns it, write replan slices can still
  drift after verify failure.

RC111 design:

- Introduce a typed `LocalizationRequirement` view compiled from existing
  `SourceLocalizationAnchor`, `WriteContextPack`, verify-failure handoff, and
  read-mode answer/evidence artifacts. It carries only precise fields:
  `path`, `role`, `kind`, `status`, `priority`, `consumer`, `source_stage`,
  `reason_code`, `required_evidence`, `owner_anchors`, and `evidence_refs`
  whose refs preserve line spans and symbols.
- Planning and replan consume this view before broad expected paths. A plan that
  edits a path with only scope/read-file support and no owner/supporting
  evidence receives a typed repair/exploration obligation; this remains a soft
  convergence loop until deterministic risk/patch-review gates have concrete
  evidence to block.
- Read exploration emits owner-discovery obligations when a question or write
  seed has broad observations but no line-backed owner anchors. This is a typed
  stage-state obligation, not a keyword interpretation of the user request.
- Handoff compaction keeps Top-N owner/supporting anchors plus evidence refs,
  and persists the full pack so controller/planner/verifier can consume their
  own typed slices without replaying raw logs or model rationale.
- Final reports continue to surface unresolved gaps, but RC111 success is
  measured upstream: fewer plans should reach apply with
  `source_owner_anchor_missing`, and replan should target failed owner slices
  instead of adjacent or test-only surfaces.

RC111 task list:

- [x] Audit current producers/consumers of `SourceLocalizationAnchor`,
  `WriteContextPack`, read answer owner anchors, planner localization review,
  and verify-failure repair handoff.
- [x] Add a shared typed `LocalizationRequirement` projection layer with unit
  tests covering owner/supporting/scope/read-file-only distinctions.
- [ ] Make read-mode exploration and final handoff preserve owner-discovery
  obligations without changing the L1 scheduler byte identity.
- [x] Make write controller/planner/replan consume the requirement view before
  broad path hints, and emit bounded exploration/repair obligations when owner
  evidence is missing.
- [x] Add hygiene tests proving no user-intent keywords, model prose,
  `<think>`, stdout narrative, or final-summary text drive localization gates.
- [x] Rerun the existing SWE spot set to compare owner-gap telemetry, patch
  surface, prediction export, and official harness dry-run consumption.
- [ ] Rerun at least one vague symptom-style issue to test end-to-end
  exploration-triggered localization before repair.
- [ ] Update this ledger with measured before/after signals and push the batch
  as a separate commit after RC110 lands.

RC111-A implementation notes:

- Added `LocalizationRequirement` / `LocalizationRequirementSet` as a shared
  typed projection over `SourceLocalizationReview`, `WriteContextPack`,
  `OwnerAnchorView`, candidate paths, and `ChangePlan` source paths.
- Controller planning hints now render typed localization requirements before
  broad expected paths; exploration requests carry those rows before success
  criteria, while owner-anchor candidates remain first.
- Replan localization critiques consume the same requirement set. The old
  stable behavior is preserved: a total absence of prior localization context
  does not become a hard replan loop; it remains exploration/plan-context
  guidance.
- `WriteContextPackFromPlanContextCoverage` persists open
  `localization_requirement` items so controller/planner/verifier handoff can
  carry the obligation as typed context.
- Read-mode synchronization begins at the shared projection boundary:
  `LocalizationRequirementsFromSourceLocalizationReview` converts TurnA
  observed-only source paths into owner-discovery obligations without changing
  the read scheduler. A later RC111 batch should attach these obligations to
  runtime read exploration/final handoff.
- Hygiene test `TestLocalizationRequirementsIgnorePlanNarrativeText` verifies
  that changing `ChangePlan.Summary` does not alter requirements; the new layer
  reads typed path/context/anchor fields only.

RC111-A SWE spot evidence:

- Command: `eval/swebench/run_codrax_swebench.py` over
  `django__django-14534`, `pytest-dev__pytest-5227`, and
  `sympy__sympy-23117` with `--prepare-python-env` and
  `--isolate-git-history`, workdir
  `eval/results/swebench/lite-smoke-20260619-rc111a-localization-requirements-3`.
- Prediction schema validation passed: 3 predictions, 0 empty patches.
- Official harness dry-run command construction passed for all 3 predictions.
- Oracle-assisted audit artifact:
  `docs/design/swebench_rc111a_localization_audit_20260619.md` /
  `.jsonl`; theoretical audit pass 0/3, fail 3/3, with all failures caused by
  typed verify failure.
- Per-instance typed result:

| instance | patch bytes | workflow | verdict | owner-gap telemetry |
| --- | ---: | --- | --- | --- |
| `django__django-14534` | 416 | `in_progress` | `predicted_failed_verify` | `django/forms/boundfield.py`, `plan_source_path_without_owner_anchor` |
| `pytest-dev__pytest-5227` | 512 | `blocked` | `predicted_failed_verify` | `src/_pytest/logging.py`, `plan_source_path_without_owner_anchor` |
| `sympy__sympy-23117` | 496 | `blocked` | `predicted_failed_verify` | `sympy/tensor/array/ndim_array.py`, `plan_source_path_without_owner_anchor` |

RC111-A interpretation:

- The new typed requirement layer made missing owner anchors visible and
  harness-consumable, but it still remains a soft planning/exploration signal.
  Plans can still proceed to apply with `source_owner_anchor_missing` and fail
  verify. RC111-B should decide when an open owner-localization requirement
  must trigger another bounded read/explore cycle before apply, while preserving
  the existing stable behavior that total absence of prior localization context
  does not create an infinite replan loop.
- This is a system gap, not a case-specific bug: the failed patches touch the
  expected source files but do not satisfy the behavioral proof. The next repair
  should join typed owner requirements with verify-failure obligations and
  patch-review/impact signals so replan targets the failing owner slice instead
  of merely carrying a post-hoc residual risk.
- The audit generator was also corrected to report typed final-report artifact
  presence dynamically. The RC111-A audit now states that final reports are
  present for 3/3 instances instead of emitting the old static
  `codrax.out`-only warning.

RC111-B implementation notes:

- Added a controller-owned pre-apply check that consumes
  `LocalizationRequirementsFromWritePlanContext`. If a ModeApply plan still has
  an open owner-localization requirement, the workflow runs one bounded
  read-only exploration before approval/apply and then returns the active batch
  to `ready_to_plan`.
- This remains automatic and low-friction: no user approval is introduced for
  low/medium-risk plans, and this is not a critical/deny gate. The objective is
  to gather stronger typed owner evidence before editing, not to block routine
  safe plans.
- The check is one-shot per batch using typed progress reasons
  `owner_localization_requirement_explored` /
  `owner_localization_requirement_degraded`, preventing exploration loops.
- Existing stability is preserved: a total absence of prior localization
  context still does not trigger the replan loop, plan-only mode is unchanged,
  and proof/no-change plans bypass the pre-apply owner exploration.
- When the exploration fires, stale active-batch `PlanID` / `PlanRef` /
  slice `PlanID` are cleared so the next controller turn replans against the
  new handoff rather than treating the rejected plan as ready.
- Focused test
  `TestOwnerLocalizationRequirementTriggersBoundedExplorationBeforeApply`
  covers the full typed path: scope-only context -> open owner requirement ->
  exploration request with typed row -> owner anchor handoff synced to run ->
  active batch reset for replan.

RC112 implementation notes:

- The post-RC111B Django spot produced a theoretically correct, harness-shaped
  patch against `django/forms/boundfield.py` and passed typed verify, but the
  SWE adapter still marked the prediction as audit-blocked because an earlier
  failed source-owner plan carried
  `python_nested_string_key_direct_access_added`.
- The exported patch's typed delivery source was a later plan using
  `.get("id")` plus fallback; the final plan/report had no PatchReview blocker.
  Therefore the bug was not PatchEffect detection or model repair quality. It
  was delivery-level authority aggregation.
- RC112 adds an `actual_diff_authority_plan_ids` input to delivery
  PatchReview aggregation. Actual-diff/effect local blockers now hard-block only
  when they come from the primary source plan that owns the exported patch.
  Earlier source-owner blockers are retained under
  `non_authoritative_local_blocker_codes` for audit visibility.
- Proof-only blockers remain governed by the existing report-plan authority, so
  a passed typed report can still demote stale proof gaps without weakening real
  actual-diff safety on the exported patch.
- This is a generalized typed-source fix: it does not inspect issue text,
  model prose, summaries, `<think>`, or language-specific user keywords.
- Verification:
  - `eval/results/swebench/.venv/bin/python -m unittest eval.swebench.run_codrax_swebench_test -v`
    passes.
  - `eval/results/swebench/.venv/bin/python -m unittest eval.swebench.run_codrax_swebench_test eval.swebench.audit_historical_results_test -v`
    passes.
  - `python3 -m py_compile eval/swebench/run_codrax_swebench.py eval/swebench/audit_historical_results.py`
    passes.
  - RC112 fresh Django smoke at
    `eval/results/swebench/lite-smoke-20260619-rc112-delivery-authority-django`
    exported a 424-byte non-empty prediction, validated with
    `validate_predictions.py`, and the official SWE-bench harness dry-run
    accepted the prediction file.
- RC112 smoke result:
  - `delivery_primary_source_plan_id` equals the sole
    `plan_patch_review_actual_diff_authority_plan_ids` entry.
  - `plan_patch_review_non_authoritative_local_blocker_codes=[]`, confirming
    the prior stale-source-owner blocker is no longer contaminating delivery
    authority.
  - The run still ended `workflow_status=blocked`, `verify_status=failed` with
    `changed_symbol_without_probe_coverage` and owner-anchor residual risk.
    This is the remaining localization/proof gap, not a PatchReview authority
    aggregation bug.

RC113 implementation notes:

- The RC112 Django smoke exposed a follow-up system gap:
  - attempt 1 failed an existing Django test with
    `None != 'id_name_0'`;
  - attempt 2 fixed the production fallback but also changed the existing test
    assertion to expect `None`, causing the verifier to fail with the inverted
    assertion.
- Existing `write_test_contract_critic` already protected user-declared
  `preserve_regression_test` constraints. RC113 generalizes that mechanism to
  verifier-discovered failed assertions:
  - `VerifyFailureHandoff.FailingTests` is projected through
    `types.ExtractTestFailureSignal`;
  - Python traceback locations of the form `File "...", line N` are parsed into
    the same typed `location=path:line` field as pytest/go-style locations;
  - the controller reads the current repo line at that location, only when the
    path is classified as a test path;
  - if the next `ChangePlan` removes or replaces that exact line, the existing
    bounded `Typed test-contract critique` retries planning once.
- This is not a blanket ban on test edits. It is a typed preservation rule for
  the concrete failed assertion line from the latest verifier evidence. New
  regression tests and production fixes remain allowed.
- Hard routing does not inspect user request keywords, model rationale,
  summaries, `<think>`, or arbitrary terminal prose. The only inputs are typed
  `TestResult`, parsed runner file:line location, repo-relative path role, and
  deterministic plan diff removed lines.
- Verification:
  - `go test ./internal/types -run 'TestExtractTestFailureSignal' -count=1`
    passes.
  - `go test ./internal/orchestrator -run 'Test(TestContractReplanHint|RunWriteControllerWorkflow_ReplansProtectedRegressionTestWeakening)' -count=1`
    passes.

RC114 implementation notes:

- RC113 fresh Django smoke at
  `eval/results/swebench/lite-smoke-20260619-rc113-failed-test-preservation-django`
  proved the first half of the fix was necessary but insufficient:
  - the first failed test traceback location was
    `.codrax/worktrees/<trace>/tests/forms_tests/tests/test_forms.py`;
  - failed-test assertion preservation did not trigger because the critic
    treated the relpath as a Codrax worktree artifact rather than
    `tests/forms_tests/tests/test_forms.py`;
  - the second plan again changed the existing assertion from expecting
    `'id_name_0'` to expecting `None`.
- RC114 fixes the path authority layer, not the Django case:
  - after resolving an absolute failure path under `RepoRoot`, deterministic
    `.codrax/worktrees/<id>/` prefixes are stripped before calling
    `normalizeWorkflowScopePath` and `ClassifySourcePathRole`;
  - the same mapping is applied to already-relative failure locations;
  - arbitrary paths are still rejected by the existing repo-boundary and
    normalize checks.
- Verification:
  - `go test ./internal/orchestrator -run 'TestTestContractReplanHint' -count=1`
    passes, including a regression where a Python traceback points inside
    `.codrax/worktrees/trace-123/tests/test_widget.py`.

RC115 implementation notes:

- RC114 smoke confirmed the worktree-path mapping reached the model: the
  planner saw `preserve_failed_test_assertion` and the protected assertion text.
  However, the controller still accepted a second plan that changed
  `self.assertEqual(..., 'id_name_0')` to `self.assertIsNone(...)`.
- Root cause: `testContractReplanHint` was a one-shot soft retry. After the
  retry was consumed, the scheduler no longer rechecked the same typed
  violation before apply.
- RC115 makes the contract deterministic:
  - first violation: append the existing typed critique and retry planning
    once;
  - repeated violation: return a scheduler error before approval/apply, so the
    workflow becomes blocked/fail-loud instead of applying a weakened local
    oracle.
- This is deliberately scoped to typed protected-test evidence. It does not
  parse model prose about whether a test is "wrong", and it does not forbid
  adding new tests or editing unprotected test lines.
- Verification:
  - `go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_(ReplansProtectedRegressionTestWeakening|BlocksPersistentProtectedRegressionTestWeakening)|TestTestContractReplanHint' -count=1`
    passes.
  - `go test ./internal/orchestrator ./internal/types -count=1` passes.
  - `go test ./...` passes.
  - `make` passes.
- SWE smoke:
  - `eval/results/swebench/lite-smoke-20260619-rc115-protected-test-hard-gate-django`
    on `django__django-14534` now ends with `workflow_status=blocked`,
    `prediction_verdict=empty_patch`, and
    `workflow_latest_progress_reason_code=plan_batch_failed_blocked`.
  - The latest progress message is
    `planner did not produce a ChangePlan after bounded retries: write controller plan weakened protected regression test contract after retry: tests/forms_tests/tests/test_forms.py`.
  - This proves the controller no longer exports or applies a second plan that
    rewrites the protected failed assertion after one structured retry.
  - It does not prove the Django task is solved. The first source patch still
    failed local verification, so the next gap is upstream repair arbitration:
    the system needs a typed way to distinguish "repair implementation while
    preserving failed oracle" from "candidate claims the oracle is wrong" and
    to require an alternate implementation search before any blocked outcome is
    considered terminal.

RC116 design and tasks:

- Gap:
  - RC115 converts unsafe test weakening into a fail-loud blocked workflow.
    That is correct for safety, but it leaves automation rough: when a bounded
    replan keeps editing a protected failed/regression test, the controller
    should still offer one deterministic implementation-side repair lane before
    giving up.
  - The lane must not parse model prose such as "the test is wrong". It should
    consume only the same typed protected-test findings already produced from
    `WriteConstraint` and `VerifyFailureHandoff` diff analysis.
- Design:
  - First protected-test violation: existing typed critique and retry.
  - Second protected-test violation: append a typed
    `protected-oracle repair lane` hint naming the protected paths, record
    `protected_test_source_only_replan_requested`, and retry once.
  - Third protected-test violation: block/fail-loud before approval/apply.
  - The hard gate remains the structural diff critic; the new hint is soft
    guidance only. Protected paths are still rejected if the model emits them.
- Tasks:
  - Add a source-only retry flag in `runControllerPlanBatch`.
  - Add a typed hint renderer fed by protected test paths, not prose.
  - Extend scheduler tests so persistent test weakening gets three attempts,
    records progress, and blocks only after the source-only lane is ignored.
  - Re-run focused orchestrator tests, full `go test ./...`, `make`, then a
    Django SWE smoke to see whether the extra repair lane yields a
    source-only candidate or a clear final block.
- Implementation notes:
  - `runControllerPlanBatch` now retries once with the original typed critique,
    then once more with a `Typed protected-oracle repair lane` hint that names
    forbidden protected test paths and asks for implementation-side repair.
  - The same structural diff critic is re-run on each emitted ChangePlan. If
    the model still changes protected lines after the source-only lane, the
    scheduler blocks before approval/apply.
  - The outer controller now refreshes its local workflow run from mutable
    state after the inner planner loop returns, so progress appended by inner
    repair lanes is not lost when the outer layer records a block.
- Verification:
  - `go test ./internal/orchestrator -run 'TestRunWriteControllerWorkflow_(ReplansProtectedRegressionTestWeakening|BlocksPersistentProtectedRegressionTestWeakening)|TestTestContractReplanHint' -count=1`
    passes.
  - `go test ./...` passes.
  - `make` passes.
- SWE smoke:
  - `eval/results/swebench/lite-smoke-20260619-rc116-protected-oracle-source-lane-django`
    on `django__django-14534` exports a non-empty 500 byte source-only patch
    for `django/forms/boundfield.py`.
  - The exported patch now preserves the failed protected test behavior while
    using the typed source-side alternative:
    `id_ = self.data['attrs'].get('id')` followed by fallback to the old
    `id_%s_%s` value when no id exists.
  - Local verification passed with 10 typed test rows, and the prediction file
    passed `validate_predictions.py --require-nonempty-patch`.
  - Official harness dry-run/import succeeded with
    `PYTHON=eval/results/swebench/.venv/bin/python DRY_RUN=1 CHECK_HARNESS_IMPORT=1`.
  - Remaining gap: the latest source patch is coherent and verified, but a
    later obligation-followup batch left `workflow_status=blocked` and
    `prediction_verdict=predicted_audit_blocked` because patch-review proof
    coverage remained weak. Next batch should split "delivery candidate
    exported and locally verified" from "non-blocking proof/impact follow-up
    incomplete" so an optional audit lane does not poison the primary delivery
    workflow state.

RC117 design and tasks:

- Gap:
  - `batch-1` in RC116 completed with a verified source patch, but
    `batch-1-obligation-repair` was a follow-up proof/impact lane that stopped
    before applying work. The durable run became `blocked`, which made the SWE
    adapter report local audit failure despite exporting the correct source
    patch.
  - Existing `completeDispatchInterruptedProofFollowupIfSourceComplete` covers
    one controller-dispatch deadline shape, but not plan-batch errors where
    mutable state still contains the latest source ChangePlan.
- Design:
  - Add a generic optional follow-up completion helper that consumes only typed
    workflow state:
    active batch purpose is `verification_proof_followup`,
    `impact_and_verification_proof_followup`, or `impact_obligation_followup`;
    all non-active batches are complete; active follow-up has no failed verify
    handoff and no applied attempt.
  - When those conditions hold, mark the active follow-up batch complete with
    `completion.verdict=unverified`, mark the run complete, and record a
    progress reason instead of blocking the primary delivery.
  - Do not complete follow-ups that already applied work or have typed failure
    evidence; those still require verify/replan/block.
- Tasks:
  - Wire the helper into controller dispatch interruption and plan-batch error
    paths before applied-patch interruption guidance.
  - Add tests for follow-up completion and for rejecting applied follow-up
    work.
  - Run focused orchestrator tests, full `go test ./...`, `make`, then rerun
    the Django SWE smoke and update adapter evidence.
- Implementation notes:
  - Added `activeBatchOptionalFollowupPurpose` for verification proof, impact,
    and combined proof/impact follow-up purposes.
  - Added `completeInterruptedFollowupIfSourceComplete`, guarded by active
    follow-up purpose, non-active batch completion, no active verify-failure
    handoff, no active failed verify, and no active applied attempt.
  - Wired the helper before applied-patch interruption guidance in controller
    dispatch and plan-batch error paths, so stale source ChangePlan state cannot
    make an optional follow-up poison the main delivery.
- Verification:
  - `go test ./internal/orchestrator -run 'TestCompleteInterruptedFollowupIfSourceComplete|TestRunControllerPlanBatch_ProofFollowup|TestRunControllerPlanBatch_PureProofFollowup' -count=1`
    passes.
  - `go test ./...` passes.
  - `make` passes.
- SWE smoke:
  - `eval/results/swebench/lite-smoke-20260619-rc117-followup-terminal-django`
    on `django__django-14534` exports a non-empty 427 byte source-only patch
    for `django/forms/boundfield.py`.
  - The durable workflow now ends with `workflow_status=complete` and latest
    progress `plan_batch_followup_unverified` instead of `blocked`.
  - `prediction_blocks_local_acceptance=false`,
    `prediction_verdict=predicted_passed_low_confidence`,
    `verify_status=passed`, and the exported patch remains owned by the latest
    source plan.
  - The predictions JSONL validates with a non-empty patch and official harness
    dry-run/import succeeds under `eval/results/swebench/.venv/bin/python`.
  - Remaining gap: local confidence is still downgraded by
    `verification_probe_missing_required_contract_ref`. Next follow-up should
    make proof-follow-up probe enrichment more reliable so a passing targeted
    probe can satisfy the required behavior contract without extra user action.

RC117 multi-instance smoke and manual audit:

- Run:
  - `eval/results/swebench/lite-smoke-20260619-rc117-multi-flask-pytest-sphinx`
    on `pallets__flask-4045`, `pytest-dev__pytest-11143`, and
    `sphinx-doc__sphinx-8801`.
  - `validate_predictions.py --require-nonempty-patch` validates all 3
    predictions, and official harness dry-run/import succeeds under
    `eval/results/swebench/.venv/bin/python`.
- Manual audit:
  - Flask patch adds a dot-name guard in `src/flask/blueprints.py`, which is
    consistent with the issue. Local verify failed because importing Flask hit
    `cannot import name 'url_quote' from werkzeug.urls`, a dependency
    compatibility problem rather than evidence that the source patch is wrong.
  - Pytest patch guards `is_rewrite_disabled()` with `isinstance(docstring,
    str)`, which is consistent with the issue where a numeric first expression
    is mistaken for a docstring. Local proof failed because the generated
    verification probe imported a non-existent `rewrite` module, and patch
    review downgraded coverage.
  - Sphinx patch tries to attach `ModuleAnalyzer.attr_docs` when creating
    annotation-only members, but local probe still reports
    `attr1.docstring should not be None`. This is a real implementation/search
    failure, not just local environment noise.
- Follow-up gaps:
  - Dependency compatibility import errors from local verify/probes need a
    shared typed unavailable path when they are outside the changed source
    surface, instead of becoming `tests_failed` and blocking otherwise coherent
    patches.
  - Verification probe import paths need better typed grounding from changed
    source modules; bad probe scaffolding should not be confused with product
    failure.
  - Sphinx shows the remaining exploration/localization gap: after a failed
    behavioral probe, replan should be able to trigger a deeper read-mode style
    exploration of the implementation semantics instead of repeatedly planning
    from stale shallow context.

RC118 probe import diagnostics and carried handoff:

- Gap:
  - RC117 showed two different unavailable-verification shapes that were hard
    to audit after the fact: a Flask project import compatibility error
    (`url_quote` missing from `werkzeug.urls`) and a Pytest generated probe
    importing an invalid short module (`rewrite`). The existing report retained
    only coarse `verification_probe_module_not_found` /
    `verification_probe_import_error` reason codes in several aggregate paths.
  - `run_tests` could run a pre-suite verification probe, mark it unavailable,
    continue into project runners, and then finalize a project-runner report
    that kept command rows but lost the richer probe-level
    `VerificationDiagnostics`. That is a handoff information-loss bug, not a
    planner prompt problem.
- Design:
  - Keep controller authority unchanged: pass/fail/unverified still comes from
    typed `FailureKind`, `VerificationStatus`, `FailureReasonCode`, and command
    outcomes. No user-intent keyword or model-prose routing is introduced.
  - Add a Python verification-probe import diagnostic projector that parses
    interpreter exception type plus traceback import boundary into compact JSON
    detail:
    `missing_module`, or `import_name` + `source_module`.
  - Carry probe-level `VerificationDiagnostics` through the whole `run_tests`
    execution context so fallback/project-suite reports preserve prior probe
    evidence into context packs and verify-failure handoff.
- Delivered:
  - `runPlanVerificationProbes` now aggregates child probe diagnostics.
  - `RunTests.Execute.finishReport` now merges carried diagnostics before
    command-derived diagnostics, preserving richer detail under the existing
    diagnostic key.
  - Added focused regressions for missing-module and `cannot import name from`
    import-boundary payloads.
- Verification:
  - `go test ./internal/tool -run 'TestRunTestsVerificationProbeImportErrorIsParserError|TestPythonVerificationProbeImportDiagnosticDetailCapturesImportName|TestVerificationDiagnosticsPreserveProbeAndSuiteSignals' -count=1`
  - `go test ./internal/tool ./internal/types ./internal/writeflow -count=1`

RC118 SWE smoke and manual audit:

- Run:
  - `eval/results/swebench/lite-smoke-20260619-rc118-probe-import-diagnostics-flask-pytest`
    on `pallets__flask-4045` and `pytest-dev__pytest-11143`.
  - `validate_predictions.py` validated two predictions with one empty patch;
    official SWE-bench harness dry-run/import succeeded.
- Evidence:
  - Flask report now preserves probe import detail as typed JSON:
    `{"exception":"ImportError","import_name":"url_quote","source_module":"werkzeug.urls"}`.
    This proves the RC118 diagnostic carrier survives into the final
    `ChangeReport.VerificationDiagnostics`.
  - Pytest report now preserves package import detail as typed JSON:
    `{"exception":"ModuleNotFoundError","missing_module":"_pytest._version"}`.
    The prior audit's coarse "bad rewrite import" impression is now narrower:
    the probe imported `_pytest.assertion.rewrite`, but the checkout/runtime
    import path lacked `_pytest._version`.
- Manual audit:
  - Pytest exported a coherent source patch for
    `src/_pytest/assertion/rewrite.py`, changing `is_rewrite_disabled()` to
    guard non-string docstring values. This is likely functionally aligned with
    the issue, but local confidence remains blocked by uncovered changed-symbol
    proof and unavailable probe startup.
  - Flask ended `empty_patch` with `write_wall_time_empty_patch` /
    `plan_batch_canceled`. The source checkout at the instance base did not
    already contain the required guard at `Blueprint.__init__`; a later planner
    no-change narrative incorrectly claimed it was already present. This is not
    a timeout-only tuning issue: it is a no-change authority gap.
- Follow-up gap:
  - **RC119: no-change plan authority.** A no-change/empty-change plan must be
    accepted only when deterministic current-source evidence proves the target
    behavior or source state already exists. The controller/planner cannot rely
    on model summaries such as "already implemented." Required inputs are typed
    current bytes, owner anchors, previous applied patch state, and optional
    passed probes. If no-change is unproven in a write request, the controller
    should re-open localization/read evidence or block with a typed reason
    instead of exporting an empty patch.

RC119 wall-time default validation:

- Change:
  - Runtime default `pipeline_write_max_seconds` was raised from 600 to 900
    seconds (15 minutes), while preserving the existing configurable
    `codrax.yaml` key and the 1800-second hard cap.
  - `codrax.yaml.example` now documents `pipeline_write_max_seconds: 900`.
- Regression:
  - Re-ran `pallets__flask-4045` as
    `eval/results/swebench/lite-smoke-20260619-rc119-walltime900-flask`.
  - The same instance that previously ended `empty_patch` at 600 seconds now
    completed in 480 seconds with a 483-byte source patch:
    `src/flask/blueprints.py` raises `ValueError` when `Blueprint.__init__`
    receives a name containing `.`.
  - `validate_predictions.py --require-nonempty-patch` passed, and official
    SWE-bench harness dry-run/import succeeded.
- Manual audit:
  - The patch is functionally aligned with the issue "Raise error when
    blueprint name contains a dot."
  - Local confidence remains downgraded because verification is unavailable
    (`pytest_import_startup_error`) and patch-review coverage is still
    uncovered (`changed_symbol_without_probe_coverage`). This is a proof
    confidence gap, not an export/harness-consumption gap.
- Updated conclusion:
  - The RC118 Flask empty patch was primarily a wall-time convergence-window
    failure. RC119 no-change authority remains worth keeping in the queue, but
    it is no longer the primary explanation for this specific Flask empty patch.

## 2026-06-19 RC121 Proof Coverage And Local Verification Confidence

Status: implemented and smoke-validated; remaining Sphinx empty-patch evidence
belongs to the upstream localization / plan-materialization queue.

Trigger evidence:

- `django__django-14534` in
  `eval/results/swebench/lite-smoke-20260619-rc120-django-sympy-sphinx`
  exported a plausible non-empty source patch, but local verification was
  unavailable due `unittest_loader_import_error`. The SWE adapter still marked
  the prediction as `predicted_audit_blocked` because
  `changed_symbol_without_probe_coverage` was treated as a local hard blocker.
- `sphinx-doc__sphinx-8801` produced a non-empty source patch and then entered
  `verify_retry_budget_exhausted`. The typed report showed many pytest
  `ERROR` rows caused by import/startup dependency gaps such as missing
  `roman` / `docutils`, with `0 failed` assertions. That is local verifier
  unavailability, not product-code proof of incorrect functionality.
- `sympy__sympy-23117` still ended `empty_patch`; this remains a convergence
  and no-change authority issue, not a proof-confidence classification issue.

System gap:

- Local acceptance had one boolean-like bucket for two different concepts:
  actual-diff hard risk and proof coverage confidence.
- `PatchReviewRecord` findings such as
  `changed_symbol_without_probe_coverage` and
  `behavior_contract_without_verify_coverage` are proof obligations. They
  should lower confidence and schedule bounded proof follow-ups, but they are
  not themselves evidence that the patch is functionally wrong.
- Pytest JSON reports that contain only collection/import/startup errors and
  zero failed assertions were being interpreted as failed tests. This could
  drive replan loops against source code when the local environment was the
  unavailable component.
- Unittest reports could also mix real passed cases with `_FailedTest`
  collection/import errors from missing optional dependencies. Those mixed
  reports are partial local verifier unavailability, not authoritative product
  assertion failures, as long as every non-passed case is a loader import
  error.

Architecture rule:

- Hard blockers remain limited to typed actual-diff/effect risks, structural
  patch-review errors, build failures, and authoritative failed assertions.
- Proof coverage gaps become typed confidence telemetry:
  `prediction_confidence_downgrade_reason`,
  `plan_patch_review_semantic_unverified_telemetry_codes`, and
  `uncovered_impact_kind_telemetry`.
- Verification environment/startup failures become `parser_error` /
  `verification_status=unavailable` with stable reason codes. They can make a
  prediction unverified, but must not be treated as failed product tests.
- Mixed unittest reports with passed cases plus only `_FailedTest` loader
  import errors also become `unavailable`; real `FAIL` blocks or non-loader
  `ERROR` blocks remain failed.
- This keeps the official SWE prediction export harness-consumable while making
  local dashboards honest about the difference between "functionally proven",
  "unverified", and "blocked by typed evidence."

Tasks:

- [x] Run RC120 SWE smoke across Django/Sphinx/SymPy and record the typed
  evidence split between export status, local verify status, and proof profile.
- [x] Narrow SWE adapter `PATCH_REVIEW_LOCAL_BLOCKER_CODES` to actual
  effect/diff blockers only.
- [x] Preserve proof-only coverage findings as confidence telemetry instead of
  local hard blockers.
- [x] Classify pytest JSON reports with `0 failed` assertions and only
  import/collection errors as `parser_error` / `unavailable`.
- [x] Classify mixed unittest reports with passed cases plus only
  `_FailedTest` import loader errors as `parser_error` / `unavailable`.
- [x] Add focused tests for proof-confidence telemetry and pytest error-only
  startup reports.
- [x] Re-run a small SWE smoke after the fix to confirm prediction export still
  works and local confidence fields no longer conflate proof gaps with hard
  blockers.
- [ ] Continue the upstream convergence queue for no-change authority and
  proof-follow-up terminal semantics, owner localization, and plan
  materialization fallback.

Acceptance additions:

- A patch with only proof coverage gaps may be `predicted_unverified` or
  `predicted_passed_low_confidence`, but not `predicted_audit_blocked` solely
  because of `changed_symbol_without_probe_coverage`.
- Actual-diff/effect blockers such as
  `python_nested_string_key_direct_access_added` still block local acceptance.
- Pytest import/startup-only JSON reports normalize to `unavailable`, so the
  controller and eval adapter do not replan source code from missing local
  dependencies.
- Unittest mixed loader import errors normalize to `unavailable` unless there
  is a real non-loader failed assertion or non-loader error.

RC121 validation:

- Focused regression:
  - `go test ./internal/tool -run 'TestParseUnittestOutput_(LoaderOnlyIsParserError|MixedPassedAndLoaderErrorsIsParserError|RealTestFailureStaysUnclassified)|TestParsePytestJSONReport_ErrorOnlyImportStartupIsUnavailable' -count=1`
    passed.
  - `go test ./internal/tool ./internal/types ./internal/orchestrator -count=1`
    passed.
  - `python3 -m unittest eval/swebench/run_codrax_swebench_test.py eval/swebench/summarize_codrax_results_test.py`
    passed.
- SWE smoke `eval/results/swebench/lite-smoke-20260619-rc121-proof-confidence`
  exported non-empty predictions for `django__django-14534` and
  `sphinx-doc__sphinx-8801`; `validate_predictions.py --require-nonempty-patch`
  accepted both predictions.
- `django__django-14534` now reports
  `prediction_verdict=predicted_unverified`,
  `prediction_audit_block_reason=""`, `verify_status=unavailable`, and
  `patch_bytes=420`. The patch is the expected bounded source fix:
  `BoundWidget.id_for_label` returns `self.data['attrs'].get('id', '')`.
  Proof gaps remain visible in residual risk / proof status instead of blocking
  local acceptance.
- The first Sphinx run in the same smoke confirmed pytest import/startup-only
  reports now normalize to `verification_status=unavailable` with
  `pytest_import_startup_error`. A later proof-repair attempt still used the
  previous binary and classified mixed unittest loader errors as failed,
  motivating the mixed unittest parser fix in this batch.
- Follow-up Sphinx smoke
  `eval/results/swebench/lite-smoke-20260619-rc121-sphinx-mixed-loader`
  ran with the rebuilt binary but ended `empty_patch` /
  `workflow_blocked_no_plan` before verification. Logs show the model shifted
  to `sphinx/ext/autodoc/importer.py` and repeatedly attempted non-applicable
  edits such as `insert_before_final_brace` for a Python file. That is not a
  local verification-confidence bug; it is an upstream localization and
  plan-materialization affordance gap to keep in the RC111 / historical
  owner-anchor queue.

## 2026-06-19 Historical RC-103+ Follow-up Queue

This queue came from the pre-RC104 three-instance smoke. It is not an official
SWE score. The current priority order is superseded by RC-105 / RC-106 /
RC-111 above, but the historical evidence remains useful context. That earlier
typed triage run produced harness-consumable predictions validated by
`validate_predictions.py` and official harness dry-run/import. Manual audit
classified these systemic gaps:

1. **Shared localization owner/evidence pre-plan authority.**
   This is the remaining upstream gap behind many wrong-source-surface patches:
   read mode can observe many useful files, and write mode can carry context
   packs, but the two modes still need one stronger typed owner/evidence anchor
   contract that distinguishes line-backed owner localization from broad
   evidence/supporting observations. The next batch should audit current
   `SourceLocalizationAnchor` production/consumption in read exploration,
   extraction/finalization handoff, write context packs, planner localization
   gates, and replan repair slices; then make controller/planner consume owner
   anchors before broader target-file hints. Hard routing must use only typed
   anchor fields such as owner kind, path, line span, consumer, priority,
   source stage, and evidence refs. RC-104 preserved matched prior
   owner/supporting/scope anchors across the plan review boundary; RC-107 is
   complete, and RC-111 is now the queued pre-plan authority closure for the
   remaining upstream owner-anchor gap.
2. **Planner repair tool affordance mismatch.**
   `django__django-14534` entered online restore/replan several times. During
   repair planning, the model attempted an unavailable `grep` call. Restoring
   broad exec access would violate the planning-lane safety model; the right fix
   is a typed search/repomap/read affordance and prompt/hint unification for
   repair lanes, so the planner can localize failure evidence without ordinary
   command execution.
3. **Probe generation repair layer.**
   `pytest-dev__pytest-5227` and `sympy__sympy-18199` both ended unverified due
   typed probe unavailability/syntax/top-level exceptions. These should feed a
   shared probe-normalization/repair layer before the verifier treats them as
   final unavailable evidence. The layer must consume structured probe source,
   language/runtime, parser diagnostics, and import boundary records, not stdout
   prose.
4. **Active-run delivery UX.**
   `django__django-14534` exported a non-empty failed-verify prediction while
   workflow status remained `in_progress`. That is acceptable for adapter
   compatibility but poor for routine users. The CLI/REPL should surface a
   single typed next-action card and auto-resume safe active runs instead of
   requiring users to discover `/workflow` commands.

## Acceptance Criteria

- SWE local acceptance no longer counts a patch as pass when typed
  `PatchReviewRecord` says the actual diff has a hard error or unverified
  semantic coverage.
- SWE local acceptance no longer counts low-confidence local verifier passes as
  authoritative local verification; they require explicit typed manual pass or
  remain unknown.
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
  non-Python projects, including Java, without requiring project-wide test
  runners or shell command probes.
- No-test Go source changes compile automatically through the standard Go
  toolchain before being marked unavailable/pass-with-warning.
- No-test TypeScript source changes compile automatically through repo-local or
  PATH `tsc --noEmit --pretty false` when available.
- Uncovered typed impact obligations become a bounded repair queue before
  finish, instead of remaining passive telemetry after local verifier success.
- Impact related-test obligations choose the smallest supported runner-native
  selector across Python, Go, Node, Ruby, Java/Kotlin, Rust integration tests,
  and Swift Package tests before falling back to broad TestSurface execution.
- Java/Kotlin and Swift no-test source changes compile when the local toolchain
  can provide attributable diagnostics; missing customer toolchains remain
  warning evidence rather than hard blockers.
- Current read/log/trace/data/operation/computer paths remain untouched.
- All new eval fields and docs distinguish export compatibility from
  functional correctness.
