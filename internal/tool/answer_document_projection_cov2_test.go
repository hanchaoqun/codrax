package tool

// COV-2 批 pins (§24.14 B-1/B-2/B-5/D-1/D-3 + §24.12 D-P0 + §24.14 D-4 ±10%
// 容差裁定, real_trace_campaign_20260705.md, 2026-07-08): the comparison
// overview's cell-level window-base and caliber disclosures.
//
//   - V1: the symptom cell's two arms both carry caliber words; the state
//     arm's gate is the coverage sentence's dominance form (census 同款),
//     not the retired bare-existence form, and it consumes the §21 CWD
//     window consensus (a cross-window numerator never pairs with a
//     single-window caliber claim). Mixed arms across sides add the B-1
//     两侧口径不同 note.
//   - V2: 主根因/关注线程症状时长/链上已归因(单项最大)/算力供给 reuse the
//     background column's 窗基 disclosure lane; the supply share speaks
//     占其查询窗 (DCS E5 同措辞).
//   - V3: the background cell's closed-set miss falls back to the top
//     data-bearing background row instead of a fabricated "—".
//   - V4: analysis-window vs user-requested-window length deviation >±10%
//     on any side discloses every windowed side; ≤10% and absence stay
//     silent.
//   - V5: the cmp_78_01 overview final-form witness.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- fixture helpers -------------------------------------------------------------

func cov2SelfStateRow(subject, state string, ms float64, ws, we float64) runtimeTraceProjTreeRow {
	node := types.TraceCausalProjectionNode{
		Subject: subject, Object: "sleep_wait", StateKind: state, ImpactMS: ms,
		Role: types.TraceCausalRoleRootCauseContext,
	}
	node.QueryWindowStartTs, node.QueryWindowEndTs = ws, we
	return runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowSelf, HasData: true, Node: node}
}

func cov2SelfHopSleepRow(subject string, ms float64, ws, we float64) runtimeTraceProjTreeRow {
	node := types.TraceCausalProjectionNode{
		Subject: subject, Object: "sleep_wait", StateKind: "s_sleep", ImpactMS: ms,
		Role: types.TraceCausalRoleCausalHop,
	}
	node.QueryWindowStartTs, node.QueryWindowEndTs = ws, we
	return runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowSelf, HasData: true, Node: node}
}

// --- V1: dominance gate + caliber words ------------------------------------------

// TestCOV2SymptomCellDominanceCollapseDiscloses pins the §24.12 D-P0 / §24.14
// B-1 fix: the state arm's gate is no longer bare existence — when the cell
// population collapses against the target's full wait-view universe (census
// 单项最大 > 入 cell 合计, the coverage sentence's own §24.15 C3 form), the
// cell discloses the exclusion instead of publishing a naked 3.262-style
// number. 突变自查: reverting the gate to the bare-existence "%.3fms" form
// reds this pin.
func TestCOV2SymptomCellDominanceCollapseDiscloses(t *testing.T) {
	model := runtimeTraceProjTreeModel{
		Target:   "main-6565",
		WindowMS: 1800.0,
		SelfRows: []runtimeTraceProjTreeRow{
			cov2SelfStateRow("main-6565", "s_sleep", 1.947, 0, 0),
			cov2SelfStateRow("main-6565", "runnable", 1.315, 0, 0),
			cov2SelfHopSleepRow("main-6565", 456.725, 0, 0),
		},
	}
	cell, arm, _ := runtimeTraceProjCompareTargetSymptomCell(types.TraceCausalProjection{}, model, true)
	if cell != "3.262ms(仅计入分析窗内直接等待;另有 1 条关注线程状态行未计入,单项最大 456.725ms)" {
		t.Fatalf("collapsed population must disclose the exclusion: %q", cell)
	}
	if arm != runtimeTraceProjCompareSymptomArmState {
		t.Fatalf("state arm expected, got %d", arm)
	}
	en, _, _ := runtimeTraceProjCompareTargetSymptomCell(types.TraceCausalProjection{}, model, false)
	if en != "3.262ms (direct waits inside the analysis window only; 1 more target state row(s) uncounted, single largest 456.725ms)" {
		t.Fatalf("EN collapsed cell mismatch: %q", en)
	}
}

// TestCOV2SymptomCellDominantControl is the negative control: a census with a
// smaller excluded row (单项最大 < 入 cell 合计) keeps the dominant form —
// caliber word only, no exclusion disclosure.
func TestCOV2SymptomCellDominantControl(t *testing.T) {
	model := runtimeTraceProjTreeModel{
		Target:   "main-6565",
		WindowMS: 1800.0,
		SelfRows: []runtimeTraceProjTreeRow{
			cov2SelfStateRow("main-6565", "s_sleep", 800.0, 0, 0),
			cov2SelfHopSleepRow("main-6565", 5.0, 0, 0),
		},
	}
	cell, _, _ := runtimeTraceProjCompareTargetSymptomCell(types.TraceCausalProjection{}, model, true)
	if cell != "800.000ms(全窗状态统计)" {
		t.Fatalf("dominant population keeps the plain caliber word: %q", cell)
	}
	if strings.Contains(cell, "另有") {
		t.Fatalf("dominant population must not disclose an exclusion: %q", cell)
	}
}

