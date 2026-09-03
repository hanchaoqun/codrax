package tracequery

// onchain_fix2_test.go — ONCHAIN-FIX-2 pins (2026-07-18; Q5/Q6 已追认 +
// 件1 包络泛化 + 件4 AXIOM-V2 偏离④ 衔接):
//
//	件2  entrance convergence — one physical D/IO view row receives ONE
//	     verdict regardless of entrance (direct BuildCriticalBlockingCalls /
//	     Run view / the bundle-shaped chain-first call). Pre-fix the direct
//	     entrances swept anchor-less and skipped the HULL-CRED four arms
//	     whole (判定机器单点; the fork was even view-order dependent).
//	件3  proven-lower-bound prefix — the beyond-cap ledger keeps the first
//	     cap segments; a prefix may prove presence (≥1 段∩ → keep ⛓ +
//	     truncated marker) but NEVER absence (前缀全不交 → envelope tier,
//	     never the disjoint demotion — 缺证≠证无).
//	件4  the formal D/IO seat's true-segment carrier (dioSegmentIntervals)
//	     feeds the AXIOM-V2 direction-support closed set; every validation
//	     miss leaves the seat out (fail-open, 宁漏勿假指).
//	件1  the rank-lane envelope-tier honest word — hull-only keep-⛓ rows
//	     wear 「(包络级凭证)」; precise tiers (single-segment µs identity /
//	     typed credential arms / RSPA-owned lanes / semantic mint lanes)
//	     never do.
//
// MUTATION self-check: reverting any direct entrance to the anchor-less
// sweep reds TestONCHAINFIX2EntranceConvergence (the worker row keeps the
// legacy lane there while the bundle demotes it — the pre-fix fork; note
// the entrance revert ALONE survives through the FromStats self-heal arm —
// only stripping BOTH reds this pin). Stripping the self-heal arm (or
// re-narrowing it with the old cachedChain==nil clause) reds
// TestONCHAINFIX2SelfHealConvergence (fourth/fifth residual-caller forms,
// 收敛加固修复轮 2026-07-18). Letting a truncated prefix mint the disjoint
// demotion (缺证判无) reds TestONCHAINFIX2PartialPrefixNeverProvesAbsence.
// Reverting the source to the all-or-nothing drop reds
// TestONCHAINFIX2PrefixKeepEndToEnd (the intersecting prefix segment is
// gone, the keep falls to the envelope tier). Accepting a short latched
// list reds TestONCHAINFIX2TruncatedShortListIllegal (修复轮: latch ⇒
// len==cap is a production invariant). Dropping the 件1 stamp reds
// TestONCHAINFIX2EnvelopeTierStamp; degrading its assign semantics to a
// sticky |= reds TestONCHAINFIX2EnvelopeTierStampAssignSemantics.

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// --- 件2: entrance convergence -------------------------------------------------

func onchainfix2VerdictFace(item CriticalBlockingCandidate) [6]interface{} {
	return [6]interface{}{
		item.ChainRelevance,
		item.ChainCredentialLaneDemoted,
		item.ChainCredentialSegmentDisjoint,
		strings.Join(item.ChainCredentialSegments, "|"),
		item.ChainCredentialEnvelopeLevel,
		item.ChainCredentialSegmentsTruncated,
	}
}

// onchainfix2CollectDioVerdicts folds a result's D/IO view rows onto their
// full verdict faces, keyed type|comm (shared by the entrance-convergence and
// self-heal pins).
func onchainfix2CollectDioVerdicts(res CriticalBlockingResult) map[string][6]interface{} {
	out := map[string][6]interface{}{}
	for _, item := range res.Items {
		if item.Type != "d_state_or_io_wait" && item.Type != "io_wait" {
			continue
		}
		out[item.Type+"|"+item.Thread.Comm] = onchainfix2VerdictFace(item)
	}
	return out
}

