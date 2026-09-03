package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// acceptedClosureHasActiveExploreContractBacktrack distinguishes a fresh,
// typed contract repair from advisory retry carry-over left by an already
// accepted investigation. The retry plan is the closed authority; rendered
// violation prose is never parsed here.
//
// Generation binding (§40.14 V7-2): RetryState is a cross-generation
// carrier — it is populated once per finalize failure and kept for the
// whole retry chain — so the arm additionally requires proof that the
// carrier and the decision it arbitrates belong to the same generation:
//
//   - the state was bound by an explore backtrack
//     (ExploreBacktrackEpoch != 0) and that backtrack opened the CURRENT
//     epoch (== mut.ExploreBacktrackEpoch()); an unbound state (finalizer
//     -only / extract fallback, downgraded explore owner) never re-opened
//     exploration and never vetoes;
//   - no completion has been decided since the backtrack
//     (CompletionGenerationAtBacktrack == mut.InvestigationCompleteGeneration()):
//     the explorer's fresh emit_investigation_complete — or a typed
//     system closure — consumes the veto. One backtrack vetoes exactly
//     once; a later finalize failure re-populates and re-binds.
//
// Every operand is a single integer or a typed enum string: precise
// signals only, per the hard-gate red line. The reads are census-bound
// by hard_arm_mutable_carrier_census_test.go.
func acceptedClosureHasActiveExploreContractBacktrack(mut *types.MutableState) bool {
	if mut == nil {
		return false
	}
	retry := mut.RetryState()
	return retry != nil &&
		len(retry.ActiveViolations) > 0 &&
		strings.TrimSpace(retry.LastPrimaryOwner) == string(LocusExplore) &&
		retry.ExploreBacktrackEpoch != 0 &&
		retry.ExploreBacktrackEpoch == mut.ExploreBacktrackEpoch() &&
		retry.CompletionGenerationAtBacktrack == mut.InvestigationCompleteGeneration()
}
