package types

// AnswerDocumentV2 is the block-only carrier introduced by Phase 2 of
// the docs/migration/block_only_carrier.md plan (B3 落地). It
// REPLACES — at terminal-state B8 — the V1 `AnswerDocument` shape +
// summary/steps/symbols/value/boolean schema. During B3-B7 the two
// carriers coexist in MutableState; at B8 the V1 type is deleted.
//
// Design principles (per block_only_carrier.md §4.2 + §7):
//   - No top-level "shape" — the LLM declares its answer through the
//     blocks it emits + each block's Kind + each block's claim_uses.
//   - One answer can mix block kinds: a Summary + Scalar +
//     OrderedList + Diagram + Caveat all belong to the same document.
//   - RenderedClaimUse is a NATIVE annotation on blocks and items
//     (not a per-payload-type bolt-on like V1).
//   - Citations are still pooled at document level (LLM references
//     them by zero-based index from blocks/items).
//   - ExactResolution / Caveats / Snippets stay as document-level
//     fields because they are answer-level concepts, not block-level.
//
// Runtime invariant (post block-only consolidation, 2026-05-05):
// the LLM-facing emit_answer_document schema no longer exposes or
// requires `document_model`. The executor always routes to the V2
// block-only path, ignores any legacy caller-supplied
// `document_model` field, and persists AnswerDocumentV2 with
// DocumentModel hard-stamped to "v2". In other words:
//   - external callers emit blocks[] only
//   - persisted/internal V2 documents still carry DocumentModel="v2"
//     as a carrier marker
//
// V1 carrier is retired at B8-T3 (2026-05-03).
//
// Per the feedback_no_system_backfill_to_user_panel red line, the
// user-visible answer must not receive hidden system-authored facts
// that the LLM did not have an evidence lane for. The tool executor
// may apply deterministic, evidence-backed compatibility repairs
// before persisting the document (for example citation_ref rebinding
// or missing row materialisation from accepted structured evidence).
// Renderer / hedging / oracles READ the persisted struct; they do not
// invent new answer facts while displaying it.
type AnswerDocumentV2 struct {
	// DocumentModel marks the persisted carrier version. It is an
	// INTERNAL field on stored/typed V2 documents, not an external
	// requirement on emit_answer_document callers. The executor
	// stamps it to "v2" after decoding blocks[].
	DocumentModel string `json:"document_model"`

	// Blocks is the ordered sequence of answer blocks. Renderer
	// emits them in declared order. Empty Blocks is invalid (every
	// V2 doc carries at least one Summary block).
	Blocks []AnswerBlock `json:"blocks"`

	// Citations is the shared zero-based pool. Each block / item
	// references citations by integer index (or -1 for "no cite").
	// Reused as-is from V1 (B3 does NOT redefine the Citation type;
	// the shared struct stays in answer_document.go).
	Citations []Citation `json:"citations,omitempty"`

	// ExactResolution carries the exact-resolution contract result
	// (status=resolved/absent/unknown + anchor/context_mode). The
	// V1 type AnswerExactResolution is reused.
	ExactResolution *AnswerExactResolution `json:"exact_resolution,omitempty"`

	// MissingRequestedRoles carries any user-requested precedence
	// roles that the current exact-absence config-precedence
	// dispatch confirmed have NO grounded binding for the exact
	// target. This is a typed surface contract, not free prose:
	// the renderer materialises explicit "missing layer" sentences
	// from these entries so finalizers do not have to improvise
	// wording like "no CLI flag binds this key".
	//
	// Empty / omitted means either:
	//   - the question did not explicitly ask for named layers, or
	//   - every requested layer has grounded coverage, or
	//   - this dispatch is not a config-precedence exact-absence
	//     answer.
	//
	// `label`, when present, is the user-facing bucket name lifted
	// from QuestionStructure (for example `CLI` rather than the
	// abstract role name `override`). The validator keys on `role`;
	// the renderer prefers `label` for user-visible wording.
	MissingRequestedRoles []AnswerMissingRequestedRole `json:"missing_requested_roles,omitempty"`

	// Caveats is a list of free-form caveats not tied to a single
	// block. Per-block caveat content lives inside BlockCaveat
	// blocks; document-level Caveats are reserved for cross-block
	// scope notes.
	Caveats []string `json:"caveats,omitempty"`

	// Snippets carries optional CodeSnippet entries the renderer
	// shows alongside the answer. Reused from V1.
	Snippets []CodeSnippet `json:"snippets,omitempty"`

	// ReadOwnerAnchors is an internal, deterministic projection of read-mode
	// source-localization owner/evidence anchors that supported the final
	// answer. It is stamped by the tool runtime from TurnAArtifacts after a
	// structured answer emit succeeds; it is not part of the LLM-facing
	// emit_answer_document schema and must never be parsed from model prose.
	ReadOwnerAnchors []OwnerAnchorViewItem `json:"read_owner_anchors,omitempty"`
}

