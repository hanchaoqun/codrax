package tracequery

// rank_chain_anchor_hullcred_test.go — HULL-CRED (§29.104 终判③ / §29.99
// 裁定池, 2026-07-17): the keep-⛓ side of the chain-lane D/IO VIEW verdict is
// credentialed per segment —
//
//	hull∩锚窗 = ∅            → demote (RNB-5B sound arm, byte-identical, no
//	                           new wire words even when segments ride the row);
//	hull∩ > 0, 段清单有效      → 逐段∩锚窗: ≥1 段真相交 keeps ⛓ with the
//	                           published per-segment credential; 全不相交
//	                           demotes ◇ + the disjoint marker + the proof;
//	hull∩ > 0, 段清单缺席      → keep ⛓ wearing the 「(包络级凭证)」 marker;
//	无区间                    → pid-level conservative rule (zero credential
//	                           demotes; a credentialed pid keeps ⛓ wearing
//	                           the same envelope marker).
//
// MUTATION self-check: reverting the keep side to the pure hull rule (hull∩>0
// → keep, segments unread) reds TestHULLCREDSegmentDisjointDemotionEndToEnd
// (the worker row keeps ⛓ on hull noise — the pre-fix fake-credential shape)
// and the 全不相交 arm of TestHULLCREDVerdictArms. Dropping the all-or-
// nothing inventory validation reds the Σ-mismatch / invalid-segment /
// over-cap arms (they must fall to the envelope tier, never adjudicate).
// Source-latch mutations (便宜修轮件1): removing the dioIntervalsOverflow
// latch (post-overflow segments rebuilding a partial list) or the cap check
// itself (unbounded collection) reds
// TestHULLCREDSourceInventoryAllOrNothingLatch — the downstream inventory
// re-validation masks both from the wire-level pins, so the source ledger is
// pinned directly.

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestHULLCREDVerdictArms(t *testing.T) {
	windows := []TimeWindow{{StartTs: 1.000, EndTs: 1.010}}
	credentialed := rspaFamilyDecision{migrate: true, anchoredMs: 5.0, fullMs: 8.0}
	zeroCredential := rspaFamilyDecision{migrate: true, anchoredMs: 0.0, fullMs: 8.0}
	row := func(hullStart, hullEnd, durationMs float64, segs ...foldInterval) CriticalBlockingCandidate {
		return CriticalBlockingCandidate{Type: "d_state_or_io_wait", DurationMs: durationMs,
			reconStartTs: hullStart, reconEndTs: hullEnd, credentialSegments: segs}
	}
	// ① ≥1 segment truly intersects → keep ⛓ with the per-segment credential.
	// The verdict hands back the validated inventory it adjudicated on — the
	// ONE slice call sites may publish (单一值源, 便宜修轮件3).
	verified := row(0.990, 1.050, 8.0,
		foldInterval{start: 1.005, end: 1.008}, foldInterval{start: 1.020, end: 1.025})
	got, gotSegs := criticalBlockingDioRowCredentialVerdict(verified, credentialed, windows)
	if got != dioCredentialKeepSegmentVerified {
		t.Fatalf("① intersecting segment must keep ⛓ segment-verified, got %d", got)
	}
	if len(gotSegs) != 2 || gotSegs[0] != verified.credentialSegments[0] || gotSegs[1] != verified.credentialSegments[1] {
		t.Fatalf("① the verdict must return the validated inventory it adjudicated on: %+v", gotSegs)
	}
	// ② hull intersects but EVERY segment lies in the hull gaps → the NEW
	// demote form (the pre-fix fake-credential keep-⛓ shape, 双向: under the
	// old hull-only rule this row kept the lane).
	disjoint := row(0.990, 1.050, 10.0,
		foldInterval{start: 0.990, end: 0.995}, foldInterval{start: 1.040, end: 1.045})
	if overlap := anchorWindowsOverlapMs(windows, disjoint.reconStartTs, disjoint.reconEndTs); overlap <= 0 {
		t.Fatalf("fixture drifted: the hull must intersect the window (hull noise), got %.3f", overlap)
	}
	got, gotSegs = criticalBlockingDioRowCredentialVerdict(disjoint, credentialed, windows)
	if got != dioCredentialDemoteSegmentDisjoint {
		t.Fatalf("② all-disjoint inventory must demote with the disjoint marker, got %d", got)
	}
	if len(gotSegs) != 2 || gotSegs[0] != disjoint.credentialSegments[0] || gotSegs[1] != disjoint.credentialSegments[1] {
		t.Fatalf("② the disjoint demotion must return its proof inventory: %+v", gotSegs)
	}
	// ③ hull intersects, inventory absent → the envelope-tier honest keep.
	if got, gotSegs = criticalBlockingDioRowCredentialVerdict(row(1.005, 1.050, 8.0), credentialed, windows); got != dioCredentialKeepEnvelope || gotSegs != nil {
		t.Fatalf("③ inventory-less keep must ride the envelope tier with a nil inventory, got %d %+v", got, gotSegs)
	}
	// ③b all-or-nothing validation: a Σ-mismatched, an invalid and an
	// over-cap inventory must NOT adjudicate — they fall to the envelope tier
	// and never leak a slice out (nil, not the raw carried list).
	mismatch := row(0.990, 1.050, 99.0, foldInterval{start: 0.990, end: 0.995})
	if got, gotSegs = criticalBlockingDioRowCredentialVerdict(mismatch, credentialed, windows); got != dioCredentialKeepEnvelope || gotSegs != nil {
		t.Fatalf("③b Σ-mismatched inventory must fall to the envelope tier, got %d %+v", got, gotSegs)
	}
	invalid := row(0.990, 1.050, 5.0, foldInterval{start: 0.995, end: 0.990})
	if got, gotSegs = criticalBlockingDioRowCredentialVerdict(invalid, credentialed, windows); got != dioCredentialKeepEnvelope || gotSegs != nil {
		t.Fatalf("③b invalid segment must fall to the envelope tier, got %d %+v", got, gotSegs)
	}
	var over []foldInterval
	for i := 0; i < CriticalBlockingCredentialSegmentCap+1; i++ {
		over = append(over, foldInterval{start: 2.0 + float64(i)*0.002, end: 2.001 + float64(i)*0.002})
	}
	overRow := row(0.990, 3.050, float64(len(over)), over...)
	if got, gotSegs = criticalBlockingDioRowCredentialVerdict(overRow, credentialed, windows); got != dioCredentialKeepEnvelope || gotSegs != nil {
		t.Fatalf("③b over-cap inventory must fall to the envelope tier, got %d %+v", got, gotSegs)
	}
	// ④ hull∩ = ∅ takes the LEGACY demote arm even when a valid disjoint
	// inventory rides the row (已终判 sound 臂零动 — no new wire words there,
	// and no inventory leaves the verdict either).
	empty := row(1.020, 1.050, 10.0,
		foldInterval{start: 1.020, end: 1.025}, foldInterval{start: 1.045, end: 1.050})
	if got, gotSegs = criticalBlockingDioRowCredentialVerdict(empty, credentialed, windows); got != dioCredentialDemoteLegacy || gotSegs != nil {
		t.Fatalf("④ hull-∅ must stay the legacy demote arm with no inventory, got %d %+v", got, gotSegs)
	}
	// ⑤ interval-less rows keep the pid-level conservative rule; the keep
	// outcome wears the envelope tier (段清单缺席保守留道 + 诚实词).
	if got, gotSegs = criticalBlockingDioRowCredentialVerdict(row(0, 0, 8.0), zeroCredential, windows); got != dioCredentialDemoteLegacy || gotSegs != nil {
		t.Fatalf("⑤ interval-less zero credential must demote legacy, got %d %+v", got, gotSegs)
	}
	if got, gotSegs = criticalBlockingDioRowCredentialVerdict(row(0, 0, 8.0), credentialed, windows); got != dioCredentialKeepEnvelope || gotSegs != nil {
		t.Fatalf("⑤ interval-less credentialed keep must ride the envelope tier, got %d %+v", got, gotSegs)
	}
	// ⑥ the boolean face (RNB-5B pin contract) equals verdict∈demote set.
	for _, tc := range []struct {
		name string
		item CriticalBlockingCandidate
		dec  rspaFamilyDecision
		want bool
	}{
		{"verified keep", verified, credentialed, false},
		{"disjoint demote", disjoint, credentialed, true},
		{"hull-∅ demote", empty, credentialed, true},
		{"interval-less keep", row(0, 0, 8.0), credentialed, false},
		{"interval-less demote", row(0, 0, 8.0), zeroCredential, true},
	} {
		if got := criticalBlockingDioRowDemotes(tc.item, tc.dec, windows); got != tc.want {
			t.Fatalf("⑥ boolean face diverged on %s: got %v", tc.name, got)
		}
	}
}

