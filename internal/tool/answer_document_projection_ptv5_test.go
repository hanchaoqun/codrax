package tool

// answer_document_projection_ptv5_test.go — PTV5 #68 (用户裁定 2026-07-05)
// display pins, each with its 突变形态:
//
//   C00 — a fallback-sourced main-line ms carries its (a)-table caliber word
//         inline, publishes NO window-share percentage, and never fires the
//         占窗>100% mark on a fallback value (虚假触发面).
//   Q1  — 有效归因 常显: every data row with a positive effective attribution
//         carries the value tag (no double print on periodic / inherited /
//         effective-sourced rows).
//   Q2  — hop-only 形态 info line 目标睡眠 X 中 Y 已由链上解释 (arithmetic
//         untouched; absent when a state-view symptom exists).
//   Q3  — 树头 N-查询窗 declaration line; metric-snapshot per-window grouping;
//         single-artifact multi-anchor-window next-step branch.
//   PTS — the on-chain overflow fold row renders 其余 N 项(链上折叠) with the
//         member roster and never leads the RN-3(a) fallback conclusion.
//   Q4  — inversion candidacy via the typed field words the shape cell;
//         runnable action word = 调度等待 (never the inversion-colliding
//         调度/优先级).
//   C22 — a periodic-source conclusion renders 有效归因 0.000ms(期内节拍已折算)
//         explicitly (0 IS the finding).
//   +   — consolidated PTV5 wording verbatims (C06/C15/C39/C41/C42).

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
	runewidth "github.com/mattn/go-runewidth"
)

// --- C00: fallback caliber word + % suppression -------------------------------

func TestPTV5FallbackCaliberWordAndShareSuppression(t *testing.T) {
	marks := &runtimeTraceProjMarkSet{}
	row := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowChain, HasData: true, Node: types.TraceCausalProjectionNode{
		Subject: "w-1", Object: "runnable_wait", StateKind: "runnable",
		CumulativeImpactMS: 250.0, // NO ImpactMS → cumulative fallback, > window
	}}
	row.marks = marks
	base, tags := runtimeTraceProjRowMetricParts(row, 100.0, true, true)
	if strings.Contains(base, "%") {
		t.Fatalf("a fallback-sourced value must not publish a window share:\n%s", base)
	}
	if marks.has(runtimeTraceProjMarkOverWindowShare) {
		t.Fatalf("the 占窗>100%% mark must never fire on a fallback value (虚假触发面)")
	}
	if !marks.has(runtimeTraceProjMarkImpactCaliberFallback) {
		t.Fatalf("the fallback caliber mark must record for the legend entry")
	}
	var caliber *runtimeTraceProjTag
	for i := range tags {
		if tags[i].Text == "链上累计" {
			caliber = &tags[i]
		}
	}
	if caliber == nil || !caliber.MainRow {
		t.Fatalf("the caliber word must ride the main line next to the number: %+v", tags)
	}

	// 突变形态: a real window projection keeps the % and carries no caliber word.
	marks2 := &runtimeTraceProjMarkSet{}
	row2 := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowChain, HasData: true, Node: types.TraceCausalProjectionNode{
		Subject: "w-1", Object: "runnable_wait", StateKind: "runnable", ImpactMS: 50.0,
	}}
	row2.marks = marks2
	base2, tags2 := runtimeTraceProjRowMetricParts(row2, 100.0, true, true)
	if !strings.Contains(base2, "50%") {
		t.Fatalf("a window-projection value keeps its share:\n%s", base2)
	}
	if marks2.has(runtimeTraceProjMarkImpactCaliberFallback) {
		t.Fatalf("no fallback → no caliber mark")
	}
	for _, tag := range tags2 {
		if tag.Text == "链上累计" || tag.Text == "实际状态" || tag.Text == "有效归因" {
			t.Fatalf("no fallback → no bare caliber word tag: %+v", tags2)
		}
	}
	// Actual-state fallback names its own caliber (EN face included).
	marks3 := &runtimeTraceProjMarkSet{}
	row3 := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowChain, HasData: true, Node: types.TraceCausalProjectionNode{
		Subject: "w-1", Object: "runnable_wait", StateKind: "runnable", ActualImpactMS: 40.0,
	}}
	row3.marks = marks3
	_, tags3 := runtimeTraceProjRowMetricParts(row3, 100.0, true, false)
	found := false
	for _, tag := range tags3 {
		if tag.Text == "actual state" {
			found = true
		}
	}
	if !found {
		t.Fatalf("actual-state fallback must name its caliber: %+v", tags3)
	}
}

// --- Q1: 有效归因常显 tag ------------------------------------------------------

