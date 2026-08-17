package tool

// answer_document_projection_close1_test.go — CLOSE-1 批 lead-election pins
// (ledger real_trace_campaign_20260705.md §29.30 用户裁定 + §29.30.1 补充,
// 2026-07-11): the election population is every seat-holding row (TreeRows ∪
// SelfRows) through ONE shared valid-seat gate (badge authority + election
// board, single implementation — runtimeTraceProjRowValidSeat), with the
// 自因成因形 crown wording for self-cause leads.
//
// Pin directions (裁定原文四向 + §29.30.1 refinements):
//   P1  平铺/树形加冕 parity — the same node set crowns the SAME row with the
//       SAME sentence on both renders (cmp_78 平铺加冕形=收敛基准); plus the
//       structural ➊行==被冕行 identity.
//   P2  自因冠词面 — Tier A (running + provable §29.27 account) speaks the
//       account decomposition through the SAME parts builder as the account
//       line (single morpheme+value source); Tier B (runnable/IO/D-state or
//       account unprovable) speaks 「关注线程自身 {state} {§24.3 category}」;
//       both never wear the external thread-name syntax.
//   P3  症状负对照 — a target_self_state row NEVER enters the election
//       population even with a stale positive Rank/eff pair.
//   P4  context_only 负对照 — a context_only row with a stale positive pair
//       neither boards nor wears a badge nor crowns (shared-gate arm).
//   P5  普通 sleep 自因形不冕 — a SelfRows sleep row WITHOUT its engine
//       symptom tier (stale form) holds no seat on either face (§29.30.1
//       远端点名负对照; four-family closed-set arm).
//   P6  零 CAP running 形不冕 — a typed computed-zero-deficit running root
//       (SupplyFoldComputed ∧ deficit≤0) crowns nothing even as the only
//       root: lead absent + zero glyphs (§29.30.1 远端点名负对照).
//   P7  vc_710 外因主导帧回归 — an external-dominant frame with SelfRows
//       present keeps the external crown BYTE-identically (外因句式零变),
//       even when the self row's raw eff is larger (无特殊加成 — the typed
//       rank order is the in-window authority).
//   P8  复核 F1 — a Background/Adjacent stanza row with a stale positive
//       Rank/eff pair holds no seat on either face (lane-kind arm inside the
//       shared gate; bare 「#N」 chip retained).
//   P9  复核捎带 V-2 — the D-state self crown never echoes the D family
//       category word beside the kernel state token.
//
// 突变自查 (each applied locally and verified red before ship):
//   (M1) population revert to TreeRows-only → P1 tree side crowns e-peer;
//   (M2) shared-gate target_self_state arm removed → P3 (binder eff-max heads
//        the board);
//   (M3) 词面 fork removed (external name kept for self-cause crowns) → P2 +
//        evolved TestSYM2FlatSelfRunnableCrownedLead;
//   (M4) self-row rank bonus injected in the board comparator → P7 (self eff
//        60 > external 2.5 flips the crown once ranks tie);
//   (M5) shared-gate eff>0 arm removed → TestSYM2ZeroDeficit… badge assert +
//        P6; context_only arm removed → P4; overflow arm →
//        TestCov4BadgeNegativeGates; TopN cap removed → P7 rank-6 badge
//        assert;
//   (M6) self four-family arm removed → P5;
//   (M7) lane-kind arm removed (default admits all kinds) → P8 +
//        TestCov4BadgeNegativeGates.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// close1TreeMixedProjection is the cmp_78 witness node set (sym2 flat fixture)
// rendered as a TREE: the wakeup path names the target, so the self rows land
// on the SelfRows lane instead of the flat chain.
func close1TreeMixedProjection() types.TraceCausalProjection {
	projection := sym2FlatMixedProjection()
	projection.WakeupPath = []string{"LaunchPoolT1-6712", "main-6565"}
	return projection
}

// close1Badge1Key returns the node key of the (single-window) ➊ row across
// all rendered collections, "" when no row wears ➊.
func close1Badge1Key(model runtimeTraceProjTreeModel) string {
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for i := range rows {
			if rows[i].Badge == 1 {
				return runtimeTraceCausalProjectionNodeKey(rows[i].Node)
			}
		}
	}
	return ""
}

