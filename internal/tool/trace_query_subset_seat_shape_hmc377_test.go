package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hmc377R11SubsetShareProjection is the representative hand shape of the
// round-11 face (the 105 网易云 后台销毁 #4 nputmethod.anco-38135 geometry,
// case A): a chain seat holding the walked share, the thread's
// cpu_affinity_or_cpuset ◇ seat keeping its restricted instants outside it,
// and the ◇ runnable remainder whose rest is partly counted on that seat.
//
// EVOLUTION RECORD (HMC377 round 12, batch-owner ruling R-377-12 ③,
// 2026-11-22): old → new = the restriction node wore the thread's
// same-source split (ChainAnchorRemainderSeat, ChainAnchoredMS 7.413,
// ChainAnchorFullMS 36.968) and the display derived 本行计 as rest − share →
// the nodes carry the engine's round-12 wire: the restriction node its typed
// subset fields (RestrictedReadyAccountMS 36.968, ShareCountedElsewhereMS
// 7.413, ShareCountedHereMS 29.555), the remainder its share counted on the
// restriction row and its own count (29.555 / 68.749), and the ⛓ node its
// same-source split with a remainder claim the engine resolved to both rows
// that carry the rest (review-population w47 HeapTaskDaemon-26594: the ⛓
// row credited the whole rest 282.170 to ◇ while ◇ published 167.008 and the
// restriction row 115.162).
func hmc377R11SubsetShareProjection() types.TraceCausalProjection {
	chain := elimChainNode("r11-chain", "nputmethod.anco-38135", "runnable_wait", "runnable", 1, 7.413, 41)
	chain.ChainAnchorFullMS, chain.ChainAnchoredMS = 105.717, 7.413
	chain.ShareClaims = []types.TraceCausalProjectionShareClaim{
		{Part: string(tracequery.RootCauseShareClaimRemainder), Role: string(tracequery.RootCauseShareHolderRemainderSeat), ShareMS: 98.304,
			Resolved: true, HolderEvidenceIDs: []string{"r11-rem", "r11-cpu"}},
	}
	adjacent := func(id, typ string, value float64, line int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: "nputmethod.anco-38135", Predicate: "root_cause_adjacent", Object: typ, TypeToken: typ, StateKind: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Rank: 2, Tier: "tertiary",
			ImpactMS: value, CumulativeImpactMS: value, EffectiveImpactMS: value, RunnableMS: value,
			LineStart: line, LineEnd: line + 9, Confidence: 0.76,
		}
	}
	restriction := adjacent("r11-cpu", "cpu_affinity_or_cpuset", 29.555, 60)
	restriction.RestrictedReadyAccountMS, restriction.ShareCountedElsewhereMS, restriction.ShareCountedHereMS = 36.968, 7.413, 29.555
	restriction.ShareClaims = []types.TraceCausalProjectionShareClaim{
		{Part: string(tracequery.RootCauseShareClaimAnchored), Role: string(tracequery.RootCauseShareHolderChainSeat), ShareMS: 7.413,
			Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r11-chain"}},
	}
	remainder := adjacent("r11-rem", "runnable_wait", 68.749, 50)
	remainder.ChainAnchorRemainderSeat, remainder.ChainAnchoredMS, remainder.ChainAnchorFullMS = true, 7.413, 105.717
	remainder.ShareCountedElsewhereMS, remainder.ShareCountedHereMS = 29.555, 68.749
	remainder.ShareClaims = []types.TraceCausalProjectionShareClaim{
		{Part: string(tracequery.RootCauseShareClaimAnchored), Role: string(tracequery.RootCauseShareHolderChainSeat), ShareMS: 7.413,
			Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r11-chain"}},
		{Part: string(tracequery.RootCauseShareClaimSubset), Role: string(tracequery.RootCauseShareHolderSubsetSeat), ShareMS: 29.555,
			Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r11-cpu"}},
	}
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"app-100"},
		WindowStartTs:           10.0,
		WindowEndTs:             11.0,
		OnChainCauses: []types.TraceCausalProjectionNode{chain,
			elimChainNode("r11-dio", "worker-300", "d_state_or_io_wait", "d_sleep", 6, 3.0, 600)},
		AdjacentCauses: []types.TraceCausalProjectionNode{remainder, restriction},
	}
}

