package tool

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A live, explicitly declared catalog plan can bind a directory member to the
// request's one window. Discovery, saved JSON and ordinary query bookkeeping
// cannot. This only defaults arguments after normal source admission: it does
// not make a candidate a trace, join clocks, or confer evidence/causal authority.
func traceCatalogRequestWindowSourceMatches(ctx *types.BusContext, p traceQueryParams, preparedPath string, start, end float64) bool {
	if ctx == nil || ctx.Mutable == nil || strings.TrimSpace(p.Path) == "" || p.Source == "attached_trace" ||
		ctx.AttachedTraceMaterial != nil || strings.TrimSpace(ctx.AttachedHitrace) != "" {
		return false
	}
	// Existing attachment/preflight selection keeps its own stricter binding.
	// A catalog cannot rescue an unrelated or ambiguous preflight selection.
	if types.NormalizeRuntimeArtifactPreflightProfile(ctx.RuntimeArtifactPreflight).HasRuntimeArtifact() {
		return false
	}
	startNS, okStart := traceCatalogExactNS(start)
	endNS, okEnd := traceCatalogExactNS(end)
	if !okStart || !okEnd {
		return false
	}
	original, err := filepath.EvalSymlinks(resolveToolPath(ctx, p.Path))
	if err != nil {
		return false
	}
	selected, ok := traceQueryRequestWindowPathIdentity(ctx, preparedPath)
	if !ok {
		return false
	}
	bound, ok := traceQueryRequestWindowPathIdentity(ctx, original)
	if !ok || selected != bound {
		return false
	}
	matched := false
	for _, catalog := range ctx.Mutable.TraceCatalogs() {
		artifact, exists := catalog.ArtifactForPath(original)
		if !exists {
			continue
		}
		if _, err := catalog.Resolve(contextFromBus(ctx), artifact.ID); err != nil {
			return false
		}
		for _, query := range catalog.Snapshot().Queries {
			if query.ArtifactID != artifact.ID || !catalog.IsDeclaredQuery(query.ID) {
				continue
			}
			// Do not elect one of several plans or infer a missing endpoint.
			if query.Window == nil || query.Window.StartNS != startNS || query.Window.EndNS != endNS {
				return false
			}
			matched = true
		}
	}
	return matched
}
