package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hasNamedTraceComparison(rm types.RequestModel) bool {
	return rm.RuntimeArtifactScopeProfile != nil && rm.RuntimeArtifactScopeProfile.Comparison.Valid()
}

func comparisonNamedTraceProfile(rm types.RequestModel, profile types.RuntimeArtifactPreflightProfile) types.RuntimeArtifactPreflightProfile {
	if !hasNamedTraceComparison(rm) {
		return profile
	}
	for _, side := range rm.RuntimeArtifactScopeProfile.Comparison.Sides() {
		profile.Artifacts = append(profile.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: side.Path, Carrier: "request_path", Detail: "Explicit independent comparison input; preparation and query statuses are separate from data availability."})
	}
	return types.NormalizeRuntimeArtifactPreflightProfile(profile)
}

func namedTraceComparisonCatalog(ctx context.Context, bus *types.BusContext, paths []string) (*tracecatalog.Catalog, error) {
	profile := bus.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
	if profile == nil || profile.Comparison == nil {
		return nil, nil
	}
	if !profile.Comparison.Valid() || len(paths) != 2 || bus.Mutable == nil {
		return nil, fmt.Errorf("independent comparison must bind exactly both named inputs; additional or invalid inputs retain atomic admission")
	}
	var exact [2]string
	for i, side := range profile.Comparison.Sides() {
		exact[i] = resolveTypedComparisonTraceSource(side.Path, bus.RepoRoot)
		matched := false
		for _, path := range paths {
			matched = matched || typedNamedTraceAdmissionPathKey(path) == typedNamedTraceAdmissionPathKey(exact[i])
		}
		if !matched {
			return nil, fmt.Errorf("comparison input does not match the current named input: %q", side.Path)
		}
		if canonical, err := filepath.EvalSymlinks(exact[i]); err == nil {
			exact[i] = canonical
		}
	}
	catalog, err := tracecatalog.DiscoverIndependentPair(ctx, exact)
	if err != nil {
		return nil, err
	}
	return bus.Mutable.InstallTraceCatalog(catalog)
}
