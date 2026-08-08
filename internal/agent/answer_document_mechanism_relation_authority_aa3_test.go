package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMechanismRelationAuthorityDoesNotTurnIndependentFactsIntoPathAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				ID:              "definition",
				Kind:            types.EvidenceDirect,
				Source:          "internal/tracequery/types.go",
				LineStart:       50,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "EventTraceMark",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				ID:              "callsite-without-edge",
				Kind:            types.EvidenceMechanism,
				Source:          "internal/tracequery/query.go",
				LineStart:       22155,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "classifyFramePhase",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"accepted_grounded_source_facts=2",
		"grounded_callsite_facts=1",
		"explicit_typed_directed_relations=0",
		"ordered_path_authority=`unproven`",
		"Several true nodes do not by themselves prove call order",
		"must not claim an ordered/complete current-source chain",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mechanism relation authority missing %q:\n%s", want, got)
		}
	}
}

func TestMechanismRelationAuthorityTypedFlowDoesNotOfferArbitraryWholeDiagram(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "helper-call", Kind: types.EvidenceRelationship,
			Subject: "NewFinalizerAgent", Predicate: "calls", Object: "NewBaseAgent",
			Source: "internal/agent/finalizer.go", LineStart: 30,
			AnchorKind: types.AnchorCall, AnchorSymbol: "NewBaseAgent",
			Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
		}},
	}
	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "edge_recipe[1]") {
		t.Fatalf("typed helper relation must remain available as a factual recipe:\n%s", got)
	}
	if strings.Contains(got, "Copy-ready optional typed diagram") || strings.Contains(got, "edge_anchors_json=") {
		t.Fatalf("typed flow must not receive an arbitrary whole-diagram replacement capsule:\n%s", got)
	}
}

func TestMechanismRelationAuthorityPublishesOnlyTypedEdgesAndFlowPathsAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
			},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				PreferredKinds: []types.DiagramKind{types.DiagramFlow},
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				ID:              "edge",
				Producer:        types.EvidenceProducerExplorerEmitEvidence,
				Kind:            types.EvidenceRelationship,
				Source:          "internal/tracequery/query.go",
				LineStart:       100,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "parseTraceMark",
				Subject:         "convertTrace",
				Object:          "parseTraceMark",
				GroundingStatus: types.GroundingGrounded,
			},
		},
		FlowFindings: []types.FlowFindingDigest{
			{ID: "supported", Path: []string{"readEvent", "parseTraceMark", "classifySpan"}, EvidenceIDs: []string{"edge"}},
			{ID: "unsupported", Path: []string{"A", "B"}, UnsupportedReason: "missing edge"},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"explicit_typed_directed_relations=1",
		"ordered_path_authority=`typed_flow_paths_present`",
		"verified_relation_component_count=1",
		"inter_component_bridge_status=`not_applicable_single_component`",
		"copy both unchanged; do not compose a different story graph",
		"#### Copy-ready optional typed diagram",
		"sequenceDiagram",
		"participant n1 as convertTrace",
		"participant n2 as parseTraceMark",
		"n1->>n2: call",
		"edge_anchors_json=`[{\"from_node\":\"n1\",\"to_node\":\"n2\",\"relation_kind\":\"call\"}]`",
		"typed_flow_path[1]=`readEvent -> parseTraceMark -> classifySpan`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mechanism relation authority missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "`A -> B`") {
		t.Fatalf("unsupported flow finding must not receive path authority:\n%s", got)
	}
}

func TestMechanismRelationAuthorityDoesNotCrownAuxiliaryFlowForProductionScopeAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis:      types.AxisFlow,
			AnalyzerHints:      types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			SourceScopeProfile: &types.SourceScopeProfile{RequestedScope: types.SourceScopeProduction},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "pipeline-definition", Kind: types.EvidenceDirect,
			Source: "internal/orchestrator/topology.go", LineStart: 24,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "pipelineTopology",
			Subject: "pipelineTopology", GroundingStatus: types.GroundingGrounded,
		}},
		FlowFindings: []types.FlowFindingDigest{{
			ID: "unrelated-test-helper",
			Path: []string{
				"internal/agent/extractor_test.go:TestBuildPrompt",
				"internal/agent/write_analyzer.go:writeAnalyzerEvaluator.BuildInitialInstruction",
			},
		}},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "ordered_path_authority=`unproven`") {
		t.Fatalf("auxiliary flow must not authorize a production mechanism path:\n%s", got)
	}
	if strings.Contains(got, "typed_flow_path[") || strings.Contains(got, "TestBuildPrompt") {
		t.Fatalf("auxiliary flow leaked into principal mechanism authority:\n%s", got)
	}
}

