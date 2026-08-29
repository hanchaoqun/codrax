package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// diagramVerifiedReadModeStagePrecedence is the narrow bridge from the shared
// checkout provider into diagram validation.  Only source-code flow views in
// the current read mode may consume it. Runtime/root-cause Trace keeps its
// separate report-local causal/temporal authority.
func diagramVerifiedReadModeStagePrecedence(ctx *types.BusContext, view *types.AnswerSemanticView) []stageauthority.PrecedenceRelation {
	if ctx == nil || ctx.AnalysisIR == nil || view == nil || view.Family == types.QFRootCauseTrace ||
		view.RelationAxis != types.AxisFlow ||
		(ctx.Mode != "" && ctx.Mode != types.ModeRead) {
		return nil
	}
	authority, ok := stageauthority.LoadReadMode(ctx.RepoRoot)
	var evidence []types.EvidenceItem
	if ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	if !ok {
		return nil
	}
	return stageauthority.SelectRequiredReadModeWorkflow(ctx.AnalysisIR.RequestModel, evidence, authority).Precedence
}

// diagramVerifiedReadModeStageEdgeAuthority is the optional-edge counterpart
// of diagramVerifiedReadModeStagePrecedence.  The latter drives requested
// completeness and participant scope; this helper only validates a relation
// the model already chose to author.  Keeping the two lanes separate prevents
// an optional truthful diagram from being rejected while also preventing its
// presence from manufacturing a new must-draw contract.
func diagramVerifiedReadModeStageEdgeAuthority(ctx *types.BusContext, view *types.AnswerSemanticView) []stageauthority.PrecedenceRelation {
	if ctx == nil || ctx.AnalysisIR == nil || view == nil || view.Family == types.QFRootCauseTrace ||
		view.RelationAxis != types.AxisFlow ||
		(ctx.Mode != "" && ctx.Mode != types.ModeRead) {
		return nil
	}
	authority, ok := stageauthority.LoadReadMode(ctx.RepoRoot)
	if !ok {
		return nil
	}
	var evidence []types.EvidenceItem
	if ctx.Mutable != nil {
		evidence = ctx.Mutable.EmittedEvidence()
	}
	return stageauthority.SelectAvailableReadModeStageEdges(
		ctx.AnalysisIR.RequestModel, evidence, authority,
	).Precedence
}

// DiagramRequestedStagePrecedenceSpineMismatches verifies that one
// model-authored diagram visibly carries the complete checkout-verified
// requested stage spine. Supporting diagrams may show other grounded calls or
// relations, but distributing adjacent stage relations across several diagrams
// or omitting them all cannot satisfy the principal requested relation view.
//
// The check consumes only Mermaid structure, exact typed relation owners, and
// provider rows selected from the typed request. It does not inspect request,
// edge-label, reasoning, or final-answer prose, and it never authors an edge.
func DiagramRequestedStagePrecedenceSpineMismatches(
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	relations []stageauthority.PrecedenceRelation,
) []DiagramCallEdgeEvidenceMismatch {
	if doc == nil || view == nil || view.Family == types.QFRootCauseTrace || len(relations) == 0 {
		return nil
	}

	edgeBlockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	bestBlockID := ""
	bestCoverage := -1
	var bestCovered []bool
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		edges := mermaidcompat.ParseEdges(block.Diagram.Body)
		labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
		anchors := diagramEvidenceEffectiveAnchorsForBlock(doc, i, edgeBlockCounts)
		typedRelations := diagramTypedAnchorRelationSet(anchors)
		candidates := make([][]mermaidcompat.Edge, len(relations))
		covered := make([]bool, len(relations))
		for _, edge := range edges {
			if !typedRelations[diagramEvidenceEdgeKey(edge.From, edge.To)][types.DiagramRelPrecedence] {
				continue
			}
			for relationIndex := range relations {
				if !diagramStagePrecedenceEdgeHasTypedAuthority(
					[]stageauthority.PrecedenceRelation{relations[relationIndex]},
					edge.From, edge.To, anchors, labels,
				) {
					continue
				}
				candidates[relationIndex] = append(candidates[relationIndex], edge)
				covered[relationIndex] = true
			}
		}
		if diagramStagePrecedenceCandidatesFormVisibleChain(candidates, 0, "") {
			return nil
		}
		coverage := 0
		for _, ok := range covered {
			if ok {
				coverage++
			}
		}
		if coverage > bestCoverage {
			bestCoverage = coverage
			bestBlockID = strings.TrimSpace(block.ID)
			bestCovered = append([]bool(nil), covered...)
		}
	}
	// Required-diagram block absence is already owned by the block-coverage
	// contract. Avoid emitting a second diagnosis when there is no diagram to
	// repair in place.
	if bestBlockID == "" {
		return nil
	}

	missing := make([]bool, len(relations))
	missingCount := 0
	for i := range relations {
		if i >= len(bestCovered) || !bestCovered[i] {
			missing[i] = true
			missingCount++
		}
	}
	// Every relation can be present yet still be visually disconnected because
	// the shared stage was declared under different node IDs. In that case the
	// whole spine, rather than any individual relation, is the failed carrier.
	if missingCount == 0 {
		for i := range missing {
			missing[i] = true
		}
	}
	out := make([]DiagramCallEdgeEvidenceMismatch, 0, len(relations))
	for i, relation := range relations {
		if !missing[i] {
			continue
		}
		out = append(out, DiagramCallEdgeEvidenceMismatch{
			BlockID:    bestBlockID,
			Issue:      diagramRequestedStageSpineIncomplete,
			FromNode:   relation.From.StageIdent,
			ToNode:     relation.To.StageIdent,
			FromSymbol: relation.From.AgentValue,
			ToSymbol:   relation.To.AgentValue,
			Relation:   types.DiagramRelPrecedence,
		})
	}
	return out
}

func diagramStagePrecedenceCandidatesFormVisibleChain(candidates [][]mermaidcompat.Edge, index int, previousNode string) bool {
	if index >= len(candidates) {
		return true
	}
	for _, edge := range candidates[index] {
		from := strings.ToLower(strings.TrimSpace(edge.From))
		to := strings.ToLower(strings.TrimSpace(edge.To))
		if from == "" || to == "" || (index > 0 && from != previousNode) {
			continue
		}
		if diagramStagePrecedenceCandidatesFormVisibleChain(candidates, index+1, to) {
			return true
		}
	}
	return false
}
