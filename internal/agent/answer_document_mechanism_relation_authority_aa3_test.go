package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type answerDocExactRelationCandidateSourceFixture []types.TypedRelationCandidate

func (f answerDocExactRelationCandidateSourceFixture) TypedRelationCandidates(q types.TypedRelationQuery) []types.TypedRelationCandidate {
	var out []types.TypedRelationCandidate
	for _, row := range f {
		if q.AllowsKind(row.Relation) {
			out = append(out, row)
		}
	}
	return out
}

func answerDocExactRelationProviderContext(precision types.TypedRelationPrecision, file string) *types.AgentContext {
	return &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PredicateAxis: types.AxisImplement,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            string(types.ReqMechanism),
				PrimaryEntities: []string{"LoopController"},
			},
		}},
		Mutable: types.NewMutableState("type relation finalizer carrier"),
		MultiGraph: answerDocExactRelationCandidateSourceFixture{{
			Relation:   types.TypedRelationImplements,
			SourceName: "LoopController",
			SourceKind: "interface",
			Member: types.TypedRelationMember{
				Name: "analyzerEvaluator", File: file, Line: 49, Kind: "struct",
				SourceRole: types.ClassifySourcePathRole(file), Distance: 1,
			},
			Carrier:   types.TypedRelationCarrierGraph,
			Precision: precision,
		}},
	}
}

func TestMechanismRelationAuthorityUsesExactProviderBeforeDiagramRepair(t *testing.T) {
	ctx := answerDocExactRelationProviderContext(types.TypedRelationPrecisionExactSymbolID, "internal/agent/analyzer.go")
	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"explicit_typed_directed_relations=1",
		"edge_recipe[1]=`n1 -> n2`; relation_kind=`type_relation`",
		"analyzerEvaluator",
		"LoopController",
		"edge_anchor_json=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("finalizer authority missing shared exact provider carrier %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "No citable typed directed relation is available") {
		t.Fatalf("finalizer must not deny a carrier accepted by the strict validator:\n%s", got)
	}

	hint := answerDocOptionalDiagramCallEdgePatchHint(ctx, false)
	if !strings.Contains(hint, "bounded exact relation boundary") ||
		!strings.Contains(hint, "edge_recipe[1]=`n1 -> n2`; relation_kind=`type_relation`") ||
		strings.Contains(hint, "No copy-ready typed relation carrier is available") {
		t.Fatalf("optional repair must repeat the exact typed direction instead of steering diagram deletion:\n%s", hint)
	}
}

func TestMechanismRelationAuthorityDoesNotPromoteNameOnlyProviderRow(t *testing.T) {
	ctx := answerDocExactRelationProviderContext(types.TypedRelationPrecisionNameOnly, "internal/agent/analyzer.go")
	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if strings.Contains(got, "edge_recipe[") || strings.Contains(got, "explicit_typed_directed_relations=1") {
		t.Fatalf("name-only provider row must remain soft and cannot become a repair carrier:\n%s", got)
	}
}