func TestPTV5EffectiveAttributionTagAlwaysOnPositive(t *testing.T) {
	marks := &runtimeTraceProjMarkSet{}
	row := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowChain, HasData: true, Node: types.TraceCausalProjectionNode{
		Subject: "w-1", Object: "runnable_wait", StateKind: "runnable",
		ImpactMS: 50.0, CumulativeImpactMS: 50.0, EffectiveImpactMS: 42.5,
	}}
	row.marks = marks
	_, tags := runtimeTraceProjRowMetricParts(row, 100.0, true, true)
	joined := ""
	for _, tag := range tags {
		joined += tag.Text + " · "
	}
	if !strings.Contains(joined, "有效归因42.500ms") {
		t.Fatalf("positive effective attribution must render on the row (Q1 常显):\n%s", joined)
	}

	// 突变形态 1: zero effective → no tag.
	row.Node.EffectiveImpactMS = 0
	row.marks = &runtimeTraceProjMarkSet{}
	_, tags = runtimeTraceProjRowMetricParts(row, 100.0, true, true)
	for _, tag := range tags {
		if strings.Contains(tag.Text, "有效归因") {
			t.Fatalf("zero effective must not tag: %+v", tags)
		}
	}
	// 突变形态 2: a periodic row keeps ONE carrier (the VS-1 tag), never two.
	periodic := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowChain, HasData: true, Node: types.TraceCausalProjectionNode{
		Subject: "vsync-1", Object: "sleep_wait", StateKind: "s_sleep",
		ImpactMS: 36.0, EffectiveImpactMS: 0.176, PeriodicSource: true, DetectedPeriodMS: 8.3,
	}}
	periodic.marks = &runtimeTraceProjMarkSet{}
	_, tags = runtimeTraceProjRowMetricParts(periodic, 100.0, true, true)
	count := 0
	for _, tag := range tags {
		if strings.Contains(tag.Text, "有效归因") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("a periodic row must carry exactly ONE 有效归因 carrier (the VS-1 tag): %+v", tags)
	}
}

// --- Q1 upstream: wakeup_causal_impact effective note --------------------------

func TestPTV5WakeupCausalImpactEmitsEffectiveNote(t *testing.T) {
	records := traceQueryTypedObservations(traceNoteKeysEmitFixtureResult(), "full.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	var impactNotes []string
	var foldSeen bool
	for _, record := range records {
		if record.Predicate != "wakeup_causal_impact" {
			continue
		}
		if record.ClaimKey == "wakeup_causal_impact:folded_overflow" {
			foldSeen = true
			joined := strings.Join(record.RichNotes, "\n")
			for _, want := range []string{"folded_rows=", "folded_min_ms=", "folded_max_ms=", "folded_subjects=", "chain_relevance=on_chain"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("fold record must carry the typed fold accounting (%s):\n%s", want, joined)
				}
			}
			continue
		}
		impactNotes = append(impactNotes, strings.Join(record.RichNotes, "\n"))
	}
	if !foldSeen {
		t.Fatalf("over-cap causal impacts must emit the PTS fold record (零静默丢弃)")
	}
	if len(impactNotes) == 0 {
		t.Fatalf("fixture must emit per-row causal impact records")
	}
	// The fixture's detailed first impact is PERIODIC: exactly one
	// effective_impact_ms note (the VS-1 lane), value = the discounted one.
	first := impactNotes[0]
	if got := strings.Count(first, "effective_impact_ms="); got != 1 {
		t.Fatalf("periodic impact row must carry exactly one effective note, got %d:\n%s", got, first)
	}
	// The synthetic overflow rows below the cap are plain non-periodic hops:
	// effective mirrors the rank lane's cumulative backfill (TotalMs — 复核
	// Med 真镜像 2026-07-06; the fixture rows carry TotalMs=2).
	if len(impactNotes) < 2 {
		t.Fatalf("expected capped per-row impact records")
	}
	if !strings.Contains(impactNotes[1], "effective_impact_ms=2.000") {
		t.Fatalf("a plain hop must publish the rank-lane effective (TotalMs backfill):\n%s", impactNotes[1])
	}
}

// --- Q2: hop-only info line ----------------------------------------------------

func ptv5HopOnlyModel(withStateRow bool) (types.TraceCausalProjection, runtimeTraceProjTreeModel) {
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
	}
	self := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-self",
		Subject: "app-1", Object: "sleep_wait", StateKind: "s_sleep",
		ImpactMS: 120.0, Confidence: 0.8,
	}
	if withStateRow {
		self.Role = types.TraceCausalRoleRootCauseContext
	}
	projection.SupportingHops = []types.TraceCausalProjectionNode{self}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	return projection, model
}

func TestPTV5HopOnlyCoverageInfoLine(t *testing.T) {
	projection, model := ptv5HopOnlyModel(false)
	if runtimeTraceProjTargetSymptomMS(model) != 0 {
		t.Skipf("fixture drifted: expected a hop-only self lane")
	}
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "目标睡眠 120.000ms 中 40.000ms 已由链上解释。") {
		t.Fatalf("hop-only shape must relate target sleep to the chain-explained share:\n%s", line)
	}
	en := runtimeTraceProjWindowLine(projection, model, false)
	if !strings.Contains(en, "Of the target's 120.000ms sleep, 40.000ms is explained on-chain.") {
		t.Fatalf("EN hop-only info line missing:\n%s", en)
	}
	// 突变形态: a state-view symptom row exists → the (a) variant renders and
	// the hop-only line stays out.
	projection2, model2 := ptv5HopOnlyModel(true)
	line2 := runtimeTraceProjWindowLine(projection2, model2, true)
	if strings.Contains(line2, "目标睡眠") && strings.Contains(line2, "已由链上解释") {
		t.Fatalf("state-view symptom shapes must not add the hop-only info line:\n%s", line2)
	}
}

