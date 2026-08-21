package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
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
		Producer: types.EvidenceProducerExplorerEmitEvidence,
		Subject:  subject, Object: object, AnchorSymbol: object,
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

func flowTestIndexedGraph(files map[string]*repotypes.FileInfo) *repotypes.Graph {
	graph := &repotypes.Graph{
		FileIndex:  files,
		SymbolDefs: make(map[string][]*repotypes.Symbol),
	}
	for path, file := range files {
		if file == nil {
			continue
		}
		if file.RelPath == "" {
			file.RelPath = path
		}
		for i := range file.Symbols {
			symbol := &file.Symbols[i]
			if symbol.File == "" {
				symbol.File = path
			}
			graph.SymbolDefs[symbol.Name] = append(graph.SymbolDefs[symbol.Name], symbol)
		}
	}
	return graph
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

func TestEmitInvestigationComplete_ProbeDefersRequestWideFlowCompletionGates(t *testing.T) {
	definition := flowOperationEvidence(types.AnchorDefinition, "Pipeline", "stages", 10)
	ctx := flowOperationCompletionContext([]types.EvidenceItem{definition})
	ctx.ExploreDispatchKind = types.NodeProbe
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute probe: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() || res.Repair != nil ||
		strings.Contains(res.Summary, "operation-level transfer") ||
		strings.Contains(res.Summary, "participant relation") {
		t.Fatalf("probe should close its navigation pass without request-wide relation repair: %+v", res)
	}
	if got := ctx.Mutable.EvidenceClosure().Stats().PreCompleteDowngrades; got != 0 {
		t.Fatalf("probe must not spend request-wide relation downgrade rounds, got %d", got)
	}

	evidenceCtx := flowOperationCompletionContext([]types.EvidenceItem{definition})
	evidenceCtx.ExploreDispatchKind = types.NodeEvidence
	evidenceCtx.AnalysisIR.RequestModel.DiagramHint = ctx.AnalysisIR.RequestModel.DiagramHint
	evidenceRes, err := (&EmitInvestigationComplete{}).Execute(evidenceCtx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute evidence: %v", err)
	}
	if evidenceRes.Repair == nil || evidenceRes.Repair.Code != "flow_operation_carrier_evidence" {
		t.Fatalf("evidence lane must retain request-wide relation proof gate: %+v", evidenceRes)
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
	// Production provenance may resolve one stage label to an unrelated
	// same-named local field while its sibling stage labels remain concepts.
	// Verified stage authority, not that accidental symbol cardinality, owns
	// admission to the stage-precedence component.
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "analyzer", Resolution: types.EntityResolutionPrescanAnchor, UseForSearch: true},
		{Surface: "explorer", Resolution: types.EntityResolutionPrescanAnchor, UseForSearch: true},
		{Surface: "extractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "finalizer", Resolution: types.EntityResolutionInferredConcept},
		{Surface: "Mutable", Resolution: types.EntityResolutionAmbiguousSymbol, UseForSearch: true},
		{Surface: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
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
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		"internal/types/context.go": {
			RelPath: "internal/types/context.go", Language: repotypes.LangGo,
			Symbols: []repotypes.Symbol{
				{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "*MutableState"},
				{Name: "Mutable", Kind: "field", Parent: "AgentContext", DeclaredType: "*MutableState"},
			},
		},
	}))

	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || res.Repair == nil || res.Repair.Code != "flow_participant_operation_evidence" {
		t.Fatalf("verified stage precedence must not erase explicit carrier operation obligations: %+v", res)
	}
	for _, surface := range []string{res.Summary, res.Repair.Hint} {
		for _, want := range []string{"Mutable", "BusContext", "typed navigation stems=["} {
			if !strings.Contains(surface, want) {
				t.Fatalf("carrier-focused repair surface missing %q:\n%s", want, surface)
			}
		}
		if strings.Contains(surface, "[extractor") || strings.Contains(surface, "extractor Mutable") ||
			strings.Contains(surface, "extractor BusContext") {
			t.Fatalf("verified stage participant must not leak from a homonymous local symbol into carrier repair:\n%s", surface)
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
	first, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	second, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if ctx.Mutable.IsInvestigationComplete() || first.Repair == nil || second.Repair == nil {
		t.Fatalf("two attempts must leave room for locate then read/materialize: first=%+v second=%+v", first, second)
	}
	res, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("third Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("third identical attempt should converge with a typed boundary: %+v", res)
	}
	if !ctx.Mutable.EvidenceClosure().HasCompletionCaveat(types.DowngradeLaneFlowOperationCarrier) ||
		!strings.Contains(res.Summary, "operation-level flow remains unproven") {
		t.Fatalf("converged close must disclose the missing operation carrier: %+v", res)
	}
}

func TestFlowOperationRepairUsesResolvedSymbolsWhenRequiredDiagramSlateIsEmpty(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{
			Surface: "emit_answer_document", ResolvedAs: "EmitAnswerDocument",
			Resolution: types.EntityResolutionSymbol, Resolved: true,
			UseForSearch: true, UseForShape: true,
		},
		{
			Surface: "Name", Resolution: types.EntityResolutionAmbiguousSymbol,
			Resolved: false, UseForSearch: true,
		},
		{
			Surface: "finalizer", Resolution: types.EntityResolutionInferredConcept,
			Resolved: true, UseForSearch: true,
		},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
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

	participants := flowOperationPlanningParticipants(ctx.AnalysisIR.RequestModel)
	if len(participants) != 1 || participants[0].Identity != "EmitAnswerDocument" ||
		participants[0].Role != types.DiagramParticipantIncidentRequired {
		t.Fatalf("only resolver-confirmed source symbols may steer empty-slate navigation: %+v", participants)
	}
	target, ok := flowOperationRepairReadTargetForMissing(ctx, nil)
	if !ok || target.file != "cmd/root.go" ||
		target.lineRange != (types.LineRange{Start: 4303, End: 4327}) {
		t.Fatalf("resolved empty-slate entity must locate its parser operation site: ok=%t target=%+v", ok, target)
	}
	if len(ctx.AnalysisIR.RequestModel.DiagramHint.Participants) != 0 ||
		len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Fatal("soft navigation must not mutate the hard participant slate or manufacture evidence")
	}
}

func TestFlowOperationRepairKeepsExplicitParticipantSlateAuthoritative(t *testing.T) {
	rm := flowOperationCompletionContext(nil).AnalysisIR.RequestModel
	rm.DiagramHint = &types.DiagramHint{Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "ExplicitStage", Role: types.DiagramParticipantContextOnly,
		}}}
	rm.AnalyzerHints.EntityProvenance = []types.EntityProvenance{{
		Surface: "DerivedSymbol", Resolution: types.EntityResolutionSymbol,
		Resolved: true, UseForSearch: true, UseForShape: true,
	}}
	got := flowOperationPlanningParticipants(rm)
	if len(got) != 1 || got[0].Identity != "ExplicitStage" ||
		got[0].Role != types.DiagramParticipantContextOnly {
		t.Fatalf("explicit model-owned participant slate must not be replaced: %+v", got)
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

func TestFlowOperationNavigationPrefersTypedCarrierAsCompleteCallArgumentAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			path := filepath.ToSlash(filepath.Join("src", language, "pipeline.src"))
			absolute := filepath.Join(repo, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
				t.Fatal(err)
			}
			lines := make([]string, 30)
			lines[0] = "run pipeline"
			lines[1] = "this.busContext.reset()"
			// A bare same-owner helper also receives the complete carrier,
			// but it is not the best bounded coordinate for a component
			// handoff investigation.
			lines[9] = "inspect(this.busContext)"
			lines[29] = "const agentContext = builder.build(this.busContext, stage)"
			source := strings.Join(lines, "\n")
			if err := os.WriteFile(absolute, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{{
					Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
				}},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				path: {
					RelPath: path, Language: language,
					Symbols: []repotypes.Symbol{{
						Name: "busContext", Parent: "Pipeline", DeclaredType: "BusContext", Line: 1,
					}},
					Relations: []repotypes.Relation{
						{
							Kind: "call", File: path, Line: 2,
							FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 2},
							ToEP:       repotypes.RelationEndpoint{Name: "reset", Receiver: "this.busContext", Line: 2},
							Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: language + "_call",
						},
						{
							Kind: "call", File: path, Line: 10,
							FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 10},
							ToEP:       repotypes.RelationEndpoint{Name: "inspect", Line: 10},
							Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: language + "_call",
						},
						{
							Kind: "call", File: path, Line: 30,
							FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 30},
							ToEP:       repotypes.RelationEndpoint{Name: "build", Receiver: "builder", Line: 30},
							Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: language + "_call",
						},
					},
				},
			}))

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || target.file != path || target.lineRange != (types.LineRange{Start: 18, End: 42}) {
				t.Fatalf("%s cross-owner typed carrier handoff must outrank local receiver/helper calls: ok=%t target=%+v", language, ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("carrier navigation must not manufacture argument evidence")
			}
		})
	}
}

func TestFlowOperationNavigationPrefersCarrierHandoffWithRequestedSiblingArgumentAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			path := filepath.ToSlash(filepath.Join("src", language, "pipeline.src"))
			absolute := filepath.Join(repo, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
				t.Fatal(err)
			}
			lines := make([]string, 30)
			lines[0] = "run pipeline"
			// Both calls hand off the same typed carrier. Only the later call
			// also carries another independently requested participant as a
			// complete argument; that makes it the higher-value read coordinate.
			lines[9] = "builder.build(this.busContext, backgroundStage)"
			lines[29] = "builder.build(this.busContext, AgentExtractor)"
			if err := os.WriteFile(absolute, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
				t.Fatal(err)
			}

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "extractor", ResolvedAs: "AgentExtractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				path: {
					RelPath: path, Language: language,
					Symbols: []repotypes.Symbol{{
						Name: "busContext", Parent: "Pipeline", DeclaredType: "BusContext", Line: 1,
					}},
					Relations: []repotypes.Relation{
						{
							Kind: "call", File: path, Line: 10,
							FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 10},
							ToEP:       repotypes.RelationEndpoint{Name: "build", Receiver: "builder", Line: 10},
							Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: language + "_call",
						},
						{
							Kind: "call", File: path, Line: 30,
							FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 30},
							ToEP:       repotypes.RelationEndpoint{Name: "build", Receiver: "builder", Line: 30},
							Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: language + "_call",
						},
					},
				},
			}))

			// extractor is already covered elsewhere in the requested graph;
			// only BusContext remains missing. The covered participant must
			// still raise the quality of the exact handoff that names it.
			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || target.file != path || target.lineRange != (types.LineRange{Start: 18, End: 42}) {
				t.Fatalf("%s requested sibling argument must prioritize the cross-participant carrier handoff: ok=%t target=%+v", language, ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("sibling-argument navigation rank must not manufacture relation evidence")
			}
		})
	}
}

func TestFlowOperationNavigationFindsOwnedCarrierHandoffAcrossSourceFilesAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			ownerPath := filepath.ToSlash(filepath.Join("src", language, "pipeline_owner.src"))
			handoffPath := filepath.ToSlash(filepath.Join("src", language, "pipeline_stage.src"))
			ownerLines := make([]string, 30)
			ownerLines[0] = "pipeline owns bus context"
			ownerLines[9] = "inspect(this.busContext)"
			handoffLines := make([]string, 40)
			handoffLines[0] = "run pipeline stage"
			handoffLines[29] = "builder.build(this.busContext, AgentExtractor)"
			for path, body := range map[string]string{
				ownerPath:   strings.Join(ownerLines, "\n"),
				handoffPath: strings.Join(handoffLines, "\n"),
			} {
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "extractor", ResolvedAs: "AgentExtractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				ownerPath: {
					RelPath: ownerPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{
						{Name: "busContext", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 1},
						{Name: "inspectCarrier", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 30},
					},
					Relations: []repotypes.Relation{{
						Kind: "call", File: ownerPath, Line: 10,
						FromEP:     repotypes.RelationEndpoint{Name: "inspectCarrier", Receiver: "Pipeline", Line: 10},
						ToEP:       repotypes.RelationEndpoint{Name: "inspect", Line: 10},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
						ResolvedBy: language + "_call",
					}},
				},
				handoffPath: {
					RelPath: handoffPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{{
						Name: "run", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 40,
					}},
					Relations: []repotypes.Relation{{
						Kind: "call", File: handoffPath, Line: 30,
						FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 30},
						ToEP:       repotypes.RelationEndpoint{Name: "build", Receiver: "builder", Line: 30},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
						ResolvedBy: language + "_call",
					}},
				},
			}))

			// extractor is already covered elsewhere; only the carrier remains
			// missing. Its exact owner spans both files, so the sibling method's
			// two-participant handoff must outrank the local one-participant use.
			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || target.file != handoffPath || target.lineRange != (types.LineRange{Start: 18, End: 42}) {
				t.Fatalf("%s owned cross-file carrier handoff must win: ok=%t target=%+v", language, ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("cross-file owner navigation must not manufacture relation evidence")
			}
		})
	}
}

