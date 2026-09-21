package types

import "strings"

// TraceUsesDependencyAnalysisWindow describes existing producer fields for
// display only. A recursive chain window is a measurement domain; it is not
// the interval of whichever state has the greatest cumulative duration.
// This helper neither changes timestamps nor grants causal/selection authority.
func TraceUsesDependencyAnalysisWindow(predicate, source string) bool {
	switch strings.TrimSpace(predicate) {
	case "wakeup_causal_impact", "wakeup_causal_aggregate":
		return true
	}
	if !strings.HasPrefix(strings.TrimSpace(predicate), "root_cause_") {
		return false
	}
	switch strings.TrimSpace(source) {
	case "wakeup_chain.causal_impacts", "wakeup_chain.aggregated_impacts":
		return true
	}
	return false
}

func TraceObservationUsesDependencyAnalysisWindow(record ObservationRecord) bool {
	if record.Producer != "trace_query" {
		return false
	}
	return TraceUsesDependencyAnalysisWindow(record.Predicate, traceObservationRichNoteValue(record.RichNotes, TraceNoteKeySource))
}

// Shared by the tool result and answer-writing handoff, with no new schema,
// note key, output repair, or validation gate. Exact state_drilldown rows keep
// their own endpoints; an aggregate is never forced into one continuous span.
const TraceDependencyAnalysisWindowGuidance = "A dependency analysis window on causal_impact/aggregated_impact and their root-cause rows is a measurement scope, not a continuous state interval. State totals may cover several disjoint intervals; actual_window is the envelope of all observed states, not the dominant state's endpoints. For a precise state start/end, use the same-source state_drilldown or thread_timeline occurrence; otherwise report the cumulative duration within the analysis window without inventing one continuous interval. This changes neither the requested window nor causal eligibility."
