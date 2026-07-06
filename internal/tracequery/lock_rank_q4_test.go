package tracequery

import (
	"strings"
	"testing"
)

// lock_rank_q4_test.go — P0-E1 mutation pins for the q4 (东湖 doFrame 3703298)
// engine batch, ledger docs/design/real_trace_campaign_20260705.md §12:
//
//   Q4-A 修1  rank lock-candidate source: a structured lock-contention span
//             publishes a type=blocking_span rank row whose SUBJECT is the
//             parsed HOLDER and whose impact is the MEASURED contention
//             duration; direct on-chain admission is gated on the precise
//             typed pair (BlockingKind + resolved counterpart), 未解析不准入.
//   Q4-B      inheritance hygiene: the material gate reads the row's OWN
//             measured duration (overlap no longer admits), the inherited
//             target_blocked value rides InheritedTargetBlockedMs only, and
//             Score is re-derived from the ranking channel.
//   §12.3-2   lead tie-break (engine sort face): 实测优先 + 已解析对端行优先,
//             承自行让位.

// The q4-shaped trace: the target thread blocks on the customer's real ART
// monitor-contention payload (holder NetworkKit_AssetsUtil_Operate_0-42067,
// never scheduled in-window → off-chain, undrilled) while a worker later
// delivers the wakeup, so a causal chain exists and the rank sort runs
// chain-aware.
// The holder tid 42067 IS present in the trace (it is scheduled just AFTER the
// waiter's window, on cpu3) so the structured-payload owner is a genuine
// host-namespace-resolvable thread — the P0-E2a byte-stability witness: the
// resolution stays payload-direct (HolderSource=contention_payload) and every
// downstream face is byte-identical to the pre-P0-E2a behavior. Its only event
// sits past the query window end, so window-stats decomposition never surfaces
// it: it stays off-chain and undrilled_peer_known while remaining resolvable.
const lockRankQ4Trace = `
        app-100 (100) [001] .... 5.000000: print: B|100|` + lockContentionCustomerMonitorPayload + `
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.112000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.112223: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.112300: print: E|100
 NetworkKit_AssetsUtil_Operate_0-42067 (600) [003] .... 5.200000: sched_switch: prev_comm=NetworkKit_AssetsUtil_Operate_0 prev_pid=42067 prev_prio=120 prev_state=R ==> next_comm=idle/3 next_pid=0 next_prio=120
`

func TestRootCauseRankPublishesResolvedLockContentionAsBlockingSpanHead(t *testing.T) {
	idx := buildTraceIndex(t, "lock_rank_q4.systrace", lockRankQ4Trace)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.115, MaxDepth: 4, MinDurationMs: 0.05, Limit: 16})
	if len(rank.Items) == 0 {
		t.Fatalf("expected ranked causes")
	}
	head := rank.Items[0]
	// q4 acceptance: the measured 112.2ms lock row takes rank1 with the
	// HOLDER as the row subject.
	if head.Type != "blocking_span" {
		t.Fatalf("resolved lock contention should lead the rank, got %+v", rank.Items)
	}
	if head.Thread.PID != 42067 || head.Thread.Comm != "NetworkKit_AssetsUtil_Operate_0" {
		t.Fatalf("lock rank row subject must be the parsed holder, got %+v", head.Thread)
	}
	if head.ImpactMs < 100 || head.CumulativeImpactMs < 100 {
		t.Fatalf("lock rank row impact must be the measured contention duration, got %+v", head)
	}
	if head.BlockingKind != "monitor_contention" || !strings.Contains(head.HolderSite, "AssetManager.java:1258") {
		t.Fatalf("typed contention semantics must travel on the rank row, got %+v", head)
	}
	if head.BlockingPeer.PID != 100 {
		t.Fatalf("BlockingPeer must be the blocked waiter, got %+v", head.BlockingPeer)
	}
	// Direct on-chain admission through the typed pair: subject (holder) is
	// off-chain, the coupling runs through the waiter's critical path.
	if head.ChainRelevance != "on_chain" || head.Tier != "primary" {
		t.Fatalf("resolved lock row must be admissible as a direct on-chain primary, got %+v", head)
	}
	if !rootCauseItemCanBeDirectOnChain(head) || !rootCauseShouldBeCoPrimary(head) {
		t.Fatalf("typed admission pair must hold for the resolved lock row: %+v", head)
	}
	// Carve-out: the contention span must NOT double-publish as a generic
	// trace_span rank row.
	for _, item := range rank.Items {
		if item.Type == "trace_span" && strings.HasPrefix(item.SpanName, "monitor contention with owner ") {
			t.Fatalf("contention span must ride the typed blocking_span lane only, got %+v", item)
		}
	}
	// RCX① (§12.3 ruling 1): the holder is named but never examined by any
	// subject==holder observation in this report — the exact q4 burial shape.
	if head.DrillStatus != DrillStatusUndrilledPeerKnown {
		t.Fatalf("unexamined resolved holder must stamp undrilled_peer_known, got %+v", head)
	}
}

