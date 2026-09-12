package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A member may waive work only through an addressed, current-source parent
// result. Native recursive rows retain their smaller occurrence windows; those
// local measurements do not redefine the parent request window or target.
func traceSupplementMemberSourceMatches(ctx *types.BusContext, source types.ObservationSourceRef, path string, member types.RuntimeArtifactTimeWindow, target traceQueryRequestTarget) bool {
	if source.Kind != types.ObservationSourceRuntimeArtifact || strings.TrimSpace(source.Path) == "" ||
		filepath.Clean(resolveToolPath(ctx, source.Path)) != filepath.Clean(path) ||
		strings.TrimSpace(source.QueryScopeID) == "" || (source.PayloadRef == "" && source.RawRef == "") ||
		!source.QueryWindowKnown || !source.QueryLineRangeKnown || source.QueryLineStart != 0 || source.QueryLineEnd != 0 ||
		!types.TraceCausalProjectionPrincipalValueSameWindow(source.QueryWindowStartTs, source.QueryWindowEndTs, *member.TimeStart, *member.TimeEnd) {
		return false
	}
	expectedScope := target.TargetScope
	if expectedScope == "" {
		expectedScope = "thread"
	}
	if source.QueryTargetScope != expectedScope {
		return false
	}
	if target.PID > 0 {
		return source.QueryTargetPID == target.PID
	}
	return source.QueryTargetPID == 0 && strings.EqualFold(strings.TrimSpace(source.QueryTargetThread), strings.TrimSpace(target.Thread))
}

func traceSupplementMemberLedger(ctx *types.BusContext, ledger types.ObservationLedger, path string, member types.RuntimeArtifactTimeWindow, target traceQueryRequestTarget) types.ObservationLedger {
	out := types.ObservationLedger{AnchorUserEntities: ledger.AnchorUserEntities}
	for _, record := range ledger.Records {
		if record.Origin == types.AnswerEvidenceOriginRuntimeArtifact && types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) &&
			traceSupplementMemberSourceMatches(ctx, record.SourceRef, path, member, target) {
			out.Records = append(out.Records, record)
		}
	}
	return out
}

func traceSupplementMemberContext(ctx *types.BusContext, member types.RuntimeArtifactTimeWindow) *types.BusContext {
	out := ctx.ShallowClone()
	ir := types.AnalysisIR{}
	if ctx.AnalysisIR != nil {
		ir = *ctx.AnalysisIR
	} else if rm := ctx.Mutable.RequestModel(); rm != nil {
		ir.RequestModel = *rm
	}
	scope := *traceSupplementRequestedArtifactScope(ctx)
	scope.TimeWindows = nil
	scope.TimeStart, scope.TimeEnd, scope.SourceQuote = member.TimeStart, member.TimeEnd, member.SourceQuote
	ir.RequestModel.RuntimeArtifactScopeProfile = &scope
	out.AnalysisIR = &ir
	return out
}

