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
		attachment.Title, n = IsolateUnvalidatedDiagramFences(attachment.Title, language)
		isolated += n
		attachment.Body, n = IsolateUnvalidatedDiagramFences(attachment.Body, language)
		isolated += n
		if n > 0 {
			attachment.Hash = answerDisplayAttachmentHash(attachment.Kind, attachment.Language, attachment.Body)
		}
		kept = append(kept, attachment)
	}
	if doc != nil && isolated > 0 {
		doc.Caveats = append(doc.Caveats, RecoveryDiagramIsolationNotice(language))
	}
	return kept, isolated
}

// IsolateAnswerDisplayAttachmentsForRender checks only unstructured display
// carriers, never the accepted document. Preserve a disclosure in the cleaned
// carrier so another render cannot silently lose it after the graph is gone.
func IsolateAnswerDisplayAttachmentsForRender(ctx *types.BusContext, attachments []types.AnswerDisplayAttachment, language string) []types.AnswerDisplayAttachment {
	kept, isolated := IsolateRecoveredAnswerDiagrams(ctx, nil, attachments, language)
	if isolated == 0 {
		return kept
	}
	notice := RecoveryDiagramIsolationNotice(language)
	// A preserved model reference can later be discarded by the accepted-doc
	// filter. Keep its generated disclosure on the separate prose-only lane.
	for i := range kept {
		if kept[i].Source == types.AnswerDisplayAttachmentSourceDiagramIsolation && kept[i].Kind == types.AnswerDisplayAttachmentText && kept[i].Body == notice {
			return kept
		}
	}
	for i := range kept {
		kept[i].Title = strings.ReplaceAll(kept[i].Title, notice, "")
		if strings.Contains(kept[i].Body, notice) {
			kept[i].Body = strings.ReplaceAll(kept[i].Body, notice, "")
			kept[i].Hash = answerDisplayAttachmentHash(kept[i].Kind, kept[i].Language, kept[i].Body)
		}
	}
	title := "Diagram display notice"
	if strings.HasPrefix(strings.ToLower(language), "zh") {
		title = "图示展示说明"
	}
	return append(kept, types.AnswerDisplayAttachment{
		Kind: types.AnswerDisplayAttachmentText, Title: title, Body: notice,
		Source: types.AnswerDisplayAttachmentSourceDiagramIsolation,
	})
}

// DeduplicateDiagramIsolationNotice only coalesces the exact system disclosure
// on an assembled display. It neither validates nor changes a document carrier.
func DeduplicateDiagramIsolationNotice(text, language string) string {
	notice := RecoveryDiagramIsolationNotice(language)
	if first := strings.Index(text, notice); first >= 0 {
		end := first + len(notice)
		return text[:end] + strings.ReplaceAll(text[end:], notice, "")
	}
	return text
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
