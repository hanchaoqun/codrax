package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	TraceThreadTimelinePredicate       = "thread_timeline_observation"
	TraceCPUFrequencyTimelinePredicate = "cpu_frequency_timeline_observation"
)

// Native timeline tables are projections of the query's existing scheduler
// partition and CPU frequency account. They do not reconstruct transitions,
// pair wakeups, change causal ranking, or establish population completeness.
func traceQueryNativeTimelineObservations(result tracequery.Result, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	start, end, known := types.TraceObservationContinuousQueryWindow(ref)
	if !known || ref.Kind != types.ObservationSourceRuntimeArtifact || ref.Path == "" || ref.Path != result.SourcePath || ref.QueryScopeID == "" || ref.PayloadRef == "" {
		return nil
	}
	var out []types.ObservationRecord
	if timeline := result.NativeTargetTimeline(); timeline != nil {
		if table, ok := traceQueryNativeThreadTimelineTable(*timeline, ref, start, end); ok {
			record := traceQueryNativeTimelineRecord(TraceThreadTimelinePredicate, traceThreadLabel(timeline.Thread), table, ref, scope, at, result)
			native, err := json.Marshal(timeline)
			if err == nil {
				record = types.PublishRuntimeMeasurementFactIdentity(record, native)
			}
			out = append(out, record)
		}
	}
	if stats := result.WindowStats; stats != nil {
		if table, ok := traceQueryNativeCPUFrequencyTable(*stats, start, end); ok {
			record := traceQueryNativeTimelineRecord(TraceCPUFrequencyTimelinePredicate, "CPU frequency", table, ref, scope, at, result)
			native, err := json.Marshal(struct {
				Window tracequery.TimeWindow
				CPU    []tracequery.CPUStats
			}{stats.Window, stats.CPU})
			if err == nil {
				record = types.PublishRuntimeMeasurementFactIdentity(record, native)
			}
			out = append(out, record)
		}
	}
	return out
}

func traceQueryNativeTimelineRecord(predicate, subject string, table types.RuntimeMeasurementTable, ref types.ObservationSourceRef, scope, at string, result tracequery.Result) types.ObservationRecord {
	r := types.ObservationRecord{ID: "trace_query:" + scope + "#" + predicate, Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
		Span:     types.ObservationSpan{StartTs: ref.QueryWindowStartTs, EndTs: ref.QueryWindowEndTs},
		ClaimKey: predicate, Predicate: predicate, Subject: subject, Object: "observed intervals",
		Value: strconv.Itoa(len(table.Rows)), Unit: "intervals", Summary: table.Label,
		ObservedAt: at, Confidence: 1}
	table.ObservationID = r.ID
	table.Notes = append([]string{"来源：" + ref.Path,
		fmt.Sprintf("所选窗口 [%s, %s) 秒；各行保留原生查询的区间和数值，不跨来源相加。%s", traceQueryDisplaySeconds(r.Span.StartTs), traceQueryDisplaySeconds(r.Span.EndTs), traceQueryNativeIndexWitnessNote(result))}, table.Notes...)
	publication := types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: ref, Tables: []types.RuntimeMeasurementTable{table}}
	data, _ := json.Marshal(publication) // all numeric inputs were validated before formatting
	r.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(data)}
	return r
}

// The parser's declared index bounds are configuration, not proof that parsing
// reached every bound, the selected measurement window, or complete capture. Keep them
// in the existing scope note so compact note selection cannot displace the
// producer's unknown-endpoint and waiting-location qualifications.
func traceQueryNativeIndexWitnessNote(result tracequery.Result) string {
	window := "索引未按时间限定"
	if result.IndexWindowed {
		window = "未发布，不能推定"
		if traceQueryNativeFinite(result.IndexTimeStart) && traceQueryNativeFinite(result.IndexTimeEnd) && result.IndexTimeEnd > result.IndexTimeStart {
			window = fmt.Sprintf("%.6f..%.6f 秒", result.IndexTimeStart, result.IndexTimeEnd)
		}
	}
	lines := "未发布，不能推定"
	switch {
	case result.IndexLineStart == 0 && result.IndexLineEnd == 0:
		lines = "索引未按行限定"
	case result.IndexLineStart > 0 && result.IndexLineEnd >= result.IndexLineStart:
		lines = fmt.Sprintf("%d..%d", result.IndexLineStart, result.IndexLineEnd)
	case result.IndexLineStart > 0 && result.IndexLineEnd == 0:
		lines = fmt.Sprintf("从第 %d 行起，末行未限定", result.IndexLineStart)
	case result.IndexLineStart == 0 && result.IndexLineEnd > 0:
		lines = fmt.Sprintf("截至第 %d 行，起行未限定", result.IndexLineEnd)
	}
	return fmt.Sprintf(" 本次查询的索引配置时间范围：%s；索引配置行范围：%s。不保证配置范围内已全部解析或采集完整；关闭事件仅按各表实际端点见证判定，各查询独立。", window, lines)
}

