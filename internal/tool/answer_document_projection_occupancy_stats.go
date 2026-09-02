package tool

import (
	"fmt"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/types"
)

// B1549: a projection node can already be a per-CPU or whole-window account.
// Its display-fold members are records, not a physical occurrence census.
// Missing member statistics stay missing; the cumulative value cannot stand
// in for a longest occurrence, even when only one projection row remains.
// This output-only helper changes no causal, pricing, or model-owned value.
func runtimeTraceOccupancyNodeCountAndMax(node types.TraceCausalProjectionNode) (int, float64) {
	if node.FamilyMemberCount > 1 {
		return node.FamilyMemberCount, node.FamilyMemberMaxMS
	}
	if node.MergedCount > 1 {
		return node.MergedCount, node.MergedMaxMS
	}
	return 0, 0
}

func runtimeTraceOccupancyStatisticCells(row runtimeTraceOccupancyCandidate, zh bool) (string, string) {
	maximum, count := "—", "—"
	if row.maxMS > 0 {
		maximum = fmt.Sprintf("%.3f%s", row.maxMS, row.unit)
		if row.recordStatistics {
			if zh {
				maximum += "（记录最大值）"
			} else {
				maximum += " (record maximum)"
			}
		}
	}
	if row.count > 0 {
		count = strconv.Itoa(row.count)
		if row.recordStatistics {
			if zh {
				count += " 条统计记录"
			} else {
				count += " records"
			}
		}
	}
	return maximum, count
}

func runtimeTraceOccupancyStatisticsLegend(zh bool) string {
	if zh {
		return "\n\n数量与最长值的口径：状态/路径及语义工作行只列已发布的统计记录数和记录最大值，一条记录可能已汇总多次发生，不能当作物理发生次数或单次最长。业务 span 族的数量和最长值来自逐个 span；— 表示未提供相应统计，不是零，也不以累计量代替。"
	}
	return "\n\nCount/maximum basis: state/path and semantic-work rows show only published record counts and record maxima. One record may already aggregate multiple occurrences, so these are not physical occurrence counts or longest-single durations. Business-span families use individual-span counts and maxima. — means the corresponding statistic was not supplied, not zero; cumulative occupancy is never substituted."
}
