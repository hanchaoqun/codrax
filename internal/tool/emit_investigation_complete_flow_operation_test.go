package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func flowOperationCompletionContext(evidence []types.EvidenceItem) *types.BusContext {
	mut := types.NewMutableState("opaque source-flow request")
	mut.AppendEvidence(evidence)
	return &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				Scenario:      types.ScenarioArchitectureExplain,
				PredicateAxis: types.AxisFlow,
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			},
			AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: false}},
		},
	}
}

func flowOperationEvidence(anchor types.AnchorKind, subject, object string, line int) types.EvidenceItem {
	return types.EvidenceItem{
		Kind: types.EvidenceRelationship, AnchorKind: anchor,
		Subject: subject, Object: object, AnchorSymbol: object,
		Source: "src/pipeline.go", LineStart: line, Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
		Producer:        types.EvidenceProducerExplorerEmitEvidence,
	}
}

func flowOperationCompletionParams(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"reason":     "source components and their currently proven transfer boundary were investigated",
		"confidence": "high", "result_kind": "resolved",
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestEmitInvestigationComplete_FlowDefinitionsRequestOneOperationPass(t *testing.T) {
	definition := flowOperationEvidence(types.AnchorDefinition, "Pipeline", "stages", 10)
	ctx := flowOperationCompletionContext([]types.EvidenceItem{definition})
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || res.Repair == nil || res.Repair.Code != "flow_operation_carrier_evidence" {
		t.Fatalf("definition-only flow should request one focused operation pass: %+v", res)
	}
	if ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("first definition-only attempt must not mark investigation complete")
	}
	repairs := ctx.Mutable.EvidenceClosure().ActiveRepairs()
	if len(repairs) != 1 || repairs[0].DowngradeLane != types.DowngradeLaneFlowOperationCarrier {
		t.Fatalf("operation repair must carry its typed lane: %+v", repairs)
	}
}

func TestEmitInvestigationComplete_FlowOperationClosesNextAttempt(t *testing.T) {
	definition := flowOperationEvidence(types.AnchorDefinition, "Pipeline", "stages", 10)
	ctx := flowOperationCompletionContext([]types.EvidenceItem{definition})
	tool := &EmitInvestigationComplete{}
	if _, err := tool.Execute(ctx, flowOperationCompletionParams(t)); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorAssignment, "Analyzer.output", "BusContext.AnalysisIR", 42),
	})
	res, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() || !strings.Contains(res.Summary, "Investigation marked complete") {
		t.Fatalf("citable operation row should close on the next attempt: %+v", res)
	}
}

func TestEmitInvestigationComplete_FlowTypedIncidentParticipantsNeedOwnCoverage(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Pipeline.Analyze", "Pipeline.Explore", 42),
	})
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind:     types.DiagramSequence,
		Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Pipeline.Analyze", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Pipeline.Explore", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Pipeline.Finalize", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantContextOnly},
		},
	}
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if ctx.Mutable.IsInvestigationComplete() || res.Repair == nil ||
		!strings.Contains(res.Summary, "Pipeline.Finalize") ||
		strings.Contains(res.Summary, "BusContext") {
		t.Fatalf("uncovered incident participant must request its own operation evidence while context-only stays exempt: %+v", res)
	}

	ctx.Mutable.AppendEvidence([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Pipeline.Explore", "Pipeline.Finalize", 57),
	})
	res, err = tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() || !strings.Contains(res.Summary, "Investigation marked complete") {
		t.Fatalf("all typed incident participants covered by Explorer-authored operations should close: %+v", res)
	}
}