// TestCOV2SymptomCellCrossBaseDisclosure pins the CWD-consensus arm (§24.14
// B-1 主臂接 CWD 窗基共识): admitted state rows positively disagreeing on
// their query windows drop the 全窗 claim and say the numerator spans
// multiple query windows; the same-window control keeps the dominant form.
func TestCOV2SymptomCellCrossBaseDisclosure(t *testing.T) {
	projection := types.TraceCausalProjection{WindowStartTs: 100.0, WindowEndTs: 100.101}
	model := runtimeTraceProjTreeModel{
		Target:   "main-6565",
		WindowMS: 101.0,
		SelfRows: []runtimeTraceProjTreeRow{
			cov2SelfStateRow("main-6565", "s_sleep", 30.0, 100.0, 100.101),
			cov2SelfStateRow("main-6565", "s_sleep", 50.0, 200.0, 200.101),
		},
	}
	cell, _, base := runtimeTraceProjCompareTargetSymptomCell(projection, model, true)
	if cell != "80.000ms(状态统计;链上/自身数据横跨多个查询窗)" {
		t.Fatalf("cross-base numerator must drop the single-window caliber claim: %q", cell)
	}
	if !base.cross || !base.mismatch {
		t.Fatalf("cross-base cell must report the cross window base: %+v", base)
	}
	// Same-window control: single provable base == the analysis window.
	control := runtimeTraceProjTreeModel{
		Target:   "main-6565",
		WindowMS: 101.0,
		SelfRows: []runtimeTraceProjTreeRow{
			cov2SelfStateRow("main-6565", "s_sleep", 30.0, 100.0, 100.101),
			cov2SelfStateRow("main-6565", "s_sleep", 50.0, 100.0, 100.101),
		},
	}
	controlCell, _, controlBase := runtimeTraceProjCompareTargetSymptomCell(projection, control, true)
	if controlCell != "80.000ms(全窗状态统计)" {
		t.Fatalf("same-window numerator keeps the dominant caliber word: %q", controlCell)
	}
	if controlBase.cross || controlBase.mismatch || !controlBase.known {
		t.Fatalf("same-window base must match the analysis window: %+v", controlBase)
	}
}

// --- V1: B-1 mixed-caliber note ---------------------------------------------------

// TestCOV2SymptomCaliberMismatchNote pins the B-1 修向 note: a state-arm cell
// beside a hop-arm cell adds the 两侧口径不同 line; the same-arm control emits
// nothing.
func TestCOV2SymptomCaliberMismatchNote(t *testing.T) {
	stateSide := types.TraceCausalProjection{
		ArtifactLabel: "a.systrace",
		WakeupPath:    []string{"srv-1", "main-1"},
		WindowStartTs: 100.0, WindowEndTs: 101.8,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-a",
			Subject: "main-1", Object: "sleep_wait", StateKind: "s_sleep",
			ImpactMS: 12.0, Confidence: 0.8,
			QueryWindowStartTs: 100.0, QueryWindowEndTs: 101.8,
		}},
	}
	hopSide := types.TraceCausalProjection{
		ArtifactLabel: "b.systrace",
		WakeupPath:    []string{"srv-2", "main-2"},
		WindowStartTs: 200.0, WindowEndTs: 201.8,
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-b",
			Subject: "main-2", Object: "sleep_wait", StateKind: "s_sleep",
			ImpactMS: 470.071, Confidence: 0.8,
			QueryWindowStartTs: 200.0, QueryWindowEndTs: 201.8,
		}},
	}
	block := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{stateSide, hopSide},
		types.ObservationLedger{}, "zh", true, runtimeTraceProjUserFocus{})
	if block == nil {
		t.Fatalf("overview must render")
	}
	if !strings.Contains(block.Text, "⚠ 关注线程症状时长列两侧口径不同(全窗状态统计 / 唤醒链采样合计),不可直接对读") {
		t.Fatalf("mixed symptom arms must add the B-1 caliber note:\n%s", block.Text)
	}
	// Same-arm control: two state sides → no caliber note.
	control := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{stateSide, stateSide},
		types.ObservationLedger{}, "zh", true, runtimeTraceProjUserFocus{})
	if control == nil {
		t.Fatalf("control overview must render")
	}
	if strings.Contains(control.Text, "两侧口径不同") {
		t.Fatalf("same-arm sides must not add the caliber note:\n%s", control.Text)
	}
}

// --- V2: per-column 窗基 notes ----------------------------------------------------

func cov2PrimarySide(label string, ws, we float64, primaryWS, primaryWE float64) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		ArtifactLabel: label,
		WakeupPath:    []string{"srv-9", "main-9"},
		WindowStartTs: ws, WindowEndTs: we,
		RootCauseFamilyObserved: true,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-" + label,
			Subject: "srv-9", Predicate: "root_cause_primary", Object: "binder_wait",
			Rank: 1, ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
			ImpactMS: 3.843, CumulativeImpactMS: 3.843, Confidence: 0.9,
			QueryWindowStartTs: primaryWS, QueryWindowEndTs: primaryWE,
		}},
	}
}

