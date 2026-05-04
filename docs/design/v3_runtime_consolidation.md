# v3 Runtime Consolidation Design

Status: in-flight (2026-05-04 → )
Baseline: `origin/main@61fe8e1`
Goals (priority-ordered): **轮次减少 / 答案丰富 / 后期维护简单**

This document supersedes `post_v2_runtime_gap_remediation.md` for the items
listed below. Earlier design docs in `docs/design/` remain valid as
historical record only.

---

## 1. Scope

Six gaps remain after v2 runtime gap remediation:

| ID | Problem | Priority |
|----|---------|----------|
| A  | RepairExecutionPlan promote/rebuild driven by global kind-set diff, not per-cluster closure | P1 |
| B  | V2 full emit / patch emit are still two write protocols (description / schema / setter / telemetry duplicated) | P1 |
| C  | Diagram relation typed-first works but skill prompt + validator output don't strongly nudge typed declaration | P2 |
| D  | Semantic quality is advisory; richness has no typed contract; no signal for "covered but prose thin" | P1 |
| E  | LLM-facing prompts retain implementation jargon (`backbone` / `spine` / `deterministic pipeline / renderer / compiler / alignment`) | P2 |
| F  | architecture.md + 17 migration docs + several code comments still describe shape-era state | P3 |

A separate prep PR (B0) unifies the ViolKind registration surface — adding a
new ViolKind today requires editing ≥6 sites; B0 collapses this to one spec.

---

## 2. Architecture Targets (sealed contract)

Quoting the requirements doc, target end-state:

1. `QuestionFamily` — classification only.
2. `AnswerSemanticView` — single runtime semantic contract.
3. `AnswerDocumentV2` — single carrier.
4. `emit_answer_document` / `emit_answer_document_patch` — single
   mutation surface, two submission shapes.
5. `contract_check_block` + `semantic_quality_reviewer` — jointly
   guarantee correctness AND completeness AND richness.
6. RepairExecutionPlan — cluster-state closure progresses owners.

No goal is descoped. v3 closes the remaining gaps to that target.

---

## 3. R3 Signal Source Rule (recap)

Hard / soft gate inputs are admitted iff one of:
- (a) **runtime current-dispatch typed signal** (typed enum / integer / boolean
      / verbatim substring) — encouraged, the more precise the better.
- (b) **abstract-generalised typed dictionary or constant** (universal
      software-engineering vocabulary, holds across any repo, contains no
      eval-case verbatim identifier).

Forbidden as gate inputs OR as prompt content:
- (c) **eval-fitted static signal** — content fitted to a specific eval
      test answer without abstraction (verbatim symbol/file path; ZH/EN cue
      tables; project-specific config keys; hardcoded stopword lists for one
      query).

Also: typed cluster-state, fingerprints, owner-stable counters etc. computed
at runtime from current-dispatch state are runtime typed signals — fully
admitted.

---

## 4. B0 — ViolKindRegistry (prep PR, blocks all of B1-B6)

### Why
Adding a new ViolKind today requires editing:
1. `internal/types/violation.go` constant
2. `internal/types/violation.go` AllViolationKinds slice
3. `internal/types/retry_state.go` DeriveSeverity switch
4. `internal/orchestrator/contract_check.go` defaultSoftKinds map
5. `internal/orchestrator/fallback_policy.go` DefaultFallbackPolicy map
6. `internal/orchestrator/retry_state.go` inferViolationLayer switch

Six sites, each easy to forget. Collapsing into one declaration is the
single largest maintenance win in v3.

### Design

New file `internal/types/violation_registry.go`:

```go
type ViolKindSpec struct {
    Kind            ViolationKind
    DefaultSeverity Severity     // → ViolationProfileFor
    SoftByDefault   bool         // → defaultSoftKinds map
    Promotable      bool         // false ⇒ permanently SOFT (e.g. label inference)
    FallbackLocus   RepairLocus  // → DefaultFallbackPolicy map (locus, then targetForLocus)
    Layer           string       // → inferViolationLayer
    Description     string       // operator-facing, NEVER LLM-facing
}

func RegisterViolKind(spec ViolKindSpec)
func ViolKindSpecFor(kind ViolationKind) (ViolKindSpec, bool)
func AllViolKindSpecs() []ViolKindSpec
```

The current 47 ViolKinds are migrated wholesale into spec form. Five
existing tables become **derived**:

- `DeriveSeverity` → registry-first, retains the legacy switch as fallback
  for the temporary migration window only (until B-series PRs land and we
  can prove byte-identical derivation).
- `defaultSoftKinds()` → returns map built from `spec.SoftByDefault`.
- `inferViolationLayer` → returns spec.Layer, fallback "contract_check".
- `DefaultFallbackPolicy()` → returns map built from
  `targetForLocus(spec.FallbackLocus)`.
- A new `permanentlySoftKinds` derived from `spec.Promotable=false` is
  consumed by `SetSoftViolationKinds` to refuse operator promotion of a
  permanently-soft kind.

### Test gates
- `TestRegistryDerivesAllLegacyTables` — for each kind in
  `AllViolationKinds()`, `(Severity, IsSoft, FallbackTarget, Layer)` from
  the registry must equal the legacy table value byte-identically.
- `TestEveryViolKindHasSpec` — `AllViolationKinds()` and
  `AllViolKindSpecs()` cover the same set.

### Compatibility
For the migration window the legacy switch in `DeriveSeverity` and the
hardcoded `DefaultFallbackPolicy` map LITERAL still exist; they are wired
to read from the registry. Deletion of the literal happens in a follow-up
PR after B1-B6 ship and no caller is observed depending on the legacy
form.

---

## 5. B1 — RepairExecutionPlan cluster closure

### Reduces iterations by
Today's `shouldRebuildExecutionPlan` rebuilds the plan whenever the fresh
violation kind set is not a strict subset of prev's kind set. In multi-
facet scenarios where one of two equally-Finalizer-owned facets resolves,
the residual still has the same kind (`ViolFacetUncovered`); rebuild fires
unnecessarily and re-clusters everything. The new design observes "X
fingerprint cleared, Y still here" and stays on the same Finalizer owner
to fix Y — saving 1-2 retry rounds.

### Design

Cluster identity is `(PrimaryKind, PrimaryFingerprint)`. Fingerprint is
extracted from `violation.Detail` via existing helpers
(`extractBlockIDFromDetail`, plus a new helper that lifts the facet-kind
substring from `validateFacetCoverage` Detail format
`"required facet \"<kind>\" (...)"`). Falls back to `Detail[:80]` hash —
all runtime current-dispatch typed substrings, no static dictionary.

`RepairExecutionPlan` adds:
```go
ClusterStates []RepairClusterExecutionState  // 1:1 with prev.Clusters

type RepairClusterExecutionState struct {
    Owner              RepairLocus
    PrimaryKind        ViolationKind
    PrimaryFingerprint string
    DerivedKinds       []ViolationKind
    PrimaryResolved    bool
    DerivedResolved    bool
    StableAttempts     int
}
```

New file `internal/orchestrator/repair_cluster_closure.go`:

- `computeClusterClosure(prev RepairExecutionPlan, fresh []Violation) []RepairClusterExecutionState`
- `clusterFingerprintOf(v Violation) string`
- `currentOwnerClustersAllClosed(plan) bool` — checks every cluster whose
  `Owner == plan.CurrentOwner`. Promote requires ALL such clusters closed.
- `currentOwnerStuck(plan, budget) bool`

Rewritten `shouldRebuildExecutionPlan`:

