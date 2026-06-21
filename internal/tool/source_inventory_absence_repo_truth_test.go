package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAbsenceRepoTruthSeed_ClosesChickenAndEgg pins the RNE-C61 root fix: when
// an active source-inventory profile declares absence but the model never ran
// the lens (observation has NO SourceClasses), the raw typed gate self-no-ops
// (blocked=false), but the repo-truth wrapper seeds the universe from the real
// git-tracked file walk and the gate then BLOCKS (demanding the lens).
func TestAbsenceRepoTruthSeed_ClosesChickenAndEgg(t *testing.T) {
	profile := &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
	}
	empty := types.SourceInventoryObservation{} // never-ran-lens: no SourceClasses, no Sets

	// Raw typed gate self-no-ops on the empty universe (the chicken-and-egg).
	if _, blocked := types.SourceInventoryExactAbsenceNeedsInventoryProof(profile, empty); blocked {
		t.Fatal("raw gate should NOT block when SourceClasses is empty (documents the chicken-and-egg)")
	}

	// Wrapper seeds repo-truth from the real repo and the gate now blocks.
	ctx := &types.BusContext{RepoRoot: "../..", AnalysisIR: &types.AnalysisIR{}}
	summary, blocked := SourceInventoryExactAbsenceNeedsInventoryProofRepoTruth(ctx, profile, empty)
	if !blocked {
		t.Fatalf("repo-truth wrapper should block a no-lens absence; summary=%q", summary)
	}
	if summary == "" {
		t.Fatal("expected a non-empty source_classes summary from the seeded universe")
	}

	// Inactive profile / nil ctx must remain inert (no extra steer, no walk).
	if _, blocked := SourceInventoryExactAbsenceNeedsInventoryProofRepoTruth(nil, profile, empty); blocked {
		t.Fatal("nil ctx must not seed or block")
	}
	inactive := &types.SourceInventoryProfile{IsSourceInventory: false}
	if _, blocked := SourceInventoryExactAbsenceNeedsInventoryProofRepoTruth(ctx, inactive, empty); blocked {
		t.Fatal("inactive profile must not block")
	}
}

// TestAbsenceRepoTruthSeed_NoOpWhenModelUniversePresent confirms the seed is a
// no-op when the model already populated SourceClasses (lens ran): the wrapper
// must defer entirely to the typed gate, not re-walk or alter the result.
func TestAbsenceRepoTruthSeed_NoOpWhenModelUniversePresent(t *testing.T) {
	profile := &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
	}
	// Model ran the lens and proved a complete zero set for the function role,
	// with a complete production-only universe — a legitimate absence.
	obs := types.SourceInventoryObservation{
		Active:        true,
		Complete:      true,
		SourceClasses: []types.SourceInventorySourceClassCount{{Role: types.SourcePathRoleProduction, Count: 3, Complete: true}},
		Sets:          []types.SourceInventoryObservationSet{{Role: types.AnswerCandidateRoleFunction, Complete: true, Count: 0}},
	}
	ctx := &types.BusContext{RepoRoot: "../.."}
	rawSummary, rawBlocked := types.SourceInventoryExactAbsenceNeedsInventoryProof(profile, obs)
	wrapSummary, wrapBlocked := SourceInventoryExactAbsenceNeedsInventoryProofRepoTruth(ctx, profile, obs)
	if rawBlocked != wrapBlocked || rawSummary != wrapSummary {
		t.Fatalf("wrapper must defer to typed gate when SourceClasses present: raw=(%q,%v) wrap=(%q,%v)",
			rawSummary, rawBlocked, wrapSummary, wrapBlocked)
	}
}
