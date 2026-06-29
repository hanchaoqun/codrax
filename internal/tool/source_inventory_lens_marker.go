package tool

import "github.com/hanchaoqun/codrax/internal/types"

func publishAndPersistSourceInventoryObservationFromLens(ctx *types.BusContext, query types.SourceInventoryLensQuery) bool {
	obs := PublishSourceInventoryObservationFromLens(ctx, query)
	if !obs.IsActive() {
		return false
	}
	persistSourceInventoryLensExecutionMarker(ctx, obs)
	return true
}

// persistSourceInventoryLensExecutionMarker records that an executable
// source_inventory lens ran and preserves its typed candidate row-set. The
// rows stay advisory navigation facts, not model-authored answer prose, but
// source-inventory completion/finalizer authorities need the durable row
// universe to detect omissions and materialize mechanical inventory fields.
func persistSourceInventoryLensExecutionMarker(ctx *types.BusContext, observation types.SourceInventoryObservation) {
	if ctx == nil || ctx.Mutable == nil || !observation.IsActive() {
		return
	}
	marker := types.CloneSourceInventoryObservation(observation)
	marker.AdvisoryOnly = true
	if !marker.IsActive() {
		return
	}
	current := ctx.Mutable.SourceInventoryObservation()
	if current.IsActive() {
		marker = types.MergeSourceInventoryObservation(current, marker)
	}
	ctx.Mutable.SetSourceInventoryObservation(marker)
}