// TestONCHAINFIX2EntranceConvergence — the hullcred witness geometry through
// all three entrances: the direct exported entrance, the Run view entrance
// and the bundle-shaped chain-first call must agree row by row on the full
// verdict face. Pre-fix the two direct entrances kept worker-200 on the
// legacy ⛓ lane (four arms skipped on the anchor-less sweep) while the
// bundle demoted it — the Q5 fork shape.
func TestONCHAINFIX2EntranceConvergence(t *testing.T) {
	idx := hullcredTraceIndex(t)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.3, MaxDepth: 4, MaxBranches: 8, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16}
	collect := onchainfix2CollectDioVerdicts

	// Bundle-shaped entrance (chain-first anchored sweep — the reference).
	nq := normalizeQuery(idx, q)
	chain := BuildWakeupChain(idx, nq)
	nq.chainAnchorWindowsByPID = chainAnchorWindowsByPID(chain)
	stats := ComputeWindowStats(idx, nq)
	bundle := collect(buildCriticalBlockingCallsFromStats(idx, nq, stats, &chain))

	// Direct exported entrance.
	direct := collect(BuildCriticalBlockingCalls(idx, q))

	// Run view entrance.
	vq := q
	vq.View = "critical_blocking_calls"
	runRes := Run(idx, vq)
	if runRes.CriticalBlocking == nil {
		t.Fatalf("Run view entrance produced no critical blocking result")
	}
	runFace := collect(*runRes.CriticalBlocking)

	if len(bundle) == 0 {
		t.Fatalf("fixture drifted: no D/IO view rows on the bundle entrance")
	}
	for key, want := range bundle {
		if got, ok := direct[key]; !ok || got != want {
			t.Fatalf("direct entrance verdict fork on %s: direct=%v bundle=%v", key, got, want)
		}
		if got, ok := runFace[key]; !ok || got != want {
			t.Fatalf("Run view entrance verdict fork on %s: run=%v bundle=%v", key, got, want)
		}
	}
	// Non-vacuity: the reference itself must hold the three adjudicated
	// shapes (disjoint demote / segment-verified keep / envelope keep) —
	// the exact fork material.
	worker := bundle["d_state_or_io_wait|worker"]
	if worker[0] != "adjacent" || worker[1] != true || worker[2] != true {
		t.Fatalf("fixture drifted: worker must demote segment-disjoint: %v", worker)
	}
	helper := bundle["d_state_or_io_wait|helper"]
	if helper[0] != "on_chain" || helper[3] != "1.065000..1.072000" {
		t.Fatalf("fixture drifted: helper must keep segment-verified: %v", helper)
	}
	env := bundle["d_state_or_io_wait|env"]
	if env[0] != "on_chain" || env[4] != true {
		t.Fatalf("fixture drifted: env must keep envelope-tier: %v", env)
	}
}

// TestONCHAINFIX2SelfHealConvergence — the FromStats residual-caller self-heal
// arm (收敛加固修复轮, 2026-07-18; the adversarial-review fourth/fifth-form
// probes adopted as pins): an anchor-less sweep handed to
// buildCriticalBlockingCallsFromStats must converge to the bundle reference
// verdict face on BOTH residual shapes —
//
//	fourth form:  nil chain + plain stats (the heal builds the chain and
//	              re-sweeps anchored);
//	fifth form:   chain supplied + anchor-less stats (pre-fix the
//	              cachedChain==nil clause gated the heal off exactly here and
//	              the four arms were skipped whole; the heal now takes the
//	              anchors straight from the supplied chain — zero extra build).
func TestONCHAINFIX2SelfHealConvergence(t *testing.T) {
	idx := hullcredTraceIndex(t)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.3, MaxDepth: 4, MaxBranches: 8, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16}
	nq := normalizeQuery(idx, q)
	chain := BuildWakeupChain(idx, nq)

	// Reference: the anchored chain-first sweep (the bundle shape).
	anchored := nq
	anchored.chainAnchorWindowsByPID = chainAnchorWindowsByPID(chain)
	reference := onchainfix2CollectDioVerdicts(
		buildCriticalBlockingCallsFromStats(idx, anchored, ComputeWindowStats(idx, anchored), &chain))
	if len(reference) == 0 {
		t.Fatalf("fixture drifted: no D/IO view rows on the reference entrance")
	}
	// Non-vacuity: the reference must hold the adjudicated fork material —
	// without the four arms the worker row keeps the bare legacy ⛓ lane.
	if worker := reference["d_state_or_io_wait|worker"]; worker[0] != "adjacent" || worker[2] != true {
		t.Fatalf("fixture drifted: worker must demote segment-disjoint on the reference: %v", worker)
	}

	assertConverges := func(form string, face map[string][6]interface{}) {
		t.Helper()
		if len(face) != len(reference) {
			t.Fatalf("%s: row census fork: %d rows vs reference %d", form, len(face), len(reference))
		}
		for key, want := range reference {
			if got, ok := face[key]; !ok || got != want {
				t.Fatalf("%s: verdict fork on %s: got=%v reference=%v", form, key, got, want)
			}
		}
	}
	// Fourth form: nil chain + plain (anchor-less) stats.
	assertConverges("fourth form (nil chain + plain stats)",
		onchainfix2CollectDioVerdicts(buildCriticalBlockingCallsFromStats(idx, nq, ComputeWindowStats(idx, nq), nil)))
	// Fifth form: chain supplied + anchor-less stats.
	assertConverges("fifth form (chain supplied + anchor-less stats)",
		onchainfix2CollectDioVerdicts(buildCriticalBlockingCallsFromStats(idx, nq, ComputeWindowStats(idx, nq), &chain)))
}

