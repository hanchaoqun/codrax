package tracequery

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestB1607AActualRunPublishesSleepInventoryBeforeChainClipping(t *testing.T) {
	idx := buildTraceIndex(t, "sleep-inventory.ftrace", donghuP9TrueBinderWaitTrace)
	q := Query{View: "wakeup_chain", PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375, MinDurationMs: 100, MaxBranches: 1, MaxDepth: 1, Limit: 1}
	result := Run(idx, q)
	if result.TargetWindowStates == nil {
		t.Fatal("fixture must produce the pre-existing target state account")
	}
	payload, err := json.Marshal(result.TargetWindowStates)
	if err != nil {
		t.Fatal(err)
	}
	var account map[string]json.RawMessage
	if err := json.Unmarshal(payload, &account); err != nil {
		t.Fatal(err)
	}
	if raw := account["sleep_inventory"]; len(raw) == 0 || string(raw) == "null" {
		t.Fatal("actual Run omitted the full target sleep inventory; the narrow D/IO roster cannot stand in for ordinary S")
	}
	inventory := result.TargetWindowStates.SleepInventory
	if inventory.Total != 1 || inventory.Emitted != 1 || math.Abs(inventory.TotalMs-1.409) > 1e-6 ||
		result.TargetWindowStates.WaitOccurrenceTotal != 0 {
		t.Fatalf("ordinary S must be counted without widening the D/IO roster: %+v", result.TargetWindowStates)
	}
	b1607AssertUnassessed(t, inventory)
	// The inventory is not an input to the old chain/root/critical assembly.
	chain := BuildWakeupChain(idx, q)
	// Run has always stamped these publication identities after construction;
	// compare the independently constructed chain at the same output phase.
	stampStateAccountPublicationKeys(&chain, nil)
	if result.WakeupChain == nil || !reflect.DeepEqual(*result.WakeupChain, chain) {
		t.Fatal("state-account publication changed the independently built causal chain")
	}
}

func b1607AssertUnassessed(t *testing.T, inventory *TargetWindowSleepInventory) {
	t.Helper()
	if inventory == nil || inventory.Scope != "constructed_target_timeline" || inventory.ScanStatus != "complete" ||
		inventory.StateClosureStatus != "not_assessed" || inventory.BinderAssociationStatus != "not_assessed" ||
		inventory.CausalAttributionStatus != "not_assessed" {
		t.Fatalf("complete input enumeration must not assess closure/Binder/cause: %+v", inventory)
	}
}

func b1607InventoryResultAccount(t *testing.T, result Result) *TargetWindowStateAccount {
	t.Helper()
	account := result.TargetWindowStates
	if result.FrameRootCauseBundle != nil {
		account = result.FrameRootCauseBundle.TargetWindowStates
		if result.TargetWindowStates != nil {
			t.Fatal("frame bundle must not duplicate the target state account")
		}
	}
	if account == nil || account.SleepInventory == nil {
		t.Fatalf("actual result lacks the sleep inventory: %+v", result)
	}
	return account
}

func b1607ManySleepsTrace(count int) string {
	var b strings.Builder
	b.WriteString("idle-0 (0) [000] .... 10.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n")
	for i := 0; i < count; i++ {
		start := 10.001 + float64(i)*0.001
		state := "S"
		if i%3 != 0 {
			state = "D"
		}
		fmt.Fprintf(&b, "target-41 (41) [000] .... %.6f: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=%s ==> next_comm=waker next_pid=2 next_prio=120\n", start, state)
		if i%3 == 2 {
			fmt.Fprintf(&b, "waker-2 (2) [000] .... %.6f: sched_blocked_reason: pid=41 iowait=1 caller=io_schedule\n", start+0.00005)
		}
		fmt.Fprintf(&b, "waker-2 (2) [000] .... %.6f: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000\n", start+0.0002)
		fmt.Fprintf(&b, "waker-2 (2) [000] .... %.6f: sched_switch: prev_comm=waker prev_pid=2 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n", start+0.0003)
	}
	fmt.Fprintf(&b, "target-41 (41) [000] .... %.6f: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n", 10.002+float64(count)*0.001)
	return b.String()
}

