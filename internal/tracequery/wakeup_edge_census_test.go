package tracequery

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// wakeup_edge_census_test.go — WAKE-CENSUS engine pins, re-derived for the
// WAKE-CENSUS-D 2A window-total source (§29.58.4 立案, RANK-U Stage 1 commit
// B, 2026-07-13; first batch §29.58/§29.62). The census counts raw
// sched_wakeup rows for the chain-thread wakee set DIRECTLY from the indexed
// inventory — never from res.Edges — with deterministic order, explicit
// overflow and the typed exit-state split.
//
// Fixture red line (§29.53 产线实铸形): every counting pin parses trace text
// through the production parser and lets BuildWakeupChain mint the census.
//
// MUTATION self-checks:
//   - re-sourcing buildWakeupEdgeCensus from res.Edges reds
//     TestWakeupEdgeCensusCountsDExitWakeups (the D-exit pair has ZERO edges)
//     and TestWakeupEdgeCensusCountsOffPathSleepExit (the below-floor sleep
//     segment has no edge either);
//   - dropping the exit-split classifier (or splitting rows instead of
//     columns) reds the 双加恒等式 assertions (sleep+d+other == count on ONE
//     pair row);
//   - replacing the typed tie keys with map/first-appearance order reds
//     TestBuildWakeupEdgeCensusDeterministicOrder;
//   - dropping the overflow accounting (or deriving it from the capped rows)
//     reds TestBuildWakeupEdgeCensusPairCapOverflow;
//   - dropping the sched_waking fallback reds
//     TestWakeupEdgeCensusSchedWakingFallback (window-total zero beside real
//     edges).

// wakeupCensusChainTrace — production-minting fixture: the engine parses this
// trace text and BuildWakeupChain itself mints the census. Target app-100 has
// TWO qualifying sleep segments woken by wakerA-200, and wakerA's own sleep in
// the first branch resolves one hop deeper to wakerB-300 — so the raw
// inventory for the wakee set {app, wakerA, wakerB} holds (wakerA→app)×2 +
// (wakerB→wakerA)×1, every one an S exit.
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
// production-minted chain: the census equals the window-total raw wakeup
// count for the chain-thread wakee set (pair counts, first/last ts, S-exit
// split, zero overflow) and re-running the build yields a byte-stable order
// (DET).
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
	if first.SleepExitCount != 2 || first.DExitCount != 0 || first.OtherExitCount != 0 {
		t.Fatalf("both app wakeups end S sleeps (双加恒等式 2+0+0=2), got %+v", first)
	}
	second := chain.WakeupEdgeCensus[1]
	if second.Waker.PID != 300 || second.Wakee.PID != 200 || second.Count != 1 ||
		second.FirstTs != 5.008 || second.LastTs != 5.008 {
		t.Fatalf("wakerB→wakerA census must read count=1 at 5.008, got %+v", second)
	}
	if second.SleepExitCount != 1 || second.DExitCount != 0 || second.OtherExitCount != 0 {
		t.Fatalf("wakerA's wakeup ends an S sleep, got %+v", second)
	}
	// Window-total identity: the census total equals the raw in-window
	// sched_wakeup row count for the wakee population (here every raw row
	// wakes a chain thread) — a census re-sourced from any truncated or
	// structurally partial view breaks this.
	total := 0
	for _, row := range chain.WakeupEdgeCensus {
		total += row.Count
		if row.SleepExitCount+row.DExitCount+row.OtherExitCount != row.Count {
			t.Fatalf("exit split must partition the count exactly: %+v", row)
		}
	}
	raw := 0
	for _, ev := range idx.Events {
		if ev.Type == EventSchedWakeup && ev.Ts >= 5.0 && ev.Ts <= 5.020 {
			raw++
		}
	}
	if total != raw {
		t.Fatalf("census total %d must equal the window-total raw row count %d", total, raw)
	}
	// DET: a second build yields the identical census.
	again := BuildWakeupChain(idx, query)
	if fmt.Sprintf("%+v", again.WakeupEdgeCensus) != fmt.Sprintf("%+v", chain.WakeupEdgeCensus) {
		t.Fatalf("census must be deterministic across builds:\n first %+v\nsecond %+v",
			chain.WakeupEdgeCensus, again.WakeupEdgeCensus)
	}
}

