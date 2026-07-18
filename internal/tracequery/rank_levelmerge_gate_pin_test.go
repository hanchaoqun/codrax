package tracequery

// rank_levelmerge_gate_pin_test.go — LEVELMERGE-1 件1 (核实官方向 C, user
// ruling 2026-07-18): the five-layer gate that killed the three
// same-(pid,family) double-seat lanes was assembled by OTHER batches
// (5d91b433 exact-twin fold widening / R5d gated authority §7.30.1 / the
// missing_wakeup zero table / the ipc.go binder target filter / the SYM
// target_self_state demotion) and had ZERO regression pins of its own — any
// one gate could silently die and resurrect the customer's E26+E28 double-
// full-seat board (runnable2 witness: Σ 26.243 > 23.471 physical). Each layer
// gets its own pin here; fixtures go through the FULL production mint chain
// (BuildIndex → BuildWakeupChain → BuildRootCauseRank, §29.120 — never
// hand-built structs) or read the typed authority function directly.

import (
	"context"
	"strings"
	"testing"
)

// Layer 1 — exact-twin folding (5d91b433 widening): on the real tieba 59566
// board the chain.RootEvidence lane holds priority-inversion runnable twins
// of admitted causal impacts, and NOT ONE of them mints a rank row — the
// aggregate/per-occurrence chain lane is the single rank carrier. The pin is
// the mechanical coexistence census: zero published RootEvidence-lane rank
// rows whose exact occurrence identity (Type|thread|lines|duration — the
// production seat key) matches a derived twin of any CausalImpact.
func TestLevelMergeGatePinTiebaTwinFoldCoexistenceZero(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Skipf("tieba fixture not present: %v", err)
	}
	q := Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	chain := BuildWakeupChain(idx, q)
	rank := BuildRootCauseRank(idx, q)

	// Baseline shape (核实官 §1.1): the board's RootEvidence lane carries
	// inversion runnable twins — the fold has real work to do here. If the
	// trace/window ever drifts to zero twins this pin is checking nothing
	// and must fail loud.
	twins := 0
	for _, root := range chain.RootEvidence {
		if root.Type == "priority_inversion_runnable_wait" {
			twins++
		}
	}
	if twins < 2 {
		t.Fatalf("fixture drift: expected the tieba 59566 board to carry >=2 priority_inversion_runnable_wait RootEvidence twins, got %d", twins)
	}

	// The production twin identity: rootEvidenceFromCausalImpact over every
	// causal impact, plus the mutation-invariant D-state family key. Any
	// published RootEvidence-lane rank row (Source=="wakeup_chain") matching
	// either key is a resurrected twin — the §20.1/5d91b433 gate died.
	twinKeys := map[string]bool{}
	for _, impact := range chain.CausalImpacts {
		seed := rootEvidenceFromCausalImpact(impact, "", 0)
		twinKeys[rootEvidenceRankSeatKey(seed)] = true
		if key, ok := rootEvidenceDStateTwinFamilyKey(seed); ok {
			twinKeys[key] = true
		}
	}
	for _, item := range rank.Items {
		if item.Source != "wakeup_chain" {
			continue
		}
		root := RootEvidence{Type: item.Type, Thread: item.Thread, DurationMs: item.ImpactMs, LineStart: item.LineStart, LineEnd: item.LineEnd}
		if twinKeys[rootEvidenceRankSeatKey(root)] {
			t.Fatalf("layer-1 gate dead: RootEvidence-lane rank row is an exact twin of an admitted causal impact: type=%s pid=%d L%d..%d",
				item.Type, item.Thread.PID, item.LineStart, item.LineEnd)
		}
		if key, ok := rootEvidenceDStateTwinFamilyKey(root); ok && twinKeys[key] {
			t.Fatalf("layer-1 gate dead (family key): type=%s pid=%d L%d..%d", item.Type, item.Thread.PID, item.LineStart, item.LineEnd)
		}
		if item.Type == "priority_inversion_runnable_wait" {
			t.Fatalf("layer-1 gate dead: the inversion runnable twin minted a rank row (pid=%d)", item.Thread.PID)
		}
	}

	// Coexistence census (核实官 §1.1 five-fold baseline): no same-
	// (pid, dominant family) pair of an eff>0 RootEvidence-lane row beside an
	// eff>0 chain-lane seat.
	chainSeats := map[string]bool{}
	for _, item := range rank.Items {
		if strings.HasPrefix(item.Source, "wakeup_chain.") && rootCauseEffectiveImpactMs(item) > 0 {
			chainSeats[wakeupCausalAggregateGroupKey(item.Thread.PID, item.DominantState)] = true
		}
	}
	for _, item := range rank.Items {
		if item.Source != "wakeup_chain" || rootCauseEffectiveImpactMs(item) <= 0 {
			continue
		}
		if chainSeats[wakeupCausalAggregateGroupKey(item.Thread.PID, item.DominantState)] {
			t.Fatalf("coexistence!=0: RootEvidence-lane row (type=%s pid=%d state=%s) beside a chain seat of the same (pid,family)",
				item.Type, item.Thread.PID, item.DominantState)
		}
	}
}

