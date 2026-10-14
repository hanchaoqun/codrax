package agent

import "github.com/hanchaoqun/codrax/internal/types"

// Read the producer's precise current capability, never the error prose. A
// bare order diagnostic or a note without an exact edge keeps its old fallback.
func answerDocNativeOrderHasLocalCandidate(f types.AnswerDiagramRelationRepairFailure, candidates []types.AnswerDiagramRelationRepairCandidate) bool {
	if f.Issue != types.DiagramRelationFailureNativeSequenceEventOrder || f.FailureRef == "" ||
		f.TargetCarrier != types.AnswerDiagramRelationRepairCarrierPriorAnchor || !f.AllowsAction("remove") || f.BodyOccurrence < 1 {
		return false
	}
	for _, c := range candidates {
		if c.AdditionRef != "" && c.BlockID == f.BlockID && c.RelationKind == f.RelationKind && c.FromIdentity == f.FromIdentity && c.ToIdentity == f.ToIdentity {
			return true
		}
	}
	return false
}