// RCX① engine pins: the three verdicts on the critical_blocking face plus the
// drilled transition once the holder gains its own subject observation.
func TestCriticalBlockingStampsDrillStatus(t *testing.T) {
	idx := buildTraceIndex(t, "drill_status_q4.systrace", lockRankQ4Trace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.115})
	var monitor *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			monitor = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if monitor == nil {
		t.Fatalf("expected the monitor contention blocking row: %+v", res.CriticalBlocking)
	}
	if monitor.DrillStatus != DrillStatusUndrilledPeerKnown {
		t.Fatalf("named-but-never-examined holder must stamp undrilled_peer_known, got %+v", monitor)
	}
	for _, item := range res.CriticalBlocking.Items {
		if item.Type == "blocking_span" && item.BlockingKind == "" && item.DrillStatus != "" {
			t.Fatalf("rows without a counterpart lane must carry no drill verdict, got %+v", item)
		}
	}

	// Same shape + the holder now emits its own in-window span (a
	// subject==holder observation) → drilled.
	drilledTrace := lockRankQ4Trace + `
 NetworkKit_AssetsUtil_Operate_0-42067 (100) [003] .... 5.050000: print: B|100|AssetManager.list
 NetworkKit_AssetsUtil_Operate_0-42067 (100) [003] .... 5.060000: print: E|100
`
	idx = buildTraceIndex(t, "drill_status_q4_drilled.systrace", drilledTrace)
	res = Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.115})
	monitor = nil
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			monitor = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if monitor == nil {
		t.Fatalf("expected the monitor contention blocking row: %+v", res.CriticalBlocking)
	}
	if monitor.DrillStatus != DrillStatusDrilled {
		t.Fatalf("holder with its own subject observation must stamp drilled, got %+v", monitor)
	}
}

// Q4-K 修1 engine half: in the frame_root_cause_bundle evidence pack the
// blocking facts append SECOND (right after the chain facts), so lock
// evidence can no longer fall past the 16-fact pack cap behind frame/rank
// facts.
func TestFrameBundleEvidencePackOrdersBlockingAfterChain(t *testing.T) {
	idx := buildTraceIndex(t, "bundle_pack_order_q4.systrace", lockRankQ4Trace)
	res := Run(idx, Query{View: "frame_root_cause_bundle", PID: 100, TimeStart: 4.99, TimeEnd: 5.115})
	if len(res.EvidencePack) == 0 {
		t.Fatalf("expected a bundle evidence pack")
	}
	blockingIdx, rankIdx, frameIdx := -1, -1, -1
	for i, fact := range res.EvidencePack {
		switch {
		case blockingIdx < 0 && fact.Predicate == "blocking_span":
			blockingIdx = i
		case rankIdx < 0 && strings.HasPrefix(fact.Predicate, "root_cause_"):
			rankIdx = i
		case frameIdx < 0 && strings.HasPrefix(fact.Predicate, "frame"):
			frameIdx = i
		}
	}
	if blockingIdx < 0 || rankIdx < 0 {
		t.Fatalf("expected both blocking and rank facts in the bundle pack (blocking=%d rank=%d): %+v", blockingIdx, rankIdx, res.EvidencePack)
	}
	if blockingIdx > rankIdx {
		t.Fatalf("blocking facts must precede rank facts in the pack (blocking=%d rank=%d)", blockingIdx, rankIdx)
	}
	if frameIdx >= 0 && blockingIdx > frameIdx {
		t.Fatalf("blocking facts must precede frame facts in the pack (blocking=%d frame=%d)", blockingIdx, frameIdx)
	}
}