// wakeupCensusDExitTrace — the §29.58.4 donghu form (gpu-token ×12 shape at
// unit scale): the target waits UNINTERRUPTIBLY (prev_state=D) and is woken
// by gpu-token-2931; the expandChain D arm reads blocked_reason and mints NO
// edge, so pre-2A the pair was structurally invisible to the census.
const wakeupCensusDExitTrace = `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
   gpu-token-2931 (2931) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.009000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=D ==> next_comm=idle/1 next_pid=0 next_prio=120
   gpu-token-2931 (2931) [002] .... 5.015000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.015200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

// TestWakeupEdgeCensusCountsDExitWakeups is the §29.58.4 closure pin: D-exit
// wakeups enter the census with the typed d_exit split even though the
// engine's edge set carries ZERO edges for the pair (the D/IO arm never mints
// edges — blocked_reason keeps the causal lane; the census is measurement
// arithmetic). Re-sourcing the census from res.Edges reds this immediately.
func TestWakeupEdgeCensusCountsDExitWakeups(t *testing.T) {
	idx := buildTraceIndex(t, "wake_census_dexit.systrace", wakeupCensusDExitTrace)
	query := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.016, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	chain := BuildWakeupChain(idx, query)
	for _, edge := range chain.Edges {
		if edge.Waker.PID == 2931 {
			t.Fatalf("fixture drifted: the D arm must not mint gpu-token edges (拒铸边 §3.1): %+v", edge)
		}
	}
	var pair *WakeupEdgeCensusPair
	for i := range chain.WakeupEdgeCensus {
		if chain.WakeupEdgeCensus[i].Waker.PID == 2931 && chain.WakeupEdgeCensus[i].Wakee.PID == 100 {
			pair = &chain.WakeupEdgeCensus[i]
			break
		}
	}
	if pair == nil {
		t.Fatalf("D-exit wakeups must reach the census without any edge (§29.58.4): %+v", chain.WakeupEdgeCensus)
	}
	if pair.Count != 2 || pair.DExitCount != 2 || pair.SleepExitCount != 0 || pair.OtherExitCount != 0 {
		t.Fatalf("gpu-token→app must count ×2 with d_exit=2 (双加恒等式), got %+v", pair)
	}
	if pair.FirstTs != 5.006 || pair.LastTs != 5.015 {
		t.Fatalf("first/last ts must bound the counted raw rows: %+v", pair)
	}
}

// wakeupCensusOffPathTrace — the off-path S-exit shape (§29.58.4 顺带灭): the
// target's FIRST sleep (5.000→5.006, 6ms) is the expanded branch; the second
// sleep (5.008→5.0084, 0.4ms) sits below the MinDurationMs floor, so the
// expansion never visits it and no edge exists for its wakeup by helper-400 —
// pre-2A that raw row leaked out of the census.
const wakeupCensusOffPathTrace = `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       wakerA-200 (200) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.008000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       helper-400 (400) [003] .... 5.008400: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.008600: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func TestWakeupEdgeCensusCountsOffPathSleepExit(t *testing.T) {
	idx := buildTraceIndex(t, "wake_census_offpath.systrace", wakeupCensusOffPathTrace)
	query := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.009, MaxDepth: 4, MinDurationMs: 1.0, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	chain := BuildWakeupChain(idx, query)
	for _, edge := range chain.Edges {
		if edge.Waker.PID == 400 {
			t.Fatalf("fixture drifted: the below-floor sleep must not be expanded into an edge: %+v", edge)
		}
	}
	var helper *WakeupEdgeCensusPair
	for i := range chain.WakeupEdgeCensus {
		if chain.WakeupEdgeCensus[i].Waker.PID == 400 {
			helper = &chain.WakeupEdgeCensus[i]
			break
		}
	}
	if helper == nil {
		t.Fatalf("an off-path S-exit wakeup must still count (window-total caliber): %+v", chain.WakeupEdgeCensus)
	}
	if helper.Count != 1 || helper.SleepExitCount != 1 {
		t.Fatalf("helper→app must count ×1 sleep_exit=1, got %+v", helper)
	}
}