// hullcredTraceIndex — ONE production-path trace exercising all three
// HULL-CRED shapes beside the target app-100:
//
//   - worker-200: two D segments [1.002,1.012]+[1.032,1.045] straddling its
//     anchor window [1.020,1.030] (it wakes app at 1.030) — the hull
//     [1.002,1.045] intersects the window while NO real segment does: the
//     pre-fix fake-credential keep-⛓ shape → segment-disjoint demotion.
//   - helper-400: one D segment [1.065,1.072] INSIDE its anchor window
//     [1.060,1.075] (it wakes app at 1.075) → segment-verified keep.
//   - env-500: 33 D segments (over CriticalBlockingCredentialSegmentCap)
//     straddling its anchor window [1.100,1.110] (it wakes app at 1.110) —
//     the inventory drops whole at the source → envelope-tier keep.
func hullcredTraceIndex(t *testing.T) *Index {
	t.Helper()
	var sb strings.Builder
	line := func(format string, args ...interface{}) {
		sb.WriteString("\t")
		sb.WriteString(strings.TrimSpace(fmt.Sprintf(format, args...)))
		sb.WriteString("\n")
	}
	line("app-100 (100) [001] .... 1.001000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52")
	// worker D segment A [1.002, 1.012] (before the anchor window).
	line("worker-200 (200) [002] .... 1.002000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120")
	line("other-300 (300) [003] .... 1.012000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002")
	// app blocks [1.020, 1.030]; worker ends the wait → worker's anchor window.
	line("app-100 (100) [001] .... 1.020000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120")
	line("worker-200 (200) [002] .... 1.028000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40")
	line("worker-200 (200) [002] .... 1.030000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001")
	line("app-100 (100) [001] .... 1.031000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52")
	// worker D segment B [1.032, 1.045] (after the anchor window).
	line("worker-200 (200) [002] .... 1.032000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120")
	line("other-300 (300) [003] .... 1.045000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002")
	// app blocks [1.060, 1.075]; helper ends the wait.
	line("app-100 (100) [001] .... 1.060000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120")
	// helper D segment [1.065, 1.072] INSIDE its window.
	line("helper-400 (400) [004] .... 1.065000: sched_switch: prev_comm=helper prev_pid=400 prev_prio=40 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120")
	line("helper-400 (400) [004] .... 1.072000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=helper next_pid=400 next_prio=40")
	line("helper-400 (400) [004] .... 1.075000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001")
	line("app-100 (100) [001] .... 1.076000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52")
	// env D segment 0 [1.090, 1.092] (before env's anchor window).
	line("env-500 (500) [005] .... 1.090000: sched_switch: prev_comm=env prev_pid=500 prev_prio=40 prev_state=D ==> next_comm=idle/5 next_pid=0 next_prio=120")
	line("other-300 (300) [003] .... 1.092000: sched_wakeup: comm=env pid=500 prio=40 target_cpu=005")
	// app blocks [1.100, 1.110]; env ends the wait → env's anchor window.
	line("app-100 (100) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120")
	line("env-500 (500) [005] .... 1.108000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=env next_pid=500 next_prio=40")
	line("env-500 (500) [005] .... 1.110000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001")
	line("app-100 (100) [001] .... 1.111000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52")
	// env D segments 1..32 (after env's window) → 33 total, over the cap.
	for i := 0; i < 32; i++ {
		base := 1.112 + float64(i)*0.004
		line("env-500 (500) [005] .... %.6f: sched_switch: prev_comm=env prev_pid=500 prev_prio=40 prev_state=D ==> next_comm=idle/5 next_pid=0 next_prio=120", base)
		line("other-300 (300) [003] .... %.6f: sched_wakeup: comm=env pid=500 prio=40 target_cpu=005", base+0.002)
		line("env-500 (500) [005] .... %.6f: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=env next_pid=500 next_prio=40", base+0.003)
	}
	line("app-100 (100) [001] .... 1.250000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120")
	return buildTraceIndex(t, "hullcred.systrace", "\n"+sb.String())
}

