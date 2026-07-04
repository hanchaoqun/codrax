package tool

// answer_document_projection_periodic_vs1_test.go — VS-1 (§7.8) presentation
// pins, berlin vsync_cust live numbers: a periodic signal source (VSyncGenerator,
// ≈8.3ms cadence) renders with the cadence label, its attribution column shows
// the discounted value (runnable + lateness = 0.176ms), the window projection
// keeps the raw 36.256ms, the conclusion no longer names the in-period sleep
// row, and the coverage numerator consumes the same discounted caliber.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func periodicVS1Records() []types.ObservationRecord {
	running := projV3Obs("root-running", "root_cause_primary", "root_cause_primary:running",
		"RSUniRenderThre-1963", "running", "4.115", 4.115, 300, 320,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=running", "effective_impact_ms=4.115")
	vsync := projV3Obs("root-vsync", "root_cause_primary", "root_cause_primary:vsync",
		"VSyncGenerator-610", "sleep_wait", "36.256", 36.256, 100, 160,
		"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=s_sleep",
		"periodic_source=true", "detected_period_ms=8.302", "lateness_ms=0.071",
		"effective_impact_ms=0.176")
	path := types.ObservationRecord{
		ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
		Object: "VSyncGenerator-610 -> app-1",
	}
	return []types.ObservationRecord{running, vsync, path}
}

func TestTraceProjectionPeriodicSourceRowZH(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), periodicVS1Records(), "")
	for _, want := range []string{
		// lossless detail table: cadence label on the impact-shape cell …
		"周期性信号源(期内睡眠为正常节拍,周期≈8.3ms)",
		// … discounted attribution with its composition …
		"0.176ms(可运行+迟到量)",
		// … and the raw window projection untouched.
		"36.256ms",
		// caliber legend explains the discount only because a periodic row exists.
		"- 周期性信号源行:有效归因 = 可运行等待全额 + 信号迟到量;期内睡眠为正常节拍,不计入有效归因(窗口投影保留原始值)。",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("VS-1 ZH rendering missing %q:\n%s", want, md)
		}
	}
	// 主根因 consumes the engine rank (running row), never the cadence sleep.
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "**主根因:**") {
			if !strings.Contains(line, "RSUniRenderThre-1963") || strings.Contains(line, "VSyncGenerator") {
				t.Fatalf("primary conclusion must not name the periodic sleep row:\n%s", line)
			}
		}
	}
}

func TestTraceProjectionPeriodicSourceRowEN(t *testing.T) {
	md := audit730Render(t, audit730Bus("en"), periodicVS1Records(), "en")
	for _, want := range []string{
		"periodic signal source (in-period sleep is normal cadence, period ≈8.3ms)",
		"0.176ms (runnable + lateness)",
		"- periodic signal source rows: attribution = runnable wait in full + signal lateness; in-period sleep is normal cadence and never counts (the window projection keeps the raw value).",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("VS-1 EN rendering missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "周期性信号源") {
		t.Fatalf("EN surface must not carry zh labels:\n%s", md)
	}
}

// TestTraceProjectionPeriodicLegendOnlyWhenPresent pins byte-stability for
// non-periodic renders: without a periodic row neither the caliber legend line
// nor the cadence label appears.
func TestTraceProjectionPeriodicLegendOnlyWhenPresent(t *testing.T) {
	md := audit730Render(t, audit730Bus(""), audit730ChainObs(), "")
	for _, banned := range []string{"周期性信号源", "可运行+迟到量"} {
		if strings.Contains(md, banned) {
			t.Fatalf("non-periodic render must stay byte-stable (%q leaked):\n%s", banned, md)
		}
	}
}

// --- unit pins ----------------------------------------------------------------

func periodicVS1Node() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E10",
		Subject: "VSyncGenerator-610", Object: "sleep_wait", Predicate: "root_cause_primary",
		StateKind: "s_sleep", ImpactMS: 36.256, CumulativeImpactMS: 36.361,
		EffectiveImpactMS: 0.176, PeriodicSource: true, DetectedPeriodMS: 8.302,
		PeriodicLatenessMS: 0.071,
		ChainRelevance:     "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
	}
}

func TestRuntimeTraceProjPeriodicRowTagAndMark(t *testing.T) {
	marks := &runtimeTraceProjMarkSet{}
	row := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowChain, HasData: true, Node: periodicVS1Node()}
	row.marks = marks
	_, tags := runtimeTraceProjRowMetricParts(row, 50.0, true, true)
	var flat []string
	for _, tag := range tags {
		flat = append(flat, tag.Text)
	}
	joined := strings.Join(flat, " · ")
	if !strings.Contains(joined, "周期性信号源(期内睡眠为正常节拍,周期≈8.3ms)·有效归因 0.176ms") {
		t.Fatalf("periodic row must carry the cadence Keep tag:\n%s", joined)
	}
	if !marks.has(runtimeTraceProjMarkPeriodicSource) {
		t.Fatalf("periodic tag emission must record its legend mark")
	}
}

