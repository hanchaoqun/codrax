package tool

// answer_document_projection_disp3_test.go — DISP-3 显示面修复批 display pins
// (docs/design/real_trace_campaign_20260705.md §29.8/§29.10-3, 792 四场景回访
// witnesses cust_trace_{huadong,cmp,textup,opendir}_792.txt; 2026-07-09):
//
//   G6-b 链车道症状泄漏 (P1) — the (a) table's 有效归因 gate covered only the
//        rank-lane tier token; a SelfRows-lane target sleep row (predicate
//        wakeup_causal_impact, no tier) printed its engine effective
//        (huadong_792 E1 "6.661ms"; cmp_792 E2/E3 + proj2 E3/E5). The
//        chain-lane (depthless) tid-alias sleep row keeps its value — its
//        effective feeds the visible 承自 chain (cmp_792 proj2 E16 承自链需保).
//   §29.10-3 多投影排版 — multi-projection reports render every projection
//        tree section (lead + 关键指标) first, all 因果明细/证据索引 after;
//        block content bytes unchanged.
//   E22 ◇席窗标回归 — the rank-window chip survives the ×N multi-window merge
//        through the RankQueryWindow fallback.
//   累计(跨线程)词值 — the fallback stanza word never lands on a single-thread
//        stanza row (huadong_792 E22 / cmp_792 proj2 E23 / textup_792 E15-E17);
//        the ×N-merged rank shape speaks the §24.2 行3 equation
//        (有效归因 V = 单次最大(a–b,共N次)); the multi-thread R3 fold keeps
//        累计(跨线程) byte-identically.
//   E19 跨窗折叠漏拒% — an on-chain overflow fold whose members straddle query
//        windows suppresses the 占窗% (huadong_792 E19 "24%"); single-window
//        folds keep the legacy share.
//   E7 ⚠消失回归 — the ⚠ predicate compares the actual against the row's own
//        per-layer projection (equality-only dual-scope carve-out) and, on ×N
//        merged rows, against the per-instance MergedMaxMS (huadong_79 E8
//        "1.433ms ⚠" vs huadong_792 E7 identical四元组 no-⚠; cmp_792 E7
//        actual 5.957 vs ×4 SUM 11.804).
//   textup 覆盖句分母 — when the target's admitted state rows carry no
//        sleep-family member, the largest sleep-state hop view joins the
//        denominator (textup_792: 0.365ms rump beside the 108.500ms sleep hop
//        rendered "分母未覆盖…不给出覆盖百分比").
//   P2-① 拆解行"原始" — the inversion running component's 原始 speaks the
//        engine supply-fold raw, never the row's whole-window display value
//        (cmp_792 E8 detail block: 拆解 "running 原始 1.392ms" vs 供给折算
//        "running 原始 2.681ms" — one block, two contradicting raws; G7
//        词值同源的拆解行漏面).
//
// Mutation self-checks (each verified RED during development, then restored):
//   M-1: dropping the G6-b chain-lane arm in the (a) table →
//        TestDisp3SelfSleepAttributionCellDash red.
//   M-2: reverting runtimeTraceProjCrossWindow to the max(impact,cum) baseline
//        → TestDisp3CrossWindowWarnRestoration red (both the E7 pair pin and
//        the merged-row arm).
//   M-3: dropping the single-thread fork in the stanza effective lane →
//        TestDisp3SingleThreadStanzaCaliberWord red.
//   M-4: dropping the sleep-hop admission arm →
//        TestDisp3CoverageDenominatorAdmitsSleepHop red.
//   M-5: reverting the multi-cluster head/tail reorder →
//        TestDisp3MultiProjectionTreesFirstDetailsAfter red.
//   M-6: dropping the chip's RankQueryWindow fallback →
//        TestDisp3RankWindowChipSurvivesMerge red.
//   M-9: reverting the P2-① raw priority (display-impact arm back above the
//        engine fold raw) → TestDisp3DecompositionRawUsesEngineRaw red.
//   M-10: removing the 复核 P2-1 donor carve-out (merged branch back to the
//        bare MergedMaxMS comparison) → TestDisp3CrossWindowWarnRestoration
//        red on the dual-scope-seed REPRO and the absence-suppression arm.
//   M-11: silencing the 复核 P3-2 residue note →
//        TestDisp3CoverageDenominatorAdmitsSleepHop red on the two-hop form.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- G6-b: 链车道自身 sleep 行有效归因泄漏 --------------------------------------

