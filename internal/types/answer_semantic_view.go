package types

// AnswerSemanticView is the typed contract that describes WHAT an
// answer must cover and HOW its block-only carrier should be shaped.
// It is the bridge between (a) the upstream FacetCoverageContract +
// AnswerSurfacePlan + ExactResolution + DiagramHint signals and
// (b) the downstream finalizer prompt + V2 validator + V2 renderer.
//
// The whole point of introducing this struct is to STOP the finalizer
// from deciding "what shape should the answer be" by re-reading the
// shape enum each time, and INSTEAD give every consumer (finalizer
// prompt, V2 validator, V2 renderer, V2 reviewer) a single typed
// view of the block-level obligations.
//
// Per docs/migration/block_only_carrier.md §5.1-5.2 the view is
// compiled at AgentContext build time from the analyzer's output;
// no system code mutates it after compile (read-only contract).
//
// Per the precise-signals-for-hard-gates red line, all V2 oracle
// code reads ONLY this view's typed fields — never the question
// prose, never the analyzer's free-form notes.
type AnswerSemanticView struct {
	// Family is the deterministic question-shape grouping the
	// analyzer's emit_analysis output places this question into.
	// Resolved by ResolveQuestionFamily (internal/types/facet_plan.go).
	// Drives WHICH compile_<family>.go produced this view.
	Family QuestionFamily

	// FacetCoverage is a pointer alias to the upstream contract so
	// V2 validators / renderer / reviewer can reach it without re-
	// compiling. Nil only when the family is QFGeneric and no facet
	// requirements apply.
	FacetCoverage *FacetCoverageContract

	// RequiredBlocks lists the block-level obligations the LLM MUST
	// satisfy when emitting AnswerDocumentV2. Empty MaxCount means
	// "no upper limit" — never "exactly zero" (use Required=false +
	// MinCount=0 for that). Order is the canonical rendering order
	// (renderer respects unless block-level priority overrides).
	RequiredBlocks []BlockRequirement

	// OptionalBlocks lists block-level enrichments the LLM MAY emit
	// to raise answer richness. Failing to emit an optional block
	// never raises a violation; the RichnessTelemetryOracle records
	// missed optional blocks for cross-Run learning.
	OptionalBlocks []BlockRequirement

	// DiagramPlan, when non-nil, captures the diagram contract —
	// whether a diagram is required, what kind (flow / sequence /
	// architecture / call_dag), and which facets must be expressed
	// as nodes vs edges. Nil for question shapes where a diagram
	// would not aid comprehension.
	DiagramPlan *DiagramFacetGraph

	// Presentation is the family-independent display contract. It widens
	// the schema surface for blocks that are presentational carriers
	// (tables, scalars, decisions, and user-requested diagrams) without
	// forcing those carriers into the family prompt as required content.
	//
	// This is deliberately typed: it is compiled from analyzer/contract
	// lanes such as DiagramContract and deterministic display affordances,
	// never by keyword-scanning the user's prose or the model's answer.
	Presentation AnswerPresentationContract

	// ExactResolution is a pointer alias to the answer contract's
	// ExactResolutionContract when the question demands the LLM
	// explicitly state status=resolved/absent/unknown. Nil for
	// question shapes that don't carry that contract.
	ExactResolution *ExactResolutionContract

	// CurrentStatusDiagnostic is a pointer alias to the answer
	// contract's current-status diagnostic obligation. Validators use
	// this typed field, rather than prompt prose, to enforce that
	// diagnostic follow-up answers emit one bounded verdict token.
	CurrentStatusDiagnostic *CurrentStatusDiagnosticContract

	// RequiredCandidateRoles carries analyzer-emitted positive role
	// bindings for principal answer rows. For example, a scalar exact-answer
	// question can require a `budget_cap` row rather than an adjacent
	// `attempt_counter` row. Validators compare these enum values against
	// AnswerDocumentV2 items[].candidate_role only; they do not infer roles
	// from user text or rendered prose.
	RequiredCandidateRoles []AnswerCandidateRole

	// RequiredMechanismAnchors carries exact user-mentioned code/tool/file
	// anchors that must remain visible for mechanism-style answers. It is
	// compiled from typed analyzer lanes (`mentioned_entities`,
	// `exact_targets`, and kind-bearing contract terms), then checked against
	// structured AnswerDocumentV2 fields such as item labels and diagram edge
	// anchors. Validators consume only those structured carriers.
	RequiredMechanismAnchors []AnswerRequiredAnchor

	// ErrorGranularityProfile requires a principal decision block to carry a
	// canonical failure-scope verdict enum. This is intentionally separate from
	// decision prose so validators and evals do not depend on language-specific
	// synonyms such as "per item" vs "item-level".
	ErrorGranularityProfile *ErrorGranularityProfile

	// MissingRequestedRoles carries the subset of user-requested
	// precedence roles that the current grounded config-precedence
	// surface still shows as absent for the exact target. This is a
	// typed answer obligation: when non-empty, the final answer must
	// disclose these missing layers explicitly instead of collapsing
	// them into vague placeholders (`N/A`, `not applicable`, etc.).
	//
	// Only config-precedence exact-absence families populate this.
	MissingRequestedRoles []AnswerMissingRequestedRole

	// SummaryMode is a pointer alias to the SurfacePlan's
	// AnswerSummarySurfaceMode so the V2 prompt builder can apply
	// the right summary stylistic mode without re-deriving it.
	SummaryMode AnswerSummarySurfaceMode

	// UncertaintyRules lists the block-level obligations that
	// trigger when a particular facet is observed as
	// drifted / log-source-divergent / external-only. Each rule
	// names the trigger facet, the expected block kind, and the
	// LLM-facing prose template the validator can echo when the
	// expected block is missing.
	UncertaintyRules []UncertaintyRule

	// RichnessCandidates lists optional facets the answer COULD
	// surface for richness. Populated by the family compile rules
	// based on FacetCoverageContract.Optional + question structure.
	// Read by RichnessTelemetryOracle (B4 / Phase 5 already shipped).
	RichnessCandidates []RichnessCandidate
}

