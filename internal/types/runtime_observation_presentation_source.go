package types

import "reflect"

// RuntimeObservationPresentationSourceKey identifies a currently held native
// source for dispatch-local display reuse only. It does not qualify a fact,
// authorize another query, or survive JSON/history restoration. The caller
// must separately validate the exact native facts it proposes to share.
func RuntimeObservationPresentationSourceKey(input ObservationLedgerInput, result ToolResult, record ObservationRecord) string {
	if !result.Success || result.ToolName != "trace_query" || result.ReusedFromRunMemo || result.TraceViewCancellation != nil || record.ID == "" {
		return ""
	}
	found := false
	for _, candidate := range result.Observations {
		if candidate.ID != record.ID {
			continue
		}
		if !reflect.DeepEqual(candidate, record) {
			return "" // One ambiguous ID cannot borrow its sibling's source.
		}
		found = true
	}
	if !found {
		return ""
	}
	original, replay := runtimeMeasurementOriginalSourceReplay(input, result, record.SourceRef)
	ref := replay.source
	if replay.preparedSource != nil {
		ref = replay.preparedSource.source
	} else if replay.nativeNavigation != nil {
		ref = replay.nativeNavigation.source
	}
	if original == "" || ref.generation == nil || ref.path != record.SourceRef.Path {
		return ""
	}
	return runtimeSamplingDigest([]string{original, ref.path, ref.identity.CacheToken()})
}
