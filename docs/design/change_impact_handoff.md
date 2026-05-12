# Change-Impact Handoff Hardening

Status: in progress (2026-05-13)

This document tracks the generalized fix for the eval finding exposed by
`u10b`: questions of the form "what production sites / files would need
changes if target X changed shape?" were explored broadly, but the final
answer collapsed to direct assignments only.

The target is not a case patch. The fix must cover cross-language change
impact questions across Go, C/C++, Cangjie, ArkTS, Java/Kotlin/Rust/Python,
and mixed-language repositories. The system should preserve model-emitted
structured facts from exploration; it must not synthesize answer text from
system heuristics.

## Failure Summary

`u10b` request:

> Which production code sites (non-test) would need changes if
> `CitationReq.Required` were changed from `bool` to an enum type with
> three states (required / optional / disabled)? List the files.

The analyzer and explorer saw the broader affected surface:

- `internal/tool/emit_investigation_complete.go`
- `internal/context/builder.go`
- `internal/analysis/gate/gate.go`
- `internal/analysis/criterion/eval.go`
- plus definition and direct assignment files

The final answer listed only four direct assignment sites because the
principal support lane contained only `assignment_fact` members.

## Root Cause By Code Location

1. Missing typed impact intent

   - `internal/tool/emit_analysis.go`: the `emit_analysis` schema has
     diagnostics and conversation-reference typed lanes, but no generic
     "change impact / migration impact" lane.
   - `internal/types/analysis_ir.go`: `RequestModel` cannot express that
     the answer set is affected sites, nor which site kinds are expected.
   - Result: downstream code only sees `question_kind=enumeration` plus
     ordinary entities. It cannot distinguish "enumerate assigned values"
     from "enumerate every code site affected by a target shape change."

2. Enumeration facet forms are too narrow for impact questions

   - `internal/types/facet_plan.go`: `QFEnumeration` hard facet
     `FacetEnumerationItem` accepts `definition_fact`, `assignment_fact`,
     `return_fact`, and `import_edge`.
   - Read/consumer sites are often emitted as `guard_condition`,
     `call_edge`, or general current-code facts. Those are valid impact
     evidence but cannot enter the principal enumeration lane.

3. Principal support curation drops heterogeneous impact surfaces

   - `internal/types/answer_support_plan_facet_evidence.go`:
     `curateEnumerationPrincipalEvidence` falls back to
     `filterDominantEnumerationPrincipalSurface`.
   - That is useful for ordinary homogeneous lists, but wrong when the
     requested answer set is intentionally heterogeneous: definitions,
     assignments, readers, validators, builders, serializers, and call
     adapters are all principal affected sites.

4. File/site enumeration lacks a dedicated member carrier

   - `internal/agent/extractor` and `emit_answer_symbol` are symbol-shaped.
   - For "list the files/sites" questions, answer members are locations or
     files, not code symbols. Existing `member_surface=source_location`
     helps, but only after the right evidence enters a principal lane.
   - When `requested_output=files`, per-line affected sites should collapse to
     unique file members while retaining the affected lines as equivalent
     support anchors. Otherwise the finalizer is forced to list every repeated
     construction site even though the user asked for files.

5. Finalizer can narrow the user's criterion

   - `internal/agent/answer_document_evaluator.go` tells finalizer to use
     support lanes, but no impact contract says: "do not answer only direct
     assignments when the user's criterion is affected sites."
   - `internal/orchestrator/contract_check.go` has topic-mismatch and
     support-member coverage checks, but without an impact profile the
     checks cannot tell that assignment-only is an impermissible narrowing.

6. Final-answer document flat-mode can lose typed citations

   - `internal/tool/emit_answer_document_v2.go` tolerates `blocks` arriving as
     a JSON-encoded string, including the common whole-document form
     `[{block...}], "citations": [...]`.
   - A newer trace showed the same form with the closing object brace included
     inside the string: `[{block...}], "citations": [...]}`. The existing
     repair path did not recognize that shape, fell through to brace-balanced
     recovery, and then treated citation objects such as
     `{"file":"x.go","line":1}` as if they were answer blocks.
   - Result: finalizer saw contradictory `unknown field file/line` errors even
     though the LLM used the documented citation schema. This is a generic
     downstream carrier drift, not a change-impact-only problem.

