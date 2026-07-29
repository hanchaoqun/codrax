package tool

// answer_document_projection_covlead_test.go — COV+LEAD 批 pins
// (docs/design/real_trace_campaign_20260705.md §24.9 D-1/D-2 + §24.11 C-1/C-3,
// 2026-07-08; coordinator supplement: cmp_78_01 双侧 witness):
//
//   L1 (§24.11 C-1, huadong_78) — the lead election's primary lane consumes
//        the SAME post-aggregation rank board the ➊➋➌ badge lane seats from:
//        lead == the ➊ row whenever ➊ exists (两车道恒等 pin); the target's
//        own symptom row can never be crowned 主根因 while a ranked cause row
//        renders. 突变自查 M1: reverting the election population to the
//        pre-aggregation projection.PrimaryRootCauses bucket reds the
//        huadong-shape assertions (the self binder rank#1 wins again).
//   C1 (§24.9 D-1, opendir_78) — the coverage numerator consumes the typed
//        TargetImpactMS (engine TargetBlockedMs) first and only falls back to
//        the §20.1-overwritable cumulative channel; 严禁伪造残差 (an
//        under-explaining caliber publishes as-is, never clamps to the
//        denominator). 突变自查 M2: reverting the per-row ladder to
//        CumulativeImpactMS-first reds the 112.175 assertions.
//   C2 (§24.9 D-2, opendir_78) — the "另有 N 条链上行未计入" disclosure
//        excludes the target's own state/hop-view rows (the 被解释对象),
//        计数如实 (the self-lock row stays counted).
//   C3 (§24.11 C-3, huadong_78 + cmp_78_01 7.0 侧) — denominator-population
//        collapse switches the coverage sentence FORM: the counted slice never
//        masquerades as the 全称 "关注线程等待", and the parenthetical never
//        claims a state family the denominator silently excluded. 突变自查 M3:
//        disabling the census gate reds the form-switch assertions.
//   C4 (§24.11 C-3 后半场, cmp_78_01 双侧) — 各链上口径合计 → 链上单项最大
//        (名实对齐: the numerator is a single-row MAX, 墙钟不可求和); the
//        retired 冒名 word never renders on any coverage arm.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- L1: lead election population == badge board -----------------------------

// covLeadHuadongProjection mirrors the huadong_78 shape: the PRIMARY bucket
// carries the 🎯 target's own binder-wait symptom row as the engine's (window-
// local) rank#1, while the rendered tree's ➊ row — the ×9 inversion aggregate
// (E9, eff 6.430) — lives on the on-chain bucket only (the old pre-aggregation
// primary-bucket election never saw it). A second rank#1 row (E17-like io row,
// eff 1.940) witnesses the cross-window ordinal collision.
func covLeadHuadongProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"tpp-189", "oney-42591"},
		WindowStartTs: 100.000,
		WindowEndTs:   100.101,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-binder",
			Subject: "oney-42591", Object: "binder_wait", TypeToken: "binder_wait",
			Predicate: "root_cause_primary", Rank: 1,
			ImpactMS: 4.577, CumulativeImpactMS: 4.577, EffectiveImpactMS: 4.577,
			ChainRelevance: "on_chain", Confidence: 0.9,
		}},
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-inversion",
				Subject: "OS_FFRT_3-46792", Object: "priority_inversion_candidate",
				Predicate: "root_cause_primary", Rank: 1,
				PriorityInversionCandidate: true,
				ImpactMS:                   6.430, CumulativeImpactMS: 6.430, EffectiveImpactMS: 6.430,
				MergedCount: 9, MergedMinMS: 0.921, MergedMaxMS: 6.430,
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 2,
				Confidence: 0.9,
			},
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-io",
				Subject: "hmfs_discard-562", Object: "io_latency",
				Predicate: "root_cause_primary", Rank: 1,
				ImpactMS: 15.156, CumulativeImpactMS: 15.156, EffectiveImpactMS: 1.940,
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
				Confidence: 0.9,
			},
		},
	}
}

