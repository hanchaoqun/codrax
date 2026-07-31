package tool

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	runtimeTraceBlockingCoverageAuthorityBlockID  = "runtime_trace_blocking_coverage_authority"
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
	title := "关键口径：目标阻塞仅为观测下界"
	if !zh {
		title = "Key caliber: target blocking is an observed lower bound"
	}
	return insertRuntimeTraceLeadAuthorityBlock(doc, types.AnswerBlock{
		ID:    runtimeTraceBlockingCoverageAuthorityBlockID,
		Kind:  types.BlockCaveat,
		Title: title,
		Text:  strings.Join(caveats, "\n\n"),
	})
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
	title := "关键口径：blocked_reason 不是完整 sleep 分区"
	if !zh {
		title = "Key caliber: blocked_reason is not a complete sleep partition"
	}
	return insertRuntimeTraceLeadAuthorityBlock(doc, types.AnswerBlock{
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
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind == types.BlockSummary {
			insertAt = i + 1
			break
		}
	}
	for insertAt < len(doc.Blocks) {
		id := strings.TrimSpace(doc.Blocks[insertAt].ID)
		if id != runtimeTraceBlockingCoverageAuthorityBlockID &&
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
