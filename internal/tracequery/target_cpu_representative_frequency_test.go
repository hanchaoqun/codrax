package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The target is below both global Top8 and its CPU's Top2. The existing
// scheduler account must remain complete without treating either display
// ranking as the population of target frequency observations.
func b1633TargetBelowBothCapsTrace() string {
	var b strings.Builder
	b.WriteString("<idle>-0 (-----) [000] .... 0.900000: cpu_frequency: state=1280000 cpu_id=0\n")
	for cpu := 1; cpu <= 8; cpu++ {
		fmt.Fprintf(&b, "<idle>-0 (-----) [%03d] .... 0.900000: cpu_frequency: state=2000000 cpu_id=%d\n", cpu, cpu)
	}
	b.WriteString("<idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=rival_a next_pid=200 next_prio=120\n")
	for cpu := 1; cpu <= 8; cpu++ {
		fmt.Fprintf(&b, "<idle>-0 (-----) [%03d] .... 1.000000: sched_switch: prev_comm=swapper/%d prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other%d next_pid=%d next_prio=120\n", cpu, cpu, cpu, 300+cpu)
	}
	b.WriteString("rival_a-200 (200) [000] .... 1.020000: sched_switch: prev_comm=rival_a prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=rival_b next_pid=201 next_prio=120\n")
	b.WriteString("rival_b-201 (201) [000] .... 1.040000: sched_switch: prev_comm=rival_b prev_pid=201 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120\n")
	b.WriteString("<idle>-0 (-----) [000] .... 1.080000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=101 next_prio=120\n")
	b.WriteString("target-101 (101) [000] .... 1.085000: sched_switch: prev_comm=target prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120\n")
	for cpu := 1; cpu <= 8; cpu++ {
		fmt.Fprintf(&b, "other%d-%d (%d) [%03d] .... 1.100000: sched_switch: prev_comm=other%d prev_pid=%d prev_prio=120 prev_state=S ==> next_comm=swapper/%d next_pid=0 next_prio=120\n", cpu, 300+cpu, 300+cpu, cpu, cpu, 300+cpu, cpu)
	}
	return b.String()
}

const b1633ChangingFrequencyTrace = `
<idle>-0 (-----) [003] .... 0.990000: cpu_frequency: state=1000000 cpu_id=3
<idle>-0 (-----) [003] .... 1.000000: sched_switch: prev_comm=swapper/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=101 next_prio=120
target-101 (101) [003] .... 1.003000: cpu_frequency: state=1800000 cpu_id=3
target-101 (101) [003] .... 1.005000: sched_switch: prev_comm=target prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/3 next_pid=0 next_prio=120
<idle>-0 (-----) [003] .... 1.007000: cpu_frequency: state=900000 cpu_id=3
<idle>-0 (-----) [003] .... 1.010000: sched_switch: prev_comm=swapper/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=renamed next_pid=101 next_prio=120
renamed-101 (101) [003] .... 1.015000: sched_switch: prev_comm=renamed prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/3 next_pid=0 next_prio=120
<idle>-0 (-----) [003] .... 1.017000: cpu_frequency: state=2000000 cpu_id=3
`

func b1633FrequencyRow(t *testing.T, account *TargetWindowStateAccount, cpu int) TargetWindowCPURunning {
	t.Helper()
	if account == nil {
		t.Fatal("target state account missing")
	}
	for _, row := range account.RunningByCPU {
		if row.CPU == cpu {
			return row
		}
	}
	t.Fatalf("CPU%d missing from target roster: %+v", cpu, account)
	return TargetWindowCPURunning{}
}

