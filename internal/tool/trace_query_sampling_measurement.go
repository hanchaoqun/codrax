package tool

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

type samplingFactGroup struct {
	record        types.ObservationRecord
	label         string
	rows, quality [][]string
	notes         []string
	endInclusive  bool
}

// A fixed selection groups flat symbols for display only, never exact stack
// paths. Physical generation and thread identity remain part of its key. Each
// native cohort retains its own denominator; no cross-cohort total is minted.
func traceQuerySamplingMeasurementPublications(result tracequery.Result, records []types.ObservationRecord) []types.ObservationRecord {
	p, q, valid := tracequery.NativePerfPopulation(result)
	if !valid || q.TargetScope != "thread" {
		return records
	}
	pid, thread, valid := tracequery.NormalizePerfSamplingSelector(q.PID, q.Thread)
	if !valid {
		return records
	}
	byClaim := map[string]types.ObservationRecord{}
	for _, r := range records {
		if r.Predicate == "perf_sample_top_symbol" {
			byClaim[r.ClaimKey] = r
		}
	}
	groups := map[string]*samplingFactGroup{}
	var order []string
	for _, c := range p.Cohorts {
		if c.Scope == nil || c.Scope.ThreadIdentity == nil || c.WeightStatus != "exact" || c.SampleCount <= 0 || c.TotalPeriod < 0 || c.ThreadIdentityCountExact == nil || !*c.ThreadIdentityCountExact {
			continue
		}
		for _, hot := range c.TopSymbols {
			base, found := byClaim[traceQueryPerfCohortClaimKey(c, hot)]
			if !found || base.Object != hot.DSO || base.Value != strconv.FormatInt(hot.Period, 10) || base.SourceRef.Path != result.SourcePath || hot.SampleCount < 0 || hot.SampleCount > c.SampleCount || hot.Period < 0 || hot.Period > c.TotalPeriod {
				continue
			}
			ref := base.SourceRef
			ref.QueryTargetPID, ref.QueryTargetThread = pid, thread
			selectorRef := ref
			selectorRef.QueryLineStart, selectorRef.QueryLineEnd = 0, 0
			subject, selected := traceQuerySamplingSelectedThread(selectorRef, c.Scope.ThreadIdentity.TID, c.Scope.ThreadIdentity.DisplayComm)
			if !selected || c.Scope.Window.StartTs != result.TimeStart || c.Scope.Window.EndTs != result.TimeEnd {
				continue
			}
			keyBytes, _ := json.Marshal([]any{ref, c.Scope.SourcePath, c.Scope.SourceGeneration, c.Scope.ThreadIdentity, c.Scope.Window, hot.Symbol, hot.DSO})
			key := traceQuerySamplingTreeDigest(keyBytes)
			g := groups[key]
			if g == nil {
				id := "perf-sampling-facts-" + key
				r := base
				r.ID, r.ClaimKey, r.Predicate, r.Subject, r.SourceRef = id, id, types.RuntimeSamplingMeasurementPredicate, subject, ref
				r.Value, r.Unit, r.Summary, r.RawExcerpt, r.RichNotes = "", "", "Native sampling facts: "+hot.Symbol+" / "+hot.DSO, "", nil
				r.Role = types.AnswerAggregateRoleSupportingCoverage
				filters, _ := json.Marshal(tracequery.PerfPopulationQueryFilters(q))
				endpoint := ")"
				if c.Scope.Window.EndInclusive {
					endpoint = "]"
				}
				g = &samplingFactGroup{record: r, label: "采样热点 — " + hot.Symbol + " / " + hot.DSO, endInclusive: c.Scope.Window.EndInclusive, notes: []string{
					"原件：" + result.SourcePath + "；采样成员：" + c.Scope.SourcePath + "；线程：" + subject,
					fmt.Sprintf("查询窗口 [%s, %s%s 秒；实际过滤条件：%s。分母是执行这些条件后的各自查询总体，不是所选热点之和。", traceQueryDisplaySeconds(result.TimeStart), traceQueryDisplaySeconds(result.TimeEnd), endpoint, filters),
					"每行是独立采样总体；记录占比=命中记录数/该行总体记录数，权重占比=命中权重/该行总体权重。不跨事件、采集方式或单位相加权重，也不推导跨总体去重记录总数。",
					"这是符号与所属文件的平面热点，不是精确调用路径；可能包含同名地址。未知不是零。记录数不等于调用次数或采样频率，权重占比不等于耗时、CPU忙碌或根因。",
					"记录时刻包络是完整选集的首末记录点，不是连续执行或阻塞时长；预览首末行不能替代它，包络内外也不能据此断言没有活动。",
				}}
				groups[key], order = g, append(order, key)
			}
			population := fmt.Sprintf("总体%d", len(g.rows)+1)
			g.rows = append(g.rows, []string{samplingFactKnown(c.Event), samplingFactKnown(c.Scope.Producer), samplingFactKnown(c.Scope.SampleKind), samplingFactKnown(c.WeightUnit), strconv.Itoa(hot.SampleCount), strconv.FormatInt(hot.Period, 10), strconv.Itoa(c.SampleCount), strconv.FormatInt(c.TotalPeriod, 10), samplingFactFraction(hot.Period, c.TotalPeriod), population, samplingFactKnown(c.Scope.Clock), samplingFactFraction(int64(hot.SampleCount), int64(c.SampleCount)), samplingFactRecordedTime(hot.RecordedTime, hot.SampleCount), samplingFactRecordedTime(c.RecordedTime, c.SampleCount), samplingFactExecutionBasis(c.Scope.SampleKind)})
			for _, row := range samplingFactQualityRows(c) {
				g.quality = append(g.quality, append(row, population, samplingFactKnown(c.Scope.SampleKind), samplingFactKnown(c.Scope.Clock)))
			}
			retained := 0
			for _, member := range c.TopSymbols {
				retained += member.SampleCount
			}
			if retained != c.SampleCount || p.CohortCount != len(p.Cohorts) {
				g.notes = append(g.notes, "热点候选或采样总体有展示截断；当前选择只包含已返回的精确热点行，分母仍为各行原生查询总体，不能声称完整热点列表。")
			}
		}
	}
	for _, key := range order {
		g := groups[key]
		tables := []types.RuntimeMeasurementTable{{ObservationID: g.record.ID, View: types.RuntimeMeasurementSummary, Label: g.label,
			Columns: []string{"采样事件", "采集方式", "样本类型", "权重单位", "命中记录数", "命中权重", "查询总体记录数", "查询总体权重", "权重占比", "总体标记", "时钟域", "记录占比", "热点记录时刻包络（秒）", "查询总体记录时刻包络（秒）", "执行状态依据"}, Rows: g.rows, Notes: g.notes}}
		if len(g.quality) > 0 {
			tables = append(tables, types.RuntimeMeasurementTable{ObservationID: g.record.ID, View: types.RuntimeMeasurementDistribution, Label: g.label + " — 查询总体质量（不是热点子集质量）",
				Columns: []string{"采样事件", "采集方式", "权重单位", "质量维度", "状态", "总体内样本记录数", "总体内权重", "查询总体记录数", "查询总体权重", "总体标记", "样本类型", "时钟域"}, Rows: g.quality, Notes: append(append([]string(nil), g.notes...), "质量项按样本记录计数，不是栈帧个数；精确帧边界不证明完整展开。质量维度彼此重叠，不能相加。")})
		}
		if r, ok := types.PublishRuntimeSamplingMeasurement(g.record, tables, g.endInclusive); ok {
			records = append(records, r)
		}
	}
	return records
}