// censusScanChain builds a minimal ChainResult population for direct
// buildWakeupEdgeCensus scans (cap/order pins where the interesting geometry
// is the PAIR SET, not the chain expansion).
func censusScanChain(target ThreadRef, window TimeWindow, nodes ...ThreadRef) *ChainResult {
	res := &ChainResult{Target: target, Window: window}
	res.Nodes = append(res.Nodes, ChainNode{Thread: target, Window: window})
	for _, node := range nodes {
		res.Nodes = append(res.Nodes, ChainNode{Thread: node, Window: window})
	}
	return res
}

// TestBuildWakeupEdgeCensusDeterministicOrder — count desc, then the typed
// tie chain (waker comm, waker pid, wakee comm, wakee pid). Tie pairs are
// inserted in REVERSE lexicographic order so first-appearance order and the
// typed key disagree — deleting the tie key cannot stay green (复核 F2 教训:
// aligned orders make a tie-key pin vacuous).
func TestBuildWakeupEdgeCensusDeterministicOrder(t *testing.T) {
	var b strings.Builder
	b.WriteString("        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	b.WriteString("        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n")
	// Reverse-lex insertion: wakerE, wakerD, wakerC — all ×1 ties.
	for i, comm := range []string{"wakerE", "wakerD", "wakerC"} {
		fmt.Fprintf(&b, "       %s-%d (%d) [002] .... 5.00%d000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n", comm, 500+i, 500+i, i+1)
	}
	// The dominant ×2 pair arrives LAST — count desc must still front it.
	b.WriteString("       wakerZ-900 (900) [002] .... 5.010000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n")
	b.WriteString("       wakerZ-900 (900) [002] .... 5.011000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n")
	idx := buildTraceIndex(t, "wake_census_order.systrace", b.String())
	res := censusScanChain(ThreadRef{Comm: "app", PID: 100}, TimeWindow{StartTs: 4.99, EndTs: 5.02})
	q := Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.02}
	rows, _, _, _ := buildWakeupEdgeCensus(idx, nil, q, res, 16)
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
// raw-row total (never derived from the capped rows).
func TestBuildWakeupEdgeCensusPairCapOverflow(t *testing.T) {
	var b strings.Builder
	b.WriteString("        mid-900 (900) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=mid next_pid=900 next_prio=52\n")
	// Pair 0 carries 3 raw rows, pairs 1..4 carry 1 each → 5 pairs, 7 rows.
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&b, "       waker0-200 (200) [002] .... 5.00%d000: sched_wakeup: comm=mid pid=900 prio=52 target_cpu=001\n", i+1)
	}
	for i := 1; i < 5; i++ {
		fmt.Fprintf(&b, "       waker%d-%d (%d) [002] .... 5.01%d000: sched_wakeup: comm=mid pid=900 prio=52 target_cpu=001\n", i, 200+i, 200+i, i)
	}
	idx := buildTraceIndex(t, "wake_census_capgeneric.systrace", b.String())
	// The counted wakee is a chain NODE, not the target — no immunity engages.
	res := censusScanChain(ThreadRef{Comm: "app", PID: 100}, TimeWindow{StartTs: 4.99, EndTs: 5.02}, ThreadRef{Comm: "mid", PID: 900})
	q := Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.02}
	rows, overflowPairs, overflowEdges, _ := buildWakeupEdgeCensus(idx, nil, q, res, 2)
	if len(rows) != 2 {
		t.Fatalf("pair cap 2 must list 2 rows, got %d", len(rows))
	}
	if rows[0].Count != 3 {
		t.Fatalf("the dominant pair must survive the cap, got %+v", rows[0])
	}
	if overflowPairs != 3 || overflowEdges != 3 {
		t.Fatalf("overflow must disclose 3 pairs / 3 rows beyond the cap, got pairs=%d edges=%d", overflowPairs, overflowEdges)
	}
}

