package tool

import "math"

// The original request endpoints and its full residence belong together.
// Occupancy is a separate, clipped ruler. A default double-zero has no
// presence bit on legacy summary/fact structs, so do not invent timestamps
// for it; a known zero start with a later completion remains displayable.
func traceQueryIORequestTimeDetail(start, end float64) string {
	if math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) || end < start || start == 0 && end == 0 {
		return ""
	}
	return " issue_ts=" + traceQueryDisplaySeconds(start) + " complete_ts=" + traceQueryDisplaySeconds(end) +
		" seconds (request interval is unclipped; occupancy uses its intersection with the query window)"
}
