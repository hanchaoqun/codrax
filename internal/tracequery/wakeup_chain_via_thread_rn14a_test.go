package tracequery

import (
	"reflect"
	"strings"
	"testing"
)

// RN-14a (§7.9, cust_runnable 2026-07-04) pins — wakeup_chain via_thread.
//
// Customer live shape: the model anchored the round on an FFRT thread
// (runnable-dominant, no wakeup edge), the user-focus thread's causal chain
// was lost, and there was no decisive way to test whether the runnable anchor
// sits ON the focus thread's wakeup chain (root-cause dependency) or merely
// competes for CPU (scheduling contention). via_thread is that decisive test:
// both verdicts are affirmative typed outputs, and via-containing branches
// are immune to the max_branches top-N cap so the connection cannot be lost
// to branch pruning.

// nested chain: io-300 wakes worker-200, worker-200 wakes app-100;
// contender-400 preempts app-100 (runnable queuing) but never wakes anyone —
// the two customer-shaped via candidates in one fixture.
const viaChainTrace = `
        app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.005000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
         io-300 (100) [003] .... 1.006000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=io next_pid=300 next_prio=30
         io-300 (100) [003] .... 1.020000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002
         io-300 (100) [003] .... 1.021000: sched_switch: prev_comm=io prev_pid=300 prev_prio=30 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 1.025000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40
     worker-200 (100) [002] .... 1.030000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200 (100) [002] .... 1.031000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 1.035000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
  contender-400 (400) [001] .... 1.036000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=R+ ==> next_comm=contender next_pid=400 next_prio=20
  contender-400 (400) [001] .... 1.038000: sched_switch: prev_comm=contender prev_pid=400 prev_prio=20 prev_state=S ==> next_comm=app next_pid=100 next_prio=52
`

func buildViaChainIndex(t *testing.T) *Index {
	t.Helper()
	return buildTraceIndex(t, "via_chain.systrace", viaChainTrace)
}

// TestWakeupChainViaThread_OnPathReportsDepthAndPerHopLatency pins the
// on-chain verdict: via io-300 (depth 2) reports the full hop walk down to
// the target with per-hop wakeup latency, all from existing wakeup edges.
func TestWakeupChainViaThread_OnPathReportsDepthAndPerHopLatency(t *testing.T) {
	idx := buildViaChainIndex(t)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.04, MinDurationMs: 0.05, ViaThread: "300"})
	via := chain.ViaThread
	if via == nil {
		t.Fatalf("via_thread=300 must produce a typed via verdict, got none; caveats=%v", chain.Caveats)
	}
	if !via.OnChain || via.Thread.PID != 300 {
		t.Fatalf("io-300 is on the wakeup path to app-100, got %+v", via)
	}
	if via.Depth != 2 {
		t.Fatalf("io-300 sits two hops above the target, want depth=2 got %+v", via)
	}
	if len(via.Hops) != 2 ||
		via.Hops[0].Waker.PID != 300 || via.Hops[0].Wakee.PID != 200 ||
		via.Hops[1].Waker.PID != 200 || via.Hops[1].Wakee.PID != 100 {
		t.Fatalf("hop walk must follow io-300 -> worker-200 -> app-100, got %+v", via.Hops)
	}
	for i, hop := range via.Hops {
		if hop.LatencyMs <= 0 || hop.WakeupLine == 0 {
			t.Fatalf("hop %d must carry per-hop wakeup latency and line anchor, got %+v", i, hop)
		}
	}
	if !strings.Contains(via.Summary, "ON wakeup path: depth=2") || !strings.Contains(via.Summary, "per-hop latency") {
		t.Fatalf("on-path summary must state depth and per-hop latency, got %q", via.Summary)
	}
}

// TestWakeupChainViaThread_NotOnPathIsSchedulingContentionVerdict pins the
// customer-critical negative verdict wording (§7.4 wording discipline):
// a thread with no wakeup edge to the target is scheduling contention
// (runnable queuing), never a wakeup dependency — and the wording must not
// borrow the compute-supply (算力) delivery vocabulary.
func TestWakeupChainViaThread_NotOnPathIsSchedulingContentionVerdict(t *testing.T) {
	idx := buildViaChainIndex(t)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.04, MinDurationMs: 0.05, ViaThread: "contender-400"})
	via := chain.ViaThread
	if via == nil {
		t.Fatalf("via_thread=contender-400 must produce a typed via verdict, got none")
	}
	if via.OnChain || len(via.Hops) != 0 || via.Thread.PID != 0 {
		t.Fatalf("contender-400 preempts but never wakes: must be off-chain with no hops, got %+v", via)
	}
	want := "via_thread contender-400 NOT on any wakeup path to app-100 in this window; its influence is scheduling contention only (runnable queuing), not a wakeup dependency"
	if via.Summary != want {
		t.Fatalf("off-chain verdict wording is load-bearing (§7.4), got %q want %q", via.Summary, want)
	}
	if strings.Contains(via.Summary, "算力") || strings.Contains(strings.ToLower(via.Summary), "compute") {
		t.Fatalf("off-chain verdict must not use compute-supply vocabulary for runnable semantics: %q", via.Summary)
	}
}