func TestCLOSE1FlatTreeCrownParity(t *testing.T) {
	flat := sym2FlatMixedProjection()
	flatModel := buildRuntimeTraceProjTreeModel(flat, nil, true)
	if strings.TrimSpace(flatModel.Target) != "" {
		t.Fatalf("fixture drifted: the flat side needs the FLAT render")
	}
	tree := close1TreeMixedProjection()
	treeModel := buildRuntimeTraceProjTreeModel(tree, nil, true)
	if strings.TrimSpace(treeModel.Target) == "" {
		t.Fatalf("fixture drifted: the tree side needs a wakeup-path target")
	}
	if len(treeModel.SelfRows) == 0 {
		t.Fatalf("fixture drifted: the tree render must route the self rows onto SelfRows")
	}
	flatLead, flatLane := runtimeTraceProjLeadSelect(flat, flatModel)
	treeLead, treeLane := runtimeTraceProjLeadSelect(tree, treeModel)
	if flatLead == nil || flatLane != runtimeTraceProjLeadLanePrimary || flatLead.EvidenceID != "e-selfrunnable" {
		t.Fatalf("flat crown must stay the self runnable row (收敛基准形), got lane=%d lead=%+v", flatLane, flatLead)
	}
	if treeLead == nil || treeLane != runtimeTraceProjLeadLanePrimary || treeLead.EvidenceID != "e-selfrunnable" {
		t.Fatalf("§29.30 P1: the TREE render must crown the SAME self runnable row (SelfRows 扩展), got lane=%d lead=%+v", treeLane, treeLead)
	}
	flatLine := runtimeTraceProjConclusionLine(flat, flatModel, true)
	treeLine := runtimeTraceProjConclusionLine(tree, treeModel, true)
	if flatLine != treeLine {
		t.Fatalf("§29.30 P1: the crown sentence must not drift with the render shape:\nflat: %s\ntree: %s", flatLine, treeLine)
	}
	// Structural pin: the ➊ row IS the crowned row (shared valid-seat gate —
	// the badge face and the election face can no longer split).
	for name, m := range map[string]struct {
		model runtimeTraceProjTreeModel
		lead  *types.TraceCausalProjectionNode
	}{"flat": {flatModel, flatLead}, "tree": {treeModel, treeLead}} {
		badge1 := close1Badge1Key(m.model)
		if badge1 == "" || badge1 != runtimeTraceCausalProjectionNodeKey(*m.lead) {
			t.Fatalf("➊行==被冕行 structural pin (%s): badge1=%q lead=%q", name, badge1, runtimeTraceCausalProjectionNodeKey(*m.lead))
		}
	}
	// EN face rides the same single wording source.
	treeLineEN := runtimeTraceProjConclusionLine(tree, treeModel, false)
	if !strings.Contains(treeLineEN, "the focused thread itself runnable scheduling-pressure candidate") {
		t.Fatalf("EN self-cause crown head missing:\n%s", treeLineEN)
	}
}

func TestCLOSE1SelfRunningTierACrownSpeaksAccountParts(t *testing.T) {
	// Tier A: a crowned self running row whose §29.27 four-state account is
	// provable (Σ==窗 at %.3f) speaks the account decomposition through the
	// SAME parts builder as the account's running line — never the external
	// syntax, never the standalone supply-fold clause (its 折算 figure lives
	// inside the parenthetical).
	running := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-selfrunning",
		Subject: "main-6565", Object: "running", TypeToken: "running", StateKind: "running",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 493.000, CumulativeImpactMS: 493.000, EffectiveImpactMS: 6.100,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 6.100, SupplyFoldIdealMS: 486.900,
		SupplyFoldKnownMS: 493.000,
		ChainRelevance:    "on_chain", Causality: "on_wakeup_chain", ChainDepth: 0,
		Confidence: 0.8,
	}
	projection := types.TraceCausalProjection{
		WakeupPath:        []string{"waker-1", "main-6565"},
		WindowStartTs:     100.000,
		WindowEndTs:       100.600,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{running},
		OnChainCauses:     []types.TraceCausalProjectionNode{running},
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{
			Subject:       "main-6565",
			WindowStartTs: 100.000, WindowEndTs: 100.600,
			RunningMS: 493.000, RunnableMS: 50.000, SleepMS: 57.000,
			DStateMS: 0, IOWaitMS: 0,
			DeterministicRunningMS: 120.000,
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if runtimeTraceProjFourStateAccountProvable(projection, model) == nil {
		t.Fatalf("fixture drifted: the account must be provable (Σ==窗)")
	}
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary || lead.EvidenceID != "e-selfrunning" {
		t.Fatalf("the self running deficit row must crown, got lane=%d lead=%+v", lane, lead)
	}
	line := runtimeTraceProjConclusionLine(projection, model, true)
	for _, want := range []string{
		"**主根因(=已证链上单项最大可消除量):** 关注线程自身 running(确定性工作 120.000ms",
		"供给折算影响 6.100ms",
		"自身执行(无确定性可优化工作) 373.000ms",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("Tier A crown must speak the account decomposition (%q):\n%s", want, line)
		}
	}
	if strings.Contains(line, "**主根因(=已证链上单项最大可消除量):** main-6565") {
		t.Fatalf("Tier A crown must not wear the external thread-name syntax:\n%s", line)
	}
	if strings.Contains(line, "供给折算缺口") {
		t.Fatalf("the standalone supply-fold clause must stay suppressed on a Tier A crown (值面单点):\n%s", line)
	}
	en := runtimeTraceProjConclusionLine(projection, model, false)
	for _, want := range []string{
		"the focused thread itself running(deterministic work 120.000ms",
		"own execution (no deterministic optimizable work) 373.000ms",
	} {
		if !strings.Contains(en, want) {
			t.Fatalf("EN Tier A crown missing %q:\n%s", want, en)
		}
	}
}