func hullcredCriticalBlockingRows(t *testing.T) map[int]*CriticalBlockingCandidate {
	t.Helper()
	idx := hullcredTraceIndex(t)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.3, MaxDepth: 4, MaxBranches: 8, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16}
	q = normalizeQuery(idx, q)
	chain := BuildWakeupChain(idx, q)
	q.chainAnchorWindowsByPID = chainAnchorWindowsByPID(chain)
	stats := ComputeWindowStats(idx, q)
	res := buildCriticalBlockingCallsFromStats(idx, q, stats, &chain)
	rows := map[int]*CriticalBlockingCandidate{}
	for i := range res.Items {
		item := &res.Items[i]
		if item.Type != "d_state_or_io_wait" && item.Type != "io_wait" {
			continue
		}
		rows[item.Thread.PID] = item
	}
	return rows
}

// TestHULLCREDSegmentDisjointDemotionEndToEnd — the production-path witness of
// 裁定③'s core arm: the worker row's hull intersects its anchor window while
// every real segment lies in the hull gap → ◇ + disjoint marker + the
// published segment inventory (claim and proof on one row), value untouched.
// Under the pre-fix hull-only rule this row KEPT ⛓ (the fake credential).
func TestHULLCREDSegmentDisjointDemotionEndToEnd(t *testing.T) {
	rows := hullcredCriticalBlockingRows(t)
	worker := rows[200]
	if worker == nil {
		t.Fatalf("worker D view row missing")
	}
	if worker.ChainRelevance != "adjacent" || !worker.ChainCredentialLaneDemoted || !worker.ChainCredentialSegmentDisjoint {
		t.Fatalf("worker must demote with the disjoint marker: %+v", worker)
	}
	if worker.ChainCredentialEnvelopeLevel {
		t.Fatalf("a demoted row must never wear the envelope keep marker: %+v", worker)
	}
	want := []string{"1.002000..1.012000", "1.032000..1.045000"}
	if len(worker.ChainCredentialSegments) != len(want) {
		t.Fatalf("worker must publish its COMPLETE segment inventory as the proof: %+v", worker.ChainCredentialSegments)
	}
	for i, entry := range want {
		if worker.ChainCredentialSegments[i] != entry {
			t.Fatalf("segment %d: want %q got %q", i, entry, worker.ChainCredentialSegments[i])
		}
	}
	if math.Abs(worker.DurationMs-23.0) > 0.5 {
		t.Fatalf("demotion must leave the value untouched (值零动): %+v", worker)
	}
}