// TestWakeupChainNoViaThread_NoVerdictAttached pins that the via report is
// strictly opt-in: without the parameter the result shape is unchanged.
func TestWakeupChainNoViaThread_NoVerdictAttached(t *testing.T) {
	idx := buildViaChainIndex(t)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 1.0, TimeEnd: 1.04, MinDurationMs: 0.05})
	if chain.ViaThread != nil {
		t.Fatalf("no via_thread parameter must mean no via verdict, got %+v", chain.ViaThread)
	}
}

// two-segment target: the 100ms sleep (woken by wakerA-30) wins the
// max_branches=1 cut; the 50ms sleep (woken by wakerB-40) is dropped by the
// top-N cap — the exact shape where the customer's connection to the focus
// thread would silently vanish without via immunity.
const viaBranchCapTrace = `
      wakerA-30 (30) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=wakerA next_pid=30 next_prio=20
         app-20 (20) [001] .... 1.100000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      wakerA-30 (30) [000] .... 1.200000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
         app-20 (20) [001] .... 1.202000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
         app-20 (20) [001] .... 1.300000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
      wakerB-40 (40) [002] .... 1.350000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
         app-20 (20) [001] .... 1.352000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
`

func chainNodePIDs(chain ChainResult) map[int]bool {
	out := map[int]bool{}
	for _, node := range chain.Nodes {
		out[node.Thread.PID] = true
	}
	return out
}

// TestWakeupChainViaThread_BranchCapImmunityExpandsDroppedViaBranch pins the
// immunity lane: with max_branches=1 the wakerB segment is normally dropped,
// but via_thread=40 forces its expansion, reports the on-path verdict, and
// discloses the immunity expansion in a caveat.
func TestWakeupChainViaThread_BranchCapImmunityExpandsDroppedViaBranch(t *testing.T) {
	idx := buildTraceIndex(t, "via_branch_cap.systrace", viaBranchCapTrace)
	base := Query{PID: 20, TimeStart: 1.0, TimeEnd: 1.5, MinDurationMs: 20, MaxBranches: 1}

	capped := BuildWakeupChain(idx, base)
	if pids := chainNodePIDs(capped); pids[40] || !pids[30] {
		t.Fatalf("control: max_branches=1 must keep wakerA-30 and drop the wakerB-40 branch, nodes=%+v", capped.Nodes)
	}
	if !containsSubstring(capped.Caveats, "were not recursed into") {
		t.Fatalf("control: dropped-segment caveat expected, got %v", capped.Caveats)
	}

	withVia := base
	withVia.ViaThread = "40"
	chain := BuildWakeupChain(idx, withVia)
	if pids := chainNodePIDs(chain); !pids[40] || !pids[30] {
		t.Fatalf("via branch must be expanded despite max_branches=1, nodes=%+v", chain.Nodes)
	}
	if chain.ViaThread == nil || !chain.ViaThread.OnChain || chain.ViaThread.Depth != 1 ||
		len(chain.ViaThread.Hops) != 1 || chain.ViaThread.Hops[0].Waker.PID != 40 || chain.ViaThread.Hops[0].Wakee.PID != 20 {
		t.Fatalf("via verdict must report wakerB-40 one hop above app-20, got %+v", chain.ViaThread)
	}
	if !containsSubstring(chain.Caveats, "via_thread=40 branch-cap immunity: expanded 1 additional segment(s)") {
		t.Fatalf("immunity expansion must be disclosed in a caveat, got %v", chain.Caveats)
	}
	if containsSubstring(chain.Caveats, "were not recursed into") {
		t.Fatalf("every qualifying segment was expanded (top-1 + via); the dropped-segment caveat must not survive: %v", chain.Caveats)
	}
}