| Condition | Action |
|-----------|--------|
| prev nil / IsEmpty / HasFailLoud | rebuild |
| any (kind, fingerprint) in fresh missing from prev | rebuild |
| any current-owner cluster: PrimaryResolved AND !DerivedResolved | rebuild (cooccurrence theory broken) |
| all current-owner clusters: PrimaryResolved AND DerivedResolved | promote next |
| any current-owner cluster: StableAttempts ≥ budget AND !PrimaryResolved | promote next (or FailLoud if RemainingOwners empty + EscalationAllowed) |
| else | same owner, stable++ |

`AdvanceRepairExecutionPlan` calls `computeClusterClosure` first, then
the rewritten `shouldRebuildExecutionPlan`, then `BuildRepairExecutionPlan`
or `PromoteNextOwner` accordingly. `PromoteNextOwner` carries forward
`ClusterStates` (unaffected clusters retain their state).

### Telemetry
`SummarizeRepairExecutionPlan` adds:
```
current=<locus> idx=N/M closed=<bool> stuck=<bool> stable=<int>
clusters=[(kind=A primary_resolved=true derived_resolved=false), ...]
```

### Configuration
New `pipeline_cluster_stable_budget` (default 2) wired in
`internal/config/runtime.go` + `cmd/root.go`.

### Tests
- `TestComputeClusterClosure_PrimaryFingerprintDistinguishesSameKindClusters`
- `TestShouldRebuildExecutionPlan_PrimaryResolvedDerivedPersists_TriggersRebuild`
- `TestShouldRebuildExecutionPlan_AllOwnerClustersClosed_PromotesNext`
- `TestShouldRebuildExecutionPlan_OneOfTwoFinalizerClustersClosed_StaysOnFinalizer`
- `TestShouldRebuildExecutionPlan_StuckOwnerWithoutProgress_PromotesNext`

### Real-eval gate
s1a / m1a single case: PASS not regress; round count unchanged or lower.

---

## 6. B2 — Three-layer quality contract (D)

### Layers

| Layer | Existing | New |
|-------|----------|-----|
| 1: hard correctness | `contract_check_block` HARD validators | — |
| 2: required completeness | `validateFacetCoverage` (promoted facets) | — |
| 3: evidence-backed richness | `validateRichnessRegression` (telemetry-only) | `validateRichnessGlaringGap` (Severity=Medium → retry) + `validatePrincipalProseUnderfilled` (per-block-kind typed signal) + reviewer reverse-check |

### Layer 3a — `validateRichnessGlaringGap`

`FacetRequirement` adds:
```go
MinEvidenceForGlaring int  // 0 ⇒ familyGlaringEvidenceThreshold(family)
```

`familyGlaringEvidenceThreshold` (in `answer_semantic_view_compile_helpers.go`):
- QFArchitecture, QFRootCauseTrace → 3
- QFCallChain, QFConfigPrecedence → 2
- default → 4

These thresholds are family-agnostic principles (high-richness families
need more evidence to meaningfully demand richness; low-richness families
default conservative). No eval-case verbatim content.

Per-family glaring facet dispatch (touched in 8 `compile_<family>.go` files):

| Family | Optional facets marked `Strength=glaring` |
|--------|------------------------------------------|
| QFArchitecture | FacetComponentRelation, FacetNearestMechanism |
| QFRootCauseTrace | FacetBranchGuard, FacetNearestMechanism |
| QFCallChain | FacetBranchGuard, FacetPrincipalPathEdge |
| QFConfigPrecedence | FacetConfigPrecedenceRole |
| QFEnumeration | (none — Required already controls completeness) |
| QFRoleLookup | (none — anchor primacy) |
| QFComparison | FacetComponentRelation |
| QFGeneric | (none) |

Each family-level decision is universal across repos. Adding new families
extends the dispatch.

Validator triggers when ALL hold:
1. `req.Tier == TierEnrichment`
2. `len(req.SourceCandidate) >= effectiveThreshold`
3. covered (block.FacetIDs / item.ClaimUse.FacetID) == false
4. `req.Strength == EnrichmentGlaring`

