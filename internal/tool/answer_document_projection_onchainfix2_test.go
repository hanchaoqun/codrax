package tool

// answer_document_projection_onchainfix2_test.go — ONCHAIN-FIX-2 display pins
// (Q5/Q6 已追认 + 件1 包络泛化, 2026-07-18):
//
//   - 件3: the truncated lower-bound prefix chip (凭证清单不完整,实际锚定
//     不小于所证) renders
//     ONLY beside a decoded non-empty inventory on the current keep-⛓ lane
//     (claim gated on proof — 下界 caliber); a demotion, a
//     disjoint marker, an inventory-less marker or an off-chain lane all
//     suppress it;
//   - 件1: the envelope word (包络级凭证) — now also minted on rank-lane
//     hull-only keeps — re-gates on the CURRENT on-chain lane (a later
//     fold/absorb pass may move a stamped row's channel).
//
// MUTATION self-check: dropping the display gate's inventory condition reds
// TestONCHAINFIX2TruncatedWordClaimGatedOnProof; dropping the lane gate reds
// TestONCHAINFIX2TruncatedWordSuppressed / TestONCHAINFIX2EnvelopeWordLaneGated.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// onchainfix2TruncatedLowerBoundProjection — the representative legend-sweep
// shape: a keep-⛓ D-state view row publishing its checked prefix with the
// truncated lower-bound marker (the lowbound-117 emit-pin geometry).
func onchainfix2TruncatedLowerBoundProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"lowbound-117", "app-100"},
		WindowStartTs: 1.4,
		WindowEndTs:   1.6,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "ofix2-lowbound-seat",
			Subject: "lowbound-117", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
			StateKind: "d_state", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 80.0, CumulativeImpactMS: 80.0, EffectiveImpactMS: 80.0,
			ChainCredentialSegments:          [][2]float64{{1.500, 1.502}, {1.520, 1.522}},
			ChainCredentialSegmentsTruncated: true,
			Rank:                             1, Tier: "primary", Confidence: 0.8, LineStart: 98, LineEnd: 99,
		}},
	}
}

func onchainfix2TruncatedNode() types.TraceCausalProjectionNode {
	return onchainfix2TruncatedLowerBoundProjection().OnChainCauses[0]
}

// TestONCHAINFIX2TruncatedWordFace — the chip renders on both faces, records
// its own mark, and the legend carries the lower-bound entry.
func TestONCHAINFIX2TruncatedWordFace(t *testing.T) {
	row := runtimeTraceProjTreeRow{Node: onchainfix2TruncatedNode(), marks: &runtimeTraceProjMarkSet{}}
	notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, true), "\n")
	if !strings.Contains(notes, "(凭证清单不完整,实际锚定不小于所证,见图例)") {
		t.Fatalf("truncated keep must speak the lower-bound chip:\n%s", notes)
	}
	notesEN := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, false), "\n")
	if !strings.Contains(notesEN, "(credential inventory incomplete; anchored share is at least the proven; see legend)") {
		t.Fatalf("en mirror of the lower-bound chip missing:\n%s", notesEN)
	}
	if !row.marks.has(runtimeTraceProjMarkChainCredentialTruncatedLowerBound) {
		t.Fatalf("the lower-bound legend mark must record at the emission site")
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(row.marks, true), "\n")
	if !strings.Contains(legend, "`(凭证清单不完整,实际锚定不小于所证)`") || !strings.Contains(legend, "实际锚定不小于此清单所证") {
		t.Fatalf("legend must carry the lower-bound entry with the 下界 caliber sentence:\n%s", legend)
	}
	legendEN := strings.Join(runtimeTraceProjLegendGroupLines(row.marks, false), "\n")
	if !strings.Contains(legendEN, "`(credential inventory incomplete; anchored share is at least the proven)`") {
		t.Fatalf("en legend must carry the lower-bound entry:\n%s", legendEN)
	}
}