7. Change-impact profile does not yet reconcile the core family fields

   - `internal/agent/analyzer.go` runs typed reconcile helpers before
     `types.ResolveQuestionFamily` is later evaluated by support-plan and
     finalizer code.
   - A trace showed the LLM correctly emitting
     `change_impact_profile.requested_output=files` while still choosing
     `intent=trace` / `question_kind=call_chain`.
   - `internal/types/facet_plan.go::ResolveQuestionFamily` gives trace intent
     higher priority than enumeration, so the answer inherited call-chain
     facets (`principal_path_edge`, `diagram_spine`, exact-resolution absence)
     even though the user asked for a file set.
   - This is a typed contradiction, not a wording problem: an active
     change-impact profile with file/site/symbol output says the principal
     answer is an affected-member set.

8. Exact-resolution is the wrong hard contract for broad impact answers

   - `internal/types/exact_lookup.go::BuildExactResolutionContract` consumes
     raw-request-aligned `exact_targets`.
   - For a change-impact question, the target (`CitationReq.Required`) is the
     changed surface whose impact is being explored, not the final answer
     target to resolve as present/absent.
   - Keeping the exact-resolution contract caused the finalizer to emit
     `exact_resolution.status=absent` and then chase negative-scope proof,
     despite the field definition itself being cited.
   - The target remains a search hint through `ChangeImpactProfile`, but it
     must not create final-answer absence obligations for broad affected-site
     outputs.

9. File-output citation alignment is still line-label shaped

   - `internal/types/answer_support_member_coverage.go` now coalesces
     `requested_output=files` obligations by unique file while retaining
     equivalent file:line anchors.
   - `internal/tool/answer_document_pre_emit_check.go` and
     `internal/orchestrator/contract_check_block.go` still mostly accept
     source-location display labels only when the label is `file:line`.
   - A file-output answer whose item label is just
     `internal/analysis/compiler/templates.go` is a valid typed principal
     member when its citation points inside that file. It should not be forced
     through symbol endpoint matching.

10. File-output prose/count drift can survive a PASS

   - `internal/agent/answer_document_evaluator.go::renderAnswerDocPrincipalMemberObligations`
     correctly exposes the typed principal lane, but older prompt text also
     allowed the finalizer to preserve numeric closure prose when it appeared
     "supported" by the evidence.
   - A `u10b` run accepted a final answer whose principal list contained seven
     affected files, while the lead summary and caveat repeated the explorer's
     stale unstructured closure count of six files.
   - `internal/types/answer_support_member_coverage.go::MissingPrincipalSupportMembers`
     detected member omission, not surface drift: a file:line item label can
     cite the right typed location and still violate `requested_output=files`.
   - The generalized fix is to make `requested_output=files` a load-bearing
     label-surface contract: the item label is the file path; file:line anchors
     remain supporting evidence inside item text/citations. Numeric file counts
     must come from the typed file obligation count or `aggregate_facts`, not
     from unstructured closure prose.

11. Owner-qualified field targets can be polluted by same-leaf fields

   - `internal/types/answer_support_plan_facet_evidence.go::curateEnumerationPrincipalEvidence`
     preserved all heterogeneous evidence for active change-impact profiles.
     That fixed assignment-only narrowing, but it also let an unrelated
     `req.Required` / `RequiredBlock.Required` guard become a principal member
     for the target `CitationReq.Required`.
   - The finalizer could recognize the distinction in prose, but
     `MissingPrincipalSupportMembers` forced it to include the unrelated file
     because the support lane had already promoted the wrong typed member.
   - The generalized fix is owner-qualified filtering for change-impact
     principal evidence. When the target is a field/member path such as
     `CitationReq.Required`, principal evidence must structurally match that
     path (`*.CitationReq.Required`) or define the owner itself (`CitationReq`).
     Leaf-only evidence (`req.Required`) remains support context, not a
     principal affected file.

12. Documentation / mechanism comments can masquerade as production code sites

   - A later `u10b` run exposed a different pollution mode: the explorer found
     comment lines in `internal/orchestrator/orchestrator.go` describing a
     degraded fallback setting, emitted them as `EvidenceMechanism` +
     `AnchorDefinition`, and the owner-qualified rule kept them because they
     mentioned `CitationReq`.
   - The final answer then listed `orchestrator.go` as a production code file
     that "writes Required=false", even though the cited anchors were
     documentation comments rather than executable code. The self-consistency
     reviewer also detected the downstream count contradiction, because the
     list had six file members while the role buckets only added up to five.
   - The generalized fix is a typed role boundary, not a comment keyword check:
     for change-impact principal lanes, `EvidenceMechanism`, related-context,
     and illustrative evidence are support context unless
     `ChangeImpactProfile.AffectedSiteKinds` explicitly includes
     `documentation`, `generated`, or `build`. Executable code-site answers stay
     grounded in direct / conditional / relationship / registration evidence.

