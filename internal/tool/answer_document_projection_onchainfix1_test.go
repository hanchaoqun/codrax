package tool

// answer_document_projection_onchainfix1_test.go — ONCHAIN-FIX-1 件1 display
// pins (mint audit 命题2 不一致①, 2026-07-18):
//
//   - the interval-less identity-inheritance keep-⛓ row wears the honest word
//     身份继承(链窗级,无区间凭证) on both faces with its legend entry — the
//     word replaces the retired fabricated overlap value;
//   - every stronger credential vocabulary suppresses it: the demotion words,
//     the HULL-CRED per-segment inventory and the envelope word all win, and
//     an off-chain lane never wears it (链上面与降道面不同行共存 — the display
//     re-gates like the envelope arm, so a corrupted / foreign artifact with
//     contradictory bits cannot speak two credential tiers on one row).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// onchainfix1IdentityInheritanceProjection — the representative legend-sweep
// shape: an interval-less identity-inheritance keep-⛓ D view row beside a
// plain chain seat as the board anchor.
func onchainfix1IdentityInheritanceProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-200", "app-100"},
		WindowStartTs: 1.0,
		WindowEndTs:   1.3,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "ofix1-chain-seat",
			Subject: "helper-400", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 17.0, CumulativeImpactMS: 17.0, EffectiveImpactMS: 17.0,
			Rank: 1, Tier: "primary", Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}, {
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ofix1-identity-seat",
			Subject: "worker-200", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
			StateKind: "d_state", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 4.0, CumulativeImpactMS: 4.0, EffectiveImpactMS: 4.0,
			ChainIdentityInheritance: true,
			Rank:                     2, Tier: "secondary", Confidence: 0.8, LineStart: 30, LineEnd: 40,
		}},
	}
}

func onchainfix1IdentityNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "ofix1-identity",
		Subject: "worker-200", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
		StateKind: "d_state", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: 4.0, CumulativeImpactMS: 4.0,
		ChainIdentityInheritance: true,
		Confidence:               0.8, LineStart: 30, LineEnd: 40,
	}
}

// TestONCHAINFIX1IdentityWordFace — the honest word renders on both faces,
// records its own mark and the legend carries the entry (bidirectional).
func TestONCHAINFIX1IdentityWordFace(t *testing.T) {
	row := runtimeTraceProjTreeRow{Node: onchainfix1IdentityNode(), marks: &runtimeTraceProjMarkSet{}}
	notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, true), "\n")
	if !strings.Contains(notes, "身份继承(链窗级,无区间凭证,见图例)") {
		t.Fatalf("the identity-inheritance keep must wear the honest word:\n%s", notes)
	}
	if strings.Contains(notes, "无链上凭证") || strings.Contains(notes, "包络级凭证") {
		t.Fatalf("no other credential vocabulary may ride the identity row:\n%s", notes)
	}
	notesEN := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, false), "\n")
	if !strings.Contains(notesEN, "identity inheritance (chain-window tier, no interval credential; see legend)") {
		t.Fatalf("en mirror of the identity word missing:\n%s", notesEN)
	}
	if !row.marks.has(runtimeTraceProjMarkChainIdentityInheritance) {
		t.Fatalf("the identity legend mark must record at the emission site")
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(row.marks, true), "\n")
	if !strings.Contains(legend, "`身份继承(链窗级,无区间凭证)`") || !strings.Contains(legend, "不铸重叠值") {
		t.Fatalf("legend must carry the identity-inheritance entry:\n%s", legend)
	}
	legendEN := strings.Join(runtimeTraceProjLegendGroupLines(row.marks, false), "\n")
	if !strings.Contains(legendEN, "`identity inheritance (chain-window tier, no interval credential)`") {
		t.Fatalf("en legend must carry the identity-inheritance entry:\n%s", legendEN)
	}
}

// TestONCHAINFIX1IdentityWordSuppressed — every stronger vocabulary and every
// off-chain lane silences the identity word (and its mark).
func TestONCHAINFIX1IdentityWordSuppressed(t *testing.T) {
	for name, mutate := range map[string]func(*types.TraceCausalProjectionNode){
		"lane_demoted": func(n *types.TraceCausalProjectionNode) {
			n.ChainRelevance = "adjacent"
			n.ChainCredentialLaneDemoted = true
		},
		"envelope_word_wins": func(n *types.TraceCausalProjectionNode) {
			n.ChainCredentialEnvelopeLevel = true
		},
		"segment_inventory_wins": func(n *types.TraceCausalProjectionNode) {
			n.ChainCredentialSegments = [][2]float64{{1.002, 1.012}}
		},
		"off_chain_lane": func(n *types.TraceCausalProjectionNode) {
			n.ChainRelevance = "adjacent"
		},
	} {
		node := onchainfix1IdentityNode()
		mutate(&node)
		row := runtimeTraceProjTreeRow{Node: node, marks: &runtimeTraceProjMarkSet{}}
		notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, true), "\n")
		if strings.Contains(notes, "身份继承") {
			t.Fatalf("%s: the identity word must stay silent:\n%s", name, notes)
		}
		if row.marks.has(runtimeTraceProjMarkChainIdentityInheritance) {
			t.Fatalf("%s: the identity mark must not fire", name)
		}
	}
}