`ViolRichnessGlaringGap` registry spec:
```
Severity=Medium, SoftByDefault=false (retry-eligible), Promotable=true,
FallbackLocus=LocusFinalizer, Layer="v2_oracle"
```

When fired, suppresses `ViolRichnessRegression` on the SAME facet to avoid
duplicate notice.

### Layer 3b — `validatePrincipalProseUnderfilled`

Per-block-kind typed combination (no length thresholds):

```
for block where SurfaceRole == SurfacePrincipal:
    switch block.Kind {
    case BlockSummary, BlockSection, BlockCaveat:
        inlineCode = countInlineCode(block.Text)
        claimCount = len(block.ClaimUses)
    case BlockOrderedList, BlockBulletList:
        inlineCode = sum(countInlineCode(item.Text) for item in items)
        claimCount = count(item.ClaimUse != nil for item in items)
    default:
        skip
    }
    if claimCount >= 3 AND inlineCode == 0:
        fire ViolPrincipalProseUnderfilled
```

`countInlineCode` uses regex `` `[^`\n]+` `` — a verbatim Markdown structural
signal, universal across languages. No keyword tables.

Registry spec same shape as 3a: Medium / not soft / Promotable=true /
LocusFinalizer / Layer="v2_oracle".

### Reviewer reverse-check

`SemanticQualityInput` adds typed signals:
```go
type SystemDetectedGap struct {
    Kind          string    // "richness_glaring" | "prose_underfilled"
    FacetKind     string    // for glaring
    BlockID       string    // for underfilled
    EvidenceCount int
}
SystemDetectedGaps []SystemDetectedGap

type FacetCoverageDepth struct {
    Kind          string
    DeclaredCount int
    AnchoredCount int
}
PromotedFacetCoverage []FacetCoverageDepth
```

Reviewer system prompt rewrites — reviewer becomes a reverse-check role:
- For each SYSTEM-DETECTED GAP, judge whether the BODY actually addresses
  it via equivalent expression. If yes → mark sufficient=true even when
  typed flagged.
- Add ADDITIONAL coverage gaps the typed validators missed; do NOT restate
  system-detected gaps.

### Iteration storm protection
SOFT/Medium clarification: new kinds are Severity=Medium →
RetryEligible=true → enter retry. Retry storm bounded by existing
OwnerStableAttempts budget; a current-owner cluster that fires the same
kind N times without progress is promoted/escalated by B1 closure logic.

### Tests
- `TestValidateRichnessGlaringGap_FiresOnGlaringWithEnoughEvidence`
- `TestValidateRichnessGlaringGap_BelowThresholdSuppressed`
- `TestValidateRichnessGlaringGap_SuppressesRichnessRegression`
- `TestValidatePrincipalProseUnderfilled_OrderedListChecksItemsAggregated`
- `TestValidatePrincipalProseUnderfilled_SummaryChecksBlockText`
- `TestValidatePrincipalProseUnderfilled_ScalarSkipped`
- `TestSemanticReviewer_SystemDetectedGapsInjected`
- `TestCompileArchitecture_MarksGlaringFacets`
- `TestFamilyGlaringEvidenceThreshold_DefaultsByFamily`

### Real-eval gate
s1a / m1a / qf_architecture: round count ≤ baseline+1 AND PASS not regress.

---

## 7. B3 — Diagram relation typed-first + SST

### Trigger
`validateDiagramRelationLegality` separates typed and label counts:

```go
typedRelCounts := map[DiagramRelationKind]int{}
labelRelCounts := map[DiagramRelationKind]int{}

for each edge:
    if typed RelationKind exists for (from,to):
        typedRelCounts[kind]++
        if labelInferred != typed: emit ViolDiagramEdgeLabelMismatch (existing SOFT permanently)
    else:
        labelRelCounts[labelInferred]++

