package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
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
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, AnchorKind: anchor,
		Subject: subject, Object: object, AnchorSymbol: object,
		Source: "src/pipeline.go", LineStart: line, Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}
	if anchor == types.AnchorAssignment || anchor == types.AnchorInitializer {
		item.Snippet = subject + " = " + object
	}
	return item
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
	ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{
		"src/pipeline.go": true,
		"src/worker.go":   true,
	})
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || res.Repair == nil || res.Repair.Code != "flow_operation_carrier_evidence" {
		t.Fatalf("definition-only flow should request one focused operation pass: %+v", res)
	}
	for _, surface := range []string{res.Summary, res.Repair.Hint} {
		for _, want := range []string{
			"Soft navigation plan (not relation evidence)",
			"typed navigation stems=[Pipeline stages]",
			"candidate source files=[src/pipeline.go src/worker.go]",
		} {
			if !strings.Contains(surface, want) {
				t.Fatalf("same-turn operation repair surface missing %q:\n%s", want, surface)
			}
		}
	}
	if ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("first definition-only attempt must not mark investigation complete")
	}
	repairs := ctx.Mutable.EvidenceClosure().ActiveRepairs()
	if len(repairs) != 1 || repairs[0].DowngradeLane != types.DowngradeLaneFlowOperationCarrier {
		t.Fatalf("operation repair must carry its typed lane: %+v", repairs)
	}
	if len(repairs[0].Files) == 0 || repairs[0].Files[0] != "src/pipeline.go" || len(repairs[0].Keywords) == 0 {
		t.Fatalf("operation repair must carry bounded typed source targets instead of a blank grep instruction: %+v", repairs[0])
	}
	if rendered := repairs[0].Render(); strings.Contains(rendered, "stems: \n") ||
		!strings.Contains(rendered, "src/pipeline.go") {
		t.Fatalf("operation repair must render executable targets without a blank stem line:\n%s", rendered)
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

func TestEmitInvestigationComplete_VerifiedStagePrecedenceClosesWithoutOperationReplay(t *testing.T) {
	definition := flowOperationEvidence(types.AnchorDefinition, "Pipeline", "stages", 10)
	ctx := flowOperationCompletionContext([]types.EvidenceItem{definition})
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx.RepoRoot = repoRoot
	ctx.Mode = types.ModeRead
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantContextOnly},
		},
	}
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() || res.Repair != nil ||
		strings.Contains(res.Summary, "operation-level transfer") ||
		strings.Contains(res.Summary, "participant relation remains unproven") {
		t.Fatalf("verified stage component should close without redundant operation replay: %+v", res)
	}
	if got := ctx.Mutable.EvidenceClosure().Stats().PreCompleteDowngrades; got != 0 {
		t.Fatalf("verified stage component must not spend a downgrade round, got %d", got)
	}
}