// disp3SelfSleepProjection mirrors the huadong_792/cmp_792 shape: the target's
// own sleep row arrives through the wakeup_causal_impact lane (Role causal_hop,
// no tier token), an alias-named (tid-matched, label-mismatched) sleep row sits
// on the depthless chain lane feeding a 承自 chain, and a self io_wait 自因行
// competes normally.
func disp3SelfSleepProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "oney.hmn.berlin-42591"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.201,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// trunk depth-1 row (worker-9) keeps the tree alive.
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
				ChainDepth: 1, ImpactMS: 30, CumulativeImpactMS: 30, Confidence: 0.8},
			// the target's own sleep hop view (G6-b leak witness: effective
			// printed 6.661ms in the (a) 有效归因 column).
			{Role: types.TraceCausalRoleCausalHop, Subject: "oney.hmn.berlin-42591",
				Object: "sleep_wait", StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: 63.541, CumulativeImpactMS: 63.541, EffectiveImpactMS: 6.661,
				Confidence: 0.7},
			// the target's own io_wait 自因行 (§24.17 自因族) keeps its value.
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "oney.hmn.berlin-42591",
				Object: "io_wait", StateKind: "io_wait", ChainRelevance: "on_chain",
				ImpactMS: 1.062, CumulativeImpactMS: 1.062, EffectiveImpactMS: 1.062,
				Confidence: 0.7},
			// tid-alias sleep row on the depthless chain lane (cmp_792 proj2
			// E16: label ≠ target label, effective feeds E17's 承自 chain).
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "com.alias.name-42591",
				Object: "sleep_wait", StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: 13.054, CumulativeImpactMS: 13.054, EffectiveImpactMS: 13.054,
				Confidence: 0.7},
			// 承自 consumer (cmp_792 proj2 E17 form): effective inherited from
			// the enclosing wait interval — must keep its annotated value.
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "APM6-LIGHT_WEIG-21678",
				Object: "sleep_wait", StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: 11.045, CumulativeImpactMS: 11.045, EffectiveImpactMS: 13.054,
				Confidence: 0.7},
		},
	}
}

func disp3TableCell(t *testing.T, rows []types.AnswerBlockItem, nameSub string) []string {
	t.Helper()
	for _, row := range rows {
		if strings.Contains(row.Cells[0], nameSub) {
			return row.Cells
		}
	}
	t.Fatalf("no (a) table row matching %q", nameSub)
	return nil
}

func TestDisp3SelfSleepAttributionCellDash(t *testing.T) {
	projection := disp3SelfSleepProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	_, rows := runtimeTraceProjDetailTable(model, true)

	// The SelfRows-lane target sleep row: 有效归因 cell "—" (G6-b repair; the
	// column's own definition is 计入根因排序, which this row never does).
	selfSleep := disp3TableCell(t, rows, "oney.hmn.berlin-42591 / sleep")
	if selfSleep[3] != "—" {
		t.Fatalf("target self sleep row must dash the attribution cell (G6-b): %q", selfSleep[3])
	}
	// Its 窗口投影 stays intact — only the attribution claim is repaired.
	if selfSleep[1] != "63.541ms" {
		t.Fatalf("window projection must stay: %q", selfSleep[1])
	}
	// The 自因族 io_wait self row keeps its attribution (§24.17 — 勿一刀切).
	ioRow := disp3TableCell(t, rows, "oney.hmn.berlin-42591 / io")
	if ioRow[3] != "1.062ms" {
		t.Fatalf("self io_wait 自因行 must keep its attribution: %q", ioRow[3])
	}
	// The tid-alias chain-lane sleep row keeps its value (E16 承自链需保).
	alias := disp3TableCell(t, rows, "com.alias.name-42591")
	if alias[3] != "13.054ms" {
		t.Fatalf("chain-lane alias sleep row must keep its attribution: %q", alias[3])
	}
	// The 承自 consumer keeps its annotated inherited value.
	inherited := disp3TableCell(t, rows, "APM6-LIGHT_WEIG-21678")
	if !strings.Contains(inherited[3], "13.054ms") || !strings.Contains(inherited[3], "承自等待区间") {
		t.Fatalf("承自 chain consumer must keep the annotated value: %q", inherited[3])
	}
	// The gated legend line rides the repair (word and legend move together).
	flags := runtimeTraceProjDetailTableLegendFlagsFor(model, true)
	if !flags.selfSymptom {
		t.Fatalf("selfSymptom legend flag must fire for the chain-lane arm: %+v", flags)
	}
	cluster := runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{})
	joined := ""
	for _, block := range cluster {
		joined += block.Text + "\n"
	}
	if !strings.Contains(joined, "关注线程自身的等待症状行不参与根因排序") {
		t.Fatalf("the gated (a)-table legend line must render:\n%s", joined)
	}
}

