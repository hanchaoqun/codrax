package tool

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// RequiredRuntimeDiagramRelationMissing checks the same typed support that
// restored the requested diagram. A model-authored unproven business role is
// not a proof that the already available native relations are absent. This is
// only minimum visual delivery, not complete population or business-role
// mapping. Every authored edge still passes its independent authority gate.
func RequiredRuntimeDiagramRelationMissing(ctx *types.BusContext, doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) bool {
	_, _, missing := requiredRuntimeDiagramRelationCoverage(ctx, doc, view)
	return missing
}

func requiredRuntimeDiagramRelationCoverage(ctx *types.BusContext, doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) ([]RuntimeDiagramRelation, []string, bool) {
	if ctx == nil || ctx.AnalysisIR == nil || doc == nil || view == nil ||
		(view.Family == types.QFRootCauseTrace && !types.RuntimeRelationDiagramUsesNativeAuthority(&ctx.AnalysisIR.RequestModel)) || view.DiagramPlan == nil ||
		!view.DiagramPlan.Required || !view.DiagramPlan.RequireStructuralEdge {
		return nil, nil, false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	contract := types.RuntimeSupportedDiagramContract(ctx.AnalysisIR, ledger, ctx.TurnRouteHint)
	if contract == nil {
		return nil, nil, false
	}
	rows := RuntimeDiagramRelations(ledger, &ctx.AnalysisIR.RequestModel)
	var blockIDs []string
	counts := diagramEvidenceBodyEdgeBlockCounts(doc)
	for i, block := range doc.Blocks {
		if block.Kind != types.BlockDiagram {
			continue
		}
		if block.Diagram == nil {
			// A current native fact diagram is a valid edgeless display, but
			// cannot discharge an independently required relationship. Rebind
			// against this view because pre-emit selectors are not persisted yet.
			if block.RuntimeMeasurement != nil && runtimeMeasurementBindingValid(block, view) {
				blockIDs = append(blockIDs, block.ID)
			}
			continue
		}
		if block.Diagram.Kind != contract.RequiredKind {
			continue
		}
		blockIDs = append(blockIDs, block.ID)
		anchors := diagramEvidenceEffectiveAnchorsForBlock(doc, i, counts)
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			for _, anchor := range anchors {
				if strings.TrimSpace(anchor.FromNode) == edge.From && strings.TrimSpace(anchor.ToNode) == edge.To &&
					anchor.HasEndpointIdentityPair() && !runtimeDiagramAnchorAliasConflict(rows, anchor) &&
					runtimeDiagramRelationProved(rows, anchor.FromIdentity, anchor.ToIdentity, anchor.RelationKind) {
					return rows, blockIDs, false
				}
			}
		}
	}
	// The existing required-block/kind gate owns absent or incompatible graphs.
	return rows, blockIDs, len(rows) > 0 && len(blockIDs) > 0
}

func preCheckRequiredRuntimeDiagramRelation(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, pctx *preEmitCheckContext) []emitFixHint {
	if pctx == nil {
		return nil
	}
	rows, blockIDs, missing := requiredRuntimeDiagramRelationCoverage(pctx.ctx, doc, view)
	if !missing {
		return nil
	}
	// Local edge edits require an existing Mermaid body. Never publish an
	// inapplicable add-edge lease for a producer-owned edgeless fact selector.
	var editableBlockIDs []string
	for _, id := range blockIDs {
		for _, block := range doc.Blocks {
			if block.ID == id && block.Diagram != nil {
				editableBlockIDs = append(editableBlockIDs, id)
				break
			}
		}
	}
	if len(editableBlockIDs) == 0 {
		return []emitFixHint{{
			Field:               "blocks[kind=diagram]",
			HardSignal:          preEmitHardSignalTypedCallEdgeEvidence,
			OffendingBlockKinds: []types.AnswerBlockKind{types.BlockDiagram},
			ExpectedShape:       "The native fact diagram is an edgeless measurement display, not the independently requested runtime relationship diagram. Preserve its exact facts as a runtime_measurement table and author the required diagram separately using an available producer-verified relation and its exact endpoint identity anchor. Use complete block replacements/additions; do not attach diagram content or relation metadata to the native fact selector. Do not invent a relationship or draw every candidate.",
			Reason:              "a current native fact display cannot satisfy an independently required runtime relationship",
		}}
	}
	// Publish only existing scoped provider candidates. The model chooses the
	// relation, node IDs, labels and position; no synthetic failure selects an
	// arbitrary missing edge or makes every provider row mandatory.
	delta, _ := json.Marshal(types.AnswerDiagramRelationRepairDelta{
		Version: types.AnswerDiagramRelationRepairDeltaVersion, PreserveUnlistedEdges: true,
		AllowedAdditions: appendRuntimeDiagramRepairCandidates(nil, rows, editableBlockIDs, 8),
	})
	return []emitFixHint{{
		Field:                          "blocks[kind=diagram].diagram.body AND blocks[].edge_anchors",
		HardSignal:                     preEmitHardSignalTypedCallEdgeEvidence,
		OffendingBlockKinds:            []types.AnswerBlockKind{types.BlockDiagram},
		ExpectedShape:                  "The requested runtime relation diagram currently shows none of its available producer-verified relations. Select an existing runtime relation candidate and show it with a matching visible edge and exact identity anchor; preserve useful business labels and other content. A disconnected role or unproven role mapping cannot substitute for the proven instance relation. Do not invent links, promote correspondence to causality, or draw every candidate merely to satisfy this minimum.",
		Reason:                         "a required runtime relation diagram is supported by current scoped native relation facts, but no such fact is visibly rendered",
		DiagramRelationRepairDeltaJSON: string(delta),
	}}
}
