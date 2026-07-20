package tracequery

// rank_chain_anchor_rspa_rnb_test.go — RNB-1 (§29.88 R2/R4 user rulings,
// 2026-07-14) engine pins: the chain-credential bipartition holds across
// EVERY runnable topology, including the two INV-D fail-open escapes that
// reproduced the customer runnable.txt W1/W2 disease at HEAD:
//
//   S5 — customer E8/E6 topology: all major runnable fragments on ONE cpu
//        (front fragment inside the dependency window) plus one sub-visible
//        dust segment on another cpu. The dust split the census into two
//        groups, the enrich overlap arm re-laned the dust group adjacent, and
//        the former per-seat "value == pid-census-full" identity gate failed
//        the WHOLE pid open — the main window seat kept its FULL
//        multi-fragment value on the chain tier (深度未解析·全额, E8 26.392
//        vs chain-proven 8.606). Fixed by the T1 census-GROUP ledger stamp.
//   S6 — customer E22 合计(共N段) form: two on-chain census groups merged
//        into one family seat + one adjacent dust group; the merged Σ ≠ pid
//        census-full → same escape, same fix (the family fold Σ's the group
//        stamps under the sum_disjoint caliber).
//
// S1/S2/S3 are the control shapes (single-group multi-segment, multi-CPU
// all-on-chain, two occurrences + tail) that already bisected correctly at
// HEAD — pinned so the group-ledger rewrite can never regress them.

import (
	"math"
	"strings"
	"testing"
)

// rspaRNBQuery is the shared query for the synthetic shapes.
func rspaRNBQuery() Query {
	return Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16}
}

// S1 control — single wakeup, three runnable segments on one cpu, only the
// front segment inside the dependency window.
func rspaRNBTraceS1(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "rspa_rnb_s1.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.028000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.029000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.030000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.031000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      other-300 (300) [003] .... 1.040000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.090000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.091000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.100000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.120000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.121000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.190000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
}

// S2 — multi-CPU runnable segments (customer E22 member roster shape), only
// the front segment carries the wakeup edge.
func rspaRNBTraceS2(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "rspa_rnb_s2.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.028000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.029000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.030000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.031000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      other-300 (300) [003] .... 1.040000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=004
     worker-200 (100) [004] .... 1.090000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [004] .... 1.091000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.100000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=005
     worker-200 (100) [005] .... 1.120000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [005] .... 1.121000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/5 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.190000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
}

// S3 — two wakeup occurrences (target sleeps twice, both woken by worker)
// plus one edge-less tail segment: multi-branch chain over the same waker.
func rspaRNBTraceS3(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "rspa_rnb_s3.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.028000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.029000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.030000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.031000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.050000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.051000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.070000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.071000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.072000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.073000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      other-300 (300) [003] .... 1.090000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.140000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.141000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.190000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
}

// S5 — CUSTOMER TOPOLOGY (E8/E6 shape): three runnable segments on cpu 2
// (27 + 50 + 20 ms; only the front one inside the [1.001, 1.028] dependency
// window) plus a 0.5ms runnable dust segment on cpu 6.
func rspaRNBTraceS5(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "rspa_rnb_s5.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.028000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.029000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.030000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.031000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      other-300 (300) [003] .... 1.040000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.090000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.091000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.100000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.120000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.121000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.150000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=006
     worker-200 (100) [006] .... 1.150500: sched_switch: prev_comm=idle/6 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [006] .... 1.151000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/6 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.190000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
}

// S6 — merged 合计(共N段) variant (customer E22 form): two on-chain census
// groups (cpu 2: 9ms, cpu 4: 16 + 50 ms) fold into one family seat, plus an
// adjacent dust group on cpu 6.
func rspaRNBTraceS6(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "rspa_rnb_s6.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.010000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.011000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.012000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=004
     worker-200 (100) [004] .... 1.028000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [004] .... 1.028500: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [004] .... 1.029000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.031000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      other-300 (300) [003] .... 1.040000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=004
     worker-200 (100) [004] .... 1.090000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [004] .... 1.091000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.150000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=006
     worker-200 (100) [006] .... 1.150500: sched_switch: prev_comm=idle/6 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [006] .... 1.151000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/6 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.190000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
}

