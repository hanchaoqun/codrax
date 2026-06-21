package types

// AnswerBlockKindRendersStructuredItems reports whether a block kind has a
// visible item surface that can carry per-member labels, details, and
// citation_ref anchors. It is the shared typed carrier predicate for final
// answer item coverage; callers decide separately whether a block is principal
// answer content.
func AnswerBlockKindRendersStructuredItems(kind AnswerBlockKind) bool {
	switch kind {
	case BlockSection, BlockOrderedList, BlockBulletList, BlockTable:
		return true
	default:
		return false
	}
}