func TestFlowOperationNavigationFindsNestedTypedCarrierHandoffAcrossOwnersAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			statePath := filepath.ToSlash(filepath.Join("src", language, "context_state.src"))
			outerPath := filepath.ToSlash(filepath.Join("src", language, "orchestrator_owner.src"))
			handoffPath := filepath.ToSlash(filepath.Join("src", language, "orchestrator_stage.src"))
			stateLines := make([]string, 25)
			stateLines[9] = "check(this.Mutable)"
			outerLines := make([]string, 10)
			outerLines[0] = "orchestrator owns typed bus context"
			handoffLines := make([]string, 40)
			handoffLines[29] = "appendStageOutputEvidenceToMutable(o.busCtx.Mutable, output.EvidenceItems)"
			for path, body := range map[string]string{
				statePath: strings.Join(stateLines, "\n"), outerPath: strings.Join(outerLines, "\n"),
				handoffPath: strings.Join(handoffLines, "\n"),
			} {
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "Mutable", Resolution: types.EntityResolutionAmbiguousSymbol, UseForSearch: true},
				{Surface: "extractor", ResolvedAs: "AgentExtractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				statePath: {
					RelPath: statePath, Language: language, Package: "types",
					Symbols: []repotypes.Symbol{
						{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "*MutableState", Line: 1},
						{Name: "inspect", Kind: "method", Receiver: "BusContext", Line: 1, EndLine: 25},
					},
					Relations: []repotypes.Relation{{
						Kind: "call", File: statePath, Line: 10,
						ToEP:       repotypes.RelationEndpoint{Name: "check", Line: 10},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
					}},
				},
				outerPath: {
					RelPath: outerPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{{
						Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", Line: 1,
					}},
				},
				handoffPath: {
					RelPath: handoffPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{{
						Name: "apply", Kind: "method", Receiver: "Orchestrator", Line: 1, EndLine: 40,
					}},
					// FromEP is intentionally empty: this is the shape emitted by the
					// production Go parser, so owner recovery must use the method span.
					Relations: []repotypes.Relation{{
						Kind: "call", File: handoffPath, Line: 30,
						ToEP:       repotypes.RelationEndpoint{Name: "appendStageOutputEvidenceToMutable", Line: 30},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
					}},
				},
			}))

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext", "Mutable"})
			if !ok || target.file != handoffPath || target.lineRange != (types.LineRange{Start: 18, End: 42}) {
				t.Fatalf("%s nested typed owner handoff must win: ok=%t target=%+v", language, ok, target)
			}
			target, ok = flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || target.file != handoffPath || target.lineRange != (types.LineRange{Start: 18, End: 42}) {
				t.Fatalf("%s outer typed carrier used inside a complete member path must win: ok=%t target=%+v", language, ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("nested owner navigation must not manufacture relation evidence")
			}
		})
	}
}

func TestFlowOperationNavigationFollowsReadCarrierHandoffToCalleeMutationAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			callerPath := filepath.ToSlash(filepath.Join("src", language, "pipeline.src"))
			calleePath := filepath.ToSlash(filepath.Join("src", language, "builder.src"))
			callerLines := make([]string, 30)
			callerLines[0] = "run pipeline"
			callerLines[1] = "this.busContext.reset()"
			callerLines[29] = "const agentContext = builder.BuildAgentContext(this.busContext, stage)"
			calleeLines := make([]string, 40)
			calleeLines[19] = "BuildAgentContext(bus, stage)"
			calleeLines[29] = "Mutable: bus.Mutable"
			for path, body := range map[string]string{
				callerPath: strings.Join(callerLines, "\n"),
				calleePath: strings.Join(calleeLines, "\n"),
			} {
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "Mutable", Resolution: types.EntityResolutionAmbiguousSymbol, UseForSearch: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				callerPath: {
					RelPath: callerPath, Language: language,
					Symbols: []repotypes.Symbol{
						{Name: "run", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 30},
						{Name: "busContext", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 1},
					},
					Relations: []repotypes.Relation{
						{
							Kind: "call", File: callerPath, Line: 2,
							FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 2},
							ToEP:       repotypes.RelationEndpoint{Name: "reset", Receiver: "this.busContext", Line: 2},
							Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: language + "_call",
						},
						{
							Kind: "call", File: callerPath, Line: 30,
							FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 30},
							ToEP:       repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 30},
							Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: language + "_call",
						},
					},
				},
				calleePath: {
					RelPath: calleePath, Language: language,
					Symbols: []repotypes.Symbol{
						{Name: "BuildAgentContext", Kind: "function", Line: 20, EndLine: 40},
						{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "MutableState", Line: 5},
						{Name: "Mutable", Kind: "field", Parent: "AgentContext", DeclaredType: "MutableState", Line: 6},
					},
					LineFeatures: map[int][]repotypes.LineFeature{
						30: {repotypes.LineFeatureMemberInitializer},
					},
				},
			}))
			missing := flowParticipantCoverageMissing(ctx, nil)
			if !flowTestSliceContains(missing, "BusContext") || !flowTestSliceContains(missing, "Mutable") {
				t.Fatalf("%s planning aliases must not close hard participant coverage: %v", language, missing)
			}

			first, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext", "Mutable"})
			if !ok || first.file != callerPath || first.lineRange != (types.LineRange{Start: 18, End: 42}) || first.alreadyRead {
				t.Fatalf("%s first hop must select the exact carrier argument handoff: ok=%t target=%+v", language, ok, first)
			}
			ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{callerPath: true})
			ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
				callerPath: {{Start: 18, End: 42}},
			})
			second, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"Mutable"})
			if !ok || second.file != calleePath || second.lineRange != (types.LineRange{Start: 18, End: 42}) || second.alreadyRead {
				t.Fatalf("%s second hop must select the exact AST-tagged callee initializer: ok=%t target=%+v", language, ok, second)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("two-hop navigation must not manufacture argument or mutation evidence")
			}
		})
	}
}

func TestFlowOperationNavigationContinuesGroundedHandoffIntoReceivingCallableBodyAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			callerPath := filepath.ToSlash(filepath.Join("src", language, "pipeline.src"))
			calleePath := filepath.ToSlash(filepath.Join("src", language, "builder.src"))
			unrelatedPath := filepath.ToSlash(filepath.Join("src", language, "unrelated.src"))
			handoff := flowOperationEvidence(types.AnchorArgument, "pipeline.busContext", "BuildAgentContext", 10)
			handoff.Source = callerPath
			handoff.AnchorSymbol = "pipeline.busContext"

			ctx := flowOperationCompletionContext([]types.EvidenceItem{handoff})
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "Mutable", Resolution: types.EntityResolutionAmbiguousSymbol, UseForSearch: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			graph := flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				callerPath: {
					RelPath: callerPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{{Name: "run", Kind: "function", Line: 1, EndLine: 20}},
					Relations: []repotypes.Relation{{
						Kind: "call", File: callerPath, Line: 10,
						FromEP:     repotypes.RelationEndpoint{Name: "run", Line: 10},
						ToEP:       repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 10},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
					}},
				},
				calleePath: {
					RelPath: calleePath, Language: language, Package: "builder",
					Symbols: []repotypes.Symbol{
						{Name: "BuildAgentContext", Kind: "function", Line: 20, EndLine: 40},
						{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "MutableState", Line: 5},
					},
					Relations: []repotypes.Relation{{
						Kind: "call", File: calleePath, Line: 29,
						FromEP:     repotypes.RelationEndpoint{Name: "BuildAgentContext", Line: 29},
						ToEP:       repotypes.RelationEndpoint{Name: "Objective", Receiver: "bus.Mutable", Line: 29},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
					}},
				},
				unrelatedPath: {
					RelPath: unrelatedPath, Language: language, Package: "other",
					Symbols: []repotypes.Symbol{{Name: "inspect", Kind: "function", Line: 1, EndLine: 10}},
					Relations: []repotypes.Relation{{
						Kind: "call", File: unrelatedPath, Line: 3,
						FromEP: repotypes.RelationEndpoint{Name: "inspect", Line: 3},
						ToEP:   repotypes.RelationEndpoint{Name: "Snapshot", Receiver: "state.Mutable", Line: 3},
					}},
				},
			})
			graph.ResolvedImports = map[string][]repotypes.ResolvedImportBinding{
				callerPath: {{
					Import:  repotypes.Import{Alias: "builder", Path: "builder", File: callerPath, Line: 1},
					Targets: []string{calleePath},
				}},
			}
			ctx.Mutable.SetSearchGraph(graph)
			ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{callerPath: true, calleePath: true})
			ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
				callerPath: {{Start: 1, End: 20}},
				calleePath: {{Start: 20, End: 40}},
			})

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"Mutable"})
			if !ok || target.file != calleePath || target.lineRange != (types.LineRange{Start: 17, End: 41}) ||
				!target.alreadyRead || !target.receivingCallableBody {
				t.Fatalf("%s grounded handoff must continue inside the unique receiver body: ok=%t target=%+v", language, ok, target)
			}
			hint := flowOperationNavigationHintForMissing(ctx, []string{"Mutable"}, nil, nil)
			if !strings.Contains(hint, "Exact receiving-callable extraction step") ||
				!strings.Contains(hint, calleePath) || strings.Contains(hint, unrelatedPath) {
				t.Fatalf("%s repair hint must name the existing receiving-callable frontier:\n%s", language, hint)
			}
			if got := ctx.Mutable.EmittedEvidence(); len(got) != 1 || types.ClaimFormOf(got[0]) != types.ClaimArgumentFlow {
				t.Fatalf("%s receiving-body navigation must not manufacture evidence: %+v", language, got)
			}
		})
	}
}

func TestFlowOperationNavigationGroundedHandoffPrefersExactInitializerOverLocalCallAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			callerPath := filepath.ToSlash(filepath.Join("src", language, "pipeline.src"))
			calleePath := filepath.ToSlash(filepath.Join("src", language, "builder.src"))
			callerLines := make([]string, 24)
			callerLines[9] = "agentContext := builder.BuildAgentContext(pipeline.busContext, stage)"
			calleeLines := make([]string, 44)
			calleeLines[19] = "BuildAgentContext(bus, stage)"
			calleeLines[28] = "objective := bus.Mutable.Objective()"
			calleeLines[29] = "Mutable: bus.Mutable"
			for path, lines := range map[string][]string{callerPath: callerLines, calleePath: calleeLines} {
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			handoff := flowOperationEvidence(types.AnchorArgument, "pipeline.busContext", "BuildAgentContext", 10)
			handoff.Source = callerPath
			handoff.AnchorSymbol = "pipeline.busContext"
			ctx := flowOperationCompletionContext([]types.EvidenceItem{handoff})
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "Mutable", ResolvedAs: "Mutable", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			graph := flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				callerPath: {
					RelPath: callerPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{{Name: "run", Kind: "function", Line: 1, EndLine: 20}},
					Relations: []repotypes.Relation{{
						Kind: "call", File: callerPath, Line: 10,
						FromEP:     repotypes.RelationEndpoint{Name: "run", Line: 10},
						ToEP:       repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 10},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
					}},
				},
				calleePath: {
					RelPath: calleePath, Language: language, Package: "builder",
					Symbols: []repotypes.Symbol{
						{Name: "BuildAgentContext", Kind: "function", Line: 20, EndLine: 40},
						{Name: "Mutable", Kind: "field", Parent: "AgentContext", DeclaredType: "MutableState", Line: 30},
					},
					Relations: []repotypes.Relation{{
						Kind: "call", File: calleePath, Line: 29,
						FromEP:     repotypes.RelationEndpoint{Name: "BuildAgentContext", Line: 29},
						ToEP:       repotypes.RelationEndpoint{Name: "Objective", Receiver: "bus.Mutable", Line: 29},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
					}},
					LineFeatures: map[int][]repotypes.LineFeature{30: {repotypes.LineFeatureMemberInitializer}},
				},
			})
			graph.ResolvedImports = map[string][]repotypes.ResolvedImportBinding{
				callerPath: {{
					Import:  repotypes.Import{Alias: "builder", Path: "builder", File: callerPath, Line: 1},
					Targets: []string{calleePath},
				}},
			}
			ctx.Mutable.SetSearchGraph(graph)
			ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{callerPath: true, calleePath: true})
			ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
				callerPath: {{Start: 1, End: 20}}, calleePath: {{Start: 20, End: 40}},
			})

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"Mutable"})
			if !ok || target.file != calleePath || target.lineRange != (types.LineRange{Start: 18, End: 42}) ||
				!target.alreadyRead || !target.receivingCallableBody || target.focusIdentity != "Mutable <- bus.Mutable" {
				t.Fatalf("%s grounded handoff must prefer the exact initializer over the local getter: ok=%t target=%+v", language, ok, target)
			}
			if got := ctx.Mutable.EmittedEvidence(); len(got) != 1 || types.ClaimFormOf(got[0]) != types.ClaimArgumentFlow {
				t.Fatalf("%s initializer navigation must not manufacture evidence: %+v", language, got)
			}
		})
	}
}

func TestFlowOperationNavigationWalksGroundedReceivingBodyBackToExactCallerHandoffAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			callerPath := filepath.ToSlash(filepath.Join("src", language, "pipeline.src"))
			calleePath := filepath.ToSlash(filepath.Join("src", language, "builder.src"))
			callerLines := make([]string, 24)
			callerLines[0] = "package pipeline"
			callerLines[9] = "result := builder.BuildAgentContext(pipeline.busContext, stage)"
			calleeLines := make([]string, 44)
			calleeLines[0] = "package builder"
			calleeLines[29] = "Mutable: bus.Mutable"
			for path, lines := range map[string][]string{callerPath: callerLines, calleePath: calleeLines} {
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			calleeInfo := &repotypes.FileInfo{
				RelPath: calleePath, Language: language, Package: "builder",
				Symbols: []repotypes.Symbol{
					{Name: "BuildAgentContext", Kind: "function", Line: 20, EndLine: 40},
					{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "MutableState", Line: 30},
				},
				LineFeatures: map[int][]repotypes.LineFeature{30: {repotypes.LineFeatureMemberInitializer}},
			}
			graph := flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				callerPath: {
					RelPath: callerPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{
						{Name: "run", Kind: "method", Receiver: "Pipeline", Line: 2, EndLine: 20},
						{Name: "busContext", Kind: "field", Parent: "Pipeline", DeclaredType: "*BusContext", Line: 1},
					},
					Relations: []repotypes.Relation{{
						Kind: "call", File: callerPath, Line: 10,
						FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 10},
						ToEP:       repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 10},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
					}},
				},
				calleePath: calleeInfo,
			})
			graph.ResolvedImports = map[string][]repotypes.ResolvedImportBinding{
				callerPath: {{
					Import:  repotypes.Import{Alias: "builder", Path: "builder", File: callerPath, Line: 1},
					Targets: []string{calleePath},
				}},
			}
			bodyOperation := flowOperationEvidence(types.AnchorInitializer, "Mutable", "bus.Mutable", 30)
			bodyOperation.Source = calleePath
			bodyOperation.Snippet = "Mutable: bus.Mutable"
			bodyOperation.OwnerIdentity = qualifiedEvidenceSymbolNameInFile(calleeInfo, &calleeInfo.Symbols[0])

			ctx := flowOperationCompletionContext([]types.EvidenceItem{bodyOperation})
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "Mutable", ResolvedAs: "Mutable", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(graph)
			ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{callerPath: true, calleePath: true})
			ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
				callerPath: {{Start: 1, End: 24}}, calleePath: {{Start: 20, End: 40}},
			})

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || target.file != callerPath || target.lineRange != (types.LineRange{Start: 1, End: 22}) ||
				!target.alreadyRead || !target.callerHandoff {
				t.Fatalf("%s grounded body operation must reverse-navigate to the unique caller handoff: ok=%t target=%+v", language, ok, target)
			}
			hint := flowOperationNavigationHintForMissing(ctx, []string{"BusContext"}, nil, nil)
			for _, want := range []string{"Exact caller-handoff extraction step", callerPath, "complete source argument"} {
				if !strings.Contains(hint, want) {
					t.Fatalf("%s caller-handoff hint missing %q:\n%s", language, want, hint)
				}
			}
			if got := ctx.Mutable.EmittedEvidence(); len(got) != 1 || got[0].LineStart != 30 {
				t.Fatalf("%s reverse navigation must not manufacture a caller edge: %+v", language, got)
			}
		})
	}
}

