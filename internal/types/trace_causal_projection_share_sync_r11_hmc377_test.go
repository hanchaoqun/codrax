package types

// HMC377 round 11 (register R10T-2): the projection sync spots of the typed
// share decisions the one resolver writes — rank item -> note -> projection
// node -> renderer. Review 10 found the round-10 spots untested: the mutants
// TM9A (same-fact absorb drops GatedShareClaimWithdrawn), TM9B (same-fact
// absorb drops ChainAnchorRepresentedHeldMS), TM9C/TM9D (the ×N form key
// forgets the partly represented / withdrawn-constituent account form) and
// TM9E (the ×N Σ row keeps one member's held share) survived every suite
// (~/hmc-work/hmc377/fix10/review-pins/mutant/full_types_TM9*.log). Round 11
// adds ShareReturnedMS on the same spots. Each pin below fails when its spot
// is removed.

import (
	"strings"
	"testing"
)

func hmc377R11SyncNode(id string, value float64, line int) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{Role: TraceCausalRoleRootCauseContext, EvidenceID: id, Subject: "binder:458_5-6326",
		Predicate: "root_cause_tertiary", Object: "runnable_wait", TypeToken: "runnable_wait", StateKind: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Rank: 4, Tier: "tertiary",
		ImpactMS: value, CumulativeImpactMS: value, EffectiveImpactMS: value, RunnableMS: value, LineStart: line, LineEnd: line + 5}
}

func hmc377R11SyncClaims() []TraceCausalProjectionShareClaim {
	return []TraceCausalProjectionShareClaim{{Part: "anchored", Role: "chain_seat", ShareMS: 1.903, Represented: true, ResolvedMS: 0.853, HolderEvidenceIDs: []string{"h"}}}
}

func TestHMC377R11SameFactAbsorbCarriesTheShareDecisions(t *testing.T) {
	t.Run("GatedShareClaimWithdrawn rides the split disclosure (TM9A)", func(t *testing.T) {
		survivor := hmc377R11SyncNode("s", 1.903, 50)
		loser := hmc377R11SyncNode("l", 1.903, 50)
		loser.GatedShareClaimedMS, loser.GatedShareFullMS, loser.GatedShareConstituentSeat, loser.GatedShareClaimWithdrawn = 1.903, 3.0, true, true
		traceCausalProjectionAbsorbSameFact(&survivor, loser, map[string]bool{}, nil)
		if !survivor.GatedShareConstituentSeat || !survivor.GatedShareClaimWithdrawn {
			t.Errorf("the split's withdrawal decision must travel with its disclosure: constituent=%t withdrawn=%t",
				survivor.GatedShareConstituentSeat, survivor.GatedShareClaimWithdrawn)
		}
	})
	t.Run("the partly represented share rides its claims (TM9B)", func(t *testing.T) {
		survivor := hmc377R11SyncNode("s", 1.05, 30)
		loser := hmc377R11SyncNode("l", 1.05, 30)
		loser.ChainAnchorRepresentedHeldMS, loser.ShareClaims = 0.853, hmc377R11SyncClaims()
		traceCausalProjectionAbsorbSameFact(&survivor, loser, map[string]bool{}, nil)
		if survivor.ChainAnchorRepresentedHeldMS != 0.853 || len(survivor.ShareClaims) != 1 {
			t.Errorf("held=%.3f claims=%d", survivor.ChainAnchorRepresentedHeldMS, len(survivor.ShareClaims))
		}
	})
	t.Run("the returned share rides its claims (round 11)", func(t *testing.T) {
		survivor := hmc377R11SyncNode("s", 6.05, 60)
		loser := hmc377R11SyncNode("l", 6.05, 60)
		loser.ShareReturnedMS, loser.ShareClaims = 1.05, hmc377R11SyncClaims()
		traceCausalProjectionAbsorbSameFact(&survivor, loser, map[string]bool{}, nil)
		if survivor.ShareReturnedMS != 1.05 || len(survivor.ShareClaims) != 1 {
			t.Errorf("returned=%.3f claims=%d", survivor.ShareReturnedMS, len(survivor.ShareClaims))
		}
	})
}