func TestOrderedFlowAuthorityRequiresBothSupportedEndpointsOrEvidenceIDAA3(t *testing.T) {
	scope := supportLaneScopeFromPlan(&types.AnswerSupportPlan{
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLaneCurrentCodePath,
			Entries: []types.AnswerSupportEntry{
				{EvidenceID: "producer-edge", Source: "src/producer.go", LineStart: 10, Subject: "Produce", AnchorSymbol: "Produce"},
				{EvidenceID: "consumer-edge", Source: "src/consumer.go", LineStart: 20, Subject: "Consume", AnchorSymbol: "Consume"},
			},
		}},
	}, true, extractorValueRankDiagnostic)
	findings := []types.FlowFindingDigest{
		{ID: "one-loose-file-hit", Path: []string{"src/producer.go:Produce", "UnrelatedHelper"}},
		{ID: "both-endpoints", Path: []string{"src/producer.go:Produce", "src/consumer.go:Consume"}},
		{ID: "evidence-replay", Path: []string{"OpaqueA", "OpaqueB"}, EvidenceIDs: []string{"producer-edge"}},
	}

	got := answerDocSupportedMechanismFlowPathsForScope(findings, scope, types.RequestModel{})
	if len(got) != 2 {
		t.Fatalf("ordered authority should keep exact endpoint/evidence linkage only, got %+v", got)
	}
	if strings.Join(got[0], " -> ") != "src/producer.go:Produce -> src/consumer.go:Consume" {
		t.Fatalf("loose single-file overlap received ordered authority: %+v", got)
	}
	if strings.Join(got[1], " -> ") != "OpaqueA -> OpaqueB" {
		t.Fatalf("support EvidenceID replay was lost: %+v", got)
	}
}

func TestMechanismRelationAuthorityWithoutSupportPlanRejectsUnrelatedRepoFlowAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "dispatch-edge", Kind: types.EvidenceRelationship,
			Producer: types.EvidenceProducerExplorerEmitEvidence,
			Subject:  "Orchestrator.runAnalyzePhase", Predicate: "calls", Object: "Orchestrator.dispatchStage",
			Source: "internal/orchestrator/orchestrator.go", LineStart: 2485,
			AnchorKind: types.AnchorCall, AnchorSymbol: "dispatchStage",
			Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
		}},
		FlowFindings: []types.FlowFindingDigest{{
			ID: "repo-wide-unrelated",
			Path: []string{
				"internal/hitraceconv/input_authority.go:conversionInputStage.String",
				"internal/repl/turn_policy_test.go:TestTurnPolicyDispatch_DataRouteBypassesSourcePipeline",
			},
		}},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "ordered_path_authority=`listed_edges_only`") {
		t.Fatalf("unrelated repo flow must not receive current-question ordered authority:\n%s", got)
	}
	if strings.Contains(got, "typed_flow_path[") || strings.Contains(got, "conversionInputStage.String") {
		t.Fatalf("unrelated repo-wide path leaked into principal authority:\n%s", got)
	}
}

func TestMechanismRelationAuthorityWithoutSupportPlanAcceptsOperationEvidenceReplayAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "dispatch-edge", Kind: types.EvidenceRelationship,
			Producer: types.EvidenceProducerExplorerEmitEvidence,
			Subject:  "Orchestrator.runAnalyzePhase", Predicate: "calls", Object: "Orchestrator.dispatchStage",
			Source: "internal/orchestrator/orchestrator.go", LineStart: 2485,
			AnchorKind: types.AnchorCall, AnchorSymbol: "dispatchStage",
			Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
		}},
		FlowFindings: []types.FlowFindingDigest{{
			ID:          "evidence-replay",
			Path:        []string{"OpaqueProducer", "OpaqueConsumer"},
			EvidenceIDs: []string{"dispatch-edge"},
		}},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "ordered_path_authority=`typed_flow_paths_present`") ||
		!strings.Contains(got, "typed_flow_path[1]=`OpaqueProducer -> OpaqueConsumer`") {
		t.Fatalf("exact operation EvidenceID replay must retain ordered authority:\n%s", got)
	}
}