// AnswerMissingRequestedRole is a typed answer-level disclosure that
// a user-requested precedence layer is absent for the current exact
// target. It is intentionally narrow:
//   - used by config-precedence exact-absence answers
//   - keyed by abstract precedence role (default/config/runtime/override)
//   - optionally preserves the user-facing bucket label (`CLI`,
//     `codrax.yaml`, `env file`, ...) for deterministic rendering
//
// The renderer uses these entries to emit explicit missing-layer
// sentences; validators use `Role` to ensure the answer discloses
// every user-requested layer that remained unbound in the grounded
// evidence surface.
type AnswerMissingRequestedRole struct {
	Role  EvidenceDiagramRole `json:"role"`
	Label string              `json:"label,omitempty"`
}

// AnswerBlock is one block in AnswerDocumentV2.Blocks. The Kind +
// Title + body fields determine how the renderer formats it; the
// FacetIDs + ClaimUses + SurfaceRole annotations let validators /
// reviewers reason about WHAT semantic role the block plays without
// inferring from prose.
type AnswerBlock struct {
	// ID is a non-empty stable identifier the LLM assigns. Used
	// internally for cross-block reference and validator messages.
	// Empty ID is rejected by the V2 schema validator.
	ID string `json:"id"`

	// Kind is the block's structural kind. Must be a value in
	// AllAnswerBlockKinds(). Validators reject invalid kinds.
	Kind AnswerBlockKind `json:"kind"`

	// Title is rendered as a sub-heading for Section / Table /
	// Diagram / Caveat blocks. Optional for OrderedList /
	// BulletList / Summary / Scalar / Decision.
	Title string `json:"title,omitempty"`

	// Text is the block's prose body. Used by Summary / Section /
	// Caveat / Decision (rationale) / Scalar (free-form notes).
	// Markdown-flavoured per the renderer.
	Text string `json:"text,omitempty"`

	// ErrorGranularityVerdict is the canonical verdict enum for
	// decision blocks answering failure-scope questions: per-item
	// rejection, whole-batch failure, partial success, fail-fast, or
	// collect-errors. Validators consume this typed field instead of
	// inferring the verdict from decision prose.
	ErrorGranularityVerdict ErrorGranularityVerdict `json:"error_granularity_verdict,omitempty"`

	// CurrentStatusVerdict is the canonical verdict enum for decision
	// blocks answering diagnostic current-status questions: still
	// present, fixed, or not enough evidence. Validators consume this
	// typed field instead of inferring the verdict from decision prose.
	CurrentStatusVerdict CurrentStatusVerdict `json:"current_status_verdict,omitempty"`

	// ScopeDisclosure is the canonical typed channel that declares why
	// a principal answer is bounded by the active sub-repo set in a
	// multi-repo workspace. When the typed
	// InactiveScopeDisclosureObligation is required (see
	// internal/types/inactive_scope_disclosure.go), at least one block
	// in the rendered AnswerDocumentV2 must carry a non-Unknown value
	// here, OR the visible answer surface must name an inactive
	// RootRel token directly. Validators consume this typed field
	// instead of scanning decision/caveat prose for active/inactive
	// scope keywords.
	ScopeDisclosure ScopeDisclosureKind `json:"scope_disclosure,omitempty"`

	// Columns is the optional header row for structured table blocks.
	// It is never required: table blocks may still carry a complete
	// model-authored markdown table in Text. When Text is empty,
	// Columns + Items[].Cells gives the renderer a low-friction
	// multi-column carrier; Items[].Label/Text remains the legacy
	// two-column fallback.
	Columns []string `json:"columns,omitempty"`

	// Items is the collection for OrderedList / BulletList / Table.
	// For Table, block.Text is the canonical visible carrier when it
	// contains a markdown table authored by the model. When Text is
	// empty, Items may render as structured rows via Cells, or as the
	// legacy Label | Text fallback. Items can also carry citations
	// for rows described by block.Text.
	Items []AnswerBlockItem `json:"items,omitempty"`

	// Diagram is the block's diagram payload when Kind=BlockDiagram.
	// Nil for all other kinds.
	Diagram *AnswerDiagramBlock `json:"diagram,omitempty"`

	// ClaimUses are the rendered claim annotations for this block
	// (when the LLM wants to declare per-block claim shapes). Most
	// blocks carry their claim uses on Items[].ClaimUse instead;
	// block-level ClaimUses are for whole-block claim shapes that
	// don't decompose into items (e.g. a Summary block whose claim
	// shape applies to the entire prose).
	ClaimUses []RenderedClaimUse `json:"claim_uses,omitempty"`

	// FacetIDs lists the FacetCoverageContract.Required[i].Kind
	// values this block covers. Cross-checked at V2 validator time
	// against the AnswerSemanticView's required facets.
	FacetIDs []string `json:"facet_ids,omitempty"`

	// SurfaceRole declares whether this block is principal answer
	// content (SurfacePrincipal) or anything else. Empty means "not
	// principal" — validators that gate claim_use coverage and slate
	// counts only fire on principal blocks.
	SurfaceRole SurfaceRole `json:"surface_role,omitempty"`

	// EdgeAnchors carry typed (from_node, to_node, claim_form)
	// triples that anchor labelled diagram edges to typed
	// claim_form values. Phase 1-B source-fix (V2 runtime eval
	// followup, 2026-05-04): pre-fix, FromNode/ToNode were inner
	// fields of RenderedClaimUse, swelling claim_use to 6 inner
	// fields and inviting LLMs to mis-fill sibling fields like
	// citation_ref into the same nested object (u3a-1 forensic
	// 7-iter retry loop). Now claim_use is back to 4 fields and
	// edge anchors live as a separate top-level array. The LLM
	// sees TWO distinct shapes — claim_use (facet/claim
	// annotation) vs edge_anchors (diagram-edge typed pairs) —
	// and the two cannot be confused.
	//
	// Validators (Phase 3-C5 validateDiagramRelationLegality)
	// read this field to match labelled edges against typed
	// claim_form expectations.
	EdgeAnchors []DiagramEdgeAnchor `json:"edge_anchors,omitempty"`
}

