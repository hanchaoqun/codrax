package types

// TraceEventRowWithinRequestedWindows tests the observed Trace/report point,
// never a business endpoint. The caller must first qualify the scope for the
// row's original capture. A business selector with no explicit Trace window
// imposes no additional report-time condition.
func TraceEventRowWithinRequestedWindows(row TraceEventSearchInventoryRow, scope *RuntimeArtifactScopeProfile) bool {
	windows := scope.ExplicitTimeWindows()
	if len(windows) == 0 {
		return true
	}
	if (row.EventType == "measure_interval" || row.EventType == "process_measure_interval") &&
		(!row.SourceTimeKnown || row.TraceTimeSeconds != row.SourceTimeSeconds) {
		return false // A synthetic ordering coordinate is not an observed time.
	}
	for _, window := range windows {
		if row.TraceTimeSeconds >= *window.TimeStart && row.TraceTimeSeconds < *window.TimeEnd {
			return true
		}
	}
	return false
}

// Called only after decoding the full original publication and qualifying the
// capture-specific request scope. Rebuild the private display table without
// changing the query inventory, coverage, stable event IDs or original bytes.
func scopeRuntimeEventTable(table RuntimeMeasurementTable, r ObservationRecord, scope *RuntimeArtifactScopeProfile) (RuntimeMeasurementTable, bool) {
	return scopeRuntimeEventTableForRequest(table, r, scope, nil, TraceEventParticipantDisplayRoster{})
}

func scopeRuntimeEventTableForRequest(table RuntimeMeasurementTable, r ObservationRecord, scope *RuntimeArtifactScopeProfile, rm *RequestModel, roster TraceEventParticipantDisplayRoster) (RuntimeMeasurementTable, bool) {
	selectTarget := rm != nil && rm.RuntimeQuestionProfile.SuppressesRootCauseRankingPrompt() && rm.RuntimeQuestionProfile.CarriesBoundedFactFamilies() && traceEventDisplayHasUserTarget(rm)
	if table.EventData == nil || !scope.HasExplicitTimeWindows() && !selectTarget {
		return table, true
	}
	if !table.nativeEventData || r.EventSearchInventory == nil || len(table.EventData.Rows) != len(r.EventSearchInventory.Rows) {
		return RuntimeMeasurementTable{}, false
	}
	out := table.Clone()
	out.EventData.Rows = nil
	for n, row := range r.EventSearchInventory.Rows {
		if selectTarget && !TraceEventRowMatchesDisplayTarget(r, row, rm, roster) {
			out.nativeEventTargetExcluded++
			continue
		}
		if TraceEventRowWithinRequestedWindows(row, scope) {
			out.EventData.Rows = append(out.EventData.Rows, table.EventData.Rows[n])
		} else {
			out.nativeEventRequestExcluded++
		}
	}
	return selectRuntimeEventTable(out, nil)
}