// TestRSPARNBBipartitionAcrossShapes — the §29.88 acceptance predicate (the
// INV-D production-scan predicate promoted to a pin): across EVERY shape, no
// on-chain window seat of the waker may publish more effective attribution
// than its typed anchor-window ceiling without carrying the bipartition
// decomposition — the 「链上·深度未解析·全额」 shape is extinct.
func TestRSPARNBBipartitionAcrossShapes(t *testing.T) {
	shapes := map[string]*Index{
		"S1": rspaRNBTraceS1(t),
		"S2": rspaRNBTraceS2(t),
		"S3": rspaRNBTraceS3(t),
		"S5": rspaRNBTraceS5(t),
		"S6": rspaRNBTraceS6(t),
	}
	for name, idx := range shapes {
		q := rspaRNBQuery()
		chain := BuildWakeupChain(idx, q)
		anchors := chainAnchorWindowsByPID(chain)
		var anchorCeiling float64
		for _, w := range anchors[200] {
			anchorCeiling += (w.EndTs - w.StartTs) * 1000
		}
		rank := BuildRootCauseRank(idx, q)
		for _, it := range rank.Items {
			if it.Thread.PID != 200 || it.AbsorbedByRankFamily {
				continue
			}
			if !rootCauseItemIsOnChain(it) || it.DominantState != string(StateRunnable) {
				continue
			}
			if !strings.HasPrefix(it.Source, "wakeup_chain") && it.ChainAnchorFullMs == 0 &&
				it.EffectiveImpactMs > anchorCeiling+rspaAnchorIdentityTolMs {
				t.Errorf("%s DISEASE: on-chain depth-unresolved window seat publishes %.3fms full (> anchor-window ceiling %.3fms): type=%s src=%s runnable=%.3f",
					name, it.EffectiveImpactMs, anchorCeiling, it.Type, it.Source, it.RunnableMs)
			}
		}
	}
}

// TestRSPARNBCustomerShapeS5 — the T1 root-fix witness (customer E8/E6
// topology): the chain member's window seat bisects on its census ledger
// stamp — the former pid-full identity gate failed exactly here and kept the
// full multi-fragment value on the chain tier (INV-D S5, HEAD FAIL 97.000 >
// 29.000).
//
// EVOLUTION RECORD (upstream 1cab900d runnable CPU continuity, merged
// 2026-07-14): chain-member runnable seats now mint as ONE thread-total
// aggregate (aggregateChainRunnableCensusByThread Σ's DurationMs AND the
// anchoredMs stamp across the pid's census groups), so the seat account is
// the pid census full — 97.5 incl. the 0.5 dust group — and the bipartition
// reads 97.5 = 27 anchored + 70.5 remainder. The mint-time ledger stamp
// stays the migration authority (the mixed-lane mint-fork escape is closed
// at the source; the fold-Σ arm still guards re-folded forms).
func TestRSPARNBCustomerShapeS5(t *testing.T) {
	idx := rspaRNBTraceS5(t)
	rank := BuildRootCauseRank(idx, rspaRNBQuery())
	var remainder, chainSeat *RootCauseRankItem
	rows := append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...)
	for i := range rows {
		item := &rows[i]
		if item.Thread.PID != 200 {
			continue
		}
		if item.Type == "runnable_wait" && item.ChainAnchorRemainderSeat && item.ChainAnchorFullMs > 90 {
			remainder = item
		}
		// The chain lane's seat may wear the inversion-candidate token (the
		// chain lane's own gated algebra) — the identity that matters is the
		// wakeup_chain source + the on-chain runnable account.
		if strings.HasPrefix(item.Source, "wakeup_chain") && rootCauseItemIsOnChain(*item) && item.RunnableMs > 0 {
			chainSeat = item
		}
	}
	if remainder == nil {
		t.Fatalf("S5 main window seat must migrate to the ◇ remainder: %+v", rows)
	}
	if math.Abs(remainder.ChainAnchorFullMs-97.5) > 0.01 ||
		math.Abs(remainder.ChainAnchoredMs-27.0) > 0.01 ||
		math.Abs(remainder.RunnableMs-70.5) > 0.01 {
		t.Fatalf("S5 thread-account bipartition drifted (want 97.5 = 27 + 70.5): %+v", remainder)
	}
	if rootCauseItemIsOnChain(*remainder) {
		t.Fatalf("S5 remainder must ride the ◇ adjacent lane: %+v", remainder)
	}
	if chainSeat == nil || math.Abs(chainSeat.RunnableMs-27.0) > 0.01 {
		t.Fatalf("S5 chain seat must keep owning the anchored 27ms: %+v", chainSeat)
	}
	rspaAssertBoardBipartitionInvariants(t, rank)
}

