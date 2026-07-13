package tool

// §21.1 CWD-2 display/tool pins (real_trace_campaign_20260705.md §21.1 立账 +
// §22 witness 详情, 2026-07-07):
//   W1 — E19 树行 %-面 (huadong_01 lines 201-204/252/467): a merged ×N row
//        whose members span MULTIPLE query windows (×14 SUM 63.831ms across
//        two query windows) must not divide by the single anchor window
//        (101ms → "63%"). The bar and the ms cell stay; the % cell is
//        suppressed, the legend entry says why, and the lossless block lists
//        the member 窗来源. Same-window rosters keep the legacy share
//        byte-identically.
//   W2 — C7 产端半场 (cmp_01 lines 497-507): a window_stats-derived rank row
//        with a typed stats-window identity emits the row-level
//        selected_window note (same key/format as the result-level lane) when
//        the result envelope carries no window; the display side's
//        window-base lanes (projection QueryWindow identity + density
//        normalization) then engage automatically. No identity → no note.
//   W3 — symptom 分母车道 (CWD 复核留账②): the coverage sentence's
//        target-symptom denominator arms must not render a percentage when
//        the coverage-feeding rows carry POSITIVELY disagreeing typed query
//        windows (分子分母窗基不可证同基 → both magnitudes, no %, no
//        residual). Agreeing/windowless shapes keep the legacy wording
//        byte-identically.
//   W4 — chain 窗共识收紧 (CWD 复核留账③b): when data-bearing chain rows
//        exist, ≥1 of them must itself carry the window identity before the
//        coverage sentence names a "chain-data window" — a windowed SELF row
//        alone no longer establishes the claim.
//
// 对抗复核收尾 (SHIP-WITH-FIXES, 2026-07-07):
//   收尾① — the consensus core also consumes the MULTI-WINDOW MERGED roster
//        (huadong E1 witness: VSyncGenerator ×9 envelope straddling two query
//        windows): the aggregator zeroes such rows' row-level QueryWindow
//        identity, so without the roster arm the symptom %-face still divided
//        a cross-window merged numerator by a single-window denominator.
//   收尾② — the BarScale legend entry scopes multi-window merged bars to
//        relative scale, so it never contradicts the no-share entry.
//   收尾③ — the conclusion line's (占窗N%) share forks on the SAME typed key
//        (runtimeTraceProjMultiWindowMergedRow): a multi-window merged lead
//        publishes 单次最大 ×N without an anchor-window share.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// --- W1: E19 multi-window ×N sum row, %-face ---------------------------------

// cwd2E19Projection is the huadong_01 E19 verbatim shape: one on-chain sleep
// row, ×14 legal disjoint-window SUM 63.831ms whose members straddle two
// query windows, rendered inside a 101ms anchor window.
func cwd2E19Projection(windows int) types.TraceCausalProjection {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "E19",
		Subject: "oney.hmn.berlin-42591", Object: "sleep_wait", StateKind: "s_sleep",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		ImpactMS: 63.831, CumulativeImpactMS: 63.831,
		MergedCount: 14, MergedMinMS: 1.035, MergedMaxMS: 6.357,
		MergedEvidenceIDs: []string{"E19a", "E19b"},
		Confidence:        0.78,
	}
	if windows >= 1 {
		node.MergedQueryWindows = []types.TraceCausalProjectionQueryWindow{
			{StartTs: 6793222.700, EndTs: 6793222.801},
		}
	}
	if windows >= 2 {
		node.MergedQueryWindows = append(node.MergedQueryWindows,
			types.TraceCausalProjectionQueryWindow{StartTs: 6793224.895, EndTs: 6793224.996})
	}
	return types.TraceCausalProjection{
		WindowStartTs: 6793224.895,
		WindowEndTs:   6793224.996,
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}
}

// cwd2MultiWindowSumProjection is the representative multi-window SUM shape
// for the NEW-7 bidirectional legend harness (probe-compatible merged values:
// the MergedSum probe token is the verbatim 3次(10.000~30.000ms)).
func cwd2MultiWindowSumProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WindowStartTs: 100.000,
		WindowEndTs:   100.101,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "E1",
			Subject: "worker-7", Object: "runnable", StateKind: "runnable",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 60.000, CumulativeImpactMS: 60.000,
			MergedCount: 3, MergedMinMS: 10.000, MergedMaxMS: 30.000,
			MergedQueryWindows: []types.TraceCausalProjectionQueryWindow{
				{StartTs: 99.000, EndTs: 99.101},
				{StartTs: 100.000, EndTs: 100.101},
			},
			MergedEvidenceIDs: []string{"E2", "E3"},
			Confidence:        0.8,
		}},
	}
}

