package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// CHAIN-BUDGET engine pins (user rulings 2026-07-18, onchain_fix_spec
// 「CHAIN-BUDGET 批」+ 候选域钉死 + 预算尺度 + 分配优先序): depth>=1 nodes
// expand MULTIPLE value-ordered sleep segments within a three-layer budget
// (global MaxChainNodes node budget + per-node MaxBranches cap + untouched
// MaxDepth), with the guaranteed tier — depth-0 top-MaxBranches branches,
// per-node top-1 recursion — never budget-gated (退化恒等: the tightest
// budget reproduces the pre-CHAIN-BUDGET chain), the per-hop sched_wakeup
// credential gate byte-identical on both lanes, and budget exhaustion
// disclosed by ONE typed caveat.
//
// MUTATION self-checks:
//   - dropping the drain call (extras never expand) reds
//     TestChainBudgetExpandsMultipleSleepSegments;
//   - gating the guaranteed tier on the node budget reds
//     TestChainBudgetDegenerateFloorIsLegacyShape (n3 disappears);
//   - counting the credential-gate rejection into the unexpanded tally reds
//     TestChainBudgetEdgelessSegmentIsCredentialGateNotBudgetVictim;
//   - dropping the floor filter reds TestChainBudgetValueFloorExcludesNoise;
//   - removing max_chain_nodes from the board fingerprint reds the
//     MaxChainNodes arm of TestRootCauseBoardParamsFingerprintSplitsOnKnobChange.

