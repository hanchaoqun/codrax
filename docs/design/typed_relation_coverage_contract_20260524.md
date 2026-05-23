# Typed Relation Coverage Contract

Date: 2026-05-24

## 1. Problem

T12 fixed the first production regression in the typed-relation family:
`interface / trait / protocol -> implementer` facts now flow into exploration,
finalization, and the pre-complete relation member-set guard. That fix is
intentionally narrow, but the current code still has an architectural smell:
the pre-complete guard is named and implemented as `Implementer`-specific logic.

The next step must not add one gate per relation. The correct shape is one
common relation coverage contract, with individual relation carriers plugged in
only when they have precise typed data.

Target relation families:

- interface / trait / protocol -> implementer
- class/type inheritance and subclass
- overrides / conformance
- caller / callee
- references / type usage
- registration / binding
- import / dependency
- package/module/export membership
- config key -> read site
- route -> handler
- event -> observer/subscriber
- external observation -> current-source anchor

## 2. Red Lines

1. User intent and model-authored answer remain authoritative.
2. System-generated relation facts are prompt/context hints or append-only
   supplements. They must not replace or delete model output.
3. No hard gate may read raw user prose or model free-form prose.
4. A relation hard gate is allowed only when every input signal is precise:
   typed relation carrier, grounded evidence for the same member, requested
   source scope, and a model-authored principal member_set that omitted that
   member without an explicit exclusion.
5. Graph-only candidates are never enough for a hard gate.
6. Name-only / ambiguous carriers are soft guidance unless they resolve to
   canonical symbol IDs or exact file paths.
7. The contract must be language-neutral across every language repomap supports.

## 3. Current Code Inventory

### Analyzer and request traits

- `internal/types/request_traits.go`
  - `ShouldSurfaceTypedRelationHints`
  - `HasTypedRelationMemberSetShape`
  - `RequiresRelationMemberSetHandoff`
- These helpers already use typed analyzer fields, not keyword matching.
- Current gap: relation shape is mostly reduced to `AxisImplement` or generic
  `IsRelationalLookup`. Other relation axes do not yet have a common kind list.

### Prompt/context relation hints

- `internal/context/typed_relations.go`
  - `ProbeTypedRelations`
  - `probeImplements`
- `internal/types/typed_relation_hint.go`
  - `TypedRelationHint`
  - `TypedRelationMember`
  - `TypedRelationAnchorKind`
  - `AllTypedRelations`
- `internal/context/builder.go`
  - `renderTypedRelationAppendix`
  - `evidenceLineForTypedMember`
- Current gap: the file already documents a future probe table, but only
  `implements` is implemented.

### Repomap graph carriers

- `internal/tool/repomap/types/types.go`
  - `Symbol`
    - `Implements`
    - `RequiredMethods`
    - `Parent`, `Receiver`, `Doc`, `Signature`
  - `Import`
  - `Relation`
    - `Kind`: `import`, `call`, `reference`, `type_usage`, `inheritance`,
      `embedding`
    - `ToEP` with `SymbolID`, `Name`, `Receiver`, `File`, `Line`
  - `FileInfo`
    - `Symbols`, `Imports`, `Relations`, `Package`
- `internal/tool/repomap/types/graph.go`
  - `ImplementersOf`
  - `FilesImporting`, `FilesImportedBy`
  - `SymbolsInFile`
  - `CallersOfID`
  - `ResolveCallTarget`
  - `TransitiveDeps`, `TransitiveReverseDeps`
- Current gap: these primitives are not exposed through one generic relation
  provider. Downstream code imports relation families ad hoc.

### Multi-repo bridge

- `internal/types/typed_relation_hint.go`
  - `TypedRelationImplementerSource`
- `internal/tool/repomap/multigraph/multigraph.go`
  - `ImplementerMembersOf`
- Current gap: only implementers have a cycle-free interface. Other relation
  families need the same boundary so `internal/tool` does not import concrete
  multigraph code and create cycles.