// covLeadRowBadge returns the Badge of the rendered tree row for subject
// (0 when the subject has no rendered tree row).
func covLeadRowBadge(model runtimeTraceProjTreeModel, subject string) int {
	for i := range model.TreeRows {
		if model.TreeRows[i].Node.Subject == subject {
			return model.TreeRows[i].Badge
		}
	}
	return 0
}

func TestCOVLeadPrimaryIsBadgeOneRow(t *testing.T) {
	projection := covLeadHuadongProjection()
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary {
		t.Fatalf("huadong shape must produce a primary-lane lead, got lane=%d lead=%+v", lane, lead)
	}
	// 修后 lead = E9 反转行,非 binder 症状行 (§24.11 C-1 verbatim acceptance).
	if lead.Subject != "OS_FFRT_3-46792" {
		t.Fatalf("lead must be the ➊ inversion row, not the target's own symptom row: got %q", lead.Subject)
	}
	// 两车道恒等 pin — EVOLUTION RECORD (§29.27.1 徽章跟随席位, 2026-07-11):
	// the badge is the pictograph of each row's PUBLISHED seat, no longer the
	// board position, so the identity evolves from "lead == the unique ➊ row"
	// to "the lead row wears its own seat's glyph" (seat #1 here → ➊) plus
	// "every badge equals its row's published seat" (badge↔seat, never
	// badge↔election). This fixture deliberately carries colliding #1
	// ordinals (the multi-board shape) — each keeps its own ➊; the §24.13
	// window-chip lane disambiguates boards, exactly like the ordinal text.
	if got := covLeadRowBadge(model, lead.Subject); got != 1 {
		t.Fatalf("the lead row publishes seat #1 and must wear ➊: got badge %d", got)
	}
	for _, row := range model.TreeRows {
		if row.Badge == 0 {
			continue
		}
		if rank, _ := runtimeTraceProjCauseRankConfidence(row); rank != row.Badge {
			t.Fatalf("badge %d must equal the row's published seat %d (徽章跟随席位): %+v",
				row.Badge, rank, row.Node)
		}
	}
	// The conclusion line consumes the same selection.
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "**主根因(=已证链上单项最大可消除量):** OS_FFRT_3-46792") {
		t.Fatalf("conclusion must name the lead row:\n%s", line)
	}
	if strings.Contains(line, "oney-42591") {
		t.Fatalf("the target's own binder symptom row must not be crowned 主根因 (§24.11 C-1):\n%s", line)
	}
}

func TestCOVLeadBoardKeyBeatsWindowLocalOrdinal(t *testing.T) {
	// Cross-window ordinal collision (two rank#1 rows): the board's
	// EffectiveImpactMS key elects the eff-max row — the window-local ordinal
	// never overrides the board (rank semantics themselves untouched: the
	// chips keep publishing the engine ordinals).
	projection := covLeadHuadongProjection()
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	lead, _ := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lead.Subject != "OS_FFRT_3-46792" {
		t.Fatalf("the eff-max rank row must lead across colliding #1 ordinals, got %+v", lead)
	}
	// Demoting the inversion row's attribution below the io row flips the
	// board head; EVOLUTION RECORD (§29.27.1, 2026-07-11): the badge follows
	// the SEAT, not the election — the new lead wears ➊ because its own
	// published seat is #1, and the demoted row KEEPS its ➊ (its seat did not
	// change; only the election moved).
	flipped := covLeadHuadongProjection()
	flipped.OnChainCauses[0].EffectiveImpactMS = 0.100
	flipped.OnChainCauses[0].MergedMaxMS = 0.100
	flippedModel := buildRuntimeTraceProjTreeModel(flipped, nil, true)
	flippedLead, _ := runtimeTraceProjLeadSelect(flipped, flippedModel)
	if flippedLead == nil || flippedLead.Subject != "hmfs_discard-562" {
		t.Fatalf("the election must move to the new eff-max row: lead=%+v", flippedLead)
	}
	if got := covLeadRowBadge(flippedModel, "hmfs_discard-562"); got != 1 {
		t.Fatalf("the new lead publishes seat #1 and must wear ➊: got badge %d", got)
	}
	if got := covLeadRowBadge(flippedModel, "OS_FFRT_3-46792"); got != 1 {
		t.Fatalf("the demoted row keeps its seat #1 and therefore its ➊ (badge follows seat, not election): got badge %d", got)
	}
}

