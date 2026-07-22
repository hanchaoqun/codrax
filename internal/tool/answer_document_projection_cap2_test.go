package tool

// answer_document_projection_cap2_test.go — CAP-2+THERM batch (§28.4/§28.5,
// docs/design/real_trace_campaign_20260705.md, 2026-07-09) display pins:
//
//	三级披露词 — the typed cluster-topology token upgrades the former
//	   簇结构不可判 degrade wording exactly three ways (不可判 / 共动 /
//	   锚推定), each verbatim, zh + EN, byte-stable legacy on absence;
//	THERM — the 窗内该簇受热限压至 X sentence rides the typed thermal_cap_khz
//	   field on every clause branch, 双向 (present/absent), zero-weight;
//	R5d word家 — the gated composition text upgrades on the gated topology
//	   token through the SAME single-source clause helper;
//	Note emission — fold_cluster_topology / fold_rail_basis / thermal_cap_khz
//	   emit exactly on their typed fields (absence = byte-identical stream);
//	Wire-token drift pin — display mirrors of CoreCapabilityTopology*.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCAP2WireTopologyTokensMirrorEngine(t *testing.T) {
	if runtimeTraceCapabilityTopologyComovement != tracequery.CoreCapabilityTopologyComovement ||
		runtimeTraceCapabilityTopologyKeyedRail != tracequery.CoreCapabilityTopologyKeyedRail {
		t.Fatalf("display topology tokens drifted from the engine constants (core_capability.go)")
	}
}

// --- 三级披露词 (verbatim, the §28.4 ruling wording) ----------------------------

func TestCAP2ThreeLevelDisclosureWordsVerbatim(t *testing.T) {
	cases := []struct {
		source, topo string
		zh, en       string
	}{
		// Level 0 (unjudged): the degrade word stands.
		{runtimeTraceCapabilitySourceFreqOnly, "",
			"簇结构不可判,按纯频率比折算", "cluster structure unjudged, frequency-ratio fold only"},
		// Level 1 (Tier-1): membership measured from co-moving change points.
		{runtimeTraceCapabilitySourceDefault, runtimeTraceCapabilityTopologyComovement,
			"按实测频点共动分簇折算", "measured co-moving frequency clusters (default capability ratios)"},
		// Level 2 (Tier-2): rail claim + membership presumption worded apart.
		{runtimeTraceCapabilitySourceDefault, runtimeTraceCapabilityTopologyKeyedRail,
			"按簇轨实测折算(成员按锚点连续推定)", "measured cluster-rail fold (membership by anchor contiguity)"},
		// Legacy/explicit: absence keeps the §26 word byte-identically.
		{runtimeTraceCapabilitySourceDefault, "",
			"按默认算力比粗算", "default capability-ratio estimate"},
	}
	for _, tc := range cases {
		if got := runtimeTraceProjCapabilityCaliberClauseTopo(tc.source, tc.topo, true); got != tc.zh {
			t.Fatalf("(%s,%s) zh clause = %q, want %q", tc.source, tc.topo, got, tc.zh)
		}
		if got := runtimeTraceProjCapabilityCaliberClauseTopo(tc.source, tc.topo, false); got != tc.en {
			t.Fatalf("(%s,%s) en clause = %q, want %q", tc.source, tc.topo, got, tc.en)
		}
	}
	// freq_only ignores any stray topology token: an unjudged structure can
	// never wear an evidence word (fail-loud wording is sticky).
	if got := runtimeTraceProjCapabilityCaliberClauseTopo(runtimeTraceCapabilitySourceFreqOnly, runtimeTraceCapabilityTopologyKeyedRail, true); got != "簇结构不可判,按纯频率比折算" {
		t.Fatalf("freq_only must keep the degrade word regardless of topo: %q", got)
	}
}

