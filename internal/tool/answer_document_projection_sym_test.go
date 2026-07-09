package tool

// answer_document_projection_sym_test.go — SYM 批 display pins (ledger
// real_trace_campaign_20260705.md §24.13 裁定一, user ruling 2026-07-08):
//
//   S2a (opendir_78 形) — the lead falls to the non-self board head (the
//        RxComputationT inversion row) while the target's self-held lock row
//        keeps its tree-head seat AND its rank ordinal (榜位序数照发).
//   S2b (cmp_78_01 双侧形, FLAT) — the shared rank board excludes typed
//        target-self rows even when the label-routed SelfRows lane cannot
//        engage (flat render: model.Target == "", the self binder row sits in
//        TreeRows); ❶ follows the board. 突变自查 M2: disabling the board
//        exclusion arm reds this (the self row is the eff-max board head).
//   S2c (全自身榜退化形) — every ranked row is the target's own symptom →
//        the honest fallback renders WITH the symptom disclosure; the legacy
//        no-rank-data shape stays byte-stable ("" conclusion).
//   S3  (语义回退可达性 + 零链括注) — the all-self shape leaves the primary
//        bucket empty, so the LEAD-SEM semantic lane is unreachable on the
//        conclusion (诚实回退 lane wins) while the compare cell's zero-chain
//        optimization presence note becomes reachable; the compare 主根因
//        column consumes the same single lead source.
//   S2d fold port — a folded self rank twin carries its typed tier onto the
//        surviving chain node together with the rank it annotates.
//   S2e display words — the self rank row speaks 关注线程自身 (UXA B#24
//        family) on the layer/position faces, never 主根因(优先处理).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// symOpendirProjection mirrors the opendir_78 shape AFTER the SYM engine half:
// the target's self-held AssetManager lock row wears the ladder-transparent
// target_self_state tier (rank#1 ordinal preserved), so the engine's primary
// slot fell to the RxComputationT inversion row (rank#2, eff 37.410).
func symOpendirProjection() types.TraceCausalProjection {
	inversion := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-inversion",
		Subject: "RxComputationT-16612", Object: "priority_inversion_candidate",
		Predicate: "root_cause_primary", Rank: 2, Tier: "primary",
		PriorityInversionCandidate: true,
		ImpactMS:                   58.919, CumulativeImpactMS: 58.919, EffectiveImpactMS: 37.410,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		Confidence: 0.9,
	}
	return types.TraceCausalProjection{
		WakeupPath:        []string{"RxComputationT-16612", "aweme-16547"},
		WindowStartTs:     33872.279,
		WindowEndTs:       33872.410,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{inversion},
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-selflock",
				Subject: "aweme-16547", Object: "blocking_span", TypeToken: "blocking_span",
				// EVOLUTION RECORD (跨批 X1, 2026-07-09): Rank 1 → 0. GAP-A G9
				// stopped assigning rank ordinals to tier=target_self_state
				// rows (序数只分给携榜位显示身份的行); the old Rank:1 fixture
				// drifted from the engine shape and let the symptom-disclosure
				// gate's dead `Rank <= 0` arm pin GREEN while the disclosure
				// was structurally dead in production (fixture 漂移致 pin 假绿).
				Predicate: "root_cause_target_self_state", Rank: 0,
				Tier:     types.TraceCausalTierTargetSelfState,
				ImpactMS: 115.944, CumulativeImpactMS: 115.944, EffectiveImpactMS: 115.944,
				ChainRelevance: "on_chain", BlockingKind: "monitor_contention",
				BlockingSubjectIsHolder: true, BlockingPeer: "LegoHandler-16865",
				Confidence: 0.67,
			},
			inversion,
		},
	}
}