// TestRSPARNBMergedShapeS6 — the Σ half of the T1 root fix (customer E22
// 合计 form; INV-D S6, HEAD FAIL 75.000 > 28.500).
//
// EVOLUTION RECORD (upstream 1cab900d, merged 2026-07-14): the thread-total
// aggregate mint Σ's the group stamps at the source — the seat account is
// the pid census full 75.5 (incl. the 0.5 dust group) and the bipartition
// reads 75.5 = 25 anchored + 50.5 remainder.
func TestRSPARNBMergedShapeS6(t *testing.T) {
	idx := rspaRNBTraceS6(t)
	rank := BuildRootCauseRank(idx, rspaRNBQuery())
	var remainder *RootCauseRankItem
	rows := append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...)
	for i := range rows {
		item := &rows[i]
		if item.Thread.PID != 200 || item.Type != "runnable_wait" || !item.ChainAnchorRemainderSeat {
			continue
		}
		if item.ChainAnchorFullMs > 70 {
			remainder = item
		}
	}
	if remainder == nil {
		t.Fatalf("S6 merged family seat must migrate to the ◇ remainder: %+v", rows)
	}
	if math.Abs(remainder.ChainAnchorFullMs-75.5) > 0.01 ||
		math.Abs(remainder.ChainAnchoredMs-25.0) > 0.01 ||
		math.Abs(remainder.RunnableMs-50.5) > 0.01 {
		t.Fatalf("S6 thread-account bipartition drifted (want 75.5 = 25 + 50.5): %+v", remainder)
	}
	rspaAssertBoardBipartitionInvariants(t, rank)
}