// TestBuildWakeupEdgeCensusTargetWakeeCapImmunity — 件5 同款: pairs that wake
// the chain TARGET never fall to the pair cap (donghu witness: the direct
// tppmgr-idle/hilogcat/binder wakers of CompThread_0-2955 — ×1 ties with late
// lexicographic keys — fell to overflow while chain-intermediate pairs stayed
// listed). Non-target pairs are evicted first; when target pairs alone exceed
// the cap they ALL stay; the eviction folds into the explicit overflow.
func TestBuildWakeupEdgeCensusTargetWakeeCapImmunity(t *testing.T) {
	var b strings.Builder
	b.WriteString("        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n")
	// Three ×2 chain-intermediate pairs — they sort FIRST on count.
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&b, "       aaa-mid%d-%d (%d) [002] .... 5.0%d1000: sched_wakeup: comm=mid pid=900 prio=52 target_cpu=001\n", i, 300+i, 300+i, i)
		fmt.Fprintf(&b, "       aaa-mid%d-%d (%d) [002] .... 5.0%d2000: sched_wakeup: comm=mid pid=900 prio=52 target_cpu=001\n", i, 300+i, 300+i, i)
	}
	// Two ×1 target-wakee pairs with LATE lexicographic keys — a blind trim
	// evicts exactly these.
	b.WriteString("       zzz-waker1-700 (700) [003] .... 5.040000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n")
	b.WriteString("       zzz-waker2-701 (701) [003] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n")
	idx := buildTraceIndex(t, "wake_census_cap.systrace", b.String())
	target := ThreadRef{Comm: "app", PID: 100}
	res := censusScanChain(target, TimeWindow{StartTs: 4.99, EndTs: 5.06}, ThreadRef{Comm: "mid", PID: 900})
	q := Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.06}
	rows, overflowPairs, overflowEdges, _ := buildWakeupEdgeCensus(idx, nil, q, res, 3)
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
	// Two ×2 intermediate pairs (4 raw rows) were evicted in their favor.
	if overflowPairs != 2 || overflowEdges != 4 {
		t.Fatalf("eviction must fold into the explicit overflow (2 pairs / 4 rows), got pairs=%d edges=%d", overflowPairs, overflowEdges)
	}
}