13. Citation-pool carrier drift can hide the real repair

   - In the same run, finalizer retries temporarily dropped `citations[]` or
     referenced a non-existent citation index while trying to preserve the
     principal list. The existing pre-check then reported broad principal
     support-member misses, which is technically true but not the repair the
     model needed first.
   - The generalized fix is a carrier-level pre-check before semantic
     support-member checks: every non-negative `citation_ref` must point into
     the model-emitted `citations[]` pool. The repair prompt tells the model to
     preserve or extend its own citation pool; runtime code does not synthesize
     missing citations.
   - A later retry showed the patch-specific variant: `emit_answer_document_patch`
     replaced the whole citation pool while leaving old blocks unchanged, so
     those old blocks' integer `citation_ref` values now pointed at different
     file:line anchors. Patch application must reject this structural coupling:
     replacing citations is only valid when all citation-bearing inherited
     blocks are also replaced / removed, or when the model uses a full emit.

14. Summary block can drift to the tail after repair

   - The final accepted document rendered the principal list and table first,
     then the summary paragraph. The facts were correct, but the answer shape
     was backwards for a user-facing file-listing answer.
   - The generalized fix is an order contract, not content rewriting: when the
     semantic view requires a `summary` block, the first rendered block must be
     `summary`. The LLM still owns the summary text; runtime only rejects a
     structurally inverted block order.

15. Target-bearing affected lines can be stranded in support context

   - A later accepted `u10b` run showed `internal/agent/analyzer.go:1915` in
     the explorer evidence buffer as an `assignment_fact`, and the cited source
     line contained `out.AnswerContract.CitationReq.Required = false`.
   - The emitted evidence carried the target only in free-form `summary`; its
     structured fields had `anchor_symbol=Required` and a condition expression
     unrelated to the assignment target. The owner-qualified target filter
     correctly refused to promote this item, because reading `summary` would
     also promote negative/context sentences such as "not CitationReq.Required".
   - Result: the finalizer's principal lane omitted a real affected production
     site and placed it in a caveat. This is an evidence handoff failure, not a
     finalizer synthesis failure.
   - The generalized fix is a pre-complete structural handoff gate: when an
     active change-impact profile has a broad file/site/symbol output, and an
     already-read source line names the owner-qualified target, any
     model-emitted affected-site evidence at that line must carry the target in
     structured fields (`snippet`, `subject`, `object`, `condition`,
     `anchor_symbol`, or `surface_terms`). If it does not, completion is
     downgraded and the model must re-emit the evidence. Runtime code does not
     recover the answer member from `summary` text.

## Generalized Design

### Typed profile

Add a `ChangeImpactProfile` on `RequestModel`:

- `is_change_impact`: true when the current request asks which code sites,
  files, APIs, config locations, declarations, consumers, callers, tests, or
  downstream artifacts would need changes if a named target changed.
- `target`: the changed symbol/path/config key/API/field/type/module.
- `target_kind`: one `AnswerSubjectKind` value, when known.
- `scope`: production, test, all, or unknown.
- `requested_output`: files, sites, symbols, steps, or unknown.
- `affected_site_kinds`: typed enum values such as definition, assignment,
  read, guard, call, return, import, construction, validation,
  serialization, config, generated, documentation, build.
- `confidence`: analyzer confidence.
- `rationale`: short LLM rationale for audit logs only.

This profile is LLM-authored structured data from the current request. The
runtime may validate enum values and normalize paths, but it must not infer
the profile by scanning raw request keywords.

### Search hints

`internal/analysis/compiler/compile.go::hintsFromRM` should merge
`ChangeImpactProfile.Target` and its subject candidates into soft hints.
These hints guide search only; they are not proof.

### Facet coverage

When `ChangeImpactProfile.IsChangeImpact` is true:

- `QFEnumeration` should allow impact-relevant claim forms for
  `FacetEnumerationItem`: definition, assignment, guard, call, return,
  import, and absence where scoped.
- Principal support curation should preserve heterogeneous evidence instead
  of keeping only the dominant member surface.