// TestWakeupChainViaThread_NonViaOverflowRolledBackKeepsCappedChainIdentical
// pins the rollback: a via thread already inside the kept top-N branch gives
// its verdict WITHOUT expanding the overflow — nodes/edges/impacts stay
// deep-equal to the capped chain and the dropped-segment caveat is re-issued
// unchanged (no phantom expansion into the pruned branch).
func TestWakeupChainViaThread_NonViaOverflowRolledBackKeepsCappedChainIdentical(t *testing.T) {
	idx := buildTraceIndex(t, "via_branch_cap.systrace", viaBranchCapTrace)
	base := Query{PID: 20, TimeStart: 1.0, TimeEnd: 1.5, MinDurationMs: 20, MaxBranches: 1}
	capped := BuildWakeupChain(idx, base)

	withVia := base
	withVia.ViaThread = "30"
	chain := BuildWakeupChain(idx, withVia)
	if chain.ViaThread == nil || !chain.ViaThread.OnChain || chain.ViaThread.Thread.PID != 30 || chain.ViaThread.Depth != 1 {
		t.Fatalf("wakerA-30 is on the kept branch, want on-path depth=1 verdict, got %+v", chain.ViaThread)
	}
	if !reflect.DeepEqual(chain.Nodes, capped.Nodes) || !reflect.DeepEqual(chain.Edges, capped.Edges) ||
		!reflect.DeepEqual(chain.CausalImpacts, capped.CausalImpacts) {
		t.Fatalf("non-via overflow expansion must be rolled back wholesale:\nvia nodes=%+v\ncapped nodes=%+v", chain.Nodes, capped.Nodes)
	}
	if !containsSubstring(chain.Caveats, "1 lower-ranked segment(s) were not recursed into") {
		t.Fatalf("dropped-segment caveat must be re-issued after rollback, got %v", chain.Caveats)
	}
	if containsSubstring(chain.Caveats, "branch-cap immunity") {
		t.Fatalf("no immunity caveat when nothing was kept, got %v", chain.Caveats)
	}
}

// TestChainThreadMatchesViaSelector_CanonicalExactOnly pins the matcher
// discipline: pid forms match by exact integer, name forms by verbatim comm
// equality — never substring/fuzzy matching (no prose keyword lanes).
func TestChainThreadMatchesViaSelector_CanonicalExactOnly(t *testing.T) {
	ffrt := ThreadRef{Comm: "OS_FFRT_2_3", PID: 49706}
	cases := []struct {
		selector string
		want     bool
	}{
		{"49706", true},
		{"pid=49706", true},
		{"OS_FFRT_2_3-49706", true},
		{"OS_FFRT_2_3", true},
		{"49707", false},
		// substring of the comm must NOT match — precise signals only.
		{"FFRT", false},
		{"OS_FFRT_2", false},
	}
	for _, tc := range cases {
		if got := chainThreadMatchesViaSelector(parseThreadSelector(tc.selector), ffrt); got != tc.want {
			t.Fatalf("selector %q vs %s: got %v want %v", tc.selector, ffrt.Comm, got, tc.want)
		}
	}
}

// F4 pin (2026-07-04 review counterexample, double-branch stitch): the via
// hop walk must never step BACKWARDS in time — after a hop at t=7.9 an edge
// at t=1.0 from another branch expansion is an impossible sequence. When a
// time-consistent continuation exists it must be taken; edge slice order
// (which the old walk followed blindly) must not matter.
func TestViaHopWalk_TimeMonotonic_CrossBranchStitchForbidden(t *testing.T) {
	via := ThreadRef{Comm: "via", PID: 50}
	mid := ThreadRef{Comm: "mid", PID: 60}
	target := ThreadRef{Comm: "app", PID: 100}
	res := &ChainResult{
		Target: target,
		Nodes: []ChainNode{
			{ID: "n1", Thread: target},
			{ID: "n2", Thread: mid},
			{ID: "n3", Thread: via},
		},
		Edges: []WakeupEdge{
			// Branch X: via wakes mid late.
			{Waker: via, Wakee: mid, WakeupTs: 7.9, LatencyMs: 1},
			// Branch Y (listed FIRST among mid's edges): mid woke the target
			// much earlier — t=1.0 after t=7.9 is not a real sequence.
			{Waker: mid, Wakee: target, WakeupTs: 1.0, LatencyMs: 1},
			// Branch X continuation: the only time-consistent path.
			{Waker: mid, Wakee: target, WakeupTs: 8.0, LatencyMs: 1},
		},
		CausalImpacts: []WakeupCausalImpact{
			{Thread: via, ChainDepth: 2, TotalMs: 1, DominantState: string(StateSSleep)},
		},
	}
	attachChainViaThreadReport("via", res)
	rep := res.ViaThread
	if rep == nil || !rep.OnChain {
		t.Fatalf("via must be on chain, got %+v", rep)
	}
	if len(rep.Hops) != 2 || rep.Hops[1].WakeupTs != 8.0 {
		t.Fatalf("walk must take the t=8.0 continuation, not stitch t=1.0 after t=7.9: %+v", rep.Hops)
	}
	for i := 1; i < len(rep.Hops); i++ {
		if rep.Hops[i].WakeupTs < rep.Hops[i-1].WakeupTs {
			t.Fatalf("hop walk stepped backwards in time: %+v", rep.Hops)
		}
	}
	if strings.Contains(rep.Summary, "跨分支") {
		t.Fatalf("a complete time-consistent path must not carry the truncation note: %q", rep.Summary)
	}
}