// --- 件3: proven-lower-bound prefix arms ---------------------------------------

// onchainfix2TruncatedRow builds a latched row whose geometry-bearing HEAD
// segments are the given ones, padded with far-tail filler µ-segments (inside
// [1.041, 1.048) — outside every test anchor window, Σ≈3.1ms) up to exactly
// CriticalBlockingCredentialSegmentCap: the 件4 修复轮 validator enforces the
// production invariant latch ⇒ len==cap, so a legal truncated fixture must
// carry the FULL checked prefix.
func onchainfix2TruncatedRow(hullStart, hullEnd, durationMs float64, segs ...foldInterval) CriticalBlockingCandidate {
	full := append([]foldInterval{}, segs...)
	for i := 0; len(full) < CriticalBlockingCredentialSegmentCap; i++ {
		start := 1.041 + float64(i)*0.0002
		full = append(full, foldInterval{start: start, end: start + 0.0001})
	}
	return CriticalBlockingCandidate{Type: "d_state_or_io_wait", DurationMs: durationMs,
		reconStartTs: hullStart, reconEndTs: hullEnd,
		credentialSegments: full, credentialSegmentsTruncated: true}
}

func TestONCHAINFIX2PartialPrefixArms(t *testing.T) {
	windows := []TimeWindow{{StartTs: 1.000, EndTs: 1.010}}
	credentialed := rspaFamilyDecision{migrate: true, anchoredMs: 5.0, fullMs: 8.0}
	// ① prefix proves presence: ≥1 prefix segment intersects → keep ⛓ with
	// the published prefix AND the truncated lower-bound marker.
	proving := onchainfix2TruncatedRow(0.990, 1.050, 80.0,
		foldInterval{start: 1.005, end: 1.008}, foldInterval{start: 1.020, end: 1.025})
	verdict, segs, truncated := criticalBlockingDioRowCredentialVerdict(proving, credentialed, windows)
	if verdict != dioCredentialKeepSegmentVerified || len(segs) != CriticalBlockingCredentialSegmentCap || !truncated {
		t.Fatalf("① intersecting prefix must keep segment-verified publishing the full checked prefix with the truncated marker: %d %d %v", verdict, len(segs), truncated)
	}
	// ② prefix proves NOTHING: all prefix segments disjoint → the envelope
	// tier, never the disjoint demotion, and no publication (a list that
	// proved nothing must not pose as a credential) — 缺证≠证无.
	unproving := onchainfix2TruncatedRow(0.990, 1.050, 80.0,
		foldInterval{start: 0.990, end: 0.995}, foldInterval{start: 1.040, end: 1.045})
	verdict, segs, truncated = criticalBlockingDioRowCredentialVerdict(unproving, credentialed, windows)
	if verdict != dioCredentialKeepEnvelope || segs != nil || truncated {
		t.Fatalf("② non-intersecting prefix must fall to the envelope tier with no publication: %d %v %v", verdict, segs, truncated)
	}
	// ③ Σ containment: a prefix whose Σ exceeds the account is invalid
	// (前缀不得超账) → envelope tier.
	overClaim := onchainfix2TruncatedRow(0.990, 1.050, 3.0,
		foldInterval{start: 1.005, end: 1.008}, foldInterval{start: 1.020, end: 1.025})
	if verdict, segs, _ = criticalBlockingDioRowCredentialVerdict(overClaim, credentialed, windows); verdict != dioCredentialKeepEnvelope || segs != nil {
		t.Fatalf("③ over-claiming prefix must fall to the envelope tier: %d %v", verdict, segs)
	}
	// ④ the hull-∅ arm outranks the prefix on both sides (the hull contains
	// every segment INCLUDING the uncollected ones, so ∅ stays a complete
	// disjointness proof even on a truncated row).
	empty := onchainfix2TruncatedRow(1.020, 1.050, 80.0,
		foldInterval{start: 1.020, end: 1.025})
	if verdict, segs, _ = criticalBlockingDioRowCredentialVerdict(empty, credentialed, windows); verdict != dioCredentialDemoteLegacy || segs != nil {
		t.Fatalf("④ hull-∅ must stay the legacy demote arm on truncated rows: %d %v", verdict, segs)
	}
}

