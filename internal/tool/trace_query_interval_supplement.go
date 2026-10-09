package tool

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Native interval navigation is independent of primary-window replay: the
// latter changes only time endpoints, whereas this lane elects a raw interval
// view observed by a trusted producer. It never elects a statistical protocol,
// owner, relation, causal conclusion or a new user answer obligation.
func traceIntervalSupplementRequest(ctx *types.BusContext) bool {
	var rm *types.RequestModel
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	} else if ctx.Mutable != nil {
		rm = ctx.Mutable.RequestModel()
	}
	if rm == nil || rm.RuntimeQuestionProfile == nil ||
		!rm.RuntimeQuestionProfile.BoundedFactSet() ||
		rm.RuntimeQuestionProfile.RequestsRuntimeWorkRelation() ||
		rm.RuntimeQuestionProfile.RequestsFrameCausality() ||
		!rm.RuntimeTargetProfile.ExplicitlyHasNoNamedTarget() {
		return false
	}
	wanted := false
	for _, family := range rm.RuntimeQuestionProfile.FactFamilies {
		switch family {
		case types.RuntimeQuestionFactOtherObservedValue, types.RuntimeQuestionFactFrequencyResidency:
			wanted = true
		case types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactOccurrenceTime:
			// Orthogonal display dimensions do not elect an interval view.
		default:
			return false
		}
	}
	for _, target := range rm.RuntimeTargets {
		if target.Active() && target.Source == "user_explicit" {
			return false
		}
	}
	if dimensions := rm.RequestedAnswerDimensions; dimensions != nil {
		for _, dimension := range dimensions.Dimensions {
			if !dimension.Required {
				continue
			}
			switch dimension.Role {
			case types.RequestedAnswerDimensionRelationPath, types.RequestedAnswerDimensionRuntimeWorkRelation,
				types.RequestedAnswerDimensionTargetEffectVerdict, types.RequestedAnswerDimensionCausalAttribution,
				types.RequestedAnswerDimensionCausalContributorSet:
				return false
			}
		}
	}
	return wanted
}

// Permit only explicitly enumerated context-independent parameters. Unknown
// future selectors must fail closed rather than being silently dropped when
// switching from point discovery to interval observations. Empty owner fields
// are inserted by the native replay producer, not authority to select an owner.
func traceIntervalSupplementParams(raw json.RawMessage, start, end float64) (map[string]json.RawMessage, string, bool) {
	var args map[string]json.RawMessage
	var p traceQueryParams
	if json.Unmarshal(raw, &args) != nil || args == nil || json.Unmarshal(raw, &p) != nil ||
		!p.TimeStart.Set() || !p.TimeEnd.Set() || p.TimeStart.Seconds() != start || p.TimeEnd.Seconds() != end {
		return nil, "", false
	}
	for key := range args {
		switch key {
		case "source", "path", "view", "time_start", "time_end", "limit":
		case "pid":
			if p.PID.Int() != 0 {
				return nil, "", false
			}
			delete(args, key)
		case "thread":
			if p.Thread != "" {
				return nil, "", false
			}
			delete(args, key)
		case "target_scope":
			if p.TargetScope != "" {
				return nil, "", false
			}
			delete(args, key)
		default:
			return nil, "", false
		}
	}
	return args, tracequery.CanonicalViewName(p.View), true
}

type traceIntervalSupplementCall struct {
	view   string
	params map[string]json.RawMessage
	ref    types.TraceQueryWindowReplayRef
}

func traceIntervalSupplementCalls(ctx *types.BusContext, path string, input types.ObservationLedgerInput, start, end float64) []traceIntervalSupplementCall {
	physical, ok := traceQueryRequestWindowPathIdentity(ctx, path)
	if !ok {
		return nil
	}
	complete, candidates := map[string]bool{}, map[string]traceIntervalSupplementCall{}
	for _, result := range input.ToolResults {
		if !result.Success || result.TraceViewCancellation != nil || result.ReusedFromRunMemo {
			continue
		}
		actual, raw, views, current := ctx.Mutable.ResolveTraceIntervalNavigation(contextFromBus(ctx), result.TraceQueryWindowReplay)
		if !current || actual != physical {
			continue
		}
		args, original, compatible := traceIntervalSupplementParams(raw, start, end)
		if !compatible {
			continue
		}
		for _, view := range views {
			if tracequery.ValidateViewName(view) != nil || tracequery.CanonicalViewName(view) != view {
				continue
			}
			if original == view {
				complete[view] = true
			} else if _, exists := candidates[view]; !exists {
				candidates[view] = traceIntervalSupplementCall{view, args, result.TraceQueryWindowReplay}
			}
		}
	}
	var calls []traceIntervalSupplementCall
	for view, call := range candidates {
		if !complete[view] {
			calls = append(calls, call)
		}
	}
	sort.Slice(calls, func(i, j int) bool { return calls[i].view < calls[j].view })
	return calls
}

