package tool

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	runtimeTraceBlockingCoverageAuthorityBlockID  = "runtime_trace_blocking_coverage_authority"
	runtimeTraceTargetStateAuthorityBlockID       = "runtime_trace_target_state_authority"
	runtimeTraceBlockedReasonCensusCaliberBlockID = "runtime_trace_blocked_reason_census_caliber"
	runtimeTraceTypedTargetConsensusSource        = "typed_entity_supplement_consensus"
)

// runtimeTraceAuthorityRequestModel returns the typed user-target authority
// used by deterministic answer-side numeric compilers.
//
// Analyzer-emitted RuntimeTargets remain the primary lane. When that lane is
// absent (a demonstrated classifier emission gap), answer authority may be
// recovered only through agreement between two structured runtime faces:
//
//   - exactly one analyzer entity that passes the strict name-pid parser; and
//   - an actually executed system supplement whose canonical positive target
//     PID is identical and came from the cursor/entities fallback lane.
//
// Model cursor identity alone is never promoted. Raw request text and model
// answer prose are never read. The returned RequestModel is a private clone,
// so this answer-time consensus cannot mutate user intent or affect future
// trace_query inheritance/supplement selection.
func runtimeTraceAuthorityRequestModel(ctx *types.BusContext) *types.RequestModel {
	if ctx == nil {
		return nil
	}
	var base *types.RequestModel
	if ctx.AnalysisIR != nil {
		cloned := ctx.AnalysisIR.RequestModel
		base = &cloned
	}
	if base == nil && ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			cloned := *rm
			base = &cloned
		}
	}
	if base == nil {
		return nil
	}
	hasUserTarget := func(rm *types.RequestModel) bool {
		if rm == nil {
			return false
		}
		for _, target := range rm.RuntimeTargets {
			if types.RuntimeTargetIsExplorationCursorSource(target.Source) {
				continue
			}
			scope := traceSupplementRuntimeTargetScope(target.Kind)
			if traceQueryTypedRuntimeTargetSafe(traceQueryRequestTarget{
				PID: target.PID, Thread: strings.TrimSpace(target.Thread), TargetScope: scope,
			}) {
				return true
			}
		}
		return false
	}
	if hasUserTarget(base) {
		return base
	}
	if ctx.Mutable == nil {
		return nil
	}
	if rm := ctx.Mutable.RequestModel(); hasUserTarget(rm) {
		cloned := *rm
		return &cloned
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || meta.TargetPID <= 0 || meta.TargetPID > types.RuntimeTargetMaxPID ||
		len(meta.Views) == 0 {
		return nil
	}
	supplementExecuted := false
	for _, result := range ctx.Mutable.SystemTraceSupplementResults() {
		if result.Success {
			supplementExecuted = true
			break
		}
	}
	if !supplementExecuted {
		return nil
	}
	switch strings.TrimSpace(meta.TargetSource) {
	case "cursor", traceSupplementTargetSourceEntitiesFallback:
	default:
		return nil
	}
	entityTarget, ok := traceSupplementEntitiesFallbackTarget(ctx)
	if !ok || entityTarget.PID != meta.TargetPID {
		return nil
	}
	cloned := *base
	cloned.RuntimeTargets = append(append([]types.RuntimeTarget(nil), base.RuntimeTargets...), types.RuntimeTarget{
		Kind:       types.RuntimeTargetKindThread,
		PID:        meta.TargetPID,
		Thread:     strings.TrimSpace(entityTarget.Thread),
		Source:     runtimeTraceTypedTargetConsensusSource,
		Confidence: 1,
	})
	return &cloned
}

// RuntimeTraceAuthorityRequestModelForAgentContext exposes the same private,
// typed answer-authority target resolution to the finalizer prompt. Keeping
// this as an adapter over the BusContext implementation prevents the
// pre-finalize guidance and post-finalize system materializer from drifting
// onto different target identities. It never reads raw request/model prose
// and never mutates either context.
func RuntimeTraceAuthorityRequestModelForAgentContext(ctx *types.AgentContext) *types.RequestModel {
	if ctx == nil {
		return nil
	}
	return runtimeTraceAuthorityRequestModel(&types.BusContext{
		AnalysisIR: ctx.AnalysisIR,
		Mutable:    ctx.Mutable,
	})
}

