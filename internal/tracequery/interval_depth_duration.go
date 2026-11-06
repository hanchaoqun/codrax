package tracequery

// IntervalDepthDuration accounts for time at the maximum simultaneous count
// of an admitted interval population over its full half-open query window.
// Zero contribution is not a measurement of system or device idle. A nil
// account means there was no usable measurement; peak zero can be measured.
type IntervalDepthDuration struct {
	WindowMs           float64 `json:"window_ms"`
	ZeroContributionMs float64 `json:"zero_contribution_ms"`
	PeakDepth          int     `json:"peak_depth"`
	PeakDurationMs     float64 `json:"peak_duration_ms"`
}

// Called only on the native sweep's positive-width spans, after grouping,
// clipping and simultaneous endpoint handling. No displayed bucket,
// retained timeline prefix or member witness participates in this count.
func (d *IntervalDepthDuration) add(depth int, ms float64) {
	if depth == 0 {
		d.ZeroContributionMs += ms
	}
	if depth > d.PeakDepth {
		d.PeakDepth, d.PeakDurationMs = depth, ms
	} else if depth == d.PeakDepth {
		d.PeakDurationMs += ms
	}
}