// --- Q3: N-查询窗 head line ----------------------------------------------------

func TestPTV5QueryWindowDeclarationLine(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"dep-2", "app-1"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		QueryWindows: []types.TraceCausalProjectionQueryWindow{
			{StartTs: 100.0, EndTs: 100.2}, {StartTs: 200.0, EndTs: 203.0},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "- 本报告数据来自 2 个查询窗(本投影锚定其一);各窗指标见 Trace 指标快照(按查询窗分组)") {
		t.Fatalf("≥2 query windows must declare the count on the tree header:\n%s", line)
	}
	en := runtimeTraceProjWindowLine(projection, model, false)
	if !strings.Contains(en, "This report draws on 2 query windows") {
		t.Fatalf("EN declaration missing:\n%s", en)
	}
	// 突变形态: a single window keeps the header byte-identical (no line).
	projection.QueryWindows = projection.QueryWindows[:1]
	if strings.Contains(runtimeTraceProjWindowLine(projection, model, true), "个查询窗") {
		t.Fatalf("single-window compiles must not declare a window count")
	}
}

// --- Q3: single-artifact multi-anchor-window next-step branch -------------------

func TestPTV5MultiWindowNextStepBranch(t *testing.T) {
	obs := func(id string, line int, window string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "root_cause_context",
			ClaimKey: "root_cause_context:" + id, Subject: "app-1", Object: "runnable_wait",
			Value: "5.000", Unit: "ms",
			Span: types.ObservationSpan{LineStart: line, LineEnd: line + 5},
			SourceRef: types.ObservationSourceRef{
				Kind: types.ObservationSourceRuntimeArtifact, Path: "one.systrace", ArtifactKind: "trace",
			},
			RichNotes:  []string{"chain_relevance=on_chain", "impact_ms=5.000", "selected_window=" + window},
			Confidence: 0.8,
		}
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			obs("r1", 100, "100.000..101.000"),
			obs("r2", 300, "200.000..203.000"),
		},
	}}})
	steps := runtimeTraceNextStepMultiWindowSteps(ledger, true)
	if len(steps) != 2 ||
		steps[0] != "本 trace 含 2 个查询窗:窗长不同时先按各自窗长归一化(占窗比例)再跨窗对比" ||
		steps[1] != "双窗对比:对每个查询窗分别执行同口径因果采样(wakeup_chain/root_cause_rank)后逐窗对比" {
		t.Fatalf("single-artifact multi-window shape must emit the CMP-9 + per-window sampling rows: %+v", steps)
	}
	// 突变形态: one window → no branch.
	single := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{obs("r1", 100, "100.000..101.000")},
	}}})
	if got := runtimeTraceNextStepMultiWindowSteps(single, true); len(got) != 0 {
		t.Fatalf("single-window ledgers must not emit the multi-window rows: %+v", got)
	}
}

// --- PTS: fold row render + never-leads -----------------------------------------

func TestPTV5OnChainFoldRowRendersAndNeverLeads(t *testing.T) {
	projection := revisit76PTV5FoldCaliberProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "其余 3 项(链上折叠)(of-1、of-2 等)") {
		t.Fatalf("the fold row must name its lane, count and roster:\n%s", fence)
	}
	// The fold row (12ms) outweighs the real chain row's discounted values —
	// it must still never lead the RN-3(a) fallback lane.
	if lead := runtimeTraceProjLeadOnChainFallback(model); lead != nil && lead.OnChainOverflowFold {
		t.Fatalf("the fold roster must never lead a conclusion: %+v", lead)
	}
}

// --- Q4: inversion candidacy display + runnable action word ---------------------

func TestPTV5InversionCandidacyTypedFieldWordsShapeCell(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "w-1", Object: "s_sleep", StateKind: "runnable",
		PriorityInversionCandidate: true,
	}
	// PTV6-C ruling B (#73, 用户裁定 2026-07-06): the shape cell speaks the
	// cause FULL word (优先级反转候选 / raw token on EN) — the deleted 反转影响
	// shape word must never resurface (负向臂), and the cell must never claim
	// a single scheduler state for the gated composite.
	if got := runtimeTraceCausalProjectionImpactShapeCell(node, true); got != "优先级反转候选" {
		t.Fatalf("typed candidacy must word the shape cell with the cause full word: %q", got)
	}
	if got := runtimeTraceCausalProjectionImpactShapeCell(node, false); got != "priority_inversion_candidate" {
		t.Fatalf("EN typed candidacy must keep the raw cause token: %q", got)
	}
	for _, zh := range []bool{true, false} {
		if got := runtimeTraceCausalProjectionImpactShapeCell(node, zh); got == "反转影响" || got == "inversion impact" {
			t.Fatalf("deleted inversion shape word resurfaced: %q", got)
		}
	}
	node.PriorityInversionCandidate = false
	if got := runtimeTraceCausalProjectionImpactShapeCell(node, true); got == "优先级反转候选" || got == "反转影响" {
		t.Fatalf("no candidacy → no inversion claim, got %q", got)
	}
}

