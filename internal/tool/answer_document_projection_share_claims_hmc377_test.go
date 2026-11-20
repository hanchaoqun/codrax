package tool

// HMC377 fix round 5 — the renderer names the seat holding a share only
// through the row's typed, engine-resolved share claims: the holder's [E#]
// when it renders on this board, the compaction form when it does not (or the
// engine did not resolve it), and a see-detail form on a node that carries no
// typed reference. It never searches the board for a holder by role marker,
// family or value.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc377Claim is one resolved typed claim naming its holder records.
func hmc377Claim(part tracequery.RootCauseShareClaimPart, role tracequery.RootCauseShareHolderRole, share float64, exact bool, holders ...string) types.TraceCausalProjectionShareClaim {
	return types.TraceCausalProjectionShareClaim{Part: string(part), Role: string(role), ShareMS: share,
		Resolved: len(holders) > 0, Exact: exact, HolderEvidenceIDs: holders}
}

// hmc377RepresentedClaim is a claim of a "represented by" sentence.
func hmc377RepresentedClaim(part tracequery.RootCauseShareClaimPart, role tracequery.RootCauseShareHolderRole, share float64, holders ...string) types.TraceCausalProjectionShareClaim {
	c := hmc377Claim(part, role, share, false, holders...)
	c.Represented = true
	return c
}

// hmc377R5ResidualProjection — review round 4 R1 delivered shape: the
// residual-lane seat (host_wakeup_edge_pre_state, no unwalked-seat marker)
// holds the satellite's 4 ms inside the unwalked wait; the engine resolved the
// satellite's claim to it.
func hmc377R5ResidualProjection(holder string) types.TraceCausalProjection {
	p := types.TraceCausalProjection{WakeupPath: []string{"dep-200", "app-100"}, WindowStartTs: 1.0, WindowEndTs: 1.1}
	p.OnChainCauses = append(p.OnChainCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "res-seat",
		Subject: "dep-200", Object: "runnable_wait", TypeToken: "runnable_wait",
		StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: 4, CumulativeImpactMS: 4, EffectiveImpactMS: 4,
		OnChainBasis: "host_wakeup_edge_pre_state", HostWakeupEdgeAnchorTS: 1.06, HostWakeupEdgeAnchorVia: "direct",
		ChainAnchoredMS: 4, ChainAnchorFullMS: 7,
		ShareClaims: []types.TraceCausalProjectionShareClaim{
			hmc377Claim(tracequery.RootCauseShareClaimRemainder, tracequery.RootCauseShareHolderRemainderSeat, 3, true, "res-twin")},
		Rank: 1, Tier: "primary", Confidence: 0.76, LineStart: 30, LineEnd: 31,
	})
	p.AdjacentCauses = append(p.AdjacentCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "res-twin",
		Subject: "dep-200", Object: "runnable_wait", TypeToken: "runnable_wait",
		StateKind: "runnable", ChainRelevance: "adjacent",
		ImpactMS: 3, CumulativeImpactMS: 3,
		ChainAnchoredMS: 4, ChainAnchorFullMS: 7, ChainAnchorRemainderSeat: true,
		ShareClaims: []types.TraceCausalProjectionShareClaim{
			hmc377Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderResidualSeat, 4, true, "res-seat")},
		Rank: 1, Confidence: 0.76, LineStart: 30, LineEnd: 73,
	}, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "res-sat",
		Subject: "dep-200", Object: "scheduler_latency", TypeToken: "scheduler_latency",
		StateKind: "runnable", ChainRelevance: "adjacent",
		ImpactMS: 3, CumulativeImpactMS: 3,
		ChainAnchoredMS: 0, ChainAnchorFullMS: 7, ChainAnchorUnwalkedMS: 4, ChainAnchorOutsideMS: 3,
		ChainAnchorRemainderSeat: true,
		ShareClaims: []types.TraceCausalProjectionShareClaim{
			hmc377Claim(tracequery.RootCauseShareClaimUnwalked, tracequery.RootCauseShareHolderResidualSeat, 4, holder != "", nonEmpty(holder)...)},
		Rank: 2, Confidence: 0.78, LineStart: 52, LineEnd: 73,
	})
	return p
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

