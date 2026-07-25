package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

// GAP-B2 (§13.3(b)/§13.6/§13.7, 2026-07-25) pins: the VS-1 periodic discount
// grew a second admission arm — d_sleep-dominant (waker→target) aggregates
// whose EVERY member carries a sched_blocked_reason caller in the
// timerWaitCallerClosedSet (timerfd_read first witness). A periodic timer
// wait is normal cadence wearing a D state; it must stop being advertised as
// eliminable I/O blocking. Invariants pinned here: one member outside the
// closed set fails the WHOLE aggregate closed (D fail-close red line, SYM-2
// seats intact — a sub-shape discount, never a D-family exemption), io_wait
// dominant stays excluded, and every raw field stays lossless.

// buildPeriodicTimerChain is the D∧timer twin of buildPeriodicVSyncChain:
// occurrence i is d_sleep-dominant with the given blocked-reason caller.
func buildPeriodicTimerChain(intervalsMs, dStateMs, runnableMs []float64, callers []string) ChainResult {
	base := 4520.100000
	start := base
	chain := ChainResult{Target: ThreadRef{PID: 4144, Comm: "main"}}
	for i := range dStateMs {
		if i > 0 {
			start += intervalsMs[i-1] / 1000
		}
		total := dStateMs[i] + runnableMs[i]
		end := start + total/1000
		chain.CausalImpacts = append(chain.CausalImpacts, WakeupCausalImpact{
			Thread:               ThreadRef{PID: 610, Comm: "TimerDispatcher"},
			ChainDepth:           1,
			DominantState:        string(StateDSleep),
			DominantImpactMs:     dStateMs[i],
			TotalMs:              total,
			DStateMs:             dStateMs[i],
			RunnableMs:           runnableMs[i],
			TargetBlockedMs:      total,
			FragmentCount:        2,
			Window:               TimeWindow{StartTs: start, EndTs: end},
			ActualWindow:         TimeWindow{StartTs: start, EndTs: end},
			LineStart:            100 + i*10,
			LineEnd:              105 + i*10,
			DFamilyBlockedCaller: callers[i%len(callers)],
		})
		chain.Nodes = append(chain.Nodes, ChainNode{
			ID:         fmt.Sprintf("timer-%d", i),
			Thread:     ThreadRef{PID: 610, Comm: "TimerDispatcher"},
			Window:     TimeWindow{StartTs: start, EndTs: end},
			Dominant:   StateDSleep,
			DurationMs: dStateMs[i],
			Depth:      1,
		})
	}
	return chain
}

var gapB2TimerIntervalsMs = []float64{8.302, 8.287, 8.315, 8.360, 8.298}
var gapB2TimerDStateMs = []float64{5.800, 6.400, 6.200, 5.900, 6.100, 5.856}
var gapB2TimerRunnableMs = []float64{0.018, 0.020, 0.015, 0.017, 0.016, 0.019}

func TestPeriodicSourceDTimerCadenceDiscount(t *testing.T) {
	chain := buildPeriodicTimerChain(gapB2TimerIntervalsMs, gapB2TimerDStateMs, gapB2TimerRunnableMs,
		[]string{"timerfd_read+0x74/0x120"})
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one TimerDispatcher aggregate, got %+v", aggregates)
	}
	agg := aggregates[0]
	if !agg.PeriodicSource || !agg.PeriodicTimerWait || agg.PeriodicTimerCaller != "timerfd_read" {
		t.Fatalf("D∧timer cadence must detect as periodic timer source: %+v", agg)
	}
	if !near(agg.DetectedPeriodMs, 8.302, 0.001) {
		t.Fatalf("detected period should be 8.302ms, got %.6f", agg.DetectedPeriodMs)
	}
	if agg.LatenessMs != 0 {
		t.Fatalf("sub-period target waits must carry zero lateness, got %.6f", agg.LatenessMs)
	}
	if !near(agg.EffectivePeriodicImpactMs, 0.105, 0.001) {
		t.Fatalf("effective impact should be the runnable sum 0.105ms, got %.6f", agg.EffectivePeriodicImpactMs)
	}
	// Raw D accounting stays lossless (the discount only redirects ranking).
	if !near(agg.DStateMs, 36.256, 0.001) || !near(agg.DominantImpactMs, 36.256, 0.001) ||
		!near(agg.ProjectedImpactMs, 36.256, 0.001) || !near(agg.RunnableMs, 0.105, 0.001) {
		t.Fatalf("raw d_state/dominant/projected/runnable sums must stay untouched: %+v", agg)
	}
	if !strings.Contains(agg.Summary, "periodic_source=true") || !strings.Contains(agg.Summary, "timer_wait_caller=timerfd_read") {
		t.Fatalf("aggregate summary must disclose the timer credential: %q", agg.Summary)
	}
	for i, impact := range chain.CausalImpacts {
		if !impact.PeriodicSource || !impact.PeriodicTimerWait {
			t.Fatalf("member %d should carry the D∧timer periodic stamp: %+v", i, impact)
		}
		if !near(impact.DStateMs, gapB2TimerDStateMs[i], 0.0001) {
			t.Fatalf("member %d raw d_state must stay untouched: %+v", i, impact)
		}
	}
	if !strings.Contains(chain.CausalImpacts[3].Summary, "timer_wait_caller=timerfd_read") {
		t.Fatalf("stamped member summary must disclose the timer credential: %q", chain.CausalImpacts[3].Summary)
	}
}