func TestPTV5RunnableActionWordSchedulingWait(t *testing.T) {
	node := types.TraceCausalProjectionNode{Subject: "w-1", Object: "runnable_wait", StateKind: "runnable"}
	if got := runtimeTraceCausalProjectionActionCell(node, true); got != "调度等待" {
		t.Fatalf("runnable action word must be 调度等待: %q", got)
	}
	if got := runtimeTraceCausalProjectionActionCell(node, false); got != "scheduling wait" {
		t.Fatalf("EN runnable action word must be scheduling wait: %q", got)
	}
	// 突变形态 (词表巧合消歧): the inversion-colliding word never returns.
	for _, zh := range []bool{true, false} {
		if got := runtimeTraceCausalProjectionActionCell(node, zh); strings.Contains(got, "优先级") || strings.Contains(got, "priority") {
			t.Fatalf("the runnable action word must not collide with inversion wording: %q", got)
		}
	}
}

// --- C22: periodic zero renders explicitly --------------------------------------

func TestPTV5PeriodicZeroConclusionRendersDiscountedZero(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"vsync-1", "app-1"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.1,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-p",
			Subject: "vsync-1", Object: "sleep_wait", StateKind: "s_sleep",
			ChainRelevance: "on_chain", Rank: 1,
			ImpactMS: 36.0, CumulativeImpactMS: 36.0, EffectiveImpactMS: 0,
			PeriodicSource: true, DetectedPeriodMS: 8.3, Confidence: 0.8,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "有效归因 0.000ms(期内节拍已折算)") {
		t.Fatalf("a pure-cadence periodic primary must state its discounted 0 explicitly:\n%s", line)
	}
	if strings.Contains(line, "36.000ms") {
		t.Fatalf("the raw cadence value must not resurrect on the headline:\n%s", line)
	}
	en := runtimeTraceProjConclusionLine(projection, model, false)
	if !strings.Contains(en, "attribution 0.000ms (in-period cadence discounted)") {
		t.Fatalf("EN periodic-zero conclusion missing:\n%s", en)
	}
}

// --- consolidated PTV5 wording verbatims ----------------------------------------

func TestPTV5SelfRowWordingShareGateAndBareChainEnd(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "app-1", Object: "sleep_wait", StateKind: "s_sleep",
		ImpactMS: 80.0, UndrillableReason: "missing_wakeup",
	}
	row := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowSelf, HasData: true, Node: node}
	row.marks = &runtimeTraceProjMarkSet{}
	// ≥50% share → 主要; the ⊘链止 marker stays bare (no typed enum on the panel).
	main, demoted := runtimeTraceProjSelfRowParts(row, 100.0, true)
	joined := strings.Join(append(append([]string{}, main...), demoted...), " ")
	if !strings.Contains(joined, "窗口内主要处于等待唤醒") {
		t.Fatalf("a ≥50%% sleep share keeps the 主要 wording: %s", joined)
	}
	if !strings.Contains(joined, "⊘链止") || strings.Contains(joined, "missing_wakeup") {
		t.Fatalf("the self row renders the bare ⊘链止 without the enum: %s", joined)
	}
	// 突变形态: a small share (or no window) drops the 主要 claim.
	row.Node.ImpactMS = 10.0
	main, demoted = runtimeTraceProjSelfRowParts(row, 100.0, true)
	joined = strings.Join(append(append([]string{}, main...), demoted...), " ")
	if strings.Contains(joined, "主要处于等待唤醒") || !strings.Contains(joined, "该段处于等待唤醒") {
		t.Fatalf("a small sleep share must keep the neutral wording: %s", joined)
	}
}

func TestPTV5ComparisonOverviewLeadDropsTypedJargon(t *testing.T) {
	// C15: the overview lead speaks no internal "typed" word on either face.
	obs := compareProjTwoTraceObs()
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: obs,
	}}})
	set := types.CompileTraceCausalProjectionSet(ledger)
	block := runtimeTraceProjCompareOverviewBlock(set.Projections, ledger, "zh", true)
	if block == nil {
		t.Fatalf("comparison fixture must build the overview block")
	}
	if !strings.Contains(block.Text, "跨 trace 对比总览:数值全部来自各工件独立投影的结构化字段") ||
		strings.Contains(block.Text, "typed") {
		t.Fatalf("zh overview lead must speak the structured-field wording without typed: %q", block.Text)
	}
	en := runtimeTraceProjCompareOverviewBlock(set.Projections, ledger, "en", false)
	if en == nil || strings.Contains(en.Text, "typed") {
		t.Fatalf("EN overview lead must not leak typed: %+v", en)
	}
}

func TestPTV5UnnamedNodeFallbackSpeaksChinese(t *testing.T) {
	node := types.TraceCausalProjectionNode{}
	if got := runtimeTraceProjDetailFullName(node, true); got != "(未命名因果节点)" {
		t.Fatalf("zh fallback name must speak zh: %q", got)
	}
	if got := runtimeTraceProjDetailFullName(node, false); got != "trace causal node" {
		t.Fatalf("EN fallback name unchanged: %q", got)
	}
}

// --- Q3: metric snapshot per-window grouping ------------------------------------