func TestDrillStatusVerdictFunction(t *testing.T) {
	universe := drillSubjectUniverse{pids: map[int]bool{42067: true}, comms: map[string]bool{"NamedOnly": true}}
	if got := drillStatusForCounterpart(ThreadRef{}, universe); got != DrillStatusPeerUnknown {
		t.Fatalf("unresolved counterpart must be peer_unknown, got %q", got)
	}
	if got := drillStatusForCounterpart(ThreadRef{PID: 42067}, universe); got != DrillStatusDrilled {
		t.Fatalf("pid member must be drilled, got %q", got)
	}
	if got := drillStatusForCounterpart(ThreadRef{Comm: "NamedOnly"}, universe); got != DrillStatusDrilled {
		t.Fatalf("comm-only member must match by exact comm, got %q", got)
	}
	if got := drillStatusForCounterpart(ThreadRef{Comm: "Ghost", PID: 777}, universe); got != DrillStatusUndrilledPeerKnown {
		t.Fatalf("resolved non-member must be undrilled_peer_known, got %q", got)
	}
}

func TestRootCauseRankKeepsUnresolvedLockContentionOffTheDirectLane(t *testing.T) {
	trace := `
        app-100 (100) [001] .... 5.000000: print: B|100|Lock contention on InternTable lock
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.112000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.112223: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.112300: print: E|100
`
	idx := buildTraceIndex(t, "lock_rank_q4_unresolved.systrace", trace)
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.115, MaxDepth: 4, MinDurationMs: 0.05, Limit: 16})
	var lock *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "blocking_span" {
			lock = &rank.Items[i]
			break
		}
	}
	if lock == nil {
		t.Fatalf("unresolved contention should still publish a typed blocking_span row: %+v", rank.Items)
	}
	// 未解析不准入: subject stays the waiter, counterpart stays empty, and the
	// row is demoted off the direct on-chain lane.
	if lock.Thread.PID != 100 {
		t.Fatalf("unresolved contention row subject must stay the waiter, got %+v", lock.Thread)
	}
	if threadRefResolved(lock.BlockingPeer) {
		t.Fatalf("unresolved contention must keep BlockingPeer empty, got %+v", lock.BlockingPeer)
	}
	if lock.ChainRelevance == "on_chain" {
		t.Fatalf("unresolved contention must not take a direct on-chain slot, got %+v", lock)
	}
	if rootCauseItemCanBeDirectOnChain(*lock) || rootCauseShouldBeCoPrimary(*lock) {
		t.Fatalf("typed admission pair must reject the unresolved lock row: %+v", lock)
	}
	if lock.DrillStatus != DrillStatusPeerUnknown {
		t.Fatalf("unresolved holder must stamp peer_unknown on the rank row, got %+v", lock)
	}
}

