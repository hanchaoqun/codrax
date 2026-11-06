package tool

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// These are display values from the same group/window as the other native
// scalars. Keep one formatting path for the tool head, investigation/final
// context and source-bound table notes; nil must not become a printed zero.
func traceQueryIntervalDepthDuration(d *tracequery.IntervalDepthDuration) string {
	if d == nil {
		return ""
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'g', 12, 64) }
	return fmt.Sprintf("峰值持续时间：%s ms（同时 %d 个，占全窗 %s%%，分母 %s ms）；本组零贡献 %s ms。",
		f(d.PeakDurationMs), d.PeakDepth, f(d.PeakDurationMs/d.WindowMs*100), f(d.WindowMs), f(d.ZeroContributionMs))
}

func appendTraceQueryIntervalDepthDuration(summary string, d *tracequery.IntervalDepthDuration) string {
	if text := traceQueryIntervalDepthDuration(d); text != "" {
		return summary + "；" + text
	}
	return summary
}

// Keep a few independently identified whole groups ahead of bulky timeline,
// stack and member data. The original payload retains each admitted group's
// values, unchanged distribution/bucket limits and unavailable diagnostics.
func writeTraceIntervalDepthDurationHead(b *strings.Builder, stats *tracequery.WindowStats) {
	if stats == nil {
		return
	}
	const limit = 2 // per family; a large IO roster cannot consume scheduler's seats
	basisWritten := false
	write := func(group, source string, start, end float64, d *tracequery.IntervalDepthDuration) bool {
		value := traceQueryIntervalDepthDuration(d)
		if value == "" {
			return false
		}
		if !basisWritten {
			b.WriteString("- 峰值持续时间与非零覆盖时长口径不同；各组仅含已确认区间，不证明系统或设备饱和、空闲或响应根因。\n")
			basisWritten = true
		}
		fmt.Fprintf(b, "  - %s，来源 %s，[%s, %s) 秒：%s\n", group, sanitizeForBanner(source),
			traceQueryDisplaySeconds(start), traceQueryDisplaySeconds(end), value)
		return true
	}
	writeOmissions := func(label string, shown, retained, total int) {
		if shown > 0 && shown < total {
			fmt.Fprintf(b, "  - %s的峰值时长预览：展示 %d 组；结果保留 %d / %d 组，包含未测量组；其余已保留组见 payload_ref。\n", label, shown, retained, total)
		}
	}
	if s := stats.SchedulerConcurrency; s != nil && s.Window != nil {
		shown := 0
		for _, g := range s.Groups {
			if write(traceQuerySchedulerConcurrencyState(g.State), g.SourcePath, s.Window.StartTs, s.Window.EndTs, g.DepthDuration) {
				shown++
				if shown == limit {
					break
				}
			}
		}
		writeOmissions("调度", shown, len(s.Groups), s.GroupCount)
	}
	if s := stats.IOInFlight; s != nil && s.Window != nil {
		shown := 0
		for _, g := range s.Groups {
			if write("同时在途请求数 "+traceQueryIOInFlightGroup(g), g.SourcePath, s.Window.StartTs, s.Window.EndTs, g.DepthDuration) {
				shown++
				if shown == limit {
					break
				}
			}
		}
		writeOmissions("IO 在途", shown, len(s.Groups), s.GroupCount)
	}
}
