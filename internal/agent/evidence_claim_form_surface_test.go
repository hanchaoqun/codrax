package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func conditionWithBodyAssignmentEvidence() types.EvidenceItem {
	return types.EvidenceItem{
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
}

func TestAnswerDocConditionRelationSurfaceUsesGuardClaimForm(t *testing.T) {
	row, ok := answerDocRelationSurfaceRowForEvidence(nil, conditionWithBodyAssignmentEvidence(), 0)
	if !ok {
		t.Fatal("condition evidence should retain a typed guard handoff")
	}
	if row.role != "guard_condition" || row.label != "Changed" {
		t.Fatalf("condition relation row = %+v, want guard-only role/label", row)
	}
	if strings.Contains(row.surface, "assigns") || strings.Contains(row.surface, "flagMaxSteps = mergedMaxSteps") {
		t.Fatalf("condition relation row leaked body assignment: %+v", row)
	}
}

func TestFormatExactResolutionSeedConditionDoesNotUseAssignmentTriple(t *testing.T) {
	got := formatExactResolutionSeed(conditionWithBodyAssignmentEvidence())
	if !strings.Contains(got, "guard condition IF") || !strings.Contains(got, "Changed") {
		t.Fatalf("exact-resolution seed lost typed guard: %q", got)
	}
	if strings.Contains(got, "assigns") || strings.Contains(got, "flagMaxSteps = mergedMaxSteps") {
		t.Fatalf("exact-resolution seed leaked body assignment: %q", got)
	}
}