func TestSYMOpendirLeadFallsToNonSelfBoardHead(t *testing.T) {
	projection := symOpendirProjection()
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary {
		t.Fatalf("opendir shape must produce a primary-lane lead, got lane=%d lead=%+v", lane, lead)
	}
	if lead.Subject != "RxComputationT-16612" {
		t.Fatalf("修后 lead = 非自身榜首 (the inversion row), got %q", lead.Subject)
	}
	// ❶ follows the same board (裁定二: 徽章随统一榜).
	badge := covLeadBadgeOneRow(model)
	if badge == nil || badge.Node.Subject != "RxComputationT-16612" {
		t.Fatalf("❶ must land on the non-self board head, got %+v", badge)
	}
	// 自持锁行留树头: the self-lock row keeps its SelfRows seat. EVOLUTION
	// RECORD (跨批 X1, 2026-07-09): the former 榜位序数照发 assertion (rank#1
	// intact) is RETIRED — GAP-A G9 no longer assigns ordinals to symptom
	// rows, and the fixture now carries the production Rank=0 shape.
	foundSelf := false
	for _, row := range model.SelfRows {
		if row.Node.EvidenceID == "e-selflock" {
			foundSelf = true
			if row.Node.Rank != 0 {
				t.Fatalf("fixture drifted: symptom rows carry NO ordinal post-G9, got %+v", row.Node)
			}
		}
	}
	if !foundSelf {
		t.Fatalf("the self-lock row must keep its tree-head seat (SelfRows): %+v", model.SelfRows)
	}
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "**主根因:** RxComputationT-16612") {
		t.Fatalf("the conclusion must crown the non-self row:\n%s", line)
	}
	if strings.Contains(line, "aweme-16547") {
		t.Fatalf("the target's self-held lock must never be crowned 主根因 (§24.13):\n%s", line)
	}
}

// symFlatCmpProjection mirrors the cmp_78_01 witness shape AFTER the SYM
// engine half, in the FLAT render (no ≥2-node wakeup path → no SelfRows lane):
// the target's own binder-wait row (rank#1, eff 3.843 — the board's eff-max)
// wears the self tier, and a smaller non-self runnable row wears the engine
// primary. Only the typed board exclusion keeps the lead off the self row.
func symFlatCmpProjection() types.TraceCausalProjection {
	primary := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-launchpool",
		Subject: "LaunchPoolT1-6712", Object: "runnable_wait",
		Predicate: "root_cause_primary", Rank: 2, Tier: "primary",
		ImpactMS: 1.900, CumulativeImpactMS: 1.900, EffectiveImpactMS: 1.900,
		ChainRelevance: "on_chain", Confidence: 0.8,
	}
	return types.TraceCausalProjection{
		WindowStartTs:     3680.819,
		WindowEndTs:       3682.619,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{primary},
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-selfbinder",
			Subject: "main-6565", Object: "binder_wait", TypeToken: "binder_wait",
			// EVOLUTION RECORD (跨批 X1, 2026-07-09): Rank 1 → 0 — the engine
			// no longer assigns ordinals to symptom rows (G9); the old fixture
			// drifted and pinned a dead gate green.
			Predicate: "root_cause_target_self_state", Rank: 0,
			Tier:     types.TraceCausalTierTargetSelfState,
			ImpactMS: 3.843, CumulativeImpactMS: 3.843, EffectiveImpactMS: 3.843,
			ChainRelevance: "on_chain", Confidence: 0.8,
		}},
	}
}