// TestONCHAINFIX2TruncatedWordClaimGatedOnProof — the marker without its
// decoded inventory (corrupt / foreign artifact) must stay silent.
func TestONCHAINFIX2TruncatedWordClaimGatedOnProof(t *testing.T) {
	node := onchainfix2TruncatedNode()
	node.ChainCredentialSegments = nil
	for _, zh := range []bool{true, false} {
		row := runtimeTraceProjTreeRow{Node: node, marks: &runtimeTraceProjMarkSet{}}
		notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, zh), "\n")
		for _, banned := range []string{"不小于所证", "at least the proven"} {
			if strings.Contains(notes, banned) {
				t.Fatalf("zh=%v: the lower-bound claim must never render without its inventory proof:\n%s", zh, notes)
			}
		}
		if row.marks.has(runtimeTraceProjMarkChainCredentialTruncatedLowerBound) {
			t.Fatalf("zh=%v: the mark must not fire without the inventory", zh)
		}
	}
}

// TestONCHAINFIX2TruncatedWordSuppressed — demotion words, the disjoint
// marker and an off-chain lane all suppress the lower-bound chip (the
// stronger/conservative vocabulary wins; the word is a keep-⛓ disclosure).
func TestONCHAINFIX2TruncatedWordSuppressed(t *testing.T) {
	shapes := []struct {
		name   string
		mutate func(*types.TraceCausalProjectionNode)
	}{
		{"lane_demoted", func(n *types.TraceCausalProjectionNode) {
			n.ChainRelevance = "adjacent"
			n.ChainCredentialLaneDemoted = true
		}},
		{"disjoint_marker", func(n *types.TraceCausalProjectionNode) {
			n.ChainRelevance = "adjacent"
			n.ChainCredentialLaneDemoted = true
			n.ChainCredentialSegmentDisjoint = true
		}},
		{"off_chain_lane", func(n *types.TraceCausalProjectionNode) {
			n.ChainRelevance = "adjacent"
		}},
	}
	for _, shape := range shapes {
		node := onchainfix2TruncatedNode()
		shape.mutate(&node)
		for _, zh := range []bool{true, false} {
			row := runtimeTraceProjTreeRow{Node: node, marks: &runtimeTraceProjMarkSet{}}
			notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, zh), "\n")
			for _, banned := range []string{"不小于所证", "at least the proven"} {
				if strings.Contains(notes, banned) {
					t.Fatalf("%s zh=%v: the lower-bound chip must stay silent:\n%s", shape.name, zh, notes)
				}
			}
			if row.marks.has(runtimeTraceProjMarkChainCredentialTruncatedLowerBound) {
				t.Fatalf("%s zh=%v: the mark must not fire", shape.name, zh)
			}
		}
	}
}

// TestONCHAINFIX2EnvelopeWordLaneGated — 件1: the envelope word re-gates on
// the CURRENT on-chain lane (rank rows may be folded/absorbed onto another
// channel after the enrich stamp; the stamped bit alone must not speak).
func TestONCHAINFIX2EnvelopeWordLaneGated(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ofix2-envelope-offlane",
		Subject: "burst-118", Object: "io_burst_episode", TypeToken: "io_burst_episode",
		StateKind: "io_wait", ChainRelevance: "adjacent",
		ImpactMS: 5.0, CumulativeImpactMS: 5.0,
		ChainCredentialEnvelopeLevel: true,
		Confidence:                   0.8, LineStart: 10, LineEnd: 20,
	}
	for _, zh := range []bool{true, false} {
		row := runtimeTraceProjTreeRow{Node: node, marks: &runtimeTraceProjMarkSet{}}
		notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, zh), "\n")
		for _, banned := range []string{"包络级凭证", "envelope-level credential"} {
			if strings.Contains(notes, banned) {
				t.Fatalf("zh=%v: the envelope word must not render off the on-chain lane:\n%s", zh, notes)
			}
		}
		if row.marks.has(runtimeTraceProjMarkChainCredentialEnvelope) {
			t.Fatalf("zh=%v: the envelope mark must not fire off the on-chain lane", zh)
		}
	}
}
