package agent

import (
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

func TestMechanismRelationAuthorityPublishesOnlyTypedEdgesAndFlowPathsAA3(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				ID:              "edge",
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
			{ID: "supported", Path: []string{"readEvent", "parseTraceMark", "classifySpan"}},
			{ID: "unsupported", Path: []string{"A", "B"}, UnsupportedReason: "missing edge"},
		},
	}

	got := renderAnswerDocMechanismRelationAuthority(ctx)
	for _, want := range []string{
		"explicit_typed_directed_relations=1",
		"ordered_path_authority=`typed_flow_paths_present`",
		"node_alias[n1]=`convertTrace`",
		"node_alias[n2]=`parseTraceMark`",
		"edge_recipe[1]=`n1 -> n2`; relation_kind=`call`; edge_anchor_json=`{\"from_node\":\"n1\",\"to_node\":\"n2\",\"relation_kind\":\"call\"}`",
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
