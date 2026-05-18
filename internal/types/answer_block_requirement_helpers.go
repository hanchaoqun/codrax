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

// CountAnswerBlocksForRequirement counts all blocks whose kind is a
// canonical or alternative carrier for the same requirement. The
// result is what MinCount / MaxCount should be compared against.
func CountAnswerBlocksForRequirement(blocks []AnswerBlock, br BlockRequirement) int {
	if len(blocks) == 0 {
		return 0
	}
	accepted := make(map[AnswerBlockKind]bool, 1+len(br.AlternativeKinds))
	for _, kind := range br.AcceptedKinds() {
		accepted[kind] = true
	}
	if len(accepted) == 0 {
		return 0
	}
	count := 0
	for _, block := range blocks {
		if accepted[block.Kind] {
			count++
		}
	}
	return count
}
