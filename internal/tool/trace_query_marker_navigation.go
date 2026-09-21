package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A previous analysis cursor is not the owner of a separately named marker
// or a structured marker inventory. Skip only that implicit cursor; explicit
// selectors, any active non-cursor target and system supplementation retain
// their existing scopes. Both request-model copies matter: target deduplication
// can otherwise hide a same-identity user focus behind a later cursor entry.
func traceQueryMarkerNavigationSkipsCursor(ctx *types.BusContext, p traceQueryParams, target traceQueryRequestTarget) bool {
	if ctx == nil || !types.RuntimeTargetIsExplorationCursorSource(target.Source) {
		return false
	}
	if scope := strings.ToLower(strings.TrimSpace(p.TargetScope)); scope != "" && scope != tracequery.TargetScopeThread {
		return false
	}
	if ctx.Mutable != nil && ctx.Mutable.SystemTraceSupplementInProgress() {
		return false
	}
	switch tracequery.CanonicalViewName(p.View) {
	case "span_window":
		if strings.TrimSpace(p.SpanName) == "" {
			return false
		}
	case tracequery.FallbackViewEventSearch:
		eventTypes := parseTraceQueryEventTypes(p.EventTypes.Strings())
		if len(eventTypes) == 0 {
			return false
		}
		for _, eventType := range eventTypes {
			if eventType != tracequery.EventTraceMark {
				return false
			}
		}
	default:
		return false
	}
	hasNonCursor := func(rm *types.RequestModel) bool {
		if rm != nil {
			for _, runtimeTarget := range rm.RuntimeTargets {
				if runtimeTarget.Active() && !types.RuntimeTargetIsExplorationCursorSource(runtimeTarget.Source) {
					return true
				}
			}
		}
		return false
	}
	if ctx.AnalysisIR != nil && hasNonCursor(&ctx.AnalysisIR.RequestModel) {
		return false
	}
	return ctx.Mutable == nil || !hasNonCursor(ctx.Mutable.RequestModel())
}
