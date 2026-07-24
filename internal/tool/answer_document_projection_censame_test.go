package tool

// answer_document_projection_censame_test.go — CENSAME-1 (§29.213 件7,
// 2026-07-24): the board-level freq_only cause promise may count only rows
// whose fence face will actually render the frequency-ratio suffix.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func censameProjection(nodes ...types.TraceCausalProjectionNode) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:              []string{"worker-9", "app-100"},
		WindowStartTs:           925.410,
		WindowEndTs:             925.610,
		RootCauseFamilyObserved: true,
		OnChainCauses:           nodes,
	}
}

func censameUnknownBasisNode(subject string, rank int) types.TraceCausalProjectionNode {
	node := clustertieNode(subject, rank, runtimeTraceCapabilityFreqOnlyReasonFmaxTie, 925.42, 925.46)
	node.SupplyFoldDeficitMS = 0
	node.SupplyFoldIdealMS = 10
	node.SupplyFoldKnownMS = 10
	node.SupplyFoldUnknownMS = 10
	return node
}

func censameUnbalancedGatedNode(subject string, rank int) types.TraceCausalProjectionNode {
	node := clustertieNode(subject, rank, runtimeTraceCapabilityFreqOnlyReasonFmaxTie, 925.42, 925.46)
	node.SupplyFoldComputed = false
	node.SupplyFoldCapabilitySource = ""
	node.SupplyFoldCapabilityFreqOnlyReason = ""
	node.PriorityInversionCandidate = true
	node.GatedRunnableMS = 2
	node.GatedRunningDeficitMS = 1
	node.EffectiveImpactMS = 5 // printed component sum 3 != published total 5
	node.GatedCapabilitySource = runtimeTraceCapabilitySourceFreqOnly
	node.GatedCapabilityFreqOnlyReason = runtimeTraceCapabilityFreqOnlyReasonFmaxTie
	return node
}

// N1 first-red/then-green: both shapes used to satisfy the wire-token census
// while producing zero frequency-ratio suffix rows. The board promise must be
// absent on both language faces.
func TestCENSAMEZeroRenderedRowsMintNoBoardCausePromise(t *testing.T) {
	fixtures := map[string]types.TraceCausalProjection{
		"supply_unknown_basis_zero_deficit": censameProjection(
			censameUnknownBasisNode("worker-9", 1),
			censameUnknownBasisNode("helper-11", 2),
		),
		"gated_unbalanced_components": censameProjection(
			censameUnbalancedGatedNode("worker-9", 1),
			censameUnbalancedGatedNode("helper-11", 2),
		),
	}
	for name, projection := range fixtures {
		for _, zh := range []bool{true, false} {
			t.Run(name+"/zh="+map[bool]string{true: "true", false: "false"}[zh], func(t *testing.T) {
				model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				fence := runtimeTraceProjTreeFence(model, zh)
				for _, forbidden := range []string{"本板成因:", "board cause:"} {
					if strings.Contains(partsplitSquash(fence), partsplitSquash(forbidden)) {
						t.Fatalf("a zero-suffix board must not promise shared row causes (zh=%v):\n%s", zh, fence)
					}
				}
				if model.FreqOnlyCauseHoistReason != "" {
					t.Fatalf("zero-suffix rows must not enter the hoist census: %q", model.FreqOnlyCauseHoistReason)
				}
			})
		}
	}
}