// TestCOV2PrimaryColumnWindowBaseNote pins §24.14 B-2 (主根因列双侧窗基 20×
// 不同源必须可见): a lead node sourced from a hot query window ≠ its side's
// analysis window names the bases; the same-window control stays silent.
// 复核 F3 (注洪泛最小合并): in this fixture the primary and on-chain lanes
// share one byte-identical base tuple — they fold into ONE ⚠ line naming
// both columns.
func TestCOV2PrimaryColumnWindowBaseNote(t *testing.T) {
	hot := cov2PrimarySide("7.0.systrace", 3680.819, 3682.619, 3680.819, 3680.900)
	full := cov2PrimarySide("6.0.systrace", 8144.608, 8146.253, 8144.608, 8146.253)
	block := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{hot, full},
		types.ObservationLedger{}, "zh", true, runtimeTraceProjUserFocus{})
	if block == nil {
		t.Fatalf("overview must render")
	}
	if !strings.Contains(block.Text, "⚠ 主根因、链上已归因(单项最大)数值来自各自所在查询窗(窗基: 81.000ms / 分析窗),不可按分析窗折算") {
		t.Fatalf("primary+on-chain lanes must fold into one base note:\n%s", block.Text)
	}
	if strings.Count(block.Text, "数值来自各自所在查询窗") != 1 {
		t.Fatalf("identical tuples must emit exactly one 窗基 line:\n%s", block.Text)
	}
	// 同窗静默 control: both leads carry their own analysis window.
	controlA := cov2PrimarySide("a.systrace", 3680.819, 3682.619, 3680.819, 3682.619)
	controlB := cov2PrimarySide("b.systrace", 8144.608, 8146.253, 8144.608, 8146.253)
	control := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{controlA, controlB},
		types.ObservationLedger{}, "zh", true, runtimeTraceProjUserFocus{})
	if control == nil {
		t.Fatalf("control overview must render")
	}
	if strings.Contains(control.Text, "数值来自各自所在查询窗") {
		t.Fatalf("same-base cells must not add the note:\n%s", control.Text)
	}
}

// TestCOV2OnChainColumnWindowBaseNote pins the on-chain column's shared
// coverage-consensus base (§24.14 B-2 second half): a chain numerator sourced
// from a hot query window names its base beside the renamed
// 链上已归因(单项最大) header (folded with the primary lane's identical
// tuple, 复核 F3).
func TestCOV2OnChainColumnWindowBaseNote(t *testing.T) {
	hot := cov2PrimarySide("7.0.systrace", 3680.819, 3682.619, 3680.819, 3680.900)
	full := cov2PrimarySide("6.0.systrace", 8144.608, 8146.253, 8144.608, 8146.253)
	block := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{hot, full},
		types.ObservationLedger{}, "zh", true, runtimeTraceProjUserFocus{})
	if block == nil {
		t.Fatalf("overview must render")
	}
	joinedColumns := strings.Join(block.Columns, "|")
	if !strings.Contains(joinedColumns, "链上已归因(单项最大)") {
		t.Fatalf("§24.15 C4 caliber word must reach the column header: %v", block.Columns)
	}
	if !strings.Contains(block.Text, "链上已归因(单项最大)数值来自各自所在查询窗(窗基: 81.000ms / 分析窗),不可按分析窗折算") {
		t.Fatalf("on-chain column must name the split window bases:\n%s", block.Text)
	}
}

// TestCOV2SupplyColumnWindowBaseNote pins §24.14 B-5/D-1: the supply share
// speaks 占其查询窗 (DCS E5 同措辞) and the note names the observation windows
// when they differ from the analysis windows (the 22× misread witness); the
// same-length control stays silent.
func TestCOV2SupplyColumnWindowBaseNote(t *testing.T) {
	sideA := cov2PrimarySide("7.0B30SP22_7315.systrace", 3680.819, 3682.619, 3680.819, 3682.619)
	sideA.ArtifactPath = "7.0B30SP22_7315.systrace"
	sideB := cov2PrimarySide("6.0B138_3900.sys.systrace", 8144.608, 8146.253, 8144.608, 8146.253)
	sideB.ArtifactPath = "6.0B138_3900.sys.systrace"
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{
		cmpcSupplyObs("supply-a", "7.0B30SP22_7315.systrace",
			cmpcSupplyNotes(0.995, 90, 30, 1.708, 62, 80.621, 2)...),
		cmpcSupplyObs("supply-b", "6.0B138_3900.sys.systrace",
			cmpcSupplyNotes(0.993, 164, 5, 2.701, 12, 93.100, 4)...),
	}}
	block := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{sideA, sideB}, ledger, "zh", true, runtimeTraceProjUserFocus{})
	if block == nil {
		t.Fatalf("overview must render")
	}
	var cells []string
	for _, item := range block.Items {
		cells = append(cells, strings.Join(item.Cells, "|"))
	}
	table := strings.Join(cells, "\n")
	if !strings.Contains(table, "就绪积压时核闲置 1.708ms(占其查询窗 2.1%)") {
		t.Fatalf("the supply share must speak 占其查询窗:\n%s", table)
	}
	if !strings.Contains(block.Text, "⚠ 算力供给占比已按各自查询窗归一化(窗基: 80.621ms / 93.100ms),非分析窗占比") {
		t.Fatalf("the supply column must name its window bases:\n%s", block.Text)
	}
	// Same-length control (window_ms == analysis window within 1ms) → silent.
	controlLedger := types.ObservationLedger{Records: []types.ObservationRecord{
		cmpcSupplyObs("supply-a", "7.0B30SP22_7315.systrace",
			cmpcSupplyNotes(0.995, 90, 30, 1.708, 62, 1800.0, 2)...),
		cmpcSupplyObs("supply-b", "6.0B138_3900.sys.systrace",
			cmpcSupplyNotes(0.993, 164, 5, 2.701, 12, 1645.0, 4)...),
	}}
	control := runtimeTraceProjCompareOverviewBlock(
		[]types.TraceCausalProjection{sideA, sideB}, controlLedger, "zh", true, runtimeTraceProjUserFocus{})
	if control == nil {
		t.Fatalf("control overview must render")
	}
	if strings.Contains(control.Text, "⚠ 算力供给占比") {
		t.Fatalf("analysis-window supply bases must not add the note:\n%s", control.Text)
	}
}

