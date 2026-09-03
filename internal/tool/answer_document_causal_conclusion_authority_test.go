package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTypedSeatFrameCausalityAddsQualifierWithoutDecrowning(t *testing.T) {
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
	if projection == nil || !strings.Contains(projection.Text, "**主根因(=已证链上单项最大可消除量):**") ||
		!strings.Contains(projection.Text, "（帧因果未证）") {
		t.Fatalf("seat-level frame authority did not preserve the defined crown and add its qualifier:\n%+v", projection)
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
	for _, forbidden := range []string{"首要可消除候选", "Leading eliminable candidate", "leading eliminable candidate"} {
		if strings.Contains(rendered.String(), forbidden) {
			t.Fatalf("retired de-crowning wording %q was emitted:\n%s", forbidden, rendered.String())
		}
	}
	for _, want := range []string{"➊..➎按可消除影响排序", "主根因(优先处理;帧因果未证)", "当前项目的帧因果尚未证明"} {
		if !strings.Contains(rendered.String(), want) {
			t.Fatalf("seat-level qualifier was not kept consistent on system surfaces, missing %q:\n%s", want, rendered.String())
		}
	}
}

func TestEarlierUnprovenProbeCannotDecrownLaterProvenSeat(t *testing.T) {
	ctx := traceAuthorityWiringContext("zh")
	// An exploratory miss is honest for that query but owns no observation
	// consumed by the final elected seat.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		RawRef:   "probe",
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:             "wakeup_chain",
			CausalConclusion: "unproven",
		},
	})
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
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName:     "trace_query",
		Success:      true,
		RawRef:       "proven",
		Observations: traceQueryTypedObservations(result, "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC()),
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                "root_cause_rank",
			TypedCausalRowCount: 1,
			CausalConclusion:    "bounded_by_typed_rows",
		},
	})
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "model_summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "keep model conclusion",
	}}}
	res, err := ApplyAndPersistMutation(ctx, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist failed: result=%+v err=%v", res, err)
	}
	projection := projectionClusterBlock(ctx.Mutable.AnswerDocumentV2().Blocks, runtimeTraceCausalProjectionBlockIDBase)
	if projection == nil {
		t.Fatalf("projection missing: %+v", ctx.Mutable.AnswerDocumentV2().Blocks)
	}
	firstLine := strings.SplitN(projection.Text, "\n", 2)[0]
	if !strings.HasPrefix(firstLine, "**主根因(=已证链上单项最大可消除量):**") ||
		strings.Contains(firstLine, "帧因果未证") || strings.Contains(firstLine, "首要可消除") {
		t.Fatalf("an earlier unrelated unproven probe changed the final seat crown: %s", firstLine)
	}
}

func TestMultiArtifactSeatsKeepFrameAuthorityAndCrownWordingIsolated(t *testing.T) {
	makeResult := func(path string, start float64, pid int) tracequery.Result {
		return tracequery.Result{
			View: "root_cause_rank", SourcePath: path, TimeStart: start, TimeEnd: start + 0.020,
			RootCauseRank: &tracequery.RootCauseRankResult{
				Window: tracequery.TimeWindow{StartTs: start, EndTs: start + 0.020},
				Items: []tracequery.RootCauseRankItem{{
					Rank: 1, Tier: "primary", Type: "binder_wait",
					Thread:   tracequery.ThreadRef{Comm: "worker", PID: pid},
					ImpactMs: 8, CumulativeImpactMs: 8, EffectiveImpactMs: 7, RunnableMs: 7,
					ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.9,
				}},
			},
		}
	}
	unproven := makeResult("a.systrace", 10, 200)
	proven := makeResult("b.systrace", 20, 300)
	input := types.ObservationLedgerInput{RequestModel: traceAuthorityFrameQuestionRequestModel(), ToolResults: []types.ToolResult{
		{
			ToolName: "trace_query", Success: true,
			Observations: traceQueryTypedObservations(unproven, unproven.SourcePath, "payload-a", "raw-a", "", time.Unix(0, 0).UTC()),
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
				View: "frame_root_cause_bundle", FrameEvidenceStatus: "absent",
				TypedCausalRowCount: 1, CausalConclusion: "unproven",
			},
		},
		{
			ToolName: "trace_query", Success: true,
			Observations: traceQueryTypedObservations(proven, proven.SourcePath, "payload-b", "raw-b", "", time.Unix(0, 0).UTC()),
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
				View: "root_cause_rank", TypedCausalRowCount: 1, CausalConclusion: "bounded_by_typed_rows",
			},
		},
	}}
	ledger := types.CompileObservationLedger(input)
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 2 {
		t.Fatalf("projection count=%d, want 2: %+v", len(set.Projections), set)
	}
	blocks := runtimeTraceCausalProjectionMultiCluster(
		set, ledger, nil, "zh", runtimeTraceProjUserFocus{}, buildRuntimeTraceProjectionSeatAuthorityIndex(input),
	)
	leads := 0
	unprovenLeads := 0
	leadIDs := map[string]bool{
		runtimeTraceCausalProjectionBlockIDBase + runtimeTraceCausalProjectionArtifactBlockIDInfix + "1": true,
		runtimeTraceCausalProjectionBlockIDBase + runtimeTraceCausalProjectionArtifactBlockIDInfix + "2": true,
	}
	for _, block := range blocks {
		if !leadIDs[block.ID] {
			continue
		}
		if !strings.HasPrefix(block.Text, "**主根因(=已证链上单项最大可消除量):**") {
			t.Fatalf("multi-board lead lost the single-source crown prefix: id=%s text=%s", block.ID, block.Text)
		}
		leads++
		if strings.Contains(strings.SplitN(block.Text, "\n", 2)[0], "帧因果未证") {
			unprovenLeads++
		}
	}
	if leads != 2 || unprovenLeads != 1 {
		t.Fatalf("multi-board authority leaked across seats: leads=%d unproven=%d blocks=%+v", leads, unprovenLeads, blocks)
	}
}