func TestHMC377R11SubsetShareHandShape(t *testing.T) {
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(hmc377R11SubsetShareProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := strings.Join(strings.Fields(runtimeTraceProjTreeFence(model, zh)), " ")
		cpu := xlane2FindRowByEvidenceID(&model, "r11-cpu")
		if cpu == nil || cpu.EvidenceTag == "" {
			t.Fatal("the restriction row must be drawn")
		}
		want := "其余98.304ms(无链上凭证;其中CPU亲和/cpuset限制下29.555ms另行计入[" + cpu.EvidenceTag + "],本行计68.749ms)"
		if !zh {
			want = "remainder 98.304ms (no chain credential; of which 29.555ms under the CPU affinity/cpuset restriction is counted separately on [" + cpu.EvidenceTag + "], this row 68.749ms)"
		}
		if !strings.Contains(strings.ReplaceAll(fence, "cpuset 限制", "cpuset限制"), want) {
			t.Fatalf("zh=%t: %q missing:\n%s", zh, want, fence)
		}
		if strings.Contains(fence, "合计还原全窗账 105.717ms") {
			t.Fatalf("zh=%t: the remainder must not claim the two-seat restoration:\n%s", zh, fence)
		}
		if !model.Marks.has(runtimeTraceProjMarkChainAnchorSubsetShare) {
			t.Fatalf("zh=%t: the 另行计入 mark must be set", zh)
		}
		chain := xlane2FindRowByEvidenceID(&model, "r11-chain")
		rem := xlane2FindRowByEvidenceID(&model, "r11-rem")
		if chain == nil || rem == nil || chain.EvidenceTag == "" || rem.EvidenceTag == "" {
			t.Fatal("the ⛓ and ◇ rows must be drawn")
		}
		// HMC377 round 12 (R-377-12 ③): the restriction row states its own
		// restricted account, never the thread's whole-window account, and
		// the ⛓ row names every carrier of its rest.
		restrictionWant := "CPU亲和/cpuset限制下合计36.968ms:其中7.413ms另行计入[" + chain.EvidenceTag + "],本行计29.555ms(无链上凭证)"
		chainWant := "其余98.304ms(◇余段席[" + rem.EvidenceTag + "]、CPU亲和/cpuset限制[" + cpu.EvidenceTag + "]分计)"
		if !zh {
			restrictionWant = "36.968ms under the CPU affinity/cpuset restriction in all: of which 7.413ms is counted separately on [" + chain.EvidenceTag + "]; this row 29.555ms (no chain credential)"
			chainWant = "remainder 98.304ms (counted by the ◇ remainder seat [" + rem.EvidenceTag + "] and the CPU affinity/cpuset restriction [" + cpu.EvidenceTag + "])"
		}
		// The fence wraps long lines under tree rails; compare on the text
		// with the rails dropped.
		joined := strings.ReplaceAll(strings.Join(strings.Fields(strings.ReplaceAll(fence, "│", " ")), " "), "cpuset 限制", "cpuset限制")
		if !strings.Contains(joined, restrictionWant) {
			t.Fatalf("zh=%t: the restriction row's own account %q missing:\n%s", zh, restrictionWant, fence)
		}
		if !strings.Contains(joined, chainWant) {
			t.Fatalf("zh=%t: the ⛓ row's rest carriers %q missing:\n%s", zh, chainWant, fence)
		}
		for _, banned := range []string{"全窗36.968ms", "full-window 36.968ms", "合计还原全窗账 36.968ms", "◇席=[" + cpu.EvidenceTag + "]", "◇ seat = [" + cpu.EvidenceTag + "]"} {
			if strings.Contains(fence, banned) {
				t.Fatalf("zh=%t: the restriction row must never state or be paired as the thread's whole-window account (%q):\n%s", zh, banned, fence)
			}
		}
	}
}
