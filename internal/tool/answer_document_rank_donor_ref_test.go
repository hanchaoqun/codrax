package tool

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRankFoldRelationRefComesOnlyFromActualIdentityDonor(t *testing.T) {
	rank, chain := rankFoldPrincipalIdentityPair()
	rank.FixDirection, rank.EffectiveImpactPublished = "lock_priority", true
	chain.FixDirection, chain.EffectiveImpactPublished = rank.FixDirection, true
	wantRef := types.TraceAnswerRelationMemberRef(rank)
	if wantRef == "" {
		t.Fatal("fixture must have a complete rank relation identity")
	}
	kept, peers, refs := runtimeTraceProjFoldSameSegmentLaneTwinsWithRefs([]types.TraceCausalProjectionNode{rank, chain})
	if len(kept) != 1 || len(peers) != 1 || len(refs) != 1 {
		t.Fatalf("same-segment fold must adopt exactly one donor: kept=%d peers=%d refs=%v", len(kept), len(peers), refs)
	}
	receipt := refs[runtimeTraceCausalProjectionNodeKey(kept[0])]
	if receipt.MemberRef != wantRef || receipt.PeerKey != runtimeTraceCausalProjectionNodeKey(rank) {
		t.Fatalf("adoption must carry exact donor identity, not a recomputed host identity: %+v", receipt)
	}
	row := runtimeTraceProjTreeRow{Node: kept[0], RankFoldPeers: []runtimeTraceProjRankFoldPeer{{
		RankIdentityAdopted: true, RelationMemberRef: receipt.MemberRef,
	}}}
	if got := runtimeTraceProjRowRelationMemberRef(row); got != wantRef {
		t.Fatalf("subtotal must consume adopted ref: got=%s want=%s", got, wantRef)
	}
	if got := types.TraceAnswerRelationMemberRef(row.Node); got == wantRef {
		t.Fatal("test must retain the real host/rank Object distinction")
	}
	// The same ordinal is not sufficient: an already-seated host retains its
	// own complete identity even if a display mirror is folded into it.
	chain.Rank, chain.RankBoardTarget, chain.RankBoardParamsFingerprint = rank.Rank, rank.RankBoardTarget, rank.RankBoardParamsFingerprint
	chain.RankQueryWindowStartTs, chain.RankQueryWindowEndTs = rank.QueryWindowStartTs, rank.QueryWindowEndTs
	kept, _, refs = runtimeTraceProjFoldSameSegmentLaneTwinsWithRefs([]types.TraceCausalProjectionNode{rank, chain})
	if len(kept) != 1 || len(refs) != 0 || !reflect.DeepEqual(kept[0], chain) {
		t.Fatalf("existing host rank identity must not acquire a donor alias: kept=%+v refs=%v", kept, refs)
	}
	row = runtimeTraceProjTreeRow{Node: kept[0]}
	if got := runtimeTraceProjRowRelationMemberRef(row); got != types.TraceAnswerRelationMemberRef(chain) || got == wantRef {
		t.Fatal("different original Object with same ordinal must remain a different relation identity")
	}
}

func TestRankFoldMissingDonorRefCannotBorrowHostIdentity(t *testing.T) {
	rank, chain := rankFoldPrincipalIdentityPair()
	rank.EffectiveImpactPublished, chain.EffectiveImpactPublished = true, true
	chain.FixDirection = "lock_priority"
	// A missing donor direction makes its stable member ref unavailable;
	// the host happens to know a direction, but must not fill that gap.
	kept, _, refs := runtimeTraceProjFoldSameSegmentLaneTwinsWithRefs([]types.TraceCausalProjectionNode{rank, chain})
	if len(kept) != 1 || len(refs) != 1 {
		t.Fatal("fixture must exercise an actual adoption with incomplete donor identity")
	}
	receipt := refs[runtimeTraceCausalProjectionNodeKey(kept[0])]
	if receipt.MemberRef != "" || types.TraceAnswerRelationMemberRef(kept[0]) == "" {
		t.Fatal("fixture must separate missing donor ref from complete host ref")
	}
	row := runtimeTraceProjTreeRow{Node: kept[0], RankFoldPeers: []runtimeTraceProjRankFoldPeer{{
		RankIdentityAdopted: true, RelationMemberRef: receipt.MemberRef,
	}}}
	if got := runtimeTraceProjRowRelationMemberRef(row); got != "" {
		t.Fatalf("adopted-but-unavailable must remain unavailable: %s", got)
	}
}

