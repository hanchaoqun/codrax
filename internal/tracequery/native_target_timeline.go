package tracequery

// NativeTargetTimeline exposes the already measured target partition to native
// display producers. It performs no scan or arithmetic, changes no query scope,
// and does not activate the legacy Timeline JSON/causal publication branch for
// other views. Missing or failed measurements remain absent.
func (r Result) NativeTargetTimeline() *TimelineResult {
	if r.Timeline != nil {
		return r.Timeline
	}
	return r.nativeTargetTimeline
}