// materializeRuntimeTraceBlockingCoverageAuthorityCaveat publishes the
// lower-bound boundary already carried by typed target-owned blocking
// intervals. It never inspects model prose: the caveat is present whenever
// the deterministic authority itself is capacity-truncated, even if the model
// happened to use careful wording. This keeps request counts, transport
// latency and proven target blocking wall clock in separate calibers.
func materializeRuntimeTraceBlockingCoverageAuthorityCaveat(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if !runtimeTraceFullReportMaterializationAllowed(ctx) {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit,
	))
	authorityRM := runtimeTraceAuthorityRequestModel(ctx)
	blocking := types.BuildTraceBlockingWallClockAuthorities(
		ledger, authorityRM,
	)
	requests := types.BuildTraceIPCRequestCensusAuthorities(
		ledger, authorityRM,
	)
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	var caveats []string
	for _, authority := range blocking {
		if authority.CoverageStatus != "lower_bound_capacity_truncated" {
			continue
		}
		var caveat string
		if zh {
			caveat = fmt.Sprintf(
				"目标阻塞覆盖权限：artifact=%s，selected_window=%s，subject=%s，blocking_type=%s；observed_blocking_lower_bound=%.3fms，observed_occurrences>=%d，coverage_status=%s；这里只证明已列出的观测下界，不授权全窗阻塞总量、唯一 occurrence，也不授权“其余请求没有阻塞”的结论。同步 IPC request 数、send-to-receive transport latency 与目标阻塞墙钟/次数是不同量纲，不能互相补齐",
				authority.ArtifactLabel,
				authority.SelectedWindow,
				authority.Subject,
				authority.Type,
				authority.ObservedMS,
				len(authority.Occurrences),
				authority.CoverageStatus,
			)
		} else {
			caveat = fmt.Sprintf(
				"Target blocking coverage authority: artifact=%s, selected_window=%s, subject=%s, blocking_type=%s; observed_blocking_lower_bound=%.3fms, observed_occurrences>=%d, coverage_status=%s. This proves only the listed observed lower bound; it does not authorize an exhaustive window total, a unique occurrence, or a claim that every other request caused no blocking. Synchronous IPC request count, send-to-receive transport latency, target blocking wall clock, and blocking occurrence count are separate calibers and cannot complete one another",
				authority.ArtifactLabel,
				authority.SelectedWindow,
				authority.Subject,
				authority.Type,
				authority.ObservedMS,
				len(authority.Occurrences),
				authority.CoverageStatus,
			)
		}
		if request, ok := matchingTraceIPCRequestCensusAuthority(authority, requests); ok {
			if zh {
				caveat += fmt.Sprintf(
					"；同窗 IPC request census=%s：requests=%d，sync=%d，oneway=%d，unknown=%d；完整请求数仍不等于完整阻塞 occurrence 数",
					request.CoverageStatus,
					request.TotalRequests,
					request.SyncRequests,
					request.OnewayRequests,
					request.UnknownRequests,
				)
			} else {
				caveat += fmt.Sprintf(
					"; same-window IPC request census=%s: requests=%d, sync=%d, oneway=%d, unknown=%d; a complete request count is still not a complete blocking-occurrence count",
					request.CoverageStatus,
					request.TotalRequests,
					request.SyncRequests,
					request.OnewayRequests,
					request.UnknownRequests,
				)
			}
		}
		caveats = append(caveats, caveat)
	}
	if len(caveats) == 0 {
		return false
	}
	title := "关键口径：目标阻塞仅为观测下界"
	lead := "本块是目标阻塞数值与覆盖范围的系统 authority；后续模型正文若给出冲突的总量、次数或“其余均无阻塞”结论，以本块为准。"
	if !zh {
		title = "Key caliber: target blocking is an observed lower bound"
		lead = "This block is the system authority for target-blocking values and coverage. If later model prose conflicts on totals, occurrence counts, or claims that every other request did not block, this block takes precedence."
	}
	return insertRuntimeTraceLeadAuthorityBlock(doc, types.AnswerBlock{
		ID:    runtimeTraceBlockingCoverageAuthorityBlockID,
		Kind:  types.BlockCaveat,
		Title: title,
		Text:  lead + "\n\n" + strings.Join(caveats, "\n\n"),
	})
}

