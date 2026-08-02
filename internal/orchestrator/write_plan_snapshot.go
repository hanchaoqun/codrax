package orchestrator

import (
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// persistImmutablePlanIDSnapshot retains an id-addressed sibling when
// PlanPath is a stable import/result alias that a later replan may overwrite.
// Workflow history and final proof assembly refer to exact plan IDs, not to
// whichever plan owns the alias at the end of the run.
func (o *Orchestrator) persistImmutablePlanIDSnapshot(plan *types.ChangePlan, aliasPath string) {
	if plan == nil {
		return
	}
	stem := writeWorkflowArtifactFileStem(plan.ID)
	dir := filepath.Dir(aliasPath)
	if stem == "" || dir == "" {
		return
	}
	identityPath := filepath.Join(dir, stem+".json")
	if filepath.Clean(identityPath) == filepath.Clean(aliasPath) {
		return
	}
	if err := types.WritePlanToFile(plan, identityPath); err != nil {
		logging.Warning("[orchestrator] ChangePlan immutable snapshot persist failed: %v", err)
		return
	}
	logging.Info("[orchestrator] ChangePlan immutable snapshot persisted: %s", identityPath)
}
