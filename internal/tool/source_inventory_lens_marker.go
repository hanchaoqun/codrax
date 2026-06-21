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
// source_inventory lens ran without turning broad navigation rows into durable
// answer-member authority. Downstream scheduler/preflight gates need the
// deterministic repo_lens provenance; final-answer member sets still come from
// model-emitted aggregate facts and verified source citations.
func persistSourceInventoryLensExecutionMarker(ctx *types.BusContext, observation types.SourceInventoryObservation) {
	if ctx == nil || ctx.Mutable == nil || !observation.IsActive() {
		return
	}
	marker := types.CloneSourceInventoryObservation(observation)
	marker.AdvisoryOnly = true
	marker.Sets = nil
	if !marker.IsActive() {
		return
	}
	current := ctx.Mutable.SourceInventoryObservation()
	if current.IsActive() {
		marker = types.MergeSourceInventoryObservation(current, marker)
	}
	ctx.Mutable.SetSourceInventoryObservation(marker)
}
