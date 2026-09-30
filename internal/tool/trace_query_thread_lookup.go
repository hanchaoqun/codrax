package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Lookup inheritance is limited to observational views with a thread input.
// Root election, frame ownership, global event inventories and causal queries
// keep their existing focus authority. Explicit call selectors always win.
func traceQueryThreadLookup(ctx *types.BusContext, p traceQueryParams) (target traceQueryRequestTarget, ok, applicable bool) {
	switch tracequery.CanonicalViewName(p.View) {
	case "process_profile", "thread_timeline", "window_stats", "scheduler_latency_stats":
	default:
		return target, false, false
	}
	if ctx == nil {
		return target, false, false
	}
	var rm *types.RequestModel
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	} else if ctx.Mutable != nil {
		rm = ctx.Mutable.RequestModel()
	}
	if rm == nil || len(rm.RuntimeThreadLookups) == 0 {
		return target, false, false
	}
	for _, focus := range rm.RuntimeTargets {
		if !types.RuntimeTargetIsExplorationCursorSource(focus.Source) {
			return target, false, false
		}
	}
	lookups, errText := parseRuntimeThreadLookups(rm.RawRequest, rm.RuntimeThreadLookups)
	if errText != "" {
		return target, false, true
	}
	identities := map[string]traceQueryRequestTarget{}
	for _, lookup := range lookups {
		pid, thread := lookup.PID, strings.TrimSpace(lookup.Thread)
		if parsedPID, name, ok := tracequery.ParseThreadSelectorIdentity(thread); ok {
			if pid > 0 && pid != parsedPID {
				return target, false, true
			}
			pid, thread = parsedPID, name
		}
		key := fmt.Sprintf("%d:%s", pid, thread)
		if pid > 0 {
			key = fmt.Sprintf("tid:%d", pid)
		}
		identities[key] = traceQueryRequestTarget{PID: pid, Thread: thread, Source: "user_thread_lookup"}
	}
	if len(identities) == 1 {
		for _, target := range identities {
			return target, true, true
		}
	}
	return target, false, true
}