// Called only after the existing causal/target and primary-window lanes are
// idle. It shares their task deadline, span fuse, two-call budget and dedicated
// publication lane; neither model transcript nor accepted completion changes.
func runTraceIntervalSupplement(ctx *types.BusContext, path, sourceLabel string, input types.ObservationLedgerInput, out TraceQuerySupplementOutcome, censusLiteWanted bool) (TraceQuerySupplementOutcome, bool) {
	if !traceIntervalSupplementRequest(ctx) || !traceQueryRequestWindowSourceMatches(ctx, path, sourceLabel) {
		return out, false
	}
	start, end, explicit := traceSupplementRequestedArtifactScope(ctx).ExplicitTimeWindow()
	if !explicit {
		return out, false
	}
	calls := traceIntervalSupplementCalls(ctx, path, input, start, end)
	if len(calls) == 0 {
		return out, false
	}
	meta := types.SystemTraceSupplementMeta{RequestedArtifactScope: types.RuntimeArtifactScopeExplicitWindow,
		WindowStart: start, WindowEnd: end, TargetSource: "native_interval_navigation", DurationBudgetS: traceSupplementMaxDuration.Seconds()}
	var results []types.ToolResult
	began := time.Now()
	limit := 2
	if censusLiteWanted {
		limit--
	}
	attempts := 0
	ctx.Mutable.BeginSystemTraceSupplementExecution()
	defer ctx.Mutable.EndSystemTraceSupplementExecution()
	for _, call := range calls {
		reason := ""
		switch {
		case end-start > traceSupplementMaxWindowSpanS:
			reason, meta.WindowBudgetS = types.TraceSupplementReasonWindowSpanExceeded, traceSupplementMaxWindowSpanS
		case contextFromBus(ctx).Err() != nil:
			reason = traceSupplementMemberCancelReason(contextFromBus(ctx))
		case attempts >= limit:
			reason = types.TraceSupplementReasonQueryBudgetExceeded
		}
		if reason != "" {
			meta.SkipReason, out.SkipReason = reason, reason
			meta.SkippedViews = append(meta.SkippedViews, call.view)
			continue
		}
		before, _, _, current := ctx.Mutable.ResolveTraceIntervalNavigation(contextFromBus(ctx), call.ref)
		if !current {
			meta.SkipReason, out.SkipReason = types.TraceSupplementReasonExecutionFailed, types.TraceSupplementReasonExecutionFailed
			meta.SkippedViews = append(meta.SkippedViews, call.view)
			continue
		}
		args := make(map[string]json.RawMessage, len(call.params))
		for key, value := range call.params {
			args[key] = value
		}
		args["view"], _ = json.Marshal(call.view)
		raw, _ := json.Marshal(args)
		attempts++
		result, err := (&TraceQuery{}).Execute(ctx, raw)
		after, _, _, stillCurrent := ctx.Mutable.ResolveTraceIntervalNavigation(contextFromBus(ctx), call.ref)
		produced, _, views, producedCurrent := ctx.Mutable.ResolveTraceIntervalNavigation(contextFromBus(ctx), result.TraceQueryWindowReplay)
		self := false
		for _, view := range views {
			self = self || view == call.view
		}
		if err != nil || !result.Success || result.TraceViewCancellation != nil || !stillCurrent || !producedCurrent || !self || before != after || before != produced {
			meta.SkipReason, out.SkipReason = types.TraceSupplementReasonExecutionFailed, types.TraceSupplementReasonExecutionFailed
			meta.SkippedViews = append(meta.SkippedViews, call.view)
			if result.TraceViewCancellation != nil {
				meta.CanceledViews = append(meta.CanceledViews, call.view)
			}
			continue
		}
		results = append(results, result)
		out.Executed = append(out.Executed, call.view)
		meta.Views = append(meta.Views, call.view)
		meta.ViewValueObservations = append(meta.ViewValueObservations, traceSupplementValueObservationCount(result))
		meta.ViewObservationFamilies = append(meta.ViewObservationFamilies, traceSupplementViewFamilyCensus(result))
		logging.Info("[trace_supplement] native_interval view=%s window=%.9g..%.9g (same source; raw observation only)", call.view, start, end)
	}
	if censusLiteWanted {
		result, _, ok := traceSupplementExecuteCensusLite(ctx, path, types.TraceSupplementReasonWindowedCensusAbsent, nil)
		if ok {
			results = append(results, result)
			out.Executed = append(out.Executed, "event_search")
			meta.CensusLite, meta.CensusLitePattern = true, traceSupplementCensusLitePattern
		}
	}
	out.Elapsed = time.Since(began)
	meta.ElapsedMS = out.Elapsed.Milliseconds()
	ctx.Mutable.SetSystemTraceSupplement(meta, results)
	return out, true
}
