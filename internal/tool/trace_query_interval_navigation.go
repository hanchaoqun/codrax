package tool

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A view's injected default granularity is not a user owner selector. Keep
// explicit scope and every effective owner; normalize only an omitted scope
// on a genuinely owner-free call. The view reapplies its own default at
// execution. The original window-replay parameters remain byte-independent.
func traceQueryIntervalNavigationParams(p traceQueryParams, effective, original json.RawMessage) json.RawMessage {
	var requested, args map[string]json.RawMessage
	if json.Unmarshal(original, &requested) != nil || json.Unmarshal(effective, &args) != nil || args == nil {
		return nil
	}
	if _, explicit := requested["target_scope"]; !explicit && p.PID.Int() == 0 && p.Thread == "" {
		delete(args, "target_scope")
	}
	bound, _ := json.Marshal(args)
	return bound
}

// Navigation is driven by parsed families, never metric-name guesses. Even a
// limited point lookup can discover a family; it cannot prove its population.
func traceQueryNativeIntervalViews(result tracequery.Result) []string {
	seen := map[string]bool{}
	for _, event := range result.Events {
		if item, ok := tracequery.NativeIntervalNavigationForEvent(event.Type); ok {
			seen[item.View] = true
		}
	}
	if result.Measurements != nil && result.Measurements.Status == "available" && tracequery.ValidMeasurements(*result.Measurements) {
		seen[tracequery.ViewMeasurements] = true
	}
	if result.ProcessMeasurements != nil && result.ProcessMeasurements.Status == "available" && tracequery.ValidProcessMeasurements(*result.ProcessMeasurements) {
		seen[tracequery.ViewProcessMeasurements] = true
	}
	// An actual successful calculation also records itself so a later raw
	// search cannot cause the same calculation to be repeated. This is not a
	// claim that legacy cpu_frequency rows were native SQL intervals.
	if result.CPUStateFrequency != nil && result.CPUStateFrequency.Status == "available" && tracequery.ValidCPUStateFrequency(*result.CPUStateFrequency) {
		seen[tracequery.ViewCPUStateFrequency] = true
	}
	var views []string
	for view := range seen {
		views = append(views, view)
	}
	sort.Strings(views)
	return views
}

func traceQueryIntervalNavigationCandidate(result tracequery.Result) types.TraceQueryWindowReplayRef {
	if len(result.TraceArtifacts) != 1 {
		return types.TraceQueryWindowReplayRef{}
	}
	source := result.TraceArtifacts[0]
	if !source.CausalCompatible || source.ClockAlignment != tracequery.TraceClockAlignmentIdentity || source.VirtualLineBase != 0 {
		return types.TraceQueryWindowReplayRef{}
	}
	return types.NativeTraceIntervalNavigationCandidate(result.SourcePath, traceQueryNativeIntervalViews(result))
}

func writeTraceNativeIntervalNavigation(b *strings.Builder, result tracequery.Result) {
	views := traceQueryNativeIntervalViews(result)
	if len(views) == 0 || len(views) == 1 && views[0] == result.View {
		return
	}
	fmt.Fprintf(b, "原生区间数据可用：%s。需要窗口内持续记录时请使用对应视图并保持来源和时间窗；事件检索只展示命中的起点，不能替代跨入窗口的区间。保留原始值类型和独立来源引用，名称不能确定单位、状态含义或同一物理资源。\n", strings.Join(views, ", "))
}