func TestFlowOperationNavigationGroundedBodyAmbiguousCallerTargetFailsClosed(t *testing.T) {
	repo := t.TempDir()
	callerPath := "src/pipeline.go"
	for path, body := range map[string]string{
		callerPath:       "package pipeline\nfunc run() {\n builder.BuildAgentContext(pipeline.busContext, stage)\n}\n",
		"src/a/build.go": "package builder\nfunc BuildAgentContext() {}\n",
		"src/b/build.go": "package builder\nfunc BuildAgentContext() {}\n",
	} {
		absolute := filepath.Join(repo, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bodyOperation := flowOperationEvidence(types.AnchorInitializer, "Mutable", "bus.Mutable", 2)
	bodyOperation.Source = "src/a/build.go"
	bodyOperation.OwnerIdentity = "builder.BuildAgentContext"
	ctx := flowOperationCompletionContext([]types.EvidenceItem{bodyOperation})
	ctx.RepoRoot = repo
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	graph := flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		callerPath: {
			RelPath: callerPath, Language: repotypes.LangGo, Package: "pipeline",
			Symbols:   []repotypes.Symbol{{Name: "run", Kind: "function", Line: 2, EndLine: 4}, {Name: "busContext", Kind: "field", Parent: "Pipeline", DeclaredType: "*BusContext", Line: 1}},
			Relations: []repotypes.Relation{{Kind: "call", File: callerPath, Line: 3, ToEP: repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 3}}},
		},
		"src/a/build.go": {RelPath: "src/a/build.go", Language: repotypes.LangGo, Package: "builder", Symbols: []repotypes.Symbol{{Name: "BuildAgentContext", Kind: "function", Line: 1, EndLine: 2}}},
		"src/b/build.go": {RelPath: "src/b/build.go", Language: repotypes.LangGo, Package: "builder", Symbols: []repotypes.Symbol{{Name: "BuildAgentContext", Kind: "function", Line: 1, EndLine: 2}}},
	})
	graph.ResolvedImports = map[string][]repotypes.ResolvedImportBinding{
		callerPath: {{Import: repotypes.Import{Alias: "builder", Path: "builder", File: callerPath}, Targets: []string{"src/a/build.go", "src/b/build.go"}}},
	}
	ctx.Mutable.SetSearchGraph(graph)
	index := flowNavigationIndexForContext(ctx)
	participants := [][]string{{"BusContext"}, {"Mutable"}}
	if target, ok := flowNavigationGroundedBodyOperationCallerHandoffReadTarget(
		ctx, index, participants, [][]string{{"BusContext"}}, ctx.Mutable.EmittedEvidence(),
	); ok || target.callerHandoff {
		t.Fatalf("ambiguous namespace target must fail closed without caller-handoff authority: ok=%t target=%+v", ok, target)
	}
}

func TestFlowOperationNavigationGroundedHandoffAmbiguousReceiverFailsClosed(t *testing.T) {
	callerPath := "src/pipeline.go"
	targetA := "src/a/builder.go"
	targetB := "src/b/builder.go"
	handoff := flowOperationEvidence(types.AnchorArgument, "pipeline.busContext", "BuildAgentContext", 10)
	handoff.Source = callerPath
	ctx := flowOperationCompletionContext([]types.EvidenceItem{handoff})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "Mutable", ResolvedAs: "Mutable", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired}},
	}
	graph := flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		callerPath: {
			RelPath: callerPath, Language: repotypes.LangGo, Package: "pipeline",
			Relations: []repotypes.Relation{{
				Kind: "call", File: callerPath, Line: 10,
				ToEP: repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 10},
			}},
		},
		targetA: {RelPath: targetA, Language: repotypes.LangGo, Package: "builder", Symbols: []repotypes.Symbol{{Name: "BuildAgentContext", Kind: "function", Line: 1, EndLine: 5}}},
		targetB: {RelPath: targetB, Language: repotypes.LangGo, Package: "builder", Symbols: []repotypes.Symbol{{Name: "BuildAgentContext", Kind: "function", Line: 1, EndLine: 5}}},
	})
	graph.ResolvedImports = map[string][]repotypes.ResolvedImportBinding{
		callerPath: {{
			Import:  repotypes.Import{Alias: "builder", Path: "builder", File: callerPath, Line: 1},
			Targets: []string{targetA, targetB},
		}},
	}
	ctx.Mutable.SetSearchGraph(graph)
	index := flowNavigationIndexForContext(ctx)
	if _, ok := flowNavigationGroundedHandoffCalleeOperationReadTarget(
		ctx, index, [][]string{{"Mutable"}}, ctx.Mutable.EmittedEvidence(),
	); ok {
		t.Fatal("ambiguous imported receiver definitions must not choose a callable body frontier")
	}
}

func TestFlowOperationNavigationResolvesAmbiguousMemberFromProjectedFileIndexAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			ctx := flowOperationCompletionContext(nil)
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "Extractor", ResolvedAs: "extractorEvaluator", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "Mutable", Resolution: types.EntityResolutionAmbiguousSymbol, UseForSearch: true},
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			declarationPath := filepath.ToSlash(filepath.Join("src", language, "context.src"))
			componentPath := filepath.ToSlash(filepath.Join("src", language, "extractor.src"))
			files := map[string]*repotypes.FileInfo{
				declarationPath: {
					RelPath: declarationPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{
						{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "MutableState", Line: 5},
						{Name: "Mutable", Kind: "field", Parent: "AgentContext", DeclaredType: "MutableState", Line: 6},
					},
				},
				componentPath: {
					RelPath: componentPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{{Name: "run", Kind: "method", Receiver: "extractorEvaluator", Line: 1, EndLine: 20}},
					Relations: []repotypes.Relation{{
						Kind: "call", File: componentPath, Line: 10,
						FromEP: repotypes.RelationEndpoint{Line: 10},
						ToEP:   repotypes.RelationEndpoint{Name: "TurnAArtifacts", Receiver: "ctx.Mutable", Line: 10},
					}},
				},
			}
			// Production scoped/multi-repo projections may retain complete
			// FileIndex symbols while omitting the legacy name-keyed index.
			ctx.Mutable.SetSearchGraph(&repotypes.Graph{FileIndex: files, SymbolDefs: map[string][]*repotypes.Symbol{}})

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"Mutable", "BusContext"})
			if !ok || target.file != componentPath {
				t.Fatalf("%s projected FileIndex must retain unique member navigation: ok=%t target=%+v", language, ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("projected member navigation must not manufacture relation evidence")
			}
		})
	}
}

func TestFlowOperationNavigationPrefersCarrierHandoffOwnedByAnotherMissingParticipant(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"internal/agent/answer_document_evaluator.go": strings.Join([]string{
			"package agent",
			"func (e *answerDocumentEvaluator) run(ctx *BusContext) {",
			"builder.Check(ctx)",
			"}",
		}, "\n"),
		"internal/agent/extractor.go": strings.Join([]string{
			"package agent",
			"func (e *extractorEvaluator) run(ctx *BusContext) {",
			"builder.Build(ctx)",
			"}",
		}, "\n"),
	}
	for path, body := range files {
		absolute := filepath.Join(repo, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	ctx := flowOperationCompletionContext(nil)
	ctx.RepoRoot = repo
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "extractor", ResolvedAs: "extractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	graphFiles := make(map[string]*repotypes.FileInfo, len(files))
	for path := range files {
		receiver := "answerDocumentEvaluator"
		callee := "Check"
		if strings.HasSuffix(path, "extractor.go") {
			receiver = "extractorEvaluator"
			callee = "Build"
		}
		graphFiles[path] = &repotypes.FileInfo{
			RelPath: path, Language: repotypes.LangGo,
			Symbols: []repotypes.Symbol{{Name: "ctx", Kind: "parameter", Parent: receiver + ".run", DeclaredType: "*BusContext", Line: 2}},
			Relations: []repotypes.Relation{{
				Kind: "call", File: path, Line: 3,
				FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: receiver, Line: 3},
				ToEP:       repotypes.RelationEndpoint{Name: callee, Receiver: "builder", Line: 3},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_call",
			}},
		}
	}
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(graphFiles))

	target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"extractor", "BusContext"})
	if !ok || target.file != "internal/agent/extractor.go" ||
		target.lineRange != (types.LineRange{Start: 1, End: 15}) {
		t.Fatalf("carrier repair should prefer the operation owned by another missing participant: ok=%t target=%+v", ok, target)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Fatal("participant-aware navigation must not manufacture relation evidence")
	}
}

func TestFlowOperationNavigationAdvancesThroughSameCallableResultConsumersAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			path := filepath.ToSlash(filepath.Join("src", language, "dispatcher.src"))
			lines := make([]string, 70)
			lines[0] = "run dispatcher"
			lines[9] = "agentContext := builder.Build(this.busContext, stage)"
			lines[29] = "output, err := agent.Execute(agentContext, skill)"
			lines[49] = "pipeline.Apply(output)"
			absolute := filepath.Join(repo, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(absolute, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
				t.Fatal(err)
			}

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{{
					Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
				}},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				path: {
					RelPath: path, Language: language,
					Symbols: []repotypes.Symbol{
						{Name: "busContext", Parent: "Pipeline", DeclaredType: "BusContext", Line: 2},
						{Name: "run", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 70},
					},
					Relations: []repotypes.Relation{
						{
							Kind: "call", File: path, Line: 10,
							FromEP: repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 10},
							ToEP:   repotypes.RelationEndpoint{Name: "Build", Receiver: "builder", Line: 10},
						},
						{
							Kind: "call", File: path, Line: 30,
							FromEP: repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 30},
							ToEP:   repotypes.RelationEndpoint{Name: "Execute", Receiver: "agent", Line: 30},
						},
						{
							Kind: "call", File: path, Line: 50,
							FromEP: repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 50},
							ToEP:   repotypes.RelationEndpoint{Name: "Apply", Receiver: "pipeline", Line: 50},
						},
					},
				},
			}))
			ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{path: true})
			ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
				path: {{Start: 1, End: 22}},
			})
			ctx.Mutable.AppendEvidence([]types.EvidenceItem{
				flowOperationEvidence(types.AnchorCall, "BusContext", "helper.Read", 5),
			})
			files, keywords := flowOperationRepairTargets(ctx, []string{"BusContext"}, ctx.Mutable.EmittedEvidence())
			hint := flowParticipantCoverageNavigationHint(
				ctx, []string{"BusContext"}, ctx.Mutable.EmittedEvidence(), files, keywords,
			)
			if !strings.Contains(hint, "Exact next navigation step") ||
				!strings.Contains(hint, "lines 18-42") {
				t.Fatalf("%s participant repair must retain the exact continuation read:\n%s", language, hint)
			}
			queueFlowParticipantCoverageRepair(ctx, []string{"BusContext"}, ctx.Mutable.EmittedEvidence())
			var queued bool
			for _, repair := range ctx.Mutable.EvidenceClosure().ActiveRepairs() {
				if repair.Kind == types.RepairReadFile && strings.Contains(repair.Origin, types.RepairOriginFlowNavigationPrefix) &&
					len(repair.LineRanges) == 1 && repair.LineRanges[0] == (types.LineRange{Start: 18, End: 42}) {
					queued = true
				}
			}
			if !queued {
				t.Fatalf("%s participant repair did not queue the exact result consumer: %+v", language, ctx.Mutable.EvidenceClosure().ActiveRepairs())
			}

			first, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || first.file != path || first.lineRange != (types.LineRange{Start: 18, End: 42}) || first.alreadyRead {
				t.Fatalf("%s first continuation must read the exact result consumer: ok=%t target=%+v", language, ok, first)
			}
			ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
				path: {{Start: 18, End: 42}},
			})
			// Reading the coordinate is not proof that the value was handed off.
			// Keep the frontier at Execute until the model emits the exact
			// source-owned argument row, then advance to Apply.
			pending, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || pending.file != path || pending.lineRange != (types.LineRange{Start: 18, End: 42}) || !pending.alreadyRead {
				t.Fatalf("%s read closure must not impersonate argument evidence: ok=%t target=%+v", language, ok, pending)
			}
			executeHandoff := flowOperationEvidence(types.AnchorArgument, "agentContext", "Execute", 30)
			executeHandoff.Source = path
			ctx.Mutable.AppendEvidence([]types.EvidenceItem{executeHandoff})
			second, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || second.file != path || second.lineRange != (types.LineRange{Start: 38, End: 62}) || second.alreadyRead {
				t.Fatalf("%s second continuation must read result application: ok=%t target=%+v", language, ok, second)
			}
			if got := ctx.Mutable.EmittedEvidence(); len(got) != 2 || got[0].Subject != "BusContext" || got[1].Subject != "agentContext" {
				t.Fatalf("same-callable continuation navigation must not manufacture evidence: %+v", got)
			}
		})
	}
}

