package tracequery

// onchain_fix1_test.go — ONCHAIN-FIX-1 pins (mint audit 命题2 不一致① + 命题1
// 残口, docs/design/onchain_mint_audit_20260718.md, 2026-07-18):
//
//   件1 — the interval-less same-pid arm of chainContextForCandidate keeps the
//   on-chain lane (fail-open 既裁: credential-less shapes are never guessed
//   off the chain) but must NOT fabricate overlapMs from the whole node-window
//   wall clock. 双向 pin: the pre-fix fabricated form (overlap == 节点窗长)
//   turns the zero-value assertions below RED; the post-fix form mints zero +
//   the typed identity-inheritance admission record. Interval-bearing rows
//   keep their measured overlap byte-identically; the analysis target's own
//   rows never carry the marker (R8 self-causality); HULL-CRED adjudicated
//   rows retire it (adjudication vocabulary wins).
//
//   件2 — the target's own RootEvidence wait seat (runnable_wait / io_wait /
//   d_state_or_io_wait) speaks the honest self token (self_wall_clock, no
//   basis — the ELIM-SELF-FIX 件1③ channel-identity-only form) instead of the
//   legacy fabricated cross-thread "on_wakeup_chain" claim; non-target rows
//   keep their legacy word faces byte-identically, and the fallback causal
//   thread set keeps the target's membership (词面改动不迁值).

import (
	"math"
	"testing"
)

func onchainFix1Chain() ChainResult {
	target := ThreadRef{Comm: "app", PID: 100}
	worker := ThreadRef{Comm: "worker", PID: 200}
	return ChainResult{
		Target: target,
		Nodes: []ChainNode{
			{ID: "target", Thread: target, Window: TimeWindow{StartTs: 1.0, EndTs: 1.1}},
			{ID: "dep", Thread: worker, Window: TimeWindow{StartTs: 1.0, EndTs: 1.1}, Depth: 1},
		},
		Edges: []WakeupEdge{{Waker: worker, Wakee: target, WakeupTs: 1.05}},
	}
}

// TestONCHAINFIX1IntervalLessSamePidArmMintsNoOverlap — the mint-site 双向
// pin: the interval-less same-pid arm keeps on_chain, mints ZERO overlap (the
// pre-fix fabricated form was the whole 100ms node window) and raises the
// typed admission record; an interval-bearing row keeps its measured overlap
// with no marker.
func TestONCHAINFIX1IntervalLessSamePidArmMintsNoOverlap(t *testing.T) {
	chain := onchainFix1Chain()
	ctx := chainContextForCandidate(chain, ThreadRef{Comm: "worker", PID: 200}, 0, 0)
	if ctx.relevance != "on_chain" {
		t.Fatalf("the fail-open identity keep must stay on the chain lane: %+v", ctx)
	}
	if ctx.overlapMs != 0 {
		t.Fatalf("interval-less rows must mint NO overlap (pre-fix fabricated form = 100.000ms node window): got %.3f", ctx.overlapMs)
	}
	if !ctx.identityInheritance {
		t.Fatalf("the typed identity-inheritance admission record must ride the arm: %+v", ctx)
	}
	// Interval-bearing negative arm: measured overlap byte-identical, no marker.
	measured := chainContextForCandidate(chain, ThreadRef{Comm: "worker", PID: 200}, 1.02, 1.05)
	if measured.relevance != "on_chain" || math.Abs(measured.overlapMs-30.0) > 1e-6 {
		t.Fatalf("interval-bearing rows keep their measured overlap: %+v", measured)
	}
	if measured.identityInheritance {
		t.Fatalf("a measured-overlap row must not wear the admission record: %+v", measured)
	}
	// Same-pid interval outside every chain window: the honest adjacent arm
	// stays byte-identical (overlap 0, no marker, no lane claim).
	outside := chainContextForCandidate(chain, ThreadRef{Comm: "worker", PID: 200}, 1.2, 1.25)
	if outside.relevance != "adjacent" || outside.overlapMs != 0 || outside.identityInheritance {
		t.Fatalf("the no-overlap adjacent arm must stay byte-identical: %+v", outside)
	}
}

