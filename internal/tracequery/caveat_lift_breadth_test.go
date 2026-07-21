package tracequery

// caveat_lift_breadth_test.go — C8PROSE-1 批 Item 2 pins (§29.163 P3-2 备案销,
// 2026-07-20): the engine caveat lifts ride ONE face-census walker
// (scanResultSupplyFoldBases) covering every PUBLISHED result face that can
// carry a SupplyFoldBasis, and the C2 anchor roster is the sorted union
// across bases (first-hit == union on every reachable shape — one memoized
// coreCapability per Result — the union is the structural enforcement).
// Disclosure lane only: no gate, score, sort or membership arm reads any of
// these caveats, so the verdict faces stay byte-identical by construction.

import (
	"strings"
	"testing"
)

func c8prose1AnchorBasis(cpus ...int) *SupplyFoldBasis {
	return &SupplyFoldBasis{ClusterLimitsAnchorMismatch: cpus}
}

// The face census is complete: all eight published rosters (chain causal
// impacts / chain aggregates / rank items / rank absorbed items, top-level
// and bundle twins) reach the lift — each face contributes a unique CPU, so
// dropping ANY face from the walker goes red here.
func TestC8Prose1CaveatLiftFaceCensusComplete(t *testing.T) {
	res := Result{
		WakeupChain: &ChainResult{
			CausalImpacts:     []WakeupCausalImpact{{SupplyFoldBasis: c8prose1AnchorBasis(0)}},
			AggregatedImpacts: []WakeupCausalAggregate{{SupplyFoldBasis: c8prose1AnchorBasis(1)}},
			// 复核收编 P3-1 (2026-07-20): the ninth serialized face —
			// nodes[].impact.supply_fold_basis; a node-ONLY basis (cpu8)
			// must reach the census (dropping the walker's node arm reds
			// on the missing cpu8).
			Nodes: []ChainNode{{Impact: &WakeupCausalImpact{SupplyFoldBasis: c8prose1AnchorBasis(8)}}},
		},
		RootCauseRank: &RootCauseRankResult{
			Items:         []RootCauseRankItem{{SupplyFoldBasis: c8prose1AnchorBasis(2)}},
			AbsorbedItems: []RootCauseRankItem{{SupplyFoldBasis: c8prose1AnchorBasis(3)}},
		},
		FrameRootCauseBundle: &FrameRootCauseBundle{
			WakeupChain: &ChainResult{
				CausalImpacts:     []WakeupCausalImpact{{SupplyFoldBasis: c8prose1AnchorBasis(4)}},
				AggregatedImpacts: []WakeupCausalAggregate{{SupplyFoldBasis: c8prose1AnchorBasis(5)}},
			},
			RootCauseRank: &RootCauseRankResult{
				Items:         []RootCauseRankItem{{SupplyFoldBasis: c8prose1AnchorBasis(6)}},
				AbsorbedItems: []RootCauseRankItem{{SupplyFoldBasis: c8prose1AnchorBasis(7)}},
			},
		},
	}
	caveats := clusterFixTwoDisclosureCaveats(res)
	if len(caveats) != 1 || !strings.Contains(caveats[0], "cluster_limits_anchor_mismatch=cpu0,cpu1,cpu2,cpu3,cpu4,cpu5,cpu6,cpu7,cpu8 —") {
		t.Fatalf("every published basis face must reach the lift (union of all nine), got %v", caveats)
	}
	// The burst lift shares the walker: a burst reason ONLY on the deepest
	// newly-covered face (bundle absorbed items) must be disclosed.
	burstOnly := Result{FrameRootCauseBundle: &FrameRootCauseBundle{
		RootCauseRank: &RootCauseRankResult{AbsorbedItems: []RootCauseRankItem{{
			SupplyFoldBasis: &SupplyFoldBasis{
				CapabilitySource:         CoreCapabilitySourceFreqOnly,
				CapabilityFreqOnlyReason: CoreCapabilityFreqOnlyReasonComoveFloorSingleBurst,
			},
		}}},
	}}
	if got := clusterFixTwoDisclosureCaveats(burstOnly); len(got) != 1 ||
		!strings.Contains(got[0], "comove_floor_single_burst") {
		t.Fatalf("the burst disclosure must reach absorbed-item bases too, got %v", got)
	}
	// The split-audit lift shares the walker as well (breadth arm).
	auditOnly := Result{RootCauseRank: &RootCauseRankResult{AbsorbedItems: []RootCauseRankItem{{
		SupplyFoldBasis: &SupplyFoldBasis{CapabilitySplitAudit: "cpu2↔cpu3 @9.000 判定臂=co_witness_floor"},
	}}}}
	if got := capabilitySplitAuditCaveat(auditOnly); !strings.Contains(got, "cpu2↔cpu3 @9.000") {
		t.Fatalf("the split-audit disclosure must reach absorbed-item bases, got %q", got)
	}
}