### Exploration pre-complete guard

- `internal/tool/emit_investigation_complete.go`
  - `relationMemberSetHandoffDowngrade`
  - `relationMemberSetGroundedImplementerGaps`
  - `relationTypedImplementersForRequest`
  - `relationGroundedImplementerEvidenceKeys`
  - `relationPrincipalMemberSetKeys`
- Current gap: the safety rule is correct, but the implementation is
  implementer-specific. It should become:
  `typed relation candidates + grounded same-member evidence + requested scope`
  instead of `implementer candidates`.

### Aggregate/finalizer consumption

- `internal/types/answer_aggregate_fact.go`
  - `PrincipalRelationMemberSetFactRefsForRequest`
  - `AnswerAggregateFactHasRelationMembers`
  - `AnswerAggregateMemberRelationParts`
  - relation left-axis helpers
- `internal/types/answer_support_plan_facet_evidence.go`
  - `aggregateRelationMemberEvidence`
  - `aggregateEvidenceSupportsRelationMember`
  - endpoint and path matching helpers
- These are already relation-generic and should be reused.

## 4. Unified Contract

### 4.1 Relation Kind

Keep the current external string values initially to avoid a schema flag day,
but centralize them as a typed closed set:

```go
type TypedRelationKind string

const (
    RelationImplements   TypedRelationKind = "implements"
    RelationExtends      TypedRelationKind = "extends"
    RelationOverrides    TypedRelationKind = "overrides"
    RelationCalledBy     TypedRelationKind = "called-by"
    RelationReferences   TypedRelationKind = "references"
    RelationImports      TypedRelationKind = "imports"
    RelationExports      TypedRelationKind = "exports"
    RelationRegisters    TypedRelationKind = "registers"
    RelationScopedTo     TypedRelationKind = "scoped-to"
)
```

Existing JSON remains unchanged: the values still serialize as strings.

### 4.2 Relation Query

The context probe and pre-complete guard should use one request object:

```go
type TypedRelationQuery struct {
    Kinds       []TypedRelationKind
    Sources     []string
    Request     *RequestModel
    MaxMembers  int
    Purpose     TypedRelationPurpose // PromptHint or CoverageGate
}
```

The query consumes typed analyzer fields and existing structural-scope helpers.
It must not inspect raw user text.

### 4.3 Relation Candidate

`TypedRelationMember` can remain the prompt-facing row. Coverage needs a richer
internal candidate:

```go
type TypedRelationCandidate struct {
    Relation   TypedRelationKind
    SourceName string
    SourceFile string
    SourceLine int
    Member     TypedRelationMember
    Carrier    TypedRelationCarrierKind
    Precision  TypedRelationPrecision
}
```

Precision levels:

- `ExactSymbolID`: canonical symbol ID or exact file path relation.
- `ExactFile`: exact file-to-file import/dependency relation.
- `ExactEvidence`: evidence-origin relation with exact source/line or external
  artifact anchor.
- `NameOnly`: graph has names but not enough identity; prompt hint only.
- `Heuristic`: never hard gate.

### 4.4 Provider Boundary

Add one cycle-free interface in `internal/types`:

```go
type TypedRelationCandidateSource interface {
    TypedRelationCandidates(TypedRelationQuery) []TypedRelationCandidate
}
```

Concrete providers:

- single-repo graph adapter in `internal/context` or repomap-facing package
- multi-repo adapter in `internal/tool/repomap/multigraph`
- external observation adapter over the existing observation/evidence ledger

Do not make `internal/tool` import concrete repomap/multigraph packages beyond
the graph type it already reads. Prefer the interface for multi-repo and
external observations.

### 4.5 Hard Gate Rule

A pre-complete missing-member gap is reported only when:

1. `RequiresRelationMemberSetHandoff(rm)` or a future typed relation handoff
   trait is true.
