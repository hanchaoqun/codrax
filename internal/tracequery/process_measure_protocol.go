package tracequery

import "github.com/hanchaoqun/codrax/internal/tracewire"

// ProcessMeasureProtocolView recognizes an exact native protocol, not a fuzzy
// metric label. It only suggests an optional observation query, never assigns
// units to raw rows or turns an observation into a causal claim.
func ProcessMeasureProtocolView(r tracewire.ProcessMeasureInterval) string {
	if r.NameKnown && r.Name == "H:PreferredFrameRate" {
		return ViewPreferredFrameRate
	}
	return ""
}
