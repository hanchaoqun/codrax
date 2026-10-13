package orchestrator

import (
	"reflect"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A legacy store could assign one truncated logical ID to different packs.
// Such an alias cannot pick an owner for a later update. Preserve both old
// inputs and the new one, while exact repeated content remains idempotent.
func retainAmbiguousWorkflowContextPack(run types.WriteWorkflowRun, pack types.WriteContextPack, matches int) types.WriteWorkflowRun {
	logging.Warning("[orchestrator] context_pack_ambiguous_upsert batch=%q pack=%q matches=%d; retaining independent contents", pack.BatchID, pack.PackID, matches)
	pack.ArtifactID = ""
	for _, existing := range run.ContextPacks {
		existing = types.NormalizeWriteContextPack(existing)
		existing.ArtifactID = ""
		if reflect.DeepEqual(existing, pack) {
			return types.NormalizeWriteWorkflowRun(run)
		}
	}
	run.ContextPacks = append(run.ContextPacks, pack)
	return types.NormalizeWriteWorkflowRun(run)
}
