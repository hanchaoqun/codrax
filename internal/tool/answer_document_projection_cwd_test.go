package tool

// §21 CWD display pins (real_trace_campaign_20260705.md §21, cmp_01 revisit
// audit 2026-07-07 — CWD 批: cross-window denominator 窗基对齐):
//   A — 排队深度方向反转: the comparison overview's 背景压力 cell must
//       normalize its numerator over the SAME window base it was measured in
//       (a ×N cross-window MAX row divides the max member by that member's
//       own query window, never the anchor window). Direction pin: on the
//       cmp_01 shape the rendered densities must order 7.0 > 6.0 exactly like
//       the tool-truth pressure densities — the shipped build displayed the
//       inverted order (336.7 vs 449.3 = cross-window sums ÷ one anchor
//       window).
//   B — 覆盖句窗基: the coverage sentence never divides a cross-window
//       numerator by the anchor-window denominator (the fabricated
//       "未归因残差 6.534ms" negative pin), and a target-sleep magnitude
//       larger than the printed window must switch disclosure form (precise
//       numeric comparison as the form gate).
//   C — E29 承自注: the inherited-attribution note names its window base
//       when a typed window identity exists.
//   D — N3 相位/锚质量: the comparison overview closes with the
//       anchor-quality asymmetry note when one side has on-chain attribution
//       and the other has none (typed signals only).

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// cwdCrossWindowMaxProjection is the representative §21-CWD shape consumed by
// the revisit76 bidirectional legend harness AND the every-surface pin below:
// one on-chain ×3 row whose members came from overlapping query windows with
// no occurrence spans — engine-published member MAX with the typed marker,
// the lossless raw Σ and the max member's own window base.
func cwdCrossWindowMaxProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WindowStartTs: 3680.800,
		WindowEndTs:   3680.901,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "E31",
			Subject: "binder:8815_1-6581", Object: "runnable", StateKind: "runnable",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 60.000, CumulativeImpactMS: 60.000,
			MergedCount: 3, MergedMinMS: 20.000, MergedMaxMS: 60.000,
			MergedCrossWindowMax: true, MergedSumMS: 110.000,
			MergedMaxWindowStartTs: 3680.569, MergedMaxWindowEndTs: 3680.719,
			MergedQueryWindows: []types.TraceCausalProjectionQueryWindow{
				{StartTs: 3680.569, EndTs: 3680.719},
				{StartTs: 3680.600, EndTs: 3680.750},
			},
			MergedEvidenceIDs: []string{"E32", "E33"},
			Confidence:        0.8,
		}},
	}
}