func TestB1633ActualRunRepresentativeKeepsBucketCaliberAndBundleParity(t *testing.T) {
	idx := buildTraceIndex(t, "changing.ftrace", b1633ChangingFrequencyTrace)
	q := Query{View: "window_stats", PID: 101, TimeStart: 1, TimeEnd: 1.017}
	var plain *TargetWindowStateAccount
	for _, view := range []string{"window_stats", "frame_root_cause_bundle"} {
		t.Run(view, func(t *testing.T) {
			q.View = view
			sweeps := 0
			q.statsSweepProbe = &sweeps
			result := Run(idx, q)
			account := result.TargetWindowStates
			if view == "frame_root_cause_bundle" {
				if result.FrameRootCauseBundle == nil {
					t.Fatalf("bundle missing: %+v", result)
				}
				account = result.FrameRootCauseBundle.TargetWindowStates
			}
			row := b1633FrequencyRow(t, account, 3)
			approxEq(t, "unchanged target running", row.RunningMs, 10)
			if row.SegmentCount != 2 || sweeps != 1 {
				t.Fatalf("display join changed segment or sweep counts: row=%+v sweeps=%d", row, sweeps)
			}
			freq := row.RepresentativeFrequency
			if freq == nil || freq.FrequencyKHz != 900000 || freq.Caliber != TargetWindowCPURepresentativeFrequencyCaliber || freq.ClusterDonorCPU != nil || freq.ClusterDonorSource != "" {
				t.Fatalf("must preserve last positive segment-start sample, not late CPU 2GHz/max/weighted/constant: %+v", freq)
			}
			if view == "window_stats" {
				plain = account
				for _, td := range result.WindowStats.TopRunning {
					if td.Thread.PID == 101 && (td.Frequency != freq.FrequencyKHz || td.CPU != 3) {
						t.Fatalf("new display changed the original bucket representative: %+v", td)
					}
				}
			} else if !reflect.DeepEqual(plain.RunningByCPU, account.RunningByCPU) {
				t.Fatalf("two actual publication sites disagree: plain=%+v bundle=%+v", plain.RunningByCPU, account.RunningByCPU)
			}
		})
	}
	// The existing scalar keeps its previous positive representative when a
	// later running start has no positive governing frequency. Do not silently
	// reinterpret that value as the frequency of the last segment itself.
	zero := buildTraceIndex(t, "later_zero.ftrace", strings.Replace(b1633ChangingFrequencyTrace, "state=900000", "state=0", 1))
	res := Run(zero, Query{View: "window_stats", PID: 101, TimeStart: 1, TimeEnd: 1.017})
	if got := b1633FrequencyRow(t, res.TargetWindowStates, 3).RepresentativeFrequency; got == nil || got.FrequencyKHz != 1000000 {
		t.Fatalf("legacy last-positive caliber changed: %+v", got)
	}
}