// wakeupCensusWakingOnlyTrace — a sched_waking-only capture: the edge lane
// accepts sched_waking (findWakeup is event-type inclusive), so a
// sched_wakeup-only census would publish window-total ZERO beside a real
// minted edge — the typed single-ruler fallback closes that contradiction.
const wakeupCensusWakingOnlyTrace = `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       wakerA-200 (200) [002] .... 5.006000: sched_waking: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func TestWakeupEdgeCensusSchedWakingFallback(t *testing.T) {
	idx := buildTraceIndex(t, "wake_census_waking.systrace", wakeupCensusWakingOnlyTrace)
	query := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.007, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	chain := BuildWakeupChain(idx, query)
	if len(chain.Edges) != 1 {
		t.Fatalf("fixture drifted: the sched_waking row must still mint the edge: %+v", chain.Edges)
	}
	if len(chain.WakeupEdgeCensus) != 1 || chain.WakeupEdgeCensus[0].Count != 1 ||
		chain.WakeupEdgeCensus[0].SleepExitCount != 1 {
		t.Fatalf("the sched_waking fallback must count the wake (never zero beside a real edge): %+v", chain.WakeupEdgeCensus)
	}
	if !strings.Contains(strings.Join(chain.Caveats, "\n"), "wakeup_edge_census_source=sched_waking") {
		t.Fatalf("the fallback source must be disclosed: %q", chain.Caveats)
	}
}

// TestWakeupEdgeCensusDonghuWakerWitness — the §29.58.4 REAL-TRACE witness
// (donghu.ftrace, the in-repo customer capture; window = the capture's own
// 13762.791708..13763.024898, target CompThread_0-2955): the raw window holds
// EXACTLY 29 sched_wakeup rows waking the target — 12 by gpu-token-id4-2931
// (the D-exit pairs the pre-2A edge-fold census could never see: the engine's
// edge set carries zero gpu-token pairs), 7 by RSUniRenderThre-2188, 2 by
// tppmgr-idle-3-273 and 8 ×1 wakers. R3B identity: the census equals an
// independent raw re-count from the indexed inventory, pair for pair.
// Exit split honesty: 11 of the 12 gpu-token wakeups end in-window-provable
// D segments; the first (13762.793064, the capture's very first row) has no
// in-window timeline coverage and lands in the honest other/unclassified
// bucket (absence never guesses — the kernel's blocked_reason row still
// carries the D causal fact on its own lane).
func TestWakeupEdgeCensusDonghuWakerWitness(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	query := Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	chain := BuildWakeupChain(idx, query)
	// The structural-absence proof: the D arm mints no gpu-token edges.
	for _, edge := range chain.Edges {
		if edge.Waker.Comm == "gpu-token-id4" {
			t.Fatalf("fixture drifted: the engine edge set must carry no gpu-token pair (§29.58.4): %+v", edge)
		}
	}
	// R3B: independent raw re-count of the target's wakeups from the indexed
	// inventory — the census must match pair for pair.
	rawByWaker := map[string]int{}
	rawTotal := 0
	// 件6 (修复轮): the recount is deliberately NOT filtered through the
	// production incarnation predicate (去自指) — the capture holds ZERO
	// sched_wakeup_new rows (raw grep verified), so the raw truth needs no
	// exclusion here; the exclusion behavior itself is pinned by the
	// discriminating fixture below (TestWakeupEdgeCensusExcludesWakeupNew).
	for _, ev := range idx.Events {
		if ev.Type != EventSchedWakeup {
			continue
		}
		if ev.WakeePID != 2955 || ev.Ts < query.TimeStart || ev.Ts > query.TimeEnd {
			continue
		}
		rawByWaker[fmt.Sprintf("%s-%d", ev.Comm, ev.PID)]++
		rawTotal++
	}
	if rawTotal != 29 || rawByWaker["gpu-token-id4-2931"] != 12 || rawByWaker["RSUniRenderThre-2188"] != 7 {
		t.Fatalf("real-trace drifted: raw window must hold 29 target wakeups (12 gpu-token / 7 RSUniRender), got total=%d byWaker=%v", rawTotal, rawByWaker)
	}
	censusByWaker := map[string]int{}
	censusTotal := 0
	var gpuToken *WakeupEdgeCensusPair
	for i := range chain.WakeupEdgeCensus {
		pair := &chain.WakeupEdgeCensus[i]
		if pair.Wakee.PID != 2955 {
			continue
		}
		key := fmt.Sprintf("%s-%d", pair.Waker.Comm, pair.Waker.PID)
		censusByWaker[key] = pair.Count
		censusTotal += pair.Count
		if key == "gpu-token-id4-2931" {
			gpuToken = pair
		}
		if pair.SleepExitCount+pair.DExitCount+pair.OtherExitCount != pair.Count {
			t.Fatalf("exit split must partition every pair exactly: %+v", pair)
		}
	}
	if censusTotal != rawTotal || len(censusByWaker) != len(rawByWaker) {
		t.Fatalf("R3B: census must equal the raw re-count (total %d vs %d, pairs %d vs %d)",
			censusTotal, rawTotal, len(censusByWaker), len(rawByWaker))
	}
	for waker, raw := range rawByWaker {
		if censusByWaker[waker] != raw {
			t.Fatalf("R3B: pair %s census=%d raw=%d", waker, censusByWaker[waker], raw)
		}
	}
	if gpuToken == nil || gpuToken.Count != 12 || gpuToken.DExitCount != 11 ||
		gpuToken.OtherExitCount != 1 || gpuToken.SleepExitCount != 0 {
		t.Fatalf("gpu-token D-exit witness drifted: %+v", gpuToken)
	}
	// Target-wakee cap immunity on the real shape: ALL wakers of the target
	// keep their census seats (the ×1 tie pairs are exactly the donghu rows a
	// blind trim evicted pre-件5).
	if len(censusByWaker) != 11 {
		t.Fatalf("all 11 target-waker pairs must survive the pair cap, got %d: %v", len(censusByWaker), censusByWaker)
	}
	// DET: a second build yields the identical census (复放 2 趟).
	again := BuildWakeupChain(idx, query)
	if fmt.Sprintf("%+v", again.WakeupEdgeCensus) != fmt.Sprintf("%+v", chain.WakeupEdgeCensus) {
		t.Fatalf("census must be deterministic across builds")
	}
}

// wakeupCensusWakeupNewTrace — 件6 (修复轮, 复核 F6 2026-07-13): the
// sched_wakeup_new DISCRIMINATING fixture. A creation row reuses the wakee's
// numeric TID inside the window; the census must count ONLY the real
// sched_wakeup (expected count hardcoded to 1 — the pin never re-implements
// the exclusion predicate it guards).
const wakeupCensusWakeupNewTrace = `        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      spawner-600 ( 600) [003] .... 5.002000: sched_wakeup_new: comm=app pid=100 prio=52 target_cpu=001
       wakerA-200 ( 200) [002] .... 5.006000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.006200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func TestWakeupEdgeCensusExcludesWakeupNew(t *testing.T) {
	idx := buildTraceIndex(t, "wake_census_wakeupnew.systrace", wakeupCensusWakeupNewTrace)
	// Fixture-activation fail-loud: the creation row must actually be in the
	// indexed window inventory (a parser drop would make this pin vacuous).
	sawNew := false
	for _, ev := range idx.Events {
		if ev.Type == EventSchedWakeup && ev.Name == "sched_wakeup_new" && ev.WakeePID == 100 {
			sawNew = true
			break
		}
	}
	if !sawNew {
		t.Fatalf("fixture drifted: the sched_wakeup_new row must reach the index")
	}
	// The full chain build fail-closes on this shape (a mid-window creation
	// row IS a thread-incarnation conflict — the honest whole-result refusal,
	// covered by the incarnation guard's own pins). The census exclusion arm
	// is pinned at the builder grain over the SAME parsed index.
	res := censusScanChain(ThreadRef{Comm: "app", PID: 100}, TimeWindow{StartTs: 5.0, EndTs: 5.007})
	q := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.007}
	rows, overflowPairs, _, _ := buildWakeupEdgeCensus(idx, nil, q, res, 16)
	if overflowPairs != 0 || len(rows) != 1 {
		t.Fatalf("exactly ONE census pair expected (the real wakeup), got %+v", rows)
	}
	if rows[0].Waker.PID != 200 || rows[0].Count != 1 || rows[0].FirstTs != 5.006 {
		t.Fatalf("the census must count only the real sched_wakeup (wakerA ×1 at 5.006): %+v", rows[0])
	}
	for _, p := range rows {
		if p.Waker.PID == 600 {
			t.Fatalf("a sched_wakeup_new creation row must never count as a wakeup: %+v", p)
		}
	}
}