// TestRSPARNBOwnershipDivergentCaseAPrime — RNB-1 case A' unit pin (B-3): a
// chain seat is PRESENT but does not hold the census-anchored account (the
// donghu keva-1 Δ0.085 production shape) — the window seat still migrates to
// the ◇ remainder, the typed double-Σ disclosure rides the row, the summary
// speaks the divergence (never the additive ownership claim), and the chain
// seat itself stays untouched (链席自账不动).
func TestRSPARNBOwnershipDivergentCaseAPrime(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1.0, EndTs: 1.03}},
		},
		CausalImpacts: []WakeupCausalImpact{
			// Chain lane's own Σ = 4.5 ≠ census anchored 5.0 (identity broken).
			{Thread: ThreadRef{PID: 200}, ChainDepth: 1, RunnableMs: 4.5},
		},
	}
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		runnableCensus: map[string]ThreadDuration{
			"200|0": {Thread: ThreadRef{PID: 200}, DurationMs: 8.0, anchoredMs: 5.0},
		},
	}
	items := []RootCauseRankItem{
		{Type: "runnable_wait", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 8.0, ImpactMs: 8.0, CumulativeImpactMs: 8.0,
			Source: "window_stats", Confidence: 0.76,
			ledgerAnchorStamped: true, ledgerAnchoredRunnableMs: 5.0},
		{Type: "runnable_wait", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 4.5, ImpactMs: 4.5, CumulativeImpactMs: 4.5,
			Source: "wakeup_chain.causal_impacts", Confidence: 0.91},
	}
	items = reanchorOnChainStateSeats(chain, stats, items)
	window := items[0]
	if !window.ChainAnchorRemainderSeat || !window.ChainAnchorOwnershipDivergent {
		t.Fatalf("divergent pid's window seat must migrate with the typed divergence marker: %+v", window)
	}
	if math.Abs(window.RunnableMs-3.0) > rspaAnchorIdentityTolMs ||
		math.Abs(window.ChainAnchoredMs-5.0) > rspaAnchorIdentityTolMs ||
		math.Abs(window.ChainAnchorFullMs-8.0) > rspaAnchorIdentityTolMs {
		t.Fatalf("case A' bipartition drifted (want 8 = 5 + 3): %+v", window)
	}
	if math.Abs(window.ChainAnchorChainLaneMs-4.5) > rspaAnchorIdentityTolMs ||
		math.Abs(window.ChainAnchorCensusMs-5.0) > rspaAnchorIdentityTolMs {
		t.Fatalf("double-Σ disclosure must carry both accounts (chain 4.5 / census 5.0): %+v", window)
	}
	if !strings.Contains(window.Summary, "Anchored-ownership divergence") ||
		strings.Contains(window.Summary, "owned by the chain seat") {
		t.Fatalf("case A' summary must speak the divergence, never the ownership claim: %q", window.Summary)
	}
	chainSeat := items[1]
	if chainSeat.RunnableMs != 4.5 || chainSeat.ChainAnchorFullMs != 0 || chainSeat.ChainRelevance != "on_chain" {
		t.Fatalf("the chain seat's own account must stay untouched (链席自账不动): %+v", chainSeat)
	}
}

// TestRSPARNBSeatValueCheckGatesCaseA — B-3: presence alone never qualifies
// case A. A chain seat whose PUBLISHED value diverges from the census-anchored
// Σ (identity gate itself passing) takes the case-A' double-account
// disposition — the additive 「owned by the chain seat」 claim would be false.
func TestRSPARNBSeatValueCheckGatesCaseA(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1.0, EndTs: 1.03}},
		},
		CausalImpacts: []WakeupCausalImpact{
			// Chain Σ == census anchored (µs identity HOLDS)...
			{Thread: ThreadRef{PID: 200}, ChainDepth: 1, RunnableMs: 5.0},
		},
	}
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		runnableCensus: map[string]ThreadDuration{
			"200|0": {Thread: ThreadRef{PID: 200}, DurationMs: 8.0, anchoredMs: 5.0},
		},
	}
	items := []RootCauseRankItem{
		{Type: "runnable_wait", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 8.0, ImpactMs: 8.0, CumulativeImpactMs: 8.0,
			Source: "window_stats", Confidence: 0.76,
			ledgerAnchorStamped: true, ledgerAnchoredRunnableMs: 5.0},
		// ...but the PUBLISHED chain seat carries a different value (a gated /
		// capped / re-derived face) — the seat does not hold the anchored 5.0.
		{Type: "runnable_wait", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 3.2, ImpactMs: 3.2, CumulativeImpactMs: 3.2,
			Source: "wakeup_chain.causal_impacts", Confidence: 0.91},
	}
	items = reanchorOnChainStateSeats(chain, stats, items)
	window := items[0]
	if !window.ChainAnchorRemainderSeat || !window.ChainAnchorOwnershipDivergent {
		t.Fatalf("value-diverging chain seat must demote case A to case A': %+v", window)
	}
	if math.Abs(window.ChainAnchorChainLaneMs-3.2) > rspaAnchorIdentityTolMs {
		t.Fatalf("the disclosure must carry the seat's PUBLISHED Σ: %+v", window)
	}
}

