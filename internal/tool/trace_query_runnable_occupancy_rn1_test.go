package tool

// trace_query_runnable_occupancy_rn1_test.go — RN-1(a) (§7.9, cust_runnable
// 2026-07-04) publisher pins. Customer live shape: the model anchored on
// OS_FFRT_2_3-49706 (runnable 2528ms of a 3000ms window, sleep=0, no wakeup
// edge) and the report had no mechanism explanation because the CMP-8
// occupancy decomposition never reached the observation ledger. The publish
// gate is precise arithmetic (runnable ≥ min(window×10%, 100ms), engine
// WindowMs > 0 — §7.10 显著门二审裁定: the absolute 100ms floor takes over on
// wide windows so a large absolute backlog can never be diluted away by the
// relative arm); below the gate or on an unbounded window nothing is
// published.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runnableOccupancyRN1Stats(runnableMs float64) tracequery.WindowStats {
	return tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 100.0, EndTs: 103.0}, // 3000ms
		RunnableTop: []tracequery.ThreadDuration{{
			Thread:     tracequery.ThreadRef{Comm: "OS_FFRT_2_3", PID: 49706},
			DurationMs: runnableMs,
			LineStart:  1200, LineEnd: 9800,
			StartTs: 100.1, EndTs: 102.9,
		}},
		CPUOccupancy: &tracequery.CPUOccupancyStats{
			WindowMs: 3000.0,
			TopThreads: []tracequery.CPUOccupancyThread{
				// The starved subject also ran a little — it must be excluded
				// from its own occupier roster.
				{Thread: tracequery.ThreadRef{Comm: "OS_FFRT_2_3", PID: 49706}, RunningMs: 90.0},
				{Thread: tracequery.ThreadRef{Comm: "RSHardwareThre", PID: 1063}, RunningMs: 1410.250},
				{Thread: tracequery.ThreadRef{Comm: "render_service", PID: 411}, RunningMs: 902.125},
				{Thread: tracequery.ThreadRef{Comm: "kworker/u16:3", PID: 77}, RunningMs: 401.000},
				{Thread: tracequery.ThreadRef{Comm: "logd.writer", PID: 55}, RunningMs: 12.000},
			},
		},
	}
}

func runnableOccupancyRN1Records(t *testing.T, stats tracequery.WindowStats) []types.ObservationRecord {
	t.Helper()
	ref := types.ObservationSourceRef{Path: "cust_runnable.systrace"}
	var out []types.ObservationRecord
	for _, obs := range traceQueryTypedWindowStatsObservations(stats, ref, "w1", "now") {
		if obs.Predicate == "runnable_occupancy" {
			out = append(out, obs)
		}
	}
	return out
}

func TestRunnableOccupancyObservationPublishedForCustomerShape(t *testing.T) {
	obs := runnableOccupancyRN1Records(t, runnableOccupancyRN1Stats(2528.000))
	if len(obs) != 1 {
		t.Fatalf("exactly one runnable_occupancy observation must publish, got %d", len(obs))
	}
	rec := obs[0]
	if rec.ClaimKey != "runnable_occupancy" || rec.Subject != "OS_FFRT_2_3-49706" {
		t.Fatalf("claim key / starved subject wrong: %+v", rec)
	}
	if rec.Value != "2528.000" || rec.Unit != "ms" {
		t.Fatalf("value must be the starved runnable ms: %q %q", rec.Value, rec.Unit)
	}
	notes := strings.Join(rec.RichNotes, "\n")
	for _, want := range []string{
		"starved_runnable_ms=2528.000",
		// top-3 occupiers in full-window cpu·ms order, subject excluded.
		"occupier_1=RSHardwareThre-1063:1410.250ms",
		"occupier_2=render_service-411:902.125ms",
		"occupier_3=kworker/u16:3-77:401.000ms",
		"window_ms=3000.000",
		"selected_window=100.000000..103.000000",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("runnable_occupancy notes missing %q:\n%s", want, notes)
		}
	}
	for _, banned := range []string{
		"occupier_4",
		// the starved subject must never occupy itself …
		"occupier_1=OS_FFRT_2_3",
		// … and a single significant starved thread carries no also_starved.
		"also_starved",
	} {
		if strings.Contains(notes, banned) {
			t.Fatalf("runnable_occupancy notes must not carry %q:\n%s", banned, notes)
		}
	}
	if !strings.Contains(rec.Summary, "runnable_occupancy OS_FFRT_2_3-49706") ||
		!strings.Contains(rec.Summary, "top_occupiers=RSHardwareThre-1063:1410.250ms") {
		t.Fatalf("summary must carry the occupier attribution: %s", rec.Summary)
	}
}

// §7.10 dual-basis gate on the customer's 3000ms window: min(3000×10%, 100ms)
// = 100ms — the ABSOLUTE floor governs, not the 300ms relative arm the RN-B
// first cut pinned (299.999/300.000). Rewritten with the second-review ruling:
// a 3000ms window is already wide enough that 100ms of runnable backlog is a
// mechanism worth publishing; the old single-basis pin was the dilution bug,
// not the contract.
func TestRunnableOccupancyObservationBelowFloorSilent(t *testing.T) {
	if obs := runnableOccupancyRN1Records(t, runnableOccupancyRN1Stats(99.999)); len(obs) != 0 {
		t.Fatalf("runnable below min(window×10%%, 100ms) must not publish: %+v", obs)
	}
	// Exactly at the gate publishes (≥, pinned).
	if obs := runnableOccupancyRN1Records(t, runnableOccupancyRN1Stats(100.000)); len(obs) != 1 {
		t.Fatalf("runnable exactly at min(window×10%%, 100ms) must publish, got %+v", obs)
	}
	// The old relative-arm boundary values sit far above the corrected gate
	// and of course still publish.
	if obs := runnableOccupancyRN1Records(t, runnableOccupancyRN1Stats(299.999)); len(obs) != 1 {
		t.Fatalf("299.999ms on a 3000ms window is significant under the dual-basis gate, got %+v", obs)
	}
}