// F4 pin, truncation face: when NO time-consistent edge continues toward the
// target, the walk truncates at the reachable prefix and the summary carries
// the 跨分支 note — it must not fabricate the impossible t=7.9→t=1.0 order.
func TestViaHopWalk_TruncatesInsteadOfImpossibleOrder(t *testing.T) {
	via := ThreadRef{Comm: "via", PID: 50}
	mid := ThreadRef{Comm: "mid", PID: 60}
	target := ThreadRef{Comm: "app", PID: 100}
	res := &ChainResult{
		Target: target,
		Nodes:  []ChainNode{{ID: "n1", Thread: target}, {ID: "n2", Thread: mid}, {ID: "n3", Thread: via}},
		Edges: []WakeupEdge{
			{Waker: via, Wakee: mid, WakeupTs: 7.9, LatencyMs: 1},
			{Waker: mid, Wakee: target, WakeupTs: 1.0, LatencyMs: 1},
		},
		CausalImpacts: []WakeupCausalImpact{
			{Thread: via, ChainDepth: 2, TotalMs: 1, DominantState: string(StateSSleep)},
		},
	}
	attachChainViaThreadReport("via", res)
	rep := res.ViaThread
	if rep == nil || !rep.OnChain {
		t.Fatalf("via must be on chain, got %+v", rep)
	}
	if len(rep.Hops) != 1 || rep.Hops[0].WakeupTs != 7.9 {
		t.Fatalf("walk must truncate after the t=7.9 hop, got %+v", rep.Hops)
	}
	if !strings.Contains(rep.Summary, "跨分支,逐跳序不可得") {
		t.Fatalf("truncated walk must carry the cross-branch note, got %q", rep.Summary)
	}
}

// F5 pin (2026-07-04 review, double branch depth=1): Depth comes from the
// min-ChainDepth impact; the hop walk must descend the SAME branch — depth=1
// pairs with exactly one hop, even when the expansion order lists the deeper
// branch's edges first (the old walk produced depth=1 with a 2-hop walk).
func TestViaHopWalk_DepthAndHopsFromSameBranch(t *testing.T) {
	via := ThreadRef{Comm: "via", PID: 50}
	mid := ThreadRef{Comm: "mid", PID: 60}
	target := ThreadRef{Comm: "app", PID: 100}
	res := &ChainResult{
		Target: target,
		Nodes:  []ChainNode{{ID: "n1", Thread: target}, {ID: "n2", Thread: mid}, {ID: "n3", Thread: via}},
		Edges: []WakeupEdge{
			// Deeper branch listed first: via -> mid -> target (2 hops).
			{Waker: via, Wakee: mid, WakeupTs: 2.0, LatencyMs: 1},
			{Waker: mid, Wakee: target, WakeupTs: 2.1, LatencyMs: 1},
			// Direct branch: via -> target (1 hop) — the min-depth branch.
			{Waker: via, Wakee: target, WakeupTs: 3.0, LatencyMs: 1},
		},
		CausalImpacts: []WakeupCausalImpact{
			{Thread: via, ChainDepth: 2, TotalMs: 1, DominantState: string(StateSSleep)},
			{Thread: via, ChainDepth: 1, TotalMs: 1, DominantState: string(StateSSleep)},
		},
	}
	attachChainViaThreadReport("via", res)
	rep := res.ViaThread
	if rep == nil || !rep.OnChain {
		t.Fatalf("via must be on chain, got %+v", rep)
	}
	if rep.Depth != 1 {
		t.Fatalf("min ChainDepth is 1, got %+v", rep)
	}
	if len(rep.Hops) != 1 || rep.Hops[0].Wakee.PID != 100 || rep.Hops[0].WakeupTs != 3.0 {
		t.Fatalf("depth=1 must pair with exactly the direct hop, got %+v", rep.Hops)
	}
	if strings.Contains(rep.Summary, "跨分支") {
		t.Fatalf("complete path must not carry the truncation note: %q", rep.Summary)
	}
}
