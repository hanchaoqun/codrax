# Gap Architecture Retriage

Date: 2026-05-23

Status: active implementation tracker. Local-small-model compatibility gaps are
explicitly out of scope for this pass unless they expose a shared production
contract defect.

## Goal

Re-audit the current gap ledgers and recent eval logs, then decide whether the
remaining issues are isolated defects or architecture gaps. This document is the
handoff for the next batches, so implementation does not depend on chat memory.

The non-negotiable boundary is unchanged:

- user intent and model-authored useful answer content are primary;
- system repairs are append-only, localized, and typed;
- hard gates consume precise structured signals only;
- grounded in-scope evidence and its rich summaries must outrank closure prose,
  stale aggregate counts, and generic support hints;
- external observations such as git, logs, traces, commands, cross-repo index,
  external documents, web, MCP, and connectors are first-class evidence through
  origin-specific refs, not fake current-source `file:line` citations.

## Inputs Reviewed

- `docs/design/eval_20260520_full_sweep_gap_tracking.md`
- `docs/design/eval_20260522_full_sweep_gap_tracking.md`
- `docs/design/unified_evidence_answer_contract_20260520.md`
- `docs/design/observation_ledger_contract_20260521.md`
- `docs/design/principal_ledger_prompt_convergence_20260523.md`
- `docs/design/json_payload_cognitive_load_gap_20260523.md`
- `docs/design/answer_surface_safety_batch_20260523.md`
- Targeted replay logs under:
  - `eval/results/principal-ledger-20260523-211258`
  - `eval/results/principal-ledger-u7l-fix-20260523-213328`
  - `eval/results/principal-ledger-short-hash-20260523-215117`
  - `eval/results/u7l-20260523-223616`

## Current Code Audit

The repo already has the right primitives. The next batches should reuse these
instead of introducing another evidence stack.

| Contract | Current code | Audit result |
| --- | --- | --- |
| Unified observation records | `internal/types/observation_ledger.go` | Good base: has current source, VCS, diff, command, runtime artifact, cross-repo index, external document, web, MCP, connector origins, source refs, spans, provenance lanes, row-set refs. |
| Shared prompt projection | `internal/types/observation_prompt_projection.go` | Good base: finalizer/reviewer share ranking, rich-note dedupe, origin-aware note budgets, and source/span formatting. |
| Context adapters | `internal/types/observation_ledger_context.go` | Good base: accepted Turn A artifacts outrank analyzer/pre-scan noise; evidence dedupe exists. |
| Finalizer observation prompt | `internal/agent/answer_document_evaluator.go` | Mostly converged on Observation Ledger, but some retry hints and schema surfaces still need continued audits for sentinel/internal wording. |
| Semantic reviewer observation prompt | `internal/orchestrator/semantic_quality_reviewer.go` | Shares the observation projection; remaining gaps are mostly reviewer-locus / advisory vs rewrite policy, not a separate ledger. |
| System supplement compilers | `internal/tool/answer_document_principal_enum_compile.go`, `internal/tool/answer_document_pre_emit_check.go` | Much safer after recent batches, but this remains a high-risk surface because deterministic supplements can still look principal if future code bypasses visible-surface coverage checks. |
| Origin helpers | `internal/types/answer_evidence_origin.go`, `internal/types/answer_claim_binding.go` | Shared helpers exist, but `observation_ledger_contract` still tracks scattered origin checks in pre-emit / contract / reviewer as an open cleanup item. |

## A/B Decision

Decision: **A — architecture remediation is still required**, but it should be
incremental and reuse existing contracts.

Reasoning:

1. The remaining high-ROI gaps are not single-case bugs. They are repeated
   boundary failures between typed evidence carriers, model-authored answer
   surfaces, deterministic supplements, reviewers, and retry/status telemetry.
2. Recent code has already closed many symptoms: schema-aware repair,
   origin-specific member sets, row-set refs, exact VCS changed paths,
   observation prompt projection, and visible supplement localization. The next
   work should consolidate these primitives rather than patch prompts.
3. Broad finalizer rewrites are the wrong default for the remaining class.
   Most residuals should be local typed repair, localized supplement, boundary
   disclosure, or telemetry-only advisory unless a precise structural defect
   would otherwise ship.

## Systemic Root Causes

### R1. External Observations Are Accepted, But Not Fully Future-Proofed

Git/log/trace/command are now mostly first-class through the observation
ledger. MCP has a support-only adapter. Web, external document, and connector
origins exist in the type system, but their future producer contract and eval
skeletons are not yet complete.

Risk if ignored: the next external source family may add a bespoke path and
reintroduce fake `file:line`, `citation_ref=-1`, or raw-prose-only support.

### R2. Gate Consumers Still Have Scattered Origin Decisions

The type layer exposes `ObservationRecordHasCurrentSourceLineSpan`,
`ObservationRecordHasStrongCurrentSourceAnchor`,
`AnswerClaimBindingHasExactCurrentSourceSupport`, and
`AnswerEvidenceOriginCarriesOriginSpecificSupport`. Some pre-emit, contract,
and reviewer branches still own local origin/routing decisions around current
source, runtime artifact, and no-source citation compatibility.

Risk if ignored: a future change may correctly add an external observation to
the ledger but still trip an old current-source citation gate or broad rewrite.

### R3. System Supplements Remain A Powerful Escape Hatch

Recent batches made supplements localized, append-only, and less dry. Still,
the compiler paths are inherently dangerous: they can improve completeness but
also make the final answer look worse than the model's own table or prose.

