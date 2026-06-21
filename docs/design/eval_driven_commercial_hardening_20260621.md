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

## Progress Ledger

| Batch | Status | Notes |
| --- | --- | --- |
| E0 | in_progress | Ledger created from current repo audit and existing eval harness. |
| E1 | pending | Six representative cases selected; run order fixed above. |
| E2 | pending | Manual audit not started. |
| E3+ | pending | Implementation batches must be added after E2 gap intake. |