// DiagramEdgeAnchor is a typed edge-anchor record. Each entry binds
// a (FromNode, ToNode) pair to (a) the typed claim_form the edge
// represents, and (b) — G3 (post_v2_runtime_gap_remediation,
// 2026-05-04) — an OPTIONAL typed RelationKind asserting the basic
// semantic relation directly. When RelationKind is set, the
// validator reads it as the authoritative relation (label
// vocabulary becomes a consistency check, not the primary source);
// when omitted, the validator falls back to the legacy
// InferRelationFromLabel path so existing answers stay valid.
//
// Lives on AnswerBlock.EdgeAnchors (Phase 1-B source-fix,
// 2026-05-04) — moved here from RenderedClaimUse fields so the
// claim_use schema does not balloon and invite sibling-field
// misplacement.
//
// Both FromNode and ToNode MUST be the verbatim node identifier
// strings as they appear in the diagram body; case-folded matching
// is downstream. ClaimForm names the typed expected claim shape
// (call_edge / guard_condition / import_edge / precedence_role /
// external_observation); ClaimUnknown means "no edge-level
// claim_form required" (e.g. containment relations). RelationKind
// is the optional typed enum (call / guard / import / precedence /
// contain / observe); empty / unknown means "let label inference
// decide".
type DiagramEdgeAnchor struct {
	FromNode     string              `json:"from_node"`
	ToNode       string              `json:"to_node"`
	RelationKind DiagramRelationKind `json:"relation_kind,omitempty"`
	ClaimForm    ClaimForm           `json:"claim_form,omitempty"`
}

// HasEdgeAnchor reports whether the anchor has a non-empty
// (FromNode, ToNode) pair. Both must be present for the anchor
// to be considered grounded.
func (e *DiagramEdgeAnchor) HasEdgeAnchor() bool {
	if e == nil {
		return false
	}
	return e.FromNode != "" && e.ToNode != ""
}

