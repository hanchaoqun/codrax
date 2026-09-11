package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

func atomicDiagramPostEditDependencyHint() string {
	return "The exact model-authored relation edits were applied to an unpublished retry base. Surviving relation metadata or sequence replies lost their visible/structural owner in that exact graph. The old edge refs are consumed. Use only the new failure_ref/action branches to choose each dependent relation repair; do not replay old operations. Cleanup rows marked decision_optional may be omitted without another disposition retry, or selected together with the metadata removals. The system chooses no edge, participant disposition, direction, label, layout, or conclusion."
}

// atomicDiagramPostEditMetadataDependencies follows only actually selected
// local removals. It reuses the ordinary typed mismatch detector on affected
// blocks; it does not inventory disconnected declarations across the answer.
func atomicDiagramPostEditMetadataDependencies(previous, staged *types.AnswerDocumentV2, source *types.AnswerDiagramRelationRepairLease, view *types.AnswerSemanticView, edits []emitAnswerDiagramEdgeEdit) []types.AnswerDiagramRelationRepairFailure {
	if previous == nil || staged == nil || source == nil || len(edits) == 0 || (view != nil && view.Family == types.QFRootCauseTrace) {
		return nil
	}
	beforeIndexes, beforeUnique := atomicDiagramUniqueBlockIndexes(previous)
	_, afterUnique := atomicDiagramUniqueBlockIndexes(staged)
	selectedPairs := make(map[string]bool)
	affectedBlocks := make(map[string]bool)
	selectedLiveRemoval := false
	for _, edit := range edits {
		if strings.TrimSpace(edit.Action) != "remove" || edit.FailureRef == "" {
			continue
		}
		for _, failure := range source.Failures {
			if failure.FailureRef != edit.FailureRef || !failure.AllowsAction("remove") || !beforeUnique[failure.BlockID] || !afterUnique[failure.BlockID] {
				continue
			}
			before := previous.Blocks[beforeIndexes[failure.BlockID]]
			if before.Diagram == nil {
				continue
			}
			selectedLiveRemoval = true
			visibleSource := false
			var matching []mermaidcompat.Edge
			for _, edge := range mermaidcompat.ParseEdges(before.Diagram.Body) {
				if edge.From == failure.FromNode && edge.To == failure.ToNode {
					matching = append(matching, edge)
				}
			}
			for index := range matching {
				if failure.CanRemoveVisibleBodyOccurrence(failure.BlockID, failure.FromNode, failure.ToNode, index+1, len(matching)) {
					visibleSource = true
				}
			}
			if visibleSource {
				selectedPairs[failure.BlockID+"\x00"+failure.FromNode+"\x00"+failure.ToNode] = true
				affectedBlocks[failure.BlockID] = true
				continue
			}
			// A partially cleaned metadata generation may continue only its
			// already-bound provenance, never restart from a metadata-only row.
			for _, candidate := range source.OptionalOrphanCleanups {
				if candidate.BlockID == failure.BlockID &&
					(candidate.ParticipantID == failure.FromNode || candidate.ParticipantID == failure.ToNode) &&
					types.AnswerDiagramOrphanMetadataDependencyMatchesBase(previous, candidate, source) &&
					types.AnswerDiagramRelationRepairOccurrenceMatchesBase(previous, failure) {
					affectedBlocks[failure.BlockID] = true
					for _, anchor := range before.EdgeAnchors {
						if anchor.FromNode == candidate.ParticipantID || anchor.ToNode == candidate.ParticipantID {
							selectedPairs[failure.BlockID+"\x00"+anchor.FromNode+"\x00"+anchor.ToNode] = true
						}
					}
				}
			}
		}
	}
	// Preserve every still-open bound metadata dependency when a batch removes
	// only a subset. A different candidate's successful removal cannot erase
	// another candidate's source. These are installed receipts, not new nodes.
	if selectedLiveRemoval {
		for _, candidate := range source.OptionalOrphanCleanups {
			if !beforeUnique[candidate.BlockID] || !afterUnique[candidate.BlockID] ||
				!types.AnswerDiagramOrphanMetadataDependencyMatchesBase(previous, candidate, source) {
				continue
			}
			affectedBlocks[candidate.BlockID] = true
			for _, anchor := range previous.Blocks[beforeIndexes[candidate.BlockID]].EdgeAnchors {
				if anchor.FromNode == candidate.ParticipantID || anchor.ToNode == candidate.ParticipantID {
					selectedPairs[candidate.BlockID+"\x00"+anchor.FromNode+"\x00"+anchor.ToNode] = true
				}
			}
		}
	}
	if view == nil {
		view = &types.AnswerSemanticView{}
	}
	var failures []types.AnswerDiagramRelationRepairFailure
	for _, block := range staged.Blocks {
		if !affectedBlocks[block.ID] {
			continue
		}
		local := &types.AnswerDocumentV2{DocumentModel: staged.DocumentModel, Blocks: []types.AnswerBlock{block}}
		for _, mismatch := range DiagramCallEdgeEvidenceMismatches(local, view, nil) {
			if !types.AnswerDiagramRelationRepairIssueHasAnchorOccurrence(mismatch.Issue) || mismatch.AnchorOccurrence < 1 ||
				!selectedPairs[block.ID+"\x00"+mismatch.FromNode+"\x00"+mismatch.ToNode] {
				continue
			}
			failures = append(failures, types.AnswerDiagramRelationRepairFailure{
				BlockID: block.ID, Issue: mismatch.Issue, FromNode: mismatch.FromNode, ToNode: mismatch.ToNode,
				FromIdentity: mismatch.FromSymbol, ToIdentity: mismatch.ToSymbol,
				RelationKind: mismatch.Relation, AnchorOccurrence: mismatch.AnchorOccurrence,
			})
		}
	}
	return failures
}