// --- §29.10-3: 多投影排版(树先、明细后) ----------------------------------------

func TestDisp3MultiProjectionTreesFirstDetailsAfter(t *testing.T) {
	got := compareProjApply(t, compareProjBus(true))
	type seat struct {
		id   string
		kind string // head | tail
	}
	var order []seat
	for _, block := range got.Blocks {
		if !strings.HasPrefix(block.ID, "runtime_trace_causal_projection_a") {
			continue
		}
		kind := "head"
		if strings.HasSuffix(block.ID, "_detail_full") || strings.HasSuffix(block.ID, "_evidence") {
			kind = "tail"
		}
		order = append(order, seat{block.ID, kind})
	}
	if len(order) < 4 {
		t.Fatalf("expected per-artifact sections: %+v", order)
	}
	// Every head (lead + 关键指标) must precede every tail (明细 + 证据索引).
	seenTail := false
	for _, s := range order {
		if s.kind == "tail" {
			seenTail = true
			continue
		}
		if seenTail {
			t.Fatalf("§29.10-3: projection tree/key-metric block %q renders after a detail block: %+v", s.id, order)
		}
	}
	// Artifact order preserved within each group (依次).
	headIDs, tailIDs := []string{}, []string{}
	for _, s := range order {
		if s.kind == "head" {
			headIDs = append(headIDs, s.id)
		} else {
			tailIDs = append(tailIDs, s.id)
		}
	}
	// EVOLUTION RECORD (审计 #63/#6 回裁, §29.25 处置委托 + §29.26 待主会话
	// 落账, 2026-07-10) — round-trip record. §29.10-3 用户裁定原文: "投影树
	// (含头/覆盖句/关键指标)依次全部优先显示,因果明细依次殿后" (the 关键指标
	// table is INSIDE each projection's priority unit; §29.18 ② 验收句 "各投影
	// lead+关键指标依次"). The DISP-3 as-built pinned the paired order
	// (a1,a1_detail,a2,a2_detail); a remote batch (e920a5d8) flipped this pin
	// to the three-tier split (a1,a2,a1_detail,a2_detail) without citing a
	// §29.10-3 re-adjudication; restored here. Any future flip of wantHeads
	// MUST cite a user re-ruling of §29.10-3.
	wantHeads := []string{
		"runtime_trace_causal_projection_a1", "runtime_trace_causal_projection_a1_detail",
		"runtime_trace_causal_projection_a2", "runtime_trace_causal_projection_a2_detail",
	}
	if strings.Join(headIDs, ",") != strings.Join(wantHeads, ",") {
		t.Fatalf("head group must keep the §29.10-3 paired order (各投影 lead+关键指标 成对依次): %v", headIDs)
	}
	for i := 1; i < len(tailIDs); i++ {
		if strings.Compare(tailIDs[i-1], tailIDs[i]) > 0 &&
			strings.HasPrefix(tailIDs[i-1], "runtime_trace_causal_projection_a2") &&
			strings.HasPrefix(tailIDs[i], "runtime_trace_causal_projection_a1") {
			t.Fatalf("tail group must keep artifact order: %v", tailIDs)
		}
	}
}

// --- E22: ◇席窗标 survives the multi-window merge --------------------------------