// TestONCHAINFIX2PartialPrefixNeverProvesAbsence — the forbidden shape pin:
// no truncated inventory may ever mint the segment-disjoint demotion, no
// matter its geometry (缺证≠证无 is a hard rule of the verdict machine).
func TestONCHAINFIX2PartialPrefixNeverProvesAbsence(t *testing.T) {
	windows := []TimeWindow{{StartTs: 1.000, EndTs: 1.010}}
	credentialed := rspaFamilyDecision{migrate: true, anchoredMs: 5.0, fullMs: 8.0}
	shapes := []CriticalBlockingCandidate{
		onchainfix2TruncatedRow(0.990, 1.050, 80.0,
			foldInterval{start: 0.990, end: 0.995}, foldInterval{start: 1.040, end: 1.045}),
		onchainfix2TruncatedRow(0.990, 1.050, 80.0,
			foldInterval{start: 0.990, end: 0.999}),
	}
	for i, item := range shapes {
		verdict, _, _ := criticalBlockingDioRowCredentialVerdict(item, credentialed, windows)
		if verdict == dioCredentialDemoteSegmentDisjoint {
			t.Fatalf("shape %d: a truncated prefix minted the disjoint demotion (缺证判无 — forbidden)", i)
		}
	}
}

// TestONCHAINFIX2TruncatedShortListIllegal — 件4 修复轮 negative arm: the
// production latch only ever drops once the prefix FILLED the cap (the source
// appends while len < cap and latches after), so a SHORT list wearing the
// latch is a foreign/corrupt shape — the validator must reject it to the
// envelope tier (never adjudicated, never published) even when its segments
// would otherwise prove an intersection.
func TestONCHAINFIX2TruncatedShortListIllegal(t *testing.T) {
	windows := []TimeWindow{{StartTs: 1.000, EndTs: 1.010}}
	credentialed := rspaFamilyDecision{migrate: true, anchoredMs: 5.0, fullMs: 8.0}
	short := CriticalBlockingCandidate{Type: "d_state_or_io_wait", DurationMs: 80.0,
		reconStartTs: 0.990, reconEndTs: 1.050,
		credentialSegments:          []foldInterval{{start: 1.005, end: 1.008}},
		credentialSegmentsTruncated: true}
	if segs, truncated := criticalBlockingCredentialSegmentInventory(short); segs != nil || truncated {
		t.Fatalf("a short latched list must fail inventory validation: %v %v", segs, truncated)
	}
	verdict, segs, truncated := criticalBlockingDioRowCredentialVerdict(short, credentialed, windows)
	if verdict != dioCredentialKeepEnvelope || segs != nil || truncated {
		t.Fatalf("a short latched list must fall to the envelope tier with no publication: %d %v %v", verdict, segs, truncated)
	}
	// Boundary control: the exact-cap latched prefix (the one production
	// shape) stays legal and adjudicates.
	full := onchainfix2TruncatedRow(0.990, 1.050, 80.0, foldInterval{start: 1.005, end: 1.008})
	if len(full.credentialSegments) != CriticalBlockingCredentialSegmentCap {
		t.Fatalf("fixture drifted: the control row must carry exactly cap segments, got %d", len(full.credentialSegments))
	}
	if verdict, segs, truncated = criticalBlockingDioRowCredentialVerdict(full, credentialed, windows); verdict != dioCredentialKeepSegmentVerified || len(segs) != CriticalBlockingCredentialSegmentCap || !truncated {
		t.Fatalf("the exact-cap latched prefix must stay adjudicable: %d %d %v", verdict, len(segs), truncated)
	}
}