// Q4-B gate pin, the exact q4 failure shape: a 1.136ms on-chain resource row
// whose only claim to materiality was a ~112ms aggregate-window OVERLAP must
// no longer be attributed at all, and a genuinely material row keeps the
// dependency value on the annotation field with Score re-derived from the
// ranking channel.
func TestOnChainResourceAttributionRequiresOwnMeasurement(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{Comm: "app", PID: 100},
		CausalImpacts: []WakeupCausalImpact{{
			Thread:           ThreadRef{Comm: "worker", PID: 200},
			Window:           TimeWindow{StartTs: 10.0, EndTs: 10.1122},
			ChainDepth:       1,
			OnChain:          true,
			DominantState:    string(StateSSleep),
			DominantImpactMs: 112.175,
			TotalMs:          112.175,
			TargetBlockedMs:  112.175,
		}},
	}
	tiny := RootCauseRankItem{
		Type:               "io_latency",
		Thread:             ThreadRef{Comm: "worker", PID: 200},
		ImpactMs:           1.136,
		CumulativeImpactMs: 1.136,
		Score:              1.136 * 0.86 * rootCauseTypeWeight("io_latency"),
		Confidence:         0.86,
		Causality:          "on_wakeup_chain",
		ChainRelevance:     "on_chain",
		StartTs:            10.0,
		EndTs:              10.1122,
	}
	material := RootCauseRankItem{
		Type:               "io_latency",
		Thread:             ThreadRef{Comm: "worker", PID: 200},
		ImpactMs:           50,
		CumulativeImpactMs: 50,
		Score:              50 * 0.86 * rootCauseTypeWeight("io_latency"),
		Confidence:         0.86,
		Causality:          "on_wakeup_chain",
		ChainRelevance:     "on_chain",
		StartTs:            10.0,
		EndTs:              10.05,
	}
	items := []RootCauseRankItem{tiny, material}
	attributeOnChainResourceItemsToWakeupDependency(chain, items)

	// P3-8: zero OWN measurement + a pre-set big OverlapMs — a weak mutant
	// that re-admits overlap into the resourceMs fallback chain must be
	// caught here.
	zeroOwn := RootCauseRankItem{
		Type:           "io_latency",
		Thread:         ThreadRef{Comm: "worker", PID: 200},
		OverlapMs:      112.0,
		Confidence:     0.86,
		Causality:      "on_wakeup_chain",
		ChainRelevance: "on_chain",
		StartTs:        10.0,
		EndTs:          10.1122,
	}
	zeroItems := []RootCauseRankItem{zeroOwn}
	attributeOnChainResourceItemsToWakeupDependency(chain, zeroItems)
	if zeroItems[0].InheritedTargetBlockedMs != 0 || zeroItems[0].EffectiveImpactMs != 0 || strings.Contains(zeroItems[0].Summary, "wakeup dependency window") {
		t.Fatalf("zero own-measurement row must never be attributed, even with a large overlap, got %+v", zeroItems[0])
	}

	got := items[0]
	if got.InheritedTargetBlockedMs != 0 || got.EffectiveImpactMs != 0 || got.TargetImpactMs != 0 {
		t.Fatalf("overlap-only row must no longer be attributed (Q4-B: own measurement gates), got %+v", got)
	}
	if strings.Contains(got.Summary, "wakeup dependency window") {
		t.Fatalf("overlap-only row must not carry the attribution note, got %q", got.Summary)
	}

	got = items[1]
	if got.InheritedTargetBlockedMs != 112.175 {
		t.Fatalf("material row must keep the dependency value on the typed annotation field, got %+v", got)
	}
	if got.EffectiveImpactMs != 0 || got.TargetImpactMs != 0 {
		t.Fatalf("inherited value must not enter the ranking channels, got %+v", got)
	}
	wantScore := rootCauseEffectiveImpactMs(got) * got.Confidence * rootCauseTypeWeight(got.Type)
	if diff := got.Score - wantScore; diff > 0.0001 || diff < -0.0001 {
		t.Fatalf("attributed row must re-derive Score from the ranking channel, got %f want %f", got.Score, wantScore)
	}
	if !strings.Contains(got.Summary, "inherited target_blocked=112.175ms (annotation only, not a ranking key)") {
		t.Fatalf("attribution must stay disclosed as an advisory note, got %q", got.Summary)
	}
}