func TestRankFoldConflictingOrAmbiguousDonorsMintNoAliases(t *testing.T) {
	for name, build := range map[string]func(rank, chain types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode{
		"different_board": func(rank, chain types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode {
			chain.RankBoardParamsFingerprint = "other-board"
			return []types.TraceCausalProjectionNode{rank, chain}
		},
		"different_type_rank_twins": func(rank, chain types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode {
			other := rank
			other.EvidenceID, other.Object = "other-rank", "priority_inversion_runnable_wait"
			return []types.TraceCausalProjectionNode{rank, chain, other}
		},
		"adjacent_0_598": func(rank, chain types.TraceCausalProjectionNode) []types.TraceCausalProjectionNode {
			chain.ChainRelevance, chain.EffectiveImpactMS = "adjacent", .598
			return []types.TraceCausalProjectionNode{rank, chain}
		},
	} {
		t.Run(name, func(t *testing.T) {
			rank, chain := rankFoldPrincipalIdentityPair()
			input := build(rank, chain)
			kept, peers, refs := runtimeTraceProjFoldSameSegmentLaneTwinsWithRefs(input)
			if !reflect.DeepEqual(kept, input) || len(peers) != 0 || len(refs) != 0 {
				t.Fatalf("no successful adoption means no identity alias: kept=%+v peers=%v refs=%v", kept, peers, refs)
			}
		})
	}
}

func TestRankFoldDonorRefKeySurvivesProofTransferWithoutEvidenceID(t *testing.T) {
	rank, chain := rankFoldPrincipalIdentityPair()
	rank.EvidenceID, chain.EvidenceID = "", ""
	rank.FixDirection, rank.EffectiveImpactPublished = "lock_priority", true
	rank.DStateRefinedNonIO, rank.BlockedReasonCaller = true, "io_schedule"
	rank.BlockedReasonWindowCount, rank.BlockedReasonWindowCaller = 2, "submit_bio"
	kept, peers, refs := runtimeTraceProjFoldSameSegmentLaneTwinsWithRefs([]types.TraceCausalProjectionNode{rank, chain})
	if len(kept) != 1 || !kept[0].DStateRefinedNonIO || kept[0].BlockedReasonCaller != rank.BlockedReasonCaller ||
		kept[0].BlockedReasonWindowCaller != rank.BlockedReasonWindowCaller {
		t.Fatalf("fixture must transfer all existing proof annotations: %+v", kept)
	}
	key := runtimeTraceCausalProjectionNodeKey(kept[0])
	if len(peers[key]) != 1 || refs[key].PeerKey != runtimeTraceCausalProjectionNodeKey(rank) ||
		refs[key].MemberRef != types.TraceAnswerRelationMemberRef(rank) {
		t.Fatalf("final keeper key must address both lossless peer and donor ref: key=%q peers=%v refs=%v", key, peers, refs)
	}
}

func TestSemanticRankFoldRelationRefMatchesOriginalRankReceipt(t *testing.T) {
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: semLeadEngineObservations(t)})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one semantic source projection: %+v", set)
	}
	projection := set.Projections[0]
	refs := map[int]string{}
	semanticCompared := 0
	for _, node := range projection.RankedSeats {
		refs[node.Rank] = types.TraceAnswerRelationMemberRef(node)
	}
	for _, section := range TraceAnswerDecisionDirectionSections(projection) {
		if len(section.MemberRefs) != len(section.Members) {
			t.Fatalf("semantic section has incomplete refs: %+v", section)
		}
		for i, node := range section.Members {
			if want := refs[node.Rank]; want != "" {
				if node.SemanticClass != "" {
					semanticCompared++
				}
				if section.MemberRefs[i] != want {
					t.Errorf("semantic fold must retain rank receipt: rank=%d type=%s got=%s want=%s", node.Rank, node.Object, section.MemberRefs[i], want)
				}
			}
		}
	}
	if semanticCompared == 0 {
		t.Fatal("semantic fixture did not publish a complete original member identity")
	}
}