// --- V3: background cell closed-set fallback ---------------------------------------

// TestCOV2BackgroundCellClosedSetFallback pins §24.14 D-3: a background stanza
// whose rows all fall outside the cross-thread-aggregate closed set (the
// cmp_78_01 trace_span rank rows) renders the top row with its caliber word —
// never the fabricated "—" that contradicted the tree stanza. 突变自查:
// disabling the fallback arm (returning "—") reds this pin.
func TestCOV2BackgroundCellClosedSetFallback(t *testing.T) {
	spanNode := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-bg",
		Subject: "NetworkStats-8570", Object: "trace_span",
		SpanName: "broadcastReceiveReg", ImpactMS: 91.940, Confidence: 0.7,
	}
	model := runtimeTraceProjTreeModel{
		Target: "main-21538",
		Background: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: spanNode},
		},
		WindowMS: 1645.0,
	}
	cell, density := runtimeTraceProjCompareBackgroundPressureCell(model, true)
	if cell == "—" {
		t.Fatalf("a data-bearing background stanza must never render the dash")
	}
	if !strings.Contains(cell, "91.940ms(背景行单项最大,非跨线程累计)") {
		t.Fatalf("the fallback cell must carry the top row with its caliber word: %q", cell)
	}
	if !strings.Contains(cell, "NetworkStats-8570") {
		t.Fatalf("the fallback cell must name the row subject: %q", cell)
	}
	if density != 0 {
		t.Fatalf("the fallback arm never mints a density window: %f", density)
	}
	// EN face.
	enCell, _ := runtimeTraceProjCompareBackgroundPressureCell(model, false)
	if !strings.Contains(enCell, "91.940ms (largest single background row, not a cross-thread cumulative)") {
		t.Fatalf("EN fallback cell mismatch: %q", enCell)
	}
	// Closed-set control: a positive cross-thread aggregate row keeps the
	// density arm untouched (the fallback never competes with it).
	aggregate := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-agg",
		Subject: "unknown-thread", Object: "cpu_pressure",
		SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
		ImpactMS:    500.0, Confidence: 0.6,
	}
	control := runtimeTraceProjTreeModel{
		Target: "main-21538",
		Background: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: spanNode},
			{Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: aggregate},
		},
		WindowMS: 1645.0,
	}
	controlCell, controlDensity := runtimeTraceProjCompareBackgroundPressureCell(control, true)
	if strings.Contains(controlCell, "背景行单项最大") || controlDensity <= 0 {
		t.Fatalf("a closed-set hit must keep the density arm: %q (window %f)", controlCell, controlDensity)
	}
	// Zero-value stanza control: nothing data-bearing → the honest dash stays.
	empty := runtimeTraceProjTreeModel{Target: "main-21538", WindowMS: 1645.0}
	if cell, _ := runtimeTraceProjCompareBackgroundPressureCell(empty, true); cell != "—" {
		t.Fatalf("an empty stanza keeps the honest dash: %q", cell)
	}
}

