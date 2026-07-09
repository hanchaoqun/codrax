package render

// answerdoc_scalar_disp3_test.go — DISP-3 item8 pins (§29.8 P3 "opendir
// 反引号整段落 metric 块形+嵌套反引号破损",
// docs/design/real_trace_campaign_20260705.md, 2026-07-09; witness
// cust_trace_opendir_792.txt lines 57/59): a scalar block whose literal is a
// prose paragraph (or itself contains backticks / newlines) renders as PLAIN
// text — no system wording added, only the broken/absurd code-span markup
// withheld. True short scalars keep the backtick wrap byte-identically
// (the pre-existing TestRenderV2_BlockScalar* pins double as the
// counter-shape guard).
//
// Mutation self-checks (verified RED during development, then restored):
//   M-R1: forcing renderScalarLiteralAsCodeSpan to always return true (the
//         pre-DISP-3 unconditional wrap) → every test below red on the
//         paragraph forms.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// disp3ScalarMixedDoc wraps one scalar literal in the opendir_792 document
// shape: untitled scalar blocks between prose blocks (the line-57/59 seats).
func disp3ScalarMixedDoc(literal string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "丢帧根因已确认。"},
			{ID: "v1", Kind: types.BlockScalar, Text: literal},
		},
	}
}

func TestDisp3ScalarNestedBacktickParagraphRendersPlain(t *testing.T) {
	// opendir_792 line 59 form: the paragraph carries its own code spans —
	// wrapping it produced broken nested backticks.
	literal := "#RxComputationT-16816 持有 AssetManager.list 锁（monitor 锁）的时长。锁等待点为 `AssetManager.getResourceValue`（AssetManager.java:761），持有点为 `AssetManager.list`（AssetManager.java:1258）。该值 112.223ms 是主线程阻塞的直接原因。"
	out := RenderAnswerDocument(disp3ScalarMixedDoc(literal), "zh")
	if !strings.Contains(out, literal) {
		t.Fatalf("the model's bytes must pass through unchanged:\n%s", out)
	}
	if strings.Contains(out, "`#RxComputationT") || strings.Contains(out, "原因。`") {
		t.Fatalf("a literal containing backticks must not be re-wrapped (nested code spans break):\n%s", out)
	}
	// The inner code spans stay verbatim (they are the model's own markup).
	if !strings.Contains(out, "`AssetManager.getResourceValue`") {
		t.Fatalf("the model's own inner code spans must survive verbatim:\n%s", out)
	}
}

func TestDisp3ScalarCJKSentenceParagraphRendersPlain(t *testing.T) {
	// opendir_792 line 57 form: whole metric-explanation paragraph, no inner
	// backticks — the CJK sentence enders mark it as prose.
	literal := "onVsync 信号触发时间（33872.288680s）到主线程实际开始执行 doFrame（33872.403888s）之间的时间差。该值 115.208ms 是主线程错失Vsync 的直接度量。"
	out := RenderAnswerDocument(disp3ScalarMixedDoc(literal), "zh")
	if !strings.Contains(out, literal) {
		t.Fatalf("plain paragraph must render verbatim:\n%s", out)
	}
	if strings.Contains(out, "`onVsync") || strings.Contains(out, "度量。`") {
		t.Fatalf("a prose paragraph must not render as a code span:\n%s", out)
	}
}

func TestDisp3ScalarNewlineAndASCIISentenceRenderPlain(t *testing.T) {
	multiline := "first line\nsecond line"
	out := RenderAnswerDocument(disp3ScalarMixedDoc(multiline), "en")
	if strings.Contains(out, "`first line") {
		t.Fatalf("a multi-line literal cannot live inside a code span:\n%s", out)
	}
	prose := "The value 115.208ms is the direct measure. The rest is scheduling overhead."
	out = RenderAnswerDocument(disp3ScalarMixedDoc(prose), "en")
	if strings.Contains(out, "`The value") {
		t.Fatalf("ASCII prose (sentence break \". \") must render plain:\n%s", out)
	}
	if !strings.Contains(out, prose) {
		t.Fatalf("prose bytes must pass through unchanged:\n%s", out)
	}
}

func TestDisp3ScalarTrueScalarsKeepCodeSpan(t *testing.T) {
	// Counter-shapes: numbers (decimals included — '.' not followed by a
	// space), identifiers and paths keep the backtick wrap byte-identically,
	// on BOTH the untitled-mixed arm and the titled arm.
	for _, literal := range []string{"42", "3.14", "internal/tool/builtin.go", "MaxRetriesPerStage=4"} {
		out := RenderAnswerDocument(disp3ScalarMixedDoc(literal), "en")
		if !strings.Contains(out, "`"+literal+"`") {
			t.Fatalf("true scalar %q must keep its code span:\n%s", literal, out)
		}
	}
	titled := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID: "v1", Kind: types.BlockScalar, Title: "重试上限", Text: "4",
		}},
	}
	out := RenderAnswerDocument(titled, "zh")
	if !strings.Contains(out, "**重试上限：** `4`") {
		t.Fatalf("titled true scalar keeps the labeled code span:\n%s", out)
	}
	// Titled arm with a prose literal: label stays, wrap withheld.
	titled.Blocks[0].Text = "该值 115.208ms 是直接度量。"
	out = RenderAnswerDocument(titled, "zh")
	if !strings.Contains(out, "**重试上限：** 该值 115.208ms 是直接度量。") {
		t.Fatalf("titled prose literal renders plain beside its label:\n%s", out)
	}
	if strings.Contains(out, "`该值") {
		t.Fatalf("titled prose literal must not wrap:\n%s", out)
	}
}