// materializeRuntimeTraceTargetStateAuthorityBlock publishes the selected
// target thread's exact state partition and, when the same deterministic
// result carries a complete occurrence roster, the occurrence count and wall
// clock sum. It is a principal value card: blocked_reason record delays,
// transport latency and free-form summaries cannot replace these typed values.
func materializeRuntimeTraceTargetStateAuthorityBlock(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if !runtimeTracePrincipalValueMaterializationAllowed(ctx) {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit,
	))
	authorityRM := runtimeTraceAuthorityRequestModel(ctx)
	states := types.BuildTraceTargetStateScopeAuthorities(types.CompileTraceCausalProjectionSet(ledger))
	targetStates := make([]types.TraceTargetStateScopeAuthority, 0, len(states))
	for _, state := range states {
		if types.ObservationRecordMatchesUserRuntimeTarget(types.ObservationRecord{
			Subject: state.Subject,
		}, authorityRM) {
			targetStates = append(targetStates, state)
		}
	}
	states = targetStates
	waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, authorityRM)
	if len(states) == 0 && len(waits) == 0 {
		return false
	}
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	rows := make([]string, 0, len(states))
	matchedWaits := map[string]bool{}
	for i, state := range states {
		if i >= 4 {
			break
		}
		var row string
		if zh {
			row = fmt.Sprintf(
				"目标线程状态主值：artifact=%s，selected_window=%.6f..%.6f，subject=%s，scope=target_thread_wall_clock_partition；running=%.3fms，runnable=%.3fms，sleep=%.3fms（其中 S 态 IO等待=%.3fms，已包含在 sleep），non_io_d_state=%.3fms，io_wait=%.3fms，accounted_total=%.3fms，window=%.3fms，coverage_status=%s",
				state.ArtifactLabel,
				state.WindowStartTs,
				state.WindowEndTs,
				state.Subject,
				state.RunningMS,
				state.RunnableMS,
				state.SleepMS,
				state.SleepIOWaitMS,
				state.DStateMS,
				state.IOWaitMS,
				state.TotalMS,
				state.WindowMS,
				state.CoverageStatus,
			)
		} else {
			row = fmt.Sprintf(
				"Target-thread state authority: artifact=%s, selected_window=%.6f..%.6f, subject=%s, scope=target_thread_wall_clock_partition; running=%.3fms, runnable=%.3fms, sleep=%.3fms (S-state IO wait=%.3fms, already included in sleep), non_io_d_state=%.3fms, io_wait=%.3fms, accounted_total=%.3fms, window=%.3fms, coverage_status=%s",
				state.ArtifactLabel,
				state.WindowStartTs,
				state.WindowEndTs,
				state.Subject,
				state.RunningMS,
				state.RunnableMS,
				state.SleepMS,
				state.SleepIOWaitMS,
				state.DStateMS,
				state.IOWaitMS,
				state.TotalMS,
				state.WindowMS,
				state.CoverageStatus,
			)
		}
		if state.CoverageStatus == "partial_unaccounted" {
			if zh {
				row += fmt.Sprintf(
					"；unaccounted=%.3fms，未覆盖部分没有足够 typed 调度边界，不能分配到任一状态",
					state.UnaccountedMS,
				)
			} else {
				row += fmt.Sprintf(
					"; unaccounted=%.3fms; the uncovered interval has insufficient typed scheduler boundaries and cannot be assigned to any state",
					state.UnaccountedMS,
				)
			}
		}
		if state.HeadCarryMS > 0 && state.HeadCarryState != "" {
			if zh {
				row += fmt.Sprintf("；head_carry=%.3fms(state=%s，已包含在对应状态)", state.HeadCarryMS, state.HeadCarryState)
			} else {
				row += fmt.Sprintf("; head_carry=%.3fms(state=%s, already included in that state)", state.HeadCarryMS, state.HeadCarryState)
			}
		}
		if state.TailOpenMS > 0 && state.TailOpenState != "" {
			if zh {
				row += fmt.Sprintf("；tail_open=%.3fms(state=%s，已包含在对应状态)", state.TailOpenMS, state.TailOpenState)
			} else {
				row += fmt.Sprintf("; tail_open=%.3fms(state=%s, already included in that state)", state.TailOpenMS, state.TailOpenState)
			}
		}
		if wait, ok := matchingTraceTargetWaitSummary(state, waits); ok {
			row += runtimeTraceTargetWaitSummarySuffix(wait, &state, zh)
			matchedWaits[wait.RecordID] = true
		}
		rows = append(rows, row)
	}
	for _, wait := range waits {
		if len(rows) >= 4 || matchedWaits[wait.RecordID] {
			continue
		}
		var row string
		if zh {
			row = fmt.Sprintf(
				"目标等待发生主值：artifact=%s，selected_window=%.6f..%.6f，subject=%s",
				wait.ArtifactLabel,
				wait.WindowStartTs,
				wait.WindowEndTs,
				wait.Subject,
			)
		} else {
			row = fmt.Sprintf(
				"Target-wait occurrence authority: artifact=%s, selected_window=%.6f..%.6f, subject=%s",
				wait.ArtifactLabel,
				wait.WindowStartTs,
				wait.WindowEndTs,
				wait.Subject,
			)
		}
		rows = append(rows, row+runtimeTraceTargetWaitSummarySuffix(wait, nil, zh))
	}
	if len(rows) == 0 {
		return false
	}
	title := "系统权威主值：目标线程状态与等待发生"
	lead := "以下值来自 typed 调度状态账和同一结果内 producer 配对的完整等待 roster；后续模型正文若给出冲突的次数、总量、明细或量纲，以本块为准。blocked_reason 的记录数/Σdelay、IPC 传输延迟与线程状态墙钟不得互相替代；其他 observation/census 的 capacity_truncated 不会降级这里已标为 complete 的 roster。"
	if !zh {
		title = "System authority: target-thread state and wait occurrences"
		lead = "These values come from the typed scheduler-state account and the complete producer-paired wait roster in one result. If later model prose conflicts on count, total, rows, or caliber, this block is authoritative. blocked_reason record count/Σdelay, IPC transport latency, and thread-state wall clock are not interchangeable; capacity_truncated on another observation/census does not downgrade a roster marked complete here."
	}
	return insertRuntimeTraceLeadAuthorityBlock(doc, types.AnswerBlock{
		ID:    runtimeTraceTargetStateAuthorityBlockID,
		Kind:  types.BlockCaveat,
		Title: title,
		Text:  lead + "\n\n" + strings.Join(rows, "\n\n"),
	})
}

