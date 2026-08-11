package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func discoverySelectionCompletionContext(evidence []types.EvidenceItem) *types.BusContext {
	mut := types.NewMutableState("opaque discover-sink request")
	mut.AppendEvidence(evidence)
	return &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentTrace,
				PredicateAxis: types.AxisCall,
				Predicates:    types.SemanticPredicates{IsCrossComponent: true},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
				CallChainEndpointProfile: &types.CallChainEndpointProfile{
					Source: "Logger.log", SinkMode: types.CallChainSinkResolutionDiscover,
				},
			},
			AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: false}},
		},
	}
}

func discoverySelectionTestEvidence(anchor types.AnchorKind, subject, object string, line int) types.EvidenceItem {
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, AnchorKind: anchor,
		Subject: subject, Object: object, AnchorSymbol: object,
		Source: "src/logger.cpp", LineStart: line, Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}
	if anchor == types.AnchorAssignment || anchor == types.AnchorInitializer {
		item.Snippet = subject + " = " + object
	}
	return item
}

func discoverySelectionCompletionParams(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"reason":     "static call path and bounded runtime selection evidence were investigated",
		"confidence": "high", "result_kind": "resolved",
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func discoverySelectionCompletionParamsWithFormDebt(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"reason": "static call path investigated", "confidence": "high", "result_kind": "resolved",
		"aggregate_facts": []map[string]any{{
			"kind": "member_set", "label": "code members", "value": "1",
			"provenance":   "emit_investigation_complete.aggregate_facts",
			"dimensions":   []map[string]string{{"name": "proof_source", "value": "source"}},
			"members":      []string{"Gate.Run (8 checks)"},
			"member_notes": []string{"unrelated decorated member"},
			"support_refs": []string{"src/logger.cpp:36"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestEmitInvestigationComplete_DiscoverSinkRequestsOneTypedSelectionRepair(t *testing.T) {
	call := discoverySelectionTestEvidence(types.AnchorCall, "Logger.log", "sink_->write", 36)
	ctx := discoverySelectionCompletionContext([]types.EvidenceItem{call})
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, discoverySelectionCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || res.Repair == nil || res.Repair.Code != "call_chain_discovery_selection_evidence" {
		t.Fatalf("missing typed selection should produce one surgical successful downgrade: %+v", res)
	}
	if ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("first missing-selection attempt must not mark investigation complete")
	}
	if repairs := ctx.Mutable.EvidenceClosure().ActiveRepairs(); len(repairs) != 1 ||
		repairs[0].DowngradeLane != types.DowngradeLaneCallChainDiscoverySelection ||
		len(repairs[0].Tools) != 1 || repairs[0].Tools[0] != "emit_evidence" {
		t.Fatalf("repair must remain typed and emit-only: %+v", repairs)
	}
}

func TestEmitInvestigationComplete_DiscoverPathExplicitSelectionRequestsTypedEvidence(t *testing.T) {
	call := discoverySelectionTestEvidence(types.AnchorCall, "make_sink", "SinkRegistry.create", 32)
	ctx := discoverySelectionCompletionContext([]types.EvidenceItem{call})
	ctx.AnalysisIR.RequestModel.CallChainEndpointProfile = &types.CallChainEndpointProfile{
		SinkMode:                    types.CallChainSinkResolutionDiscoverPath,
		RuntimeSelectionRequired:    true,
		RuntimeSelectionSourceQuote: "运行时具体的 sink 是如何被选择出来的",
	}
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, discoverySelectionCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || res.Repair == nil || res.Repair.Code != "call_chain_discovery_selection_evidence" {
		t.Fatalf("explicit discover_path selection must use the same typed evidence lane: %+v", res)
	}
}

func TestEmitInvestigationComplete_DiscoverSinkSelectionFactClosesWithoutRetryStorm(t *testing.T) {
	call := discoverySelectionTestEvidence(types.AnchorCall, "Logger.log", "sink_->write", 36)
	ctx := discoverySelectionCompletionContext([]types.EvidenceItem{call})
	tool := &EmitInvestigationComplete{}
	if _, err := tool.Execute(ctx, discoverySelectionCompletionParams(t)); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	selection := discoverySelectionTestEvidence(types.AnchorAssignment, "sink_", "ConsoleSink", 22)
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{selection})
	res, err := tool.Execute(ctx, discoverySelectionCompletionParams(t))
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() || !strings.Contains(res.Summary, "Investigation marked complete") {
		t.Fatalf("connected selection fact should close on the next attempt: %+v", res)
	}
}

func TestEmitInvestigationComplete_DiscoverSinkNoProgressConvergesWithBoundary(t *testing.T) {
	call := discoverySelectionTestEvidence(types.AnchorCall, "Logger.log", "sink_->write", 36)
	ctx := discoverySelectionCompletionContext([]types.EvidenceItem{call})
	tool := &EmitInvestigationComplete{}
	if _, err := tool.Execute(ctx, discoverySelectionCompletionParams(t)); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	res, err := tool.Execute(ctx, discoverySelectionCompletionParams(t))
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("second identical attempt should converge with disclosure, got %+v", res)
	}
	found := false
	for _, caveat := range ctx.Mutable.EvidenceClosure().CompletionCaveats() {
		if caveat.Lane == types.DowngradeLaneCallChainDiscoverySelection {
			found = true
		}
	}
	if !found || !strings.Contains(res.Summary, "runtime target selection remains unproven") {
		t.Fatalf("converged close must retain typed caveat and visible boundary: caveats=%+v note=%q",
			ctx.Mutable.EvidenceClosure().CompletionCaveats(), res.Summary)
	}
}

func TestEmitInvestigationComplete_ConvergedSelectionLaneStaysClosedAcrossLaterFormRepair(t *testing.T) {
	call := discoverySelectionTestEvidence(types.AnchorCall, "Logger.log", "sink_->write", 36)
	ctx := discoverySelectionCompletionContext([]types.EvidenceItem{call})
	tool := &EmitInvestigationComplete{}
	params := discoverySelectionCompletionParamsWithFormDebt(t)

	first, err := tool.Execute(ctx, params)
	if err != nil || first.Repair == nil || first.Repair.Code != "call_chain_discovery_selection_evidence" {
		t.Fatalf("first attempt should request selection evidence: result=%+v err=%v", first, err)
	}
	second, err := tool.Execute(ctx, params)
	if err != nil || second.Repair == nil || second.Repair.Code == "call_chain_discovery_selection_evidence" {
		t.Fatalf("second attempt should converge selection then reach form repair: result=%+v err=%v", second, err)
	}
	if !ctx.Mutable.EvidenceClosure().HasCompletionCaveat(types.DowngradeLaneCallChainDiscoverySelection) {
		t.Fatal("selection convergence caveat should persist while the later form gate repairs")
	}

	third, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("third Execute: %v", err)
	}
	if third.Repair != nil && third.Repair.Code == "call_chain_discovery_selection_evidence" {
		t.Fatalf("later form failure must not reopen the converged selection lane: %+v", third)
	}
	for _, repair := range ctx.Mutable.EvidenceClosure().ActiveRepairs() {
		if repair.DowngradeLane == types.DowngradeLaneCallChainDiscoverySelection {
			t.Fatalf("converged selection repair must not remain queued: %+v", repair)
		}
	}
}

func TestEmitInvestigationComplete_NewSelectionEvidenceUpgradesConvergedLane(t *testing.T) {
	call := discoverySelectionTestEvidence(types.AnchorCall, "Logger.log", "sink_->write", 36)
	ctx := discoverySelectionCompletionContext([]types.EvidenceItem{call})
	tool := &EmitInvestigationComplete{}
	params := discoverySelectionCompletionParamsWithFormDebt(t)
	if _, err := tool.Execute(ctx, params); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if _, err := tool.Execute(ctx, params); err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !ctx.Mutable.EvidenceClosure().HasCompletionCaveat(types.DowngradeLaneCallChainDiscoverySelection) {
		t.Fatal("fixture must first converge selection")
	}

	ctx.Mutable.AppendEvidence([]types.EvidenceItem{
		discoverySelectionTestEvidence(types.AnchorAssignment, "sink_", "ConsoleSink", 22),
	})
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("upgraded Execute: %v", err)
	}
	if ctx.Mutable.EvidenceClosure().HasCompletionCaveat(types.DowngradeLaneCallChainDiscoverySelection) {
		t.Fatalf("new precise selection evidence must clear the obsolete caveat: %+v", res)
	}
	if res.Repair != nil && res.Repair.Code == "call_chain_discovery_selection_evidence" {
		t.Fatalf("proved selection must not be requested again: %+v", res)
	}
}
