package tool

import "github.com/hanchaoqun/codrax/internal/types"

// PersistSourceInventoryLensObservation lets adapters attach adapter-owned
// coordinates after the shared lens publisher returns.
func PersistSourceInventoryLensObservation(ctx *types.BusContext, observation types.SourceInventoryObservation) {
	persistSourceInventoryLensObservation(ctx, observation)
}
