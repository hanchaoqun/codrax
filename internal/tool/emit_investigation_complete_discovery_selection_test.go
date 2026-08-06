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
	return types.EvidenceItem{
		Kind: types.EvidenceRelationship, AnchorKind: anchor,
		Subject: subject, Object: object, AnchorSymbol: object,
		Source: "src/logger.cpp", LineStart: line, Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}
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