func disp3ChipProjection() types.TraceCausalProjection {
	// Seed = the rank-2 member (production huadong_792 E22 shape: the merged
	// row's effective is the per-instance max the rank lane counts).
	merged := types.TraceCausalProjectionMergeOccurrenceRows([]types.TraceCausalProjectionNode{
		{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "m2",
			Subject: "oney.hmn.berlin-42591", Object: "trace_span",
			SpanName: "H:ReceiveVsync", ChainRelevance: "adjacent",
			ImpactMS: 9.169, CumulativeImpactMS: 9.169, EffectiveImpactMS: 9.169,
			Rank: 2, Confidence: 0.7,
			QueryWindowStartTs: 6793222.700, QueryWindowEndTs: 6793222.901},
		{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "m1",
			Subject: "oney.hmn.berlin-42591", Object: "trace_span",
			SpanName: "H:ReceiveVsync", ChainRelevance: "adjacent",
			ImpactMS: 8.611, CumulativeImpactMS: 8.611, EffectiveImpactMS: 8.611,
			Rank: 5, Confidence: 0.7,
			QueryWindowStartTs: 6793224.299, QueryWindowEndTs: 6793224.501},
	})
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "target-1"},
		WindowStartTs: 6793222.700,
		WindowEndTs:   6793222.901,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				ChainDepth: 1, ImpactMS: 12.444, CumulativeImpactMS: 12.444,
				EffectiveImpactMS: 5.071, Rank: 1, Confidence: 0.8,
				QueryWindowStartTs: 6793224.299, QueryWindowEndTs: 6793224.501},
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "sysmgr-reclaim0-8",
				Object: "running", StateKind: "running", ChainRelevance: "on_chain",
				ImpactMS: 2.770, CumulativeImpactMS: 2.770, EffectiveImpactMS: 0.813,
				Rank: 1, Confidence: 0.8,
				QueryWindowStartTs: 6793222.700, QueryWindowEndTs: 6793222.901},
		},
		AdjacentCauses: []types.TraceCausalProjectionNode{merged},
	}
}

func TestDisp3RankWindowChipSurvivesMerge(t *testing.T) {
	projection := disp3ChipProjection()
	if projection.AdjacentCauses[0].QueryWindowStartTs != 0 {
		t.Fatalf("fixture premise: the merge must zero the row-level window")
	}
	if projection.AdjacentCauses[0].RankQueryWindowStartTs != 6793222.700 {
		t.Fatalf("fixture premise: the merge must keep the rank member's window")
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "根因排序#2·窗6793222.700–6793222.901s") {
		t.Fatalf("the merged ◇ seat must keep its rank-window chip (E22 回归修):\n%s", fence)
	}
}

// --- 累计(跨线程) 词值: 单线程区段行禁跨线程词 -----------------------------------

func disp3StanzaProjection() types.TraceCausalProjection {
	projection := disp3ChipProjection()
	// A single-thread ◇ trace-span row whose effective equals its projection
	// (cmp_792 proj2 E23 / textup_792 E15 form).
	projection.AdjacentCauses = append(projection.AdjacentCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e23",
		Subject: "com.xs.fm.lite-21538", Object: "trace_span", SpanName: "bindApplication",
		ChainRelevance: "adjacent", ImpactMS: 150.0, CumulativeImpactMS: 150.0,
		EffectiveImpactMS: 150.0, Rank: 8, Confidence: 0.7,
		QueryWindowStartTs: 6793222.700, QueryWindowEndTs: 6793222.901})
	// A multi-thread R3 fold keeps the cross-thread word (the word is exactly
	// right there).
	projection.BackgroundCauses = append(projection.BackgroundCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "r3",
		ChainRelevance: "background", MergedCount: 2, MergedMinMS: 4.708, MergedMaxMS: 6.605,
		MergedSubjects: []string{"binder:8815_2-6583", "network-29126"},
		ImpactMS:       6.605, CumulativeImpactMS: 9.9, EffectiveImpactMS: 9.9, Confidence: 0.6})
	return projection
}