// Layer 2 — the R5d gated authority arm (§7.30.1): a typed priority-inversion
// row owns its gated result INCLUDING ZERO. Falling through at zero would
// resurrect the raw runnable/running wall clock under the inversion label —
// exactly the second防线 that keeps a revived RootEvidence inversion twin
// (whose gated channel was never seeded) at eff 0 / context_only instead of
// re-competing at raw value.
func TestLevelMergeGatePinGatedAuthorityIncludesZero(t *testing.T) {
	for _, typ := range []string{"priority_inversion_candidate", "priority_inversion_runnable_wait"} {
		zero := RootCauseRankItem{
			Type: typ, Thread: ThreadRef{PID: 7}, DominantState: string(StateRunnable),
			RunnableMs: 12.5, CumulativeImpactMs: 30, ImpactMs: 12.5, EffectiveImpactMs: 0,
		}
		if got := rootCauseEffectiveImpactMsUncapped(zero); got != 0 {
			t.Fatalf("layer-2 gate dead: %s with gated 0 must stay 0 (raw resurrection), got %.3f", typ, got)
		}
		nonzero := zero
		nonzero.EffectiveImpactMs = 3.75
		if got := rootCauseEffectiveImpactMsUncapped(nonzero); got != 3.75 {
			t.Fatalf("layer-2 gate: %s must publish the gated value verbatim, got %.3f", typ, got)
		}
	}
	// The family predicate itself is load-bearing (both tokens, nothing else).
	if !rootCauseTypeIsPriorityInversion("priority_inversion_candidate") ||
		!rootCauseTypeIsPriorityInversion("priority_inversion_runnable_wait") ||
		rootCauseTypeIsPriorityInversion("runnable_wait") {
		t.Fatalf("inversion type family predicate drifted")
	}
}

// Layer 3 — the missing_wakeup zero table: the RootEvidence lane keeps
// publishing missing_wakeup as lossless evidence, but its effective
// attribution is a typed zero (no election seat, Rank-0 side rail). Unit arm
// pins the zero-table membership; the donghu 2955 real board pins the
// Rank-0/eff-0 shape end to end.
func TestLevelMergeGatePinMissingWakeupZeroTable(t *testing.T) {
	item := RootCauseRankItem{
		Type: "missing_wakeup", Thread: ThreadRef{PID: 9}, Source: "wakeup_chain",
		ImpactMs: 8.793, CumulativeImpactMs: 8.793,
	}
	if got := rootCauseEffectiveImpactMsUncapped(item); got != 0 {
		t.Fatalf("layer-3 gate dead: missing_wakeup must sit in the effective zero table, got %.3f", got)
	}
}

func TestLevelMergeGatePinMissingWakeupRankZeroOnDonghu(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Skipf("donghu fixture not present: %v", err)
	}
	q := Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	rank := BuildRootCauseRank(idx, q)
	seen := 0
	for _, item := range rank.Items {
		if item.Type != "missing_wakeup" {
			continue
		}
		seen++
		if item.Rank != 0 || rootCauseEffectiveImpactMs(item) != 0 {
			t.Fatalf("layer-3 gate dead: missing_wakeup row competes (rank=%d eff=%.3f)", item.Rank, rootCauseEffectiveImpactMs(item))
		}
	}
	if seen == 0 {
		t.Fatalf("fixture drift: the donghu 2955 board is expected to publish a missing_wakeup side row")
	}
}

