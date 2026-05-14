package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRecoverAnswerDocumentV2FromText_ContentJSONDocument(t *testing.T) {
	content := `**

{
  "blocks": [
    {
      "id": "summary",
      "kind": "summary",
      "surface_role": "principal",
      "text": "Recovered summary text."
    },
    {
      "id": "diagram",
      "kind": "diagram",
      "diagram": {
        "kind": "sequence",
        "language": "mermaid",
        "body": "sequenceDiagram\nA->>B: call"
      }
    }
  ],
  "citations": [
    {"file": "internal/agent/agent.go", "line": 859}
  ]
}`

	rec, ok := RecoverAnswerDocumentV2FromText(content)
	if !ok {
		t.Fatal("expected recovery from plain assistant JSON content")
	}
	if !rec.Lossless {
		t.Fatalf("valid answer_document JSON should recover losslessly: %+v", rec)
	}
	if rec.Document == nil || len(rec.Document.Blocks) != 2 {
		t.Fatalf("recovered doc blocks = %+v", rec.Document)
	}
	if got := rec.Document.Blocks[0].Text; got != "Recovered summary text." {
		t.Fatalf("summary text = %q", got)
	}
	if rec.Document.Blocks[1].Kind != types.BlockDiagram || rec.Document.Blocks[1].Diagram == nil {
		t.Fatalf("diagram block was not preserved: %+v", rec.Document.Blocks[1])
	}
	if len(rec.Document.Citations) != 1 || rec.Document.Citations[0].File != "internal/agent/agent.go" {
		t.Fatalf("citations not preserved: %+v", rec.Document.Citations)
	}
}

func TestRecoverAnswerDocumentV2FromText_VisibleFallbackForInvalidBlockShape(t *testing.T) {
	content := `{
  "blocks": [
    {
      "id": "",
      "kind": "not_a_kind",
      "title": "Recovered Section",
      "text": "Visible block text should survive.",
      "items": [
        {"label": "First", "text": "Item text", "candidate_role": "not_a_role"}
      ]
    }
  ]
}`

	rec, ok := RecoverAnswerDocumentV2FromText(content)
	if !ok {
		t.Fatal("expected visible fallback recovery")
	}
	if rec.Lossless {
		t.Fatalf("invalid block shape must be marked non-lossless: %+v", rec)
	}
	if rec.Document == nil || len(rec.Document.Blocks) != 1 {
		t.Fatalf("recovered doc blocks = %+v", rec.Document)
	}
	blk := rec.Document.Blocks[0]
	if blk.Kind != types.BlockSection {
		t.Fatalf("invalid kind should fall back to section, got %q", blk.Kind)
	}
	if !strings.Contains(blk.Text, "Visible block text should survive") {
		t.Fatalf("visible text lost: %+v", blk)
	}
	if len(blk.Items) != 1 || blk.Items[0].Label != "First" || blk.Items[0].Text != "Item text" {
		t.Fatalf("visible items lost: %+v", blk.Items)
	}
	if strings.TrimSpace(blk.ID) == "" {
		t.Fatalf("visible fallback should synthesize an id: %+v", blk)
	}
}

func TestRecoverAnswerDocumentV2FromText_UnwrapsSerializedToolCallArguments(t *testing.T) {
	content := `{"name":"emit_answer_document","arguments":"{\"blocks\":[{\"id\":\"s\",\"kind\":\"summary\",\"text\":\"wrapped argument text\"}]}"}`
	rec, ok := RecoverAnswerDocumentV2FromText(content)
	if !ok {
		t.Fatal("expected recovery from serialized tool-call arguments")
	}
	if rec.Document == nil || len(rec.Document.Blocks) != 1 {
		t.Fatalf("recovered doc blocks = %+v", rec.Document)
	}
	if got := rec.Document.Blocks[0].Text; got != "wrapped argument text" {
		t.Fatalf("summary text = %q", got)
	}
	if !strings.Contains(rec.Mode, "nested") {
		t.Fatalf("recovery mode should record nested argument unwrap, got %q", rec.Mode)
	}
}