func TestB1607AActualRunInventoryIgnoresChainAndDisplayBudgets(t *testing.T) {
	idx := buildTraceIndex(t, "many-sleeps.ftrace", b1607ManySleepsTrace(39))
	var first *TargetWindowSleepInventory
	for _, view := range []string{"wakeup_chain", "root_cause_rank", "frame_root_cause_bundle"} {
		for _, limited := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/limited=%t", view, limited), func(t *testing.T) {
				q := Query{View: view, PID: 41, TimeStart: 10, TimeEnd: 10.041, MinDurationMs: 0.001, MaxBranches: 16, MaxDepth: 4, MaxChainNodes: 64, Limit: 64}
				if limited {
					q.MinDurationMs, q.MaxBranches, q.MaxDepth, q.MaxChainNodes, q.Limit = 100, 1, 1, 1, 1
				}
				account := b1607InventoryResultAccount(t, Run(idx, q))
				inventory := account.SleepInventory
				b1607AssertUnassessed(t, inventory)
				if inventory.Total != 39 || inventory.Emitted != 32 || len(inventory.Occurrences) != 32 || inventory.OutputStatus != "incomplete" ||
					math.Abs(inventory.TotalMs-7.8) > 1e-6 || math.Abs(inventory.SleepMs-2.6) > 1e-6 ||
					math.Abs(inventory.DStateMs-2.6) > 1e-6 || math.Abs(inventory.IOWaitMs-2.6) > 1e-6 {
					t.Fatalf("39 full sub-ms intervals must be measured before the 32-row cap: %+v", inventory)
				}
				if account.WaitOccurrenceTotal != 26 || account.WaitOccurrenceStatus != "complete" {
					t.Fatalf("the independent old D/IO roster changed: %+v", account)
				}
				for i, occurrence := range inventory.Occurrences {
					if occurrence.Ordinal != i+1 || occurrence.DurationMs >= 1 || (i > 0 && occurrence.StartTs <= inventory.Occurrences[i-1].StartTs) {
						t.Fatalf("chronological sub-ms source rows changed: %+v", inventory.Occurrences)
					}
				}
				if first == nil {
					first = inventory
				} else if !reflect.DeepEqual(first, inventory) {
					t.Fatal("changing causal/display budgets or entry point changed the full target inventory")
				}
			})
		}
	}
}

func TestB1607AActualRunPreservesClippedAndActualLedgers(t *testing.T) {
	idx := buildTraceIndex(t, "sleep-clip.ftrace", donghuP9TrueBinderWaitTrace)
	for _, state := range []string{"S", "D"} {
		t.Run(state, func(t *testing.T) {
			if state == "D" {
				idx = buildTraceIndex(t, "d-clip.ftrace", strings.Replace(donghuP9TrueBinderWaitTrace, "prev_state=S", "prev_state=D", 1))
			}
			q := Query{View: "wakeup_chain", PID: 17267, TimeStart: 13762.836, TimeEnd: 13762.837, MinDurationMs: 100}
			result := Run(idx, q)
			inventory := b1607InventoryResultAccount(t, result).SleepInventory
			b1607AssertUnassessed(t, inventory)
			if inventory.Total != 1 || math.Abs(inventory.TotalMs-1) > 1e-6 {
				t.Fatalf("clipped window census omitted or expanded the state interval: %+v", inventory)
			}
			occurrence := inventory.Occurrences[0]
			if !occurrence.WindowClamped() || occurrence.StartTs != q.TimeStart || occurrence.EndTs != q.TimeEnd ||
				occurrence.ActualStartTs != 13762.835861 || occurrence.ActualEndTs != 13762.837270 {
				t.Fatalf("clipped and actual ledgers must remain distinct: %+v", occurrence)
			}
			if result.WakeupChain == nil || len(result.WakeupChain.BinderWaits) != 0 {
				t.Fatal("full state inventory must not turn a window-external request into a Binder donor")
			}
		})
	}
}