// TestHULLCREDSegmentVerifiedKeepEndToEnd — the ≥1-true-intersection keep: the
// helper row keeps ⛓ carrying its per-segment credential, no envelope word,
// no demotion markers.
func TestHULLCREDSegmentVerifiedKeepEndToEnd(t *testing.T) {
	rows := hullcredCriticalBlockingRows(t)
	helper := rows[400]
	if helper == nil {
		t.Fatalf("helper D view row missing")
	}
	if helper.ChainRelevance != "on_chain" || helper.ChainCredentialLaneDemoted || helper.ChainCredentialSegmentDisjoint || helper.ChainCredentialEnvelopeLevel {
		t.Fatalf("helper must keep ⛓ unmarked except for the credential: %+v", helper)
	}
	if len(helper.ChainCredentialSegments) != 1 || helper.ChainCredentialSegments[0] != "1.065000..1.072000" {
		t.Fatalf("helper must publish its per-segment credential: %+v", helper.ChainCredentialSegments)
	}
}

// TestHULLCREDEnvelopeTierEndToEnd — the cost-degraded tier: env-500's 33 D
// segments exceed the cap, the inventory drops whole at the ledger source and
// the row keeps ⛓ wearing ONLY the envelope-level honest marker (fail-open
// 保守留道不变,只加诚实词).
func TestHULLCREDEnvelopeTierEndToEnd(t *testing.T) {
	rows := hullcredCriticalBlockingRows(t)
	env := rows[500]
	if env == nil {
		t.Fatalf("env D view row missing")
	}
	if env.ChainRelevance != "on_chain" || env.ChainCredentialLaneDemoted || env.ChainCredentialSegmentDisjoint {
		t.Fatalf("env must keep the ⛓ lane: %+v", env)
	}
	if !env.ChainCredentialEnvelopeLevel {
		t.Fatalf("the inventory-less keep must wear the envelope-level marker: %+v", env)
	}
	if len(env.ChainCredentialSegments) != 0 {
		t.Fatalf("an over-cap group must publish NO inventory (all-or-nothing): %+v", env.ChainCredentialSegments)
	}
}

