package repl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
)

func TestDataMaterialExtractorWritesTextEvidenceWithJSONCompatibility(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "material.png"), []byte("not-really-an-image"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{{
			ToolCalls: []llm.ToolCall{{
				Name: dataMaterialExtractionTool.Name,
				Params: []byte(`{
					"source_path": "material.png",
					"text": "row one\nrow two",
					"confidence": 0.91,
					"notes": true
				}`),
			}},
		}},
	}
	extractor := NewDataMaterialExtractor(adapter)
	got, err := extractor.ExtractDataMaterials(context.Background(), root, filepath.Join(root, ".codrax", "data-extract"), []dataquery.NonTextRequiredMaterial{{
		Path:             "material.png",
		Kind:             "image",
		ExtractionStatus: "needs_text_extraction",
	}})
	if err != nil {
		t.Fatalf("ExtractDataMaterials: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d extraction(s), want 1", len(got))
	}
	if got[0].TextPath == "" || got[0].Confidence != "0.91" || got[0].Notes != "true" {
		t.Fatalf("unexpected extraction: %+v", got[0])
	}
	raw, err := os.ReadFile(filepath.Join(root, got[0].TextPath))
	if err != nil {
		t.Fatalf("read extracted text: %v", err)
	}
	if !strings.Contains(string(raw), "row one\nrow two") {
		t.Fatalf("extracted text not written: %q", raw)
	}
	if len(adapter.calls) != 1 || len(adapter.calls[0].messages) < 2 {
		t.Fatalf("adapter calls=%+v", adapter.calls)
	}
	userMsg := adapter.calls[0].messages[1]
	if len(userMsg.ContentParts) != 1 || userMsg.ContentParts[0].Type != "image_url" || !strings.HasPrefix(userMsg.ContentParts[0].ImageURL, "data:image/png;base64,") {
		t.Fatalf("missing multimodal content part: %+v", userMsg.ContentParts)
	}
}