func TestMechanismRelationAuthorityExactProviderIsCrossLanguage(t *testing.T) {
	for _, file := range []string{
		"controller.go",
		"src/Controller.java",
		"src/Controller.kt",
		"src/controller.ets",
		"src/controller.cj",
		"include/controller.hpp",
		"src/controller.rs",
	} {
		t.Run(file, func(t *testing.T) {
			ctx := answerDocExactRelationProviderContext(types.TypedRelationPrecisionExactFile, file)
			if got := renderAnswerDocMechanismRelationAuthority(ctx); !strings.Contains(got, "relation_kind=`type_relation`") {
				t.Fatalf("cross-language exact provider row lost before finalizer authoring: %s", got)
			}
		})
	}
}

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
	if !strings.Contains(got, "place the exact endpoint node inside that component's Mermaid subgraph/group") ||
		!strings.Contains(got, "Do not retarget the relation to an abstract component node") {
		t.Fatalf("typed component relation guidance must preserve exact endpoint and presentation layers:\n%s", got)
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
		"preserve its node IDs, exact edge topology, annotation carriers, and anchor array",
		"replace only visible node/message/annotation wording with model-authored business/domain language",
		"#### Copy-ready optional typed diagram",
		"sequenceDiagram",
		"participant n1 as convertTrace",
		"participant n2 as parseTraceMark",
		"n1->>n2: call",
		"edge_anchors_json=`[{\"from_node\":\"n1\",\"to_node\":\"n2\",\"from_identity\":\"convertTrace\",\"to_identity\":\"parseTraceMark\",\"relation_kind\":\"call\"}]`",
		"typed_flow_path[1]=`readEvent -> parseTraceMark -> classifySpan`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mechanism relation authority missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "copy both unchanged") {
		t.Fatalf("evidence skeleton must not contradict the business display layer by freezing visible metadata labels:\n%s", got)
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

func TestMechanismRelationAuthoritySoftParticipantCoverageIsLanguageNeutralAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
			PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            string(types.ReqMechanism),
				PrimaryEntities: []string{"Pipeline::Analyzer", "ArkRunner", "cj.mod.Consumer", "DetachedStage"},
			},
		}},
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "cpp-ark", Producer: types.EvidenceProducerExplorerEmitEvidence,
				Kind: types.EvidenceRelationship, Subject: "Pipeline.Analyzer", Predicate: "calls", Object: "ArkRunner",
				Source: "src/pipeline.cpp", LineStart: 20, AnchorKind: types.AnchorCall, AnchorSymbol: "ArkRunner",
				Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
			},
			{
				ID: "ark-cj", Producer: types.EvidenceProducerExplorerEmitEvidence,
				Kind: types.EvidenceRelationship, Subject: "ArkRunner", Predicate: "calls", Object: "cj::mod::Consumer",
				Source: "entry.ets", LineStart: 30, AnchorKind: types.AnchorCall, AnchorSymbol: "cj::mod::Consumer",
				Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "soft_named_participant_relation_coverage: incident=[Pipeline::Analyzer ArkRunner cj.mod.Consumer]; no_incident_typed_relation=[DetachedStage]") {
		t.Fatalf("cross-language typed participant incidence was not surfaced as soft context:\n%s", got)
	}
	if !strings.Contains(got, "not completeness authority") || !strings.Contains(got, "independent/unproven") {
		t.Fatalf("soft coverage boundary must forbid synthetic completeness:\n%s", got)
	}

	// Broad/augmented Entities are search hints only; without the analyzer's
	// original PrimaryEntities shortlist they must not create this checklist.
	ctx.AnalysisIR.RequestModel.AnalyzerHints.PrimaryEntities = nil
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"NoiseA", "NoiseB"}
	if got := renderAnswerDocMechanismRelationAuthority(ctx); strings.Contains(got, "soft_named_participant_relation_coverage") {
		t.Fatalf("broad entity hints must not be promoted into relation coverage context:\n%s", got)
	}
}

func TestMechanismRelationAuthorityTypedParticipantRolesStaySoftAndLanguageNeutralAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
			PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{
				{Identity: "Pipeline::Analyzer", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "ArkRunner", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "cj.mod.Consumer", Role: types.DiagramParticipantIncidentRequired},
				{Identity: "SharedContext", Role: types.DiagramParticipantContextOnly},
			}},
		}},
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "cpp-ark", Producer: types.EvidenceProducerExplorerEmitEvidence,
				Kind: types.EvidenceRelationship, Subject: "Pipeline.Analyzer", Predicate: "calls", Object: "ArkRunner",
				Source: "src/pipeline.cpp", LineStart: 20, AnchorKind: types.AnchorCall, AnchorSymbol: "ArkRunner",
				Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "typed_named_participant_relation_coverage: incident=[Pipeline::Analyzer ArkRunner]; no_incident_typed_relation=[cj.mod.Consumer]; context_only=[SharedContext]") {
		t.Fatalf("typed cross-language participant roles were not surfaced:\n%s", got)
	}
	for _, want := range []string{"planning/coverage guidance", "not relation evidence", "source_operation_missing=[cj.mod.Consumer]", "request_visible_boundary_only=[]", "Keep `context_only` participants outside the path"} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed participant authority boundary missing %q:\n%s", want, got)
		}
	}

	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	if got := renderAnswerDocMechanismRelationAuthority(ctx); strings.Contains(got, "typed_named_participant_relation_coverage") {
		t.Fatalf("runtime trace must stay on its independent causal authority lane:\n%s", got)
	}
}

func TestMechanismRelationAuthoritySeparatesRequestBoundaryFromSourceOperationAA3(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain, PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{
			Kind:     string(types.ReqMechanism),
			Entities: []string{"Analyzer", "stage", "product mode"},
			EntityProvenance: []types.EntityProvenance{
				{Surface: "Analyzer", Resolution: types.EntityResolutionSymbol, Resolved: true, UseForShape: true},
				{Surface: "stage", Resolution: types.EntityResolutionAmbiguousSymbol, UseForSearch: true},
				{Surface: "product mode", Resolution: types.EntityResolutionInferredConcept},
			},
		},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true, Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "stage", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "product mode", Role: types.DiagramParticipantIncidentRequired},
		}},
	}}, EvidenceItems: []types.EvidenceItem{{
		ID: "support", Producer: types.EvidenceProducerExplorerEmitEvidence,
		Kind: types.EvidenceRelationship, Subject: "Dispatch", Predicate: "calls", Object: "BuildContext",
		Source: "src/pipeline.go", LineStart: 20, AnchorKind: types.AnchorCall, AnchorSymbol: "BuildContext",
		Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
	}}}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"source_operation_missing=[Analyzer]",
		"request_visible_boundary_only=[stage product mode]",
		"do not search for or connect a homonymous operation",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed source/display split missing %q:\n%s", want, got)
		}
	}
}

