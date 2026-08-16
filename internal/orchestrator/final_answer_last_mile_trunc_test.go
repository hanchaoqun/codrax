package orchestrator

// TRUNC 批 (P1, §29.10-1, 2026-07-09) — huadong_792 witness pins,损失②:
// 「系统补充：trace_query 关键观测核对」整块丢失。
//
// 机理:finalizer parseOutputV2 产出的 FinalAnswer 携带 last-mile 系统补充
// (renderAnswerDocumentWithLastMileSupplements);orchestrator 的重渲染路径
// (attachFirstDraftReference→appendAnswerDisplayAttachment / finalizer
// auto-repair / transient-failure recovery)曾直接调
// render.RenderAnswerDocumentWithAttachments 覆写 FinalAnswer——系统补充
// 全部静默蒸发。huadong 是四场景唯一走第一稿保留重渲染的,故唯一丢块。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// truncObservationMutable builds the minimal trace_query observation fixture
// (same shape as the agent package's PTV5 supplement pins) so the finalize
// last-mile pipeline deterministically produces the 系统补充 block.
func truncObservationMutable(t *testing.T) *types.MutableState {
	t.Helper()
	mu := types.NewMutableState("")
	var obs []types.ObservationRecord
	for i := 0; i < 3; i++ {
		obs = append(obs, types.ObservationRecord{
			ID:              fmt.Sprintf("trace_query:trunc:%d", i),
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt"},
			Span:            types.ObservationSpan{LineStart: 100 + i*10, LineEnd: 105 + i*10},
			ClaimKey:        fmt.Sprintf("state_drilldown:t%d:S", i),
			Subject:         fmt.Sprintf("t%d-1", i),
			Predicate:       "state_drilldown",
			Object:          "S",
			Value:           "21.000",
			Unit:            "ms",
			RichNotes:       []string{"source=top_sleep"},
			SupportRefs:     []string{fmt.Sprintf("attached_trace.txt:%d-%d", 100+i*10, 105+i*10)},
		})
	}
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: obs,
	}}})
	return mu
}

const truncOrchTailSentinel = "尾部完整性哨兵TRUNCTAIL第一稿收束"

func truncOrchDraftBody(n int) string {
	line := "第一稿根因段:优先级反转候选,供给折算缺口按大核满频折算,下界,单列口径不计入有效归因。\n"
	var b strings.Builder
	for len([]rune(b.String())) < n {
		b.WriteString(line)
	}
	b.WriteString(truncOrchTailSentinel)
	return b.String()
}

// TestTRUNCAttachFirstDraftReferencePreservesLastMileSupplements is the
// huadong_792/B882 witness-shape repro: oracle 拒稿→重写后，accepted
// structured answer 必须仍含系统补充块，但 rejected 第一稿已降为
// retry telemetry，不得再以整页附件重复发布。
func TestTRUNCAttachFirstDraftReferencePreservesLastMileSupplements(t *testing.T) {
	mut := truncObservationMutable(t)
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "最终稿主体。",
		}},
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Language: "zh",
			Mutable:  mut,
		},
	}
	out := &agent.StageOutput{FinalAnswer: "最终稿主体。\n\n---\n\n> **系统补充：trace_query 关键观测核对**\n>\n> (seeded)"}
	o.attachFirstDraftReference(out, truncOrchDraftBody(40000), []types.Violation{{Kind: types.ViolMustInclude}}, true, nil)

	// P3-1 加固:Contains 对补充块/面板双重追加形不敏感——升级为恰一次
	// 计数断言(0=蒸发即 witness 损失②;>1=文本追加叠加形)。
	if got := strings.Count(out.FinalAnswer, truncSupplementTitle); got != 1 {
		t.Fatalf("last-mile trace_query supplement must appear EXACTLY once after first-draft re-render, got %d (0 = huadong_792 loss #2; >1 = text-append duplication)", got)
	}
	if got := strings.Count(out.FinalAnswer, truncDraftPanelTitle); got != 0 {
		t.Fatalf("accepted structured answer must not repeat rejected first-draft telemetry, got %d:\n%.400s", got, out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "最终稿主体。") {
		t.Fatalf("structured body missing:\n%.400s", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, truncOrchTailSentinel) {
		t.Fatalf("rejected draft body escaped behind the accepted structured answer")
	}
	if strings.Contains(out.FinalAnswer, "\n...\n") {
		t.Fatalf("bare \"...\" truncation marker resurfaced in final answer")
	}
}

const (
	truncSupplementTitle = "系统补充：trace_query 关键观测核对"
	truncDraftPanelTitle = "第一稿答案（校验前参考）"
)

// truncSupplementSegment extracts the trace_query supplement block (from its
// title line to the end of its blockquote run) for byte-drift comparison
// across overwrite rounds. Requires the title to appear exactly once.
func truncSupplementSegment(t *testing.T, answer string) string {
	t.Helper()
	if got := strings.Count(answer, truncSupplementTitle); got != 1 {
		t.Fatalf("supplement title count = %d, want exactly 1:\n%.400s", got, answer)
	}
	idx := strings.Index(answer, truncSupplementTitle)
	seg := answer[idx:]
	// The supplement is a contiguous "> " blockquote run; cut at the first
	// line that leaves the quote (or take to end-of-answer).
	lines := strings.Split(seg, "\n")
	end := len(lines)
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" && !strings.HasPrefix(trimmed, ">") {
			end = i
			break
		}
	}
	return strings.TrimRight(strings.Join(lines[:end], "\n"), "\n")
}