func TestCOVLeadEmptyPrimaryBucketKeepsNoConclusion(t *testing.T) {
	// The legacy no-rank-data gate survives the population unification: an
	// EMPTY primary bucket yields no conclusion even when ranked on-chain rows
	// render (lane 1 only replaces the election population, never the
	// no-conclusion shapes).
	projection := covLeadHuadongProjection()
	projection.PrimaryRootCauses = nil
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if lead, lane := runtimeTraceProjLeadSelect(projection, model); lead != nil || lane != runtimeTraceProjLeadLaneNone {
		t.Fatalf("empty primary bucket must keep the legacy no-conclusion behavior, got lane=%d lead=%+v", lane, lead)
	}
	if line := runtimeTraceProjConclusionLine(projection, model, true); line != "" {
		t.Fatalf("empty primary bucket must render no conclusion line:\n%s", line)
	}
}

// --- C1: coverage numerator consumes the typed TargetImpactMS ----------------

func TestCOVNumeratorConsumesTargetImpactChannel(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true,
		Node: types.TraceCausalProjectionNode{
			Subject: "RxComputationT-16612", Object: "blocking_span",
			ImpactMS: 58.919, CumulativeImpactMS: 58.919, TargetImpactMS: 112.175,
		},
	}
	model := runtimeTraceProjTreeModel{WindowMS: 131.0, TreeRows: []runtimeTraceProjTreeRow{row}}
	if got := runtimeTraceProjDepth1Cumulative(model); got != 112.175 {
		t.Fatalf("the numerator must consume the typed target caliber 112.175 (never the §20.1-overwritable cumulative 58.919), got %.3f", got)
	}
	// Legacy fallback: no typed target caliber → the cumulative channel keeps
	// every byte (M2 negative control).
	legacy := row
	legacy.Node.TargetImpactMS = 0
	legacyModel := runtimeTraceProjTreeModel{WindowMS: 131.0, TreeRows: []runtimeTraceProjTreeRow{legacy}}
	if got := runtimeTraceProjDepth1Cumulative(legacyModel); got != 58.919 {
		t.Fatalf("without the typed caliber the legacy cumulative channel must stand, got %.3f", got)
	}
	// RNB numerator invariance: a folded rank peer competes with the SAME
	// ladder (typed target caliber first).
	peered := legacy
	peered.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{
		Rank: 1, CumulativeImpactMS: 58.919, DisplayImpactMS: 58.919, TargetImpactMS: 112.175,
	}}
	peeredModel := runtimeTraceProjTreeModel{WindowMS: 131.0, TreeRows: []runtimeTraceProjTreeRow{peered}}
	if got := runtimeTraceProjDepth1Cumulative(peeredModel); got != 112.175 {
		t.Fatalf("a folded rank peer's typed target caliber must stay in the MAX competition, got %.3f", got)
	}
}

