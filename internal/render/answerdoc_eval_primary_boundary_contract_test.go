package render

import (
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEvalPrimaryAnswerTailBoundariesMatchRendererEmissions(t *testing.T) {
	runner, err := os.ReadFile("../../eval/run.sh")
	if err != nil {
		t.Fatalf("read eval runner: %v", err)
	}
	function := string(runner)
	start := strings.Index(function, "scope_primary_stdout() {")
	if start < 0 {
		t.Fatal("eval runner lost scope_primary_stdout")
	}
	function = function[start:]
	if end := strings.Index(function, "\n}\n"); end >= 0 {
		function = function[:end]
	}

	doc := &types.AnswerDocumentV2{
		Blocks:    []types.AnswerBlock{{ID: "answer", Kind: types.BlockSummary, Text: "answer"}},
		Citations: []types.Citation{{File: "src/a.go", Line: 1, LineEnd: 1}},
		Snippets:  []types.CodeSnippet{{File: "src/a.go", StartLine: 1, EndLine: 1, Code: "package a"}},
	}
	zh := RenderAnswerDocument(doc, "zh") + RenderAnswerDocumentWithAttachments(
		&types.AnswerDocumentV2{Blocks: doc.Blocks},
		[]types.AnswerDisplayAttachment{{Kind: types.AnswerDisplayAttachmentText, Body: "raw"}},
		"zh",
	)
	en := RenderAnswerDocument(doc, "en") + RenderAnswerDocumentWithAttachments(
		&types.AnswerDocumentV2{Blocks: doc.Blocks},
		[]types.AnswerDisplayAttachment{{Kind: types.AnswerDisplayAttachmentText, Body: "raw"}},
		"en",
	)
	for _, boundary := range []string{
		"**引用**：",
		"**Citations:**",
		"**关键代码**：",
		"**Key snippets:**",
		"> **系统保留内容**",
		"> **System-preserved content**",
	} {
		if !strings.Contains(zh+en, boundary) {
			t.Fatalf("contract boundary is not emitted by the renderer: %q", boundary)
		}
		if !strings.Contains(function, boundary) {
			t.Fatalf("scope_primary_stdout does not stop at renderer boundary %q:\n%s", boundary, function)
		}
	}
}