// Roster union semantics: same-roster bases keep the pre-batch bytes
// (first-hit ≡ union — the A/B identity arm), divergent rosters disclose the
// sorted union instead of silently dropping the later basis.
func TestC8Prose1CaveatLiftRosterUnionFirstHitEquivalence(t *testing.T) {
	single := Result{RootCauseRank: &RootCauseRankResult{Items: []RootCauseRankItem{
		{SupplyFoldBasis: c8prose1AnchorBasis(1)},
	}}}
	twinned := Result{RootCauseRank: &RootCauseRankResult{Items: []RootCauseRankItem{
		{SupplyFoldBasis: c8prose1AnchorBasis(1)},
		{SupplyFoldBasis: c8prose1AnchorBasis(1)},
	}}}
	a, b := clusterFixTwoDisclosureCaveats(single), clusterFixTwoDisclosureCaveats(twinned)
	if len(a) != 1 || len(b) != 1 || a[0] != b[0] {
		t.Fatalf("same-roster bases must stay byte-identical to the single-basis form:\n a=%v\n b=%v", a, b)
	}
	if !strings.Contains(a[0], "cluster_limits_anchor_mismatch=cpu1 —") {
		t.Fatalf("the single-anchor roster must not duplicate members, got %q", a[0])
	}
	// Divergent rosters: the later basis is no longer dropped; the roster is
	// the sorted union (the {5}-first order proves sorting, not first-hit).
	divergent := Result{RootCauseRank: &RootCauseRankResult{Items: []RootCauseRankItem{
		{SupplyFoldBasis: c8prose1AnchorBasis(5)},
		{SupplyFoldBasis: c8prose1AnchorBasis(1)},
	}}}
	got := clusterFixTwoDisclosureCaveats(divergent)
	if len(got) != 1 || !strings.Contains(got[0], "cluster_limits_anchor_mismatch=cpu1,cpu5 —") {
		t.Fatalf("divergent rosters must disclose the sorted union, got %v", got)
	}
	// Anchor-free results stay caveat-silent (absence preserves every byte).
	if got := clusterFixTwoDisclosureCaveats(Result{RootCauseRank: &RootCauseRankResult{
		Items: []RootCauseRankItem{{SupplyFoldBasis: &SupplyFoldBasis{}}},
	}}); len(got) != 0 {
		t.Fatalf("no anchors → no caveat, got %v", got)
	}
}

// The split-audit lift keeps FIRST-hit selection by contract (the field is
// the first co-movement split localization sample; a union of audit strings
// would misdescribe it) — the walk order pins Items before AbsorbedItems.
func TestC8Prose1SplitAuditFirstHitOrderPinned(t *testing.T) {
	res := Result{RootCauseRank: &RootCauseRankResult{
		Items: []RootCauseRankItem{{
			SupplyFoldBasis: &SupplyFoldBasis{CapabilitySplitAudit: "A1"},
		}},
		AbsorbedItems: []RootCauseRankItem{{
			SupplyFoldBasis: &SupplyFoldBasis{CapabilitySplitAudit: "A2"},
		}},
	}}
	got := capabilitySplitAuditCaveat(res)
	if !strings.Contains(got, "capability_freq_only_split_audit=A1 —") {
		t.Fatalf("the published Items audit must win the first-hit slot, got %q", got)
	}
	if strings.Contains(got, "A2") {
		t.Fatalf("the audit lift stays single-sample (first hit), got %q", got)
	}
}
