package tool

import (
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
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
}