// TestONCHAINFIX2PrefixKeepEndToEnd — production path: a beyond-cap D group
// whose FIRST-cap prefix holds an anchored segment keeps ⛓ publishing the
// prefix with the truncated marker. Under the pre-fix all-or-nothing source
// the whole inventory dropped and this row could only reach the envelope
// tier — the thrown-away credential the Q6 ruling recovered.
func TestONCHAINFIX2PrefixKeepEndToEnd(t *testing.T) {
	var sb strings.Builder
	line := func(format string, args ...interface{}) {
		sb.WriteString("\t")
		sb.WriteString(strings.TrimSpace(fmt.Sprintf(format, args...)))
		sb.WriteString("\n")
	}
	// app blocks [1.100, 1.110]; env ends the wait → env's anchor window.
	line("app-100 (100) [001] .... 1.001000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52")
	line("app-100 (100) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120")
	// env D segment 0 [1.102, 1.104] INSIDE the anchor window (prefix head).
	line("env-600 (600) [005] .... 1.102000: sched_switch: prev_comm=env prev_pid=600 prev_prio=40 prev_state=D ==> next_comm=idle/5 next_pid=0 next_prio=120")
	line("other-300 (300) [003] .... 1.104000: sched_wakeup: comm=env pid=600 prio=40 target_cpu=005")
	line("env-600 (600) [005] .... 1.105000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=env next_pid=600 next_prio=40")
	line("env-600 (600) [005] .... 1.110000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001")
	line("app-100 (100) [001] .... 1.111000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52")
	// env D segments 1..(cap+1) after the window → beyond-cap group.
	for i := 0; i < CriticalBlockingCredentialSegmentCap+1; i++ {
		base := 1.112 + float64(i)*0.004
		line("env-600 (600) [005] .... %.6f: sched_switch: prev_comm=env prev_pid=600 prev_prio=40 prev_state=D ==> next_comm=idle/5 next_pid=0 next_prio=120", base)
		line("other-300 (300) [003] .... %.6f: sched_wakeup: comm=env pid=600 prio=40 target_cpu=005", base+0.002)
		line("env-600 (600) [005] .... %.6f: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=env next_pid=600 next_prio=40", base+0.003)
	}
	line("app-100 (100) [001] .... 1.280000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120")
	idx := buildTraceIndex(t, "ofix2_prefix_keep.systrace", "\n"+sb.String())
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.3, MaxDepth: 4, MaxBranches: 8, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16}
	res := BuildCriticalBlockingCalls(idx, q)
	for _, item := range res.Items {
		if item.Thread.PID != 600 || (item.Type != "d_state_or_io_wait" && item.Type != "io_wait") {
			continue
		}
		if item.ChainRelevance != "on_chain" || item.ChainCredentialLaneDemoted || item.ChainCredentialSegmentDisjoint {
			t.Fatalf("env-600 must keep ⛓ on its anchored prefix segment: %+v", item)
		}
		if len(item.ChainCredentialSegments) != CriticalBlockingCredentialSegmentCap {
			t.Fatalf("env-600 must publish its full checked prefix (%d segments), got %d",
				CriticalBlockingCredentialSegmentCap, len(item.ChainCredentialSegments))
		}
		if item.ChainCredentialSegments[0] != "1.102000..1.104000" {
			t.Fatalf("prefix head must be the anchored segment: %+v", item.ChainCredentialSegments[0])
		}
		if !item.ChainCredentialSegmentsTruncated {
			t.Fatalf("the published prefix must wear the truncated lower-bound marker: %+v", item)
		}
		if item.ChainCredentialEnvelopeLevel {
			t.Fatalf("a prefix-verified keep must not wear the envelope word: %+v", item)
		}
		return
	}
	t.Fatalf("env-600 D view row missing: %+v", res.Items)
}

// --- 件4: dioSegmentIntervals carrier + AXIOM basis ----------------------------

// TestONCHAINFIX2DioSegmentCarrier — the formal D/IO seats push their member
// ledgers' true segments down (validated all-or-nothing); the overflowed
// group carries nothing; the AXIOM direction-support resolver reads the new
// closed-set basis.
func TestONCHAINFIX2DioSegmentCarrier(t *testing.T) {
	idx := hullcredTraceIndex(t)
	q := Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.3, MaxDepth: 4, MaxBranches: 8, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 32}
	rank := BuildRootCauseRank(idx, q)
	pool := append(append([]RootCauseRankItem{}, rank.Items...), rank.AbsorbedItems...)
	// EVOLUTION RECORD (STATERES-1, §40.30 V-STATE-1 plan A, 2026-09-02): the
	// worker (a chain member whose D segments 1.002..1.012 / 1.032..1.045 lie
	// outside its chain windows) now takes the R3 credential of its DIRECT
	// edge toward the target at 1.030: the pre-edge segment rides the ⛓ seat
	// (10ms, via=direct) and the post-edge segment rides the ◇ clone (13ms).
	// The carrier promise is unchanged — the two rows of one account together
	// carry exactly the 2 true close-site segments, each Σ reproducing its own
	// published account.
	var worker, remainder, env *RootCauseRankItem
	for i := range pool {
		item := &pool[i]
		if item.Type != "d_state_or_io_wait" && item.Type != "io_wait" {
			continue
		}
		if !strings.HasPrefix(item.Source, "window_stats") {
			continue
		}
		switch item.Thread.PID {
		case 200:
			if item.ChainAnchorRemainderSeat {
				remainder = item
			} else {
				worker = item
			}
		case 500:
			env = item
		}
	}
	if worker == nil || remainder == nil {
		t.Fatalf("worker D/IO ⛓ seat and ◇ clone must both be present (worker=%v remainder=%v)", worker != nil, remainder != nil)
	}
	if worker.OnChainBasis != RootCauseOnChainBasisHostWakeupEdgeState || worker.HostWakeupEdgeAnchorVia != HostWakeupEdgeAnchorViaDirect {
		t.Fatalf("worker ⛓ seat must wear the direct-edge state credential: %+v", worker)
	}
	if len(worker.dioSegmentIntervals) != 1 || len(remainder.dioSegmentIntervals) != 1 {
		t.Fatalf("the two rows must carry the 2 true close-site segments (1 pre-edge + 1 post-edge), got %d/%d: %+v %+v",
			len(worker.dioSegmentIntervals), len(remainder.dioSegmentIntervals), worker.dioSegmentIntervals, remainder.dioSegmentIntervals)
	}
	for _, row := range []*RootCauseRankItem{worker, remainder} {
		segMs := 0.0
		for _, seg := range row.dioSegmentIntervals {
			segMs += (seg.end - seg.start) * 1000
		}
		if math.Abs(segMs-(row.DStateMs+row.IOWaitMs)) > rspaAnchorIdentityTolMs {
			t.Fatalf("segment Σ must reproduce the row account: %.6f vs %.6f", segMs, row.DStateMs+row.IOWaitMs)
		}
	}
	intervals, basis := rootCauseItemDirectionSupport(worker)
	if basis != RootCauseDirectionBasisDioSegments || len(intervals) != 1 {
		t.Fatalf("direction support must resolve the dio segment basis on the priced share: %q %d", basis, len(intervals))
	}
	if env != nil && len(env.dioSegmentIntervals) != 0 {
		t.Fatalf("the overflowed env group must carry NO seat inventory (fail-open): %+v", env.dioSegmentIntervals)
	}
}

