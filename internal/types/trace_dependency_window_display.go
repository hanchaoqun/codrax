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
	if !RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
		return false
	}
	return TraceUsesDependencyAnalysisWindow(record.Predicate, traceObservationRichNoteValue(record.RichNotes, TraceNoteKeySource))
}

// TraceMeasurementWindowDisplayRole labels existing typed measurement
// families without promoting a timestamp hull to a physical occurrence.
// Every current top_* drilldown source sums ThreadDuration records, and
// state_churn sums a state inventory. Unknown/absent drilldown sources lack
// single-occurrence proof and remain conservatively labelled as scopes.
// A thread_timeline interval is a different producer family and is untouched.
func TraceMeasurementWindowDisplayRole(predicate, source string, zh bool) string {
	if TraceUsesDependencyAnalysisWindow(predicate, source) {
		if zh {
			return "依赖分析窗口"
		}
		return "dependency analysis window"
	}
	switch strings.TrimSpace(predicate) {
	case "state_churn", "running_time", "runnable_wait", "sleep_wait", "d_state_or_io_wait", "io_wait":
		if zh {
			return "累计状态统计范围"
		}
		return "cumulative state measurement scope"
	}
	if strings.TrimSpace(predicate) != "state_drilldown" {
		return ""
	}
	switch strings.TrimSpace(source) {
	case "top_sleep", "top_runnable", "top_running", "top_io_wait", "top_d_state", "state_churn":
		if zh {
			return "累计状态统计范围"
		}
		return "cumulative state measurement scope"
	default:
		if zh {
			return "状态下钻统计范围（未证明单次发生）"
		}
		return "state drilldown measurement scope (single occurrence unproven)"
	}
}

func TraceObservationMeasurementWindowDisplayRole(record ObservationRecord, zh bool) string {
	if !RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
		return ""
	}
	return TraceMeasurementWindowDisplayRole(record.Predicate, traceObservationRichNoteValue(record.RichNotes, TraceNoteKeySource), zh)
}

// Shared by the tool result and answer-writing handoff, with no new schema,
// note key, output repair, or validation gate. Only a typed occurrence such
// as a thread_timeline interval certifies continuous state endpoints.
const TraceDependencyAnalysisWindowGuidance = "A dependency analysis window on causal_impact/aggregated_impact and their root-cause rows is a measurement scope, not a continuous state interval. State totals may cover several disjoint intervals; actual_window is the envelope of all observed states, not the dominant state's endpoints. For a precise state start/end, use a same-source thread_timeline interval; state_drilldown is cumulative measurement guidance, not occurrence proof. Otherwise report the cumulative duration within the analysis window without inventing one continuous interval. This changes neither the requested window nor causal eligibility."

const TraceStateDrilldownWindowGuidance = "state_drilldown is a cumulative state measurement scope, not a continuous state interval. Its top_sleep/top_runnable/top_running/top_io_wait/top_d_state rows sum state durations; start/end bound those records and may contain gaps. Typed state_accounting distinguishes observed boundaries, open tails and unknown closure; its times are accounted contributions, not invented actual endpoints. Missing metadata means unknown closure. state_churn keeps each state's account separately. Preserve source, recommended_views, chain_required and recursive; precise occurrences need same-source occurrence evidence, never cumulative duration equated to envelope width. Requested windows and causal eligibility are unchanged."
