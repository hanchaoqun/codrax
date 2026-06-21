# Codrax Eval-Driven Commercial Hardening Ledger

Date: 2026-06-21  
Branch: `main`

## Goal

Run representative eval cases in small controlled batches, manually audit the
outputs and logs, record architecture-level gaps before code changes, and then
close those gaps with generalized typed mechanisms.

This ledger is the task source for this campaign. New gaps, plan changes, and
follow-up tasks discovered while running evals must be written here before
implementation continues.

## Non-Negotiable Constraints

- No hard gate may parse user prose, model rationale, prompt text, visible
  thinking, rendered answer prose, ranker scores, grep hit counts, or elapsed
  time.
- Hard gates may consume only typed artifacts, schema-validated enums, precise
  booleans/integers, parser results, normalized paths, structured tool results,
  and deterministic policy decisions.
- Fixes must be per-class and system-level, not per-shape guard patches or
  single-case prompt patches.
- Read mode, write mode, trace/log/data/operation lanes, and computer-operation
  surfaces must remain isolated where their stability contracts differ.
- Handoff must preserve prioritized typed evidence, observations, repair
  descriptors, source-localization anchors, and proof state for downstream
  consumers without rendering large raw tool summaries into prompts.
- Tool JSON/schema repair must flow through the unified typed repair layer and
  schema descriptors, not through model prose interpretation.

## Eval Batch Policy

- Batch size: 6 representative cases.
- Concurrency: 2 cases in parallel.
- Default wall time: use the eval runner timeout; if a case times out, capture
  the result directory and classify the typed subsystem that consumed the
  budget.
- Every case audit records:
  - verdict from harness,
  - manual correctness assessment,
  - tool usage, especially `repo_map` and `trace_query`,
  - iteration/tool-call counts,
  - performance and memory symptoms visible in logs,
  - noise symptoms such as repeated low-delta repair, duplicate evidence, or
    stage-inappropriate tool suggestions,
  - handoff fidelity from exploration to extraction/final report,
  - proof/verification confidence.
- Audit searches must exclude bulky result environments and migration archives
  unless the target is an explicit result directory. The first exploration of
  this campaign confirmed that unconstrained repository-wide grep over
  `eval/results/**` produces severe noise and performance pollution.

## Representative Case Order

| Slot | Case | Lane | Why it is representative |
| --- | --- | --- | --- |
| 1 | `eval/cases/qf_relation_subagent_registry.case` | read | Relation/member-set correctness, handoff preservation, answer evidence shrinkage, and low-delta convergence. |
| 2 | `eval/cases/harmony/arkts_repomap.case` | read | Cross-language source inventory, auxiliary/corpus source classes, `repo_map` adoption, and false-absence protection. |
| 3 | `eval/cases/trace_query_wakeup_causal_io_chain.case` | trace/read | Attached trace routing, `trace_query` use, causal path projection, and source-tool noise avoidance. |
| 4 | `eval/cases/sr_ts_workspace_impls.case` | read | JS/TS workspace relation navigation, implementer/member-set proof, helper exclusion, and repo-map efficiency. |
| 5 | `eval/cases/github_issue_fmt_tm_year_overflow_symptom.case` | write | Symptom-driven C++ localization and online repair rather than direct patch instruction following. |
| 6 | `eval/cases/patch_cpp_typo.case` | write | Low-risk C++ plan path, bounded patch generation, emit-change-plan schema stability, and non-Python coverage. |

If a case cannot launch because of provider or environment failure, record the
failure here as an eval-infrastructure gap and substitute the next case from
the same lane family only after documenting the substitution.

## Manual Audit Rubric

