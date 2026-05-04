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
//                  |                                    | (2) Replace∩Add ∅
//                  |                                    | (3) Add∩Remove ∅
//                  |                                    | (4) Unchanged ⊆ prev.ids
// Citations        | Replace | Append                  | (5) Replace XOR Append
// ExactResolution  | Replace                           | nil = inherit prev
// Caveats          | Replace                           | nil = inherit prev
// Snippets         | Replace                           | nil = inherit prev
//
// Op semantics:
//
//   - UnchangedBlockIDs: block ids from prev emit copied verbatim
//     (every annotation field — claim_use / facet_ids / surface_role
//     / items / diagram — preserved byte-identical). The id MUST
//     exist in prev (invariant 4). LLM never re-emits fields on
//     these blocks; structural preservation against drop-on-retry.
//   - ReplaceBlocks: full block payloads override the prev emit's
//     block with the same id. id MUST exist in prev. id MUST NOT
//     also appear in RemoveBlockIDs (1) or AddBlocks (2).
//   - AddBlocks: full block payloads APPENDED at tail. id MUST NOT
//     already exist in prev. id MUST NOT also appear in
//     ReplaceBlocks (2) or RemoveBlockIDs (3).
//   - RemoveBlockIDs: block ids dropped from new doc. id MUST exist
//     in prev. id MUST NOT also appear in ReplaceBlocks (1) or
//     AddBlocks (3).
//   - Citations: ReplaceCitations and AppendCitations are mutually
//     exclusive (5); use Replace for holistic re-pick, Append for
//     additive (e.g. negative-pattern citation without rewriting
//     pool).
//   - ExactResolution / Caveats / Snippets: replace-or-inherit.
//
// Block id is LLM-provided (never system-generated). Replace targets
// existing id; Add inserts new id; Remove drops by id; Unchanged
// flows through byte-identical. Block ordering: removed blocks drop
// in place; replaced blocks substitute at original position;
// unchanged blocks preserve original position; added blocks append
// at tail. This determinism preserves the LLM's original ordering
// when the LLM didn't intend reorganization.
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
	UnchangedBlockIDs []string                      `json:"unchanged_block_ids,omitempty"`
	ReplaceBlocks     []AnswerBlock                 `json:"replace_blocks,omitempty"`
	AddBlocks         []AnswerBlock                 `json:"add_blocks,omitempty"`
	RemoveBlockIDs    []string                      `json:"remove_block_ids,omitempty"`
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
	// ReplaceCaveats / ReplaceSnippets: when non-nil, replace.
	ReplaceCaveats  []string      `json:"replace_caveats,omitempty"`
	ReplaceSnippets []CodeSnippet `json:"replace_snippets,omitempty"`
}

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
		p.ReplaceCitations == nil &&
		len(p.AppendCitations) == 0 &&
		p.ReplaceExactResolution == nil &&
		p.ReplaceCaveats == nil &&
		p.ReplaceSnippets == nil
}

// ApplyAnswerDocumentV2Patch applies p to prev and returns a new
// AnswerDocumentV2 with the patch applied. prev MUST be non-nil
// (R16 patch path REQUIRES a prev emit — first dispatches use
// emit_answer_document, not emit_answer_document_patch). Returns
// (nil, error) on any validation failure (unknown ids, dup ids,
// conflicting ops, etc.).
//
// Determinism: the resulting block order is
//   1. Every prev block in original order, EXCEPT removed and
//      replaced. Replaced blocks substitute the new payload at the
//      original position.
//   2. Added blocks appended in declaration order.
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

	// Handle ExactResolution / Caveats / Snippets: replace-only
	// surfaces.
	if p.ReplaceExactResolution != nil {
		out.ExactResolution = p.ReplaceExactResolution
	} else {
		out.ExactResolution = prev.ExactResolution
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
	for _, prevBlock := range prev.Blocks {
		if removed[prevBlock.ID] {
			continue
		}
		if replacement, ok := replacedBy[prevBlock.ID]; ok {
			out.Blocks = append(out.Blocks, replacement)
			continue
		}
		// Inherit prev block as-is. This is the load-bearing
		// preservation: every typed annotation (claim_use,
		// facet_ids, surface_role, items[].claim_use, diagram
		// payload) flows through structurally — LLM never has a
		// chance to drop them.
		out.Blocks = append(out.Blocks, prevBlock)
	}
	// Append new blocks.
	out.Blocks = append(out.Blocks, p.AddBlocks...)

	return out, nil
}

// validatePatchStructure rejects malformed patches before Apply
// touches the doc. Catches:
//   - unknown ids in UnchangedBlockIDs / RemoveBlockIDs / ReplaceBlocks
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

	// RemoveBlockIDs must each name an existing prev block.
	removeSet := make(map[string]bool, len(p.RemoveBlockIDs))
	for _, id := range p.RemoveBlockIDs {
		if id == "" {
			return fmt.Errorf("patch: remove_block_ids contains empty id")
		}
		if !prevIDs[id] {
			return fmt.Errorf("patch: remove_block_ids[%q] not present in previous emit", id)
		}
		if removeSet[id] {
			return fmt.Errorf("patch: remove_block_ids[%q] duplicated", id)
		}
		removeSet[id] = true
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

	return nil
}