func TestEmitInvestigationComplete_VerifiedStagesStillRequestExplicitCarrierOperations(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx := flowOperationCompletionContext([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: types.ReadModePipelineOrchestratorFile, LineStart: 1685,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "runReadSchedulerLoop",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx.RepoRoot = repoRoot
	ctx.Mode = types.ModeRead
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{
		"analyzer", "explorer", "extractor", "finalizer", "Mutable", "BusContext",
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "explorer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	}

	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || res.Repair == nil || res.Repair.Code != "flow_participant_operation_evidence" {
		t.Fatalf("verified stage precedence must not erase explicit carrier operation obligations: %+v", res)
	}
	for _, surface := range []string{res.Summary, res.Repair.Hint} {
		for _, want := range []string{"Mutable", "BusContext", "typed navigation stems=[Mutable BusContext]"} {
			if !strings.Contains(surface, want) {
				t.Fatalf("carrier-focused repair surface missing %q:\n%s", want, surface)
			}
		}
	}
	if ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("explicit unproven carrier relations must receive one focused source-operation pass")
	}
}

func TestRequestedWorkflowAuthorityConsumersAcceptPartialAnalyzerSlate(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx := flowOperationCompletionContext([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: types.ReadModePipelineOrchestratorFile, LineStart: 1685,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Orchestrator.Run",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx.RepoRoot = repoRoot
	ctx.Mode = types.ModeRead
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramSequence, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "Orchestrator.Run", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{
		IsDimensionedAnswer: true,
		Dimensions: []types.RequestedAnswerDimension{{
			Index: 1, Label: "stage", SourceQuote: "stage", Required: true,
			Role: types.RequestedAnswerDimensionStageWorkflow,
		}},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFArchitecture, RelationAxis: types.AxisFlow,
		DiagramPlan: &types.DiagramFacetGraph{Kind: types.DiagramSequence, Required: true},
	}
	if got := completionVerifiedReadModeStagePrecedence(ctx); len(got) != 3 {
		t.Fatalf("completion stage precedence=%d, want 3 from shared typed workflow authority", len(got))
	}
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 3 {
		t.Fatalf("diagram stage precedence=%d, want 3 from the same authority", len(got))
	}

	ctx.Mutable = types.NewMutableState("no grounded stage authority")
	if got := completionVerifiedReadModeStagePrecedence(ctx); len(got) != 0 {
		t.Fatalf("completion accepted stage authority without grounded source: %+v", got)
	}
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 0 {
		t.Fatalf("diagram validator accepted stage authority without grounded source: %+v", got)
	}
}

func TestRequestedWorkflowAuthorityConsumersAcceptGroundedCanonicalStageSpan(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: types.ReadModePipelineEnumsFile, LineStart: 34,
			Scope: types.ScopeLine, AnchorKind: types.AnchorStringLiteral,
			Subject: "StageAnalyze", AnchorSymbol: "StageAnalyze", GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceDirect, Source: types.ReadModePipelineEnumsFile, LineStart: 37,
			Scope: types.ScopeLine, AnchorKind: types.AnchorStringLiteral,
			Subject: "StageFinalize", AnchorSymbol: "StageFinalize", GroundingStatus: types.GroundingGrounded,
		},
	}
	ctx := flowOperationCompletionContext(evidence)
	ctx.RepoRoot = repoRoot
	ctx.Mode = types.ModeRead
	ctx.AnalysisIR.RequestModel.Intent = types.IntentExplain
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisFlow
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramSequence, Required: true,
	}
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = nil
	view := &types.AnswerSemanticView{
		Family: types.QFArchitecture, RelationAxis: types.AxisFlow,
		DiagramPlan: &types.DiagramFacetGraph{Kind: types.DiagramSequence, Required: true},
	}
	if got := completionVerifiedReadModeStagePrecedence(ctx); len(got) != 3 {
		t.Fatalf("completion evidence-span precedence=%d, want 3", len(got))
	}
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 3 {
		t.Fatalf("diagram evidence-span precedence=%d, want 3", len(got))
	}

	ctx.Mutable = types.NewMutableState("one grounded endpoint is insufficient")
	ctx.Mutable.AppendEvidence(evidence[:1])
	if got := completionVerifiedReadModeStagePrecedence(ctx); len(got) != 0 {
		t.Fatalf("completion must fail closed for one endpoint: %+v", got)
	}
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 0 {
		t.Fatalf("diagram validator must fail closed for one endpoint: %+v", got)
	}
}

func TestRequestedWorkflowAuthorityConsumersAcceptTypedStageEndpointSpan(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ctx := flowOperationCompletionContext([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: types.ReadModePipelineOrchestratorFile, LineStart: 1685,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "runReadSchedulerLoop",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx.RepoRoot = repoRoot
	ctx.Mode = types.ModeRead
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramSequence, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "codrax", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "analyze", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Mermaid", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = nil
	view := &types.AnswerSemanticView{
		Family: types.QFArchitecture, RelationAxis: types.AxisFlow,
		DiagramPlan: &types.DiagramFacetGraph{Kind: types.DiagramSequence, Required: true},
	}
	if got := completionVerifiedReadModeStagePrecedence(ctx); len(got) != 3 {
		t.Fatalf("completion stage endpoint span=%d, want 3: %+v", len(got), got)
	}
	if got := diagramVerifiedReadModeStagePrecedence(ctx, view); len(got) != 3 {
		t.Fatalf("diagram stage endpoint span=%d, want 3: %+v", len(got), got)
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

func TestFlowOperationCompletionGateUsesRouteAuthorityAfterInvalidExcludeFailOpen(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.TurnRouteHint = types.TurnRouteHint{
		Route:                     "repo",
		Source:                    "artifact",
		NeedsRepoAccess:           true,
		CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional,
	}
	ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		// emit_analysis deliberately fails an invalid model-authored exclusion
		// open to allow. Route optionality must still prevent a source proof gate.
		CurrentSourceMode: types.ExternalObservationCurrentSourceAllow,
		Confidence:        0.9,
	}
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{{
			ID: "trace_query:window#window_stats:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact},
			Subject:   "app-17267", Predicate: "running_ms", Object: "157.248",
		}},
	})
	if flowOperationEvidenceRequired(ctx) {
		t.Fatal("typed route-optional trace evidence must not be reclassified as current-source flow after invalid exclusion repair")
	}
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Repair != nil && res.Repair.Code == "flow_operation_carrier_evidence" {
		t.Fatalf("route-optional trace must not receive a source-operation repair: %+v", res)
	}
	if strings.Contains(res.Summary, "operation-level transfer") ||
		strings.Contains(res.Summary, "producer, transfer/merge boundary") {
		t.Fatalf("route-optional trace must not be taught a contradictory source-operation contract: %+v", res)
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

func TestFlowOperationCompletionGateExcludesBoundedRuntimeFactsDespiteGenericSourceAllow(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.AttachedHitrace = "raw-21 (20) [005] .... 3.003000: perf_sample: cpu=5 cpu_known=true"
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{
			types.RuntimeQuestionFactCountOrDuration,
			types.RuntimeQuestionFactFrequencyResidency,
		},
	}
	ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		// Generic source allowance is not an explicit request to prove current
		// checkout operation flow.
		CurrentSourceMode: types.ExternalObservationCurrentSourceAllow,
		Confidence:        0.95,
	}
	if flowOperationEvidenceRequired(ctx) {
		t.Fatal("bounded runtime facts must not inherit a source operation-flow contract from AxisFlow")
	}
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() || res.Repair != nil ||
		strings.Contains(res.Summary, "operation-level flow remains unproven") {
		t.Fatalf("bounded runtime facts should complete on the first attempt: %+v", res)
	}
}

func TestFlowOperationCompletionGatePreservesBoundedRuntimeFactsWithExplicitCurrentSource(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.AttachedHitrace = "raw-21 (20) [005] .... 3.003000: perf_sample: cpu=5 cpu_known=true"
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope:        types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration},
	}
	ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes: []types.CurrentSourceExplanationMode{
			types.CurrentSourceExplanationTraceCurrentFlow,
		},
		SourceQuotes: []string{"trace the observed flow into current source"},
		Confidence:   0.95,
	}
	if !flowOperationEvidenceRequired(ctx) {
		t.Fatal("an independent typed current-source request must retain the source operation-flow contract")
	}
}

