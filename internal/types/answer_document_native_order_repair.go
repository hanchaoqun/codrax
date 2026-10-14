package types

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
)

// The ordering producer already proved this event. A local movement may own
// only one exact current anchor and its matching same-pair body occurrence.
// This is a display capability, not proof of an event or its timestamp.
func answerDiagramNativeOrderExactCarrier(base *AnswerDocumentV2, f AnswerDiagramRelationRepairFailure) bool {
	if base == nil || f.Issue != DiagramRelationFailureNativeSequenceEventOrder || f.BodyOccurrence < 1 ||
		!f.RelationKind.IsValid() || !strings.HasPrefix(f.FromIdentity, "runtime_instance_") || !strings.HasPrefix(f.ToIdentity, "runtime_instance_") ||
		len(AnswerDiagramRelationRepairFailureBaseAnchorCandidates(base, f)) != 1 {
		return false
	}
	count, matched := 0, false
	for _, b := range base.Blocks {
		if b.ID != f.BlockID {
			continue
		}
		count++
		if b.Kind != BlockDiagram || b.Diagram == nil || b.Diagram.Kind != DiagramSequence || len(b.Diagram.RuntimeNotesV1) != 0 {
			return false
		}
		bodyCount := 0
		for _, edge := range mermaidcompat.ParseEdges(b.Diagram.Body) {
			if edge.From == f.FromNode && edge.To == f.ToNode {
				bodyCount++
			}
		}
		occurrence := 0
		for _, a := range b.EdgeAnchors {
			if a.FromNode != f.FromNode || a.ToNode != f.ToNode {
				continue
			}
			occurrence++
			if occurrence == f.BodyOccurrence && a.RelationKind == f.RelationKind && a.FromIdentity == f.FromIdentity && a.ToIdentity == f.ToIdentity {
				matched = true
			}
		}
		if bodyCount != occurrence || bodyCount < f.BodyOccurrence {
			return false
		}
	}
	return count == 1 && matched
}

// A re-placement selector is scoped to both current anchor metadata and the
// exact statement bytes used to mint its body occurrence/placement choices.
func AnswerDiagramNativeOrderFailureMatchesBase(base *AnswerDocumentV2, f AnswerDiagramRelationRepairFailure) bool {
	return answerDiagramNativeOrderExactCarrier(base, f) && f.FailureRef != "" && f.FailureRef == answerDiagramRelationRepairFailureRef(base, f)
}

func answerDiagramNativeOrderReinsertion(base *AnswerDocumentV2, failures []AnswerDiagramRelationRepairFailure, c AnswerDiagramRelationRepairCandidate) bool {
	for _, f := range failures {
		if answerDiagramNativeOrderExactCarrier(base, f) && c.BlockID == f.BlockID && c.RelationKind == f.RelationKind &&
			c.FromIdentity == f.FromIdentity && c.ToIdentity == f.ToIdentity {
			return true
		}
	}
	return false
}

// Remove+add is one transaction. An ordering repair must preserve the exact
// event population; deleting an inconvenient proved event is not a repair.
func answerDiagramNativeOrderPopulationViolations(lease *AnswerDiagramRelationRepairLease, result *AnswerDocumentV2) []AnswerDiagramRelationRepairScopeViolation {
	var out []AnswerDiagramRelationRepairScopeViolation
	for _, f := range lease.Failures {
		if f.Issue != DiagramRelationFailureNativeSequenceEventOrder {
			continue
		}
		count := func(anchors []DiagramEdgeAnchor) int {
			n := 0
			for _, a := range anchors {
				if a.RelationKind == f.RelationKind && a.FromIdentity == f.FromIdentity && a.ToIdentity == f.ToIdentity {
					n++
				}
			}
			return n
		}
		before, after, present := 0, 0, false
		for _, b := range lease.Blocks {
			if b.BlockID == f.BlockID {
				before += count(b.BaseAnchors)
			}
		}
		for _, b := range result.Blocks {
			if b.ID == f.BlockID {
				present = true
				after += count(b.EdgeAnchors)
			}
		}
		if !present && lease.AllowTargetDiagramRemoval {
			continue // Keep the pre-existing explicit optional-diagram removal contract.
		}
		if before != after {
			out = append(out, AnswerDiagramRelationRepairScopeViolation{BlockID: f.BlockID, Issue: "native_order_event_population_changed", FromNode: f.FromNode, ToNode: f.ToNode})
		}
	}
	return out
}