| Dimension | Pass signal | Gap signal |
| --- | --- | --- |
| Correctness | Final answer or patch addresses the actual task, not just harness substrings. | Patch/answer satisfies a weak string check but misses the functional issue, or uses the wrong source surface. |
| Tool choice | `repo_map`, `trace_query`, source inventory, and typed verify tools are used when their lane owns the navigation/proof problem. | Model avoids efficient typed tools, calls unavailable tools in a stage, or falls back to broad reads/grep for typed navigation. |
| Noise | Evidence and repair loops stop after typed obligations are satisfied. | Repeated "verification not stable" loops, duplicate supplements, repeated schema repair, or low-delta exploration. |
| Performance | Round count, wall time, and context volume are proportional to task complexity. | High iteration counts, high CPU/memory, root-scope source inventory scans, or long context assembly. |
| Handoff | Extractor/finalizer/report consumers receive prioritized typed facts and refs. | Rich exploration facts disappear, or raw bulky summaries crowd out principal evidence. |
| Proof | Passed/failed/unverified/unavailable states are explicit and correctly influence next action. | Missing dependency or unavailable verifier is treated as a source failure, or narrow proof is overstated. |
| Red lines | Hard logic consumes typed fields only. | Any new hard decision depends on user keywords, model prose, prompt hints, or natural-language summaries. |

## Current Known Open Gaps To Recheck

| ID | Priority | Gap | Current owner / expected direction |
| --- | --- | --- | --- |
| EDH-1 | P0 | Broad eval/log auditing can accidentally scan huge historical result trees and dependency environments. | Add bounded audit tooling or documented search presets that default-exclude `eval/results/**`, virtualenvs, and migration archives while still allowing explicit result-dir audit. |
| EDH-2 | P0 | Read-loop noise still has open relevance/repair-debt owners: support chains, low-delta retries, and support-only range repair demotion. | Continue from `read_mode_noise_convergence_eval_gap_20260620.md` RNE-C1/C4/C6 with typed relevance budgets and progress deltas. |
| EDH-3 | P0 | Completion preflight and context assembly remain slow in large ledgers. | Continue RNE-C12 with cached `CompletionPreflightView`, timing telemetry, and status-card consumption. |
| EDH-4 | P0 | Contract severity still mixes blocking, repaired, and audit-only states. | Continue RNE-C15 with typed severity split so successful answers are not polluted by repaired warnings. |
| EDH-5 | P0 | Cross-language member/facet projection must remain language-neutral and include C/C++, Cangjie, ArkTS, JS/TS, Java/Kotlin, Ruby, Go, config/workflow, generated/vendor/third-party/corpus surfaces. | Continue SourceInventoryMemberProjection work from RNE-C58; avoid Python-only or Go-only assumptions. |
| EDH-6 | P0 | Write-mode symptom-driven localization and proof confidence remain the main SWE/eval correctness risk. | Use write evals and SWE artifacts to decide whether the bug is localization, impact analysis, patch critic, verifier scope, or handoff loss before implementing. |
| EDH-7 | P0 | Verify-failure replan can produce the correct small patch but still block on stale or under-integrated source-owner localization state. | Make write apply gating consume the latest batch plan, actual-diff owner anchors, failed-verification observation, and approval state as one typed authority snapshot. Do not block a replan whose changed lines have structural owner anchors. |
| EDH-8 | P0 | Micro write tasks can duplicate planning after a valid plan because path-only localization is treated as unresolved even when the patch itself is a bounded single-owner edit. | Add a typed micro-plan acceptance path based on normalized paths, actual diff hunks, owner anchors, and risk policy. This must be language-neutral and not infer intent from request text. |
| EDH-9 | P0 | Read final answers can show repaired/audit-only contract warnings and system supplements after exact evidence already satisfies the answer. | Split final-answer contract states into blocking, repaired, audit-only, and displayable states. Routine exact answers must not show low-confidence caveats created by already-repaired obligations. |
| EDH-10 | P0 | `repo_map(source_inventory)` can under-project members for ArkTS/decorator/facet-heavy files even when `read_file` evidence has exact symbols. | Extend the SourceInventoryMemberProjection authority so source inventory and direct read evidence share a typed member/facet projection. Keep it cross-language and class-aware. |
| EDH-11 | P1 | Trace/read final answers can over-render raw structured observations and duplicate metric sections, while citation accounting may report zero citations for artifact-backed facts. | Add a trace observation projection for final-answer consumption: principal causal facts first, supporting artifact refs second, bulky diagnostics behind audit/detail surfaces. |
| EDH-12 | P1 | Eval harness result discovery is brittle around discarded write worktrees and concurrent run summaries. | Make eval reporting consume typed run artifacts when available and explicitly distinguish product failure, eval-infrastructure failure, missing worktree, and missing final report. |

## Batch E1 Results