// TestCWDCrossWindowMaxRowWearsMaxFormOnEverySurface mirrors the §11-N2 union
// every-surface pin for the ×N 第五式: the cross-window MAX form token in the
// fence and the (a)-table name cell, its own legend entry (and the ABSENCE of
// the sum entry — a MAX value must never be glossed 数值为总和), and a
// lossless 合并明细 line naming the caliber, the raw Σ, the max member's own
// window base and the window sources.
func TestCWDCrossWindowMaxRowWearsMaxFormOnEverySurface(t *testing.T) {
	projection := cwdCrossWindowMaxProjection()
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		lang := "en"
		if zh {
			lang = "zh"
		}
		lead := runtimeTraceProjLeadText(projection, model, lang, zh)
		lossless := runtimeTraceProjDetailFullText(model, zh)

		token := "3次跨窗取最大(单项20.000~60.000ms)"
		if !zh {
			token = "n=3 cross-window max(each 20.000~60.000ms)"
		}
		if !strings.Contains(fence, token) {
			t.Fatalf("zh=%v: fence must carry the cross-window MAX form token:\n%s", zh, fence)
		}
		// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 重叠窗 →
		// 互相重叠的查询窗 (窗族); 见无损块 → 见明细 (无损块→明细); lossless block
		// → detail blocks (EN mirror); N次跨窗取最大口径(N 窗互相重叠,重叠窗量值不求和)
		// → ×N 跨窗取最大(N 个查询窗互相重叠,互相重叠的查询窗量值不可求和) (合并明细族).
		maxEntry := "- `N次跨窗取最大(单项a~b)` = N 次实例来自互相重叠的查询窗,互相重叠的查询窗量值不可求和且重叠段无法逐段核销,数值取成员最大(以该成员自身查询窗为基),a~b 为单次范围;原始和与窗来源见明细。"
		sumEntry := "- `N次(a~b)` = 同一(线程,原因)的 N 次实例合并,数值为总和,a~b 为单次范围。"
		if !zh {
			maxEntry = "- `n=N cross-window max(each a~b)` = the N instances come from OVERLAPPING query windows: overlapping-window magnitudes never sum and the overlap cannot be deducted per segment, so the value is the member MAX (normalized over that member's own query window), a~b the per-instance range; the raw sum and the window sources live in the detail blocks."
			sumEntry = "- `N次(a~b)` = N instances of one (thread, cause) merged; the value is the SUM, a~b the per-instance range."
		}
		if !strings.Contains(lead, maxEntry) {
			t.Fatalf("zh=%v: cross-window MAX legend entry missing:\n%s", zh, lead)
		}
		if strings.Contains(lead, sumEntry) {
			t.Fatalf("zh=%v: the SUM legend entry must NOT render for a MAX-only tree (口径谎言): %s", zh, lead)
		}
		wantDetail := "3次跨窗取最大(2 个查询窗互相重叠,互相重叠的查询窗量值不可求和),原始和 110.000ms 供对照,单次 20.000~60.000ms;最大成员窗基=查询窗 3680.569~3680.719s"
		wantWindows := "3680.569~3680.719s、3680.600~3680.750s"
		if !zh {
			wantDetail = "n=3 cross-window MAX caliber (2 overlapping windows; overlapping-window magnitudes never sum), raw sum 110.000ms for cross-checking, each 20.000~60.000ms; max-member window base = query window 3680.569~3680.719s"
			wantWindows = "3680.569~3680.719s, 3680.600~3680.750s"
		}
		if !strings.Contains(lossless, wantDetail) {
			t.Fatalf("zh=%v: lossless merge detail must state the MAX caliber + raw Σ + max-member window base:\n%s", zh, lossless)
		}
		if !strings.Contains(lossless, wantWindows) {
			t.Fatalf("zh=%v: lossless block must list the member window sources:\n%s", zh, lossless)
		}
		if zh && !strings.Contains(lossless, "窗来源") {
			t.Fatalf("lossless block must carry the 窗来源 lane:\n%s", lossless)
		}
		_, rows := runtimeTraceProjDetailTable(model, zh)
		tokenSeen := false
		for _, row := range rows {
			if len(row.Cells) > 0 && strings.Contains(row.Cells[0], token) {
				tokenSeen = true
			}
		}
		if !tokenSeen {
			t.Fatalf("zh=%v: detail-table name cell must carry the MAX form token: %+v", zh, rows)
		}
		flags := runtimeTraceProjDetailTableLegendFlagsFor(model, zh)
		if !flags.mergedWindowMax || flags.mergedSum || flags.mergedUnion {
			t.Fatalf("zh=%v: MAX row must raise mergedWindowMax and never mergedSum/mergedUnion: %+v", zh, flags)
		}
	}
}

// --- A: 排队深度 direction pin (full pipeline, cmp_01 shape) --------------------