func TestPTV5MetricSnapshotGroupsByQueryWindow(t *testing.T) {
	bus := newBusForMutationTest()
	early := cmpbSnapshotObservation("late", "late-9", "3.000", "state_drilldown", "state_drilldown:late-9:s_sleep",
		"selected_window=200.000..203.000")
	late := cmpbSnapshotObservation("early", "early-7", "5.000", "state_drilldown", "state_drilldown:early-7:s_sleep",
		"selected_window=100.000..101.000")
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		// Ledger order puts the LATER window first — grouping must reorder
		// within the tier by ascending window start.
		Observations: []types.ObservationRecord{early, late},
	}}
	items := runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	if len(items) != 2 {
		t.Fatalf("expected both snapshot rows, got %+v", items)
	}
	if !strings.HasPrefix(items[0].Label, "查询窗 100.000–101.000s · early-7") ||
		!strings.HasPrefix(items[1].Label, "查询窗 200.000–203.000s · late-9") {
		t.Fatalf("multi-window snapshots must group by ascending record window: %+v", items)
	}
	// 突变形态: one distinct window → labels stay byte-identical (no prefix).
	sameA := cmpbSnapshotObservation("a", "one-1", "3.000", "state_drilldown", "state_drilldown:one-1:s_sleep",
		"selected_window=100.000..101.000")
	sameB := cmpbSnapshotObservation("b", "two-2", "5.000", "state_drilldown", "state_drilldown:two-2:s_sleep",
		"selected_window=100.000..101.000")
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{sameA, sameB}}}
	items = runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	for _, item := range items {
		if strings.Contains(item.Label, "查询窗") {
			t.Fatalf("single-window snapshots must not grow window prefixes: %+v", items)
		}
	}
}

// --- C33/C34/C35: gated (a)-legend rows + E#(+N) intro half-sentence -------------

func TestPTV5GatedDetailLegendRows(t *testing.T) {
	blocks := runtimeTraceCausalProjectionCluster(revisit76PTV4BadgeMergeProjection(), "zh", runtimeTraceProjUserFocus{})
	var detail *types.AnswerBlock
	for i := range blocks {
		if strings.HasSuffix(blocks[i].ID, "_detail") {
			detail = &blocks[i]
		}
	}
	if detail == nil {
		t.Fatalf("fixture must render the (a) detail table")
	}
	// The PTV4 fixture shows ×N sum, ×N max and ×N同值 forms → the gated
	// legend row lists exactly the present forms.
	if !strings.Contains(detail.Text, "- ×N(a–b) = N 次合并,数值为总和;×N(a–b)取最大 = 跨线程折叠,数值取成员最大(墙钟不求和);×N同值 = 同一测量重复发布,数值即那一次。") {
		t.Fatalf("×N legend row must list the present forms:\n%s", detail.Text)
	}
	// 突变形态: a plain projection carries neither gated row.
	plain := runtimeTraceCausalProjectionCluster(revisit76FlatUndrillableProjection(), "zh", runtimeTraceProjUserFocus{})
	for i := range plain {
		if strings.HasSuffix(plain[i].ID, "_detail") {
			if strings.Contains(plain[i].Text, "×N(a–b)") || strings.Contains(plain[i].Text, "双席") {
				t.Fatalf("plain tables must not grow the gated legend rows:\n%s", plain[i].Text)
			}
		}
	}
}

func TestPTV5EvidenceIntroExplainsMergedNotation(t *testing.T) {
	projection := revisit76PTV5FoldCaliberProjection()
	// The fold row carries merged evidence ids → the (+N) notation renders and
	// the intro grows its half-sentence. (Locators added locally so the
	// evidence index has entries at all.)
	for i := range projection.OnChainCauses {
		projection.OnChainCauses[i].LineStart = 1000 + i
		projection.OnChainCauses[i].LineEnd = 1005 + i
		projection.OnChainCauses[i].SupportRefs = []string{fmt.Sprintf("ptv5.systrace:%d-%d", 1000+i, 1005+i)}
	}
	projection.OnChainCauses[1].MergedEvidenceIDs = []string{"ptv5-fold-2", "ptv5-fold-3"}
	blocks := runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{})
	var evidence *types.AnswerBlock
	for i := range blocks {
		if strings.HasSuffix(blocks[i].ID, "_evidence") {
			evidence = &blocks[i]
		}
	}
	if evidence == nil {
		t.Fatalf("fixture must render the evidence index")
	}
	if !strings.Contains(evidence.Text, "E#(+N) 表示该行另合并了 N 条同类观测,合并明细见对应条目的审计 merged_ids。") {
		t.Fatalf("merged notation must be explained in the intro:\n%s", evidence.Text)
	}
	// 突变形态: no merged evidence anywhere → intro byte-identical (no half-sentence).
	plainBlocks := runtimeTraceCausalProjectionCluster(revisit76FlatUndrillableProjection(), "zh", runtimeTraceProjUserFocus{})
	for i := range plainBlocks {
		if strings.HasSuffix(plainBlocks[i].ID, "_evidence") {
			if strings.Contains(plainBlocks[i].Text, "E#(+N)") {
				t.Fatalf("merge-free rosters must not explain the notation:\n%s", plainBlocks[i].Text)
			}
		}
	}
}