func TestMechanismRelationAuthorityBroadSupportPlanCannotOutrankExplorerOperationScopeAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
			PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationTraceCurrentFlow},
				Confidence:                          0.95,
			},
		}},
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "selected-operation", Producer: types.EvidenceProducerExplorerEmitEvidence,
				Kind: types.EvidenceRelationship, Subject: "runAnalyzePhase", Predicate: "assigns", Object: "AnalysisIR",
				Source: "internal/orchestrator/orchestrator.go", LineStart: 2520,
				AnchorKind: types.AnchorAssignment, AnchorSymbol: "AnalysisIR",
				Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
			},
			{
				ID: "auto-unrelated", Producer: "dataflow.lowerer.go",
				Kind: types.EvidenceRelationship, Subject: "BaseAgent.executeTool", Predicate: "calls", Object: "validateWriteAnalyzerToolPolicy",
				Source: "internal/agent/agent.go", LineStart: 3249,
				AnchorKind: types.AnchorCall, AnchorSymbol: "validateWriteAnalyzerToolPolicy",
				Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
			},
		},
		FlowFindings: []types.FlowFindingDigest{
			{ID: "unrelated-auto-path", Path: []string{"internal/agent/agent.go:BaseAgent.executeTool", "internal/agent/agent.go:validateWriteAnalyzerToolPolicy"}, EvidenceIDs: []string{"auto-unrelated"}},
			{ID: "selected-path", Path: []string{"runAnalyzePhase", "AnalysisIR"}, EvidenceIDs: []string{"selected-operation"}},
		},
	}

	if plan := answerSupportPlan(ctx); plan == nil {
		t.Fatal("fixture must compile a support plan so the R221 bypass arm is exercised")
	}
	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "typed_flow_path[1]=`runAnalyzePhase -> AnalysisIR`") {
		t.Fatalf("selected operation replay must retain principal ordered authority:\n%s", got)
	}
	if strings.Contains(got, "typed_flow_path[2]") ||
		strings.Contains(got, "typed_flow_path[1]=`internal/agent/agent.go:BaseAgent.executeTool") {
		t.Fatalf("broad support plan promoted an automatic unrelated ordered path:\n%s", got)
	}
}

func TestMechanismRelationCopyReadyFlowPreservesDisconnectedTypedComponentsAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)}},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				PreferredKinds: []types.DiagramKind{types.DiagramFlow},
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "edge-a", Kind: types.EvidenceRelationship,
				Source: "src/a.cpp", LineStart: 10, AnchorKind: types.AnchorCall,
				Subject: "Factory<\"json\">", Object: "Parser.parse",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				ID: "edge-b", Kind: types.EvidenceRegistration,
				Source: "src/b.cpp", LineStart: 20, AnchorKind: types.AnchorDefinition,
				Subject: "Registry", Object: "Plugin",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		`n1["Factory&lt;&quot;json&quot;&gt;"]`,
		`n2["Parser.parse"]`,
		`n3["Registry"]`,
		`n4["Plugin"]`,
		"n1 -->|call| n2",
		"n3 -->|register| n4",
		"verified_relation_component_count=2",
		"inter_component_bridge_status=`unproven_between_components`",
		"verified_component[1]=`n1:Factory<\"json\"> | n2:Parser.parse`",
		"verified_component[2]=`n3:Registry | n4:Plugin`",
		"relation_segment_class=`invocation_segment`",
		"relation_segment_class=`binding_segment`",
		"cross_component_execution_order_status=`unproven`",
		"cross_component_value_handoff_status=`unproven`",
		"component indices are stable identifiers, not execution/phase order",
		"Present them as independently proved segments",
		"Do not place the components as consecutive numbered hops",
		"does NOT prove the program can never connect them",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("copy-ready flow missing %q:\n%s", want, got)
		}
	}
	for _, bridge := range []string{"n2 --> n3", "n2 -->|call| n3", "n2 -->|register| n3"} {
		if strings.Contains(got, bridge) {
			t.Fatalf("copy-ready flow invented a bridge %q:\n%s", bridge, got)
		}
	}
}

