package types

import (
	"fmt"
	"strings"
)

// answer_document_v2_patch.go — R16 protocol-level retry preservation
// (post_shape_residual_audit.md, 2026-05-04). Replaces R14's
// prompt-level "Hard Rule: byte-identical preserve" with a
// structurally guaranteed delta protocol. LLM declares which
// blocks to keep / replace / add / remove; system applies the
// patch deterministically.
//
// Why R14 alone is insufficient (m1a r2 真证据 from R14_eval_deep_
// audit.md §3 类 J):
//   iter 0: 13 claim_use ✓ (correct emit)
//   iter 1: 18 claim_use (LLM re-quotes prev emit + new = double)
//   iter 4: 4 claim_use (LLM started dropping)
//   iter 7-10: 0 claim_use (full retry 失忆)
// Hard Rule rendered × 2 but LLM regenerated anyway. Prompt-level
// preservation has hard reliability ceiling for generative LLMs.
//
// R16 fix: the LLM **structurally cannot** drop a field it didn't
// touch. unchanged_block_ids list signals "keep this block from
// prev emit verbatim"; system copies that block over without LLM
// having a chance to rewrite it.

// AnswerDocumentV2Patch — Mutation Contract (canonical reference)
//
// AnswerDocumentV2Patch IS the unified mutation model over V2
// AnswerDocument. Every retry-path edit expresses through one of the
// operations below; there is no second carrier protocol.
//
// MUTATION CONTRACT TABLE
// =======================
//
// Op group         | Operations                       | Mutual exclusion
// -----------------|----------------------------------|---------------------
// Blocks           | Unchanged | Replace | Add | Remove | (1) Replace∩Remove ∅
//
//	|                                    | (2) Replace∩Add ∅
//	|                                    | (3) Add∩Remove ∅
//	|                                    | (4) Unchanged ⊆ prev.ids
//
// Block metadata   | BlockFieldEditsV1                | (6) FieldEdit∩Replace/Remove ∅
// Typed receipts   | BlockReceiptEditsV1              | (7) ReceiptEdit∩Replace/Remove ∅
//
// Citations        | Replace | Append                  | (5) Replace XOR Append
// ExactResolution  | Replace                           | nil = inherit prev
// MissingRequestedRoles | Replace                      | nil = inherit prev
// Caveats          | Replace                           | nil = inherit prev
// Snippets         | Replace                           | nil = inherit prev
//
// Op semantics:
//
//   - UnchangedBlockIDs: block ids from prev emit copied verbatim
//     (every annotation/display field — columns / claim_use /
//     facet_ids / surface_role / items including cells / diagram —
//     preserved byte-identical). The id MUST
//     exist in prev (invariant 4). LLM never re-emits fields on
//     these blocks; structural preservation against drop-on-retry.
//   - ReplaceBlocks: full block payloads override the prev emit's
//     block with the same id. id MUST exist in prev. id MUST NOT
//     also appear in RemoveBlockIDs (1) or AddBlocks (2).
//   - AddBlocks: full block payloads APPENDED at tail. id MUST NOT
//     already exist in prev. id MUST NOT also appear in
//     ReplaceBlocks (2) or RemoveBlockIDs (3).
//   - RemoveBlockIDs: block ids that MUST be absent from the new doc.
//     Removing an id already absent from prev is an idempotent no-op;
//     this lets a retry repeat an already-satisfied removal without
//     learning the rejected-draft base's incidental shape. id MUST NOT
//     also appear in ReplaceBlocks (1) or AddBlocks (3).
//   - BlockFieldEditsV1: exact, model-authored assignment of one closed-enum
//     metadata field on one existing model block. Every unmentioned field is
//     copied from the immutable previous block. The v1 whitelist excludes all
//     visible prose, diagram, relation, evidence, citation, item, and layout
//     carriers. A field edit cannot target a whole-block Replace/Remove in the
//     same patch (6); Unchanged is accepted as a redundant preservation claim.
//   - BlockReceiptEditsV1: exact, model-authored selection of one schema-
//     published typed receipt pair on one existing model block. Every other
//     block field is copied unchanged. The executor binds the selected id and
//     conclusion against the current dispatch contract; it never chooses one.
//   - Citations: ReplaceCitations and AppendCitations are mutually
//     exclusive (5); use Replace for holistic re-pick, Append for
//     additive (e.g. negative-pattern citation without rewriting
//     pool).
//   - ExactResolution / MissingRequestedRoles / Caveats /
//     Snippets: replace-or-inherit.
//
// Within the patch's op surface, block id is LLM-provided with ONE
// sanctioned system exception: the fused-diagram split (internal/tool
// splitFusedDiagramPatchBlocks / peekSplitDiagramBlockID, 2026-06-12)
// re-homes a diagram payload the model fused onto a rows-carrying
// block into a separate kind=diagram entry whose id is system-derived
// as `<source-id>_diagram`. (Post-merge materializers separately
// insert advisory blocks under reserved system ids — runtime trace
// facts, citation supplements, carrier blocks, scope caveats; see the
// maxBlocksPerDoc headroom invariant in
// internal/tool/answer_document_mutation_runtime.go. Those are all
// non-diagram kinds, so the split's collision discipline below
// suffix-numbers past them.) The derivation is
// collision-disciplined: it suffix-numbers past every id the model
// has any claim on — the patch's own replace/add ids, every prev-doc
// block id whose kind is not diagram, and every id named in
// unchanged_block_ids or remove_block_ids. The ONLY id a derived
// half may share is a prev-doc kind=diagram block the model left
// unclaimed — typically, but not provably, a prior split's persisted
// half (provenance is not tracked; a model-authored kind=diagram
// block whose id matches the derivation is refreshed the same way).
// That collision flows through the add→replace tolerance
// (normalizeAnswerDocumentPatchBlockOps) and refreshes the stale
// half in place — count-neutral, so it consumes no block-cap
// headroom. Model-authored non-diagram content is therefore never
// replaced by a derived id, removes always execute, and unchanged
// declarations stay byte-identical (R16). On the patch path derived
// halves flow into AddBlocks AFTER the model's own add entries so
// add_blocks[i] error indices stay the model's. Every other block id
// in the patch's op lists is LLM-provided. Replace targets existing id; Add inserts new id;
// Remove drops by id; Unchanged flows through byte-identical. Block
// ordering: removed blocks drop in place; replaced blocks substitute
// at original position; unchanged blocks preserve original position;
// added blocks append at tail. This determinism preserves the LLM's
// original ordering when the LLM didn't intend reorganization.
//
// VALIDATION INVARIANT (post-apply)
// ==================================
// The resulting AnswerDocumentV2 MUST pass the same V2 validator
// chain that emit_answer_document_v2 runs (no V1 fields, every block
// kind valid, every block id unique, etc.). Apply rejects
// pre-validation when the patch itself is malformed (unknown ids,
// dup ids, conflicting ops, mutually-exclusive Citations both set,
// etc.).
//
// HISTORY
// =======
// R16 protocol-level retry preservation (post_shape_residual_audit.md,
// 2026-05-04). Replaces R14 prompt-level "Hard Rule: byte-identical
// preserve" — m1a r2 真证据 from R14_eval_deep_audit.md §3 类 J shows
// LLM dropped fields on retry despite the prompt directive. R16 makes
// preservation structural: LLM declares what to keep / replace / add
// / remove; system copies what was declared kept verbatim, LLM never
// has the opportunity to rewrite preserved fields.
//
// Phase 1 mutation-contract documentation (this godoc, 2026-05-04):
// the operations table above is the canonical reference. Tools that
// emit patches MUST select exactly one op per field group; validators
// MUST treat the merged doc as truth, NOT inspect the patch shape.
type AnswerDocumentV2Patch struct {
	UnchangedBlockIDs []string      `json:"unchanged_block_ids,omitempty"`
	ReplaceBlocks     []AnswerBlock `json:"replace_blocks,omitempty"`
	AddBlocks         []AnswerBlock `json:"add_blocks,omitempty"`
	RemoveBlockIDs    []string      `json:"remove_block_ids,omitempty"`
	// ModelBlockOrder is an optional complete permutation of the previous
	// document's model-authored block ids. It changes only their relative
	// positions: system-generated blocks retain both their relative order and
	// their occupied slots. The model must select every id exactly once; the
	// executor never derives or chooses a reader-facing layout.
	//
	// To keep the permutation universe immutable and auditable, this operation
	// cannot be combined with add_blocks or remove_block_ids. Whole-block
	// replacement and local metadata edits remain compatible because they do not
	// change the model-owned id roster.
	ModelBlockOrder     []string                   `json:"model_block_order,omitempty"`
	BlockFieldEditsV1   []AnswerBlockFieldEditV1   `json:"block_field_edits_v1,omitempty"`
	BlockReceiptEditsV1 []AnswerBlockReceiptEditV1 `json:"block_receipt_edits_v1,omitempty"`
	// ReplaceCitations is OPTIONAL. When non-nil, the resulting
	// doc's Citations slice is REPLACED entirely (used when the
	// LLM needs to re-pick citations holistically). When nil,
	// citations from prev emit are inherited unchanged.
	ReplaceCitations []Citation `json:"replace_citations,omitempty"`
	// AppendCitations is OPTIONAL. When non-empty AND
	// ReplaceCitations is nil, these are appended to prev
	// citations. Useful for adding a NegativePattern citation
	// without rewriting the pool.
	AppendCitations []Citation `json:"append_citations,omitempty"`
	// ReplaceExactResolution: when non-nil, replaces prev
	// ExactResolution. nil means "keep prev".
	ReplaceExactResolution *AnswerExactResolution `json:"replace_exact_resolution,omitempty"`
	// ReplaceMissingRequestedRoles: when non-nil, replaces prev
	// MissingRequestedRoles. nil means "keep prev".
	ReplaceMissingRequestedRoles []AnswerMissingRequestedRole `json:"replace_missing_requested_roles,omitempty"`
	// ReplaceCaveats / ReplaceSnippets: when non-nil, replace.
	ReplaceCaveats  []string      `json:"replace_caveats,omitempty"`
	ReplaceSnippets []CodeSnippet `json:"replace_snippets,omitempty"`

	// AddBlockCompanionLineages is an executor-only sidecar created when a
	// model-authored fused block is losslessly split during this patch. It is
	// never decoded from the model payload and never selects a visible action.
	AddBlockCompanionLineages []AnswerBlockCompanionLineage `json:"-"`
}