// --- C11: flat head clause / C12: span class dedup / C41+C42 half-English -------

func TestPTV5FlatHeadClauseAndTrunkedHeadClause(t *testing.T) {
	flatProjection := revisit76FlatUndrillableProjection()
	flatModel := buildRuntimeTraceProjTreeModel(flatProjection, nil, true)
	runtimeTraceProjTreeFence(flatModel, true)
	flatLead := runtimeTraceProjLeadText(flatProjection, flatModel, "zh", true)
	if !strings.Contains(flatLead, "- 各行按层级平铺,不构成上下游链。") || strings.Contains(flatLead, "向上游追溯") {
		t.Fatalf("flat renders must not claim upstream tracing in the head clause:\n%s", flatLead)
	}
	trunked := revisit76RichChainProjection()
	trunkedModel := buildRuntimeTraceProjTreeModel(trunked, nil, true)
	runtimeTraceProjTreeFence(trunkedModel, true)
	trunkedLead := runtimeTraceProjLeadText(trunked, trunkedModel, "zh", true)
	if !strings.Contains(trunkedLead, "- 自上而下 = 从关注线程向上游追溯。") {
		t.Fatalf("trunked renders keep the upstream head clause:\n%s", trunkedLead)
	}
}

func TestPTV5SemanticSpanClassRendersOnceOnTreeRow(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleSemanticSpan, Subject: "app-1",
		SpanName: "VerifyClass x", SemanticClass: "class_verification",
		ImpactMS: 5.0,
	}
	row := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowSemantic, HasData: true, Node: node}
	row.marks = &runtimeTraceProjMarkSet{}
	_, tags := runtimeTraceProjRowMetricParts(row, 100.0, true, true)
	classCount := 0
	actionBare := false
	for _, tag := range tags {
		if strings.Contains(tag.Text, "class_verification") {
			classCount++
		}
		if tag.Text == "优化点" {
			actionBare = true
		}
	}
	if classCount != 1 || !actionBare {
		t.Fatalf("the class token renders ONCE (shape tag); the action cell trims to the bare word: %+v", tags)
	}
	// The detail (b) surface keeps the full action cell (明细两处照旧).
	if got := runtimeTraceCausalProjectionActionCell(node, true); got != "优化点·class_verification" {
		t.Fatalf("the lossless action cell keeps the class: %q", got)
	}
}

func TestPTV5LegendAndIntroDropHalfEnglishRoster(t *testing.T) {
	blocks := runtimeTraceCausalProjectionCluster(revisit76PTV4BadgeMergeProjection(), "zh", runtimeTraceProjUserFocus{})
	var detail, full *types.AnswerBlock
	for i := range blocks {
		if strings.HasSuffix(blocks[i].ID, "_detail") {
			detail = &blocks[i]
		}
		if strings.HasSuffix(blocks[i].ID, "_detail_full") {
			full = &blocks[i]
		}
	}
	if detail == nil || full == nil {
		t.Fatalf("fixture must render both detail surfaces")
	}
	if !strings.Contains(detail.Text, "×N 全部成员清单") || strings.Contains(detail.Text, "全 roster") {
		t.Fatalf("the (a) legend speaks zh for the roster pointer:\n%s", detail.Text)
	}
	if !strings.Contains(full.Text, "树内省略行清单") || strings.Contains(full.Text, "省略行 roster") {
		t.Fatalf("the (b) intro speaks zh for the omitted-row roster:\n%s", full.Text)
	}
}

// --- 复核批 (2026-07-06) pins -----------------------------------------------------

