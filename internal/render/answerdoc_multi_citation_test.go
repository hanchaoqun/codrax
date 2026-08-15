package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderV2ListItemShowsEveryExplicitCitationAnchor(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID: "evidence", Kind: types.BlockOrderedList, Title: "Evidence",
			Items: []types.AnswerBlockItem{{
				ID: "row", Label: "explorer", Text: "registration and returned name",
				CitationRef: 0, CitationRefs: []int{1},
			}},
		}},
		Citations: []types.Citation{
			{File: "register.go", Line: 10},
			{File: "name.go", Line: 30},
		},
	}
	got := RenderAnswerDocument(doc, "en")
	for _, want := range []string{"register.go:10", "name.go:30"} {
		if !strings.Contains(got, want) {
			t.Fatalf("render missing %q:\n%s", want, got)
		}
	}
}
