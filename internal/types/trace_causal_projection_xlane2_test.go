package types

// trace_causal_projection_xlane2_test.go — XLANE-2 件3 pins (E11 rider,
// §29.109 记录① / §29.104.2 定谳⑤族, 2026-07-17): the R1 same-fact absorb is
// the single mint point of the E11 three-face contradiction — an explicitly
// on-chain seated survivor (❶ badge + ├─链上─ lane + 根因排序#N) that
// inherited a ◇ demoted view's whole-seat demotion marker wore the
// 「无链上凭证(整席降道)」 sentence on the same row. 诚实面胜出: the chain
// admission is a positive typed proof; the demotion marker is a per-view
// absence-of-proof and must never cross onto the explicit chain face. The
// ordinal arm is channel-scoped the same way (a chain survivor must not wear
// a stolen 邻近影响 ordinal as 根因排序#N).

import (
	"strconv"
	"testing"
)

// xlane2ChainSeatView is the production shape of the on-chain seated view of
// the E11 fact (engine rank row: explicit on_chain relevance + chain-board
// seat; the runnable2/fused-donghu window seat family).
func xlane2ChainSeatView() TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role: TraceCausalRolePrimaryRootCause, EvidenceID: "xlane2-chain-view",
		Subject: "shadowhook-task-64305", Object: "runnable_wait", TypeToken: "runnable_wait",
		StateKind: "runnable", ChainRelevance: "on_chain",
		ImpactMS: 23.471, CumulativeImpactMS: 23.471, EffectiveImpactMS: 23.471,
		Rank: 1, Tier: "primary", Confidence: 0.8,
		RankBoardTarget: "ease.cloudmusic-63993",
		LineStart:       30, LineEnd: 40,
	}
}

// xlane2DemotedView is the production shape of the R4 credential-demoted view
// of the SAME physical fact (engine demotion arm pairs the marker with
// ChainRelevance="adjacent" + Causality="adjacent_to_wakeup_chain"; values
// untouched — same subject, same 3dp value, same line range, so the R1
// same-fact key collides with the chain view).
func xlane2DemotedView() TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Role: TraceCausalRoleRootCauseContext, EvidenceID: "xlane2-demoted-view",
		Subject: "shadowhook-task-64305", Object: "priority_inversion_runnable_wait",
		StateKind: "runnable", ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
		ImpactMS: 23.471, CumulativeImpactMS: 23.471, EffectiveImpactMS: 23.471,
		Rank: 9, Confidence: 0.7,
		ChainCredentialLaneDemoted: true,
		LineStart:                  30, LineEnd: 40,
	}
}

// 件3 core (病形红→修后绿): the merged E11 fact keeps its chain face free of
// BOTH whole-seat demotion markers — the sentence face can no longer
// contradict the ❶/lane/seat faces — while the absorbed view's evidence id
// still folds in losslessly.
func TestXLANE2MergeSameFactsKeepsChainFaceFreeOfDemotionMarkers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*TraceCausalProjectionNode)
		check  func(TraceCausalProjectionNode) bool
	}{
		{"credential-demoted marker", func(n *TraceCausalProjectionNode) {},
			func(n TraceCausalProjectionNode) bool { return n.ChainCredentialLaneDemoted }},
		{"represented marker", func(n *TraceCausalProjectionNode) {
			n.ChainCredentialLaneDemoted = false
			n.ChainAnchorRepresentedByChainSeat = true
		}, func(n TraceCausalProjectionNode) bool { return n.ChainAnchorRepresentedByChainSeat }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loser := xlane2DemotedView()
			tc.mutate(&loser)
			out := TraceCausalProjection{
				OnChainCauses:  []TraceCausalProjectionNode{xlane2ChainSeatView()},
				AdjacentCauses: []TraceCausalProjectionNode{loser},
			}
			traceCausalProjectionMergeSameFacts(&out)
			if len(out.AdjacentCauses) != 0 {
				t.Fatalf("fixture drifted: the demoted view must same-fact fold into the chain view (key = subject+value+lines): %+v", out.AdjacentCauses)
			}
			if len(out.OnChainCauses) != 1 {
				t.Fatalf("exactly one merged chain view expected: %+v", out.OnChainCauses)
			}
			survivor := out.OnChainCauses[0]
			if tc.check(survivor) {
				t.Fatalf("件3: the explicit chain face must never inherit the whole-seat demotion marker (三面矛盾): %+v", survivor)
			}
			if survivor.Rank != 1 {
				t.Fatalf("件3: the chain seat keeps its own ordinal untouched: %+v", survivor)
			}
			// The QUIET account-identity memory replaces the marker: the ×N
			// same-kind fold key keeps forking (overlapping absorbed accounts
			// must never re-Σ — the fused donghu 低频运行 32.877 false-sum
			// witness), while no word face fires on it.
			if !survivor.AbsorbedWholeSeatDemotedView {
				t.Fatalf("件3: the chain survivor must keep the absorbed-demoted account memory")
			}
			if traceCausalProjectionAnchorFormKey(survivor) != "absorbed_demoted_view" {
				t.Fatalf("件3: the fold key must fork on the absorbed-demoted memory: %q",
					traceCausalProjectionAnchorFormKey(survivor))
			}
			found := false
			for _, id := range survivor.MergedEvidenceIDs {
				if id == loser.EvidenceID {
					found = true
				}
			}
			if !found {
				t.Fatalf("件3: the absorbed view's evidence id must fold in losslessly: %+v", survivor.MergedEvidenceIDs)
			}
		})
	}
}