func TestCallChainSupportLanePublishesTypedEntryRolesWithoutInventingOrderAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "assign", Kind: types.EvidenceDirect,
				Source: "src/logger.cpp", LineStart: 27, AnchorKind: types.AnchorAssignment,
				Subject: "Logger", Object: "sink_", GroundingStatus: types.GroundingGrounded,
			},
			{
				ID: "guard", Kind: types.EvidenceConditional,
				Source: "src/logger.cpp", LineStart: 30, AnchorKind: types.AnchorCondition,
				Subject: "Logger.log", Condition: "level < min_level_", GroundingStatus: types.GroundingGrounded,
			},
			{
				ID: "call", Kind: types.EvidenceRelationship,
				Source: "src/logger.cpp", LineStart: 36, AnchorKind: types.AnchorCall,
				Subject: "Logger.log", Object: "Sink.write", GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	got := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Only `directed_hop` and `typed_step_backbone` entries establish ordered principal hops",
		"Do not turn adjacent non-hop facts into implicit bridges",
		"[entry_role=`value_flow_fact`]",
		"[entry_role=`control_fact`]",
		"[entry_role=`directed_hop`]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed call-chain entry role missing %q:\n%s", want, got)
		}
	}
}

func TestMechanismRelationComponentBoundaryIsWiredIntoFinalizerPromptAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "edge-a", Kind: types.EvidenceRelationship,
				Source: "a.go", LineStart: 10, AnchorKind: types.AnchorCall,
				Subject: "A", Object: "B", GroundingStatus: types.GroundingGrounded,
			},
			{
				ID: "edge-b", Kind: types.EvidenceRelationship,
				Source: "c.go", LineStart: 20, AnchorKind: types.AnchorCall,
				Subject: "C", Object: "D", GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	got := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Current-Source Mechanism Relation Authority",
		"verified_relation_component_count=2",
		"inter_component_bridge_status=`unproven_between_components`",
		"Do not narrate them as one continuous end-to-end path",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("finalizer prompt wiring missing %q:\n%s", want, got)
		}
	}
}

func TestDisconnectedTypedComponentsDoNotPromoteSoftExplorerPathToRequiredAnswerAA3(t *testing.T) {
	mu := types.NewMutableState("explain the end-to-end call chain")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "proposed end-to-end path",
		Value:   "4",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Factory", "Parser", "Registry", "Plugin"},
		SupportRefs: []string{
			"Factory @ a.go:10",
			"Parser @ a.go:10",
			"Registry @ b.go:20",
			"Plugin @ b.go:20",
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind: string(types.ReqCallChain),
			},
		}},
		Mutable: mu,
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "edge-a", Kind: types.EvidenceRelationship,
				Source: "a.go", LineStart: 10, AnchorKind: types.AnchorCall,
				Subject: "Factory", Object: "Parser", GroundingStatus: types.GroundingGrounded,
			},
			{
				ID: "edge-b", Kind: types.EvidenceRegistration,
				Source: "b.go", LineStart: 20, AnchorKind: types.AnchorDefinition,
				Subject: "Registry", Object: "Plugin", GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"inter_component_bridge_status=`unproven_between_components`",
		"cross_component_execution_order_status=`unproven`",
		"## Advisory Model-Inferred Member Sets",
		"fact_authority=`advisory_model_inference`",
		"principal_contract=`not_authorized`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("soft disconnected-path boundary missing %q:\n%s", want, prompt)
		}
	}
	for _, banned := range []string{
		"## Required Principal Member Set",
		"Use this lane as the principal member slate",
		"Every member listed below MUST appear verbatim",
	} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("soft explorer path was promoted by %q:\n%s", banned, prompt)
		}
	}
}

