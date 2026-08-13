package tool

import (
	"encoding/json"
	"path/filepath"
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