func traceQueryNativeThreadTimelineTable(timeline tracequery.TimelineResult, ref types.ObservationSourceRef, start, end float64) (types.RuntimeMeasurementTable, bool) {
	table := types.RuntimeMeasurementTable{View: types.RuntimeMeasurementTimeline, Label: "线程状态时序 — " + traceThreadLabel(timeline.Thread), DefaultPresentation: true,
		Columns: []string{"线程", "状态", "开始（秒，含）", "结束（秒，不含）", "持续（ms）", "执行CPU", "实际起点（秒）", "实际终点（秒）", "起点行", "末端证据行", "唤醒行", "等待位置"},
		Notes: []string{"唤醒是事件点，不等于上CPU；运行区间的实际起点才是已观测的上CPU时间。选中窗口起点可能裁切已运行区间，不能把裁切点说成上CPU。",
			"实际终点未知表示未观察到关闭事件，所选区间末端仅是本次计量边界。末端证据行也可能只是等待位置引用，不单独证明等待结束。窗外实际起止只作边界证据，不计入所选窗口耗时。未展示区间不补零；CPU未知不当CPU0。状态区间不新增唤醒依赖或根因证明。"}}
	if timeline.IntegrityFailure != "" || timeline.Window.StartTs != start || timeline.Window.EndTs != end || len(timeline.Intervals) == 0 ||
		(timeline.Thread.PID <= 0 && strings.TrimSpace(timeline.Thread.Comm) == "") ||
		(ref.QueryTargetPID > 0 && ref.QueryTargetPID != timeline.Thread.PID) {
		return table, false
	}
	previousEnd := start
	for _, iv := range timeline.Intervals {
		state, valid := traceQueryNativeTimelineState(iv.State)
		if !valid || !traceQueryNativeIntervalValid(iv.StartTs, iv.EndTs, iv.DurationMs, start, end) || iv.StartTs < previousEnd ||
			iv.StartLine < 0 || iv.EndLine < 0 || iv.WakeupLine < 0 || iv.BlockedReasonLine < 0 ||
			(timeline.Thread.PID > 0 && iv.Thread.PID != timeline.Thread.PID) ||
			(timeline.Thread.PID <= 0 && iv.Thread.Comm != timeline.Thread.Comm) ||
			(iv.CPUKnown && (iv.State != tracequery.StateRunning || iv.CPU < 0)) ||
			!traceQueryNativeFinite(iv.ActualStartTs) || !traceQueryNativeFinite(iv.ActualEndTs) || !traceQueryNativeFinite(iv.ActualDurationMs) || iv.ActualDurationMs < 0 || iv.ActualEndTs < iv.ActualStartTs {
			return table, false
		}
		previousEnd = iv.EndTs
		cpu := "不适用"
		if iv.State == tracequery.StateRunning {
			cpu = "未知"
			if iv.CPUKnown {
				cpu = fmt.Sprintf("CPU%d", iv.CPU)
			}
		}
		actualStart, actualEnd := "未知", "未知"
		if iv.ActualEndTs >= iv.ActualStartTs && (iv.ActualStartTs != 0 || iv.ActualEndTs != 0) {
			if iv.ActualStartTs > iv.StartTs || iv.ActualEndTs < iv.EndTs {
				return table, false
			}
			if iv.StartLine > 0 {
				actualStart = traceQueryDisplaySeconds(iv.ActualStartTs)
			}
			// The scheduler enrichment preserves an open D/IO wait's marker
			// in EndLine when no closing event exists. A blocked_reason row is
			// not a sched_switch/wakeup boundary, even when its line is nonzero.
			if iv.EndLine > 0 && iv.EndLine != iv.BlockedReasonLine {
				actualEnd = traceQueryDisplaySeconds(iv.ActualEndTs)
			}
		}
		wait := "未观测"
		if iv.BlockedReasonCaller != "" && iv.BlockedReasonLine > 0 {
			wait = fmt.Sprintf("%s（行%d）", iv.BlockedReasonCaller, iv.BlockedReasonLine)
		}
		table.Rows = append(table.Rows, []string{traceThreadLabel(iv.Thread), state, traceQueryDisplaySeconds(iv.StartTs), traceQueryDisplaySeconds(iv.EndTs),
			strconv.FormatFloat(iv.DurationMs, 'g', 9, 64), cpu, actualStart, actualEnd, traceQueryNativeLine(iv.StartLine), traceQueryNativeLine(iv.EndLine), traceQueryNativeLine(iv.WakeupLine), wait})
	}
	if timeline.HeadState != nil && timeline.HeadState.Status == "unknown" {
		table.Notes = append(table.Notes, "窗口开头状态未知，不将缺失时段补为零或任意线程状态。")
	}
	table.Notes = append(table.Notes, timeline.Caveats...)
	return table, true
}