func TestFlowOperationNavigationReturnsToRequestedSiblingArgumentAfterCalleeBodyEvidence(t *testing.T) {
	repo := t.TempDir()
	const dispatcherPath = "internal/orchestrator/dispatcher.go"
	const builderPath = "internal/context/builder.go"
	for path, body := range map[string]string{
		dispatcherPath: strings.Join([]string{
			"package orchestrator",
			"func (o *Orchestrator) dispatch() {",
			"ac := ctxbuilder.BuildAgentContext(o.busCtx, types.AgentExtractor, types.StageExtract)",
			"_ = ac",
			"}",
		}, "\n"),
		builderPath: strings.Join([]string{
			"package context",
			"func BuildAgentContext(bus *types.BusContext, agentName types.AgentName, stage types.PipelineStage) *types.AgentContext {",
			"objective := bus.Mutable.Objective()",
			"return &types.AgentContext{Objective: objective}",
			"}",
		}, "\n"),
	} {
		absolute := filepath.Join(repo, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	busHandoff := flowOperationEvidence(types.AnchorArgument, "o.busCtx", "BuildAgentContext", 3)
	busHandoff.Source = dispatcherPath
	busHandoff.OwnerIdentity = "Orchestrator.dispatch"
	busHandoff.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "o.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
	}}
	bodyOperation := flowOperationEvidence(types.AnchorCall, "BuildAgentContext", "bus.Mutable.Objective", 3)
	bodyOperation.Source = builderPath
	bodyOperation.OwnerIdentity = "context.BuildAgentContext"
	bodyOperation.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "bus.Mutable", Type: "*MutableState", Owner: "BuildAgentContext",
	}}
	ctx := flowOperationCompletionContext([]types.EvidenceItem{busHandoff, bodyOperation})
	ctx.RepoRoot = repo
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "Extractor", ResolvedAs: "types.AgentExtractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "BusContext", ResolvedAs: "types.BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "Mutable", ResolvedAs: "MutableState", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	graph := flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		dispatcherPath: {
			RelPath: dispatcherPath, Language: repotypes.LangGo, Package: "orchestrator",
			Symbols: []repotypes.Symbol{
				{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", Line: 1},
				{Name: "dispatch", Kind: "method", Receiver: "Orchestrator", Line: 2, EndLine: 5},
			},
			Relations: []repotypes.Relation{{
				Kind: "call", File: dispatcherPath, Line: 3,
				FromEP:     repotypes.RelationEndpoint{Name: "dispatch", Receiver: "Orchestrator", Line: 3},
				ToEP:       repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "ctxbuilder", Line: 3},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
			}},
		},
		builderPath: {
			RelPath: builderPath, Language: repotypes.LangGo, Package: "context",
			Symbols: []repotypes.Symbol{{Name: "BuildAgentContext", Kind: "function", Line: 2, EndLine: 5}},
			Relations: []repotypes.Relation{{
				Kind: "call", File: builderPath, Line: 3,
				FromEP:     repotypes.RelationEndpoint{Name: "BuildAgentContext", Line: 3},
				ToEP:       repotypes.RelationEndpoint{Name: "Objective", Receiver: "bus.Mutable", Line: 3},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
			}},
		},
	})
	graph.ResolvedImports = map[string][]repotypes.ResolvedImportBinding{
		dispatcherPath: {{
			Import:  repotypes.Import{Alias: "ctxbuilder", Path: "context", File: dispatcherPath, Line: 1},
			Targets: []string{builderPath},
		}},
	}
	ctx.Mutable.SetSearchGraph(graph)
	ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{dispatcherPath: true, builderPath: true})
	ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
		dispatcherPath: {{Start: 1, End: 5}}, builderPath: {{Start: 1, End: 5}},
	})

	target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext", "Mutable"})
	if !ok || !target.callerHandoff || !target.alreadyRead || target.file != dispatcherPath ||
		target.focusIdentity != "types.AgentExtractor" {
		t.Fatalf("callee-body frontier must return to the un-emitted requested sibling argument: ok=%t target=%+v", ok, target)
	}
	hint := flowOperationNavigationHintForMissing(ctx, []string{"BusContext", "Mutable"}, nil, nil)
	for _, want := range []string{`exact complete source argument "types.AgentExtractor"`, "request-relevant sibling arguments"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("sibling-argument extraction guidance missing %q:\n%s", want, hint)
		}
	}
	keyWithSiblingFrontier, hasFrontier := flowParticipantCoverageBlockerKey(ctx, []string{"BusContext", "Mutable"})
	if !hasFrontier {
		t.Fatal("parser-owned sibling argument must participate in the typed progress frontier")
	}

	siblingHandoff := flowOperationEvidence(types.AnchorArgument, "types.AgentExtractor", "BuildAgentContext", 3)
	siblingHandoff.Source = dispatcherPath
	siblingHandoff.OwnerIdentity = "Orchestrator.dispatch"
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{siblingHandoff})
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("model-authored sibling argument should join the requested carrier component: %v", got)
	}
	keyAfterJoin, _ := flowParticipantCoverageBlockerKey(ctx, []string{"BusContext", "Mutable"})
	if keyAfterJoin != keyWithSiblingFrontier {
		t.Fatal("navigation cursor changes must not reset convergence while the typed missing set is unchanged")
	}
	if len(ctx.Mutable.EmittedEvidence()) != 3 {
		t.Fatal("navigation must not manufacture the sibling evidence row")
	}
}

func TestFlowParticipantCoverageBlockerKeyIgnoresAdvancingNavigationCursor(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "Dispatch", "BuildContext", 42),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Analyzer"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	missing := []string{"Analyzer"}
	before, _ := flowParticipantCoverageBlockerKey(ctx, missing)

	// An unrelated but precise source operation can advance the parser-owned
	// repair cursor. It does not cover Analyzer and therefore is not progress on
	// this gate's proof debt.
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "BuildContext", "local.Load", 43),
	})
	after, _ := flowParticipantCoverageBlockerKey(ctx, missing)
	if before != after {
		t.Fatalf("same typed missing set must keep one convergence identity: before=%d after=%d", before, after)
	}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); !flowTestSliceContains(got, "Analyzer") {
		t.Fatalf("fixture accidentally resolved the requested participant: %v", got)
	}
}

func TestFlowOperationNavigationPrefersDirectMultiParticipantOperationOverUnrelatedCarrierArgument(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"internal/agent/answer_document_evaluator.go": strings.Join([]string{
			"package agent",
			"func (e *answerDocumentEvaluator) run(ctx *BusContext) {",
			"builder.Check(ctx)",
			"}",
		}, "\n"),
		"internal/agent/extractor.go": strings.Join([]string{
			"package agent",
			"func (e *extractorEvaluator) run(ctx *BusContext) {",
			"ctx.Mutable.TurnAArtifacts()",
			"}",
		}, "\n"),
	}
	for path, body := range files {
		absolute := filepath.Join(repo, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	ctx := flowOperationCompletionContext(nil)
	ctx.RepoRoot = repo
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "extractor", ResolvedAs: "extractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "Mutable", ResolvedAs: "Mutable", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		"internal/agent/answer_document_evaluator.go": {
			RelPath: "internal/agent/answer_document_evaluator.go", Language: repotypes.LangGo,
			Symbols: []repotypes.Symbol{{Name: "ctx", Kind: "parameter", Parent: "answerDocumentEvaluator.run", DeclaredType: "*BusContext", Line: 2}},
			Relations: []repotypes.Relation{{
				Kind: "call", File: "internal/agent/answer_document_evaluator.go", Line: 3,
				FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "answerDocumentEvaluator", Line: 3},
				ToEP:       repotypes.RelationEndpoint{Name: "Check", Receiver: "builder", Line: 3},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_call",
			}},
		},
		"internal/agent/extractor.go": {
			RelPath: "internal/agent/extractor.go", Language: repotypes.LangGo,
			Symbols: []repotypes.Symbol{
				{Name: "run", Kind: "method", Receiver: "extractorEvaluator", Line: 2, EndLine: 4},
				{Name: "ctx", Kind: "parameter", Parent: "extractorEvaluator.run", DeclaredType: "*BusContext", Line: 2},
			},
			Relations: []repotypes.Relation{{
				Kind: "call", File: "internal/agent/extractor.go", Line: 3,
				// Several language extractors leave FromEP empty on member calls;
				// the exact enclosing method range is the parser-owned owner.
				FromEP:     repotypes.RelationEndpoint{Line: 3},
				ToEP:       repotypes.RelationEndpoint{Name: "TurnAArtifacts", Receiver: "ctx.Mutable", Line: 3},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_call",
			}},
		},
	}))

	target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"extractor", "Mutable", "BusContext"})
	if !ok || target.file != "internal/agent/extractor.go" ||
		target.lineRange != (types.LineRange{Start: 1, End: 15}) {
		t.Fatalf("direct operation touching two requested participants must outrank an unrelated carrier argument: ok=%t target=%+v", ok, target)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Fatal("multi-participant navigation rank must not manufacture relation evidence")
	}
}

func TestFlowOperationNavigationPrefersValueHandoffCallOverMultiParticipantTypeOccurrenceAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			typeUsePath := filepath.ToSlash(filepath.Join("src", language, "analyzer.src"))
			ownerPath := filepath.ToSlash(filepath.Join("src", language, "orchestrator_owner.src"))
			handoffPath := filepath.ToSlash(filepath.Join("src", language, "extract_handoff.src"))
			for path, body := range map[string]string{
				typeUsePath: strings.Join([]string{
					"analyzer bus context helper", "", "", "", "", "", "", "", "", "BusContext local view",
				}, "\n"),
				ownerPath: "orchestrator owns bus context",
				handoffPath: strings.Join([]string{
					"extract stage handoff", "", "", "", "", "", "", "", "", "builder.BuildAgentContext(this.busContext, AgentExtractor)",
				}, "\n"),
			} {
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "analyzer", ResolvedAs: "analyzerEvaluator", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "extractor", Resolution: types.EntityResolutionInferredConcept},
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramArchitecture, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "analyzer", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				typeUsePath: {
					RelPath: typeUsePath, Language: language,
					Symbols: []repotypes.Symbol{{Name: "inspect", Kind: "method", Receiver: "analyzerEvaluator", Line: 1, EndLine: 10}},
					Relations: []repotypes.Relation{{
						Kind: "type_usage", File: typeUsePath, Line: 10,
						FromEP:     repotypes.RelationEndpoint{Line: 10},
						ToEP:       repotypes.RelationEndpoint{Name: "BusContext", Line: 10},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
					}},
				},
				ownerPath: {
					RelPath: ownerPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{{Name: "busContext", Kind: "field", Parent: "Orchestrator", DeclaredType: "BusContext", Line: 1}},
				},
				handoffPath: {
					RelPath: handoffPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{{Name: "extract", Kind: "method", Receiver: "Orchestrator", Line: 1, EndLine: 10}},
					Relations: []repotypes.Relation{{
						Kind: "call", File: handoffPath, Line: 10,
						FromEP:     repotypes.RelationEndpoint{Line: 10},
						ToEP:       repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 10},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter,
					}},
				},
			}))

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || target.file != handoffPath || target.lineRange != (types.LineRange{Start: 1, End: 22}) {
				t.Fatalf("%s complete carrier handoff call must outrank a multi-label type occurrence: ok=%t target=%+v", language, ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("value-handoff navigation must not manufacture relation evidence")
			}
		})
	}
}

func TestFlowOperationNavigationBalancesBindingBudgetAcrossMissingParticipantsAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			files := make(map[string]*repotypes.FileInfo)
			// More than the legacy global file budget of one carrier. These
			// legitimate Mutable bindings must not starve the independently
			// requested BusContext carrier before candidate quality is scored.
			for i := 0; i < maxFlowOperationRepairFiles+2; i++ {
				path := filepath.ToSlash(filepath.Join("src", language, "a_state_"+string(rune('a'+i))+".src"))
				lines := make([]string, 12)
				lines[9] = "mutable.Read()"
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
					t.Fatal(err)
				}
				files[path] = &repotypes.FileInfo{
					RelPath: path, Language: language,
					Symbols: []repotypes.Symbol{{
						Name: "mutable", Kind: "field", Parent: "StateOwner", DeclaredType: "MutableState", Line: 1,
					}},
					Relations: []repotypes.Relation{{
						Kind: "call", File: path, Line: 10,
						ToEP: repotypes.RelationEndpoint{Name: "Read", Receiver: "mutable", Line: 10},
					}},
				}
			}

			localPath := filepath.ToSlash(filepath.Join("src", language, "b_local.src"))
			dispatchPath := filepath.ToSlash(filepath.Join("src", language, "z_dispatch.src"))
			localLines := make([]string, 12)
			localLines[9] = "busContext.Context()"
			dispatchLines := make([]string, 40)
			dispatchLines[9] = "renderer.Render(this.busContext.Language)"
			dispatchLines[29] = "agentContext := builder.BuildAgentContext(this.busContext, AgentFinalizer, stage)"
			for path, body := range map[string]string{
				localPath: strings.Join(localLines, "\n"), dispatchPath: strings.Join(dispatchLines, "\n"),
			} {
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			files[localPath] = &repotypes.FileInfo{
				RelPath: localPath, Language: language,
				Relations: []repotypes.Relation{{
					Kind: "call", File: localPath, Line: 10,
					ToEP: repotypes.RelationEndpoint{Name: "Context", Receiver: "BusContext", Line: 10},
				}},
			}
			files[dispatchPath] = &repotypes.FileInfo{
				RelPath: dispatchPath, Language: language,
				Symbols: []repotypes.Symbol{
					{Name: "busContext", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 1},
					{Name: "dispatch", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 40},
				},
				Relations: []repotypes.Relation{
					{Kind: "call", File: dispatchPath, Line: 10,
						FromEP: repotypes.RelationEndpoint{Name: "dispatch", Receiver: "Pipeline", Line: 10},
						ToEP:   repotypes.RelationEndpoint{Name: "Render", Receiver: "renderer", Line: 10}},
					{Kind: "call", File: dispatchPath, Line: 30,
						FromEP: repotypes.RelationEndpoint{Name: "dispatch", Receiver: "Pipeline", Line: 30},
						ToEP:   repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 30}},
				},
			}

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "Finalizer", ResolvedAs: "AgentFinalizer", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "Mutable", ResolvedAs: "MutableState", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(files))

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"Mutable", "BusContext"})
			if !ok || target.file != dispatchPath || target.lineRange != (types.LineRange{Start: 18, End: 42}) {
				t.Fatalf("%s per-participant binding budget must retain the whole-carrier cross-component handoff: ok=%t target=%+v", language, ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("balanced navigation must not manufacture relation evidence")
			}
		})
	}
}

