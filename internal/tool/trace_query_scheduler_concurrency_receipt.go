package tool

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A table is another projection of this exact accepted population, not a
// model-created aggregate or a roster inferred from the depth series.
func traceQuerySchedulerConcurrencyReceipt(r types.ObservationRecord, s *tracequery.SchedulerConcurrencyStats, g tracequery.SchedulerConcurrencyGroup) string {
	if g.AcceptedIntervalCount != len(g.Members)+g.OmittedMembers+g.MemberWitnessUnavailableCount {
		return ""
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'g', 12, 64) }
	window := "未确定连续时间窗口；平均值和分布不可用，不是零。"
	if s.Window != nil {
		window = fmt.Sprintf("时间窗口 [%s, %s) 秒", traceQueryDisplaySeconds(s.Window.StartTs), traceQueryDisplaySeconds(s.Window.EndTs))
	}
	notes := []string{window, "来源：" + g.SourcePath,
		"仅统计已确认起止的线程状态区间，覆盖全部正线程号；同一线程的重叠区间先合并。未闭合、来源冲突或身份不明的区间不补齐。",
		"零表示这些已确认区间没有贡献，不证明系统空闲或没有等待线程；采集完整性未知。这些统计不证明目标等待、优先级反转或响应根因。",
		fmt.Sprintf("已接纳区间 %d、线程 %d；本次分组 %d，未展示分组 %d。查询线程号 %d 不过滤本统计总体。", g.AcceptedIntervalCount, g.ThreadCount, s.GroupCount, s.OmittedGroups, s.QueryPID),
		fmt.Sprintf("本组区间核对：候选 %d、接纳 %d、未闭合 %d、来源冲突 %d、来源未确定 %d、身份排除 %d、无效 %d。", g.Coverage.CandidateIntervals, g.Coverage.AcceptedIntervals, g.Coverage.OpenEndedIntervals, g.Coverage.SourceConflictIntervals, g.Coverage.UnresolvedSourceIntervals, g.Coverage.IdentityExcludedIntervals, g.Coverage.InvalidIntervals)}
	if s.LineStart > 0 || s.LineEnd > 0 {
		notes = append(notes, fmt.Sprintf("行范围 %d–%d 优先于时间参数，不能由行选择推定连续时间分母。", s.LineStart, s.LineEnd))
	}
	var reasons []string
	for _, reason := range g.Coverage.Reasons {
		reasons = append(reasons, traceQuerySchedulerConcurrencyReason(reason))
	}
	if len(reasons) > 0 {
		notes = append(notes, "覆盖限制："+strings.Join(reasons, "；"))
	}
	table := func(view types.RuntimeMeasurementView, columns []string, rows [][]string, extra ...string) types.RuntimeMeasurementTable {
		return types.RuntimeMeasurementTable{ObservationID: r.ID, View: view,
			Label: traceQuerySchedulerConcurrencyState(g.State), Columns: columns, Rows: rows,
			Notes: append(append([]string(nil), notes...), extra...)}
	}
	values := []string{"不可用", "不可用", "不可用", "不可用"}
	if v := g.Values; v != nil {
		values = []string{strconv.Itoa(v.PeakThreads), f(v.MeanThreads), f(v.BusyMs), f(v.ThreadMs)}
	}
	summary := table(types.RuntimeMeasurementSummary,
		[]string{"峰值线程数", "全窗平均线程数", "已确认区间覆盖时长 (ms)", "线程时间合计 (线程·ms)"}, [][]string{values},
		"全窗平均按各深度的持续时间加权，含已确认总体无贡献的时间；不是非零桶均值或桶峰值均值。覆盖时长是区间并集，线程时间合计不是响应延迟。")
	if g.Values == nil {
		summary.Notes = append(summary.Notes, "测量不可用："+traceQuerySchedulerConcurrencyReason(g.ValuesUnavailableReason))
	}
	var members [][]string
	for _, m := range g.Members {
		if m.SourcePath != g.SourcePath || m.ID == "" || m.Thread.PID <= 0 || m.StartLocalLine <= 0 || m.EndLocalLine <= 0 {
			return ""
		}
		intersection, contribution := "不可用", "不可用"
		if m.WindowContributionMs != nil {
			intersection, contribution = "无重叠", f(*m.WindowContributionMs)
		}
		if w := m.WindowContribution; w != nil {
			intersection = "[" + traceQueryDisplaySeconds(w.StartTs) + ", " + traceQueryDisplaySeconds(w.EndTs) + ")"
		}
		members = append(members, []string{traceThreadLabel(m.Thread), strconv.Itoa(m.StartLocalLine), strconv.Itoa(m.EndLocalLine),
			traceQueryDisplaySeconds(m.ActualStartTs), traceQueryDisplaySeconds(m.ActualEndTs), intersection, contribution})
	}
	memberTable := table(types.RuntimeMeasurementMembers,
		[]string{"线程", "起点来源行", "终点来源行", "实际起点 (s)", "实际终点 (s)", "窗口内区间 (s)", "窗口内时长 (ms)"}, members,
		fmt.Sprintf("已接纳区间 %d = 展示 %d + 展示上限省略 %d + 端点见证不可用 %d；汇总/分布/分桶基于全部已接纳区间。", g.AcceptedIntervalCount, len(members), g.OmittedMembers, g.MemberWitnessUnavailableCount),
		"实际端点未按查询窗口裁剪，窗口内时长另列。同线程重叠成员不能直接相加重算线程时间；仅有深度不能反推参与线程，也不能把排除的开放尾线程当成峰值成员。")
	var depths [][]string
	var distributionNotes []string
	if d := g.Distribution; d != nil {
		for _, depth := range d.Depths {
			depths = append(depths, []string{strconv.Itoa(depth.Threads), f(depth.DurationMs), f(depth.WindowShare * 100)})
		}
		distributionNotes = append(distributionNotes,
			fmt.Sprintf("完整窗口 %s ms；深度档 %d，展示 %d，省略 %d。按持续时间累计的50%%/95%%/99%%分位线程数为 %d/%d/%d。", f(d.WindowMs), d.DepthCount, len(d.Depths), d.OmittedDepths, d.P50Threads, d.P95Threads, d.P99Threads))
	} else {
		distributionNotes = append(distributionNotes, "持续时间分布不可用；空表不是全窗零线程。")
	}
	distributionNotes = append(distributionNotes, "各深度按完整窗口内的实际持续时间计量，包含已确认总体的零贡献时段；不是按桶峰值或事件次数计数。分位数取累计持续时间达到对应比例的最小线程数。")
	distribution := table(types.RuntimeMeasurementDistribution, []string{"同时线程数", "持续时间 (ms)", "全窗时间占比 (%)"}, depths, distributionNotes...)
	var buckets [][]string
	for _, b := range g.Buckets {
		buckets = append(buckets, []string{traceQueryDisplaySeconds(b.Window.StartTs), traceQueryDisplaySeconds(b.Window.EndTs),
			strconv.Itoa(b.Values.PeakThreads), f(b.Values.MeanThreads), f(b.Values.BusyMs), f(b.Values.ThreadMs)})
	}
	timeline := table(types.RuntimeMeasurementTimeline,
		[]string{"起点（含，s）", "终点（不含，s）", "桶内峰值线程数", "桶内平均线程数", "区间覆盖时长 (ms)", "线程时间 (线程·ms)"}, buckets,
		fmt.Sprintf("分桶间隔 %s ms；共 %d 桶，展示 %d，省略 %d。完整汇总和分布不由展示桶反算。", f(s.BucketMs), g.BucketCount, len(g.Buckets), g.OmittedBuckets),
		"每桶采用实际半开区间，末桶可短于设定间隔；保留已确认总体的零贡献桶。桶内峰值不代表整桶一直维持该线程数，平均值按桶内真实持续时间加权。")
	if g.BucketsUnavailableReason != "" {
		timeline.Notes = append(timeline.Notes, "时间分桶不可用，空表不是零线程曲线。")
	}
	p := types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef,
		Tables: []types.RuntimeMeasurementTable{summary, memberTable, distribution, timeline}}
	data, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return types.TraceNoteKeyRuntimeMeasurement + "=" + string(data)
}
