package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

// wakeup_edge_census_test.go — WAKE-CENSUS engine pins (立案 §29.58,
// 2026-07-13, docs/design/real_trace_campaign_20260705.md; PRC-F1 witness
//「OS_IPC_14_34911 ×4」三重假). The census folds over the FULL edge set —
// never a capped view — with deterministic order and explicit overflow.
//
// MUTATION self-checks:
//   - re-sourcing buildWakeupEdgeCensus from a truncated edge slice (the
//     publication row-cap face) reds TestBuildWakeupChainMintsEdgeCensus
//     (the ×3 pair count drops);
//   - deleting the physical-row dedup reds TestBuildWakeupEdgeCensusDedup
//     (the duplicated physical row would count twice);
//   - replacing the typed tie keys with map/first-appearance order reds
//     TestBuildWakeupEdgeCensusDeterministicOrder (pairs are inserted in
//     REVERSE lexicographic order, so first-appearance order disagrees);
//   - dropping the overflow edge accounting (or deriving it from the capped
//     rows) reds TestBuildWakeupEdgeCensusPairCapOverflow.

func censusTestEdge(waker, wakee ThreadRef, ts float64, line int) WakeupEdge {
	return WakeupEdge{Waker: waker, Wakee: wakee, WakeupTs: ts, WakeupLine: line}
}

// TestBuildWakeupEdgeCensusFoldAndBounds — count + first/last ts fold per
// pair over the full input.
func TestBuildWakeupEdgeCensusFoldAndBounds(t *testing.T) {
	a := ThreadRef{Comm: "wakerA", PID: 200}
	b := ThreadRef{Comm: "wakerB", PID: 300}
	target := ThreadRef{Comm: "app", PID: 100}
	edges := []WakeupEdge{
		censusTestEdge(a, target, 5.010, 30),
		censusTestEdge(b, target, 5.019, 33),
		censusTestEdge(a, target, 5.002, 12),
		censusTestEdge(a, target, 5.030, 41),
	}
	rows, overflowPairs, overflowEdges := buildWakeupEdgeCensus(edges, ThreadRef{}, 16)
	if overflowPairs != 0 || overflowEdges != 0 {
		t.Fatalf("no overflow expected below the pair cap, got pairs=%d edges=%d", overflowPairs, overflowEdges)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 census pairs, got %+v", rows)
	}
	if rows[0].Waker.PID != 200 || rows[0].Count != 3 || rows[0].FirstTs != 5.002 || rows[0].LastTs != 5.030 {
		t.Fatalf("wakerA pair must fold count=3 first=5.002 last=5.030, got %+v", rows[0])
	}
	if rows[1].Waker.PID != 300 || rows[1].Count != 1 || rows[1].FirstTs != 5.019 || rows[1].LastTs != 5.019 {
		t.Fatalf("wakerB pair must fold count=1 at 5.019, got %+v", rows[1])
	}
}

// TestBuildWakeupEdgeCensusDedup — the SAME physical sched_wakeup row
// observed by two branch expansions (same pair + ts + line) counts once;
// a DIFFERENT physical row of the same pair still counts.
func TestBuildWakeupEdgeCensusDedup(t *testing.T) {
	a := ThreadRef{Comm: "wakerA", PID: 200}
	target := ThreadRef{Comm: "app", PID: 100}
	edges := []WakeupEdge{
		censusTestEdge(a, target, 5.010, 30),
		censusTestEdge(a, target, 5.010, 30), // identical physical row (branch republication)
		censusTestEdge(a, target, 5.020, 55), // distinct physical row
	}
	rows, _, _ := buildWakeupEdgeCensus(edges, ThreadRef{}, 16)
	if len(rows) != 1 || rows[0].Count != 2 {
		t.Fatalf("physical republication must collapse (want count=2), got %+v", rows)
	}
}

