package tool

import (
	"fmt"
	"math"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ActualTotalMS sums the original state segments selected by this occurrence.
// It does not itself establish a same-state or outer-window total.
// ActualWindow is only their locating envelope: never derive a duration or a
// continuous-running claim from it. The projected state split cannot explain
// the actual split because clipping can change each component independently.
func runtimeTraceProjActualCaliberText(node types.TraceCausalProjectionNode, zh bool) string {
	start, end := node.ActualWindowStartTs, node.ActualWindowEndTs
	if !math.IsNaN(start) && !math.IsNaN(end) && !math.IsInf(start, 0) && !math.IsInf(end, 0) && start >= 0 && end > start {
		if zh {
			return fmt.Sprintf("状态段 %.3fms / 所选原始状态段合计 %.3fms；原始段范围 %.6f..%.6fs（定位包络不证明连续执行）；合计可含不同状态，不能据此替代同状态、业务窗或全查询窗统计",
				node.ActualImpactMS, node.ActualTotalMS, start, end)
		}
		return fmt.Sprintf("state segment %.3fms / selected original-state segments total %.3fms; original-segment extent %.6f..%.6fs (the locating envelope does not establish continuous execution); may include different states and does not by itself establish a same-state, business-window or whole-query-window total",
			node.ActualImpactMS, node.ActualTotalMS, start, end)
	}
	// Legacy records without a published valid extent keep their existing
	// two-value disclosure. Never borrow the projection/query/business window.
	if zh {
		return fmt.Sprintf("状态段 %.3fms/线程合计 %.3fms(两口径,来源不同)", node.ActualImpactMS, node.ActualTotalMS)
	}
	return fmt.Sprintf("state segment %.3fms / thread-level total %.3fms (two calibers, different sources)", node.ActualImpactMS, node.ActualTotalMS)
}
