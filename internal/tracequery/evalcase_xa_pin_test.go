package tracequery

// evalcase_xa_pin_test.go — EVALCASE-DH batch, xxx_all/tieba family engine
// pins on the committed donghu_tieba_frame.systrace (mining ledger
// evalcase_xa_cmp_mining.md §0/§1; expectations re-collected at HEAD
// 1ada2c49f and hand-cross-checked).
//
// Cases:
//
//	XA-L2  ns 鬼 owner — the lock burst's four named owner tids
//	       (62020/62022/62023/60340) are ALL container-namespace ids with
//	       ZERO host-tid-space presence; the carve keeps them on the typed
//	       audit lane (owner_tid_raw + presence=absent) and derives the host
//	       PROCESS only (ns_pid=60194 → com.baidu.tieba-59566) — binding an
//	       owner to any host thread name would be fabrication.
//	XA-L3  哨兵语义 — `owner tid: 18446744073709551615` (uint64-1) and
//	       `owner tid: 0` are two DISTINCT explicit no-holder sentinels;
//	       both parse as typed ownerless (never a tid), and the census keeps
//	       them apart from real container owners. The burst is PURE 形B:
//	       84 "Lock contention on …" spans, zero monitor-contention grammar.
//	XA-F1  freq_only 簇 verdict + R6 首簇 donor — cpu_frequency samples exist
//	       only on cpu3/4/5; the R6 derivation closes cpu0/1/2 into the
//	       first cluster; ONE derived domain < 2 ⇒ the class judgment stays
//	       honestly freq_only (no fabricated small/middle/big topology), the
//	       fold basis is the global peak 2189000 at cap 1, and every
//	       frequency-weighted face on cpu0-2 carries the typed donor
//	       disclosure (donor_cpu=3, source=freq_change_point_derived) plus
//	       the window-level reuse caveat.
//	XA-W1  wakeup target_cpu 全退化 — every in-window sched_wakeup carries
//	       target_cpu=0 while wakeups are emitted from 6 CPUs; the engine
//	       publishes the ADVISORY degradation caveat and keeps runnable CPU
//	       attribution governed by exact migration/sched-in endpoints
//	       (precise signals gate, noisy signals disclose — §1 red line).
//
// Fixture red line: real capture — every number is a measured pin.

import (
	"math"
	"strings"
	"testing"
)

const (
	evalcaseXALockStart = 34579.453
	evalcaseXALockEnd   = 34579.4975
	evalcaseXAFullStart = 34579.450627
	evalcaseXAFullEnd   = 34579.595184
)

// XA-C2 target value channel: a whole-artifact target-scoped window_stats
// query must publish the exact target account independently of the Top-8
// background lists. This is the production shape used by the narrow
// D-state/io_wait supplement.
func TestEvalcaseXAC2WholeArtifactTargetStateAccount(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	res := Run(idx, Query{View: "window_stats", PID: 59566})
	account := res.TargetWindowStates
	if account == nil {
		t.Fatal("whole-artifact target window_stats must publish target_window_states")
	}
	if account.Thread.PID != 59566 {
		t.Fatalf("target identity drifted: %+v", account.Thread)
	}
	if math.Abs(account.IOWaitMs-0.635) > 0.001 {
		t.Fatalf("target io_wait must retain the full three-fragment account: got %.6f want 0.635", account.IOWaitMs)
	}
	if math.Abs(account.DStateMs) > 0.001 {
		t.Fatalf("all three proven iowait fragments must stay out of non-IO D-state: %+v", account)
	}
}