func TestDisp3SingleThreadStanzaCaliberWord(t *testing.T) {
	projection := disp3StanzaProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "累计(跨线程)") &&
			(strings.Contains(line, "150.000") || strings.Contains(line, "9.169")) {
			t.Fatalf("单线程区段行 must not wear the cross-thread word: %q", line)
		}
	}
	// The ×N-merged rank shape speaks the §24.2 行3 equation with the existing
	// closed-set caliber word (huadong_792 E22 repaired form).
	if !strings.Contains(fence, "有效归因 9.169ms = 单次最大(8.611–9.169ms,共2次)") {
		t.Fatalf("merged single-thread stanza rank row must speak the 单次最大 equation:\n%s", fence)
	}
	// equal-value single-thread stanza row: no second tag (the main number is
	// the value; the (a) table keeps the attribution).
	if strings.Contains(fence, "有效归因150.000ms") || strings.Contains(fence, "有效归因 150.000ms") {
		t.Fatalf("equal-value stanza row must not repeat its main number:\n%s", fence)
	}
	// The multi-thread R3 fold keeps 累计(跨线程) byte-identically.
	if !strings.Contains(fence, "累计(跨线程)9.900ms") {
		t.Fatalf("multi-thread fold keeps the cross-thread word:\n%s", fence)
	}
	// Legend entries follow the words that rendered.
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "`单次最大(a–b,共N次)`") {
		t.Fatalf("the 单次最大 legend entry must ride the equation:\n%s", lead)
	}
}

// --- E19: 跨窗折叠漏拒% -----------------------------------------------------------

func disp3OverflowFoldProjection(windows int) types.TraceCausalProjection {
	fold := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e19",
		ChainRelevance: "on_chain", OnChainOverflowFold: true,
		MergedCount: 11, MergedMinMS: 5.335, MergedMaxMS: 48.518,
		ImpactMS: 48.518, CumulativeImpactMS: 48.518,
		MergedSubjects: []string{"OS_mmi_EventHdr-43103", "VSyncGenerator-2270"},
		Confidence:     0.6,
	}
	fold.MergedQueryWindows = []types.TraceCausalProjectionQueryWindow{
		{StartTs: 6793222.700, EndTs: 6793222.901},
	}
	if windows > 1 {
		fold.MergedQueryWindows = append(fold.MergedQueryWindows,
			types.TraceCausalProjectionQueryWindow{StartTs: 6793222.031, EndTs: 6793225.370})
	}
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "target-1"},
		WindowStartTs: 6793222.700,
		WindowEndTs:   6793222.901,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
				ChainDepth: 1, ImpactMS: 30, CumulativeImpactMS: 30, Confidence: 0.8},
			fold,
		},
	}
}

func TestDisp3OverflowFoldCrossWindowNoShare(t *testing.T) {
	// Cross-window fold: the 占窗% is suppressed (§21.1 CWD-2 ① gate now sees
	// the fold through its member window roster) and the legend teaches why.
	model := buildRuntimeTraceProjTreeModel(disp3OverflowFoldProjection(2), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	foldLine := ""
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "链上折叠") {
			foldLine = line
			break
		}
	}
	if foldLine == "" {
		t.Fatalf("fold row missing:\n%s", fence)
	}
	if strings.Contains(foldLine, "%") {
		t.Fatalf("cross-window fold must not publish a 占窗%% (E19 漏拒%%修): %q", foldLine)
	}
	lead := runtimeTraceProjLeadText(disp3OverflowFoldProjection(2), model, "zh", true)
	if !strings.Contains(lead, "多窗合并行不显示占窗%") {
		t.Fatalf("the multi-window no-share legend entry must ride:\n%s", lead)
	}
	// Single-window fold keeps the legacy share byte-identically (24%).
	singleModel := buildRuntimeTraceProjTreeModel(disp3OverflowFoldProjection(1), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	singleFence := runtimeTraceProjTreeFence(singleModel, true)
	singleLine := ""
	for _, line := range strings.Split(singleFence, "\n") {
		if strings.Contains(line, "链上折叠") {
			singleLine = line
			break
		}
	}
	if !strings.Contains(singleLine, "24%") {
		t.Fatalf("single-window fold keeps the legacy share: %q", singleLine)
	}
}

// --- E7: ⚠ 消失回归 ---------------------------------------------------------------

