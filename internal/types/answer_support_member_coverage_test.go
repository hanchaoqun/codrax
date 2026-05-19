package types

import "testing"

func TestAnswerSupportMemberObligationStableHandoffKeys(t *testing.T) {
	ob := AnswerSupportMemberObligation{
		Label:    "StageAnalyze",
		Location: "internal/types/context.go:42",
		EquivalentLocations: []string{
			"internal/types/context.go:43",
			"internal/types/context.go:44",
		},
	}
	if got, want := ob.StableItemID(), "support-stageanalyze"; got != want {
		t.Fatalf("StableItemID = %q, want %q", got, want)
	}
	if got, want := ob.StableCitationKey(), "internal/types/context.go:42"; got != want {
		t.Fatalf("StableCitationKey = %q, want %q", got, want)
	}
}

func TestAnswerSupportMemberObligationStableItemIDFallsBackToLocation(t *testing.T) {
	ob := AnswerSupportMemberObligation{
		Location: "internal/analysis/criterion/eval.go:17",
	}
	if got, want := ob.StableItemID(), "support-internal_analysis_criterion_eval_go_17"; got != want {
		t.Fatalf("StableItemID fallback = %q, want %q", got, want)
	}
}
