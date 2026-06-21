package tool

import "github.com/hanchaoqun/codrax/internal/types"

// source_inventory_absence_repo_truth.go — repo-truth seed for the source-class
// absence gate (RNE-C61 chicken-and-egg root fix).
//
// The typed gate types.SourceInventoryExactAbsenceNeedsInventoryProof self-
// no-ops when observation.SourceClasses is empty (source_inventory_absence.go:
// 17). That empty state is exactly the never-ran-lens state: the source-class
// universe is computed (scope-independently, from git-tracked files via
// ClassifySourcePathRole) ONLY inside the repo_map(view="source_inventory")
// lens path, so a model that declares absence WITHOUT ever running the lens
// leaves the universe empty — and the gate that should DEMAND the lens stays
// inert because it has no universe to compare against. This wrapper closes that
// loop: when an active source-inventory profile declares absence with no model-
// populated universe, it seeds the observation with the repo-truth universe
// (the same git-tracked walk the lens uses) BEFORE the gate runs, so the gate
// becomes repo-truth-anchored instead of model-observation-dependent.
//
// Lens-run absences already carry SourceClasses, so the seed is a no-op for
// them; a truly empty repo yields no universe and the gate stays inert
// (correct). The git-tracked walk runs only on this empty-universe absence
// path, not on every gate call.
func SourceInventoryExactAbsenceNeedsInventoryProofRepoTruth(ctx *types.BusContext, profile *types.SourceInventoryProfile, observation types.SourceInventoryObservation) (string, bool) {
	if ctx != nil && profile != nil && profile.Active() && len(observation.SourceClasses) == 0 {
		observation = AttachSourceInventorySourceClassUniverse(ctx, observation, types.SourceInventoryLensQuery{})
	}
	return types.SourceInventoryExactAbsenceNeedsInventoryProof(profile, observation)
}