// TestONCHAINFIX1RankEnrichStampsAdmissionRecord — the rank enrich stamp: a
// non-target interval-less closed-list row carries the marker with zero
// overlap; the target's own row is exempt (R8); an interval-less RESOURCE row
// keeps the 17985 demote arm byte-identically (adjacent, zero overlap, no
// marker).
func TestONCHAINFIX1RankEnrichStampsAdmissionRecord(t *testing.T) {
	chain := onchainFix1Chain()
	items := []RootCauseRankItem{
		rootCauseItem("binder_wait", ThreadRef{Comm: "worker", PID: 200}, 5, 0.8, 10, 12, "wakeup_chain", "worker binder wait"),
		rootCauseItem("binder_wait", ThreadRef{Comm: "app", PID: 100}, 5, 0.8, 14, 16, "wakeup_chain", "self binder wait"),
		rootCauseItem("io_latency", ThreadRef{Comm: "worker", PID: 200}, 5, 0.8, 20, 22, "io_latency_stats", "worker io latency"),
	}
	items = enrichRootCauseItemsWithChainContext(chain, items)
	worker := items[0]
	if worker.ChainRelevance != "on_chain" || !worker.ChainIdentityInheritance {
		t.Fatalf("the non-target interval-less row must keep the lane WITH the admission record: %+v", worker)
	}
	if worker.OverlapMs != 0 {
		t.Fatalf("the enrich stamp must not publish a fabricated overlap: %+v", worker)
	}
	self := items[1]
	if self.ChainIdentityInheritance {
		t.Fatalf("R8: the analysis target's own row never wears identity inheritance: %+v", self)
	}
	resource := items[2]
	if resource.ChainRelevance != "adjacent" || resource.ChainIdentityInheritance || resource.OverlapMs != 0 {
		t.Fatalf("the interval-less resource demote arm must stay byte-identical: %+v", resource)
	}
}

// TestONCHAINFIX1CriticalBlockingEnrichStampsAdmissionRecord — the
// critical_blocking enrich (the main fabricated-overlap face: D/IO VIEW rows
// publish no StartTs/EndTs): zero overlap + marker for non-target rows, target
// exempt.
func TestONCHAINFIX1CriticalBlockingEnrichStampsAdmissionRecord(t *testing.T) {
	chain := onchainFix1Chain()
	items := enrichCriticalBlockingWithChainContext(chain, []CriticalBlockingCandidate{{
		Type: "d_state_or_io_wait", Thread: ThreadRef{Comm: "worker", PID: 200},
		DurationMs: 4, LineStart: 30, LineEnd: 31, Confidence: 0.8, Summary: "worker D wait",
	}, {
		Type: "d_state_or_io_wait", Thread: ThreadRef{Comm: "app", PID: 100},
		DurationMs: 2, LineStart: 40, LineEnd: 41, Confidence: 0.8, Summary: "self D wait",
	}})
	worker := items[0]
	if worker.Thread.PID != 200 {
		// The enrich sorts nothing here, but stay order-independent anyway.
		worker = items[1]
	}
	if worker.ChainRelevance != "on_chain" || !worker.ChainIdentityInheritance {
		t.Fatalf("the interval-less D view row must keep the lane WITH the admission record: %+v", worker)
	}
	if worker.OverlapMs != 0 {
		t.Fatalf("the D view row must not publish a fabricated overlap (pre-fix form = 100.000ms node window): %+v", worker)
	}
	for _, item := range items {
		if item.Thread.PID == 100 && item.ChainIdentityInheritance {
			t.Fatalf("R8: the target's own view row never wears identity inheritance: %+v", item)
		}
	}
}

