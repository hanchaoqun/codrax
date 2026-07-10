package tracediag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func writeV2ProvenanceHeader(rw *reportWriter, opts Options, script *Script, tracePath string, info os.FileInfo, flavorHint tracequery.TraceFlavor, at time.Time, sourceVersion tracequery.TraceSourceVersion, expanded int) {
	rw.line("# codrax tracediag 自动采集报告")
	rw.line(fmt.Sprintf("codrax_version=%s build_time=%s", opts.Version, opts.BuildTime))
	rw.line(fmt.Sprintf("generated_at=%s", at.Format(time.RFC3339)))
	// Keep the v2 automatic-collection header on the same round-trip-safe
	// provenance contract as v1: reports are customer-return artifacts, so
	// local absolute paths must never leave the collection machine. The source
	// universe fingerprint below remains the exact reconciliation authority.
	rw.line(fmt.Sprintf("trace=%s primary_size_bytes=%d source_universe_bytes=%d", filepath.Base(tracePath), info.Size(), sourceVersion.SourceBytes()))
	rw.line(fmt.Sprintf("source_fingerprint=%s source_lock=tracequery_source_universe source_lock_status=validated", sourceVersion.Fingerprint()))
	rw.line(fmt.Sprintf("trace_flavor_hint=%s", string(flavorHint)))
	rw.line(fmt.Sprintf("script=%s version=%d discoveries=%d logical_steps=%d expanded_instances=%d", filepath.Base(opts.ScriptPath), script.Version, len(script.Discoveries), len(script.Steps), expanded))
	if strings.TrimSpace(opts.WindowOverride) != "" {
		rw.line(fmt.Sprintf("window_override=%s source=cli_flag target=defaults.window", clampToken(opts.WindowOverride)))
	}
	if script.tidOverrideSet {
		rw.line(fmt.Sprintf("tid_override=%d source=cli_flag target=pid_from:tid", script.tidOverride))
	}
	if strings.TrimSpace(script.Description) != "" {
		rw.line(fmt.Sprintf("description=%s", clampToken(script.Description)))
	}
	rw.line(fmt.Sprintf("limits: generated_windows=%d expanded_steps=%d report_lines=%d validated_worst_report_lines=%d", script.v2Limits.MaxGeneratedWindows, script.v2Limits.MaxExpandedSteps, rw.totalCap, script.v2WorstReportLines))
	rw.line("coverage_scope=selected_candidate_windows_only")
	rw.line("说明: 自动窗由确定性 typed discovery 生成（零 LLM）；它用于补采证据，不等同于根因裁定，也不把未选父窗解释为事件不存在。")
}

func writeV2DiscoverySection(rw *reportWriter, ordinal, total int, outcome v2DiscoveryOutcome) {
	rw.line("")
	rw.line(strings.Repeat("=", 80))
	rw.line(fmt.Sprintf("[自动窗发现 %d/%d] label=%s strategy=%s", ordinal, total, outcome.spec.Label, outcome.spec.Strategy))
	parts := []string{}
	if outcome.spec.Window != "" {
		parts = append(parts, "parent_window="+outcome.spec.Window)
	}
	if outcome.spec.LineStart > 0 || outcome.spec.LineEnd > 0 {
		parts = append(parts, fmt.Sprintf("parent_lines=%d..%d", outcome.spec.LineStart, outcome.spec.LineEnd))
	}
	if len(outcome.spec.Families) > 0 {
		parts = append(parts, "families=["+strings.Join(outcome.spec.Families, ",")+"]")
	} else {
		parts = append(parts, "families=[block,storage](default)")
	}
	parts = append(parts,
		fmt.Sprintf("max_windows=%d", outcome.spec.MaxWindows),
		fmt.Sprintf("max_window_ms=%s", formatFloatToken(outcome.spec.MaxWindowMS)),
		fmt.Sprintf("padding_ms=%s", formatFloatToken(outcome.spec.PaddingMS)),
	)
	rw.line("参数: " + strings.Join(parts, " "))
	rw.line(strings.Repeat("-", 80))
	if outcome.err != nil {
		rw.line(fmt.Sprintf("[发现失败] engine error (verbatim): %v", outcome.err))
		return
	}
	body := renderV2DiscoveryBody(outcome.spec, outcome.result)
	for _, line := range body.lines {
		rw.line(line)
	}
	if body.total > len(body.lines) {
		rw.line(fmt.Sprintf("…发现明细共 %d 行,按帽截断至 %d,余 %d 行未列", body.total, len(body.lines), body.total-len(body.lines)))
	}
}

