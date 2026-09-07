package agent

// traceBlockingCapacityScopeNote explains only the existing typed capacity
// status. It is model-facing guidance, not a completeness/causality decision:
// a capped result says nothing about capture loss, and other missing-evidence
// reasons must not be silently renamed to result-capacity limits.
func traceBlockingCapacityScopeNote(status string, zh bool) string {
	if status != "lower_bound_capacity_truncated" {
		return ""
	}
	if zh {
		return "此处容量截断指查询返回条目或链遍历结果的容量裁剪，不代表 Trace 采集缺失，也不据此证明采集完整；缺失收尾事件或未知状态仍需各自独立的证据说明。"
	}
	return "Here capacity truncation refers to query-returned rows or chain-traversal result limits, not evidence of trace capture loss or capture completeness. Missing closure events or unknown states still require their own separate evidence."
}