for each EdgeRelations contract (kind, min):
    if typedRelCounts[kind] >= min:
        // pass clean
    elif typedRelCounts[kind] + labelRelCounts[kind] >= min:
        // pass + emit ViolDiagramRelationLabelOnly (new SOFT, Promotable)
    else:
        // existing HARD ViolDiagramEdgeUnsupported
```

When the contract fails, Detail mentions edges that were `typed-occupied`
by a different RelationKind so the LLM understands why labelling those
edges with the contract's keyword didn't help.

### New ViolKind
`ViolDiagramRelationLabelOnly` registry spec:
```
Severity=Soft, SoftByDefault=true, Promotable=true,
FallbackLocus=LocusFinalizer, Layer="v2_oracle"
```

### Skill prompt SST
New file `internal/skill/diagram_relation_doc.go`:
```go
func BuildDiagramRelationContractDoc() string
```
Renders a unified LLM-facing markdown section: typed-first prose +
relation_kind primary surface + label vocabulary as compatibility
fallback. All content derived from `types.AllDiagramRelationKinds()`,
`types.DiagramRelationKeywords()`, `types.ClaimFormForRelation()`.

`internal/skill/defaults.go` lines 213-228 are replaced by
`BuildDiagramRelationContractDoc()`. Adding a relation kind / keyword /
claim_form mapping → one source-of-truth edit, prompt regenerates.

### Tests
- `TestValidateDiagramRelationLegality_TypedSatisfies_NoLabelOnlyViolation`
- `TestValidateDiagramRelationLegality_LabelOnlySatisfies_FiresLabelOnly`
- `TestValidateDiagramRelationLegality_NeitherSatisfies_FiresUnsupported`
- `TestValidateDiagramEdgeUnsupported_DetailNamesTypedOccupiedEdges`
- `TestBuildDiagramRelationContractDoc_DerivedFromTypedTables`

### Real-eval gate
qf_architecture single case: PASS not regress.

---

## 8. B4 — V2 mutation single protocol

### Pre-step (mandatory before code)
Full-repo grep:
```
grep -rn "SetAnswerDocumentV2\b\|SetAnswerDocumentV2FromPatch\b\|LastEmitFromPatch\b" \
  internal/ cmd/