func TestEmitInvestigationComplete_FlowParticipantCoverageRequestsOneFocusedPass(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Dispatch", "BuildContext", 42),
	})
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantContextOnly},
		},
	}
	ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{
		"src/analyzer.go": true,
		"src/pipeline.go": true,
	})
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || res.Repair == nil || res.Repair.Code != "flow_participant_operation_evidence" ||
		!strings.Contains(res.Summary, "Analyzer") {
		t.Fatalf("uncovered incident participant should request one focused operation pass: %+v", res)
	}
	for _, surface := range []string{res.Summary, res.Repair.Hint} {
		for _, want := range []string{
			"Soft navigation plan (not relation evidence)",
			"typed navigation stems=[Analyzer]",
			"candidate source files=[src/pipeline.go src/analyzer.go]",
		} {
			if !strings.Contains(surface, want) {
				t.Fatalf("same-turn participant repair surface missing %q:\n%s", want, surface)
			}
		}
	}
	if ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("first uncovered-participant attempt must not mark investigation complete")
	}
	repairs := ctx.Mutable.EvidenceClosure().ActiveRepairs()
	if len(repairs) != 1 || repairs[0].DowngradeLane != types.DowngradeLaneFlowParticipantCoverage {
		t.Fatalf("participant repair must carry its typed lane: %+v", repairs)
	}
	if !flowTestSliceContains(repairs[0].Keywords, "Analyzer") ||
		!flowTestSliceContains(repairs[0].Files, "src/pipeline.go") ||
		!flowTestSliceContains(repairs[0].Files, "src/analyzer.go") {
		t.Fatalf("participant repair must turn typed participants and the read closure into bounded navigation targets: %+v", repairs[0])
	}
	if !strings.Contains(repairs[0].Rationale, types.FlowOperationEvidenceEmissionGuide) ||
		!strings.Contains(repairs[0].Rationale, "exact writer and reader operation sites") {
		t.Fatalf("participant repair must reuse the flow-operation teaching source without a divergent contract: %q", repairs[0].Rationale)
	}
	wantBlocker := types.ComputeDowngradeTypedIdentifierSetKey(
		string(types.DowngradeLaneFlowParticipantCoverage), []string{"Analyzer"},
	)
	if got := ctx.Mutable.EvidenceClosure().LatestProgressDecision().Delta.BlockerKey; got != wantBlocker {
		t.Fatalf("production participant gate must key convergence on the exact missing set: got=%d want=%d", got, wantBlocker)
	}
}

