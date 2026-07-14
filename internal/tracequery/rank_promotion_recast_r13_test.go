package tracequery

import (
	"fmt"
	"testing"
)

// rank_promotion_recast_r13_test.go — 新裁定 B「升舱换值」不变量 pin (R13,
// rank_order_v2_design_20260712.md §6.2/§8 R13, GREENLIT 2026-07-12; wording
// finalized per RANK-U design §2.2-3, ledger §29.64 遗留项, 2026-07-13).
//
// Ruling: when a ◇/▒ row is promoted onto the CHAIN channel, its published
// effective MUST be a target-thread-window wall-clock caliber — a cross-
// thread-caliber value may never ride a channel flip onto the board (the 4165
// wc_srvinit 54.608 假#1 借道形). The pin is deliberately NOT「禁一切携值」:
// value carry is LEGAL exactly when the value's caliber is already the target
// window's wall clock, witnessed by a typed marker —
//
//   - the SELF-SEM basis (OnChainBasis=self_deterministic_span): the value is
//     the target thread's own in-window projection union by mint construction
//     (§29.61.1 ②) — the enrich keep arm preserves relevance AND value;
//   - compute_supply: minted on the supply-fold lane whose effective IS the
//     target-window supply-discounted deficit (§29.27 canonical ①) — the one
//     cross-thread-additivity token that is direct-on-chain capable, and its
//     on-chain value is target-caliber by mint, never a carried aggregate.
//
// The engine currently has NO off-chain-mint→on-chain value-carry path
// (mint-time channel + the enrich demotion arms close every route); these
// pins freeze that shape as an explicit invariant against future edits.
//
// MUTATION self-checks (recorded, cp-copy recovery only — never git checkout):
//   - adding a cross-thread aggregate token to rootCauseTypeCanBeDirectOnChain
//     reds TestR13DirectOnChainCrossThreadCensusIdentity;
//   - deleting the E-Gap③ aggregate demotion arm (or the
//     rootCauseItemCanBeDirectOnChain gate) in
//     enrichRootCauseItemsWithChainContext reds
//     TestR13CrossThreadAggregateNeverPromotedCarryingValue.

// TestR13DirectOnChainCrossThreadCensusIdentity is the census identity:
// direct-on-chain capability ∩ cross-thread additivity == {compute_supply}.
// Any future token entering BOTH sets must consciously break this pin and
// justify a mint-time target-caliber recast (the compute_supply precedent),
// otherwise the 54.608 carry form re-opens.
func TestR13DirectOnChainCrossThreadCensusIdentity(t *testing.T) {
	var crossThreadDirect []string
	for _, token := range CausalTokenUniverse() {
		spec, ok := CausalTokenSpecFor(token)
		if !ok || !rootCauseTypeCanBeDirectOnChain(token) {
			continue
		}
		if spec.Additivity == CausalAdditivityCrossThreadCPUms {
			crossThreadDirect = append(crossThreadDirect, token)
		}
	}
	if len(crossThreadDirect) != 1 || crossThreadDirect[0] != "compute_supply" {
		t.Fatalf("R13 census identity broken: direct-on-chain ∩ cross-thread additivity must be exactly {compute_supply} (supply-fold recasts to target caliber at mint); got %v", crossThreadDirect)
	}
}

// TestR13CrossThreadAggregateNeverPromotedCarryingValue is the behavioral
// negative arm (4165 假#1 借道封口): a cross-thread aggregate row (typed
// registry Subject==aggregate_only ∧ Additivity==cross_thread_cpu_ms) that
// LOOKS maximally chain-coupled — resolved thread equal to a chain node,
// fully overlapping window, dominant magnitude — must still never come out of
// enrich on the chain channel, and its published value must ride through
// unchanged (the value never sneaks onto the board under a flipped channel).
func TestR13CrossThreadAggregateNeverPromotedCarryingValue(t *testing.T) {
	chain := ChainResult{
		Target: ThreadRef{PID: 100, Comm: "app"},
		Nodes: []ChainNode{{
			ID:     "n1",
			Thread: ThreadRef{PID: 200, Comm: "worker"},
			Window: TimeWindow{StartTs: 5.000, EndTs: 5.050},
		}},
	}
	for _, token := range []string{"cpu_pressure", "supply_pressure", "cpu_frequency_limit"} {
		spec, ok := CausalTokenSpecFor(token)
		if !ok || spec.Subject != CausalSubjectAggregateOnly || spec.Additivity != CausalAdditivityCrossThreadCPUms {
			t.Fatalf("fixture drifted: %s must be a cross-thread aggregate registry token", token)
		}
		item := RootCauseRankItem{
			Type:               token,
			Thread:             ThreadRef{PID: 200, Comm: "worker"},
			StartTs:            5.010,
			EndTs:              5.040,
			ImpactMs:           54.608,
			CumulativeImpactMs: 54.608,
			EffectiveImpactMs:  54.608,
			Summary:            fmt.Sprintf("%s aggregate", token),
		}
		out := enrichRootCauseItemsWithChainContext(chain, []RootCauseRankItem{item})
		if len(out) != 1 {
			t.Fatalf("%s: enrich must keep the row", token)
		}
		if out[0].ChainRelevance == "on_chain" {
			t.Fatalf("%s: a cross-thread aggregate must never be promoted onto the chain channel (54.608 借道形): %+v", token, out[0])
		}
		if out[0].EffectiveImpactMs != 54.608 {
			t.Fatalf("%s: the demoted row's published value must ride through unchanged (no silent recast off-channel): %+v", token, out[0])
		}
	}
}

// TestR13SelfBasisCarryIsTargetCaliberTyped is the ALLOWED-carry arm in the
// ruling's exact wording: the SELF-SEM row keeps its on-chain channel through
// enrich AND carries its mint value — legal precisely because the value's
// caliber is the target thread's in-window wall clock, witnessed by the typed
// OnChainBasis marker (never a subject/class recomposition). Production-
// minted fixture (donghu verbatim rows — §29.53 产线实铸形 red line).
func TestR13SelfBasisCarryIsTargetCaliberTyped(t *testing.T) {
	idx := buildTraceIndex(t, "r13_selfsem_donghu_jit.systrace", selfSemDonghuJitTrace)
	rank := BuildRootCauseRank(idx, selfSemDonghuQuery())
	var self *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "jit_compile" {
			self = &rank.Items[i]
			break
		}
	}
	if self == nil {
		t.Fatalf("fixture drifted: no jit_compile rank row was minted: %+v", rank.Items)
	}
	if self.ChainRelevance != "on_chain" || self.OnChainBasis != RootCauseOnChainBasisSelfDeterministicSpan {
		t.Fatalf("self row must hold the chain channel on the typed basis marker: %+v", self)
	}
	// The carried value IS the target-window wall-clock union (participation
	// == published effective == window-projection member union): the typed
	// witness that this carry is caliber-legal under R13.
	if self.EffectiveImpactMs <= 0 || self.EffectiveImpactMs != self.CumulativeImpactMs ||
		self.ImpactMs != self.EffectiveImpactMs {
		t.Fatalf("R13 self carry must be the target-caliber window union, one value on every channel: %+v", self)
	}
}