// cwdCompareObs builds the cmp_01 two-artifact ledger: per artifact three
// supply_pressure rank observations (no occurrence Span ts) measured over
// OVERLAPPING query windows, plus a rank=1 wall-clock row whose
// selected_window (LAST in partition order) anchors that artifact's
// projection window — exactly the specimen's "×N cross-window sum ÷ anchor
// window" production chain.
func cwdCompareObs() []types.ObservationRecord {
	const artifactA = "7.0B30SP22_7315.systrace"
	const artifactB = "6.0B138_3900.sys.systrace"
	supply := func(id, artifact, value string, impact float64, lineStart, lineEnd int, window string) types.ObservationRecord {
		return compareProjObs(id, artifact, "root_cause_primary", "root_cause_primary:"+id,
			"unknown-thread", "supply_pressure", value, impact, lineStart, lineEnd,
			types.ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
			"rank=2", "tier=secondary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"subject_kind=aggregate_metric", "type=supply_pressure",
			"selected_window="+window)
	}
	return []types.ObservationRecord{
		// Artifact A (7.0): windows 100/150/101ms; the first two OVERLAP.
		supply("a-sup-1", artifactA, "5563.967", 5563.967, 137694, 154571, "3680.568000..3680.668000"),
		supply("a-sup-2", artifactA, "13821.784", 13821.784, 99303, 124384, "3680.569000..3680.719000"),
		supply("a-sup-3", artifactA, "5800.766", 5800.766, 140577, 157305, "3680.818000..3680.919000"),
		compareProjObs("a-run", artifactA, "root_cause_primary", "root_cause_primary:a-run",
			"NetworkStats-9060", "running", "47.562", 47.562, 104007, 113223,
			types.ObservationSpan{LineStart: 104007, LineEnd: 113223, StartTs: 3680.820, EndTs: 3680.868},
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"dominant_state=running", "selected_window=3680.818000..3680.919000"),
		// Artifact B (6.0): 884ms window containing two 150ms windows.
		supply("b-sup-1", artifactB, "28914.394", 28914.394, 101795, 231908, "8144.358000..8145.242000"),
		supply("b-sup-2", artifactB, "8203.039", 8203.039, 101888, 124141, "8144.358000..8144.508000"),
		supply("b-sup-3", artifactB, "8258.251", 8258.251, 101795, 124203, "8144.458000..8144.608000"),
		compareProjObs("b-run", artifactB, "root_cause_primary", "root_cause_primary:b-run",
			"sysevent_store-1870", "running", "23.100", 23.100, 101900, 102400,
			types.ObservationSpan{LineStart: 101900, LineEnd: 102400, StartTs: 8144.620, EndTs: 8144.644},
			"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
			"dominant_state=running", "selected_window=8144.608000..8144.708000"),
	}
}

// TestCWDQueueDepthDirectionMatchesToolTruth is the §21-CWD direction pin:
// on the cmp_01 dual-artifact overlapping-window fixture the comparison
// overview's two 平均排队深度 densities must (a) each divide the member MAX
// by that member's OWN query window (92.1 = 13821.784/150ms and 32.7 =
// 28914.394/884ms — the same-base readings that match the tool's own
// pressure_density direction 7.0 > 6.0), and (b) order A > B. The shipped
// build divided cross-window SUMS by each side's anchor window (249.4 vs
// 453.8 here; 336.7 vs 449.3 on the customer specimen) and INVERTED the
// flagship comparison's conclusion — reverting the same-base normalization
// must red on the direction assertion, not merely on a wording diff.
func TestCWDQueueDepthDirectionMatchesToolTruth(t *testing.T) {
	bus := compareProjBus(true)
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: cwdCompareObs(),
	}}
	got := compareProjApply(t, bus)
	compare := projectionClusterBlock(got.Blocks, "runtime_trace_causal_projection_compare")
	if compare == nil || compare.Kind != types.BlockTable || len(compare.Items) < 2 {
		t.Fatalf("comparison overview table missing: %+v", got.Blocks)
	}
	rowA := strings.Join(compare.Items[0].Cells, " | ")
	rowB := strings.Join(compare.Items[1].Cells, " | ")
	if !strings.Contains(rowA, "≈平均排队深度 92.1(跨窗取最大 13821.784ms,跨线程累计,非墙钟)") {
		t.Fatalf("row A must normalize the MAX member over its OWN 150ms query window:\n%s", rowA)
	}
	if !strings.Contains(rowB, "≈平均排队深度 32.7(跨窗取最大 28914.394ms,跨线程累计,非墙钟)") {
		t.Fatalf("row B must normalize the MAX member over its OWN 884ms query window:\n%s", rowB)
	}
	// Direction assertion against tool truth (7.0 > 6.0): parsed, not string-
	// matched, so ANY regression that flips the order reds here.
	density := regexp.MustCompile(`≈平均排队深度 ([0-9.]+)`)
	parse := func(row string) float64 {
		m := density.FindStringSubmatch(row)
		if m == nil {
			t.Fatalf("no queue-depth density on row:\n%s", row)
		}
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			t.Fatalf("unparseable density %q: %v", m[1], err)
		}
		return v
	}
	if dA, dB := parse(rowA), parse(rowB); !(dA > dB) {
		t.Fatalf("direction inverted: displayed queue depth A=%.1f must exceed B=%.1f (tool truth: 7.0 > 6.0)", dA, dB)
	}
	// Negative guard: the specimen's cross-window sums must not surface as the
	// cell magnitude (25186.517 = A's Σ; 45375.684 = B's Σ, the exact specimen
	// figure) and the anchor-window densities must be gone.
	for _, banned := range []string{"25186.517ms", "45375.684ms", "≈平均排队深度 249", "≈平均排队深度 453"} {
		if strings.Contains(rowA, banned) || strings.Contains(rowB, banned) {
			t.Fatalf("cross-window sum ÷ anchor window resurfaced (%q):\nA: %s\nB: %s", banned, rowA, rowB)
		}
	}
	// §21 CWD 复核 F3-note fix: bases here are the MAX members' own query
	// windows (150ms / 884ms), NOT the projection windows — the closing note
	// must name the real normalization bases and must not claim the projection
	// windows themselves differ (that sentence stays reserved for the legacy
	// shape where every density base IS its side's projection window).
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: ⚠ notes moved out
	// of table rows into block Text lines (layout-L1); 两侧投影窗长不等 →
	// 两侧分析窗长度不等 (窗族: 投影窗→分析窗).
	noteText := compare.Text
	if !strings.Contains(noteText, "⚠ 背景压力已按各自数值所在窗归一化(窗基: 150.000ms / 884.000ms)") {
		t.Fatalf("F3 note must name the real normalization bases:\n%s", noteText)
	}
	if strings.Contains(noteText, "两侧分析窗长度不等") {
		t.Fatalf("F3 note must not claim projection windows differ when bases are member windows:\n%s", noteText)
	}
	// The note lines live in the block text, never back in the grid rows.
	for _, item := range compare.Items {
		if strings.Contains(strings.Join(item.Cells, " "), "⚠ 背景压力已按") {
			t.Fatalf("the F3 note must not ride a table row anymore: %+v", item.Cells)
		}
	}
}

