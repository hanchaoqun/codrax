package tool

import (
	"fmt"
	"math"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func samplingFactRecordedTime(b *tracequery.PerfSampleTimeBounds, total int) string {
	if b == nil || b.KnownSampleCount < 0 || b.UnknownSampleCount < 0 || b.KnownSampleCount+b.UnknownSampleCount != total {
		return "未知（未提供完整时刻统计）"
	}
	if b.KnownSampleCount == 0 || b.FirstTs == nil || b.LastTs == nil || math.IsNaN(*b.FirstTs) || math.IsNaN(*b.LastTs) || math.IsInf(*b.FirstTs, 0) || math.IsInf(*b.LastTs, 0) || *b.FirstTs > *b.LastTs {
		return "未知（记录时刻不可用）"
	}
	text := fmt.Sprintf("[%s, %s]", traceQueryDisplaySeconds(*b.FirstTs), traceQueryDisplaySeconds(*b.LastTs))
	if b.UnknownSampleCount > 0 {
		text += fmt.Sprintf("（时刻已知%d/%d）", b.KnownSampleCount, total)
	}
	return text
}

// Interpret only the recorded type, never CPU coordinates, sample weights or
// display timestamps. Even on/off-CPU records do not establish an interval.
func samplingFactExecutionBasis(kind string) string {
	switch kind {
	case "on_cpu":
		return "已记录on_cpu（仅采样点）"
	case "off_cpu":
		return "已记录off_cpu（非阻塞区间）"
	default:
		return "未确立（样本类型未知）"
	}
}
