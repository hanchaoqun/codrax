package tracequery

import (
	"testing"
)

// parse_batch_p_test.go — 批 P (§7.11, docs/design/customer_dead_session_
// audit_20260703.md) reference-parity correction pins, hmtrace-flavor
// authoritative line shapes:
//
//   - A-1: INT-declared numeric fields tolerate the hmtrace REAL float-string
//     shape ("state=2200000.0") — Atoi fast path preserved byte-for-byte,
//     ParseFloat truncation fallback, non-numeric stays 0. Genuinely-float
//     fields are untouched (export_format.rs declares these INT; both shapes
//     coexist).
//   - A-2: sched comm values with spaces ("Signal Catcher") extract by KNOWN
//     " key=" boundary backtracking instead of the generic kvRE [^ ]+ value
//     pattern; space-free comms stay byte-identical; a comm containing '='
//     never swallows the following key. Other event families untouched.
//   - B-1: prev_state first-char coverage extends by the precise single
//     chars I (TASK_IDLE → sleep family, reference consumer parity) and
//     T/t → stopped, X/Z → dead (typed non-Unknown classes, lane sums skip
//     them). R+ / D|K modifier behavior unchanged.
//   - B-2 (folded into the VS-2b/2c correction round): keyless positional
//     clock_set_rate payloads ("clock_set_rate: <name> <rate>") retain the
//     rate value; keyed shapes unchanged.

