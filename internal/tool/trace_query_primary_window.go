package tool

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Capture only context-independent window queries. A line, instance, recipe
// or span selector cannot be widened/reinterpreted as an artifact-wide query.
func traceQueryWindowReplayParams(p traceQueryParams, path, sourceLabel string, raw json.RawMessage) json.RawMessage {
	if !p.TimeStart.Set() || !p.TimeEnd.Set() || p.LineStart.Int() != 0 || p.LineEnd.Int() != 0 ||
		p.BusinessSpanRef != "" || p.SpanName != "" || p.RecipeName != "" || p.InteractionDirection != "" || p.ViaThread != "" {
		return nil
	}
	plan, err := traceCatalogQueryPlan(p, raw, "")
	if err != nil {
		return nil
	}
	var args map[string]json.RawMessage
	if json.Unmarshal(plan.Parameters, &args) != nil {
		return nil
	}
	set := func(key string, value any) { args[key], _ = json.Marshal(value) }
	if sourceLabel == "attached_trace" {
		set("source", "attached_trace")
	} else {
		set("source", "path")
	}
	set("path", path)
	set("pid", p.PID.Int())
	set("thread", p.Thread)
	set("target_scope", p.TargetScope)
	out, _ := json.Marshal(args)
	return out
}

type primaryWindowCall struct {
	view, key  string
	params     map[string]json.RawMessage
	start, end float64
	ref        types.TraceQueryWindowReplayRef
}

// Replay changes only the two time endpoints. It never elects a new family,
// borrows a target from a neighboring query or re-labels the original totals.
func primaryWindowCalls(ctx *types.BusContext, path, sourceLabel string, input types.ObservationLedgerInput) []primaryWindowCall {
	if !traceQueryRequestWindowSourceMatches(ctx, path, sourceLabel) {
		return nil
	}
	physical, ok := traceQueryRequestWindowPathIdentity(ctx, path)
	if !ok {
		return nil
	}
	var calls []primaryWindowCall
	for _, result := range input.ToolResults {
		if !result.Success || result.TraceViewCancellation != nil {
			continue
		}
		actual, raw, current := ctx.Mutable.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay)
		if !current || actual != physical {
			continue
		}
		var params map[string]json.RawMessage
		var p traceQueryParams
		if json.Unmarshal(raw, &params) != nil || json.Unmarshal(raw, &p) != nil || !p.TimeStart.Set() || !p.TimeEnd.Set() {
			continue
		}
		delete(params, "time_start")
		delete(params, "time_end")
		key, _ := json.Marshal(params)
		calls = append(calls, primaryWindowCall{view: p.View, key: string(key), params: params,
			start: p.TimeStart.Seconds(), end: p.TimeEnd.Seconds(), ref: result.TraceQueryWindowReplay})
	}
	return calls
}

