package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFormatRelationDossierEvidenceConditionDoesNotBecomeBodyRelation(t *testing.T) {
	item := types.EvidenceItem{
		Kind:            types.EvidenceRelationship,
		Scope:           types.ScopeLine,
		Subject:         "flagMaxSteps",
		Predicate:       "assigns",
		Object:          "mergedMaxSteps",
		Condition:       `!cmd.Flags().Changed("pipeline-max-steps")`,
		Source:          "cmd/root.go",
		LineStart:       2664,
		AnchorKind:      types.AnchorCondition,
		AnchorSymbol:    "Changed",
		Snippet:         "if !cmd.Flags().Changed(\"pipeline-max-steps\") {\n\tflagMaxSteps = mergedMaxSteps\n}",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}

	got := formatRelationDossierEvidence([]types.EvidenceItem{item})
	if !strings.Contains(got, "verified guard") || !strings.Contains(got, "guard condition IF") {
		t.Fatalf("condition dossier lost typed guard: %q", got)
	}
	if strings.Contains(got, "flagMaxSteps -> mergedMaxSteps") || strings.Contains(got, "relation=assigns") {
		t.Fatalf("condition dossier leaked body relation: %q", got)
	}
}