// TestRSPARNBInversionRetypeLaneArm — R4 lane arm for the priority-inversion
// rewritten window seat (former structural escape: no switch arm at all): an
// unanchored share demotes the WHOLE seat to ◇ with every published value
// untouched (the gated eff is a displacement measurement — indivisible along
// the anchor boundary); a fully-anchored account keeps the chain lane
// byte-identically.
func TestRSPARNBInversionRetypeLaneArm(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1.0, EndTs: 1.03}},
		},
		CausalImpacts: []WakeupCausalImpact{
			{Thread: ThreadRef{PID: 200}, ChainDepth: 1, RunnableMs: 5.0},
		},
	}
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		runnableCensus: map[string]ThreadDuration{
			"200|0": {Thread: ThreadRef{PID: 200}, DurationMs: 8.0, anchoredMs: 5.0},
		},
	}
	partial := RootCauseRankItem{Type: "priority_inversion_runnable_wait", Thread: ThreadRef{PID: 200},
		Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
		RunnableMs: 8.0, ImpactMs: 8.0, CumulativeImpactMs: 8.0, EffectiveImpactMs: 2.0, GatedRunnableMs: 2.0,
		Score: 1.5, Source: "window_stats", Confidence: 0.76,
		ledgerAnchorStamped: true, ledgerAnchoredRunnableMs: 5.0}
	full := partial
	full.ledgerAnchoredRunnableMs = 8.0
	items := reanchorOnChainStateSeats(chain, stats, []RootCauseRankItem{partial, full})
	got := items[0]
	if !got.ChainCredentialLaneDemoted || got.ChainRelevance != "adjacent" || got.Causality != "adjacent_to_wakeup_chain" {
		t.Fatalf("partially anchored inversion seat must demote its lane: %+v", got)
	}
	if got.RunnableMs != 8.0 || got.EffectiveImpactMs != 2.0 || got.GatedRunnableMs != 2.0 || got.Score != 1.5 ||
		got.ChainAnchorFullMs != 0 || got.ChainAnchoredMs != 0 {
		t.Fatalf("lane demotion must leave every published value untouched (值零动): %+v", got)
	}
	if !strings.Contains(got.Summary, "no chain credential for the full account") {
		t.Fatalf("lane demotion must disclose: %q", got.Summary)
	}
	kept := items[1]
	if kept.ChainCredentialLaneDemoted || kept.ChainRelevance != "on_chain" {
		t.Fatalf("fully anchored inversion seat keeps the chain lane byte-identically: %+v", kept)
	}
}

// TestRSPARNBCPUAffinityLaneArm — R4 lane arm for cpu_affinity_or_cpuset
// (INV-C B-2: full-window runnable rode the chain tier with no gate at all):
// the satellite carries no interval inventory, so a pid with ANY unanchored
// census share demotes the whole row to ◇ (values untouched); a fully
// anchored pid keeps the chain lane.
func TestRSPARNBCPUAffinityLaneArm(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{PID: 200}, Depth: 1, Window: TimeWindow{StartTs: 1.0, EndTs: 1.03}},
			{Thread: ThreadRef{PID: 300}, Depth: 1, Window: TimeWindow{StartTs: 1.0, EndTs: 1.03}},
		},
		CausalImpacts: []WakeupCausalImpact{
			{Thread: ThreadRef{PID: 200}, ChainDepth: 1, RunnableMs: 5.0},
			{Thread: ThreadRef{PID: 300}, ChainDepth: 1, RunnableMs: 6.0},
		},
	}
	stats := WindowStats{
		chainAnchorsByPID:      chainAnchorWindowsByPID(chain),
		offCPUProducerDisjoint: true,
		runnableCensus: map[string]ThreadDuration{
			"200|0": {Thread: ThreadRef{PID: 200}, DurationMs: 8.0, anchoredMs: 5.0},
			// pid 300: fully anchored census (remainder ≤ tol) → keep.
			"300|0": {Thread: ThreadRef{PID: 300}, DurationMs: 6.0, anchoredMs: 6.0},
		},
	}
	items := []RootCauseRankItem{
		{Type: "cpu_affinity_or_cpuset", Thread: ThreadRef{PID: 200}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 8.0, ImpactMs: 8.0, CumulativeImpactMs: 8.0,
			Source: "window_stats.cpu_constraints", Confidence: 0.72},
		{Type: "cpu_affinity_or_cpuset", Thread: ThreadRef{PID: 300}, Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: 6.0, ImpactMs: 6.0, CumulativeImpactMs: 6.0,
			Source: "window_stats.cpu_constraints", Confidence: 0.72},
	}
	items = reanchorOnChainStateSeats(chain, stats, items)
	demoted := items[0]
	if !demoted.ChainCredentialLaneDemoted || demoted.ChainRelevance != "adjacent" {
		t.Fatalf("unanchored-share affinity satellite must demote its lane: %+v", demoted)
	}
	if demoted.RunnableMs != 8.0 || demoted.ImpactMs != 8.0 || demoted.CumulativeImpactMs != 8.0 {
		t.Fatalf("affinity lane demotion must leave values untouched: %+v", demoted)
	}
	kept := items[1]
	if kept.ChainCredentialLaneDemoted || kept.ChainRelevance != "on_chain" {
		t.Fatalf("fully anchored pid's affinity satellite keeps the chain lane: %+v", kept)
	}
}