func TestB1633ActualRunFrequencyOwnDonorMissingAndTaint(t *testing.T) {
	for _, tc := range []struct {
		name, body, topology string
		want                 int64
		donor                int
	}{
		{"same_cluster", clusterReuseB3Fixture, "middle=2-3;big=7", 1000000, 2},
		{"derived_cluster", strings.ReplaceAll(strings.ReplaceAll(clusterReuseB3Fixture, "[002]", "[004]"), "cpu_id=2", "cpu_id=4"), "", 1000000, 4},
		{"unknown_derived_gap", clusterReuseB3Fixture, "", 0, -1},
		{"other_valid_cluster", clusterReuseB3Fixture, "middle=2;big=3-7", 2000000, 7},
		{"isolated_cluster", clusterReuseB3Fixture, "middle=2;big=3;prime=7", 0, -1},
		{"own_sample_wins", "<idle>-0 (-----) [003] .... 4.900000: cpu_frequency: state=1200000 cpu_id=3\n" + clusterReuseB3Fixture, "middle=2-3;big=7", 1200000, -1},
		{"no_sample", strings.ReplaceAll(strings.ReplaceAll(clusterReuseB3Fixture, "cpu_frequency:", "ignored_frequency:"), "state=1000000", "state=0"), "middle=2-3;big=7", 0, -1},
		{"own_malformed_no_donor", "<idle>-0 (-----) [003] .... 4.999000: cpu_frequency: state=broken cpu_id=3\n" + clusterReuseB3Fixture, "middle=2-3;big=7", 0, -1},
		{"malformed_donor_not_used", strings.Replace(clusterReuseB3Fixture, "state=1000000", "state=broken", 1), "middle=2-3;big=7", 0, -1},
		{"own_rollback_no_donor", "<idle>-0 (-----) [003] .... 4.999000: cpu_frequency: state=1200000 cpu_id=3\n<idle>-0 (-----) [003] .... 4.998000: cpu_frequency: state=1300000 cpu_id=3\n" + clusterReuseB3Fixture, "middle=2-3;big=7", 0, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, tc.name+".ftrace", tc.body)
			res := Run(idx, Query{View: "window_stats", PID: 200, TimeStart: 5, TimeEnd: 5.01, CoreTopology: tc.topology})
			row := b1633FrequencyRow(t, res.TargetWindowStates, 3)
			approxEq(t, "all cases keep observed running", row.RunningMs, 10)
			freq := row.RepresentativeFrequency
			if tc.want == 0 {
				if freq != nil {
					t.Fatalf("unavailable/poisoned frequency was replaced by an assumed value: %+v", freq)
				}
				return
			}
			if freq == nil || freq.FrequencyKHz != tc.want || freq.Caliber != TargetWindowCPURepresentativeFrequencyCaliber {
				t.Fatalf("wrong representative: got=%+v want=%d", freq, tc.want)
			}
			if tc.donor >= 0 {
				source := ClusterFreqSourceExplicit
				if tc.topology == "" {
					source = ClusterFreqSourceDerived
				}
				if freq.ClusterDonorCPU == nil || *freq.ClusterDonorCPU != tc.donor || freq.ClusterDonorSource != source {
					t.Fatalf("same-resolver donor provenance lost: %+v", freq)
				}
			} else if freq.ClusterDonorCPU != nil || freq.ClusterDonorSource != "" {
				t.Fatalf("own sample incorrectly marked borrowed: %+v", freq)
			}
		})
	}
}

func TestB1633ExplicitZeroWindowAndUnknownStatsKeepTheirScope(t *testing.T) {
	const trace = `<idle>-0 (-----) [000] .... 0.000000: cpu_frequency: state=1000000 cpu_id=0
<idle>-0 (-----) [000] .... 0.001000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=101 next_prio=120
target-101 (101) [000] .... 0.010000: sched_switch: prev_comm=target prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`
	idx := buildTraceIndex(t, "zero.ftrace", trace)
	q := Query{View: "window_stats", PID: 101, TimeStart: 0, TimeStartSet: true, TimeEnd: .01, TimeEndSet: true}
	res := Run(idx, q)
	row := b1633FrequencyRow(t, res.TargetWindowStates, 0)
	if !res.TargetWindowStates.Window.StartSet || row.RepresentativeFrequency == nil || row.RepresentativeFrequency.FrequencyKHz != 1000000 {
		t.Fatalf("real zero start is not an unknown query: %+v", res.TargetWindowStates)
	}
	q.View = "thread_timeline"
	res = Run(idx, q)
	if res.WindowStats != nil {
		t.Fatal("timeline view must not start a stats sweep just for frequency")
	}
	row = b1633FrequencyRow(t, res.TargetWindowStates, 0)
	if row.RepresentativeFrequency != nil {
		t.Fatal("absent stats/private census is unknown, not a TopN or CPU fallback")
	}
	// Relation-only indexes retain their existing unavailable stats contract;
	// even a separately obtained measurable target timeline cannot bootstrap a
	// complete pre-cap frequency census from that partial event population.
	q.View = "window_stats"
	tl, ok := targetWindowTimeline(idx, q, ThreadRef{PID: 101}, queryResultTimeWindow(q))
	idx.RelationScoped = true
	stats := ComputeWindowStats(idx, q)
	account := buildTargetWindowStateAccount(idx, tl, ok, tl.Thread, queryResultTimeWindow(q), &stats)
	stampTargetWindowCPURepresentativeFrequencies(account, idx, q, &stats)
	if stats.targetCPUFrequencyCensus != nil || b1633FrequencyRow(t, account, 0).RepresentativeFrequency != nil {
		t.Fatal("relation-scoped index gained complete frequency provenance")
	}
}