func TestCLOSE1SymptomRowNeverEntersElectionPopulation(t *testing.T) {
	// P3: even with a STALE positive Rank/eff pair, the target_self_state
	// symptom row never boards from the SelfRows lane (shared-gate tier arm;
	// it is also the eff-max row here, so a gate regression flips the crown).
	tree := close1TreeMixedProjection()
	for i := range tree.OnChainCauses {
		if tree.OnChainCauses[i].EvidenceID == "e-selfbinder" {
			tree.OnChainCauses[i].Rank = 1 // deliberate stale-rank mutation
		}
	}
	model := buildRuntimeTraceProjTreeModel(tree, nil, true)
	board := runtimeTraceProjRankBoard(runtimeTraceProjLeadElectionRows(model))
	for _, row := range board {
		if row.Node.IsTargetSelfStateRow() {
			t.Fatalf("症状永不加冕: a target_self_state row entered the election population: %+v", row.Node)
		}
	}
	lead, _ := runtimeTraceProjLeadSelect(tree, model)
	if lead == nil || lead.EvidenceID == "e-selfbinder" {
		t.Fatalf("the symptom row must never crown, got %+v", lead)
	}
	// FLAT twin: on the flat render the stale symptom row is a chain-lane
	// (depthless) row — the SelfRows four-family arm cannot shadow the tier
	// arm there, so this half bites when the shared gate's
	// IsTargetSelfStateRow arm alone is removed (mutation M2 witness).
	flat := sym2FlatMixedProjection()
	for i := range flat.OnChainCauses {
		if flat.OnChainCauses[i].EvidenceID == "e-selfbinder" {
			flat.OnChainCauses[i].Rank = 1 // deliberate stale-rank mutation
		}
	}
	flatModel := buildRuntimeTraceProjTreeModel(flat, nil, true)
	for _, row := range runtimeTraceProjRankBoard(runtimeTraceProjLeadElectionRows(flatModel)) {
		if row.Node.IsTargetSelfStateRow() {
			t.Fatalf("症状永不加冕 (flat): a target_self_state row entered the election population: %+v", row.Node)
		}
	}
	if flatLead, _ := runtimeTraceProjLeadSelect(flat, flatModel); flatLead == nil || flatLead.EvidenceID == "e-selfbinder" {
		t.Fatalf("the flat symptom row must never crown, got %+v", flatLead)
	}
}

func TestCLOSE1ContextOnlyRowHoldsNoSeat(t *testing.T) {
	// P4: a context_only row with a stale positive Rank/eff pair holds no
	// seat on either face (shared-gate arm — never a board member, never a
	// glyph, never the crown).
	projection, _ := contextOnlyDisplayProjection()
	for i := range projection.OnChainCauses {
		if projection.OnChainCauses[i].EvidenceID == "handoff-context" {
			projection.OnChainCauses[i].Rank = 1
			projection.OnChainCauses[i].EffectiveImpactMS = 3
		}
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	board := runtimeTraceProjRankBoard(runtimeTraceProjLeadElectionRows(model))
	for _, row := range board {
		if row.Node.IsContextOnlyRow() {
			t.Fatalf("context_only must never board: %+v", row.Node)
		}
	}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.Node.IsContextOnlyRow() && row.Badge != 0 {
				t.Fatalf("context_only must never wear a badge: %+v", row)
			}
		}
	}
	lead, _ := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lead.IsContextOnlyRow() {
		t.Fatalf("context_only must never crown, got %+v", lead)
	}
}