Result root: `eval/results/eval-driven-20260621-batch1b`

| Case | Harness verdict | Manual verdict | Key observations |
| --- | --- | --- | --- |
| `qf_relation_subagent_registry.case` | PASS | Correct answer: only `explorer` can call subagents through the registered tool path. | 150s, about 75k context tokens, 12 explorer iterations, and repaired contract warnings still appeared in the final answer. This is a noise/severity gap, not an answer correctness gap. |
| `harmony/arkts_repomap.case` | PASS | Correct symbols: `Index`, `defaultHeader`, `GlobalCard`. | Analyzer formed an early false absence prior, exploration recovered with `list_files`/`read_file`, and final answer still showed range/support caveats. `repo_map(source_inventory)` under-projected member rows. |
| `trace_query_wakeup_causal_io_chain.case` | PASS | Correct causal chain from attached trace evidence. | Good `trace_query` adoption. Final surface over-rendered structured observations and duplicated metric-style supplements; citation accounting reported zero citations at one point despite artifact-backed facts. |
| `sr_ts_workspace_impls.case` | PASS | Correct implementers: `ExponentialBackoff` and `FixedDelay`; `JitterHelper` correctly excluded. | Late completion pressure forced `repo_map(source_inventory)` after exact direct evidence existed. Small fixture still took 90s, showing navigation/readiness noise. |
| `github_issue_fmt_tm_year_overflow_symptom.case` | FAIL | The system found the correct missing change after verify failure, but blocked before applying it. | First patch was incomplete. Verify failure identified `calendar_year` overflow. Replan produced the correct `int` to `long long` patch, then apply blocked on source-owner localization. This is EDH-7. |
| `patch_cpp_typo.case` | PASS | Correct low-risk C++ patch. | Valid plan was duplicated because path-only localization was considered insufficient, and `emit_change_plan` repair repeated around whitespace-sensitive old text. This is EDH-8 plus schema-repair cost. |

## Manual Audit Intake

The batch shows the strongest current commercial risk is no longer "cannot find
any patch". It is state and evidence authority integration after the system has
already found useful evidence:

- Write mode can identify the correct failure point from verifier output but
  then let stale localization or approval state block the latest small repair.
- Read mode can gather enough precise evidence but still spend extra rounds
  satisfying secondary navigation or contract obligations and then display those
  repaired obligations as user-facing uncertainty.
- Source inventory is valuable and should be encouraged, but its member/facet
  projection must not become a weaker truth source than direct read evidence.
- Trace queries are being used appropriately, but final-answer projection needs
  a compact principal-evidence view instead of dumping raw observation detail.
- Eval infrastructure should classify missing/discarded worktree artifacts as
  reportability gaps without hiding the underlying product gap.

## Delivery Plan

### Batch E0: Ledger And Harness Discipline

Deliverables:
- Land this ledger.
- Push to `main`.
- Confirm current branch is clean before launching evals.

Validation:
- `git status --short --branch`

### Batch E1: Run Representative Eval Batch

Deliverables:
- Build a stable Codrax binary or use the runner's rebuild path.
- Run the six selected cases with two-way parallelism.
- Store summary, result directories, and raw logs under `eval/results`.
- Do not modify product code during the run.

Validation:
- Each case has `run-1.out`, `run-1.metrics.txt`, `run-1.verdict`, and logs.

### Batch E2: Manual Audit And Gap Intake

Deliverables:
- Read each final output and latest debug log.
- Update this ledger with a result table and new gap IDs before coding.
- Classify failures by architecture class: localization, source inventory,
  proof coverage, patch critic, impact analysis, verifier, handoff, schema
  repair, status-card/UX, performance, or eval infrastructure.

Validation:
- No gap remains only in chat or temporary notes.

### Batch E3+: Generalized Fix Batches

Deliverables:
- For each gap class, explore the current code before editing.
- Add precise tasks and tests to this ledger.
- Implement one generalized batch at a time.
- Run focused tests and relevant eval reruns.
- Commit and push after each batch.

Validation:
- Focused tests pass.
- `go test ./...` passes when the batch touches shared infrastructure.
- Representative reruns show reduced gap symptoms without regressions.

### Batch E3: Write Replan Authority And State Convergence

Gap IDs: EDH-7, EDH-8.

