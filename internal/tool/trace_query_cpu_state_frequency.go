package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const TraceCPUStateFrequencyPredicate = "cpu_state_frequency_observation"

func traceQueryCPUStateFrequencySchema(schema json.RawMessage) json.RawMessage {
	var obj map[string]any
	if json.Unmarshal(schema, &obj) != nil {
		return schema
	}
	properties, _ := obj["properties"].(map[string]any)
	view, _ := properties["view"].(map[string]any)
	if view == nil {
		return schema
	}
	description, _ := view["description"].(string)
	view["description"] = description + " " + tracequery.CPUStateFrequencyTeaching
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(obj) != nil {
		return schema
	}
	return out.Bytes()
}

func traceQueryCPUStateFrequencyInputRepair(p traceQueryParams) *types.ToolResult {
	if tracequery.CanonicalViewName(p.View) != tracequery.ViewCPUStateFrequency || (p.PID.Int() == 0 && strings.TrimSpace(p.Thread) == "") {
		return nil
	}
	hint := "cpu_state_frequency measures CPU-owned control lanes, not the task that emitted a row. Remove pid/thread and keep the requested time window. Use thread_timeline separately for the named thread's scheduler intervals; CPU control residency alone does not establish thread execution or a response root cause."
	return &types.ToolResult{ToolName: "trace_query", Success: false, Summary: hint, Timestamp: time.Now(), Repair: &types.ToolRepair{Code: "trace_query_cpu_owned_view", Fields: []string{"pid", "thread"}, Hint: hint}}
}

func (t *TraceQuery) streamCPUStateFrequency(ctx *types.BusContext, p traceQueryParams, path, sourceLabel, callCaveat string, window traceQueryNormalizedWindow) (types.ToolResult, bool) {
	if tracequery.CanonicalViewName(p.View) != tracequery.ViewCPUStateFrequency {
		return types.ToolResult{}, false
	}
	if tracequery.TracePathRequiresCompositeIndex(path) {
		// Preserve bundle identity through the existing indexed path. The
		// engine withholds this view where interval/ownership semantics are
		// not proven; never unwrap a child and borrow its measurements.
		return types.ToolResult{}, false
	}
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, window.RequestedStart, window.RequestedEnd)
	result, err := tracequery.StreamCPUStateFrequency(contextFromBus(ctx), path, q)
	if err != nil {
		if traceQueryIsCancellation(err) {
			return traceQueryCancellationResult(p.View, path, err), true
		}
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "CPU control interval query failed: " + err.Error(), Timestamp: time.Now()}, true
	}
	traceQueryAppendCallCaveats(&result, callCaveat)
	payload, failure := traceQueryMarshalPayload(t.Name(), result)
	if failure != nil {
		return *failure, true
	}
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	preview, rawRef := StoreBlob(ctx, t.Name(), traceQuerySummary(result, p, sourceLabel, payloadRef))
	if rawRef == "" {
		rawRef = payloadRef
	}
	now := time.Now()
	return types.ToolResult{ToolName: t.Name(), Success: true, Summary: preview, RawRef: rawRef, Timestamp: now,
		Observations:           traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now, q),
		TraceQuerySourceRead:   traceQuerySourceReadCandidate(result),
		TraceStatistics:        traceQueryStatisticsCandidate(result),
		TraceEvidenceAuthority: traceQueryEvidenceAuthorityWithSource(result, sourceLabel, payloadRef, rawRef, "", now, q)}, true
}

func traceQueryCPUStateFrequencyObservations(p *tracequery.CPUStateFrequencyResult, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if p == nil {
		return nil
	}
	r := types.ObservationRecord{ID: "trace_query:" + scope + "#cpu_state_frequency", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
		Span:     types.ObservationSpan{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs},
		ClaimKey: TraceCPUStateFrequencyPredicate, Predicate: TraceCPUStateFrequencyPredicate, Subject: "CPU controls", Object: p.Status,
		Summary:    "Per-CPU observed idle/frequency intersection with explicit unknown coverage; not thread execution or causal rank",
		ObservedAt: at, Confidence: 1}
	// Bind display receipts before the 16-item handoff lens. The complete
	// producer result is validated and its own omission counts stay explicit.
	receipt := traceQueryCPUStateFrequencyReceipt(r, *p)
	// This is a bounded handoff lens. Aggregate values retain their original
	// full-window denominators; payloadRef still points to the native result.
	copy := *p
	copy.CPUs = append([]tracequery.CPUStateFrequencyCPU(nil), p.CPUs...)
	if len(copy.CPUs) > 16 {
		copy.OmittedCPUs += len(copy.CPUs) - 16
		copy.CPUs = copy.CPUs[:16]
	}
	for i := range copy.CPUs {
		row := &copy.CPUs[i]
		if len(row.Groups) > 16 {
			row.OmittedGroups += len(row.Groups) - 16
			row.Groups = row.Groups[:16]
		}
		if len(row.Intervals) > 16 {
			row.OmittedIntervals += len(row.Intervals) - 16
			row.Intervals = row.Intervals[:16]
		}
	}
	data, err := json.Marshal(copy)
	if err != nil {
		return nil
	}
	r.RichNotes = []string{types.TraceNoteKeyCPUStateFrequency + "=" + string(data)}
	if receipt != "" {
		r.RichNotes = append(r.RichNotes, receipt)
	}
	return []types.ObservationRecord{r}
}

