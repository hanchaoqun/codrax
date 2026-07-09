package tracequery

import "testing"

// P0-E CHAIN-PATH 根修 engine pins (ledger §22.1, huadong_78 §24.11-A witness,
// 2026-07-09): every ChainNode / WakeupEdge / WakeupCausalImpact carries its
// TRUE recursion depth and its owning branch ordinal, so the publication layer
// can serialize one REAL path per branch instead of the retired cross-branch
// flattened walk (which stitched 29 elements with an oney⇄VSync ×7 ping-pong
// ladder and minted fake L26/L27 trunk depths downstream).
//
// MUTATION self-checks:
//   - dropping `Depth: depth` / `Branch: branch` from the expandChain node
//     mint reds TestBuildWakeupChainStampsBranchIdentityP0E;
//   - passing a constant branch to the per-segment expandChain calls reds the
//     same test (the two target segments must land in DIFFERENT branches);
//   - dropping `impact.ChainBranch = branch` reds the impact assertions.

// branchIdentityTrace: target app-100 has TWO qualifying sleep segments —
// segment 1 (5.000–5.010) woken by depA-200, whose own sleep resolves one hop
// deeper to depC-400 (depth 2); segment 2 (5.012–5.019) woken by depB-300.
// Two top-level expansions ⇒ two branches with real per-branch depths.
const branchIdentityTrace = `
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
       depC-400 (400) [003] .... 4.995000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=depC next_pid=400 next_prio=30
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       depA-200 (200) [002] .... 5.001000: sched_switch: prev_comm=depA prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       depC-400 (400) [003] .... 5.008000: sched_wakeup: comm=depA pid=200 prio=40 target_cpu=002
       depA-200 (200) [002] .... 5.008500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=depA next_pid=200 next_prio=40
       depA-200 (200) [002] .... 5.010000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.010200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       depB-300 (300) [004] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.019200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
       depC-400 (400) [003] .... 5.019500: sched_switch: prev_comm=depC prev_pid=400 prev_prio=30 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
`

func TestBuildWakeupChainStampsBranchIdentityP0E(t *testing.T) {
	idx := buildTraceIndex(t, "p0e_branch_identity.systrace", branchIdentityTrace)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace})

	// Node identity: (pid → branch, depth). The target mints one depth-0 node
	// PER branch; depA/depC belong to the first (longer-sleep) branch, depB to
	// the second.
	type ident struct{ branch, depth int }
	byPID := map[int][]ident{}
	for _, node := range chain.Nodes {
		if node.Branch <= 0 {
			t.Fatalf("P0-E: every engine-minted node must carry its owning branch, got %+v", node)
		}
		byPID[node.Thread.PID] = append(byPID[node.Thread.PID], ident{node.Branch, node.Depth})
	}
	targets := byPID[100]
	if len(targets) != 2 || targets[0].depth != 0 || targets[1].depth != 0 || targets[0].branch == targets[1].branch {
		t.Fatalf("target must mint one depth-0 node per branch (two DISTINCT branches), got %+v", targets)
	}
	depA, depB, depC := byPID[200], byPID[300], byPID[400]
	if len(depA) != 1 || depA[0].depth != 1 {
		t.Fatalf("depA should sit at real depth 1 of its branch, got %+v", depA)
	}
	if len(depC) != 1 || depC[0].depth != 2 || depC[0].branch != depA[0].branch {
		t.Fatalf("depC should sit at real depth 2 of depA's branch, got depC=%+v depA=%+v", depC, depA)
	}
	if len(depB) != 1 || depB[0].depth != 1 || depB[0].branch == depA[0].branch {
		t.Fatalf("depB should sit at depth 1 of the OTHER branch, got depB=%+v depA=%+v", depB, depA)
	}

	// Edge identity: every edge carries its owning branch, matching its
	// waker's branch.
	wakerBranch := map[int]int{200: depA[0].branch, 300: depB[0].branch, 400: depC[0].branch}
	for _, edge := range chain.Edges {
		if edge.Branch <= 0 {
			t.Fatalf("P0-E: every engine edge must carry its owning branch, got %+v", edge)
		}
		if want := wakerBranch[edge.Waker.PID]; want != 0 && edge.Branch != want {
			t.Fatalf("edge branch %d disagrees with waker %d's node branch %d", edge.Branch, edge.Waker.PID, want)
		}
	}

	// Impact identity: chain impacts carry the same branch as their node.
	for _, impact := range chain.CausalImpacts {
		if impact.ChainBranch <= 0 {
			t.Fatalf("P0-E: every causal impact must carry its owning branch, got %+v", impact)
		}
		if want := wakerBranch[impact.Thread.PID]; want != 0 && impact.ChainBranch != want {
			t.Fatalf("impact branch %d disagrees with node branch %d for pid %d", impact.ChainBranch, want, impact.Thread.PID)
		}
	}
}