func TestFlowOperationRepairTargetsCarryRelatedGroundedSourceAliases(t *testing.T) {
	evidence := []types.EvidenceItem{
		{
			Source: "internal/types/context.go", LineStart: 113,
			GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition,
			Subject: "MutableState", AnchorSymbol: "MutableState",
		},
		{
			Source: "internal/orchestrator/orchestrator.go", LineStart: 8435,
			GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition,
			Subject: "applyStageOutput", Object: "BusContext", AnchorSymbol: "applyStageOutput",
		},
		{
			Source: "internal/agent/finalizer.go", LineStart: 22,
			GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition,
			Subject: "NewFinalizerAgent", AnchorSymbol: "NewFinalizerAgent",
		},
	}
	ctx := flowOperationCompletionContext(evidence)
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{
		"internal/types/context.go":             true,
		"internal/orchestrator/orchestrator.go": true,
	})
	files, keywords := flowOperationRepairTargets(ctx, []string{"Mutable", "BusContext"}, evidence)
	for _, want := range []string{"Mutable", "BusContext", "MutableState", "applyStageOutput"} {
		if !flowTestSliceContains(keywords, want) {
			t.Fatalf("related exact source alias %q missing from bounded navigation keywords: %v", want, keywords)
		}
	}
	if flowTestSliceContains(keywords, "NewFinalizerAgent") {
		t.Fatalf("unrelated grounded identity leaked into carrier navigation keywords: %v", keywords)
	}
	for _, want := range []string{"internal/types/context.go", "internal/orchestrator/orchestrator.go"} {
		if !flowTestSliceContains(files, want) {
			t.Fatalf("related source file %q missing from repair targets: %v", want, files)
		}
	}
}

func TestFlowOperationRepairTargetsCarryParserOwnedCarrierBindingAliases(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"BusContext"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	ctx.Mutable.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
		"internal/orchestrator/orchestrator.go": {
			RelPath: "internal/orchestrator/orchestrator.go",
			Symbols: []repotypes.Symbol{{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext"}},
		},
		"internal/orchestrator/orchestrator_test.go": {
			RelPath: "internal/orchestrator/orchestrator_test.go",
			Symbols: []repotypes.Symbol{{Name: "fixtureBus", Kind: "field", DeclaredType: "*types.BusContext"}},
		},
	}})

	files, keywords := flowOperationRepairTargets(ctx, []string{"BusContext"}, nil)
	if !flowTestSliceContains(keywords, "BusContext") || !flowTestSliceContains(keywords, "busCtx") {
		t.Fatalf("typed carrier and exact parser binding should both drive soft navigation: %v", keywords)
	}
	if flowTestSliceContains(keywords, "fixtureBus") {
		t.Fatalf("auxiliary-only binding must not steer a production repair: %v", keywords)
	}
	if !flowTestSliceContains(files, "internal/orchestrator/orchestrator.go") ||
		flowTestSliceContains(files, "internal/orchestrator/orchestrator_test.go") {
		t.Fatalf("declared-binding files must honor principal source scope: %v", files)
	}
	hint := flowOperationNavigationHint(files, keywords)
	if !strings.Contains(hint, "parser-owned field/parameter/property binding") ||
		!strings.Contains(hint, "complete arguments") {
		t.Fatalf("navigation must explain how the typed carrier reaches exact operation syntax: %q", hint)
	}
}

func TestFlowOperationRepairTargetsCarryParserIncidentSitesAcrossSupportedLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			ctx := flowOperationCompletionContext(nil)
			ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"emit_answer_document"}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{{
					Identity: "emit_answer_document", Role: types.DiagramParticipantIncidentRequired,
				}},
			}
			path := "src/" + language + "/tool_binding.src"
			ctx.Mutable.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
				path: {
					RelPath: path, Language: language,
					Relations: []repotypes.Relation{
						{
							Kind: "call", File: path, Line: 3,
							FromEP:     repotypes.RelationEndpoint{Name: "preview", File: path, Line: 3},
							ToEP:       repotypes.RelationEndpoint{Name: "EmitAnswerDocumentPreview", File: path, Line: 3},
							Confidence: repotypes.ConfidenceAST,
							Provenance: "tree_sitter", ResolvedBy: language + "_call",
						},
						{
							Kind: "type_usage", File: path, Line: 17,
							FromEP:     repotypes.RelationEndpoint{Name: "registerTools", File: path, Line: 17},
							ToEP:       repotypes.RelationEndpoint{Name: "EmitAnswerDocument", File: path, Line: 17},
							Confidence: repotypes.ConfidenceAST,
							Provenance: "tree_sitter", ResolvedBy: language + "_type_usage",
						},
					},
				},
			}})

			files, keywords := flowOperationRepairTargets(ctx, []string{"emit_answer_document"}, nil)
			if !flowTestSliceContains(files, path) {
				t.Fatalf("%s parser incident site missing from soft navigation: files=%v keywords=%v", language, files, keywords)
			}
			for _, want := range []string{"EmitAnswerDocument", "registerTools"} {
				if !flowTestSliceContains(keywords, want) {
					t.Fatalf("%s parser endpoint %q missing from soft navigation: files=%v keywords=%v", language, want, files, keywords)
				}
			}
			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"emit_answer_document"})
			if !ok || target.file != path || target.lineRange.Start != 5 || target.lineRange.End != 29 {
				t.Fatalf("%s parser incident must prefer the exact cross-language identity over an earlier lexical match, got ok=%t target=%+v", language, ok, target)
			}
		})
	}
}

