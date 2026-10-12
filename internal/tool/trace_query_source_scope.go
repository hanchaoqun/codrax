package tool

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Called after source admission. Only a current preparation receipt may bind
// a derived query path to a comparison's original input. This does not prepare
// files, guess roles from names, or change the shared request model.
func traceQuerySourceRequestedScope(ctx *types.BusContext, path string) (*types.RuntimeArtifactScopeProfile, bool) {
	requested := traceSupplementRequestedArtifactScope(ctx)
	if requested == nil || requested.Comparison == nil {
		return requested, true
	}
	if ctx == nil || !requested.Comparison.Valid() {
		return nil, false
	}
	materials := []*attachment.TraceMaterial{ctx.AttachedTraceMaterial}
	if ctx.TraceInputPreparer != nil {
		materials = append(materials, ctx.TraceInputPreparer.PreparedMaterials()...)
	}
	for _, material := range materials {
		if material == nil || !types.RuntimeArtifactComparisonPathEqual(material.QueryPath(), path, ctx.RepoRoot) {
			continue
		}
		preview := material.Preview()
		if material == ctx.AttachedTraceMaterial {
			preview = ctx.AttachedHitrace
		}
		if material.Validate(contextFromBus(ctx), preview) != nil {
			continue
		}
		for _, side := range requested.Comparison.Sides() {
			if material.MatchesOriginalSource(types.RuntimeArtifactComparisonLocalPath(side.Path, ctx.RepoRoot)) {
				return types.RuntimeArtifactDisplayScopeForOriginal(requested, side.Path, ctx.RepoRoot)
			}
		}
	}
	return nil, false
}

// A supplement may reuse its normal single-source planning without seeing the
// other side's ruler. This is a local shallow context, never a global IR edit.
func traceQuerySourceScopeContext(ctx *types.BusContext, path string) (*types.BusContext, bool) {
	requested := traceSupplementRequestedArtifactScope(ctx)
	if requested == nil || requested.Comparison == nil {
		return ctx, true
	}
	scope, known := traceQuerySourceRequestedScope(ctx, path)
	if !known {
		return ctx, false
	}
	out := ctx.ShallowClone()
	var ir types.AnalysisIR
	if ctx.AnalysisIR != nil {
		ir = *ctx.AnalysisIR
	} else if rm := ctx.Mutable.RequestModel(); rm != nil {
		ir.RequestModel = *rm
	}
	ir.RequestModel.RuntimeArtifactScopeProfile = scope
	out.AnalysisIR = &ir
	return out, true
}

func traceQueryRequestedWindowSourceMatches(ctx *types.BusContext, path, label string) bool {
	if scope := traceSupplementRequestedArtifactScope(ctx); scope != nil && scope.Comparison != nil {
		_, known := traceQuerySourceRequestedScope(ctx, path)
		return known
	}
	return traceQueryRequestWindowSourceMatches(ctx, path, label)
}

// Only the local supplement input is narrowed. The run's complete ledger,
// other source results and their causal projections remain unchanged.
func traceQueryComparisonSourceInput(ctx *types.BusContext, path string, input types.ObservationLedgerInput) types.ObservationLedgerInput {
	requested := traceSupplementRequestedArtifactScope(ctx)
	if requested == nil || requested.Comparison == nil {
		return input
	}
	scopes := types.BuildRuntimeObservationDisplayScopes(input)
	keep := func(results []types.ToolResult) []types.ToolResult {
		var out []types.ToolResult
		for _, result := range results {
			if len(result.Observations) == 0 {
				continue
			}
			all := true
			for _, record := range result.Observations {
				_, known := scopes.Scope(record, requested)
				if !known || !types.RuntimeArtifactComparisonPathEqual(record.SourceRef.Path, path, ctx.RepoRoot) {
					all = false
					break
				}
			}
			if all {
				out = append(out, result)
			}
		}
		return out
	}
	input.ToolResults = keep(input.ToolResults)
	input.SystemTraceSupplementResults = keep(input.SystemTraceSupplementResults)
	return input
}

func traceQuerySourceCallWindows(ctx *types.BusContext, path, label string, input types.ObservationLedgerInput) []types.TraceQueryCallWindow {
	if scope := traceSupplementRequestedArtifactScope(ctx); scope == nil || scope.Comparison == nil {
		return ctx.Mutable.TraceQueryCallWindows()
	}
	var windows []types.TraceQueryCallWindow
	for _, call := range primaryWindowCalls(ctx, path, label, input) {
		// Carry the actual producer's semantic settings, not just its ruler.
		// Dropping these would silently change automatic follow-up analysis.
		raw, err := json.Marshal(call.params)
		var params traceQueryParams
		if err != nil || json.Unmarshal(raw, &params) != nil {
			continue
		}
		windows = append(windows, types.TraceQueryCallWindow{
			View: call.view, TimeStart: call.start, TimeEnd: call.end,
			TraceFlavor:  strings.TrimSpace(params.TraceFlavor),
			CoreTopology: strings.TrimSpace(params.CoreTopology),
			Platform:     strings.TrimSpace(params.Platform),
		})
	}
	return windows
}