// Uses only the otherwise-idle supplement branch. Existing target/chain
// supplementation keeps priority and its two slots; this path also has at most
// two total engine calls (not two per window), under the SAME task deadline.
func runPrimaryWindowSupplement(ctx *types.BusContext, path, sourceLabel string, input types.ObservationLedgerInput, out TraceQuerySupplementOutcome, censusLiteWanted bool) (TraceQuerySupplementOutcome, bool) {
	windows := traceSupplementRequestedArtifactScope(ctx).ExplicitTimeWindows()
	if len(windows) == 0 {
		return out, false
	}
	calls := primaryWindowCalls(ctx, path, sourceLabel, input)
	if len(calls) == 0 {
		return out, false
	}
	// Prefer already-successful semantic projections over generic raw discovery
	// when they compete for the same bounded slot. This is only soft ordering of
	// existing calls, never a new view election or an evidence-authority gate.
	// Within each tier keep completion-order-independent query-shape order.
	sort.SliceStable(calls, func(i, j int) bool {
		iRaw := calls[i].view == tracequery.FallbackViewEventSearch
		jRaw := calls[j].view == tracequery.FallbackViewEventSearch
		if iRaw != jRaw {
			return !iRaw
		}
		return calls[i].key < calls[j].key
	})
	meta := types.SystemTraceSupplementMeta{RequestedArtifactScope: types.RuntimeArtifactScopeExplicitWindow,
		TargetSource: "original_query", DurationBudgetS: traceSupplementMaxDuration.Seconds()}
	results := []types.ToolResult{}
	start := time.Now()
	attempts, missing := 0, 0
	callLimit := 2
	if censusLiteWanted {
		callLimit--
	} // reserve the existing census adjunct
	seen := map[string]bool{}
	priorWindows := map[[2]float64]int{}
	ctx.Mutable.BeginSystemTraceSupplementExecution()
	defer ctx.Mutable.EndSystemTraceSupplementExecution()
	for _, window := range windows {
		m := types.SystemTraceSupplementWindowMeta{WindowStart: *window.TimeStart, WindowEnd: *window.TimeEnd}
		windowKey := [2]float64{m.WindowStart, m.WindowEnd}
		if prior, ok := priorWindows[windowKey]; ok {
			meta.MemberWindows = append(meta.MemberWindows, meta.MemberWindows[prior])
			continue
		}
		priorWindows[windowKey] = len(meta.MemberWindows)
		for _, call := range calls {
			keyData, _ := json.Marshal([]any{call.key, m.WindowStart, m.WindowEnd})
			key := string(keyData)
			if seen[key] {
				continue
			}
			seen[key] = true
			present := false
			for _, old := range calls {
				if old.key == call.key && old.start == m.WindowStart && old.end == m.WindowEnd {
					present = true
					break
				}
			}
			if present {
				continue
			}
			missing++
			if m.WindowEnd-m.WindowStart > traceSupplementMaxWindowSpanS {
				m.SkipReason = types.TraceSupplementReasonWindowSpanExceeded
				m.WindowBudgetS = traceSupplementMaxWindowSpanS
				m.SkippedViews = append(m.SkippedViews, call.view)
				continue
			}
			if ctx.Ctx.Err() != nil {
				m.SkipReason = traceSupplementMemberCancelReason(ctx.Ctx)
				m.SkippedViews = append(m.SkippedViews, call.view)
				continue
			}
			if attempts == callLimit {
				m.SkipReason = types.TraceSupplementReasonQueryBudgetExceeded
				m.SkippedViews = append(m.SkippedViews, call.view)
				continue
			}
			if _, _, current := ctx.Mutable.ResolveTraceQueryWindowReplay(call.ref); !current {
				m.SkipReason = types.TraceSupplementReasonExecutionFailed
				m.SkippedViews = append(m.SkippedViews, call.view)
				continue
			}
			args := make(map[string]json.RawMessage, len(call.params)+2)
			for k, v := range call.params {
				args[k] = v
			}
			args["time_start"], _ = json.Marshal(m.WindowStart)
			args["time_end"], _ = json.Marshal(m.WindowEnd)
			raw, _ := json.Marshal(args)
			attempts++
			result, err := (&TraceQuery{}).Execute(ctx, raw)
			oldPath, _, current := ctx.Mutable.ResolveTraceQueryWindowReplay(call.ref)
			newPath, _, newCurrent := ctx.Mutable.ResolveTraceQueryWindowReplay(result.TraceQueryWindowReplay)
			if err != nil || !result.Success || !current || !newCurrent || newPath != oldPath || result.TraceViewCancellation != nil {
				m.SkipReason = types.TraceSupplementReasonExecutionFailed
				m.SkippedViews = append(m.SkippedViews, call.view)
				if result.TraceViewCancellation != nil {
					m.CanceledViews = append(m.CanceledViews, call.view)
					if ctx.Ctx.Err() != nil {
						m.SkipReason = traceSupplementMemberCancelReason(ctx.Ctx)
					}
				}
				continue
			}
			results = append(results, result)
			out.Executed = append(out.Executed, call.view)
			m.Views = append(m.Views, call.view)
			m.ViewValueObservations = append(m.ViewValueObservations, traceSupplementValueObservationCount(result))
			m.ViewObservationFamilies = append(m.ViewObservationFamilies, traceSupplementViewFamilyCensus(result))
			logging.Info("[trace_supplement] primary_window view=%s window=%.9g..%.9g (same source and effective filters; original exploration retained)", call.view, m.WindowStart, m.WindowEnd)
		}
		meta.MemberWindows = append(meta.MemberWindows, m)
		meta.Views = append(meta.Views, m.Views...)
		meta.ViewValueObservations = append(meta.ViewValueObservations, m.ViewValueObservations...)
		meta.ViewObservationFamilies = append(meta.ViewObservationFamilies, m.ViewObservationFamilies...)
	}
	if missing == 0 {
		return out, false
	}
	if censusLiteWanted {
		result, _, ok := traceSupplementExecuteCensusLite(ctx, path, types.TraceSupplementReasonWindowedCensusAbsent, nil)
		if ok {
			results = append(results, result)
			out.Executed = append(out.Executed, "event_search")
			meta.CensusLite = true
			meta.CensusLitePattern = traceSupplementCensusLitePattern
		}
	}
	out.Elapsed = time.Since(start)
	meta.ElapsedMS = out.Elapsed.Milliseconds()
	ctx.Mutable.SetSystemTraceSupplement(meta, results)
	return out, true
}
