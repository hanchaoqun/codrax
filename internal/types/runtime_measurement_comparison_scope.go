package types

import (
	"context"
	"github.com/hanchaoqun/codrax/internal/attachment"
)

// Both the shared table contract and specialized fact handoff use this same
// current-result origin/window check. An observation ID cannot borrow another
// result's valid read receipt, even when that ID is present in the contract.
func runtimeMeasurementQueryDisplayScope(input ObservationLedgerInput, result ToolResult, r ObservationRecord) (*RuntimeArtifactScopeProfile, string, bool) {
	var requested *RuntimeArtifactScopeProfile
	if input.RequestModel != nil {
		requested = input.RequestModel.RuntimeArtifactScopeProfile
	}
	start, end, known := TraceObservationContinuousQueryWindow(r.SourceRef)
	original := runtimeMeasurementOriginalSource(input, result, r.SourceRef)
	if r.Predicate == RuntimeResourceMeasurementPredicate && original == "" {
		return nil, "", false
	}
	requested, qualified := RuntimeArtifactDisplayScopeForOriginal(requested, original, input.RepoRoot)
	if !qualified || !runtimeSamplingMeasurementWithinWindows(r, requested) || !runtimeResourceMeasurementWithinWindows(r, requested) {
		return nil, "", false
	}
	// Event receipts own individual points and are projected row-by-row from a
	// wider lookup envelope; other native query windows must be contained.
	if requested.HasExplicitTimeWindows() && known && !requested.ContainsExplicitTimeWindow(start, end) && r.Predicate != TraceEventSearchInventoryPredicate {
		return nil, "", false
	}
	return requested, original, true
}

// RuntimeResourceSupplementaryQueryCurrent validates this exact native result,
// not a same-ID contract entry. It grants only supplementary fact display.
func RuntimeResourceSupplementaryQueryCurrent(input ObservationLedgerInput, result ToolResult, r ObservationRecord) bool {
	if !result.Success || result.ToolName != "trace_query" || result.ReusedFromRunMemo || !RuntimeResourceQueryIsSupplementary(r, input.RequestModel) {
		return false
	}
	if _, valid := DecodeRuntimeMeasurementPublication(r); !valid {
		return false
	}
	_, _, valid := runtimeMeasurementQueryDisplayScope(input, result, r)
	return valid
}

// The original input comes from a current private prepared/replay receipt,
// not SourceRef display strings, model parameters or a derived-file basename.
func runtimeMeasurementOriginalSource(input ObservationLedgerInput, result ToolResult, source ObservationSourceRef) string {
	consumer := input.runtimeMeasurementPairConsumer
	if consumer == nil {
		return ""
	}
	ref := result.TraceQueryWindowReplay
	if nav := ref.preparedSource; nav != nil {
		path, _, current := consumer.resolveTracePreparedQuerySource(context.Background(), nav)
		if current && nav.material != nil && path == source.Path {
			return runtimeMeasurementBoundOriginalSource(input, nav.material, nav.material.SourcePath())
		}
		return ""
	}
	if nav := ref.nativeNavigation; nav != nil {
		path, _, current := consumer.resolveTracePreparedQuerySource(context.Background(), nav)
		if current && path == source.Path {
			return runtimeMeasurementBoundOriginalSource(input, nav.material, path)
		}
		return ""
	}
	path, _, current := consumer.ResolveTraceQueryWindowReplay(ref)
	if !current {
		path, _, current = consumer.ResolveTraceQueryWindowReplay(result.TraceStatistics.replay)
	}
	if current && path == source.Path {
		return path
	}
	return ""
}

func runtimeMeasurementBoundOriginalSource(input ObservationLedgerInput, material *attachment.TraceMaterial, original string) string {
	if material == nil || input.RequestModel == nil || input.RequestModel.RuntimeArtifactScopeProfile == nil {
		return original
	}
	matched := ""
	for _, side := range input.RequestModel.RuntimeArtifactScopeProfile.Comparison.Sides() {
		if material.MatchesOriginalSource(RuntimeArtifactComparisonLocalPath(side.Path, input.RepoRoot)) {
			if matched != "" {
				return "" // Two requested aliases cannot own one physical input.
			}
			matched = side.Path
		}
	}
	if matched != "" {
		return matched
	}
	return original
}