func TestSYMFlatSelfRowExcludedFromBoard(t *testing.T) {
	projection := symFlatCmpProjection()
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if strings.TrimSpace(model.Target) != "" {
		t.Fatalf("fixture drifted: this pin needs the FLAT render (no SelfRows lane)")
	}
	// The self binder row renders in TreeRows (depthless) — seat preserved —
	// but never seats on the shared board. EVOLUTION RECORD (跨批 X1,
	// 2026-07-09): the ordinal-preserved clause is retired with the G9
	// renumbering (symptom rows carry Rank=0 in production).
	var selfRow *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		if model.TreeRows[i].Node.EvidenceID == "e-selfbinder" {
			selfRow = &model.TreeRows[i]
		}
	}
	if selfRow == nil {
		t.Fatalf("the flat self binder row must keep its tree seat: %+v", model.TreeRows)
	}
	for _, row := range runtimeTraceProjRankBoard(model.TreeRows) {
		if row.Node.IsTargetSelfStateRow() {
			t.Fatalf("board 只收非自身 rank 行 (§24.13): %+v", row.Node)
		}
	}
	// The self row is the eff-max (3.843 > 1.900): without the typed exclusion
	// it would head the board — lead and ❶ must land on the non-self row.
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary || lead.Subject != "LaunchPoolT1-6712" {
		t.Fatalf("lead must fall to the non-self board head, got lane=%d lead=%+v", lane, lead)
	}
	badge := covLeadBadgeOneRow(model)
	if badge == nil || badge.Node.Subject != "LaunchPoolT1-6712" {
		t.Fatalf("❶ must skip the self row (徽章随统一榜), got %+v", badge)
	}
	if selfRow.Badge != 0 {
		t.Fatalf("the self row never wears a badge: %+v", selfRow)
	}
	// S3: the compare 主根因 column consumes the same single source.
	cell := runtimeTraceProjComparePrimaryCell(projection, model, true)
	if !strings.Contains(cell, "LaunchPoolT1-6712") || strings.Contains(cell, "main-6565") {
		t.Fatalf("the compare primary cell must follow the same lead source:\n%s", cell)
	}
}

// symAllSelfProjection is the 全自身榜退化形: rank data exists, but every
// ranked row is the target's own symptom — the SYM ladder left the primary
// bucket empty.
func symAllSelfProjection(impact float64) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WindowStartTs: 100.000,
		WindowEndTs:   100.101,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-selfbinder",
			Subject: "main-6565", Object: "binder_wait", TypeToken: "binder_wait",
			// EVOLUTION RECORD (跨批 X1, 2026-07-09): Rank 1 → 0 (engine G9
			// production shape) — the disclosure tests below are therefore the
			// END-TO-END Rank=0 witnesses: a symptom row WITHOUT an ordinal
			// must still produce the §24.16 disclosure sentence (the dead
			// `Rank <= 0` gate arm this batch removed silently killed it).
			Predicate: "root_cause_target_self_state", Rank: 0,
			Tier:     types.TraceCausalTierTargetSelfState,
			ImpactMS: impact, CumulativeImpactMS: impact, EffectiveImpactMS: impact,
			ChainRelevance: "on_chain", Confidence: 0.8,
		}},
	}
}

func TestSYMAllSelfBoardHonestFallbackWithDisclosure(t *testing.T) {
	projection := symAllSelfProjection(4.577)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if lead, lane := runtimeTraceProjLeadSelect(projection, model); lead != nil || lane != runtimeTraceProjLeadLaneNone {
		t.Fatalf("the all-self board must not elect a lead, got lane=%d lead=%+v", lane, lead)
	}
	line := runtimeTraceProjConclusionLine(projection, model, true)
	want := "**主根因:** 窗口内未定位到链上主根因(关注线程自身等待/持锁 4.577ms,见关注线程自身行)。"
	if line != want {
		t.Fatalf("诚实回退+症状披露 (§24.13):\nwant %q\ngot  %q", want, line)
	}
	en := runtimeTraceProjConclusionLine(projection, buildRuntimeTraceProjTreeModel(projection, nil, false), false)
	wantEN := "**Primary root cause:** no on-chain primary root cause was located in the window (the focused thread itself waited/held a lock for 4.577ms; see its own state rows)."
	if en != wantEN {
		t.Fatalf("EN honest fallback:\nwant %q\ngot  %q", wantEN, en)
	}
	// The compare cell rides the same disclosure.
	cell := runtimeTraceProjComparePrimaryCell(projection, model, true)
	if !strings.Contains(cell, "未定位到链上主根因") || !strings.Contains(cell, "关注线程自身等待/持锁 4.577ms") {
		t.Fatalf("the compare cell must carry the honest fallback + disclosure:\n%s", cell)
	}
}