func TestFlowOperationNavigationReadIsAdvisoryAndAdvancesAcrossUnreadParserSites(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"emit_answer_document"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "emit_answer_document", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	ctx.Mutable.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
		"cmd/root.go": {
			RelPath: "cmd/root.go", Language: "go",
			Relations: []repotypes.Relation{{
				Kind: "type_usage", File: "cmd/root.go", Line: 4315,
				FromEP:     repotypes.RelationEndpoint{Name: "registerTools", File: "cmd/root.go", Line: 4315},
				ToEP:       repotypes.RelationEndpoint{Name: "EmitAnswerDocument", File: "cmd/root.go", Line: 4315},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_type_usage",
			}},
		},
		"internal/agent/agent.go": {
			RelPath: "internal/agent/agent.go", Language: "go",
			Relations: []repotypes.Relation{
				{
					// A lexical/substring-compatible site may occur earlier in the
					// same file. It is useful only after exact identities are exhausted.
					Kind: "call", File: "internal/agent/agent.go", Line: 1152,
					FromEP:     repotypes.RelationEndpoint{Name: "streamPreviewBuffer.emitPreview", File: "internal/agent/agent.go", Line: 1152},
					ToEP:       repotypes.RelationEndpoint{Name: "EmitAnswerDocumentPreview", File: "internal/agent/agent.go", Line: 1152},
					Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_call",
				},
				{
					Kind: "type_usage", File: "internal/agent/agent.go", Line: 3788,
					FromEP:     repotypes.RelationEndpoint{Name: "buildToolSchemas", File: "internal/agent/agent.go", Line: 3788},
					ToEP:       repotypes.RelationEndpoint{Name: "EmitAnswerDocument", File: "internal/agent/agent.go", Line: 3788},
					Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_type_usage",
				},
			},
		},
	}})

	target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"emit_answer_document"})
	if !ok || target.file != "cmd/root.go" || target.lineRange != (types.LineRange{Start: 4303, End: 4327}) {
		t.Fatalf("first unread parser site should be selected deterministically: ok=%t target=%+v", ok, target)
	}
	beforeEvidence := len(ctx.Mutable.EmittedEvidence())
	queueFlowOperationNavigationRead(ctx, []string{"emit_answer_document"}, "bounded navigation only", "participant", types.DowngradeLaneFlowParticipantCoverage)
	pending := ctx.Mutable.EvidenceClosure().PendingReads()
	if len(pending) != 1 || pending[0].File != "cmd/root.go" || len(pending[0].LineRanges) != 1 ||
		pending[0].LineRanges[0] != target.lineRange {
		t.Fatalf("navigation repair must queue exactly one surgical read: %+v", pending)
	}
	if types.ClassifyPendingReadRepair(pending[0]) != types.RepairDebtAdvisory ||
		types.PendingReadBlocksAcceptedClosure(pending[0]) {
		t.Fatalf("parser navigation read must remain soft after typed closure: %+v", pending[0])
	}
	if got := len(ctx.Mutable.EmittedEvidence()); got != beforeEvidence {
		t.Fatalf("navigation scheduling must not manufacture evidence: before=%d after=%d", beforeEvidence, got)
	}

	ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{"cmd/root.go": true})
	ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
		"cmd/root.go": {{Start: 4303, End: 4327}},
	})
	next, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"emit_answer_document"})
	if !ok || next.file != "internal/agent/agent.go" || next.lineRange != (types.LineRange{Start: 3776, End: 3800}) {
		t.Fatalf("after the first site is read the repair must prefer the exact identity over an earlier lexical match, got ok=%t target=%+v", ok, next)
	}
}