// covOpendirProjection mirrors the opendir_78 shape end-to-end: hop-only
// target sleep (E2, ×3 merged, 115.353), a depth-1 chain row whose cumulative
// was §20.1-overwritten to 58.919 while its typed target caliber says 112.175
// (E4), and the target's own unadmitted self-lock row (E1, blocking_span,
// 115.944 — the D-2 disclosure member).
func covOpendirProjection(targetImpact float64) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"RxComputationT-16612", "aweme-16547"},
		WindowStartTs: 33872.279,
		WindowEndTs:   33872.410,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-selflock",
			Subject: "aweme-16547", Object: "blocking_span", TypeToken: "blocking_span",
			Predicate: "root_cause_primary", Rank: 1,
			ImpactMS: 115.944, CumulativeImpactMS: 115.944, EffectiveImpactMS: 115.944,
			ChainRelevance: "on_chain", Confidence: 0.9,
		}},
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
			Subject: "RxComputationT-16612", Object: "running",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 58.919, CumulativeImpactMS: 58.919, TargetImpactMS: targetImpact,
			Confidence: 0.8,
		}},
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-sleepview",
			Subject: "aweme-16547", Object: "s_sleep", StateKind: "s_sleep",
			Predicate: "wakeup_causal_impact",
			ImpactMS:  115.353, EffectiveImpactMS: 115.353,
			MergedCount: 3, MergedMinMS: 1.096, MergedMaxMS: 112.175,
			Confidence: 0.78,
		}},
	}
}

func TestCOVOpendirCoverageSentenceSelfConsistent(t *testing.T) {
	// opendir_78 形 fixture (§24.9 D-1 acceptance): 修后覆盖句自洽 — the
	// numerator speaks the 已由链上解释 caliber, so the hop info line and the
	// whole-window arithmetic agree with the ~97% explained sleep instead of
	// fabricating "已归因45%/未归因55%".
	projection := covOpendirProjection(112.175)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "关注线程睡眠 115.353ms 中 112.175ms 已由链上解释。") {
		t.Fatalf("the hop info line must speak the typed target caliber:\n%s", line)
	}
	if !strings.Contains(line, "链上已归因 112.175ms(86%)") {
		t.Fatalf("the whole-window arm must consume the same numerator:\n%s", line)
	}
	for _, banned := range []string{"58.919", "45%", "55%"} {
		if strings.Contains(line, banned) {
			t.Fatalf("the §20.1-overwritten cumulative must not reach the coverage sentence (%q):\n%s", banned, line)
		}
	}
	// C2 (§24.9 D-2): the disclosure counts ONLY the self-lock row — the ×3
	// sleep hop view (the sentence's own denominator body) is excluded, 计数如实.
	if !strings.Contains(line, "另有 1 条链上行未计入上句已归因数值(单项最大 115.944ms;墙钟不可加和,详见明细/树)。") {
		t.Fatalf("the D-2 disclosure must count 1 (self-lock) and exclude the target's own sleep views:\n%s", line)
	}
	if strings.Contains(line, "另有 4 条") {
		t.Fatalf("the pre-fix conflated count must be dead:\n%s", line)
	}
}

func TestCOVOpendirLegacyChannelWithoutTargetCaliber(t *testing.T) {
	// M2 negative control: no typed target caliber → the legacy sentence bytes
	// stand (the fabrication is an ENGINE-channel property this batch only
	// stops CONSUMING; the honest fallback keeps the old numbers verbatim).
	projection := covOpendirProjection(0)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "关注线程睡眠 115.353ms 中 58.919ms 已由链上解释。") {
		t.Fatalf("without the typed caliber the legacy hop line must stand byte-identically:\n%s", line)
	}
}

func TestCOVNumeratorNeverFabricatesResidual(t *testing.T) {
	// 严禁伪造残差: an under-explaining typed caliber publishes AS-IS — the
	// numerator is never clamped toward the denominator/sleep.
	projection := covOpendirProjection(60.0)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if got := runtimeTraceProjDepth1Cumulative(model); got != 60.0 {
		t.Fatalf("an under-explaining caliber must publish as-is (60.000), got %.3f", got)
	}
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "关注线程睡眠 115.353ms 中 60.000ms 已由链上解释。") {
		t.Fatalf("the shortfall must be disclosed honestly, never inflated:\n%s", line)
	}
	if strings.Contains(line, "115.353ms 中 115.353ms") {
		t.Fatalf("the numerator must never clamp to the denominator:\n%s", line)
	}
}

// --- C2: the disclosure's typed state-view exclusion --------------------------