The hard gate stays precise: it reads the typed profile and deterministic
`ClaimFormOf` projection. No raw prose scoring or noisy ranker score may
decide a hard validation.

### Support lane wording

The support lane should explicitly name the impact boundary:

- affected sites are principal members;
- direct assignments are one affected-site kind, not the whole answer;
- readers, guards, validators, construction sites, serialization sites, and
  call adapters are also principal when typed evidence supports them;
- contextual helper mechanisms remain context unless the emitted evidence
  itself is an affected site.
- when `requested_output=files`, file paths are the principal members and
  file:line affected sites are evidence/rationale for those files.
- mechanism / related-context / illustrative evidence is supporting context for
  code-site impact answers. It becomes principal only when the typed profile
  requests documentation, generated artifacts, or build artifacts.

### Final-answer guard

For impact answers, the rendered principal list/table must not narrow the
criterion to one site kind unless the typed impact profile says the requested
output is that one kind. A finalizer answer that says "only direct
assignments" while the profile requests broad affected sites should be
reworked as topic mismatch / impact undercoverage.

The guard should not synthesize missing members. It should request a
finalizer-only rewrite when support lanes already contain the members, or an
explore/extract retry when typed support lanes are incomplete.

### Typed family reconcile

When `ChangeImpactProfile.Active()` and `requested_output` is `files`, `sites`,
or `symbols`, analyzer post-processing must reconcile the typed classification
onto a set-valued answer:

- set `intent=enumerate`;
- set `question_kind=enumeration`;
- set `predicates.is_category_enumeration=true`;
- set `predicates.is_relational_lookup=true`;
- clear `predicates.is_cross_component` so a broad affected-file/site set is
  not mistaken for a multi-topic component comparison;
- clear scalar/count flags that contradict a set-valued principal answer;
- leave `requested_output=steps` alone so genuine migration-procedure questions
  can still use mechanism/trace scaffolds.

This rule consumes only the typed profile emitted by the analyzer. It does not
scan raw text for "files", "impact", or language-specific cues.

### Exact-resolution boundary

`BuildExactResolutionContract` must opt out for active change-impact profiles
whose output is broad affected members (`files`, `sites`, `symbols`, or
`unknown`). Exact targets can still seed search through analyzer hints and
`ChangeImpactProfile.SubjectCandidates`; they do not imply a finalizer
present/absent verdict block.

### File-output label alignment

When a principal list/table item label is a code/config path, the label is
aligned if:

- the label equals or suffix-matches the citation's file path; and
- the citation points inside that same file.

For active `requested_output=files` support lanes, member coverage still reads
the typed support plan and requires one cited row per file obligation. The
label-grounding validators only learn that file paths are display locations,
not declaration symbols. They do not invent file members or waive typed
coverage.

### File-output surface contract

When `ChangeImpactProfile.RequestedOutput=files`, the principal member surface
is the file path, even if the supporting evidence is a specific source line.
Validators enforce this by reading only:

- the typed `ChangeImpactProfile`;
- `AnswerSupportPlan` principal obligations derived from model-emitted
  evidence;
- `AnswerDocumentV2.blocks[].items[].label`;
- `AnswerDocumentV2.citations[]`.

If an answer cites the right member but labels it as `file:line`, the pre-emit
and post-emit checks ask for a finalizer-only rewrite. This is not a prose
keyword check and it does not synthesize missing answer content; it only
preserves the model's own structured file members at the user-requested
surface.

The finalizer prompt now states that the typed principal lane member count is
the file count for this profile. Unstructured closure reason text is treated as
audit context only when it conflicts with typed obligations or structured
aggregate facts.

### Target-surface materialization gate

For active broad change-impact outputs, `emit_investigation_complete` performs
one additional handoff check before finalization:

- input signals are precise: the analyzer's typed `ChangeImpactProfile`, the
  deterministic `ClaimForm` projection, model-emitted evidence fields, and
  already-read source-line text from the grounding index;
- if the source line contains the normalized owner-qualified target but the
  evidence fields do not, the tool returns a `RepairEmitEvidence` downgrade;
- the repair asks the explorer to re-emit the same grounded line with the
  target in a structured field, usually by copying the actual line into
  `snippet`;
- summary text remains audit prose and is never promoted into principal answer
  members.