func atomicDiagramCarryMetadataOrphanCandidates(previous, staged *types.AnswerDocumentV2, source, dependency *types.AnswerDiagramRelationRepairLease, edits []emitAnswerDiagramEdgeEdit) []types.AnswerDiagramOrphanCleanupCandidate {
	if previous == nil || staged == nil || source == nil || dependency == nil {
		return nil
	}
	beforeIndexes, beforeUnique := atomicDiagramUniqueBlockIndexes(previous)
	afterIndexes, afterUnique := atomicDiagramUniqueBlockIndexes(staged)
	selected := make(map[string]bool)
	for _, edit := range edits {
		if strings.TrimSpace(edit.Action) == "remove" && edit.FailureRef != "" {
			selected[edit.FailureRef] = true
		}
	}
	var out []types.AnswerDiagramOrphanCleanupCandidate
	for _, candidate := range source.OptionalOrphanCleanups {
		if !beforeUnique[candidate.BlockID] || !afterUnique[candidate.BlockID] {
			continue
		}
		before, after := previous.Blocks[beforeIndexes[candidate.BlockID]], staged.Blocks[afterIndexes[candidate.BlockID]]
		if before.Diagram == nil || after.Diagram == nil {
			continue
		}
		beforeDecl, beforeCount := atomicDiagramUniqueRemovableDeclaration(before.Diagram.Body, candidate.ParticipantID)
		afterDecl, afterCount := atomicDiagramUniqueRemovableDeclaration(after.Diagram.Body, candidate.ParticipantID)
		if beforeCount != 1 || afterCount != 1 || beforeDecl != afterDecl {
			continue
		}
		proven := types.AnswerDiagramOrphanMetadataDependencyMatchesBase(previous, candidate, source)
		if !proven && candidate.MetadataDependency == nil {
			incident, removable := atomicDiagramBaseIncidentEdgesAreRemoveCapableFailures(before, candidate.ParticipantID, source)
			if incident > 0 && removable {
				for _, failure := range source.Failures {
					if selected[failure.FailureRef] && failure.BlockID == candidate.BlockID &&
						(failure.FromNode == candidate.ParticipantID || failure.ToNode == candidate.ParticipantID) &&
						(failure.TargetCarrier == types.AnswerDiagramRelationRepairCarrierVisibleBodyEdge || failure.TargetCarrier == types.AnswerDiagramRelationRepairCarrierPriorAnchor) {
						proven = true
					}
				}
			}
		}
		if proven {
			if bound, ok := types.BindAnswerDiagramOrphanMetadataDependency(staged, candidate, dependency); ok {
				out = append(out, bound)
			}
		}
	}
	return out
}