func TestMechanismRelationCopyReadyDiagramFollowsSequenceContractAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				Required: true, RequiredKind: types.DiagramSequence,
			}},
		},
		EvidenceItems: []types.EvidenceItem{{
			ID: "edge", Kind: types.EvidenceRelationship,
			Source: "src/call.go", LineStart: 10, AnchorKind: types.AnchorCall,
			Subject: "Caller", Object: "Callee", GroundingStatus: types.GroundingGrounded,
		}},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"#### Copy-ready optional typed diagram",
		"sequenceDiagram",
		"participant n1 as Caller",
		"participant n2 as Callee",
		"n1->>n2: call",
		"edge_anchors_json=`[{\"from_node\":\"n1\",\"to_node\":\"n2\",\"relation_kind\":\"call\"}]`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sequence copy-ready diagram missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "flowchart TD") {
		t.Fatalf("sequence contract must not receive a competing flowchart example:\n%s", got)
	}
	if strings.Contains(got, "edge_recipe[1]") || strings.Contains(got, "node_alias[n1]") {
		t.Fatalf("copy-ready diagram intent must not receive duplicate per-edge JSON teaching:\n%s", got)
	}
}

func TestMechanismRelationCopyReadyDiagramSharesQualifiedCallableIdentityAA3(t *testing.T) {
	inbound := types.EvidenceItem{
		ID: "inbound", Kind: types.EvidenceRelationship,
		Source: "src/main.rs", LineStart: 20, AnchorKind: types.AnchorCall,
		Subject: "run", Object: "walker::collect_files", AnchorSymbol: "walker::collect_files",
		GroundingStatus: types.GroundingGrounded,
	}
	inner := types.EvidenceItem{
		ID: "inner", Kind: types.EvidenceRelationship,
		Source: "src/walker.rs", LineStart: 6, AnchorKind: types.AnchorCall,
		Subject: "collect_files", Object: "walk", AnchorSymbol: "walk", OwnerSymbol: "collect_files",
		GroundingStatus: types.GroundingGrounded,
	}
	definition := types.EvidenceItem{
		ID: "definition", Kind: types.EvidenceMechanism,
		Source: "src/walker.rs", LineStart: 4, AnchorKind: types.AnchorDefinition,
		AnchorSymbol: "collect_files", GroundingStatus: types.GroundingGrounded,
	}
	description := definition
	description.ID = "description"
	description.LineStart = 3
	description.Predicate = "documents"
	description.Producer = types.EvidenceProducerAutoPairRoleDescription
	description.DerivedFrom = []string{definition.ID}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				PreferredKinds: []types.DiagramKind{types.DiagramSequence},
			}},
		},
		EvidenceItems: []types.EvidenceItem{inbound, inner, definition, description},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"typed_relation_graph: unique_endpoint_relations=2; nodes=3; weak_components=1",
		"single_linear_relation_graph=true",
		"verified_relation_component_count=1",
		"participant n1 as run",
		"participant n2 as walker::collect_files",
		"participant n3 as walk",
		"n1->>n2: call",
		"n2->>n3: call",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("shared qualified callable identity missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "participant n3 as collect_files") ||
		strings.Contains(got, "inter_component_bridge_status=`unproven_between_components`") {
		t.Fatalf("copy-ready diagram split one typed callable into two nodes:\n%s", got)
	}
}