// BlockRequirement is one obligation entry on AnswerSemanticView's
// RequiredBlocks / OptionalBlocks slice. It tells the V2 validator
// (a) which block kinds are acceptable, (b) how many of them are
// required, (c) which facets each block must cover, (d) which claim
// forms each block's payload may rest on, and (e) a human-readable
// rationale the prompt builder can render to the LLM.
//
// Key invariants:
//   - Kind is always set; "" is invalid.
//   - MaxCount == 0 means "no upper limit" — never "exactly zero".
//     For "must-not-emit" semantics use Required=false + MinCount=0.
//   - FacetIDs may be empty when no facet-coverage check is required
//     for this block (e.g. a generic Summary block).
//   - AcceptableClaimForms restricts which ClaimForm values payloads
//     within this block may declare; empty = no claim-form check.
//   - Rationale is LLM-facing prose, NEVER internal Go terminology
//     (R4 red line).
type BlockRequirement struct {
	Kind     AnswerBlockKind
	MinCount int
	MaxCount int // 0 means "no upper limit"
	Required bool

	// FacetIDs names the FacetCoverageContract.Required[i].Kind
	// values this block must cover. Cross-checked at V2 validator
	// time against the rendered block's FacetIDs slice.
	FacetIDs []string

	// AcceptableClaimForms enumerates the ClaimForm values payloads
	// within this block may declare via ClaimUse. Empty = unrestricted.
	AcceptableClaimForms []ClaimForm

	// Rationale is rendered into the finalizer prompt under the
	// `## Required Answer Blocks` section. LLM-facing language, no
	// internal jargon (R4).
	Rationale string

	// SurfaceRoleHint, when set to SurfacePrincipal, tells the LLM
	// this block is principal answer content. Empty means "not
	// principal" — supporting context, framing prose, and
	// diagram-only contributions all default to empty. Validators
	// use the hint to distinguish which blocks count as the
	// "principal payload" when checking claim use coverage.
	SurfaceRoleHint SurfaceRole
}

