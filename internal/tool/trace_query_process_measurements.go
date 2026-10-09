package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const TraceProcessMeasurementsPredicate = "process_measurements_observation"

func traceQueryProcessMeasurementsSchema(schema json.RawMessage) json.RawMessage {
	var obj map[string]any
	if json.Unmarshal(schema, &obj) != nil {
		return schema
	}
	properties, _ := obj["properties"].(map[string]any)
	for _, key := range []string{"view", "pid", "target_scope"} {
		if field, ok := properties[key].(map[string]any); ok {
			description, _ := field["description"].(string)
			extra := " For process_measurements only, pid selects a process, process scope is the default, and omitting pid queries all recorded owners; thread scope is not supported."
			if key == "view" {
				extra = " " + tracequery.ProcessMeasurementsTeaching
			}
			field["description"] = description + extra
		}
	}
	var out bytes.Buffer
	e := json.NewEncoder(&out)
	e.SetEscapeHTML(false)
	if e.Encode(obj) != nil {
		return schema
	}
	return out.Bytes()
}

func traceQueryProcessMeasurementsInput(p traceQueryParams) (traceQueryParams, *types.ToolResult) {
	if tracequery.CanonicalViewName(p.View) != tracequery.ViewProcessMeasurements && tracequery.CanonicalViewName(p.View) != tracequery.ViewPreferredFrameRate {
		return p, nil
	}
	scope := strings.ToLower(strings.TrimSpace(p.TargetScope))
	if strings.TrimSpace(p.Thread) != "" || scope != "" && scope != tracequery.TargetScopeProcess {
		hint := p.View + " describes process-owned observations. Omit thread; use an optional process pid. Query thread_timeline separately for thread execution."
		return p, &types.ToolResult{ToolName: "trace_query", Summary: hint, Timestamp: time.Now(), Repair: &types.ToolRepair{Code: "trace_query_process_owned_view", Fields: []string{"thread", "target_scope"}, Hint: hint}}
	}
	p.TargetScope = tracequery.TargetScopeProcess
	return p, nil
}

// Only an unambiguous typed process focus can narrow this owner-based view.
// Thread cursors and display names never become process identity.
func traceQueryProcessMeasurementTarget(ctx *types.BusContext, p traceQueryParams) (traceQueryParams, string) {
	if p.PID.Int() > 0 {
		return p, ""
	}
	targets := map[int]bool{}
	collect := func(rm *types.RequestModel) {
		if rm == nil {
			return
		}
		for _, t := range rm.RuntimeTargets {
			if types.NormalizeRuntimeTargetKind(t.Kind) == types.RuntimeTargetKindProcess && t.PID > 0 && t.PID <= traceQueryMaxInheritedPID && t.Active() && !types.RuntimeTargetIsExplorationCursorSource(t.Source) {
				targets[t.PID] = true
			}
		}
	}
	if ctx != nil {
		if ctx.AnalysisIR != nil {
			collect(&ctx.AnalysisIR.RequestModel)
		}
		if ctx.Mutable != nil {
			collect(ctx.Mutable.RequestModel())
		}
	}
	if len(targets) == 1 {
		for pid := range targets {
			p.PID = FlexInt(pid)
		}
		return p, fmt.Sprintf("trace_query_target_inherited=process_measurements process_pid=%d", p.PID.Int())
	}
	return p, "trace_query_target_inheritance_skipped=process_measurements; no unambiguous typed process focus"
}

func (t *TraceQuery) streamProcessMeasurements(ctx *types.BusContext, p traceQueryParams, path, sourceLabel, callCaveat string, window traceQueryNormalizedWindow) (types.ToolResult, bool) {
	if (tracequery.CanonicalViewName(p.View) != tracequery.ViewProcessMeasurements && tracequery.CanonicalViewName(p.View) != tracequery.ViewPreferredFrameRate) || tracequery.TracePathRequiresCompositeIndex(path) {
		return types.ToolResult{}, false
	}
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, window.RequestedStart, window.RequestedEnd)
	result, err := tracequery.StreamProcessMeasurements(contextFromBus(ctx), path, q)
	if err != nil {
		if traceQueryIsCancellation(err) {
			return traceQueryCancellationResult(p.View, path, err), true
		}
		return types.ToolResult{ToolName: t.Name(), Summary: "Process measurement query failed: " + err.Error(), Timestamp: time.Now()}, true
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

func traceQueryProcessMeasurementsObservations(p *tracequery.ProcessMeasurementsResult, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if p == nil || !tracequery.ValidProcessMeasurements(*p) {
		return nil
	}
	r := types.ObservationRecord{ID: "trace_query:" + scope + "#process_measurements", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref, Span: types.ObservationSpan{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs},
		ClaimKey: TraceProcessMeasurementsPredicate, Predicate: TraceProcessMeasurementsPredicate, Subject: "process measurements", Object: p.Status,
		Summary: "Process-owned source values and intervals; missing values remain unknown, not thread execution or causal evidence", ObservedAt: at, Confidence: 1}
	receipt := traceQueryProcessMeasurementsReceipt(r, *p)
	if receipt == "" {
		return nil
	}
	r.RichNotes = []string{receipt}
	return []types.ObservationRecord{r}
}

func TraceProcessMeasurementsText(p tracequery.ProcessMeasurementsResult) string {
	var b strings.Builder
	right := ")"
	if p.Window.EndInclusive {
		right = "]"
	}
	fmt.Fprintf(&b, "进程量测；查询=[%.9f,%.9f%s秒；状态=%s；匹配记录=%d，展示=%d，省略=%d；无法定位时间的记录=%d（不计入窗口总体）。\n", p.Window.StartTs, p.Window.EndTs, right, p.Status, p.TotalRows, len(p.Rows), p.OmittedRows, p.UnpositionedRows)
	b.WriteString("原始记录各自保留，不填区间空洞或累加重叠。本原值视图不解释单位及累计/存量含义；无已验证协议时不由指标名称推断。已识别精确协议时另列可选统计导航；这些量测不代表线程执行或响应根因。\n")
	for _, note := range traceProcessMeasurementNavigation(p) {
		b.WriteString(note + "\n")
	}
	for i, row := range p.Rows {
		if i == 8 {
			fmt.Fprintf(&b, "预览另省略%d条；完整保留行可从测量表选择。\n", len(p.Rows)-i)
			break
		}
		fmt.Fprintf(&b, "- %s；指标=%q；原值=%s；原起点=%s ns；原持续=%s ns；窗口内=%s..%s ns；来源行=%d。\n", processMeasurementOwner(row.Record), row.Record.Name, processMeasurementScalar(row.Record.Value), processMeasurementScalar(row.Record.StartNS), processMeasurementScalar(row.Record.DurationNS), processMeasurementEndpoint(row.ClippedStartNS), processMeasurementEndpoint(row.ClippedEndNS), row.SourceLine)
	}
	for _, note := range p.Caveats {
		fmt.Fprintf(&b, "- %s\n", note)
	}
	return b.String()
}

func traceProcessMeasurementNavigation(p tracequery.ProcessMeasurementsResult) []string {
	var notes []string
	for _, view := range p.AvailableDerivedViews {
		notes = append(notes, fmt.Sprintf("可选原生统计：view=%s，沿用当前来源、窗口和进程选择；完整扫描识别了其精确协议。原值可直接引用；如需覆盖、持续时间或分布，可用该视图或等价的已验证统计。只问原值时无需额外查询，不能由原始行填补未知或重叠。", view))
	}
	return notes
}
