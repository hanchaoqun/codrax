package render_test

import (
	"html"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSequenceNoteRepairReachesEveryAnswerSurface(t *testing.T) {
	raw, err := os.ReadFile("../mermaidcompat/testdata/sequence_multiline_notes.mmd")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	want := mermaidcompat.NormalizeSourceForMarkdown(body)
	if want == body || strings.Count(want, "<br/>") != 5 || len(mermaidcompat.ParseEdges(want)) != 0 {
		t.Fatal("fixture should repair exactly five notes without inventing arrows")
	}
	fence := "```mermaid\n" + body + "```\n"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "diagram", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: body}}}}
	// Both a typed diagram and a prose fence must persist the same portable
	// source. A display repair must not mutate the model's structured evidence.
	markdown := render.RenderAnswerDocument(doc, "zh")
	if !strings.Contains(markdown, strings.TrimSpace(want)) || doc.Blocks[0].Diagram.Body != body {
		t.Fatalf("typed document repair missing or mutated evidence: %s", markdown)
	}
	for _, input := range []string{fence, markdown} {
		browser, err := preview.RenderMarkdownHTML([]byte(input))
		if err != nil || !strings.Contains(browser, html.EscapeString(strings.TrimSpace(want))) {
			t.Fatalf("browser does not share source repair: %v %s", err, browser)
		}
		standalone, err := preview.RenderStandaloneMarkdownHTML("Note repair", []byte(input))
		if err != nil || !strings.Contains(standalone, html.EscapeString(strings.TrimSpace(want))) {
			t.Fatalf("saved HTML does not share source repair: %v", err)
		}
		terminal := render.RenderMermaidBlocks(input)
		if !strings.Contains(terminal, "```text") || strings.Contains(terminal, "# ⚠") {
			t.Fatalf("terminal repair failed: %s", terminal)
		}
		for _, label := range []string{"loader-2", "target-41", "worker-3", "S态睡眠", "可运行", "IO等待", "caller=io_schedule"} {
			if !strings.Contains(terminal, label) {
				t.Fatalf("terminal lost note/participant %q: %s", label, terminal)
			}
		}
	}
}
