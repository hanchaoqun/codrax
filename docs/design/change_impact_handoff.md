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

5. Finalizer can narrow the user's criterion

   - `internal/agent/answer_document_evaluator.go` tells finalizer to use
     support lanes, but no impact contract says: "do not answer only direct
     assignments when the user's criterion is affected sites."
   - `internal/orchestrator/contract_check.go` has topic-mismatch and
     support-member coverage checks, but without an impact profile the
     checks cannot tell that assignment-only is an impermissible narrowing.

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

### Final-answer guard

For impact answers, the rendered principal list/table must not narrow the
criterion to one site kind unless the typed impact profile says the requested
output is that one kind. A finalizer answer that says "only direct
assignments" while the profile requests broad affected sites should be
reworked as topic mismatch / impact undercoverage.

The guard should not synthesize missing members. It should request a
finalizer-only rewrite when support lanes already contain the members, or an
explore/extract retry when typed support lanes are incomplete.

## Implementation Checklist

- [x] Add `ChangeImpactProfile` and `ImpactAffectedSiteKind` types.
- [x] Extend `emit_analysis` schema, parser, summary, and tests.
- [x] Update analyzer prompt text with contract-oriented impact guidance.
- [x] Project impact targets into compiler search hints.
- [x] Extend enumeration facet acceptable forms when impact profile is active.
- [x] Preserve heterogeneous principal evidence for impact profiles.
- [x] Update support-lane guidance for impact answers.
- [x] Add tests proving guard/read/call/validation sites enter principal lane.
- [x] Add negative test: ordinary enumeration still keeps homogeneous curation.
- [ ] Add finalizer/contract tests preventing assignment-only narrowing when
      typed impact profile requires broad affected sites.
- [ ] Run focused tests and `go test ./...`.
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