// TestONCHAINFIX2DioSegmentCarrierFailOpenArms — unit negative arms: slice
// members, overflowed ledgers, Σ mismatches and the MAX-fallback caliber all
// leave the carrier absent.
func TestONCHAINFIX2DioSegmentCarrierFailOpenArms(t *testing.T) {
	td := ThreadDuration{Thread: ThreadRef{Comm: "w", PID: 9}, DurationMs: 4.0,
		dioIntervals: []foldInterval{{start: 1.0, end: 1.002}, {start: 1.01, end: 1.012}}}
	member := dioStateMemberFromTd(string(StateDSleep), td, "")
	mint := func(members []dioStateFamilyMember, producerDisjoint bool) RootCauseRankItem {
		return mintRootCauseDIOStateSeat(Query{}, WindowStats{}, false, producerDisjoint,
			td.Thread, false, members, "", false)
	}
	// Positive control (whole-td, sum-disjoint, Σ identity).
	if seat := mint([]dioStateFamilyMember{member}, true); len(seat.dioSegmentIntervals) != 2 {
		t.Fatalf("positive control must carry the segments: %+v", seat.dioSegmentIntervals)
	}
	// ① slice member (cause partition) — cannot attribute the group segments.
	slice := member
	slice.wholeTd = false
	if seat := mint([]dioStateFamilyMember{slice}, true); len(seat.dioSegmentIntervals) != 0 {
		t.Fatalf("① slice member must leave the carrier absent")
	}
	// ② overflowed ledger — a truncated prefix cannot re-derive the account.
	overflowTd := td
	overflowTd.dioIntervalsOverflow = true
	if seat := mint([]dioStateFamilyMember{dioStateMemberFromTd(string(StateDSleep), overflowTd, "")}, true); len(seat.dioSegmentIntervals) != 0 {
		t.Fatalf("② overflowed ledger must leave the carrier absent")
	}
	// ③ Σ mismatch — the inventory cannot reproduce the member account.
	badTd := td
	badTd.DurationMs = 9.0
	if seat := mint([]dioStateFamilyMember{dioStateMemberFromTd(string(StateDSleep), badTd, "")}, true); len(seat.dioSegmentIntervals) != 0 {
		t.Fatalf("③ Σ-mismatched ledger must leave the carrier absent")
	}
	// ④ MAX-fallback caliber (producerDisjoint=false) — the seat value is a
	// lower bound the segment Σ would over-claim.
	if seat := mint([]dioStateFamilyMember{member, member}, false); len(seat.dioSegmentIntervals) != 0 {
		t.Fatalf("④ MAX-fallback fold must leave the carrier absent")
	}
}

// --- 件1: rank-lane envelope tier ----------------------------------------------