// --- A: density denominator unit pins -------------------------------------------

func TestCWDDensityWindowSameBase(t *testing.T) {
	close := func(a, b float64) bool { d := a - b; return d > -0.0005 && d < 0.0005 }
	// (1) A cross-window MAX row divides by the MAX member's own query window.
	maxRow := types.TraceCausalProjectionNode{
		MergedCount: 3, MergedCrossWindowMax: true,
		MergedMaxWindowStartTs: 3680.569, MergedMaxWindowEndTs: 3680.719,
	}
	if got := runtimeTraceProjCrossThreadDensityWindowMS(maxRow, 101.0, true); !close(got, 150.0) {
		t.Fatalf("MAX row must normalize over the max member's own window (150ms): %f", got)
	}
	// (2) A MAX row without a recorded member window renders NO density —
	// never the anchor window (a cross-base estimate).
	if got := runtimeTraceProjCrossThreadDensityWindowMS(types.TraceCausalProjectionNode{
		MergedCount: 3, MergedCrossWindowMax: true,
	}, 101.0, true); got != 0 {
		t.Fatalf("MAX row without a window base must render no density: %f", got)
	}
	// (3) A row measured over its OWN typed query window divides by that
	// window, not the anchor.
	if got := runtimeTraceProjCrossThreadDensityWindowMS(types.TraceCausalProjectionNode{
		QueryWindowStartTs: 8144.358, QueryWindowEndTs: 8145.242,
	}, 101.0, true); !close(got, 884.0) {
		t.Fatalf("a row with its own query window must normalize over it (884ms): %f", got)
	}
	// (4) A merged multi-window row without a resolvable base renders NO
	// density (the anchor window is not the base of a cross-window numerator).
	if got := runtimeTraceProjCrossThreadDensityWindowMS(types.TraceCausalProjectionNode{
		MergedCount: 3,
		MergedQueryWindows: []types.TraceCausalProjectionQueryWindow{
			{StartTs: 1.0, EndTs: 1.1}, {StartTs: 2.0, EndTs: 2.1},
		},
	}, 101.0, true); got != 0 {
		t.Fatalf("multi-window merged row must not divide by the anchor window: %f", got)
	}
	// (5) Windowless single rows keep the anchor fallback byte-identically.
	if got := runtimeTraceProjCrossThreadDensityWindowMS(types.TraceCausalProjectionNode{}, 101.0, true); !close(got, 101.0) {
		t.Fatalf("windowless row keeps the legacy anchor fallback: %f", got)
	}
	// (6) A non-merged node's own occurrence span still wins over its query
	// window (the legacy first lane, byte-preserved).
	if got := runtimeTraceProjCrossThreadDensityWindowMS(types.TraceCausalProjectionNode{
		StartTs: 10.0, EndTs: 10.2, QueryWindowStartTs: 9.0, QueryWindowEndTs: 11.0,
	}, 101.0, true); !close(got, 200.0) {
		t.Fatalf("own occurrence span must stay the first denominator lane: %f", got)
	}
	// (7) A MAX row that inherited a member-impact ENVELOPE span still
	// normalizes over the MAX member's own window — the envelope is a
	// multi-member span, not the base the single MAX member was measured
	// over (cross-base mix banned).
	if got := runtimeTraceProjCrossThreadDensityWindowMS(types.TraceCausalProjectionNode{
		MergedCount: 3, MergedCrossWindowMax: true,
		StartTs: 1.000, EndTs: 1.240,
		MergedMaxWindowStartTs: 0.900, MergedMaxWindowEndTs: 1.150,
	}, 101.0, true); !close(got, 250.0) {
		t.Fatalf("MAX row must prefer the max member's window over the member envelope: %f", got)
	}
}