// AnswerBlockEditableFieldV1 is the closed first-version field vocabulary for
// lossless local block metadata repair. Adding a field is a protocol change:
// it must remain non-visible, closed-enum, model-selected, and have explicit
// validation below. Free-form strings and nested carriers are intentionally
// not representable.
type AnswerBlockEditableFieldV1 string

const (
	AnswerBlockFieldTraceCausalClaimCaliber AnswerBlockEditableFieldV1 = "trace_causal_claim_caliber"
	AnswerBlockFieldCurrentStatusVerdict    AnswerBlockEditableFieldV1 = "current_status_verdict"
	AnswerBlockFieldErrorGranularityVerdict AnswerBlockEditableFieldV1 = "error_granularity_verdict"
	AnswerBlockFieldScopeDisclosure         AnswerBlockEditableFieldV1 = "scope_disclosure"
	AnswerBlockFieldSurfaceRole             AnswerBlockEditableFieldV1 = "surface_role"
	// AnswerBlockFieldAddFacetID appends one model-selected, schema-projected
	// facet membership to an exact existing block. It is deliberately additive:
	// replacing the full facet_ids array for a one-value ownership repair makes
	// the model replay unrelated typed memberships and can duplicate visible
	// relation rows when a relation-repair lease is active.
	AnswerBlockFieldAddFacetID AnswerBlockEditableFieldV1 = "add_facet_id"
)