func TestTypedSeatFrameCausalityEnglishQualifierKeepsDefinedCrown(t *testing.T) {
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
	input := types.ObservationLedgerInput{RequestModel: traceAuthorityFrameQuestionRequestModel(), ToolResults: []types.ToolResult{{
		ToolName:     "trace_query",
		Success:      true,
		Observations: traceQueryTypedObservations(result, "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC()),
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                      "frame_root_cause_bundle",
			FrameFlowCausalConclusion: tracequery.FrameFlowCausalityUnproven,
			TypedCausalRowCount:       1,
			CausalConclusion:          "unproven",
		},
	}}}
	set := types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(input))
	if len(set.Projections) != 1 {
		t.Fatalf("projection count=%d", len(set.Projections))
	}
	blocks := runtimeTraceCausalProjectionClusterWithAuthority(
		set.Projections[0], "en", runtimeTraceProjUserFocus{}, buildRuntimeTraceProjectionSeatAuthorityIndex(input),
	)
	lead := projectionClusterBlock(blocks, runtimeTraceCausalProjectionBlockIDBase)
	if lead == nil || !strings.Contains(lead.Text, "**Primary root cause (= the largest single proven on-chain eliminable contribution):**") ||
		!strings.Contains(lead.Text, "(frame causality unproven)") {
		t.Fatalf("English crown/qualifier mismatch: %+v", lead)
	}
	for _, block := range blocks {
		wire := block.Title + "\n" + block.Text
		if strings.Contains(wire, "Leading eliminable candidate") || strings.Contains(wire, "leading eliminable candidate") {
			t.Fatalf("retired English de-crowning wording emitted: %s", wire)
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

// TestTypedSeatFrameCausalityGateClosedKeepsBareCrown — QUALGATE-1 (user
// ruling §40.30 V-QUAL-1 plan A, 2026-09-02): the SAME absent-frame typed
// authority under a request whose analyzer decision says it is NOT a frame
// question publishes no frame claim anywhere — the crown keeps its defined
// words without 「（帧因果未证）」 and the coverage boundary speaks only the
// generic causal ceiling.
func TestTypedSeatFrameCausalityGateClosedKeepsBareCrown(t *testing.T) {
	ctx := traceAuthorityWiringContext("zh")
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FrameCausalityRequested = false
	result := tracequery.Result{
		View: "root_cause_rank", SourcePath: "customer.systrace", TimeStart: 10, TimeEnd: 10.020,
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 10, EndTs: 10.020},
			Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Tier: "primary", Type: "binder_wait",
				Thread:   tracequery.ThreadRef{Comm: "worker", PID: 200},
				ImpactMs: 8, CumulativeImpactMs: 8, EffectiveImpactMs: 7, RunnableMs: 7,
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", Confidence: 0.9,
			}},
		},
	}
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query", Success: true,
		Observations: traceQueryTypedObservations(result, "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC()),
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View: "frame_root_cause_bundle", FrameEvidenceStatus: "absent", TypedCausalRowCount: 1, CausalConclusion: "unproven",
		},
	})
	// A second, frame-flow evaluator (temporal edges only) — its frame-origin
	// verdict is equally out of scope on a non-frame question.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query", Success: true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View: "frame_flow", FrameEvidenceStatus: "present", TypedCausalRowCount: 1, CausalConclusion: "unproven",
			FrameFlowCausalConclusion: tracequery.FrameFlowCausalityUnproven, FrameFlowEdgeCount: 3,
		},
	})
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "model_summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "model conclusion"},
	}}
	res, err := ApplyAndPersistMutation(ctx, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("persist failed: result=%+v err=%v", res, err)
	}
	persisted := ctx.Mutable.AnswerDocumentV2()
	projection := projectionClusterBlock(persisted.Blocks, runtimeTraceCausalProjectionBlockIDBase)
	if projection == nil || !strings.Contains(projection.Text, "**主根因(=已证链上单项最大可消除量):**") {
		t.Fatalf("the defined crown must render: %+v", projection)
	}
	var rendered strings.Builder
	for _, block := range persisted.Blocks {
		rendered.WriteString(block.Title + "\n" + block.Text + "\n")
		for _, item := range block.Items {
			rendered.WriteString(item.Text + "\n")
		}
	}
	for _, forbidden := range []string{"帧因果未证", "帧级因果尚未证明", "帧关系证据尚不足",
		// 复核收编: the frame-origin "unproven" verdict must not turn into the
		// generic "no usable on-chain observation" sentence beneath a crown.
		"当前没有可用的链上因果观测"} {
		if strings.Contains(rendered.String(), forbidden) {
			t.Fatalf("gate closed: no surface may make a frame claim or deny the published on-chain observation (%q):\n%s", forbidden, rendered.String())
		}
	}
}
