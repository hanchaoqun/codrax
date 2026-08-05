package types

import "strings"

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

// AnswerBlockRendersStructuredItems reports whether this concrete block's
// Items are actually rendered as individual visible rows. A table may carry
// Items only as citation sidecars for a model-authored Markdown table in Text
// (or in one item's Label/Text); the renderer returns after that Markdown
// carrier and does not render the remaining Items. Coverage validators must
// use this block-aware predicate rather than treating those hidden sidecars as
// visible enumeration rows.
func AnswerBlockRendersStructuredItems(block AnswerBlock) bool {
	if !AnswerBlockKindRendersStructuredItems(block.Kind) {
		return false
	}
	if block.Kind != BlockTable {
		return true
	}
	if AnswerTextLooksLikeMarkdownTable(block.Text) {
		return false
	}
	for _, item := range block.Items {
		if AnswerTextLooksLikeMarkdownTable(item.Label) || AnswerTextLooksLikeMarkdownTable(item.Text) {
			return false
		}
	}
	return true
}

// AnswerTextLooksLikeMarkdownTable is the shared renderer/validator classifier
// for a complete Markdown table carrier. It recognizes structure only; table
// contents never become hard-gate keywords.
func AnswerTextLooksLikeMarkdownTable(text string) bool {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 2 {
		return false
	}
	for i := 0; i+1 < len(lines); i++ {
		header := strings.TrimSpace(lines[i])
		separator := strings.TrimSpace(lines[i+1])
		if header == "" || separator == "" || !strings.Contains(header, "|") {
			continue
		}
		if answerMarkdownTableSeparator(separator) {
			return true
		}
	}
	return false
}

func answerMarkdownTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || !strings.Contains(line, "-") {
		return false
	}
	line = strings.Trim(line, "|")
	cells := strings.Split(line, "|")
	valid := 0
	for _, cell := range cells {
		cell = strings.Trim(strings.TrimSpace(cell), ":")
		if len(cell) < 3 {
			return false
		}
		for _, r := range cell {
			if r != '-' {
				return false
			}
		}
		valid++
	}
	return valid >= 2
}