// chainBudgetTrace: target app-100 sleeps once (5.000→5.040, woken by
// dep2-200). Inside dep2's edge-closed window [5.000, 5.040] dep2 has FOUR
// sleep segments:
//   - A 5.002→5.012 (10ms, top-1) woken by dep3-300 — the guaranteed lane;
//   - B 5.014→5.020 (6ms, top-2) woken by dep4-400 — the extra lane正臂;
//   - C 5.022→5.027 (5ms, top-3) with NO sched_wakeup row — the per-hop
//     credential hard-gate negative (no child, no edge, not a budget victim);
//   - D 5.028→5.0288 (0.8ms) woken by dep5-500 — carries a real edge but
//     sits below the 1ms extra-segment value floor: never a candidate, never
//     counted unexpanded (价值地板负臂).
//
// dep3/dep4 run flat in their own windows (running root evidence, no deeper
// recursion), keeping the shape two levels deep.
const chainBudgetTrace = `
        app-100 (100) [001] .... 4.999000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
       dep3-300 (300) [003] .... 5.001000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep3 next_pid=300 next_prio=30
       dep4-400 (400) [004] .... 5.001500: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep4 next_pid=400 next_prio=35
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       dep2-200 (200) [002] .... 5.002000: sched_switch: prev_comm=dep2 prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       dep3-300 (300) [003] .... 5.012000: sched_wakeup: comm=dep2 pid=200 prio=40 target_cpu=002
       dep2-200 (200) [002] .... 5.012200: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep2 next_pid=200 next_prio=40
       dep3-300 (300) [003] .... 5.012500: sched_switch: prev_comm=dep3 prev_pid=300 prev_prio=30 prev_state=R ==> next_comm=idle/3 next_pid=0 next_prio=120
       dep2-200 (200) [002] .... 5.014000: sched_switch: prev_comm=dep2 prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       dep4-400 (400) [004] .... 5.020000: sched_wakeup: comm=dep2 pid=200 prio=40 target_cpu=002
       dep2-200 (200) [002] .... 5.020200: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep2 next_pid=200 next_prio=40
       dep4-400 (400) [004] .... 5.020500: sched_switch: prev_comm=dep4 prev_pid=400 prev_prio=35 prev_state=R ==> next_comm=idle/4 next_pid=0 next_prio=120
       dep2-200 (200) [002] .... 5.022000: sched_switch: prev_comm=dep2 prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       dep2-200 (200) [002] .... 5.027000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep2 next_pid=200 next_prio=40
       dep2-200 (200) [002] .... 5.028000: sched_switch: prev_comm=dep2 prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       dep5-500 (500) [005] .... 5.028800: sched_wakeup: comm=dep2 pid=200 prio=40 target_cpu=002
       dep2-200 (200) [002] .... 5.028900: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep2 next_pid=200 next_prio=40
       dep2-200 (200) [002] .... 5.040000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.040200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func chainBudgetQuery() Query {
	return Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.041, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace}
}

func chainBudgetNodeByPID(chain ChainResult) map[int][]ChainNode {
	out := map[int][]ChainNode{}
	for _, node := range chain.Nodes {
		out[node.Thread.PID] = append(out[node.Thread.PID], node)
	}
	return out
}

// Pin ② — 多段展开正臂: with the default budget, dep2's top-2 sleep segment
// expands into its OWN sub-chain beside the guaranteed top-1: two children,
// two independent per-hop sched_wakeup credentials, distinct segment
// ordinals, each edge carrying its own wakeup row values.
func TestChainBudgetExpandsMultipleSleepSegments(t *testing.T) {
	idx := buildTraceIndex(t, "chain_budget_multi.systrace", chainBudgetTrace)
	chain := BuildWakeupChain(idx, chainBudgetQuery())
	byPID := chainBudgetNodeByPID(chain)
	if len(byPID[300]) != 1 || len(byPID[400]) != 1 {
		t.Fatalf("expected BOTH dep3 (top-1) and dep4 (top-2 extra) children, got dep3=%d dep4=%d nodes=%d", len(byPID[300]), len(byPID[400]), len(chain.Nodes))
	}
	dep3, dep4 := byPID[300][0], byPID[400][0]
	if dep3.SegmentOrdinal != 0 {
		t.Fatalf("the guaranteed top-1 child must keep segment_ordinal 0 (wire-stable absence), got %d", dep3.SegmentOrdinal)
	}
	if dep4.SegmentOrdinal != 2 {
		t.Fatalf("the extra child must carry its value-order segment ordinal 2, got %d", dep4.SegmentOrdinal)
	}
	if dep3.Branch != dep4.Branch || dep3.Depth != 2 || dep4.Depth != 2 {
		t.Fatalf("extra sub-chains stay in their parent's branch at the true depth, got dep3=%+v dep4=%+v", dep3, dep4)
	}
	// The edge-closed child windows are the segments' own credential windows.
	if dep4.Window.StartTs != 5.014 || dep4.Window.EndTs != 5.020 {
		t.Fatalf("extra child window must be the edge-closed [segment start, wakeup ts], got %+v", dep4.Window)
	}
	var edge3, edge4 *WakeupEdge
	for i := range chain.Edges {
		switch chain.Edges[i].Waker.PID {
		case 300:
			edge3 = &chain.Edges[i]
		case 400:
			edge4 = &chain.Edges[i]
		}
	}
	if edge3 == nil || edge4 == nil {
		t.Fatalf("both segment expansions must mint their own wakeup edge, got %+v", chain.Edges)
	}
	if edge3.SegmentOrdinal != 0 || edge4.SegmentOrdinal != 2 {
		t.Fatalf("edges must mirror the expansion-lane ordinals, got %d/%d", edge3.SegmentOrdinal, edge4.SegmentOrdinal)
	}
	if edge4.WakeupTs != 5.020 {
		t.Fatalf("the extra edge must carry ITS OWN sched_wakeup row, got %+v", edge4)
	}
	// No budget note: nothing at/above the floor was left unexpanded (C is a
	// credential-gate rejection, D is below the floor).
	for _, caveat := range chain.Caveats {
		if strings.Contains(caveat, "chain_expansion_budget_reached") {
			t.Fatalf("no budget disclosure may fire when nothing was budget-dropped: %q", caveat)
		}
	}
}

// Pin ⑤ — 逐跳凭证硬门: dep2's third sleep segment (5ms, above the floor)
// has no sched_wakeup row — it must not mint a child, an edge, or an extra
// missing_wakeup root-evidence seat, and it is a credential-gate rejection,
// NOT a budget victim (no budget disclosure).
// Pin ④ — 价值地板负臂: the 0.8ms segment D carries a REAL edge but sits
// below WakeupChainExtraSegmentFloorMs: never expanded, never counted.
func TestChainBudgetEdgelessSegmentIsCredentialGateNotBudgetVictim(t *testing.T) {
	idx := buildTraceIndex(t, "chain_budget_gate.systrace", chainBudgetTrace)
	chain := BuildWakeupChain(idx, chainBudgetQuery())
	byPID := chainBudgetNodeByPID(chain)
	if len(byPID[500]) != 0 {
		t.Fatalf("价值地板: the sub-floor 0.8ms segment must not expand (dep5 child minted)")
	}
	if got := len(byPID[200]); got != 1 {
		t.Fatalf("dep2 mints exactly one node, got %d", got)
	}
	missing := 0
	for _, ev := range chain.RootEvidence {
		if ev.Type == "missing_wakeup" && ev.Thread.PID == 200 {
			missing++
		}
	}
	if missing != 0 {
		t.Fatalf("an edge-less EXTRA segment must not multiply missing_wakeup seats (the node's top-1 arm owns that lane), got %d", missing)
	}
	for _, caveat := range chain.Caveats {
		if strings.Contains(caveat, "chain_expansion_budget_reached") {
			t.Fatalf("credential-gate rejections are not budget victims: %q", caveat)
		}
	}
}

// Pin ① — 退化恒等: at the tightest budget tier (max_chain_nodes=1) the
// chain is byte-identical to the pre-CHAIN-BUDGET single-most-interesting-
// interval recursion — the guaranteed tier is never budget-gated — plus
// exactly ONE typed budget disclosure counting every at/above-floor
// candidate left unexpanded (诚实披露在各档位如实, spec ③).
func TestChainBudgetDegenerateFloorIsLegacyShape(t *testing.T) {
	idx := buildTraceIndex(t, "chain_budget_floor.systrace", chainBudgetTrace)
	q := chainBudgetQuery()
	q.MaxChainNodes = 1
	chain := BuildWakeupChain(idx, q)
	byPID := chainBudgetNodeByPID(chain)
	// Legacy shape: app → dep2 → dep3 (top-1 spine only), dep4 never minted.
	if len(chain.Nodes) != 3 || len(byPID[100]) != 1 || len(byPID[200]) != 1 || len(byPID[300]) != 1 || len(byPID[400]) != 0 {
		t.Fatalf("tightest budget must reproduce the legacy top-1 spine (3 nodes, no dep4), got %d nodes byPID=%v", len(chain.Nodes), byPID)
	}
	if len(chain.Edges) != 2 {
		t.Fatalf("legacy spine has exactly 2 edges, got %d", len(chain.Edges))
	}
	for _, node := range chain.Nodes {
		if node.SegmentOrdinal != 0 {
			t.Fatalf("no extra-lane ordinal may appear at the tightest tier, got %+v", node)
		}
	}
	for _, edge := range chain.Edges {
		if edge.SegmentOrdinal != 0 {
			t.Fatalf("no extra-lane edge ordinal may appear at the tightest tier, got %+v", edge)
		}
	}
	// Pin ③ — 预算耗尽披露: N counts the at/above-floor candidates left
	// unexpanded — B (6ms, real edge) and C (5ms, edge-less but still an
	// unexplored at/above-floor candidate); the sub-floor D never counts.
	want := "chain_expansion_budget_reached=true; 2 additional candidate sleep segment(s) at/above the 1.000ms value floor were not expanded into the wakeup chain (global budget max_chain_nodes=1, per-node cap max_branches=16); raise max_chain_nodes or narrow the window if an unexpanded segment could carry the real dependency"
	found := 0
	for _, caveat := range chain.Caveats {
		if strings.Contains(caveat, "chain_expansion_budget_reached") {
			found++
			if caveat != want {
				t.Fatalf("budget disclosure wording drifted:\n got %q\nwant %q", caveat, want)
			}
		}
	}
	if found != 1 {
		t.Fatalf("exactly ONE budget disclosure must fire at the tightest tier, got %d", found)
	}
	// Byte-identity modulo the single disclosure line: the default-budget
	// build on a candidate-free prefix of the same machinery must serialize
	// identically — pinned structurally here by asserting the tight build's
	// faces equal the default build's guaranteed-lane subset.
	full := BuildWakeupChain(idx, chainBudgetQuery())
	tightJSON := chainBudgetFacesJSON(t, chain)
	legacyJSON := chainBudgetFacesJSON(t, chainBudgetStripExtras(full))
	if tightJSON != legacyJSON {
		t.Fatalf("tightest-budget faces must be byte-identical to the default build minus its extra-lane sub-chains:\n tight  %s\n legacy %s", tightJSON, legacyJSON)
	}
}

// chainBudgetStripExtras removes the extra-lane sub-chains (every node/edge/
// impact minted under a SegmentOrdinal>=2 expansion) from a chain — the
// guaranteed-lane subset the degenerate tier must equal byte-for-byte.
func chainBudgetStripExtras(chain ChainResult) ChainResult {
	drop := map[string]bool{}
	for _, edge := range chain.Edges {
		if edge.SegmentOrdinal >= 2 {
			drop[edge.From] = true
		}
	}
	// Transitively drop descendants of dropped roots.
	for changed := true; changed; {
		changed = false
		for _, edge := range chain.Edges {
			if drop[edge.To] && !drop[edge.From] {
				drop[edge.From] = true
				changed = true
			}
		}
	}
	droppedWindows := map[string]TimeWindow{}
	for _, node := range chain.Nodes {
		if drop[node.ID] {
			droppedWindows[node.ID] = node.Window
		}
	}
	var nodes []ChainNode
	for _, node := range chain.Nodes {
		if !drop[node.ID] {
			nodes = append(nodes, node)
		}
	}
	var edges []WakeupEdge
	for _, edge := range chain.Edges {
		if !drop[edge.From] && !drop[edge.To] && edge.SegmentOrdinal < 2 {
			edges = append(edges, edge)
		}
	}
	var impacts []WakeupCausalImpact
	for _, impact := range chain.CausalImpacts {
		dropped := false
		for _, w := range droppedWindows {
			if impact.Window == w && drop[chainBudgetImpactNodeID(chain, impact)] {
				dropped = true
				break
			}
		}
		if !dropped {
			impacts = append(impacts, impact)
		}
	}
	keptPID := map[int]bool{}
	for _, node := range nodes {
		keptPID[node.Thread.PID] = true
	}
	droppedPID := map[int]bool{}
	for _, node := range chain.Nodes {
		if drop[node.ID] {
			droppedPID[node.Thread.PID] = true
		}
	}
	var roots []RootEvidence
	for _, root := range chain.RootEvidence {
		if droppedPID[root.Thread.PID] && !keptPID[root.Thread.PID] {
			continue
		}
		roots = append(roots, root)
	}
	chain.Nodes, chain.Edges, chain.CausalImpacts, chain.RootEvidence = nodes, edges, impacts, roots
	return chain
}

func chainBudgetImpactNodeID(chain ChainResult, impact WakeupCausalImpact) string {
	for _, node := range chain.Nodes {
		if node.Thread.PID == impact.Thread.PID && node.Window == impact.Window {
			return node.ID
		}
	}
	return ""
}

// chainBudgetFacesJSON serializes the identity-bearing chain faces with node
// IDs re-normalized (extra-lane minting shifts n%d counters) and caveats
// excluded (the tight tier adds exactly the pinned budget disclosure).
func chainBudgetFacesJSON(t *testing.T, chain ChainResult) string {
	t.Helper()
	rename := map[string]string{}
	for i, node := range chain.Nodes {
		rename[node.ID] = fmt.Sprintf("N%d", i+1)
	}
	type faceEdge struct {
		From, To string
		WakeupTs float64
		Line     int
		Ordinal  int
	}
	type faceNode struct {
		ID     string
		PID    int
		Window TimeWindow
		Depth  int
		Branch int
	}
	var nodes []faceNode
	for _, node := range chain.Nodes {
		nodes = append(nodes, faceNode{rename[node.ID], node.Thread.PID, node.Window, node.Depth, node.Branch})
	}
	var edges []faceEdge
	for _, edge := range chain.Edges {
		edges = append(edges, faceEdge{rename[edge.From], rename[edge.To], edge.WakeupTs, edge.WakeupLine, edge.SegmentOrdinal})
	}
	blob, err := json.Marshal(struct {
		Nodes   []faceNode
		Edges   []faceEdge
		Impacts []WakeupCausalImpact
		Roots   []RootEvidence
	}{nodes, edges, chain.CausalImpacts, chain.RootEvidence})
	if err != nil {
		t.Fatal(err)
	}
	return string(blob)
}

// Pin ②支臂 — 每节点子分支帽(MaxBranches 泛化到全深度): with max_branches=2
// a node spends one slot on the guaranteed top-1 and may register exactly ONE
// extra candidacy — top-2 (B) expands, top-3 (C) is trimmed at candidacy and
// counted into the honest unexpanded tally.
func TestChainBudgetPerNodeCapTrimsCandidacy(t *testing.T) {
	idx := buildTraceIndex(t, "chain_budget_pernode.systrace", chainBudgetTrace)
	q := chainBudgetQuery()
	q.MaxBranches = 2
	chain := BuildWakeupChain(idx, q)
	byPID := chainBudgetNodeByPID(chain)
	if len(byPID[400]) != 1 {
		t.Fatalf("top-2 must still expand under the per-node cap of 2, got %v", byPID)
	}
	// 返工 P3① 旋钮词面: this victim died at the PER-NODE cap, so the remedy
	// names max_branches (raising max_chain_nodes cannot help it).
	want := "chain_expansion_budget_reached=true; 1 additional candidate sleep segment(s) at/above the 1.000ms value floor were not expanded into the wakeup chain (global budget max_chain_nodes=96, per-node cap max_branches=2); raise max_branches or narrow the window if an unexpanded segment could carry the real dependency"
	found := 0
	for _, caveat := range chain.Caveats {
		if strings.Contains(caveat, "chain_expansion_budget_reached") {
			found++
			if caveat != want {
				t.Fatalf("per-node-cap disclosure drifted:\n got %q\nwant %q", caveat, want)
			}
		}
	}
	if found != 1 {
		t.Fatalf("the per-node-cap trim must disclose exactly once, got %d", found)
	}
}

// Pin ⑧ — 确定性: the same trace under the same parameters builds a
// byte-identical chain twice (fresh index each time; the frontier ordering
// and drain are typed-total-ordered, zero randomness).
func TestChainBudgetDeterministicRebuild(t *testing.T) {
	idxA := buildTraceIndex(t, "chain_budget_det_a.systrace", chainBudgetTrace)
	idxB := buildTraceIndex(t, "chain_budget_det_b.systrace", chainBudgetTrace)
	a, err := json.Marshal(BuildWakeupChain(idxA, chainBudgetQuery()))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(BuildWakeupChain(idxB, chainBudgetQuery()))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("chain build must be byte-reproducible under fixed parameters")
	}
}

// Pin ①(真 trace 锚) + ⑨(tieba 反形收敛量化) — the real-trace acceptance
// baseline from onchain_segment_audit_20260718.md: at the OLD caps
// (max_branches=8, tightest node budget) the tieba 59566 flagship chain must
// reproduce the pre-CHAIN-BUDGET shape captured on main 5b891792b
// (24 nodes / 16 edges / 8 root-evidence seats), and at the new defaults the
// audited reverse-form leak (真边段未锚定) must converge: 59843
// 5.436ms(20.7%) → ≤1.0ms, 60595 5.173ms(26.0%) → ≤1.5ms.
func TestChainBudgetTiebaLeakConvergesAndLegacyAnchorHolds(t *testing.T) {
	const trace = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("real-trace fixture absent: %v", err)
	}
	idx, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	base := Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805}

	legacy := base
	legacy.MaxBranches = 8
	legacy.MaxChainNodes = 1
	legacyChain := BuildWakeupChain(idx, legacy)
	if len(legacyChain.Nodes) != 24 || len(legacyChain.Edges) != 16 || len(legacyChain.RootEvidence) != 8 || len(legacyChain.CausalImpacts) != 24 {
		t.Fatalf("退化恒等 real-trace anchor drifted from the main-5b891792b capture (want 24/16/8/24): nodes=%d edges=%d rootEv=%d impacts=%d",
			len(legacyChain.Nodes), len(legacyChain.Edges), len(legacyChain.RootEvidence), len(legacyChain.CausalImpacts))
	}

	chain := BuildWakeupChain(idx, base)
	anchors := chainAnchorWindowsByPID(chain)
	chainPIDs := map[int]bool{chain.Target.PID: true}
	for _, n := range chain.Nodes {
		if n.Thread.PID > 0 {
			chainPIDs[n.Thread.PID] = true
		}
	}
	qa := base
	qa.chainAnchorWindowsByPID = anchors
	stats := ComputeWindowStats(idx, qa)
	missed := map[int]float64{}
	scan := func(census map[string]ThreadDuration, dio bool) {
		for _, td := range census {
			if td.Thread.PID <= 0 || len(anchors[td.Thread.PID]) == 0 {
				continue
			}
			ivs := td.runnableIntervals
			if dio {
				ivs = td.dioIntervals
			}
			for _, iv := range ivs {
				edgeIn := false
				for _, ev := range idx.Events {
					if (ev.Type == EventSchedWakeup || ev.Type == EventSchedWaking) && ev.PID == td.Thread.PID &&
						chainPIDs[ev.WakeePID] && ev.Ts >= iv.start && ev.Ts <= iv.end+rspaIOCompletionClosureTolS {
						edgeIn = true
						break
					}
				}
				if !edgeIn {
					continue
				}
				anch := anchorWindowsOverlapMs(anchors[td.Thread.PID], iv.start, iv.end)
				missed[td.Thread.PID] += (iv.end-iv.start)*1000 - anch
			}
		}
	}
	scan(stats.runnableCensus, false)
	scan(stats.dstateCensus, true)
	scan(stats.iowaitCensus, true)
	if missed[59843] > 1.0 {
		t.Fatalf("tieba 59843 reverse-form leak must converge from the audited 5.436ms(20.7%%) to <=1.0ms, got %.3fms", missed[59843])
	}
	if missed[60595] > 1.5 {
		t.Fatalf("tieba 60595 reverse-form leak must converge from the audited 5.173ms(26.0%%) to <=1.5ms, got %.3fms", missed[60595])
	}
}

// chainBudgetContestTrace (返工 P1-1/P1-2a): dep2 carries THREE extra sleep
// segments beyond its guaranteed top-1 (A 10ms → dep3):
//   - B2 4ms EARLY  (5.013→5.017) woken by dep6-600 (dep6 has no sched data);
//   - B  6ms LATER  (5.019→5.025) woken by dep4-400 (dep4 has no sched data);
//   - B3 2ms LAST   (5.027→5.029) woken by dep6-600 again.
//
// Value order B(6) > B2(4) > B3(2) OPPOSES start-ts order B2 < B < B3, so a
// mid-drain budget exhaustion observably separates the greedy value order
// from every rival order (ts-first, asc, registration). dep6 is visited by
// TWO expansions → two nodes → the zero-value trace_gap source fold is
// observable on the same fixture.
const chainBudgetContestTrace = `
        app-100 (100) [001] .... 4.999000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
       dep3-300 (300) [003] .... 5.001000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep3 next_pid=300 next_prio=30
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       dep2-200 (200) [002] .... 5.002000: sched_switch: prev_comm=dep2 prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       dep3-300 (300) [003] .... 5.012000: sched_wakeup: comm=dep2 pid=200 prio=40 target_cpu=002
       dep2-200 (200) [002] .... 5.012200: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep2 next_pid=200 next_prio=40
       dep3-300 (300) [003] .... 5.012500: sched_switch: prev_comm=dep3 prev_pid=300 prev_prio=30 prev_state=R ==> next_comm=idle/3 next_pid=0 next_prio=120
       dep2-200 (200) [002] .... 5.013000: sched_switch: prev_comm=dep2 prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       dep6-600 (600) [006] .... 5.017000: sched_wakeup: comm=dep2 pid=200 prio=40 target_cpu=002
       dep2-200 (200) [002] .... 5.017200: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep2 next_pid=200 next_prio=40
       dep2-200 (200) [002] .... 5.019000: sched_switch: prev_comm=dep2 prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       dep4-400 (400) [004] .... 5.025000: sched_wakeup: comm=dep2 pid=200 prio=40 target_cpu=002
       dep2-200 (200) [002] .... 5.025200: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep2 next_pid=200 next_prio=40
       dep2-200 (200) [002] .... 5.027000: sched_switch: prev_comm=dep2 prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
       dep6-600 (600) [006] .... 5.029000: sched_wakeup: comm=dep2 pid=200 prio=40 target_cpu=002
       dep2-200 (200) [002] .... 5.029200: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep2 next_pid=200 next_prio=40
       dep2-200 (200) [002] .... 5.040000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.040200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

