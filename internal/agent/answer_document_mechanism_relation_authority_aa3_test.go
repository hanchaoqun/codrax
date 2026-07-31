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
		"explicit_caller_callee_edges=0",
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
		"explicit_caller_callee_edges=1",
		"ordered_path_authority=`typed_flow_paths_present`",
		"grounded_edge[1]=`convertTrace -> parseTraceMark`",
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
