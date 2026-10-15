package tracequery

import "math"

// PerfSampleTimeBounds describes recorded sample coordinates in the enclosing
// cohort's clock, after its native selectors and before any display limit.
// These first/last points are neither continuous execution nor a duration,
// recording-coverage claim, or evidence of absence between/outside the points.
// Nil bounds are legacy/unavailable. A known zero timestamp is not missing.
type PerfSampleTimeBounds struct {
	KnownSampleCount   int      `json:"known_sample_count"`
	UnknownSampleCount int      `json:"unknown_sample_count"`
	FirstTs            *float64 `json:"first_ts"`
	LastTs             *float64 `json:"last_ts"`
}

type perfSampleTimeAcc struct {
	known, unknown int
	first, last    float64
}

func (a *perfSampleTimeAcc) add(ts float64) {
	if math.IsNaN(ts) || math.IsInf(ts, 0) {
		a.unknown++
		return
	}
	if a.known == 0 || ts < a.first {
		a.first = ts
	}
	if a.known == 0 || ts > a.last {
		a.last = ts
	}
	a.known++
}

func (a perfSampleTimeAcc) summary() *PerfSampleTimeBounds {
	if a.known+a.unknown == 0 {
		return nil
	}
	b := &PerfSampleTimeBounds{KnownSampleCount: a.known, UnknownSampleCount: a.unknown}
	if a.known > 0 {
		first, last := a.first, a.last
		b.FirstTs, b.LastTs = &first, &last
	}
	return b
}