func TestDisp3CrossWindowWarnRestoration(t *testing.T) {
	// huadong 回归 witness pair (79 E8 "1.433ms ⚠" / 792 E7 identical四元组
	// no-⚠): actual above the per-layer projection crosses the window even
	// when it sits below the chain total.
	e7 := types.TraceCausalProjectionNode{
		ImpactMS: 1.272, CumulativeImpactMS: 1.475, ActualImpactMS: 1.433,
	}
	if !runtimeTraceProjCrossWindow(e7) {
		t.Fatalf("E7 form (actual 1.433 > projection 1.272) must wear ⚠ again")
	}
	// dual-scope carve-out: actual duplicating the chain total is one
	// measurement — byte-stable no-⚠.
	dual := types.TraceCausalProjectionNode{
		ImpactMS: 5.0, CumulativeImpactMS: 8.0, ActualImpactMS: 8.0,
	}
	if runtimeTraceProjCrossWindow(dual) {
		t.Fatalf("dual-scope duplicate (actual == chain total) must not cross")
	}
	// cmp_792 E7 merged form (engine-cast through the ONE merge authority):
	// the ×4 SUM masked the member that crossed its own window — the
	// per-instance MergedMaxMS baseline restores the ⚠. The seed member's own
	// chain total (5.902) travels as the donor field; actual 5.957 is not its
	// duplicate, so the carve-out stays open.
	occ := func(impact, cum, actual float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role:    types.TraceCausalRoleRootCauseContext,
			Subject: "wk:1/1/0/8-6537", Object: "running", StateKind: "running",
			ImpactMS: impact, CumulativeImpactMS: cum, ActualImpactMS: actual,
			Confidence: 0.7,
		}
	}
	merged := types.TraceCausalProjectionMergeOccurrenceRows([]types.TraceCausalProjectionNode{
		occ(5.902, 5.902, 5.957), occ(1.127, 1.127, 0), occ(2.450, 2.450, 0), occ(2.325, 2.325, 0),
	})
	if !runtimeTraceProjRound3Equal(merged.ImpactMS, 11.804) || merged.MergedMaxMS != 5.902 || merged.ActualImpactMS != 5.957 {
		t.Fatalf("fixture premise drifted: impact=%.3f max=%.3f actual=%.3f", merged.ImpactMS, merged.MergedMaxMS, merged.ActualImpactMS)
	}
	if merged.MergedActualDonorCumulativeMS != 5.902 {
		t.Fatalf("the merge must carry the actual donor's pre-merge chain total: %.3f", merged.MergedActualDonorCumulativeMS)
	}
	if !runtimeTraceProjCrossWindow(merged) {
		t.Fatalf("merged row's actual (5.957) crosses its instance max (5.902) — ⚠ required")
	}
	// huadong_792 E1 counter-shape: actual equals the instance max — no ⚠.
	mergedFlat := types.TraceCausalProjectionMergeOccurrenceRows([]types.TraceCausalProjectionNode{
		occ(6.661, 6.661, 6.661), occ(1.035, 1.035, 0), occ(2.100, 2.100, 0),
	})
	if runtimeTraceProjCrossWindow(mergedFlat) {
		t.Fatalf("actual equal to the instance max does not cross")
	}
	// 复核 P2-1 REPRO (berlin E2 dual-scope seed): the seed's own actual ==
	// its own PRE-MERGE chain total (the no-⚠ dual-scope shape); the merge
	// overwrites the row cumulative with the SUM (46.300) and a 25.000 member
	// raises MergedMaxMS below the actual — pre-P2-1 the merged row wore a
	// fabricated ⚠. The donor-level carve-out keeps it no-⚠.
	dualSeed := types.TraceCausalProjectionMergeOccurrenceRows([]types.TraceCausalProjectionNode{
		occ(21.300, 27.900, 27.900), occ(25.000, 25.000, 0),
	})
	if !runtimeTraceProjRound3Equal(dualSeed.CumulativeImpactMS, 46.300) || dualSeed.MergedMaxMS != 25.000 || dualSeed.ActualImpactMS != 27.900 {
		t.Fatalf("REPRO premise drifted: cum=%.3f max=%.3f actual=%.3f", dualSeed.CumulativeImpactMS, dualSeed.MergedMaxMS, dualSeed.ActualImpactMS)
	}
	if runtimeTraceProjCrossWindow(dualSeed) {
		t.Fatalf("P2-1: a dual-scope seed must not wear a fabricated ⚠ after the merge")
	}
	// Absence arm (宁漏勿假): a merged row whose actual did not travel the
	// merge authority (no donor field) suppresses ⚠ outright.
	orphan := types.TraceCausalProjectionNode{
		MergedCount: 4, MergedMinMS: 1.127, MergedMaxMS: 5.902,
		ImpactMS: 11.804, CumulativeImpactMS: 11.804, ActualImpactMS: 5.957,
	}
	if runtimeTraceProjCrossWindow(orphan) {
		t.Fatalf("a merged row without the donor field must suppress ⚠ (conservative)")
	}
	// actual below projection stays byte-stable.
	inWindow := types.TraceCausalProjectionNode{
		ImpactMS: 108.5, CumulativeImpactMS: 108.5, ActualImpactMS: 108.5,
	}
	if runtimeTraceProjCrossWindow(inWindow) {
		t.Fatalf("actual == projection must not cross")
	}
}