// TestRSPARNBCriticalBlockingZeroCredentialDemotion — B-4 (§29.88 R4; customer
// E9/E10 witness): the interval-less chain-lane D/IO VIEW rows of a pid whose
// census family account proves ZERO anchored credential ride ◇ with values
// untouched; a pid with anchored credential keeps the legacy lane (the tieba
// 60555 negative control shape), and the analysis target is exempt.
func TestRSPARNBCriticalBlockingZeroCredentialDemotion(t *testing.T) {
	// worker-200 blocks in D entirely OUTSIDE its dependency window
	// ([1.001, 1.010]): census anchored = 0 → its critical_blocking D row
	// must ride ◇. helper-400 blocks INSIDE its window → keeps ⛓.
	idx := buildTraceIndex(t, "rspa_rnb_b4.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.002000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.009000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.011000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      other-300 (300) [003] .... 1.050000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.050500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.051000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.060000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     helper-400 (400) [004] .... 1.061000: sched_switch: prev_comm=helper prev_pid=400 prev_prio=40 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120
     helper-400 (400) [004] .... 1.070000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=helper next_pid=400 next_prio=40
     helper-400 (400) [004] .... 1.071000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 1.072000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 1.190000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16}
	q = normalizeQuery(idx, q)
	chain := BuildWakeupChain(idx, q)
	q.chainAnchorWindowsByPID = chainAnchorWindowsByPID(chain)
	stats := ComputeWindowStats(idx, q)
	res := buildCriticalBlockingCallsFromStats(idx, q, stats, &chain)
	var workerD, helperD *CriticalBlockingCandidate
	for i := range res.Items {
		item := &res.Items[i]
		if item.Type != "d_state_or_io_wait" && item.Type != "io_wait" {
			continue
		}
		switch item.Thread.PID {
		case 200:
			workerD = item
		case 400:
			helperD = item
		}
	}
	if workerD == nil {
		t.Fatalf("worker D view row missing: %+v", res.Items)
	}
	if workerD.ChainRelevance != "adjacent" || !workerD.ChainCredentialLaneDemoted {
		t.Fatalf("B-4: zero-credential D view row must ride ◇ with the typed marker: %+v", workerD)
	}
	if math.Abs(workerD.DurationMs-40.5) > 0.5 {
		t.Fatalf("B-4 demotion must leave the value untouched: %+v", workerD)
	}
	// EVOLUTION RECORD (复核 P1-B, 2026-07-14): the former negative arm read
	// `helperD != nil && rel=="on_chain" && demoted` — after a wrongful
	// demotion rel flips to adjacent, so the conjunction was vacuously true
	// (假 pin: the B-4 gate mutation stayed green). The arm now REQUIRES the
	// helper row present, on the chain lane, and unmarked.
	if helperD == nil {
		t.Fatalf("anchored-credential helper D view row missing: %+v", res.Items)
	}
	if helperD.ChainRelevance != "on_chain" || helperD.ChainCredentialLaneDemoted {
		t.Fatalf("anchored-credential pid must keep the legacy on-chain lane unmarked: %+v", helperD)
	}
}