// TestTRUNCFourRoundOverwriteSupplementStable(P3-1 收编复核 4 轮覆写形):
// attach被拒第一稿→系统评审注→两轮 finalizer auto-repair 共 4 次 FinalAnswer 覆写,
// 系统补充块每轮恰一次且内容跨轮逐字节零漂移——咽喉若退化为文本追加形
// (在旧 FinalAnswer 上叠加渲染),Count==1 立即咬红。
//
// typed accepted-doc 边界下，被拒模型第一稿始终为零；独立系统评审注
// 从轮2开始始终恰一份。此处 pin "覆写不叠加、不复制，且 source role 不混淆"。
func TestTRUNCFourRoundOverwriteSupplementStable(t *testing.T) {
	mut := truncObservationMutable(t)
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "最终稿主体。",
		}},
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Language: "zh",
			Mutable:  mut,
		},
	}
	out := &agent.StageOutput{FinalAnswer: "最终稿主体。"}

	// Round 1: first-draft attachment (the huadong_792 witness path).
	o.attachFirstDraftReference(out, truncOrchDraftBody(20000), []types.Violation{{Kind: types.ViolMustInclude}}, true, nil)
	seg1 := truncSupplementSegment(t, out.FinalAnswer)
	if got := strings.Count(out.FinalAnswer, truncDraftPanelTitle); got != 0 {
		t.Fatalf("round 1: rejected draft panel title count = %d, want 0", got)
	}

	// Round 2: draft review note (second attachment, same overwrite path).
	o.attachDraftReviewNote(out, "", []types.Violation{{Kind: types.ViolCitation}})
	seg2 := truncSupplementSegment(t, out.FinalAnswer)
	if got := strings.Count(out.FinalAnswer, truncDraftPanelTitle); got != 0 {
		t.Fatalf("round 2: rejected draft panel title count = %d, want 0", got)
	}
	if got := strings.Count(out.FinalAnswer, draftReviewNoteTitle("zh")); got != 1 {
		t.Fatalf("round 2: system review attachment count = %d, want 1", got)
	}

	// Rounds 3+4: two deterministic auto-repair overwrites (facet metadata
	// repairs, each mutating the doc so the overwrite actually fires).
	for round, facet := range []string{"current_code_path", "resolved_literal_or_symbol"} {
		repaired := o.tryAutoRepairFinalizerAnswerDocument(out, []types.Violation{{
			Kind:   types.ViolFacetUncovered,
			Detail: fmt.Sprintf(`typed evidence is evidence-sufficient but required facet "%s" is not declared`, facet),
		}})
		if !repaired {
			t.Fatalf("round %d: auto-repair overwrite did not fire for facet %s", round+3, facet)
		}
		seg := truncSupplementSegment(t, out.FinalAnswer)
		if seg != seg1 {
			t.Fatalf("round %d: supplement block drifted byte-wise across overwrites:\nfirst: %q\nnow:   %q", round+3, seg1, seg)
		}
		if got := strings.Count(out.FinalAnswer, truncDraftPanelTitle); got != 0 {
			t.Fatalf("round %d: rejected draft panel title count = %d, want 0", round+3, got)
		}
		if got := strings.Count(out.FinalAnswer, draftReviewNoteTitle("zh")); got != 1 {
			t.Fatalf("round %d: system review attachment count = %d, want 1", round+3, got)
		}
	}
	if seg2 != seg1 {
		t.Fatalf("round 2: supplement block drifted byte-wise:\nround1: %q\nround2: %q", seg1, seg2)
	}
}

// TestTRUNCFinalAnswerRerenderChokepointStructural: 突变 pin——orchestrator
// 内除 contract_check.go(校验面,非客户面)外,任何非测试文件不得直接调用
// render.RenderAnswerDocumentWithAttachments 覆写客户面答案;客户面重渲染
// 必须走 renderFinalAnswerWithLastMileSupplements 单一咽喉,否则系统补充
// 会再次静默蒸发。
func TestTRUNCFinalAnswerRerenderChokepointStructural(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	allowed := map[string]bool{
		"contract_check.go": true, // checker-facing FullMarkdown projection, not the customer answer surface
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(raw), "render.RenderAnswerDocumentWithAttachments(") && !allowed[name] {
			t.Fatalf("%s calls render.RenderAnswerDocumentWithAttachments directly — customer-facing FinalAnswer re-renders MUST go through renderFinalAnswerWithLastMileSupplements (TRUNC §29.10-1: direct re-render drops 系统补充 blocks)", name)
		}
	}
}