// TestPTV5FallbackRowsHoldRowCapFullWidthSweep (复核 P1 簇二): full-width
// sweep of the C00 fallback shapes at depths 0..7, zh+en, long CJK+pid name.
// The COMMON pair (caliber word + E#) holds the 100-cell cap at EVERY depth —
// the name budget reserves the word and the floor erodes with it (名字中截
// 让位). The HEAVY 跨窗回退行 shape (caliber + ⚠实际 + E#(+N)) holds the cap
// through zh depth 5 / en depth 3 and past that keeps the T1
// integrity-floor discipline (marks whole, recorded as-is) at the measured
// plateau — strictly better than the pre-batch ⚠+⊘+E# triple (zh peaked 109,
// en 118): zh 104@6/108@7, en 101@4/105@5/109@6/113@7. Ceilings may only
// DECREASE.
func TestPTV5FallbackRowsHoldRowCapFullWidthSweep(t *testing.T) {
	heavyCeiling := map[bool]map[int]int{
		true:  {6: 104, 7: 108},
		false: {4: 101, 5: 105, 6: 109, 7: 113},
	}
	build := func(depth int, withActual bool) runtimeTraceProjTreeRow {
		node := types.TraceCausalProjectionNode{
			Subject: "很长的中文线程名字组件服务进程处理器渲染管线-1234567",
			Object:  "runnable_wait", StateKind: "runnable",
			CumulativeImpactMS: 5234.567,
		}
		if withActual {
			node.ActualImpactMS = 6000.123
		}
		row := runtimeTraceProjTreeRow{
			Kind: runtimeTraceProjTreeRowChain, HasData: true, Node: node,
			Depth: depth, Indent: depth, Ancestors: make([]bool, depth),
			EvidenceTag: "E12(+3)",
		}
		for i := range row.Ancestors {
			row.Ancestors[i] = true
		}
		row.marks = &runtimeTraceProjMarkSet{}
		return row
	}
	mainLine := func(row runtimeTraceProjTreeRow, zh bool) string {
		line := runtimeTraceProjTreeRowLine(row, runtimeTraceProjTreeLabelColumnMax, 100.0, true, zh)
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			return line[:i]
		}
		return line
	}
	for _, zh := range []bool{true, false} {
		for depth := 0; depth <= 7; depth++ {
			// COMMON pair: strict cap at every depth.
			common := mainLine(build(depth, false), zh)
			if w := runewidth.StringWidth(common); w > runtimeTraceProjTreeRowMaxWidth {
				t.Fatalf("zh=%v depth=%d: COMMON fallback pair is %d cells (> %d):\n%q",
					zh, depth, w, runtimeTraceProjTreeRowMaxWidth, common)
			}
			// HEAVY 跨窗回退行: strict cap up to the measured boundary, the
			// quantified plateau past it (only-decrease).
			heavy := mainLine(build(depth, true), zh)
			cap := runtimeTraceProjTreeRowMaxWidth
			if ceiling, ok := heavyCeiling[zh][depth]; ok {
				cap = ceiling
			}
			if w := runewidth.StringWidth(heavy); w > cap {
				t.Fatalf("zh=%v depth=%d: HEAVY fallback row is %d cells (> %d):\n%q",
					zh, depth, w, cap, heavy)
			}
			if !strings.Contains(heavy, "链上累计") && !strings.Contains(heavy, "chain total") {
				t.Fatalf("zh=%v depth=%d: the caliber word must survive the width fit:\n%s", zh, depth, heavy)
			}
		}
	}
}

// TestPTV5SnapshotPerWindowFloor (复核 Med): when one window's candidates
// would monopolize the legacy 2 slots, the per-window floor still gives every
// window at least one row — the tree header's 按查询窗分组 claim stays true.
func TestPTV5SnapshotPerWindowFloor(t *testing.T) {
	bus := newBusForMutationTest()
	a1 := cmpbSnapshotObservation("a1", "busy-1", "9.000", "state_drilldown", "state_drilldown:busy-1:s_sleep",
		"selected_window=100.000..101.000")
	a2 := cmpbSnapshotObservation("a2", "busy-2", "8.000", "state_drilldown", "state_drilldown:busy-2:s_sleep",
		"selected_window=100.000..101.000")
	b1 := cmpbSnapshotObservation("b1", "quiet-3", "2.000", "state_drilldown", "state_drilldown:quiet-3:s_sleep",
		"selected_window=200.000..203.000")
	bus.ToolResults = []types.ToolResult{{
		ToolName: "trace_query", Success: true,
		// Window A's two candidates lead the ledger — the pre-floor selector
		// burned both slots on them and window B vanished (整窗缺席).
		Observations: []types.ObservationRecord{a1, a2, b1},
	}}
	items := runtimeTraceMetricSnapshotItems(cmpbSnapshotDoc(), bus)
	var winA, winB int
	for _, item := range items {
		if strings.Contains(item.Label, "查询窗 100.000–101.000s") {
			winA++
		}
		if strings.Contains(item.Label, "查询窗 200.000–203.000s") {
			winB++
		}
	}
	if winA < 1 || winB < 1 {
		t.Fatalf("every query window must keep at least one snapshot row (A=%d B=%d): %+v", winA, winB, items)
	}
}

// TestPTV5QueryWindowsTruncationRendersLowerBound (复核 Low): >8 distinct
// windows latch the truncated flag and every count face renders ≥8, never a
// fake exact number.
func TestPTV5QueryWindowsTruncationRendersLowerBound(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:            []string{"dep-2", "app-1"},
		WindowStartTs:         100.0,
		WindowEndTs:           100.2,
		QueryWindowsTruncated: true,
	}
	for i := 0; i < 8; i++ {
		projection.QueryWindows = append(projection.QueryWindows,
			types.TraceCausalProjectionQueryWindow{StartTs: 100 + float64(i)*10, EndTs: 101 + float64(i)*10})
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "本报告数据来自 ≥8 个查询窗") {
		t.Fatalf("truncated window list must render a lower bound:\n%s", line)
	}
	// 突变形态: untruncated keeps the exact count.
	projection.QueryWindowsTruncated = false
	if line := runtimeTraceProjWindowLine(projection, model, true); !strings.Contains(line, "本报告数据来自 8 个查询窗") || strings.Contains(line, "≥") {
		t.Fatalf("untruncated list keeps the exact count:\n%s", line)
	}
}