// FrameEvidenceAuthority is result-wide. An auto-window aggregate containing
// other parent results must not lend its "present" bit to this member.
func traceSupplementMemberFramePresent(ctx *types.BusContext, input types.ObservationLedgerInput, path string, member types.RuntimeArtifactTimeWindow, target traceQueryRequestTarget) bool {
	for _, result := range append(append([]types.ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...) {
		if result.TraceEvidenceAuthority == nil || result.TraceEvidenceAuthority.FrameEvidenceStatus != "present" || len(result.Observations) == 0 {
			continue
		}
		all := true
		for _, record := range result.Observations {
			if !traceSupplementMemberSourceMatches(ctx, record.SourceRef, path, member, target) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func traceSupplementMemberCancelReason(ctx context.Context) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return types.TraceSupplementReasonDurationBudgetExceeded
	}
	return types.TraceSupplementReasonCanceledByCaller
}

// The caller already acquired the task latch, resolved the source, and made ONE
// deadline context. Never recursively invoke RunTraceQuerySystemSupplement or
// SetSystemTraceSupplement here per member: either loses another member's work.
func runTraceSupplementMembers(ctx *types.BusContext, path, sourceLabel string, input types.ObservationLedgerInput, ledger types.ObservationLedger,
	members []types.RuntimeArtifactTimeWindow, target traceQueryRequestTarget, targetSource string, targetOK bool, out TraceQuerySupplementOutcome) TraceQuerySupplementOutcome {
	// The engine publishes the physical path (for example macOS /private/var
	// rather than /var). Resolve the already authorized source once; no
	// candidate capture is chosen from an observation or from its display label.
	if physical, err := filepath.EvalSymlinks(path); err == nil {
		path = physical
	}
	start := time.Now()
	meta := types.SystemTraceSupplementMeta{TargetPID: target.PID, TargetThread: target.Thread, TargetSource: targetSource,
		RequestedArtifactScope: types.RuntimeArtifactScopeExplicitWindow, DurationBudgetS: traceSupplementMaxDuration.Seconds()}
	results := []types.ToolResult{}
	unique := map[[2]float64]int{}
	warm := false
	for _, result := range input.ToolResults {
		if !result.Success || result.ToolName != "trace_query" {
			continue
		}
		for _, record := range result.Observations {
			if record.SourceRef.Path != "" && filepath.Clean(resolveToolPath(ctx, record.SourceRef.Path)) == filepath.Clean(path) {
				warm = true
				break
			}
		}
	}
	coldExceeded := false
	if !warm {
		if info, err := os.Stat(path); err == nil {
			coldExceeded = info.Size() > traceSupplementMaxColdBytes
		}
	}
	ctx.Mutable.BeginSystemTraceSupplementExecution()
	defer ctx.Mutable.EndSystemTraceSupplementExecution()
	for _, member := range members {
		key := [2]float64{*member.TimeStart, *member.TimeEnd}
		if earlier, ok := unique[key]; ok {
			// This is shared execution metadata, not a second execution or an
			// assertion that repeated requested members have unique ownership.
			meta.MemberWindows = append(meta.MemberWindows, meta.MemberWindows[earlier])
			continue
		}
		unique[key] = len(meta.MemberWindows)
		m := types.SystemTraceSupplementWindowMeta{WindowStart: key[0], WindowEnd: key[1]}
		memberCtx := traceSupplementMemberContext(ctx, member)
		present := traceSupplementFamilyPresence{}
		if targetOK {
			present = traceSupplementFamilies(traceSupplementMemberLedger(ctx, ledger, path, member, target))
		}
		views := traceSupplementViewsForRequest(memberCtx, present, traceSupplementVsyncFamilyHit(ctx), traceSupplementMemberFramePresent(ctx, input, path, member, target))
		switch {
		case len(views) == 0:
			m.SkipReason = types.TraceSupplementReasonFamiliesPresent
		case !targetOK:
			m.SkipReason, m.SkippedViews = types.TraceSupplementReasonNoTypedTarget, views
		case key[1]-key[0] > traceSupplementMaxWindowSpanS:
			m.SkipReason, m.SkippedViews, m.WindowBudgetS = types.TraceSupplementReasonWindowSpanExceeded, views, traceSupplementMaxWindowSpanS
		case coldExceeded:
			m.SkipReason, m.SkippedViews = types.TraceSupplementReasonColdBudgetExceeded, views
		default:
			memberStart := time.Now()
			for i, view := range views {
				if ctx.Ctx.Err() != nil {
					m.SkipReason, m.SkippedViews = traceSupplementMemberCancelReason(ctx.Ctx), append([]string(nil), views[i:]...)
					break
				}
				// Only member endpoints and the common target are supplied. Model
				// calls from another source may not lend platform/clock parameters.
				raw, err := json.Marshal(traceSupplementCallParams{View: view, PID: target.PID, Thread: target.Thread,
					TargetScope: traceSupplementTargetScopeForView(target, view), TimeStart: key[0], TimeEnd: key[1]})
				if err != nil {
					m.SkipReason, m.SkippedViews = types.TraceSupplementReasonExecutionFailed, append(m.SkippedViews, view)
					continue
				}
				result, execErr := (&TraceQuery{}).Execute(ctx, raw)
				if execErr != nil {
					m.SkipReason, m.SkippedViews = types.TraceSupplementReasonExecutionFailed, append(m.SkippedViews, view)
					continue
				}
				if result.TraceViewCancellation != nil {
					m.CanceledViews = append(m.CanceledViews, view)
					m.SkipReason = traceSupplementMemberCancelReason(ctx.Ctx)
				}
				if !result.Success || (result.TraceViewCancellation != nil && len(result.Observations) == 0) {
					if result.TraceViewCancellation == nil {
						m.SkipReason, m.SkippedViews = types.TraceSupplementReasonExecutionFailed, append(m.SkippedViews, view)
					}
					continue
				}
				ctx.Mutable.RegisterTraceQueryBlobRefsFromToolResult(result)
				m.Views = append(m.Views, view)
				m.ViewValueObservations = append(m.ViewValueObservations, traceSupplementValueObservationCount(result))
				m.ViewObservationFamilies = append(m.ViewObservationFamilies, traceSupplementViewFamilyCensus(result))
				results = append(results, result)
				if traceSupplementAfterViewHook != nil {
					traceSupplementAfterViewHook(view)
				}
			}
			m.ElapsedMS = time.Since(memberStart).Milliseconds()
			if len(m.Views) == 0 && m.SkipReason == "" {
				m.SkipReason = types.TraceSupplementReasonExecutionFailed
			}
		}
		meta.Views = append(meta.Views, m.Views...)
		meta.ViewValueObservations = append(meta.ViewValueObservations, m.ViewValueObservations...)
		meta.ViewObservationFamilies = append(meta.ViewObservationFamilies, m.ViewObservationFamilies...)
		meta.SkippedViews = append(meta.SkippedViews, m.SkippedViews...)
		meta.CanceledViews = append(meta.CanceledViews, m.CanceledViews...)
		if m.SkipReason != "" && m.SkipReason != types.TraceSupplementReasonFamiliesPresent && meta.SkipReason == "" {
			meta.SkipReason = m.SkipReason
		}
		meta.MemberWindows = append(meta.MemberWindows, m)
	}
	// The generator census is whole-capture work, independent of member count.
	// It rides the same deadline and is appended once, never replacing members.
	censusPresent := false
	for _, record := range ledger.Records {
		if record.SourceRef.Path != "" && filepath.Clean(resolveToolPath(ctx, record.SourceRef.Path)) == filepath.Clean(path) && traceSupplementObservationsCarryVsyncCensus([]types.ObservationRecord{record}) {
			censusPresent = true
			break
		}
	}
	if traceSupplementVsyncFamilyHit(ctx) && !censusPresent && !traceSupplementResultsCarryVsyncCensus(results) && ctx.Ctx.Err() == nil {
		if result, _, ok := traceSupplementExecuteCensusLite(ctx, path, types.TraceSupplementReasonWindowedCensusAbsent); ok {
			results = append(results, result)
			meta.CensusLite, meta.CensusLitePattern = true, traceSupplementCensusLitePattern
		}
	}
	out.Elapsed, out.SkipReason = time.Since(start), meta.SkipReason
	out.Executed = append([]string(nil), meta.Views...)
	if meta.CensusLite {
		out.Executed = append(out.Executed, "event_search")
	}
	if len(out.Executed) == 0 && out.SkipReason == "" {
		out.SkipReason = types.TraceSupplementReasonFamiliesPresent
	}
	meta.ElapsedMS = out.Elapsed.Milliseconds()
	ctx.Mutable.SetSystemTraceSupplement(meta, results)
	logging.Info("[trace_supplement] member_windows=%d executed=%s total_elapsed=%s source=%s (one attempt/deadline; original member bounds retained)", len(members), strings.Join(out.Executed, ","), out.Elapsed.Round(time.Millisecond), sourceLabel)
	return out
}
