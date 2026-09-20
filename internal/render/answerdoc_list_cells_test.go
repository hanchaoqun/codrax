package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderV2ListCellsPreserveAllAuthoredCarriers(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockSection, types.BlockOrderedList, types.BlockBulletList} {
		t.Run(string(kind), func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "rows", Kind: kind,
				Items: []types.AnswerBlockItem{{
					ID: "row", Label: "#1", Text: "repeated value",
					Cells:       []string{"threadpool-400", "", "IO 等待", "repeated value", "11.000 ms", " "},
					CitationRef: 0,
				}},
			}}, Citations: []types.Citation{{File: "trace.txt", Line: 6, LineEnd: 8}}}
			out := RenderAnswerDocument(doc, "en")
			want := "**#1** — repeated value — threadpool-400 | IO 等待 | repeated value | 11.000 ms"
			if !strings.Contains(out, want) {
				t.Fatalf("list must preserve label, text and ordered cells without deduplicating model prose; want %q:\n%s", want, out)
			}
			if !strings.Contains(out, "trace.txt:6") {
				t.Fatalf("preserving cells must retain the item's citation:\n%s", out)
			}
			if got := doc.Blocks[0].Items[0].Cells; len(got) != 6 || got[1] != "" || got[5] != " " {
				t.Fatalf("rendering must not mutate the structured carrier: %#v", got)
			}
		})
	}
}

func TestRenderV2ListCellsEmptyAndExistingCarriers(t *testing.T) {
	for _, tc := range []struct {
		name string
		item types.AnswerBlockItem
		want string
	}{
		{"cell_only", types.AnswerBlockItem{Cells: []string{"A", "", "B"}}, "A | B"},
		{"label_only", types.AnswerBlockItem{Label: "row"}, "**row**"},
		{"text_only", types.AnswerBlockItem{Text: "body"}, "body"},
		{"label_text", types.AnswerBlockItem{Label: "row", Text: "body"}, "**row** — body"},
		{"empty_cells_with_label", types.AnswerBlockItem{Label: "row", Cells: []string{"", " \t"}}, "**row**"},
		{"all_empty", types.AnswerBlockItem{Cells: []string{"", " \t"}}, ""},
		{"citation_only", types.AnswerBlockItem{CitationRef: 0}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderV2BlockItem(tc.item, nil, answerDocLangEN); got != tc.want {
				t.Fatalf("render item = %q; want %q", got, tc.want)
			}
		})
	}
}