// TestCWD2E19MultiWindowSumSuppressesAnchorShare is the W1 E19 verbatim pin:
// the two-window ×14 SUM keeps its bar, ms cell, plain SUM form and 求和口径
// wording, drops the anchor-window "63%", raises the dedicated legend entry,
// and lists both member windows on the lossless 窗来源 lane. The same-window
// control keeps the legacy 63% byte-identically (突变自查: reverting the
// %-suppression branch reds the banned-63% assertion below).
func TestCWD2E19MultiWindowSumSuppressesAnchorShare(t *testing.T) {
	for _, zh := range []bool{true, false} {
		lang := "en"
		if zh {
			lang = "zh"
		}
		multi := buildRuntimeTraceProjTreeModel(cwd2E19Projection(2), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(multi, zh)
		if !strings.Contains(fence, "63.831ms") {
			t.Fatalf("zh=%v: the ms cell must survive:\n%s", zh, fence)
		}
		sumTok := "14次(1.035~6.357ms)"
		if !zh {
			sumTok = "n=14(1.035~6.357ms)"
		}
		if !strings.Contains(fence, sumTok) || strings.Contains(fence, ")union") || strings.Contains(fence, "跨窗取最大") || strings.Contains(fence, "cross-window max") {
			t.Fatalf("zh=%v: the row must keep the plain SUM form:\n%s", zh, fence)
		}
		if !strings.Contains(fence, "█") {
			t.Fatalf("zh=%v: the bar stays (relative scale):\n%s", zh, fence)
		}
		if strings.Contains(fence, "63%") {
			t.Fatalf("zh=%v: 绝不跨窗分子÷单锚窗分母打 %% — the anchor-window 63%% must not render:\n%s", zh, fence)
		}
		if !multi.Marks.has(runtimeTraceProjMarkMergedMultiWindowNoShare) {
			t.Fatalf("zh=%v: the no-share mark must record for the legend", zh)
		}
		// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 成员窗见无损块窗来源
		// → 成员窗来源见明细 (无损块→明细); ×14 求和口径 → 同一线程 14 次实例合并求和
		// (合并明细族).
		lead := runtimeTraceProjLeadText(cwd2E19Projection(2), multi, lang, zh)
		entry := "- 多窗合并行不显示占窗% = 该行成员横跨多个查询窗,合并值与单一锚定窗不同基,不作跨窗除法(时长条仅示意相对量级);成员窗来源见明细。"
		if !zh {
			entry = "- multi-window merged rows show no window share = the row's members span multiple query windows, so the merged value shares no base with the single anchor window and is never divided across bases (the bar is relative scale only); the member windows live in the detail blocks' window sources."
		}
		if !strings.Contains(lead, entry) {
			t.Fatalf("zh=%v: the no-share legend entry must render:\n%s", zh, lead)
		}
		lossless := runtimeTraceProjDetailFullText(multi, zh)
		if zh && !strings.Contains(lossless, "同一线程 14 次实例合并求和,单次 1.035~6.357ms") {
			t.Fatalf("the SUM caliber wording must stay (disjoint windows are a legal sum):\n%s", lossless)
		}
		if !strings.Contains(lossless, "6793222.700~6793222.801s") || !strings.Contains(lossless, "6793224.895~6793224.996s") {
			t.Fatalf("zh=%v: both member windows must be disclosed on the 窗来源 lane:\n%s", zh, lossless)
		}

		// 复核收尾② (W1-a 图例互斥): the BarScale entry itself scopes the
		// multi-window merged rows' bars to relative scale, so the two
		// co-rendered entries never read as contradictory.
		if zh && !strings.Contains(lead, "多窗合并行的时长条只作相对量级(见其专项条目)") {
			t.Fatalf("the BarScale entry must scope multi-window merged bars to relative scale:\n%s", lead)
		}

		// Same-window control: a single-window roster keeps the legacy share.
		single := buildRuntimeTraceProjTreeModel(cwd2E19Projection(1), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		sfence := runtimeTraceProjTreeFence(single, zh)
		if !strings.Contains(sfence, "63%") {
			t.Fatalf("zh=%v: the single-window control keeps the legacy 63%% share:\n%s", zh, sfence)
		}
		if single.Marks.has(runtimeTraceProjMarkMergedMultiWindowNoShare) {
			t.Fatalf("zh=%v: the single-window control must not record the no-share mark", zh)
		}
	}
}

// --- W2: window_stats rank row selected_window note ---------------------------

func cwd2SupplyRankItem(stamped bool) tracequery.RootCauseRankItem {
	item := tracequery.RootCauseRankItem{
		Rank: 1, Tier: "primary", Type: "supply_pressure",
		SubjectKind:        tracequery.RootCauseSubjectKindAggregateMetric,
		ImpactMs:           13821.784,
		CumulativeImpactMs: 13821.784,
		Confidence:         0.58,
		LineStart:          99303, LineEnd: 124384,
		Source:  "window_stats.supply_pressure_summary",
		Summary: "supply pressure summary",
	}
	if stamped {
		item.StatsWindowStartTs, item.StatsWindowEndTs = 3680.569, 3680.719
	}
	return item
}

func cwd2RankObservations(t *testing.T, window tracequery.TimeWindow, stamped bool) []types.ObservationRecord {
	t.Helper()
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: window,
			Items:  []tracequery.RootCauseRankItem{cwd2SupplyRankItem(stamped)},
		},
	}
	return traceQueryTypedObservations(result, "7315.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
}

func cwd2FindRankRecord(t *testing.T, records []types.ObservationRecord) types.ObservationRecord {
	t.Helper()
	for _, record := range records {
		if strings.HasPrefix(record.ClaimKey, "root_cause_") {
			return record
		}
	}
	t.Fatalf("no rank record emitted: %+v", records)
	return types.ObservationRecord{}
}

// TestCWD2WindowStatsRankRowEmitsRowLevelSelectedWindow is the W2 producer
// pin: a stamped window_stats rank row inside a window-less result envelope
// (the C7 shape) emits the selected_window note from its OWN identity — same
// key, same format as the result-level lane. Negative: an unstamped row in
// the same envelope emits NO selected_window note (absence never guesses).
// Byte-identity: when the result envelope carries a window, it wins verbatim
// (突变自查: dropping the row-level fallback reds the positive leg).
func TestCWD2WindowStatsRankRowEmitsRowLevelSelectedWindow(t *testing.T) {
	record := cwd2FindRankRecord(t, cwd2RankObservations(t, tracequery.TimeWindow{}, true))
	joined := strings.Join(record.RichNotes, "\n")
	if !strings.Contains(joined, "selected_window=3680.569000..3680.719000") {
		t.Fatalf("§21.1 CWD-2 ②: the stamped row must emit its own selected_window note:\n%s", joined)
	}

	bare := cwd2FindRankRecord(t, cwd2RankObservations(t, tracequery.TimeWindow{}, false))
	if strings.Contains(strings.Join(bare.RichNotes, "\n"), "selected_window=") {
		t.Fatalf("an identity-less row must emit NO selected_window note:\n%v", bare.RichNotes)
	}

	enveloped := cwd2FindRankRecord(t, cwd2RankObservations(t, tracequery.TimeWindow{StartTs: 1.0, EndTs: 2.0}, true))
	envelopedNotes := strings.Join(enveloped.RichNotes, "\n")
	if !strings.Contains(envelopedNotes, "selected_window=1.000000..2.000000") {
		t.Fatalf("the result-level window must win byte-identically:\n%s", envelopedNotes)
	}
	if strings.Contains(envelopedNotes, "3680.569000") {
		t.Fatalf("the row-level identity must not double-publish beside the result window:\n%s", envelopedNotes)
	}
}

// TestCWD2WindowStatsRankNoteCaughtByWindowBaseLanes is the W2 display-catch
// pin (显示端窗基车道自动接住): the emitted note compiles into the projection
// node's typed QueryWindow identity, and the density lane normalizes the
// cross-thread aggregate over ITS OWN window instead of the anchor window —
// the C7 damage (窗口1 行静默投影进窗口2 锚定树) becomes structurally
// impossible for stamped rows.
func TestCWD2WindowStatsRankNoteCaughtByWindowBaseLanes(t *testing.T) {
	records := cwd2RankObservations(t, tracequery.TimeWindow{}, true)
	projection := types.TraceCausalProjectionFromObservationRecords(records)
	var node *types.TraceCausalProjectionNode
	for _, lane := range [][]types.TraceCausalProjectionNode{
		projection.PrimaryRootCauses, projection.OnChainCauses,
		projection.AdjacentCauses, projection.BackgroundCauses, projection.SupportingHops,
	} {
		for i := range lane {
			if lane[i].Object == "supply_pressure" || lane[i].TypeToken == "supply_pressure" {
				node = &lane[i]
			}
		}
	}
	if node == nil {
		t.Fatalf("compiled projection must carry the supply_pressure node: %+v", projection)
	}
	if node.QueryWindowStartTs != 3680.569 || node.QueryWindowEndTs != 3680.719 {
		t.Fatalf("the selected_window note must compile into the node's QueryWindow identity: %+v", node)
	}
	// Density lane: the 150ms own window is the base, never the 101ms anchor.
	if got := runtimeTraceProjCrossThreadDensityWindowMS(*node, 101.0, true); got < 149.9 || got > 150.1 {
		t.Fatalf("density must normalize over the row's own 150ms window, got %f", got)
	}
}

// --- W3: symptom-denominator cross-base gate -----------------------------------

// cwd2SymptomProjection: 🎯 target app-1 publishes its own state-view symptom
// row (80ms sleep) and the depth-1 chain row attributes 40ms; the two rows'
// typed query windows are parameterized.
func cwd2SymptomProjection(chainW, selfW [2]float64) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"dep-2", "app-1"},
		WindowStartTs: 100.000,
		WindowEndTs:   100.101,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
				Subject: "dep-2", Object: "blocking_span",
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
				ImpactMS: 40.000, CumulativeImpactMS: 40.000,
				QueryWindowStartTs: chainW[0], QueryWindowEndTs: chainW[1],
				Confidence: 0.8,
			},
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-self",
				Subject: "app-1", Object: "sleep_wait", StateKind: "s_sleep",
				ImpactMS:           80.000,
				QueryWindowStartTs: selfW[0], QueryWindowEndTs: selfW[1],
				Confidence: 0.8,
			},
		},
	}
}

