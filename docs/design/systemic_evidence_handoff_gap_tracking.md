# Systemic Evidence Handoff Gap Tracking

Status: active (2026-05-13)

This document tracks cross-case shortcomings found by recent REPL/eval audits.
It is intentionally a code-location ledger, not a new architecture. Each fix
must reuse the existing typed pipeline:

`emit_analysis` profile -> `AnswerSurfacePlan` / `AnswerSupportPlan` /
`StableAggregateFacts` -> `emit_answer_document` pre-checks ->
orchestrator contract checks.

The red line stays unchanged: hard gates read precise typed signals only.
Exploration may be rich, but downstream stages consume only model-emitted
structured data, not raw thinking text or system-invented answer content.

## Gap Ledger

| ID | Symptom | Code Location | Generalized Fix | Status |
| --- | --- | --- | --- | --- |
| G1 | Change-impact file answers can mix file-set and site-set surfaces, then ship stale file counts from closure prose. | `internal/types/answer_support_member_coverage.go`, `internal/tool/answer_document_pre_emit_check.go`, `internal/orchestrator/contract_check_block.go`, `internal/agent/answer_document_evaluator.go` | Treat `ChangeImpactProfile.requested_output=files` as a typed principal label-surface contract. File paths are principal labels; file:line anchors are support. Counts come from typed file obligations or `aggregate_facts`, not unstructured closure prose. | Implemented in this phase |
| G2 | Exploration facts exist in tool/thinking prose but are not reusable by finalizer. | `internal/tool/emit_investigation_complete.go`, `internal/agent/explorer.go`, `internal/agent/extractor.go`, `internal/types/answer_aggregate_fact.go` | Route derived totals, unique sets, bucket counts, and exhaustive member sets through `aggregate_facts`; route principal answer members through support lanes or `emit_answer_symbol`. Keep closure prose as audit context only. | Existing, keep auditing |
| G3 | Context helpers can be promoted into principal answer members for relation/architecture questions. | `internal/analysis/amplifier`, `internal/types/request_traits.go`, `internal/types/answer_support_plan_facet_evidence.go` | Use typed principal-vs-context boundaries: hard pins only for explicit bounded member lanes; relation targets and helper names stay search/context hints until emitted as support-lane principal evidence. | Existing, keep auditing |
| G4 | Source-to-sink call-chain answers can lose same-file intermediate evidence, causing finalizer to backfill with nearby helpers. | `internal/tool/emit_investigation_complete.go`, `internal/types/principal_span_waiver.go`, `internal/types/answer_support_plan_call_chain.go` | Require typed intermediate call/guard evidence before closure, or a model-declared `principal_span_waiver` enum when no user-code intermediate exists. | Existing, needs more eval coverage |
| G5 | Role/citation drift: an item can assert one role but cite an adjacent definition/guard. | `internal/types/evidence_claim_role.go`, `internal/tool/answer_document_pre_emit_check.go`, `internal/orchestrator/contract_check_block.go` | Centralize role projection by `ClaimForm` and evidence fields; pre/post validators align item surface role to the cited evidence role. Future route/macro/span roles extend the projection, not validators. | Existing, keep extending by claim form |
| G6 | Assignment/field evidence is still uneven across Go structs, C/C++ designated initializers, ArkTS object literals, Cangjie declarations, and config syntaxes. | `internal/tool/repomap`, `internal/types/claim_form.go`, `internal/types/answer_principal_member_surface.go`, `internal/agent/explorer.go` | Prefer tree-sitter/repomap surfaces to classify assignment/field/member evidence into language-neutral `ClaimForm` + `MemberSurface`. Fallback string evidence remains soft guidance, not a hard gate. | Design target |
| G7 | Aggregate count/member-set drift appears when the model emits only prose counts or partial aggregate facts. | `internal/types/answer_aggregate_fact.go`, `internal/tool/emit_investigation_complete.go`, `internal/agent/aggregate_fact_render.go` | Keep aggregate facts model-owned and structurally self-consistent: `len(members)==value`, file:line totals require `unique_count`, member-set value may be canonicalized only from emitted members. | Existing, keep enforcing |
| G8 | Diagrams and block shapes can reflect system scaffolding instead of the user's requested answer surface. | `internal/types/answer_semantic_view_*`, `internal/agent/answer_document_evaluator.go`, `internal/orchestrator/contract_check_block.go` | Compile answer family and allowed block kinds from typed profiles/facets; finalizer may enrich diagrams from grounded seeds, but principal blocks must stay aligned to the user-requested surface. | Existing, keep auditing |
| G9 | Owner-qualified change-impact targets can be polluted by unrelated fields with the same leaf name. | `internal/types/answer_support_plan_facet_evidence.go` | For `Owner.member` targets, principal evidence must structurally match the owner-qualified path or define the owner itself; leaf-only matches stay context. This covers Go selectors, C/C++ `::` / `->`, ArkTS/Cangjie member paths, and config-like dotted paths. | Implemented in this phase |

## Implementation Order

1. Land G1 because it is currently reproducible after `u10b` passes: a PASS can
   still contain file-count drift. This is a finalizer-local contract fix.
2. Re-run `u10b` and a random eval shard. Any new failures are assigned to the
   ledger before code changes.
3. Land G9 before broad random eval resumes; a PASS that includes an unrelated
   same-leaf field is a principal-set correctness failure, not a richness issue.
4. Expand G6 using repomap/tree-sitter surfaces rather than language-specific
   prompt patches. This must include Go, C/C++, Cangjie, ArkTS, and mixed
   repositories.
5. Continue sampling G2/G4/G5/G8 with random eval order. If an issue is already
   covered by an existing typed lane, strengthen the consumer/gate instead of
   adding another prompt-only rule.

## Non-Goals

- Do not parse raw thinking text to recover answer content.
- Do not let system code compute a user-facing answer unless the value comes
  from model-emitted structured fields or deterministic tool output explicitly
  supplied as answer evidence.
- Do not add keyword classifiers for user intent. Use analyzer-emitted typed
  profiles, facet contracts, and structural answer-document fields.