// --- B: coverage sentence window base --------------------------------------------

// cwdCoverageProjection reproduces the cmp_01 7.0 coverage shape: anchor
// window 3680.818–3680.919 (101ms), every chain/self row measured in query
// window 3680.568–3680.720 (152ms), hop-only target sleep 115.902ms.
func cwdCoverageProjection(chainWindowStart, chainWindowEnd float64) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"NetworkStats-9060", "app-1"},
		WindowStartTs: 3680.818,
		WindowEndTs:   3680.919,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
			Subject: "NetworkStats-9060", Object: "blocking_span",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 94.466, CumulativeImpactMS: 94.466,
			QueryWindowStartTs: chainWindowStart, QueryWindowEndTs: chainWindowEnd,
			Confidence: 0.8,
		}},
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-self",
			Subject: "app-1", Object: "sleep_wait", StateKind: "s_sleep",
			ImpactMS:           115.902,
			QueryWindowStartTs: chainWindowStart, QueryWindowEndTs: chainWindowEnd,
			Confidence: 0.8,
		}},
	}
}

// TestCWDCoverageSentenceCrossWindowBase pins the §21-CWD repeat-P0 fix: a
// cross-window chain must not be divided by the anchor window. Negative pin:
// the fabricated "未归因残差 6.534ms" (= 101.000 − 94.466 in a window the
// chain never measured) is banned; the denominator and its label switch to
// the chain-data window, and the >window target sleep names its base.
func TestCWDCoverageSentenceCrossWindowBase(t *testing.T) {
	projection := cwdCoverageProjection(3680.568, 3680.720)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if runtimeTraceProjTargetSymptomMS(model) != 0 {
		t.Fatalf("fixture drifted: expected a hop-only self lane")
	}
	line := runtimeTraceProjWindowLine(projection, model, true)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: on-chain 已归因 →
	// 链上已归因, 未归因残差 → 未归因 (归因族); 目标睡眠 → 关注线程睡眠 (其他族);
	// 该链数据窗 → 该查询窗, 非上句关注窗口 → 非上句分析窗 (窗族). Negative pins
	// keep the old-form guards and add the new-form equivalents.
	// The naked anchor-window division and its fabricated residual are banned.
	for _, banned := range []string{
		"未归因残差 6.534ms", "未归因 6.534ms", "94.466ms(94%)",
		"目标睡眠 115.902ms 中", "关注线程睡眠 115.902ms 中",
	} {
		if strings.Contains(line, banned) {
			t.Fatalf("cross-window numerator divided by the anchor window (%q):\n%s", banned, line)
		}
	}
	// Same-base rendering: denominator = the chain-data window, named.
	if !strings.Contains(line, "链上已归因 94.466ms(62%),未归因 57.534ms(38%)(口径:链上数据来自查询窗 3680.568s → 3680.720s 共 152.000ms,分母取该查询窗,非上句分析窗;两窗基不可混除)。") {
		t.Fatalf("coverage must divide over the chain-data window with the base named:\n%s", line)
	}
	// The contradiction hard gate: target sleep 115.902 > 窗口 101.000 must
	// carry its window base instead of the naked legacy form.
	if !strings.Contains(line, "关注线程睡眠 115.902ms(取自查询窗 3680.568s → 3680.720s,非上句分析窗)中 94.466ms 已由链上解释。") {
		t.Fatalf("the >window target sleep must name its window base:\n%s", line)
	}

	en := runtimeTraceProjWindowLine(projection, model, false)
	if !strings.Contains(en, "the denominator is that window, not the analysis window above") ||
		!strings.Contains(en, "not the analysis window above), 94.466ms is explained on-chain.") {
		t.Fatalf("EN same-base coverage rendering missing:\n%s", en)
	}

	// Control (byte-stability): the SAME shape measured in the anchor window
	// itself keeps the legacy whole-window rendering, with the >window sleep
	// taking the neutral beyond-the-window clause (its base IS the anchor
	// window — the 非上句关注窗口 claim must never render for it).
	same := cwdCoverageProjection(3680.818, 3680.919)
	sameModel := buildRuntimeTraceProjTreeModel(same, nil, true)
	sameLine := runtimeTraceProjWindowLine(same, sameModel, true)
	if !strings.Contains(sameLine, "链上已归因 94.466ms(94%),未归因 6.534ms(6%)。") {
		t.Fatalf("same-window chains keep the legacy whole-window coverage byte-identically:\n%s", sameLine)
	}
	if !strings.Contains(sameLine, "关注线程睡眠 115.902ms(该状态时长超出上句分析窗)中 94.466ms 已由链上解释。") {
		t.Fatalf("same-window >window sleep takes the neutral beyond-the-window clause:\n%s", sameLine)
	}
	if strings.Contains(sameLine, "非上句分析窗") {
		t.Fatalf("same-window shape must never claim 非上句分析窗:\n%s", sameLine)
	}

	// Control (fail-open): windowless rows keep the legacy rendering exactly —
	// absence of identity is never treated as a mismatch.
	bare := cwdCoverageProjection(0, 0)
	bareModel := buildRuntimeTraceProjTreeModel(bare, nil, true)
	bareLine := runtimeTraceProjWindowLine(bare, bareModel, true)
	if !strings.Contains(bareLine, "链上已归因 94.466ms(94%),未归因 6.534ms(6%)。") {
		t.Fatalf("windowless chains keep the legacy whole-window coverage (fail-open):\n%s", bareLine)
	}
	if !strings.Contains(bareLine, "关注线程睡眠 115.902ms(该状态时长超出上句分析窗)中 94.466ms 已由链上解释。") {
		t.Fatalf("windowless >window sleep still switches the disclosure form (numeric gate):\n%s", bareLine)
	}
}