func TestPeriodicSourceDTimerOneForeignCallerFailsClosed(t *testing.T) {
	// D fail-close: ONE member whose caller is outside the closed set kills
	// the discount for the whole aggregate — no majority vote, no sub-count.
	callers := []string{
		"timerfd_read+0x74/0x120", "timerfd_read+0x74/0x120", "timerfd_read+0x74/0x120",
		"z_erofs_runqueue+0x9c/0x2e0", "timerfd_read+0x74/0x120", "timerfd_read+0x74/0x120",
	}
	chain := buildPeriodicTimerChain(gapB2TimerIntervalsMs, gapB2TimerDStateMs, gapB2TimerRunnableMs, callers)
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggregates)
	}
	if aggregates[0].PeriodicSource || aggregates[0].PeriodicTimerWait {
		t.Fatalf("a foreign D caller must fail the whole aggregate closed: %+v", aggregates[0])
	}
	for i, impact := range chain.CausalImpacts {
		if impact.PeriodicSource || impact.PeriodicTimerWait {
			t.Fatalf("member %d must stay undiscounted: %+v", i, impact)
		}
	}
}

func TestPeriodicSourceDTimerUnknownOrEmptyCallerFailsClosed(t *testing.T) {
	for _, caller := range []string{"", "unknown"} {
		chain := buildPeriodicTimerChain(gapB2TimerIntervalsMs, gapB2TimerDStateMs, gapB2TimerRunnableMs, []string{caller})
		aggregates := aggregateWakeupCausalImpacts(&chain)
		if len(aggregates) != 1 || aggregates[0].PeriodicSource {
			t.Fatalf("caller %q must fail closed (禁猜): %+v", caller, aggregates)
		}
	}
}

func TestPeriodicSourceIOWaitDominantStaysExcluded(t *testing.T) {
	// io_wait keeps its bytes: the D∧timer arm admits d_sleep only.
	chain := buildPeriodicTimerChain(gapB2TimerIntervalsMs, gapB2TimerDStateMs, gapB2TimerRunnableMs,
		[]string{"timerfd_read+0x74/0x120"})
	for i := range chain.CausalImpacts {
		chain.CausalImpacts[i].DominantState = string(StateIOWait)
		chain.CausalImpacts[i].IOWaitMs = chain.CausalImpacts[i].DStateMs
		chain.CausalImpacts[i].DStateMs = 0
	}
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 || aggregates[0].PeriodicSource {
		t.Fatalf("io_wait dominant must stay outside the periodic arms: %+v", aggregates)
	}
}

func TestTimerWaitCallerClosedSetPinned(t *testing.T) {
	// The survey-derived closed set is a word authority: extending it is a
	// deliberate act with survey evidence, never a drive-by.
	if len(timerWaitCallerClosedSet) != 1 || !timerWaitCallerClosedSet["timerfd_read"] {
		t.Fatalf("timer closed set drifted: %+v", timerWaitCallerClosedSet)
	}
	for raw, want := range map[string]bool{
		"timerfd_read":              true,
		"timerfd_read+0x74/0x120":   true,
		"  timerfd_read+0x74/0x120": true,
		"hrtimer_nanosleep+0x9c":    false, // not surveyed — must NOT pass by name plausibility
		"z_erofs_runqueue+0x9c":     false,
		"unknown":                   false,
		"":                          false,
	} {
		if got := isTimerWaitCaller(raw); got != want {
			t.Fatalf("isTimerWaitCaller(%q) = %v, want %v", raw, got, want)
		}
	}
	if got := TimerWaitCallerSymbol("timerfd_read+0x74/0x120"); got != "timerfd_read" {
		t.Fatalf("symbol reduction drifted: %q", got)
	}
}

