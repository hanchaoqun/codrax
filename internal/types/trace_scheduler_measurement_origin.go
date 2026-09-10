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