func TestMechanismRelationAuthorityMapsExactOperationOwnerToTypedParticipantAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
			PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: []types.DiagramParticipantHint{{
				Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired,
			}}},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "mutable-call", Producer: types.EvidenceProducerExplorerEmitEvidence,
			Kind: types.EvidenceRelationship, Subject: "analyzerEvaluator.BuildInitialInstruction", Predicate: "calls",
			Object: "ctx.Mutable.ResetPrescanSummary", Source: "internal/agent/analyzer.go", LineStart: 89,
			AnchorKind: types.AnchorCall, AnchorSymbol: "ctx.Mutable.ResetPrescanSummary",
			Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
		}},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "typed_named_participant_relation_coverage: incident=[Mutable]; no_incident_typed_relation=[]") {
		t.Fatalf("exact receiver/member call did not cover typed participant:\n%s", got)
	}
	for _, want := range []string{"exact receiver/owner participant", "visible business/component label", "edge_anchors.from_identity/to_identity", "changes no relation kind"} {
		if !strings.Contains(got, want) {
			t.Fatalf("owner projection authority boundary missing %q:\n%s", want, got)
		}
	}
}

func TestMechanismRelationAuthorityMapsExactStaticBindingToTypedParticipantAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain,
			PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: []types.DiagramParticipantHint{{
				Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
			}}},
		}},
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "bus-context-write", Producer: types.EvidenceProducerExplorerEmitEvidence,
				Kind: types.EvidenceRelationship, Subject: "o.busCtx.EvidenceItems", Predicate: "assigns", Object: "output.EvidenceItems",
				Source: "internal/orchestrator/orchestrator.go", LineStart: 2520,
				AnchorKind: types.AnchorAssignment, AnchorSymbol: "o.busCtx.EvidenceItems", Scope: types.ScopeLine,
				GroundingStatus: types.GroundingGrounded, Snippet: "o.busCtx.EvidenceItems = output.EvidenceItems",
				OwnerIdentity: "Orchestrator.applyStageOutput",
				DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{
					Binding: "Orchestrator.busCtx", Type: "*types.BusContext", Owner: "Orchestrator",
				}},
			},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "typed_named_participant_relation_coverage: incident=[BusContext]; no_incident_typed_relation=[]") {
		t.Fatalf("exact static binding did not cover its typed participant:\n%s", got)
	}
	contract := renderAnswerDocDiagramContract(ctx, &types.DiagramContract{
		Required:     true,
		RequiredKind: types.DiagramArchitecture,
		Participants: ctx.AnalysisIR.RequestModel.DiagramHint.Participants,
	})
	if strings.Contains(contract, `participant_identity="BusContext"`) || strings.Contains(contract, "boundary_recipe[") {
		t.Fatalf("diagram contract must consume the same final participant coverage as current-source authority:\n%s\n--- authority ---\n%s", contract, got)
	}
	for _, want := range []string{"parser-stamped static declaration", "identity-only", "do not turn the declaration into an edge", "Untyped, ambiguous, or differently-owned bindings remain unproven"} {
		if !strings.Contains(got, want) {
			t.Fatalf("declared-binding authority boundary missing %q:\n%s", want, got)
		}
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
				Kind: types.EvidenceRelationship, Subject: "busCtx.AnalysisIR", Predicate: "assigns", Object: "out.AnalysisIR",
				Source: "internal/orchestrator/orchestrator.go", LineStart: 2520,
				AnchorKind: types.AnchorAssignment, AnchorSymbol: "AnalysisIR",
				Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded,
				Snippet: "o.busCtx.AnalysisIR = out.AnalysisIR",
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
			{ID: "selected-path", Path: []string{"out.AnalysisIR", "busCtx.AnalysisIR"}, EvidenceIDs: []string{"selected-operation"}},
		},
	}

	if plan := answerSupportPlan(ctx); plan == nil {
		t.Fatal("fixture must compile a support plan so the R221 bypass arm is exercised")
	}
	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "typed_flow_path[1]=`out.AnalysisIR -> busCtx.AnalysisIR`") {
		t.Fatalf("selected operation replay must retain principal ordered authority:\n%s", got)
	}
	if !strings.Contains(got, "relation_kind=`data_flow`") ||
		!strings.Contains(got, "node_alias[n1]=`out.AnalysisIR`") ||
		!strings.Contains(got, "node_alias[n2]=`o.busCtx.AnalysisIR`") {
		t.Fatalf("typed flow request must teach the exact RHS -> LHS data-flow relation:\n%s", got)
	}
	if strings.Contains(got, "typed_flow_path[2]") ||
		strings.Contains(got, "typed_flow_path[1]=`internal/agent/agent.go:BaseAgent.executeTool") {
		t.Fatalf("broad support plan promoted an automatic unrelated ordered path:\n%s", got)
	}
}