func hmc377R5Rows(model runtimeTraceProjTreeModel, ids ...string) map[string]*runtimeTraceProjTreeRow {
	out := map[string]*runtimeTraceProjTreeRow{}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for i := range rows {
			for _, id := range ids {
				if rows[i].Node.EvidenceID == id {
					out[id] = &rows[i]
				}
			}
		}
	}
	return out
}

// Review round 4 R1 (TestRv4RenderResidualOwnerThreeWay): the share owned by
// the residual-lane seat is named by that seat's [E#] — through the claim,
// whatever role marker the seat wears.
func TestHMC377RenderNamesTheResidualOwnerThroughItsClaim(t *testing.T) {
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(hmc377R5ResidualProjection("res-seat"), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		rows := hmc377R5Rows(model, "res-sat", "res-seat")
		sat, seat := rows["res-sat"], rows["res-seat"]
		if sat == nil || seat == nil {
			t.Fatalf("fixture rows render")
		}
		if sat.ChainAnchorUnwalkedInvisible || sat.ChainAnchorUnwalkedUnreferenced || sat.ChainAnchorUnwalkedRef != strings.TrimSpace(seat.EvidenceTag) || !sat.ChainAnchorUnwalkedExact {
			t.Fatalf("zh=%t: the satellite names the residual seat %q: ref %q invisible %t unreferenced %t exact %t",
				zh, seat.EvidenceTag, sat.ChainAnchorUnwalkedRef, sat.ChainAnchorUnwalkedInvisible, sat.ChainAnchorUnwalkedUnreferenced, sat.ChainAnchorUnwalkedExact)
		}
		fence := strings.ReplaceAll(runtimeTraceProjTreeFence(model, zh), "\n", " ")
		if strings.Contains(fence, "未上本榜") || strings.Contains(fence, "not on this board") {
			t.Fatalf("zh=%t: the seat holding the share is on this board, the face says it is not:\n%s", zh, fence)
		}
		word := "(⛓唤醒锚定席)"
		if !zh {
			word = "(⛓ wakeup-anchored seat)"
		}
		if !rspaFenceContains(runtimeTraceProjTreeFence(model, zh), word) {
			t.Fatalf("zh=%t: the satellite's unwalked part names its seat:\n%s", zh, fence)
		}
	}
}

