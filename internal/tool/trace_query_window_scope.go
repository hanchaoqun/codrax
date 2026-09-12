package tool

import (
	"fmt"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Scope is presentation metadata, never a reason to remove a chain, clip an
// account, or change the model's selected causes. Ordinary elected windows
// retain their existing presentation; explicit requests get the shared ruler.
func runtimeTraceQueryScopePreface(text string, scope types.TraceQueryWindowScope, zh bool) string {
	if !scope.RequestedWindowKnown && scope.RequestedWindowCount <= 1 {
		return text
	}
	lang := "en"
	if zh {
		lang = "zh"
	}
	note := scope.Format(lang)
	if note == "" {
		return text
	}
	if text == "" {
		return note
	}
	return note + "\n\n" + text
}

func runtimeTraceQueryScopeTitleSuffix(scope types.TraceQueryWindowScope, zh bool) string {
	if scope.RequestedWindowCount > 1 {
		if scope.Role == types.TraceQueryWindowScopeRequestedPrincipal && scope.RequestedWindowOrdinal > 0 {
			if zh {
				return fmt.Sprintf("（第 %d/%d 个时间窗 %.6f–%.6f 秒）", scope.RequestedWindowOrdinal, scope.RequestedWindowCount, scope.QueryWindowStartTs, scope.QueryWindowEndTs)
			}
			return fmt.Sprintf(" (Window %d/%d %.6f–%.6f seconds)", scope.RequestedWindowOrdinal, scope.RequestedWindowCount, scope.QueryWindowStartTs, scope.QueryWindowEndTs)
		}
		if scope.Role == types.TraceQueryWindowScopeUnknownQueryWindow {
			if zh {
				return "（查询范围未明确）"
			}
			return " (Query window unknown)"
		}
		if zh {
			return fmt.Sprintf("（补充查询 %.6f–%.6f 秒）", scope.QueryWindowStartTs, scope.QueryWindowEndTs)
		}
		return fmt.Sprintf(" (Supplementary query %.6f–%.6f seconds)", scope.QueryWindowStartTs, scope.QueryWindowEndTs)
	}
	if !scope.IsSupportingExploration() {
		return ""
	}
	if zh {
		return "（补充查询范围）"
	}
	return " (Supplementary query window)"
}
