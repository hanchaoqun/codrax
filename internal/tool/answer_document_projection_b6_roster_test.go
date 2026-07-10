package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b6RosterProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"ranked-10", "target-1"},
		WindowStartTs: 5.0,
		WindowEndTs:   5.1,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rank-seat-e1",
				Subject: "ranked-10", Predicate: "root_cause_secondary", Object: "running",
				TypeToken: "running", StateKind: "running", ChainRelevance: "on_chain",
				ChainDepth: 1, Rank: 2, ImpactMS: 20, CumulativeImpactMS: 20,
				EffectiveImpactMS: 20, Confidence: 0.8,
			},
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "fold-e2",
				Predicate: "root_cause_context", ChainRelevance: "on_chain",
				OnChainOverflowFold: true, MergedCount: 3,
				MergedSubjects: []string{"ranked-10", "hidden-11"},
				MergedMinMS:    1, MergedMaxMS: 3, ImpactMS: 3, CumulativeImpactMS: 3,
				Confidence: 0.6,
			},
		},
	}
}

func TestB6FoldRosterPointsToPublishedRankSeat(t *testing.T) {
	projection := b6RosterProjection()
	zhModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var zhFold *runtimeTraceProjTreeRow
	for i := range zhModel.TreeRows {
		if zhModel.TreeRows[i].Node.OnChainOverflowFold {
			zhFold = &zhModel.TreeRows[i]
			break
		}
	}
	if zhFold == nil || len(zhFold.Node.MergedSubjects) != 2 ||
		zhFold.Node.MergedSubjects[0] != "ranked-10(见榜位#2)" || zhFold.Node.MergedSubjects[1] != "hidden-11" {
		t.Fatalf("model roster must retain both exact members and add one pointer: %+v", zhFold)
	}
	zhFence := runtimeTraceProjTreeFence(zhModel, true)
	if !strings.Contains(zhFence, "见榜位#2") {
		t.Fatalf("fold roster must point at the already-published rank seat:\n%s", zhFence)
	}
	zhDetail := runtimeTraceProjDetailFullText(zhModel, true)
	if !strings.Contains(zhDetail, "ranked-10(见榜位#2)") || !strings.Contains(zhDetail, "hidden-11") {
		t.Fatalf("lossless detail roster must retain both members plus pointer:\n%s", zhDetail)
	}

	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	var enFold *runtimeTraceProjTreeRow
	for i := range enModel.TreeRows {
		if enModel.TreeRows[i].Node.OnChainOverflowFold {
			enFold = &enModel.TreeRows[i]
			break
		}
	}
	if enFold == nil || enFold.Node.MergedSubjects[0] != "ranked-10 (see rank #2)" ||
		enFold.Node.MergedSubjects[1] != "hidden-11" {
		t.Fatalf("EN model roster pointer parity failed: %+v", enFold)
	}
	enFence := runtimeTraceProjTreeFence(enModel, false)
	if !strings.Contains(enFence, "see") {
		t.Fatalf("EN fold roster pointer parity failed:\n%s", enFence)
	}

	// Display-only guarantee: the source projection roster is never rewritten.
	if got := strings.Join(projection.OnChainCauses[1].MergedSubjects, ","); got != "ranked-10,hidden-11" {
		t.Fatalf("B6 must not mutate typed projection provenance, got %q", got)
	}
}

func TestB6FoldRosterAmbiguousRankDoesNotGuess(t *testing.T) {
	for _, ranks := range [][2]int{{2, 3}, {2, 2}} {
		model := runtimeTraceProjTreeModel{
			TreeRows: []runtimeTraceProjTreeRow{
				{HasData: true, Node: types.TraceCausalProjectionNode{Subject: "same-10", Rank: ranks[0], QueryWindowStartTs: 1, QueryWindowEndTs: 2}},
				{HasData: true, Node: types.TraceCausalProjectionNode{Subject: "same-10", Rank: ranks[1], QueryWindowStartTs: 3, QueryWindowEndTs: 4}},
				{HasData: true, Node: types.TraceCausalProjectionNode{
					OnChainOverflowFold: true, MergedCount: 1, MergedSubjects: []string{"same-10"},
				}},
			},
		}
		runtimeTraceProjAnnotateFoldRosterRankPointers(&model, true)
		if got := model.TreeRows[2].Node.MergedSubjects[0]; got != "same-10" {
			t.Fatalf("two visible seat occurrences ranks=%v must fail open without a guessed pointer, got %q", ranks, got)
		}
	}
}