func TestEmitInvestigationComplete_FlowParserBackgroundDoesNotDischargeTypedParticipant(t *testing.T) {
	background := flowOperationEvidence(types.AnchorCall, "Pipeline.Analyze", "Pipeline.Finalize", 42)
	background.Producer = types.EvidenceProducerRepoMapStructuralRelation
	ctx := flowOperationCompletionContext([]types.EvidenceItem{background})
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind:     types.DiagramSequence,
		Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Pipeline.Analyze", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Pipeline.Finalize", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if ctx.Mutable.IsInvestigationComplete() || res.Repair == nil ||
		!strings.Contains(res.Summary, "Pipeline.Analyze") ||
		!strings.Contains(res.Summary, "Pipeline.Finalize") {
		t.Fatalf("repo-map/background relation must not silently discharge Explorer-owned participant coverage: %+v", res)
	}
}

func TestEmitInvestigationComplete_FlowAmbiguousShortParticipantFailsClosed(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Alpha.run", "Alpha.done", 42),
		flowOperationEvidence(types.AnchorCall, "Beta.run", "Beta.done", 57),
	})
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind:     types.DiagramSequence,
		Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "run", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if ctx.Mutable.IsInvestigationComplete() || res.Repair == nil || !strings.Contains(res.Summary, "run") {
		t.Fatalf("same-named qualified endpoints must not discharge one ambiguous short participant: %+v", res)
	}
}

func TestEmitInvestigationComplete_FlowNoProgressConvergesWithBoundary(t *testing.T) {
	definition := flowOperationEvidence(types.AnchorDefinition, "Pipeline", "stages", 10)
	ctx := flowOperationCompletionContext([]types.EvidenceItem{definition})
	tool := &EmitInvestigationComplete{}
	if _, err := tool.Execute(ctx, flowOperationCompletionParams(t)); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	res, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("second identical attempt should converge with a typed boundary: %+v", res)
	}
	if !ctx.Mutable.EvidenceClosure().HasCompletionCaveat(types.DowngradeLaneFlowOperationCarrier) ||
		!strings.Contains(res.Summary, "operation-level flow remains unproven") {
		t.Fatalf("converged close must disclose the missing operation carrier: %+v", res)
	}
}

func TestFlowOperationCompletionGateExcludesRuntimeTraceFamily(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	ctx.AnalysisIR.RequestModel.Scenario = types.ScenarioPerformanceBottleneck
	if flowOperationEvidenceRequired(ctx) {
		t.Fatal("runtime Trace flow must stay on its typed causal/on-chain contracts")
	}
}

func TestFlowOperationCompletionGateExcludesAttachedRuntimeFlowWithoutCurrentSource(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.AttachedHitrace = "CPU:0 [001] 7.000000: tracing_mark_write: B|20|frame=77"
	ctx.AnalysisIR.RequestModel.Scenario = types.ScenarioPerformanceBottleneck
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeBoundedSelector,
		SourceQuote:    "frame=77",
		Confidence:     0.95,
	}
	if flowOperationEvidenceRequired(ctx) {
		t.Fatal("attached runtime flow without a typed current-source obligation must not request source operation evidence")
	}

	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() || res.Repair != nil ||
		strings.Contains(res.Summary, "flow-operation") ||
		strings.Contains(res.Summary, "operation-level transfer") {
		t.Fatalf("attached runtime flow should complete on the first attempt without a source-flow repair: %+v", res)
	}
	if got := ctx.Mutable.EvidenceClosure().Stats().PreCompleteDowngrades; got != 0 {
		t.Fatalf("attached runtime flow should not consume a pre-complete downgrade, got %d", got)
	}
}

func TestFlowOperationCompletionGatePreservesMixedRuntimeCurrentSourceRequest(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.AttachedHitrace = "CPU:0 [001] 7.000000: tracing_mark_write: B|20|frame=77"
	ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes: []types.CurrentSourceExplanationMode{
			types.CurrentSourceExplanationTraceCurrentFlow,
		},
		SourceQuotes: []string{"trace the observed flow into current source"},
		Confidence:   0.95,
	}
	if !flowOperationEvidenceRequired(ctx) {
		t.Fatal("an explicit mixed runtime+current-source flow must retain the source operation evidence gate")
	}
}