func TestFlowOperationNavigationPrefersMissingToCoveredParticipantJoinAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			localPath := filepath.ToSlash(filepath.Join("src", language, "a_local.src"))
			handoffPath := filepath.ToSlash(filepath.Join("src", language, "z_extract.src"))
			localLines := make([]string, 20)
			localLines[9] = "env.Value = this.busContext.Mutable.Read()"
			handoffLines := make([]string, 40)
			handoffLines[29] = "agentContext := builder.BuildAgentContext(this.busContext, AgentExtractor, stage)"
			for path, body := range map[string]string{
				localPath: strings.Join(localLines, "\n"), handoffPath: strings.Join(handoffLines, "\n"),
			} {
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "extractor", ResolvedAs: "extractorToolObligationView", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "Mutable", ResolvedAs: "MutableState", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				localPath: {
					RelPath: localPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{
						{Name: "busContext", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 1},
						{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "MutableState", Line: 1},
						{Name: "buildEnv", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 20},
					},
					Relations: []repotypes.Relation{{
						Kind: "call", File: localPath, Line: 10,
						FromEP: repotypes.RelationEndpoint{Name: "buildEnv", Receiver: "Pipeline", Line: 10},
						ToEP:   repotypes.RelationEndpoint{Name: "Read", Receiver: "this.busContext.Mutable", Line: 10},
					}},
				},
				handoffPath: {
					RelPath: handoffPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{
						{Name: "extractStageHasRequiredWork", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 40},
					},
					Relations: []repotypes.Relation{{
						Kind: "call", File: handoffPath, Line: 30,
						FromEP: repotypes.RelationEndpoint{Name: "extractStageHasRequiredWork", Receiver: "Pipeline", Line: 30},
						ToEP:   repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 30},
					}},
				},
			}))

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"Mutable", "BusContext"})
			if !ok || target.file != handoffPath || target.lineRange != (types.LineRange{Start: 18, End: 42}) {
				t.Fatalf("%s join from missing carriers to the covered stage must outrank a disconnected local read: ok=%t target=%+v", language, ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("connection-gain navigation must not manufacture relation evidence")
			}
		})
	}
}

func TestFlowOperationNavigationPrefersCallSiteWithWholeValueContinuationAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			path := filepath.ToSlash(filepath.Join("src", language, "pipeline.src"))
			lines := make([]string, 45)
			lines[9] = "probe = builder.BuildAgentContext(this.busContext, AgentExtractor, stage)"
			lines[10] = "count = len(probe.Rows)"
			lines[29] = "agentContext = builder.BuildAgentContext(this.busContext, AgentExtractor, stage)"
			lines[30] = "output = agent.Execute(agentContext)"
			lines[31] = "pipeline.Apply(output)"
			absolute := filepath.Join(repo, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(absolute, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
				t.Fatal(err)
			}

			probeBuild := &repotypes.Relation{
				Kind: "call", File: path, Line: 10,
				FromEP: repotypes.RelationEndpoint{Name: "probeStage", Receiver: "Pipeline", Line: 10},
				ToEP:   repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 10},
			}
			dispatchBuild := &repotypes.Relation{
				Kind: "call", File: path, Line: 30,
				FromEP: repotypes.RelationEndpoint{Name: "dispatch", Receiver: "Pipeline", Line: 30},
				ToEP:   repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 30},
			}
			graph := flowTestIndexedGraph(map[string]*repotypes.FileInfo{path: {
				RelPath: path, Language: language, Package: "pipeline",
				Symbols: []repotypes.Symbol{
					{Name: "busContext", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 1},
					{Name: "probeStage", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 20},
					{Name: "dispatch", Kind: "method", Receiver: "Pipeline", Line: 21, EndLine: 40},
				},
				Relations: []repotypes.Relation{
					*probeBuild,
					{Kind: "call", File: path, Line: 11,
						FromEP: repotypes.RelationEndpoint{Name: "probeStage", Receiver: "Pipeline", Line: 11},
						ToEP:   repotypes.RelationEndpoint{Name: "len", Line: 11}},
					*dispatchBuild,
					{Kind: "call", File: path, Line: 31,
						FromEP: repotypes.RelationEndpoint{Name: "dispatch", Receiver: "Pipeline", Line: 31},
						ToEP:   repotypes.RelationEndpoint{Name: "Execute", Receiver: "agent", Line: 31}},
					{Kind: "call", File: path, Line: 32,
						FromEP: repotypes.RelationEndpoint{Name: "dispatch", Receiver: "Pipeline", Line: 32},
						ToEP:   repotypes.RelationEndpoint{Name: "Apply", Receiver: "pipeline", Line: 32}},
				},
			}})

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "extractor", ResolvedAs: "AgentExtractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(graph)
			index := flowNavigationIndexForContext(ctx)
			probeSite, dispatchSite := index.relationsByFile[path][0], index.relationsByFile[path][2]
			if got := flowNavigationCallResultContinuationDepth(ctx, index, probeSite.relation, probeSite.ownerSurfaces); got != 0 {
				t.Fatalf("%s member projection must not count as a whole-value continuation: %d", language, got)
			}
			if got := flowNavigationCallResultContinuationDepth(ctx, index, dispatchSite.relation, dispatchSite.ownerSurfaces); got != 2 {
				t.Fatalf("%s dispatch continuation depth=%d, want 2", language, got)
			}

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || target.file != path || target.lineRange != (types.LineRange{Start: 18, End: 42}) {
				t.Fatalf("%s full-value dispatcher must outrank the earlier probe call: ok=%t target=%+v", language, ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("continuation-depth navigation must not manufacture relation evidence")
			}
		})
	}
}

func TestFlowOperationContinuationDepthSolvesOwnerDAGOnce(t *testing.T) {
	repo := t.TempDir()
	path := "src/pipeline.go"
	const callCount = 160
	lines := make([]string, callCount)
	relations := make([]repotypes.Relation, callCount)
	for i := 0; i < callCount; i++ {
		callee := "Step"
		argument := fmt.Sprintf("v%d", i)
		if i == 0 {
			callee = "Build"
			argument = "seed"
		}
		lines[i] = fmt.Sprintf("v%d := %s(%s)", i+1, callee, argument)
		relations[i] = repotypes.Relation{
			Kind: "call", File: path, Line: i + 1,
			FromEP: repotypes.RelationEndpoint{Name: "pipeline", Line: i + 1},
			ToEP:   repotypes.RelationEndpoint{Name: callee, Line: i + 1},
		}
	}
	absolute := filepath.Join(repo, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	graph := flowTestIndexedGraph(map[string]*repotypes.FileInfo{path: {
		RelPath: path, Language: "go", Package: "pipeline",
		Symbols:   []repotypes.Symbol{{Name: "pipeline", Kind: "function", Line: 1, EndLine: callCount}},
		Relations: relations,
	}})
	ctx := flowOperationCompletionContext(nil)
	ctx.RepoRoot = repo
	ctx.Mutable.SetSearchGraph(graph)
	index := flowNavigationIndexForContext(ctx)
	if index == nil {
		t.Fatal("missing navigation index")
	}
	sites := index.relationsByFile[path]
	if len(sites) != callCount {
		t.Fatalf("indexed call sites=%d, want %d", len(sites), callCount)
	}
	for i, site := range sites {
		got := flowNavigationCallResultContinuationDepth(ctx, index, site.relation, site.ownerSurfaces)
		want := min(flowNavigationCallResultContinuationMaxHops, callCount-i-1)
		if got != want {
			t.Fatalf("site %d depth=%d, want %d", i, got, want)
		}
	}
	if got := len(index.callResultContinuationByOwner); got != 1 {
		t.Fatalf("same owner DAG should be compiled once, cache entries=%d", got)
	}
}

func TestFlowOperationNavigationCarriesContinuationRankAcrossGroundedFastPathsAllLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			callerPath := filepath.ToSlash(filepath.Join("src", language, "pipeline.src"))
			calleePath := filepath.ToSlash(filepath.Join("src", language, "builder.src"))
			callerLines := make([]string, 45)
			callerLines[9] = "probe = builder.BuildAgentContext(this.busContext, AgentExtractor, stage)"
			callerLines[10] = "count = len(probe.Rows)"
			callerLines[29] = "agentContext = builder.BuildAgentContext(this.busContext, AgentExtractor, stage)"
			callerLines[30] = "output = agent.Execute(agentContext)"
			callerLines[31] = "pipeline.Apply(output)"
			calleeLines := make([]string, 45)
			calleeLines[29] = "Mutable: bus.Mutable"
			for path, lines := range map[string][]string{callerPath: callerLines, calleePath: calleeLines} {
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			calleeInfo := &repotypes.FileInfo{
				RelPath: calleePath, Language: language, Package: "builder",
				Symbols: []repotypes.Symbol{
					{Name: "BuildAgentContext", Kind: "function", Line: 20, EndLine: 40},
					{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "MutableState", Line: 30},
				},
				LineFeatures: map[int][]repotypes.LineFeature{30: {repotypes.LineFeatureMemberInitializer}},
			}
			graph := flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				callerPath: {
					RelPath: callerPath, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{
						{Name: "busContext", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 1},
						{Name: "probeStage", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 20},
						{Name: "dispatch", Kind: "method", Receiver: "Pipeline", Line: 21, EndLine: 40},
					},
					Relations: []repotypes.Relation{
						{Kind: "call", File: callerPath, Line: 10,
							FromEP: repotypes.RelationEndpoint{Name: "probeStage", Receiver: "Pipeline", Line: 10},
							ToEP:   repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 10}},
						{Kind: "call", File: callerPath, Line: 11,
							FromEP: repotypes.RelationEndpoint{Name: "probeStage", Receiver: "Pipeline", Line: 11},
							ToEP:   repotypes.RelationEndpoint{Name: "len", Line: 11}},
						{Kind: "call", File: callerPath, Line: 30,
							FromEP: repotypes.RelationEndpoint{Name: "dispatch", Receiver: "Pipeline", Line: 30},
							ToEP:   repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 30}},
						{Kind: "call", File: callerPath, Line: 31,
							FromEP: repotypes.RelationEndpoint{Name: "dispatch", Receiver: "Pipeline", Line: 31},
							ToEP:   repotypes.RelationEndpoint{Name: "Execute", Receiver: "agent", Line: 31}},
						{Kind: "call", File: callerPath, Line: 32,
							FromEP: repotypes.RelationEndpoint{Name: "dispatch", Receiver: "Pipeline", Line: 32},
							ToEP:   repotypes.RelationEndpoint{Name: "Apply", Receiver: "pipeline", Line: 32}},
					},
				},
				calleePath: calleeInfo,
			})
			graph.ResolvedImports = map[string][]repotypes.ResolvedImportBinding{
				callerPath: {{
					Import:  repotypes.Import{Alias: "builder", Path: "builder", File: callerPath, Line: 1},
					Targets: []string{calleePath},
				}},
			}
			bodyOperation := flowOperationEvidence(types.AnchorInitializer, "Mutable", "bus.Mutable", 30)
			bodyOperation.Source = calleePath
			bodyOperation.Snippet = "Mutable: bus.Mutable"
			bodyOperation.OwnerIdentity = qualifiedEvidenceSymbolNameInFile(calleeInfo, &calleeInfo.Symbols[0])

			ctx := flowOperationCompletionContext([]types.EvidenceItem{bodyOperation})
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "Mutable", ResolvedAs: "Mutable", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(graph)
			ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{callerPath: true, calleePath: true})
			ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
				callerPath: {{Start: 1, End: 45}}, calleePath: {{Start: 20, End: 40}},
			})

			first, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || first.file != callerPath || first.lineRange != (types.LineRange{Start: 18, End: 42}) ||
				!first.callerHandoff {
				t.Fatalf("%s grounded caller fast path selected the probe instead of dispatcher: ok=%t target=%+v", language, ok, first)
			}

			buildHandoff := flowOperationEvidence(types.AnchorArgument, "this.busContext", "BuildAgentContext", 30)
			buildHandoff.Source = callerPath
			ctx.Mutable.AppendEvidence([]types.EvidenceItem{buildHandoff})
			second, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"Mutable"})
			if !ok || second.file != callerPath || second.lineRange != (types.LineRange{Start: 19, End: 43}) ||
				second.callerHandoff || !second.alreadyRead {
				t.Fatalf("%s grounded handoff did not resume at Execute: ok=%t target=%+v", language, ok, second)
			}

			executeHandoff := flowOperationEvidence(types.AnchorArgument, "agentContext", "Execute", 31)
			executeHandoff.Source = callerPath
			ctx.Mutable.AppendEvidence([]types.EvidenceItem{executeHandoff})
			third, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"Mutable"})
			if !ok || third.file != callerPath || third.lineRange != (types.LineRange{Start: 20, End: 44}) ||
				third.callerHandoff || !third.alreadyRead {
				t.Fatalf("%s grounded continuation did not advance to Apply: ok=%t target=%+v", language, ok, third)
			}
			if got := ctx.Mutable.EmittedEvidence(); len(got) != 3 {
				t.Fatalf("navigation manufactured evidence instead of preserving three model rows: %+v", got)
			}
		})
	}
}