// --- P2-①: 拆解行"原始"分量取行值非引擎 raw ---------------------------------------

// disp3DecompositionRawProjection mirrors the cmp_792 E8 shape: an inversion
// gated-composite running row whose whole-window display value (1.392 =
// runnable 0.856 + gated running 0.536) DIFFERS from the engine's supply-fold
// running raw (2.681 = ideal 1.969 + deficit 0.712). Pre-DISP-3 the 拆解子行
// printed the row value as "running 原始" while the 供给折算 line printed the
// fold raw — one detail block, two contradicting raws.
func disp3DecompositionRawProjection(withFold bool) types.TraceCausalProjection {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e8",
		Subject: "wk:1/1/0/8-46802", Object: "running", StateKind: "running",
		ChainRelevance: "on_chain", PriorityInversionCandidate: true,
		ImpactMS: 1.392, CumulativeImpactMS: 1.392, EffectiveImpactMS: 1.392,
		GatedRunnableMS: 0.856, GatedRunningDeficitMS: 0.536,
		Rank: 9, Confidence: 0.8,
	}
	if withFold {
		node.SupplyFoldComputed = true
		node.SupplyFoldKnownMS = 2.681
		node.SupplyFoldIdealMS = 1.969
		node.SupplyFoldDeficitMS = 0.712
	}
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "target-1"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.050,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				ChainDepth: 1, ImpactMS: 5, CumulativeImpactMS: 5, Confidence: 0.8},
			node,
		},
	}
}

func TestDisp3DecompositionRawUsesEngineRaw(t *testing.T) {
	projection := disp3DecompositionRawProjection(true)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "running 原始 2.681ms → 计入 0.536ms(折算,按下游消费核)") {
		t.Fatalf("拆解子行 must speak the engine fold raw (词值同源):\n%s", fence)
	}
	if strings.Contains(fence, "running 原始 1.392ms") {
		t.Fatalf("the row's whole-window display value must not impersonate the component raw:\n%s", fence)
	}
	// One block, ONE raw: the detail 拆解 line and the 供给折算 line agree.
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "running 原始 2.681ms → 计入 0.536ms(折算,按下游消费核)") {
		t.Fatalf("detail 拆解 line must carry the same engine raw:\n%s", detail)
	}
	if strings.Contains(detail, "running 原始 1.392ms") {
		t.Fatalf("the cmp_792 E8 contradiction (two raws in one block) must be dead:\n%s", detail)
	}
	// 行3 equation and the runnable component stay byte-identical.
	if !strings.Contains(fence, "有效归因 1.392ms = runnable(全额) 0.856ms + running(折算) 0.536ms") {
		t.Fatalf("行3 equation unchanged:\n%s", fence)
	}
	if !strings.Contains(fence, "runnable 原始 0.856ms → 计入 0.856ms(全额)") {
		t.Fatalf("runnable sub-row unchanged:\n%s", fence)
	}
	// No-fold fallback counterpart: without fold accounting the running-state
	// display value remains the only known raw (legacy byte-stable).
	bare := disp3DecompositionRawProjection(false)
	bareFence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(bare, newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if !strings.Contains(bareFence, "running 原始 1.392ms → 计入 0.536ms(折算,按下游消费核)") {
		t.Fatalf("no-fold rows keep the display-impact fallback raw:\n%s", bareFence)
	}
}

// --- textup 覆盖句分母 --------------------------------------------------------------

