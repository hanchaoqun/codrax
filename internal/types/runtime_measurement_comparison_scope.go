package types

import (
	"context"
	"github.com/hanchaoqun/codrax/internal/attachment"
)

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