2. Provider returns an `ExactSymbolID`, `ExactFile`, or `ExactEvidence`
   candidate inside requested source scope.
3. `ctx.Mutable.EmittedEvidence()` has grounded or recovered evidence for the
   same relation member and the same file/artifact anchor.
4. A model-authored principal member_set exists but omits that member.
5. The omitted member is not present in `excluded[]`.

If no member_set exists, the existing downgrade should ask for a structured
handoff. If candidates exist but no same-member grounded evidence exists, the
system may provide soft guidance only.

## 5. Relation Family Maturity Matrix

| Relation family | Existing carrier | Initial contract mode | Notes |
|---|---|---:|---|
| implements | `Symbol.Implements`, `Graph.ImplementersOf`, `MultiGraph.ImplementerMembersOf` | Hard-gate eligible | Already shipped for exact + grounded evidence; migrate to common helper first. |
| import/dependency | `FileInfo.Imports`, `ImportGraph`, `ReverseImports`, transitive deps | Hard-gate eligible for exact file paths | Works across resolver-supported languages; unresolved imports are negative/uncertain evidence, not hard coverage. |
| inheritance/subclass | `Relation.Kind=inheritance`, symbols parent/receiver | Prompt hint first, hard-gate after exact source/member evidence | Extractor support varies by language; gate only exact rows with file/line. |
| caller/callee | `CallersOfID`, `ResolveCallTarget`, `Relation.Kind=call` | Hard-gate eligible only with canonical target ID | `CallersOf(name)` remains legacy/name-only and must be soft. |
| references/type usage | `Relation.Kind=reference/type_usage`, `RankIndex.RefCountByID` | Soft first | Often high volume and noisy; use as ranking/hints unless exact endpoint evidence exists. |
| overrides/conformance | language extractors and `Relation.Kind=inheritance` derivatives | Soft first | Need extractor audit before hard-gate. |
| registration/binding | LLM evidence, `AnchorAssignment`, relation labels | Evidence-driven first | Different languages/frameworks express this differently; graph-only carrier is not stable enough yet. |
| scoped-to / package exports | `FileInfo.Package`, `SymbolsInFile`, exported flag | Prompt hint / source-inventory boundary | Must not revive old source-inventory rewrite bugs; append-only support only unless user asks inventory. |
| route/config/event observer | framework-specific evidence and external observation anchors | Evidence-driven | Add typed relation only when upstream emits exact structured evidence. |
| external observation -> source anchor | observation ledger, runtime artifact evidence, source evidence | Hard-gate eligible after exact artifact anchor + source anchor | Covers logs, traces, VCS, command output, MCP/web/connector artifacts. |

## 6. Data Flow

```mermaid
flowchart TD
  A["analyzer typed request model"] --> B["relation query builder"]
  B --> C["relation candidate providers"]
  C --> D["TypedRelationHints in AgentContext"]
  D --> E["explorer prompt structured evidence appendix"]
  E --> F["LLM emit_evidence / aggregate_facts"]
  F --> G["pre-complete relation coverage check"]
  G --> H["extractor / finalizer relation member refs"]
  H --> I["answer panel"]
```

The same provider feeds prompt hints and coverage checking, but the purpose flag
changes behavior:

- `PromptHint`: may include exact and soft candidates, clearly tagged by
  provenance/precision.
- `CoverageGate`: exact candidates only, and only when paired with grounded
  same-member evidence.

## 7. Implementation Plan

### R0. Design and inventory

Status: **Done**

- [x] Inspect `TypedRelationHint`, context probe, graph carriers, aggregate
  relation matchers, and pre-complete relation member-set guard.
- [x] Confirm there are reusable graph primitives for imports, calls,
  inheritance, references, scoped symbols, and implementers.
- [x] Confirm existing aggregate/finalizer helpers are already relation-generic
  and should not be duplicated.
- [x] Land this design and link it from the gap tracker.

### R1. Common relation types and provider boundary

Status: **Done**