func TestMechanismRelationProjectionDistinguishesBindingAndExecutionDirectionAA3(t *testing.T) {
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, AnchorKind: types.AnchorAssignment,
		Subject: "busCtx.AnalysisIR", Object: "output.AnalysisIR",
		Snippet: "o.busCtx.AnalysisIR = output.AnalysisIR",
		Scope:   types.ScopeLine, GroundingStatus: types.GroundingGrounded,
	}
	if relation, from, to := answerDocMechanismRelationProjection(item, types.RequestModel{PredicateAxis: types.AxisFlow}); relation != types.DiagramRelDataFlow || from != "output.AnalysisIR" || to != "o.busCtx.AnalysisIR" {
		t.Fatalf("flow projection=(%q,%q,%q), want data_flow RHS -> LHS", relation, from, to)
	}
	if relation, from, to := answerDocMechanismRelationProjection(item, types.RequestModel{PredicateAxis: types.AxisCall}); relation != types.DiagramRelAssignment || from != "o.busCtx.AnalysisIR" || to != "output.AnalysisIR" {
		t.Fatalf("binding projection=(%q,%q,%q), want assignment LHS -> RHS", relation, from, to)
	}
	item.Subject = "applyStageOutput"
	if relation, _, _ := answerDocMechanismRelationProjection(item, types.RequestModel{PredicateAxis: types.AxisFlow}); relation != types.DiagramRelUnknown {
		t.Fatalf("false assignment endpoints minted %q authority", relation)
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
		"internal validation metadata",
		"Do not copy those tokens into Mermaid labels/Notes, titles, tables, or answer prose",
		"Express each proved segment with the repository's domain participants and operations",
		"if no useful domain annotation exists, omit the visible Note",
		"This does not authorize a cross-component edge",
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
				Subject: "sink_", Object: "selectedSink", Snippet: "sink_ = selectedSink;", GroundingStatus: types.GroundingGrounded,
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
		"[entry_role=`assignment_state_or_binding_fact`]",
		"[entry_role=`guard_condition_fact`]",
		"[entry_role=`directed_hop`]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed call-chain entry role missing %q:\n%s", want, got)
		}
	}
}

