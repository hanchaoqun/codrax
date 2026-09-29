package tracequery

import "github.com/hanchaoqun/codrax/internal/types"

func eventSearchScanScope(q Query, indexRestricted bool, basis string, observedCount int) *types.TraceEventSearchScanScope {
	s := &types.TraceEventSearchScanScope{
		LineStart: q.LineStart, LineEnd: q.LineEnd,
		IndexRestricted: indexRestricted, ObservedBasis: basis, ObservedCount: observedCount,
	}
	if q.LineStart == 0 && q.LineEnd == 0 {
		// Reflect the existing eventInQueryBase/raw scan gates exactly. Do not
		// silently fix legacy zero-bound behavior by claiming a gate executed.
		if q.TimeStart > 0 {
			s.TimeStart, s.TimeStartApplied = q.TimeStart, true
		}
		if q.TimeEnd > 0 {
			s.TimeEnd, s.TimeEndApplied = q.TimeEnd, true
		}
	}
	return s
}
