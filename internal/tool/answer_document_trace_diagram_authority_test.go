package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func temporalTraceDiagramTestContext(authorities ...types.TraceEvidenceAuthority) *preEmitCheckContext {
	mut := types.NewMutableState("frame flow")
	for i := range authorities {
		authority := authorities[i]
		mut.AppendDispatchToolResult(types.ToolResult{
			ToolName:               "trace_query",
			Success:                true,
			TraceEvidenceAuthority: &authority,
		})
	}
	return newPreEmitCheckContext(&types.BusContext{Mutable: mut})
}

func reportLocalTemporalTraceDiagramTestContext() *preEmitCheckContext {
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
	return newPreEmitCheckContext(&types.BusContext{Mutable: mut})
}

func temporalTraceDiagramTestDocument(relation types.DiagramRelationKind) *types.AnswerDocumentV2 {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "frame-flow",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence,
			Body: "sequenceDiagram\n  participant UI\n  participant RS\n  UI->>RS: temporal adjacency (unproven)",
		},
	}}}
	if relation.IsValid() {
		doc.Blocks[0].EdgeAnchors = []types.DiagramEdgeAnchor{{
			FromNode: "UI", ToNode: "RS", RelationKind: relation,
		}}
	}
	return doc
}

func TestPreCheckRuntimeTraceTemporalDiagramAuthorityRequiresTypedTemporalOwner(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFRootCauseTrace}
	pctx := temporalTraceDiagramTestContext(types.TraceEvidenceAuthority{
		View:                       "frame_flow",
		FrameFlowEdgeCount:         1,
		FrameFlowRelationAuthority: "temporal_sequence",
		FrameFlowCausalConclusion:  "unproven",
		CausalConclusion:           "unproven",
	})

	for _, relation := range []types.DiagramRelationKind{types.DiagramRelUnknown, types.DiagramRelCall, types.DiagramRelCallback} {
		hints := preCheckRuntimeTraceTemporalDiagramAuthority(temporalTraceDiagramTestDocument(relation), view, pctx)
		if len(hints) != 1 || hints[0].HardSignal != preEmitHardSignalTypedCallEdgeEvidence ||
			!strings.Contains(hints[0].ExpectedShape, "relation_kind=temporal") {
			t.Fatalf("relation=%q hints=%+v, want one precise temporal-owner repair", relation, hints)
		}
	}
	if hints := preCheckRuntimeTraceTemporalDiagramAuthority(
		temporalTraceDiagramTestDocument(types.DiagramRelTemporal), view, pctx,
	); len(hints) != 0 {
		t.Fatalf("typed temporal adjacency should pass: %+v", hints)
	}
	if hints := preCheckRuntimeTraceTemporalDiagramAuthority(
		temporalTraceDiagramTestDocument(types.DiagramRelObserve), view, pctx,
	); len(hints) != 0 {
		t.Fatalf("a separately grounded observation edge must remain eligible as background support: %+v", hints)
	}
}

func TestPreCheckRuntimeTraceTemporalDiagramAuthorityDoesNotStickyDowngradeMixedProvedChain(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFRootCauseTrace}
	pctx := temporalTraceDiagramTestContext(
		types.TraceEvidenceAuthority{
			View: "frame_flow", FrameFlowEdgeCount: 1,
			FrameFlowRelationAuthority: "temporal_sequence",
			FrameFlowCausalConclusion:  "unproven", CausalConclusion: "unproven",
		},
		types.TraceEvidenceAuthority{
			View: "wakeup_chain", TypedCausalRowCount: 2,
			CausalConclusion: "bounded_by_typed_rows",
		},
	)
	if hints := preCheckRuntimeTraceTemporalDiagramAuthority(
		temporalTraceDiagramTestDocument(types.DiagramRelCall), view, pctx,
	); len(hints) != 0 {
		t.Fatalf("an unrelated unproven frame probe must not report-wide downgrade a proved typed chain: %+v", hints)
	}
}

func TestPreCheckDiagramRelationRoutesGenericFlowToReportLocalRuntimeAuthority(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	doc := temporalTraceDiagramTestDocument(types.DiagramRelTemporal)
	if hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, reportLocalTemporalTraceDiagramTestContext()); len(hints) != 0 {
		t.Fatalf("generic flow-axis trace diagram was incorrectly routed to source-call authority: %+v", hints)
	}
}

func TestPreCheckDiagramRelationKeepsReportLocalRuntimeBlockWhenSiblingCausalResultExists(t *testing.T) {
	pctx := reportLocalTemporalTraceDiagramTestContext()
	pctx.ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View: "wakeup_chain", TypedCausalRowCount: 2,
			CausalConclusion: "bounded_by_typed_rows",
		},
	})
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	if hints := preCheckDiagramCallEdgeEvidenceAlignment(
		temporalTraceDiagramTestDocument(types.DiagramRelTemporal), view, pctx,
	); len(hints) != 0 {
		t.Fatalf("a proved sibling query must not revoke the frame result's local temporal authority: %+v", hints)
	}
}

func TestPreCheckDiagramRelationDoesNotExemptSiblingSourceDiagram(t *testing.T) {
	doc := temporalTraceDiagramTestDocument(types.DiagramRelTemporal)
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:   "source-flow",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind: types.DiagramSequence,
			Body: "sequenceDiagram\n  participant A\n  participant B\n  A->>B: source call",
		},
	})
	view := &types.AnswerSemanticView{Family: types.QFGeneric, RelationAxis: types.AxisFlow}
	hints := preCheckDiagramCallEdgeEvidenceAlignment(doc, view, reportLocalTemporalTraceDiagramTestContext())
	if len(hints) == 0 || !strings.Contains(hints[0].ExpectedShape, "missing") &&
		!strings.Contains(hints[0].ExpectedShape, "missing_call_anchor") {
		t.Fatalf("runtime-owned block exempted an unrelated source block: %+v", hints)
	}
}

func TestPreCheckDiagramRelationRejectsTemporalAliasWithoutLocalTypedEndpoint(t *testing.T) {
	doc := temporalTraceDiagramTestDocument(types.DiagramRelTemporal)
	doc.Blocks[0].Diagram.Body = "sequenceDiagram\n  participant UI\n  participant GPU\n  UI->>GPU: temporal adjacency (unproven)"
	doc.Blocks[0].EdgeAnchors[0].ToNode = "GPU"
	owned, hints := preCheckReportLocalRuntimeTemporalDiagramAuthority(doc, reportLocalTemporalTraceDiagramTestContext())
	if len(owned) != 0 || len(hints) != 1 || !strings.Contains(hints[0].Reason, "not a subset") {
		t.Fatalf("unproved temporal endpoint did not fail closed: owned=%v hints=%+v", owned, hints)
	}
}

func TestPreCheckDiagramRelationRequiresOneTemporalAnchorPerOccurrence(t *testing.T) {
	doc := temporalTraceDiagramTestDocument(types.DiagramRelTemporal)
	doc.Blocks[0].Diagram.Body += "\n  UI->>RS: repeated temporal adjacency"
	owned, hints := preCheckReportLocalRuntimeTemporalDiagramAuthority(doc, reportLocalTemporalTraceDiagramTestContext())
	if len(owned) != 0 || len(hints) != 1 {
		t.Fatalf("duplicate visible occurrence borrowed one typed row/anchor: owned=%v hints=%+v", owned, hints)
	}
}
