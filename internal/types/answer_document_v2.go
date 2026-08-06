package types

import "strings"

// AnswerDocumentJSONShapeFirstTeaching is the compact carrier decision shared
// by the finalizer prompt and the emit tool description. Keep this before the
// larger semantic catalog: visible list prose and block-level evidence
// annotations are sibling lanes, and confusing them can otherwise turn an enum
// such as "call_edge" into user-visible answer text.
const AnswerDocumentJSONShapeFirstTeaching = "JSON SHAPE FIRST: emit one object with native blocks[] and citations[] arrays. Visible list/table rows use blocks[i].items[j].text (plus optional label/cells/citation_ref); evidence annotations use blocks[i].claim_uses[] at block level. Never put claim_form/facet_id/evidence_id inside items[], and never quote an object or array as a JSON string."

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

	// ReadSourceLocalization is the full typed read-side source-localization
	// review behind ReadOwnerAnchors. It preserves observed-only / weak /
	// owner-supported localization status for audit and deterministic
	// supplements even when no owner anchor is strong enough to render as
	// proof. It is stamped from TurnAArtifacts, not emitted by the model.
	ReadSourceLocalization *SourceLocalizationReview `json:"read_source_localization,omitempty"`

	// ReadNavigationCoverage is the typed repo_map navigation coverage observed
	// during read-mode exploration. It is derived from AnalysisIR navigation
	// policy plus producer-published ToolResult observations. It is not emitted
	// by the model and must never be reconstructed from model prose.
	ReadNavigationCoverage *RepoMapNavigationCoverage `json:"read_navigation_coverage,omitempty"`

	// ReadLocalizerFollowup is the deterministic read-side follow-up request
	// derived from ReadSourceLocalization plus ReadNavigationCoverage. It tells
	// downstream schedulers/reports that owner localization or repo_map
	// navigation evidence is still needed; it is stamped by the runtime and
	// never parsed from model-authored text.
	ReadLocalizerFollowup *ReadLocalizerFollowup `json:"read_localizer_followup,omitempty"`

	// ReadReasoningGraph is the compact typed evidence-ledger header for this
	// accepted read-mode answer. It is projected from AnalysisIR, TurnA
	// artifacts, source-localization/navigation authorities, aggregate facts,
	// and the accepted answer document. It is internal runtime metadata, not an
	// LLM-facing emit_answer_document schema field.
	ReadReasoningGraph *AnswerReasoningGraphSummary `json:"read_reasoning_graph,omitempty"`

	// CurrentStatusVerdictDowngrade is the typed evidence downgrade for the
	// current-status decision verdict (SPR #72, RTC ledger §8.3). Stamped by
	// the tool runtime at persist time when the origin-lane observation
	// ledger carries zero current_source evidence for the run; the renderer
	// downgrades the verdict surface to a caveat form and the obligation
	// gates stop demanding/consuming the verdict. The block's own
	// CurrentStatusVerdict field is never modified — it is the audit
	// position. Not an LLM-facing emit_answer_document schema field.
	CurrentStatusVerdictDowngrade *CurrentStatusVerdictDowngrade `json:"current_status_verdict_downgrade,omitempty"`
}