func TestFlowOperationNavigationDoesNotLexicallyStarveHighGainBindingWithinParticipantAcrossLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			repo := t.TempDir()
			files := make(map[string]*repotypes.FileInfo)
			for i := 0; i < maxFlowOperationRepairFiles+2; i++ {
				path := filepath.ToSlash(filepath.Join("src", language, "a_local_"+string(rune('a'+i))+".src"))
				alias := "localBus" + string(rune('A'+i))
				lines := make([]string, 15)
				lines[9] = alias + ".Read()"
				absolute := filepath.Join(repo, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(absolute, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
					t.Fatal(err)
				}
				files[path] = &repotypes.FileInfo{
					RelPath: path, Language: language, Package: "pipeline",
					Symbols: []repotypes.Symbol{
						{Name: alias, Kind: "field", Parent: "Local" + string(rune('A'+i)), DeclaredType: "BusContext", Line: 1},
						{Name: "inspect", Kind: "method", Receiver: "Local" + string(rune('A'+i)), Line: 1, EndLine: 15},
					},
					Relations: []repotypes.Relation{{
						Kind: "call", File: path, Line: 10,
						FromEP: repotypes.RelationEndpoint{Name: "inspect", Receiver: "Local" + string(rune('A'+i)), Line: 10},
						ToEP:   repotypes.RelationEndpoint{Name: "Read", Receiver: alias, Line: 10},
					}},
				}
			}

			handoffPath := filepath.ToSlash(filepath.Join("src", language, "z_dispatch.src"))
			handoffLines := make([]string, 40)
			handoffLines[29] = "agentContext := builder.BuildAgentContext(this.busContext, AgentExtractor, stage)"
			absolute := filepath.Join(repo, filepath.FromSlash(handoffPath))
			if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(absolute, []byte(strings.Join(handoffLines, "\n")), 0o600); err != nil {
				t.Fatal(err)
			}
			files[handoffPath] = &repotypes.FileInfo{
				RelPath: handoffPath, Language: language, Package: "pipeline",
				Symbols: []repotypes.Symbol{
					{Name: "busContext", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 1},
					{Name: "extractorDispatch", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 40},
				},
				Relations: []repotypes.Relation{{
					Kind: "call", File: handoffPath, Line: 30,
					FromEP: repotypes.RelationEndpoint{Name: "extractorDispatch", Receiver: "Pipeline", Line: 30},
					ToEP:   repotypes.RelationEndpoint{Name: "BuildAgentContext", Receiver: "builder", Line: 30},
				}},
			}

			ctx := flowOperationCompletionContext(nil)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "extractor", ResolvedAs: "extractorToolObligationView", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
				{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(files))

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
			if !ok || target.file != handoffPath || target.lineRange != (types.LineRange{Start: 18, End: 42}) {
				t.Fatalf("%s high-gain binding must survive same-participant lexical file caps: ok=%t target=%+v", language, ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("binding prioritization must not manufacture relation evidence")
			}
		})
	}
}

func TestFlowOperationNavigationUsesCanonicalTypedParticipantResolution(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Extractor", "Mutable", "BusContext"}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "Extractor", ResolvedAs: "extractorEvaluator", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true},
		{Surface: "Mutable", ResolvedAs: "MutableState", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true},
		{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		"internal/orchestrator/helper.go": {
			RelPath: "internal/orchestrator/helper.go", Language: repotypes.LangGo,
			Symbols: []repotypes.Symbol{{Name: "busCtx", Kind: "parameter", Parent: "helper", DeclaredType: "*BusContext", Line: 2}},
			Relations: []repotypes.Relation{{
				Kind: "call", File: "internal/orchestrator/helper.go", Line: 3,
				FromEP: repotypes.RelationEndpoint{Name: "helper", Line: 3},
				ToEP:   repotypes.RelationEndpoint{Name: "Context", Receiver: "BusContext", Line: 3},
			}},
		},
		"internal/agent/extractor.go": {
			RelPath: "internal/agent/extractor.go", Language: repotypes.LangGo,
			Symbols: []repotypes.Symbol{{Name: "BuildInitialInstruction", Kind: "method", Receiver: "extractorEvaluator", Line: 241, EndLine: 631}},
			Relations: []repotypes.Relation{{
				Kind: "call", File: "internal/agent/extractor.go", Line: 262,
				FromEP: repotypes.RelationEndpoint{Line: 262},
				ToEP:   repotypes.RelationEndpoint{Name: "TurnAArtifacts", Receiver: "ctx.Mutable", Line: 262},
			}},
		},
	}))
	ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{"internal/agent/extractor.go": true})
	ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
		"internal/agent/extractor.go": {{Start: 240, End: 270}},
	})

	target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"Extractor", "Mutable", "BusContext"})
	if !ok || target.file != "internal/agent/extractor.go" || !target.alreadyRead {
		t.Fatalf("canonical typed alias must retain the direct already-read operation: ok=%t target=%+v", ok, target)
	}
}

func TestFlowParticipantCoverageBindsCanonicalTypedAliasesToExistingOperation(t *testing.T) {
	operation := flowOperationEvidence(types.AnchorCall,
		"extractorEvaluator.BuildInitialInstruction", "ctx.Mutable.TurnAArtifacts", 262)
	operation.Source = "internal/agent/extractor.go"
	operation.OwnerIdentity = "extractorEvaluator.BuildInitialInstruction"
	operation.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "ctx.Mutable", Type: "*MutableState", Owner: "extractorEvaluator.BuildInitialInstruction",
	}}
	ctx := flowOperationCompletionContext([]types.EvidenceItem{operation})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"Extractor", "Mutable", "BusContext"}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "Extractor", ResolvedAs: "extractorEvaluator", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true},
		{Surface: "Mutable", ResolvedAs: "MutableState", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true},
		{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	missing := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence())
	if flowTestSliceContains(missing, "Extractor") || flowTestSliceContains(missing, "Mutable") || !flowTestSliceContains(missing, "BusContext") {
		t.Fatalf("existing typed Extractor->Mutable operation should bind canonical aliases while unrelated BusContext stays unproven: %v", missing)
	}
}

func TestFlowOperationNavigationFindsSingleTokenComponentThroughEnclosingCallable(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			ctx := flowOperationCompletionContext(nil)
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
				{Surface: "extractor", ResolvedAs: "extractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
			}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
				},
			}
			path := filepath.ToSlash(filepath.Join("src", language, "pipeline.src"))
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
				path: {
					RelPath: path, Language: language,
					Symbols: []repotypes.Symbol{{Name: "Build", Kind: "method", Receiver: "extractorEvaluator", Line: 10, EndLine: 30}},
					Relations: []repotypes.Relation{{
						Kind: "call", File: path, Line: 20,
						FromEP:     repotypes.RelationEndpoint{Line: 20},
						ToEP:       repotypes.RelationEndpoint{Name: "Load", Receiver: "snapshot", Line: 20},
						Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "parser_call",
					}},
				},
			}))

			target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"extractor"})
			if !ok || target.file != path || target.lineRange != (types.LineRange{Start: 8, End: 32}) {
				t.Fatalf("single-token participant should navigate through parser-owned enclosing callable: ok=%t target=%+v", ok, target)
			}
			if len(ctx.Mutable.EmittedEvidence()) != 0 {
				t.Fatal("enclosing-callable navigation must not manufacture relation evidence")
			}
		})
	}
}

func TestFlowOperationNavigationKeepsHigherQualityAlreadyReadJoinAheadOfUnreadLocalUse(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"internal/orchestrator/helper.go": strings.Join([]string{
			"package orchestrator",
			"func helper(busCtx *BusContext) {",
			"busCtx.Context()",
			"}",
		}, "\n"),
		"internal/agent/extractor.go": strings.Join([]string{
			"package agent",
			"func (e *extractorEvaluator) run(ctx *BusContext) {",
			"ctx.Mutable.TurnAArtifacts()",
			"}",
		}, "\n"),
	}
	for path, body := range files {
		absolute := filepath.Join(repo, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	ctx := flowOperationCompletionContext(nil)
	ctx.RepoRoot = repo
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "extractor", ResolvedAs: "extractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "Mutable", ResolvedAs: "Mutable", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		"internal/orchestrator/helper.go": {
			RelPath: "internal/orchestrator/helper.go", Language: repotypes.LangGo,
			Symbols: []repotypes.Symbol{{Name: "busCtx", Kind: "parameter", Parent: "helper", DeclaredType: "*BusContext", Line: 2}},
			Relations: []repotypes.Relation{{
				Kind: "call", File: "internal/orchestrator/helper.go", Line: 3,
				FromEP:     repotypes.RelationEndpoint{Name: "helper", Line: 3},
				ToEP:       repotypes.RelationEndpoint{Name: "Context", Receiver: "busCtx", Line: 3},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_call",
			}},
		},
		"internal/agent/extractor.go": {
			RelPath: "internal/agent/extractor.go", Language: repotypes.LangGo,
			Symbols: []repotypes.Symbol{{Name: "ctx", Kind: "parameter", Parent: "extractorEvaluator.run", DeclaredType: "*BusContext", Line: 2}},
			Relations: []repotypes.Relation{{
				Kind: "call", File: "internal/agent/extractor.go", Line: 3,
				FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "extractorEvaluator", Line: 3},
				ToEP:       repotypes.RelationEndpoint{Name: "TurnAArtifacts", Receiver: "ctx.Mutable", Line: 3},
				Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_call",
			}},
		},
	}))
	ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{"internal/agent/extractor.go": true})
	ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{
		"internal/agent/extractor.go": {{Start: 1, End: 15}},
	})

	target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"extractor", "Mutable", "BusContext"})
	if !ok || target.file != "internal/agent/extractor.go" || !target.alreadyRead {
		t.Fatalf("already-read direct join must outrank unread unrelated local use: ok=%t target=%+v", ok, target)
	}
	queueFlowOperationNavigationRead(ctx, []string{"extractor", "Mutable", "BusContext"}, "navigation only", "participant", types.DowngradeLaneFlowParticipantCoverage)
	if pending := ctx.Mutable.EvidenceClosure().PendingReads(); len(pending) != 0 {
		t.Fatalf("already-read operation must not queue a duplicate source read: %+v", pending)
	}
	hint := flowOperationNavigationHintForMissing(ctx, []string{"extractor", "Mutable", "BusContext"}, nil, nil)
	for _, want := range []string{"already present in the read closure", "without another read_file/repo_map/grep call", "internal/agent/extractor.go"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("already-read extraction guidance missing %q:\n%s", want, hint)
		}
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Fatal("already-read navigation must not manufacture relation evidence")
	}
}

func TestFlowOperationNavigationDoesNotTreatQuotedCarrierAsArgumentBinding(t *testing.T) {
	repo := t.TempDir()
	const path = "src/pipeline.go"
	absolute := filepath.Join(repo, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(strings.Join([]string{
		"func run() {",
		"o.busContext.Reset()",
		`builder.Build("o.busContext", stage)`,
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := flowOperationCompletionContext(nil)
	ctx.RepoRoot = repo
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{{
			Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
		}},
	}
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		path: {
			RelPath: path, Language: repotypes.LangGo,
			Symbols: []repotypes.Symbol{{Name: "busContext", Parent: "Orchestrator", DeclaredType: "*BusContext", Line: 1}},
			Relations: []repotypes.Relation{
				{
					Kind: "call", File: path, Line: 2,
					FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Orchestrator", Line: 2},
					ToEP:       repotypes.RelationEndpoint{Name: "Reset", Receiver: "o.busContext", Line: 2},
					Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_call",
				},
				{
					Kind: "call", File: path, Line: 3,
					FromEP:     repotypes.RelationEndpoint{Name: "run", Receiver: "Orchestrator", Line: 3},
					ToEP:       repotypes.RelationEndpoint{Name: "Build", Receiver: "builder", Line: 3},
					Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_call",
				},
			},
		},
	}))
	target, ok := flowOperationRepairReadTargetForMissing(ctx, []string{"BusContext"})
	if !ok || target.lineRange != (types.LineRange{Start: 1, End: 14}) {
		t.Fatalf("quoted display text must not outrank the real local binding operation: ok=%t target=%+v", ok, target)
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

func TestFlowParticipantCoverageDoesNotPromoteDisconnectedLocalOperationsIntoRequestedRelation(t *testing.T) {
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
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		"internal/types/context.go": {
			RelPath: "internal/types/context.go",
			Symbols: []repotypes.Symbol{
				{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "*MutableState"},
				{Name: "Mutable", Kind: "field", Parent: "AgentContext", DeclaredType: "*MutableState"},
			},
		},
	}))

	resolved := flowResolveParticipantIdentity(ctx, ctx.AnalysisIR.RequestModel,
		ctx.AnalysisIR.RequestModel.DiagramHint.Participants[0])
	for _, want := range []string{"Mutable", "BusContext.Mutable", "*MutableState"} {
		if !flowTestSliceContains(resolved.surfaces, want) {
			t.Fatalf("unique requested-owner binding should late-resolve %q: %+v", want, resolved)
		}
	}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); !flowTestSliceContains(got, "Mutable") || !flowTestSliceContains(got, "BusContext") {
		t.Fatalf("disconnected local operations must not masquerade as the requested member/owner relation: %v", got)
	}
	if !flowMissingParticipantsHaveLocalOperations(ctx, ctx.Mutable.EmittedEvidence(), []string{"Mutable", "BusContext"}) {
		t.Fatal("separate citable operations on every missing participant should classify the deficit as relation-only")
	}

	// A separately grounded operation that really joins the two requested
	// identities closes the relation scope. The completion gate still does not
	// choose whether or how the model renders that operation.
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "BusContext.SetMutable", "Mutable.Load", 96),
	})
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("a typed cross-participant operation should close the requested relation scope: %v", got)
	}
}

