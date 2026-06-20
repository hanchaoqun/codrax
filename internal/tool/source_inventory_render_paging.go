package tool

import "github.com/hanchaoqun/codrax/internal/types"

func sourceInventoryObservationPageAlreadyApplied(observation types.SourceInventoryObservation, offset int) bool {
	if observation.Page == nil || observation.Page.Offset != offset {
		return false
	}
	visible := 0
	for _, set := range observation.Sets {
		visible += len(set.Members)
	}
	if visible == 0 || observation.Page.Total <= visible {
		return false
	}
	return observation.Page.Emitted <= 0 || visible <= observation.Page.Emitted
}