func runtimeTraceTargetWaitSummarySuffix(
	wait types.TraceTargetWaitSummaryAuthority,
	state *types.TraceTargetStateScopeAuthority,
	zh bool,
) string {
	identity := ""
	if state != nil {
		pairedStateMS := state.DStateMS + state.IOWaitMS + state.SleepIOWaitMS
		if math.Abs(wait.WallClockMS-pairedStateMS) <= 0.002 {
			if zh {
				identity = "；occurrence_wall_clock_sum 与上述 D/IO 配对状态账一致"
			} else {
				identity = "; occurrence_wall_clock_sum matches the paired D/IO state account above"
			}
		}
	}
	var b strings.Builder
	if zh {
		fmt.Fprintf(
			&b,
			"；target_wait_occurrence_roster=complete，roster_scope=producer_paired_complete，occurrences=%d，d_state_occurrences=%d，io_wait_occurrences=%d，sleep_iowait_occurrences=%d，other_wait_occurrences=%d，occurrence_wall_clock_sum=%.3fms，wait_callers=%s，unrelated_capacity_truncation_does_not_downgrade=true%s",
			wait.Count,
			wait.DStateOccurrences,
			wait.IOWaitOccurrences,
			wait.SleepIOWaitOccurrences,
			wait.OtherWaitOccurrences,
			wait.WallClockMS,
			runtimeTraceWaitCallerRoster(wait.Callers, zh),
			identity,
		)
	} else {
		fmt.Fprintf(
			&b,
			"; target_wait_occurrence_roster=complete, roster_scope=producer_paired_complete, occurrences=%d, d_state_occurrences=%d, io_wait_occurrences=%d, sleep_iowait_occurrences=%d, other_wait_occurrences=%d, occurrence_wall_clock_sum=%.3fms, wait_callers=%s, unrelated_capacity_truncation_does_not_downgrade=true%s",
			wait.Count,
			wait.DStateOccurrences,
			wait.IOWaitOccurrences,
			wait.SleepIOWaitOccurrences,
			wait.OtherWaitOccurrences,
			wait.WallClockMS,
			runtimeTraceWaitCallerRoster(wait.Callers, zh),
			identity,
		)
	}
	for _, occurrence := range wait.Occurrences {
		b.WriteString("\n- target_wait_occurrence=")
		b.WriteString(occurrence.CanonicalLine())
	}
	return b.String()
}