- [x] Add typed constants around existing relation string values.
- [x] Add `TypedRelationQuery`, `TypedRelationCandidate`,
  `TypedRelationCandidateSource`, precision enum, and helper canonicalizers in
  `internal/types`.
- [x] Keep JSON/string wire values stable.
- [x] Add structural tests:
  - every `AllTypedRelations()` value maps to anchor kind
  - every relation kind has a declared precision policy
  - provider boundary has no package cycle
- [x] Add a generic `MultiGraph.TypedRelationCandidates` bridge for the already
  precise implementer relation while preserving the older
  `TypedRelationImplementerSource` compatibility path.
- [x] Centralize relation carrier selection in
  `internal/context.typedRelationCarriersFromBus`: merge generic
  `BusContext.MultiGraph` providers and legacy `Mutable.SearchGraph()` with
  stable de-dup. This prevents future single-vs-multi repo drift where a caller
  accidentally reads only one graph-shaped field, and also handles early prompt
  assembly when a MultiGraph provider is present but currently returns no active
  candidates.

### R2. Migrate implementer coverage to common helper

Status: **Done**

- [x] Replace `relationTypedImplementerGap` and
  `relationMemberSetGroundedImplementerGaps` with a generic relation coverage
  helper.
- [x] Keep existing implementer behavior equivalent:
  - graph-only members not forced
  - production source scope honored
  - omitted grounded implementer prompts structured handoff
  - multi-repo provider still works
- [x] Add regression tests that prove the generic helper catches the current s5a
  class without mentioning implementers in gate logic.

Implementation note: this batch deliberately does not activate imports,
inheritance, caller/callee, or registration carriers yet. It only moves the
existing precise implementer safety rule onto the common candidate contract, so
future relation families can reuse the same exact-carrier + grounded-evidence +
scope + model-authored-member_set rule without adding one-off gates.

Regression note: the first focused replay after R1/R2 initially failed because
the explorer prompt still read typed relation hints only from
`Mutable.SearchGraph()`, while the normal single-repo run was using
`BusContext.MultiGraph` as the active carrier. A later replay exposed the
opposite edge: MultiGraph may be present but empty/early, so relation hints must
not stop there. The fix is not a one-off qf patch: relation probes now go
through the centralized carrier sequence and merge outputs with relation/source/
member/file de-dup. Tests pin provider-preferred, legacy fallback, and
cross-carrier de-dup behavior.

Schema-repair note from the same replay: the finalizer can emit a block with a
valid typed `diagram` payload but a stale outer `kind` discriminator (for
example `kind=section`). This is a lossless schema mismatch, not a content
error. The unified answer-block normalizer and final persist chokepoint now
repair only this precise condition by setting `kind=diagram`. The system still
does **not** infer diagrams from Mermaid-looking prose or `text`, and still
rejects `kind=diagram` blocks that omit the typed `diagram` object.

Axis-drift note: a later replay showed the analyzer can classify an explicit
interface type-relationship diagram as `predicate_axis=define` while still
emitting typed `answer_subject=interface_name` and `diagram_hint=architecture`.
The short-term repair allows typed relation probes for this schema shape, but
this must remain a prompt-hint allowance only: hard coverage still requires
exact graph candidates plus grounded same-member evidence. The long-term
architecture task is to move this and future relation triggers into one central
`TypedRelationQuery` kind-selection policy:

- interface / trait / protocol diagrams may request `implements` and `extends`;
- caller/callee questions map through typed axis/profile to `called-by`;
- import/export/dependency questions map through typed axis/profile to
  `imports` / `exports`;
- registration/event/config/route/external-observation relations enter through
  their precise typed profiles or evidence carriers;
- no relation kind is selected from raw user prose or model free-form text.

### R3. Import/dependency relation provider

Status: **Done — 2026-05-24**

- Add exact file relation candidates for direct imports and reverse imports.
- Use `ImportGraph` / `ReverseImports` rather than scanning import strings.
- Treat unresolved imports as negative/uncertain evidence, not hard coverage.
- Add Go + at least one non-Go fixture/eval case.