// XA-L3 census + XA-L2 ghost-owner precondition, on the unbounded pairing.
func TestEvalcaseXAL3PureFormBCensusWithSentinels(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	q := Query{PID: 59566, TimeStart: evalcaseXALockStart, TimeEnd: evalcaseXALockEnd, MinDurationMs: 0.0001}
	spans := evalcaseUnboundedSpans(t, idx, q, 500)
	lockSpans := 0
	sentinelRows := 0
	ownerCensus := map[int]int{}
	for _, s := range spans {
		if !strings.Contains(s.Name, "contention") {
			continue
		}
		if strings.Contains(s.Name, "monitor contention with owner") {
			t.Fatalf("XA-L3: tieba burst must be PURE 形B — monitor grammar appeared: %q", s.Name)
		}
		info, ok := parseLockContentionPayload(s.Name)
		if !ok {
			t.Fatalf("XA-L3: 形B payload failed to parse: %q", s.Name)
		}
		if info.Morphology != "lock_contention_on" {
			t.Fatalf("XA-L3: morphology drifted: %q for %q", info.Morphology, s.Name)
		}
		lockSpans++
		if info.OwnerAbsent {
			sentinelRows++
			if info.Owner.PID != 0 {
				t.Fatalf("XA-L3: sentinel minted a tid: %+v", info)
			}
		} else if info.Owner.PID > 0 {
			ownerCensus[info.Owner.PID]++
		}
	}
	// Hand census (shell-verified 2026-07-18): 84 形B rows; owners
	// 62020×47, 62022×5, 62023×1, 60340×1; sentinels (uint64-1 + 0) ×30.
	if lockSpans != 84 {
		t.Fatalf("XA-L3: lock census drifted: %d spans (want 84)", lockSpans)
	}
	if sentinelRows != 30 {
		t.Fatalf("XA-L3: sentinel census drifted: %d (want 30)", sentinelRows)
	}
	want := map[int]int{62020: 47, 62022: 5, 62023: 1, 60340: 1}
	for tid, n := range want {
		if ownerCensus[tid] != n {
			t.Fatalf("XA-L3: owner census drifted: %v (want %v)", ownerCensus, want)
		}
	}
	// XA-L2 precondition: every named owner tid is a host-space ghost.
	for tid := range want {
		if idx.tidPresent(tid) {
			t.Fatalf("XA-L2: fixture fact drifted — owner tid %d must be absent from the host tid space", tid)
		}
	}
	// Both sentinel spellings parse ownerless directly (typed-lane pins).
	for _, payload := range []string{
		"Lock contention on ClassLinker classes lock (owner tid: 18446744073709551615)",
		"Lock contention on thread suspend count lock (owner tid: 0)",
	} {
		info, ok := parseLockContentionPayload(payload)
		if !ok || !info.OwnerAbsent || info.Owner.PID != 0 {
			t.Fatalf("XA-L3: sentinel spelling drifted: %q → ok=%v %+v", payload, ok, info)
		}
	}
}

// XA-L2 carve face: ghost owner rides the audit lane, host derivation stops
// at PROCESS level, no host-thread fabrication.
func TestEvalcaseXAL2GhostOwnerCarve(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	q := normalizeQuery(idx, Query{PID: 59566, TimeStart: evalcaseXALockStart, TimeEnd: evalcaseXALockEnd, MinDurationMs: 0.0001})
	spans := evalcaseUnboundedSpans(t, idx, q, 500)
	stats := ComputeWindowStats(idx, q)
	stats.TraceSpans = spans
	rows := collectBlockingSpanRows(idx, q, stats)
	lockRows := 0
	ghost62020 := 0
	for _, r := range rows {
		if r.cand.BlockingKind == "" {
			continue
		}
		lockRows++
		cand := r.cand
		if cand.OwnerTidRaw == 62020 {
			ghost62020++
			if cand.OwnerTidPresence != OwnerTidPresenceAbsent {
				t.Fatalf("XA-L2: ghost owner presence verdict drifted: %+v", cand)
			}
			if !strings.Contains(cand.HolderHostProcess, "ns_pid=60194") || !strings.Contains(cand.HolderHostProcess, "tgid=59566") {
				t.Fatalf("XA-L2: ns process derivation drifted: %q", cand.HolderHostProcess)
			}
			// The container tid must NEVER be published as a resolved host
			// Peer: either the peer is empty, or it is a wakeup-edge
			// counterpart with an explicit non-payload holder_source.
			if cand.Peer.PID == 62020 {
				t.Fatalf("XA-L2: container owner tid leaked into Peer: %+v", cand.Peer)
			}
			if cand.Peer.PID != 0 && cand.HolderSource != "wakeup_edge" {
				t.Fatalf("XA-L2: resolved peer without a typed non-payload source: %+v", cand)
			}
		}
	}
	if lockRows != 84 {
		t.Fatalf("XA-L2: carve row census drifted: %d (want 84 — 形B rows never fold across distinct locks)", lockRows)
	}
	if ghost62020 != 47 {
		t.Fatalf("XA-L2: owner-62020 rows drifted: %d (want 47)", ghost62020)
	}
}

