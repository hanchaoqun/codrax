package types

import "strings"

// TraceWakeupPreWaitGuidance qualifies an existing edge measurement without
// changing its endpoints, value, or causal authority. Shared by the tool and
// the model handoff so an arrow's left endpoint cannot silently own the wait.
const TraceWakeupPreWaitGuidance = "Each edge's pre_wakeup_wait belongs to the wakee (right-hand endpoint), not the waker: it runs from that wakee's selected sleep/blocking segment start to sched_wakeup. It is not the waker's own sleep, IO wait or running time, not post-wakeup scheduling delay, and not a duration proved to be caused by the waker. Read each thread's own states from its separately measured state account. Express these roles in the answer language rather than copying field names."

// TraceWakeupPreWaitDisplay keeps the measured value adjacent to its existing
// typed owner. The caller supplies the already formatted millisecond value;
// this helper does not infer a state, remeasure an interval, or parse prose.
func TraceWakeupPreWaitDisplay(wakee, valueMS string) string {
	wakee = strings.TrimSpace(wakee)
	if wakee == "" {
		wakee = "(identity not published)"
	}
	return "wakee=" + wakee + " pre_wakeup_wait=" + valueMS + "ms"
}