func TestCOVDisclosureStateViewPredicate(t *testing.T) {
	// The typed 目标自身状态行 predicate: pure state re-descriptions (hop rows
	// whose influence point is itself a registered scheduler-state token) are
	// the denominator body; a blocked-wait row ON a counterpart — resolved or
	// the F6 type-token reject — stays a countable disclosure member.
	sleepView := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
		Node: types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleCausalHop, Subject: "aweme-16547",
			Object: "s_sleep", StateKind: "s_sleep",
		}}
	lockWait := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
		Node: types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleCausalHop, Subject: "app_main-100",
			Object: "lock_contention", StateKind: "s_sleep", TypeToken: "monitor_contention",
		}}
	selfLock := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
		Node: types.TraceCausalProjectionNode{
			Role: types.TraceCausalRolePrimaryRootCause, Subject: "aweme-16547",
			Object: "blocking_span", TypeToken: "blocking_span",
		}}
	if !runtimeTraceProjTargetSelfStateViewRow(sleepView) {
		t.Fatalf("a hop sleep re-description (Object=s_sleep) is the denominator body")
	}
	if runtimeTraceProjTargetSelfStateViewRow(lockWait) {
		t.Fatalf("a blocked-wait hop row on an (unresolved) counterpart must stay a disclosure member (F6 roster honesty)")
	}
	if runtimeTraceProjTargetSelfStateViewRow(selfLock) {
		t.Fatalf("the unclassified self-lock row must stay a disclosure member (计数如实)")
	}
}

// --- C3: denominator-population collapse form-switch --------------------------

// covHuadongCoverageProjection mirrors the huadong_78 C-3 shape: ONE tiny
// D-state row feeds the denominator (0.011, anchor window) while the target's
// binder wait (4.577, no StateKind) and its ×29 multi-window sleep view
// (single-instance max 6.661) are excluded, and the depth-1 chain row (4.431)
// was measured in a positively different query window (crossBase).
func covHuadongCoverageProjection(offWindowBinder bool) types.TraceCausalProjection {
	binder := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-binder",
		Subject: "oney-42591", Object: "binder_wait", TypeToken: "binder_wait",
		ImpactMS: 4.577, EffectiveImpactMS: 4.577, Confidence: 0.8,
	}
	if offWindowBinder {
		binder.QueryWindowStartTs, binder.QueryWindowEndTs = 100.200, 100.300
	}
	return types.TraceCausalProjection{
		WakeupPath:    []string{"tpp-189", "oney-42591"},
		WindowStartTs: 100.000,
		WindowEndTs:   100.101,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
				Subject: "tpp-189", Object: "sleep_wait",
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
				ImpactMS: 4.431, CumulativeImpactMS: 4.431,
				QueryWindowStartTs: 100.500, QueryWindowEndTs: 100.652,
				Confidence: 0.8,
			},
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-dstate",
				Subject: "oney-42591", Object: "d_state", StateKind: "d_state",
				ImpactMS:           0.011,
				QueryWindowStartTs: 100.000, QueryWindowEndTs: 100.101,
				Confidence: 0.8,
			},
			binder,
		},
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-sleepview",
			Subject: "oney-42591", Object: "s_sleep", StateKind: "s_sleep",
			Predicate: "wakeup_causal_impact",
			ImpactMS:  6.661, EffectiveImpactMS: 6.661,
			MergedCount: 29, MergedMinMS: 1.035, MergedMaxMS: 6.661,
			MergedQueryWindows: []types.TraceCausalProjectionQueryWindow{
				{StartTs: 100.000, EndTs: 100.101}, {StartTs: 100.200, EndTs: 100.300},
			},
			Confidence: 0.78,
		}},
	}
}

