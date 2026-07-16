package hitraceconv

import "github.com/hanchaoqun/codrax/internal/tracequery"

// stageProfilerTraceClass is the only producer-side tracequery verdict mint.
// It deliberately lives outside the bounded sorter implementation: the sorter
// owns ordering and authenticated storage, while this adapter owns semantic
// classification. Production extraction enables the adapter before admitting
// its first row; infrastructure-only sorter tests may carry the honest
// unclassified class None, which the Profiler publication throat rejects.
func (s *traceDBRowSink) stageProfilerTraceClass(row *renderedRow) error {
	if row == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_trace_class_row_missing"}
	}
	if s == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_trace_class_sink_missing"}
	}
	if row.profilerTraceClass != profilerTraceClassNone {
		return &traceDBOutputInvariantError{Reason: "profiler_trace_class_preassigned"}
	}
	provenance := row.profilerProvenance()
	if !provenance.sourceValid() || provenance.TraceClass != profilerTraceClassNone {
		return &traceDBOutputInvariantError{Reason: "profiler_row_provenance_invalid"}
	}
	if !s.profilerTraceClassification {
		return nil
	}
	if provenance.PublisherSlot == profilerPairPublisherNone {
		return &traceDBOutputInvariantError{Reason: "profiler_trace_class_publisher_missing"}
	}
	event, parsed, err := parseOwnedSystraceRow(1, row.line)
	if err != nil {
		return &traceDBOutputInvariantError{Reason: "profiler_trace_class_parse_panic", Cause: err}
	}
	if !parsed {
		return &traceDBOutputInvariantError{Reason: "profiler_trace_class_unparsed"}
	}
	textSource := provenance.PublisherSlot == profilerPairPublisherSession ||
		provenance.TextMessageOrdinal != 0 || provenance.Flags&profilerPairRowProvenanceText != 0
	if event.Type == tracequery.EventUnknown {
		if !textSource {
			return &traceDBOutputInvariantError{Reason: "profiler_trace_class_structured_unknown"}
		}
		row.profilerTraceClass = profilerTraceClassTextIntentionalUnknown
		return nil
	}
	if textSource {
		row.profilerTraceClass = profilerTraceClassTextKnown
	} else {
		row.profilerTraceClass = profilerTraceClassStructuredKnown
	}
	return nil
}
