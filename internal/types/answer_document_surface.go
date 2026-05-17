package types

import "strings"

// AnswerBlockItemVisibleSurface returns the user-visible textual surface of a
// list/table item. Keep this as the shared source for validator/index code so
// optional table cells are not treated as invisible metadata.
func AnswerBlockItemVisibleSurface(item AnswerBlockItem) string {
	var b strings.Builder
	appendAnswerVisibleSurface(&b, item.Label)
	appendAnswerVisibleSurface(&b, item.Text)
	for _, cell := range item.Cells {
		appendAnswerVisibleSurface(&b, cell)
	}
	return strings.TrimSpace(b.String())
}

// AnswerBlockVisibleSurface returns the visible text carried by a block,
// including structured table headers/cells. It intentionally does not include
// citations because citation refs are indexes, not user-visible answer text.
func AnswerBlockVisibleSurface(block AnswerBlock) string {
	var b strings.Builder
	appendAnswerVisibleSurface(&b, block.Title)
	appendAnswerVisibleSurface(&b, block.Text)
	for _, col := range block.Columns {
		appendAnswerVisibleSurface(&b, col)
	}
	for _, item := range block.Items {
		appendAnswerVisibleSurface(&b, AnswerBlockItemVisibleSurface(item))
	}
	if block.Diagram != nil {
		appendAnswerVisibleSurface(&b, block.Diagram.Body)
	}
	return strings.TrimSpace(b.String())
}

func appendAnswerVisibleSurface(b *strings.Builder, s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(s)
}