// 返工 P1-1 — 贪心分配序 pin: the budget exhausts MID-DRAIN with two live
// rival candidates, so the value-desc allocation order becomes OBSERVABLE:
// max_chain_nodes=4 admits exactly ONE extra sub-chain beyond the guaranteed
// spine (app, dep2, dep3), and the winner MUST be the largest-value segment
// B (6ms → dep4) even though the rival B2 (4ms → dep6) starts EARLIER and
// was registered EARLIER. Mutations that red this pin: value-asc flip (B3
// wins → dep6), ts-first flip (B2 wins → dep6), registration-order drain
// (B2 wins → dep6). The two losers join the honest budget tally (N=2,
// budget knob wording — they died at the global admission gate).
func TestChainBudgetDrainGreedyValueOrderUnderExhaustion(t *testing.T) {
	idx := buildTraceIndex(t, "chain_budget_contest.systrace", chainBudgetContestTrace)
	q := chainBudgetQuery()
	q.MaxChainNodes = 4
	chain := BuildWakeupChain(idx, q)
	byPID := chainBudgetNodeByPID(chain)
	if len(byPID[400]) != 1 {
		t.Fatalf("greedy value order must expand the LARGEST-value extra segment (6ms → dep4) first, got byPID=%v", byPID)
	}
	if len(byPID[600]) != 0 {
		t.Fatalf("the smaller/earlier rivals must NOT win the last budget slot (value order, not ts/registration order), got dep6 nodes=%d", len(byPID[600]))
	}
	if len(chain.Nodes) != 4 {
		t.Fatalf("max_chain_nodes=4 admits exactly one extra sub-chain over the 3-node guaranteed spine, got %d nodes", len(chain.Nodes))
	}
	var winner *WakeupEdge
	for i := range chain.Edges {
		if chain.Edges[i].Waker.PID == 400 {
			winner = &chain.Edges[i]
		}
	}
	if winner == nil || winner.SegmentOrdinal != 2 {
		t.Fatalf("the winner is dep2's top-2 BY VALUE (segment ordinal 2), got %+v", winner)
	}
	want := "chain_expansion_budget_reached=true; 2 additional candidate sleep segment(s) at/above the 1.000ms value floor were not expanded into the wakeup chain (global budget max_chain_nodes=4, per-node cap max_branches=16); raise max_chain_nodes or narrow the window if an unexpanded segment could carry the real dependency"
	found := 0
	for _, caveat := range chain.Caveats {
		if strings.Contains(caveat, "chain_expansion_budget_reached") {
			found++
			if caveat != want {
				t.Fatalf("mid-drain exhaustion disclosure drifted:\n got %q\nwant %q", caveat, want)
			}
		}
	}
	if found != 1 {
		t.Fatalf("exactly ONE budget disclosure must fire on mid-drain exhaustion, got %d", found)
	}
}

