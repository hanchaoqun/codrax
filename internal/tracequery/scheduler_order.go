package tracequery

import "fmt"

// schedulerOrderViolation is a precise per-lane timestamp regression. Global
// ftrace rows from independent CPUs may interleave out of timestamp order and
// remain computable; a single CPU or TID moving backwards does not have a
// provable elapsed-time interpretation and scheduler duration faces fail
// closed for that query.
type schedulerOrderViolation struct {
	Lane       string
	ID         int
	PreviousTs float64
	CurrentTs  float64
	Line       int
	SourcePath string
}

func (v *schedulerOrderViolation) reason() string {
	if v == nil {
		return ""
	}
	reason := fmt.Sprintf("scheduler_lane_timestamp_regressed lane=%s id=%d previous_ts=%.6f current_ts=%.6f line=%d", v.Lane, v.ID, v.PreviousTs, v.CurrentTs, v.Line)
	if v.SourcePath != "" {
		reason += fmt.Sprintf(" source=%s", v.SourcePath)
	}
	return reason
}

type schedulerOrderTracker struct {
	lastPID map[int]float64
	lastCPU map[int]float64
}

func newSchedulerOrderTracker() *schedulerOrderTracker {
	return &schedulerOrderTracker{lastPID: map[int]float64{}, lastCPU: map[int]float64{}}
}

func (t *schedulerOrderTracker) observe(ev Event, onlyPID int) *schedulerOrderViolation {
	if t == nil {
		return nil
	}
	observePID := func(pid int) *schedulerOrderViolation {
		if pid <= 0 || (onlyPID > 0 && pid != onlyPID) {
			return nil
		}
		if previous, ok := t.lastPID[pid]; ok && ev.Ts < previous {
			t.lastPID[pid] = ev.Ts
			return &schedulerOrderViolation{Lane: "tid", ID: pid, PreviousTs: previous, CurrentTs: ev.Ts, Line: ev.Line}
		}
		t.lastPID[pid] = ev.Ts
		return nil
	}
	switch ev.Type {
	case EventSchedWakeup, EventSchedWaking:
		if schedWakeupStartsNewIncarnation(ev) {
			// Exact lifecycle reset: the reused numeric TID begins a new lane.
			if ev.WakeePID > 0 && (onlyPID <= 0 || ev.WakeePID == onlyPID) {
				t.lastPID[ev.WakeePID] = ev.Ts
			}
			return nil
		}
		return observePID(ev.WakeePID)
	case EventSchedBlockedReason:
		return observePID(ev.WakeePID)
	case EventCPUConstraint:
		if cf := ev.ConstraintFields; cf != nil && cf.Kind == "sched_migrate_task" {
			return observePID(cf.PID)
		}
	case EventSchedSwitch:
		if onlyPID <= 0 {
			if previous, ok := t.lastCPU[ev.CPU]; ok && ev.Ts < previous {
				t.lastCPU[ev.CPU] = ev.Ts
				return &schedulerOrderViolation{Lane: "cpu", ID: ev.CPU, PreviousTs: previous, CurrentTs: ev.Ts, Line: ev.Line}
			}
			t.lastCPU[ev.CPU] = ev.Ts
		}
		if violation := observePID(ev.NextPID); violation != nil {
			return violation
		}
		if violation := observePID(ev.PrevPID); violation != nil {
			return violation
		}
		if stateFromPrevState(ev.PrevState) == StateDead && ev.PrevPID > 0 {
			delete(t.lastPID, ev.PrevPID)
		}
	}
	return nil
}

const schedulerOrderFailureCap = 64

// auditSchedulerOrderEvent returns every independently observable CPU/TID
// rollback for one physical scheduler row. The split trackers prevent a CPU
// violation from masking a simultaneous target-TID violation in a composite or
// window-pruned index.
func auditSchedulerOrderEvent(cpuTracker, pidTracker *schedulerOrderTracker, ev Event) []schedulerOrderViolation {
	var out []schedulerOrderViolation
	add := func(v *schedulerOrderViolation) {
		if v == nil {
			return
		}
		for _, existing := range out {
			if existing.Lane == v.Lane && existing.ID == v.ID && existing.Line == v.Line && existing.PreviousTs == v.PreviousTs && existing.CurrentTs == v.CurrentTs {
				return
			}
		}
		out = append(out, *v)
	}
	add(cpuTracker.observe(ev, 0))
	for _, pid := range schedulerEventRolePIDs(ev) {
		add(pidTracker.observe(ev, pid))
	}
	return out
}

func schedulerOrderFailuresFromEvents(events []Event, q Query, onlyPID, limit int) ([]schedulerOrderViolation, bool) {
	if limit <= 0 || limit > schedulerOrderFailureCap {
		limit = schedulerOrderFailureCap
	}
	cpuTracker, pidTracker := newSchedulerOrderTracker(), newSchedulerOrderTracker()
	out := make([]schedulerOrderViolation, 0, min(limit, 8))
	truncated := false
	for _, ev := range events {
		for _, failure := range auditSchedulerOrderEvent(cpuTracker, pidTracker, ev) {
			if !schedulerOrderViolationRelevantToQuery(&failure, q, onlyPID) {
				continue
			}
			if len(out) >= limit {
				truncated = true
				continue
			}
			out = append(out, failure)
		}
	}
	return out, truncated
}

func schedulerStateOrderViolationForQuery(idx *Index, q Query, onlyPID int) *schedulerOrderViolation {
	if idx == nil {
		return nil
	}
	// Composite indexes are canonically sorted only after each physical child
	// has been parsed. Preserve a child-local lane rollback as a typed poison
	// marker; otherwise sorting would erase the only proof that elapsed
	// scheduler durations are unsafe.
	for i := range idx.schedulerOrderFailures {
		if schedulerOrderViolationRelevantToQuery(&idx.schedulerOrderFailures[i], q, onlyPID) {
			copy := idx.schedulerOrderFailures[i]
			return &copy
		}
	}
	if idx.schedulerOrderFailuresCapped {
		lane, id := "scheduler", -1
		if onlyPID > 0 {
			lane, id = "tid", onlyPID
		}
		return &schedulerOrderViolation{Lane: lane, ID: id, SourcePath: "order_audit_truncated"}
	}
	tracker := newSchedulerOrderTracker()
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) {
			continue
		}
		if violation := tracker.observe(ev, onlyPID); schedulerOrderViolationRelevantToQuery(violation, q, onlyPID) {
			return violation
		}
	}
	return nil
}

func schedulerOrderViolationRelevantToQuery(v *schedulerOrderViolation, q Query, onlyPID int) bool {
	if v == nil {
		return false
	}
	if onlyPID > 0 && v.Lane == "tid" && v.ID != onlyPID {
		return false
	}
	// CPU-lane rollback is relevant to global aggregates, but a target-only
	// timeline reads its TID lane and is not invalidated by an unrelated CPU.
	if onlyPID > 0 && v.Lane == "cpu" {
		return false
	}
	if q.LineStart > 0 && v.Line < q.LineStart {
		return false
	}
	if q.LineEnd > 0 && v.Line > q.LineEnd {
		return false
	}
	if q.LineStart == 0 && q.LineEnd == 0 && q.TimeEnd > 0 && v.PreviousTs > q.TimeEnd && v.CurrentTs > q.TimeEnd {
		return false
	}
	return true
}