func TestCOVCrossBaseDenominatorCollapseFormSwitch(t *testing.T) {
	// huadong_78 形 (§24.11 C-3): the crossBase arm switches FORM — 0.011ms
	// never masquerades as the 全称 "关注线程等待".
	projection := covHuadongCoverageProjection(false)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if got := runtimeTraceProjTargetSymptomMS(model); got != 0.011 {
		t.Fatalf("fixture drifted: the denominator must be the lone 0.011 D-state row, got %.3f", got)
	}
	line := runtimeTraceProjWindowLine(projection, model, true)
	want := "仅计入分析窗内直接等待 0.011ms;另有 2 条关注线程状态行未计入分母(单项最大 6.661ms);链上单项最大 4.431ms — 链上/自身数据横跨多个查询窗,分子分母窗基不可证同基:不给出覆盖百分比,不计未归因。"
	if !strings.Contains(line, want) {
		t.Fatalf("the crossBase collapse must switch to the census disclosure form:\n%s", line)
	}
	if strings.Contains(line, "关注线程等待(sleep/D-state/runnable) 0.011ms") {
		t.Fatalf("0.011ms must not masquerade as the 全称 wait claim:\n%s", line)
	}
	en := runtimeTraceProjWindowLine(projection, buildRuntimeTraceProjTreeModel(projection, nil, false), false)
	if !strings.Contains(en, "Only the direct wait inside the analysis window, 0.011ms, is counted; 2 more focused-thread state row(s) are not in the denominator (single largest 6.661ms)") {
		t.Fatalf("EN crossBase collapse must fork the same way:\n%s", en)
	}
}

func TestCOVCrossBaseCollapseAllOffWindowForm(t *testing.T) {
	// Every excluded row provably off-window (typed identities: binder carries
	// an off-anchor query window; the ×29 sleep view is multi-window merged) →
	// the 在其他查询窗 form renders (the task's canonical wording).
	projection := covHuadongCoverageProjection(true)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "仅计入分析窗内直接等待 0.011ms;另有 2 条关注线程状态行在其他查询窗,未计入分母(单项最大 6.661ms)") {
		t.Fatalf("the all-off-window census must name the other query windows:\n%s", line)
	}
}

// covCmp70Projection mirrors the cmp_78_01 7.0-side collapse WITHOUT a window
// conflict (coordinator supplement): the parenthetical claimed
// "(sleep/D-state/runnable)" while a 456.725ms sleep hop view and a 3.843ms
// binder wait (no StateKind) were silently excluded by the Role/StateKind
// lanes and only a tiny runnable row (3.262) fed the denominator.
func covCmp70Projection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "main-6565"},
		WindowStartTs: 100.000,
		WindowEndTs:   101.800,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
				Subject: "waker-1", Object: "sleep_wait",
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
				ImpactMS: 65.232, CumulativeImpactMS: 65.232, Confidence: 0.8,
			},
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-runnable",
				Subject: "main-6565", Object: "runnable", StateKind: "runnable",
				ImpactMS: 3.262, Confidence: 0.8,
			},
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-binder",
				Subject: "main-6565", Object: "binder_wait", TypeToken: "binder_wait",
				ImpactMS: 3.843, EffectiveImpactMS: 3.843, Confidence: 0.8,
			},
		},
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-sleepview",
			Subject: "main-6565", Object: "s_sleep", StateKind: "s_sleep",
			Predicate: "wakeup_causal_impact",
			ImpactMS:  456.725, EffectiveImpactMS: 456.725, Confidence: 0.78,
		}},
	}
}