// TestRSPARNBFoldLedgerStampCaliberGuard — P2 顺带 (D1 复核, 2026-07-14): the
// family fold Σ's the census-group ledger stamps ONLY under the sum_disjoint
// caliber — any other caliber (interval union / MAX fallback) publishes a
// value that is NOT the member Σ, so the stamps must CLEAR (fail open) or the
// migration would bisect a fabricated account.
func TestRSPARNBFoldLedgerStampCaliberGuard(t *testing.T) {
	mk := func(start, end float64, anchored float64, producerDisjoint bool) RootCauseRankItem {
		ms := (end - start) * 1000
		return RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{PID: 200},
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain",
			DominantState: string(StateRunnable), RunnableMs: ms, ImpactMs: ms, CumulativeImpactMs: ms,
			StartTs: start, EndTs: end, Source: "window_stats", Confidence: 0.76,
			memberSegmentsProducerDisjoint: producerDisjoint,
			ledgerAnchorStamped:            true, ledgerAnchoredRunnableMs: anchored}
	}
	// Negative arm: overlapping intervals without the producer proof → the
	// fold takes a non-Σ caliber → stamps cleared.
	out := foldSameThreadTypeRankFamilies(Query{}, true, []RootCauseRankItem{
		mk(1.000, 1.010, 4.0, false), mk(1.005, 1.015, 3.0, false)})
	if len(out) != 1 {
		t.Fatalf("family must fold: %d rows", len(out))
	}
	if out[0].MemberFoldCaliber == RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("fixture drifted: overlap without producer proof must not take the Σ caliber: %+v", out[0].MemberFoldCaliber)
	}
	if out[0].ledgerAnchorStamped || out[0].ledgerAnchoredRunnableMs != 0 {
		t.Fatalf("non-Σ fold must clear the ledger stamps (宁漏勿猜): %+v", out[0])
	}
	// Positive arm: producer-disjoint members Σ the stamps exactly.
	out = foldSameThreadTypeRankFamilies(Query{}, true, []RootCauseRankItem{
		mk(1.000, 1.010, 4.0, true), mk(1.005, 1.015, 3.0, true)})
	if len(out) != 1 || !out[0].ledgerAnchorStamped || math.Abs(out[0].ledgerAnchoredRunnableMs-7.0) > rspaAnchorIdentityTolMs {
		t.Fatalf("Σ-caliber fold must carry the summed stamps: %+v", out[0])
	}
}