// TestCWD2SymptomDenominatorCrossBaseGate pins W3: positively disagreeing
// typed windows between the chain numerator and the target's symptom rows
// switch the coverage sentence to the no-percentage disclosure form; the
// agreeing-window and windowless controls keep the legacy symptom arithmetic
// byte-identically (突变自查: reverting the crossBase arm reds the banned-%
// assertion).
func TestCWD2SymptomDenominatorCrossBaseGate(t *testing.T) {
	crossed := cwd2SymptomProjection([2]float64{100.500, 100.652}, [2]float64{100.000, 100.101})
	model := buildRuntimeTraceProjTreeModel(crossed, nil, true)
	if runtimeTraceProjTargetSymptomMS(model) != 80.0 {
		t.Fatalf("fixture drifted: the target must publish an 80ms symptom denominator")
	}
	line := runtimeTraceProjWindowLine(crossed, model, true)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 目标等待 → 关注线程等待
	// (其他族); on-chain 归因口径合计 → 各链上口径合计 (归因族); 不计未归因残差 →
	// 不计未归因 (归因族); on-chain 已归因 → 链上已归因 (归因族); 「。 」粘连句 →
	// 每句独立 "\n- " bullet (layout).
	// COV 批 (§24.11 C-3, 2026-07-08). EVOLUTION RECORD: 各链上口径合计 →
	// 链上单项最大 (名实对齐: the numerator is a single-row MAX, 墙钟不可求和).
	// This fixture's target published its FULL wait as the denominator (no
	// excluded wait-view self rows) — the C-3 form-switch stays silent and the
	// legacy crossBase form is the pinned shape (负向 pin for the census gate).
	want := "\n- 关注线程等待(sleep/D-state/runnable) 80.000ms;链上单项最大 40.000ms — 链上/自身数据横跨多个查询窗,分子分母窗基不可证同基:不给出覆盖百分比,不计未归因。"
	if !strings.Contains(line, want) {
		t.Fatalf("cross-base symptom coverage must publish both magnitudes without %%:\n%s", line)
	}
	for _, banned := range []string{"(50%)", "未归因 40.000ms"} {
		if strings.Contains(line, banned) {
			t.Fatalf("cross-base symptom coverage must not render the percentage/residual (%q):\n%s", banned, line)
		}
	}
	en := runtimeTraceProjWindowLine(crossed, buildRuntimeTraceProjTreeModel(crossed, nil, false), false)
	if !strings.Contains(en, "the chain/self data spans multiple query windows and the numerator/denominator window bases cannot be proven identical: no coverage percentage, no unattributed residual.") {
		t.Fatalf("EN cross-base symptom disclosure missing:\n%s", en)
	}

	legacy := "\n- 关注线程等待(sleep/D-state/runnable) 80.000ms 中链上已归因 40.000ms(50%),未归因 40.000ms(50%)。"
	// Agreeing-window control: both rows measured in ONE window (even a
	// non-anchor one) prove the same base — the legacy arithmetic stays.
	same := cwd2SymptomProjection([2]float64{100.500, 100.652}, [2]float64{100.500, 100.652})
	sameLine := runtimeTraceProjWindowLine(same, buildRuntimeTraceProjTreeModel(same, nil, true), true)
	if !strings.Contains(sameLine, legacy) {
		t.Fatalf("agreeing-window symptom coverage must stay byte-identical:\n%s", sameLine)
	}
	// Windowless control (fail-open): absence of identity never fires the gate.
	bare := cwd2SymptomProjection([2]float64{0, 0}, [2]float64{0, 0})
	bareLine := runtimeTraceProjWindowLine(bare, buildRuntimeTraceProjTreeModel(bare, nil, true), true)
	if !strings.Contains(bareLine, legacy) {
		t.Fatalf("windowless symptom coverage must stay byte-identical (fail-open):\n%s", bareLine)
	}
}