// TestHULLCREDLegacyDemoteArmCarriesNoNewFields — the hull-∅ sound arm stays
// byte-identical on the wire fields: the RNB-5B B-4 witness trace's worker
// row (point-contact hull, zero credential) demotes WITHOUT any HULL-CRED
// marker or inventory (∅ 判定臂零动).
func TestHULLCREDLegacyDemoteArmCarriesNoNewFields(t *testing.T) {
	idx := buildTraceIndex(t, "hullcred_legacy.systrace", `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.000500: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
      other-300 (300) [003] .... 1.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
     worker-200 (100) [002] .... 1.002000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.009000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.011000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
      other-300 (300) [003] .... 1.050000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
        app-100 (100) [001] .... 1.190000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
	`)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.2, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16}
	q = normalizeQuery(idx, q)
	chain := BuildWakeupChain(idx, q)
	q.chainAnchorWindowsByPID = chainAnchorWindowsByPID(chain)
	stats := ComputeWindowStats(idx, q)
	res := buildCriticalBlockingCallsFromStats(idx, q, stats, &chain)
	for i := range res.Items {
		item := res.Items[i]
		if item.Thread.PID != 200 || (item.Type != "d_state_or_io_wait" && item.Type != "io_wait") {
			continue
		}
		if !item.ChainCredentialLaneDemoted {
			t.Fatalf("fixture drifted: the zero-credential worker row must still demote: %+v", item)
		}
		if item.ChainCredentialSegmentDisjoint || item.ChainCredentialEnvelopeLevel || len(item.ChainCredentialSegments) != 0 {
			t.Fatalf("∅-arm demotion must carry NO HULL-CRED field (零动): %+v", item)
		}
		return
	}
	t.Fatalf("worker D view row missing: %+v", res.Items)
}