// TestPTV5BadgeNeverLandsOnOverflowFold (复核 Low): the typed gate — a fold
// row that somehow carries Rank + effective attribution still gets no ❶❷❸
// badge (the 账本 "永不" claim is a gate, not an incidental zero-field escape).
func TestPTV5BadgeNeverLandsOnOverflowFold(t *testing.T) {
	projection := revisit76PTV5FoldCaliberProjection()
	projection.OnChainCauses[1].Rank = 1
	projection.OnChainCauses[1].EffectiveImpactMS = 99.0
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	for _, row := range model.TreeRows {
		if row.Node.OnChainOverflowFold && row.Badge != 0 {
			t.Fatalf("the overflow fold row must never win a badge: %+v", row)
		}
	}
}

// TestPTV5FoldRowLosslessBlockNamesItsLane (复核 Low): the (b) block's full
// name for the fold row names the fold lane, never the anonymous fallback.
func TestPTV5FoldRowLosslessBlockNamesItsLane(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		OnChainOverflowFold: true, ChainRelevance: "on_chain",
		MergedCount: 3, MergedSubjects: []string{"of-1", "of-2"},
		CumulativeImpactMS: 12.0,
	}
	if got := runtimeTraceProjDetailFullName(node, true); got != "其余 3 项(链上折叠)(of-1、of-2 等)" {
		t.Fatalf("zh fold block name must name the lane: %q", got)
	}
	if got := runtimeTraceProjDetailFullName(node, false); !strings.Contains(got, "3 more (on-chain fold)") {
		t.Fatalf("EN fold block name must name the lane: %q", got)
	}
}

// TestPTV5CumulativeOnlyFoldRowCarriesCaliberWordNoShare (复核 P1 ②): a fold
// whose value came from a cumulative-only member renders the C00 caliber word
// and never a window share.
func TestPTV5CumulativeOnlyFoldRowCarriesCaliberWordNoShare(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		OnChainOverflowFold: true, ChainRelevance: "on_chain",
		MergedCount: 2, MergedMinMS: 3.0, MergedMaxMS: 55.0,
		CumulativeImpactMS: 55.0, // ImpactMS deliberately 0 — cumulative-source fold
		MergedSubjects:     []string{"cum-1"},
	}
	row := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowDepthless, HasData: true, Node: node}
	row.marks = &runtimeTraceProjMarkSet{}
	base, tags := runtimeTraceProjRowMetricParts(row, 100.0, true, true)
	if strings.Contains(base, "%") {
		t.Fatalf("cumulative-source fold must not publish a window share:\n%s", base)
	}
	found := false
	for _, tag := range tags {
		if tag.Text == "链上累计" {
			found = true
		}
	}
	if !found {
		t.Fatalf("cumulative-source fold must carry the caliber word: %+v", tags)
	}
}

// TestPTV5PeriodicEffectiveFallbackSingleCarrier (复核 Low 双 carrier 角): a
// periodic row whose display falls back to the EFFECTIVE lane keeps exactly
// ONE 有效归因 carrier (the VS-1 tag) — no C00 caliber word on top.
func TestPTV5PeriodicEffectiveFallbackSingleCarrier(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "vsync-1", Object: "sleep_wait", StateKind: "s_sleep",
		EffectiveImpactMS: 0.5, PeriodicSource: true, DetectedPeriodMS: 8.3,
		// No ImpactMS / CumulativeImpactMS → display falls back to Effective.
	}
	row := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowChain, HasData: true, Node: node}
	row.marks = &runtimeTraceProjMarkSet{}
	_, tags := runtimeTraceProjRowMetricParts(row, 100.0, true, true)
	carriers := 0
	for _, tag := range tags {
		if strings.Contains(tag.Text, "有效归因") {
			carriers++
		}
	}
	if carriers != 1 {
		t.Fatalf("periodic effective-fallback row must keep ONE carrier: %+v", tags)
	}
	if row.marks.has(runtimeTraceProjMarkImpactCaliberFallback) {
		t.Fatalf("no caliber word (and no legend entry) on the periodic effective fallback")
	}
}

// TestPTV5PeriodicNeverClaimsInheritedAttribution (复核 Low 互斥核查): the 10×
// inherited heuristic must not misfire on a periodic row — its effective is
// COMPUTED (VS-1), so the 承自归因 carrier never stacks next to the VS-1 tag.
func TestPTV5PeriodicNeverClaimsInheritedAttribution(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "vsync-1", Object: "sleep_wait", StateKind: "s_sleep",
		ImpactMS: 36.0, CumulativeImpactMS: 0.05, EffectiveImpactMS: 0.9,
		PeriodicSource: true, DetectedPeriodMS: 8.3,
	}
	if runtimeTraceProjEffectiveInherited(node) {
		t.Fatalf("a periodic row must never classify as inherited attribution")
	}
	row := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowChain, HasData: true, Node: node}
	row.marks = &runtimeTraceProjMarkSet{}
	_, tags := runtimeTraceProjRowMetricParts(row, 100.0, true, true)
	for _, tag := range tags {
		if strings.Contains(tag.Text, "承自归因") {
			t.Fatalf("no 承自归因 carrier on periodic rows: %+v", tags)
		}
	}
	// 突变形态: the same magnitudes WITHOUT the periodic flag stay inherited.
	node.PeriodicSource = false
	if !runtimeTraceProjEffectiveInherited(node) {
		t.Fatalf("the non-periodic 10× shape keeps the inherited classification")
	}
}