// §7.10 wide-window dilution pin (the customer counterexample that forced the
// second review): a 3.3s window with a 200ms absolute backlog — 24 whole
// 120fps frame budgets — was silent under the single-basis 10% gate (330ms).
// Under min(window×10%, 100ms) the 100ms floor governs and it publishes.
func TestRunnableOccupancyObservationWideWindowDilutionPublishes(t *testing.T) {
	stats := runnableOccupancyRN1Stats(200.000)
	stats.Window = tracequery.TimeWindow{StartTs: 100.0, EndTs: 103.3}
	stats.CPUOccupancy.WindowMs = 3300.0
	obs := runnableOccupancyRN1Records(t, stats)
	if len(obs) != 1 {
		t.Fatalf("200ms backlog on a 3.3s window must publish under the dual-basis gate, got %+v", obs)
	}
	if !strings.Contains(strings.Join(obs[0].RichNotes, "\n"), "starved_runnable_ms=200.000") {
		t.Fatalf("published observation must carry the 200ms backlog: %v", obs[0].RichNotes)
	}
}

// §7.10 dual-basis three-state pin on the shared helper itself: (a) narrow
// window — the relative arm governs; (b) the 1000ms crossover where both arms
// agree; (c) wide window — the absolute floor governs and the gate NEVER
// tightens as the window widens (monotonicity that kills the dilution
// counter-intuition).
func TestRunnableSignificanceThresholdDualBasisThreeStates(t *testing.T) {
	// (a) relative arm: 500ms window → min(50, 100) = 50ms.
	if got := traceQueryRunnableSignificanceThresholdMs(500.0); got != 50.0 {
		t.Fatalf("narrow-window threshold must be the relative arm 50ms, got %.3f", got)
	}
	if traceQueryRunnableSignificant(49.999, 500.0) || !traceQueryRunnableSignificant(50.000, 500.0) {
		t.Fatalf("relative-arm boundary must gate at exactly 50ms")
	}
	// (b) crossover: 1000ms window → both arms are exactly 100ms.
	if got := traceQueryRunnableSignificanceThresholdMs(1000.0); got != 100.0 {
		t.Fatalf("crossover threshold must be 100ms, got %.3f", got)
	}
	// (c) absolute floor: 3300ms window → min(330, 100) = 100ms.
	if got := traceQueryRunnableSignificanceThresholdMs(3300.0); got != 100.0 {
		t.Fatalf("wide-window threshold must be the absolute floor 100ms, got %.3f", got)
	}
	if traceQueryRunnableSignificant(99.999, 3300.0) || !traceQueryRunnableSignificant(200.0, 3300.0) {
		t.Fatalf("absolute-floor boundary must gate at exactly 100ms")
	}
	// Unbounded window: no denominator, no verdict.
	if traceQueryRunnableSignificanceThresholdMs(0) != 0 || traceQueryRunnableSignificant(500.0, 0) {
		t.Fatalf("unbounded window must refuse a significance verdict")
	}
}

// Unbounded window (engine WindowMs=0 — queryWindowWallMs refused to
// estimate): no denominator, no observation.
func TestRunnableOccupancyObservationUnboundedWindowSilent(t *testing.T) {
	stats := runnableOccupancyRN1Stats(2528.000)
	stats.CPUOccupancy.WindowMs = 0
	if obs := runnableOccupancyRN1Records(t, stats); len(obs) != 0 {
		t.Fatalf("unbounded window must not publish runnable_occupancy: %+v", obs)
	}
	// No occupancy decomposition at all: same silence.
	stats = runnableOccupancyRN1Stats(2528.000)
	stats.CPUOccupancy = nil
	if obs := runnableOccupancyRN1Records(t, stats); len(obs) != 0 {
		t.Fatalf("missing cpu_occupancy must not publish runnable_occupancy: %+v", obs)
	}
}

// Multiple significant starved threads: still ONE observation (the largest
// runnable wins) with a typed also_starved count for the rest.
func TestRunnableOccupancyObservationAlsoStarvedFold(t *testing.T) {
	stats := runnableOccupancyRN1Stats(2528.000)
	stats.RunnableTop = append(stats.RunnableTop,
		tracequery.ThreadDuration{Thread: tracequery.ThreadRef{Comm: "OS_FFRT_2_4", PID: 49707}, DurationMs: 640.000},
		tracequery.ThreadDuration{Thread: tracequery.ThreadRef{Comm: "tiny-wait", PID: 12}, DurationMs: 40.000}, // below floor
	)
	obs := runnableOccupancyRN1Records(t, stats)
	if len(obs) != 1 {
		t.Fatalf("multiple starved threads must still fold to one observation, got %d", len(obs))
	}
	if obs[0].Subject != "OS_FFRT_2_3-49706" {
		t.Fatalf("largest runnable must stay the subject: %s", obs[0].Subject)
	}
	if !strings.Contains(strings.Join(obs[0].RichNotes, "\n"), "also_starved=1") {
		t.Fatalf("also_starved must count the other significant starved threads: %v", obs[0].RichNotes)
	}
}