// Q4-D pin: an inversion candidate whose wait interval is COVERED by the
// target's resolved monitor/lock contention span demotes to note form (typed
// flag + wording), while non-covering / unresolved / non-target contention
// evidence changes nothing. All gate inputs typed; the observation survives.
func TestDemoteLockDominatedInversionCandidates(t *testing.T) {
	target := ThreadRef{Comm: "app", PID: 100}
	holder := ThreadRef{Comm: "NetworkKit_AssetsUtil_Operate_0", PID: 42067}
	chain := ChainResult{Target: target}
	stats := WindowStats{TraceSpans: []TraceSpanSummary{
		{ // resolved contention on the target, covers 5.010..5.080
			Thread: target, Name: lockContentionCustomerMonitorPayload,
			StartTs: 5.000, EndTs: 5.112, DurationMs: 112.0,
		},
		{ // resolved contention on ANOTHER thread — must not demote
			Thread: ThreadRef{Comm: "other", PID: 900}, Name: lockContentionCustomerMonitorPayload,
			StartTs: 5.000, EndTs: 5.200, DurationMs: 200.0,
		},
		{ // unresolved contention on the target — must not demote
			Thread: target, Name: "Lock contention on InternTable lock",
			StartTs: 5.000, EndTs: 5.300, DurationMs: 300.0,
		},
	}}
	covered := RootCauseRankItem{
		Type:    "priority_inversion_candidate",
		Thread:  ThreadRef{Comm: "RxComputationT", PID: 500},
		StartTs: 5.010, EndTs: 5.080,
		Summary: "RxComputationT runnable wait behind lower-priority competitor",
	}
	outside := RootCauseRankItem{
		Type:    "priority_inversion_candidate",
		Thread:  ThreadRef{Comm: "OtherWaker", PID: 600},
		StartTs: 5.100, EndTs: 5.150, // extends past the resolved cover's 5.112 end
		Summary: "OtherWaker runnable wait",
	}
	nonInversion := RootCauseRankItem{
		Type:    "runnable_wait",
		Thread:  ThreadRef{Comm: "Plain", PID: 700},
		StartTs: 5.010, EndTs: 5.080,
		Summary: "plain runnable wait",
	}
	// P2-2 per-CLASS: the RunnableTop-lane inversion form
	// (applyRunnableTopPriorityInversion) demotes under the same typed gate.
	runnableForm := RootCauseRankItem{
		Type:    "priority_inversion_runnable_wait",
		Thread:  ThreadRef{Comm: "RunnableForm", PID: 800},
		StartTs: 5.020, EndTs: 5.070,
		Summary: "runnable-top inversion form",
	}
	items := []RootCauseRankItem{covered, outside, nonInversion, runnableForm}
	demoteLockDominatedInversionCandidates(chain, stats, items)
	if !items[0].PriorityInversionLockDominated {
		t.Fatalf("covered inversion candidate must demote to note form, got %+v", items[0])
	}
	if !strings.Contains(items[0].Summary, "锁等待主导") || !strings.Contains(items[0].Summary, "monitor_contention held by "+threadLabel(holder)) {
		t.Fatalf("demotion note must name the resolved contention, got %q", items[0].Summary)
	}
	if items[0].Type != "priority_inversion_candidate" {
		t.Fatalf("demotion must not delete the observation, got %+v", items[0])
	}
	if items[1].PriorityInversionLockDominated || strings.Contains(items[1].Summary, "锁等待主导") {
		t.Fatalf("non-covered candidate must stay untouched, got %+v", items[1])
	}
	if items[2].PriorityInversionLockDominated || strings.Contains(items[2].Summary, "锁等待主导") {
		t.Fatalf("non-inversion rows must stay untouched, got %+v", items[2])
	}
	if !items[3].PriorityInversionLockDominated || !strings.Contains(items[3].Summary, "锁等待主导") {
		t.Fatalf("P2-2: priority_inversion_runnable_wait must demote under the same gate, got %+v", items[3])
	}
	// P2-1: a demoted row must not ride the inversion co-primary lane; the
	// same row without the flag keeps its legacy admission.
	demoted := items[0]
	demoted.ChainRelevance = "on_chain"
	demoted.ImpactMs = 5
	demoted.RunnableMs = 5
	if rootCauseShouldBeCoPrimary(demoted) {
		t.Fatalf("P2-1: lock-dominated inversion row must not be co-primary, got %+v", demoted)
	}
	demoted.PriorityInversionLockDominated = false
	if !rootCauseShouldBeCoPrimary(demoted) {
		t.Fatalf("P2-1: undemoted inversion row keeps co-primary admission, got %+v", demoted)
	}
	demotedRunnable := items[3]
	demotedRunnable.ChainRelevance = "on_chain"
	demotedRunnable.ImpactMs = 5
	demotedRunnable.RunnableMs = 5
	if rootCauseShouldBeCoPrimary(demotedRunnable) {
		t.Fatalf("P2-1 per-CLASS: demoted priority_inversion_runnable_wait must not be co-primary, got %+v", demotedRunnable)
	}
	demotedRunnable.PriorityInversionLockDominated = false
	if !rootCauseShouldBeCoPrimary(demotedRunnable) {
		t.Fatalf("P2-1: undemoted priority_inversion_runnable_wait keeps co-primary admission, got %+v", demotedRunnable)
	}
}