// TestBuildWakeupEdgeCensusDeterministicOrder — count desc, then the typed
// tie chain (waker comm, waker pid, wakee comm, wakee pid). Tie pairs are
// inserted in REVERSE lexicographic order so first-appearance order and the
// typed key disagree — deleting the tie key cannot stay green (复核 F2 教训:
// aligned orders make a tie-key pin vacuous).
func TestBuildWakeupEdgeCensusDeterministicOrder(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	var edges []WakeupEdge
	// Reverse-lex insertion: wakerE, wakerD, wakerC — all ×1 ties.
	for i, comm := range []string{"wakerE", "wakerD", "wakerC"} {
		edges = append(edges, censusTestEdge(ThreadRef{Comm: comm, PID: 500 + i}, target, 5.001+float64(i)*0.001, 10+i))
	}
	// The dominant ×2 pair arrives LAST — count desc must still front it.
	edges = append(edges,
		censusTestEdge(ThreadRef{Comm: "wakerZ", PID: 900}, target, 5.010, 40),
		censusTestEdge(ThreadRef{Comm: "wakerZ", PID: 900}, target, 5.020, 50),
	)
	rows, _, _ := buildWakeupEdgeCensus(edges, ThreadRef{}, 16)
	var got []string
	for _, row := range rows {
		got = append(got, fmt.Sprintf("%s×%d", row.Waker.Comm, row.Count))
	}
	want := "wakerZ×2,wakerC×1,wakerD×1,wakerE×1"
	if strings.Join(got, ",") != want {
		t.Fatalf("census order must be count desc + typed tie keys:\n got %s\nwant %s", strings.Join(got, ","), want)
	}
}

// TestBuildWakeupEdgeCensusPairCapOverflow — the pair cap trims the LISTED
// rows only; the overflow disclosure carries the beyond-cap pairs AND their
// deduplicated edge total (never derived from the capped rows).
func TestBuildWakeupEdgeCensusPairCapOverflow(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	var edges []WakeupEdge
	// Pair 0 carries 3 edges, pairs 1..4 carry 1 edge each → 5 pairs, 7 edges.
	for i := 0; i < 3; i++ {
		edges = append(edges, censusTestEdge(ThreadRef{Comm: "waker0", PID: 200}, target, 5.001+float64(i)*0.001, 10+i))
	}
	for i := 1; i < 5; i++ {
		edges = append(edges, censusTestEdge(ThreadRef{Comm: fmt.Sprintf("waker%d", i), PID: 200 + i}, target, 5.100+float64(i)*0.001, 40+i))
	}
	rows, overflowPairs, overflowEdges := buildWakeupEdgeCensus(edges, ThreadRef{}, 2)
	if len(rows) != 2 {
		t.Fatalf("pair cap 2 must list 2 rows, got %d", len(rows))
	}
	if rows[0].Count != 3 {
		t.Fatalf("the dominant pair must survive the cap, got %+v", rows[0])
	}
	if overflowPairs != 3 || overflowEdges != 3 {
		t.Fatalf("overflow must disclose 3 pairs / 3 edges beyond the cap, got pairs=%d edges=%d", overflowPairs, overflowEdges)
	}
}

// TestBuildWakeupEdgeCensusTargetWakeeCapImmunity — 件5 同款: pairs that wake
// the chain TARGET never fall to the pair cap (donghu witness: the direct
// tppmgr-idle/hilogcat/binder wakers of CompThread_0-2955 — ×1 ties with late
// lexicographic keys — fell to overflow while chain-intermediate pairs stayed
// listed). Non-target pairs are evicted first; when target pairs alone exceed
// the cap they ALL stay; the eviction folds into the explicit overflow.
func TestBuildWakeupEdgeCensusTargetWakeeCapImmunity(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	other := ThreadRef{Comm: "mid", PID: 900}
	var edges []WakeupEdge
	// Three ×2 chain-intermediate pairs — they sort FIRST on count.
	for i := 0; i < 3; i++ {
		waker := ThreadRef{Comm: fmt.Sprintf("aaa-mid%d", i), PID: 300 + i}
		edges = append(edges,
			censusTestEdge(waker, other, 5.001+float64(i)*0.01, 10+2*i),
			censusTestEdge(waker, other, 5.002+float64(i)*0.01, 11+2*i),
		)
	}
	// Two ×1 target-wakee pairs with LATE lexicographic keys — a blind trim
	// evicts exactly these.
	edges = append(edges,
		censusTestEdge(ThreadRef{Comm: "zzz-waker1", PID: 700}, target, 5.100, 90),
		censusTestEdge(ThreadRef{Comm: "zzz-waker2", PID: 701}, target, 5.200, 95),
	)
	rows, overflowPairs, overflowEdges := buildWakeupEdgeCensus(edges, target, 3)
	if len(rows) != 3 {
		t.Fatalf("pair cap 3 must list 3 rows, got %+v", rows)
	}
	targetPairs := 0
	for _, row := range rows {
		if row.Wakee.PID == target.PID {
			targetPairs++
		}
	}
	if targetPairs != 2 {
		t.Fatalf("BOTH target-wakee pairs must survive the cap, got %d in %+v", targetPairs, rows)
	}
	// Two ×2 intermediate pairs (4 edges) were evicted in their favor.
	if overflowPairs != 2 || overflowEdges != 4 {
		t.Fatalf("eviction must fold into the explicit overflow (2 pairs / 4 edges), got pairs=%d edges=%d", overflowPairs, overflowEdges)
	}
	// Target pairs alone past the cap: ALL stay.
	edges = edges[:0]
	for i := 0; i < 4; i++ {
		edges = append(edges, censusTestEdge(ThreadRef{Comm: fmt.Sprintf("w%d", i), PID: 500 + i}, target, 5.3+float64(i)*0.01, 200+i))
	}
	rows, overflowPairs, _ = buildWakeupEdgeCensus(edges, target, 3)
	if len(rows) != 4 || overflowPairs != 0 {
		t.Fatalf("target-wakee pairs alone past the cap must ALL stay, got %d rows overflowPairs=%d", len(rows), overflowPairs)
	}
}