func TestCallChainTypedBridgeAndBranchHandoffUsesExactTypedJoinsOnlyAA3(t *testing.T) {
	plan := &types.AnswerSupportPlan{
		Family: types.QFCallChain,
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLaneCurrentCodePath,
			Entries: []types.AnswerSupportEntry{
				{EvidenceID: "py-call", ClaimForm: types.ClaimCallEdge, Subject: "FastTokenizer.tokenize", Object: "_fastlex.tokenize_bytes", Location: "tokenizer.py:21"},
				{EvidenceID: "binding", ClaimForm: types.ClaimRegistrationEdge, Subject: "_fastlex", Object: "py::tokenize_bytes", Location: "lib.rs:47"},
				{EvidenceID: "wrapper-call", ClaimForm: types.ClaimCallEdge, Subject: "py::tokenize_bytes", Object: "core::tokenize_bytes", Location: "lib.rs:42"},
				{EvidenceID: "guard", ClaimForm: types.ClaimGuardCondition, Source: "tokenizer.py", OwnerSymbol: "FastTokenizer.tokenize", AnchorSymbol: "_HAVE_NATIVE", Location: "tokenizer.py:20"},
				{EvidenceID: "enabled", ClaimForm: types.ClaimAssignmentFact, Source: "tokenizer.py", Subject: "_HAVE_NATIVE", Object: "True", Location: "tokenizer.py:3"},
				{EvidenceID: "disabled", ClaimForm: types.ClaimAssignmentFact, Source: "tokenizer.py", Subject: "_HAVE_NATIVE", Object: "False", Location: "tokenizer.py:6"},
				{EvidenceID: "nearby", ClaimForm: types.ClaimAssignmentFact, Source: "other.py", Subject: "_HAVE_NATIVE", Object: "False", Location: "other.py:1"},
			},
		}},
	}

	got := renderAnswerDocCallChainSemanticHandoffs(plan)
	for _, want := range []string{
		"Typed bridge and branch handoff (advisory)",
		"call_target=`_fastlex.tokenize_bytes`",
		"registered_export=`_fastlex.tokenize_bytes`",
		"registered_callable=`py::tokenize_bytes`",
		"downstream_execution_status=`proved_by_exact_registered_callable_call`",
		"guard_symbol=`_HAVE_NATIVE`",
		"True @ tokenizer.py:3 [enabled]",
		"False @ tokenizer.py:6 [disabled]",
		"state_coverage=`multiple_grounded_states`",
		"branch_call_mapping_status=`unproven_without_typed_branch_ownership`",
		"do not replace its explanation, create a call edge, or authorize a conclusion",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed bridge/branch handoff missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "other.py:1") {
		t.Fatalf("same-name state from another source must not join the guard:\n%s", got)
	}

	plan.Lanes[0].Entries[1].Object = "tokenize_bytes"
	got = renderAnswerDocCallChainSemanticHandoffs(plan)
	if !strings.Contains(got, "downstream_execution_status=`unresolved_from_unqualified_registered_callable`") {
		t.Fatalf("short registered callable must not guess the qualified wrapper:\n%s", got)
	}
	if strings.Contains(got, "proved_by_exact_registered_callable_call") {
		t.Fatalf("short-name coincidence must not close downstream execution:\n%s", got)
	}

	plan.Lanes[0].Entries = []types.AnswerSupportEntry{
		{EvidenceID: "py-call", ClaimForm: types.ClaimCallEdge, Subject: "FastTokenizer.tokenize", Object: "_fastlex.tokenize_bytes", Source: "tokenizer.py", Location: "tokenizer.py:21"},
		{EvidenceID: "binding", ClaimForm: types.ClaimRegistrationEdge, Subject: "m", Object: "wrap_pyfunction!(tokenize_bytes, m)", OwnerSymbol: "py::_fastlex", AnchorKind: types.AnchorCall, Source: "lib.rs", Location: "lib.rs:47", Producer: types.EvidenceProducerExplorerEmitEvidence},
		{EvidenceID: "wrapper-call", ClaimForm: types.ClaimCallEdge, Subject: "py::tokenize_bytes", Object: "super::tokenize_bytes", Source: "lib.rs", Location: "lib.rs:42", Producer: types.EvidenceProducerExplorerEmitEvidence},
	}
	got = renderAnswerDocCallChainSemanticHandoffs(plan)
	for _, want := range []string{
		"registered_export=`_fastlex.tokenize_bytes`",
		"registered_callable=`py::tokenize_bytes`",
		"binding_endpoint_status=`exact_owner_reference_join`",
		"downstream_execution_status=`proved_by_exact_registered_callable_call`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("exact owner/reference registration handoff missing %q:\n%s", want, got)
		}
	}
}

