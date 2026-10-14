package types

// TraceSchedulerIdentityTeaching describes only the displayed event endpoints.
// It does not resolve generations or widen existing owner/causal contracts.
const TraceSchedulerIdentityTeaching = "调度事件按角色分别列出记录线程、唤醒目标线程、切出/切入线程。载荷 pid/prev_pid/next_pid 表示线程编号（TID），不是所属进程编号；TGID 仅来自独立已知记录，缺失保持未知。编号和名称只复述本条观测，不新增主体归属、线程代次、等待或根因证明。"

func traceSchedulerSemanticFieldMatches(key, eventType string) bool {
	switch key {
	case "scheduler.wakee.comm", "scheduler.wakee.tid", "scheduler.wakee.tgid":
		return eventType == "sched_wakeup" || eventType == "sched_waking"
	case "scheduler.prev.comm", "scheduler.prev.tid", "scheduler.prev.tgid", "scheduler.next.comm", "scheduler.next.tid", "scheduler.next.tgid":
		return eventType == "sched_switch"
	}
	return false
}