func TestCOVRoleLaneCollapseFormSwitchWithoutWindowConflict(t *testing.T) {
	// EVOLUTION RECORD (DISP-3 §29.8 P2-⑤ "textup 覆盖句分母排除目标 sleep",
	// real_trace_campaign_20260705.md, 2026-07-09; textup_792 witness line 60).
	// The pre-DISP-3 pin asserted the census disclaimer ("仅计入分析窗内直接等待
	// 3.262ms…分母未覆盖…不给出覆盖百分比") on this shape. The DISP-3 admission
	// arm REPAIRS the under-representation itself: when the admitted state rows
	// carry no sleep-family member, the largest sleep-state hop view joins the
	// denominator (the F1 nesting concern is vacuous — there is no enclosing
	// sleep segment for it to double count against), so the sentence renders
	// the honest full-denominator percentage instead of refusing arithmetic.
	// The §24.11 C-3 principle (the counted slice must never masquerade as the
	// 全称 wait) is satisfied by completing the denominator; the census
	// disclaimer stays pinned on the window-conflicted / multi-window shapes
	// (TestCOVCrossBase* above and the multi-window variant below).
	projection := covCmp70Projection()
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if got := runtimeTraceProjTargetSymptomMS(model); got != 459.987 {
		t.Fatalf("the sleep hop must join the runnable denominator (3.262+456.725), got %.3f", got)
	}
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "关注线程等待(sleep/D-state/runnable) 459.987ms 中链上已归因 65.232ms(14%),未归因 394.755ms(86%)。") {
		t.Fatalf("the admitted-hop shape must render the honest full-denominator arm:\n%s", line)
	}
	if strings.Contains(line, "仅计入分析窗内直接等待") {
		t.Fatalf("the census disclaimer must not fire once the denominator is complete:\n%s", line)
	}
	// Multi-window sleep view (huadong_78 ×N family): the admission stays
	// closed and the pinned census disclaimer renders unchanged.
	crossWin := covCmp70Projection()
	crossWin.SupportingHops[0].MergedCount = 29
	crossWin.SupportingHops[0].MergedMinMS = 1.035
	crossWin.SupportingHops[0].MergedMaxMS = 456.725
	crossWin.SupportingHops[0].MergedQueryWindows = []types.TraceCausalProjectionQueryWindow{
		{StartTs: 100.000, EndTs: 100.101}, {StartTs: 100.200, EndTs: 100.300},
	}
	crossModel := buildRuntimeTraceProjTreeModel(crossWin, nil, true)
	if got := runtimeTraceProjTargetSymptomMS(crossModel); got != 3.262 {
		t.Fatalf("a multi-window sleep view must stay out of the denominator, got %.3f", got)
	}
	crossLine := runtimeTraceProjWindowLine(crossWin, crossModel, true)
	// The ×N member windows positively disagree → the §21.1 CWD-2 ③ crossBase
	// census wording (the huadong_78 家族 form, same as TestCOVCrossBase*).
	want := "仅计入分析窗内直接等待 3.262ms;另有 2 条关注线程状态行未计入分母(单项最大 456.725ms);链上单项最大 65.232ms — 链上/自身数据横跨多个查询窗,分子分母窗基不可证同基:不给出覆盖百分比,不计未归因。"
	if !strings.Contains(crossLine, want) {
		t.Fatalf("the collapse must keep the census disclaimer on the multi-window shape:\n%s", crossLine)
	}
	// The parenthetical must never claim a state family the denominator
	// silently excluded (声称含 sleep 就不能静默排除全部 sleep 行).
	if strings.Contains(crossLine, "(sleep/D-state/runnable) 3.262ms") {
		t.Fatalf("the 全称 family claim must not render over a collapsed denominator:\n%s", crossLine)
	}
	en := runtimeTraceProjWindowLine(crossWin, buildRuntimeTraceProjTreeModel(crossWin, nil, false), false)
	if !strings.Contains(en, "the chain/self data spans multiple query windows and the numerator/denominator window bases cannot be proven identical: no coverage percentage, no unattributed residual") {
		t.Fatalf("EN collapse form missing:\n%s", en)
	}
}

