package types

import "strings"

// AcceptedKinds returns the canonical block kind plus any typed
// alternative carriers for the same BlockRequirement obligation.
// Empty / duplicate kinds are removed while preserving declaration
// order so prompt rendering remains stable.
func (br BlockRequirement) AcceptedKinds() []AnswerBlockKind {
	seen := make(map[AnswerBlockKind]bool, 1+len(br.AlternativeKinds))
	out := make([]AnswerBlockKind, 0, 1+len(br.AlternativeKinds))
	add := func(kind AnswerBlockKind) {
		if kind == "" || seen[kind] {
			return
		}
		seen[kind] = true
		out = append(out, kind)
	}
	add(br.Kind)
	for _, kind := range br.AlternativeKinds {
		add(kind)
	}
	return out
}

// AcceptsKind reports whether kind satisfies this requirement's
// carrier constraint.
func (br BlockRequirement) AcceptsKind(kind AnswerBlockKind) bool {
	if kind == "" {
		return false
	}
	for _, accepted := range br.AcceptedKinds() {
		if accepted == kind {
			return true
		}
	}
	return false
}

// AcceptedKindsLabel renders a compact stable label for LLM-facing
// diagnostics and prompts, e.g. "ordered_list/table/bullet_list".
func (br BlockRequirement) AcceptedKindsLabel() string {
	kinds := br.AcceptedKinds()
	if len(kinds) == 0 {
		return ""
	}
	parts := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		parts = append(parts, string(kind))
	}
	return strings.Join(parts, "/")
}

// AnswerBlockCountsForRequirement reports whether block belongs to the
// requirement's carrier domain. Kind is necessary but not sufficient when the
// requirement declares FacetIDs: another independently-answerable dimension
// may legitimately use the same table/list/section kind. Counting that sibling
// block against this requirement can make one contract demand the block while
// another caps it, producing an impossible repair loop.
//
// Facet matching is any-overlap across block.facet_ids and
// block.claim_uses[].facet_id, consistent with the existing typed principal,
// coverage, and allowed-claim-form ownership checks. A requirement with
// several facets describes one carrier family; separate facet completeness
// validation still proves every required facet. Empty FacetIDs preserves the
// historical kind-only behavior for generic summary and presentation
// requirements.
func AnswerBlockCountsForRequirement(block AnswerBlock, br BlockRequirement) bool {
	if !br.AcceptsKind(block.Kind) {
		return false
	}
	if len(br.FacetIDs) == 0 {
		return true
	}
	for _, requiredFacet := range br.FacetIDs {
		requiredFacet = strings.TrimSpace(requiredFacet)
		if requiredFacet == "" {
			continue
		}
		for _, blockFacet := range block.FacetIDs {
			if strings.TrimSpace(blockFacet) == requiredFacet {
				return true
			}
		}
		for _, claim := range block.ClaimUses {
			if strings.TrimSpace(claim.FacetID) == requiredFacet {
				return true
			}
		}
	}
	return false
}

// CountAnswerBlocksForRequirement counts blocks in the requirement's typed
// carrier domain. The result is what MinCount / MaxCount are compared against.
func CountAnswerBlocksForRequirement(blocks []AnswerBlock, br BlockRequirement) int {
	if len(blocks) == 0 {
		return 0
	}
	count := 0
	for _, block := range blocks {
		if AnswerBlockCountsForRequirement(block, br) {
			count++
		}
	}
	return count
}

// CountAnswerBlocksForRequirementKinds retains the deliberately broader
// kind-only question used by ownership floors such as Trace model-authorship:
// those checks ask whether the model emitted any principal-shaped payload at
// all, before facet metadata is complete. It must not be used for a
// BlockRequirement's MinCount / MaxCount validation.
func CountAnswerBlocksForRequirementKinds(blocks []AnswerBlock, br BlockRequirement) int {
	count := 0
	for _, block := range blocks {
		if br.AcceptsKind(block.Kind) {
			count++
		}
	}
	return count
}
