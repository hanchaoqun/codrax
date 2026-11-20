package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 fix round 3 — the tree faces of the typed three-way split (Honeybird
// reboot witness numbers: Executor$Worker-10894's ready account 206.862 =
// 29.904 walked (the ⛓ inversion chain seat) + 0.012 unwalked (its own
// wakeup-anchored seat) + 176.946 outside (the ◇ remainder)). Every value the
// rows print is a typed engine part; no row computes a part as full − another.
func hmc377ThreeWayProjection(withOwnSeat, withChainSeat bool) types.TraceCausalProjection {
	p := types.TraceCausalProjection{
		WakeupPath:    []string{"Executor$Worker-10894", "app-10566"},
		WindowStartTs: 162.372184,
		WindowEndTs:   163.616645,
	}
	if withChainSeat {
		p.OnChainCauses = append(p.OnChainCauses, types.TraceCausalProjectionNode{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "hbr-chain",
			Subject: "Executor$Worker-10894", Object: "priority_inversion_candidate", TypeToken: "priority_inversion_candidate",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 29.904, CumulativeImpactMS: 29.983, EffectiveImpactMS: 29.904, GatedRunnableMS: 29.904,
			ChainAnchoredMS: 29.904, ChainAnchorFullMS: 206.862, ChainAnchorUnwalkedMS: 0.012, ChainAnchorOutsideMS: 176.946,
			Rank: 1, Tier: "primary", Confidence: 0.9, LineStart: 10, LineEnd: 20,
		})
	}
	if withOwnSeat {
		p.OnChainCauses = append(p.OnChainCauses, types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "hbr-own",
			Subject: "Executor$Worker-10894", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 0.012, CumulativeImpactMS: 0.012, EffectiveImpactMS: 0.012,
			OnChainBasis: "host_wakeup_edge_pre_state", HostWakeupEdgeAnchorTS: 163.315, HostWakeupEdgeAnchorVia: "direct",
			ChainAnchorUnwalkedSeat: true,
			Rank:                    2, Tier: "secondary", Confidence: 0.76, LineStart: 30, LineEnd: 31,
		})
	}
	p.AdjacentCauses = append(p.AdjacentCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "hbr-remainder",
		Subject: "Executor$Worker-10894", Object: "runnable_wait", TypeToken: "runnable_wait",
		StateKind: "runnable", ChainRelevance: "adjacent",
		ImpactMS: 176.946, CumulativeImpactMS: 176.946,
		ChainAnchoredMS: 29.904, ChainAnchorFullMS: 206.862, ChainAnchorUnwalkedMS: 0.012, ChainAnchorOutsideMS: 176.946,
		ChainAnchorRemainderSeat: true,
		Rank:                     1, Confidence: 0.8, LineStart: 10, LineEnd: 40,
	})
	return p
}

func hmc377ThreeWayRows(t *testing.T, model runtimeTraceProjTreeModel) (chain, own, rem *runtimeTraceProjTreeRow) {
	t.Helper()
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent} {
		for i := range rows {
			switch rows[i].Node.EvidenceID {
			case "hbr-chain":
				chain = &rows[i]
			case "hbr-own":
				own = &rows[i]
			case "hbr-remainder":
				rem = &rows[i]
			}
		}
	}
	return chain, own, rem
}

