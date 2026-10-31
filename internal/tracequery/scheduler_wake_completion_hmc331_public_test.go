package tracequery

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func hmc331CompletionSource(t *testing.T, state, variation string) string {
	t.Helper()
	wake := func(kind string, ts float64, pid, cpu int) string {
		return fmt.Sprintf("waker-%d (%d) [%03d] .... %.6f: %s: comm=target pid=41 prio=120 target_cpu=%03d", pid, pid, cpu, ts, kind, cpu)
	}
	marker := func(ts float64) string {
		return fmt.Sprintf("waker-2 (2) [001] .... %.6f: sched_blocked_reason: pid=41 iowait=1 caller=io_schedule+0x1", ts)
	}
	rows := []string{
		"idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120 next_info=1,0,0,0,0,0,0",
		fmt.Sprintf("target-41 (41) [000] .... 1.010000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120", state),
		wake("sched_waking", 1.012, 2, 0),
	}
	switch variation {
	case "repeated_waking":
		rows = append(rows, wake("sched_waking", 1.012005, 2, 0))
	case "cross_switch":
		rows = append(rows, "idle-0 (0) [000] .... 1.012010: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120")
	case "new_incarnation":
		rows = append(rows, wake("sched_wakeup_new", 1.012010, 2, 0))
	case "waker_new_incarnation":
		rows = append(rows, "other-3 (3) [001] .... 1.012010: sched_wakeup_new: comm=waker pid=2 prio=120 target_cpu=000")
	case "gap_only":
		rows = append(rows, marker(1.012010))
	}
	if variation != "missing_completion" {
		pid := 2
		if variation == "different_waker" {
			pid = 3
		}
		rows = append(rows, wake("sched_wakeup", 1.012020, pid, 1))
	}
	if variation == "multiple_completions" {
		rows = append(rows, wake("sched_wakeup", 1.012021, 2, 1))
	}
	if variation != "gap_only" {
		markerTs := 1.012022
		if variation == "same_time_marker" || variation == "same_time_malformed" {
			markerTs = 1.012020
		}
		rows = append(rows, marker(markerTs))
	}
	if variation == "multiple_markers" {
		rows = append(rows, marker(1.012023))
	}
	if variation == "malformed_bound_marker" {
		rows = append(rows, "waker-2 (2) [001] .... 1.012023: sched_blocked_reason: pid=41 pid=bad iowait=1 caller=opaque_peer")
	}
	if variation == "same_time_malformed" {
		rows = append(rows, "waker-2 (2) [001] .... 1.012020: sched_blocked_reason: pid=41 pid=bad iowait=1 caller=opaque_peer")
	}
	if variation == "waker_later_incarnation" {
		rows = append(rows, "other-3 (3) [001] .... 1.012030: sched_wakeup_new: comm=waker pid=2 prio=120 target_cpu=000")
	}
	if variation != "cross_switch" {
		rows = append(rows, "idle-0 (0) [000] .... 1.020000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120 next_info=1,0,0,0,0,0,0")
	}
	rows = append(rows, "target-41 (41) [000] .... 1.040000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120")
	path := filepath.Join(t.TempDir(), "completion-metadata.ftrace")
	if err := os.WriteFile(path, []byte(strings.Join(rows, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHMC331WakeCompletionMetadataDoesNotMoveStateBoundary(t *testing.T) {
	for _, variation := range []string{"unique", "same_time_marker", "waker_later_incarnation"} {
		for _, state := range []string{"S", "D"} {
			t.Run(variation+"/"+state, func(t *testing.T) {
				idx, err := BuildIndex(t.Context(), hmc331CompletionSource(t, state, variation))
				if err != nil {
					t.Fatal(err)
				}
				r := Run(idx, hmc331WakeBoundaryQuery("window_stats", 1, 1.04))
				if r.TargetWindowStates == nil || r.WindowStats == nil {
					t.Fatal("public state account/statistics unavailable")
				}
				a := r.TargetWindowStates
				hmc331WakeBoundaryNear(t, "first transition remains end of sleeping", a.RunnableMs, 8)
				hmc331WakeBoundaryNear(t, "running remains scheduler duration", a.RunningMs, 30)
				hmc331WakeBoundaryNear(t, "total does not add reason overlay", a.TotalMs, 40)
				if state == "S" {
					hmc331WakeBoundaryNear(t, "S retained", a.SleepMs, 2)
					hmc331WakeBoundaryNear(t, "S subset metadata retained", a.SleepIOWaitMs, 2)
					hmc331WakeBoundaryNear(t, "S never becomes D/IO", a.IOWaitMs, 0)
				} else {
					hmc331WakeBoundaryNear(t, "D reason metadata retained", a.IOWaitMs, 2)
					hmc331WakeBoundaryNear(t, "D not double counted", a.DStateMs, 0)
					var total float64
					for _, row := range r.WindowStats.IOWaitTop {
						if row.Thread.PID == 41 {
							total += row.DurationMs
						}
					}
					hmc331WakeBoundaryNear(t, "legacy offCPU I/O agrees", total, 2)
				}
			})
		}
	}
}

func TestHMC331WakeCompletionMetadataRejectsUnboundEvidence(t *testing.T) {
	for _, variation := range []string{"gap_only", "missing_completion", "different_waker", "repeated_waking", "multiple_completions", "multiple_markers", "cross_switch", "new_incarnation", "waker_new_incarnation", "malformed_bound_marker", "same_time_malformed"} {
		for _, state := range []string{"S", "D"} {
			t.Run(variation+"/"+state, func(t *testing.T) {
				idx, err := BuildIndex(t.Context(), hmc331CompletionSource(t, state, variation))
				if err != nil {
					t.Fatal(err)
				}
				r := Run(idx, hmc331WakeBoundaryQuery("thread_timeline", 1, 1.04))
				if r.Timeline == nil || r.Timeline.IntegrityFailure != "" {
					if variation == "new_incarnation" && r.Timeline != nil && r.Timeline.IntegrityFailure == "thread_incarnation_conflict" {
						return // Mixed native generations must still fail closed.
					}
					t.Fatalf("public fixture did not reach timeline: %+v", r.Timeline)
				}
				found := false
				for _, interval := range r.Timeline.Intervals {
					if interval.StartTs != 1.01 {
						continue
					}
					found = interval.EndTs > interval.StartTs && (interval.State == StateSSleep || interval.State == StateDSleep)
					if interval.State == StateIOWait || interval.BlockedReasonIOWaitKnown || interval.BlockedReasonLine > 0 {
						t.Errorf("unbound completion refined original sleep: %+v", interval)
					}
				}
				if !found {
					t.Fatal("the original positive native S/D instance must remain visible")
				}
			})
		}
	}
}

func TestHMC331WakeCompletionCarrierDoesNotEscapeNativeScope(t *testing.T) {
	idx, err := BuildIndex(t.Context(), hmc331CompletionSource(t, "D", "unique"))
	if err != nil {
		t.Fatal(err)
	}
	q := withSchedulerWakeCompletions(idx, hmc331WakeBoundaryQuery("window_stats", 1, 1.04), 0, nil, false)
	if schedulerWakeCompletionFor(idx, q, 41, 1.012, 3) == nil {
		t.Fatal("real parsed first/completed pair did not initialize local proof")
	}
	other, err := BuildIndex(t.Context(), hmc331CompletionSource(t, "D", "unique"))
	if err != nil {
		t.Fatal(err)
	}
	if schedulerWakeCompletionFor(other, q, 41, 1.012, 3) != nil || schedulerWakeCompletionFor(idx, q, 41, 1.012, 4) != nil {
		t.Fatal("query-local proof crossed physical index or first-wake line")
	}
	narrow := q
	narrow.TimeEnd = 1.012001
	if schedulerWakeCompletionFor(idx, narrow, 41, 1.012, 3) != nil {
		t.Fatal("copied query borrowed completion outside its new window")
	}
	lineQ := q
	lineQ.LineEnd = 3
	if schedulerWakeCompletionFor(idx, lineQ, 41, 1.012, 3) != nil {
		t.Fatal("copied query borrowed completion outside its new line domain")
	}
	start := offCPUStart{thread: ThreadRef{PID: 41}, state: StateDSleep, ts: 1.01, line: 2}
	if io, _, _, _ := offCPUDStateVerdict(start, 1.012, blockedReasonsByPID(idx, q)); io {
		t.Fatal("nil-index legacy matcher acquired a completion proof")
	}
}

func TestHMC331WakeCompletionIdleEmitterUsesNativeCPUInstance(t *testing.T) {
	for _, sameCPU := range []bool{false, true} {
		t.Run(fmt.Sprint(sameCPU), func(t *testing.T) {
			path := hmc331CompletionSource(t, "S", "unique")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.ReplaceAll(string(body), "waker-2 (2)", "idle-0 (0)")
			if sameCPU {
				text = strings.ReplaceAll(text, "idle-0 (0) [000] .... 1.012000", "idle-0 (0) [001] .... 1.012000")
			}
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			idx, err := BuildIndex(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			a := Run(idx, hmc331WakeBoundaryQuery("window_stats", 1, 1.04)).TargetWindowStates
			if a == nil {
				t.Fatal("public idle emitter fixture lost native account")
			}
			want := 0.0
			if sameCPU {
				want = 2
			}
			hmc331WakeBoundaryNear(t, "native idle CPU identity", a.SleepIOWaitMs, want)
		})
	}
}

func TestHMC331WakeCompletionUnwindowedPublicBuilderKeepsOpaqueCompetitors(t *testing.T) {
	for _, variation := range []string{"unique", "malformed_bound_marker", "same_time_malformed"} {
		for _, state := range []string{"S", "D"} {
			t.Run(variation+"/"+state, func(t *testing.T) {
				idx, err := BuildIndex(t.Context(), hmc331CompletionSource(t, state, variation))
				if err != nil {
					t.Fatal(err)
				}
				// The public builder intentionally accepts an unbounded query;
				// unlike Run, it does not replace zero bounds with source limits.
				tl := ThreadTimeline(idx, Query{PID: 41, TargetScope: "thread"})
				if tl.IntegrityFailure != "" {
					t.Fatalf("native timeline unavailable: %+v", tl)
				}
				found := false
				for _, interval := range tl.Intervals {
					if interval.StartTs != 1.01 || interval.EndTs <= interval.StartTs {
						continue
					}
					found = true
					hmc331WakeBoundaryNear(t, "unwindowed first transition remains fixed", (interval.EndTs-interval.StartTs)*1000, 2)
					if variation == "unique" {
						if !interval.BlockedReasonIOWaitKnown || interval.BlockedReasonLine == 0 {
							t.Errorf("unique native completion metadata disappeared: %+v", interval)
						}
					} else if interval.BlockedReasonIOWaitKnown || interval.BlockedReasonLine > 0 || interval.State == StateIOWait {
						t.Errorf("opaque same-envelope competitor was silently dropped: %+v", interval)
					}
				}
				if !found {
					t.Fatal("original positive native sleep instance disappeared")
				}
			})
		}
	}
}

func TestHMC331WakeCompletionTargetChainRetainsExactNativeReason(t *testing.T) {
	for _, variation := range []string{"unique", "malformed_bound_marker", "completion_outside_source_query"} {
		t.Run(variation, func(t *testing.T) {
			path := hmc331CompletionSource(t, "D", variation)
			idx, err := BuildIndex(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			q := hmc331WakeBoundaryQuery("wakeup_chain", 1.01, 1.0121)
			if variation == "completion_outside_source_query" {
				q.TimeEnd = 1.012001
			}
			r := Run(idx, q)
			if r.WakeupChain == nil {
				t.Fatal("public chain unavailable")
			}
			found := 0
			for _, root := range r.WakeupChain.RootEvidence {
				if root.Thread.PID != 41 || root.Type != "io_wait" && root.Type != "d_state_or_io_wait" {
					continue
				}
				found++
				hmc331WakeBoundaryNear(t, "target root retains first-transition measure", root.DurationMs, 2)
				if variation != "unique" {
					if root.Type == "io_wait" || root.LineEnd == 5 || strings.Contains(root.Summary, "caller=io_schedule") {
						t.Errorf("unproved source metadata acquired root authority: %+v", root)
					}
					continue
				}
				if root.Type != "io_wait" || root.LineEnd != 5 || !strings.Contains(root.Summary, "caller=io_schedule+0x1") {
					t.Errorf("target branch discarded source-proved native wait: %+v", root)
				}
				// The public root's measurement receipt must describe the newly
				// projected partition, not the preprojection unknown-D partition.
				original := normalizeQuery(idx, q)
				local := original
				local.TimeEnd = 1.012
				cache := newChainQueryCache(idx, nil)
				before := cache.timeline(local, root.Thread)
				copyBefore := cloneTimelineResult(before)
				after := cache.projectOriginalIntervalMetadata(original, local, root.Thread, before)
				if !reflect.DeepEqual(before, copyBefore) || !reflect.DeepEqual(before, cache.timeline(local, root.Thread)) {
					t.Fatal("projection mutated a caller or cached measurement partition")
				}
				if before.MeasurementDomain == nil || after.MeasurementDomain == nil ||
					before.MeasurementDomain.PartitionID == after.MeasurementDomain.PartitionID ||
					!reflect.DeepEqual(after.MeasurementDomain, buildTimelineMeasurementDomain(local, after)) {
					t.Fatalf("projected metadata did not rebuild its exact measurement receipt: before=%+v after=%+v", before.MeasurementDomain, after.MeasurementDomain)
				}
				matched := false
				if root.MeasurementSources != nil {
					for _, domain := range root.MeasurementSources.Domains {
						matched = matched || domain.PartitionID == after.MeasurementDomain.PartitionID
					}
				}
				if !matched {
					t.Fatalf("public root did not bind the projected partition: %+v", root.MeasurementSources)
				}
			}
			if found != 1 {
				t.Fatalf("exact target wait must retain one root identity, got %d: %+v", found, r.WakeupChain.RootEvidence)
			}
		})
	}
}

func TestHMC331WakeCompletionChainRetainsExactNativeReason(t *testing.T) {
	for _, variation := range []string{"unique", "malformed_bound_marker"} {
		t.Run(variation, func(t *testing.T) {
			// A real dependency window includes the waker's complete D cycle.
			// The separate target-chain control also covers a branch ending at
			// first waking, with metadata already proved by its source query.
			rows := []string{
				"idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120",
				"idle-0 (0) [001] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=2 next_prio=120",
				"target-41 (41) [000] .... 1.010000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120",
				"worker-2 (2) [001] .... 1.010000: sched_switch: prev_comm=worker prev_pid=2 prev_prio=120 prev_state=D ==> next_comm=idle next_pid=0 next_prio=120",
				"waker-9 (9) [002] .... 1.018000: sched_waking: comm=worker pid=2 prio=120 target_cpu=001",
				"waker-9 (9) [002] .... 1.018020: sched_wakeup: comm=worker pid=2 prio=120 target_cpu=001",
				"waker-9 (9) [002] .... 1.018022: sched_blocked_reason: pid=2 iowait=1 caller=io_schedule+0x1",
			}
			if variation == "malformed_bound_marker" {
				rows = append(rows, "waker-9 (9) [002] .... 1.018023: sched_blocked_reason: pid=2 pid=bad iowait=1 caller=opaque_peer")
			}
			rows = append(rows,
				"idle-0 (0) [001] .... 1.018100: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=2 next_prio=120",
				"worker-2 (2) [001] .... 1.020000: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000",
				"idle-0 (0) [000] .... 1.020100: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120",
				"observer-3 (3) [002] .... 1.030000: tracing_mark_write: C|3|observed|1",
			)
			path := hmc331WriteTransitionSource(t, strings.Join(rows, "\n")+"\n")
			idx, err := BuildIndex(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			r := Run(idx, hmc331WakeBoundaryQuery("wakeup_chain", 1.01, 1.021))
			if r.WakeupChain == nil {
				t.Fatal("public chain unavailable")
			}
			found := 0
			for _, root := range r.WakeupChain.RootEvidence {
				if root.Thread.PID != 2 || root.Type != "io_wait" && root.Type != "d_state_or_io_wait" {
					continue
				}
				found++
				hmc331WakeBoundaryNear(t, "root keeps first-transition measure", root.DurationMs, 8)
				if variation == "unique" {
					if root.Type != "io_wait" || root.LineEnd != 7 || !strings.Contains(root.Summary, "caller=io_schedule+0x1") {
						t.Errorf("chain lost exact native reason after first-wake measurement: %+v", root)
					}
				} else if root.Type == "io_wait" || root.LineEnd == 7 || strings.Contains(root.Summary, "caller=io_schedule") {
					t.Errorf("ambiguous marker acquired root reason authority: %+v", root)
				}
			}
			if found != 1 {
				t.Fatalf("exact native wait must retain one root identity, got %d: %+v", found, r.WakeupChain.RootEvidence)
			}
		})
	}
}

func TestHMC331WakeCompletionMetadataRetainsWindowOwnership(t *testing.T) {
	for _, state := range []string{"S", "D"} {
		t.Run(state, func(t *testing.T) {
			path := hmc331CompletionSource(t, state, "unique")
			full, err := BuildIndex(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			q := hmc331WakeBoundaryQuery("window_stats", 1.011, 1.018)
			cut, err := BuildIndexWithOptions(t.Context(), path, BuildOptions{AllowWindowedParse: true, TimeStart: q.TimeStart, TimeEnd: q.TimeEnd, TimeStartSet: true, TimeEndSet: true})
			if err != nil || !cut.Windowed || len(cut.Events) >= len(full.Events) {
				t.Fatalf("real prefix-pruned fixture required: err=%v", err)
			}
			for _, idx := range []*Index{full, cut} {
				a := Run(idx, q).TargetWindowStates
				if a == nil {
					t.Fatal("native target account missing")
				}
				hmc331WakeBoundaryNear(t, "narrow first transition ready", a.RunnableMs, 6)
				if state == "S" {
					hmc331WakeBoundaryNear(t, "narrow S subset", a.SleepIOWaitMs, 1)
				} else {
					hmc331WakeBoundaryNear(t, "narrow D/IO", a.IOWaitMs, 1)
				}
			}
			q.TimeEnd = 1.012001 // The actual completion lies outside this query.
			a := Run(full, q).TargetWindowStates
			if a == nil || a.SleepIOWaitMs != 0 || a.IOWaitMs != 0 {
				t.Fatalf("a wider index lent out-of-query completion authority: %+v", a)
			}
		})
	}
}