// AnswerReasoningGraphSummary is the compact read-mode evidence-ledger header
// attached to accepted AnswerDocumentV2 artifacts. Full graph events live in
// the reasoninggraph projection layer; answer docs carry only stable refs and
// counts for audit/status/eval consumers.
type AnswerReasoningGraphSummary struct {
	GraphID          string   `json:"graph_id,omitempty"`
	EventCount       int      `json:"event_count,omitempty"`
	LastEventKind    string   `json:"last_event_kind,omitempty"`
	LastReasonCode   string   `json:"last_reason_code,omitempty"`
	EventRefs        []string `json:"event_refs,omitempty"`
	ReadEventCount   int      `json:"read_event_count,omitempty"`
	ToolEventCount   int      `json:"tool_event_count,omitempty"`
	RepairEventCount int      `json:"repair_event_count,omitempty"`
	LLMEventCount    int      `json:"llm_event_count,omitempty"`
	NodeCount        int      `json:"node_count,omitempty"`
	EvidenceRefCount int      `json:"evidence_ref_count,omitempty"`
	AnswerBlockCount int      `json:"answer_block_count,omitempty"`
	CitationCount    int      `json:"citation_count,omitempty"`
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

	// TraceCausalClaimCaliber is the model-authored causal strength of the
	// principal Trace summary. It is exposed only when the typed Trace causal
	// contract is active and is never inferred from or rendered over Text.
	TraceCausalClaimCaliber TraceCausalClaimCaliber `json:"trace_causal_claim_caliber,omitempty"`

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

	// SourceInventoryFamily is an optional exact typed partition key for a
	// principal source-inventory block. The finalizer copies it from the
	// row-local source_inventory family handoff when a block intentionally
	// carries only one construct family. Validators must never infer this key
	// from Title, Text, item prose, or the user's wording.
	SourceInventoryFamily string `json:"source_inventory_family,omitempty"`

	// Columns is the optional header row for structured table blocks.
	// It is never required: table blocks may still carry a complete
	// model-authored markdown table in Text. When Text is empty, either put
	// every visible value in Items[].Cells (and leave Label empty), or use
	// Label as the first visible value and Cells/Text for the remaining
	// values. In the latter form Columns may include the label header or omit
	// only that header, in which case the renderer adds a neutral one.
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

	// SystemGeneratedKind is an in-memory marker set only by deterministic
	// normalizers after the model has emitted an AnswerDocument. It is excluded
	// from JSON so the LLM cannot author or repair it. Pre-emit validators use
	// this marker instead of inferring system supplement identity from rendered
	// titles or localized prose.
	SystemGeneratedKind AnswerSystemGeneratedBlockKind `json:"-"`

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

	// RelationClaims are model-authored declarations that bind value
	// comparisons/additions in this block to exact typed relation authorities
	// accepted during investigation. The system validates and preserves this
	// metadata but never derives it from Text and never rewrites Text from it.
	RelationClaims []AnswerRelationClaim `json:"relation_claims,omitempty"`
}

type AnswerSystemGeneratedBlockKind string

const (
	AnswerSystemGeneratedBlockUnknown AnswerSystemGeneratedBlockKind = ""

	AnswerSystemGeneratedPrincipalEnumerationMissing AnswerSystemGeneratedBlockKind = "principal_enumeration_missing"
	AnswerSystemGeneratedPrincipalEnumerationRows    AnswerSystemGeneratedBlockKind = "principal_enumeration_rows"
	AnswerSystemGeneratedPrincipalEnumerationFields  AnswerSystemGeneratedBlockKind = "principal_enumeration_fields"
	AnswerSystemGeneratedPrincipalEnumerationNotes   AnswerSystemGeneratedBlockKind = "principal_enumeration_notes"
	AnswerSystemGeneratedPrincipalEnumerationSection AnswerSystemGeneratedBlockKind = "principal_enumeration_section"

	// AnswerSystemGeneratedNegativeSearchAuthority marks the deterministic
	// current-source no-match scope roster. It is deliberately separate from
	// principal-enumeration and runtime-trace supplements: the block only
	// bounds which typed query returned zero rows, and never authors a global
	// absence conclusion.
	AnswerSystemGeneratedNegativeSearchAuthority AnswerSystemGeneratedBlockKind = "negative_search_authority"

	// AnswerSystemGeneratedEvidenceSupplement marks deterministic source,
	// negative-proof, or other evidence rows appended from typed investigation
	// facts. These blocks may disclose facts and provenance, but never become
	// model-authored claim carriers or authorize rewriting model blocks.
	AnswerSystemGeneratedEvidenceSupplement AnswerSystemGeneratedBlockKind = "evidence_supplement"

	// AnswerSystemGeneratedEvidenceScope marks the deterministic uncertainty
	// boundary derived from the typed evidence-coverage contract. It is kept
	// separate from the model's caveats so ownership survives snapshot/recovery.
	AnswerSystemGeneratedEvidenceScope AnswerSystemGeneratedBlockKind = "evidence_scope"

	// AnswerSystemGeneratedRuntimeTrace marks blocks minted by the
	// deterministic runtime-trace report assembler. The field carrying this
	// value is json:"-", so a model can never self-assign the authority that
	// lets a block suppress another system block, participate in structural
	// report ordering, or feed the prose-scalar evidence lane.
	AnswerSystemGeneratedRuntimeTrace AnswerSystemGeneratedBlockKind = "runtime_trace"
)

