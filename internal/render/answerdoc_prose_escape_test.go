package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocumentRecoversLiteralParagraphEscapesOnlyInProse(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: `first\n\nsecond`},
			{ID: "scalar", Kind: types.BlockScalar, Title: "literal", Text: `scalar\n\nvalue`},
			{
				ID:   "diagram",
				Kind: types.BlockDiagram,
				Text: `caption\n\ncontinued`,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramFlow,
					Language: "mermaid",
					Body: `flowchart LR
  A["node\n\nlabel"] --> B`,
				},
			},
		},
		Snippets: []types.CodeSnippet{{File: "example.txt", Language: "text", Code: `code\n\nvalue`}},
	}

	out := RenderAnswerDocument(doc, "en")
	for _, want := range []string{"first\n\nsecond", "caption\n\ncontinued"} {
		if !strings.Contains(out, want) {
			t.Fatalf("prose paragraph escape was not recovered (%q):\n%s", want, out)
		}
	}
	for _, want := range []string{`scalar\n\nvalue`, `node<br/><br/>label`, `code\n\nvalue`} {
		if !strings.Contains(out, want) {
			t.Fatalf("non-prose literal was rewritten (%q):\n%s", want, out)
		}
	}
}

func TestNormalizeLiteralParagraphEscapesInProsePreservesCode(t *testing.T) {
	in := "before\\n\\nafter `inline\\n\\ncode`\n\n```text\nfenced\\n\\ncode\n```"
	out := normalizeLiteralParagraphEscapesInProse(in)
	for _, want := range []string{"before\n\nafter", "`inline\\n\\ncode`", "fenced\\n\\ncode"} {
		if !strings.Contains(out, want) {
			t.Fatalf("unexpected prose recovery for %q:\n%s", want, out)
		}
	}
}