// AnswerBlockFieldEditV1 applies one model-selected closed-enum metadata value
// to one exact existing block. Scalar fields are assigned; add_facet_id adds
// one membership without replacing the existing set. The operation carries no
// inference/default branch: Value is always supplied by the model and checked
// by the ordinary merged-document validators after structural validation.
type AnswerBlockFieldEditV1 struct {
	BlockID string                     `json:"block_id"`
	Field   AnswerBlockEditableFieldV1 `json:"field"`
	Value   string                     `json:"value"`
}

// AnswerBlockReceiptEditableFieldV1 is the closed typed-receipt vocabulary
// for lossless local retry repair. Values remain model-authored and are later
// bound to the exact dispatch-local contract.
type AnswerBlockReceiptEditableFieldV1 string

const (
	AnswerBlockReceiptFieldRuntimeWorkRelation          AnswerBlockReceiptEditableFieldV1 = "runtime_work_relation"
	AnswerBlockReceiptFieldConceptualTerminalResolution AnswerBlockReceiptEditableFieldV1 = "conceptual_terminal_resolution"
)

// AnswerBlockReceiptEditValueV1 carries the union of exact receipt selectors.
// Structural validation rejects fields that do not belong to the selected
// receipt kind, so a runtime observation id cannot leak into a conceptual
// terminal receipt and vice versa.
type AnswerBlockReceiptEditValueV1 struct {
	ObservationID string `json:"observation_id,omitempty"`
	EvidenceID    string `json:"evidence_id,omitempty"`
	Conclusion    string `json:"conclusion"`
}

// AnswerBlockReceiptEditV1 applies one model-selected schema-published receipt
// pair to an existing block without replaying any visible or typed siblings.
type AnswerBlockReceiptEditV1 struct {
	BlockID string                            `json:"block_id"`
	Field   AnswerBlockReceiptEditableFieldV1 `json:"field"`
	Value   AnswerBlockReceiptEditValueV1     `json:"value"`
}

// AnswerDocumentPatchOperationTeaching is the one compact, shared explanation
// of patch operation semantics used by finalizer prompts and retry hints. The
// projected tool schema remains the sole authority for JSON field types and
// required shapes; this text prevents two easy semantic mistakes: assuming
// an unmentioned block is deleted, or assuming ReplaceBlocks merges fields.
const AnswerDocumentPatchOperationTeaching = "Patch semantics: preserve an existing block with `unchanged_block_ids`, edit it with `replace_blocks`, append with `add_blocks`, and intentionally delete with `remove_block_ids`; omitting a previous block id from all four operations does not delete it. When only block positions change, `model_block_order` is a complete model-selected permutation of every previous model-authored block id; it changes no content, leaves system-generated slots untouched, and cannot be combined with add/remove. For one projected closed-enum metadata operation, use `block_field_edits_v1`: it preserves every unmentioned field and never chooses the value; use only the exact block_id/field/value branch in the current schema. When add_facet_id is published, it adds only that model-selected membership and never copies or changes a relation. `replace_snippets` is only for code snippets shaped as {file,start_line,end_line,language?,code}; block items, diagrams, evidence_ids, and other answer-block fields belong in a full `replace_blocks` entry. For a local diagram relation fix, prefer model-authored `diagram_edge_edits`; every unmentioned carrier remains preserved. A block named by `diagram_edge_edits`, `diagram_boundary_edits`, `diagram_boundary_replacements`, `diagram_relation_scope_edits`, or `diagram_participant_edits` is already preserved, and a redundant unchanged id is absorbed. Choose only an exact branch present in the current tool schema. A failure-only branch selects one live carrier; an addition-only branch selects one typed candidate and still requires model-authored visible nodes and label. Reuse endpoint ids that already have explicit declarations. For add/replace, if one chosen endpoint is new/implicit, also author its matching from_node_visible_label/to_node_visible_label so the system can encode your reader-facing node name instead of exposing an internal id; omit those fields for endpoints already declared. A published boundary_ref/action branch changes only that participant-boundary row and preserves every unmentioned boundary and visible graph carrier; use whole-array `diagram_boundary_replacements` only when no local boundary branch is published. When the schema publishes `diagram_relation_scope_edits`, use its exact block_id/action branch to change only the block-level coverage disclosure; do not replace the lease-target diagram. Omit legacy coordinates and hidden technical fields for every ref-selected branch, and never combine refs outside a single published branch. Refs never choose an action, relation, or visible wording. If selected edge edits isolate an optional_orphan_cleanups row, use `diagram_participant_edits` to choose remove_if_isolated or retain_as_context with visible_label. If a participant mismatch publishes participant_ref, use the exact ensure_visible branch and author node_id plus visible_label; it adds one disconnected declaration only and does not authorize an edge. Every `replace_blocks` entry replaces the whole existing block rather than merging fields; copy unchanged fields because an omitted field is deleted. Follow native schema types and never wrap an array or object payload in a JSON string."