// DiagramFacetGraph captures the diagram-level obligations that
// AnswerSemanticView carries. The contract is more granular than
// "just emit a diagram" — it specifies which facets must be
// expressed as nodes (typically entity-shaped facets like
// FacetActor / FacetComponent) vs edges (typically relation-shaped
// facets like FacetCallEdge / FacetDataflow), and which TYPED
// relation kinds those edges must carry (Phase 3-C4).
//
// Validators read NodeFacets / EdgeFacets to enforce that
// (a) the diagram block exists when Required=true, (b) every NodeFacet
// is represented as a Mermaid node, (c) every EdgeFacet is
// represented as a Mermaid edge connecting the right pair of nodes.
//
// EdgeRelations adds a per-relation-kind expectation: for each
// contract entry (Kind, Min, ClaimForm), the validator confirms the
// diagram body has at least Min labelled edges resolving to Kind via
// InferRelationFromLabel — and (Phase 3-C5) that each such edge is
// supported by a claim_use whose ClaimForm matches and whose
// FromNode/ToNode anchor the edge endpoints. Min=0 entries make the
// relation expected-when-present rather than required.
type DiagramFacetGraph struct {
	Required      bool
	Kind          DiagramKind                   // flow / sequence / architecture / call_dag
	NodeFacets    []string                      // FacetIDs that must appear as diagram nodes
	EdgeFacets    []string                      // FacetIDs that must appear as diagram edges
	EdgeRelations []DiagramEdgeRelationContract // typed relations the family expects on edges
}

// DiagramEdgeRelationContract names a single (relation, min count,
// expected claim_form) tuple this diagram must satisfy. Compiled
// once by the family's compile_<family>.go and read by Phase 3-C5
// validateDiagramEdgeSupport. ClaimForm == ClaimUnknown means the
// relation is recognised but has no edge-level claim_form (e.g.
// DiagramRelContain whose support lives at block-level facets).
type DiagramEdgeRelationContract struct {
	Kind      DiagramRelationKind
	Min       int
	ClaimForm ClaimForm
}

// UncertaintyRule names a trigger condition + expected block + LLM-
// facing message. When the V2 validator detects the trigger (e.g.
// "log-source drift observed") and the expected BlockCaveat block
// is missing, it raises ViolUncertaintyBlockMissing with this rule's
// MissingMessage as the repair text.
type UncertaintyRule struct {
	// TriggerFacet is the FacetCoverageContract.Required[i].Kind
	// value whose presence triggers the uncertainty obligation.
	// Empty TriggerFacet rules fire on any facet — used for blanket
	// "always carry a caveat block on shape=value" rules.
	TriggerFacet string

	// ExpectedBlockKind is the block kind the answer must include
	// to satisfy the uncertainty obligation (typically BlockCaveat).
	ExpectedBlockKind AnswerBlockKind

	// MissingMessage is the LLM-facing repair prose echoed when the
	// expected block is missing. NEVER internal jargon (R4).
	MissingMessage string
}

// RichnessCandidate names an optional facet the V2 RichnessTelemetry
// oracle considers when computing answer richness coverage. Mirror
// of the Phase 5 RichnessTier metric (already shipped) — this
// struct just brings the candidate into the typed AnswerSemanticView
// surface so V2 validators don't have to re-derive it from the
// FacetCoverageContract at every dispatch.
type RichnessCandidate struct {
	Kind     AnswerBlockKind
	FacetID  string
	Optional bool
}

// AllAnswerBlockKindsForSemanticView is a convenience alias to
// AllAnswerBlockKinds() exposed under a name that documents the
// semantic-view-side use case (structural tests assert every
// declared block kind is in fact reachable by some compile_<family>
// path or otherwise an explicit no-op).
func AllAnswerBlockKindsForSemanticView() []AnswerBlockKind {
	return AllAnswerBlockKinds()
}