Tasks:
- Explore the write controller state machine, owner-localization gate,
  approval/fingerprint handling, and actual-diff anchor projection before
  editing.
- Build a single typed pre-apply authority view for the active batch that folds
  latest plan, actual diff hunks, structural owner anchors, verifier failure
  observations, approval state, and risk decision.
- Allow bounded replan application when every edited hunk has a normalized path
  and structural owner anchor, even if earlier plan-context localization was
  path-only or stale.
- Reject contradictory states deterministically: a batch cannot be both
  `needs_replan` and applying an older plan, and a replaced plan cannot carry
  forward approval for a changed fingerprint.
- Add focused tests for verify-failure replan, micro single-line C/C++ patch,
  stale owner-localization requirement, and approval fingerprint mismatch.

Validation:
- Focused `go test` for write controller scheduler.
- Rerun `github_issue_fmt_tm_year_overflow_symptom.case` and
  `patch_cpp_typo.case`.

### Batch E4: Read Contract Severity And Noise Convergence

Gap IDs: EDH-2, EDH-4, EDH-9.

Tasks:
- Explore answer contract, retry guidance, completion preflight, and final
  rendering paths before editing.
- Promote contract severity into typed states: blocking, repaired,
  audit-only, and displayable.
- Stop rendering repaired/audit-only warnings into routine exact answers.
- Add progress-delta checks so repeated completion retries require a new typed
  obligation, not repeated low-delta "verification not stable" loops.
- Add tests with exact relation/member answers and repaired support-chain
  obligations.

Validation:
- Focused read-mode contract tests.
- Rerun `qf_relation_subagent_registry.case` and
  `sr_ts_workspace_impls.case`.

### Batch E5: Cross-Language Source Inventory Projection

Gap IDs: EDH-5, EDH-10.

Tasks:
- Explore existing `SourcePathRole`, `SourceScope`,
  `SourceInventoryObservation`, direct read symbol extraction, and language
  parsers before editing.
- Make source inventory and read-file symbol evidence share a typed member/facet
  projection authority.
- Ensure the projection is class-aware across repo-owned, generated, vendor,
  third-party, corpus, fixture, example, and test surfaces.
- Cover C/C++, Cangjie, ArkTS, JS/TS, Java/Kotlin, Ruby, Go,
  config/workflow, and parser-fallback paths without language-specific hard
  gates on request prose.

Validation:
- Source inventory focused tests across representative languages.
- Rerun `harmony/arkts_repomap.case`.

### Batch E6: Trace Projection And Citation Surface

Gap IDs: EDH-11.

Tasks:
- Explore trace artifact projection, answer document emission, and citation
  accounting before editing.
- Add a compact typed trace-observation final-answer view with principal causal
  claims, supporting spans, and optional audit detail.
- Normalize artifact-backed facts so citation accounting reflects structured
  trace evidence without forcing raw metric dumps into the principal answer.

Validation:
- Focused trace answer tests.
- Rerun `trace_query_wakeup_causal_io_chain.case`.

### Batch E7: Eval Reporting Hardening

Gap IDs: EDH-1, EDH-12.

Tasks:
- Explore eval runner result discovery, write worktree cleanup, final report
  emission, and summary generation before editing.
- Add typed product/eval-infra verdict categories and robust artifact lookup.
- Keep audit search defaults bounded and explicit-result-dir aware.

Validation:
- Rerun the same six-case batch with two-way parallelism.
- Confirm summary rows and product gaps remain inspectable after worktree
  cleanup.

## Progress Ledger

| Batch | Status | Notes |
| --- | --- | --- |
| E0 | done | Ledger created and pushed in commit `b45f02dc7`. |
| E1 | done | Six representative cases ran under `eval/results/eval-driven-20260621-batch1b` with two-way parallelism. |
| E2 | done | Manual audit completed and gap IDs EDH-7 through EDH-12 added before code changes. |
| E3 | in_progress | Start with write replan authority/state convergence because it caused the only batch harness failure. |
| E4 | pending | Read contract severity and noise convergence. |
| E5 | pending | Cross-language source inventory projection. |
| E6 | pending | Trace projection and citation surface. |
| E7 | pending | Eval reporting hardening and six-case rerun. |