func runtimeTraceWaitCallerRoster(callers []string, zh bool) string {
	if len(callers) == 0 {
		if zh {
			return "无已解析 caller"
		}
		return "none resolved"
	}
	return strings.Join(callers, "|")
}

func matchingTraceTargetWaitSummary(
	state types.TraceTargetStateScopeAuthority,
	waits []types.TraceTargetWaitSummaryAuthority,
) (types.TraceTargetWaitSummaryAuthority, bool) {
	var matches []types.TraceTargetWaitSummaryAuthority
	for _, wait := range waits {
		stateArtifact := strings.TrimSpace(state.ArtifactLabel)
		waitArtifact := strings.TrimSpace(wait.ArtifactLabel)
		if strings.EqualFold(strings.TrimSpace(state.Subject), strings.TrimSpace(wait.Subject)) &&
			math.Abs(state.WindowStartTs-wait.WindowStartTs) <= 0.001 &&
			math.Abs(state.WindowEndTs-wait.WindowEndTs) <= 0.001 &&
			(stateArtifact == "" || waitArtifact == "" || strings.EqualFold(stateArtifact, waitArtifact)) {
			matches = append(matches, wait)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return types.TraceTargetWaitSummaryAuthority{}, false
}

func matchingTraceIPCRequestCensusAuthority(
	blocking types.TraceBlockingWallClockAuthority,
	requests []types.TraceIPCRequestCensusAuthority,
) (types.TraceIPCRequestCensusAuthority, bool) {
	for _, request := range requests {
		if strings.EqualFold(strings.TrimSpace(request.ArtifactLabel), strings.TrimSpace(blocking.ArtifactLabel)) &&
			strings.TrimSpace(request.SelectedWindow) == strings.TrimSpace(blocking.SelectedWindow) &&
			strings.EqualFold(strings.TrimSpace(request.Subject), strings.TrimSpace(blocking.Subject)) {
			return request, true
		}
	}
	return types.TraceIPCRequestCensusAuthority{}, false
}

type runtimeTraceBlockedReasonCensusCaliber struct {
	artifact       string
	selectedWindow string
	subject        string
	count          int
	callers        string
	callerOverflow int
}

// materializeRuntimeTraceBlockedReasonCensusCaliberCaveat prevents a complete
// caller-record census from being promoted into a complete scheduler-state
// duration partition. The record count and its self-reported delay sums remain
// valuable exact evidence, but they explain only the records they own. A
// state interval roster is a separate typed surface.
func materializeRuntimeTraceBlockedReasonCensusCaliberCaveat(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	if !runtimeTraceFullReportMaterializationAllowed(ctx) {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit,
	))
	rows := runtimeTraceBlockedReasonCensusCalibers(ledger, runtimeTraceAuthorityRequestModel(ctx))
	if len(rows) == 0 {
		return false
	}
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	caveats := make([]string, 0, len(rows))
	for _, row := range rows {
		var caveat string
		if zh {
			caveat = fmt.Sprintf(
				"阻塞原因普查口径：artifact=%s，selected_window=%s，subject=%s，blocked_reason_records=%d，callers=%s，caliber=caller_linked_record_census_not_scheduler_state_partition；这些计数及 Σdelay 只说明其自身已记录的 caller 关联证据，不能单独证明每一段 sleep 都有该 caller，也不能外推为整个 sleep 墙钟均由这些原因构成",
				row.artifact, row.selectedWindow, row.subject, row.count, row.callers,
			)
			if row.callerOverflow > 0 {
				caveat += fmt.Sprintf("；caller_roster_overflow=%d，展示的 caller roster 还不是完整列表", row.callerOverflow)
			}
		} else {
			caveat = fmt.Sprintf(
				"Blocked-reason census caliber: artifact=%s, selected_window=%s, subject=%s, blocked_reason_records=%d, callers=%s, caliber=caller_linked_record_census_not_scheduler_state_partition. These counts and Σdelay values describe only their recorded caller-linked evidence; by themselves they do not prove that every sleep interval has one of those callers or that the entire sleep wall clock is explained by those reasons",
				row.artifact, row.selectedWindow, row.subject, row.count, row.callers,
			)
			if row.callerOverflow > 0 {
				caveat += fmt.Sprintf("; caller_roster_overflow=%d, so the displayed caller roster is also incomplete", row.callerOverflow)
			}
		}
		caveats = append(caveats, caveat)
	}
	if len(caveats) == 0 {
		return false
	}
	title := "关键口径：blocked_reason 不是完整 sleep 分区"
	lead := "本块是 blocked_reason 记录口径的系统 authority；后续模型正文若把记录数或 Σdelay 当作线程状态发生次数/墙钟总量，以本块和同窗目标状态主值为准。"
	if !zh {
		title = "Key caliber: blocked_reason is not a complete sleep partition"
		lead = "This block is the system authority for blocked_reason record caliber. If later model prose treats record count or Σdelay as thread-state occurrence count or wall-clock total, use this block and the same-window target-state authority instead."
	}
	return insertRuntimeTraceLeadAuthorityBlock(doc, types.AnswerBlock{
		ID:    runtimeTraceBlockedReasonCensusCaliberBlockID,
		Kind:  types.BlockCaveat,
		Title: title,
		Text:  lead + "\n\n" + strings.Join(caveats, "\n\n"),
	})
}

func runtimeTraceBlockedReasonCensusCalibers(
	ledger types.ObservationLedger,
	rm *types.RequestModel,
) []runtimeTraceBlockedReasonCensusCaliber {
	if rm == nil || len(rm.RuntimeTargets) == 0 {
		return nil
	}
	byKey := map[string]runtimeTraceBlockedReasonCensusCaliber{}
	for _, record := range ledger.Records {
		if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
			record.GroundingPolicy != types.ClaimGroundingHard ||
			strings.TrimSpace(record.Predicate) != "blocked_reason_census" ||
			!types.ObservationRecordMatchesUserRuntimeTarget(record, rm) {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(record.Value))
		if err != nil || count <= 0 || record.ResultCount == nil || *record.ResultCount != count {
			continue
		}
		artifact := strings.TrimSpace(record.SourceRef.ArtifactID)
		if artifact == "" {
			artifact = strings.TrimSpace(record.SourceRef.Path)
		}
		window := strings.TrimSpace(runtimeTraceObservationRichNoteValue(
			record.RichNotes, types.TraceNoteKeySelectedWindow,
		))
		callers := strings.TrimSpace(runtimeTraceObservationRichNoteValue(
			record.RichNotes, types.TraceNoteKeyBlockedReasonCensus,
		))
		if artifact == "" || window == "" || strings.TrimSpace(record.Subject) == "" || callers == "" {
			continue
		}
		overflow, _ := strconv.Atoi(strings.TrimSpace(runtimeTraceObservationRichNoteValue(
			record.RichNotes, types.TraceNoteKeyBlockedReasonCensusOverflow,
		)))
		row := runtimeTraceBlockedReasonCensusCaliber{
			artifact:       artifact,
			selectedWindow: window,
			subject:        strings.TrimSpace(record.Subject),
			count:          count,
			callers:        callers,
			callerOverflow: overflow,
		}
		key := strings.Join([]string{
			strings.ToLower(row.artifact),
			row.selectedWindow,
			strings.ToLower(row.subject),
			strconv.Itoa(row.count),
			row.callers,
			strconv.Itoa(row.callerOverflow),
		}, "\x00")
		byKey[key] = row
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]runtimeTraceBlockedReasonCensusCaliber, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func insertRuntimeTraceLeadAuthorityBlock(doc *types.AnswerDocumentV2, block types.AnswerBlock) bool {
	if doc == nil || strings.TrimSpace(block.ID) == "" || strings.TrimSpace(block.Text) == "" {
		return false
	}
	if answerDocumentHasRuntimeTraceSystemBlockID(doc, block.ID) {
		return false
	}
	if len(doc.Blocks) >= maxBlocksPerDoc {
		return false
	}
	markRuntimeTraceSystemBlock(&block)
	insertAt := 0
	for insertAt < len(doc.Blocks) {
		id := strings.TrimSpace(doc.Blocks[insertAt].ID)
		if id != runtimeTraceFrequencyAuthorityBlockID &&
			id != runtimeTraceBlockingCoverageAuthorityBlockID &&
			id != runtimeTraceTargetStateAuthorityBlockID &&
			id != runtimeTraceBlockedReasonCensusCaliberBlockID {
			break
		}
		insertAt++
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
	copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
	doc.Blocks[insertAt] = block
	return true
}
