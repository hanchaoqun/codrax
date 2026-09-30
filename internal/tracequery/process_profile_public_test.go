package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const processProfileTrace = `# tracer: nop
ui-10 (10) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ui next_pid=10 next_prio=120
worker-11 (10) [001] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=11 next_prio=120
ui-10 (10) [000] .... 1.002000: tracing_mark_write: B|10|Load
worker-11 (10) [001] .... 1.003000: tracing_mark_write: B|10|Load
worker-11 (10) [001] .... 1.004000: sched_switch: prev_comm=worker prev_pid=11 prev_prio=120 prev_state=D ==> next_comm=idle next_pid=0 next_prio=120
worker-11 (10) [001] .... 1.004010: sched_blocked_reason: pid=11 iowait=1 caller=submit_bio
ui-10 (10) [000] .... 1.005000: sched_switch: prev_comm=ui prev_pid=10 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
irq-90 (90) [000] .... 1.007000: sched_wakeup: comm=ui pid=10 prio=120 target_cpu=000
irq-90 (90) [001] .... 1.008000: sched_wakeup: comm=worker pid=11 prio=120 target_cpu=001
ui-10 (10) [000] .... 1.008000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ui next_pid=10 next_prio=120
worker-11 (10) [001] .... 1.009000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=11 next_prio=120
ui-10 (10) [000] .... 1.010000: tracing_mark_write: E|10
worker-11 (10) [001] .... 1.010000: tracing_mark_write: E|10
late-12 (10) [002] .... 1.011000: tracing_mark_write: I|10|ObservedOnly
ui-10 (10) [000] .... 1.020000: sched_switch: prev_comm=ui prev_pid=10 prev_prio=120 prev_state=D ==> next_comm=idle next_pid=0 next_prio=120
worker-11 (10) [001] .... 1.020000: sched_switch: prev_comm=worker prev_pid=11 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
outside-13 (10) [002] .... 1.020000: tracing_mark_write: I|10|RightBoundary
`

func profileResult(t *testing.T, text string, limit int) Result {
	t.Helper()
	idx := buildTraceIndex(t, "profile.systrace", text)
	r := Run(idx, Query{View: "process_profile", PID: 10, TimeStart: 1, TimeEnd: 1.02, TimeStartSet: true, TimeEndSet: true, Limit: limit})
	if r.ProcessProfile == nil {
		t.Fatalf("missing profile: %+v", r)
	}
	return r
}

func TestProcessProfileFullCensusBeforeNativeLimit(t *testing.T) {
	text := processProfileTrace
	for i := 100; i < 155; i++ {
		text += fmt.Sprintf("member-%d (10) [002] .... 1.012000: tracing_mark_write: I|10|seen\n", i)
	}
	p := profileResult(t, text, 500).ProcessProfile
	if p.ThreadCount != 58 || p.EmittedThreads != 40 || p.OmittedThreads != 18 || p.UnavailableThreads != 56 || !ValidProcessProfile(*p) {
		t.Fatalf("capacity changes statistics: %+v", p)
	}
}

func TestProcessProfileSleepGroupCountBeforeLimit(t *testing.T) {
	var tl TimelineResult
	for i := 0; i < 12; i++ {
		tl.Intervals = append(tl.Intervals, Interval{State: StateSSleep, DurationMs: float64(i + 1), BlockedReasonCaller: fmt.Sprintf("caller_%d", i)})
	}
	rows, total := processProfileSleepGroups(tl)
	if total != 12 || len(rows) != 8 || rows[0].DurationMs != 12 || rows[7].DurationMs != 5 {
		t.Fatalf("groups: %d %+v", total, rows)
	}
}

func TestProcessProfileRejectsScopeWithoutWholeWindow(t *testing.T) {
	for _, q := range []Query{
		{View: "process_profile", PID: 10, TimeStart: 1, TimeEnd: 1},
		{View: "process_profile", PID: 10, TimeStart: 1, TimeEnd: 1.02, LineStart: 2, LineEnd: 17},
	} {
		idx := buildTraceIndex(t, "profile.systrace", processProfileTrace)
		r := Run(idx, q)
		if r.ProcessProfile != nil && (r.ProcessProfile.Status != "unavailable" || len(r.ProcessProfile.Threads) > 0) {
			t.Fatalf("scope fabricated denominator: %+v", r.ProcessProfile)
		}
	}
	idx := buildTraceIndex(t, "profile.systrace", processProfileTrace)
	idx.RelationScoped = true
	r := Run(idx, Query{View: "process_profile", PID: 10, TimeStart: 1, TimeEnd: 1.02})
	if r.ProcessProfile != nil && r.ProcessProfile.Status != "unavailable" {
		t.Fatal("relation-only scope published membership")
	}
}

