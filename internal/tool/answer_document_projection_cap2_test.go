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

// --- Governance cap authority (policy != thermal, zero-weight) ----------------

func TestCAP2ThermalCapSentence(t *testing.T) {
	node := capClauseNode(5, 15, 20, 0, 5, runtimeTraceCapabilitySourceDefault)
	node.GovernanceCapKHz = 1850000
	node.GovernanceCapClusterClass = "big"
	node.GovernanceCapMechanism = tracequery.SupplyFoldGovernanceCapThermalRail
	node.GovernanceCapWitnessed = true
	clause, keep, ok := runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if !ok || !strings.Contains(clause, ";窗内大核簇明确热控轨上限为 1.85GHz(不是 cpufreq 策略上限,不单独证明实际绑定影响)") {
		t.Fatalf("the thermal-rail sentence must append with exact mechanism:\n%s", clause)
	}
	if keep != "供给折算缺口" {
		t.Fatalf("the keep marker stays on the mechanism word (THERM is an appendix): %q", keep)
	}
	en, _, ok := runtimeTraceProjSupplyFoldClause(node, 0, false, false)
	if !ok || !strings.Contains(en, "; this big-core cluster had a 1.85GHz explicitly thermal-named rail ceiling in-window (not a cpufreq policy ceiling and not proof by itself of binding impact)") {
		t.Fatalf("EN thermal-rail sentence missing:\n%s", en)
	}
	// Zero-weight: the deficit figure is untouched by the sentence.
	if !strings.Contains(clause, "供给折算缺口 5.000ms") {
		t.Fatalf("THERM must not move any number:\n%s", clause)
	}
	// CR-3 件⑥ F-10 (CR-2 冷读 D5 witness — 1.53GHz wore the thermal word
	// with zero in-window event): an UNWITNESSED press states the governed
	// frequency without the thermal cause claim.
	node.GovernanceCapWitnessed = false
	unwitnessed, _, ok := runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if !ok || !strings.Contains(unwitnessed, ";大核簇治理上限记录为 1.85GHz(所选上限的窗内原因事件未见证)") {
		t.Fatalf("an unwitnessed ceiling must stay mechanism-neutral:\n%s", unwitnessed)
	}
	if strings.Contains(unwitnessed, "热控") || strings.Contains(unwitnessed, "运行于") {
		t.Fatalf("an unwitnessed ceiling proves neither a thermal source nor actual running frequency:\n%s", unwitnessed)
	}
	unwitnessedEN, _, _ := runtimeTraceProjSupplyFoldClause(node, 0, false, false)
	if !strings.Contains(unwitnessedEN, "; this big-core cluster has a 1.85GHz governance ceiling (no in-window source event witnessed for the selected ceiling)") {
		t.Fatalf("EN unwitnessed form missing:\n%s", unwitnessedEN)
	}
	// A witnessed generic cpufreq policy limit remains policy evidence. It
	// must not borrow the thermal word and does not claim the ceiling bound
	// actual performance by itself.
	node.GovernanceCapMechanism = tracequery.SupplyFoldGovernanceCapPolicyLimit
	node.GovernanceCapWitnessed = true
	policy, _, _ := runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if !strings.Contains(policy, ";窗内大核簇策略频率上限为 1.85GHz(不是热控轨证据,不单独证明热机制或实际绑定影响)") || strings.Contains(policy, "明确热控轨") {
		t.Fatalf("policy limit must stay policy-only:\n%s", policy)
	}
	// 双向: no typed press, no sentence — byte-identical to the pre-THERM form.
	node.GovernanceCapKHz = 0
	bare, _, _ := runtimeTraceProjSupplyFoldClause(node, 0, false, true)
	if strings.Contains(bare, "热限") || strings.Contains(bare, "限压") {
		t.Fatalf("no typed press must render no THERM words:\n%s", bare)
	}
	// The sentence rides the affirmative branch too (any fold branch).
	affirmative := capClauseNode(0, 2.641, 2.641, 0, 0, runtimeTraceCapabilitySourceDefault)
	affirmative.GovernanceCapKHz = 1550000
	affirmative.GovernanceCapClusterClass = "prime"
	affirmative.GovernanceCapMechanism = tracequery.SupplyFoldGovernanceCapThermalRail
	affirmative.GovernanceCapWitnessed = true
	sentence, _, ok := runtimeTraceProjSupplyFoldClause(affirmative, 0, false, true)
	if !ok || !strings.Contains(sentence, ";窗内超大核簇明确热控轨上限为 1.55GHz(不是 cpufreq 策略上限,不单独证明实际绑定影响)") {
		t.Fatalf("the THERM sentence must ride every fold branch:\n%s", sentence)
	}
}