// IsEmpty reports whether the patch carries zero modifications.
// Empty patches are explicitly rejected at Apply time — every
// retry MUST declare some change (even if it's just an
// UnchangedBlockIDs list that asserts "keep everything").
func (p *AnswerDocumentV2Patch) IsEmpty() bool {
	if p == nil {
		return true
	}
	return len(p.UnchangedBlockIDs) == 0 &&
		len(p.ReplaceBlocks) == 0 &&
		len(p.AddBlocks) == 0 &&
		len(p.RemoveBlockIDs) == 0 &&
		len(p.ModelBlockOrder) == 0 &&
		len(p.BlockFieldEditsV1) == 0 &&
		len(p.BlockReceiptEditsV1) == 0 &&
		p.ReplaceCitations == nil &&
		len(p.AppendCitations) == 0 &&
		p.ReplaceExactResolution == nil &&
		p.ReplaceMissingRequestedRoles == nil &&
		p.ReplaceCaveats == nil &&
		p.ReplaceSnippets == nil
}

// ApplyAnswerDocumentV2Patch applies p to prev and returns a new
// AnswerDocumentV2 with the patch applied. prev MUST be non-nil
// (R16 patch path REQUIRES a prev emit — first dispatches use
// emit_answer_document, not emit_answer_document_patch). Returns
// (nil, error) on any validation failure (unknown unchanged/replace ids,
// dup ids, conflicting ops, etc.). Unknown remove ids are intentionally
// accepted as idempotent no-ops.
//
// CONTRACT VALIDATION CHOKEPOINT (Phase 2-B2 audit, 2026-05-04):
// This function only enforces structural validity (block id
// uniqueness, kind enum, mutual-exclusion invariants 1-5,
// diagram-payload presence). It does NOT run the V2 oracle suite
// (block coverage / claim_use / facet / diagram edge / absence /
// richness). Those validators are owned by the orchestrator's
// runContractCheck (orchestrator/contract_check.go) which reads
// mut.AnswerDocumentV2() AFTER SetAnswerDocumentV2FromPatch
// commits the merged doc. Both emit paths (full + patch) write
// through the same MutableState surface, so runContractCheck sees
// identical merged-doc input regardless of provenance — the
// validation chokepoint is unified at the orchestrator layer.
//
// REGRESSION GUARD: If a future commit short-circuits patch emit
// to bypass mut.SetAnswerDocumentV2FromPatch (or its sibling
// SetAnswerDocumentV2), the post-merge V2 oracle suite will not
// observe the merged doc and silent contract-violations would ship.
// Tests in contract_check_block_test.go (Test*_PatchMergedDoc*)
// pin this contract by injecting a known-violating block via the
// patch path and asserting the same V2 oracles fire that would have
// fired on a fresh full emit of the same payload.
//
// Determinism: the resulting block order is
//  1. Every prev block in original order, EXCEPT removed and
//     replaced. Replaced blocks substitute the new payload at the
//     original position.
//  2. Added blocks appended in declaration order.
//
// This preserves the LLM's original block ordering when it
// chooses Unchanged + Replace, which keeps the user-facing
// rendered answer stable.
func ApplyAnswerDocumentV2Patch(prev *AnswerDocumentV2, p *AnswerDocumentV2Patch) (*AnswerDocumentV2, error) {
	if prev == nil {
		return nil, fmt.Errorf("ApplyAnswerDocumentV2Patch: prev is required (first emit must use emit_answer_document, not emit_answer_document_patch)")
	}
	if p == nil || p.IsEmpty() {
		return nil, fmt.Errorf("ApplyAnswerDocumentV2Patch: empty patch — every retry must declare some change (use unchanged_block_ids to assert preservation)")
	}

	// Validate patch structure.
	if err := validatePatchStructure(prev, p); err != nil {
		return nil, err
	}

	// Build the merged doc.
	out := &AnswerDocumentV2{
		DocumentModel: "v2",
	}

	// Handle Citations: ReplaceCitations wins; else inherit prev +
	// AppendCitations.
	if p.ReplaceCitations != nil {
		out.Citations = append([]Citation(nil), p.ReplaceCitations...)
	} else {
		out.Citations = append([]Citation(nil), prev.Citations...)
		if len(p.AppendCitations) > 0 {
			out.Citations = append(out.Citations, p.AppendCitations...)
		}
	}

	// Handle ExactResolution / MissingRequestedRoles / Caveats /
	// Snippets: replace-only surfaces.
	if p.ReplaceExactResolution != nil {
		out.ExactResolution = p.ReplaceExactResolution
	} else {
		out.ExactResolution = prev.ExactResolution
	}
	if p.ReplaceMissingRequestedRoles != nil {
		out.MissingRequestedRoles = append([]AnswerMissingRequestedRole(nil), p.ReplaceMissingRequestedRoles...)
	} else {
		out.MissingRequestedRoles = append([]AnswerMissingRequestedRole(nil), prev.MissingRequestedRoles...)
	}
	if p.ReplaceCaveats != nil {
		out.Caveats = append([]string(nil), p.ReplaceCaveats...)
	} else {
		out.Caveats = append([]string(nil), prev.Caveats...)
	}
	if p.ReplaceSnippets != nil {
		out.Snippets = append([]CodeSnippet(nil), p.ReplaceSnippets...)
	} else {
		out.Snippets = append([]CodeSnippet(nil), prev.Snippets...)
	}
	if len(prev.ReadOwnerAnchors) > 0 {
		out.ReadOwnerAnchors = NormalizeOwnerAnchorView(OwnerAnchorView{Items: prev.ReadOwnerAnchors}, 0).Items
	}
	out.ReadSourceLocalization = CloneSourceLocalizationReviewPtr(prev.ReadSourceLocalization)
	if prev.ReadNavigationCoverage != nil {
		coverage := NormalizeRepoMapNavigationCoverage(*prev.ReadNavigationCoverage)
		out.ReadNavigationCoverage = &coverage
	}
	out.ReadLocalizerFollowup = CloneReadLocalizerFollowupPtr(prev.ReadLocalizerFollowup)
	out.ReadReasoningGraph = CloneAnswerReasoningGraphSummaryPtr(prev.ReadReasoningGraph)

	// Block merge:
	//
	//   1. Build a removed-set + replaced-by map.
	//   2. Walk prev.Blocks in order:
	//      - id in removed-set → drop
	//      - id in replaced-by → emit the replacement
	//      - id in unchanged-set OR not mentioned → emit prev as-is
	//   3. Append AddBlocks at the tail.
	removed := make(map[string]bool, len(p.RemoveBlockIDs))
	for _, id := range p.RemoveBlockIDs {
		removed[id] = true
	}
	replacedBy := make(map[string]AnswerBlock, len(p.ReplaceBlocks))
	for _, b := range p.ReplaceBlocks {
		replacedBy[b.ID] = b
	}
	editedBy := make(map[string]AnswerBlock, len(p.BlockFieldEditsV1)+len(p.BlockReceiptEditsV1))
	for _, edit := range p.BlockFieldEditsV1 {
		block, ok := answerDocumentBlockByID(prev, edit.BlockID)
		if !ok {
			return nil, fmt.Errorf("patch: block_field_edits_v1 target %q disappeared after validation", edit.BlockID)
		}
		if current, exists := editedBy[edit.BlockID]; exists {
			block = current
		}
		updated, err := applyAnswerBlockFieldEditV1(block, edit)
		if err != nil {
			return nil, err
		}
		editedBy[edit.BlockID] = updated
	}
	for _, edit := range p.BlockReceiptEditsV1 {
		block, ok := answerDocumentBlockByID(prev, edit.BlockID)
		if !ok {
			return nil, fmt.Errorf("patch: block_receipt_edits_v1 target %q disappeared after validation", edit.BlockID)
		}
		if current, exists := editedBy[edit.BlockID]; exists {
			block = current
		}
		updated, err := applyAnswerBlockReceiptEditV1(block, edit)
		if err != nil {
			return nil, err
		}
		editedBy[edit.BlockID] = updated
	}
	for _, prevBlock := range prev.Blocks {
		if removed[prevBlock.ID] {
			continue
		}
		if replacement, ok := replacedBy[prevBlock.ID]; ok {
			out.Blocks = append(out.Blocks, replacement)
			continue
		}
		if edited, ok := editedBy[prevBlock.ID]; ok {
			out.Blocks = append(out.Blocks, edited)
			continue
		}
		// Inherit prev block as-is. This is the load-bearing
		// preservation: every typed annotation (claim_uses,
		// facet_ids, surface_role, edge_anchors, diagram payload)
		// flows through structurally — LLM never has a chance to
		// drop them.
		out.Blocks = append(out.Blocks, prevBlock)
	}
	// Append new blocks.
	out.Blocks = append(out.Blocks, p.AddBlocks...)

	// A model-authored order operation is applied only after the ordinary
	// content merge so replacement/local-edit semantics stay unchanged. The
	// validated permutation supplies one model block for each existing model
	// slot. System-authored blocks never enter the permutation and therefore
	// keep their exact occupied slots and relative order.
	if len(p.ModelBlockOrder) > 0 {
		byID := make(map[string]AnswerBlock, len(p.ModelBlockOrder))
		for _, block := range out.Blocks {
			if block.SystemGeneratedKind == AnswerSystemGeneratedBlockUnknown {
				byID[block.ID] = block
			}
		}
		ordered := make([]AnswerBlock, 0, len(p.ModelBlockOrder))
		for _, id := range p.ModelBlockOrder {
			ordered = append(ordered, byID[id])
		}
		next := 0
		for i := range out.Blocks {
			if out.Blocks[i].SystemGeneratedKind != AnswerSystemGeneratedBlockUnknown {
				continue
			}
			out.Blocks[i] = ordered[next]
			next++
		}
	}

	// Companion provenance follows only pairs that still exist after the
	// model-authored patch. Removing either half retires the pair; retaining or
	// replacing both keeps it. New split pairs are appended by the executor.
	present := make(map[string]bool, len(out.Blocks))
	for _, block := range out.Blocks {
		if id := strings.TrimSpace(block.ID); id != "" {
			present[id] = true
		}
	}
	lineages := append([]AnswerBlockCompanionLineage(nil), prev.BlockCompanionLineages...)
	lineages = append(lineages, p.AddBlockCompanionLineages...)
	lineages = NormalizeAnswerBlockCompanionLineages(lineages)
	for _, lineage := range lineages {
		if present[lineage.VisibleBlockID] && present[lineage.DiagramBlockID] {
			out.BlockCompanionLineages = append(out.BlockCompanionLineages, lineage)
		}
	}

	return out, nil
}