func TestMechanismRelationCopyReadyDeduplicatesVisualPairBeforeOptionalLimitAA3(t *testing.T) {
	evidence := []types.EvidenceItem{
		{
			ID: "first-callsite", Kind: types.EvidenceRelationship,
			Source: "src/call.go", LineStart: 10, AnchorKind: types.AnchorCall,
			Subject: "Root", Object: "Target0", GroundingStatus: types.GroundingGrounded,
		},
		{
			ID: "second-callsite-same-pair", Kind: types.EvidenceRelationship,
			Source: "src/call.go", LineStart: 11, AnchorKind: types.AnchorCall,
			Subject: "Root", Object: "Target0", GroundingStatus: types.GroundingGrounded,
		},
	}
	for i := 1; i <= 7; i++ {
		evidence = append(evidence, types.EvidenceItem{
			ID: fmt.Sprintf("edge-%d", i), Kind: types.EvidenceRelationship,
			Source: "src/call.go", LineStart: 20 + i, AnchorKind: types.AnchorCall,
			Subject: "Root", Object: fmt.Sprintf("Target%d", i), GroundingStatus: types.GroundingGrounded,
		})
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				PreferredKinds: []types.DiagramKind{types.DiagramSequence},
			}},
		},
		EvidenceItems: evidence,
	}

	if got := answerDocMechanismCopyReadyRelationLimit(ctx); got != answerDocMechanismOptionalRelationLimit {
		t.Fatalf("optional relation limit=%d, want %d", got, answerDocMechanismOptionalRelationLimit)
	}
	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"explicit_typed_directed_relations=9",
		"participant n9 as Target7",
		"source_relation_duplicates_collapsed_for_visual=1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("deduplicated optional visual missing %q:\n%s", want, got)
		}
	}
	if arrows := strings.Count(got, "->>"); arrows != 8 {
		t.Fatalf("copy-ready arrow count=%d, want 8 unique endpoint relations:\n%s", arrows, got)
	}
	if anchors := strings.Count(got, `"relation_kind":"call"`); anchors != 8 {
		t.Fatalf("copy-ready anchor count=%d, want 8 unique endpoint relations:\n%s", anchors, got)
	}
}

func TestMechanismRelationRequiredDiagramCarriesBroadTypedRelationSurfaceAA3(t *testing.T) {
	evidence := make([]types.EvidenceItem, 0, 21)
	for i := 1; i <= 21; i++ {
		evidence = append(evidence, types.EvidenceItem{
			ID: fmt.Sprintf("edge-%d", i), Kind: types.EvidenceRelationship,
			Source: "src/call.go", LineStart: 100 + i, AnchorKind: types.AnchorCall,
			Subject: "Root", Object: fmt.Sprintf("Target%02d", i), GroundingStatus: types.GroundingGrounded,
		})
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				Required: true, RequiredKind: types.DiagramSequence,
			}},
		},
		EvidenceItems: evidence,
	}

	if got := answerDocMechanismCopyReadyRelationLimit(ctx); got != answerDocMechanismRequiredRelationLimit {
		t.Fatalf("required relation limit=%d, want %d", got, answerDocMechanismRequiredRelationLimit)
	}
	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"explicit_typed_directed_relations=21",
		"participant n22 as Target21",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("required copy-ready diagram missing %q:\n%s", want, got)
		}
	}
	if arrows := strings.Count(got, "->>"); arrows != 21 {
		t.Fatalf("required copy-ready arrow count=%d, want 21:\n%s", arrows, got)
	}
	if strings.Contains(got, "additional unique typed relation") {
		t.Fatalf("21-edge required diagram must not inherit the optional eight-edge truncation:\n%s", got)
	}
}

func TestMechanismRelationAuthorityPublishesTypedFanOutTopologyAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		}},
		EvidenceItems: []types.EvidenceItem{
			{ID: "a", Kind: types.EvidenceRelationship, Source: "src/a.go", LineStart: 10, AnchorKind: types.AnchorCall, Subject: "Root", Object: "Left", GroundingStatus: types.GroundingGrounded},
			{ID: "b", Kind: types.EvidenceRelationship, Source: "src/a.go", LineStart: 11, AnchorKind: types.AnchorCall, Subject: "Root", Object: "Right", GroundingStatus: types.GroundingGrounded},
			{ID: "c", Kind: types.EvidenceRelationship, Source: "src/a.go", LineStart: 12, AnchorKind: types.AnchorCall, Subject: "Right", Object: "Leaf", GroundingStatus: types.GroundingGrounded},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"typed_relation_graph: unique_endpoint_relations=3",
		"nodes=4",
		"weak_components=1",
		"max_out_degree=2",
		"fan_out_present=true",
		"single_linear_relation_graph=false",
		"relation inventory is not one linear hop chain",
		"Do not turn its relation count into a hop count",
		"sibling fan-out/fan-in edges as consecutive intermediates",
		"topology fields describe only the shape of grounded directed relations",
		"None of them proves concurrent/parallel execution, temporal order, a join, or runtime convergence",
		"separate typed control-flow, concurrency, or runtime evidence",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed fan-out topology missing %q:\n%s", want, got)
		}
	}
}

