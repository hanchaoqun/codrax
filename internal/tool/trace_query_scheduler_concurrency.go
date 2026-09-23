package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Publish the engine's closed-interval population, not an estimate of every
// runnable/running thread. No display row creates a wait or causal credential.
func traceQuerySchedulerConcurrencyState(state string) string {
	switch state {
	case tracequery.SchedulerConcurrencyStateRunnable:
		return "同时就绪线程数"
	case tracequery.SchedulerConcurrencyStateRunning:
		return "同时运行线程数"
	default:
		return "调度线程数（状态未说明）"
	}
}

func traceQuerySchedulerConcurrencySummary(group tracequery.SchedulerConcurrencyGroup) string {
	label := traceQuerySchedulerConcurrencyState(group.State)
	if v := group.Values; v != nil {
		return fmt.Sprintf("%s：峰值=%d，全窗平均=%.9g；有线程时长=%.9g ms，线程时间合计=%.9g thread·ms；仅已确认闭合区间，不是目标等待或根因", label, v.PeakThreads, v.MeanThreads, v.BusyMs, v.ThreadMs)
	}
	return label + "：未能测量（" + traceQuerySchedulerConcurrencyReason(group.ValuesUnavailableReason) + "；不能按零处理）；仅已确认闭合区间，不是目标等待或根因"
}

func traceQuerySchedulerConcurrencyReason(reason string) string {
	labels := map[string]string{
		"line_bounds_take_precedence":                 "行范围优先，未建立时间分母",
		"finite_positive_time_window_not_determined":  "未确定有效起止时间",
		"no_accepted_closed_intervals":                "没有已确认闭合区间",
		"non_finite_aggregate":                        "统计值超出有效数值范围",
		"requested_window_exceeds_artifact":           "请求窗口超出已记录的时间范围",
		"source_audit_limited_to_retained_rows":       "来源核对仅覆盖保留记录",
		"window_head_not_fully_classified":            "窗口起点的状态未完全确定",
		"open_intervals_excluded":                     "未闭合区间未纳入",
		"cross_source_members_excluded":               "来源冲突区间未纳入",
		"physical_source_unresolved":                  "部分区间未解析到实际来源",
		"ambiguous_thread_lifecycle_excluded":         "线程身份不明确的区间未纳入",
		"invalid_or_contradictory_endpoints_excluded": "无效或矛盾端点未纳入",
		"capture_completeness_not_established":        "未证明采集完整",
	}
	if label := labels[reason]; label != "" {
		return label
	}
	return "统计前提未满足，详见结构化结果"
}

func traceQuerySchedulerConcurrencyBasis(stats *tracequery.SchedulerConcurrencyStats) string {
	window := "unknown"
	if stats.Window != nil {
		window = traceQueryDisplaySeconds(stats.Window.StartTs) + ".." + traceQueryDisplaySeconds(stats.Window.EndTs)
	}
	return "selected_window=" + window + "; " + stats.Population + "; " + stats.ThreadScope
}

func traceQuerySchedulerConcurrencyCoverage(c tracequery.SchedulerConcurrencyCoverage) string {
	return fmt.Sprintf("status=%s; candidate=%d accepted=%d open=%d", c.Status, c.CandidateIntervals, c.AcceptedIntervals, c.OpenEndedIntervals)
}

func traceQuerySchedulerConcurrencyExclusions(c tracequery.SchedulerConcurrencyCoverage) string {
	return fmt.Sprintf("source_conflict=%d unresolved_source=%d identity_excluded=%d identity_tids=%d invalid=%d", c.SourceConflictIntervals, c.UnresolvedSourceIntervals, c.IdentityExcludedIntervals, c.IdentityExcludedTIDs, c.InvalidIntervals)
}

func traceQuerySchedulerConcurrencyTimeline(group tracequery.SchedulerConcurrencyGroup) string {
	var parts []string
	for _, segment := range group.Segments {
		part := fmt.Sprintf("[%s,%s):%d", traceQueryDisplaySeconds(segment.StartTs), traceQueryDisplaySeconds(segment.EndTs), segment.Threads)
		if len(strings.Join(append(parts, part), ";")) > 90 {
			break
		}
		parts = append(parts, part)
	}
	return fmt.Sprintf("seconds:threads; omitted=%d; %s", group.OmittedSegments+len(group.Segments)-len(parts), strings.Join(parts, ";"))
}

func traceQuerySchedulerConcurrencyWindowNotes(stats *tracequery.SchedulerConcurrencyStats) []string {
	if stats.Window == nil {
		return nil
	}
	return []string{types.TraceNoteKeySelectedWindow + "=" + traceQueryDisplaySeconds(stats.Window.StartTs) + ".." + traceQueryDisplaySeconds(stats.Window.EndTs)}
}

func traceQuerySchedulerConcurrencyNotes(stats *tracequery.SchedulerConcurrencyStats, group tracequery.SchedulerConcurrencyGroup) []string {
	notes := traceQueryTypedKVNotes([][2]string{
		{types.TraceNoteKeySchedulerConcurrencyGroup, fmt.Sprintf("state=%s; intervals=%d threads=%d", group.State, group.AcceptedIntervalCount, group.ThreadCount)},
		{types.TraceNoteKeySchedulerConcurrencyBasis, traceQuerySchedulerConcurrencyBasis(stats)},
		{types.TraceNoteKeySchedulerConcurrencyCoverage, traceQuerySchedulerConcurrencyCoverage(group.Coverage)},
		{types.TraceNoteKeySchedulerConcurrencyExclusions, traceQuerySchedulerConcurrencyExclusions(group.Coverage)},
		{types.TraceNoteKeySchedulerConcurrencyTimeline, traceQuerySchedulerConcurrencyTimeline(group)},
		{types.TraceNoteKeySchedulerConcurrencyScope, fmt.Sprintf("groups=%d omitted_groups=%d; lines=%d..%d; unavailable=%s; %s", stats.GroupCount, stats.OmittedGroups, stats.LineStart, stats.LineEnd, group.ValuesUnavailableReason, strings.Join(group.Coverage.Reasons, ","))},
	})
	return append(notes, traceQuerySchedulerConcurrencyWindowNotes(stats)...)
}