// validatePatchStructure rejects malformed patches before Apply
// touches the doc. Catches:
//   - unknown ids in UnchangedBlockIDs / ReplaceBlocks
//   - dup ids within a single op (e.g. two Replace entries for
//     same id, two Add entries for same id, Add id colliding with
//     prev id)
//   - cross-op conflicts (Replace + Remove on same id, etc.)
//   - empty block ids on Replace / Add
func validatePatchStructure(prev *AnswerDocumentV2, p *AnswerDocumentV2Patch) error {
	prevIDs := make(map[string]bool, len(prev.Blocks))
	for _, b := range prev.Blocks {
		prevIDs[b.ID] = true
	}

	// UnchangedBlockIDs must each name an existing prev block.
	for _, id := range p.UnchangedBlockIDs {
		if id == "" {
			return fmt.Errorf("patch: unchanged_block_ids contains empty id")
		}
		if !prevIDs[id] {
			return fmt.Errorf("patch: unchanged_block_ids[%q] not present in previous emit", id)
		}
	}

	// RemoveBlockIDs are idempotent postconditions: the requested id must
	// be absent after apply. An id already absent from prev is therefore a
	// valid no-op. Empty ids, duplicates, and cross-op contradictions remain
	// malformed and are rejected below.
	removeSet := make(map[string]bool, len(p.RemoveBlockIDs))
	for _, id := range p.RemoveBlockIDs {
		if id == "" {
			return fmt.Errorf("patch: remove_block_ids contains empty id")
		}
		if removeSet[id] {
			return fmt.Errorf("patch: remove_block_ids[%q] duplicated", id)
		}
		removeSet[id] = true
	}

	// ModelBlockOrder is a complete, exact permutation of the immutable
	// previous model-owned roster. This is deliberately stricter than a partial
	// "move before" command: no executor choice or inferred residual order is
	// possible. Add/remove would change the roster mid-transaction, so those
	// combinations fail closed and the model must perform them in another patch.
	if len(p.ModelBlockOrder) > 0 {
		if len(p.AddBlocks) > 0 || len(p.RemoveBlockIDs) > 0 {
			return fmt.Errorf("patch: model_block_order cannot be combined with add_blocks or remove_block_ids; first settle the block roster, then submit one complete model-owned permutation")
		}
		modelIDs := make(map[string]bool)
		for _, block := range prev.Blocks {
			if block.SystemGeneratedKind == AnswerSystemGeneratedBlockUnknown {
				modelIDs[block.ID] = true
			}
		}
		if len(p.ModelBlockOrder) != len(modelIDs) {
			return fmt.Errorf("patch: model_block_order must list every model-authored previous block exactly once: got %d ids, want %d", len(p.ModelBlockOrder), len(modelIDs))
		}
		seen := make(map[string]bool, len(p.ModelBlockOrder))
		for _, raw := range p.ModelBlockOrder {
			id := strings.TrimSpace(raw)
			if id == "" || id != raw {
				return fmt.Errorf("patch: model_block_order contains an empty or whitespace-padded id")
			}
			if seen[id] {
				return fmt.Errorf("patch: model_block_order[%q] duplicated", id)
			}
			if !modelIDs[id] {
				return fmt.Errorf("patch: model_block_order[%q] is not a model-authored block in the previous emit", id)
			}
			seen[id] = true
		}
	}

	// ReplaceBlocks: each must have non-empty id, id in prev, no
	// dup within Replace, no overlap with RemoveBlockIDs.
	replaceSet := make(map[string]bool, len(p.ReplaceBlocks))
	for _, b := range p.ReplaceBlocks {
		id := strings.TrimSpace(b.ID)
		if id == "" {
			return fmt.Errorf("patch: replace_blocks entry with empty id (kind=%s)", b.Kind)
		}
		if !prevIDs[id] {
			return fmt.Errorf("patch: replace_blocks[%q] not present in previous emit (use add_blocks for new blocks)", id)
		}
		if replaceSet[id] {
			return fmt.Errorf("patch: replace_blocks[%q] duplicated", id)
		}
		if removeSet[id] {
			return fmt.Errorf("patch: replace_blocks[%q] also in remove_block_ids — pick one", id)
		}
		replaceSet[id] = true
	}

	// BlockFieldEditsV1: exact existing model block, one assignment per
	// block+field, valid closed-enum value, and no whole-block mutation of the
	// same target. Redundant Unchanged is intentionally harmless.
	fieldEditSet := make(map[string]bool, len(p.BlockFieldEditsV1))
	for _, edit := range p.BlockFieldEditsV1 {
		id := strings.TrimSpace(edit.BlockID)
		if id == "" {
			return fmt.Errorf("patch: block_field_edits_v1 contains empty block_id")
		}
		if id != edit.BlockID {
			return fmt.Errorf("patch: block_field_edits_v1 block_id=%q must match the exact previous block id without surrounding whitespace", edit.BlockID)
		}
		block, ok := answerDocumentBlockByID(prev, id)
		if !ok {
			return fmt.Errorf("patch: block_field_edits_v1[%q] not present in previous emit", id)
		}
		if block.SystemGeneratedKind != AnswerSystemGeneratedBlockUnknown {
			return fmt.Errorf("patch: block_field_edits_v1[%q] cannot edit a system-generated block", id)
		}
		if removeSet[id] {
			return fmt.Errorf("patch: block_field_edits_v1[%q] also in remove_block_ids — pick one", id)
		}
		if replaceSet[id] {
			return fmt.Errorf("patch: block_field_edits_v1[%q] also in replace_blocks — pick one", id)
		}
		key := id + "\x00" + string(edit.Field)
		if fieldEditSet[key] {
			return fmt.Errorf("patch: block_field_edits_v1[%q].%s duplicated", id, edit.Field)
		}
		if _, err := applyAnswerBlockFieldEditV1(block, edit); err != nil {
			return err
		}
		fieldEditSet[key] = true
	}

	// BlockReceiptEditsV1: exact existing model block, one assignment per
	// block+receipt field, and no whole-block mutation of the same target.
	receiptEditSet := make(map[string]bool, len(p.BlockReceiptEditsV1))
	for _, edit := range p.BlockReceiptEditsV1 {
		id := strings.TrimSpace(edit.BlockID)
		if id == "" {
			return fmt.Errorf("patch: block_receipt_edits_v1 contains empty block_id")
		}
		if id != edit.BlockID {
			return fmt.Errorf("patch: block_receipt_edits_v1 block_id=%q must match the exact previous block id without surrounding whitespace", edit.BlockID)
		}
		block, ok := answerDocumentBlockByID(prev, id)
		if !ok {
			return fmt.Errorf("patch: block_receipt_edits_v1[%q] not present in previous emit", id)
		}
		if block.SystemGeneratedKind != AnswerSystemGeneratedBlockUnknown {
			return fmt.Errorf("patch: block_receipt_edits_v1[%q] cannot edit a system-generated block", id)
		}
		if removeSet[id] {
			return fmt.Errorf("patch: block_receipt_edits_v1[%q] also in remove_block_ids — pick one", id)
		}
		if replaceSet[id] {
			return fmt.Errorf("patch: block_receipt_edits_v1[%q] also in replace_blocks — pick one", id)
		}
		key := id + "\x00" + string(edit.Field)
		if receiptEditSet[key] {
			return fmt.Errorf("patch: block_receipt_edits_v1[%q].%s duplicated", id, edit.Field)
		}
		if _, err := applyAnswerBlockReceiptEditV1(block, edit); err != nil {
			return err
		}
		receiptEditSet[key] = true
	}

	// AddBlocks: each must have non-empty id, id NOT in prev,
	// no dup within Add, no overlap with Replace / Remove.
	addSet := make(map[string]bool, len(p.AddBlocks))
	for _, b := range p.AddBlocks {
		id := strings.TrimSpace(b.ID)
		if id == "" {
			return fmt.Errorf("patch: add_blocks entry with empty id (kind=%s)", b.Kind)
		}
		if prevIDs[id] {
			return fmt.Errorf("patch: add_blocks[%q] already exists in previous emit (use replace_blocks to modify)", id)
		}
		if addSet[id] {
			return fmt.Errorf("patch: add_blocks[%q] duplicated", id)
		}
		if replaceSet[id] {
			return fmt.Errorf("patch: add_blocks[%q] also in replace_blocks — pick one", id)
		}
		if removeSet[id] {
			return fmt.Errorf("patch: add_blocks[%q] also in remove_block_ids — pick one", id)
		}
		// Validate kind.
		if !IsValidAnswerBlockKind(b.Kind) {
			return fmt.Errorf("patch: add_blocks[%q] kind=%q is not valid", id, b.Kind)
		}
		addSet[id] = true
	}

	// Replace block kind sanity: require valid kind on each
	// replacement (same gate emit_answer_document applies).
	for _, b := range p.ReplaceBlocks {
		if !IsValidAnswerBlockKind(b.Kind) {
			return fmt.Errorf("patch: replace_blocks[%q] kind=%q is not valid", b.ID, b.Kind)
		}
	}

	// Mutation-contract invariant (5): Citations Replace XOR Append.
	// ReplaceCitations re-pickes the pool wholesale; AppendCitations
	// adds to the inherited pool. Setting BOTH signals contradictory
	// LLM intent and is rejected — the LLM must commit to one mode.
	if p.ReplaceCitations != nil && len(p.AppendCitations) > 0 {
		return fmt.Errorf("patch: replace_citations and append_citations are mutually exclusive (contract invariant 5); set exactly one")
	}
	if p.ReplaceCitations != nil {
		for _, b := range prev.Blocks {
			if removeSet[b.ID] || replaceSet[b.ID] {
				continue
			}
			if answerBlockHasCitationRefs(b) {
				return fmt.Errorf("patch: replace_citations cannot preserve citation-bearing block %q; replace/remove that block too, use append_citations, or re-emit a full emit_answer_document so every citation_ref is renumbered against the new pool", b.ID)
			}
		}
	}

	return nil
}

