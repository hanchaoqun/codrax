package tracequery

import "testing"

func perfTimelineTestSample(line int, ts float64, tid int, comm string) Event {
	return Event{
		Line: line, Ts: ts, Type: EventPerfSample, PID: tid, Comm: comm, CPU: 0,
		PerfFields: &PerfFields{TID: tid, PID: tid, Comm: comm, Period: 1, Symbol: "work"},
	}
}

func perfTimelineTestReuse(line int, ts float64, tid int) []Event {
	return []Event{
		{Line: line, Ts: ts, Type: EventSchedSwitch, CPU: 0, NextPID: tid, NextComm: "old"},
		{Line: line + 1, Ts: ts + 0.1, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 7, WakeePID: tid, WakeeComm: "new"},
	}
}

func TestPerfTimelineIgnoresUnrelatedReusedTIDWithoutPerfContribution(t *testing.T) {
	events := append(perfTimelineTestReuse(1, 1.0, 900), perfTimelineTestSample(3, 1.3, 100, "target"))
	idx := &Index{Events: events, FirstTs: 1, LastTs: 1.3, TimestampOrder: TraceTimestampOrderMonotonic}

	res := BuildPerfTimeline(idx, Query{PID: 100, TimeStart: 0.9, TimeEnd: 1.5, MinDurationMs: 1})
	if len(res.Buckets) != 1 {
		t.Fatalf("unrelated tid=900 reuse must not erase pid=100 perf bucket: %+v", res)
	}
	if containsSubstring(res.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("unrelated non-contributor reuse must not emit an identity failure: %v", res.Caveats)
	}

	unfiltered := BuildPerfTimeline(idx, Query{TimeStart: 0.9, TimeEnd: 1.5, MinDurationMs: 1})
	if len(unfiltered.Buckets) != 1 || containsSubstring(unfiltered.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("unfiltered view depends on actual perf contributors, not every scheduler tid: %+v", unfiltered)
	}
}

func TestPerfTimelineFailsClosedWhenReusedTIDContributes(t *testing.T) {
	events := append(perfTimelineTestReuse(1, 1.0, 100), perfTimelineTestSample(3, 1.3, 100, "target"))
	idx := &Index{Events: events, FirstTs: 1, LastTs: 1.3, TimestampOrder: TraceTimestampOrderMonotonic}

	res := BuildPerfTimeline(idx, Query{PID: 100, TimeStart: 0.9, TimeEnd: 1.5, MinDurationMs: 1})
	if len(res.Buckets) != 0 || !containsSubstring(res.Caveats, "thread_identity_fail_closed=true") || !containsSubstring(res.Caveats, "tid=100") {
		t.Fatalf("a reused contributing tid must fail the whole PID-keyed timeline closed: %+v", res)
	}
}

func TestPerfTimelineCommFilterFailsWhenAnyMatchedContributorReusesTID(t *testing.T) {
	events := append(perfTimelineTestReuse(1, 1.0, 101),
		perfTimelineTestSample(3, 1.25, 100, "render"),
		perfTimelineTestSample(4, 1.30, 101, "render"),
	)
	idx := &Index{Events: events, FirstTs: 1, LastTs: 1.3, TimestampOrder: TraceTimestampOrderMonotonic}

	res := BuildPerfTimeline(idx, Query{Thread: "render", TimeStart: 0.9, TimeEnd: 1.5, MinDurationMs: 1})
	if len(res.Buckets) != 0 || !containsSubstring(res.Caveats, "tid=101") {
		t.Fatalf("comm-filtered contributors cannot merge across one reused tid: %+v", res)
	}
}

func TestPerfTimelineLifecycleAuditCapRemainsFailClosed(t *testing.T) {
	idx := &Index{
		Events:                          []Event{perfTimelineTestSample(1, 1.1, 100, "target")},
		FirstTs:                         1.1,
		LastTs:                          1.1,
		TimestampOrder:                  TraceTimestampOrderMonotonic,
		threadIncarnationFailuresCapped: true,
	}
	res := BuildPerfTimeline(idx, Query{PID: 100, TimeStart: 1, TimeEnd: 1.2, MinDurationMs: 1})
	if len(res.Buckets) != 0 || !containsSubstring(res.Caveats, "lifecycle_audit_truncated") {
		t.Fatalf("a capped lifecycle audit cannot prove a contributor safe: %+v", res)
	}
}