Detailed design:

1. Add a reusable graph adapter package under repomap, not a new evidence
   stack:
   - input: `*repomap/types.Graph` + `types.TypedRelationQuery`;
   - output: `[]types.TypedRelationCandidate`;
   - supported in this batch: existing exact `implements` rows plus new
     `imports` / `exports` rows.
   The adapter may import `internal/types`, but the low-level
   `repomap/types.Graph` package must stay free of `internal/types` imports.
   This avoids forcing the graph model itself to know about answer-stage
   contracts while still preventing duplicate provider logic in context,
   multigraph, and pre-complete coverage.

2. Import/export semantics:
   - `imports`: source file/package/directory depends on member file(s).
   - `exports`: source file is imported by member file(s) (reverse import
     edge). The name is kept for compatibility with the existing relation enum;
     it is a typed reverse-dependency row, not a claim that the source language
     has an `export` keyword.
   - For exact file sources, precision is `ExactFile` and rows are eligible for
     future hard coverage only after the existing downstream requirements also
     hold: current-source scope, grounded same-member evidence, and a
     model-authored principal `member_set` omission.
   - For directory/package scoped prompt hints, rows may be surfaced as
     prompt guidance but must be `NameOnly` / soft when the source itself is not
     an exact file. This keeps broad package questions useful without letting a
     directory-wide graph walk become a hard gate.

3. Grounding and source-file policy:
   - The candidate member for `imports` is the imported file path; the observed
     relation source is the importer file.
   - The candidate member for `exports` is the importing file path; the observed
     relation source is also that importing file.
   - No fake line numbers are invented. If repomap cannot map an import
     statement line to a resolved edge, the edge remains file-exact and
     line-less. Any later hard gate still requires separate grounded evidence
     emitted by the model.

4. Multi-repo behavior:
   - `MultiGraph.TypedRelationCandidates` delegates to the same graph adapter
     for active sub-repos and prefixes file paths back to path-from-parent.
   - Cross-sub-repo import edges are not invented; current repomap namespaces
     each sub-repo independently.
   - Exact file sources remain hard-gate eligible only inside the active graph
     that owns the file. Package/name-only sources are prompt-only.

5. Language coverage:
   - The implementation does not parse language syntax. It consumes
     `ImportGraph`, `ReverseImports`, `FileInfo.Package`, and `FileInfo.Language`
     that are already produced by repomap's language extractors/resolvers.
   - Tests must prove the adapter is language-neutral by running the same graph
     edge fixture across `SupportedReadLanguages()` and by pinning a non-Go
     package/directory prompt case.

R3 task list:

- [x] Audit existing import graph primitives and typed relation selector.
- [x] Add shared graph relation adapter for `imports` / `exports`.
- [x] Wire single-repo prompt probing through the shared adapter.
- [x] Wire `MultiGraph.TypedRelationCandidates` through the shared adapter with
  path-from-parent prefixing.
- [x] Update coverage fallback to use the shared graph adapter instead of
  implementer-only graph fallback.
- [x] Add unit tests for exact file imports, reverse imports, directory/package
  soft prompt hints, multi-repo path prefixing, and all supported repomap
  languages.
- [x] Run focused unit tests and update this document with results.

R3 implementation results:

- Added `internal/tool/repomap/relation` as the shared graph adapter. It
  consumes existing repomap graph carriers (`ImplementersOf`, `ImportGraph`,
  `ReverseImports`, `FileInfo.Package`, `FileInfo.Language`) and emits
  `TypedRelationCandidate` rows without adding a second evidence stack.
- `internal/context.ProbeTypedRelations` now uses the same adapter for
  single-repo graphs. For import-path inventory questions, exact required files
  are added to typed query sources before probing, so analyzer entities like
  `explorer.go` do not hide exact paths such as `internal/agent/explorer.go`.