func (k AnswerSystemGeneratedBlockKind) IsPrincipalEnumerationSupplement() bool {
	switch k {
	case AnswerSystemGeneratedPrincipalEnumerationMissing,
		AnswerSystemGeneratedPrincipalEnumerationRows,
		AnswerSystemGeneratedPrincipalEnumerationFields,
		AnswerSystemGeneratedPrincipalEnumerationNotes:
		return true
	default:
		return false
	}
}

// IsRuntimeTraceSupplement reports whether the block kind was minted by the
// deterministic runtime-trace report assembler. Callers must still validate
// the exact reserved block ID when the ID itself carries semantics.
func (k AnswerSystemGeneratedBlockKind) IsRuntimeTraceSupplement() bool {
	return k == AnswerSystemGeneratedRuntimeTrace
}

// CaptureSystemGeneratedBlockKinds snapshots the block-id →
// SystemGeneratedKind authority map of an IN-MEMORY document whose
// json:"-" markers are still live. It is the ONLY legitimate producer of
// the sidecar consumed by ReauthenticateSystemSnapshotBlockKinds: call it
// at the exact moment a system-side JSON snapshot of the document is
// taken (FRCAP draft-ledger DocJSON, RetryState.PrevEmitJSON), so the
// authority that json.Marshal is about to strip is preserved out-of-band.
//
// The map never crosses a JSON boundary (its carriers are json:"-" /
// in-memory struct fields), so a model can never author or repair it —
// the same unforgeability contract as AnswerBlock.SystemGeneratedKind
// itself. Only non-empty kinds on non-empty IDs are captured. Returns nil
// when the document carries no system-generated block (the common
// non-trace case pays nothing).
func CaptureSystemGeneratedBlockKinds(doc *AnswerDocumentV2) map[string]AnswerSystemGeneratedBlockKind {
	if doc == nil {
		return nil
	}
	var out map[string]AnswerSystemGeneratedBlockKind
	for i := range doc.Blocks {
		id := strings.TrimSpace(doc.Blocks[i].ID)
		kind := doc.Blocks[i].SystemGeneratedKind
		if id == "" || kind == AnswerSystemGeneratedBlockUnknown {
			continue
		}
		if out == nil {
			out = make(map[string]AnswerSystemGeneratedBlockKind)
		}
		out[id] = kind
	}
	return out
}