// TestRuntimeTraceProjPeriodicCoverageConsumesDiscount pins the coverage lane:
// the on-chain attributed numerator uses the discounted caliber for periodic
// rows — never the cadence-dominated raw cumulative.
func TestRuntimeTraceProjPeriodicCoverageConsumesDiscount(t *testing.T) {
	periodic := runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowChain, HasData: true, Depth: 1, Node: periodicVS1Node(),
	}
	model := runtimeTraceProjTreeModel{WindowMS: 50.0, TreeRows: []runtimeTraceProjTreeRow{periodic}}
	if got := runtimeTraceProjDepth1Cumulative(model); got != 0.176 {
		t.Fatalf("coverage numerator must consume the discounted 0.176ms, got %.3f", got)
	}
	plain := periodic
	plain.Node.PeriodicSource = false
	control := runtimeTraceProjTreeModel{WindowMS: 50.0, TreeRows: []runtimeTraceProjTreeRow{plain}}
	if got := runtimeTraceProjDepth1Cumulative(control); got != 36.361 {
		t.Fatalf("non-periodic coverage must keep the raw cumulative, got %.3f", got)
	}
}

// TestRuntimeTraceProjPeriodicLeadSelection pins the rankless V1 fallback and
// the sole-primary conclusion: a periodic row competes only with its
// discounted value (0 included), and if it ever leads, the stated magnitude is
// the discounted attribution, never the raw cadence sleep.
func TestRuntimeTraceProjPeriodicLeadSelection(t *testing.T) {
	if got := runtimeTraceProjLeadSelectionValue(periodicVS1Node()); got != 0.176 {
		t.Fatalf("periodic selection value must be the discounted 0.176, got %.3f", got)
	}
	zero := periodicVS1Node()
	zero.EffectiveImpactMS = 0
	if got := runtimeTraceProjLeadSelectionValue(zero); got != 0 {
		t.Fatalf("pure-cadence selection value must be 0 (never the raw display impact), got %.3f", got)
	}

	// Rankless fallback: the real 4.115ms running row beats the 36ms cadence row.
	running := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E4",
		Subject: "RSUniRenderThre-1963", Object: "running", StateKind: "running",
		ImpactMS: 4.115, CumulativeImpactMS: 4.115, EffectiveImpactMS: 4.115,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
	}
	projection := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{periodicVS1Node(), running},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "RSUniRenderThre-1963") || strings.Contains(line, "VSyncGenerator") {
		t.Fatalf("rankless fallback must not pick the periodic cadence row:\n%s", line)
	}

	// Sole periodic primary: the conclusion states the discounted magnitude.
	sole := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{periodicVS1Node()},
	}
	soleModel := buildRuntimeTraceProjTreeModel(sole, nil, true)
	soleLine := runtimeTraceProjConclusionLine(sole, soleModel, true)
	if !strings.Contains(soleLine, "0.176ms") || strings.Contains(soleLine, "36.") {
		t.Fatalf("a leading periodic row must state the discounted attribution:\n%s", soleLine)
	}
}

// TestRuntimeTraceProjComparePrimaryCellPeriodicOverride pins F2 (adversarial
// review 2026-07-04): the comparison overview's primary cell consumes the SAME
// periodic override as the conclusion line on BOTH value branches (cumulative
// and merged single-max) — the cell shows the discounted attribution with the
// caliber note, never the raw cadence sleep.
func TestRuntimeTraceProjComparePrimaryCellPeriodicOverride(t *testing.T) {
	sole := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{periodicVS1Node()},
	}
	model := buildRuntimeTraceProjTreeModel(sole, nil, true)
	cell := runtimeTraceProjComparePrimaryCell(sole, model, true)
	if !strings.Contains(cell, "0.176ms(周期性,期内睡眠不计)") {
		t.Fatalf("periodic primary cell must state the discounted value with the caliber note:\n%s", cell)
	}
	if strings.Contains(cell, "36.") {
		t.Fatalf("periodic primary cell must not leak the raw cadence sleep:\n%s", cell)
	}
	if en := runtimeTraceProjComparePrimaryCell(sole, buildRuntimeTraceProjTreeModel(sole, nil, false), false); !strings.Contains(en, "0.176ms (periodic; in-period sleep excluded)") {
		t.Fatalf("EN periodic primary cell missing the caliber note:\n%s", en)
	}

	// Merged branch: a periodic ×N fold's single-max is still cadence — the
	// shared override drives this branch too.
	merged := periodicVS1Node()
	merged.MergedCount = 6
	merged.MergedMaxMS = 6.4
	mergedProj := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{merged},
	}
	mergedModel := buildRuntimeTraceProjTreeModel(mergedProj, nil, true)
	mergedCell := runtimeTraceProjComparePrimaryCell(mergedProj, mergedModel, true)
	if !strings.Contains(mergedCell, "0.176ms(周期性,期内睡眠不计)") || strings.Contains(mergedCell, "单次最大") {
		t.Fatalf("merged periodic primary cell must take the discounted override, not the raw single-max:\n%s", mergedCell)
	}

	// Control: a non-periodic primary keeps the raw cumulative bytes.
	plain := periodicVS1Node()
	plain.PeriodicSource = false
	plainProj := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{plain},
	}
	plainModel := buildRuntimeTraceProjTreeModel(plainProj, nil, true)
	plainCell := runtimeTraceProjComparePrimaryCell(plainProj, plainModel, true)
	if !strings.Contains(plainCell, "36.361ms") || strings.Contains(plainCell, "周期性") {
		t.Fatalf("non-periodic primary cell must stay byte-stable:\n%s", plainCell)
	}
}