func writeTraceSchedulerConcurrency(b *strings.Builder, stats *tracequery.SchedulerConcurrencyStats) {
	if stats == nil {
		return
	}
	window := "未确定"
	if stats.Window != nil {
		window = traceQueryDisplaySeconds(stats.Window.StartTs) + ".." + traceQueryDisplaySeconds(stats.Window.EndTs) + " 秒"
	}
	fmt.Fprintf(b, "- 调度并发统计：窗口 %s；仅已确认闭合区间，统计范围为全部正线程号；分组 %d，未展示 %d；不能据此保证采集完整或断言系统空闲。\n", window, stats.GroupCount, stats.OmittedGroups)
	if stats.LineStart > 0 || stats.LineEnd > 0 {
		fmt.Fprintf(b, "  - 所选原始行范围：%d..%d；行范围优先，不能据此推定完整时间窗内的平均线程数。\n", stats.LineStart, stats.LineEnd)
	}
	if stats.WindowUnavailableReason != "" {
		fmt.Fprintf(b, "  - 时间窗口径：%s。\n", traceQuerySchedulerConcurrencyReason(stats.WindowUnavailableReason))
	}
	c := stats.Coverage
	if len(c.Reasons) > 0 {
		var reasons []string
		for _, reason := range c.Reasons {
			reasons = append(reasons, traceQuerySchedulerConcurrencyReason(reason))
		}
		fmt.Fprintf(b, "  - 覆盖限制：%s。\n", strings.Join(reasons, "；"))
	}
	fmt.Fprintf(b, "  - 区间核对：候选 %d，接纳 %d，未闭合 %d，来源冲突 %d，来源未解 %d，身份排除区间 %d、线程 %d，无效 %d；未闭合和排除项不补成零。\n", c.CandidateIntervals, c.AcceptedIntervals, c.OpenEndedIntervals, c.SourceConflictIntervals, c.UnresolvedSourceIntervals, c.IdentityExcludedIntervals, c.IdentityExcludedTIDs, c.InvalidIntervals)
	for _, group := range stats.Groups {
		fmt.Fprintf(b, "  - 来源 %s；%s；接纳区间 %d、线程 %d；未展示分段 %d。\n", sanitizeForBanner(group.SourcePath), traceQuerySchedulerConcurrencySummary(group), group.AcceptedIntervalCount, group.ThreadCount, group.OmittedSegments)
		for _, segment := range group.Segments {
			fmt.Fprintf(b, "    - [%s,%s) 秒：%d 个线程（已确认闭合区间）。\n", traceQueryDisplaySeconds(segment.StartTs), traceQueryDisplaySeconds(segment.EndTs), segment.Threads)
		}
	}
}

func traceQueryTypedSchedulerConcurrencyObservations(stats *tracequery.SchedulerConcurrencyStats, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if stats == nil {
		return nil
	}
	var out []types.ObservationRecord
	for i, group := range stats.Groups {
		groupRef := ref
		groupRef.Path = group.SourcePath
		identity, _ := json.Marshal([]string{group.SourcePath, group.State})
		digest := sha256.Sum256(identity)
		value := ""
		if group.Values != nil {
			value = strconv.Itoa(group.Values.PeakThreads)
		}
		out = append(out, types.ObservationRecord{
			ID:     fmt.Sprintf("trace_query:%s#scheduler_concurrency:%d", scope, i+1),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: groupRef,
			ClaimKey: "scheduler_concurrency:" + hex.EncodeToString(digest[:]), Subject: group.State,
			Predicate: "scheduler_concurrency", Value: value, Unit: "threads",
			Summary: traceQuerySchedulerConcurrencySummary(group), RichNotes: traceQuerySchedulerConcurrencyNotes(stats, group),
			ObservedAt: at, Confidence: .72,
		})
	}
	c := stats.Coverage
	out = append(out, types.ObservationRecord{
		ID:     fmt.Sprintf("trace_query:%s#scheduler_concurrency_coverage", scope),
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
		ClaimKey: "scheduler_concurrency_coverage", Subject: "scheduler", Predicate: "scheduler_concurrency_coverage",
		Value: c.Status, Unit: "status", Summary: "调度区间核对；排除项与未闭合区间不是零，不保证采集完整或构成根因",
		RichNotes: append(traceQueryTypedKVNotes([][2]string{
			{types.TraceNoteKeySchedulerConcurrencyBasis, traceQuerySchedulerConcurrencyBasis(stats)},
			{types.TraceNoteKeySchedulerConcurrencyCoverage, traceQuerySchedulerConcurrencyCoverage(c)},
			{types.TraceNoteKeySchedulerConcurrencyExclusions, traceQuerySchedulerConcurrencyExclusions(c)},
			{types.TraceNoteKeySchedulerConcurrencyScope, fmt.Sprintf("groups=%d omitted_groups=%d; lines=%d..%d; unavailable=%s; %s", stats.GroupCount, stats.OmittedGroups, stats.LineStart, stats.LineEnd, stats.WindowUnavailableReason, strings.Join(c.Reasons, ","))},
		}), traceQuerySchedulerConcurrencyWindowNotes(stats)...), ObservedAt: at, Confidence: 1,
	})
	return out
}