func TestCAP2GovernanceCapLaneIdentitySeparatesNearbyMechanisms(t *testing.T) {
	thermal := capClauseNode(5, 15, 20, 0, 5, runtimeTraceCapabilitySourceDefault)
	thermal.GovernanceCapKHz = 2340000
	thermal.GovernanceCapClusterClass = "prime"
	thermal.GovernanceCapMechanism = tracequery.SupplyFoldGovernanceCapThermalRail
	thermal.GovernanceCapWitnessed = true
	thermalClause, _, ok := runtimeTraceProjSupplyFoldClause(thermal, 0, false, true)
	if !ok || !strings.Contains(thermalClause, "超大核簇明确热控轨上限为 2.34GHz") ||
		!strings.Contains(thermalClause, "不是 cpufreq 策略上限") {
		t.Fatalf("the exact cluster lane and thermal mechanism boundary must travel with 2.34GHz:\n%s", thermalClause)
	}

	policy := thermal
	policy.GovernanceCapKHz = 2100000
	policy.GovernanceCapClusterClass = "middle"
	policy.GovernanceCapMechanism = tracequery.SupplyFoldGovernanceCapPolicyLimit
	policyClause, _, ok := runtimeTraceProjSupplyFoldClause(policy, 0, false, true)
	if !ok || !strings.Contains(policyClause, "中核簇策略频率上限为 2.10GHz") ||
		!strings.Contains(policyClause, "不是热控轨证据") {
		t.Fatalf("the exact cluster lane and policy mechanism boundary must travel with 2.10GHz:\n%s", policyClause)
	}
	if strings.Contains(policyClause, "2.34GHz") || strings.Contains(thermalClause, "中核簇策略频率上限") {
		t.Fatalf("nearby cluster values and mechanisms must remain disjoint:\nthermal=%s\npolicy=%s", thermalClause, policyClause)
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
	projection.OnChainCauses[0].GovernanceCapKHz = 1850000
	projection.OnChainCauses[0].GovernanceCapMechanism = tracequery.SupplyFoldGovernanceCapThermalRail
	projection.OnChainCauses[0].GovernanceCapWitnessed = true
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
		ClusterTopologySource:     tracequery.CoreCapabilityTopologyKeyedRail,
		RailFamily:                "m3_c#_freq",
		RailGoverned:              []tracequery.SupplyFoldRailGoverned{{CPU: 12, Rail: "m3_c3_freq"}, {CPU: 13, Rail: "m3_c3_freq"}},
		GovernanceCapKHz:          1850000,
		GovernanceCapClusterClass: "prime",
		GovernanceCapMechanism:    tracequery.SupplyFoldGovernanceCapThermalRail,
		GovernanceCapWitnessed:    true,
	}
	joined := strings.Join(traceQueryTypedSupplyFoldRichNotes(basis, 1, 4), "\n")
	for _, want := range []string{
		types.TraceNoteKeyFoldClusterTopology + "=keyed_rail",
		types.TraceNoteKeyFoldRailBasis + "=族=m3_c#_freq;cpu12 频点=簇轨 m3_c3_freq;cpu13 频点=簇轨 m3_c3_freq",
		types.TraceNoteKeyGovernanceCapKHz + "=1850000",
		types.TraceNoteKeyGovernanceCapClusterClass + "=prime",
		types.TraceNoteKeyGovernanceCapMechanism + "=thermal_rail",
		types.TraceNoteKeyGovernanceCapWitnessed + "=true",
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
		types.TraceNoteKeyFoldClusterTopology, types.TraceNoteKeyFoldRailBasis, types.TraceNoteKeyGovernanceCapKHz,
	} {
		if strings.Contains(bare, absent) {
			t.Fatalf("empty fields must emit no %q note:\n%s", absent, bare)
		}
	}
}
