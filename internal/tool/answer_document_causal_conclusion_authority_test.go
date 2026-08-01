package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTypedUnprovenCausalityPreservesModelConclusionAndAddsProjection(t *testing.T) {
	ctx := traceAuthorityWiringContext("zh")
	result := tracequery.Result{
		View:       "root_cause_rank",
		SourcePath: "customer.systrace",
		TimeStart:  10,
		TimeEnd:    10.020,
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 10, EndTs: 10.020},
			Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Tier: "primary", Type: "binder_wait",
				Thread:             tracequery.ThreadRef{Comm: "worker", PID: 200},
				ImpactMs:           8,
				CumulativeImpactMs: 8,
				EffectiveImpactMs:  7,
				RunnableMs:         7,
				ChainRelevance:     "on_chain",
				Causality:          "on_wakeup_chain",
				Confidence:         0.9,
			}},
		},
	}
	records := traceQueryTypedObservations(result, "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName:     "trace_query",
		Success:      true,
		Observations: records,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                "frame_root_cause_bundle",
			FrameEvidenceStatus: "absent",
			TypedCausalRowCount: 1,
			CausalConclusion:    "unproven",
		},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "model_summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "arbitrary model overclaim"},
			{ID: "model_chain", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal, Items: []types.AnswerBlockItem{{ID: "x", Text: "arbitrary chain overclaim", CitationRef: -1}}},
			{ID: "model_timeline", Kind: types.BlockSection, Title: "timeline", Text: "keep this bounded context"},
		},
	}
	wantModelWire, err := modelOwnedAnswerBlockWire(doc)
	if err != nil {
		t.Fatalf("snapshot model blocks: %v", err)
	}
	res, err := ApplyAndPersistMutation(ctx, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist failed: result=%+v err=%v", res, err)
	}
	persisted := ctx.Mutable.AnswerDocumentV2()
	if persisted == nil || len(persisted.Blocks) == 0 {
		t.Fatal("persisted document missing")
	}
	if err := requireModelOwnedAnswerBlockWirePreserved(wantModelWire, persisted); err != nil {
		t.Fatalf("runtime trace system changed the model block wire: %v", err)
	}
	lead := persisted.Blocks[0]
	if lead.ID != "model_summary" || lead.Text != "arbitrary model overclaim" ||
		lead.SurfaceRole != types.SurfacePrincipal || RuntimeTraceSystemBlock(lead) {
		t.Fatalf("system must not replace the model-authored conclusion: %+v", lead)
	}
	for _, wantID := range []string{"model_summary", "model_chain", "model_timeline"} {
		if block := projectionClusterBlock(persisted.Blocks, wantID); block == nil {
			t.Fatalf("model-owned reasoning block %q was deleted under an unproven ceiling", wantID)
		}
	}
	projection := projectionClusterBlock(persisted.Blocks, runtimeTraceCausalProjectionBlockIDBase)
	if projection == nil || !strings.Contains(projection.Text, "首要可消除候选(不等于已证帧因果)") ||
		strings.Contains(projection.Text, "主根因(=已证链上单项最大可消除量)") {
		t.Fatalf("system projection retained a proven-cause crown under unproven authority:\n%+v", projection)
	}
	var rendered strings.Builder
	for _, block := range persisted.Blocks {
		rendered.WriteString(block.Title)
		rendered.WriteString("\n")
		rendered.WriteString(block.Text)
		for _, item := range block.Items {
			rendered.WriteString("\n")
			rendered.WriteString(item.Text)
		}
	}
	for _, forbidden := range []string{
		"标题主根因", "主根因=已证", "因果位置: 主根因", "对主根因",
		"headline primary root cause", "primary root cause = the seat", "causal position: primary", "Drill into the primary root cause",
	} {
		if strings.Contains(rendered.String(), forbidden) {
			t.Fatalf("unproven system surfaces retained proven-cause wording %q:\n%s", forbidden, rendered.String())
		}
	}
	for _, want := range []string{"候选前五", "首要可消除候选(优先验证;帧因果未证)"} {
		if !strings.Contains(rendered.String(), want) {
			t.Fatalf("unproven system surfaces missing authority-aware wording %q:\n%s", want, rendered.String())
		}
	}
}

func TestBoundedTypedCausalityKeepsModelPrincipal(t *testing.T) {
	ctx := traceAuthorityWiringContext("en")
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                "root_cause_rank",
			TypedCausalRowCount: 1,
			CausalConclusion:    "bounded_by_typed_rows",
		},
	})
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "model_summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "keep model principal",
	}}}
	// No post-model causal conclusion materializer exists: both unproven and
	// bounded lanes leave synthesis to the finalizer model. Deterministic trace
	// projection remains a sibling evidence surface.
	if len(doc.Blocks) != 1 || doc.Blocks[0].ID != "model_summary" {
		t.Fatalf("model principal changed: %+v", doc.Blocks)
	}
}