// TestONCHAINFIX1HullcredAdjudicationRetiresAdmissionRecord — adjudicated rows
// speak the adjudication vocabulary: all three HULL-CRED end-to-end witnesses
// (disjoint demote / per-segment keep / envelope keep) clear the marker.
func TestONCHAINFIX1HullcredAdjudicationRetiresAdmissionRecord(t *testing.T) {
	rows := hullcredCriticalBlockingRows(t)
	for pid, row := range rows {
		if row.ChainIdentityInheritance {
			t.Fatalf("pid %d: an adjudicated D/IO view row must not keep the identity-inheritance marker: %+v", pid, row)
		}
	}
}

// TestONCHAINFIX1FailOpenKeepsAdmissionRecordEndToEnd — the production-path
// fail-open witness (no anchor windows → no dioDecisions → no adjudication):
// the interval-less chain-member D view row keeps ⛓ with the marker and an
// honest ZERO overlap. Under the pre-fix shape this row published
// overlap == the whole chain-node window wall clock (伪造形 pinned RED here).
//
// EVOLUTION RECORD (ONCHAIN-FIX-2 收敛加固修复轮, 2026-07-18): the original
// construction reached the fail-open verdict by handing FromStats a supplied
// chain beside an anchor-less sweep — exactly the fifth residual-caller form
// the hardened self-heal now converges (it takes the anchors from the
// supplied chain and re-sweeps; on this geometry the adjudication then runs
// and HONESTLY demotes the post-wake D hull). The un-anchorable shape this
// witness documents is a chain whose depth>0 nodes carry no usable
// dependency windows (chainAnchorWindowsByPID → nil — e.g. unresolved jump
// windows): the self-heal finds nothing to anchor with and the fail-open
// enrich verdict IS the published row, as before.
func TestONCHAINFIX1FailOpenKeepsAdmissionRecordEndToEnd(t *testing.T) {
	idx := buildTraceIndex(t, "onchainfix1_failopen.systrace", `
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
	// The genuinely un-anchorable chain shape (see EVOLUTION RECORD above):
	// the depth>0 dependency windows are unresolved, so
	// chainAnchorWindowsByPID returns nil — the self-heal has nothing to
	// anchor with, buildRSPAFamilyDecisions fail-opens to nil and the
	// HULL-CRED adjudication block never runs — the enrich verdict IS the
	// published row.
	for i := range chain.Nodes {
		if chain.Nodes[i].Depth > 0 {
			chain.Nodes[i].Window = TimeWindow{}
		}
	}
	if chainAnchorWindowsByPID(chain) != nil {
		t.Fatalf("fixture drifted: the fail-open witness needs an un-anchorable chain")
	}
	stats := ComputeWindowStats(idx, q)
	res := buildCriticalBlockingCallsFromStats(idx, q, stats, &chain)
	var worker *CriticalBlockingCandidate
	for i := range res.Items {
		item := &res.Items[i]
		if item.Thread.PID == 200 && (item.Type == "d_state_or_io_wait" || item.Type == "io_wait") {
			worker = item
			break
		}
	}
	if worker == nil {
		t.Fatalf("worker D view row missing: %+v", res.Items)
	}
	if worker.ChainRelevance != "on_chain" {
		t.Fatalf("the fail-open keep must stay on the chain lane: %+v", worker)
	}
	if worker.OverlapMs != 0 {
		t.Fatalf("伪造形: the interval-less view row published a fabricated overlap %.3fms (pre-fix = whole node window)", worker.OverlapMs)
	}
	if !worker.ChainIdentityInheritance {
		t.Fatalf("the fail-open keep must carry the identity-inheritance admission record: %+v", worker)
	}
}

// TestONCHAINFIX1SelfRootEvidenceSpeaksSelfToken — 件2: the target's own
// RootEvidence wait seat carries the honest self token with no basis and no
// fabricated faces (zero overlap, no admission marker — self-causality is R8,
// not identity inheritance); the non-target sibling keeps its legacy word
// face byte-identically.
func TestONCHAINFIX1SelfRootEvidenceSpeaksSelfToken(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	dep := ThreadRef{Comm: "io-dep", PID: 300}
	chain := ChainResult{
		Target: target,
		Nodes: []ChainNode{
			{ID: "target", Thread: target, Window: TimeWindow{StartTs: 5.000, EndTs: 5.010}},
		},
		RootEvidence: []RootEvidence{{
			Type: "io_wait", Thread: target, DurationMs: 3,
			LineStart: 50, LineEnd: 55, Summary: "self io wait", Confidence: 0.88,
		}, {
			Type: "io_wait", Thread: dep, DurationMs: 2,
			LineStart: 60, LineEnd: 62, Summary: "dep io wait", Confidence: 0.8,
		}},
	}
	rank := buildRootCauseRankFrom(nil, Query{TimeStart: 5.000, TimeEnd: 5.010, Limit: 12}, chain, WindowStats{})
	var self, other *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Type != "io_wait" || item.Source != "wakeup_chain" {
			continue
		}
		switch item.Thread.PID {
		case target.PID:
			self = item
		case dep.PID:
			other = item
		}
	}
	if self == nil {
		t.Fatalf("the self RootEvidence seat must mint: %+v", rank.Items)
	}
	if self.Causality != RootCauseCausalitySelfWallClock {
		t.Fatalf("件2: the self seat must speak the honest self token, got causality=%q (legacy fabricated form: on_wakeup_chain)", self.Causality)
	}
	if self.ChainRelevance != "on_chain" {
		t.Fatalf("the self seat stays on the chain channel (R8): %+v", self)
	}
	if self.OnChainBasis != "" {
		t.Fatalf("件2 is the channel-identity-only form — no basis stamp: %+v", self)
	}
	if self.OverlapMs != 0 || self.ChainIdentityInheritance {
		t.Fatalf("零值面: the self seat mints no overlap and no identity-inheritance marker: %+v", self)
	}
	if other == nil {
		t.Fatalf("the non-target RootEvidence seat must mint: %+v", rank.Items)
	}
	if other.Causality == RootCauseCausalitySelfWallClock || other.Causality == RootCauseCausalitySelfDeterministic {
		t.Fatalf("负臂: a non-target RootEvidence row must never wear a self token: %+v", other)
	}
}

// TestONCHAINFIX1FallbackCausalThreadSetKeepsSelfMembership — the 件2 word
// change must not move values: the fallback causal-thread set (rankCausal-
// ThreadSet) still counts the wakeup_chain-lane self seat under its new self
// token, and still refuses non-wakeup_chain self-token rows (the SELF-basis /
// symptom lanes never fed the set).
func TestONCHAINFIX1FallbackCausalThreadSetKeepsSelfMembership(t *testing.T) {
	rank := RootCauseRankResult{Items: []RootCauseRankItem{{
		Type: "io_wait", Thread: ThreadRef{Comm: "app", PID: 100},
		Source: "wakeup_chain", Causality: RootCauseCausalitySelfWallClock,
	}, {
		Type: "runnable_wait", Thread: ThreadRef{Comm: "app", PID: 100},
		Source: "window_stats", Causality: RootCauseCausalitySelfWallClock,
	}, {
		Type: "runnable_wait", Thread: ThreadRef{Comm: "worker", PID: 200},
		Source: "wakeup_chain.causal_impacts", Causality: "on_wakeup_chain",
	}}}
	set := rankCausalThreadSet(rank)
	if !set[100] {
		t.Fatalf("the wakeup_chain-lane self seat must keep the target's membership (词面改动不迁值): %+v", set)
	}
	if !set[200] {
		t.Fatalf("legacy on_wakeup_chain rows keep contributing: %+v", set)
	}
	onlyWindowStats := RootCauseRankResult{Items: []RootCauseRankItem{{
		Type: "runnable_wait", Thread: ThreadRef{Comm: "app", PID: 100},
		Source: "window_stats", Causality: RootCauseCausalitySelfWallClock,
	}}}
	if set := rankCausalThreadSet(onlyWindowStats); len(set) != 0 {
		t.Fatalf("负臂: non-wakeup_chain self-token rows never feed the fallback set: %+v", set)
	}
}