func TestB1633ActualRunTargetIdentityIsNotCommOrIncarnationFallback(t *testing.T) {
	const sameName = `<idle>-0 (-----) [000] .... 0.990000: cpu_frequency: state=1000000 cpu_id=0
<idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=102 next_prio=120
<idle>-0 (-----) [001] .... 1.000000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=101 next_prio=120
target-102 (102) [000] .... 1.010000: sched_switch: prev_comm=target prev_pid=102 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
target-101 (101) [001] .... 1.010000: sched_switch: prev_comm=target prev_pid=101 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
`
	idx := buildTraceIndex(t, "same_name.ftrace", sameName)
	q := Query{View: "window_stats", PID: 101, TimeStart: 1, TimeEnd: 1.01, CoreTopology: "small=0;big=1"}
	res := Run(idx, q)
	if row := b1633FrequencyRow(t, res.TargetWindowStates, 1); row.RepresentativeFrequency != nil {
		t.Fatalf("target lacking samples borrowed from same-name other TID/CPU: %+v", row)
	}
	q.PID = 102
	res = Run(idx, q)
	if row := b1633FrequencyRow(t, res.TargetWindowStates, 0); row.RepresentativeFrequency == nil || row.RepresentativeFrequency.FrequencyKHz != 1000000 {
		t.Fatalf("healthy other TID's own sample was lost: %+v", row)
	}
	q.PID, q.Thread = 0, "target"
	if res = Run(idx, q); res.TargetWindowStates != nil {
		t.Fatalf("ambiguous name must not select one thread's frequency: %+v", res.TargetWindowStates)
	}
	const incarnations = `<idle>-0 (-----) [000] .... 0.990000: cpu_frequency: state=1000000 cpu_id=0
<idle>-0 (-----) [001] .... 0.990000: cpu_frequency: state=2000000 cpu_id=1
<idle>-0 (-----) [000] .... 1.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=120
old-42 (100) [000] .... 1.006000: sched_switch: prev_comm=old prev_pid=42 prev_prio=120 prev_state=X ==> next_comm=swapper/0 next_pid=0 next_prio=120
maker-99 (99) [001] .... 1.007000: sched_wakeup_new: comm=new pid=42 prio=120 target_cpu=001
<idle>-0 (-----) [001] .... 1.010000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=new next_pid=42 next_prio=120
new-42 (200) [001] .... 1.018000: sched_switch: prev_comm=new prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
`
	idx = buildTraceIndex(t, "incarnations.ftrace", incarnations)
	q = Query{View: "window_stats", PID: 42, TimeStart: 1, TimeEnd: 1.02}
	res = Run(idx, q)
	if res.TargetWindowStates != nil {
		t.Fatalf("conflicting target lifecycle must not gain a combined frequency account: %+v", res.TargetWindowStates)
	}
	if res.WindowStats == nil || !containsSubstring(res.WindowStats.Caveats, "thread_incarnation_conflict") {
		t.Fatalf("fixture must reach original lifecycle rejection: %+v", res.WindowStats)
	}
	q.TimeStart = 1.007
	res = Run(idx, q)
	row := b1633FrequencyRow(t, res.TargetWindowStates, 1)
	approxEq(t, "new incarnation running only", row.RunningMs, 8)
	if len(res.TargetWindowStates.RunningByCPU) != 1 || row.RepresentativeFrequency == nil || row.RepresentativeFrequency.FrequencyKHz != 2000000 {
		t.Fatalf("new lifecycle must keep its CPU sample without old CPU0 bucket: %+v", res.TargetWindowStates)
	}
}