func DecodeTraceCPUStateFrequency(r types.ObservationRecord) (tracequery.CPUStateFrequencyResult, bool) {
	var p tracequery.CPUStateFrequencyResult
	ref := r.SourceRef
	_, _, continuousWindow := types.TraceObservationContinuousQueryWindow(ref)
	if r.Negative || r.Predicate != TraceCPUStateFrequencyPredicate || r.ClaimKey != r.Predicate || r.Subject != "CPU controls" ||
		r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) ||
		r.Role != types.AnswerAggregateRoleSupportingCoverage || r.GroundingPolicy != types.ClaimGroundingHard ||
		r.ProvenanceLane != types.ObservationProvenanceArtifactSpan || ref.Kind != types.ObservationSourceRuntimeArtifact ||
		ref.Path == "" || ref.QueryScopeID == "" || ref.PayloadRef == "" || !continuousWindow || ref.QueryTargetPID != 0 || ref.QueryTargetThread != "" {
		return p, false
	}
	count := 0
	for _, note := range r.RichNotes {
		if strings.HasPrefix(note, types.TraceNoteKeyCPUStateFrequency+"=") {
			count++
			raw := json.RawMessage(strings.TrimPrefix(note, types.TraceNoteKeyCPUStateFrequency+"="))
			// Reuse the exact recursive member check (including escaped/case
			// aliases); a typed handoff must not choose a last-wins value.
			if !traceRootCauseReportHasUniqueNestedKeys(raw) || json.Unmarshal(raw, &p) != nil {
				return p, false
			}
		}
	}
	return p, count == 1 && tracequery.ValidCPUStateFrequency(p) && p.SourcePath == ref.Path && p.Status == r.Object &&
		p.Window.StartTs == ref.QueryWindowStartTs && p.Window.EndTs == ref.QueryWindowEndTs && p.Window.StartTs == r.Span.StartTs && p.Window.EndTs == r.Span.EndTs
}

func TraceCPUStateFrequencyText(p tracequery.CPUStateFrequencyResult, limit int) string {
	var b strings.Builder
	if p.Status == "unavailable" {
		fmt.Fprintf(&b, "CPU状态与频率联合区间无法计量：reason=%s；窗口=[%.9f, %.9f)秒。覆盖率、驻留时长均未知，不能按零处理。\n", p.Reason, p.Window.StartTs, p.Window.EndTs)
		return b.String()
	}
	fmt.Fprintf(&b, "窗口=[%.9f, %.9f)秒；墙钟=%.9gms；观测CPU数=%d；全部核时间=%.9g CPU-ms；状态和频率均已知=%.9g CPU-ms，联合未知=%.9g CPU-ms。\n", p.Window.StartTs, p.Window.EndTs, p.WindowWallMs, p.CPUCount, p.CPUTimeMs, p.KnownJointMs, p.UnknownJointMs)
	shown := 0
	for _, cpu := range p.CPUs {
		if shown >= limit {
			break
		}
		shown++
		fmt.Fprintf(&b, "CPU%d；核类型=%s（依据=%s）；状态已知=%.9gms；频率已知=%.9gms；联合已知=%.9gms，联合未知=%.9gms；每核占比分母=%.9gms。\n", cpu.CPU, cpu.CoreClass, cpu.TopologySource, cpu.IdleKnownMs, cpu.FrequencyKnownMs, cpu.JointKnownMs, cpu.UnknownJointMs, cpu.WindowWallMs)
		if cpu.IdleUnavailable != "" || cpu.FrequencyUnavailable != "" {
			fmt.Fprintf(&b, "状态不可计量原因=%q；频率不可计量原因=%q。\n", cpu.IdleUnavailable, cpu.FrequencyUnavailable)
		}
		b.WriteString("|状态|频率(kHz)|持续(ms)|每核完整窗占比(%)|\n|---|---:|---:|---:|\n")
		for _, group := range cpu.Groups {
			state, freq := cpuStateFrequencyLabels(group.CPUStateFrequencyValue)
			fmt.Fprintf(&b, "|%s|%s|%.9g|%.9g|\n", state, freq, group.DurationMs, group.WindowPct)
		}
		for _, interval := range cpu.Intervals {
			state, freq := cpuStateFrequencyLabels(interval.CPUStateFrequencyValue)
			fmt.Fprintf(&b, "- [%.9f,%.9f)秒：%s，%skHz，%.9gms；状态行=%d，频率行=%d\n", interval.StartTs, interval.EndTs, state, freq, interval.DurationMs, interval.IdleLine, interval.FrequencyLine)
		}
		fmt.Fprintf(&b, "组合数=%d，省略=%d；区间数=%d，省略=%d。\n", cpu.GroupCount, cpu.OmittedGroups, cpu.TotalIntervals, cpu.OmittedIntervals)
	}
	fmt.Fprintf(&b, "展示CPU=%d，省略=%d；省略不改变汇总。active仅表示已观测idle退出，不证明某线程运行；idle编号不自动命名为C1/C2。CPU核时间可相加，不能当成单段墙钟、功耗或响应根因；保留未知，不借其它核频率补齐。\n", shown, p.CPUCount-shown)
	for _, caveat := range p.Caveats {
		fmt.Fprintf(&b, "- %s\n", caveat)
	}
	return b.String()
}

func cpuStateFrequencyLabels(v tracequery.CPUStateFrequencyValue) (string, string) {
	state, freq := "未知", "未知"
	if v.StateKnown {
		state = "非idle（已观测退出）"
		if v.IdleState != nil {
			state = fmt.Sprintf("idle状态%d", *v.IdleState)
			if v.StateEncoding == "native_sql_idle" {
				state = fmt.Sprintf("源CPU状态码%d（含义未核实）", *v.IdleState)
			}
		}
	}
	if v.FrequencyKnown && v.FrequencyKHz != nil {
		freq = strconv.FormatInt(*v.FrequencyKHz, 10)
	}
	return state, freq
}
