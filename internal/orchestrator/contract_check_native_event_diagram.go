package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

// A receipt-only native chart displays facts, not source/causal relations.
// Its presence must neither satisfy a required relation diagram nor hide an
// ordinary sibling from that diagram's existing validators.
func isBoundNativeEventFactDiagram(block types.AnswerBlock) bool {
	ref := block.RuntimeMeasurement
	return block.Kind == types.BlockDiagram && ref.IsBound() &&
		(ref.EventSelection != nil && ref.BoundTable.EventData != nil || ref.BoundTable.HasNativeSamplingFacts()) &&
		block.Diagram == nil && block.Text == "" &&
		len(block.Items) == 0 && len(block.Columns) == 0 && block.RuntimeWorkRelation == nil &&
		len(block.EdgeAnchors) == 0 && len(block.ParticipantBoundaries) == 0 &&
		len(block.RelationClaims) == 0 && len(block.ClaimUses) == 0 &&
		(block.TraceCausalClaimCaliber == "" || block.TraceCausalClaimCaliber == types.TraceCausalClaimNoConclusion) && block.RequestedRelationScope == ""
}
