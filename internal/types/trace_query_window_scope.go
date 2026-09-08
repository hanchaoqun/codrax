package types

import (
	"fmt"
	"math"
	"strings"
)

// TraceQueryWindowScopeRole describes a query's time ruler, not causal
// eligibility, enumeration completeness, or whether another query exists.
type TraceQueryWindowScopeRole string

const (
	TraceQueryWindowScopeRequestedPrincipal    TraceQueryWindowScopeRole = "requested_scope_principal"
	TraceQueryWindowScopeSupportingExploration TraceQueryWindowScopeRole = "supporting_exploration"
	TraceQueryWindowScopeElectedQueryWindow    TraceQueryWindowScopeRole = "elected_query_window"
	TraceQueryWindowScopeUnknownQueryWindow    TraceQueryWindowScopeRole = "unknown_query_window"
)

// TraceQueryWindowScope keeps the validated request ruler separate from the
// actual query ruler. It is compiler-owned disclosure metadata: consumers must
// not use it to clip values, change a target, or remove a valid exploration.
// The zero value denotes a legacy carrier without this disclosure metadata.
type TraceQueryWindowScope struct {
	Role                   TraceQueryWindowScopeRole `json:"role"`
	RequestedWindowKnown   bool                      `json:"requested_window_known,omitempty"`
	RequestedWindowStartTs float64                   `json:"requested_window_start_ts,omitempty"`
	RequestedWindowEndTs   float64                   `json:"requested_window_end_ts,omitempty"`
	QueryWindowStartTs     float64                   `json:"query_window_start_ts,omitempty"`
	QueryWindowEndTs       float64                   `json:"query_window_end_ts,omitempty"`
}

// ResolveTraceQueryWindowScope reads only the already-validated request
// profile and producer-owned query endpoints. It never parses a request quote
// or an occurrence interval to recover a missing query window.
func ResolveTraceQueryWindowScope(requested *RuntimeArtifactScopeProfile, start, end float64) TraceQueryWindowScope {
	scope := TraceQueryWindowScope{}
	if requestedStart, requestedEnd, ok := requested.ExplicitTimeWindow(); ok &&
		traceQueryScopeWindowPresent(requestedStart, requestedEnd) {
		scope.RequestedWindowKnown = true
		scope.RequestedWindowStartTs, scope.RequestedWindowEndTs = requestedStart, requestedEnd
	}
	return scope.ForWindow(start, end)
}

// ForWindow reuses the request authority with a different actual QUERY window
// (for example a board or state account). It must not receive a node's own
// occurrence interval. Classification never edits either input ruler.
func (scope TraceQueryWindowScope) ForWindow(start, end float64) TraceQueryWindowScope {
	out := TraceQueryWindowScope{
		Role:                   TraceQueryWindowScopeUnknownQueryWindow,
		RequestedWindowKnown:   scope.RequestedWindowKnown && traceQueryScopeWindowPresent(scope.RequestedWindowStartTs, scope.RequestedWindowEndTs),
		RequestedWindowStartTs: scope.RequestedWindowStartTs,
		RequestedWindowEndTs:   scope.RequestedWindowEndTs,
	}
	if !out.RequestedWindowKnown {
		out.RequestedWindowStartTs, out.RequestedWindowEndTs = 0, 0
	}
	if !traceQueryScopeWindowPresent(start, end) {
		return out
	}
	out.QueryWindowStartTs, out.QueryWindowEndTs = start, end
	out.Role = TraceQueryWindowScopeElectedQueryWindow
	if out.RequestedWindowKnown {
		out.Role = TraceQueryWindowScopeSupportingExploration
		if TraceCausalProjectionPrincipalValueSameWindow(out.RequestedWindowStartTs, out.RequestedWindowEndTs, start, end) {
			out.Role = TraceQueryWindowScopeRequestedPrincipal
		}
	}
	return out
}

func (scope TraceQueryWindowScope) IsSupportingExploration() bool {
	return scope.Role == TraceQueryWindowScopeSupportingExploration
}

func traceQueryScopeWindowPresent(start, end float64) bool {
	return !math.IsInf(start, 0) && !math.IsInf(end, 0) &&
		TraceCausalProjectionWindowPresent(start, end)
}

// Format is the shared reader-facing scope explanation. Matching windows do
// not imply complete coverage; a mismatching query does not imply the absence
// of an exact-window account elsewhere. Internal role tokens never appear.
func (scope TraceQueryWindowScope) Format(lang string) string {
	if scope.Role == "" {
		return ""
	}
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	if scope.Role == TraceQueryWindowScopeUnknownQueryWindow {
		if scope.RequestedWindowKnown {
			if zh {
				return fmt.Sprintf("用户指定范围 %.6f–%.6f 秒；实际查询范围未明确，不能视为指定范围的账户", scope.RequestedWindowStartTs, scope.RequestedWindowEndTs)
			}
			return fmt.Sprintf("requested window %.6f–%.6f seconds; actual query window is unknown and cannot be treated as an account of the requested window", scope.RequestedWindowStartTs, scope.RequestedWindowEndTs)
		}
		if zh {
			return "实际查询范围未明确"
		}
		return "actual query window is unknown"
	}
	if scope.Role == TraceQueryWindowScopeSupportingExploration {
		if zh {
			return fmt.Sprintf("用户指定范围 %.6f–%.6f 秒；补充查询范围 %.6f–%.6f 秒；本查询数值不能代替指定范围的独立账户", scope.RequestedWindowStartTs, scope.RequestedWindowEndTs, scope.QueryWindowStartTs, scope.QueryWindowEndTs)
		}
		return fmt.Sprintf("requested window %.6f–%.6f seconds; supplementary query window %.6f–%.6f seconds; this query does not substitute for an account of the requested window", scope.RequestedWindowStartTs, scope.RequestedWindowEndTs, scope.QueryWindowStartTs, scope.QueryWindowEndTs)
	}
	if scope.Role == TraceQueryWindowScopeRequestedPrincipal {
		if zh {
			return fmt.Sprintf("用户指定范围 %.6f–%.6f 秒；查询范围 %.6f–%.6f 秒（与指定范围一致）", scope.RequestedWindowStartTs, scope.RequestedWindowEndTs, scope.QueryWindowStartTs, scope.QueryWindowEndTs)
		}
		return fmt.Sprintf("requested window %.6f–%.6f seconds; query window %.6f–%.6f seconds (matches the requested window)", scope.RequestedWindowStartTs, scope.RequestedWindowEndTs, scope.QueryWindowStartTs, scope.QueryWindowEndTs)
	}
	if scope.Role == TraceQueryWindowScopeElectedQueryWindow {
		if zh {
			return fmt.Sprintf("查询范围 %.6f–%.6f 秒（未绑定明确的用户时间窗）", scope.QueryWindowStartTs, scope.QueryWindowEndTs)
		}
		return fmt.Sprintf("query window %.6f–%.6f seconds (no explicit requested time window is bound)", scope.QueryWindowStartTs, scope.QueryWindowEndTs)
	}
	return ""
}
