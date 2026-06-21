package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryObservationForNavigationRender(observation types.SourceInventoryObservation) types.SourceInventoryObservation {
	if !observation.IsActive() {
		return observation
	}
	out := types.CloneSourceInventoryObservation(observation)
	changed := false
	for setIdx := range out.Sets {
		for memberIdx := range out.Sets[setIdx].Members {
			attrs := out.Sets[setIdx].Members[memberIdx].Attributes
			if len(attrs) == 0 {
				continue
			}
			kept := attrs[:0]
			for _, attr := range attrs {
				if sourceInventoryObservationAttributeIsNavigationContext(attr) {
					changed = true
					continue
				}
				kept = append(kept, attr)
			}
			out.Sets[setIdx].Members[memberIdx].Attributes = kept
		}
	}
	if !changed {
		return observation
	}
	return out
}

func sourceInventoryObservationAttributeIsNavigationContext(attr types.SourceInventoryObservationAttribute) bool {
	return attr.Role == types.AnswerCandidateRolePackage &&
		strings.TrimSpace(attr.Reason) == sourceInventoryGraphPackageRowContextReason
}