func TestCLOSE1PlainSleepSelfRowNeverCrownsOrWearsBadge(t *testing.T) {
	// P5 (§29.30.1 远端点名): a SelfRows sleep row WITHOUT its engine symptom
	// tier (stale/legacy persisted form, eff-max and rank #1 here) is outside
	// the §24.17 self-cause four-family closed set — it holds no seat on
	// either face, and the external candidate crowns.
	staleSleep := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-selfsleep",
		Subject: "main-1", Object: "sleep_wait", TypeToken: "sleep_wait", StateKind: "s_sleep",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 10.000, CumulativeImpactMS: 10.000, EffectiveImpactMS: 10.000,
		ChainRelevance: "on_chain", Confidence: 0.8,
	}
	peer := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-peer",
		Subject: "waker-2", Object: "runnable_wait", TypeToken: "runnable_wait",
		Predicate: "root_cause_secondary", Rank: 2, Tier: "secondary",
		ImpactMS: 1.200, CumulativeImpactMS: 1.200, EffectiveImpactMS: 1.200,
		ChainRelevance: "on_chain", Confidence: 0.7,
	}
	projection := types.TraceCausalProjection{
		WakeupPath:        []string{"waker-2", "main-1"},
		WindowStartTs:     100.000,
		WindowEndTs:       100.600,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{staleSleep, peer},
		OnChainCauses:     []types.TraceCausalProjectionNode{staleSleep, peer},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	selfSeen := false
	for _, row := range model.SelfRows {
		if row.Node.EvidenceID != "e-selfsleep" {
			continue
		}
		selfSeen = true
		if row.Badge != 0 {
			t.Fatalf("a stale plain-sleep self row must never wear a badge: %+v", row)
		}
	}
	if !selfSeen {
		t.Fatalf("fixture drifted: the sleep row must land on SelfRows")
	}
	board := runtimeTraceProjRankBoard(runtimeTraceProjLeadElectionRows(model))
	for _, row := range board {
		if row.Node.EvidenceID == "e-selfsleep" {
			t.Fatalf("普通 sleep 自因形不冕: the sleep self row entered the board: %+v", row.Node)
		}
	}
	lead, _ := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lead.EvidenceID != "e-peer" {
		t.Fatalf("the external candidate must crown, got %+v", lead)
	}
}

func TestCLOSE1ZeroCapRunningRootCrownsNothing(t *testing.T) {
	// P6 (§29.30.1 远端点名): a typed computed-zero-deficit running root
	// (SupplyFoldComputed ∧ deficit≤0 — the engine's own 「无供给缺口」
	// verdict) crowns nothing even as the ONLY root: crowning it would
	// contradict its verdict. Lead absent + zero glyphs is the honest form.
	zero := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-zerorunning",
		Subject: "main-6565", Object: "running", TypeToken: "running", StateKind: "running",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 493.000, CumulativeImpactMS: 493.000, EffectiveImpactMS: 0,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 0, SupplyFoldIdealMS: 493.000,
		SupplyFoldKnownMS: 493.000,
		ChainRelevance:    "on_chain", Confidence: 0.8,
	}
	projection := types.TraceCausalProjection{
		WindowStartTs:     100.000,
		WindowEndTs:       100.600,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{zero},
		OnChainCauses:     []types.TraceCausalProjectionNode{zero},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead != nil {
		t.Fatalf("零 CAP running 不得加冕 (lane=%d): %+v", lane, lead)
	}
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "未定位到链上主根因") {
		t.Fatalf("the honest fallback must speak:\n%s", line)
	}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.Badge != 0 {
				t.Fatalf("zero-seat shape must wear zero glyphs: %+v", row)
			}
		}
	}
}

