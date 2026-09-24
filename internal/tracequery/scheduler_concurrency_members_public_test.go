package tracequery

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func concurrencyWitnessPartition(t *testing.T, g SchedulerConcurrencyGroup) {
	t.Helper()
	if len(g.Members)+g.OmittedMembers+g.MemberWitnessUnavailableCount != g.AcceptedIntervalCount {
		t.Fatalf("non-disjoint witness census: %+v", g)
	}
}

func TestSchedulerConcurrencyPublicMigrationAndPreemptionMembers(t *testing.T) {
	for _, state := range []string{"R", "R+"} {
		t.Run("preempt_"+state, func(t *testing.T) {
			body := concurrencyPublicTrace(concurrencyPublicSwitch(1.001, 0, 0, 10, "S"), concurrencyPublicSwitch(1.003, 0, 10, 11, state), concurrencyPublicSwitch(1.007, 0, 11, 10, "S"), concurrencyPublicSwitch(1.009, 0, 10, 0, "S"))
			g := concurrencyPublicGroup(t, concurrencyPublicRun(t, body, 1, 1.01), "runnable")
			concurrencyPublicValues(t, g, 1, .4, 4, 4)
			concurrencyWitnessPartition(t, g)
			if len(g.Members) != 1 || g.Members[0].Thread.PID != 10 || g.Members[0].ActualStartTs != 1.003 || g.Members[0].Closure != "sched_in" {
				t.Fatalf("preemption without wake lost: %+v", g)
			}
		})
	}
	for _, tc := range []struct {
		name    string
		rows    []string
		cpus    []int
		lengths []float64
	}{
		{"explicit_migration", []string{concurrencyPublicWake(1.001, 10), "mover-400 (400) [000] .... 1.003000000: sched_migrate_task: comm=worker pid=10 prio=120 orig_cpu=0 dest_cpu=1", concurrencyPublicSwitch(1.006, 1, 0, 10, "S")}, []int{0, 1}, []float64{2, 3}},
		{"sched_in_migration", []string{concurrencyPublicWake(1.001, 10), concurrencyPublicSwitch(1.006, 1, 0, 10, "S")}, []int{1}, []float64{5}},
		{"conflicting_wakes", []string{concurrencyPublicWake(1.001, 10), strings.Replace(concurrencyPublicWake(1.002, 10), "target_cpu=000", "target_cpu=001", 1), concurrencyPublicSwitch(1.006, 2, 0, 10, "S")}, []int{-1}, []float64{5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := concurrencyPublicGroup(t, concurrencyPublicRun(t, concurrencyPublicTrace(tc.rows...), 1, 1.01), "runnable")
			concurrencyPublicValues(t, g, 1, .5, 5, 5)
			concurrencyWitnessPartition(t, g)
			if len(g.Members) != len(tc.cpus) {
				t.Fatalf("wrong members: %+v", g)
			}
			for i, cpu := range tc.cpus {
				m := g.Members[i]
				if cpu < 0 {
					if m.CPU != nil {
						t.Fatalf("unknown CPU gained authority: %+v", m)
					}
				} else if m.CPU == nil || *m.CPU != cpu {
					t.Fatalf("incorrect attributed CPU: %+v", m)
				}
				if m.WindowContributionMs == nil || math.Abs(*m.WindowContributionMs-tc.lengths[i]) > 1e-8 {
					t.Fatalf("wrong interval contribution: %+v", m)
				}
			}
		})
	}
}

func TestSchedulerConcurrencyPublicLineUnknownAndNativeZeroMembers(t *testing.T) {
	idx := buildTraceIndex(t, "line.systrace", concurrencyPublicTrace(concurrencyPublicSwitch(0, 0, 0, 10, "S"), concurrencyPublicSwitch(0, 0, 10, 0, "S")))
	q := Query{View: "window_stats", TimeStart: 0, TimeEnd: .01, TimeStartSet: true, TimeEndSet: true}
	g := concurrencyPublicGroup(t, Run(idx, q).WindowStats.SchedulerConcurrency, "running")
	if len(g.Members) != 1 || g.Members[0].WindowContributionMs == nil || *g.Members[0].WindowContributionMs != 0 || g.Distribution == nil || g.Distribution.Depths[0].Threads != 0 {
		t.Fatalf("known zero omitted: %+v", g)
	}
	q.LineStart, q.LineEnd = 1, 2
	g = concurrencyPublicGroup(t, Run(idx, q).WindowStats.SchedulerConcurrency, "running")
	if g.ThreadCount != 1 || g.AcceptedIntervalCount != 1 {
		t.Fatalf("line-selected owner inventory became an uncomputed zero: %+v", g)
	}
	if len(g.Members) != 1 || g.Members[0].WindowContributionMs != nil || g.Members[0].WindowContribution != nil || g.Values != nil || g.Distribution != nil || len(g.Buckets) != 0 || g.BucketsUnavailableReason != "line_bounds_take_precedence" {
		t.Fatalf("line selector inherited conflicting time denominator: %+v", g)
	}
	concurrencyWitnessPartition(t, g)
}

