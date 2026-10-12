package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// IsolateRecoveredAnswerDiagrams is a display-only projection of a recovery
// copy. It never accepts a document, modifies stored failed evidence, or repairs
// an edge. Each graph is checked against today's typed evidence independently:
// an unrelated table failure or another invalid graph cannot erase a valid one.
// Unstructured attachments have lost their relationship metadata; a writable
// Source string cannot restore that authority (including system-looking names).
func IsolateRecoveredAnswerDiagrams(ctx *types.BusContext, doc *types.AnswerDocumentV2, attachments []types.AnswerDisplayAttachment, language string) ([]types.AnswerDisplayAttachment, int) {
	isolated := 0
	if doc != nil {
		view := types.BuildAnswerSemanticViewForBusContext(ctx)
		pctx := newPreEmitCheckContext(ctx)
		counts := diagramEvidenceBodyEdgeBlockCounts(doc)
		passed := make([]bool, len(doc.Blocks))
		// Compute against the unchanged generation before replacing any block;
		// sibling standalone owners must not gain uniqueness through isolation.
		for i, block := range doc.Blocks {
			if block.Kind == types.BlockDiagram && block.Diagram != nil {
				passed[i] = block.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown || recoveredDiagramChecked(ctx, doc, i, view, pctx, counts)
			}
		}
		for i := range doc.Blocks {
			block := &doc.Blocks[i]
			if block.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown {
				continue // private producer marker; never reconstructed from JSON
			}
			if block.Kind == types.BlockDiagram && block.Diagram != nil && !passed[i] {
				*block = types.AnswerBlock{ID: block.ID, Kind: types.BlockCaveat, Text: RecoveryDiagramIsolationNotice(language)}
				isolated++
				continue
			}
			clean := func(s *string) { var n int; *s, n = IsolateUnvalidatedDiagramFences(*s, language); isolated += n }
			clean(&block.Title)
			clean(&block.Text)
			for j := range block.Columns {
				clean(&block.Columns[j])
			}
			for j := range block.Items {
				clean(&block.Items[j].Label)
				clean(&block.Items[j].Text)
				for k := range block.Items[j].Cells {
					clean(&block.Items[j].Cells[k])
				}
			}
		}
		for i := range doc.Caveats {
			var n int
			doc.Caveats[i], n = IsolateUnvalidatedDiagramFences(doc.Caveats[i], language)
			isolated += n
		}
	}
	kept := make([]types.AnswerDisplayAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Kind == types.AnswerDisplayAttachmentDiagram {
			// Even an exact duplicate of a checked graph need not be appended:
			// its structured original is already rendered with all its owners.
			isolated++
			continue
		}
		var n int
		attachment.Body, n = IsolateUnvalidatedDiagramFences(attachment.Body, language)
		isolated += n
		kept = append(kept, attachment)
	}
	if doc != nil && isolated > 0 {
		doc.Caveats = append(doc.Caveats, RecoveryDiagramIsolationNotice(language))
	}
	return kept, isolated
}

func recoveredDiagramChecked(ctx *types.BusContext, original *types.AnswerDocumentV2, index int, view *types.AnswerSemanticView, pctx *preEmitCheckContext, counts map[string]int) bool {
	if ctx == nil || ctx.AnalysisIR == nil || view == nil {
		return false
	}
	block := original.Blocks[index]
	block.EdgeAnchors = diagramEvidenceEffectiveAnchorsForBlock(original, index, counts)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{block}}
	owned, temporalHints := preCheckReportLocalRuntimeTemporalDiagramAuthority(doc, pctx)
	if len(temporalHints) > 0 {
		return false
	}
	if len(owned) == 0 {
		if view.Family == types.QFRootCauseTrace && !types.RuntimeRelationDiagramUsesNativeAuthority(&ctx.AnalysisIR.RequestModel) {
			return false
		}
		// Ordinary optional diagrams may intentionally bypass source authority
		// during normal emit. In recovery that is not a positive proof receipt.
		edges := mermaidcompat.ParseEdges(block.Diagram.Body)
		if len(edges) == 0 {
			return false
		}
		owners := diagramTypedAnchorRelationSet(block.EdgeAnchors)
		replies := diagramSequenceStructuralReplyIndexSet(block.Diagram.Kind, edges, owners)
		for i, edge := range edges {
			if replies[i] {
				continue
			}
			if len(owners[diagramEvidenceEdgeKey(edge.From, edge.To)]) == 0 {
				return false
			}
		}
	}
	if len(preCheckDiagramVisibleLabelConsistency(doc, view, pctx)) > 0 || len(preCheckDiagramNoArrowOwnershipDirection(doc, pctx)) > 0 || validateModelAuthoredAnswerRelationClaims(ctx, doc) != nil {
		return false
	}
	evidence := DiagramEvidenceForValidation(ctx, doc, view, nil)
	for _, mismatch := range DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, view, evidence) {
		// A missing requested spine is a completeness failure, not evidence
		// that a smaller already-proved supporting relationship is false.
		if mismatch.Issue != diagramRequestedStageSpineIncomplete {
			return false
		}
	}
	for _, mismatch := range DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, original, view, evidence) {
		if mismatch.BlockID != block.ID {
			continue
		}
		switch mismatch.Issue {
		case DiagramParticipantCoverageMissingBoundary, DiagramParticipantCoverageTypedEdgeMissing,
			DiagramParticipantCoverageIdentityMissing, DiagramParticipantCoverageComponentSplit:
			continue // requested coverage remains degraded; not a local truth grant
		default:
			return false
		}
	}
	return true
}

func RecoveryDiagramIsolationNotice(language string) string {
	if strings.HasPrefix(strings.ToLower(language), "zh") {
		return "恢复稿中的部分图未通过当前证据核验，已暂不展示；已核验的图和原始量测事实保留，失败原稿仍保留供审计。"
	}
	return "Some recovered diagrams could not be verified against current evidence and are not displayed. Verified diagrams and measured facts are preserved; the failed draft remains available for audit."
}