// TestCWDCoverageLegacyShapeBytePreserved pins the untouched lane: a hop-only
// shape whose sleep fits INSIDE the window renders the legacy sentences
// byte-identically (the PTV5 Q2 pin's exact wording).
func TestCWDCoverageLegacyShapeBytePreserved(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"dep-2", "app-1"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
			Subject: "dep-2", Object: "sleep_wait", StateKind: "s_sleep",
			ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 40.0, CumulativeImpactMS: 40.0, Confidence: 0.8,
		}},
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-self",
			Subject: "app-1", Object: "sleep_wait", StateKind: "s_sleep",
			ImpactMS: 120.0, Confidence: 0.8,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: on-chain 已归因 →
	// 链上已归因, 未归因残差 → 未归因 (归因族); 目标睡眠 → 关注线程睡眠 (其他族).
	// The pin's intent (byte-verbatim shape) stays — bytes migrated to the new
	// canonical wording.
	if !strings.Contains(line, "链上已归因 40.000ms(20%),未归因 160.000ms(80%)。") ||
		!strings.Contains(line, "关注线程睡眠 120.000ms 中 40.000ms 已由链上解释。") {
		t.Fatalf("in-window hop-only shape must stay byte-identical:\n%s", line)
	}
}

// --- C: 承自注 window base --------------------------------------------------------