// TestRSPARNBDemotedSideLaneSurvivesTruncation — D1 修复轮 层1 (§29.88 复核,
// 2026-07-14): an R4 lane-demoted seat rides the dedicated side lane — as a
// plain candidate it sorted behind every on-chain row and structurally died
// at the candidate cap (donghu 2955 witness: all three demoted rows
// 47.678/22.408/16.013 vanished from the wire), which made the
//「值零动,通道位归位」promise false on the published board.
func TestRSPARNBDemotedSideLaneSurvivesTruncation(t *testing.T) {
	var items []RootCauseRankItem
	for i := 0; i < 6; i++ {
		v := float64(100 - i)
		items = append(items, RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{PID: 100 + i},
			Causality: "on_wakeup_chain", ChainRelevance: "on_chain", DominantState: string(StateRunnable),
			RunnableMs: v, ImpactMs: v, CumulativeImpactMs: v, EffectiveImpactMs: v, Score: v,
			Source: "window_stats", Confidence: 0.8})
	}
	items = append(items, RootCauseRankItem{Type: "cpu_affinity_or_cpuset", Thread: ThreadRef{PID: 9163},
		Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent", DominantState: string(StateRunnable),
		RunnableMs: 47.678, ImpactMs: 47.678, CumulativeImpactMs: 47.678, EffectiveImpactMs: 47.678, Score: 34.0,
		Source: "window_stats.cpu_constraints", Confidence: 0.72,
		ChainCredentialLaneDemoted: true})
	out, _, candidateTotal, candidateEmitted, sideTotal, sideEmitted := truncateRootCauseRankCandidatesAndSideRows(items, 4)
	if candidateTotal != 6 || candidateEmitted != 4 {
		t.Fatalf("fixture drifted: candidates %d→%d", candidateTotal, candidateEmitted)
	}
	if sideTotal != 1 || sideEmitted != 1 {
		t.Fatalf("the demoted seat must ride the side lane: total=%d emitted=%d", sideTotal, sideEmitted)
	}
	found := false
	for _, item := range out {
		if item.ChainCredentialLaneDemoted && item.Thread.PID == 9163 && item.RunnableMs == 47.678 {
			found = true
		}
	}
	if !found {
		t.Fatalf("demoted seat vanished from the published board (值零动通道归位 broken): %+v", out)
	}
}

// TestRSPARNBSummaryTwinVisibilityPatch — D1 修复轮 层3 engine half: the
// engine EN co-publication claims re-verify against the PUBLISHED board — a
// truncation-killed twin downgrades "(owned by the chain seat)" /
// "(published as a separate adjacent seat)" to the honest unpublished form;
// co-published pairs keep their bytes.
func TestRSPARNBSummaryTwinVisibilityPatch(t *testing.T) {
	remainder := RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{PID: 200},
		ChainRelevance: "adjacent", DominantState: string(StateRunnable),
		RunnableMs: 20.0, ChainAnchoredMs: 7.0, ChainAnchorFullMs: 27.0, ChainAnchorRemainderSeat: true,
		Summary: rspaRemainderSummary(ThreadRef{Comm: "w", PID: 200}, "runnable (scheduling-pressure candidate)", 20.0, 7.0, 27.0)}
	clipped := RootCauseRankItem{Type: "runnable_wait", Thread: ThreadRef{PID: 300},
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", DominantState: string(StateRunnable),
		RunnableMs: 7.0, ChainAnchoredMs: 7.0, ChainAnchorFullMs: 27.0,
		Summary: rspaAnchoredSummary(ThreadRef{Comm: "x", PID: 300}, "runnable", 7.0, 27.0)}
	items := []RootCauseRankItem{remainder, clipped}
	rspaPatchSummariesForTwinVisibility(items)
	if !strings.Contains(items[0].Summary, rspaSummaryOwnedByChainSeatUnpublished) {
		t.Fatalf("twin-less remainder must downgrade the ownership claim: %q", items[0].Summary)
	}
	if !strings.Contains(items[1].Summary, rspaSummaryRemainderTwinUnpublished) {
		t.Fatalf("twin-less clipped seat must downgrade the co-publication claim: %q", items[1].Summary)
	}
	// Co-published pair: both keep their claims byte-identically.
	pairRem := remainder
	pairRem.Summary = rspaRemainderSummary(ThreadRef{Comm: "w", PID: 200}, "runnable (scheduling-pressure candidate)", 20.0, 7.0, 27.0)
	pairClip := clipped
	pairClip.Thread = ThreadRef{PID: 200}
	pairClip.Summary = rspaAnchoredSummary(ThreadRef{Comm: "w", PID: 200}, "runnable", 7.0, 27.0)
	pair := []RootCauseRankItem{pairRem, pairClip}
	rspaPatchSummariesForTwinVisibility(pair)
	if !strings.Contains(pair[0].Summary, rspaSummaryOwnedByChainSeat) ||
		!strings.Contains(pair[1].Summary, rspaSummaryRemainderTwinPublished) {
		t.Fatalf("co-published pair must keep the claims: %q / %q", pair[0].Summary, pair[1].Summary)
	}
}