// TestONCHAINFIX2EnvelopeTierClassifier — the per-type disposition arms of
// rootCauseHullKeepIsEnvelopeTier (逐类型负臂; PRECISE typed signals only).
func TestONCHAINFIX2EnvelopeTierClassifier(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	base := func(typ string) RootCauseRankItem {
		return RootCauseRankItem{Type: typ, Thread: ThreadRef{Comm: "w", PID: 200},
			ChainRelevance: "on_chain", StartTs: 1.0, EndTs: 1.010,
			ImpactMs: 2.0, CumulativeImpactMs: 2.0, Source: "window_stats"}
	}
	// ① envelope wearer: an aggregate-envelope resource row (value ≠ envelope
	// length, no typed credential) — the 「裸 hull∩」 audit shape.
	if !rootCauseHullKeepIsEnvelopeTier(base("io_burst_episode"), target) {
		t.Fatalf("① identity-broken io_burst_episode keep must wear the envelope word")
	}
	// ② single-segment µs identity: value fills the envelope → the hull IS
	// the one segment (io_latency single-record shape).
	ident := base("io_latency")
	ident.CumulativeImpactMs = 10.0
	ident.ImpactMs = 10.0
	if rootCauseHullKeepIsEnvelopeTier(ident, target) {
		t.Fatalf("② µs-identity io_latency must NOT wear the envelope word")
	}
	// ③ M-IO closure typed credential.
	closure := base("io_latency")
	closure.resourceClosureEvaluated = true
	closure.ResourceCompletionClosure = true
	if rootCauseHullKeepIsEnvelopeTier(closure, target) {
		t.Fatalf("③ closure-credentialed io_latency must NOT wear the envelope word")
	}
	// ④ host-window containment typed credential (superset proof).
	contained := base("block_io_by_inode")
	contained.resourceHostContainmentEvaluated = true
	contained.resourceHostWindowContained = true
	if rootCauseHullKeepIsEnvelopeTier(contained, target) {
		t.Fatalf("④ containment-credentialed row must NOT wear the envelope word")
	}
	// ⑤ RSPA-owned types: the re-anchoring machinery owns their vocabulary.
	for _, typ := range []string{"runnable_wait", "d_state_or_io_wait", "io_wait",
		"scheduler_latency", "low_frequency", "cpu_affinity_or_cpuset",
		"priority_inversion_runnable_wait", "fragmented_runnable_wait", "fragmented_d_state_or_io_wait"} {
		if rootCauseHullKeepIsEnvelopeTier(base(typ), target) {
			t.Fatalf("⑤ RSPA-owned type %s must NOT wear the envelope word", typ)
		}
		if !rspaReanchorOwnedType(typ) {
			t.Fatalf("⑤ closed-set drift: %s missing from rspaReanchorOwnedType", typ)
		}
	}
	// ⑥ semantic span work: mint-time exact-intersection lane.
	if rootCauseHullKeepIsEnvelopeTier(base("jit_compile"), target) {
		t.Fatalf("⑥ semantic span row must NOT wear the envelope word")
	}
	// ⑦ blocking_span: typed resolved-pair lane.
	if rootCauseHullKeepIsEnvelopeTier(base("blocking_span"), target) {
		t.Fatalf("⑦ blocking_span must NOT wear the envelope word")
	}
	// ⑧ constructive wakeup_chain source.
	constructive := base("io_burst_episode")
	constructive.Source = "wakeup_chain.causal_impacts"
	if rootCauseHullKeepIsEnvelopeTier(constructive, target) {
		t.Fatalf("⑧ wakeup_chain-sourced row must NOT wear the envelope word")
	}
	// ⑨ the analysis target's own rows (self-causality lanes own them).
	self := base("io_burst_episode")
	self.Thread = target
	if rootCauseHullKeepIsEnvelopeTier(self, target) {
		t.Fatalf("⑨ target self row must NOT wear the envelope word")
	}
	// ⑩ interval-less rows: the identity-inheritance word owns them.
	nowin := base("io_burst_episode")
	nowin.StartTs, nowin.EndTs = 0, 0
	if rootCauseHullKeepIsEnvelopeTier(nowin, target) {
		t.Fatalf("⑩ interval-less row must NOT wear the envelope word (identity word owns it)")
	}
	// ⑪ non-on-chain / basis-bearing rows.
	adj := base("io_burst_episode")
	adj.ChainRelevance = "adjacent"
	if rootCauseHullKeepIsEnvelopeTier(adj, target) {
		t.Fatalf("⑪ off-chain row must NOT wear the envelope word")
	}
	basis := base("io_burst_episode")
	basis.OnChainBasis = RootCauseOnChainBasisHostWakeupEdge
	if rootCauseHullKeepIsEnvelopeTier(basis, target) {
		t.Fatalf("⑪ basis-bearing row must NOT wear the envelope word")
	}
}