```
Produce an impact-table commit before refactoring. If callsite count
exceeds 30, revisit deletion strategy (currently planned `a` path —
delete legacy setters outright).

### New files
- `internal/tool/answer_document_block_schema.go`:
  - `BuildAnswerBlockFieldsSchema() json.RawMessage` — single-block
    properties (id/kind/title/text/items/diagram/claim_uses/edge_anchors/
    facet_ids/surface_role).
  - `BuildFullEmitParametersSchema() json.RawMessage` — top-level full
    emit parameters (`document_model`, `blocks`, `citations`,
    `exact_resolution`, `caveats`, `snippets`).
  - `BuildPatchEmitParametersSchema() json.RawMessage` — patch parameters
    (`unchanged_block_ids`, `replace_blocks`, `add_blocks`,
    `remove_block_ids`, `replace_*`). Each block-array entry inlines
    `BuildAnswerBlockFieldsSchema` plus an envelope description naming
    the id-must-exist / id-must-not-exist rule.
  - `BuildAnswerDocumentSemanticContractDescription() string` — the
    block semantic contract (kind table, claim_use rules, citation pool
    rules) shared by both Description() outputs.

- `internal/tool/answer_document_mutation_runtime.go`:
  - `ApplyAndPersistMutation(ctx, toolName, mutation, prev, now) (ToolResult, error)`
    — single write closure: `mutation.Apply(prev)` → merged-doc
    validation (unique block id, diagram payload, max blocks) →
    `ctx.Mutable.SetAnswerDocumentV2WithMutation(merged, mutation.Kind)`
    → ToolResult Summary built from `mutation.Summary()`.

### Setter unification
`internal/types/context.go`:
- New `SetAnswerDocumentV2WithMutation(doc *AnswerDocumentV2, kind MutationKind)`.
- DELETE `SetAnswerDocumentV2` and `SetAnswerDocumentV2FromPatch`.
- All callsites migrate.
- `LastEmitFromPatch` field retained on MutableState (RetryState read
  surface) but updated only by the new setter.

### Wiring
- `internal/tool/emit_answer_document.go` Description routes through
  `BuildAnswerDocumentSemanticContractDescription`. Parameters routes
  through `BuildFullEmitParametersSchema`.
- `internal/tool/emit_answer_document_v2.go::executeAnswerDocumentV2`
  collapses to: parse → build typed blocks via
  `NormalizeEmitAnswerBlock` → `mutation := NewReplaceAllMutation(doc)` →
  `ApplyAndPersistMutation`.
- `internal/tool/emit_answer_document_patch.go::Execute` collapses to:
  parse → build patch → `mutation := NewPartialMutation(patch)` →
  `ApplyAndPersistMutation(prev=...)`.
- All operator-facing log lines and ToolResult Summaries use
  `mutation.Summary()`.

### Tests
- `TestSetAnswerDocumentV2WithMutation_PartialKindUpdatesLastEmitFromPatchTrue`
- `TestApplyAndPersistMutation_FullAndPatchByteIdenticalMergedDoc`
- `TestBlockSchemaSST_FullAndPatchUseSameBlockProperties`
- `TestNoMutationVerbiageInLLMFacing` (glossary lint)
- All existing emit/patch tests retained for behaviour parity.

### Real-eval gate
s1a / m1a / qf_architecture / u3a: PASS not regress.

---

## 9. B5 — Prompt jargon cleanup (整批彻底重构)

Per user decision: refactor entire surface in one pass; observe afterward.

### E1 — `backbone` / `spine` family
- `internal/agent/answer_document_evaluator.go:483` "Resolved Step Sequence"
  prose body — replace step-backbone language with neutral "ordered anchor
  sequence" description.
- `internal/agent/answer_document_evaluator.go:618` `FacetDiagramSpine`
  label — "Diagram facet (every node grounded in a citation; relationships
  supported by typed claim_use)".
- `internal/agent/extractor.go:1451` retry hint "compiled anchor backbone"
  → "the resolved anchor list when available".
- `internal/agent/explorer.go:3931` retry hint "backbone batch" → "the
  failure path" / "the principal anchor list".
- `internal/agent/explorer.go:4895` retry hint "keep the failure backbone
  grounded first" → similar generic surface.

### E2 — `deterministic pipeline / renderer / compiler / alignment` family
- `internal/skill/defaults.go:116` Goal "A deterministic renderer turns
  the structure into user-visible prose." → "The structured emit IS the
  delivery; rendering is automatic."
- `internal/skill/defaults.go:150` OutputFormat — same replacement.
- `internal/skill/defaults.go:259` BlockScalar prose — drop reference to
  "renderer" and "MEANING".
- `internal/skill/defaults.go:290` Mermaid — "deterministic alignment" →
  "consistent layout".
- `internal/skill/analysis_contract.go:127, 142, 153, 338, 344` — "the
  deterministic pipeline / fallback / compiler" → "the system's
  downstream inference" / "an automatic fallback".

### E3 — Glossary blocklist
`internal/skill/glossary.go::InternalTermsBlocklist` adds:
`"backbone"`, `"spine"`, `"deterministic pipeline"`,
`"deterministic renderer"`, `"deterministic alignment"`,
`"deterministic compiler"`.

Run lint tests:
- `TestNoInternalTermsInPrompts`
- `TestReviewerPrompts_LLMFacingNoInternalJargon`
- `TestNoInternalTermsInToolSchemas`

`FacetDiagramSpine` enum string literal `"diagram_spine"` is a protocol
field (LLM emits this as `facet_id`), not jargon — kept.

### Real-eval gate
After full E1+E2+E3 ship, cross-case eval:
- s1a / m1a / u3a / qf_architecture / qf_config_precedence — round count
  not regress; PASS not regress.

---

## 10. B6 — Documentation purge

`docs/architecture.md`:
- L718 `#### Summary 长度：shape-tiered cap` → `block-kind-tiered cap`;
  table header `| shape |` → `| block kind |`.