func TestMechanismRelationGraphTopologyRecognizesOnlyExactLinearGraphAA3(t *testing.T) {
	linear, ok := answerDocMechanismRelationGraphTopology([]answerDocMechanismRelationEdge{
		{from: "A", to: "B", relation: types.DiagramRelCall},
		{from: "B", to: "C", relation: types.DiagramRelCall},
	})
	if !ok || !linear.singleLinearGraph || linear.weakComponents != 1 || linear.fanOutPresent || linear.fanInPresent {
		t.Fatalf("linear topology misclassified: %+v ok=%t", linear, ok)
	}
	disconnected, ok := answerDocMechanismRelationGraphTopology([]answerDocMechanismRelationEdge{
		{from: "A", to: "B", relation: types.DiagramRelCall},
		{from: "C", to: "D", relation: types.DiagramRelCall},
	})
	if !ok || disconnected.singleLinearGraph || !disconnected.disconnected || disconnected.weakComponents != 2 {
		t.Fatalf("disconnected topology misclassified: %+v ok=%t", disconnected, ok)
	}
}

func TestMechanismRelationCopyReadySequenceOmitsNonMessageAndAmbiguousRelationsAA3(t *testing.T) {
	grounded := func(id string, kind types.EvidenceKind, anchor types.AnchorKind, subject, object string, line int) types.EvidenceItem {
		return types.EvidenceItem{
			ID: id, Kind: kind, Source: "src/registry.cpp", LineStart: line,
			AnchorKind: anchor, Subject: subject, Object: object,
			GroundingStatus: types.GroundingGrounded,
		}
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				RequiredKind: types.DiagramSequence,
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			grounded("call", types.EvidenceRelationship, types.AnchorCall, "Logger.log", "Sink.write", 10),
			grounded("register", types.EvidenceRegistration, types.AnchorDefinition, "SinkRegistry::create", "ConsoleSink", 20),
			grounded("guard", types.EvidenceConditional, types.AnchorCondition, "SinkRegistry::create", "ConsoleSink", 21),
			grounded("return", types.EvidenceDirect, types.AnchorReturn, "SinkRegistry::create", "ConsoleSink", 22),
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"participant n1 as Logger.log",
		"participant n2 as Sink.write",
		"n1->>n2: call",
		`edge_anchors_json=` + "`" + `[{"from_node":"n1","to_node":"n2","relation_kind":"call"}]` + "`",
		"visual_omitted_relation_count=3",
		"omitted_relation_kinds=`guard,register,return`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sequence visual subset missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"participant n3 as SinkRegistry::create",
		"participant n4 as ConsoleSink",
		"n3->>n4: register",
		"n3->>n4: guard",
		"n3->>n4: return",
		`"relation_kind":"register"`,
		`"relation_kind":"guard"`,
		`"relation_kind":"return"`,
	} {
		if strings.Contains(got[strings.Index(got, "#### Copy-ready optional typed diagram"):], forbidden) {
			t.Fatalf("copy-ready sequence taught non-message relation %q:\n%s", forbidden, got)
		}
	}
}

func TestMechanismRelationAuthorityDoesNotSuggestDiagramWithoutTypedPresentationIntentAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "edge", Kind: types.EvidenceRelationship,
			Source: "src/call.go", LineStart: 10, AnchorKind: types.AnchorCall,
			Subject: "Caller", Object: "Callee", GroundingStatus: types.GroundingGrounded,
		}},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "edge_recipe[1]") {
		t.Fatalf("typed relation recipe must remain available:\n%s", got)
	}
	if strings.Contains(got, "Copy-ready optional typed diagram") || strings.Contains(got, "```mermaid") {
		t.Fatalf("a prose-only presentation must not be tempted into an unnecessary graph:\n%s", got)
	}
}