func TestCLOSE1SelfDStateCrownAvoidsCategoryEcho(t *testing.T) {
	// 复核捎带 V-2: the D-state self crown speaks the kernel state word once —
	// 「关注线程自身 D-state」, never the 「D-state D状态候选」 echo (the
	// runnable-precedent duplication shape, avoided the same way).
	dstate := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-selfdstate",
		Subject: "main-1", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
		StateKind: "d_state", Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 5.000, CumulativeImpactMS: 5.000, EffectiveImpactMS: 5.000,
		ChainRelevance: "on_chain", Confidence: 0.8,
	}
	projection := types.TraceCausalProjection{
		WakeupPath:        []string{"waker-1", "main-1"},
		WindowStartTs:     100.000,
		WindowEndTs:       100.600,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{dstate},
		OnChainCauses:     []types.TraceCausalProjectionNode{dstate},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "**主根因(=已证链上单项最大可消除量):** 关注线程自身 D-state") {
		t.Fatalf("the D-state self crown must speak the 自因成因形 head:\n%s", line)
	}
	if strings.Contains(line, "D状态候选") {
		t.Fatalf("V-2: the crown must not echo the D family category word:\n%s", line)
	}
}

func TestCLOSE1StanzaLaneStaleRankHoldsNoSeat(t *testing.T) {
	// CLOSE-1 复核 F1: a Background/Adjacent stanza row carrying a STALE
	// positive Rank/eff pair holds no valid seat on EITHER face — no glyph
	// (the bare 「#N」 chip stays on the detail face) and never the crown;
	// the on-chain rank-2 row crowns. Pre-F1 the badge authority traversed
	// all four collections without a lane-kind arm, so the stanza row wore
	// ➊ while 主根因 crowned the chain row (SPLIT REPRODUCED by the review
	// probe).
	for _, lane := range []string{"background", "adjacent"} {
		stale := types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-stanza-stale",
			Subject: "stanza-42", Object: "runnable_wait", TypeToken: "runnable_wait",
			Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
			ImpactMS: 50.000, CumulativeImpactMS: 50.000, EffectiveImpactMS: 50.000,
			ChainRelevance: "adjacent", Confidence: 0.6,
		}
		onChain := types.TraceCausalProjectionNode{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-chain",
			Subject: "waker-9", Object: "runnable_wait", TypeToken: "runnable_wait",
			Predicate: "root_cause_secondary", Rank: 2, Tier: "secondary",
			ImpactMS: 3.000, CumulativeImpactMS: 3.000, EffectiveImpactMS: 3.000,
			ChainRelevance: "on_chain", Confidence: 0.8,
		}
		projection := types.TraceCausalProjection{
			WindowStartTs:     100.000,
			WindowEndTs:       100.600,
			PrimaryRootCauses: []types.TraceCausalProjectionNode{onChain},
			OnChainCauses:     []types.TraceCausalProjectionNode{onChain},
		}
		if lane == "background" {
			projection.BackgroundCauses = []types.TraceCausalProjectionNode{stale}
		} else {
			projection.AdjacentCauses = []types.TraceCausalProjectionNode{stale}
		}
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		staleSeen := false
		for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
			for _, row := range rows {
				if row.Node.EvidenceID != "e-stanza-stale" {
					continue
				}
				staleSeen = true
				if row.Badge != 0 {
					t.Fatalf("[%s] a stanza-lane row must never wear a glyph (F1 lane arm): %+v", lane, row)
				}
			}
		}
		if !staleSeen {
			t.Fatalf("[%s] fixture drifted: the stanza row must render", lane)
		}
		lead, _ := runtimeTraceProjLeadSelect(projection, model)
		if lead == nil || lead.EvidenceID != "e-chain" {
			t.Fatalf("[%s] the on-chain row must crown, got %+v", lane, lead)
		}
		// 双面一致: no rendered row wears ➊ (the stale stanza rank never
		// mints a glyph).
		if badge1 := close1Badge1Key(model); badge1 != "" {
			t.Fatalf("[%s] no row may wear ➊ here (stale stanza rank is seatless): %q", lane, badge1)
		}
		// EVOLUTION RECORD (UXR-1 §29.36.2, 2026-07-11, supersedes F1's
		// "bare 「#N」 chip stays" clause): ordinal chips are channel-worded
		// (禁裸 #N) — a ◇ adjacent seat reads 邻近影响 #1 (its own independent
		// channel) and a ▒ background row prints NO seat at all (通道3
		// 无序数). The F1 core (stanza rank never glyphs, never crowns) is
		// asserted above unchanged.
		detail := runtimeTraceProjDetailFullText(model, true)
		if strings.Contains(detail, "根因排序: #1") {
			t.Fatalf("[%s] a stanza row must not wear the on-chain 根因排序 chip (§29.36.2 通道词面):\n%s", lane, detail)
		}
		switch lane {
		case "adjacent":
			if !strings.Contains(detail, "邻近影响: #1") {
				t.Fatalf("[%s] the ◇ seat must read 邻近影响 #1 on the detail face:\n%s", lane, detail)
			}
		case "background":
			if strings.Contains(detail, "邻近影响: #1") {
				t.Fatalf("[%s] a ▒ row prints no seat chip (通道3 无序数):\n%s", lane, detail)
			}
		}
	}
}