// TestONCHAINFIX2EnvelopeTierStamp — the enrich pass stamps the flag on a
// hull-kept aggregate-envelope row and leaves the µs-identity sibling clean
// (production enrich path; the stamp is an unconditional recompute).
func TestONCHAINFIX2EnvelopeTierStamp(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{Comm: "app", PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{Comm: "app", PID: 100}, Window: TimeWindow{StartTs: 1.0, EndTs: 1.2}},
			{Thread: ThreadRef{Comm: "worker", PID: 200}, Window: TimeWindow{StartTs: 1.0, EndTs: 1.05}, Depth: 1},
		},
		Edges: []WakeupEdge{{Waker: ThreadRef{Comm: "worker", PID: 200}, Wakee: ThreadRef{Comm: "app", PID: 100}, WakeupTs: 1.05}},
	}
	items := []RootCauseRankItem{
		{Type: "io_burst_episode", Thread: ThreadRef{Comm: "worker", PID: 200},
			StartTs: 1.0, EndTs: 1.04, ImpactMs: 2.0, CumulativeImpactMs: 2.0,
			Source: "window_stats.io_burst", resourceExactChainHostWork: true},
		{Type: "io_latency", Thread: ThreadRef{Comm: "worker", PID: 200},
			StartTs: 1.0, EndTs: 1.01, ImpactMs: 10.0, CumulativeImpactMs: 10.0,
			Source: "window_stats.io_latency"},
	}
	items = enrichRootCauseItemsWithChainContext(chain, items)
	if items[0].ChainRelevance != "on_chain" || !items[0].ChainCredentialEnvelopeLevel {
		t.Fatalf("hull-kept envelope row must wear the flag: %+v", items[0])
	}
	if items[1].ChainRelevance != "on_chain" || items[1].ChainCredentialEnvelopeLevel {
		t.Fatalf("µs-identity row must stay clean: %+v", items[1])
	}
	// Idempotency + clearing: a second pass over a row whose lane moved must
	// clear the stamp (unconditional recompute, never |=).
	items[0].ChainRelevance = "adjacent"
	items = enrichRootCauseItemsWithChainContext(chain, items)
	if items[0].ChainRelevance == "on_chain" && !items[0].ChainCredentialEnvelopeLevel {
		t.Fatalf("re-kept row must re-stamp")
	}
	if items[0].ChainRelevance != "on_chain" && items[0].ChainCredentialEnvelopeLevel {
		t.Fatalf("off-lane row must not keep the stamp")
	}
}

// TestONCHAINFIX2EnvelopeTierStampAssignSemantics — the TRUE assign pin
// (修复轮, 2026-07-18): the stamp is an unconditional recompute, so a
// pre-seeded flag on a row that does NOT qualify must CLEAR on enrich. The
// prior clearing assertion was vacuous — the enrich pass re-decides the lane,
// so its manually-demoted row flipped back on-chain and the off-lane branch
// never executed; a sticky |= stamp stayed green. Two non-qualifying forms
// carry the pre-seeded flag here:
//
//	① a row enrich itself keeps OFF-lane (thread outside the chain →
//	   background) — the stale-stamp-after-lane-move shape;
//	② an ON-chain row owned by a stronger tier (single-segment µs identity)
//	   — recompute-to-false on a row that stays on the ⛓ lane.
//
// A |= (or any keep-if-set) mutation of the stamp reds both arms.
func TestONCHAINFIX2EnvelopeTierStampAssignSemantics(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{Comm: "app", PID: 100},
		Nodes: []ChainNode{
			{Thread: ThreadRef{Comm: "app", PID: 100}, Window: TimeWindow{StartTs: 1.0, EndTs: 1.2}},
			{Thread: ThreadRef{Comm: "worker", PID: 200}, Window: TimeWindow{StartTs: 1.0, EndTs: 1.05}, Depth: 1},
		},
		Edges: []WakeupEdge{{Waker: ThreadRef{Comm: "worker", PID: 200}, Wakee: ThreadRef{Comm: "app", PID: 100}, WakeupTs: 1.05}},
	}
	items := []RootCauseRankItem{
		// ① pre-seeded stamp on a thread with NO chain membership.
		{Type: "io_burst_episode", Thread: ThreadRef{Comm: "far", PID: 999},
			StartTs: 1.0, EndTs: 1.04, ImpactMs: 2.0, CumulativeImpactMs: 2.0,
			Source: "window_stats.io_burst", ChainCredentialEnvelopeLevel: true},
		// ② pre-seeded stamp on the µs-identity on-chain sibling.
		{Type: "io_latency", Thread: ThreadRef{Comm: "worker", PID: 200},
			StartTs: 1.0, EndTs: 1.01, ImpactMs: 10.0, CumulativeImpactMs: 10.0,
			Source: "window_stats.io_latency", ChainCredentialEnvelopeLevel: true},
	}
	items = enrichRootCauseItemsWithChainContext(chain, items)
	if items[0].ChainRelevance == "on_chain" {
		t.Fatalf("fixture drifted: the far row must stay off the chain lane: %+v", items[0])
	}
	if items[0].ChainCredentialEnvelopeLevel {
		t.Fatalf("① off-lane row kept its pre-seeded stamp (sticky |= — the stamp must recompute): %+v", items[0])
	}
	if items[1].ChainRelevance != "on_chain" {
		t.Fatalf("fixture drifted: the µs-identity row must hold the chain lane: %+v", items[1])
	}
	if items[1].ChainCredentialEnvelopeLevel {
		t.Fatalf("② µs-identity on-chain row kept its pre-seeded stamp (sticky |= — the stronger tier owns it): %+v", items[1])
	}
}
