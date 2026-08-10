package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// acceptedClosureHasActiveExploreContractBacktrack distinguishes a fresh,
// typed contract repair from advisory retry carry-over left by an already
// accepted investigation. The retry plan is the closed authority; rendered
// violation prose is never parsed here.
func acceptedClosureHasActiveExploreContractBacktrack(mut *types.MutableState) bool {
	if mut == nil {
		return false
	}
	retry := mut.RetryState()
	return retry != nil &&
		len(retry.ActiveViolations) > 0 &&
		strings.TrimSpace(retry.LastPrimaryOwner) == string(LocusExplore)
}