func TestB1633RepresentativeFrequencyOwnsDonorAndLegacyBytes(t *testing.T) {
	idx := buildTraceIndex(t, "donor.ftrace", clusterReuseB3Fixture)
	q := Query{View: "window_stats", PID: 200, TimeStart: 5, TimeEnd: 5.01, CoreTopology: "middle=2-3;big=7"}
	stats := ComputeWindowStats(idx, q)
	window := queryResultTimeWindow(q)
	tl, ok := targetWindowTimeline(idx, q, ThreadRef{PID: 200}, window)
	newAccount := func() *TargetWindowStateAccount {
		a := buildTargetWindowStateAccount(idx, tl, ok, tl.Thread, window, &stats)
		stampTargetWindowCPURepresentativeFrequencies(a, idx, q, &stats)
		return a
	}
	a := newAccount()
	freq := b1633FrequencyRow(t, a, 3).RepresentativeFrequency
	if freq == nil || freq.ClusterDonorCPU == nil || *freq.ClusterDonorCPU != 2 {
		t.Fatal("fixture must publish same-cluster donor2")
	}
	payload, _ := json.Marshal(a)
	var roundtrip TargetWindowStateAccount
	if err := json.Unmarshal(payload, &roundtrip); err != nil {
		t.Fatal(err)
	}
	roundtripPayload, _ := json.Marshal(roundtrip)
	if string(payload) != string(roundtripPayload) {
		t.Fatal("public optional frequency source must survive JSON unchanged")
	}
	*freq.ClusterDonorCPU = 77
	freq.FrequencyKHz = 77
	b := newAccount()
	got := b1633FrequencyRow(t, b, 3).RepresentativeFrequency
	if got == nil || got.FrequencyKHz != 1000000 || got.ClusterDonorCPU == nil || *got.ClusterDonorCPU != 2 {
		t.Fatalf("published optional pointers aliased private census or another account: %+v", got)
	}
	legacy := TargetWindowCPURunning{CPU: 0, RunningMs: 3}
	legacyPayload, _ := json.Marshal(legacy)
	if string(legacyPayload) != `{"cpu":0,"running_ms":3}` {
		t.Fatalf("absent frequency changed legacy wire bytes: %s", legacyPayload)
	}
}