This keeps the principal lane complete without weakening the owner-qualified
same-leaf filter. It also generalizes across languages because the normalized
target comparison accepts selectors, namespace members, property accesses, and
config-like dotted paths already represented in read source text.

### Owner-qualified target filter

For change-impact profiles whose target is an owner-qualified path
(`Owner.member`, `Owner::member`, `owner->member`, etc.), the principal support
curator filters evidence by typed surfaces:

- keep evidence whose structured fields (`subject`, `object`, `condition`,
  `snippet`, `surface_terms`, `anchor_symbol`, `owner_symbol`) contain the
  normalized owner-qualified target;
- keep definition evidence for the owner itself, because the type/schema
  declaration is part of the migration surface;
- keep a leaf field/member definition only when a nearby definition evidence
  item in the same file identifies the owner;
- demote leaf-only evidence with a different owner into supporting context.

The filter does not read raw request keywords and does not infer answer
members from repository text. It consumes the analyzer-emitted target and
model-emitted structured evidence fields, so it generalizes across Go field
selectors, C/C++ `::` / `->` members, ArkTS object/property access, Cangjie
members, and other repo-map backed languages.

### Structured answer carrier recovery

`emit_answer_document` flat-mode recovery should be schema-aware:

- if a stringified `blocks` payload begins with a blocks array and continues
  with document-level siblings, rebuild the full document object and preserve
  `citations`, `caveats`, `exact_resolution`, `snippets`, and
  `missing_requested_roles` at the top level;
- support both `[{block...}], "citations": [...]` and
  `[{block...}], "citations": [...]}` without forcing the model into repeated
  schema guessing;
- keep the final brace-balanced fallback, but only retain objects that are
  block-shaped (`id` + `kind`). Citation objects must never enter the block
  decoder.

This is a typed carrier repair, not answer completion: it only preserves fields
the model already emitted in the tool payload.

## Implementation Checklist

- [x] Add `ChangeImpactProfile` and `ImpactAffectedSiteKind` types.
- [x] Extend `emit_analysis` schema, parser, summary, and tests.
- [x] Update analyzer prompt text with contract-oriented impact guidance.
- [x] Project impact targets into compiler search hints.
- [x] Extend enumeration facet acceptable forms when impact profile is active.
- [x] Preserve heterogeneous principal evidence for impact profiles.
- [x] Treat `requested_output=files|sites` as a typed source-location member
      surface so extractor does not force file/path answers through
      `emit_answer_symbol`.
- [x] Coalesce `requested_output=files` support obligations by unique file while
      preserving affected file:line anchors as equivalent support locations.
- [x] Update support-lane guidance for impact answers.
- [x] Add tests proving guard/read/call/validation sites enter principal lane.
- [x] Add negative test: ordinary enumeration still keeps homogeneous curation.
- [x] Add finalizer/contract tests preventing assignment-only narrowing when
      typed impact profile requires broad affected sites.
- [x] Repair `emit_answer_document` flat-mode whole-document string recovery so
      emitted citation pools survive trailing-brace payloads.
- [x] Add a fallback shape filter so citation objects cannot be decoded as
      answer blocks.
- [x] Run focused tests and `go test ./...`.
- [x] Reconcile active change-impact file/site/symbol outputs away from
      trace/call-chain family drift.
- [x] Disable exact-resolution final-answer contracts for broad
      change-impact outputs while preserving search hints.
- [x] Accept typed file-output principal labels backed by citations in that
      file, without weakening ordinary symbol-label grounding.
- [x] Reject change-impact file-output principal items that label cited members
      as `file:line` sites instead of file paths.
- [x] Teach the finalizer to use typed file obligations / aggregate facts as
      the source of file counts instead of stale unstructured closure prose.
- [x] Filter change-impact principal evidence by owner-qualified target path so
      unrelated same-leaf fields stay support context.
- [x] Downgrade completion when a change-impact source line names the target
      but the accepted evidence only carries it in summary prose.
- [ ] Re-run `u10b` and keep the random eval sweep moving.

## Red Lines

- Do not hard-gate on raw keywords or noisy search/ranker scores.
- Do not let the system invent answer members. The model must emit typed
  evidence or aggregate/member facts; the system only preserves and validates
  those facts.
- Do not make Go-only assumptions. Site kinds must map to language-neutral
  syntax facts so tree-sitter / repomap can support C/C++, Cangjie, ArkTS,
  and other languages.
- Do not weaken ordinary enumeration. Impact behavior is activated only by
  the typed profile.