// TestCOV2BackgroundFallbackValueLaneAndCaliberSource pins the 复核 F1 fixes
// (two witnesses): ① a cumulative-sourced background row (ImpactMS=0,
// CumulativeImpactMS>0) must NOT claim 非跨线程累计 while its own tree stanza
// row wears 累计(跨线程) — the cell speaks the SAME shared word; ② a
// MergedCount>1 fold row publishes its single-instance MergedMaxMS (census
// 同款), never the Σ under a 单项最大 label; PeriodicSource rows never lead.
// 突变自查: reverting the value lane to raw DisplayImpact (or the word gate to
// the unconditional 非跨线程累计) reds these pins.
func TestCOV2BackgroundFallbackValueLaneAndCaliberSource(t *testing.T) {
	// ① cumulative-sourced row: value 500, word = 累计(跨线程), never 非跨线程累计.
	cumNode := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-cum",
		Subject: "pool-worker", Object: "trace_span", SpanName: "poolDrain",
		CumulativeImpactMS: 500.0, Confidence: 0.6,
	}
	cumModel := runtimeTraceProjTreeModel{
		Target: "main-1", WindowMS: 1645.0,
		Background: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: cumNode},
		},
	}
	cell, _ := runtimeTraceProjCompareBackgroundPressureCell(cumModel, true)
	if !strings.Contains(cell, "500.000ms(背景行单项最大,累计(跨线程))") {
		t.Fatalf("a cumulative-sourced fallback value must wear the tree's own cum word: %q", cell)
	}
	if strings.Contains(cell, "非跨线程累计") {
		t.Fatalf("a cumulative-sourced value must never claim 非跨线程累计: %q", cell)
	}
	en, _ := runtimeTraceProjCompareBackgroundPressureCell(cumModel, false)
	if !strings.Contains(en, "500.000ms (largest single background row, cross-thread cum)") {
		t.Fatalf("EN cumulative-sourced fallback word mismatch: %q", en)
	}
	// ② merged fold row: Σ=4.500 never publishes; the single-instance max
	// 0.900 does, and the window-sourced word stays true for it.
	mergedNode := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-merged",
		Subject: "verify-pool", Object: "trace_span", SpanName: "VerifyClass",
		ImpactMS: 4.500, MergedCount: 5, MergedMinMS: 0.100, MergedMaxMS: 0.900,
		Confidence: 0.6,
	}
	mergedModel := runtimeTraceProjTreeModel{
		Target: "main-1", WindowMS: 1645.0,
		Background: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: mergedNode},
		},
	}
	mergedCell, _ := runtimeTraceProjCompareBackgroundPressureCell(mergedModel, true)
	if !strings.Contains(mergedCell, "0.900ms(背景行单项最大,非跨线程累计)") {
		t.Fatalf("a merged fold row must publish its single-instance max: %q", mergedCell)
	}
	if strings.Contains(mergedCell, "4.500ms") {
		t.Fatalf("the merged Σ must never wear the 单项最大 label: %q", mergedCell)
	}
	// A merged row without a single-instance magnitude never competes.
	noMax := mergedNode
	noMax.MergedMaxMS = 0
	noMaxModel := runtimeTraceProjTreeModel{
		Target: "main-1", WindowMS: 1645.0,
		Background: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: noMax},
		},
	}
	if cell, _ := runtimeTraceProjCompareBackgroundPressureCell(noMaxModel, true); cell != "—" {
		t.Fatalf("a Σ-only merged row must not compete: %q", cell)
	}
	// PeriodicSource rows never lead the cell (raw cadence-dominated).
	periodic := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-per",
		Subject: "vsync-timer", Object: "trace_span", SpanName: "tick",
		ImpactMS: 900.0, PeriodicSource: true, Confidence: 0.6,
	}
	periodicModel := runtimeTraceProjTreeModel{
		Target: "main-1", WindowMS: 1645.0,
		Background: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: periodic},
		},
	}
	if cell, _ := runtimeTraceProjCompareBackgroundPressureCell(periodicModel, true); cell != "—" {
		t.Fatalf("a periodic row must not lead the fallback cell: %q", cell)
	}
}

// --- V4: ±10% user-window deviation disclosure -------------------------------------

