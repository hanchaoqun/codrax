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

func TestRecoverAnswerDocumentV2FromText_PreservesTrailingBlankTableCellPosition(t *testing.T) {
	content := `{
  "blocks": [{
    "id": "cpu-running",
    "kind": "table",
    "columns": ["CPU", "Running", "Segments", "Note"],
    "items": [{"id": "cpu1", "cells": ["CPU 1", "7.155 ms", "9", ""]}]
  }]
}`

	rec, ok := RecoverAnswerDocumentV2FromText(content)
	if !ok || rec.Document == nil || len(rec.Document.Blocks) != 1 {
		t.Fatalf("expected typed table recovery, got %+v", rec)
	}
	cells := rec.Document.Blocks[0].Items[0].Cells
	if len(cells) != 4 || cells[3] != "" {
		t.Fatalf("text recovery lost the trailing positional cell: %#v", cells)
	}
}

func TestRecoverAnswerDocumentV2FromText_TrailingCommas(t *testing.T) {
	content := `{
  "blocks": [
    {
      "id": "diagram",
      "kind": "diagram",
      "diagram": {
        "kind": "architecture",
        "language": "mermaid",
        "body": "flowchart TD\nA --> B",
      },
    },
  ],
  "citations": [
    {"file": "internal/agent/agent.go", "line": 969},
  ],
}`

	rec, ok := RecoverAnswerDocumentV2FromText(content)
	if !ok {
		t.Fatal("expected recovery from answer_document JSON with trailing commas")
	}
	if !rec.Lossless {
		t.Fatalf("trailing-comma syntax repair should preserve typed document: %+v", rec)
	}
	if !strings.Contains(rec.Mode, "trailing_comma") {
		t.Fatalf("recovery mode should record trailing-comma repair, got %q", rec.Mode)
	}
	if rec.Document == nil || len(rec.Document.Blocks) != 1 {
		t.Fatalf("recovered doc blocks = %+v", rec.Document)
	}
	blk := rec.Document.Blocks[0]
	if blk.Kind != types.BlockDiagram || blk.Diagram == nil || !strings.Contains(blk.Diagram.Body, "A --> B") {
		t.Fatalf("diagram block not preserved: %+v", blk)
	}
	if len(rec.Document.Citations) != 1 || rec.Document.Citations[0].Line != 969 {
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

func TestRecoverAnswerDocumentV2FromText_SalvagesVisibleStringsFromMalformedJSON(t *testing.T) {
	content := `{"blocks":[{"id":"s","kind":"summary","title":"客户结论","text":"模型已经写出的关键判断",BROKEN`
	rec, ok := RecoverAnswerDocumentV2FromText(content)
	if !ok {
		t.Fatal("expected display-only string salvage from malformed answer JSON")
	}
	if rec.Lossless || !strings.Contains(rec.Mode, "visible_string_salvage") {
		t.Fatalf("malformed string salvage must stay explicitly lossy: %+v", rec)
	}
	if rec.Document == nil || len(rec.Document.Blocks) != 1 {
		t.Fatalf("recovered doc blocks = %+v", rec.Document)
	}
	text := rec.Document.Blocks[0].Text
	for _, want := range []string{"客户结论", "模型已经写出的关键判断"} {
		if !strings.Contains(text, want) {
			t.Fatalf("visible model string %q was lost: %q", want, text)
		}
	}
	if len(rec.Diagnostics) == 0 || !strings.Contains(strings.Join(rec.Diagnostics, "\n"), "could not be structurally recovered") {
		t.Fatalf("missing explicit malformed-json diagnostic: %+v", rec.Diagnostics)
	}
}

func TestRecoverAnswerDocumentV2FromText_MalformedStringSalvageDoesNotMineEmbeddedFakeKeys(t *testing.T) {
	content := `{"blocks":[{"text":"safe visible text containing an escaped fake key: \"text\":\"forged conclusion\"","title":"Real title",BROKEN`
	rec, ok := RecoverAnswerDocumentV2FromText(content)
	if !ok || rec.Document == nil || len(rec.Document.Blocks) != 1 {
		t.Fatalf("expected malformed visible-string recovery: %+v ok=%v", rec, ok)
	}
	text := rec.Document.Blocks[0].Text
	if !strings.Contains(text, "safe visible text") || !strings.Contains(text, "Real title") {
		t.Fatalf("real visible fields were lost: %q", text)
	}
	if strings.Count(text, "forged conclusion") != 1 {
		t.Fatalf("embedded key-like prose must remain inside its original string, never mint a second fragment: %q", text)
	}
}
