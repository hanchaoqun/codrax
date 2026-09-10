package types

// TraceSchedulerMeasurementOrigin binds native partition references to the
// final observation's enclosing result receipt. Neither half alone identifies
// a numeric value, proves complete capture coverage, or permits adding rows.
// Unknown receipt fields and missing native sources are retained as unknown.
type TraceSchedulerMeasurementOrigin struct {
	SourceRef          ObservationSourceRef              `json:"source_ref"`
	ObservedAt         string                            `json:"observed_at,omitempty"`
	MeasurementSources *TraceSchedulerMeasurementSources `json:"measurement_sources,omitempty"`
}

// TraceSchedulerMeasurementOriginsFromRecord must consume the final compiled
// record, after source requalification and tool-result defaults. Always keep
// one origin, including legacy/unknown inputs; dropping it would hide an
// unknown contributor when another row is later absorbed. No ID, prose, span,
// or enclosing query window is used to invent a native partition.
func TraceSchedulerMeasurementOriginsFromRecord(record ObservationRecord) []TraceSchedulerMeasurementOrigin {
	return []TraceSchedulerMeasurementOrigin{cloneTraceSchedulerMeasurementOrigin(TraceSchedulerMeasurementOrigin{
		SourceRef: record.SourceRef, ObservedAt: record.ObservedAt, MeasurementSources: record.MeasurementSources,
	})}
}

// CloneTraceSchedulerMeasurementOrigins preserves every origin and its order.
// It does not merge matching receipts or erase unknown/duplicate contributors.
func CloneTraceSchedulerMeasurementOrigins(in []TraceSchedulerMeasurementOrigin) []TraceSchedulerMeasurementOrigin {
	if in == nil {
		return nil
	}
	out := make([]TraceSchedulerMeasurementOrigin, len(in))
	for i := range in {
		out[i] = cloneTraceSchedulerMeasurementOrigin(in[i])
	}
	return out
}

// ConcatTraceSchedulerMeasurementOrigins preserves the sources of each actual
// contributing member in order. A legacy member without origins contributes
// one unknown origin; no members contributes none. Repeated origins are not
// deduplicated: result receipts and native sources do not establish numeric
// identity or authorize adding the members' values. Every output owns its
// nested storage, including repeated references in the inputs.
func ConcatTraceSchedulerMeasurementOrigins(members ...[]TraceSchedulerMeasurementOrigin) []TraceSchedulerMeasurementOrigin {
	if len(members) == 0 {
		return nil
	}
	size := 0
	for _, origins := range members {
		if len(origins) == 0 {
			size++
		} else {
			size += len(origins)
		}
	}
	out := make([]TraceSchedulerMeasurementOrigin, 0, size)
	for _, origins := range members {
		if len(origins) == 0 {
			out = append(out, TraceSchedulerMeasurementOrigin{})
			continue
		}
		for _, origin := range origins {
			out = append(out, cloneTraceSchedulerMeasurementOrigin(origin))
		}
	}
	return out
}

func cloneTraceSchedulerMeasurementOrigin(in TraceSchedulerMeasurementOrigin) TraceSchedulerMeasurementOrigin {
	if in.SourceRef.ClockOffsetSec != nil {
		value := *in.SourceRef.ClockOffsetSec
		in.SourceRef.ClockOffsetSec = &value
	}
	if in.SourceRef.ClockSlope != nil {
		value := *in.SourceRef.ClockSlope
		in.SourceRef.ClockSlope = &value
	}
	in.MeasurementSources = CloneTraceSchedulerMeasurementSources(in.MeasurementSources)
	return in
}
