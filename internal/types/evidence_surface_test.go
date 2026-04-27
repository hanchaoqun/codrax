package types

import "testing"

func TestExactResolutionSurfaceEvidencePool_IncludesAnswerChainEvidence(t *testing.T) {
	emitted := []EvidenceItem{{
		Kind:            EvidenceDirect,
		Source:          "internal/config/runtime.go",
		LineStart:       32,
		AnchorKind:      AnchorDefinition,
		AnchorSymbol:    "RuntimeSettings",
		ContextRole:     EvidenceContextRoleAbsenceSupport,
		GroundingStatus: GroundingGrounded,
	}}
	chains := []AnswerChain{{
		Item: EvidenceItem{
			Kind:            EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			AnchorKind:      AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     EvidenceContextRoleRelatedContext,
			DiagramRole:     EvidenceDiagramRoleDefault,
			GroundingStatus: GroundingGrounded,
		},
	}}

	pool := ExactResolutionSurfaceEvidencePool(emitted, nil, chains)
	if len(pool) != 2 {
		t.Fatalf("pool len = %d, want 2", len(pool))
	}
	var sawDefault bool
	for _, item := range pool {
		if item.Source == "internal/types/config.go" && item.LineStart == 707 && item.DiagramRole == EvidenceDiagramRoleDefault {
			sawDefault = true
		}
	}
	if !sawDefault {
		t.Fatalf("pool missing answer-chain default anchor: %+v", pool)
	}
}
