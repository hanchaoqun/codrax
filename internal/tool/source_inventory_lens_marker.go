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
	// Unconditional merge (§29.122 病B fix round): the merge early arms preserve
	// a typed executed-empty lens credential on the durable carrier that a bare
	// replace would drop; marker is active and already Clone-normalized, so the
	// zero-current arm stores exactly the historic bytes.
	ctx.Mutable.SetSourceInventoryObservation(
		types.MergeSourceInventoryObservation(ctx.Mutable.SourceInventoryObservation(), marker))
}
