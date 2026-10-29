package types

import (
	"crypto/sha256"
	"encoding/json"
)

// This receipt follows the private snapshot pattern used for native facts.
// It grants no identity, edge, causal, or completion authority. Only the
// emitter, after checking the current native relation pool, may publish it.
type nativeDiagramCoverageReceipt struct {
	snapshot  [sha256.Size]byte
	unmapped  []string
	relations int
}

func cloneNativeDiagramCoverageReceipt(in *nativeDiagramCoverageReceipt) *nativeDiagramCoverageReceipt {
	if in == nil {
		return nil
	}
	out := *in
	out.unmapped = append([]string(nil), in.unmapped...)
	return &out
}

func nativeDiagramCoverageSnapshot(block AnswerBlock) [sha256.Size]byte {
	data, _ := json.Marshal([]any{block.ID, block.Kind, block.Diagram, block.EdgeAnchors,
		block.ParticipantBoundaries, block.RequestedRelationScope})
	return sha256.Sum256(data)
}

// BindNativeDiagramCoverage records a current producer-validated display
// distinction. Call with no participants to revoke a previous receipt.
func (block *AnswerBlock) BindNativeDiagramCoverage(unmapped []string, relations int) {
	if block == nil {
		return
	}
	block.nativeDiagramCoverage = nil
	if block.Kind != BlockDiagram || block.Diagram == nil || relations <= 0 || len(unmapped) == 0 {
		return
	}
	available := map[string]bool{}
	for _, boundary := range block.ParticipantBoundaries {
		if boundary.Status == DiagramParticipantBoundaryUnproven {
			available[boundary.Participant] = true
		}
	}
	seen := map[string]bool{}
	for _, participant := range unmapped {
		if !available[participant] || seen[participant] {
			return
		}
		seen[participant] = true
	}
	block.nativeDiagramCoverage = &nativeDiagramCoverageReceipt{
		snapshot: nativeDiagramCoverageSnapshot(*block),
		unmapped: append([]string(nil), unmapped...), relations: relations,
	}
}

// NativeDiagramCoverage returns copies only when the exact diagram, endpoint
// anchors and boundary generation remain unchanged. JSON restoration cannot
// recreate this private receipt; a fresh emit must revalidate it.
func (block AnswerBlock) NativeDiagramCoverage() (unmapped []string, relations int) {
	r := block.nativeDiagramCoverage
	if r == nil || r.snapshot != nativeDiagramCoverageSnapshot(block) {
		return nil, 0
	}
	return append([]string(nil), r.unmapped...), r.relations
}