func TestCOVHealthyNestedShapeKeepsLegacyArms(t *testing.T) {
	// C3 negative pin: the F1 healthy nested shape (a hop view SMALLER than
	// its enclosing admitted state sum) keeps every legacy arm byte-identically
	// — the census fork fires only on the numerically impossible nesting.
	projection := covCmp70Projection()
	projection.OnChainCauses[1].ImpactMS = 10.000 // admitted sleep-family denominator
	projection.OnChainCauses[1].Object, projection.OnChainCauses[1].StateKind = "sleep_wait", "s_sleep"
	projection.OnChainCauses[2].ImpactMS = 0 // binder row carries no wall clock of its own
	projection.OnChainCauses[2].EffectiveImpactMS = 0
	projection.SupportingHops[0].ImpactMS = 8.000 // nested hop view ≤ admitted
	projection.SupportingHops[0].EffectiveImpactMS = 8.000
	projection.OnChainCauses[0].ImpactMS = 9.500
	projection.OnChainCauses[0].CumulativeImpactMS = 9.500
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "关注线程等待(sleep/D-state/runnable) 10.000ms 中链上已归因 9.500ms(95%),未归因 0.500ms(5%)。") {
		t.Fatalf("the healthy nested shape must keep the legacy percentage arm:\n%s", line)
	}
	if strings.Contains(line, "仅计入分析窗内直接等待") {
		t.Fatalf("the census form must not fire on the healthy nested shape:\n%s", line)
	}
}

// --- C4: 链上单项最大 (名实对齐) + cmp_78_01 6.0 侧 legacy control -------------

func TestCOVCmp60SideWholeWindowShapeAndNoTotalsWord(t *testing.T) {
	// cmp_78_01 6.0 侧 witness (coordinator supplement): whole-window arm +
	// hop info line, legacy channel (no typed target caliber on the fixture) —
	// and the retired 合计 冒名 word never renders. NOTE (留账,不在本批 scope):
	// the whole-window 未归因 94% counts the target's RUNNING time as
	// unattributed — a denominator-semantics gap, not a numerator-channel one;
	// reported back to the coordinator as an open item.
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-2", "main-21538"},
		WindowStartTs: 100.000,
		WindowEndTs:   101.645,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
			Subject: "waker-2", Object: "sleep_wait",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 92.346, CumulativeImpactMS: 92.346, Confidence: 0.8,
		}},
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-sleepview",
			Subject: "main-21538", Object: "s_sleep", StateKind: "s_sleep",
			Predicate: "wakeup_causal_impact",
			ImpactMS:  470.071, EffectiveImpactMS: 470.071, Confidence: 0.78,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "关注线程睡眠 470.071ms 中 92.346ms 已由链上解释。") {
		t.Fatalf("the 6.0-side hop info line must keep its legacy shape:\n%s", line)
	}
	if !strings.Contains(line, "链上已归因 92.346ms(6%)") {
		t.Fatalf("the 6.0-side whole-window arm must keep the legacy channel:\n%s", line)
	}
	covAssertNoTotalsWord(t, line)
}

// covAssertNoTotalsWord is the C4 negative sweep: the retired 冒名 word (and
// its EN twin) never renders on a coverage surface.
func covAssertNoTotalsWord(t *testing.T, line string) {
	t.Helper()
	for _, banned := range []string{"各链上口径合计", "口径合计", "caliber totals"} {
		if strings.Contains(line, banned) {
			t.Fatalf("the retired totals 冒名 word %q must not render (§24.11 C-3 后半场):\n%s", banned, line)
		}
	}
}

func TestCOVNoTotalsWordAcrossCoverageArms(t *testing.T) {
	// Sweep every fixture family of this file (crossBase collapse, Role-lane
	// collapse, healthy nested, opendir hop-only, cmp 6.0 whole-window) on
	// both language faces: the numerator wording is 链上单项最大 everywhere.
	fixtures := []types.TraceCausalProjection{
		covHuadongCoverageProjection(false),
		covHuadongCoverageProjection(true),
		covCmp70Projection(),
		covOpendirProjection(112.175),
		covOpendirProjection(0),
	}
	for i, projection := range fixtures {
		for _, zh := range []bool{true, false} {
			model := buildRuntimeTraceProjTreeModel(projection, nil, zh)
			line := runtimeTraceProjWindowLine(projection, model, zh)
			covAssertNoTotalsWord(t, line)
			if strings.Contains(line, "合计") && !strings.Contains(line, "取自查询窗") {
				t.Fatalf("fixture %d (zh=%v): no coverage arm may claim a 合计 (墙钟不可求和):\n%s", i, zh, line)
			}
		}
	}
}