func TestMechanismRelationAuthorityPublishesSchemaNativeTypedRecipesAA3(t *testing.T) {
	grounded := func(id string, kind types.EvidenceKind, anchor types.AnchorKind, subject, object string, line int) types.EvidenceItem {
		return types.EvidenceItem{
			ID:              id,
			Kind:            kind,
			Source:          "src/mechanism.go",
			LineStart:       line,
			AnchorKind:      anchor,
			Subject:         subject,
			Object:          object,
			GroundingStatus: types.GroundingGrounded,
		}
	}
	typeRelation := grounded("type", types.EvidenceRelationship, types.AnchorDefinition, "Concrete", "Contract", 60)
	typeRelation.Producer = types.EvidenceProducerRepoMapStructuralRelation
	typeRelation.Predicate = "implements"
	uncitable := grounded("recovered", types.EvidenceRelationship, types.AnchorCall, "Recovered", "MustNotTeach", 70)
	uncitable.GroundingStatus = types.GroundingRecovered

	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		}},
		EvidenceItems: []types.EvidenceItem{
			grounded("call", types.EvidenceRelationship, types.AnchorCall, "Caller", "Callee", 10),
			grounded("callback", types.EvidenceRelationship, types.AnchorCallback, "Executor", "Handler", 20),
			grounded("registration", types.EvidenceRegistration, types.AnchorDefinition, "Registry", "Plugin", 30),
			grounded("assignment", types.EvidenceDirect, types.AnchorAssignment, "receiver", "Implementation", 40),
			grounded("return", types.EvidenceDirect, types.AnchorReturn, "factory", "Concrete", 50),
			typeRelation,
			uncitable,
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"accepted_grounded_source_facts=7",
		"grounded_callsite_facts=2",
		"explicit_typed_directed_relations=6",
		"relation_kind=`call`",
		"relation_kind=`callback`",
		"relation_kind=`register`",
		"relation_kind=`assignment`",
		"relation_kind=`return`",
		"relation_kind=`type_relation`",
		"edge_anchor_json=`{\"from_node\":\"n1\",\"to_node\":\"n2\",\"relation_kind\":\"call\"}`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mechanism relation authoring capsule missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "MustNotTeach") {
		t.Fatalf("uncitable recovered relation must not become a copyable recipe:\n%s", got)
	}
}

func TestTypedMechanismAuthoritySuppressesFalseClosureRoleSynthesisAA3(t *testing.T) {
	const falseRole = "MarkerPID 为 B/E span 配对标识键"
	item := types.EvidenceItem{
		ID:                 "marker-pid-definition",
		Kind:               types.EvidenceDirect,
		Source:             "internal/hitraceconv/streamerdb_sync_span_authority.go",
		LineStart:          113,
		LineEnd:            116,
		AnchorKind:         types.AnchorDefinition,
		AnchorSymbol:       "MarkerPID",
		Snippet:            "MarkerPID is the PID encoded inside tracing_mark_write; HeaderTGID remains the default and namespace PIDs are preserved.",
		Summary:            falseRole,
		GroundingStatus:    types.GroundingGrounded,
		LoadBearingSummary: false,
	}
	mu := types.NewMutableState("explain the current conversion mechanism")
	mu.SetInvestigationComplete(falseRole)
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		AcceptedClosureReason: falseRole,
		EvidenceItems:         []types.EvidenceItem{item},
	})
	mu.AppendEvidence([]types.EvidenceItem{item})
	ctx := &types.AgentContext{
		Mutable:       mu,
		EvidenceItems: []types.EvidenceItem{item},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentExplain,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes: []types.CurrentSourceExplanationMode{
					types.CurrentSourceExplanationExplainCurrentMechanism,
				},
				Confidence: 0.95,
			},
		}},
	}

	for name, got := range map[string]string{
		"extractor": renderExtractorAcceptedClosure(ctx, nil),
		"finalizer": (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil),
	} {
		if strings.Contains(got, falseRole) {
			t.Fatalf("%s prompt promoted an untyped closure role over grounded source evidence:\n%s", name, got)
		}
		if !strings.Contains(got, "model-authored closure reason omitted") {
			t.Fatalf("%s prompt did not disclose the typed mechanism authority boundary:\n%s", name, got)
		}
	}

	authority := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"accepted_grounded_source_facts=1",
		"ordered_path_authority=`unproven`",
		"proves that local fact only",
	} {
		if !strings.Contains(authority, want) {
			t.Fatalf("typed mechanism authority missing %q:\n%s", want, authority)
		}
	}
}