// TestHULLCREDSourceInventoryAllOrNothingLatch — 便宜修轮件1 (P2): the SOURCE
// ledger's all-or-nothing latch pinned directly at the addDurationCause close
// site. The wire-level envelope pins above are masked by the downstream
// re-validation (criticalBlockingCredentialSegmentInventory drops an over-cap
// list again), so a latch-less source (post-overflow segments rebuilding a
// partial list — mutation M6) and a cap-less source (unbounded collection —
// mutation M6b) both survived them. This pin reads the ThreadDuration ledger
// itself: cap+2 D segments must leave dioIntervals==nil with the overflow
// latched, and segCount==cap+2 proves TWO post-cap segments actually flowed
// through the latched arm (a cap+1 fixture cannot distinguish the latch from
// a one-shot drop). The control group pins the collection arm non-vacuously.
func TestHULLCREDSourceInventoryAllOrNothingLatch(t *testing.T) {
	var sb strings.Builder
	line := func(format string, args ...interface{}) {
		sb.WriteString("\t")
		sb.WriteString(strings.TrimSpace(fmt.Sprintf(format, args...)))
		sb.WriteString("\n")
	}
	// env-700: cap+2 D segments [base, base+0.002] on cpu 005.
	overCount := CriticalBlockingCredentialSegmentCap + 2
	for i := 0; i < overCount; i++ {
		base := 1.010 + float64(i)*0.004
		line("env-700 (700) [005] .... %.6f: sched_switch: prev_comm=env prev_pid=700 prev_prio=40 prev_state=D ==> next_comm=idle/5 next_pid=0 next_prio=120", base)
		line("other-300 (300) [003] .... %.6f: sched_wakeup: comm=env pid=700 prio=40 target_cpu=005", base+0.002)
		line("env-700 (700) [005] .... %.6f: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=env next_pid=700 next_prio=40", base+0.003)
	}
	// ctl-800: two D segments, safely below the cap (the collection control).
	line("ctl-800 (800) [006] .... 1.200000: sched_switch: prev_comm=ctl prev_pid=800 prev_prio=40 prev_state=D ==> next_comm=idle/6 next_pid=0 next_prio=120")
	line("other-300 (300) [003] .... 1.202000: sched_wakeup: comm=ctl pid=800 prio=40 target_cpu=006")
	line("ctl-800 (800) [006] .... 1.203000: sched_switch: prev_comm=idle/6 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ctl next_pid=800 next_prio=40")
	line("ctl-800 (800) [006] .... 1.210000: sched_switch: prev_comm=ctl prev_pid=800 prev_prio=40 prev_state=D ==> next_comm=idle/6 next_pid=0 next_prio=120")
	line("other-300 (300) [003] .... 1.212500: sched_wakeup: comm=ctl pid=800 prio=40 target_cpu=006")
	line("ctl-800 (800) [006] .... 1.213000: sched_switch: prev_comm=idle/6 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ctl next_pid=800 next_prio=40")
	idx := buildTraceIndex(t, "hullcred_latch.systrace", "\n"+sb.String())
	q := Query{TimeStart: 1.0, TimeEnd: 1.3, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16}
	q = normalizeQuery(idx, q)
	stats := ComputeWindowStats(idx, q)
	groups := map[int]ThreadDuration{}
	for _, td := range stats.DStateTop {
		groups[td.Thread.PID] = td
	}
	env, ok := groups[700]
	if !ok {
		t.Fatalf("env D-state ledger group missing: %+v", stats.DStateTop)
	}
	// Fixture pin: all cap+2 segments must have flowed through the ONE close
	// site — segments 33 AND 34 exercised the cap arm / the latched arm.
	if env.segCount != overCount {
		t.Fatalf("fixture drifted: env group must close %d D segments, got %d", overCount, env.segCount)
	}
	if !env.dioIntervalsOverflow {
		t.Fatalf("over-cap group must latch dioIntervalsOverflow at the source")
	}
	// 闩死语义: after the overflow drop, LATER segments must never resurrect
	// a partial list — the inventory stays nil, not a rebuilt suffix.
	if env.dioIntervals != nil {
		t.Fatalf("over-cap group must drop the WHOLE inventory and stay dropped (all-or-nothing latch), got %d segment(s): %+v", len(env.dioIntervals), env.dioIntervals)
	}
	ctl, ok := groups[800]
	if !ok {
		t.Fatalf("ctl D-state ledger group missing: %+v", stats.DStateTop)
	}
	if ctl.dioIntervalsOverflow || ctl.segCount != 2 || len(ctl.dioIntervals) != 2 {
		t.Fatalf("control group must collect its exact inventory un-latched: overflow=%v segCount=%d inventory=%+v", ctl.dioIntervalsOverflow, ctl.segCount, ctl.dioIntervals)
	}
	wantCtl := [][2]float64{{1.200, 1.202}, {1.210, 1.2125}}
	for i, want := range wantCtl {
		if math.Abs(ctl.dioIntervals[i].start-want[0]) > 1e-6 || math.Abs(ctl.dioIntervals[i].end-want[1]) > 1e-6 {
			t.Fatalf("control segment %d drifted: want %.6f..%.6f got %.6f..%.6f", i, want[0], want[1], ctl.dioIntervals[i].start, ctl.dioIntervals[i].end)
		}
	}
}