// wakeupCensusChainTrace — production-minting fixture (产线实铸形纪律): the
// engine parses this trace text and BuildWakeupChain itself mints the census.
// Target app-100 has TWO qualifying sleep segments woken by wakerA-200, and
// wakerA's own sleep in the first branch resolves one hop deeper to
// wakerB-300 — so the full edge set folds to (wakerA→app)×2 + (wakerB→wakerA)×1.
const wakeupCensusChainTrace = `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
       wakerB-300 (300) [003] .... 4.995000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=wakerB next_pid=300 next_prio=30
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       wakerA-200 (200) [002] .... 5.001000: sched_switch: prev_comm=wakerA prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       wakerB-300 (300) [003] .... 5.008000: sched_wakeup: comm=wakerA pid=200 prio=40 target_cpu=002
       wakerA-200 (200) [002] .... 5.008500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=wakerA next_pid=200 next_prio=40
       wakerA-200 (200) [002] .... 5.010000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.010200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       wakerA-200 (200) [002] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.019200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
       wakerB-300 (300) [003] .... 5.019500: sched_switch: prev_comm=wakerB prev_pid=300 prev_prio=30 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
`

// TestBuildWakeupChainMintsEdgeCensus — end-to-end engine pin on a
// production-minted chain: the census equals a whole-edge-set fold (pair
// counts, first/last ts, zero overflow) and re-running the build yields a
// byte-stable order (DET).
func TestBuildWakeupChainMintsEdgeCensus(t *testing.T) {
	idx := buildTraceIndex(t, "wake_census_chain.systrace", wakeupCensusChainTrace)
	query := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	chain := BuildWakeupChain(idx, query)
	if len(chain.Edges) != 3 {
		t.Fatalf("fixture drifted: expected 3 engine edges, got %+v", chain.Edges)
	}
	if chain.WakeupEdgeCensusOverflowPairs != 0 || chain.WakeupEdgeCensusOverflowEdges != 0 {
		t.Fatalf("2 pairs never overflow the pair cap, got pairs=%d edges=%d",
			chain.WakeupEdgeCensusOverflowPairs, chain.WakeupEdgeCensusOverflowEdges)
	}
	if len(chain.WakeupEdgeCensus) != 2 {
		t.Fatalf("expected 2 census pairs, got %+v", chain.WakeupEdgeCensus)
	}
	first := chain.WakeupEdgeCensus[0]
	if first.Waker.PID != 200 || first.Wakee.PID != 100 || first.Count != 2 ||
		first.FirstTs != 5.010 || first.LastTs != 5.019 {
		t.Fatalf("wakerA→app census must read count=2 first=5.010 last=5.019, got %+v", first)
	}
	second := chain.WakeupEdgeCensus[1]
	if second.Waker.PID != 300 || second.Wakee.PID != 200 || second.Count != 1 ||
		second.FirstTs != 5.008 || second.LastTs != 5.008 {
		t.Fatalf("wakerB→wakerA census must read count=1 at 5.008, got %+v", second)
	}
	// Whole-set identity: the census total equals the deduplicated edge count
	// — a census re-sourced from any truncated edge view breaks this.
	total := 0
	for _, row := range chain.WakeupEdgeCensus {
		total += row.Count
	}
	if total != len(chain.Edges) {
		t.Fatalf("census total %d must equal the full edge set size %d", total, len(chain.Edges))
	}
	// DET: a second build yields the identical census.
	again := BuildWakeupChain(idx, query)
	if fmt.Sprintf("%+v", again.WakeupEdgeCensus) != fmt.Sprintf("%+v", chain.WakeupEdgeCensus) {
		t.Fatalf("census must be deterministic across builds:\n first %+v\nsecond %+v",
			chain.WakeupEdgeCensus, again.WakeupEdgeCensus)
	}
}
