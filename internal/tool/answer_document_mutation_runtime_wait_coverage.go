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
)

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
	blocking := types.BuildTraceBlockingWallClockAuthorities(
		ledger, &ctx.AnalysisIR.RequestModel,
	)
	requests := types.BuildTraceIPCRequestCensusAuthorities(
		ledger, &ctx.AnalysisIR.RequestModel,
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
	if !runtimeTraceFullReportMaterializationAllowed(ctx) {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit,
	))
	states := types.BuildTraceTargetStateScopeAuthorities(types.CompileTraceCausalProjectionSet(ledger))
	if len(states) == 0 {
		return false
	}
	waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &ctx.AnalysisIR.RequestModel)
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	rows := make([]string, 0, len(states))
	for i, state := range states {
		if i >= 4 {
			break
		}
		var row string
		if zh {
			row = fmt.Sprintf(
				"目标线程状态主值：artifact=%s，selected_window=%.6f..%.6f，subject=%s，scope=target_thread_wall_clock_partition；running=%.3fms，runnable=%.3fms，sleep=%.3fms（其中 S 态 IO等待=%.3fms，已包含在 sleep），non_io_d_state=%.3fms，io_wait=%.3fms，partition_total=%.3fms",
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
			)
		} else {
			row = fmt.Sprintf(
				"Target-thread state authority: artifact=%s, selected_window=%.6f..%.6f, subject=%s, scope=target_thread_wall_clock_partition; running=%.3fms, runnable=%.3fms, sleep=%.3fms (S-state IO wait=%.3fms, already included in sleep), non_io_d_state=%.3fms, io_wait=%.3fms, partition_total=%.3fms",
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
			)
		}
		if wait, ok := matchingTraceTargetWaitSummary(state, waits); ok {
			pairedStateMS := state.DStateMS + state.IOWaitMS + state.SleepIOWaitMS
			identity := ""
			if math.Abs(wait.WallClockMS-pairedStateMS) <= 0.002 {
				if zh {
					identity = "；occurrence_wall_clock_sum 与上述 D/IO 配对状态账一致"
				} else {
					identity = "; occurrence_wall_clock_sum matches the paired D/IO state account above"
				}
			}
			if zh {
				row += fmt.Sprintf(
					"；target_wait_occurrence_roster=complete，occurrences=%d，d_state_occurrences=%d，io_wait_occurrences=%d，sleep_iowait_occurrences=%d，other_wait_occurrences=%d，occurrence_wall_clock_sum=%.3fms%s",
					wait.Count,
					wait.DStateOccurrences,
					wait.IOWaitOccurrences,
					wait.SleepIOWaitOccurrences,
					wait.OtherWaitOccurrences,
					wait.WallClockMS,
					identity,
				)
			} else {
				row += fmt.Sprintf(
					"; target_wait_occurrence_roster=complete, occurrences=%d, d_state_occurrences=%d, io_wait_occurrences=%d, sleep_iowait_occurrences=%d, other_wait_occurrences=%d, occurrence_wall_clock_sum=%.3fms%s",
					wait.Count,
					wait.DStateOccurrences,
					wait.IOWaitOccurrences,
					wait.SleepIOWaitOccurrences,
					wait.OtherWaitOccurrences,
					wait.WallClockMS,
					identity,
				)
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return false
	}
	title := "系统权威主值：目标线程状态与等待发生"
	lead := "以下值来自同一显式窗的 typed 调度状态账；后续模型正文若给出冲突的次数、总量或量纲，以本块为准。blocked_reason 的记录数/Σdelay、IPC 传输延迟与线程状态墙钟不得互相替代。"
	if !zh {
		title = "System authority: target-thread state and wait occurrences"
		lead = "These values come from the typed scheduler-state account for the same selected window. If later model prose conflicts on count, total, or caliber, this block is authoritative. blocked_reason record count/Σdelay, IPC transport latency, and thread-state wall clock are not interchangeable."
	}
	return insertRuntimeTraceLeadAuthorityBlock(doc, types.AnswerBlock{
		ID:    runtimeTraceTargetStateAuthorityBlockID,
		Kind:  types.BlockCaveat,
		Title: title,
		Text:  lead + "\n\n" + strings.Join(rows, "\n\n"),
	})
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
	rows := runtimeTraceBlockedReasonCensusCalibers(ledger, &ctx.AnalysisIR.RequestModel)
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
		if id != runtimeTraceBlockingCoverageAuthorityBlockID &&
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
