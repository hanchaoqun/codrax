package orchestrator

import (
	"context"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeTemporalPostCheckFixture() (*types.AnswerDocumentV2, *types.AnswerSemanticView, *types.MutableState, *types.BusContext) {
	mut := types.NewMutableState("frame flow")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                       "frame_flow",
			FrameFlowEdgeCount:         1,
			FrameFlowRelationAuthority: "temporal_sequence",
			FrameFlowCausalConclusion:  "unproven",
			CausalConclusion:           "unproven",
		},
		Observations: []types.ObservationRecord{{
			ID:              "frame-edge-1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query:run1",
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact,
				Path: "/tmp/frame.trace",
			},
			Span:      types.ObservationSpan{LineStart: 10, LineEnd: 11},
			ClaimKey:  "evidence_fact:frame_temporal_sequence",
			Subject:   "UI",
			Predicate: "frame_temporal_sequence",
			Object:    "RS",
		}},
	})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{includeRuntimeTemporalPostCheckBlock()}}
	view := &types.AnswerSemanticView{
		Family:       types.QFGeneric,
		RelationAxis: types.AxisFlow,
		DiagramPlan: &types.DiagramFacetGraph{
			Kind: types.DiagramFlow,
			EdgeRelations: []types.DiagramEdgeRelationContract{{
				Kind: types.DiagramRelGuard,
				Min:  1,
			}},
		},
	}
	bus := &types.BusContext{Mutable: mut}
	return doc, view, mut, bus
}

func includeRuntimeTemporalPostCheckBlock() types.AnswerBlock {
	return types.AnswerBlock{
		ID:   "runtime-frame",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramSequence,
			Language: "mermaid",
			Body:     "sequenceDiagram\n  participant UI\n  participant RS\n  UI-->>RS: temporal adjacency (unproven)",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "UI", ToNode: "RS", RelationKind: types.DiagramRelTemporal,
		}},
	}
}

func TestRuntimeTemporalDiagramPostCheckUsesSameTypedOwnerAsPreEmit(t *testing.T) {
	doc, view, mut, bus := runtimeTemporalPostCheckFixture()
	violations := runV2BlockOraclesWithOracleContext(context.Background(), doc, view, mut, nil, nil, bus)
	for _, violation := range violations {
		if violation.Kind == types.ViolDiagramCallEdgeUnproven ||
			violation.Kind == types.ViolDiagramRelationLabelOnly {
			t.Fatalf("post-check reclassified accepted runtime temporal block: %+v", violation)
		}
	}
}

func TestRuntimeTemporalDiagramPostCheckDoesNotExemptSiblingSourceBlock(t *testing.T) {
	doc, view, mut, bus := runtimeTemporalPostCheckFixture()
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:   "source-flow",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramSequence,
			Language: "mermaid",
			Body:     "sequenceDiagram\n  participant A\n  participant B\n  A->>B: source call",
		},
	})
	violations := runV2BlockOraclesWithOracleContext(context.Background(), doc, view, mut, nil, nil, bus)
	for _, violation := range violations {
		if violation.Kind == types.ViolDiagramCallEdgeUnproven {
			return
		}
	}
	t.Fatalf("runtime temporal block exempted sibling source diagram: %+v", violations)
}
