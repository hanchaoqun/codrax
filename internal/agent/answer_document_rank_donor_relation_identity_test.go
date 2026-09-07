package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A direction has four ranked contributors but its bounded overview publishes
// only the leading two. One contributor also has a same-segment chain view:
// displaying that view must not invent a fifth relation member.
func rankDonorRelationProjection() types.TraceCausalProjection {
	inside := true
	seat := func(rank int, subject, direction, token string, value, start, end float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			EvidenceID: fmt.Sprintf("rank-%d", rank), Rank: rank, Subject: subject,
			Predicate: "root_cause_secondary", Object: token, TypeToken: token,
			FixDirection: direction, ImpactMS: value, EffectiveImpactMS: value, EffectiveImpactPublished: true,
			ChainRelevance: "on_chain", ChainCredentialCensus: "interval_proven", WithinRequestedWindow: &inside,
			PriorityInversionCandidate: direction == "lock_priority",
			StartTs:                    start, EndTs: end, LineStart: 100 * rank, LineEnd: 100*rank + 10,
			RankBoardTarget: "target-100", RankBoardParamsFingerprint: "board-a",
			QueryWindowStartTs: 10, QueryWindowEndTs: 10.1,
			RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.1,
		}
	}
	seats := []types.TraceCausalProjectionNode{
		seat(1, "target-100", "frequency_thermal", "running", 58.320, 10, 10.05),
		seat(2, "target-100", "io_dependency", "io_latency", 12.658, 10.050, 10.063),
		seat(3, "worker-200", "lock_priority", "priority_inversion_candidate", 7.405, 10.064, 10.073),
		seat(4, "worker-300", "lock_priority", "priority_inversion_candidate", 4.710, 10.074, 10.08),
		seat(5, "target-100", "scheduling_supply", "runnable_wait", 3.956, 10.081, 10.085),
		seat(6, "worker-400", "lock_priority", "priority_inversion_candidate", 3.429, 10.086, 10.09),
		seat(7, "worker-500", "lock_priority", "priority_inversion_candidate", 3.309, 10.091, 10.095),
	}
	chain := seats[2]
	chain.EvidenceID, chain.Predicate, chain.Object = "chain-twin", "wakeup_causal_impact", "running"
	chain.Rank, chain.RankBoardTarget, chain.RankBoardParamsFingerprint = 0, "", ""
	chain.RankQueryWindowStartTs, chain.RankQueryWindowEndTs = 0, 0
	chain.StateKind, chain.ImpactMS = "running", 8.294
	onChain := append(append([]types.TraceCausalProjectionNode(nil), seats...), chain)
	return types.TraceCausalProjection{
		ArtifactLabel: "customer.trace", WindowStartTs: 10, WindowEndTs: 10.1,
		WakeupPath: []string{"worker-200", "target-100"}, RankedSeats: seats, OnChainCauses: onChain,
	}
}

func TestRankDonorRelationIdentitySharedBySubtotalRosterAndBothPromptFaces(t *testing.T) {
	projection := rankDonorRelationProjection()
	refs := []string{
		types.TraceAnswerRelationMemberRef(projection.RankedSeats[2]),
		types.TraceAnswerRelationMemberRef(projection.RankedSeats[3]),
	}
	additional := []string{
		types.TraceAnswerRelationMemberRef(projection.RankedSeats[5]),
		types.TraceAnswerRelationMemberRef(projection.RankedSeats[6]),
	}
	for _, ref := range append(append([]string(nil), refs...), additional...) {
		if ref == "" {
			t.Fatal("fixture must have complete original rank identities")
		}
	}
	var found bool
	for _, section := range tool.TraceAnswerDecisionDirectionSections(projection) {
		if section.Direction != "lock_priority" {
			continue
		}
		found = true
		if len(section.Members) != 2 || section.SubtotalMS != 12.115 || strings.Join(section.MemberRefs, ",") != strings.Join(refs, ",") {
			t.Errorf("the displayed two-seat subtotal must retain its exact rank donor refs: %+v", section)
		}
	}
	if !found {
		t.Fatal("lock/priority overview was removed instead of preserving identity")
	}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}
	for name, text := range map[string]string{
		"handoff": renderAnswerDocTraceDecisionHandoffSet(set, runtimeTraceGuidanceView{}),
		"compact": renderTraceFinalCompactAuthorityLedger(set),
	} {
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{
				"member_refs=`" + strings.Join(refs, ",") + "`; physical_relation=`mutually_exclusive`",
				"member_count=4; headline_value_role=`exact_typed_subtotal`; headline_value=12.115ms",
				"headline_member_refs=`" + strings.Join(refs, ",") + "`; additional_unresolved_member_refs=`" + strings.Join(additional, ",") + "`",
				"additional_members_emitted=2; additional_members_total=2; additional_members_complete=`true`",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("folded 4-member direction must partition as 2+2, missing %q:\n%s", want, text)
				}
			}
		})
	}
}

func TestRankRelationIdentityKeepsDifferentBoardsTypesAndAdjacentRowsDistinct(t *testing.T) {
	base := rankDonorRelationProjection().RankedSeats[2]
	want := types.TraceAnswerRelationMemberRef(base)
	for name, mutate := range map[string]func(*types.TraceCausalProjectionNode){
		"board":  func(n *types.TraceCausalProjectionNode) { n.RankBoardParamsFingerprint = "another-board" },
		"target": func(n *types.TraceCausalProjectionNode) { n.RankBoardTarget = "another-target-100" },
		"type":   func(n *types.TraceCausalProjectionNode) { n.Object = "priority_inversion_runnable_wait" },
		"adjacent_0_598": func(n *types.TraceCausalProjectionNode) {
			n.ChainRelevance, n.EffectiveImpactMS = "adjacent", .598
		},
	} {
		t.Run(name, func(t *testing.T) {
			node := base
			mutate(&node)
			if got := types.TraceAnswerRelationMemberRef(node); got == "" || got == want {
				t.Fatalf("same ordinal is not sufficient for relation identity: got=%q original=%q", got, want)
			}
		})
	}
}