func answerDocumentBlockByID(doc *AnswerDocumentV2, id string) (AnswerBlock, bool) {
	if doc == nil {
		return AnswerBlock{}, false
	}
	for _, block := range doc.Blocks {
		if block.ID == id {
			return block, true
		}
	}
	return AnswerBlock{}, false
}

func applyAnswerBlockFieldEditV1(block AnswerBlock, edit AnswerBlockFieldEditV1) (AnswerBlock, error) {
	id := strings.TrimSpace(edit.BlockID)
	value := strings.TrimSpace(edit.Value)
	switch edit.Field {
	case AnswerBlockFieldTraceCausalClaimCaliber:
		if block.Kind != BlockSummary {
			return AnswerBlock{}, fmt.Errorf("patch: block_field_edits_v1[%q].trace_causal_claim_caliber requires kind=summary", id)
		}
		v, ok := NormalizeTraceCausalClaimCaliber(value)
		if !ok {
			return AnswerBlock{}, fmt.Errorf("patch: block_field_edits_v1[%q].trace_causal_claim_caliber=%q is invalid", id, edit.Value)
		}
		block.TraceCausalClaimCaliber = v
	case AnswerBlockFieldCurrentStatusVerdict:
		if block.Kind != BlockDecision {
			return AnswerBlock{}, fmt.Errorf("patch: block_field_edits_v1[%q].current_status_verdict requires kind=decision", id)
		}
		v, ok := NormalizeCurrentStatusVerdict(value)
		if !ok || v == CurrentStatusUnknown {
			return AnswerBlock{}, fmt.Errorf("patch: block_field_edits_v1[%q].current_status_verdict=%q is invalid", id, edit.Value)
		}
		block.CurrentStatusVerdict = v
	case AnswerBlockFieldErrorGranularityVerdict:
		if block.Kind != BlockDecision {
			return AnswerBlock{}, fmt.Errorf("patch: block_field_edits_v1[%q].error_granularity_verdict requires kind=decision", id)
		}
		v, ok := NormalizeErrorGranularityVerdict(value)
		if !ok || v == ErrorGranularityUnknown {
			return AnswerBlock{}, fmt.Errorf("patch: block_field_edits_v1[%q].error_granularity_verdict=%q is invalid", id, edit.Value)
		}
		block.ErrorGranularityVerdict = v
	case AnswerBlockFieldScopeDisclosure:
		v, ok := NormalizeScopeDisclosureKind(value)
		if !ok || v == ScopeDisclosureUnknown {
			return AnswerBlock{}, fmt.Errorf("patch: block_field_edits_v1[%q].scope_disclosure=%q is invalid", id, edit.Value)
		}
		block.ScopeDisclosure = v
	case AnswerBlockFieldSurfaceRole:
		v, ok := NormalizeSurfaceRole(value)
		if !ok || v == "" {
			return AnswerBlock{}, fmt.Errorf("patch: block_field_edits_v1[%q].surface_role=%q is invalid", id, edit.Value)
		}
		block.SurfaceRole = v
	case AnswerBlockFieldAddFacetID:
		facet := AnswerFacetKind(value)
		if !IsKnownAnswerFacetKind(facet) {
			return AnswerBlock{}, fmt.Errorf("patch: block_field_edits_v1[%q].add_facet_id=%q is invalid", id, edit.Value)
		}
		for _, existing := range block.FacetIDs {
			if strings.TrimSpace(existing) == string(facet) {
				return block, nil
			}
		}
		block.FacetIDs = append(block.FacetIDs, string(facet))
	default:
		return AnswerBlock{}, fmt.Errorf("patch: block_field_edits_v1[%q].field=%q is not in the v1 whitelist", id, edit.Field)
	}
	return block, nil
}