// TestCOV2UserWindowDeviationNotes pins the §24.14 D-4 ruling: >±10% on any
// side → ONE typed disclosure line with a clause per windowed side (复核 F3
// 两行折一行两分句; F4 %.1f so a 10.3% deviation never displays as the
// silence threshold's own "10%"); all-within → silent; no typed user window →
// silent (absence never guesses). 突变自查: disabling the ±10% gate (always
// silent) reds the positive half.
func TestCOV2UserWindowDeviationNotes(t *testing.T) {
	sideA := types.TraceCausalProjection{ArtifactLabel: "7.0.systrace", WindowStartTs: 3680.819, WindowEndTs: 3682.619}
	sideB := types.TraceCausalProjection{ArtifactLabel: "6.0.systrace", WindowStartTs: 8144.608, WindowEndTs: 8146.253}
	focus := runtimeTraceProjUserFocus{Entities: []string{"8144.608", "8145.492"}}
	notes := runtimeTraceProjCompareUserWindowDeviationNotes(
		[]types.TraceCausalProjection{sideA, sideB}, focus, true)
	if len(notes) != 1 {
		t.Fatalf("the deviation disclosure is one folded line: %v", notes)
	}
	if notes[0] != "⚠ 7.0.systrace:分析窗 1800.000ms,较你指定的 884.000ms 长 103.6%;6.0.systrace:分析窗 1645.000ms,长 86.1%:窗口按数据边界对齐构造" {
		t.Fatalf("folded deviation line mismatch: %q", notes[0])
	}
	// Co-disclosure: one side within ±10% still discloses when the other
	// triggers (对读基不同构必须可见) — same folded line, two clauses.
	within := types.TraceCausalProjection{ArtifactLabel: "w.systrace", WindowStartTs: 100.0, WindowEndTs: 100.930}
	over := types.TraceCausalProjection{ArtifactLabel: "o.systrace", WindowStartTs: 200.0, WindowEndTs: 201.500}
	pair := runtimeTraceProjCompareUserWindowDeviationNotes(
		[]types.TraceCausalProjection{within, over},
		runtimeTraceProjUserFocus{Entities: []string{"100.000", "100.884"}}, true)
	if len(pair) != 1 {
		t.Fatalf("any-side trigger must fold both clauses into one line: %v", pair)
	}
	if !strings.Contains(pair[0], "w.systrace:分析窗 930.000ms,与你指定的 884.000ms 偏差 5.2%(±10% 内)") {
		t.Fatalf("the within-tolerance clause must state its own deviation: %q", pair[0])
	}
	if !strings.Contains(pair[0], "o.systrace:分析窗 1500.000ms,长 69.7%") {
		t.Fatalf("the triggering clause must state its deviation: %q", pair[0])
	}
	// All-within silence (≤10% 静默).
	silent := runtimeTraceProjCompareUserWindowDeviationNotes(
		[]types.TraceCausalProjection{within, within},
		runtimeTraceProjUserFocus{Entities: []string{"100.000", "100.884"}}, true)
	if silent != nil {
		t.Fatalf("all-within sides must stay silent: %v", silent)
	}
	// Absence: no typed user window → nothing (never guessed).
	if got := runtimeTraceProjCompareUserWindowDeviationNotes(
		[]types.TraceCausalProjection{sideA, sideB}, runtimeTraceProjUserFocus{}, true); got != nil {
		t.Fatalf("no user window must stay silent: %v", got)
	}
	// EN face keeps the same facts on one line.
	en := runtimeTraceProjCompareUserWindowDeviationNotes(
		[]types.TraceCausalProjection{sideA, sideB}, focus, false)
	if len(en) != 1 ||
		!strings.Contains(en[0], "103.6% longer than your requested 884.000ms") ||
		!strings.Contains(en[0], "1645.000ms is 86.1% longer") {
		t.Fatalf("EN deviation line mismatch: %v", en)
	}
	// Shorter-window arm: analysis window under the requested length.
	short := types.TraceCausalProjection{ArtifactLabel: "s.systrace", WindowStartTs: 100.0, WindowEndTs: 100.400}
	shortNotes := runtimeTraceProjCompareUserWindowDeviationNotes(
		[]types.TraceCausalProjection{short},
		runtimeTraceProjUserFocus{Entities: []string{"100.000", "100.884"}}, true)
	if len(shortNotes) != 1 || !strings.Contains(shortNotes[0], "短 54.8%") {
		t.Fatalf("shorter-window side must speak the 短 arm: %v", shortNotes)
	}
}

// --- V5: cmp_78_01 overview final-form witness -------------------------------------