func TestCWDInheritedNoteWindowBase(t *testing.T) {
	// (1) A row carrying its own typed query window names it.
	own := types.TraceCausalProjectionNode{
		QueryWindowStartTs: 3680.569, QueryWindowEndTs: 3680.719,
	}
	if got := runtimeTraceProjInheritedWindowBaseSuffix(own, true); got != ";窗基=查询窗 3680.569~3680.719s" {
		t.Fatalf("zh own-window suffix wrong: %q", got)
	}
	if got := runtimeTraceProjInheritedWindowBaseSuffix(own, false); got != "; window base = query window 3680.569~3680.719s" {
		t.Fatalf("en own-window suffix wrong: %q", got)
	}
	// (2) A merged row without a single window names the member roster.
	merged := types.TraceCausalProjectionNode{
		MergedCount: 3,
		MergedQueryWindows: []types.TraceCausalProjectionQueryWindow{
			{StartTs: 3680.569, EndTs: 3680.719}, {StartTs: 3680.818, EndTs: 3680.919},
		},
	}
	if got := runtimeTraceProjInheritedWindowBaseSuffix(merged, true); got != ";窗基=成员查询窗 3680.569~3680.719s、3680.818~3680.919s(非本行渲染窗)" {
		t.Fatalf("zh roster suffix wrong: %q", got)
	}
	// (3) No typed window → no label, never a guess.
	if got := runtimeTraceProjInheritedWindowBaseSuffix(types.TraceCausalProjectionNode{}, true); got != "" {
		t.Fatalf("windowless node must add no suffix: %q", got)
	}

	// Integration: the E29 shape (inherited effective ≫ own cumulative) renders
	// the 承自注 with the window base in the lossless block.
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 3680.818,
		WindowEndTs:   3680.919,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E29",
			Subject: "worker-9", Object: "cpu_pressure",
			ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 52.850, CumulativeImpactMS: 158.550, EffectiveImpactMS: 2994.269,
			QueryWindowStartTs: 3680.569, QueryWindowEndTs: 3680.719,
			Confidence: 0.8,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	lossless := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(lossless, "有效归因 2994.269ms 承自等待区间,非本行实测;窗基=查询窗 3680.569~3680.719s") {
		t.Fatalf("承自注 must name the donor window base:\n%s", lossless)
	}
}

// --- D: anchor-quality asymmetry note ---------------------------------------------

func TestCWDCompareChainAsymmetryNote(t *testing.T) {
	labels := []string{"7.0B30SP22_7315.systrace", "6.0B138_3900.sys.systrace"}
	// Asymmetric: one side attributed, one side chainless with the typed flag.
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 锚窗质量不对称 →
	// 两侧分析窗证据深度不对称, 锚窗内 → 的分析窗内 (窗族: 锚窗→分析窗);
	// on-chain 归因/已归因 → 链上归因/链上已归因 (归因族); 行际对读 → 逐行对比.
	note := runtimeTraceProjCompareChainAsymmetryNote(labels, []float64{94.466, 0}, []bool{false, true}, true)
	want := "⚠ 两侧分析窗证据深度不对称:6.0B138_3900.sys.systrace(该侧唤醒链下钻未执行) 的分析窗内无链上归因;7.0B30SP22_7315.systrace 的分析窗内链上已归因 94.466ms。主根因/链上已归因两列不可直接逐行对比"
	if note != want {
		t.Fatalf("zh asymmetry note wrong:\n got %q\nwant %q", note, want)
	}
	en := runtimeTraceProjCompareChainAsymmetryNote(labels, []float64{94.466, 0}, []bool{false, true}, false)
	if !strings.Contains(en, "Anchor-window quality is asymmetric") ||
		!strings.Contains(en, "its wakeup-chain drilldown did not run") ||
		!strings.Contains(en, "94.466ms") {
		t.Fatalf("en asymmetry note wrong: %q", en)
	}
	// Without the typed not-run flag the clause stays out (never guessed).
	note = runtimeTraceProjCompareChainAsymmetryNote(labels, []float64{94.466, 0}, []bool{false, false}, true)
	if strings.Contains(note, "唤醒链下钻未执行") {
		t.Fatalf("the not-run clause requires the typed flag: %q", note)
	}
	// Symmetric shapes emit NOTHING (both attributed / both chainless).
	if got := runtimeTraceProjCompareChainAsymmetryNote(labels, []float64{94.466, 40.0}, []bool{false, false}, true); got != "" {
		t.Fatalf("both-attributed must emit no note: %q", got)
	}
	if got := runtimeTraceProjCompareChainAsymmetryNote(labels, []float64{0, 0}, []bool{true, true}, true); got != "" {
		t.Fatalf("both-chainless must emit no note: %q", got)
	}
}