func TestFlowOperationNavigationHintUsesExactQueuedReadInsteadOfBroadSearch(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"emit_answer_document"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "emit_answer_document", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	ctx.Mutable.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
		"cmd/root.go": {
			RelPath: "cmd/root.go", Language: "go",
			Relations: []repotypes.Relation{{
				Kind: "type_usage", File: "cmd/root.go", Line: 4315,
				FromEP:     repotypes.RelationEndpoint{Name: "registerTools", File: "cmd/root.go", Line: 4315},
				ToEP:       repotypes.RelationEndpoint{Name: "EmitAnswerDocument", File: "cmd/root.go", Line: 4315},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
			}},
		},
	}})

	got := flowOperationNavigationHintForMissing(ctx, []string{"emit_answer_document"}, []string{"cmd/root.go"}, []string{"EmitAnswerDocument"})
	for _, want := range []string{`path="cmd/root.go"`, "line_offset=4302", "limit=25", "covers lines 4303-4327", "Do not run a broad repo_map/grep first"} {
		if !strings.Contains(got, want) {
			t.Fatalf("exact navigation hint missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "First use repo_map/grep") {
		t.Fatalf("exact parser target must supersede broad-search teaching:\n%s", got)
	}
	queueFlowParticipantCoverageRepair(ctx, []string{"emit_answer_document"}, nil)
	for _, repair := range ctx.Mutable.EvidenceClosure().ActiveRepairs() {
		if repair.DowngradeLane != types.DowngradeLaneFlowParticipantCoverage {
			continue
		}
		if !strings.Contains(repair.Rationale, `path="cmd/root.go"`) || strings.Contains(repair.Rationale, "First use repo_map/grep") {
			t.Fatalf("durable repair must carry the same exact direct-read instruction:\n%s", repair.Rationale)
		}
		return
	}
	t.Fatal("missing durable flow participant repair")
}

func TestFlowParticipantCoverageLateResolvesUniqueStaticMemberUnderRequestedOwner(t *testing.T) {
	busOperation := flowOperationEvidence(types.AnchorAssignment, "o.busCtx.EvidenceItems", "output.EvidenceItems", 20)
	busOperation.Snippet = "o.busCtx.EvidenceItems = output.EvidenceItems"
	busOperation.OwnerIdentity = "Orchestrator.applyStageOutput"
	busOperation.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
	}}
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		busOperation,
		flowOperationEvidence(types.AnchorCall, "explorerEvaluator.BuildInitialInstruction", "ctx.Mutable.ResetPrescanSummary", 89),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Mutable", "BusContext"}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "Mutable", Resolution: types.EntityResolutionPrescanAnchor, Resolved: true, UseForSearch: true},
		{Surface: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	ctx.Mutable.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
		"internal/types/context.go": {
			RelPath: "internal/types/context.go",
			Symbols: []repotypes.Symbol{
				{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "*MutableState"},
				{Name: "Mutable", Kind: "field", Parent: "AgentContext", DeclaredType: "*MutableState"},
			},
		},
	}})

	resolved := flowResolveParticipantIdentity(ctx, ctx.AnalysisIR.RequestModel,
		ctx.AnalysisIR.RequestModel.DiagramHint.Participants[0])
	for _, want := range []string{"Mutable", "BusContext.Mutable", "*MutableState"} {
		if !flowTestSliceContains(resolved.surfaces, want) {
			t.Fatalf("unique requested-owner binding should late-resolve %q: %+v", want, resolved)
		}
	}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("independently grounded operations should cover both late-resolved member and owner: %v", got)
	}
}

func TestFlowParticipantCoverageLateResolvedMemberQueuesExactRepairCoordinates(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Dispatch", "BuildContext", 42),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Mutable", "BusContext"}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "Mutable", Resolution: types.EntityResolutionPrescanAnchor, Resolved: true, UseForSearch: true},
		{Surface: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	ctx.Mutable.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
		"internal/types/context.go": {
			RelPath: "internal/types/context.go",
			Symbols: []repotypes.Symbol{
				{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "*MutableState"},
				{Name: "Mutable", Kind: "field", Parent: "AgentContext", DeclaredType: "*MutableState"},
			},
		},
	}})

	missing := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence())
	if !flowTestSliceContains(missing, "Mutable") {
		t.Fatalf("unique requested-owner member must no longer be skipped by completion: %v", missing)
	}
	files, keywords := flowOperationRepairTargets(ctx, []string{"Mutable"}, ctx.Mutable.EmittedEvidence())
	if !flowTestSliceContains(files, "internal/types/context.go") {
		t.Fatalf("late-resolved declaration file missing from bounded repair: files=%v keywords=%v", files, keywords)
	}
	for _, want := range []string{"Mutable", "BusContext.Mutable", "*MutableState"} {
		if !flowTestSliceContains(keywords, want) {
			t.Fatalf("late-resolved repair coordinate %q missing: files=%v keywords=%v", want, files, keywords)
		}
	}
}

func TestFlowParticipantCoverageLateResolutionFailsClosedOnRequestedOwnerAmbiguity(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Dispatch", "BuildContext", 42),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Mutable", "BusContext"}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "Mutable", Resolution: types.EntityResolutionPrescanAnchor, Resolved: true, UseForSearch: true},
		{Surface: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	ctx.Mutable.SetSearchGraph(&repotypes.Graph{FileIndex: map[string]*repotypes.FileInfo{
		"src/a.go": {RelPath: "src/a.go", Symbols: []repotypes.Symbol{{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "*StateA"}}},
		"src/b.go": {RelPath: "src/b.go", Symbols: []repotypes.Symbol{{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "*StateB"}}},
	}})

	resolved := flowResolveParticipantIdentity(ctx, ctx.AnalysisIR.RequestModel,
		ctx.AnalysisIR.RequestModel.DiagramHint.Participants[0])
	if len(resolved.surfaces) != 0 || len(resolved.files) != 0 {
		t.Fatalf("ambiguous parser bindings must fail closed, not force a homonym search: %+v", resolved)
	}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); flowTestSliceContains(got, "Mutable") {
		t.Fatalf("ambiguous member must remain visible/unproven instead of becoming a hard repair: %v", got)
	}
}

func flowTestSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestEmitInvestigationComplete_BlocksOnLatestRequiredEvidenceItemValidationRepair(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "emit_evidence",
		Success:  true,
		Repair: &types.ToolRepair{
			Code:   types.ToolRepairCodeEvidenceItemValidation,
			Hint:   "correct items[1].line_start",
			Fields: []string{"items[1].line_start"},
			Metadata: map[string]string{
				"repair_status":       types.ToolRepairStatusActionRequired,
				"completion_blocking": "true",
			},
		},
	})
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceItemValidation ||
		ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("completion must preserve the unresolved local evidence repair without closing: %+v", res)
	}
	if got := ctx.Mutable.EvidenceClosure().Stats().PreCompleteDowngrades; got != 0 {
		t.Fatalf("unresolved schema repair must not count as flow no-progress, got %d", got)
	}

	// A later successful emit_evidence result supersedes the local latch. The
	// ordinary flow-operation gate should then run instead of replaying stale
	// item-validation debt.
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "emit_evidence", Success: true})
	res, err = tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute after successful re-emit: %v", err)
	}
	if res.Repair == nil || res.Repair.Code != "flow_operation_carrier_evidence" {
		t.Fatalf("latest successful re-emit must clear the validation latch and restore the normal gate: %+v", res)
	}
}

func TestRequiredRelationRepairDebtSurvivesUnrelatedSuccessfulEmitUntilExactRowsExist(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	assignment := buildEmitEvidenceAssignmentEndpointRepair([]emitEvidenceAssignmentEndpointRepair{{
		itemIndex: 0, anchor: types.AnchorAssignment,
		receiver: "state", value: "next", source: "src/pipeline.go", line: 42,
	}})
	call := buildEmitEvidenceCallEndpointRepair([]emitEvidenceCallEndpointRepair{{
		itemIndex: 1, caller: "Pipeline.run", callee: "Worker.step", source: "src/pipeline.go", line: 55,
	}})
	repair := mergeEmitEvidenceRelationEndpointRepairs(assignment, call)
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "emit_evidence", Success: true, Repair: repair,
	})
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "emit_evidence", Success: true, Summary: "unrelated grounded definition",
	})
	if got := pendingBlockingEmitEvidenceItemValidationRepair(ctx); got == nil {
		t.Fatal("an unrelated successful emit must not clear exact relation repair debt")
	}

	ctx.Mutable.AppendEvidence([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorAssignment, "state", "next", 42),
	})
	if got := pendingBlockingEmitEvidenceItemValidationRepair(ctx); got == nil {
		t.Fatal("a partially repaired multi-row obligation must remain pending")
	}

	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		Subject: "Pipeline.run", Object: "Worker.step", AnchorSymbol: "Worker.step",
		Predicate: "calls", Source: "src/pipeline.go", LineStart: 55, Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}})
	if got := pendingBlockingEmitEvidenceItemValidationRepair(ctx); got != nil {
		t.Fatalf("all exact typed rows now exist; durable repair debt should clear: %+v", got)
	}
}

func TestRequiredValueTransferRepairAcceptsEquivalentAssignmentInitializerSpelling(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	repair := buildEmitEvidenceAssignmentEndpointRepair([]emitEvidenceAssignmentEndpointRepair{{
		itemIndex: 0, anchor: types.AnchorInitializer,
		receiver: "Stage", value: "StageAnalyze", source: "src/pipeline.go", line: 47,
	}})
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "emit_evidence", Success: true, Repair: repair,
	})
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorAssignment,
		Subject: "Stage", Object: "StageAnalyze", AnchorSymbol: "Stage",
		Predicate: "assigns", Source: "src/pipeline.go", LineStart: 47,
		Scope: types.ScopeLine, Snippet: "Stage: StageAnalyze,",
		GroundingStatus: types.GroundingGrounded,
	}})
	if got := pendingBlockingEmitEvidenceItemValidationRepair(ctx); got != nil {
		t.Fatalf("an exact value-transfer tuple must discharge an equivalent initializer obligation: %+v", got)
	}

	ctx = flowOperationCompletionContext(nil)
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "emit_evidence", Success: true, Repair: repair,
	})
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
		Subject: "Stage", Object: "StageAnalyze", AnchorSymbol: "StageAnalyze",
		Predicate: "calls", Source: "src/pipeline.go", LineStart: 47,
		Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
	}})
	if got := pendingBlockingEmitEvidenceItemValidationRepair(ctx); got == nil {
		t.Fatal("a non-value-transfer relation at the same location must not clear the obligation")
	}
}

