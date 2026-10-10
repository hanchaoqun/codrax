package types

import "context"

// HasCurrentNativeMeasurementInvestigation recognizes an actual native query
// product, not a success flag, acquisition status, model claim, or source read
// permission. The shared display collector retains decoder, conflict and
// requested-window rules; the additional replay check binds single-query
// facts to this consumer just as pair.reportFor does for comparison facts.
// This only prevents an empty-investigation fallback. It grants neither
// absence/completeness nor causal authority, and zero-valued/empty native
// tables remain legitimate query products.
func HasCurrentNativeMeasurementInvestigation(input ObservationLedgerInput) bool {
	consumer := input.runtimeMeasurementPairConsumer
	if consumer == nil {
		return false
	}
	currentResults := func(results []ToolResult) []ToolResult {
		out := make([]ToolResult, 0, len(results))
		for _, result := range results {
			if result.ToolName != "trace_query" || !result.Success || result.ReusedFromRunMemo {
				continue
			}
			// Pair publications are separately revalidated by the collector.
			// Never re-mint their receipt from a model-visible report.
			var path string
			var current bool
			if result.TraceQueryWindowReplay.preparedSource != nil {
				// Converted/native database input has a prepared material
				// credential, not a single physical event-file read right.
				// Use its existing source/epoch validation together with the
				// actual publication below, never navigation alone. A failed
				// whole-input check cannot fall back to the derived file's
				// still-live physical replay after the original was replaced.
				path, _, current = consumer.resolveTracePreparedQuerySource(context.Background(), result.TraceQueryWindowReplay.preparedSource)
			} else {
				path, _, current = consumer.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay)
				if !current {
					path, _, current = consumer.ResolveTraceQueryWindowReplay(result.TraceStatistics.replay)
				}
			}
			accepted := make([]ObservationRecord, 0, len(result.Observations))
			if current {
				for _, record := range result.Observations {
					physical, _, ok := traceSourcePhysicalPath(record.SourceRef.Path)
					if ok && physical == path {
						accepted = append(accepted, record)
					}
				}
			}
			result.Observations = accepted
			out = append(out, result)
		}
		return out
	}
	input.ToolResults = currentResults(input.ToolResults)
	input.SystemTraceSupplementResults = currentResults(input.SystemTraceSupplementResults)
	var native []RuntimeMeasurementPublication
	for _, publication := range collectRuntimeMeasurementPublications(input) {
		// Status-only and log display tables do not claim a native trace
		// measurement source; they cannot rescue an otherwise empty query.
		if publication.Source.Kind != ObservationSourceRuntimeArtifact || publication.Source.Path == "" {
			continue
		}
		if input.RequestModel != nil && input.RequestModel.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
			if _, _, known := TraceObservationContinuousQueryWindow(publication.Source); !known {
				continue
			}
		}
		native = append(native, publication)
	}
	return len(coalesceRuntimeMeasurementPublications(native)) > 0
}