func TestFlowParticipantRelationOnlyRepairUsesContextWithoutMintingAuthority(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "ToolA.Execute", "localA.Load", 40),
		flowOperationEvidence(types.AnchorCall, "ToolB.Execute", "localB.Store", 41),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"ToolA", "ToolB", "Finalizer"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "ToolA", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "ToolB", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Finalizer", Role: types.DiagramParticipantContextOnly},
		},
	}
	missing := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence())
	if !flowTestSliceContains(missing, "ToolA") || !flowTestSliceContains(missing, "ToolB") ||
		flowTestSliceContains(missing, "Finalizer") {
		t.Fatalf("only disconnected incident participants should remain missing: %v", missing)
	}
	if !flowMissingParticipantsHaveLocalOperations(ctx, ctx.Mutable.EmittedEvidence(), missing) {
		t.Fatal("typed local operations on both incident participants should select relation-focused navigation")
	}
	files, keywords := flowOperationRepairTargets(ctx, missing, ctx.Mutable.EmittedEvidence())
	if !flowTestSliceContains(keywords, "Finalizer") {
		t.Fatalf("typed context-only identity should guide the bounded bridge search: files=%v keywords=%v", files, keywords)
	}
	hint := flowParticipantCoverageNavigationHint(ctx, missing, ctx.Mutable.EmittedEvidence(), files, keywords)
	for _, want := range []string{"Relation-focused navigation", "do not collect another local member call/return", "direct or multi-hop component"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("relation-only navigation missing %q:\n%s", want, hint)
		}
	}
	if strings.Contains(hint, "Exact next navigation") {
		t.Fatalf("relation-only deficit must not send the model back to another isolated local occurrence:\n%s", hint)
	}

	queueFlowParticipantCoverageRepair(ctx, missing, ctx.Mutable.EmittedEvidence())
	for _, repair := range ctx.Mutable.EvidenceClosure().ActiveRepairs() {
		if repair.DowngradeLane != types.DowngradeLaneFlowParticipantCoverage {
			continue
		}
		if repair.Kind == types.RepairReadFile {
			t.Fatalf("relation-only repair must not queue another local direct read: %+v", repair)
		}
		if !flowTestSliceContains(repair.Keywords, "Finalizer") {
			t.Fatalf("durable soft repair should preserve typed context scope: %+v", repair)
		}
	}
}

func TestFlowParticipantRelationOnlyClassificationFailsClosedWithoutEveryLocalOperation(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "ToolA.Execute", "localA.Load", 40),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"ToolA", "ToolB"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "ToolA", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "ToolB", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	missing := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence())
	if flowMissingParticipantsHaveLocalOperations(ctx, ctx.Mutable.EmittedEvidence(), missing) {
		t.Fatalf("a participant without any typed operation must retain locate/read recovery room: %v", missing)
	}
}

func TestEmitInvestigationComplete_RelationOnlyParticipantDeficitConvergesAfterFocusedPass(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "ToolA.Execute", "localA.Load", 40),
		flowOperationEvidence(types.AnchorCall, "ToolB.Execute", "localB.Store", 41),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"ToolA", "ToolB", "Finalizer"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "ToolA", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "ToolB", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Finalizer", Role: types.DiagramParticipantContextOnly},
		},
	}
	tool := &EmitInvestigationComplete{}
	first, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if ctx.Mutable.IsInvestigationComplete() || first.Repair == nil ||
		!strings.Contains(first.Repair.Hint, "Relation-focused navigation") {
		t.Fatalf("first relation-only attempt should receive exactly one bridge-focused pass: %+v", first)
	}
	second, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() ||
		!ctx.Mutable.EvidenceClosure().HasCompletionCaveat(types.DowngradeLaneFlowParticipantCoverage) ||
		!strings.Contains(second.Summary, "participant relation remains unproven") {
		t.Fatalf("second unchanged relation-only attempt should converge honestly: %+v", second)
	}
}

func TestFlowParticipantStrictProviderSubsetWithLocalBoundaryUsesOneFocusedPass(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "BuildAgentContext", "bus.Mutable.Objective", 41),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{
		"Analyzer", "Explorer", "Extractor", "Finalizer", "BusContext", "Mutable",
	}
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "Mutable", Resolution: types.EntityResolutionPrescanAnchor, Resolved: true, UseForSearch: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantContextOnly},
			{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		"internal/types/context.go": {
			RelPath: "internal/types/context.go",
			Symbols: []repotypes.Symbol{{
				Name: "Mutable", Kind: "field", Parent: "BusContext",
				DeclaredType: "*MutableState", Line: 20,
			}},
		},
	}))
	rows := []stageauthority.StageRow{
		{StageIdent: "StageAnalyze", StageValue: "analyze", AgentIdent: "AgentAnalyzer", AgentValue: "analyzer"},
		{StageIdent: "StageExplore", StageValue: "explore", AgentIdent: "AgentExplorer", AgentValue: "explorer"},
		{StageIdent: "StageExtract", StageValue: "extract", AgentIdent: "AgentExtractor", AgentValue: "extractor"},
		{StageIdent: "StageFinalize", StageValue: "finalize", AgentIdent: "AgentFinalizer", AgentValue: "finalizer"},
	}
	precedence := []stageauthority.PrecedenceRelation{
		{From: rows[0], To: rows[1]}, {From: rows[1], To: rows[2]}, {From: rows[2], To: rows[3]},
	}

	assessment := assessFlowParticipantCoverage(ctx, ctx.Mutable.EmittedEvidence(), precedence...)
	if len(assessment.missing) != 1 || assessment.missing[0] != "Mutable" {
		t.Fatalf("strict stage provider should leave the local carrier outside request scope: %+v", assessment)
	}
	if !assessment.boundedPartialUnprovenAvailable {
		t.Fatalf("an exact publishable local-only candidate must make the partial boundary convergent: %+v", assessment)
	}
	if got := flowParticipantCoverageConvergenceThreshold(assessment, true, true); got != 2 {
		t.Fatalf("an advancing navigation cursor must not require a third close after one focused pass, threshold=%d", got)
	}
	ordinary := assessment
	ordinary.boundedPartialUnprovenAvailable = false
	if got := flowParticipantCoverageConvergenceThreshold(ordinary, true, true); got != 0 {
		t.Fatalf("a generic relation deficit with a live exact frontier must retain the full locate/read allowance, threshold=%d", got)
	}
	if assessment.missing[0] == "" || len(ctx.Mutable.EmittedEvidence()) != 1 {
		t.Fatal("classification must preserve the missing boundary and never manufacture relation evidence")
	}
}

func TestFlowParticipantCoverageAcceptsTypedMultiHopRelationComponent(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "ToolA.Execute", "shared.Dispatch", 40),
		flowOperationEvidence(types.AnchorCall, "shared.Dispatch", "ToolB.Execute", 41),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"ToolA", "ToolB"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramArchitecture, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "ToolA", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "ToolB", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("a typed multi-hop component connecting the requested participants should close coverage: %v", got)
	}
}

func TestFlowParticipantCompletionKeepsDisconnectedTypedIslandsOpen(t *testing.T) {
	ctx := flowOperationCompletionContext([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "ToolA.Execute", "ToolB.Accept", 40),
		flowOperationEvidence(types.AnchorCall, "ToolC.Execute", "ToolD.Accept", 41),
	})
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"ToolA", "ToolB", "ToolC", "ToolD"}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "ToolA", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "ToolB", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "ToolC", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "ToolD", Role: types.DiagramParticipantIncidentRequired},
		},
	}

	missing := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence())
	if flowTestSliceContains(missing, "ToolA") || flowTestSliceContains(missing, "ToolB") ||
		!flowTestSliceContains(missing, "ToolC") || !flowTestSliceContains(missing, "ToolD") {
		t.Fatalf("completion should retain the earliest equal-sized typed island and request a bridge for the other: %v", missing)
	}

	// One model-authored typed bridge joins the two already valid islands. The
	// completion gate observes it but never synthesizes it.
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{
		flowOperationEvidence(types.AnchorCall, "ToolB.Accept", "ToolC.Execute", 42),
	})
	if got := flowParticipantCoverageMissing(ctx, ctx.Mutable.EmittedEvidence()); len(got) != 0 {
		t.Fatalf("one typed bridge should close connected participant coverage: %v", got)
	}
}

func TestFlowSelectedCallResultRequiresExactWholeValueConsumer(t *testing.T) {
	repo := t.TempDir()
	const source = "src/pipeline.go"
	body := strings.Join([]string{
		"package pipeline",
		"func (p *Pipeline) run() {",
		"ctx := builder.Build(p.bus, types.AgentExtractor)",
		"return agent.Execute(ctx)",
		"}",
	}, "\n")
	absolute := filepath.Join(repo, filepath.FromSlash(source))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	assignment := flowOperationEvidence(types.AnchorAssignment, "ctx", "builder.Build", 3)
	assignment.Source = source
	assignment.Snippet = "ctx := builder.Build(p.bus, types.AgentExtractor)"
	assignment.AnchorSymbol = "ctx"
	busArgument := flowOperationEvidence(types.AnchorArgument, "p.bus", "builder.Build", 3)
	busArgument.Source = source
	busArgument.Snippet = assignment.Snippet
	busArgument.AnchorSymbol = "p.bus"
	busArgument.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{
		Binding: "p.bus", Type: "BusContext",
	}}
	agentArgument := flowOperationEvidence(types.AnchorArgument, "types.AgentExtractor", "builder.Build", 3)
	agentArgument.Source = source
	agentArgument.Snippet = assignment.Snippet
	agentArgument.AnchorSymbol = "types.AgentExtractor"
	evidence := []types.EvidenceItem{assignment, busArgument, agentArgument}
	ctx := flowOperationCompletionContext(evidence)
	ctx.RepoRoot = repo
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{
		{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
		{Surface: "Extractor", ResolvedAs: "types.AgentExtractor", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true},
	}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
		Kind: types.DiagramFlow, Required: true,
		Participants: []types.DiagramParticipantHint{
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		},
	}
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		source: {
			RelPath: source, Language: repotypes.LangGo, Package: "pipeline",
			Symbols: []repotypes.Symbol{
				{Name: "bus", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 1},
				{Name: "AgentExtractor", Kind: "const", DeclaredType: "AgentName", Line: 1},
				{Name: "run", Kind: "method", Receiver: "Pipeline", Line: 2, EndLine: 5},
			},
			Relations: []repotypes.Relation{
				{Kind: "call", File: source, Line: 3,
					FromEP: repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 3},
					ToEP:   repotypes.RelationEndpoint{Name: "Build", Receiver: "builder", Line: 3}},
				{Kind: "call", File: source, Line: 4,
					FromEP: repotypes.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 4},
					ToEP:   repotypes.RelationEndpoint{Name: "Execute", Receiver: "agent", Line: 4}},
			},
		},
	}))
	ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{source: true})
	ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{source: {{Start: 1, End: 5}}})

	repair, missing := flowOperationMissingSelectedResultConsumer(ctx, evidence)
	if !missing || repair.argument != "ctx" || repair.receiver != "agent.Execute" ||
		repair.consumerLine != 4 || !repair.target.alreadyRead {
		t.Fatalf("exact whole-value consumer should be the bounded missing handoff: missing=%t repair=%+v", missing, repair)
	}

	res, err := (&EmitInvestigationComplete{}).Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || res.Repair == nil || res.Repair.Code != "flow_value_consumer_evidence" ||
		res.Repair.Metadata["lane"] != string(types.DowngradeLaneFlowValueConsumerCoverage) ||
		!strings.Contains(res.Repair.Hint, `subject="ctx"`) ||
		!strings.Contains(res.Repair.Hint, `object="agent.Execute"`) {
		t.Fatalf("completion must expose the exact consumer repair without minting it: %+v", res)
	}
	if ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("missing exact consumer handoff must not silently mark investigation complete")
	}

	consumer := flowOperationEvidence(types.AnchorArgument, "ctx", "agent.Execute", 4)
	consumer.Source = source
	consumer.Snippet = "return agent.Execute(ctx)"
	consumer.AnchorSymbol = "ctx"
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{consumer})
	if got, stillMissing := flowOperationMissingSelectedResultConsumer(ctx, ctx.Mutable.EmittedEvidence()); stillMissing {
		t.Fatalf("model-authored exact consumer must close the value continuation, got %+v", got)
	}
}