// The same claim, unresolved by the engine (its holder is off the published
// board): the face says so; a node compiled without any typed reference names
// no holder and claims no absence.
func TestHMC377RenderUnresolvedAndUnreferencedClaims(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(hmc377R5ResidualProjection(""), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	sat := hmc377R5Rows(model, "res-sat")["res-sat"]
	if sat == nil || !sat.ChainAnchorUnwalkedInvisible || sat.ChainAnchorUnwalkedRef != "" {
		t.Fatalf("an unresolved claim renders as compacted: %+v", sat)
	}
	if fence := runtimeTraceProjTreeFence(model, true); !strings.Contains(fence, "唤醒锚定席未上本榜") {
		t.Fatalf("an unresolved claim says its seat is not on this board:\n%s", fence)
	}
	p := hmc377R5ResidualProjection("res-seat")
	p.AdjacentCauses[1].ShareClaims = nil
	model = buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	sat = hmc377R5Rows(model, "res-sat")["res-sat"]
	if sat == nil || !sat.ChainAnchorUnwalkedUnreferenced || sat.ChainAnchorUnwalkedInvisible {
		t.Fatalf("a node without a typed reference is unreferenced, not compacted: %+v", sat)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "唤醒锚定席见明细") || strings.Contains(fence, "唤醒锚定席未上本榜") {
		t.Fatalf("an unreferenced node names no holder and claims no absence:\n%s", fence)
	}
}

// The pair sentence names the ONE seat the remainder's typed claim resolved
// to — a same-thread same-family row of the same value is never taken for it
// (round 4's renderer picked the counterpart by family and value).
func TestHMC377RenderPairReadsTheTypedHolderNotAValueMatch(t *testing.T) {
	remainder := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{EvidenceID: "rem",
		Subject: "JankManager-4321", TypeToken: "runnable_wait", StateKind: "runnable",
		ChainRelevance: "adjacent", ImpactMS: 29.432, CumulativeImpactMS: 29.432,
		ChainAnchoredMS: 1.759, ChainAnchorFullMS: 31.191, ChainAnchorRemainderSeat: true,
		ShareClaims: []types.TraceCausalProjectionShareClaim{
			hmc377Claim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderChainSeat, 1.759, true, "holder")},
	}, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E9"}
	decoy := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{EvidenceID: "decoy",
		Subject: "JankManager-4321", TypeToken: "runnable_wait", StateKind: "runnable",
		ChainRelevance: "on_chain", ImpactMS: 1.759, CumulativeImpactMS: 1.759, Rank: 2,
	}, Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: "E1"}
	holder := decoy
	holder.Node.EvidenceID, holder.Node.Rank, holder.EvidenceTag = "holder", 3, "E2"
	model := runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{decoy, holder}, Adjacent: []runtimeTraceProjTreeRow{remainder}}
	runtimeTraceProjMarkAccountRelations(&model, true)
	if model.Adjacent[0].AccountRelRef != "E2" || model.TreeRows[1].AccountRelRef != "E9" || model.TreeRows[0].AccountRelRef != "" {
		t.Fatalf("the pair names the typed holder E2 (never the equal-valued E1): rem→%q holder→%q decoy→%q",
			model.Adjacent[0].AccountRelRef, model.TreeRows[1].AccountRelRef, model.TreeRows[0].AccountRelRef)
	}
}

// A represented satellite whose holder renders on no surface of this board
// (or whose claim the engine did not resolve) never says its time is already
// represented on the chain tier and keeps its own value in the overview.
func TestHMC377RenderRepresentedHolderOffTheBoard(t *testing.T) {
	p := types.TraceCausalProjection{WakeupPath: []string{"app-100"}, WindowStartTs: 1.0, WindowEndTs: 1.1}
	p.AdjacentCauses = append(p.AdjacentCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "b-sat",
		Subject: "dep-200", Object: "scheduler_latency", TypeToken: "scheduler_latency",
		StateKind: "runnable", ChainRelevance: "adjacent",
		ImpactMS: 9, CumulativeImpactMS: 9,
		ChainAnchorRepresentedByChainSeat: true,
		ShareClaims: []types.TraceCausalProjectionShareClaim{
			hmc377RepresentedClaim(tracequery.RootCauseShareClaimAnchored, tracequery.RootCauseShareHolderClippedSeat, 9, "b-clipped-dead")},
		Rank: 2, Confidence: 0.78, LineStart: 10, LineEnd: 56,
	})
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := strings.ReplaceAll(runtimeTraceProjTreeFence(model, zh), "\n", " ")
		if strings.Contains(fence, "同段物理时间已由链上席全额代表") || strings.Contains(fence, "already fully represented on the chain tier") {
			t.Fatalf("zh=%t: the holder is off this board — no representation claim:\n%s", zh, fence)
		}
		want := "持有这段时间的本线程席未上本榜"
		if !zh {
			want = "the seat of this thread holding that time is not on this board"
		}
		if !strings.Contains(fence, want) {
			t.Fatalf("zh=%t: the satellite says the holding seat is off this board:\n%s", zh, fence)
		}
		row := hmc377R5Rows(model, "b-sat")["b-sat"]
		if row == nil || runtimeTraceProjElimRepresentedExcluded(*row) || runtimeTraceProjElimRepresentedOnBoard(*row) {
			t.Fatalf("zh=%t: the satellite is never excluded from the overview as represented", zh)
		}
	}
}