// The clause surfaces consume the node token end to end (the Dominant branch
// as representative; the pre-CAP-2 shape stays byte-stable).
func TestCAP2SupplyFoldClauseTopologyUpgrade(t *testing.T) {
	node := capClauseNode(5, 15, 20, 0, 5, runtimeTraceCapabilitySourceDefault)
	node.SupplyFoldTopologySource = runtimeTraceCapabilityTopologyComovement
	clause, _, ok := runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if !ok || !strings.Contains(clause, "(运行频点非最高,按全域最大核最高频折算,下界,按实测频点共动分簇折算)") {
		t.Fatalf("Tier-1 clause must carry the co-movement word:\n%s", clause)
	}
	node.SupplyFoldTopologySource = runtimeTraceCapabilityTopologyKeyedRail
	clause, _, ok = runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if !ok || !strings.Contains(clause, "(运行频点非最高,按全域最大核最高频折算,下界,按簇轨实测折算(成员按锚点连续推定))") {
		t.Fatalf("Tier-2 clause must carry the anchor-presumption word:\n%s", clause)
	}
	if strings.Contains(clause, "按默认算力比粗算") {
		t.Fatalf("the upgraded word replaces the bare default-table word (legend carries the ratio detail):\n%s", clause)
	}
	// Byte-stable legacy control.
	node.SupplyFoldTopologySource = ""
	clause, _, _ = runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if !strings.Contains(clause, "(运行频点非最高,按全域最大核最高频折算,下界,按默认算力比粗算)") {
		t.Fatalf("absence must keep the §26 wording byte-identically:\n%s", clause)
	}
}

// --- THERM: 窗内该簇受热限压至 X (双向, zero-weight) ----------------------------

func TestCAP2ThermalCapSentence(t *testing.T) {
	node := capClauseNode(5, 15, 20, 0, 5, runtimeTraceCapabilitySourceDefault)
	node.ThermalCapKHz = 1850000
	// CR-3 件⑥ F-10 (2026-07-12): the 受热限压 word now requires the typed
	// in-window witness bit beside the cap value.
	node.ThermalCapWitnessed = true
	clause, keep, ok := runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if !ok || !strings.Contains(clause, ";窗内该簇受热限压至 1.85GHz") {
		t.Fatalf("the THERM sentence must append with 数值+单位:\n%s", clause)
	}
	if keep != "供给折算缺口" {
		t.Fatalf("the keep marker stays on the mechanism word (THERM is an appendix): %q", keep)
	}
	en, _, ok := runtimeTraceProjSupplyFoldClause(node, 0, false, false)
	if !ok || !strings.Contains(en, "; a thermal/policy cap pressed this cluster to 1.85GHz in-window") {
		t.Fatalf("EN THERM sentence missing:\n%s", en)
	}
	// Zero-weight: the deficit figure is untouched by the sentence.
	if !strings.Contains(clause, "供给折算缺口 5.000ms") {
		t.Fatalf("THERM must not move any number:\n%s", clause)
	}
	// CR-3 件⑥ F-10 (CR-2 冷读 D5 witness — 1.53GHz wore the thermal word
	// with zero in-window event): an UNWITNESSED press states the governed
	// frequency without the thermal cause claim.
	node.ThermalCapWitnessed = false
	unwitnessed, _, ok := runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if !ok || !strings.Contains(unwitnessed, ";窗内该簇运行于 1.85GHz(限压原因未见证)") {
		t.Fatalf("an unwitnessed press must speak the honest governed-frequency form:\n%s", unwitnessed)
	}
	if strings.Contains(unwitnessed, "受热限压") {
		t.Fatalf("the thermal word requires the in-window witness:\n%s", unwitnessed)
	}
	unwitnessedEN, _, _ := runtimeTraceProjSupplyFoldClause(node, 0, false, false)
	if !strings.Contains(unwitnessedEN, "; this cluster ran governed at 1.85GHz in-window (cap cause unwitnessed)") {
		t.Fatalf("EN unwitnessed form missing:\n%s", unwitnessedEN)
	}
	// 双向: no typed press, no sentence — byte-identical to the pre-THERM form.
	node.ThermalCapKHz = 0
	bare, _, _ := runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if strings.Contains(bare, "热限") || strings.Contains(bare, "限压") {
		t.Fatalf("no typed press must render no THERM words:\n%s", bare)
	}
	// The sentence rides the affirmative branch too (any fold branch).
	affirmative := capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceDefault)
	affirmative.ThermalCapKHz = 1550000
	affirmative.ThermalCapWitnessed = true
	sentence, _, ok := runtimeTraceProjSupplyFoldClause(affirmative, 0, false, true)
	if !ok || !strings.Contains(sentence, ";窗内该簇受热限压至 1.55GHz") {
		t.Fatalf("the THERM sentence must ride every fold branch:\n%s", sentence)
	}
}

// --- R5d word家: the gated composition upgrades on the gated token -------------

