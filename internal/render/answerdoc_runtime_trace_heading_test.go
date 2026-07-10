package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTraceSystemBlocksRenderAsNavigableChapters(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "runtime_trace_causal_projection", Kind: types.BlockSection,
			Title: "Trace 因果投影", Text: "tree",
			SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace,
		},
		{
			ID: "runtime_trace_semantic_optimizations", Kind: types.BlockTable,
			Title: "确定性优化点", Text: "typed optimization",
			SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace,
		},
		{
			ID: "runtime_trace_causal_projection_evidence", Kind: types.BlockBulletList,
			Title: "证据索引", Items: []types.AnswerBlockItem{{Text: "E1"}},
			SystemGeneratedKind: types.AnswerSystemGeneratedRuntimeTrace,
		},
	}}

	got := RenderAnswerDocument(doc, "zh-CN")
	for _, heading := range []string{"## Trace 因果投影", "## 确定性优化点", "## 证据索引"} {
		if !strings.Contains(got, heading) {
			t.Fatalf("authenticated runtime-trace block missing chapter heading %q:\n%s", heading, got)
		}
	}
	if strings.Contains(got, "### Trace 因果投影") || strings.Contains(got, "**确定性优化点**") || strings.Contains(got, "**证据索引**") {
		t.Fatalf("runtime-trace blocks retained visually weak legacy headings:\n%s", got)
	}
}

func TestModelBlocksCannotSelfPromoteByRuntimeTraceTitleOrID(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "runtime_trace_causal_projection", Kind: types.BlockSection, Title: "Trace 因果投影", Text: "model"},
		{ID: "runtime_trace_semantic_optimizations", Kind: types.BlockTable, Title: "确定性优化点", Text: "model"},
	}}

	got := RenderAnswerDocument(doc, "zh-CN")
	if !strings.Contains(got, "### Trace 因果投影") || !strings.Contains(got, "**确定性优化点**") {
		t.Fatalf("model-authored lookalikes must retain ordinary heading levels:\n%s", got)
	}
	if strings.Contains("\n"+got, "\n## Trace 因果投影\n") ||
		strings.Contains("\n"+got, "\n## 确定性优化点\n") {
		t.Fatalf("reserved title/id alone promoted a model block:\n%s", got)
	}
}