func TestCENSAMEParticipationPredicateMirrorsSuffixProducers(t *testing.T) {
	row := func(node types.TraceCausalProjectionNode) runtimeTraceProjTreeRow {
		return runtimeTraceProjTreeRow{Node: node, HasData: true, Kind: runtimeTraceProjTreeRowCause}
	}
	assert := func(name string, node types.TraceCausalProjectionNode, want bool) {
		t.Helper()
		got := len(runtimeTraceProjFreqOnlyRowRenderedCauses(row(node), 200)) > 0
		if got != want {
			t.Fatalf("%s participation=%v, want %v", name, got, want)
		}
	}

	assert("unknown_basis_zero_deficit", censameUnknownBasisNode("worker-9", 1), false)
	assert("gated_unbalanced", censameUnbalancedGatedNode("worker-9", 1), false)
	assert("ordinary_fold", clustertieNode("worker-9", 1,
		runtimeTraceCapabilityFreqOnlyReasonFmaxTie, 925.42, 925.46), true)

	gated := censameUnbalancedGatedNode("worker-9", 1)
	gated.EffectiveImpactMS = 3 // 2 runnable + 1 running deficit balances
	assert("balanced_gated", gated, true)

	// A ranked inversion cause row suppresses its Triple mechanism clause
	// under §24②; the same typed inversion without cause-node identity keeps
	// the ordinary fold clause and therefore remains a participant.
	inversionCause := clustertieNode("worker-9", 1,
		runtimeTraceCapabilityFreqOnlyReasonFmaxTie, 925.42, 925.46)
	inversionCause.PriorityInversionCandidate = true
	inversionCause.RunnableMS = 20
	assert("ranked_inversion_triple_suppressed", inversionCause, false)
	inversionNonCause := inversionCause
	inversionNonCause.Rank = 0
	assert("non_cause_inversion_triple_renders", inversionNonCause, true)
}

func TestCENSAMEBoardPromiseImpliesTwoRenderedSuffixRows(t *testing.T) {
	fixtures := []types.TraceCausalProjection{
		clustertieCauseHoistProjection(),
		censameProjection(
			censameUnknownBasisNode("worker-9", 1),
			censameUnknownBasisNode("helper-11", 2),
		),
		censameProjection(
			censameUnbalancedGatedNode("worker-9", 1),
			censameUnbalancedGatedNode("helper-11", 2),
		),
	}
	for i, projection := range fixtures {
		for _, zh := range []bool{true, false} {
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := runtimeTraceProjTreeFence(model, zh)
			head, suffix := "本板成因:", "按频率比"
			if !zh {
				head, suffix = "board cause:", "frequency-ratio basis"
			}
			if !strings.Contains(partsplitSquash(fence), partsplitSquash(head)) {
				continue
			}
			renderedRows := 0
			for _, line := range strings.Split(fence, "\n") {
				if !strings.Contains(line, head) && strings.Contains(partsplitSquash(line), partsplitSquash(suffix)) {
					renderedRows++
				}
			}
			if renderedRows < 2 {
				t.Fatalf("fixture=%d zh=%v: board promise requires >=2 rendered suffix rows, got %d:\n%s",
					i, zh, renderedRows, fence)
			}
		}
	}
}

// N5: the real ×N merger clears a member-scoped supply-fold account but
// intentionally preserves the typed capability source/reason. Such merged
// rows must not re-enter the hoist census on those source tokens alone.
func TestCENSAMEMergedOccurrenceRowsWithClearedFoldMintNoPromise(t *testing.T) {
	merge := func(subject string, rank int) types.TraceCausalProjectionNode {
		var members []types.TraceCausalProjectionNode
		for i := 0; i < 3; i++ {
			node := clustertieNode(subject, rank,
				runtimeTraceCapabilityFreqOnlyReasonFmaxTie,
				925.42+float64(i)*0.01, 925.425+float64(i)*0.01)
			members = append(members, node)
		}
		return types.TraceCausalProjectionMergeOccurrenceRows(members)
	}
	first, second := merge("worker-9", 1), merge("helper-11", 2)
	for _, node := range []types.TraceCausalProjectionNode{first, second} {
		if node.SupplyFoldComputed || node.SupplyFoldCapabilitySource != runtimeTraceCapabilitySourceFreqOnly ||
			node.SupplyFoldCapabilityFreqOnlyReason != runtimeTraceCapabilityFreqOnlyReasonFmaxTie {
			t.Fatalf("fixture must reproduce the ×N cleared-account/source-preserved shape: %+v", node)
		}
	}
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(censameProjection(first, second),
			newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		if strings.Contains(fence, "本板成因:") || strings.Contains(fence, "board cause:") {
			t.Fatalf("zh=%v: ×N rows with cleared folds must mint no promise:\n%s", zh, fence)
		}
	}
}