// HasTypedRelation reports whether the anchor declares a typed
// RelationKind — i.e. the LLM emitted an explicit relation rather
// than relying on label inference. Validators check this to decide
// whether to consult the typed enum (true) or fall back to
// InferRelationFromLabel (false). DiagramRelUnknown is the empty
// sentinel and counts as "not typed".
func (e *DiagramEdgeAnchor) HasTypedRelation() bool {
	if e == nil {
		return false
	}
	return e.RelationKind.IsValid()
}

// AnswerBlockItem is one item inside a list / table block. Claim
// annotations live on the parent block's ClaimUses (block-level only)
// — items carry the rendered surface (label / text / cells) and citation.
type AnswerBlockItem struct {
	// ID is optional; useful when a downstream block needs to
	// reference this specific item.
	ID string `json:"id,omitempty"`

	// Label is the item's primary visible text. For Table items it
	// is the row header / first-column value.
	Label string `json:"label,omitempty"`

	// Text is the item's body text (description / rationale / row
	// content). Markdown-flavoured.
	Text string `json:"text,omitempty"`

	// Cells is the optional multi-column table row payload. It is
	// rendered only for table blocks whose block.Text is empty; it
	// lets the model emit one array per row instead of hand-building
	// markdown table syntax. Label remains available as a row key /
	// first column, and Text remains a compatibility detail field.
	Cells []string `json:"cells,omitempty"`

	// CandidateRole is an optional typed category or scalar/literal role for
	// this visible row. It is used when the current request carries an
	// AnswerExclusionPolicy / AnswerRoleProfile or when the answer needs to
	// distinguish functions, tool names, import paths, budget caps, attempt
	// counters, etc. without validators inferring roles from prose.
	CandidateRole AnswerCandidateRole `json:"candidate_role,omitempty"`

	// CitationRef is a zero-based index into AnswerDocumentV2.
	// Citations, or -1 when no citation backs this item. Renderer
	// resolves the index to a (file, line) cite at render time.
	CitationRef int `json:"citation_ref,omitempty"`
}

// AnswerDiagramBlock is the payload for BlockDiagram blocks. It
// separates the diagram's structural metadata (Kind / Language) from
// its body so renderers / validators can introspect without parsing
// the body string.
type AnswerDiagramBlock struct {
	// Kind is the DiagramKind enum (flow / sequence / architecture /
	// call_dag). Drives renderer's diagram-format choice.
	Kind DiagramKind `json:"kind"`

	// Language is the diagram source language: "mermaid" (preferred,
	// renderer applies deterministic alignment) or "text" (ASCII art
	// fallback). Empty defaults to "mermaid".
	Language string `json:"language,omitempty"`

	// Body is the raw diagram source. For Mermaid: the part inside
	// the fenced code block (without the ```mermaid fences — the
	// renderer adds them). For text: pre-formatted ASCII art.
	Body string `json:"body"`
}

// AllowedDocumentModels enumerates the persisted/internal
// DocumentModel values the V2 runtime recognises on stored
// AnswerDocumentV2 instances. It does NOT describe the
// LLM-facing emit_answer_document schema, which is blocks[]-only
// and does not require callers to send document_model.
//
// Adding a new model (e.g. "v3" some day) requires updating this
// list AND the emit_answer_document tool's schema enum AND the
// Execute dispatch — see post_shape_retirement_consolidated_audit.md
// §8 Batch B3 for the four-layer-consistency rule.
func AllowedDocumentModels() []string {
	return []string{"v2"}
}

// IsV2Document reports whether the doc's DocumentModel field marks
// it as the V2 carrier. Used by the tool layer / validator /
// renderer / oracles to dispatch on carrier version. The check is
// EXACT — empty / unknown values do NOT count as V2.
func IsV2Document(doc *AnswerDocumentV2) bool {
	return doc != nil && doc.DocumentModel == "v2"
}

// V2 carrier runtime toggles. Live in types (leaf package) so any
// package — agent / orchestrator / render — can read them without
// import cycles. cmd/root.go writes them once at startup.

// B8-T7 (block_only_carrier.md §5.8, 2026-05-03): EmitV2Default /
// SetEmitV2Default / V1OracleStrictMode / SetV1OracleStrictMode
// gating knobs deleted. V2 is the only carrier; V1 schema is
// rejected at runtime since B8-T3.