// TestCOV2Cmp78OverviewFinalForm is the cmp_78_01 witness, reconciled against
// the specimen原文 /Users/han/opt/customlogs/cust_trace_cmp_78_01.txt 总览
// lines 207-210 (复核收尾 item 4): 症状 3.262 vs 470.071, 主根因 3.843 vs
// 35.613, 链上已归因 65.232 vs 92.346, 供给率 99.5/99.3 · 闲置 1.708/2.701 ·
// 占其查询窗 2.1%/2.9%, 分析窗 3680.819s→3682.619s vs 8144.608s→8146.253s.
// The final form adds what the specimen lacked: both symptom calibers + the
// mixed-arm note, the primary column's 20× window-base split, the supply
// window bases, the 6.0 background row instead of the fabricated "—" (D-3),
// and the folded D-4 deviation line (86.1% on the 6.0 side). Deliberate
// non-goals here: the lead election's subject naming (SYM/SYM-2 domain) and
// the 优化点 column (DCS/RCM domain).
func TestCOV2Cmp78OverviewFinalForm(t *testing.T) {
	fullWindow70 := [2]float64{3680.819, 3682.619}
	hotWindow70 := [2]float64{3680.819, 3680.900}
	fullWindow60 := [2]float64{8144.608, 8146.253}
	side70 := types.TraceCausalProjection{
		ArtifactLabel: "7.0B30SP22_7315.systrace",
		ArtifactPath:  "7.0B30SP22_7315.systrace",
		WakeupPath:    []string{"binder:8815_1-6581", "main-6565"},
		WindowStartTs: fullWindow70[0], WindowEndTs: fullWindow70[1],
		RootCauseFamilyObserved: true,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E1",
			Subject: "binder:8815_1-6581", Predicate: "root_cause_primary",
			Object: "binder_wait", Rank: 1, ChainRelevance: "on_chain",
			Causality: "on_wakeup_chain", ImpactMS: 3.843, CumulativeImpactMS: 3.843,
			Confidence:         0.9,
			QueryWindowStartTs: hotWindow70[0], QueryWindowEndTs: hotWindow70[1],
		}},
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E2",
				Subject: "main-6565", Object: "sleep_wait", StateKind: "s_sleep",
				ImpactMS: 1.947, Confidence: 0.8,
				QueryWindowStartTs: fullWindow70[0], QueryWindowEndTs: fullWindow70[1]},
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E3",
				Subject: "main-6565", Object: "runnable_wait", StateKind: "runnable",
				ImpactMS: 1.315, Confidence: 0.8,
				QueryWindowStartTs: fullWindow70[0], QueryWindowEndTs: fullWindow70[1]},
			// E16 (specimen 链上已归因 65.232): the largest depth-1 caliber —
			// the on-chain cell reads 65.232, not the primary's 3.843.
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E16",
				Subject: "binder:8815_1-6581", Object: "blocking_span",
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
				ImpactMS: 65.232, CumulativeImpactMS: 65.232, Confidence: 0.8,
				QueryWindowStartTs: hotWindow70[0], QueryWindowEndTs: hotWindow70[1]},
		},
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "E4",
			Subject: "main-6565", Object: "sleep_wait", StateKind: "s_sleep",
			ImpactMS: 456.725, Confidence: 0.8,
			QueryWindowStartTs: fullWindow70[0], QueryWindowEndTs: fullWindow70[1],
		}},
	}
	side60 := types.TraceCausalProjection{
		ArtifactLabel: "6.0B138_3900.sys.systrace",
		ArtifactPath:  "6.0B138_3900.sys.systrace",
		WakeupPath:    []string{"srv-1", "main-21538"},
		WindowStartTs: fullWindow60[0], WindowEndTs: fullWindow60[1],
		RootCauseFamilyObserved: true,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E1",
			Subject: "srv-1", Predicate: "root_cause_primary",
			Object: "binder_wait", Rank: 1, ChainRelevance: "on_chain",
			Causality: "on_wakeup_chain", ImpactMS: 35.613, CumulativeImpactMS: 35.613,
			Confidence:         0.9,
			QueryWindowStartTs: fullWindow60[0], QueryWindowEndTs: fullWindow60[1],
		}},
		OnChainCauses: []types.TraceCausalProjectionNode{
			// E5 (specimen 链上已归因 92.346): the largest depth-1 caliber.
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E5",
				Subject: "srv-1", Object: "blocking_span",
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
				ImpactMS: 92.346, CumulativeImpactMS: 92.346, Confidence: 0.8,
				QueryWindowStartTs: fullWindow60[0], QueryWindowEndTs: fullWindow60[1]},
		},
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "E6",
			Subject: "main-21538", Object: "sleep_wait", StateKind: "s_sleep",
			ImpactMS: 470.071, Confidence: 0.8,
			QueryWindowStartTs: fullWindow60[0], QueryWindowEndTs: fullWindow60[1],
		}},
		// D-3 terminal form: the specimen's 6.0 tree stanza published the
		// NetworkStats trace_span rank row while the overview cell said "—";
		// the model now carries the row and the cell shows it.
		BackgroundCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E8",
			Subject: "NetworkStats-8570", Object: "trace_span",
			SpanName: "broadcastReceiveReg", ChainRelevance: "background",
			Causality: "background", ImpactMS: 91.940, Confidence: 0.7,
			QueryWindowStartTs: fullWindow60[0], QueryWindowEndTs: fullWindow60[1],
		}},
	}
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{
		cmpcSupplyObs("supply-70", "7.0B30SP22_7315.systrace",
			cmpcSupplyNotes(0.995, 90, 30, 1.708, 62, 80.621, 2)...),
		cmpcSupplyObs("supply-60", "6.0B138_3900.sys.systrace",
			cmpcSupplyNotes(0.993, 164, 5, 2.701, 12, 93.100, 4)...),
	}}
	focus := runtimeTraceProjUserFocus{Entities: []string{"8144.608", "8145.492"}}
	blocks := runtimeTraceProjCompareOverviewBlocks(
		[]types.TraceCausalProjection{side70, side60}, ledger, "zh", true, focus)
	if len(blocks) == 0 {
		t.Fatalf("cmp_78 overview must render")
	}
	block := &blocks[0]
	var rows []string
	for _, item := range block.Items {
		rows = append(rows, strings.Join(item.Cells, "|"))
	}
	table := strings.Join(rows, "\n")
	// V1: both symptom cells carry caliber words — the 3.262 side discloses
	// its collapsed population + cross-window numerator, the 470.071 side
	// keeps its hop-caliber annotation. Specimen values verbatim (line
	// 207-210: 3.262 / 470.071 / 456.725 单项最大 in the 218 disclosure).
	if !strings.Contains(table, "3.262ms(仅计入分析窗内直接等待;另有 1 条关注线程状态行未计入,单项最大 456.725ms;链上/自身数据横跨多个查询窗)") {
		t.Fatalf("7.0 symptom cell must disclose the collapsed population:\n%s", table)
	}
	if !strings.Contains(table, "470.071ms(唤醒链采样到的关注线程睡眠合计,非全窗状态统计)") {
		t.Fatalf("6.0 symptom cell must keep the hop caliber annotation:\n%s", table)
	}
	if !strings.Contains(block.Text, "两侧口径不同(全窗状态统计 / 唤醒链采样合计),不可直接对读") {
		t.Fatalf("the mixed-arm note must close the table:\n%s", block.Text)
	}
	// Specimen 对账: primary magnitudes 3.843 / 35.613, on-chain 65.232 /
	// 92.346, and the 分析窗 cells verbatim.
	if !strings.Contains(table, "3.843ms") || !strings.Contains(table, "35.613ms") {
		t.Fatalf("primary magnitudes must match the specimen:\n%s", table)
	}
	if !strings.Contains(table, "|65.232ms|") || !strings.Contains(table, "|92.346ms|") {
		t.Fatalf("on-chain cells must match the specimen 65.232/92.346:\n%s", table)
	}
	if !strings.Contains(table, "3680.819s → 3682.619s") || !strings.Contains(table, "8144.608s → 8146.253s") {
		t.Fatalf("分析窗 cells must match the specimen:\n%s", table)
	}
	// V2: the primary column's 20× window-base split is visible (its own
	// line — the symptom/on-chain lanes fold on a different tuple).
	if !strings.Contains(block.Text, "⚠ 主根因数值来自各自所在查询窗(窗基: 81.000ms / 分析窗),不可按分析窗折算") {
		t.Fatalf("the primary column must name the 20× base split:\n%s", block.Text)
	}
	// V2: the on-chain column carries the caliber header; its base note folds
	// with the symptom lane (复核 F3 — identical cross-window tuple: hot
	// chain rows vs full-window self rows on 7.0).
	if !strings.Contains(strings.Join(block.Columns, "|"), "链上已归因(单项最大)") {
		t.Fatalf("the on-chain header must carry the caliber word: %v", block.Columns)
	}
	if !strings.Contains(block.Text, "⚠ 关注线程症状时长、链上已归因(单项最大)数值来自各自所在查询窗(窗基: 横跨多个查询窗 / 分析窗),不可按分析窗折算") {
		t.Fatalf("the symptom+on-chain lanes must fold into one base note:\n%s", block.Text)
	}
	// V3: the 6.0 background cell shows the stanza's top row (specimen said
	// "—" against its own 91.940ms rank rows — the D-3 contradiction).
	if !strings.Contains(table, "91.940ms(背景行单项最大,非跨线程累计)") {
		t.Fatalf("the 6.0 background cell must show the stanza's top row:\n%s", table)
	}
	// V2: the supply share names its own query window in-cell and in the note
	// (specimen ratios/idle values verbatim; 2.1%/2.9% = the specimen shares).
	if !strings.Contains(table, "供给率 99.5% · 就绪积压时核闲置 1.708ms(占其查询窗 2.1%)") ||
		!strings.Contains(table, "供给率 99.3% · 就绪积压时核闲置 2.701ms(占其查询窗 2.9%)") {
		t.Fatalf("supply cells must speak 占其查询窗:\n%s", table)
	}
	if !strings.Contains(block.Text, "⚠ 算力供给占比已按各自查询窗归一化(窗基: 80.621ms / 93.100ms),非分析窗占比") {
		t.Fatalf("the supply column must name its window bases:\n%s", block.Text)
	}
	// V4 → PTV8-LAD L6 (§24.19 留账 完整分层) EVOLUTION RECORD: the full-trigger
	// shape carries 6 classed notes (1 口径矛盾 + 3 窗基 + 2 披露: the RTC-2
	// disjoint-time-base note and the D-4 deviation line), so the two
	// lowest-importance disclosures fold past the visible cap — the table
	// face keeps the top-4 by importance plus ONE pointer line, and the folded
	// lines ride the 对比注记明细 sibling block WHOLE (any-side trigger /
	// both-side clauses unchanged, same helper — see
	// TestCOV2UserWindowDeviationNotes for the wording pins).
	d4 := "⚠ 7.0B30SP22_7315.systrace:分析窗 1800.000ms,较你指定的 884.000ms 长 103.6%;6.0B138_3900.sys.systrace:分析窗 1645.000ms,长 86.1%:窗口按数据边界对齐构造"
	if strings.Contains(block.Text, d4) {
		t.Fatalf("the D-4 disclosure must fold past the visible cap on the full-trigger shape:\n%s", block.Text)
	}
	if !strings.Contains(block.Text, "⚠ 其余 2 条注记见下方「对比注记明细」") {
		t.Fatalf("the fold pointer line must name the folded count:\n%s", block.Text)
	}
	if len(blocks) != 2 || blocks[1].ID != runtimeTraceCausalProjectionCompareNotesBlockID {
		t.Fatalf("the folded shape must emit the 对比注记明细 sibling: %+v", blocks)
	}
	if !strings.Contains(blocks[1].Text, d4) {
		t.Fatalf("the notes-detail block must carry the folded D-4 line whole:\n%s", blocks[1].Text)
	}
	// L6: the caliber-conflict note outranks every window-base note — it leads
	// the layered table face even though it is built after the base lanes.
	if conflict, base := strings.Index(block.Text, "两侧口径不同"), strings.Index(block.Text, "窗基: "); conflict < 0 || base < 0 || conflict > base {
		t.Fatalf("the caliber-conflict note must lead the layered notes:\n%s", block.Text)
	}
	// 复核 F3 flood budget, tightened by L6: the table face carries at most
	// visible-cap + 1 pointer = 5 ⚠ lines; the full set lives in the sibling.
	if got := strings.Count(block.Text, "⚠ "); got > 5 {
		t.Fatalf("note flood budget exceeded (%d ⚠ lines):\n%s", got, block.Text)
	}
}