// TestCWDCompareOverviewCarriesAsymmetryNoteRow wires the note through the
// real overview builder: side A carries on-chain attribution, side B carries
// none and the typed WakeupChainRecommendedNotRun flag — the table must close
// with the ⚠ 锚窗质量不对称 row; the symmetric control emits none.
func TestCWDCompareOverviewCarriesAsymmetryNoteRow(t *testing.T) {
	chainSide := func(label string) types.TraceCausalProjection {
		return types.TraceCausalProjection{
			ArtifactLabel: label,
			WakeupPath:    []string{"dep-2", "app-1"},
			WindowStartTs: 3680.818, WindowEndTs: 3680.919,
			OnChainCauses: []types.TraceCausalProjectionNode{{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
				Subject: "dep-2", Object: "blocking_span",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 94.466, CumulativeImpactMS: 94.466, Confidence: 0.8,
			}},
		}
	}
	flatSide := types.TraceCausalProjection{
		ArtifactLabel:                "6.0B138_3900.sys.systrace",
		WindowStartTs:                8144.608,
		WindowEndTs:                  8144.708,
		WakeupChainRecommendedNotRun: true,
		BackgroundCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-bg",
			Subject: "logd.writer-7756", Object: "runnable_wait", StateKind: "runnable",
			ChainRelevance: "background", Causality: "background",
			ImpactMS: 83.893, Confidence: 0.7,
		}},
	}
	block := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{chainSide("7.0B30SP22_7315.systrace"), flatSide},
		types.ObservationLedger{}, "zh", true, runtimeTraceProjUserFocus{})
	if block == nil {
		t.Fatalf("overview block missing")
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: the ⚠ note left the
	// table rows for the block Text lines (layout-L1); 锚窗质量不对称 →
	// 两侧分析窗证据深度不对称, 锚窗内 → 的分析窗内 (窗族); on-chain 归因/已归因 →
	// 链上归因/链上已归因 (归因族).
	if !strings.Contains(block.Text, "⚠ 两侧分析窗证据深度不对称:6.0B138_3900.sys.systrace(该侧唤醒链下钻未执行) 的分析窗内无链上归因;7.0B30SP22_7315.systrace 的分析窗内链上已归因 94.466ms") {
		t.Fatalf("asymmetric sides must add the anchor-quality note line:\n%s", block.Text)
	}
	// The note must not ride a table row anymore.
	for _, item := range block.Items {
		if strings.Contains(strings.Join(item.Cells, " "), "证据深度不对称") {
			t.Fatalf("the asymmetry note must not ride a table row: %+v", item.Cells)
		}
	}
	// Symmetric control: two attributed sides → no note line.
	symmetric := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{chainSide("A.trace"), chainSide("B.trace")},
		types.ObservationLedger{}, "zh", true, runtimeTraceProjUserFocus{})
	if symmetric == nil {
		t.Fatalf("symmetric overview block missing")
	}
	if strings.Contains(symmetric.Text, "证据深度不对称") {
		t.Fatalf("symmetric sides must not add the note: %q", symmetric.Text)
	}
	for _, item := range symmetric.Items {
		if strings.Contains(strings.Join(item.Cells, " "), "证据深度不对称") {
			t.Fatalf("symmetric sides must not add the note: %+v", item.Cells)
		}
	}
}