func TestMechanismRelationRegistrationOwnerReferenceJoinFailsClosedAA3(t *testing.T) {
	base := []types.AnswerSupportEntry{
		{EvidenceID: "py-call", ClaimForm: types.ClaimCallEdge, Subject: "FastTokenizer.tokenize", Object: "_fastlex.tokenize_bytes", Source: "tokenizer.py"},
		{EvidenceID: "binding", ClaimForm: types.ClaimRegistrationEdge, Subject: "m", Object: "wrap_pyfunction!(tokenize_bytes, m)", OwnerSymbol: "py::_fastlex", AnchorKind: types.AnchorCall, Source: "lib.rs", Producer: types.EvidenceProducerExplorerEmitEvidence},
		{EvidenceID: "wrapper-a", ClaimForm: types.ClaimCallEdge, Subject: "py::tokenize_bytes", Object: "py::core", Source: "lib.rs", Producer: types.EvidenceProducerExplorerEmitEvidence},
	}
	plan := &types.AnswerSupportPlan{Family: types.QFCallChain, Lanes: []types.AnswerSupportLane{{
		Kind: types.SupportLaneCurrentCodePath, Entries: base,
	}}}

	assertNoJoin := func(name string, mutate func([]types.AnswerSupportEntry) []types.AnswerSupportEntry) {
		t.Helper()
		entries := append([]types.AnswerSupportEntry(nil), base...)
		entries = mutate(entries)
		plan.Lanes[0].Entries = entries
		if got := renderAnswerDocCallChainSemanticHandoffs(plan); strings.Contains(got, "registered_export_handoff") {
			t.Fatalf("%s must fail closed:\n%s", name, got)
		}
	}
	assertNoJoin("ambiguous callable owners", func(entries []types.AnswerSupportEntry) []types.AnswerSupportEntry {
		entries[1].OwnerSymbol = "_fastlex"
		entries[2].Subject = "a::tokenize_bytes"
		return append(entries, types.AnswerSupportEntry{EvidenceID: "wrapper-b", ClaimForm: types.ClaimCallEdge, Subject: "b::tokenize_bytes", Object: "b::core", Source: "lib.rs", Producer: types.EvidenceProducerExplorerEmitEvidence})
	})
	assertNoJoin("missing stamped owner", func(entries []types.AnswerSupportEntry) []types.AnswerSupportEntry {
		entries[1].OwnerSymbol = ""
		return entries
	})
	assertNoJoin("untrusted owner provenance", func(entries []types.AnswerSupportEntry) []types.AnswerSupportEntry {
		entries[1].Producer = ""
		return entries
	})
	assertNoJoin("different binding owner", func(entries []types.AnswerSupportEntry) []types.AnswerSupportEntry {
		entries[1].OwnerSymbol = "py::other_module"
		return entries
	})
	assertNoJoin("different callable namespace", func(entries []types.AnswerSupportEntry) []types.AnswerSupportEntry {
		entries[2].Subject = "other::tokenize_bytes"
		return entries
	})
	assertNoJoin("substring-only reference", func(entries []types.AnswerSupportEntry) []types.AnswerSupportEntry {
		entries[1].Object = "wrap_pyfunction!(tokenize_bytes_extra, m)"
		return entries
	})
}

func TestRegisteredExportHandoffMapsToSequenceNoteEndpointsAA3(t *testing.T) {
	aliases := []answerDocMechanismAliasRow{
		{alias: "n1", identity: "FastTokenizer.tokenize"},
		{alias: "n2", identity: "_fastlex.tokenize_bytes"},
		{alias: "n3", identity: "py.tokenize_bytes"},
		{alias: "n4", identity: "tokenize_bytes"},
	}
	handoff := answerDocRegisteredExportHandoff{
		callTarget:         "_fastlex.tokenize_bytes",
		registeredCallable: "py::tokenize_bytes",
		bindingEvidenceID:  "binding",
	}
	rows := answerDocMechanismSemanticHandoffRows(aliases, []answerDocRegisteredExportHandoff{handoff})
	if len(rows) != 1 || rows[0].from != "n2" || rows[0].to != "n3" {
		t.Fatalf("registered export handoff did not map to exact existing endpoint aliases: %+v", rows)
	}
	var rendered strings.Builder
	renderAnswerDocMechanismRelationAuthoringCapsule(&rendered, []answerDocMechanismRelationEdge{
		{from: "FastTokenizer.tokenize", to: "_fastlex.tokenize_bytes", relation: types.DiagramRelCall},
		{from: "py.tokenize_bytes", to: "tokenize_bytes", relation: types.DiagramRelCall},
	}, nil, []answerDocRegisteredExportHandoff{handoff}, 8, types.DiagramSequence)
	for _, want := range []string{
		"Note over n2,n3: Export binding is verified; describe the runtime boundary in business language, not as a call",
		`edge_anchors_json=`,
	} {
		if !strings.Contains(rendered.String(), want) {
			t.Fatalf("copy-ready sequence lost exact non-call export handoff %q:\n%s", want, rendered.String())
		}
	}

	// Two compatible aliases would make the target ambiguous and must not
	// create even a non-edge Note.
	aliases = append(aliases, answerDocMechanismAliasRow{alias: "n5", identity: "py::tokenize_bytes"})
	if got := answerDocMechanismSemanticHandoffRows(aliases, []answerDocRegisteredExportHandoff{handoff}); len(got) != 0 {
		t.Fatalf("ambiguous callable aliases must fail closed: %+v", got)
	}
}