func renderV2DiscoveryBody(spec *WindowDiscovery, result *tracequery.WindowDiscoveryResult) stepBody {
	sink := &bodySink{cap: spec.EffectiveMaxLines()}
	if result == nil {
		sink.emit("result: unavailable")
		return stepBody{lines: sink.lines, total: sink.total}
	}
	sink.emit(fmt.Sprintf("result: complete=%t identity_complete=%t parse_complete=%t scanned_lines=%d endpoints_replayed=%d endpoints_in_parent=%d retained_candidates=%d generated_windows=%d",
		result.Complete, result.IdentityComplete, result.ParseComplete, result.ScannedLineCount, result.EndpointCount, result.ScopedEndpointCount, result.RetainedCandidateCount, len(result.Windows)))
	sink.emit("selection_basis=" + clampToken(result.SelectionBasis))
	for _, stats := range result.Families {
		sink.emit(fmt.Sprintf("- family=%s replay_endpoints=%d(start=%d done=%d) parent_endpoints=%d(start=%d done=%d) pairs=%d ambiguous_closed=%d ambiguous_open=%d unpaired_done=%d invalid_identity=%d lifecycle_resets=%d time_rollbacks=%d roster_overflow=%d",
			stats.Family, stats.EndpointCount, stats.StartCount, stats.DoneCount,
			stats.ScopedEndpointCount, stats.ScopedStartCount, stats.ScopedDoneCount,
			stats.CompletedPairCount, stats.ClosedAmbiguousCount, stats.OpenAmbiguousCount,
			stats.UnpairedDoneCount, stats.InvalidIdentityCount, stats.LifecycleResetLaneCount,
			stats.TimestampRollbackCount, stats.CohortEventOverflowCount))
	}
	for _, candidate := range result.Candidates {
		sink.emit(fmt.Sprintf("- candidate rank=%d family=%s kind=%s selected=%t collectible=%t single_window=%t required_windows=%d endpoints=%d(start=%d done=%d) max_depth=%d lines=%d-%d core=[%s..%s] identity=%s fingerprint=%s selection=%s blocked=%s",
			candidate.Rank, candidate.Family, candidate.Kind, candidate.Selected,
			candidate.CollectionComplete, candidate.FitsSingleWindow, candidate.RequiredWindowCount,
			candidate.EndpointCount, candidate.StartCount, candidate.DoneCount, candidate.MaxDepth,
			candidate.FirstLine, candidate.LastLine, formatSecondsToken(candidate.CoreStartTs), formatSecondsToken(candidate.CoreEndTs),
			clampToken(candidate.Identity), candidate.IdentityFingerprint, clampToken(candidate.SelectionReason), clampToken(candidate.CollectionBlockedReason)))
	}
	for _, window := range result.Windows {
		sink.emit(fmt.Sprintf("- generated_window ordinal=%d candidate_rank=%d slice=%d family=%s kind=%s window=[%s..%s] width_ms=%s core=[%s..%s] core_lines=%d-%d fingerprint=%s",
			window.Ordinal, window.CandidateRank, window.CandidateWindow, window.Family, window.Kind,
			formatSecondsToken(window.StartTs), formatSecondsToken(window.EndTs), formatFloatToken((window.EndTs-window.StartTs)*1000),
			formatSecondsToken(window.CoreStartTs), formatSecondsToken(window.CoreEndTs), window.CoreLineStart, window.CoreLineEnd, window.IdentityFingerprint))
	}
	for _, caveat := range result.Caveats {
		sink.emit("- caveat: " + clampToken(caveat))
	}
	return stepBody{lines: sink.lines, total: sink.total}
}

func writeV2ExecutionPlan(rw *reportWriter, instances []v2StepInstance) {
	rw.line("")
	rw.line(strings.Repeat("=", 80))
	rw.line("[已解析执行计划]")
	rw.line("coverage_scope=selected_candidate_windows_only; generated windows are system-derived evidence scopes, not user-explicit frame windows")
	for i, instance := range instances {
		if instance.blockedErr != nil {
			rw.line(fmt.Sprintf("- instance=%d logical_step=%d label=%s view=%s status=blocked reason=%s", i+1, instance.logicalOrdinal, instance.logicalLabel, instance.step.View, clampToken(instance.blockedErr.Error())))
			continue
		}
		if origin := instance.step.windowOrigin; origin != nil {
			rw.line(fmt.Sprintf("- instance=%d logical_step=%d label=%s view=%s fanout=%d/%d origin=%s#window%d candidate_rank=%d candidate_slice=%d family=%s kind=%s resolved_window=%s",
				i+1, instance.logicalOrdinal, instance.logicalLabel, instance.step.View, instance.instanceOrdinal, instance.instanceCount,
				origin.DiscoveryLabel, origin.WindowOrdinal, origin.CandidateRank, origin.CandidateWindow, origin.Family, origin.Kind, instance.step.Window))
			continue
		}
		rw.line(fmt.Sprintf("- instance=%d logical_step=%d label=%s view=%s origin=static window=%s", i+1, instance.logicalOrdinal, instance.logicalLabel, instance.step.View, firstNonEmptyV2(instance.step.Window, "(unbounded)")))
	}
}

