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

// runtimeTraceAuthorityRequestModel returns the typed user-target authority
// used by deterministic answer-side numeric compilers.
//
// Only analyzer-emitted user_explicit RuntimeTargets under a named target
// declaration carry answer authority. Analyzer entity strings, model cursors,
// and supplement metadata are exploration hints and are never promoted into a
// user identity by agreement/consensus. The returned RequestModel is a private
// clone and raw request/model prose is never read.
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
		if rm.RuntimeTargetProfile != nil && !rm.RuntimeTargetProfile.NamedTarget() {
			return false
		}
		for _, target := range rm.RuntimeTargets {
			if strings.TrimSpace(target.Source) != "user_explicit" {
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
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); hasUserTarget(rm) {
			cloned := *rm
			return &cloned
		}
	}
	return nil
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
				"目标阻塞观测：工件=%s，窗口=%s，线程=%s，类型=%s；当前至少观测到 %d 段、合计 %.3fms。结果达到容量上限，因此这里只能作为下界，不能据此给出全窗阻塞总量、唯一发生段，或断言其余请求都没有阻塞。同步 IPC 请求数、send-to-receive 传输延迟与目标阻塞墙钟/次数是不同口径，不能互相补齐",
				authority.ArtifactLabel,
				authority.SelectedWindow,
				authority.Subject,
				authority.Type,
				len(authority.Occurrences),
				authority.ObservedMS,
			)
		} else {
			intervalWord := "intervals"
			if len(authority.Occurrences) == 1 {
				intervalWord = "interval"
			}
			caveat = fmt.Sprintf(
				"Target blocking observation: artifact=%s, window=%s, thread=%s, type=%s; at least %d %s totaling %.3fms were observed. The result reached its capacity limit, so this is a lower bound, not an exhaustive window total or a unique occurrence, and it does not show that every other request caused no blocking. Synchronous IPC request count, send-to-receive transport latency, target blocking wall clock, and blocking occurrence count are separate measures and cannot complete one another",
				authority.ArtifactLabel,
				authority.SelectedWindow,
				authority.Subject,
				authority.Type,
				len(authority.Occurrences),
				intervalWord,
				authority.ObservedMS,
			)
		}
		if request, ok := matchingTraceIPCRequestCensusAuthority(authority, requests); ok {
			if zh {
				caveat += fmt.Sprintf(
					"；同窗共记录 IPC 请求 %d 次（同步 %d、单向 %d、类型未知 %d）。即使请求清单完整，也不等于阻塞发生段清单完整",
					request.TotalRequests,
					request.SyncRequests,
					request.OnewayRequests,
					request.UnknownRequests,
				)
			} else {
				caveat += fmt.Sprintf(
					"; the same window contains %d IPC requests (%d synchronous, %d oneway, %d unknown). A complete request roster is still not a complete blocking-interval roster",
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
	title := "目标阻塞的观测范围"
	if !zh {
		title = "Observed scope of target blocking"
	}
	return insertRuntimeTraceDataBoundaryBlock(doc, types.AnswerBlock{
		ID:    runtimeTraceBlockingCoverageAuthorityBlockID,
		Kind:  types.BlockCaveat,
		Title: title,
		Text:  strings.Join(caveats, "\n\n"),
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
				"目标线程状态：工件=%s，窗口=%.6f..%.6f，线程=%s；running %.3fms，runnable %.3fms，sleep %.3fms（其中 S 态 IO 等待 %.3fms，已包含在 sleep），非 IO D-state %.3fms，io_wait %.3fms；已归账 %.3fms / 窗口 %.3fms，覆盖=%s",
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
				runtimeTraceTargetStateCoverageLabel(state.CoverageStatus, true),
			)
		} else {
			row = fmt.Sprintf(
				"Target-thread state: artifact=%s, window=%.6f..%.6f, thread=%s; running %.3fms, runnable %.3fms, sleep %.3fms (including %.3fms of S-state IO wait), non-IO D-state %.3fms, io_wait %.3fms; accounted %.3fms / window %.3fms, coverage=%s",
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
				runtimeTraceTargetStateCoverageLabel(state.CoverageStatus, false),
			)
		}
		if state.CoverageStatus == "partial_unaccounted" {
			if zh {
				row += fmt.Sprintf(
					"；另有 %.3fms 未归账：该区间缺少足够的调度边界，不能分配到任一状态",
					state.UnaccountedMS,
				)
			} else {
				row += fmt.Sprintf(
					"; %.3fms remains unaccounted because the interval lacks sufficient scheduler boundaries to assign a state",
					state.UnaccountedMS,
				)
			}
		}
		if state.HeadCarryMS > 0 && state.HeadCarryState != "" {
			if zh {
				row += fmt.Sprintf("；窗口起点继承 %.3fms（状态=%s，已计入对应状态）", state.HeadCarryMS, state.HeadCarryState)
			} else {
				row += fmt.Sprintf("; %.3fms is carried in at the window head (state=%s, already included)", state.HeadCarryMS, state.HeadCarryState)
			}
		}
		if state.TailOpenMS > 0 && state.TailOpenState != "" {
			if zh {
				row += fmt.Sprintf("；窗口尾部开放 %.3fms（状态=%s，已计入对应状态）", state.TailOpenMS, state.TailOpenState)
			} else {
				row += fmt.Sprintf("; %.3fms remains open at the window tail (state=%s, already included)", state.TailOpenMS, state.TailOpenState)
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
		scopeLabel := ""
		switch wait.RequestedScopeRole {
		case types.TraceTargetWaitScopeRequestedPrincipal:
			if zh {
				scopeLabel = "请求主范围；"
			} else {
				scopeLabel = "requested scope; "
			}
		case types.TraceTargetWaitScopeSupportingExploration:
			if zh {
				scopeLabel = "探索子范围；"
			} else {
				scopeLabel = "supporting exploration scope; "
			}
		}
		var row string
		if zh {
			row = fmt.Sprintf(
				"目标等待：%s工件=%s，窗口=%.6f..%.6f，线程=%s",
				scopeLabel,
				wait.ArtifactLabel,
				wait.WindowStartTs,
				wait.WindowEndTs,
				wait.Subject,
			)
		} else {
			row = fmt.Sprintf(
				"Target waits: %sartifact=%s, window=%.6f..%.6f, thread=%s",
				scopeLabel,
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
	title := "目标线程状态与等待明细"
	lead := "以下为所选窗口内的调度状态账；若存在请求主范围与探索子范围，请求主范围先列，探索子范围只用于下钻，不能替代主范围的次数、总量或清单。若同时列出逐段等待，次数和总量来自同一查询结果的完整配对。D-state、io_wait 与 S 态 IO 等待是分开的记录类型；blocked_reason 记录数、IPC 传输延迟和线程状态墙钟也属于不同口径，不能互相替代。"
	if !zh {
		title = "Target-thread states and wait details"
		lead = "This is the scheduler-state account for the selected window. When both a requested scope and supporting exploration scopes exist, the requested scope is listed first; exploration scopes are drill-down only and cannot replace its count, total, or roster. When per-interval waits are listed, their count and total come from the complete pairing in the same query result. D-state, io_wait, and S-state IO wait are separate record kinds; blocked_reason record counts, IPC transport latency, and thread-state wall clock are also different measures and are not interchangeable."
	}
	return insertRuntimeTraceDataBoundaryBlock(doc, types.AnswerBlock{
		ID:    runtimeTraceTargetStateAuthorityBlockID,
		Kind:  types.BlockCaveat,
		Title: title,
		Text:  lead + "\n\n" + strings.Join(rows, "\n\n"),
	})
}

func runtimeTraceTargetStateCoverageLabel(status string, zh bool) string {
	switch strings.TrimSpace(status) {
	case "complete":
		if zh {
			return "完整"
		}
		return "complete"
	case "partial_unaccounted":
		if zh {
			return "部分（存在未归账区间）"
		}
		return "partial (unaccounted interval present)"
	default:
		if zh {
			return "未说明"
		}
		return "not stated"
	}
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
				identity = "；逐段合计与上述 D/IO 状态账一致"
			} else {
				identity = "; the interval sum matches the paired D/IO state account above"
			}
		}
	}
	var b strings.Builder
	if zh {
		fmt.Fprintf(
			&b,
			"；等待明细完整，共 %d 段（D-state %d、io_wait %d、S 态 IO 等待 %d、其他 %d），墙钟合计 %.3fms，已解析 caller：%s%s",
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
			"; the wait roster is complete: %d intervals (D-state %d, io_wait %d, S-state IO wait %d, other %d), totaling %.3fms wall clock; resolved callers: %s%s",
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
		if zh {
			fmt.Fprintf(
				&b,
				"\n- 第 %d 段：%s，%s..%s，%.3fms，iowait=%s，caller=%s",
				occurrence.Ordinal,
				occurrence.State,
				occurrence.StartToken(),
				occurrence.EndToken(),
				occurrence.DurationM,
				occurrence.IOWait,
				occurrence.Caller,
			)
		} else {
			fmt.Fprintf(
				&b,
				"\n- Interval %d: %s, %s..%s, %.3fms, iowait=%s, caller=%s",
				occurrence.Ordinal,
				occurrence.State,
				occurrence.StartToken(),
				occurrence.EndToken(),
				occurrence.DurationM,
				occurrence.IOWait,
				occurrence.Caller,
			)
		}
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
				"工件=%s，窗口=%s，线程=%s：记录到 %d 条 blocked_reason，caller=%s。这些记录及其 Σdelay 只描述已建立 caller 关联的事件，不是完整的线程调度状态分区；不能据此认定每一段 sleep 都具有这些 caller，也不能把整个 sleep 墙钟都归因于它们",
				row.artifact, row.selectedWindow, row.subject, row.count, row.callers,
			)
			if row.callerOverflow > 0 {
				caveat += fmt.Sprintf("；另有 %d 个 caller 未在此紧凑列表中展示", row.callerOverflow)
			}
		} else {
			caveat = fmt.Sprintf(
				"Artifact=%s, window=%s, thread=%s: %d blocked_reason records were observed, with callers=%s. These records and their Σdelay describe only events with a caller association; they are not a complete scheduler-state partition. They do not show that every sleep interval has one of these callers or that these reasons explain the entire sleep wall clock",
				row.artifact, row.selectedWindow, row.subject, row.count, row.callers,
			)
			if row.callerOverflow > 0 {
				caveat += fmt.Sprintf("; %d additional callers are omitted from this compact list", row.callerOverflow)
			}
		}
		caveats = append(caveats, caveat)
	}
	if len(caveats) == 0 {
		return false
	}
	title := "blocked_reason 的记录口径"
	if !zh {
		title = "How to read blocked_reason records"
	}
	return insertRuntimeTraceDataBoundaryBlock(doc, types.AnswerBlock{
		ID:    runtimeTraceBlockedReasonCensusCaliberBlockID,
		Kind:  types.BlockCaveat,
		Title: title,
		Text:  strings.Join(caveats, "\n\n"),
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

func insertRuntimeTraceDataBoundaryBlock(doc *types.AnswerDocumentV2, block types.AnswerBlock) bool {
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
	doc.Blocks = append(doc.Blocks, block)
	return true
}