// 返工 P1-2a — 零值 trace_gap 源头 fold pin: dep6 is visited by TWO budget
// expansions (B2 and B3) and has no scheduler data — two nodes on the chain
// face (lossless per-window summaries), but exactly ONE zero-value trace_gap
// RootEvidence disclosure per (pid, gapKind).「同 pid 双行面目全同」重复席
// 是 REJECT P1-2 病形本体; dropping the mint-site fold reds this pin (2 rows).
func TestChainBudgetZeroTraceGapRootEvidenceFoldsPerPidKind(t *testing.T) {
	idx := buildTraceIndex(t, "chain_budget_gapfold.systrace", chainBudgetContestTrace)
	chain := BuildWakeupChain(idx, chainBudgetQuery())
	byPID := chainBudgetNodeByPID(chain)
	if len(byPID[600]) != 2 {
		t.Fatalf("both dep6 segments must expand under the default budget (node face lossless), got %d", len(byPID[600]))
	}
	gaps := 0
	for _, root := range chain.RootEvidence {
		if root.Type == "trace_gap" && root.Thread.PID == 600 {
			gaps++
			if root.GapKind != TraceGapKindNoSchedData {
				t.Fatalf("dep6 has no scheduler rows — the typed criterion must say so, got %q", root.GapKind)
			}
		}
	}
	if gaps != 1 {
		t.Fatalf("zero-value trace_gap RootEvidence must fold per (pid,gapKind): want exactly 1 dep6 row, got %d", gaps)
	}
}

