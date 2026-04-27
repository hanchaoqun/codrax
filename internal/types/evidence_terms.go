package types

import "strings"

// EvidenceItemMentionsAnyTerm reports whether the evidence item's
// grounded text surfaces any validated same-scope term. It is a shared
// lexical helper used by explorer/finalizer/tooling to keep exact-target
// nearby-context narrowing aligned.
func EvidenceItemMentionsAnyTerm(item EvidenceItem, terms []string) bool {
	if len(terms) == 0 {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		item.Subject,
		item.Predicate,
		item.Object,
		item.AnchorSymbol,
		item.Condition,
		item.Snippet,
		item.Summary,
	}, "\n"))
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" && strings.Contains(text, term) {
			return true
		}
	}
	return false
}

// EvidenceItemStructurallyMentionsAnyTerm is the stricter sibling of
// EvidenceItemMentionsAnyTerm: it only scans fields that are expected
// to come from grounded code/config structure (source path, anchored
// symbols, condition/snippet text), and deliberately ignores LLM-authored
// free-form summary/predicate prose. Use this helper when the system is
// deciding whether an evidence item can SATISFY a same-scope contract,
// not merely whether it is topically adjacent.
func EvidenceItemStructurallyMentionsAnyTerm(item EvidenceItem, terms []string) bool {
	if len(terms) == 0 {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		item.Source,
		item.Subject,
		item.Object,
		item.AnchorSymbol,
		item.Condition,
		item.Snippet,
	}, "\n"))
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" && strings.Contains(text, term) {
			return true
		}
	}
	return false
}
