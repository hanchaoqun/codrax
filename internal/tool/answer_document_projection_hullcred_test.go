package tool

// answer_document_projection_hullcred_test.go — HULL-CRED display pins
// (§29.104 终判③, 2026-07-17):
//
//   - the per-segment-proven demotion speaks its OWN word fork 无链上凭证
//     (逐段核验,整席降道) ONLY when the decoded segment inventory rides the
//     row beside the marker (claim gated on proof); an inventory-less marker
//     falls back to the generic R4 bytes;
//   - the envelope-tier keep-⛓ row wears the (包络级凭证) honest word — and
//     NEVER beside a demotion: the display arm re-gates on !LaneDemoted so a
//     corrupted both-bools record cannot speak the contradictory word pair
//     (便宜修轮件2);
//   - untouched demoted rows keep the legacy R4 bytes with no new word;
//   - the types-side decode cap mirrors the engine cap (types cannot import
//     tracequery, so the equality is pinned here where both are visible).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hullcredCredentialTiersProjection — the representative legend-sweep shape
// (worker/env geometry of the engine end-to-end pins): the per-segment-proven
// ◇ demotion beside the envelope-tier ⛓ keep, with a plain chain seat as the
// board anchor.
func hullcredCredentialTiersProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-200", "app-100"},
		WindowStartTs: 1.0,
		WindowEndTs:   1.3,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "hullcred-chain-seat",
			Subject: "helper-400", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 17.0, CumulativeImpactMS: 17.0, EffectiveImpactMS: 17.0,
			Rank: 1, Tier: "primary", Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}, {
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "hullcred-envelope-seat",
			Subject: "env-500", Object: "io_wait", TypeToken: "io_wait",
			StateKind: "io_wait", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 6.0, CumulativeImpactMS: 6.0, EffectiveImpactMS: 6.0,
			ChainCredentialEnvelopeLevel: true,
			Rank:                         2, Tier: "secondary", Confidence: 0.84, LineStart: 30, LineEnd: 40,
		}},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "hullcred-disjoint-seat",
			Subject: "worker-200", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
			StateKind: "d_state", ChainRelevance: "adjacent",
			ImpactMS: 23.0, CumulativeImpactMS: 23.0,
			ChainCredentialLaneDemoted:     true,
			ChainCredentialSegmentDisjoint: true,
			ChainCredentialSegments:        [][2]float64{{1.002, 1.012}, {1.032, 1.045}},
			Confidence:                     0.8, LineStart: 50, LineEnd: 60,
		}},
	}
}

// TestHULLCREDCapMirrorsEngine — cap 双包镜像 pin.
func TestHULLCREDCapMirrorsEngine(t *testing.T) {
	if types.TraceCausalProjectionChainCredentialSegmentCap != tracequery.CriticalBlockingCredentialSegmentCap {
		t.Fatalf("the types-side credential segment cap drifted from the engine: %d vs %d",
			types.TraceCausalProjectionChainCredentialSegmentCap, tracequery.CriticalBlockingCredentialSegmentCap)
	}
}

func hullcredDisjointNode() types.TraceCausalProjectionNode {
	// The engine-minted wire shape of the segment-disjoint demotion (worker
	// witness of TestHULLCREDSegmentDisjointDemotionEndToEnd after decode).
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "hullcred-disjoint",
		Subject: "worker-200", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
		StateKind: "d_state", ChainRelevance: "adjacent",
		ImpactMS: 23.0, CumulativeImpactMS: 23.0,
		ChainCredentialLaneDemoted:     true,
		ChainCredentialSegmentDisjoint: true,
		ChainCredentialSegments:        [][2]float64{{1.002, 1.012}, {1.032, 1.045}},
		Confidence:                     0.8, LineStart: 10, LineEnd: 20,
	}
}