// TestWakeupExitStateBucketClassifier unit-pins the typed exit classifier:
// containing interval (StartTs < ts <= EndTs) wins; a boundary ts belongs to
// the interval it ENDS (the exited state, never the post-wake segment); gaps
// fall to the nearest preceding interval; no interval → other (absence never
// guesses).
func TestWakeupExitStateBucketClassifier(t *testing.T) {
	intervals := []Interval{
		{State: StateRunning, StartTs: 5.000, EndTs: 5.002},
		{State: StateDSleep, StartTs: 5.002, EndTs: 5.006},
		{State: StateRunning, StartTs: 5.006, EndTs: 5.008},
		{State: StateSSleep, StartTs: 5.009, EndTs: 5.012},
	}
	for _, tc := range []struct {
		ts   float64
		want string
	}{
		{5.004, "d"},      // inside the D interval
		{5.006, "d"},      // boundary: the D interval ENDS here — exited state
		{5.010, "sleep"},  // inside the S interval
		{5.0085, "other"}, // gap: nearest preceding interval is running
		{5.007, "other"},  // running is never a sleep/D exit
		{4.999, "other"},  // before the first interval: unclassifiable
	} {
		if got := wakeupExitStateBucket(intervals, tc.ts); got != tc.want {
			t.Fatalf("ts=%.4f: got %q want %q", tc.ts, got, tc.want)
		}
	}
	if got := wakeupExitStateBucket(nil, 5.0); got != "other" {
		t.Fatalf("missing timeline must fall to the honest other bucket, got %q", got)
	}
}