// 件1 wire parser: strict all-or-nothing against the member count — a
// truncated / malformed / count-mismatched set yields nil (a partial set
// could fake a member-subset verdict), a complete one parses verbatim.
func TestXLANE2ParseMemberLineRangesStrict(t *testing.T) {
	if got := traceCausalProjectionParseMemberLineRanges("100..110|120..130", 2); len(got) != 2 ||
		got[0] != [2]int{100, 110} || got[1] != [2]int{120, 130} {
		t.Fatalf("complete set must parse verbatim: %+v", got)
	}
	for name, tc := range map[string]struct {
		raw   string
		count int
	}{
		"count mismatch":  {"100..110|120..130", 3},
		"malformed entry": {"100..110|x..130", 2},
		"zero start":      {"0..110|120..130", 2},
		"inverted range":  {"110..100|120..130", 2},
		"single member":   {"100..110", 1},
	} {
		if got := traceCausalProjectionParseMemberLineRanges(tc.raw, tc.count); got != nil {
			t.Fatalf("%s: strict parse must drop the WHOLE set: %+v", name, got)
		}
	}
}

// 件2 wire parser: entries parse independently (each clause its own truth) —
// invalid entries drop without poisoning the valid ones; the cap bounds the
// roster.
func TestXLANE2ParseSelfGapSemanticOverlaps(t *testing.T) {
	got := traceCausalProjectionParseSelfGapSemanticOverlaps("15.000@600..620|bogus|0.000@1..2|5.000@700..705")
	if len(got) != 2 || got[0].OverlapMS != 15.0 || got[0].LineStart != 600 || got[0].LineEnd != 620 ||
		got[1].OverlapMS != 5.0 || got[1].LineStart != 700 {
		t.Fatalf("per-entry parse drifted: %+v", got)
	}
	raw := ""
	for i := 0; i < 8; i++ {
		if i > 0 {
			raw += "|"
		}
		raw += "1.000@" + strconv.Itoa(100+i) + ".." + strconv.Itoa(105+i)
	}
	if got := traceCausalProjectionParseSelfGapSemanticOverlaps(raw); len(got) != TraceCausalProjectionSelfGapSemanticOverlapCap {
		t.Fatalf("cap: 8 entries must keep %d, got %d", TraceCausalProjectionSelfGapSemanticOverlapCap, len(got))
	}
	if got := traceCausalProjectionParseSelfGapSemanticOverlaps(""); got != nil {
		t.Fatalf("empty note must parse to nil")
	}
}