func TestHMC377R11AnchorFormKeysKeepCarvedAccountsApart(t *testing.T) {
	plain := hmc377R11SyncNode("p", 1.0, 10)
	cases := []struct {
		name string
		set  func(*TraceCausalProjectionNode)
		want string
	}{
		{"partly represented satellite (TM9C)", func(n *TraceCausalProjectionNode) { n.ChainAnchorRepresentedHeldMS = 0.853 }, "anchor_represented_partial"},
		{"constituent whose claim seat was withdrawn (TM9D)", func(n *TraceCausalProjectionNode) {
			n.GatedShareConstituentSeat, n.GatedShareClaimWithdrawn = true, true
		}, "gated_share_constituent_withdrawn"},
		{"◇ remainder that took a share back", func(n *TraceCausalProjectionNode) {
			n.ChainAnchorRemainderSeat, n.ChainAnchorFullMS, n.ShareReturnedMS = true, 6.903, 1.05
		}, "anchor_remainder_returned"},
		{"residual that took its carved share back", func(n *TraceCausalProjectionNode) {
			n.GatedShareFullMS, n.GatedShareClaimedMS, n.ShareReturnedMS = 3.0, 1.903, 1.903
		}, "gated_share_residual_returned"},
		{"satellite that carries a withdrawn holder's share", func(n *TraceCausalProjectionNode) { n.ShareReturnedMS = 1.903 }, "share_returned"},
	}
	if got := traceCausalProjectionAnchorFormKey(plain); got != "" {
		t.Fatalf("precondition: a plain row has no form key, got %q", got)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := hmc377R11SyncNode("n", 1.0, 20)
			tc.set(&node)
			if got := traceCausalProjectionAnchorFormKey(node); got != tc.want {
				t.Errorf("form key %q, want %q", got, tc.want)
			}
			// The fork is what keeps the ×N merge from re-Σ the carved row
			// with plain rows of the same (subject, object).
			nodes := []TraceCausalProjectionNode{node, hmc377R11SyncNode("p1", 1.0, 30), hmc377R11SyncNode("p2", 1.0, 40)}
			out := traceCausalProjectionAggregateSameKind(nodes)
			kept := false
			for _, n := range out {
				kept = kept || n.EvidenceID == "n" && n.MergedCount <= 1
			}
			if !kept {
				t.Errorf("the carved row was re-Σ with plain rows: %+v", out)
			}
		})
	}
}

func TestHMC377R11MemberSumRowClearsPerMemberShares(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*TraceCausalProjectionNode)
		read func(TraceCausalProjectionNode) float64
	}{
		{"held share (TM9E)", func(n *TraceCausalProjectionNode) {
			n.ChainAnchorRepresentedHeldMS, n.ShareClaims = 0.5, hmc377R11SyncClaims()
		},
			func(n TraceCausalProjectionNode) float64 { return n.ChainAnchorRepresentedHeldMS }},
		{"returned share (round 11)", func(n *TraceCausalProjectionNode) { n.ShareReturnedMS, n.ShareClaims = 0.5, hmc377R11SyncClaims() },
			func(n TraceCausalProjectionNode) float64 { return n.ShareReturnedMS }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var nodes []TraceCausalProjectionNode
			for i, line := range []int{10, 20, 30} {
				n := hmc377R11SyncNode(strings.Repeat("m", i+1), 1.0, line)
				tc.set(&n)
				nodes = append(nodes, n)
			}
			aggregate := traceCausalProjectionMergeSameKindMemberValues(nodes, 0, []int{0, 1, 2}, false)
			if got := tc.read(aggregate); got != 0 || aggregate.ShareClaims != nil {
				t.Errorf("the ×N Σ row wears one member's share %.3f / claims %+v", got, aggregate.ShareClaims)
			}
		})
	}
}
