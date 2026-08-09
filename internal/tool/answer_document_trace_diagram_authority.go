package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// preCheckRuntimeTraceTemporalDiagramAuthority closes the structured diagram
// half of an exact runtime authority boundary. It activates only when this
// report has positive unproven frame-temporal edges and no typed causal row in
// any consumed trace result. A mixed report with a proved wakeup/IPC/lock/flow
// row therefore stays out of this report-wide fallback instead of being
// downgraded by an unrelated earlier frame probe.
//
// The check reads only TraceEvidenceAuthority, parsed Mermaid edges, and the
// schema-valid relation_kind enum. It never scans the request, reasoning, edge
// messages, block prose, or rendered answer for causal words.
func preCheckRuntimeTraceTemporalDiagramAuthority(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, pctx *preEmitCheckContext) []emitFixHint {
	if doc == nil || view == nil || view.Family != types.QFRootCauseTrace || pctx == nil ||
		!runtimeTraceOnlyUnprovenFrameTemporalAuthority(pctx.ctx) {
		return nil
	}

	bodyEdgeBlockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	var mismatches []DiagramCallEdgeEvidenceMismatch
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		edges := mermaidcompat.ParseEdges(block.Diagram.Body)
		if len(edges) == 0 {
			continue
		}
		relations := diagramTypedAnchorRelationSet(
			diagramEvidenceEffectiveAnchorsForBlock(doc, i, bodyEdgeBlockCounts),
		)
		for _, edge := range edges {
			key := diagramEvidenceEdgeKey(edge.From, edge.To)
			typed := relations[key]
			if len(typed) == 1 && (typed[types.DiagramRelTemporal] || typed[types.DiagramRelObserve]) {
				continue
			}
			mismatches = append(mismatches, DiagramCallEdgeEvidenceMismatch{
				BlockID:  block.ID,
				Issue:    "runtime_temporal_edge_owner_required",
				FromNode: strings.TrimSpace(edge.From),
				ToNode:   strings.TrimSpace(edge.To),
				Relation: firstDiagramRelation(typed),
			})
		}
	}
	if len(mismatches) == 0 {
		return nil
	}

	parts := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		relation := string(mismatch.Relation)
		if relation == "" {
			relation = "missing"
		}
		parts = append(parts, fmt.Sprintf("block=%q edge=%s->%s relation_kind=%s",
			mismatch.BlockID, mismatch.FromNode, mismatch.ToNode, relation))
	}
	return []emitFixHint{{
		Field:                       "blocks[kind=diagram].diagram.body AND blocks[].edge_anchors[]",
		HardSignal:                  preEmitHardSignalTypedCallEdgeEvidence,
		OffendingBlockKinds:         preEmitDiagramMismatchBlockKinds(doc, mismatches),
		ExpectedShape:               types.RuntimeTraceTemporalDiagramRelationContract + " Repair only these visible pairs: " + strings.Join(parts, "; ") + diagramRelationSurgicalRepairInstruction,
		Reason:                      "the report-local typed trace authority contains measured temporal adjacency but zero typed causal rows. A call/callback/other causal relation enum or an unowned arrow would upgrade ordering into causality; relation_kind=temporal preserves frame ordering, while a separately grounded relation_kind=observe remains background evidence rather than a cause.",
		DiagramRelationFailurePairs: diagramRelationFailurePairFingerprints(mismatches),
	}}
}

func runtimeTraceOnlyUnprovenFrameTemporalAuthority(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, 1)
	results := append(append([]types.ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...)
	// Keep the raw per-dispatch authority rows as well as the ledger merge.
	// The ledger intentionally coalesces repeated trace-query results for
	// publication, while this ceiling must notice any proved typed causal row
	// before applying a report-wide temporal-only fallback.
	if ctx.Mutable != nil {
		results = append(results, ctx.Mutable.DispatchToolResults()...)
		results = append(results, ctx.Mutable.SystemTraceSupplementResults()...)
	}
	hasUnprovenTemporalFrameEdge := false
	for _, result := range results {
		authority := result.TraceEvidenceAuthority
		if authority == nil {
			continue
		}
		if authority.TypedCausalRowCount > 0 || strings.TrimSpace(authority.CausalConclusion) == "bounded_by_typed_rows" {
			return false
		}
		frameConclusion := strings.TrimSpace(authority.FrameFlowCausalConclusion)
		if frameConclusion != "" && frameConclusion != "unproven" {
			return false
		}
		if frameConclusion == "unproven" &&
			strings.TrimSpace(authority.FrameFlowRelationAuthority) == "temporal_sequence" &&
			authority.FrameFlowEdgeCount > 0 {
			hasUnprovenTemporalFrameEdge = true
		}
	}
	return hasUnprovenTemporalFrameEdge
}

func firstDiagramRelation(relations map[types.DiagramRelationKind]bool) types.DiagramRelationKind {
	for _, relation := range types.AllDiagramRelationKinds() {
		if relations[relation] {
			return relation
		}
	}
	return types.DiagramRelUnknown
}