func TestSYMDisclosureSingleLargestOverManySelfRows(t *testing.T) {
	// Two overlapping self rows: the disclosure takes the single largest
	// magnitude (墙钟不可加和 — 单项最大, never Σ).
	projection := symAllSelfProjection(4.577)
	projection.OnChainCauses = append(projection.OnChainCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-selflock",
		Subject: "main-6565", Object: "blocking_span", TypeToken: "blocking_span",
		Predicate: "root_cause_target_self_state", Rank: 2,
		Tier:     types.TraceCausalTierTargetSelfState,
		ImpactMS: 115.944, CumulativeImpactMS: 115.944, EffectiveImpactMS: 115.944,
		ChainRelevance: "on_chain", Confidence: 0.67,
	})
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "(关注线程自身等待/持锁 单项最大 115.944ms,见关注线程自身行)") {
		t.Fatalf("multi-row disclosure must use the 单项最大 caliber:\n%s", line)
	}
	if strings.Contains(line, "120.521") {
		t.Fatalf("the disclosure must never sum overlapping self rows:\n%s", line)
	}
}

func TestSYMLegacyNoRankShapeStaysByteStable(t *testing.T) {
	// Control: strip the typed tier → no ranked self rows → the legacy
	// no-rank-data shape keeps its "" conclusion byte-identically (the
	// disclosure lane never fires on legacy shapes).
	projection := symAllSelfProjection(4.577)
	projection.OnChainCauses[0].Tier = ""
	projection.OnChainCauses[0].Predicate = "root_cause_context"
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if line := runtimeTraceProjConclusionLine(projection, model, true); line != "" {
		t.Fatalf("legacy no-rank-data shape must keep the empty conclusion:\n%s", line)
	}
	// A 0ms self row never publishes a disclosure either.
	zero := symAllSelfProjection(0)
	zeroModel := buildRuntimeTraceProjTreeModel(zero, nil, true)
	if line := runtimeTraceProjConclusionLine(zero, zeroModel, true); line != "" {
		t.Fatalf("a valueless self row must not mint a 0ms disclosure:\n%s", line)
	}
}

// --- S3: LEAD-SEM reachability + zero-chain presence note interplay ------------

func TestSYMAllSelfShapeSemanticInterplay(t *testing.T) {
	projection := symAllSelfProjection(4.577)
	projection.SemanticSpans = []types.TraceCausalProjectionNode{{
		Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "e-jit",
		Subject: "hwid-123", Object: "jit_compile", Predicate: "trace_semantic_span",
		SpanName: "JIT compiling method00", SemanticClass: "jit_compile",
		ImpactMS: 83.893, CumulativeImpactMS: 83.893,
		Confidence: 0.8,
	}}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	// 语义回退可达性 (pin 覆盖): the empty primary bucket keeps the LEAD-SEM
	// lane unreachable — the conclusion is the 诚实回退 lane, never an
	// optimization-span statement.
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "窗口内未定位到链上主根因(关注线程自身等待/持锁 4.577ms") {
		t.Fatalf("the honest fallback + disclosure must win on the all-self shape:\n%s", line)
	}
	if strings.Contains(line, "语义优化span") || strings.Contains(line, "确定性优化点") {
		t.Fatalf("the LEAD-SEM lane must stay unreachable on the empty primary bucket:\n%s", line)
	}
	// 零链括注: the compare cell's optimization presence note IS reachable
	// beside the disclosure (DCS E6 F3b lane, now exercised by the SYM shape).
	cell := runtimeTraceProjComparePrimaryCell(projection, model, true)
	if !strings.Contains(cell, "(存在确定性优化点: JIT compiling method00 83.893ms") {
		t.Fatalf("the zero-chain presence note must render beside the fallback:\n%s", cell)
	}
	if !strings.Contains(cell, "关注线程自身等待/持锁 4.577ms") {
		t.Fatalf("the disclosure must ride the compare cell too:\n%s", cell)
	}
}

// --- 复核 F1: the RN-3(a) on-chain fallback lane never crowns a self row -------