func TestBatchP_A1_FloatNumericFieldTolerance(t *testing.T) {
	intern := newStringInterner()
	cases := []struct {
		name  string
		line  string
		check func(t *testing.T, ev Event)
	}{
		{
			name: "cpu_frequency float string enters the timeline",
			line: `          <idle>-0     (-----) [011] d..2 168758.700000: cpu_frequency: state=2200000.0 cpu_id=11`,
			check: func(t *testing.T, ev Event) {
				if ev.Type != EventCPUFrequency || ev.Frequency != 2200000 {
					t.Fatalf("float-string frequency must truncate to 2200000, got %+v", ev)
				}
				if !ev.CPUForFieldValid || ev.CPUForField != 11 {
					t.Fatalf("cpu_id must stay keyed, got %+v", ev)
				}
			},
		},
		{
			name: "cpu_frequency integer fast path does not regress",
			line: `          <idle>-0     (-----) [011] d..2 168758.700000: cpu_frequency: state=2200000 cpu_id=11`,
			check: func(t *testing.T, ev Event) {
				if ev.Frequency != 2200000 {
					t.Fatalf("integer string must keep the Atoi fast path, got %+v", ev)
				}
			},
		},
		{
			name: "cpu_frequency non-numeric stays zero",
			line: `          <idle>-0     (-----) [011] d..2 168758.700000: cpu_frequency: state=abc cpu_id=11`,
			check: func(t *testing.T, ev Event) {
				if ev.Frequency != 0 {
					t.Fatalf("non-numeric must stay 0 (fail-quiet contract), got %+v", ev)
				}
			},
		},
		{
			name: "cpu_idle float state",
			line: `          <idle>-0     (-----) [002] d..2 168758.700000: cpu_idle: state=1.0 cpu_id=2`,
			check: func(t *testing.T, ev Event) {
				if ev.Type != EventCPUIdle || ev.State != 1 {
					t.Fatalf("cpu_idle float state must truncate to 1, got %+v", ev)
				}
			},
		},
		{
			name: "cpu_idle idle-exit sentinel in float shape",
			line: `          <idle>-0     (-----) [002] d..2 168758.700000: cpu_idle: state=4294967295.0 cpu_id=2`,
			check: func(t *testing.T, ev Event) {
				if ev.State != 4294967295 {
					t.Fatalf("idle-exit sentinel float shape must survive, got %+v", ev)
				}
			},
		},
		{
			name: "cpu_frequency_limits float min/max",
			line: `          <idle>-0     (-----) [011] d..2 168758.700000: cpu_frequency_limits: min=300000.0 max=2200000.0 cpu_id=11`,
			check: func(t *testing.T, ev Event) {
				if ev.Type != EventCPUFrequencyLimit || ev.FrequencyMin != 300000 || ev.FrequencyMax != 2200000 {
					t.Fatalf("limits float min/max must truncate and stay DISTINCT (§7.11 C-1), got %+v", ev)
				}
			},
		},
		{
			name: "clock_set_rate keyed float rate",
			line: `          <idle>-0     (-----) [000] d..2 168758.700000: clock_set_rate: uni_clk state=1866000000.0 cpu_id=0`,
			check: func(t *testing.T, ev Event) {
				if ev.Type != EventClockSetRate || ev.Frequency != 1866000000 {
					t.Fatalf("keyed float clock rate must truncate, got %+v", ev)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok := ParseLine(1, tc.line, intern)
			if !ok {
				t.Fatalf("line must parse: %s", tc.line)
			}
			tc.check(t, ev)
		})
	}
}

// End-to-end A-1 witness: an hmtrace-flavor trace whose EVERY cpu_frequency
// row uses the float-string shape still feeds the frequency timeline — the
// VS-2 fold resolves the same fmax and deficit as the integer-shape ledger
// pin (fail-quiet timeline loss was the customer symptom: empty fmax and
// residency for the whole window).
func TestBatchP_A1_FloatFrequencyTimelineEndToEnd(t *testing.T) {
	idx := buildTraceIndex(t, "batch_p_a1_float.systrace", `
      <idle>-0 (-----) [002] .... 4.900000: cpu_frequency: state=1000000.0 cpu_id=2
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=2000000.0 cpu_id=7
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [002] .... 5.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.010000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.020000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
	`)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil || !dep.SupplyFoldBasis.AllKnown() {
		t.Fatalf("float-shape frequency rows must still cover the fold basis: %+v", dep.SupplyFoldBasis)
	}
	if dep.SupplyFoldDeficitMs < 4.7 || dep.SupplyFoldDeficitMs > 5.3 {
		t.Fatalf("float-shape timeline must fold identically to the integer ledger pin (~5ms), got %.3f", dep.SupplyFoldDeficitMs)
	}
	if dep.SupplyFoldBasis.FmaxKHz != 2000000 {
		t.Fatalf("fmax must resolve from the float-shape samples, got %+v", dep.SupplyFoldBasis)
	}
}

func TestBatchP_A2_SchedCommSpaceBoundary(t *testing.T) {
	intern := newStringInterner()

	ev, ok := ParseLine(1, `           ACCS0-2716  ( 2519) [000] d..3 168758.662877: sched_switch: prev_comm=Signal Catcher prev_pid=812 prev_prio=110 prev_state=S ==> next_comm=Jit thread pool next_pid=900 next_prio=97`, intern)
	if !ok {
		t.Fatal("sched_switch line must parse")
	}
	if ev.PrevComm != "Signal Catcher" || ev.PrevPID != 812 || ev.PrevState != "S" {
		t.Fatalf("prev_comm with a space must survive to the known-key boundary: %+v", ev)
	}
	if ev.NextComm != "Jit thread pool" || ev.NextPID != 900 || ev.NextPrio != 97 {
		t.Fatalf("next_comm with spaces must survive: %+v", ev)
	}

	// Byte parity: a space-free comm is byte-identical to the generic kv
	// extraction (no behavior drift for the overwhelmingly common shape).
	ev, ok = ParseLine(2, `  HeapTaskDaemon-2532  ( 2519) [000] d..3 168758.663107: sched_switch: prev_comm=HeapTaskDaemon prev_pid=2532 prev_prio=124 prev_state=R ==> next_comm=rcu_preempt next_pid=7 next_prio=98`, intern)
	if !ok {
		t.Fatal("line must parse")
	}
	if ev.PrevComm != "HeapTaskDaemon" || ev.NextComm != "rcu_preempt" {
		t.Fatalf("space-free comm must stay byte-identical: %+v", ev)
	}

	// A comm containing '=' must not swallow the following known key: the
	// boundary is the CLOSED key set, not any token containing '='.
	ev, ok = ParseLine(3, `           ACCS0-2716  ( 2519) [000] d..3 168758.662877: sched_switch: prev_comm=x=1 svc prev_pid=812 prev_prio=110 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120`, intern)
	if !ok {
		t.Fatal("line must parse")
	}
	if ev.PrevComm != "x=1 svc" || ev.PrevPID != 812 {
		t.Fatalf("'=' inside a comm must not swallow prev_pid: %+v", ev)
	}

	// sched_wakeup / sched_waking comm shares the same boundary extraction.
	wake, ok := ParseLine(4, `           ACCS0-2716  ( 2519) [000] d..5 168758.662877: sched_waking: comm=Signal Catcher pid=812 prio=110 target_cpu=002`, intern)
	if !ok {
		t.Fatal("sched_waking line must parse")
	}
	if wake.WakeeComm != "Signal Catcher" || wake.WakeePID != 812 || wake.TargetCPU != 2 {
		t.Fatalf("wakeup comm with a space must survive: %+v", wake)
	}
}

func TestBatchP_B1_PrevStateMapping(t *testing.T) {
	cases := []struct {
		prev string
		want ThreadState
	}{
		{"S", StateSSleep},
		{"D", StateDSleep},
		{"D|K", StateDSleep}, // modifier drop semantics unchanged
		{"R", StateRunnable},
		{"R+", StateRunnable}, // preempt marker unchanged
		{"I", StateSSleep},    // TASK_IDLE → sleep family (§7.11 B-1)
		{"T", StateStopped},
		{"t", StateStopped},
		{"X", StateDead},
		{"Z", StateDead},
		{"?", StateUnknown}, // unlisted codes stay Unknown
	}
	for _, tc := range cases {
		if got := stateFromPrevState(tc.prev); got != tc.want {
			t.Errorf("stateFromPrevState(%q) = %q, want %q", tc.prev, got, tc.want)
		}
	}
}

// prev_state=I sleep segments enter the window state aggregation (the ledger
// pin: kworker idle time was fail-quietly DROPPED as Unknown before B-1);
// stopped segments stay OUT of every pressure lane — StateStopped is typed
// non-Unknown for the interval faces but is intentionally no lane member.
func TestBatchP_B1_IdleSleepEntersWindowAggregation(t *testing.T) {
	idx := buildTraceIndex(t, "batch_p_b1_idle.systrace", `
     kworker/2:1-500 (  500) [002] d..3 5.000000: sched_switch: prev_comm=kworker/2:1 prev_pid=500 prev_prio=120 prev_state=I ==> next_comm=swapper/2 next_pid=0 next_prio=120
        stopper-600 (  600) [003] d..3 5.000000: sched_switch: prev_comm=stopper prev_pid=600 prev_prio=120 prev_state=T ==> next_comm=swapper/3 next_pid=0 next_prio=120
          <idle>-0   (-----) [002] d..3 5.010000: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=kworker/2:1 next_pid=500 next_prio=120
          <idle>-0   (-----) [003] d..3 5.010000: sched_switch: prev_comm=swapper/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=stopper next_pid=600 next_prio=120
	`)
	stats := ComputeWindowStats(idx, normalizeQuery(idx, Query{TimeStart: 5.0, TimeEnd: 5.02}))
	foundIdleSleep := false
	for _, td := range stats.SleepTop {
		if td.Thread.PID == 500 {
			foundIdleSleep = true
			if td.DurationMs < 9.7 || td.DurationMs > 10.3 {
				t.Fatalf("prev_state=I sleep segment must book ~10ms, got %+v", td)
			}
		}
	}
	if !foundIdleSleep {
		t.Fatalf("prev_state=I segment must enter the sleep aggregation (§7.11 B-1), got %+v", stats.SleepTop)
	}
	for _, lane := range [][]ThreadDuration{stats.SleepTop, stats.DStateTop, stats.RunnableTop, stats.IOWaitTop} {
		for _, td := range lane {
			if td.Thread.PID == 600 {
				t.Fatalf("stopped (prev_state=T) time must not enter any pressure lane, got %+v", td)
			}
		}
	}
}

func TestBatchP_B2_ClockSetRatePositional(t *testing.T) {
	intern := newStringInterner()
	cases := []struct {
		name      string
		line      string
		wantType  EventType
		wantRate  int
		wantClock string
	}{
		{
			name:      "positional integer rate",
			line:      `          <idle>-0     (-----) [000] d..2 168758.700000: clock_set_rate: ddr_freq 1866000000`,
			wantType:  EventClockSetRate,
			wantRate:  1866000000,
			wantClock: "ddr_freq",
		},
		{
			name:      "positional float rate (hmtrace flavor)",
			line:      `          <idle>-0     (-----) [000] d..2 168758.700000: clock_set_rate: cpu-cluster.0 2200000.0`,
			wantType:  EventClockSetRate,
			wantRate:  2200000,
			wantClock: "cpu-cluster.0",
		},
		{
			name:      "keyed shape does not regress (cpu-freq lane reclassified)",
			line:      `          <idle>-0     (-----) [000] d..2 168758.700000: clock_set_rate: pid_freq state=1800000 cpu_id=0`,
			wantType:  EventCPUFrequency,
			wantRate:  1800000,
			wantClock: "pid_freq",
		},
		{
			name:      "keyed shape does not regress (non-cpu lane)",
			line:      `          <idle>-0     (-----) [000] d..2 168758.700000: clock_set_rate: heca_ddr_freq state=3744 cpu_id=0`,
			wantType:  EventClockSetRate,
			wantRate:  3744,
			wantClock: "heca_ddr_freq",
		},
		{
			name:      "positional rate on a reclassified cpu-freq lane",
			line:      `          <idle>-0     (-----) [000] d..2 168758.700000: clock_set_rate: cpu_freq 3000000`,
			wantType:  EventCPUFrequency,
			wantRate:  3000000,
			wantClock: "cpu_freq",
		},
		{
			name:      "name-only payload keeps zero",
			line:      `          <idle>-0     (-----) [000] d..2 168758.700000: clock_set_rate: ddr_freq`,
			wantType:  EventClockSetRate,
			wantRate:  0,
			wantClock: "ddr_freq",
		},
		{
			name:      "non-numeric successor keeps zero",
			line:      `          <idle>-0     (-----) [000] d..2 168758.700000: clock_set_rate: ddr_freq on cpu_id=1`,
			wantType:  EventClockSetRate,
			wantRate:  0,
			wantClock: "ddr_freq",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok := ParseLine(1, tc.line, intern)
			if !ok {
				t.Fatalf("line must parse: %s", tc.line)
			}
			if ev.Type != tc.wantType || ev.Frequency != tc.wantRate || ev.ClockName != tc.wantClock {
				t.Fatalf("got type=%s rate=%d clock=%q, want type=%s rate=%d clock=%q (%+v)",
					ev.Type, ev.Frequency, ev.ClockName, tc.wantType, tc.wantRate, tc.wantClock, ev)
			}
		})
	}
}