func TestB1633FrequencyCensusRequiresExactQueryCaptureAndTarget(t *testing.T) {
	idx := buildTraceIndex(t, "same.ftrace", b1633ChangingFrequencyTrace)
	otherCapture := buildTraceIndex(t, "same.ftrace", b1633ChangingFrequencyTrace)
	q := Query{View: "window_stats", PID: 101, TimeStart: 1, TimeEnd: 1.017}
	stats := ComputeWindowStats(idx, q)
	window := queryResultTimeWindow(q)
	tl, ok := targetWindowTimeline(idx, q, ThreadRef{PID: 101}, window)
	if !ok || len(tl.Intervals) == 0 {
		t.Fatal("exact source timeline missing")
	}
	newAccount := func() *TargetWindowStateAccount {
		return buildTargetWindowStateAccount(idx, tl, true, tl.Thread, window, &stats)
	}
	for _, tc := range []struct {
		name string
		edit func(*Query, **Index, **WindowStats, *TargetWindowStateAccount)
	}{
		{"different_capture_same_basename", func(_ *Query, source **Index, _ **WindowStats, _ *TargetWindowStateAccount) { *source = otherCapture }},
		{"different_line_query", func(q *Query, _ **Index, _ **WindowStats, _ *TargetWindowStateAccount) { q.LineStart = 2 }},
		{"different_event_filter", func(q *Query, _ **Index, _ **WindowStats, _ *TargetWindowStateAccount) {
			q.EventTypes = []EventType{EventCPUFrequency}
		}},
		{"different_pattern", func(q *Query, _ **Index, _ **WindowStats, _ *TargetWindowStateAccount) { q.Pattern = "another" }},
		{"different_topology", func(q *Query, _ **Index, _ **WindowStats, _ *TargetWindowStateAccount) { q.CoreTopology = "big=0-7" }},
		{"different_view", func(q *Query, _ **Index, _ **WindowStats, _ *TargetWindowStateAccount) { q.View = "thread_timeline" }},
		{"different_query_target", func(q *Query, _ **Index, _ **WindowStats, _ *TargetWindowStateAccount) { q.PID = 102 }},
		{"different_account_target_same_name", func(_ *Query, _ **Index, _ **WindowStats, a *TargetWindowStateAccount) { a.Thread.PID = 102 }},
		{"missing_target_tid", func(_ *Query, _ **Index, _ **WindowStats, a *TargetWindowStateAccount) { a.Thread.PID = 0 }},
		{"invalid_cpu", func(_ *Query, _ **Index, _ **WindowStats, a *TargetWindowStateAccount) { a.RunningByCPU[0].CPU = -1 }},
		{"different_account_window", func(_ *Query, _ **Index, _ **WindowStats, a *TargetWindowStateAccount) { a.Window.EndTs += 0.001 }},
		{"nil_stats", func(_ *Query, _ **Index, s **WindowStats, _ *TargetWindowStateAccount) { *s = nil }},
		{"legacy_topn_only", func(_ *Query, _ **Index, s **WindowStats, _ *TargetWindowStateAccount) {
			*s = &WindowStats{Window: stats.Window, TopRunning: append([]ThreadDuration(nil), stats.TopRunning...)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query, source, input, account := q, idx, &stats, newAccount()
			tc.edit(&query, &source, &input, account)
			stampTargetWindowCPURepresentativeFrequencies(account, source, query, input)
			for _, row := range account.RunningByCPU {
				if row.RepresentativeFrequency != nil {
					t.Fatalf("mismatched/unknown source gained frequency: %+v", row)
				}
			}
		})
	}
	account := newAccount()
	account.Thread.Comm = "same_tid_new_display_name"
	before, _ := json.Marshal(stats)
	stampTargetWindowCPURepresentativeFrequencies(account, idx, q, &stats)
	if b1633FrequencyRow(t, account, 3).RepresentativeFrequency == nil {
		t.Fatal("exact TID repair must not depend on display comm equality")
	}
	first, _ := json.Marshal(account)
	stampTargetWindowCPURepresentativeFrequencies(account, idx, q, &stats)
	second, _ := json.Marshal(account)
	after, _ := json.Marshal(stats)
	if string(first) != string(second) || string(before) != string(after) {
		t.Fatal("display join must be idempotent and leave all original stats unchanged")
	}
	var oldStats WindowStats
	if err := json.Unmarshal(before, &oldStats); err != nil {
		t.Fatal(err)
	}
	legacy := newAccount()
	stampTargetWindowCPURepresentativeFrequencies(legacy, idx, q, &oldStats)
	if b1633FrequencyRow(t, legacy, 3).RepresentativeFrequency != nil {
		t.Fatal("JSON roundtrip must not fabricate private execution provenance")
	}
	dead, stop := context.WithCancel(context.Background())
	stop()
	res := Run(idx, q.WithRunContext(dead))
	if res.ViewCancellation == nil || res.TargetWindowStates != nil {
		t.Fatalf("pre-canceled run published a partial account: %+v", res)
	}
}

func TestB1633ExplicitTargetCannotBorrowAnotherPresentCensusMember(t *testing.T) {
	idx := buildTraceIndex(t, "other_member.ftrace", b1633TargetBelowBothCapsTrace())
	q := Query{View: "window_stats", PID: 101, TimeStart: 1, TimeEnd: 1.1}
	stats := ComputeWindowStats(idx, q)
	window := queryResultTimeWindow(q)
	other, ok := targetWindowTimeline(idx, q, ThreadRef{PID: 200}, window)
	account := buildTargetWindowStateAccount(idx, other, ok, other.Thread, window, &stats)
	if account == nil || account.Thread.PID != 200 || len(account.RunningByCPU) != 1 {
		t.Fatal("negative premise needs another real TID represented in the same full census")
	}
	stampTargetWindowCPURepresentativeFrequencies(account, idx, q, &stats)
	if account.RunningByCPU[0].RepresentativeFrequency != nil {
		t.Fatalf("exact query PID101 cannot authorize a different present TID200 account: %+v", account.RunningByCPU[0])
	}
}

func TestB1633H4ActualTargetCPU7Representative(t *testing.T) {
	if _, err := os.Stat(donghuWitnessTracePath); err != nil {
		t.Skipf("optional production witness unavailable: %v", err)
	}
	idx, err := BuildIndex(context.Background(), donghuWitnessTracePath)
	if err != nil {
		t.Fatal(err)
	}
	res := Run(idx, Query{View: "window_stats", PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898})
	row := b1633FrequencyRow(t, res.TargetWindowStates, 7)
	approxEq(t, "original H4 target CPU7 running", row.RunningMs, 11.030)
	approxEq(t, "original H4 all CPU running", res.TargetWindowStates.RunningMs, 157.248)
	if row.RepresentativeFrequency == nil || row.RepresentativeFrequency.FrequencyKHz != 1280000 {
		t.Fatalf("existing CPU7 target bucket was omitted from full roster: %+v", row)
	}
	for _, td := range res.WindowStats.TopRunning {
		if td.Thread.PID == 17267 && td.CPU == 7 {
			t.Fatalf("target CPU7 unexpectedly entered unchanged global Top8: %+v", td)
		}
	}
}

func TestB1633ActualRunTargetFrequencySurvivesBothDisplayCaps(t *testing.T) {
	idx := buildTraceIndex(t, "target_frequency.ftrace", b1633TargetBelowBothCapsTrace())
	sweeps := 0
	result := Run(idx, Query{View: "window_stats", PID: 101, TimeStart: 1, TimeEnd: 1.1, statsSweepProbe: &sweeps})
	if result.WindowStats == nil || result.TargetWindowStates == nil {
		t.Fatalf("real query must expose its existing scheduler accounts: %+v", result)
	}
	if sweeps != 1 || len(result.WindowStats.TopRunning) != 8 {
		t.Fatalf("original single sweep/Top8 contract changed: sweeps=%d rows=%+v", sweeps, result.WindowStats.TopRunning)
	}
	for _, td := range result.WindowStats.TopRunning {
		if td.Thread.PID == 101 {
			t.Fatalf("invalid test premise: target entered global Top8: %+v", td)
		}
		approxEq(t, "unchanged other-thread running", td.DurationMs, 100)
	}
	occ := result.WindowStats.CPUOccupancy
	if occ == nil {
		t.Fatal("existing per-CPU occupancy missing")
	}
	foundCPU0 := false
	for _, cpu := range occ.PerCPUTop {
		if cpu.CPU != 0 {
			continue
		}
		foundCPU0 = true
		if len(cpu.Top) != 2 {
			t.Fatalf("original per-CPU Top2 changed: %+v", cpu)
		}
		for _, td := range cpu.Top {
			if td.Thread.PID == 101 {
				t.Fatalf("invalid test premise: target entered CPU Top2: %+v", td)
			}
			approxEq(t, "unchanged rival running", td.DurationMs, 20)
		}
	}
	if !foundCPU0 {
		t.Fatal("fixture CPU0 must have two other top occupants")
	}
	account := result.TargetWindowStates
	approxEq(t, "target running", account.RunningMs, 5)
	if account.Thread.PID != 101 || len(account.RunningByCPU) != 1 || account.RunningByCPU[0].CPU != 0 || account.RunningByCPU[0].SegmentCount != 1 {
		t.Fatalf("existing exact target CPU roster changed: %+v", account)
	}
	payload, err := json.Marshal(account.RunningByCPU[0])
	if err != nil {
		t.Fatal(err)
	}
	var row struct {
		RepresentativeFrequency *struct {
			FrequencyKHz int64  `json:"frequency_khz"`
			Caliber      string `json:"caliber"`
		} `json:"representative_frequency"`
	}
	if err := json.Unmarshal(payload, &row); err != nil {
		t.Fatal(err)
	}
	if row.RepresentativeFrequency == nil || row.RepresentativeFrequency.FrequencyKHz != 1280000 || row.RepresentativeFrequency.Caliber != "last_positive_running_segment_start" {
		t.Fatalf("complete target bucket must disclose its known representative frequency despite both caps, not borrow other threads: %s", payload)
	}
}