// TestHULLCREDSegmentDisjointWordFace — the fork speaks the per-segment
// wording on both faces, records its own mark (not the generic R4 mark), and
// the legend carries the new entry.
func TestHULLCREDSegmentDisjointWordFace(t *testing.T) {
	row := runtimeTraceProjTreeRow{Node: hullcredDisjointNode(), marks: &runtimeTraceProjMarkSet{}}
	notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, true), "\n")
	if !strings.Contains(notes, "无链上凭证(逐段核验,整席不入链上榜,见图例)") {
		t.Fatalf("disjoint fork must speak the per-segment word:\n%s", notes)
	}
	if strings.Contains(notes, "无链上凭证(整席不入链上榜,见图例)") {
		t.Fatalf("the generic R4 chip must not double-speak beside the fork:\n%s", notes)
	}
	notesEN := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, false), "\n")
	if !strings.Contains(notesEN, "no chain credential (per-segment verified; whole seat off the on-chain board; see legend)") {
		t.Fatalf("en mirror of the disjoint fork missing:\n%s", notesEN)
	}
	if !row.marks.has(runtimeTraceProjMarkChainCredentialSegmentDisjoint) {
		t.Fatalf("the disjoint legend mark must record at the emission site")
	}
	if row.marks.has(runtimeTraceProjMarkChainCredentialDemoted) {
		t.Fatalf("the generic R4 mark must not fire on the fork arm")
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(row.marks, true), "\n")
	if !strings.Contains(legend, "`无链上凭证(逐段核验,整席不入链上榜)`") || !strings.Contains(legend, "包络端点是嘈声") {
		t.Fatalf("legend must carry the per-segment entry:\n%s", legend)
	}
	legendEN := strings.Join(runtimeTraceProjLegendGroupLines(row.marks, false), "\n")
	if !strings.Contains(legendEN, "`no chain credential (per-segment verified; whole seat off the on-chain board)`") {
		t.Fatalf("en legend must carry the per-segment entry:\n%s", legendEN)
	}
}

// TestHULLCREDDisjointClaimGatedOnProof — an inventory-less disjoint marker
// (corrupt / foreign artifact) must fall back to the generic R4 bytes: the
// per-segment claim never renders without its proof.
func TestHULLCREDDisjointClaimGatedOnProof(t *testing.T) {
	node := hullcredDisjointNode()
	node.ChainCredentialSegments = nil
	row := runtimeTraceProjTreeRow{Node: node, marks: &runtimeTraceProjMarkSet{}}
	notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, true), "\n")
	if strings.Contains(notes, "逐段核验") {
		t.Fatalf("the per-segment claim must never render without its inventory:\n%s", notes)
	}
	if !strings.Contains(notes, "无链上凭证(整席不入链上榜,见图例)") {
		t.Fatalf("the inventory-less marker must keep the generic R4 bytes:\n%s", notes)
	}
	if !row.marks.has(runtimeTraceProjMarkChainCredentialDemoted) || row.marks.has(runtimeTraceProjMarkChainCredentialSegmentDisjoint) {
		t.Fatalf("the fallback must record the generic mark only")
	}
}

// TestHULLCREDEnvelopeWordFace — the keep-⛓ envelope tier wears the honest
// word on both faces with its legend entry; the demotion word families stay
// silent.
func TestHULLCREDEnvelopeWordFace(t *testing.T) {
	row := runtimeTraceProjTreeRow{marks: &runtimeTraceProjMarkSet{}, Node: types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "hullcred-envelope",
		Subject: "env-500", Object: "io_wait", TypeToken: "io_wait",
		StateKind: "io_wait", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: 66.0, CumulativeImpactMS: 66.0,
		ChainCredentialEnvelopeLevel: true,
		Confidence:                   0.84, LineStart: 30, LineEnd: 40,
	}}
	notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, true), "\n")
	if !strings.Contains(notes, "(包络级凭证,见图例)") {
		t.Fatalf("the envelope keep must wear the honest word:\n%s", notes)
	}
	if strings.Contains(notes, "无链上凭证") {
		t.Fatalf("the demotion word family is forbidden on a keep-⛓ row:\n%s", notes)
	}
	notesEN := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, false), "\n")
	if !strings.Contains(notesEN, "(envelope-level credential; see legend)") {
		t.Fatalf("en mirror of the envelope word missing:\n%s", notesEN)
	}
	if !row.marks.has(runtimeTraceProjMarkChainCredentialEnvelope) {
		t.Fatalf("the envelope legend mark must record at the emission site")
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(row.marks, true), "\n")
	if !strings.Contains(legend, "`(包络级凭证)`") || !strings.Contains(legend, "逐段区间清单缺席") {
		t.Fatalf("legend must carry the envelope entry:\n%s", legend)
	}
	legendEN := strings.Join(runtimeTraceProjLegendGroupLines(row.marks, false), "\n")
	if !strings.Contains(legendEN, "`(envelope-level credential)`") {
		t.Fatalf("en legend must carry the envelope entry:\n%s", legendEN)
	}
}