// TestPacingIdleMintsOnBinderFreeTrace pins §13.3(a): the pacing scan is
// decoupled from binder edges — a chain with ZERO IPC edges still mints the
// idle-cadence row when the segment-ending waker carries a typed VS-1
// periodic aggregate and the segment length matches the period.
func TestPacingIdleMintsOnBinderFreeTrace(t *testing.T) {
	chain := buildPeriodicVSyncChain(berlinVSyncIntervalsMs, berlinVSyncSleepMs, berlinVSyncRunnableMs)
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	if len(chain.AggregatedImpacts) != 1 || !chain.AggregatedImpacts[0].PeriodicSource {
		t.Fatalf("fixture precondition: waker must be a typed periodic source: %+v", chain.AggregatedImpacts)
	}
	// The waker's own aggregate proves the period; give each node a
	// segment-ending wakeup edge from that waker and a segment length ≈ one
	// period so pacingVerdict accepts it.
	period := chain.AggregatedImpacts[0].DetectedPeriodMs
	waker := ThreadRef{PID: 777, Comm: "TimerTick"}
	// Re-point the aggregate at the waker identity the edges will carry.
	chain.AggregatedImpacts[0].Thread = waker
	for i := range chain.Nodes {
		chain.Nodes[i].DurationMs = period
		chain.Nodes[i].EvidenceLine = 100 + i*10
		chain.Edges = append(chain.Edges, WakeupEdge{
			Waker:      waker,
			Wakee:      chain.Nodes[i].Thread,
			WakeupTs:   chain.Nodes[i].Window.EndTs,
			WakeupLine: 900 + i,
		})
	}
	waits, pacing, _ := findBinderWaitsForChain(&Index{}, chain, nil, nil)
	if len(waits) != 0 {
		t.Fatalf("no binder edges can mint binder waits: %+v", waits)
	}
	if len(pacing) == 0 {
		t.Fatalf("binder-free trace must still reach the pacing arm (§13.3(a) decoupling)")
	}
	if pacing[0].Kind != "periodic_idle" || pacing[0].TimerWaitCaller != "" {
		t.Fatalf("S-sleep segment mints the periodic lane without a timer credential: %+v", pacing[0])
	}
}

// TestPacingIdleDTimerNodeNeedsClosedSetCaller pins the D-side admission: a
// d_sleep node reaches the pacing arm ONLY through the timer closed-set
// credential on its impact record; a foreign D caller stays off the lane.
func TestPacingIdleDTimerNodeNeedsClosedSetCaller(t *testing.T) {
	build := func(caller string) ChainResult {
		chain := buildPeriodicTimerChain(gapB2TimerIntervalsMs, gapB2TimerDStateMs, gapB2TimerRunnableMs,
			[]string{caller})
		chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
		waker := ThreadRef{PID: 777, Comm: "TimerTick"}
		period := 8.302
		// Period evidence: a typed periodic aggregate on the waker (S form —
		// the sleeper's own D discount is independent of this fixture knob).
		chain.AggregatedImpacts = append(chain.AggregatedImpacts, WakeupCausalAggregate{
			Thread: waker, PeriodicSource: true, DetectedPeriodMs: period,
		})
		for i := range chain.Nodes {
			chain.Nodes[i].DurationMs = period
			chain.Edges = append(chain.Edges, WakeupEdge{
				Waker:      waker,
				Wakee:      chain.Nodes[i].Thread,
				WakeupTs:   chain.Nodes[i].Window.EndTs,
				WakeupLine: 900 + i,
			})
		}
		return chain
	}
	timer := build("timerfd_read+0x74/0x120")
	_, pacing, _ := findBinderWaitsForChain(&Index{}, timer, nil, nil)
	if len(pacing) == 0 {
		t.Fatal("d_sleep node with a closed-set timer caller must mint the idle-cadence row")
	}
	if pacing[0].TimerWaitCaller != "timerfd_read" {
		t.Fatalf("the row must carry the typed timer credential: %+v", pacing[0])
	}
	if !strings.Contains(pacing[0].Summary, "D-state timer wait (sched_blocked_reason caller=timerfd_read)") ||
		!strings.Contains(pacing[0].Summary, "not eliminable I/O blocking") {
		t.Fatalf("the mechanism sentence must render: %q", pacing[0].Summary)
	}
	foreign := build("z_erofs_runqueue+0x9c/0x2e0")
	_, pacing, _ = findBinderWaitsForChain(&Index{}, foreign, nil, nil)
	if len(pacing) != 0 {
		t.Fatalf("a foreign D caller must stay off the idle-cadence lane (D fail-close): %+v", pacing)
	}
}

// TestPeriodicSourceDTimerMinOccurrenceGate — 复核修 F3 (dual review
// 2026-07-25, mutation-verified hole): the D∧timer arm shares the
// wakeupPeriodicMinOccurrences gate with the S arm; 3 timer-credentialed
// occurrences must NOT detect (a demand-driven worker doing a few timerfd
// epoll waits at coincidental spacing keeps its io_blocking bytes).
func TestPeriodicSourceDTimerMinOccurrenceGate(t *testing.T) {
	chain := buildPeriodicTimerChain(
		[]float64{8.302, 8.287},
		[]float64{5.800, 6.400, 6.200},
		[]float64{0.018, 0.020, 0.015},
		[]string{"timerfd_read+0x74/0x120"})
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 || aggregates[0].PeriodicSource || aggregates[0].PeriodicTimerWait {
		t.Fatalf("3 timer occurrences are below the min-occurrence gate and must keep io_blocking bytes: %+v", aggregates)
	}
}

