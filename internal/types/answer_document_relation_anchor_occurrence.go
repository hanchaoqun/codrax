package types

import "strings"

// AnswerDiagramRelationRepairIssueHasAnchorOccurrence identifies only the
// two metadata-stale diagnostics that select no visible body occurrence.
func AnswerDiagramRelationRepairIssueHasAnchorOccurrence(issue string) bool {
	return issue == "typed_anchor_without_visible_edge" || issue == "typed_anchor_reversed_against_visible_edge"
}

// AnswerDiagramRelationRepairOccurrenceMatchesBase revalidates a system-owned
// metadata selector against the current rejected draft before any edits run.
// Legacy refs keep their existing selection lane. The snapshot is opaque;
// model wording is neither interpreted nor turned into evidence authority.
func AnswerDiagramRelationRepairOccurrenceMatchesBase(base *AnswerDocumentV2, failure AnswerDiagramRelationRepairFailure) bool {
	return failure.AnchorOccurrence > 0 && failure.FailureRef != "" &&
		len(AnswerDiagramRelationRepairFailureBaseAnchorCandidates(base, failure)) == 1 &&
		failure.FailureRef == answerDiagramRelationRepairFailureRef(base, failure)
}

// A metadata occurrence authorizes at most one removal, and only an explicit
// replace capability can contribute replacement budget. Legacy failures keep
// their pre-existing relation scope checks.
func answerDiagramRelationOccurrenceRemovalAllowed(blockID, key string, missing int, base []DiagramEdgeAnchor, failures []AnswerDiagramRelationRepairFailure) bool {
	seen := make(map[int]bool)
	for _, failure := range failures {
		if strings.TrimSpace(failure.BlockID) != blockID {
			continue
		}
		if failure.AnchorOccurrence == 0 {
			if anchor, ok := answerDiagramRelationAnchorByKey(base, key); ok && answerDiagramRelationFailureMatchesAnchor(failure, anchor) {
				return true
			}
			continue
		}
		matches := AnswerDiagramRelationRepairFailureAnchorCandidates(failure, base)
		if len(matches) == 1 && answerDiagramRelationAnchorSemanticKey(matches[0]) == key && failure.AllowsAction(string(AnswerDiagramRelationRepairActionRemove)) {
			seen[failure.AnchorOccurrence] = true
		}
	}
	return missing <= len(seen)
}