- L1690-1708 isMeasurementScalar paragraph rewritten using
  `BlockScalar + AnswerSemanticView.NeedsPrincipalScalar()` mental model;
  remove `RequiredAnswerShape` / `AnalyzerHints.Shape` references.
- L1924 `shape-aware, 多语言` → `block-kind-aware, 多语言`.
- L1966 `summary_cap_*` per-shape cap → per-block-kind cap.
- §3 add new sub-section "V2 写入主链 (单一收口)":
  ```
  - 用户语义合同：AnswerSemanticView
  - 唯一载体：AnswerDocumentV2
  - 唯一写入协议：AnswerDocumentMutation (kind: replace_all | partial)
  - 校验主链：contract_check_block + semantic_quality_reviewer
  - 修复编排：RepairExecutionPlan (cluster-state closure)
  - ViolKind 注册：ViolKindRegistry (单源真理)
  ```

`docs/migration/*.md` × 17 files:
- Each gets a header line:
  `> Status: archived (2026-05-04). Current architecture lives in docs/architecture.md.`

Code comments:
- `internal/orchestrator/contract_check.go:141` — drop V1 reference.
- `internal/orchestrator/contract_check_block.go:950` — drop V1 history,
  describe as "V2 facet coverage validator".
- `internal/types/answer_document.go:7` — generic description.
- `internal/tool/emit_answer_document.go:19` — collapse V1 history block
  to one-line reference.
- `internal/tool/emit_answer_document_v2.go:435+` — V1 carrier residue
  comment kept (provenance for misplaced-hint table entries).

### Gate
`grep -rn "shape-aware\|shape-tiered cap" docs/architecture.md` → 0 results.

---

## 11. Construction DAG

```
B0  (registry prep)               # serial, prerequisite
 │
 ├──► B1 (cluster closure)
 ├──► B2 (three-layer quality)
 └──► B3 (diagram typed-first)    # B1+B2+B3 parallel — first wave
 │
 ├──► B4 (mutation single protocol)
 └──► B5 (prompt jargon cleanup)  # B4+B5 parallel — second wave
 │
 └──► B6 (docs purge)             # third wave
```

Each PR's ship gate:

| PR | Unit/integration test | Real eval |
|----|----------------------|-----------|
| B0 | `TestRegistryDerivesAllLegacyTables` byte-identical | n/a |
| B1 | 5 closure tests + orchestrator integration | s1a / m1a (no regress) |
| B2 | 9 validator/reviewer/compile tests | s1a / m1a / qf_architecture (rounds ≤ baseline+1) |
| B3 | 5 diagram tests | qf_architecture |
| B4 | mutation/setter/schema tests + glossary lint | s1a / m1a / qf_architecture / u3a |
| B5 | 3 lint tests pass | s1a / m1a / u3a / qf_architecture / qf_config_precedence (rounds, PASS) |
| B6 | grep `shape-aware\|shape-tiered cap` returns 0 | n/a |

---

## 12. Audit checklist (every commit)

- [ ] R3 signal source: runtime typed OR abstract-generalised typed; no
      eval-fitted verbatim
- [ ] R4 LLM-facing jargon: blocklist clean; lint passes
- [ ] R5 no system backfill: system never writes user-panel answer text
- [ ] R6 examples: abstract placeholders only; cross-case applicability
- [ ] R7 deletion three-step: grep canonical / consumer audit / 1 case real
- [ ] SST: dictionaries / headings via constant or function; no copy-paste
- [ ] Registry sync: new ViolKind → 1 spec entry only (post-B0)
- [ ] L1: read mode byte-identical preserved
- [ ] eval FAIL never relaxes case spec; system fix only