func TestB1607AUnknownHeadAndSyntheticTailRemainUnassessed(t *testing.T) {
	for _, state := range []string{"S", "D"} {
		t.Run(state, func(t *testing.T) {
			path := writeSchedulerCarryTrace(t, "unknown-boundary.systrace",
				"idle-0 (0) [000] .... 9.500000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=waker next_pid=2 next_prio=120",
				"waker-2 (2) [000] .... 10.002000: sched_switch: prev_comm=waker prev_pid=2 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120",
				fmt.Sprintf("target-41 (41) [000] .... 10.003000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=%s ==> next_comm=waker next_pid=2 next_prio=120", state),
				"waker-2 (2) [000] .... 10.003050: sched_blocked_reason: pid=41 iowait=1 caller=io_schedule",
				"waker-2 (2) [000] .... 10.010000: sched_switch: prev_comm=waker prev_pid=2 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120",
			)
			idx := buildSchedulerCarryWindow(t, path, 10, 10.009)
			q := Query{View: "window_stats", PID: 41, TimeStart: 10, TimeEnd: 10.009}
			inventory := b1607InventoryResultAccount(t, Run(idx, q)).SleepInventory
			b1607AssertUnassessed(t, inventory)
			if inventory.HeadState == nil || inventory.HeadState.Status != "unknown" || inventory.Total != 1 || math.Abs(inventory.TotalMs-6) > 1e-6 {
				t.Fatalf("enumeration completeness must not fill the unknown prefix or close the open tail: %+v", inventory)
			}
			if state == "D" && inventory.Occurrences[0].EndLine == 0 {
				t.Fatal("D fixture must exercise the legacy reason-line locator that is not a closure")
			}
			t.Logf("state=%s scan=%s output=%s head=%s closure=%s end_line=%d", state, inventory.ScanStatus, inventory.OutputStatus, inventory.HeadState.Status, inventory.StateClosureStatus, inventory.Occurrences[0].EndLine)
		})
	}
}

func TestB1607AInventoryUnionAndSourceValuesAreIndependent(t *testing.T) {
	window := TimeWindow{StartTs: 100, EndTs: 100.1}
	tl := cov4Timeline([]Interval{cov4Interval(StateSSleep, 100.01, 100.04), cov4Interval(StateSSleep, 100.02, 100.05)})
	tl.HeadState = &TimelineHeadState{Status: "unknown", Reason: "test_unknown"}
	before := cloneTimelineResult(tl)
	headBefore := *tl.HeadState
	before.HeadState = &headBefore
	got := buildTargetWindowSleepInventory(tl, window)
	b1607AssertUnassessed(t, got)
	if got.Total != 2 || math.Abs(got.TotalMs-40) > 1e-6 || math.Abs(got.SleepMs-40) > 1e-6 || got.Occurrences[0].DurationMs != tl.Intervals[0].DurationMs {
		t.Fatalf("union must not sum overlapping rows or rewrite source durations: %+v", got)
	}
	if !reflect.DeepEqual(tl, before) || !reflect.DeepEqual(got, buildTargetWindowSleepInventory(tl, window)) {
		t.Fatal("inventory construction mutated source data or is not deterministic")
	}
	got.HeadState.Reason = "mutated result"
	got.Occurrences[0].Summary = "mutated result"
	if !reflect.DeepEqual(tl, before) {
		t.Fatal("inventory aliases the timeline's mutable state")
	}
}

func TestB1607AActualRunPreservesRecoveredHeadAndSleepIOOverlay(t *testing.T) {
	t.Run("recovered head", func(t *testing.T) {
		path := writeSchedulerCarryTrace(t, "sleep-recovered.systrace",
			"target-42 (42) [000] .... 0.100000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=7 next_prio=120",
			"other-7 (7) [000] .... 1.050000: sched_wakeup: comm=target pid=42 prio=120 target_cpu=000",
			"other-7 (7) [000] .... 1.080000: sched_switch: prev_comm=other prev_pid=7 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42 next_prio=120",
			"target-42 (42) [000] .... 1.120000: sched_switch: prev_comm=target prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=other next_pid=7 next_prio=120",
		)
		idx := buildSchedulerCarryWindow(t, path, 1, 1.1)
		q := Query{View: "wakeup_chain", PID: 42, TimeStart: 1, TimeEnd: 1.1, MinDurationMs: 100}
		inventory := b1607InventoryResultAccount(t, Run(idx, q)).SleepInventory
		b1607AssertUnassessed(t, inventory)
		if inventory.HeadState == nil || inventory.HeadState.Status != "recovered" || inventory.HeadState.State != StateSSleep ||
			inventory.Total != 1 || math.Abs(inventory.TotalMs-50) > 1e-6 || !inventory.Occurrences[0].WindowClamped() ||
			inventory.Occurrences[0].ActualStartTs != 0.1 {
			t.Fatalf("exact recovered head state and pre-window source start must survive publication: %+v", inventory)
		}
	})
	t.Run("sleep IO marker stays an overlay", func(t *testing.T) {
		idx := g12PlatformTrace(t)
		q := g12PlatformQuery()
		q.View, q.PID, q.Thread = "wakeup_chain", 562, ""
		account := b1607InventoryResultAccount(t, Run(idx, q))
		inventory := account.SleepInventory
		b1607AssertUnassessed(t, inventory)
		markedSleepMs := 0.0
		for _, row := range inventory.Occurrences {
			if row.BlockedReasonIOWaitKnown && row.BlockedReasonIOWait > 0 {
				if row.State != StateSSleep || row.BlockedReasonLine <= 0 {
					t.Fatalf("IO marker cannot silently reclassify this S-state fixture: %+v", row)
				}
				markedSleepMs += row.DurationMs
			}
		}
		if math.Abs(markedSleepMs-7.919) > 0.05 || math.Abs(markedSleepMs-account.SleepIOWaitMs) > 1e-6 ||
			inventory.IOWaitMs != 0 || account.IOWaitMs != 0 {
			t.Fatalf("S-side IO evidence must preserve the existing non-additive accounting: inventory=%+v account=%+v", inventory, account)
		}
	})
}