// §12.3 ruling 2 engine sort face: at equal measured impact inside the
// on-chain tier, the resolved-counterpart row leads and the row banking on an
// inherited annotation yields.
func TestSortRootCauseRankItemsAppliesRuling2TieBreaks(t *testing.T) {
	resolved := RootCauseRankItem{
		Type:               "blocking_span",
		Thread:             ThreadRef{Comm: "holder", PID: 42067},
		BlockingKind:       "monitor_contention",
		BlockingPeer:       ThreadRef{Comm: "app", PID: 100},
		ImpactMs:           50,
		CumulativeImpactMs: 50,
		Score:              10,
		ChainRelevance:     "on_chain",
		LineStart:          30,
	}
	measured := RootCauseRankItem{
		Type:               "io_wait",
		Thread:             ThreadRef{Comm: "io", PID: 300},
		ImpactMs:           50,
		CumulativeImpactMs: 50,
		Score:              10,
		ChainRelevance:     "on_chain",
		LineStart:          20,
	}
	inherited := RootCauseRankItem{
		Type:                     "io_latency",
		Thread:                   ThreadRef{Comm: "res", PID: 400},
		ImpactMs:                 50,
		CumulativeImpactMs:       50,
		InheritedTargetBlockedMs: 112.175,
		Score:                    10,
		ChainRelevance:           "on_chain",
		LineStart:                10,
	}
	items := []RootCauseRankItem{inherited, measured, resolved}
	sortRootCauseRankItems(items, true)
	if items[0].Thread.PID != 42067 || items[1].Thread.PID != 300 || items[2].Thread.PID != 400 {
		t.Fatalf("ruling-2 tie-breaks must order resolved-peer > measured > inherited-carrying, got %+v", items)
	}
}

// P2-3 pin (absorbs Q4-F/Q5-D at the engine source): the q4 E1/E2 shape — the
// SAME lock printed once as the rich "monitor contention with owner …" form
// and once as the "Lock contention on a monitor lock (owner tid: N)" form,
// overlapping in time — folds into ONE candidate at the source: exactly one
// blocking row, exactly one rank lock row, one drill stamp; the richer form
// survives with the MAX duration and MergedLines records the folded line.
const lockRankQ4DualFormTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|` + lockContentionCustomerMonitorPayload + `
        app-100 (100) [001] .... 5.000010: print: B|100|` + lockContentionCustomerLockPayload + `
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.112000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.112223: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.112230: print: E|100
        app-100 (100) [001] .... 5.112300: print: E|100
 NetworkKit_AssetsUtil_Operate_0-42067 (600) [003] .... 5.200000: sched_switch: prev_comm=NetworkKit_AssetsUtil_Operate_0 prev_pid=42067 prev_prio=120 prev_state=R ==> next_comm=idle/3 next_pid=0 next_prio=120
