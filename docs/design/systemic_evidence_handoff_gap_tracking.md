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
| G6 | Assignment/field evidence is still uneven across Go structs, C/C++ designated initializers, ArkTS object literals, Cangjie declarations, and config syntaxes. | `internal/tool/repomap`, `internal/types/evidence.go`, `internal/types/claim_form.go`, `internal/tool/ground/ground.go`, `internal/agent/explorer.go` | Classify field/property/member literal rows through the language-neutral `anchor_kind=initializer`, then project them to `ClaimAssignmentFact` with assignment-like grounding and support-lane behavior. Fallback string evidence remains soft guidance, not a hard gate. | Implemented for initializer anchors; keep extending with repomap member surfaces |
| G7 | Aggregate count/member-set drift appears when the model emits only prose counts or partial aggregate facts. | `internal/types/answer_aggregate_fact.go`, `internal/tool/emit_investigation_complete.go`, `internal/agent/aggregate_fact_render.go` | Keep aggregate facts model-owned and structurally self-consistent: `len(members)==value`, file:line totals require `unique_count`, member-set value may be canonicalized only from emitted members. | Existing, keep enforcing |
| G8 | Diagrams and block shapes can reflect system scaffolding instead of the user's requested answer surface. | `internal/types/answer_semantic_view_*`, `internal/agent/answer_document_evaluator.go`, `internal/orchestrator/contract_check_block.go` | Compile answer family and allowed block kinds from typed profiles/facets; finalizer may enrich diagrams from grounded seeds, but principal blocks must stay aligned to the user-requested surface. | Existing, keep auditing |
| G9 | Owner-qualified change-impact targets can be polluted by unrelated fields with the same leaf name. | `internal/types/answer_support_plan_facet_evidence.go` | For `Owner.member` targets, principal evidence must structurally match the owner-qualified path or define the owner itself; leaf-only matches stay context. This covers Go selectors, C/C++ `::` / `->`, ArkTS/Cangjie member paths, and config-like dotted paths. | Implemented in this phase |
| G10 | Change-impact file answers can promote documentation / mechanism comments as production code sites. | `internal/types/answer_support_plan_facet_evidence.go` | Treat mechanism, related-context, and illustrative evidence as support context for change-impact principal lanes unless the analyzer's typed profile explicitly requests documentation / generated / build artifacts. Code-site answers should be driven by direct / conditional / relationship / registration evidence, not comment-only mechanism anchors. | Implemented in this phase |
| G11 | Finalizer retry can drop `citations[]` or reference an out-of-range citation index, then downstream diagnostics misreport principal-member coverage. | `internal/tool/answer_document_pre_emit_check.go` | Add a schema-shape pre-check for `blocks[].items[].citation_ref` against the emitted citation pool before semantic member checks. The repair asks the model to preserve / extend its own citation pool; system code never invents citations. | Implemented in this phase |
| G12 | Patch retries can replace the citation pool while preserving old citation-bearing blocks, causing citation role drift across unchanged content. | `internal/types/answer_document_v2_patch.go`, `internal/tool/emit_answer_document_patch.go` | If a patch uses `replace_citations`, every inherited block with non-negative `citation_ref` must also be replaced or removed. Otherwise use `append_citations` or full emit. This keeps citation indexes structural instead of relying on the model to remember hidden coupling. | Implemented in this phase |
| G13 | Retry full emits can put `summary` after lists/tables, rendering the answer backwards even when the facts pass. | `internal/tool/answer_document_pre_emit_check.go` | When the semantic view requires a summary block, pre-emit enforces that the first rendered block is `summary`. This is order-only; it does not create or rewrite content. | Implemented in this phase |
| G14 | Change-impact exploration can emit a real affected line as context because the target is present only in `summary`, not in target-bearing structured fields. | `internal/types/answer_support_plan_facet_evidence.go`, `internal/tool/emit_investigation_complete.go`, `internal/agent/explorer.go` | For active change-impact broad outputs, pre-complete checks the already-read source line against the typed target. If the line names the target but the evidence fields do not, completion is downgraded and the model must re-emit grounded evidence with the target in `snippet` / `subject` / `object` / `surface_terms`. The system never promotes summary text into the answer set. | Implemented in this phase |
| G15 | Struct/object/config initializer rows can be mislabeled as `definition`, causing citation role drift and dry `definition_fact` prose for non-symbol principal sites. | `internal/types/evidence.go`, `internal/tool/emit_evidence.go`, `internal/tool/ground/ground.go`, `internal/types/answer_support_plan_*` | Add a first-class `AnchorInitializer` surface and wire it through schema, grounding, `ClaimFormOf`, exact lookup, call-chain/root-cause/facet support lanes, axis compatibility, and concrete-value projection. This covers Go literal fields, C/C++ designated initializers, ArkTS object properties, Cangjie named arguments, and config object leaves without case-specific prompt patches. | Implemented in this phase |
| G16 | Comparison / mechanism answers can be routed through `QFEnumeration` after analyzer drift, then every principal support entry is forced into the main answer as if it were a user-requested set member. | `internal/types/answer_support_plan.go`, `internal/types/answer_support_plan_facet_evidence.go`, `internal/types/answer_support_member_coverage.go`, `internal/agent/answer_document_evaluator.go` | Split support lanes into two typed policies: `required` member coverage for real set-valued enumerations / change-impact / explicit boundaries, and `enrichment_only` for enumeration-shaped mechanism or comparison fallbacks. The policy consumes typed request fields (`ChangeImpactProfile`, predicates, `QuestionStructure`, `AnalyzerHints.Kind`) and never scans user text. | Implemented in this phase |
| G17 | Citation role alignment can widen an explicitly emitted block `claim_uses` to all semantic-view acceptable forms, so comparison prose that merely names two endpoints is treated as a call-edge assertion. | `internal/tool/answer_document_pre_emit_check.go`, `internal/types/evidence_role_alignment.go` | Let block-local `claim_uses` take precedence. Semantic-view acceptable forms are a fallback only when the block did not emit claim-use forms. Facet-derived call-edge widening remains available for untyped call-chain blocks, but it must not override explicit non-edge claim forms. | Implemented in this phase |
| G18 | `semantic_quality_reviewer` can emit `sufficient=false` without structured concerns; current parsing turns that into a non-fatal dispatch failure and ships the answer. | `internal/orchestrator/semantic_quality_reviewer.go`, `internal/orchestrator/contract_check.go` | Treat the model's structured `sufficient=false` verdict as load-bearing even when concerns are malformed. Normalize it into a generic semantic-quality concern using only the model-emitted verdict / confidence / reasoning, so the finalizer gets a retry signal instead of a silent pass. | Implemented in this phase |
| G19 | Line-owner / enclosing-function drift can render a valid citation under the wrong function name after retries. | `internal/tool/repomap/index/cache.go`, `internal/tool/emit_evidence.go`, `internal/tool/emit_answer_document_v2.go` | Fix the shared repomap cache invalidation path: changed-file detection now compares current content hashes against cached hashes even in clean git worktrees, so a pull/rebase/commit cannot leave stale symbol spans behind. The focused regression uses a clean committed repo with stale cached hashes. | Implemented in this phase |