func TestRegistrationOwnerReferenceHandoffIsWiredIntoFinalizerPromptAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
			DiagramHint:   &types.DiagramHint{Kind: types.DiagramSequence},
		}},
		EvidenceItems: []types.EvidenceItem{
			{ID: "py-call", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "FastTokenizer.tokenize", Object: "_fastlex.tokenize_bytes", Source: "tokenizer.py", LineStart: 21, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded},
			{ID: "binding", Kind: types.EvidenceRegistration, AnchorKind: types.AnchorCall, Subject: "m", Object: "wrap_pyfunction!(tokenize_bytes, m)", OwnerSymbol: "py::_fastlex", Source: "lib.rs", LineStart: 47, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerExplorerEmitEvidence},
			{ID: "wrapper-call", Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "py::tokenize_bytes", Object: "super::tokenize_bytes", Source: "lib.rs", LineStart: 42, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerExplorerEmitEvidence},
		},
	}
	got := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"registered_export_handoff",
		"registered_export=`_fastlex.tokenize_bytes`",
		"registered_callable=`py::tokenize_bytes`",
		"binding_endpoint_status=`exact_owner_reference_join`",
		"semantic_handoff_note[1]=`n2,n5`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("finalizer prompt lost exact registration handoff %q:\n%s", want, got)
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
		"edge_anchors_json=`[{\"from_node\":\"n1\",\"to_node\":\"n2\",\"from_identity\":\"Caller\",\"to_identity\":\"Callee\",\"relation_kind\":\"call\"}]`",
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

func TestMechanismRelationCopyReadySequencePreservesNonMessageRelationsAsNotesAA3(t *testing.T) {
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
		"participant n3 as SinkRegistry::create",
		"participant n4 as ConsoleSink",
		"Note over n3,n4: Runtime binding is verified; describe the selected implementation",
		"Note over n3,n4: Selection condition is verified; describe the business condition",
		"Note over n3,n4: Factory result is verified; describe the created implementation",
		`edge_anchors_json=` + "`" + `[{"from_node":"n1","to_node":"n2","from_identity":"Logger.log","to_identity":"Sink.write","relation_kind":"call"}]` + "`",
		"visual_annotation_relation_count=3",
		"annotation_relation_kinds=`guard,register,return`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sequence visual subset missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"n3->>n4: register",
		"n3->>n4: guard",
		"n3->>n4: return",
		`"relation_kind":"register"`,
		`"relation_kind":"guard"`,
		`"relation_kind":"return"`,
	} {
		if strings.Contains(got[strings.Index(got, "#### Copy-ready optional typed diagram"):], forbidden) {
			t.Fatalf("copy-ready sequence converted a non-message Note into edge authority %q:\n%s", forbidden, got)
		}
	}
}

func TestMechanismRelationCopyReadySequencePreservesUnaryGuardAsOneParticipantNoteAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				RequiredKind: types.DiagramSequence,
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "call", Kind: types.EvidenceRelationship,
				Source: "src/registry.cpp", LineStart: 32, AnchorKind: types.AnchorCall,
				Subject: "make_sink", Object: "SinkRegistry::create",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				ID: "guard", Kind: types.EvidenceConditional,
				Source: "src/registry.cpp", LineStart: 17, AnchorKind: types.AnchorCondition,
				Subject: "SinkRegistry::create", Condition: `kind == "console"`,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"typed_unary_annotations=1",
		"participant n2 as SinkRegistry::create",
		"unary_note_recipe[1]=`n2`",
		"relation_kind=`guard`",
		"detail=`kind == \"console\"`",
		`Note over n2: Selection condition is verified (kind == &quot;console&quot;); describe the business condition`,
		"visual_annotation_relation_count=1",
		"annotation_relation_kinds=`guard`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("unary guard visual subset missing %q:\n%s", want, got)
		}
	}
	copyReady := got[strings.Index(got, "#### Copy-ready optional typed diagram"):]
	for _, forbidden := range []string{
		"n2->>n2",
		`"relation_kind":"guard"`,
		`"from_node":"n2"`,
	} {
		if strings.Contains(copyReady, forbidden) {
			t.Fatalf("unary guard minted edge authority %q:\n%s", forbidden, got)
		}
	}
}

func TestMechanismUnaryAnnotationCopyReadyMatrixIsClosedAA3(t *testing.T) {
	for _, kind := range []types.DiagramKind{
		types.DiagramSequence, types.DiagramFlow, types.DiagramArchitecture, types.DiagramCallDAG,
	} {
		if !answerDocMechanismUnaryAnnotationSafeForCopyReadyDiagram(kind, types.DiagramRelGuard) {
			t.Fatalf("reviewed family %q must carry unary guard without an edge", kind)
		}
	}
	if answerDocMechanismUnaryAnnotationSafeForCopyReadyDiagram(types.DiagramSequence, types.DiagramRelCall) ||
		answerDocMechanismUnaryAnnotationSafeForCopyReadyDiagram(types.DiagramNone, types.DiagramRelGuard) ||
		answerDocMechanismUnaryAnnotationSafeForCopyReadyDiagram(types.DiagramKind("future"), types.DiagramRelGuard) {
		t.Fatal("unknown/non-unary relations must fail closed")
	}
}