// --- W4: chain window consensus requires chain attestation ----------------------

// cwd2HopOnlyProjection is the cmp_01 hop-only coverage shape with the window
// identities parameterized per lane: one depth-1 chain row (94.466ms) and one
// hop-view target sleep (115.902ms).
func cwd2HopOnlyProjection(chainW, hopW [2]float64) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"NetworkStats-9060", "app-1"},
		WindowStartTs: 3680.818,
		WindowEndTs:   3680.919,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
			Subject: "NetworkStats-9060", Object: "blocking_span",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 94.466, CumulativeImpactMS: 94.466,
			QueryWindowStartTs: chainW[0], QueryWindowEndTs: chainW[1],
			Confidence: 0.8,
		}},
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-self",
			Subject: "app-1", Object: "sleep_wait", StateKind: "s_sleep",
			ImpactMS:           115.902,
			QueryWindowStartTs: hopW[0], QueryWindowEndTs: hopW[1],
			Confidence: 0.8,
		}},
	}
}

// TestCWD2ChainWindowConsensusRequiresChainAttestation pins W4: with a
// window-LESS chain numerator row, a windowed self row alone must no longer
// establish the "链上数据来自查询窗 …" naming — the coverage keeps the legacy
// whole-window rendering (fail-open), while the hop-sleep magnitude still
// names its own per-row window base. The chain-attested control keeps the
// same-base naming lane engaged (突变自查: reverting the ≥1-windowed-chain-row
// predicate reds the banned-naming assertion).
func TestCWD2ChainWindowConsensusRequiresChainAttestation(t *testing.T) {
	foreign := [2]float64{3680.568, 3680.720}
	unattested := cwd2HopOnlyProjection([2]float64{0, 0}, foreign)
	model := buildRuntimeTraceProjTreeModel(unattested, nil, true)
	line := runtimeTraceProjWindowLine(unattested, model, true)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: on-chain 已归因 →
	// 链上已归因, 未归因残差 → 未归因 (归因族); 目标睡眠 → 关注线程睡眠 (其他族);
	// 非上句关注窗口 → 非上句分析窗 (窗族).
	if strings.Contains(line, "链上数据来自查询窗") {
		t.Fatalf("§21.1 CWD-2 ④: a self row alone must not name a chain-data window no chain row attested:\n%s", line)
	}
	if !strings.Contains(line, "链上已归因 94.466ms(94%),未归因 6.534ms(6%)。") {
		t.Fatalf("the unattested shape keeps the legacy whole-window coverage (fail-open):\n%s", line)
	}
	if !strings.Contains(line, "关注线程睡眠 115.902ms(取自查询窗 3680.568s → 3680.720s,非上句分析窗)中 94.466ms 已由链上解释。") {
		t.Fatalf("the hop-sleep magnitude still names its own per-row window base:\n%s", line)
	}

	// Chain-attested control: the naming lane engages exactly as before.
	attested := cwd2HopOnlyProjection(foreign, foreign)
	attestedLine := runtimeTraceProjWindowLine(attested, buildRuntimeTraceProjTreeModel(attested, nil, true), true)
	if !strings.Contains(attestedLine, "链上数据来自查询窗 3680.568s → 3680.720s") {
		t.Fatalf("a windowed chain row must keep the same-base naming lane engaged:\n%s", attestedLine)
	}
}

