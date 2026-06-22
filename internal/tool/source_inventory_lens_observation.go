package tool

import "github.com/hanchaoqun/codrax/internal/types"

// persistSourceInventoryLensObservation stores the full typed source-inventory
// lens for downstream completion/handoff authorities while callers may still
// render only the current query. This keeps user-visible navigation noise low
// without dropping complete-lens proof from the durable carrier.
func persistSourceInventoryLensObservation(ctx *types.BusContext, observation types.SourceInventoryObservation) {
	if ctx == nil || ctx.Mutable == nil || !observation.IsActive() {
		return
	}
	durable := types.CloneSourceInventoryObservation(observation)
	if current := ctx.Mutable.SourceInventoryObservation(); current.IsActive() {
		durable = types.MergeSourceInventoryObservation(current, durable)
	}
	ctx.Mutable.SetSourceInventoryObservation(durable)
}