func TestCAP2GatedCompositionTopologyUpgrade(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "worker-9", PriorityInversionCandidate: true,
		GatedRunnableMS: 20.713, GatedRunningDeficitMS: 16.697,
		GatedCapabilitySource: runtimeTraceCapabilitySourceDefault,
		GatedTopologySource:   runtimeTraceCapabilityTopologyComovement,
	}
	text := runtimeTraceProjInversionCompositionText(node, true)
	if !strings.Contains(text, "running 折算 16.697ms(运行频点非最高,按全域最大核最高频折算,按实测频点共动分簇折算)") {
		t.Fatalf("the gated composition must upgrade on the gated topology token:\n%s", text)
	}
	node.GatedTopologySource = ""
	if got := runtimeTraceProjInversionCompositionText(node, true); !strings.Contains(got, "(运行频点非最高,按全域最大核最高频折算,按默认算力比粗算)") {
		t.Fatalf("absence keeps the §26 gated wording byte-identically:\n%s", got)
	}
}

// --- legend seats: each upgraded word teaches its own entry on demand ----------

func TestCAP2TopologyLegendSeats(t *testing.T) {
	projection := capRunningDeficitProjection(runtimeTraceCapabilitySourceDefault)
	projection.OnChainCauses[0].SupplyFoldTopologySource = runtimeTraceCapabilityTopologyKeyedRail
	projection.OnChainCauses[0].ThermalCapKHz = 1850000
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "按簇轨实测折算(成员按锚点连续推定)") {
		t.Fatalf("the G1 sub-row caliber must carry the Tier-2 word:\n%s", fence)
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if !strings.Contains(legend, "- `按簇轨实测折算(成员按锚点连续推定)` =") {
		t.Fatalf("the keyed-rail legend entry must render with the word:\n%s", legend)
	}
	if strings.Contains(legend, "- `按默认算力比粗算` =") {
		t.Fatalf("the default-table entry stays off an upgraded shape:\n%s", legend)
	}
	comove := capRunningDeficitProjection(runtimeTraceCapabilitySourceDefault)
	comove.OnChainCauses[0].SupplyFoldTopologySource = runtimeTraceCapabilityTopologyComovement
	comoveModel := buildRuntimeTraceProjTreeModel(comove, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	comoveFence := runtimeTraceProjTreeFence(comoveModel, true) // marks emit at render time
	if !strings.Contains(comoveFence, "按实测频点共动分簇折算") {
		t.Fatalf("the G1 sub-row caliber must carry the Tier-1 word:\n%s", comoveFence)
	}
	comoveLegend := strings.Join(runtimeTraceProjLegendGroupLines(comoveModel.Marks, true), "\n")
	if !strings.Contains(comoveLegend, "- `按实测频点共动分簇折算` =") {
		t.Fatalf("the co-movement legend entry must render with the word:\n%s", comoveLegend)
	}
}

// --- note emission (typed fields → keys, absence byte-identical) ---------------

func TestCAP2NoteEmission(t *testing.T) {
	basis := &tracequery.SupplyFoldBasis{
		KnownMs: 5, CapabilitySource: tracequery.CoreCapabilitySourceDefault,
		ClusterTopologySource: tracequery.CoreCapabilityTopologyKeyedRail,
		RailFamily:            "m3_c#_freq",
		RailGoverned:          []tracequery.SupplyFoldRailGoverned{{CPU: 12, Rail: "m3_c3_freq"}, {CPU: 13, Rail: "m3_c3_freq"}},
		ThermalCapKHz:         1850000,
	}
	joined := strings.Join(traceQueryTypedSupplyFoldRichNotes(basis, 1, 4), "\n")
	for _, want := range []string{
		types.TraceNoteKeyFoldClusterTopology + "=keyed_rail",
		types.TraceNoteKeyFoldRailBasis + "=族=m3_c#_freq;cpu12 频点=簇轨 m3_c3_freq;cpu13 频点=簇轨 m3_c3_freq",
		types.TraceNoteKeyThermalCapKHz + "=1850000",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("note %q must emit:\n%s", want, joined)
		}
	}
	// Absence: the pre-CAP-2 stream stays byte-identical (no new keys).
	bare := strings.Join(traceQueryTypedSupplyFoldRichNotes(&tracequery.SupplyFoldBasis{
		KnownMs: 5, CapabilitySource: tracequery.CoreCapabilitySourceDefault,
	}, 1, 4), "\n")
	for _, absent := range []string{
		types.TraceNoteKeyFoldClusterTopology, types.TraceNoteKeyFoldRailBasis, types.TraceNoteKeyThermalCapKHz,
	} {
		if strings.Contains(bare, absent) {
			t.Fatalf("empty fields must emit no %q note:\n%s", absent, bare)
		}
	}
}