// --- 复核收尾①: multi-window merged numerator fires the symptom gate ------------

// cwd2MergedSymptomProjection is the huadong E1 constructed form: the depth-1
// chain numerator is a multi-window MERGED row (×14, roster parameterized;
// the aggregator zeroes the row-level QueryWindow identity for mixed rosters,
// so rowW is zero on the witness shape) beside a windowed target symptom row.
func cwd2MergedSymptomProjection(rosterWindows int, rowW, selfW [2]float64) types.TraceCausalProjection {
	chain := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
		Subject: "dep-2", Object: "runnable", StateKind: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		ImpactMS: 63.831, CumulativeImpactMS: 63.831,
		MergedCount: 14, MergedMinMS: 1.035, MergedMaxMS: 6.357,
		QueryWindowStartTs: rowW[0], QueryWindowEndTs: rowW[1],
		Confidence: 0.78,
	}
	if rosterWindows >= 1 {
		chain.MergedQueryWindows = []types.TraceCausalProjectionQueryWindow{
			{StartTs: 100.000, EndTs: 100.101},
		}
	}
	if rosterWindows >= 2 {
		chain.MergedQueryWindows = append(chain.MergedQueryWindows,
			types.TraceCausalProjectionQueryWindow{StartTs: 100.500, EndTs: 100.601})
	}
	return types.TraceCausalProjection{
		WakeupPath:    []string{"dep-2", "app-1"},
		WindowStartTs: 100.000,
		WindowEndTs:   100.101,
		OnChainCauses: []types.TraceCausalProjectionNode{
			chain,
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-self",
				Subject: "app-1", Object: "sleep_wait", StateKind: "s_sleep",
				ImpactMS:           80.000,
				QueryWindowStartTs: selfW[0], QueryWindowEndTs: selfW[1],
				Confidence: 0.8,
			},
		},
	}
}