func TestProcessProfilePublicStatesCapacityAndUnknown(t *testing.T) {
	r := profileResult(t, processProfileTrace, 0)
	p := r.ProcessProfile
	if p.Status != "observed_members" || p.ThreadCount != 3 || len(p.Threads) != 3 || p.UnavailableThreads != 1 {
		t.Fatalf("census: %+v", p)
	}
	for _, row := range p.Threads {
		switch row.Thread.PID {
		case 10:
			v := row.States.Values
			if v == nil || !near(v.RunningMs, 17, 1e-6) || !near(v.SleepMs, 2, 1e-6) || !near(v.RunnableMs, 1, 1e-6) || row.RunningWindowPct == nil || !near(*row.RunningWindowPct, 85, 1e-6) {
				t.Fatalf("ui: %+v", row)
			}
			if len(row.SleepGroups) != 1 || row.SleepGroups[0].CallerKnown {
				t.Fatalf("missing caller fabricated: %+v", row)
			}
		case 11:
			v := row.States.Values
			if v == nil || !near(v.RunningMs, 15, 1e-6) || !near(v.IOWaitMs, 4, 1e-6) || !near(v.DStateMs, 0, 1e-6) {
				t.Fatalf("worker: %+v", row)
			}
			if len(row.SleepGroups) != 1 || row.SleepGroups[0].Caller != "submit_bio" {
				t.Fatalf("caller missing: %+v", row)
			}
		case 12:
			if row.States.Values != nil || row.RunningWindowPct != nil || !near(row.States.UnknownMs, 20, 1e-6) {
				t.Fatalf("unknown became zero: %+v", row)
			}
		default:
			t.Fatalf("foreign/right-boundary thread: %+v", row)
		}
	}
	limited := profileResult(t, processProfileTrace, 1).ProcessProfile
	if limited.ThreadCount != 3 || limited.EmittedThreads != 1 || limited.OmittedThreads != 2 || limited.UnavailableThreads != 1 {
		t.Fatalf("limit changed population: %+v", limited)
	}
	if r.RootCauseRank != nil || r.WakeupChain != nil {
		t.Fatal("overview invented causal authority")
	}
	if len(r.EvidencePack) < 1 || !ValidProcessProfile(*p) {
		t.Fatal("profile lacks handoff facts")
	}
	for _, row := range p.Threads {
		if row.Thread.PID == 12 {
			continue
		}
		want := 8.0
		if row.Thread.PID == 11 {
			want = 7
		}
		if len(row.BusinessHotspots) != 1 || row.BusinessGroupCount != 1 || row.BusinessHotspots[0].Name != "Load" || !near(row.BusinessHotspots[0].InclusiveMs, want, 1e-6) {
			t.Fatalf("cross-thread business identity or clipped cost: %+v", row)
		}
	}
}

func TestProcessProfilePublicNativeIdentityAndBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"no native TGID", strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(processProfileTrace, " (10)", ""), " (90)", ""), "B|10|", "B|10|")},
		{"conflicting native TGID", strings.Replace(processProfileTrace, "ui-10 (10) [000] .... 1.002", "ui-10 (77) [000] .... 1.002", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := profileResult(t, tc.text, 10).ProcessProfile
			if p.Status != "unavailable" || len(p.Threads) != 0 {
				t.Fatalf("fabricated membership: %+v", p)
			}
		})
	}
	idx := buildTraceIndex(t, "profile.systrace", processProfileTrace)
	before, _ := json.Marshal(idx.Events)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Run(idx, Query{View: "process_profile", PID: 10, TimeStart: 1, TimeEnd: 1.02}.WithRunContext(ctx))
	if r.ProcessProfile != nil || r.ViewCancellation == nil {
		t.Fatalf("canceled profile published: %+v", r)
	}
	after, _ := json.Marshal(idx.Events)
	if string(before) != string(after) {
		t.Fatal("mutated index")
	}
}