func applyAnswerBlockReceiptEditV1(block AnswerBlock, edit AnswerBlockReceiptEditV1) (AnswerBlock, error) {
	id := strings.TrimSpace(edit.BlockID)
	conclusion := strings.TrimSpace(edit.Value.Conclusion)
	switch edit.Field {
	case AnswerBlockReceiptFieldRuntimeWorkRelation:
		observationID := strings.TrimSpace(edit.Value.ObservationID)
		if observationID == "" || strings.TrimSpace(edit.Value.EvidenceID) != "" {
			return AnswerBlock{}, fmt.Errorf("patch: block_receipt_edits_v1[%q].runtime_work_relation requires only observation_id and conclusion", id)
		}
		value := RuntimeWorkRelationConclusion(conclusion)
		if !value.IsValid() {
			return AnswerBlock{}, fmt.Errorf("patch: block_receipt_edits_v1[%q].runtime_work_relation conclusion=%q is invalid", id, edit.Value.Conclusion)
		}
		block.RuntimeWorkRelation = &AnswerRuntimeWorkRelationReceipt{
			ObservationID: observationID,
			Conclusion:    value,
		}
	case AnswerBlockReceiptFieldConceptualTerminalResolution:
		if strings.TrimSpace(edit.Value.ObservationID) != "" {
			return AnswerBlock{}, fmt.Errorf("patch: block_receipt_edits_v1[%q].conceptual_terminal_resolution does not accept observation_id", id)
		}
		value := ConceptualTerminalResolutionConclusion(conclusion)
		if !value.IsValid() {
			return AnswerBlock{}, fmt.Errorf("patch: block_receipt_edits_v1[%q].conceptual_terminal_resolution conclusion=%q is invalid", id, edit.Value.Conclusion)
		}
		block.ConceptualTerminalResolution = &AnswerConceptualTerminalResolutionReceipt{
			EvidenceID: strings.TrimSpace(edit.Value.EvidenceID),
			Conclusion: value,
		}
	default:
		return AnswerBlock{}, fmt.Errorf("patch: block_receipt_edits_v1[%q].field=%q is not in the v1 whitelist", id, edit.Field)
	}
	return block, nil
}

func answerBlockHasCitationRefs(b AnswerBlock) bool {
	for _, item := range b.Items {
		if len(AnswerBlockItemCitationRefs(item)) > 0 {
			return true
		}
	}
	return false
}