// TestCWD2SymptomGateFiresOnMultiWindowMergedNumerator pins 复核收尾① (M3 残口,
// huadong E1 witness): a multi-window merged chain numerator whose ROW-level
// window identity was zeroed by the aggregator must still fire the W3 gate
// through its typed roster — the legacy "已归因 63.831ms(80%)" cross-window
// division is banned. The single-window merged control (roster=1, row identity
// kept by the aggregator) keeps the legacy symptom arithmetic byte-identically
// (不误伤). 突变自查: reverting the consensus roster arm reds the banned-%
// assertion.
func TestCWD2SymptomGateFiresOnMultiWindowMergedNumerator(t *testing.T) {
	crossed := cwd2MergedSymptomProjection(2, [2]float64{0, 0}, [2]float64{100.000, 100.101})
	model := buildRuntimeTraceProjTreeModel(crossed, nil, true)
	if runtimeTraceProjTargetSymptomMS(model) != 80.0 {
		t.Fatalf("fixture drifted: the target must publish an 80ms symptom denominator")
	}
	line := runtimeTraceProjWindowLine(crossed, model, true)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 目标等待 → 关注线程等待
	// (其他族); on-chain 归因口径合计 → 各链上口径合计, 不计未归因残差 → 不计未归因,
	// on-chain 已归因 → 链上已归因 (归因族).
	// COV 批 (§24.11 C-3, 2026-07-08). EVOLUTION RECORD: 各链上口径合计 →
	// 链上单项最大 (名实对齐,墙钟不可求和); no excluded wait-view self rows here
	// → the C-3 form-switch stays silent (负向 pin).
	if !strings.Contains(line, "\n- 关注线程等待(sleep/D-state/runnable) 80.000ms;链上单项最大 63.831ms — 链上/自身数据横跨多个查询窗,分子分母窗基不可证同基:不给出覆盖百分比,不计未归因。") {
		t.Fatalf("the multi-window merged numerator must fire the cross-base disclosure:\n%s", line)
	}
	for _, banned := range []string{"(80%)", "未归因 16.169ms"} {
		if strings.Contains(line, banned) {
			t.Fatalf("the cross-window merged numerator must not divide the symptom denominator (%q):\n%s", banned, line)
		}
	}

	// Single-window merged control: roster=1 and the row identity survives —
	// the legacy symptom arithmetic stays byte-identical.
	same := cwd2MergedSymptomProjection(1, [2]float64{100.000, 100.101}, [2]float64{100.000, 100.101})
	sameLine := runtimeTraceProjWindowLine(same, buildRuntimeTraceProjTreeModel(same, nil, true), true)
	if !strings.Contains(sameLine, "\n- 关注线程等待(sleep/D-state/runnable) 80.000ms 中链上已归因 63.831ms(80%),未归因 16.169ms(20%)。") {
		t.Fatalf("single-window merged numerator keeps the legacy symptom coverage:\n%s", sameLine)
	}
}

