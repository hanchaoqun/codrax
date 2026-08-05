package types

import (
	"strings"
	"testing"
)

func TestAnswerBlockVisibleSurface_MarkdownTableExcludesHiddenCitationSidecars(t *testing.T) {
	block := AnswerBlock{
		Kind: BlockTable,
		Text: "| Category | Count |\n|---|---|\n| Types | 2 |",
		Items: []AnswerBlockItem{
			{Label: "KindAlpha", CitationRef: 0},
			{Label: "KindBeta", CitationRef: 1},
		},
	}

	surface := AnswerBlockVisibleSurface(block)
	if !strings.Contains(surface, "| Types | 2 |") {
		t.Fatalf("visible Markdown carrier missing from surface: %q", surface)
	}
	for _, hidden := range []string{"KindAlpha", "KindBeta"} {
		if strings.Contains(surface, hidden) {
			t.Fatalf("renderer-hidden citation sidecar %q leaked into visible surface: %q", hidden, surface)
		}
	}
	if AnswerBlockRendersStructuredItems(block) {
		t.Fatal("authored Markdown table must be the sole visible row carrier")
	}
}

func TestAnswerBlockVisibleSurface_StructuredTableIncludesRows(t *testing.T) {
	block := AnswerBlock{
		Kind:    BlockTable,
		Columns: []string{"Member", "Kind"},
		Items: []AnswerBlockItem{
			{Cells: []string{"KindAlpha", "type"}},
		},
	}

	surface := AnswerBlockVisibleSurface(block)
	if !AnswerBlockRendersStructuredItems(block) || !strings.Contains(surface, "KindAlpha") {
		t.Fatalf("structured table rows must remain visible: %q", surface)
	}
}