// periodicVS1ChainRow builds a data-bearing depth-1 chain row from the berlin
// periodic node with the given discounted attribution.
func periodicVS1ChainRow(effective float64) runtimeTraceProjTreeRow {
	node := periodicVS1Node()
	node.EffectiveImpactMS = effective
	return runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowChain, HasData: true, Depth: 1, Node: node,
	}
}

// TestRuntimeTraceProjPeriodicCoverageThreeShapes pins the F5 coverage lanes
// (adversarial review 2026-07-04), all three adjudicated shapes:
//   - 纯 0: attribution discounts to exactly 0 → the coverage sentence still
//     renders ("on-chain 已归因 0.000ms"), with the cadence third item;
//   - 0.176: partial attribution → sentence + cadence item (raw − effective);
//   - H10 不绕过: a data-bearing depth-1 periodic row at 0 must NOT trip the
//     "all transit" fallback into resurrecting a deeper raw cumulative.
func TestRuntimeTraceProjPeriodicCoverageThreeShapes(t *testing.T) {
	projection := types.TraceCausalProjection{WindowStartTs: 100.0, WindowEndTs: 100.05}

	pureZero := runtimeTraceProjTreeModel{
		WindowMS: 50.0, TreeRows: []runtimeTraceProjTreeRow{periodicVS1ChainRow(0)},
	}
	line := runtimeTraceProjWindowLine(projection, pureZero, true)
	if !strings.Contains(line, "on-chain 已归因 0.000ms") {
		t.Fatalf("zero-discount coverage sentence must not disappear:\n%s", line)
	}
	if !strings.Contains(line, "其中 36.361ms 为周期性信号源期内正常节拍(不计归因、不属未解释残差)。") {
		t.Fatalf("zero-discount coverage must carry the cadence third item:\n%s", line)
	}
	enLine := runtimeTraceProjWindowLine(projection, pureZero, false)
	if !strings.Contains(enLine, "On-chain attributed 0.000ms") ||
		!strings.Contains(enLine, "36.361ms is a periodic signal source's normal in-period cadence (not attributed, not unexplained residual)") {
		t.Fatalf("EN zero-discount coverage must mirror the zh shape:\n%s", enLine)
	}

	partial := runtimeTraceProjTreeModel{
		WindowMS: 50.0, TreeRows: []runtimeTraceProjTreeRow{periodicVS1ChainRow(0.176)},
	}
	line = runtimeTraceProjWindowLine(projection, partial, true)
	if !strings.Contains(line, "on-chain 已归因 0.176ms") {
		t.Fatalf("discounted attribution must feed the coverage numerator:\n%s", line)
	}
	if !strings.Contains(line, "其中 36.185ms 为周期性信号源期内正常节拍(不计归因、不属未解释残差)。") {
		t.Fatalf("cadence item must be raw − effective:\n%s", line)
	}

	deeper := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-21", Object: "runnable_wait",
		StateKind: "runnable", ImpactMS: 30.0, CumulativeImpactMS: 30.0,
		ChainRelevance: "on_chain", ChainDepth: 2,
	}
	h10 := runtimeTraceProjTreeModel{
		WindowMS: 50.0,
		TreeRows: []runtimeTraceProjTreeRow{
			periodicVS1ChainRow(0),
			{Kind: runtimeTraceProjTreeRowChain, HasData: true, Depth: 2, Node: deeper},
		},
	}
	if got := runtimeTraceProjDepth1Cumulative(h10); got != 0 {
		t.Fatalf("H10 fallback must not bypass a data-bearing depth-1 periodic row (got %.3f)", got)
	}
	// Control: with NO depth-1 data row the H10 fallback still reaches depth 2.
	transit := runtimeTraceProjTreeModel{
		WindowMS: 50.0,
		TreeRows: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowChain, HasData: true, Depth: 2, Node: deeper},
		},
	}
	if got := runtimeTraceProjDepth1Cumulative(transit); got != 30.0 {
		t.Fatalf("H10 fallback for genuine all-transit depth-1 must survive (got %.3f)", got)
	}
}