func TestB1607AInventoryUnavailableIsNotMeasuredZero(t *testing.T) {
	window := TimeWindow{StartTs: 100, EndTs: 100.1}
	for _, tc := range []struct {
		name string
		tl   TimelineResult
	}{
		{"absent", TimelineResult{}},
		{"integrity failure", TimelineResult{IntegrityFailure: "scheduler_row_parse_incomplete", Intervals: []Interval{cov4Interval(StateSSleep, 100, 100.1)}}},
		{"zero width", cov4Timeline([]Interval{cov4Interval(StateSSleep, 100, 100)})},
		{"nonfinite", cov4Timeline([]Interval{cov4Interval(StateSSleep, 100, math.Inf(1))})},
		{"reversed", cov4Timeline([]Interval{cov4Interval(StateSSleep, 100.1, 100)})},
		{"unknown state only", cov4Timeline([]Interval{cov4Interval(StateUnknown, 100, 100.1)})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildTargetWindowSleepInventory(tc.tl, window); got != nil {
				t.Fatalf("unavailable state evidence must not become a zero-wait inventory: %+v", got)
			}
		})
	}
	knownRunning := cov4Timeline([]Interval{cov4Interval(StateRunning, 100, 100.1)})
	got := buildTargetWindowSleepInventory(knownRunning, window)
	b1607AssertUnassessed(t, got)
	if got.Total != 0 || got.TotalMs != 0 || got.Emitted != 0 || got.OutputStatus != "complete" {
		t.Fatalf("a measured non-sleep timeline may report no observed sleeps: %+v", got)
	}
}

func TestB1607AActualRunKeepsIdentityFailClosed(t *testing.T) {
	trace := "idle-0 (0) [000] .... 10.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n" +
		"target-41 (41) [000] .... 10.001000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=waker next_pid=2 next_prio=120\n" +
		"waker-2 (2) [000] .... 10.002000: sched_wakeup_new: comm=target pid=41 prio=120 target_cpu=000\n" +
		"waker-2 (2) [000] .... 10.003000: sched_switch: prev_comm=waker prev_pid=2 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n" +
		"target-41 (41) [000] .... 10.004000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=waker next_pid=2 next_prio=120\n" +
		"waker-2 (2) [000] .... 10.005000: sched_wakeup: comm=target pid=41 prio=120 target_cpu=000\n" +
		"waker-2 (2) [000] .... 10.006000: sched_switch: prev_comm=waker prev_pid=2 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120\n"
	idx := buildTraceIndex(t, "identity-sleeps.ftrace", trace)
	q := Query{View: "wakeup_chain", PID: 41, TimeStart: 10, TimeEnd: 10.006}
	tl := ThreadTimeline(idx, q)
	if tl.IntegrityFailure != "thread_incarnation_conflict" {
		t.Fatalf("fixture must exercise the existing lifecycle gate: %+v", tl)
	}
	result := Run(idx, q)
	if result.TargetWindowStates != nil {
		t.Fatalf("an identity-failed timeline must not acquire a complete sleep census: %+v", result.TargetWindowStates)
	}
}