// TestHULLCREDEnvelopeWordDemotionMutualExclusion — 便宜修轮件2: the envelope
// word and the demotion word family are 双官同指 mutually exclusive on one
// row. The engine's four-arm verdict never sets both bools, but a corrupted /
// foreign record can — the display arm must re-gate on !LaneDemoted so the
// contradictory pair 「无链上凭证」+「(包络级凭证)」 never co-renders: the
// demotion chip wins (conservative face), the envelope word and its mark stay
// silent, on both language faces.
func TestHULLCREDEnvelopeWordDemotionMutualExclusion(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "hullcred-both-bools",
		Subject: "env-500", Object: "io_wait", TypeToken: "io_wait",
		StateKind: "io_wait", ChainRelevance: "adjacent",
		ImpactMS: 6.0, CumulativeImpactMS: 6.0,
		ChainCredentialLaneDemoted:   true,
		ChainCredentialEnvelopeLevel: true,
		Confidence:                   0.84, LineStart: 30, LineEnd: 40,
	}
	for _, zh := range []bool{true, false} {
		row := runtimeTraceProjTreeRow{Node: node, marks: &runtimeTraceProjMarkSet{}}
		notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, zh), "\n")
		for _, banned := range []string{"包络级凭证", "envelope-level credential"} {
			if strings.Contains(notes, banned) {
				t.Fatalf("zh=%v: the envelope word must never render beside a demotion (%s):\n%s", zh, banned, notes)
			}
		}
		demoteChip := "无链上凭证(整席不入链上榜,见图例)"
		if !zh {
			demoteChip = "no chain credential (whole seat off the on-chain board; see legend)"
		}
		if !strings.Contains(notes, demoteChip) {
			t.Fatalf("zh=%v: the demotion chip must still render on the both-bools row:\n%s", zh, notes)
		}
		if row.marks.has(runtimeTraceProjMarkChainCredentialEnvelope) {
			t.Fatalf("zh=%v: the envelope legend mark must not fire beside a demotion", zh)
		}
		if !row.marks.has(runtimeTraceProjMarkChainCredentialDemoted) {
			t.Fatalf("zh=%v: the generic demotion mark must still record", zh)
		}
	}
}

// TestHULLCREDUntouchedRowsKeepLegacyBytes — 负臂: a plain R4-demoted row
// (no HULL-CRED markers) keeps the legacy chip byte-identically and never
// wears a new word; a plain keep-⛓ row wears nothing.
func TestHULLCREDUntouchedRowsKeepLegacyBytes(t *testing.T) {
	demoted := runtimeTraceProjTreeRow{marks: &runtimeTraceProjMarkSet{}, Node: types.TraceCausalProjectionNode{
		Subject: "logd.writer-112", Object: "cpu_affinity_or_cpuset", TypeToken: "cpu_affinity_or_cpuset",
		ChainRelevance:             "adjacent",
		ChainCredentialLaneDemoted: true,
	}}
	notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(demoted, true), "\n")
	if !strings.Contains(notes, "无链上凭证(整席不入链上榜,见图例)") {
		t.Fatalf("the legacy R4 chip bytes must survive unchanged:\n%s", notes)
	}
	for _, banned := range []string{"逐段核验", "包络级凭证"} {
		if strings.Contains(notes, banned) {
			t.Fatalf("no HULL-CRED word may appear on an untouched row (%s):\n%s", banned, notes)
		}
	}
	if demoted.marks.has(runtimeTraceProjMarkChainCredentialSegmentDisjoint) || demoted.marks.has(runtimeTraceProjMarkChainCredentialEnvelope) {
		t.Fatalf("no HULL-CRED mark may fire on an untouched row")
	}
	keep := runtimeTraceProjTreeRow{marks: &runtimeTraceProjMarkSet{}, Node: types.TraceCausalProjectionNode{
		Subject: "helper-400", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
		ChainRelevance: "on_chain",
		// The segment-verified keep publishes its inventory but wears NO new
		// word (the credential is the inventory itself; wording unchanged).
		ChainCredentialSegments: [][2]float64{{1.065, 1.072}},
	}}
	notesKeep := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(keep, true), "\n")
	for _, banned := range []string{"逐段核验", "包络级凭证", "无链上凭证"} {
		if strings.Contains(notesKeep, banned) {
			t.Fatalf("a segment-verified keep row must stay word-silent (%s):\n%s", banned, notesKeep)
		}
	}
}