func TestHMC377ThreeWaySplitNamesEveryPartWithItsSeat(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(hmc377ThreeWayProjection(true, true), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	chain, own, rem := hmc377ThreeWayRows(t, model)
	if chain == nil || own == nil || rem == nil {
		t.Fatalf("fixture drifted: all three seats render (chain=%v own=%v rem=%v)", chain != nil, own != nil, rem != nil)
	}
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	for _, pin := range []string{
		"同源三分:全窗206.862ms=本行锚定29.904ms+唤醒锚定0.012ms(⛓唤醒锚定席)+其余176.946ms(◇余段席)",
		"同源三分:全窗206.862ms=锚定29.904ms(⛓链上席)+唤醒锚定0.012ms(⛓唤醒锚定席)+本行其余176.946ms(目标此时并未在等待本线程,无链上凭证)",
		"同源三分对席:◇席=[" + strings.TrimSpace(rem.EvidenceTag) + "],唤醒锚定席=[" + strings.TrimSpace(own.EvidenceTag) + "],⛓席=本行;三席合计还原全窗账 206.862ms(规则见图例)",
		"同源三分对席:⛓席=[" + strings.TrimSpace(chain.EvidenceTag) + "],唤醒锚定席=[" + strings.TrimSpace(own.EvidenceTag) + "],◇席=本行;三席合计还原全窗账 206.862ms(规则见图例)",
		"本行锚定 29.904ms(14%)",
	} {
		if !rspaFenceContains(fence, pin) {
			t.Fatalf("missing %q:\n%s", pin, fence)
		}
	}
	for _, stale := range []string{"其余176.958ms", "锚定29.916ms", "同源二分:全窗206.862ms", "链上锚定合计 29.904ms"} {
		if strings.Contains(fence, stale) {
			t.Fatalf("no row may compute a part as full − anchored or fold the unwalked part (%q):\n%s", stale, fence)
		}
	}
	if !model.Marks.has(runtimeTraceProjMarkChainAnchorThreeWay) || !model.Marks.has(runtimeTraceProjMarkChainAnchorRelation) {
		t.Fatalf("the three-way and relation legend marks record at their emission sites")
	}
	modelEN := buildRuntimeTraceProjTreeModel(hmc377ThreeWayProjection(true, true), newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	for _, pin := range []string{
		"same-source three-way split: whole-window 206.862ms=this row 29.904ms anchored + 0.012ms wakeup-anchored (⛓ wakeup-anchored seat) + remainder 176.946ms (◇ remainder seat)",
		"same-source three-way split: whole-window 206.862ms=29.904ms anchored (⛓ chain seat) + 0.012ms wakeup-anchored (⛓ wakeup-anchored seat) + this remainder 176.946ms (the analysed thread was not waiting for this thread then; no chain credential)",
		"the three seats' sum restores the whole-window account 206.862ms (rule in the legend)",
	} {
		if !rspaFenceContains(fenceEN, pin) {
			t.Fatalf("en missing %q:\n%s", pin, fenceEN)
		}
	}
}

// The unwalked seat compacted off the board: every sentence that names it
// says so, and the pair restores the full account with the three typed parts
// it names (HMC377 fix round 4: an off-board seat's value cannot be checked
// against the account's part, so the pair never claims the three SEATS add up
// — the account may hold only part of the thread's share).
func TestHMC377ThreeWaySplitSaysWhenTheWakeupAnchoredSeatIsOffTheBoard(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(hmc377ThreeWayProjection(false, true), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if strings.Contains(fence, "三席合计还原") {
		t.Fatalf("the off-board wakeup-anchored seat's value is unknown here — no three-seat sum claim:\n%s", fence)
	}
	for _, pin := range []string{
		"同源三分:全窗206.862ms=本行锚定29.904ms+唤醒锚定0.012ms(唤醒锚定席未上本榜,见明细)+其余176.946ms(◇余段席)",
		"同源三分:全窗206.862ms=锚定29.904ms(⛓链上席)+唤醒锚定0.012ms(唤醒锚定席未上本榜,见明细)+本行其余176.946ms",
		"唤醒锚定席未上本榜(见明细),⛓席=本行;锚定、唤醒锚定0.012ms、其余三份合计还原全窗账 206.862ms",
		"唤醒锚定席未上本榜(见明细),◇席=本行;锚定、唤醒锚定0.012ms、其余三份合计还原全窗账 206.862ms",
	} {
		if !rspaFenceContains(fence, pin) {
			t.Fatalf("missing %q:\n%s", pin, fence)
		}
	}
}

// The walked part's seat is off the board while the unwalked seat survives:
// the remainder must not call the surviving unwalked seat its ⛓ chain seat
// (round 2 counted any same-thread on-chain row as the walked twin), and the
// pair sentence is not minted with the unwalked seat as the walked half.
func TestHMC377UnwalkedSeatNeverStandsInForTheWalkedTwin(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(hmc377ThreeWayProjection(true, false), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	_, own, rem := hmc377ThreeWayRows(t, model)
	if own == nil || rem == nil {
		t.Fatalf("fixture drifted")
	}
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !rspaFenceContains(fence, "同源三分:全窗206.862ms=锚定29.904ms(锚定席未上本榜,见明细)+唤醒锚定0.012ms(⛓唤醒锚定席)+本行其余176.946ms") {
		t.Fatalf("the walked part's seat is off the board and the remainder says so:\n%s", fence)
	}
	if rem.AccountRelRef == strings.TrimSpace(own.EvidenceTag) {
		t.Fatalf("the unwalked seat was paired as the walked half: %+v", rem.AccountRelRef)
	}
}

// A two-way split (no unwalked part) keeps its bytes.
func TestHMC377TwoWaySplitKeepsItsBytes(t *testing.T) {
	p := hmc377ThreeWayProjection(false, true)
	p.OnChainCauses[0].ChainAnchorUnwalkedMS, p.OnChainCauses[0].ChainAnchorOutsideMS = 0, 0
	p.AdjacentCauses[0].ChainAnchorUnwalkedMS, p.AdjacentCauses[0].ChainAnchorOutsideMS = 0, 0
	p.AdjacentCauses[0].ChainAnchorFullMS, p.OnChainCauses[0].ChainAnchorFullMS = 206.850, 206.850
	model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	for _, pin := range []string{
		"同源二分:全窗206.850ms=本行锚定29.904ms+其余176.946ms(◇余段席)",
		"同源二分:全窗206.850ms=锚定29.904ms(⛓链上席)+本行其余176.946ms(无链上凭证)",
		"合计还原全窗账 206.850ms(规则见图例)",
		"链上锚定合计 29.904ms",
	} {
		if !rspaFenceContains(fence, pin) {
			t.Fatalf("two-way bytes changed, missing %q:\n%s", pin, fence)
		}
	}
	if strings.Contains(fence, "同源三分") || model.Marks.has(runtimeTraceProjMarkChainAnchorThreeWay) {
		t.Fatalf("a two-way split never speaks the three-way form:\n%s", fence)
	}
}

// hmc377ThreeWayLegendProjection is the representative shape for the legend
// sweeps: the three-way split plus an on-chain D/IO seat of another thread
// (the ⛓ glyph the ownership words reuse is the D/IO state icon, whose own
// legend entry renders with that seat — the rspa fixture's composition).
func hmc377ThreeWayLegendProjection() types.TraceCausalProjection {
	p := hmc377ThreeWayProjection(true, true)
	p.OnChainCauses = append(p.OnChainCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "hbr-dio",
		Subject: "CompThread-2955", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
		StateKind: "d_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: 3.598, CumulativeImpactMS: 3.598, EffectiveImpactMS: 3.598,
		Rank: 3, Tier: "secondary", Confidence: 0.8, LineStart: 50, LineEnd: 60,
	})
	return p
}
