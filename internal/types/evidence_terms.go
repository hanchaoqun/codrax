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