// Layer 4 — the binder IPC target filter (ipc.go): only edges MENTIONING the
// query thread enter the IPC graph, so findBinderWaitsForChain can never see
// (and binder_wait evidence can never mint for) a non-target dependency
// thread. Unit arm pins the filter predicate; the synthetic production-chain
// trace pins the end-to-end absence: a dependency thread with two D-dominant
// binder round-trips mints its (pid, d_sleep) aggregate WITHOUT any
// binder_wait seat, and no rank row of a non-target pid carries the
// binder_wait type.
func TestLevelMergeGatePinBinderEdgeTargetFilter(t *testing.T) {
	edge := IPCEdge{Sender: ThreadRef{PID: 200}, Receiver: ThreadRef{PID: 300}, DestThread: 301}
	if ipcEdgeMentionsQuery(edge, Query{PID: 100}) {
		t.Fatalf("layer-4 gate dead: an edge not mentioning the query pid entered the IPC graph")
	}
	for _, q := range []Query{{PID: 200}, {PID: 300}, {PID: 301}} {
		if !ipcEdgeMentionsQuery(edge, q) {
			t.Fatalf("layer-4 gate over-filters: pid %d IS mentioned by the edge", q.PID)
		}
	}
}

const levelMergeBinderSynthTrace = `
      app_target-100   (  100) [000] .... 10.000000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app_target next_pid=100 next_prio=120
      dep_worker-200   (  200) [001] .... 10.000100: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep_worker next_pid=200 next_prio=120
      dep_worker-200   (  200) [001] .... 10.000500: binder_transaction: transaction=5001 dest_node=77 dest_proc=300 dest_thread=0 reply=0 flags=0x10 code=0x5
      dep_worker-200   (  200) [001] .... 10.000600: sched_wakeup: comm=binder_srv pid=301 prio=120 target_cpu=002
      binder_srv-301   (  300) [002] .... 10.000700: sched_switch: prev_comm=swapper/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=binder_srv next_pid=301 next_prio=120
      app_target-100   (  100) [000] .... 10.001000: sched_switch: prev_comm=app_target prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
      dep_worker-200   (  200) [001] .... 10.001200: sched_switch: prev_comm=dep_worker prev_pid=200 prev_prio=120 prev_state=D ==> next_comm=swapper/1 next_pid=0 next_prio=120
      binder_srv-301   (  300) [002] .... 10.001500: binder_transaction_received: transaction=5001
      binder_srv-301   (  300) [002] .... 10.022000: binder_transaction: transaction=5002 dest_node=0 dest_proc=200 dest_thread=200 reply=1 flags=0x0 code=0x0
      binder_srv-301   (  300) [002] .... 10.022100: sched_wakeup: comm=dep_worker pid=200 prio=120 target_cpu=001
      dep_worker-200   (  200) [001] .... 10.022300: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep_worker next_pid=200 next_prio=120
      dep_worker-200   (  200) [001] .... 10.022400: binder_transaction_received: transaction=5002
      dep_worker-200   (  200) [001] .... 10.023000: sched_wakeup: comm=app_target pid=100 prio=120 target_cpu=000
      app_target-100   (  100) [000] .... 10.023200: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app_target next_pid=100 next_prio=120
      dep_worker-200   (  200) [001] .... 10.050500: binder_transaction: transaction=6001 dest_node=77 dest_proc=300 dest_thread=0 reply=0 flags=0x10 code=0x5
      dep_worker-200   (  200) [001] .... 10.050600: sched_wakeup: comm=binder_srv pid=301 prio=120 target_cpu=002
      app_target-100   (  100) [000] .... 10.051000: sched_switch: prev_comm=app_target prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
      dep_worker-200   (  200) [001] .... 10.051200: sched_switch: prev_comm=dep_worker prev_pid=200 prev_prio=120 prev_state=D ==> next_comm=swapper/1 next_pid=0 next_prio=120
      binder_srv-301   (  300) [002] .... 10.051500: binder_transaction_received: transaction=6001
      binder_srv-301   (  300) [002] .... 10.070000: binder_transaction: transaction=6002 dest_node=0 dest_proc=200 dest_thread=200 reply=1 flags=0x0 code=0x0
      binder_srv-301   (  300) [002] .... 10.070100: sched_wakeup: comm=dep_worker pid=200 prio=120 target_cpu=001
      dep_worker-200   (  200) [001] .... 10.070300: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep_worker next_pid=200 next_prio=120
      dep_worker-200   (  200) [001] .... 10.070400: binder_transaction_received: transaction=6002
      dep_worker-200   (  200) [001] .... 10.071000: sched_wakeup: comm=app_target pid=100 prio=120 target_cpu=000
      app_target-100   (  100) [000] .... 10.071200: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app_target next_pid=100 next_prio=120
      app_target-100   (  100) [000] .... 10.090000: sched_switch: prev_comm=app_target prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120
`