`

func TestSameLockDualPrintFormsFoldToSingleRow(t *testing.T) {
	idx := buildTraceIndex(t, "lock_rank_q4_dual.systrace", lockRankQ4DualFormTrace)

	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.115})
	var contention []CriticalBlockingCandidate
	for _, item := range res.CriticalBlocking.Items {
		if item.BlockingKind != "" {
			contention = append(contention, item)
		}
	}
	if len(contention) != 1 {
		t.Fatalf("dual print forms of the same lock must fold to exactly one blocking row, got %d: %+v", len(contention), contention)
	}
	folded := contention[0]
	if folded.Peer.Comm != "NetworkKit_AssetsUtil_Operate_0" || folded.Peer.PID != 42067 || folded.HolderSite == "" {
		t.Fatalf("the information-richer form must survive the fold, got %+v", folded)
	}
	if folded.DurationMs < 112.25 {
		t.Fatalf("the folded row must take the larger measured duration, got %+v", folded)
	}
	if len(folded.MergedLines) != 1 {
		t.Fatalf("the folded duplicate must be recorded on MergedLines, got %+v", folded)
	}
	if folded.DrillStatus != DrillStatusUndrilledPeerKnown {
		t.Fatalf("the folded row carries a single drill stamp, got %+v", folded)
	}

	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.115, MaxDepth: 4, MinDurationMs: 0.05, Limit: 16})
	lockRows := 0
	for _, item := range rank.Items {
		if item.Type == "blocking_span" {
			lockRows++
		}
	}
	if lockRows != 1 {
		t.Fatalf("dual print forms must publish exactly one rank lock row, got %d: %+v", lockRows, rank.Items)
	}
}

// P3-4 pin: a counterpart whose ONLY appearance is a resource decomposition
// surface (file/block IO, bursts, page cache, workqueue, …) is still
// EXAMINED — the universe must cover every own-conduct stats face so such
// counterparts don't flip to a false undrilled_peer_known.
func TestDrillSubjectUniverseCoversResourceSurfaces(t *testing.T) {
	fileThread := ThreadRef{Comm: "file-io", PID: 901}
	blockThread := ThreadRef{Comm: "block-io", PID: 902}
	burstThread := ThreadRef{Comm: "burst", PID: 903}
	cacheThread := ThreadRef{Comm: "cache", PID: 904}
	issueThread := ThreadRef{Comm: "issuer", PID: 905}
	completeThread := ThreadRef{Comm: "completer", PID: 906}
	workThread := ThreadRef{Comm: "kworker", PID: 907}
	stats := &WindowStats{
		FileIOByInode:     []FileIOSummary{{Thread: fileThread}},
		BlockIOByInode:    []BlockIOByInodeSummary{{Thread: blockThread}},
		IOBurstEpisodes:   []IOBurstEpisodeSummary{{Thread: burstThread}},
		PageCacheByInode:  []PageCacheSummary{{Thread: cacheThread}},
		IOLatencies:       []IOLatencySummary{{IssueThread: issueThread, CompleteThread: completeThread}},
		WorkqueueActivity: []WorkqueueActivity{{Thread: workThread}},
	}
	universe := buildDrillSubjectUniverse(nil, stats)
	for _, thread := range []ThreadRef{fileThread, blockThread, burstThread, cacheThread, issueThread, workThread} {
		if got := drillStatusForCounterpart(thread, universe); got != DrillStatusDrilled {
			t.Fatalf("resource-surface subject %s must count as examined, got %q", threadLabel(thread), got)
		}
	}
	// The IO completer is the io_latency row's COUNTERPART lane, never
	// auto-drilled by its own row.
	if got := drillStatusForCounterpart(completeThread, universe); got != DrillStatusUndrilledPeerKnown {
		t.Fatalf("io completer must not be auto-drilled by the issuer's row, got %q", got)
	}
}

// P3-5 pin: io_latency rows carry the counterpart lane (Peer = the block-IO
// completer) through the same three-verdict stamp.
func TestCriticalBlockingDrillStatusCoversIOLatency(t *testing.T) {
	universe := drillSubjectUniverse{pids: map[int]bool{906: true}, comms: map[string]bool{}}
	items := []CriticalBlockingCandidate{
		{Type: "io_latency", Peer: ThreadRef{Comm: "completer", PID: 906}},
		{Type: "io_latency", Peer: ThreadRef{Comm: "ghost-completer", PID: 907}},
		{Type: "io_latency"},
		{Type: "blocked_reason"},
	}
	stampCriticalBlockingDrillStatus(items, universe)
	if items[0].DrillStatus != DrillStatusDrilled {
		t.Fatalf("examined completer must stamp drilled, got %+v", items[0])
	}
	if items[1].DrillStatus != DrillStatusUndrilledPeerKnown {
		t.Fatalf("unexamined completer must stamp undrilled_peer_known, got %+v", items[1])
	}
	if items[2].DrillStatus != DrillStatusPeerUnknown {
		t.Fatalf("unresolved completer must stamp peer_unknown, got %+v", items[2])
	}
	if items[3].DrillStatus != "" {
		t.Fatalf("rows without a counterpart lane carry no verdict, got %+v", items[3])
	}
}