func TestCLOSE1ExternalDominantFrameKeepsExternalCrown(t *testing.T) {
	// P7 (vc_710 外因主导帧回归): with SelfRows PRESENT and the self row's raw
	// eff even larger, the typed rank order keeps the external inversion row
	// crowned — byte-identical external sentence (外因句式零变, 无特殊加成).
	// EVOLUTION RECORD (CROWNCAL-1, 2026-07-28): this fixture's effective
	// (2.500) differs from its printed 链上累计 (58.919) — the crowned line now
	// carries the elected effective beside the definition (bare honest form:
	// no gated components, so no caliber claim). 外因句式零变 still holds for
	// the SENTENCE STRUCTURE (no self-account bonus wording); the effective
	// arm is a value disclosure, not a special addition.
	inversion := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-inversion",
		Subject: "RxComputationT-16612", Object: "priority_inversion_candidate",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		PriorityInversionCandidate: true,
		ImpactMS:                   58.919, CumulativeImpactMS: 58.919, EffectiveImpactMS: 2.500,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		Confidence: 0.9,
	}
	selfRunnable := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-selfrunnable",
		Subject: "main-6565", Object: "runnable_wait", TypeToken: "runnable_wait",
		Predicate: "root_cause_secondary", Rank: 2, Tier: "secondary",
		ImpactMS: 60.000, CumulativeImpactMS: 60.000, EffectiveImpactMS: 60.000,
		ChainRelevance: "on_chain", Confidence: 0.8,
	}
	rankSix := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-ranksix",
		Subject: "helper-77", Object: "runnable_wait", TypeToken: "runnable_wait",
		Predicate: "root_cause_tertiary", Rank: 6, Tier: "tertiary",
		ImpactMS: 99.000, CumulativeImpactMS: 99.000, EffectiveImpactMS: 99.000,
		ChainRelevance: "on_chain", Confidence: 0.6,
	}
	projection := types.TraceCausalProjection{
		WakeupPath:        []string{"RxComputationT-16612", "main-6565"},
		WindowStartTs:     3680.819,
		WindowEndTs:       3682.619,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{inversion},
		OnChainCauses:     []types.TraceCausalProjectionNode{inversion, selfRunnable, rankSix},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary || lead.EvidenceID != "e-inversion" {
		t.Fatalf("外因主导帧: the inversion row must keep the crown, got lane=%d lead=%+v", lane, lead)
	}
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if line != "**主根因(=已证链上单项最大可消除量):** RxComputationT-16612 优先级反转候选 链上累计 58.919ms，有效归因 2.500ms。" {
		t.Fatalf("the external crown sentence must stay byte-identical (外因句式零变):\n%s", line)
	}
	if strings.Contains(line, "关注线程自身") {
		t.Fatalf("an external crown must never wear the self-cause head:\n%s", line)
	}
	// Structural ➊行==被冕行 + the seat pictographs follow the typed ranks.
	if badge1 := close1Badge1Key(model); badge1 != runtimeTraceCausalProjectionNodeKey(*lead) {
		t.Fatalf("➊行==被冕行: badge1=%q lead=%q", badge1, runtimeTraceCausalProjectionNodeKey(*lead))
	}
	for _, row := range model.SelfRows {
		if row.Node.EvidenceID == "e-selfrunnable" && row.Badge != 2 {
			t.Fatalf("the self #2 seat wears ➋ (badge follows the seat): %+v", row)
		}
	}
	// Rank∈1..TopN arm: a rank-6 seat wears no glyph even at eff-max (TopN
	// cap inside the shared gate — mutation (M5d) target).
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.Node.EvidenceID == "e-ranksix" && row.Badge != 0 {
				t.Fatalf("a rank-6 row must never wear a badge: %+v", row)
			}
		}
	}
}
