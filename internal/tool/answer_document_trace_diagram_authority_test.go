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