func disp3TextupCoverageProjection(withStateRows bool) types.TraceCausalProjection {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"RenderThread-51342", "unyuan.app.chat-50820"},
		WindowStartTs: 15151.840,
		WindowEndTs:   15151.971,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// depth-1 chain row: the coverage numerator (链上单项最大 108.5).
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "RenderThread-51342",
				Object: "running", StateKind: "running", ChainRelevance: "on_chain",
				ChainDepth: 1, ImpactMS: 108.472, CumulativeImpactMS: 108.5, Confidence: 0.8},
			// the target's sleep exists ONLY as the hop view (textup form).
			{Role: types.TraceCausalRoleCausalHop, Subject: "unyuan.app.chat-50820",
				Object: "sleep_wait", StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: 108.5, CumulativeImpactMS: 108.5, Confidence: 0.7},
		},
	}
	if withStateRows {
		projection.OnChainCauses = append(projection.OnChainCauses,
			types.TraceCausalProjectionNode{
				Role: types.TraceCausalRoleRootCauseContext, Subject: "unyuan.app.chat-50820",
				Object: "io_wait", StateKind: "io_wait", ChainRelevance: "on_chain",
				ImpactMS: 0.345, CumulativeImpactMS: 0.345, Confidence: 0.7},
			types.TraceCausalProjectionNode{
				Role: types.TraceCausalRoleRootCauseContext, Subject: "unyuan.app.chat-50820",
				Object: "d_state", StateKind: "d_state", ChainRelevance: "on_chain",
				ImpactMS: 0.020, CumulativeImpactMS: 0.020, Confidence: 0.7})
	}
	return projection
}

func TestDisp3CoverageDenominatorAdmitsSleepHop(t *testing.T) {
	projection := disp3TextupCoverageProjection(true)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if strings.Contains(line, "仅计入分析窗内直接等待") {
		t.Fatalf("the rump-denominator census arm must not fire once the sleep hop is admitted:\n%s", line)
	}
	if !strings.Contains(line, "关注线程等待(sleep/D-state/runnable) 108.865ms") {
		t.Fatalf("the denominator must include the sleep hop (108.500 + 0.365):\n%s", line)
	}
	if !strings.Contains(line, "已归因 108.500ms") {
		t.Fatalf("the numerator stays the depth-1 chain caliber:\n%s", line)
	}
	// 复核 P3-2 negative half: the single-hop shape carries NO residue clause.
	if strings.Contains(line, "未计入分母") {
		t.Fatalf("single-hop admission must not emit a residue clause:\n%s", line)
	}
	// 复核 P3-2: a SECOND disjoint same-window sleep hop loses the MAX race —
	// its magnitude must not vanish silently under the 全称 percentage; the
	// existing census phrase is appended verbatim (wording only, denominator
	// and percentages untouched).
	twoHops := disp3TextupCoverageProjection(true)
	twoHops.OnChainCauses = append(twoHops.OnChainCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, Subject: "unyuan.app.chat-50820",
		Object: "sleep_wait_2", StateKind: "s_sleep", ChainRelevance: "on_chain",
		ImpactMS: 50.0, CumulativeImpactMS: 50.0, Confidence: 0.7})
	twoModel := buildRuntimeTraceProjTreeModel(twoHops, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	twoLine := runtimeTraceProjWindowLine(twoHops, twoModel, true)
	if !strings.Contains(twoLine, "关注线程等待(sleep/D-state/runnable) 108.865ms") {
		t.Fatalf("MAX admission keeps the largest hop only (denominator unchanged):\n%s", twoLine)
	}
	if !strings.Contains(twoLine, "另有 1 条关注线程状态行未计入分母(单项最大 50.000ms)。") {
		t.Fatalf("the MAX admission's silent loser must be disclosed (P3-2):\n%s", twoLine)
	}
	// Counter-shape (huadong/opendir form): NO state rows at all → the
	// symptom stays 0 and the hop-only info line renders byte-identically.
	pure := disp3TextupCoverageProjection(false)
	pureModel := buildRuntimeTraceProjTreeModel(pure, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	pureLine := runtimeTraceProjWindowLine(pure, pureModel, true)
	if !strings.Contains(pureLine, "关注线程睡眠 108.500ms 中 108.500ms 已由链上解释") {
		t.Fatalf("the hop-only shape keeps its legacy info line:\n%s", pureLine)
	}
	if strings.Contains(pureLine, "关注线程等待(sleep/D-state/runnable) 108.500ms 中") {
		t.Fatalf("symptom==0 shapes must not gain a symptom denominator:\n%s", pureLine)
	}
}