func writeV2InstanceHeader(rw *reportWriter, ordinal, total int, instance v2StepInstance) {
	rw.line("")
	rw.line(strings.Repeat("=", 80))
	rw.line(fmt.Sprintf("[执行实例 %d/%d] logical_step=%d label=%s view=%s instance=%d/%d", ordinal, total, instance.logicalOrdinal, instance.logicalLabel, instance.step.View, instance.instanceOrdinal, instance.instanceCount))
	rw.line("参数: " + stepParamsEcho(&instance.step))
	if requested, clamped := instance.step.MaxLinesClamped(); clamped {
		rw.line(fmt.Sprintf("⚠ max_lines=%d 超过硬帽 %d,已夹取为 %d", requested, HardStepMaxLines, instance.step.EffectiveMaxLines()))
	}
	if origin := instance.step.windowOrigin; origin != nil {
		rw.line(fmt.Sprintf("自动窗 provenance: discovery=%s window_ordinal=%d candidate_rank=%d candidate_slice=%d family=%s kind=%s core=[%s..%s] core_lines=%d-%d rank_basis=%s fingerprint=%s",
			origin.DiscoveryLabel, origin.WindowOrdinal, origin.CandidateRank, origin.CandidateWindow, origin.Family, origin.Kind,
			formatSecondsToken(origin.CoreStartTs), formatSecondsToken(origin.CoreEndTs), origin.CoreLineStart, origin.CoreLineEnd,
			clampToken(origin.RankBasis), origin.IdentityFingerprint))
		rw.line("注: 本窗为系统确定性派生窗(FrameWindowAutoDerived=true)，不是用户显式帧窗；仅用于补采该候选的完整端点证据。")
	}
	if _, _, windowSet := instance.step.WindowBounds(); windowSet && (instance.step.LineStart > 0 || instance.step.LineEnd > 0) {
		if instance.step.View == ViewFormatCensus {
			rw.line("注: 本步为普查步,时间窗与行区间取交集过滤(普查步语义;引擎查询步则 line 界恒生效、time 界仅在无 line 界时生效)")
		} else {
			rw.line("注: 行区间生效时时间窗不参与过滤(引擎语义:line 界恒生效,time 界仅在无 line 界时生效)")
		}
	}
	rw.line(strings.Repeat("-", 80))
}

func writeV2StatusSummary(rw *reportWriter, discoveries []v2DiscoveryOutcome, statuses []v2StepStatus) {
	rw.line("")
	rw.line(strings.Repeat("=", 80))
	rw.line("[发现与执行状态摘要]")
	failed := 0
	for _, outcome := range discoveries {
		if outcome.err != nil {
			failed++
			rw.line(fmt.Sprintf("- discovery label=%s strategy=%s 状态=失败: %s", outcome.spec.Label, outcome.spec.Strategy, clampToken(outcome.err.Error())))
			continue
		}
		rw.line(fmt.Sprintf("- discovery label=%s strategy=%s 状态=成功 generated_windows=%d complete=%t identity_complete=%t", outcome.spec.Label, outcome.spec.Strategy, len(outcome.result.Windows), outcome.result.Complete, outcome.result.IdentityComplete))
	}
	for i, status := range statuses {
		instance := status.instance
		if status.err != nil {
			failed++
			rw.line(fmt.Sprintf("- instance=%d label=%s fanout=%d/%d 状态=失败: %s", i+1, instance.logicalLabel, instance.instanceOrdinal, instance.instanceCount, clampToken(status.err.Error())))
			continue
		}
		rw.line(fmt.Sprintf("- instance=%d label=%s fanout=%d/%d 状态=成功 输出行=%d/%d", i+1, instance.logicalLabel, instance.instanceOrdinal, instance.instanceCount, status.bodyShown, status.bodyTotal))
	}
	if failed > 0 {
		rw.line(fmt.Sprintf("结论: %d 个 discovery/执行实例失败或被 typed dependency 阻断；独立实例已继续，未发生父窗静默回退。", failed))
	} else {
		rw.line(fmt.Sprintf("结论: 全部 %d 个 discovery 与 %d 个执行实例成功；source_lock_status=validated。", len(discoveries), len(statuses)))
	}
}

func firstNonEmptyV2(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