// XA-F1 freq_only verdict + donor disclosure.
func TestEvalcaseXAF1FreqOnlyDonorDisclosure(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	// Sample-absence structure facts (rules 1+3 preconditions).
	tls := indexFreqSampleTimelines(idx)
	for _, cpu := range []int{0, 1, 2} {
		if len(tls[cpu]) != 0 {
			t.Fatalf("XA-F1: fixture fact drifted — cpu%d must have zero cpu_frequency samples", cpu)
		}
	}
	// Verdict: ONE derived domain [0..5] ⇒ honestly freq_only, no class map.
	capability := indexDerivedCoreCapability(idx)
	if capability.usable() || capability.source != CoreCapabilitySourceFreqOnly {
		t.Fatalf("XA-F1: capability must stay freq_only (no fabricated topology), got usable=%v source=%q", capability.usable(), capability.source)
	}
	if len(capability.classByCluster) != 0 {
		t.Fatalf("XA-F1: freq_only must not mint class labels: %v", capability.classByCluster)
	}
	cache := newChainQueryCache(idx, nil)
	fm, refCap, refClass := cache.supplyFoldGlobalMaxBasis(cache.coreCapability(""))
	if fm.khz != 2189000 || fm.source != SupplyFoldFmaxSourceObserved || refClass != "" || refCap != 1 {
		t.Fatalf("XA-F1: fold basis drifted: %d/%s class=%q cap=%v (want 2189000 observed, classless, cap 1)", fm.khz, fm.source, refClass, refCap)
	}
	// Donor disclosure on the frequency-weighted balance face.
	q := normalizeQuery(idx, Query{PID: 59566, TimeStart: evalcaseXAFullStart, TimeEnd: evalcaseXAFullEnd})
	stats := ComputeWindowStats(idx, q)
	if stats.ComputeSupplyBalance == nil {
		t.Fatalf("XA-F1: compute supply balance missing")
	}
	for _, p := range stats.ComputeSupplyBalance.PerCPU {
		switch p.CPU {
		case 0, 1, 2:
			if p.FrequencyClusterDonorCPU == nil || *p.FrequencyClusterDonorCPU != 3 || p.FrequencyClusterDonorSource != ClusterFreqSourceDerived {
				t.Fatalf("XA-F1: cpu%d must carry donor_cpu=3 source=freq_change_point_derived, got %+v", p.CPU, p)
			}
		case 3, 4, 5:
			if p.FrequencyClusterDonorCPU != nil {
				t.Fatalf("XA-F1: sampled cpu%d must NOT carry a donor (its own samples govern): %+v", p.CPU, p)
			}
		}
		if p.MaxFrequencyKHz != 2189000 {
			t.Fatalf("XA-F1: cpu%d fmax drifted: %d", p.CPU, p.MaxFrequencyKHz)
		}
	}
	// Window-level reuse caveat: the R6 derivation words + the per-cpu reuse
	// map + the honest "no explicit core_topology" head.
	found := false
	for _, c := range stats.Caveats {
		if strings.Contains(c, "cluster-shared frequency reuse from freq-change-point derived clusters (no explicit core_topology)") &&
			strings.Contains(c, "cpu0←cpu3,cpu1←cpu3,cpu2←cpu3") &&
			strings.Contains(c, "R6 首簇/区间闭包") {
			found = true
		}
	}
	if !found {
		t.Fatalf("XA-F1: cluster-freq reuse caveat missing/drifted: %v", stats.Caveats)
	}
}

// XA-W1 wakeup 退化 advisory.
func TestEvalcaseXAW1WakeupTargetDegradedAdvisory(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseTiebaFixture)
	// Structure fact: all 2130 sched_wakeup rows carry target_cpu=0 and
	// sched_waking is absent entirely.
	wakeups, nonZero, waking := 0, 0, 0
	for _, ev := range idx.Events {
		switch ev.Type {
		case "sched_wakeup":
			wakeups++
			if ev.TargetCPU != 0 {
				nonZero++
			}
		case "sched_waking":
			waking++
		}
	}
	if wakeups != 2130 || nonZero != 0 || waking != 0 {
		t.Fatalf("XA-W1: fixture fact drifted: wakeups=%d nonZero=%d waking=%d", wakeups, nonZero, waking)
	}
	q := normalizeQuery(idx, Query{PID: 59566, TimeStart: evalcaseXAFullStart, TimeEnd: evalcaseXAFullEnd})
	stats := ComputeWindowStats(idx, q)
	// Cold-read F2 (§29.137): the advisory caveat is a DISPLAY face, not a
	// promise face — pin the typed tokens (marker + census figure + advisory
	// stance), never the full prose (any wording batch would false-red it).
	found := false
	for _, c := range stats.Caveats {
		if strings.Contains(c, "wakeup_target_cpu_degraded=true") &&
			strings.Contains(c, "total=2130") &&
			strings.Contains(c, "advisory only") {
			found = true
		}
	}
	if !found {
		t.Fatalf("XA-W1: degradation advisory missing/drifted: %v", stats.Caveats)
	}
	// The degradation stays ADVISORY: per-CPU runnable attribution keeps its
	// own verified continuity account (mismatch_segments=0 on this trace).
	degradedGate := false
	for _, c := range stats.Caveats {
		if strings.Contains(c, "runnable_cpu_continuity_degraded=true") && strings.Contains(c, "mismatch_segments=0") {
			degradedGate = true
		}
	}
	if !degradedGate {
		t.Fatalf("XA-W1: runnable continuity account missing its verified shape: %v", stats.Caveats)
	}
}