func TestSYMOnChainFallbackNeverCrownsSelfRow(t *testing.T) {
	// 复核 witness shape: the primary bucket holds only a demotable aggregate
	// row (裁定1: aggregate-metric rows demote to the background stanza), and
	// the flat tree's only ranked row is the target's own binder wait — the
	// window's largest on-chain wait. Pre-F1 the RN-3(a) fallback crowned it:
	// "主根因: main-6565 binder等待…(链不可上溯,按窗口内最大链上等待)".
	// 突变自查 F1: deleting the fallback's IsTargetSelfStateRow arm reds this.
	projection := symAllSelfProjection(3.843)
	projection.PrimaryRootCauses = []types.TraceCausalProjectionNode{{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-cpu-pressure",
		Subject: "cpu", Object: "cpu_pressure", Predicate: "root_cause_primary",
		Rank: 3, Tier: "primary", SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
		ImpactMS: 846.793, CumulativeImpactMS: 846.793,
		ChainRelevance: "background", Confidence: 0.6,
	}}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if len(model.Background) == 0 {
		t.Fatalf("fixture drifted: the aggregate primary must demote to the background stanza")
	}
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead != nil || lane != runtimeTraceProjLeadLaneNone {
		t.Fatalf("the RN-3(a) fallback must never crown the target's own row, got lane=%d lead=%+v", lane, lead)
	}
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "窗口内未定位到链上主根因,见背景压力段(关注线程自身等待/持锁 3.843ms,见关注线程自身行)") {
		t.Fatalf("the honest fallback + disclosure must take over:\n%s", line)
	}
	if strings.Contains(line, "**主根因:** main-6565") || strings.Contains(line, "链不可上溯,按窗口内最大链上等待") {
		t.Fatalf("the fallback lane must not re-crown the self binder wait (§24.13 复核 F1):\n%s", line)
	}
	cell := runtimeTraceProjComparePrimaryCell(projection, model, true)
	if !strings.Contains(cell, "未定位到链上主根因") || !strings.Contains(cell, "关注线程自身等待/持锁 3.843ms") {
		t.Fatalf("the compare cell must follow the same honest fallback:\n%s", cell)
	}
	if strings.Contains(cell, "main-6565 · ") {
		t.Fatalf("the compare cell must not crown the self row either:\n%s", cell)
	}
}

// --- 复核 F2: the disclosure magnitude rides the lead-selection caliber --------

func TestSYMDisclosureMergedSelfRowUsesPerInstanceMax(t *testing.T) {
	// A ×3 merged self row publishes ImpactMS as the member SUM (120.000) —
	// the disclosure competes with the SAME single-instance caliber as every
	// lead lane (MergedMaxMS 60.000) and wears the 单项最大 word even though
	// it is a single row. 突变自查 F2: reverting the magnitude to the display
	// impact (ImpactMS-first) reds both assertions.
	projection := symAllSelfProjection(120.000)
	projection.OnChainCauses[0].MergedCount = 3
	projection.OnChainCauses[0].MergedMinMS = 20.000
	projection.OnChainCauses[0].MergedMaxMS = 60.000
	projection.OnChainCauses[0].CumulativeImpactMS = 120.000
	projection.OnChainCauses[0].EffectiveImpactMS = 0
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "(关注线程自身等待/持锁 单项最大 60.000ms,见关注线程自身行)") {
		t.Fatalf("a merged self row must disclose its per-instance max with the 单项最大 word:\n%s", line)
	}
	if strings.Contains(line, "120.000") {
		t.Fatalf("the merged SUM must never publish as the disclosure magnitude:\n%s", line)
	}
}

func TestSYMDisclosurePeriodicSelfRowUsesDiscountedValue(t *testing.T) {
	// A periodic self row competes with its discounted attribution — raw
	// cadence sleep never publishes (VS-1 discipline on the disclosure too).
	projection := symAllSelfProjection(50.000)
	projection.OnChainCauses[0].PeriodicSource = true
	projection.OnChainCauses[0].EffectiveImpactMS = 5.500
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "(关注线程自身等待/持锁 5.500ms,见关注线程自身行)") {
		t.Fatalf("a periodic self row must disclose its discounted attribution:\n%s", line)
	}
	if strings.Contains(line, "50.000") {
		t.Fatalf("raw cadence sleep must never publish as the disclosure magnitude:\n%s", line)
	}
	// Discounted to exactly 0 → no disclosure at all (never a 0ms claim).
	zero := symAllSelfProjection(50.000)
	zero.OnChainCauses[0].PeriodicSource = true
	zero.OnChainCauses[0].EffectiveImpactMS = 0
	zeroModel := buildRuntimeTraceProjTreeModel(zero, nil, true)
	if line := runtimeTraceProjConclusionLine(zero, zeroModel, true); line != "" {
		t.Fatalf("a fully-discounted periodic self row must not mint a disclosure:\n%s", line)
	}
}

