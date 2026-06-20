package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// #2: the loop gate must consume the typed source-class-universe absence signal
// (the finalize-gate twin) so a source-family exact absence cannot close
// mid-loop while the class universe is open, AND must declare repo_map as a
// required tool (RNE-C61) so the restricted completion surface exposes it.
func TestSourceInventoryClassUniverseAbsenceDowngrade(t *testing.T) {
	newCtx := func() *types.BusContext {
		mut := types.NewMutableState("how many .ets sources?")
		mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
			Active:       true,
			AdvisoryOnly: true,
			Complete:     true,
			Lens:         []string{"source_class_universe", "count"},
			SourceClasses: []types.SourceInventorySourceClassCount{{
				Role:     types.SourcePathRoleThirdParty,
				Count:    1, // a repo-owned thirdparty/corpus source exists
				Complete: true,
			}},
		})
		return &types.BusContext{
			Mutable: mut,
			AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				SourceInventoryProfile: &types.SourceInventoryProfile{
					IsSourceInventory: true,
					TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				},
			}},
		}
	}

	// resolved result_kind: gate is inert (only absence is gated).
	if got := sourceInventoryClassUniverseAbsenceDowngrade(newCtx(), "resolved"); got != "" {
		t.Fatalf("resolved must not trigger the absence gate, got %q", got)
	}

	// absence + open class universe: gate fires AND raises a repo_map repair.
	ctx := newCtx()
	got := sourceInventoryClassUniverseAbsenceDowngrade(ctx, "absence")
	if got == "" {
		t.Fatal("absence declared while the source-class universe is open must downgrade")
	}
	repairsExposeRepoMap := false
	for _, r := range ctx.Mutable.EvidenceClosure().ActiveRepairs() {
		for _, tool := range types.RepairDirectiveRequiredTools(r) {
			if tool == "repo_map" {
				repairsExposeRepoMap = true
			}
		}
	}
	if !repairsExposeRepoMap {
		t.Fatal("RNE-C61: the absence downgrade must declare repo_map as a required tool so the restricted surface exposes it")
	}

	// nil-safety.
	if sourceInventoryClassUniverseAbsenceDowngrade(nil, "absence") != "" {
		t.Fatal("nil ctx must be inert")
	}

	// subtype-inert: a complete zero member set for the requested role is a
	// genuine bounded absence -> the predicate clears, gate does not fire.
	zeroCtx := newCtx()
	obs := zeroCtx.Mutable.SourceInventoryObservation()
	obs.Sets = []types.SourceInventoryObservationSet{{Role: types.AnswerCandidateRoleFunction, Complete: true, Count: 0}}
	zeroCtx.Mutable.SetSourceInventoryObservation(obs)
	if got := sourceInventoryClassUniverseAbsenceDowngrade(zeroCtx, "absence"); got != "" {
		t.Fatalf("a complete zero member_set is a genuine bounded absence and must close, got downgrade %q", got)
	}
}