func TestFlowSelectedCallResultConsumerContinuesAcrossAuthoredAssignmentsForAllLanguages(t *testing.T) {
	for _, language := range repotypes.SupportedReadLanguages() {
		t.Run(string(language), func(t *testing.T) {
			repo := t.TempDir()
			const source = "src/pipeline.source"
			body := strings.Join([]string{
				"run pipeline",
				"func run() {",
				"ctx = builder.Build(bus)",
				"output = agent.Execute(ctx)",
				"pipeline.Apply(output)",
				"}",
			}, "\n")
			absolute := filepath.Join(repo, filepath.FromSlash(source))
			if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}

			ctxAssignment := flowOperationEvidence(types.AnchorAssignment, "ctx", "builder.Build", 3)
			ctxAssignment.Source, ctxAssignment.Snippet, ctxAssignment.AnchorSymbol = source, "ctx = builder.Build(bus)", "ctx"
			busArgument := flowOperationEvidence(types.AnchorArgument, "bus", "builder.Build", 3)
			busArgument.Source, busArgument.Snippet, busArgument.AnchorSymbol = source, ctxAssignment.Snippet, "bus"
			busArgument.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{Binding: "bus", Type: "BusContext"}}
			executeArgument := flowOperationEvidence(types.AnchorArgument, "ctx", "agent.Execute", 4)
			executeArgument.Source, executeArgument.Snippet, executeArgument.AnchorSymbol = source, "output = agent.Execute(ctx)", "ctx"
			outputAssignment := flowOperationEvidence(types.AnchorAssignment, "output", "agent.Execute", 4)
			outputAssignment.Source, outputAssignment.Snippet, outputAssignment.AnchorSymbol = source, executeArgument.Snippet, "output"
			evidence := []types.EvidenceItem{ctxAssignment, busArgument, executeArgument, outputAssignment}

			ctx := flowOperationCompletionContext(evidence)
			ctx.RepoRoot = repo
			ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{{
				Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol,
				Resolved: true, UseForSearch: true, UseForShape: true,
			}}
			ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{
				Kind: types.DiagramFlow, Required: true,
				Participants: []types.DiagramParticipantHint{{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired}},
			}
			ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{source: {
				RelPath: source, Language: language,
				Symbols: []repotypes.Symbol{{Name: "bus", Kind: "field", DeclaredType: "BusContext", Line: 1}, {Name: "run", Kind: "function", Line: 2, EndLine: 6}},
				Relations: []repotypes.Relation{
					{Kind: "call", File: source, Line: 3, FromEP: repotypes.RelationEndpoint{Name: "run", Line: 3}, ToEP: repotypes.RelationEndpoint{Name: "Build", Receiver: "builder", Line: 3}},
					{Kind: "call", File: source, Line: 4, FromEP: repotypes.RelationEndpoint{Name: "run", Line: 4}, ToEP: repotypes.RelationEndpoint{Name: "Execute", Receiver: "agent", Line: 4}},
					{Kind: "call", File: source, Line: 5, FromEP: repotypes.RelationEndpoint{Name: "run", Line: 5}, ToEP: repotypes.RelationEndpoint{Name: "Apply", Receiver: "pipeline", Line: 5}},
				},
			}}))
			ctx.Mutable.EvidenceClosure().SetReadSet(map[string]bool{source: true})
			ctx.Mutable.EvidenceClosure().AddReadRanges(map[string][]types.LineRange{source: {{Start: 1, End: 6}}})

			repair, missing := flowOperationMissingSelectedResultConsumer(ctx, evidence)
			if !missing || repair.argument != "output" || repair.receiver != "pipeline.Apply" ||
				repair.producerLine != 4 || repair.consumerLine != 5 {
				t.Fatalf("selected chain must advance past covered ctx consumer: missing=%t repair=%+v", missing, repair)
			}
			applyArgument := flowOperationEvidence(types.AnchorArgument, "output", "pipeline.Apply", 5)
			applyArgument.Source, applyArgument.Snippet, applyArgument.AnchorSymbol = source, "pipeline.Apply(output)", "output"
			if got, stillMissing := flowOperationMissingSelectedResultConsumer(ctx, append(evidence, applyArgument)); stillMissing {
				t.Fatalf("model-authored final whole-value consumer must close selected chain: %+v", got)
			}
		})
	}
}

func TestFlowSelectedCallResultConsumerFailsClosedOnSameLineAmbiguity(t *testing.T) {
	repo := t.TempDir()
	const source = "src/pipeline.go"
	body := "package pipeline\nfunc run() {\nctx := builder.Build(bus)\nleft.Use(ctx); right.Use(ctx)\n}\n"
	absolute := filepath.Join(repo, filepath.FromSlash(source))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	assignment := flowOperationEvidence(types.AnchorAssignment, "ctx", "builder.Build", 3)
	assignment.Source, assignment.Snippet, assignment.AnchorSymbol = source, "ctx := builder.Build(bus)", "ctx"
	argument := flowOperationEvidence(types.AnchorArgument, "bus", "builder.Build", 3)
	argument.Source, argument.Snippet, argument.AnchorSymbol = source, assignment.Snippet, "bus"
	argument.DeclaredIdentityBindings = []types.EvidenceDeclaredIdentityBinding{{Binding: "bus", Type: "BusContext", Owner: "run"}}
	ctx := flowOperationCompletionContext([]types.EvidenceItem{assignment, argument})
	ctx.RepoRoot = repo
	ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance = []types.EntityProvenance{{Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true}}
	ctx.AnalysisIR.RequestModel.DiagramHint = &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired}}}
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{source: {
		RelPath: source, Language: repotypes.LangGo, Package: "pipeline",
		Symbols: []repotypes.Symbol{{Name: "bus", Kind: "field", DeclaredType: "BusContext", Line: 1}, {Name: "run", Kind: "function", Line: 2, EndLine: 5}},
		Relations: []repotypes.Relation{
			{Kind: "call", File: source, Line: 3, FromEP: repotypes.RelationEndpoint{Name: "run", Line: 3}, ToEP: repotypes.RelationEndpoint{Name: "Build", Receiver: "builder", Line: 3}},
			{Kind: "call", File: source, Line: 4, FromEP: repotypes.RelationEndpoint{Name: "run", Line: 4}, ToEP: repotypes.RelationEndpoint{Name: "Use", Receiver: "left", Line: 4}},
			{Kind: "call", File: source, Line: 4, FromEP: repotypes.RelationEndpoint{Name: "run", Line: 4}, ToEP: repotypes.RelationEndpoint{Name: "Use", Receiver: "right", Line: 4}},
		},
	}}))
	if got, missing := flowOperationMissingSelectedResultConsumer(ctx, ctx.Mutable.EmittedEvidence()); missing {
		t.Fatalf("ambiguous same-line consumers must fail closed, got %+v", got)
	}
}

func TestFlowParticipantCoverageJoinsUniqueQualifiedAndBareCallableEndpoint(t *testing.T) {
	evidence := []types.EvidenceItem{
		flowOperationEvidence(types.AnchorArgument, "o.busCtx", "ctxbuilder.BuildAgentContext", 15),
		flowOperationEvidence(types.AnchorCall, "BuildAgentContext", "bus.Mutable.Objective", 29),
	}
	participants := []types.DiagramParticipantHint{
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	scope := buildFlowParticipantRelationScope(
		types.RequestModel{},
		participants,
		[][]string{{"o.busCtx"}, {"bus.Mutable.Objective"}},
		evidence,
		nil,
	)
	for i, participant := range participants {
		if i >= len(scope.participantCovered) || !scope.participantCovered[i] {
			t.Fatalf("unique qualified/bare callable spelling should join %s into one typed component: %+v", participant.Identity, scope)
		}
	}
}

func TestFlowParticipantCoverageDoesNotJoinAmbiguousCallableTail(t *testing.T) {
	evidence := []types.EvidenceItem{
		flowOperationEvidence(types.AnchorArgument, "o.busCtx", "ctxbuilder.BuildAgentContext", 15),
		flowOperationEvidence(types.AnchorCall, "BuildAgentContext", "bus.Mutable.Objective", 29),
		flowOperationEvidence(types.AnchorCall, "otherpkg.BuildAgentContext", "Other.Execute", 40),
	}
	participants := []types.DiagramParticipantHint{
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	scope := buildFlowParticipantRelationScope(
		types.RequestModel{},
		participants,
		[][]string{{"o.busCtx"}, {"bus.Mutable.Objective"}},
		evidence,
		nil,
	)
	for i, participant := range participants {
		if i < len(scope.participantCovered) && scope.participantCovered[i] {
			t.Fatalf("ambiguous qualified callable tail must not join %s across packages: %+v", participant.Identity, scope)
		}
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
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		"internal/types/context.go": {
			RelPath: "internal/types/context.go",
			Symbols: []repotypes.Symbol{
				{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "*MutableState"},
				{Name: "Mutable", Kind: "field", Parent: "AgentContext", DeclaredType: "*MutableState"},
			},
		},
	}))

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
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		"src/a.go": {RelPath: "src/a.go", Symbols: []repotypes.Symbol{{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "*StateA"}}},
		"src/b.go": {RelPath: "src/b.go", Symbols: []repotypes.Symbol{{Name: "Mutable", Kind: "field", Parent: "BusContext", DeclaredType: "*StateB"}}},
	}))

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

func TestRequiredRelationItemValidationConvergesWithoutMintingEvidence(t *testing.T) {
	seed := flowOperationEvidence(types.AnchorCall, "Pipeline.run", "Worker.step", 12)
	ctx := flowOperationCompletionContext([]types.EvidenceItem{seed})
	repair := buildEmitEvidenceArgumentFlowRepair([]emitEvidenceArgumentFlowRepair{{
		itemIndex: 1, argument: "types.AgentExtractor", receiver: "ctxbuilder.BuildAgentContext",
		source: "internal/orchestrator/extract_work.go", line: 15,
	}})
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "emit_evidence", Success: true, Repair: repair,
	})
	tool := &EmitInvestigationComplete{}
	for attempt := 1; attempt <= 2; attempt++ {
		res, err := tool.Execute(ctx, flowOperationCompletionParams(t))
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceItemValidation ||
			ctx.Mutable.IsInvestigationComplete() {
			t.Fatalf("attempt %d must keep the exact model-authored repair actionable: %+v", attempt, res)
		}
	}

	res, err := tool.Execute(ctx, flowOperationCompletionParams(t))
	if err != nil {
		t.Fatalf("converged attempt: %v", err)
	}
	if !ctx.Mutable.IsInvestigationComplete() || !strings.Contains(res.Summary, "explicitly unproven") {
		t.Fatalf("third identical blocker must close with an unproven boundary: %+v", res)
	}
	gotEvidence := ctx.Mutable.EmittedEvidence()
	if len(gotEvidence) != 1 || gotEvidence[0].Subject != seed.Subject || gotEvidence[0].Object != seed.Object {
		t.Fatalf("convergence must not mint or rewrite relation evidence: %+v", gotEvidence)
	}
	key, ok := requiredRelationItemValidationBlockerKey(repair)
	if !ok || !ctx.Mutable.EvidenceClosure().HasCompletionCaveatForBlocker(
		types.DowngradeLaneRequiredRelationItemValidation, key) {
		t.Fatalf("exact typed caveat missing for blocker=%d: %+v", key, ctx.Mutable.EvidenceClosure().CompletionCaveats())
	}
}

func TestRequiredRelationItemValidationConvergenceIsBlockerScopedAndUpgradeable(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	repairA := buildEmitEvidenceArgumentFlowRepair([]emitEvidenceArgumentFlowRepair{{
		itemIndex: 0, argument: "left", receiver: "Pipeline.Build", source: "src/pipeline.go", line: 20,
	}})
	repairB := buildEmitEvidenceArgumentFlowRepair([]emitEvidenceArgumentFlowRepair{{
		itemIndex: 1, argument: "right", receiver: "Pipeline.Run", source: "src/pipeline.go", line: 30,
	}})
	keyA, okA := requiredRelationItemValidationBlockerKey(repairA)
	keyB, okB := requiredRelationItemValidationBlockerKey(repairB)
	if !okA || !okB || keyA == keyB {
		t.Fatalf("independent typed repairs need distinct stable keys: A=%d/%t B=%d/%t", keyA, okA, keyB, okB)
	}
	if requiredRelationItemValidationConverges(ctx, keyA) ||
		requiredRelationItemValidationConverges(ctx, keyA) ||
		!requiredRelationItemValidationConverges(ctx, keyA) {
		t.Fatal("first exact blocker must converge only on its third identical attempt")
	}
	if requiredRelationItemValidationConverges(ctx, keyB) {
		t.Fatal("a different exact blocker must retain its own repair budget")
	}

	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "emit_evidence", Success: true, Repair: repairA})
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorArgument,
		Subject: "left", Object: "Pipeline.Build", AnchorSymbol: "left", Predicate: "passes argument",
		Source: "src/pipeline.go", LineStart: 20, Scope: types.ScopeLine,
		GroundingStatus: types.GroundingGrounded,
	}})
	reconcileSatisfiedRequiredRelationItemValidationCaveats(ctx, ctx.Mutable.EmittedEvidence())
	if ctx.Mutable.EvidenceClosure().HasCompletionCaveatForBlocker(
		types.DowngradeLaneRequiredRelationItemValidation, keyA) {
		t.Fatal("later exact model-authored evidence must upgrade only its scoped boundary")
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

func TestFlowNavigationIndexIsReusedAndRebuiltWithSearchGraph(t *testing.T) {
	ctx := flowOperationCompletionContext(nil)
	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		"src/first.go": {
			RelPath: "src/first.go",
			Symbols: []repotypes.Symbol{{Name: "Carrier", DeclaredType: "*FirstType"}},
		},
	}))
	first := flowNavigationIndexForContext(ctx)
	if first == nil || flowNavigationIndexForContext(ctx) != first {
		t.Fatal("one immutable search graph must reuse its derived navigation index")
	}
	if got := flowNavigationSymbols(first, []string{"FirstType"}); len(got) != 1 || got[0].file != "src/first.go" {
		t.Fatalf("first derived symbol lookup mismatch: %+v", got)
	}

	ctx.Mutable.SetSearchGraph(flowTestIndexedGraph(map[string]*repotypes.FileInfo{
		"src/second.go": {
			RelPath: "src/second.go",
			Symbols: []repotypes.Symbol{{Name: "Carrier", DeclaredType: "*SecondType"}},
		},
	}))
	second := flowNavigationIndexForContext(ctx)
	if second == nil || second == first {
		t.Fatal("replacement search graph must rebuild its derived navigation index")
	}
	if got := flowNavigationSymbols(second, []string{"FirstType"}); len(got) != 0 {
		t.Fatalf("stale first-graph symbol leaked into replacement index: %+v", got)
	}
	if got := flowNavigationSymbols(second, []string{"SecondType"}); len(got) != 1 || got[0].file != "src/second.go" {
		t.Fatalf("replacement derived symbol lookup mismatch: %+v", got)
	}
}