// A-1 sequel pin (2026-07-04 review, F2): the float fallback range-checks
// BEFORE the int conversion. int(f) on a beyond-int64 float saturates to a
// POSITIVE math.MaxInt64 on arm64/amd64 — a u64 sentinel or 1e19 shape then
// sailed through every <=0 consumer filter, polluting fmax and fabricating
// supply gaps. Four rejected shapes pinned (u64 sentinel / 1e19 / 1e300 /
// negative), plus the survivors: real float frequencies, the u32 cpu_idle
// exit sentinel, and the byte-for-byte Atoi integer fast path.
func TestBatchP_A1_FloatToleranceSaturationGuard(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		// Rejected: sentinel semantics, not magnitudes.
		{"u64 sentinel float shape", "18446744073709551615.0", 0},
		{"u64 sentinel integer shape overflows Atoi into the float path", "18446744073709551615", 0},
		{"1e19 scientific shape", "1e19", 0},
		{"1e300 scientific shape", "1e300", 0},
		{"negative float shape", "-2200000.0", 0},
		{"NaN stays zero", "NaN", 0},
		{"Inf stays zero", "Inf", 0},
		// Survivors: real magnitudes below atoiFloatTolerantMax.
		{"real float frequency truncates", "2200000.0", 2200000},
		{"u32 cpu_idle exit sentinel survives", "4294967295.0", 4294967295},
		{"Hz-shaped lane magnitude survives", "2200000000.0", 2200000000},
		// Atoi fast path untouched (byte-for-byte contract), including sign.
		{"integer fast path", "2200000", 2200000},
		{"negative integer keeps Atoi semantics", "-5", -5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := atoiFloatTolerant(tc.raw); got != tc.want {
				t.Fatalf("atoiFloatTolerant(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
	// End-to-end witness: a u64-sentinel cpu_frequency row must not enter the
	// timeline as a positive fake frequency.
	intern := newStringInterner()
	ev, ok := ParseLine(1, `          <idle>-0     (-----) [011] d..2 168758.700000: cpu_frequency: state=18446744073709551615 cpu_id=11`, intern)
	if !ok {
		t.Fatalf("sentinel line must still parse structurally")
	}
	if ev.Frequency != 0 {
		t.Fatalf("u64 sentinel must collapse to 0, not saturate: %+v", ev)
	}
}