func TestMechanismRelationCopyReadyFlowPreservesUnaryGuardAsStandaloneFactNodeAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				Required: true, RequiredKind: types.DiagramCallDAG,
				PreferredKinds: []types.DiagramKind{types.DiagramCallDAG},
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "call", Kind: types.EvidenceRelationship,
				Source: "src/registry.cpp", LineStart: 32, AnchorKind: types.AnchorCall,
				Subject: "make_sink", Object: "SinkRegistry::create",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				ID: "guard", Kind: types.EvidenceConditional,
				Source: "src/registry.cpp", LineStart: 17, AnchorKind: types.AnchorCondition,
				Subject: "SinkRegistry::create", Condition: `kind == "console"`,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"visual_annotation_relation_count=1",
		"annotation_relation_kinds=`guard`",
		`u1["Selection condition for SinkRegistry::create (kind == &quot;console&quot;); describe the business condition"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("flow-family unary fact node missing %q:\n%s", want, got)
		}
	}
	copyReady := got[strings.Index(got, "#### Copy-ready optional typed diagram"):]
	for _, forbidden := range []string{"n2 --> u1", "u1 --> n2", `"relation_kind":"guard"`} {
		if strings.Contains(copyReady, forbidden) {
			t.Fatalf("flow-family unary guard minted an edge %q:\n%s", forbidden, got)
		}
	}
}

func TestMechanismRelationCopyReadyReceiptSeparatesRenderedAndOmittedKindsAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}},
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{
				Required: true, RequiredKind: types.DiagramCallDAG,
				PreferredKinds: []types.DiagramKind{types.DiagramCallDAG},
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				ID: "call", Kind: types.EvidenceRelationship,
				Source: "src/registry.cpp", LineStart: 32, AnchorKind: types.AnchorCall,
				Subject: "make_sink", Object: "SinkRegistry::create",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				ID: "return", Kind: types.EvidenceDirect,
				Source: "src/registry.cpp", LineStart: 18, AnchorKind: types.AnchorReturn,
				Subject: "SinkRegistry::create", Object: "ConsoleSink",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	if !strings.Contains(got, "visual_omitted_relation_kinds=`return`") {
		t.Fatalf("call_dag must disclose its actually omitted return relation:\n%s", got)
	}
	copyReady := got[strings.Index(got, "#### Copy-ready optional typed diagram"):]
	if strings.Contains(copyReady, "visual_annotation_relation_count=0") ||
		strings.Contains(copyReady, "return`; these are preserved") {
		t.Fatalf("omitted return must not be described as a rendered annotation:\n%s", got)
	}
}

func TestMechanismRelationCopyReadyMatrixIsClosedAndCompleteAA3(t *testing.T) {
	allButContain := map[types.DiagramRelationKind]bool{}
	for _, relation := range types.AllDiagramRelationKinds() {
		allButContain[relation] = relation != types.DiagramRelContain
	}
	want := map[types.DiagramKind]map[types.DiagramRelationKind]bool{
		types.DiagramFlow:         allButContain,
		types.DiagramArchitecture: allButContain,
		types.DiagramSequence: {
			types.DiagramRelCall:     true,
			types.DiagramRelCallback: true,
		},
		types.DiagramCallDAG: {
			types.DiagramRelCall: true,
		},
	}
	for kind, relations := range want {
		for _, relation := range types.AllDiagramRelationKinds() {
			got := answerDocMechanismRelationSafeForCopyReadyDiagram(kind, relation)
			if got != relations[relation] {
				t.Errorf("diagram capability kind=%s relation=%s got=%t want=%t", kind, relation, got, relations[relation])
			}
		}
	}
	if answerDocMechanismRelationSafeForCopyReadyDiagram(types.DiagramNone, types.DiagramRelCall) {
		t.Fatal("no-diagram presentation must not claim a visual relation capability")
	}
	if answerDocMechanismRelationSafeForCopyReadyDiagram(types.DiagramKind("future"), types.DiagramRelCall) {
		t.Fatal("unknown future diagram family must fail closed")
	}
	if answerDocMechanismRelationSafeForCopyReadyDiagram(types.DiagramFlow, types.DiagramRelationKind("future")) {
		t.Fatal("unknown future relation must fail closed instead of becoming a generic arrow")
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
			func() types.EvidenceItem {
				item := grounded("assignment", types.EvidenceDirect, types.AnchorAssignment, "receiver", "Implementation", 40)
				item.Snippet = "receiver = Implementation"
				return item
			}(),
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
		"edge_anchor_json=`{\"from_node\":\"n1\",\"to_node\":\"n2\",\"from_identity\":\"Caller\",\"to_identity\":\"Callee\",\"relation_kind\":\"call\"}`",
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
