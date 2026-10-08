package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceQueryRequestWindowTeaching = "When both time bounds are omitted and there is no line scope or business_span_ref, trace_query defaults to the current request's one validated explicit time window for the same admitted capture. Explicit tool bounds (including a single endpoint) are never overwritten; multiple request windows are never combined or selected automatically. Without that request scope the existing whole-observed-source defaults remain unchanged."

// This is parameter defaulting for the shared time filter, not a view-specific
// repair or a new evidence authority. Every registered view uses the same time
// parameters. An explicit caller scope always wins, including one-sided bounds,
// line scopes and a selected business instance. Multiple request windows are
// never collapsed or elected here.
func traceQueryApplyRequestWindow(ctx *types.BusContext, p traceQueryParams, path, sourceLabel string) (traceQueryParams, string) {
	if ctx == nil || tracequery.ValidateViewName(p.View) != nil ||
		p.TimeStart.Set() || p.TimeEnd.Set() || p.LineStart.Int() != 0 || p.LineEnd.Int() != 0 ||
		strings.TrimSpace(p.BusinessSpanRef) != "" ||
		(ctx.Mutable != nil && ctx.Mutable.SystemTraceSupplementInProgress()) {
		return p, ""
	}
	start, end, ok := traceSupplementRequestedArtifactScope(ctx).ExplicitTimeWindow()
	if !ok || (!traceQueryRequestWindowSourceMatches(ctx, path, sourceLabel) &&
		!traceCatalogRequestWindowSourceMatches(ctx, p, path, start, end)) {
		return p, ""
	}
	// The profile already contains exact trace-clock seconds. Do not introduce
	// event-search lookup tolerance by reparsing shortened decimal display text.
	p.TimeStart, p.TimeEnd = traceSecondFromAutoWindow(start), traceSecondFromAutoWindow(end)
	return p, fmt.Sprintf("trace_query_request_window_inherited=validated_single_request_window; time_start=%.9g time_end=%.9g; explicit tool bounds, line scopes and business references remain unchanged", start, end)
}

// Called only after source preparation/admission. The selected attachment or a
// unique current preflight trace must resolve to the same prepared coordinate.
// Preflight is used only for source navigation identity, never causal authority.
// Analyzer hints, request prose, basename similarity and bundle-child guessing
// cannot bind the request window to a different capture.
func traceQueryRequestWindowSourceMatches(ctx *types.BusContext, path, sourceLabel string) bool {
	if sourceLabel == "attached_trace" && (ctx.AttachedTraceMaterial != nil || strings.TrimSpace(ctx.AttachedHitrace) != "") {
		return true
	}
	selected, ok := traceQueryRequestWindowPathIdentity(ctx, path)
	if !ok {
		return false
	}
	if ctx.AttachedTraceMaterial != nil {
		attached, valid := traceQueryRequestWindowPathIdentity(ctx, ctx.AttachedTraceMaterial.QueryPath())
		return valid && selected == attached
	}
	if strings.TrimSpace(ctx.AttachedHitrace) != "" {
		// The attached-source resolver has already materialized this exact blob;
		// a path call must not create or substitute one while checking identity.
		attached, valid := traceQueryRequestWindowPathIdentity(ctx, filepath.Join(ctx.WorkDir, promptctx.AttachedTraceBlobName))
		return valid && selected == attached
	}
	paths := map[string]bool{}
	for _, artifact := range types.NormalizeRuntimeArtifactPreflightProfile(ctx.RuntimeArtifactPreflight).Artifacts {
		if artifact.RuntimeArtifactKind() != "trace" {
			continue
		}
		if artifact.Carrier == "attachment" || strings.TrimSpace(artifact.Source) == "" {
			return false
		}
		identity, valid := traceQueryRequestWindowPathIdentity(ctx, resolveToolPath(ctx, artifact.Source))
		if !valid {
			return false
		}
		paths[identity] = true
	}
	return len(paths) == 1 && paths[selected]
}

func traceQueryRequestWindowPathIdentity(ctx *types.BusContext, path string) (string, bool) {
	path = traceQueryPreparedNamedPath(ctx, path)
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	abs, err := filepath.Abs(resolved)
	return filepath.Clean(abs), err == nil
}