// --- S2d: fold port — the typed tier travels with the rank ---------------------

// NOTE (SYM-2 §24.17, 2026-07-08): the transplant is FAMILY-AGNOSTIC by design
// — it ports whatever tier the engine minted, so the narrowing happened at the
// mint (engine) and this pin keeps guarding the port mechanics. The inversion
// token below no longer wears the self tier in production (自因族 competes);
// the mechanics pin stays on the historical shape.
func TestSYMFoldPortsSelfTierWithRank(t *testing.T) {
	rankTwin := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-rank",
		Subject: "aweme-16547", Object: "priority_inversion_candidate",
		Predicate: "root_cause_target_self_state", Rank: 1,
		Tier:                       types.TraceCausalTierTargetSelfState,
		PriorityInversionCandidate: true,
		EffectiveImpactMS:          37.410, CumulativeImpactMS: 58.919,
		LineStart: 100, LineEnd: 200,
	}
	chainTwin := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-chain",
		Subject: "aweme-16547", Object: "priority_inversion_candidate",
		Predicate: "wakeup_causal_impact", ChainRelevance: "on_chain",
		PriorityInversionCandidate: true,
		EffectiveImpactMS:          37.410, CumulativeImpactMS: 58.919,
		LineStart: 100, LineEnd: 200,
	}
	kept, peers := runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rankTwin, chainTwin})
	if len(kept) != 1 || len(peers) != 1 {
		t.Fatalf("fixture drifted: the twins must fold, got kept=%d peers=%d", len(kept), len(peers))
	}
	if kept[0].Rank != 1 || !kept[0].IsTargetSelfStateRow() {
		t.Fatalf("the typed self tier must travel WITH the ported rank (board exclusion survives the fold): %+v", kept[0])
	}
	// The merged row never seats on the board.
	rows := []runtimeTraceProjTreeRow{{Node: kept[0], Kind: runtimeTraceProjTreeRowDepthless, HasData: true}}
	if board := runtimeTraceProjRankBoard(rows); len(board) != 0 {
		t.Fatalf("the folded self row must stay off the board: %+v", board[0].Node)
	}
}

// --- S2e: display words — 关注线程自身 family, never root-cause words ----------

func TestSYMSelfRowDisplayWords(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "main-6565", Object: "binder_wait",
		Predicate: "root_cause_target_self_state", Rank: 1,
		Tier: types.TraceCausalTierTargetSelfState, ChainRelevance: "on_chain",
	}
	if got := runtimeTraceCausalProjectionLayerCell(node, true); got != "关注线程自身" {
		t.Fatalf("zh layer word must join the B#24 family, got %q", got)
	}
	if got := runtimeTraceCausalProjectionLayerCell(node, false); got != "the focused thread itself" {
		t.Fatalf("en layer word, got %q", got)
	}
	if got := runtimeTraceCausalProjectionPriorityCell(node, true); got != "自身状态" {
		t.Fatalf("zh priority word, got %q", got)
	}
	if got := runtimeTraceCausalProjectionPriorityCell(node, false); got != "self state" {
		t.Fatalf("en priority word, got %q", got)
	}
	merged := runtimeTraceProjDetailPositionMerged(node, true, false)
	if merged != "关注线程自身" {
		t.Fatalf("the merged 因果位置 cell passes the family word through, got %q", merged)
	}
	if strings.Contains(merged, "主根因") {
		t.Fatalf("a self rank row must never claim 主根因(优先处理): %q", merged)
	}
}