- `internal/tool/repomap/multigraph.MultiGraph` delegates import/export
  candidate lookup to the shared adapter per active sub-repo, then prefixes
  returned files back to parent-repo paths. It does not invent cross-sub-repo
  edges.
- `emit_investigation_complete` uses the same provider for pre-complete
  relation coverage. For `imports` / `exports`, grounded evidence may live at
  the observed importer file or the member file; this avoids forcing the model
  to read an imported target file when the relation was already observed at the
  import site.
- A replay exposed a separate systemic evidence-matching risk: path-like
  members with the same tail segment, such as `internal/types` and
  `internal/tool/repomap/types`, could inherit the wrong citation when the
  model's member_set omitted support refs. The aggregate matcher now accepts a
  tail fallback only when that tail is unique, and otherwise prefers exact
  multi-segment path-suffix identity. This fix is language-neutral for import /
  use / require style paths and prevents the support plan from silently
  mis-citing same-tail members.
- Focused tests passed:
  `go test ./internal/types ./internal/tool/repomap/relation ./internal/context ./internal/tool/repomap/multigraph ./internal/tool`.
- Focused eval passed:
  `eval/results/qf_imports-20260524-051527`. The final answer preserved all
  import members, including `github.com/hanchaoqun/codrax/internal/types`
  with the correct `internal/agent/explorer.go:29` citation, with no finalizer
  retry and no answer-contract rejection.

R3 residual note:

- The same `qf_imports` replay still used four analyzer rounds before emitting
  `emit_analysis`. This was not a reject loop and did not affect final answer
  correctness, but it is a lower-priority analyzer pre-scan efficiency gap to
  revisit under prompt/agent-budget work rather than the typed relation
  provider contract.

### R4. Inheritance/subclass relation provider

Status: **Pending**

- Read `Relation.Kind=inheritance` and symbol definitions.
- Emit prompt hints across languages that populate inheritance relations.
- Allow hard coverage only for exact source/member evidence.
- Add fixtures for at least Java/Kotlin/TypeScript/Python or other supported
  languages with reliable extractor output.

### R5. Caller/callee provider

Status: **Pending**

- Prefer `CallersOfID` and `ResolveCallTarget`.
- Mark `CallersOf(name)` results as `NameOnly` / prompt-only.
- Add tests where same method name appears on multiple receivers.

### R6. Registration/binding and event observer evidence carrier

Status: **Pending**

- Start evidence-driven: only exact LLM/tool-emitted evidence enters hard
  coverage.
- Share the same relation member parser and support-ref matching.
- Avoid framework-specific keyword tables.

### R7. External observation relation carrier

Status: **Pending**

- Model log/trace/VCS/command/MCP/web/connector observations as relation
  candidates only when they have exact artifact anchor plus current-source
  anchor.
- Reuse existing external evidence/artifact ledgers and blob/page readers.
- Add negative observation support through the same exact/uncertain split.

### R8. Prompt and finalizer ledger alignment

Status: **Pending**

- Keep typed relation hints as a single unified structured-evidence appendix.
- Do not create duplicate per-relation sections.
- Ensure rich `summary` / `note` from model evidence beats dry graph hints for
  duplicate tuples.
- Preserve localized append-only supplement wording.

### R9. Central relation-kind selection

Status: **Done — 2026-05-24**

- Replace scattered relation-trigger checks with one
  `TypedRelationQuery`-building policy that maps typed request shape to allowed
  relation kinds.
- Keep current short-term interface-diagram support as the first test case, but
  migrate it into this central selector.
- Add tests for interface diagram, call relation, import/export relation,
  registration/event relation, and external-observation→source-anchor relation.
- Prove every selector input is typed (`AnswerSubject`, `DiagramHint`,
  `PredicateAxis`, request profiles, or precise evidence carrier), never raw
  request/model prose.

Detailed design:

1. Add one selector in `internal/types`:
   `BuildTypedRelationQuery(RequestModel, TypedRelationPurpose, maxMembers)`.
   The selector returns a `TypedRelationQuery` containing:
   - `Kinds`: closed `TypedRelationKind` enum values selected only from typed
     request fields.
   - `Sources`: the existing provenance-aware
     `StructuralRelationScopeCandidates` lane, with the same prompt/coverage
     fallbacks the current callers already use.
   - `Purpose`: `prompt_hint` or `coverage_gate`, so providers can keep soft
     prompt rows separate from exact hard-gate rows.

2. Selector inputs are intentionally narrow:
   - `PredicateAxis=implement` selects `implements`.
   - `AnswerSubject=interface_name` plus non-empty `DiagramHint` selects
     `implements` and `extends`.
   - `PredicateAxis=call` selects `called-by`.
   - `PredicateAxis=register` selects `registers`.
   - `SourceInventoryProfile` with principal/import-path roles selects
     `imports` and `exports`.
   - Existing category/count/relational shapes continue to request
     `implements` for prompt compatibility, but hard coverage still requires
     exact provider rows, current-source scope, and grounded member evidence
     before it can complain.

3. The selector must not inspect:
   - `RequestModel.RawRequest`
   - analyzer `EntityAxes` free-form strings
   - model free-form prose, closure prose, or finalizer prose
   - localized keywords such as "implements", "imports", "注册"

4. Migration points:
   - `ShouldSurfaceTypedRelationHints`, `HasTypedRelationMemberSetShape`, and
     `HasInterfaceTypedRelationDiagramShape` become wrappers over the selector
     where possible.
   - `internal/context/typed_relations.go` builds its provider query through the
     selector instead of hard-coding `implements`.
   - `internal/tool/emit_investigation_complete.go` uses the same selector for
     relation coverage gates. Legacy implementer fallback only runs when the
     query allows `implements`.

5. Safety / non-regression:
   - A selected relation kind is only a request to a provider. If no exact
     provider exists yet, the result is empty and no prompt/gate behavior
     changes.
   - Hard gates remain downstream of exact carrier precision +
     current-source scope + same-member grounded evidence + model-authored
     member_set comparison.
   - The first batch does not implement new relation providers. It only
     centralizes query construction so future `extends`, `called-by`,
     `imports`, `exports`, `registers`, runtime/MCP/web carriers can plug into
     one contract without adding side gates.

6. Carrier-resolved source facts:
   - The selector also has
     `BuildTypedRelationQueryWithResolvedSources(RequestModel, purpose,
     maxMembers, []TypedRelationSourceFact)`.
   - `TypedRelationSourceFact` is a typed provider record such as
     "LoopController resolved as interface at file:line"; it is not derived
     from raw request text or model prose.
   - In the first shipped batch, exact interface / trait / protocol source
     facts activate `implements` + `extends` prompt hints for diagram requests
     even when the analyzer only emitted a broad architecture diagram shape.
   - This source-fact path is intentionally generic: later relation families
     must plug precise source/evidence facts into the same selector instead of
     adding side doors.

R9 task list:

- [x] Audit existing typed relation entry points and identify duplicated kind
  selection.
- [x] Add central `TypedRelationQuery` builder in `internal/types`.
- [x] Migrate prompt hint probing and pre-complete coverage gates to the
  builder.
- [x] Add tests for interface diagram, call/register axes, import-path profile,
  legacy prompt compatibility, and raw-prose non-selection.
- [x] Run focused unit tests and `qf_type_relation_loop_controller`.
- [x] Update this document with results and push the batch.

R9 implementation results:

- `internal/types.BuildTypedRelationQuery` is now the single typed request
  selector for relation kinds.
- `internal/types.BuildTypedRelationQueryWithResolvedSources` lets exact
  carrier facts contribute source-kind information without scanning prose.
- `internal/context.ProbeTypedRelations` builds provider queries through the
  selector and resolves exact source facts from both single-repo graphs and
  generic providers.
