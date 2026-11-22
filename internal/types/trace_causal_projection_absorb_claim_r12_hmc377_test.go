package types

// HMC377 round 12 (register R11T-5): the R1 same-fact absorb carries the
// LEVELMERGE-1 split's typed gated-share claim with the split family it
// carries. The split's [E#] pointer, withdrawal wording and successor reading
// come from that claim (ShareClaims, part gated_share); the round-11 absorb
// copied only the claim-seat line intervals, so an absorbed split survivor
// lost its [E#] (review-pins/probe/{absorb_probe_head.log,
// lm_pointer_probe_head.log}).

import "testing"

func TestHMC377R12SameFactAbsorbCarriesTheGatedShareClaim(t *testing.T) {
	claim := TraceCausalProjectionShareClaim{Part: "gated_share", Role: "gated_share_seat", ShareMS: 10,
		Resolved: true, Exact: true, ResolvedMS: 10, HolderEvidenceIDs: []string{"lm-inv"}}
	node := func(id string) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{Role: TraceCausalRoleRootCauseContext, EvidenceID: id, Subject: "dep_worker-200",
			Object: "runnable_wait", TypeToken: "runnable_wait", ImpactMS: 5, EffectiveImpactMS: 5, LineStart: 100, LineEnd: 110}
	}
	t.Run("split family", func(t *testing.T) {
		survivor, loser := node("s"), node("l")
		loser.GatedShareClaimedMS, loser.GatedShareFullMS, loser.GatedShareClaimSeats = 10, 15, []string{"300..305"}
		loser.ShareClaims = []TraceCausalProjectionShareClaim{claim}
		traceCausalProjectionAbsorbSameFact(&survivor, loser, map[string]bool{}, nil)
		if survivor.GatedShareFullMS != 15 || len(survivor.ShareClaims) != 1 || survivor.ShareClaims[0].Part != "gated_share" ||
			len(survivor.ShareClaims[0].HolderEvidenceIDs) != 1 {
			t.Errorf("the survivor carries the split without its claim: full=%.3f claims=%+v", survivor.GatedShareFullMS, survivor.ShareClaims)
		}
	})
	t.Run("overlap disclosure", func(t *testing.T) {
		survivor, loser := node("s"), node("l")
		loser.GatedShareOverlapDisclosureMS, loser.GatedShareClaimSeats = 10, []string{"300..305"}
		loser.ShareClaims = []TraceCausalProjectionShareClaim{claim}
		traceCausalProjectionAbsorbSameFact(&survivor, loser, map[string]bool{}, nil)
		if survivor.GatedShareOverlapDisclosureMS != 10 || len(survivor.ShareClaims) != 1 {
			t.Errorf("the survivor carries the disclosure without its claim: claims=%+v", survivor.ShareClaims)
		}
	})
	t.Run("a survivor with its own split keeps its own claim", func(t *testing.T) {
		survivor, loser := node("s"), node("l")
		own := claim
		own.HolderEvidenceIDs = []string{"own-inv"}
		survivor.ShareClaims = []TraceCausalProjectionShareClaim{own}
		loser.GatedShareClaimedMS, loser.GatedShareFullMS = 10, 15
		loser.ShareClaims = []TraceCausalProjectionShareClaim{claim}
		traceCausalProjectionAbsorbSameFact(&survivor, loser, map[string]bool{}, nil)
		if len(survivor.ShareClaims) != 1 || survivor.ShareClaims[0].HolderEvidenceIDs[0] != "own-inv" {
			t.Errorf("claims=%+v", survivor.ShareClaims)
		}
	})
}
