package mermaidcompat

import (
	"strings"
	"testing"
)

func TestQuotedEdgeFragmentPreservesInlineNodeLabels(t *testing.T) {
	for _, label := range []string{
		"literal &amp;quot; and &amp;lt; and &amp;gt; and &amp;amp;",
		"ends in >", "ends in ;", "A --> B;", `escaped \";`,
	} {
		body := "flowchart LR\n A[\"" + label + "\"] --> B[\"sink\"]"
		if got := NormalizeFlowchartQuotedEdgeFragments(body); got != body {
			t.Fatalf("ordinary quoted node label was treated as a new edge fragment:\nwant %s\ngot %s", body, got)
		}
	}
}

func TestStandaloneNodeDeclarationsUnicodeAndInjectionBoundaries(t *testing.T) {
	for _, family := range []string{"flowchart LR", "sequenceDiagram"} {
		for _, id := range []string{"提交端", "Émetteur", "Δέκτης", "e\u0301metteur", strings.Repeat("节", 128)} {
			body := family + "\n"
			got, ok := AddRemovableNodeDeclaration(body, id, "业务参与者")
			if !ok || len(RemovableNodeDeclarations(got)) != 1 || len(ParseEdges(got)) != 0 {
				t.Fatalf("legal Unicode identity lost or invented topology: %q %q ok=%t", family, got, ok)
			}
			if retry, ok := AddRemovableNodeDeclaration(got, id, "duplicate"); ok || retry != got {
				t.Fatal("duplicate declaration must fail without mutation")
			}
		}
		for _, id := range []string{"提交\nA-->B", "A;B", "A\x00", "A B", "1提交", "\u0301A", "A\u202eB", strings.Repeat("节", 129)} {
			body := family + "\n"
			if got, ok := AddRemovableNodeDeclaration(body, id, "label"); ok || got != body {
				t.Fatalf("unsafe node identity accepted: %q", id)
			}
		}
	}
}