Risk if ignored: a new supplement path can publish a competing "complete system
table" or generic caveat even when the model-authored answer is sufficient.

### R4. Rich Evidence Ranking Is Mostly Centralized, But Eval Coverage Is Thin

`PrioritizeObservationRecords` preserves one best record per requested origin
and boosts strong current-source rows only when current source is requested.
This is the correct contract, but executable coverage is still stronger for
VCS/log/trace than for future MCP/web/connector and cross-repo-index facts.

Risk if ignored: a mixed request such as "based on MCP docs/log line/diff,
analyze current code" can lose one lane under prompt budgeting.

### R5. Retry/Review Telemetry Is Improving, But Advisory Work Can Still Look
Like Rewrite Pressure

Retry counters and combined reviewer notices are better. The open systemic
handoff gap G76 still says advisory-only contract checks can dominate accepted
runs and look like finalizer pressure.

Risk if ignored: customer UX still reads "the system is fighting itself" even
when the answer is accepted.

## Priority Order

1. **Batch A — external observation contract hardening.**
   Lock down future web/external-document/MCP/connector shape and source/span
   projection with code tests. This is low-risk and prevents future evidence
   families from inventing separate carriers.
2. **Batch B — ledger-based origin gate consolidation.**
   Audit and migrate the highest-risk pre-emit / reviewer / contract origin
   checks to shared helpers. Add tests proving external observations prefer
   local supplement/boundary disclosure over finalizer rewrite when a current
   source citation is not required.
3. **Batch C — system supplement safety fence.**
   Add a developer-facing regression suite that every deterministic supplement
   path must pass: preserve model-authored prose/tables, append only
   non-overlapping typed facts, never render all-empty or principal-looking
   tables, and localize system-authored labels.
4. **Batch D — eval expansion and log audit.**
   Add or refresh evals for:
   - git diff hunk + current code;
   - log line + current code;
   - trace window + current code;
   - MCP JSON field exists/absent skeleton;
   - web paragraph contains / absent skeleton;
   - connector row exists / absent skeleton;
   - cross-repo index fact + current source.
5. **Batch E — advisory-cost telemetry cleanup.**
   Separate expensive advisory-only checks from hard contract checks in
   telemetry and status. Do not change hard safety semantics.

## Batch Task List

| Batch | Status | Task | Code / Doc Areas | Validation |
| --- | --- | --- | --- | --- |
| T0 | Done | Create this retriage document with A/B decision, root causes, and task order. | `docs/design/gap_architecture_retriage_20260523.md` | Doc review |
| T1 | Done | Add code-level tests that aggregate facts for web, external docs, MCP, connectors, cross-repo index, VCS, command, log, and trace all project through `ObservationLedger` with origin-local `SourceRef` / `ObservationSpan`, not current-source citations. | `internal/types/observation_ledger.go`, `internal/types/observation_ledger_test.go`, `internal/types/observation_prompt_projection_test.go` | `go test ./internal/types` |
| T2 | Pending | Extend MCP response projection, if current fields are too weak, without duplicating the ledger. Prefer typed fields only when producers can populate them; otherwise keep support-only `RawRef`. | `internal/types/context.go`, `internal/types/observation_ledger.go`, MCP tests | `go test ./internal/types ./internal/mcp` |
| T3 | Pending | Audit pre-emit / contract / reviewer origin decisions and replace local origin switches with shared ledger helpers where safe. | `internal/tool/answer_document_pre_emit_check.go`, `internal/orchestrator/contract_check.go`, `internal/orchestrator/semantic_quality_reviewer.go` | focused unit tests plus no broad behavior drift |
| T4 | Pending | Add supplement safety guard tests across current-source, VCS, runtime, command, cross-repo, web/MCP/connector-like origins. | `internal/tool/*supplement*_test.go`, `internal/render/answerdoc_test.go` | no duplicate/dry supplement regression |
| T5 | Pending | Add executable eval cases for existing producers and placeholder/documented skeletons for future MCP/web/connector producers. | `eval/cases`, docs | targeted eval batch, no product-code change during log collection |
| T6 | Pending | Run targeted eval batch and refresh gap ledgers with every retry/reject, classifying model error vs system over-gate. | `eval/results`, `docs/design/eval_*.md` | per-run log audit |

## Implementation Notes For Future Batches

- Do not turn `emit_evidence` into a catch-all. It remains current checkout
  source/config/doc evidence only.
- Do not parse raw user/model prose for hard decisions. Visible-coverage checks
  may inspect the already rendered answer surface to decide whether a typed row
  is visibly covered, but not to infer user intent or evidence origin.
- A non-current observation can be principal answer evidence. It just cannot
  create current-source citation pressure.
- If a model table is good but mechanically incomplete, the system may add a
  clearly marked supplemental block. It must not rewrite or replace the table.
- If a non-critical, non-lossless issue remains, prefer localized boundary
  disclosure or accepted-with-advisory telemetry over finalizer rewrite.

## Progress Log

- 2026-05-23 T1 complete:
  - Added a ledger compiler regression covering external document, web page,
    MCP resource, connector resource, cross-repo index, VCS metadata, VCS diff,
    command output, log artifact, and trace artifact aggregate facts.
  - Verified every non-current origin keeps origin-local `SourceRef` /
    `ObservationSpan` details and does not become current-source
    citation-eligible.
  - Reused the existing Observation Ledger instead of introducing a new carrier.
  - Filled existing `ObservationSourceRef` passthrough gaps for `mime_type`,
    `fetched_at`, `tool_call_id`, and cross-repo `path`.
  - Validation: `go test ./internal/types`.