// chainBudgetViaTrace (返工 P2-3, 对抗官腐坏形探针收编): the target has THREE
// qualifying depth-0 sleep segments; with max_branches=2 the third (S3 6ms →
// dep5) is a via-immunity overflow probe. dep5's own window carries a top-1
// sleep (→ dep7) plus an EXTRA sleep segment (→ dep8) that registers into the
// budget frontier DURING the probe expansion. The via selector matches
// nothing in that subtree, so the whole probe rolls back — and the
// registered extra MUST roll back with it, or the drain expands a side chain
// of nodes that no longer exist (悬挂边/幻影节点).
const chainBudgetViaTrace = `
        app-100 (100) [001] .... 4.999000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
       dep2-200 (200) [002] .... 5.000500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep2 next_pid=200 next_prio=40
       dep3-300 (300) [003] .... 5.011500: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep3 next_pid=300 next_prio=30
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       dep2-200 (200) [002] .... 5.010000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.010200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
       dep2-200 (200) [002] .... 5.010900: sched_switch: prev_comm=dep2 prev_pid=200 prev_prio=40 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       dep3-300 (300) [003] .... 5.020000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.020200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
       dep3-300 (300) [003] .... 5.020900: sched_switch: prev_comm=dep3 prev_pid=300 prev_prio=30 prev_state=R ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.022000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       dep5-500 (500) [005] .... 5.022500: sched_switch: prev_comm=dep5 prev_pid=500 prev_prio=45 prev_state=S ==> next_comm=idle/5 next_pid=0 next_prio=120
       dep7-700 (700) [007] .... 5.024500: sched_wakeup: comm=dep5 pid=500 prio=45 target_cpu=005
       dep5-500 (500) [005] .... 5.024600: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep5 next_pid=500 next_prio=45
       dep5-500 (500) [005] .... 5.025000: sched_switch: prev_comm=dep5 prev_pid=500 prev_prio=45 prev_state=S ==> next_comm=idle/5 next_pid=0 next_prio=120
       dep8-800 (800) [008] .... 5.026500: sched_wakeup: comm=dep5 pid=500 prio=45 target_cpu=005
       dep5-500 (500) [005] .... 5.026600: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep5 next_pid=500 next_prio=45
       dep5-500 (500) [005] .... 5.028000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.028200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

// 返工 P2-3 — via 回滚负臂 pin: a rolled-back via overflow probe leaves ZERO
// residue — no phantom nodes from its subtree (dep5/dep7/dep8), no dangling
// edge endpoints, and no drained side chain hanging off a removed node.
// Stripping the frontier truncation from the rollback arm reds this pin: the
// drain then expands dep8's registered extra against a parent node that no
// longer exists (dangling edge / phantom node 形).
func TestChainBudgetViaRollbackDropsRegisteredExtras(t *testing.T) {
	idx := buildTraceIndex(t, "chain_budget_via.systrace", chainBudgetViaTrace)
	q := chainBudgetQuery()
	q.MaxBranches = 2
	q.ViaThread = "dep2"
	chain := BuildWakeupChain(idx, q)
	byPID := chainBudgetNodeByPID(chain)
	for _, pid := range []int{500, 700, 800} {
		if len(byPID[pid]) != 0 {
			t.Fatalf("rolled-back via probe subtree must leave no phantom node, got pid=%d nodes=%d (all=%v)", pid, len(byPID[pid]), byPID)
		}
	}
	ids := map[string]bool{}
	for _, node := range chain.Nodes {
		ids[node.ID] = true
	}
	for _, edge := range chain.Edges {
		if !ids[edge.From] || !ids[edge.To] {
			t.Fatalf("dangling edge endpoint after via rollback: %s -> %s (nodes=%v)", edge.From, edge.To, ids)
		}
	}
	// Pristine shape: the two kept branches (S1 → dep2, S2 → dep3) and the
	// target's two branch nodes — four nodes, zero dangling.
	if len(chain.Nodes) != 4 || len(byPID[100]) != 2 || len(byPID[200]) != 1 || len(byPID[300]) != 1 {
		t.Fatalf("pristine via-rollback shape drifted (want 4 nodes: app x2, dep2, dep3), got %d nodes byPID=%v", len(chain.Nodes), byPID)
	}
	// No budget disclosure: the rolled-back candidate is not a budget victim.
	for _, caveat := range chain.Caveats {
		if strings.Contains(caveat, "chain_expansion_budget_reached") {
			t.Fatalf("a rolled-back via probe's extras are not budget victims: %q", caveat)
		}
	}
}

// 返工 P1-2 回归绿 (tieba 板): the REJECT squeeze shape on the real trace —
// the wider chain minted FOUR identical zero-value trace_gap rows for
// 59843 (a thread holding the board's rank#1 valued seat), flooding the
// bounded gap side lane, halving the target-side cap, and killing the
// RenderThread/KitAnalyticsTim sleep_wait context disclosures (挤出病形).
// Post-fix: one folded RootEvidence row at the chain source, ZERO 59843
// blind-spot rank rows (同窗去重 — the thread has valued data in this very
// window), and all four target-side context rows back on the published
// board. Reverting either fix layer reds this pin.
func TestChainBudgetTiebaBoardSideLaneRegression(t *testing.T) {
	const trace = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("real-trace fixture absent: %v", err)
	}
	idx, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	chain := BuildWakeupChain(idx, q)
	gapRows := 0
	for _, root := range chain.RootEvidence {
		if root.Type == "trace_gap" && root.Thread.PID == 59843 {
			gapRows++
		}
	}
	if gapRows != 1 {
		t.Fatalf("source fold: the chain must carry exactly ONE 59843 blind-spot disclosure (pre-fix flood: 4 identical rows), got %d", gapRows)
	}
	rank := BuildRootCauseRank(idx, q)
	var sleep59891, sleep3535, span9388, span7640 bool
	for _, item := range rank.Items {
		if item.Type == "trace_gap" && item.Thread.PID == 59843 {
			t.Fatalf("同窗去重: 59843 holds a valued seat on this board — its zero blind-spot rank row must not publish (pre-fix: two identical published rows), got %+v", item)
		}
		switch {
		case item.Type == "sleep_wait" && item.Thread.PID == 59891:
			sleep59891 = true
		case item.Type == "sleep_wait" && item.Thread.PID == 3535:
			sleep3535 = true
		case item.Type == "trace_span" && item.Thread.PID == 9388:
			span9388 = true
		case item.Type == "trace_span" && item.Thread.PID == 7640:
			span7640 = true
		}
	}
	if !sleep59891 || !sleep3535 {
		t.Fatalf("挤出回归: both sleep_wait context disclosures must be back on the published board (59891=%v 3535=%v)", sleep59891, sleep3535)
	}
	if !span9388 || !span7640 {
		t.Fatalf("the pre-existing trace_span context disclosures must survive the fix (9388=%v 7640=%v)", span9388, span7640)
	}
}

// 返工 P1-2 回归绿 (donghu 17267 板): the valued pacing_idle 15.758 context
// disclosure died at the halved target-side cap behind a smaller newcomer
// (blind channel-first fill); post-fix the cap fills true-value rows first
// (帽内先保真值行) and the row is back. Zero-value gap rows publish at most
// once per (pid,kind) and only for threads WITHOUT a valued seat on the
// same board.
func TestChainBudgetDonghu17267BoardSideLaneRegression(t *testing.T) {
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("real-trace fixture absent: %v", err)
	}
	idx, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
	rank := BuildRootCauseRank(idx, q)
	valuedThreads := map[int]bool{}
	for _, item := range rank.Items {
		if item.Type != "trace_gap" && rootCauseEffectiveImpactMs(item) > 0 {
			valuedThreads[item.Thread.PID] = true
		}
	}
	var pacing bool
	gapSeats := map[int]int{}
	for _, item := range rank.Items {
		if item.Type == "pacing_idle" && item.Thread.PID == 17267 && item.EffectiveImpactMs > 15 {
			pacing = true
		}
		if item.Type == "trace_gap" {
			gapSeats[item.Thread.PID]++
		}
	}
	if !pacing {
		t.Fatalf("挤出回归: the valued pacing_idle 15.758 context row must be back on the published board")
	}
	for pid, seats := range gapSeats {
		if seats != 1 {
			t.Fatalf("零值行不再重复占席: pid=%d publishes %d blind-spot rows", pid, seats)
		}
		if valuedThreads[pid] {
			t.Fatalf("同窗去重: pid=%d holds a valued seat yet still publishes a blind-spot row", pid)
		}
	}
}

// Pin ⑩ 支臂(引擎侧): the exploration budget is separate from the LLM view
// budget — a chain larger than the typed family row cap still builds fully
// (nodes/edges faces complete); the tool layer's row caps own the prompt
// face. Pinned here as: the donghu 2955 default build exceeds the legacy
// shape yet stays within the global node budget.
func TestChainBudgetGlobalNodeBudgetBounds(t *testing.T) {
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("real-trace fixture absent: %v", err)
	}
	idx, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	chain := BuildWakeupChain(idx, Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898})
	if len(chain.Nodes) <= 36 {
		t.Fatalf("the new budget must expand beyond the legacy 36-node donghu 2955 chain, got %d", len(chain.Nodes))
	}
	// MaxDepth-bounded overshoot only: the admission gate runs before every
	// extra expansion, so the node count may exceed the budget by at most one
	// sub-chain (<= MaxDepth nodes).
	cap := ViewCapacityFor("wakeup_chain")
	if len(chain.Nodes) > cap.MaxChainNodes+cap.MaxDepth {
		t.Fatalf("global node budget must bound the chain (%d + %d overshoot), got %d", cap.MaxChainNodes, cap.MaxDepth, len(chain.Nodes))
	}
	extras := 0
	for _, edge := range chain.Edges {
		if edge.SegmentOrdinal >= 2 {
			extras++
		}
	}
	if extras == 0 {
		t.Fatalf("donghu 2955 carries a measured 6-segment extras inventory; the extra lane must fire")
	}
}