// ReauthenticateSystemSnapshotBlockKinds re-stamps the in-memory
// SystemGeneratedKind authority marker onto doc — the json.Unmarshal
// product of a SYSTEM-SIDE snapshot of an already-persisted document —
// from kinds, the sidecar CaptureSystemGeneratedBlockKinds produced from
// the SAME document at snapshot time. Returns how many blocks were
// re-stamped.
//
// Marker-stripping class root fix (audit 2026-07-10): SystemGeneratedKind
// is json:"-" (the model must never author authority), so every
// system-side marshal/unmarshal round trip — the FRCAP best-draft ledger
// DocJSON, RetryState.PrevEmitJSON consumed by the patch base / recovery
// draft / ParseOutput no-emit fallback — silently demoted genuine system
// blocks to model grade: `##` report chapters degraded, prose-scalar
// evidence feeds went dark (false ViolProseScalarUngrounded on
// system-published numerals), and reserved-ID collision normalization
// renamed real system blocks to model_runtime_trace_* (duplicate
// chapters). This helper restores exactly the authority that provably
// existed in memory when the snapshot was taken — nothing more.
//
// APPLICABILITY IS DELIBERATELY NARROW — MISUSE MINTS FORGED AUTHORITY:
//
//   - ONLY call on the unmarshal product of a snapshot this process took
//     from its own MutableState document (system-side provenance), with
//     the kinds map captured IN THE SAME MOMENT by
//     CaptureSystemGeneratedBlockKinds.
//   - NEVER call on model-direct JSON: emit_answer_document /
//     emit_answer_document_patch tool params, text-recovered drafts from
//     raw model output, or any payload that ever rode an LLM prompt or
//     response. Those lanes must keep the zero-value kind so
//     normalizeRuntimeTraceReservedBlockIDCollisions can rename reserved-
//     ID lookalikes (the forgery lane json:"-" exists to close).
//   - NEVER synthesize the kinds map (e.g. by reserved-ID spelling): a
//     blanket re-mark by ID would re-mint authority for model-authored
//     lookalikes that bypassed the persist choke.
//
// TestSystemSnapshotReauthCallSitesWhitelisted structurally pins the
// allowed caller set; extending it requires re-auditing provenance.
//
// Blocks whose ID is absent from kinds — or already carrying a non-empty
// kind — are left untouched, so the helper is idempotent and can never
// escalate a model-authored block.
func ReauthenticateSystemSnapshotBlockKinds(doc *AnswerDocumentV2, kinds map[string]AnswerSystemGeneratedBlockKind) int {
	if doc == nil || len(kinds) == 0 {
		return 0
	}
	restamped := 0
	for i := range doc.Blocks {
		if doc.Blocks[i].SystemGeneratedKind != AnswerSystemGeneratedBlockUnknown {
			continue
		}
		id := strings.TrimSpace(doc.Blocks[i].ID)
		if id == "" {
			continue
		}
		kind, ok := kinds[id]
		if !ok || kind == AnswerSystemGeneratedBlockUnknown {
			continue
		}
		doc.Blocks[i].SystemGeneratedKind = kind
		restamped++
	}
	return restamped
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
// is downstream. RelationKind is the current typed authority. The
// model-facing schema requires it and exposes call / guard / import /
// precedence / contain / type_relation / observe / register.
// ClaimForm remains on the wire only for persisted-document and old
// tool-call compatibility; current prompts and schemas do not ask the
// model to repeat this derived value. Empty / unknown RelationKind is
// accepted only by legacy runtime paths, which may recover from an old
// ClaimForm or fall back to label inference.
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

	// Cells is the optional multi-column table row payload. It is rendered
	// only for table blocks whose block.Text is empty. With an empty Label,
	// emit one cell per Columns entry. With a non-empty Label, Label is the
	// first visible value and Cells/Text carry the remaining values; Columns
	// may include or omit only the corresponding label header.
	Cells []string `json:"cells,omitempty"`

	// CandidateRole is an optional typed category or scalar/literal role for
	// this visible row. It is used when the current request carries an
	// AnswerExclusionPolicy / AnswerRoleProfile or when the answer needs to
	// distinguish functions, tool names, import paths, budget caps, attempt
	// counters, etc. without validators inferring roles from prose.
	CandidateRole AnswerCandidateRole `json:"candidate_role,omitempty"`

	// SourceInventoryRowID is an optional exact identity copied from a
	// Principal Enumeration Rows row_id. It lets the source-inventory
	// citation binder distinguish identical member labels across declaration
	// families or files without inspecting block titles or visible prose.
	SourceInventoryRowID string `json:"source_inventory_row_id,omitempty"`

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