func traceQueryNativeCPUFrequencyTable(stats tracequery.WindowStats, start, end float64) (types.RuntimeMeasurementTable, bool) {
	table := types.RuntimeMeasurementTable{View: types.RuntimeMeasurementTimeline, Label: "CPU频率时序", DefaultPresentation: true,
		Columns: []string{"CPU", "开始（秒，含）", "结束（秒，不含）", "频率（kHz）", "持续（ms）", "证据起行", "证据末行"},
		Notes: []string{"每行是该CPU已观测频率在所选窗内的驻留段；未覆盖区间频率未知，不用末次频率回填。证据末行保留原始引用，可能与起行相同，不单独证明区间末端发生频率变化。",
			"频率驻留不等于线程运行或CPU空闲；不能据此量化执行速度、功耗或判定性能瓶颈与响应根因。CPU空闲状态需独立的CPU状态证据，不能由目标线程未运行推算。"}}
	if stats.Window.StartTs != start || stats.Window.EndTs != end {
		return table, false
	}
	seen := map[int]bool{}
	for _, cpu := range stats.CPU {
		if cpu.CPU < 0 || seen[cpu.CPU] {
			return table, false
		}
		seen[cpu.CPU] = true
		previousEnd := start
		for _, iv := range cpu.FrequencyResidency {
			if iv.Frequency <= 0 || !traceQueryNativeIntervalValid(iv.StartTs, iv.EndTs, iv.DurationMs, start, end) || iv.EndTs <= iv.StartTs ||
				iv.StartTs < previousEnd || iv.LineStart < 0 || iv.LineEnd < 0 {
				return table, false
			}
			previousEnd = iv.EndTs
			table.Rows = append(table.Rows, []string{fmt.Sprintf("CPU%d", cpu.CPU), traceQueryDisplaySeconds(iv.StartTs), traceQueryDisplaySeconds(iv.EndTs),
				strconv.FormatInt(iv.Frequency, 10), strconv.FormatFloat(iv.DurationMs, 'g', 9, 64), traceQueryNativeLine(iv.LineStart), traceQueryNativeLine(iv.LineEnd)})
		}
	}
	return table, len(table.Rows) > 0
}

func traceQueryNativeIntervalValid(start, end, duration, windowStart, windowEnd float64) bool {
	if !traceQueryNativeFinite(start) || !traceQueryNativeFinite(end) || !traceQueryNativeFinite(duration) || start < windowStart || end > windowEnd || end < start || duration < 0 {
		return false
	}
	want := (end - start) * 1000
	return math.Abs(duration-want) <= math.Max(1e-7, want*1e-9)
}

func traceQueryNativeFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func traceQueryNativeLine(line int) string {
	if line <= 0 {
		return "未知"
	}
	return strconv.Itoa(line)
}

func traceQueryNativeTimelineState(state tracequery.ThreadState) (string, bool) {
	labels := map[tracequery.ThreadState]string{tracequery.StateRunning: "运行", tracequery.StateRunnable: "可运行", tracequery.StateSSleep: "S态睡眠",
		tracequery.StateDSleep: "D态等待", tracequery.StateIOWait: "I/O等待", tracequery.StateStopped: "已停止", tracequery.StateDead: "已结束", tracequery.StateUnknown: "未知"}
	label, ok := labels[state]
	return label, ok
}
