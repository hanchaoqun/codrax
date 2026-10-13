package tool

import "github.com/hanchaoqun/codrax/internal/types"

// TraceMeasurementTypeDisplayLabel preserves the ruler of this exact typed
// value. IO family membership, completion closure and a peer identity cannot
// substitute for its published caliber. This is vocabulary only: it grants
// neither a root-cause seat nor a blocking relationship.
func TraceMeasurementTypeDisplayLabel(token, ioValueCaliber string, zh bool) string {
	if types.TraceUsesIOValueCaliber(token, ioValueCaliber) {
		return types.TraceIOValueCaliberLabel(ioValueCaliber, zh)
	}
	return TraceRootCauseTypeDisplayLabel(token, zh)
}