// 复核收尾② (对抗复核 MX1 突变逃逸, 2026-07-09): via-immune overflow
// expansions must continue the branch numbering AFTER the capped set — the
// MX1 mutant (nextBranch = len(branches)-1) collided the first kept overflow
// branch with the LAST capped branch's ordinal and the whole suite stayed
// green. With MaxBranches=1 the capped set is {1} and the via-kept overflow
// segment must mint ordinal 2 — never a re-used capped ordinal.
func TestExpandViaImmuneBranchOrdinalNoCollisionP0E(t *testing.T) {
	idx := buildTraceIndex(t, "p0e_via_ordinal.systrace", branchIdentityTrace)
	chain := BuildWakeupChain(idx, Query{
		PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.5,
		MaxBranches: 1, ViaThread: "depB-300", TraceFlavorHint: TraceFlavorHarmonyHitrace,
	})
	capped := map[int]bool{}
	viaBranch := 0
	for _, node := range chain.Nodes {
		switch node.Thread.PID {
		case 200, 400: // depA/depC — the capped (longest-segment) branch
			capped[node.Branch] = true
		case 300: // depB — the via-kept overflow branch
			viaBranch = node.Branch
		}
	}
	if len(capped) != 1 || !capped[1] {
		t.Fatalf("capped set must be exactly branch 1, got %v", capped)
	}
	if viaBranch != 2 {
		t.Fatalf("via-kept overflow branch must mint ordinal len(branches)+1 = 2, got %d", viaBranch)
	}
	if capped[viaBranch] {
		t.Fatalf("overflow ordinal %d collides with the capped set %v", viaBranch, capped)
	}
	if chain.ViaThread == nil || !chain.ViaThread.OnChain {
		t.Fatalf("via verdict must confirm the kept expansion, got %+v", chain.ViaThread)
	}
}

// 复核收尾③f: nil-impact transit nodes carry their TRUE typed depth — the
// pre-P0-E flattened walk defaulted them to 0 and sorted their edges last
// (the B1-b overshoot lesion). depB emits ONLY the wakeup line (no sched
// _switch rows in its aligned window), so its node minted with Impact==nil —
// exactly the transit shape; its Depth/Branch must still be real.
func TestBuildWakeupChainNilImpactTransitCarriesTrueDepthP0E(t *testing.T) {
	idx := buildTraceIndex(t, "p0e_transit_depth.systrace", branchIdentityTrace)
	chain := BuildWakeupChain(idx, Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace})
	for _, node := range chain.Nodes {
		if node.Thread.PID != 300 {
			continue
		}
		if node.Impact != nil {
			t.Fatalf("fixture drift: depB should be the nil-impact transit shape, got impact %+v", node.Impact)
		}
		if node.Depth != 1 || node.Branch <= 0 {
			t.Fatalf("nil-impact transit must carry its TRUE typed depth/branch, got depth=%d branch=%d", node.Depth, node.Branch)
		}
		return
	}
	t.Fatal("depB transit node missing")
}

// A single-thread aggregate whose occurrences span DIFFERENT branches carries
// NO branch identity (0) — absence never guesses (cross-branch aggregates have
// no single owning chain).
func TestWakeupCausalAggregateCrossBranchDropsBranchIdentityP0E(t *testing.T) {
	chain := &ChainResult{
		CausalImpacts: []WakeupCausalImpact{
			{Thread: ThreadRef{Comm: "dep", PID: 7}, ChainDepth: 1, ChainBranch: 1, TotalMs: 3, DominantState: string(StateSSleep), Window: TimeWindow{StartTs: 1, EndTs: 2}},
			{Thread: ThreadRef{Comm: "dep", PID: 7}, ChainDepth: 1, ChainBranch: 2, TotalMs: 4, DominantState: string(StateSSleep), Window: TimeWindow{StartTs: 3, EndTs: 4}},
		},
	}
	aggs := aggregateWakeupCausalImpacts(chain)
	if len(aggs) != 1 {
		t.Fatalf("expected one aggregate, got %+v", aggs)
	}
	if aggs[0].ChainBranch != 0 {
		t.Fatalf("cross-branch aggregate must not claim a single branch, got %d", aggs[0].ChainBranch)
	}

	same := &ChainResult{
		CausalImpacts: []WakeupCausalImpact{
			{Thread: ThreadRef{Comm: "dep", PID: 7}, ChainDepth: 1, ChainBranch: 3, TotalMs: 3, DominantState: string(StateSSleep), Window: TimeWindow{StartTs: 1, EndTs: 2}},
			{Thread: ThreadRef{Comm: "dep", PID: 7}, ChainDepth: 2, ChainBranch: 3, TotalMs: 4, DominantState: string(StateSSleep), Window: TimeWindow{StartTs: 3, EndTs: 4}},
		},
	}
	aggs = aggregateWakeupCausalImpacts(same)
	if len(aggs) != 1 || aggs[0].ChainBranch != 3 {
		t.Fatalf("same-branch aggregate keeps its branch identity, got %+v", aggs)
	}
}