- `internal/tool/repomap/multigraph.MultiGraph` now implements
  `TypedRelationSourceFactProvider`, so single-repo and multi-repo runs use the
  same source-fact lane.
- `internal/tool/emit_investigation_complete.go` uses the same selector for
  relation coverage gates. The legacy implementer fallback only runs when the
  query explicitly allows `implements`.
- Unit tests passed:
  `go test ./internal/types ./internal/context ./internal/tool/repomap/multigraph ./internal/tool`.
- Focused eval passed:
  `eval/results/qf_type_relation_loop_controller-20260524-044346`.
  The earlier failed replay showed the exact architecture gap: the analyzer
  emitted a broad diagram hint plus entity, but not an interface answer subject;
  resolved graph source facts now close that gap without using raw prose.

R9 follow-up relation family activation:

These are the next provider-mapping tasks. They must extend the same
`BuildTypedRelationQuery*` selector and `TypedRelationCandidateSource` /
`TypedRelationSourceFactProvider` boundaries. Do not add new per-family gates or
raw-text selectors.

| Family | Exact typed source/evidence fact needed | Provider work | Gate posture |
|---|---|---|---|
| `called-by` / call graph | function/method source resolves to canonical symbol ID; receiver disambiguated when present | Map `CallersOfID` / `ResolveCallTarget` rows into `TypedRelationCandidate` and add ambiguity tests for duplicate method names | Hard only for exact symbol ID + grounded same-member evidence; name-only callers remain prompt-only. |
| `imports` / `exports` | file, package, module, or import path resolves through repomap import graph | Add provider over `ImportGraph`, `ReverseImports`, and exact package/module export rows; include Go plus non-Go fixtures covered by repomap import extraction | Hard only for exact file/import-path rows inside requested scope; unresolved imports become uncertainty/negative evidence. |
| `registers` / event observer / binding | model/tool emits exact registration evidence or graph extractor emits exact binding relation | Keep evidence-driven first; add provider rows only when source and member both have exact anchors | No framework keyword tables; hard only after exact evidence + grounded same-member evidence. |
| `extends` / inheritance / overrides | source type resolves as class/interface/trait/protocol and relation endpoint has exact graph file:line | Generalize graph relation rows for inheritance/override/conformance across languages with reliable extractor output | Prompt first; hard only for exact rows with grounded evidence. |
| references / type usage | endpoint resolves to exact symbol ID and requested question asks for reference membership | Provider can expose capped prompt hints from exact graph references; high-volume rows need rank/budget policy | Soft by default; hard only for explicit bounded member_set with exact evidence. |
| external observation -> source anchor | log/trace/VCS/command/MCP/web/connector artifact span plus current-source anchor | Reuse external evidence/artifact ledger and blob/page readers to create exact observation relation rows | Hard only when both artifact anchor and current-source anchor are exact; otherwise append localized uncertainty. |

This backlog is intentionally relation-family agnostic: adding a family means
supplying typed facts and provider tests, not teaching downstream stages a new
case-by-case rule.

### R10. Eval and telemetry

Status: **Pending**

- Add focused evals for import/dependency, caller/callee, inheritance, and
  external observation + current source.
- Record for each relation family:
  - prompt hint count
  - hard gate count
  - soft guidance count
  - finalizer retry count
  - answer richness retention

## 8. Non-goals

- Do not make source-inventory repair a relation answer author.
- Do not infer relation intent by keyword matching raw user/model text.
- Do not force graph-only candidates into final answers.
- Do not replace model tables or prose with system-generated tables.
- Do not implement framework-specific route/config/event detectors until a
  precise structured carrier exists.

## 9. First Batch Recommendation

Start with R1 + R2 only. They lower architecture risk without expanding
behavioral surface. After implementers are migrated to the common helper and
tests prove parity, add imports/dependencies as R3 because that carrier is exact
file-to-file data and broadly useful across languages.
