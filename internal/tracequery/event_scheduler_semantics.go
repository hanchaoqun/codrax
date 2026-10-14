package tracequery

import "strconv"

// A display projection of already admitted typed endpoints, not another parser
// or thread resolver. Event transports do not retain endpoint presence for 0;
// keep that ambiguity explicit rather than interpreting a default as idle.
func (p *traceEventSemanticProjector) schedulerIdentities(event Event) {
	coordinates := ProjectTraceEventInventoryCoordinates(event)
	endpoint := func(role, comm string, tid int) {
		prefix := "scheduler." + role + "."
		p.optional(prefix+"comm", comm)
		known := tid > 0 && int64(tid) <= 2147483647
		if known {
			p.known(prefix+"tid", strconv.Itoa(tid))
		} else {
			p.unknown(prefix+"tid", "unavailable", "endpoint_tid_presence_unverified")
		}
		// Same physical row + same positive TID can reuse its independently
		// observed TGID. No neighboring name match or tid==tgid assumption.
		if known && coordinates.EmitterTIDKnown && coordinates.EmitterTGIDKnown &&
			tid == coordinates.EmitterTID && int64(coordinates.EmitterTGID) <= 2147483647 {
			p.known(prefix+"tgid", strconv.Itoa(coordinates.EmitterTGID))
		} else {
			p.unknown(prefix+"tgid", "unavailable", "endpoint_process_not_recorded")
		}
	}
	switch event.Type {
	case EventSchedWakeup, EventSchedWaking:
		endpoint("wakee", event.WakeeComm, event.WakeePID)
	case EventSchedSwitch:
		endpoint("prev", event.PrevComm, event.PrevPID)
		endpoint("next", event.NextComm, event.NextPID)
	}
}
