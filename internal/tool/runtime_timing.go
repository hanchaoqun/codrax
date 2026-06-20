package tool

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	toolRuntimeTimingWarnTotalMillis = int64(2000)
	toolRuntimeTimingWarnPhaseMillis = int64(1000)
)

func recordToolRuntimeTiming(dst *[]types.ToolRuntimeTiming, phase string, started time.Time, count int) {
	if dst == nil {
		return
	}
	phase = strings.TrimSpace(phase)
	if phase == "" || started.IsZero() {
		return
	}
	elapsed := time.Since(started).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}
	if count < 0 {
		count = 0
	}
	*dst = append(*dst, types.ToolRuntimeTiming{
		Phase:         phase,
		ElapsedMillis: elapsed,
		Status:        "success",
		Count:         count,
	})
}

func attachToolRuntimeTimings(result *types.ToolResult, timings []types.ToolRuntimeTiming) {
	if result == nil || len(timings) == 0 {
		return
	}
	result.RuntimeTimings = append(result.RuntimeTimings, timings...)
	logToolRuntimeTimings(result.ToolName, timings)
}

func logToolRuntimeTimings(toolName string, timings []types.ToolRuntimeTiming) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" || len(timings) == 0 {
		return
	}
	total := int64(0)
	slowest := types.ToolRuntimeTiming{}
	for _, timing := range timings {
		if timing.ElapsedMillis < 0 {
			continue
		}
		total += timing.ElapsedMillis
		if timing.ElapsedMillis > slowest.ElapsedMillis {
			slowest = timing
		}
	}
	phases := formatToolRuntimeTimingPhases(timings, 6)
	if total >= toolRuntimeTimingWarnTotalMillis || slowest.ElapsedMillis >= toolRuntimeTimingWarnPhaseMillis {
		logging.Warning("[tool_timing] tool=%s total_ms=%d slowest_phase=%s slowest_ms=%d phases=%s",
			toolName, total, slowest.Phase, slowest.ElapsedMillis, phases)
		return
	}
	logging.Debug("[tool_timing] tool=%s total_ms=%d slowest_phase=%s slowest_ms=%d phases=%s",
		toolName, total, slowest.Phase, slowest.ElapsedMillis, phases)
}

func formatToolRuntimeTimingPhases(timings []types.ToolRuntimeTiming, max int) string {
	if max <= 0 || len(timings) == 0 {
		return ""
	}
	limit := len(timings)
	truncated := 0
	if limit > max {
		limit = max
		truncated = len(timings) - max
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		phase := strings.TrimSpace(timings[i].Phase)
		if phase == "" {
			phase = "unknown"
		}
		elapsed := timings[i].ElapsedMillis
		if elapsed < 0 {
			elapsed = 0
		}
		count := timings[i].Count
		if count < 0 {
			count = 0
		}
		parts = append(parts, fmt.Sprintf("%s:%dms/%d", phase, elapsed, count))
	}
	if truncated > 0 {
		parts = append(parts, fmt.Sprintf("+%d", truncated))
	}
	return strings.Join(parts, ",")
}