// TestPeriodicSourceDTimerAperiodicFailsClosed — 复核修 F3: the cadence gates
// (robust period, early-fire veto, in-band ratio) bind the D arm exactly as
// the S arm; aperiodic timer-credentialed waits must not take the discount.
func TestPeriodicSourceDTimerAperiodicFailsClosed(t *testing.T) {
	chain := buildPeriodicTimerChain(
		[]float64{3.1, 21.7, 6.9, 44.0, 12.3},
		gapB2TimerDStateMs, gapB2TimerRunnableMs,
		[]string{"timerfd_read+0x74/0x120"})
	aggregates := aggregateWakeupCausalImpacts(&chain)
	if len(aggregates) != 1 || aggregates[0].PeriodicSource || aggregates[0].PeriodicTimerWait {
		t.Fatalf("aperiodic timer waits must fail the cadence gates closed: %+v", aggregates)
	}
}

// timerWireTrace: the target (app-20) blocks; its waker (tmr-30) spends the
// blocking window in a D-state timer wait (sched_blocked_reason iowait=0
// caller=timerfd_read) before waking the target — the GAP-B2 wire fixture:
// the dependency occurrence's typed DFamilyBlockedCaller must be stamped at
// summarize time from the physical blocked-reason row.
const timerWireTrace = `
        app-20   (   20) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        tmr-30   (   30) [000] .... 1.105000: sched_switch: prev_comm=tmr prev_pid=30 prev_prio=20 prev_state=D ==> next_comm=idle/0 next_pid=0 next_prio=120
      idle/0-0   (    0) [000] .... 1.106000: sched_blocked_reason: pid=30 iowait=0 caller=timerfd_read+0x74/0x120
      idle/0-0   (    0) [000] .... 1.175000: sched_wakeup: comm=tmr pid=30 prio=20 target_cpu=000
        tmr-30   (   30) [000] .... 1.176000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=tmr next_pid=30 next_prio=20
        tmr-30   (   30) [000] .... 1.180000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
        app-20   (   20) [001] .... 1.220000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
        app-20   (   20) [001] .... 1.300000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        tmr-30   (   30) [000] .... 1.305000: sched_switch: prev_comm=tmr prev_pid=30 prev_prio=20 prev_state=D ==> next_comm=idle/0 next_pid=0 next_prio=120
      idle/0-0   (    0) [000] .... 1.375000: sched_wakeup: comm=tmr pid=30 prio=20 target_cpu=000
        tmr-30   (   30) [000] .... 1.376000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=tmr next_pid=30 next_prio=20
        tmr-30   (   30) [000] .... 1.380000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
        app-20   (   20) [001] .... 1.420000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
`

func TestSummarizeStampsDFamilyBlockedCallerFromTrace(t *testing.T) {
	idx := buildTraceIndex(t, "timer-wire.systrace", timerWireTrace)
	chain := BuildWakeupChain(idx, Query{PID: 20, TimeStart: 1.10, TimeEnd: 1.43, MaxDepth: 4, MinDurationMs: 1})
	withCaller, withoutCaller := 0, 0
	for _, impact := range chain.CausalImpacts {
		if impact.Thread.PID != 30 || impact.DominantState != string(StateDSleep) {
			continue
		}
		// 复核修 F4 (dual review 2026-07-25, mutation-verified hole): the two
		// D windows discriminate window discipline from a stale/nearest guess
		// — the 1.105..1.176 window carries its VERBATIM in-window row, the
		// 1.305..1.376 window has no sched_blocked_reason row at all and must
		// stay empty (禁猜: types.go DFamilyBlockedCaller contract).
		if impact.Window.StartTs < 1.3 {
			if impact.DFamilyBlockedCaller != "timerfd_read+0x74/0x120" {
				t.Fatalf("first D window must carry its verbatim in-window caller: %+v", impact)
			}
			withCaller++
		} else {
			if impact.DFamilyBlockedCaller != "" {
				t.Fatalf("a D window with no blocked_reason row must stay empty, never inherit an earlier window's caller: %+v", impact)
			}
			withoutCaller++
		}
	}
	if withCaller == 0 || withoutCaller == 0 {
		t.Fatalf("fixture must produce both D windows for pid 30 (with=%d without=%d): %+v", withCaller, withoutCaller, chain.CausalImpacts)
	}
}