func samplingFactKnown(s string) string {
	if label, ok := map[string]string{"raw_perfdata_fallback": "原始 perf 数据", "simpleperf_report_sample": "simpleperf 样本报告", "exact": "精确", "available": "已记录", "unavailable": "不可用", "not_recorded": "未记录", "partial": "部分", "complete": "完整", "recorded": "已记录", "opaque_flat_label": "仅平面标签", "exact_frames": "精确帧边界", "legacy_frame_boundaries_unavailable": "帧边界不可用", "symbolized": "已符号化", "verified": "已验证"}[s]; ok {
		return label
	}
	if s == "" || s == "unknown" {
		return "未知"
	}
	return s
}
func samplingFactFraction(n, d int64) string {
	if d <= 0 {
		return "未定义（总体权重为零）"
	}
	return fmt.Sprintf("%d/%d", n, d)
}

func samplingFactQualityRows(c tracequery.PerfCohort) [][]string {
	if c.Quality == nil {
		return [][]string{{samplingFactKnown(c.Event), samplingFactKnown(c.Scope.Producer), samplingFactKnown(c.WeightUnit), "质量信息", "未提供", "未知", "未知", strconv.Itoa(c.SampleCount), strconv.FormatInt(c.TotalPeriod, 10)}}
	}
	var rows [][]string
	q := c.Quality
	for _, dimension := range []struct {
		label  string
		values []tracequery.PerfValueCount
	}{
		{"样本类型", q.SampleKinds}, {"帧边界", q.FrameStatuses}, {"调用链记录", q.CallchainStatuses}, {"符号解析", q.SymbolizationStatuses}, {"输入完整性", q.InputIntegrityIssues}, {"解析限制", q.ParserCaveats}, {"时钟", q.Clocks}, {"时钟可信度", q.ClockConfidences},
		{"记录CPU（不代表执行状态）", q.RecordedCPUs},
	} {
		for _, value := range dimension.values {
			label := samplingFactKnown(value.Value)
			if dimension.label == "记录CPU（不代表执行状态）" {
				if cpu, ok := strings.CutPrefix(value.Value, "cpu:"); ok {
					label = "CPU " + cpu
				} else if value.Value == "other_known_cpus" {
					label = fmt.Sprintf("其他已知CPU（合并%d个）", q.RecordedCPUOmittedCount)
				}
			}
			rows = append(rows, []string{samplingFactKnown(c.Event), samplingFactKnown(c.Scope.Producer), samplingFactKnown(c.WeightUnit), dimension.label, label, strconv.Itoa(value.SampleCount), strconv.FormatInt(value.Period, 10), strconv.Itoa(c.SampleCount), strconv.FormatInt(c.TotalPeriod, 10)})
		}
	}
	return rows
}