func TestLevelMergeGatePinBinderNonTargetLaneUnreachable(t *testing.T) {
	idx := buildTraceIndex(t, "levelmerge_binder_synth.ftrace", levelMergeBinderSynthTrace)
	q := Query{PID: 100, TimeStart: 10.000, TimeEnd: 10.095, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 1}
	chain := BuildWakeupChain(idx, q)
	rank := BuildRootCauseRank(idx, q)

	// The dependency's (200, d_sleep) aggregate exists (two D occurrences).
	haveDepAggregate := false
	for _, aggregate := range chainRankAggregateCensus(chain) {
		if aggregate.Thread.PID == 200 && aggregate.DominantState == string(StateDSleep) && aggregate.OccurrenceCount >= 2 {
			haveDepAggregate = true
		}
	}
	if !haveDepAggregate {
		t.Fatalf("fixture drift: the (200,d_sleep) production aggregate is expected on this trace")
	}
	// The non-target binder lane never mints: no binder_wait evidence and no
	// binder_wait rank row for pid 200 (the ipc.go filter kills the edges).
	for _, wait := range chain.BinderWaits {
		if wait.Thread.PID == 200 {
			t.Fatalf("layer-4 gate dead: binder_wait evidence minted for the non-target dependency thread")
		}
	}
	for _, item := range rank.Items {
		if item.Type == "binder_wait" && item.Thread.PID != q.PID {
			t.Fatalf("layer-4 gate dead: a non-target binder_wait rank row minted (pid=%d)", item.Thread.PID)
		}
	}
}

// Layer 5 — the SYM target_self_state demotion: the target's OWN binder_wait
// row (the only reachable binder rank shape) is a wait symptom — registry
// wakeup-chain lane — and demotes to the Rank-0 side rail instead of
// competing. Pinned at the predicate level (registry single source) plus the
// ladder behavior.
func TestLevelMergeGatePinTargetBinderWaitDemotes(t *testing.T) {
	if !rootCauseItemIsTargetWaitSymptomType(RootCauseRankItem{Type: "binder_wait"}) ||
		!rootCauseItemIsTargetWaitSymptomType(RootCauseRankItem{Type: "missing_wakeup"}) {
		t.Fatalf("layer-5 gate dead: the wait-symptom registry lane lost a member")
	}
	items := []RootCauseRankItem{{
		Type: "binder_wait", Thread: ThreadRef{PID: 100}, Source: "wakeup_chain",
		ImpactMs: 16.164, CumulativeImpactMs: 16.164, EffectiveImpactMs: 16.164,
		SubjectIsAnalysisTarget: true,
	}}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier != RootCauseTierTargetSelfState || items[0].Rank != 0 {
		t.Fatalf("layer-5 gate dead: target binder_wait must ride the Rank-0 %s rail, got tier=%s rank=%d",
			RootCauseTierTargetSelfState, items[0].Tier, items[0].Rank)
	}
}