func TestEmitInvestigationComplete_FlowParticipantCoverageClosesAfterIncidentOperation(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Dispatch", "BuildContext", 42),
	})
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	tool := &EmitInvestigationComplete{}
	if _, err := tool.Execute(ctx, flowOperationCompletionParams(t)); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorAssignment, "Analyzer", "AnalysisIR", 55),
	})
	res, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() || !strings.Contains(res.Summary, "Investigation marked complete") {
		t.Fatalf("incident operation should close participant coverage: %+v", res)
	}
}

func TestFlowParticipantCoverageUsesTypedEntityAliasForDecoratedDisplayLabel(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorAssignment, "Analyzer", "AnalysisIR", 55),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Analyzer", "AnalysisIR"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "Analyzer agent", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("typed entity alias should resolve the display label without minting an edge: %v", got)
	}
}

func TestFlowParticipantCoverageDoesNotHardSearchAmbiguousOrConceptLabels(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Dispatch", "BuildContext", 42),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"stage", "codrax read mode"}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "stage", Resolution: types.EntityResolutionAmbiguousSymbol, UseForSearch: true},
		{Surface: "codrax read mode", Resolution: types.EntityResolutionInferredConcept},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramSequence, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "stage", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "codrax read mode", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("non-unique display labels must not trigger impossible source operation repair: %v", got)
	}
}

func TestFlowParticipantCoverageStillRequiresUniqueTypedSourceIdentity(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Dispatch", "BuildContext", 42),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Analyzer"}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{{
		Surface: "Analyzer", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true,
	}}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramSequence, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence())
	if len(got) != 1 || got[0] != "Analyzer" {
		t.Fatalf("unique typed source participant must retain hard operation coverage: %v", got)
	}
}

func TestFlowParticipantCoverageTreatsExactReceiverOperationAsParticipantIncident(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "analyzerEvaluator.BuildInitialInstruction", "ctx.Mutable.ResetPrescanSummary", 89),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Mutable"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("exact receiver/member call should cover its typed participant without minting an edge: %v", got)
	}
}

func TestFlowParticipantCoverageUsesExactStaticDeclaredBindingWithoutMintingEdge(t *testing.T) {
	operation := flowOperationEvidence(types.AnchorAssignment, "o.busCtx.EvidenceItems", "output.EvidenceItems", 20)
	operation.Snippet = "o.busCtx.EvidenceItems = output.EvidenceItems"
	operation.OwnerIdentity = "Orchestrator.applyStageOutput"
	operation.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
	}}
	ctx := flowOperationCompletionContext([]types.EvidenceItem{operation})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"BusContext"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("exact declared binding should cover its typed participant without minting an edge: %v", got)
	}
	if got := types.FlowOperationEvidence(ctx.Mutable.EmittedEvidence()); len(got) != 1 || got[0].LineStart != operation.LineStart {
		t.Fatalf("declaration must remain outside operation authority: %+v", got)
	}
}

func TestEmitInvestigationComplete_FlowParticipantCoverageNoProgressConverges(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Dispatch", "BuildContext", 42),
	})
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	tool := &EmitInvestigationComplete{}
	if _, err := tool.Execute(ctx, flowOperationCompletionParams(t)); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	res, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if ctx.Mutable.IsInvestigationComplete() || res.Repair == nil {
		t.Fatalf("second no-progress close must retain one bounded locate/read repair turn: %+v", res)
	}
	res, err = tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("third Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() ||
		!ctx.Mutable.EvidenceClosure().HasCompletionCaveat(types.DowngradeLaneFlowParticipantCoverage) ||
		!strings.Contains(res.Summary, "participant relation remains unproven") {
		t.Fatalf("third no-progress close should converge with participant boundary: %+v", res)
	}
}

func TestFlowParticipantCoverageExcludesRuntimeTraceAndContextOnlyParticipants(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Dispatch", "BuildContext", 42),
	})
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "BusContext", Role: types.DiagramParticipantContextOnly,
		}},
	}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("context-only participant must not require an incident edge: %v", got)
	}
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	ctx.AnalysisIR.RequestModel.DiagramHint.Participants = []types.DiagramParticipantHint{{
		Identity: "render-thread", Role: types.DiagramParticipantIncidentRequired,
	}}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("runtime Trace participant must stay on causal/on-chain contracts: %v", got)
	}
}