## Implementation Order

1. Land G1 because it is currently reproducible after `u10b` passes: a PASS can
   still contain file-count drift. This is a finalizer-local contract fix.
2. Re-run `u10b` and a random eval shard. Any new failures are assigned to the
   ledger before code changes.
3. Land G9 before broad random eval resumes; a PASS that includes an unrelated
   same-leaf field is a principal-set correctness failure, not a richness issue.
4. Expand G10 before broad eval resumes; otherwise comment-only anchors can
   become principal production-code file members and force finalizer into
   count / role contradictions.
5. Expand G6 using repomap/tree-sitter surfaces rather than language-specific
   prompt patches. This must include Go, C/C++, Cangjie, ArkTS, and mixed
   repositories.
6. Land G12/G13 before continuing random eval. They are carrier/rendering
   stability fixes and apply to every answer family, not just change-impact.
7. Land G14 before broad eval resumes; otherwise real affected sites can be
   visible in read-file output but absent from the principal lane because the
   model failed to structure the target.
8. Land G15 with G6: initializer is a typed evidence surface, not a
   definition-like fallback. The prompt only teaches the model which enum to
   choose; hard consumers use the enum and deterministic grounding.
9. Land G16/G17 together because they are the same principal-vs-context
   boundary at two layers: one controls whether support entries become hard
   answer members, the other controls whether an item surface asserts an edge
   role. Both must prefer local typed contracts over broad semantic defaults.
10. Land G18 before trusting PASS results. A reviewer that says
   `sufficient=false` has already supplied a precise structured verdict; a
   malformed concern list is a retry-shaping problem, not permission to ship.
11. Investigate G19 with a file-symbol span regression before broad changes.
   Owner lookup is language-neutral infrastructure, so fixes must sit in the
   shared repomap / citation enrichment path.
12. Continue sampling G2/G4/G5/G8 with random eval order. If an issue is already
   covered by an existing typed lane, strengthen the consumer/gate instead of
   adding another prompt-only rule.

## Non-Goals

- Do not parse raw thinking text to recover answer content.
- Do not let system code compute a user-facing answer unless the value comes
  from model-emitted structured fields or deterministic tool output explicitly
  supplied as answer evidence.
- Do not add keyword classifiers for user intent. Use analyzer-emitted typed
  profiles, facet contracts, and structural answer-document fields.