func TestSchedulerConcurrencyPublicWindowedMemberPhysicalOrigins(t *testing.T) {
	for _, state := range []string{"running", "runnable"} {
		t.Run(state, func(t *testing.T) {
			rows := []string{concurrencyPublicSwitch(.2, 0, 0, 10, "S"), concurrencyPublicSwitch(1.04, 0, 10, 0, "S")}
			if state == "runnable" {
				rows = []string{concurrencyPublicWake(.2, 10), concurrencyPublicSwitch(1.04, 0, 0, 10, "S")}
			}
			path := writeSchedulerCarryTrace(t, "physical.systrace", rows...)
			idx := buildSchedulerCarryWindow(t, path, 1, 1.05)
			if !idx.Windowed {
				t.Fatal("fixture not windowed")
			}
			q := Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.05, TimeStartSet: true, TimeEndSet: true}
			g := concurrencyPublicGroup(t, Run(idx, q).WindowStats.SchedulerConcurrency, state)
			if len(g.Members) != 1 || g.Members[0].ActualStartTs != .2 || g.Members[0].StartLocalLine != 2 || g.Members[0].ActualEndTs != 1.04 {
				t.Fatalf("head source witness lost/fabricated: %+v", g)
			}
			concurrencyWitnessPartition(t, g)
		})
	}
	// A checkpoint updated by duplicate wake retains the original state time,
	// but not that time's original row. Preserve statistics; do not forge the row.
	path := writeSchedulerCarryTrace(t, "updated.systrace", concurrencyPublicWake(.2, 10), concurrencyPublicWake(.3, 10), concurrencyPublicSwitch(1.04, 0, 0, 10, "S"))
	idx := buildSchedulerCarryWindow(t, path, 1, 1.05)
	g := concurrencyPublicGroup(t, Run(idx, Query{View: "window_stats", TimeStart: 1, TimeEnd: 1.05, TimeStartSet: true, TimeEndSet: true}).WindowStats.SchedulerConcurrency, "runnable")
	concurrencyPublicValues(t, g, 1, .8, 40, 40)
	if g.MemberWitnessUnavailableCount != 1 || len(g.Members) != 0 {
		t.Fatalf("later wake forged start identity: %+v", g)
	}
	concurrencyWitnessPartition(t, g)
}

func TestSchedulerConcurrencyPublicMembersPhysicalIdentityUnderRebase(t *testing.T) {
	idx := buildTraceIndex(t, "identity.systrace", concurrencyPublicTrace(concurrencyPublicSwitch(1.001, 0, 0, 10, "S"), concurrencyPublicSwitch(1.004, 0, 10, 0, "S")))
	q := Query{View: "window_stats", TimeStart: 0, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true}
	before := concurrencyPublicGroup(t, Run(idx, q).WindowStats.SchedulerConcurrency, "running")
	// Same physically parsed source, relocated within a virtual composite. It
	// is not a second source parse and does not change normal bundle admission.
	copyIdx := *idx
	copyIdx.Events = append([]Event(nil), idx.Events...)
	copyIdx.TraceArtifacts = append([]TraceArtifactSource(nil), idx.TraceArtifacts...)
	for i := range copyIdx.Events {
		copyIdx.Events[i].Line += 100
	}
	for i := range copyIdx.TraceArtifacts {
		copyIdx.TraceArtifacts[i].VirtualLineBase += 100
	}
	copyIdx.LineCount += 100
	after := concurrencyPublicGroup(t, Run(&copyIdx, q).WindowStats.SchedulerConcurrency, "running")
	if len(before.Members) != 1 || len(after.Members) != 1 {
		t.Fatalf("source rows not retained: before=%+v after=%+v", before, after)
	}
	b, a := before.Members[0], after.Members[0]
	if b.ID != a.ID || b.StartLocalLine != a.StartLocalLine || b.EndLocalLine != a.EndLocalLine || b.StartLine+100 != a.StartLine || b.EndLine+100 != a.EndLine {
		t.Fatalf("identity uses virtual coordinates: before=%+v after=%+v", b, a)
	}
	if !reflect.DeepEqual(before.Values, after.Values) {
		t.Fatal("witness rebasing changed numeric population")
	}
	beforeWire, _ := json.Marshal(idx.Events)
	Run(idx, q)
	afterWire, _ := json.Marshal(idx.Events)
	if string(beforeWire) != string(afterWire) {
		t.Fatal("query mutated original event inventory")
	}
}