// 件3 negative arm (无关形保形): on every non-explicit-chain survivor the
// markers keep the XLANE-1 OR-monotone inheritance — the demotion story is
// never dropped wordlessly (◇ survivor here; the bare-"" survivor is pinned
// by TestXLANEAbsorbSameFactCarriesRepresentedMarker).
func TestXLANE2AbsorbKeepsDemotionMarkerOnNonChainFaces(t *testing.T) {
	survivor := TraceCausalProjectionNode{
		Subject: "shadowhook-task-64305", Object: "runnable", ChainRelevance: "adjacent",
	}
	traceCausalProjectionAbsorbSameFact(&survivor, xlane2DemotedView(), map[string]bool{}, nil)
	if !survivor.ChainCredentialLaneDemoted {
		t.Fatalf("负向臂: a ◇ survivor must keep inheriting the demotion marker (story never dropped)")
	}
}

// 件3 ordinal arm: a chain-face survivor never adopts a WHOLE-SEAT DEMOTED
// ◇ view's ordinal (the demotion retired that seat's competing; adopting it
// as 根因排序#N is the E11 three-face family's ordinal arm) — while the
// legacy adoptions all keep working: RANK-U same-channel seat adoption,
// bare-"" pairs, and the UNDEMOTED adjacent seat view (the XLANE-3 fused
// cross-step form — refusing there would silently drop a board seat).
func TestXLANE2AbsorbRankAdoptionRefusesDemotedOrdinalOnChainFace(t *testing.T) {
	chainSurvivor := xlane2ChainSeatView()
	chainSurvivor.Rank = 0
	chainSurvivor.Tier = ""
	chainSurvivor.RankBoardTarget = ""
	traceCausalProjectionAbsorbSameFact(&chainSurvivor, xlane2DemotedView(), map[string]bool{}, nil)
	if chainSurvivor.Rank != 0 || chainSurvivor.RankBoardTarget != "" {
		t.Fatalf("件3: a chain-face survivor must not adopt a demoted view's ordinal/board: rank=%d board=%q",
			chainSurvivor.Rank, chainSurvivor.RankBoardTarget)
	}
	// Positive control 1 (RANK-U): same chain channel → seat adopts as one pair.
	spanView := xlane2ChainSeatView()
	spanView.Rank = 0
	spanView.Tier = ""
	spanView.RankBoardTarget = ""
	rankView := xlane2ChainSeatView()
	rankView.EvidenceID = "xlane2-rank-view"
	rankView.Rank = 2
	rankView.Tier = "secondary"
	traceCausalProjectionAbsorbSameFact(&spanView, rankView, map[string]bool{}, nil)
	if spanView.Rank != 2 || spanView.Tier != "secondary" || spanView.RankBoardTarget != "ease.cloudmusic-63993" {
		t.Fatalf("RANK-U regression: same-channel seat adoption must keep working: %+v", spanView)
	}
	// Positive control 2 (legacy): bare-relevance pairs keep adopting.
	bare := TraceCausalProjectionNode{Subject: "t-1", Object: "runnable"}
	bareLoser := TraceCausalProjectionNode{Subject: "t-1", Object: "runnable", Rank: 3, Tier: "tertiary"}
	traceCausalProjectionAbsorbSameFact(&bare, bareLoser, map[string]bool{}, nil)
	if bare.Rank != 3 {
		t.Fatalf("legacy regression: bare-relevance pairs must keep adopting: %+v", bare)
	}
	// Positive control 3 (XLANE-3 fused form): an UNDEMOTED adjacent seat view
	// still hands its seat to the chain-face survivor (seat follows the fact;
	// no silent seat loss).
	fusedSurvivor := xlane2ChainSeatView()
	fusedSurvivor.Rank = 0
	fusedSurvivor.Tier = ""
	fusedSurvivor.RankBoardTarget = ""
	adjacentSeat := xlane2DemotedView()
	adjacentSeat.ChainCredentialLaneDemoted = false
	adjacentSeat.RankBoardTarget = "shadowhook-task-64305"
	traceCausalProjectionAbsorbSameFact(&fusedSurvivor, adjacentSeat, map[string]bool{}, nil)
	if fusedSurvivor.Rank != 9 || fusedSurvivor.RankBoardTarget != "shadowhook-task-64305" {
		t.Fatalf("XLANE-3 regression: an undemoted adjacent seat must keep adopting: %+v", fusedSurvivor.Rank)
	}
}
