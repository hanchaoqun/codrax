package render

import (
	"os"
	"regexp"
	"sort"
	"strconv"
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
	emittedBoundaries := []string{
		"**引用**：",
		"**Citations:**",
		"**关键代码**：",
		"**Key snippets:**",
		"> **系统保留内容**",
		"> **System-preserved content**",
	}
	for _, boundary := range emittedBoundaries {
		if !strings.Contains(zh+en, boundary) {
			t.Fatalf("contract boundary is not emitted by the renderer: %q", boundary)
		}
		if !strings.Contains(function, boundary) {
			t.Fatalf("scope_primary_stdout does not stop at renderer boundary %q:\n%s", boundary, function)
		}
	}

	// Pin the reverse direction as well: the runner cannot silently add a
	// made-up exact heading that no production answer surface owns. The two
	// raw-final headings are legacy recovery surfaces emitted by the agent
	// evaluator rather than RenderAnswerDocument, so keep them in this closed
	// set explicitly. Trace projection uses the regex arm above and has its own
	// projection contract.
	wantRunnerLiterals := append(append([]string(nil), emittedBoundaries...),
		"**模型最后一轮原文：**",
		"**Raw final model text:**",
	)
	sort.Strings(wantRunnerLiterals)
	literalPattern := regexp.MustCompile(`\$0 == ("(?:\\.|[^"\\])*")`)
	var gotRunnerLiterals []string
	for _, match := range literalPattern.FindAllStringSubmatch(function, -1) {
		literal, err := strconv.Unquote(match[1])
		if err != nil {
			t.Fatalf("decode runner boundary %q: %v", match[1], err)
		}
		gotRunnerLiterals = append(gotRunnerLiterals, literal)
	}
	sort.Strings(gotRunnerLiterals)
	if strings.Join(gotRunnerLiterals, "\x00") != strings.Join(wantRunnerLiterals, "\x00") {
		t.Fatalf("scope_primary_stdout exact boundaries drifted from production-owned closed set:\n got=%q\nwant=%q",
			gotRunnerLiterals, wantRunnerLiterals)
	}
}