// --- 复核收尾③: conclusion-line window share on the multi-window merged lead ----

func cwd2MergedLeadProjection(rosterWindows int) types.TraceCausalProjection {
	node := types.TraceCausalProjectionNode{
		EvidenceID: "E3", Subject: "hmfs_discard-26-562", Object: "io_latency",
		Rank: 1, ImpactMS: 13.324, CumulativeImpactMS: 13.324,
		MergedCount: 7, MergedMinMS: 0.352, MergedMaxMS: 1.940,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
	}
	if rosterWindows >= 1 {
		node.MergedQueryWindows = []types.TraceCausalProjectionQueryWindow{
			{StartTs: 6793222.700, EndTs: 6793222.801},
		}
	}
	if rosterWindows >= 2 {
		node.MergedQueryWindows = append(node.MergedQueryWindows,
			types.TraceCausalProjectionQueryWindow{StartTs: 6793224.895, EndTs: 6793224.996})
	}
	return types.TraceCausalProjection{
		WindowStartTs:     6793222.700,
		WindowEndTs:       6793222.801,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{node},
	}
}

// TestCWD2ConclusionMultiWindowMergedLeadSuppressesWindowShare pins 复核收尾③
// (W1-b): the conclusion line's (占窗N%) forks on the SAME typed key as the
// tree-row %-face — a multi-window merged lead publishes 单次最大 ×N with NO
// anchor-window share; the single-window roster control keeps the legacy
// share byte-identically. 突变自查: reverting the conclusion gate reds the
// banned-(占窗 assertion.
func TestCWD2ConclusionMultiWindowMergedLeadSuppressesWindowShare(t *testing.T) {
	multi := cwd2MergedLeadProjection(2)
	line := runtimeTraceProjConclusionLine(multi, buildRuntimeTraceProjTreeModel(multi, nil, true), true)
	if !strings.Contains(line, "单次最大 1.940ms(共7次)") {
		t.Fatalf("the merged lead must keep the single-instance max form:\n%s", line)
	}
	if strings.Contains(line, "(占窗") {
		t.Fatalf("a multi-window merged lead must not publish an anchor-window share:\n%s", line)
	}
	en := runtimeTraceProjConclusionLine(multi, buildRuntimeTraceProjTreeModel(multi, nil, false), false)
	if !strings.Contains(en, "single max 1.940ms (of 7)") || strings.Contains(en, "% of window)") {
		t.Fatalf("EN multi-window merged lead must suppress the share:\n%s", en)
	}

	// Single-window roster control: the legacy share stays byte-identical.
	single := cwd2MergedLeadProjection(1)
	sline := runtimeTraceProjConclusionLine(single, buildRuntimeTraceProjTreeModel(single, nil, true), true)
	if !strings.Contains(sline, "单次最大 1.940ms(共7次)(占窗2%)") {
		t.Fatalf("the single-window merged lead keeps the legacy share:\n%s", sline)
	}
}
