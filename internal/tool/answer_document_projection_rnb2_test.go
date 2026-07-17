package tool

// answer_document_projection_rnb2_test.go — RNB-2 显示诚实批 display pins
// (§29.88 W3 病①②③, ledger docs/design/real_trace_campaign_20260705.md;
// witness /Users/han/opt/customlogs/runnable.txt E32↔E9/E10, 2026-07-15).
//
//	件1  pair class (0) case-A fallback 选席修真 — value-verified candidacy,
//	     anchored==0 mints no pointer, ≥2 verified candidates fail open;
//	件2  R2 ×N merge anchorForm fork key + merge-body triple handling
//	     (seed 三元组不得冒充合并行账);
//	件3  锚定 0.000 括注词面 — the ownership bracket 「(⛓链上席)」 downgrades
//	     to 「(无锚定段)」 when no seat holds any anchored share.
//
// Every pin here is a would-be-red on the pre-RNB-2 code: dropping the
// anchored>0 gate re-mints the E32↔E9 false pointer; dropping the value gate
// re-admits the 5.368-for-0.000 pick; dropping the ambiguity gate re-enables
// the largest-seat coin flip; dropping the anchorForm fork re-merges the
// remainder seat with plain ◇ rows (10.643 行1 beside a 9.272 行2 「本行」).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// --- 件1: pair class (0) case-A fallback ------------------------------------

func rnb2RemainderRow(anchored, full, impact float64) runtimeTraceProjTreeRow {
	return runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "workShark-6666", TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
			ChainRelevance: "adjacent", ImpactMS: impact, CumulativeImpactMS: impact,
			ChainAnchoredMS: anchored, ChainAnchorFullMS: full, ChainAnchorRemainderSeat: true,
		},
		Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E32",
	}
}

func rnb2ChainViewRow(tag string, impact float64) runtimeTraceProjTreeRow {
	return runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "workShark-6666", TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
			ChainRelevance: "on_chain", ImpactMS: impact, CumulativeImpactMS: impact,
		},
		Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: tag,
	}
}

// 件1(i): a 0.000-anchored remainder has NO anchored-share holder — the case-A
// fallback must not name one (the customer E32↔E9 shape: 「⛓席(本行)=凭证锚定
// 段合计…合计还原全窗账 9.272」 was arithmetically false on E9's 5.368).
func TestRNB2ZeroAnchoredRemainderMintsNoPairPointer(t *testing.T) {
	remainder := rnb2RemainderRow(0, 9.272, 9.272)
	view := rnb2ChainViewRow("E9", 5.368)
	model := runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{view},
		Adjacent: []runtimeTraceProjTreeRow{remainder},
	}
	runtimeTraceProjMarkAccountRelations(&model, true)
	if got := model.Adjacent[0].AccountRelRef; got != "" {
		t.Fatalf("anchored==0 must mint no pair pointer (宁漏勿假指), got ref=%q", got)
	}
	if got := model.TreeRows[0].AccountRelRef; got != "" {
		t.Fatalf("anchored==0 must leave the chain view row sentence-less, got ref=%q", got)
	}
	if model.TreeRows[0].AccountRelSameSourceAnchoredSide {
		t.Fatalf("no row may claim the anchored side of a 0.000-anchored split")
	}
}

// 件1(ii): a case-A candidate whose published display does NOT µs-equal the
// remainder's ChainAnchoredMS is never named as the anchored-share holder.
func TestRNB2ValueMismatchedChainSeatIsNotPicked(t *testing.T) {
	remainder := rnb2RemainderRow(3.598, 36.757, 33.159)
	view := rnb2ChainViewRow("E9", 5.368) // ≠ 3.598 anchored
	model := runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{view},
		Adjacent: []runtimeTraceProjTreeRow{remainder},
	}
	runtimeTraceProjMarkAccountRelations(&model, true)
	if got := model.Adjacent[0].AccountRelRef; got != "" {
		t.Fatalf("value-mismatched chain seat must fail the case-A value gate, got ref=%q", got)
	}
}

// 件1(iii): TWO value-verified case-A candidates are genuinely ambiguous —
// fail open (the pre-RNB-2 largest-seat tiebreak is retired on this arm).
func TestRNB2AmbiguousChainSeatCandidatesFailOpen(t *testing.T) {
	remainder := rnb2RemainderRow(1.759, 31.191, 29.432)
	a := rnb2ChainViewRow("E2", 1.759)
	b := rnb2ChainViewRow("E3", 1.759)
	model := runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{a, b},
		Adjacent: []runtimeTraceProjTreeRow{remainder},
	}
	runtimeTraceProjMarkAccountRelations(&model, true)
	if got := model.Adjacent[0].AccountRelRef; got != "" {
		t.Fatalf("≥2 value-verified candidates must fail open (禁猜), got ref=%q", got)
	}
}

// 件1(ii) clipped arm: a clipped counterpart from a DIFFERENT bipartition
// (display ≠ this remainder's anchored share) must not be spoken of as the ⛓
// half of THIS remainder's account.
func TestRNB2ClippedPickValueGateBindsToo(t *testing.T) {
	remainder := rnb2RemainderRow(3.598, 36.757, 33.159)
	clipped := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "workShark-6666", TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
			ChainRelevance: "on_chain", ImpactMS: 2.000, CumulativeImpactMS: 2.000,
			ChainAnchoredMS: 2.000, ChainAnchorFullMS: 5.100,
		},
		Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: "E4",
	}
	model := runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{clipped},
		Adjacent: []runtimeTraceProjTreeRow{remainder},
	}
	runtimeTraceProjMarkAccountRelations(&model, true)
	if got := model.Adjacent[0].AccountRelRef; got != "" {
		t.Fatalf("a different-bipartition clipped row must fail the value gate, got ref=%q", got)
	}
}

// 件1 positive control: the healthy single value-verified case-A candidate
// keeps its pointer byte-identically (the pre-existing
// TestRSPARemainderPairsToChainLaneSeatWhenNoClippedRow contract).
func TestRNB2HealthyCaseAPairStillMints(t *testing.T) {
	remainder := rnb2RemainderRow(1.759, 31.191, 29.432)
	seat := rnb2ChainViewRow("E2", 1.759)
	model := runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{seat},
		Adjacent: []runtimeTraceProjTreeRow{remainder},
	}
	runtimeTraceProjMarkAccountRelations(&model, true)
	if model.Adjacent[0].AccountRelRef != "E2" || model.Adjacent[0].AccountRelSameSourceFullMS != 31.191 {
		t.Fatalf("the healthy case-A pair must keep minting: ref=%q full=%.3f",
			model.Adjacent[0].AccountRelRef, model.Adjacent[0].AccountRelSameSourceFullMS)
	}
}

// 件2 捎带 (mirror-arm carve): a merged row whose member triples were cleared
// is a KNOWN Σ of split accounts — value-equality with some pair's full
// account must not admit it to the full-window MIRROR arm.
func TestRNB2MergedClearedRowNeverMirrorCandidate(t *testing.T) {
	anchored := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "workShark-6666", TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
			ChainRelevance: "on_chain", ImpactMS: 5.000, CumulativeImpactMS: 5.000,
			ChainAnchoredMS: 5.000, ChainAnchorFullMS: 10.643,
		},
		Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: "E1",
	}
	remainder := rnb2RemainderRow(5.000, 10.643, 5.643)
	merged := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "workShark-6666", TypeToken: "d_state_or_io_wait", StateKind: "d_sleep",
			ChainRelevance: "adjacent", ImpactMS: 10.643, CumulativeImpactMS: 10.643,
			MergedCount: 3, ChainAnchorRemainderSeat: true, MergedChainAnchorMemberAccounts: true,
		},
		Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E5",
	}
	model := runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{anchored},
		Adjacent: []runtimeTraceProjTreeRow{remainder, merged},
	}
	runtimeTraceProjMarkAccountRelations(&model, true)
	if model.Adjacent[1].AccountRelMirrorAnchoredRef != "" || model.Adjacent[1].AccountRelMirrorRemainderRef != "" {
		t.Fatalf("a merged-cleared Σ row must never wear the full-window mirror sentence: %+v",
			model.Adjacent[1])
	}
}

// --- 件6: divergent 行2 双「锚定」词面消歧 -------------------------------------

// 件6 (§29.90 残留③ P4): when the divergent row's own anchored share differs
// from the pid-census Σ (satellite interval account / multi-group stamped
// seat), the two 「锚定」 mentions each carry their account qualifier; the
// µs-equal window-seat form keeps the RNB-1 pinned bytes (negative arm lives
// in TestRSPARNBDivergentRemainderRendersDoubleAccountSentence).
func TestRNB2DivergentRowDisambiguatesTwoAnchoredWords(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"sat-59953", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rnb2-divergent-sat",
			Subject: "sat-59953", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "adjacent",
			ImpactMS: 2.438, CumulativeImpactMS: 2.438,
			ChainAnchoredMS: 8.338, ChainAnchorFullMS: 10.776, ChainAnchorRemainderSeat: true,
			ChainAnchorOwnershipDivergent: true, ChainAnchorChainLaneMS: 5.0, ChainAnchorCensusMS: 7.1,
			Rank: 1, Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}},
	}
	// Space-stripped comparison (existing precedent — the width governor may
	// break a line exactly on a space, which the joiner cannot restore).
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := strings.ReplaceAll(rspaFenceJoined(runtimeTraceProjTreeFence(model, true)), " ", "")
	if !strings.Contains(fence, "全窗10.776ms=本席锚定8.338ms+本行其余2.438ms(无链上凭证);链席自账Σ5.000ms与pid全窗锚定账Σ7.100ms失合(差2.100ms)") {
		t.Fatalf("differing anchored values must each carry their account qualifier:\n%s", fence)
	}
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := strings.ReplaceAll(rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false)), " ", "")
	if !strings.Contains(fenceEN, "whole-window10.776ms=8.338msanchoredbythisseat'sownaccount+thisremainder2.438ms(nochaincredential);thechainseat'sownΣ5.000msdivergesfromthepid-wideanchored-ledgerΣ7.100ms(delta2.100ms)") {
		t.Fatalf("en mirror of the disambiguated pair missing:\n%s", fenceEN)
	}
}

// --- 件2: trunk ×2 fold exclusion + merged-row 行2 qualifier ------------------

// 件2: re-anchored / lane-demoted rows are never "plain occurrence" material —
// the trunk ×2 fold must fail open on them (engine account identity a display
// re-merge would corrupt).
func TestRNB2TrunkFoldExcludesAnchorFormRows(t *testing.T) {
	plain := types.TraceCausalProjectionNode{
		Subject: "t-1", StateKind: "d_sleep", ImpactMS: 1.0,
	}
	if !runtimeTraceProjTrunkPlainStateOccurrence(plain) {
		t.Fatalf("fixture: the plain row must qualify")
	}
	remainder := plain
	remainder.ChainAnchorRemainderSeat = true
	remainder.ChainAnchorFullMS = 9.272
	clipped := plain
	clipped.ChainAnchorFullMS = 5.1
	clipped.ChainAnchoredMS = 2.0
	demoted := plain
	demoted.ChainCredentialLaneDemoted = true
	for name, node := range map[string]types.TraceCausalProjectionNode{
		"remainder": remainder, "clipped": clipped, "demoted": demoted,
	} {
		if runtimeTraceProjTrunkPlainStateOccurrence(node) {
			t.Fatalf("the %s form must fail open from the trunk ×2 fold", name)
		}
	}
}

// 件2: the merged-cleared row speaks the seed-member qualifier on 行2 — never
// the per-seat 「全窗X=…本行其余Y」 arithmetic it can no longer own.
func TestRNB2MergedMemberAccountsQualifierRenders(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"workShark-6666", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rnb2-merged-remainder",
			Subject: "workShark-6666", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
			StateKind: "d_sleep", ChainRelevance: "adjacent",
			ImpactMS: 10.643, CumulativeImpactMS: 10.643,
			MergedCount: 3, MergedMinMS: 0.478, MergedMaxMS: 9.272,
			ChainAnchorRemainderSeat: true, MergedChainAnchorMemberAccounts: true,
			Rank: 1, Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "同源二分账留在各成员(种子成员账,不代表本合并行合计);成员拆分见证据索引") {
		t.Fatalf("the merged row must speak the seed-member qualifier:\n%s", fence)
	}
	if strings.Contains(fence, "本行其余") || strings.Contains(fence, "同源二分:全窗") {
		t.Fatalf("the per-seat bipartition arithmetic may not ride a member-Σ row:\n%s", fence)
	}
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !rspaFenceContains(fenceEN, "the same-source split accounts stay on the individual members (seed-member accounts, never this merged row's total); see the evidence index for the member splits") {
		t.Fatalf("en mirror of the seed-member qualifier missing:\n%s", fenceEN)
	}
}

// --- 件5: AFF-EVID 约束描述行 --------------------------------------------------

// 件5 (§29.88.6): the affinity/cpuset seat renders the typed constraint
// description on its own face — allowed set vs observed exclusion + group +
// restricted flag + basis kind; a payload-less legacy row renders nothing.
func TestRNB2AffinityConstraintDescriptionRenders(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"logd.writer-9163", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rnb2-affinity",
			Subject: "logd.writer-9163", Object: "cpu_affinity_or_cpuset", TypeToken: "cpu_affinity_or_cpuset",
			StateKind: "runnable", ChainRelevance: "adjacent",
			ImpactMS: 47.678, CumulativeImpactMS: 47.678,
			ChainCredentialLaneDemoted: true,
			CPUConstraintKind:          "sched_switch_next_info",
			CPUConstraintCPUSet:        "background",
			CPUConstraintPolicy:        "next_info affinity=ffb group=2 restricted=true",
			CPUConstraintAllowedCPUs:   []int{0, 1, 3, 4, 5, 6, 7, 8, 9, 10, 11},
			CPUConstraintExcludedCPUs:  []int{2, 12, 13},
			// R5a (§29.88.4 场景② 按核档, RNB-4): the tier-exclusion proof
			// pair — the obligatory mention's inputs (donghu mask=ffb shape).
			CPUConstraintAllowedMaxTierKHz: 2270000,
			CPUConstraintGlobalMaxTierKHz:  2750000,
			Rank:                           1, Confidence: 0.72, LineStart: 10, LineEnd: 20,
		}, {
			// Payload-less legacy affinity row — negative arm (no description).
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rnb2-affinity-legacy",
			Subject: "hilogcat-9503", Object: "cpu_affinity_or_cpuset", TypeToken: "cpu_affinity_or_cpuset",
			StateKind: "runnable", ChainRelevance: "adjacent",
			ImpactMS: 16.013, CumulativeImpactMS: 16.013,
			Rank: 2, Confidence: 0.64, LineStart: 30, LineEnd: 40,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := strings.ReplaceAll(rspaFenceJoined(runtimeTraceProjTreeFence(model, true)), " ", "")
	// B11 (DISPLAY-HYG 二轮): the tier pair wears GHz (÷1e6, %.2f) — the
	// space-stripped compare keeps the pin wrap-independent.
	if !strings.Contains(fence, "CPU约束描述:允许核0-1,3-11·排除全域观测核2,12-13·绑核排除更大核档(允许核最高档2.27GHz<全域最大核档2.75GHz)·cpuset组background·策略restricted=true·判定依据sched_switch_next_info") {
		t.Fatalf("the constraint description must render from the typed payload (R5a mention included):\n%s", fence)
	}
	if strings.Count(fence, "CPU约束描述") != 1 {
		t.Fatalf("a payload-less affinity row must render no description:\n%s", fence)
	}
	// R5a negative arm (禁无中生有): the pair-less legacy row must not wear
	// the mention.
	if strings.Count(fence, "绑核排除更大核档") != 1 {
		t.Fatalf("the tier-exclusion mention must render exactly on the proof-bearing seat:\n%s", fence)
	}
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := strings.ReplaceAll(rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false)), " ", "")
	if !strings.Contains(fenceEN, "CPU-constraintdescription:allowedCPUs0-1,3-11·excludesobservedCPUs2,12-13·bindingexcludesabiggercoretier(allowedmaxtier2.27GHz<globalmaxtier2.75GHz)·cpusetgroupbackground·policyrestricted=true·basissched_switch_next_info") {
		t.Fatalf("en mirror of the constraint description missing:\n%s", fenceEN)
	}
}

// 件5: the wire round trip — the emitted cpu_constraint_* notes decode back
// into the projection node fields (R2' 消费面).
func TestRNB2AffinityPayloadWireRoundTrip(t *testing.T) {
	item := tracequery.RootCauseRankItem{
		Rank: 1, Tier: "primary", Type: "cpu_affinity_or_cpuset",
		Thread:   tracequery.ThreadRef{Comm: "logd.writer", PID: 9163},
		ImpactMs: 47.678, ProjectedImpactMs: 47.678, CumulativeImpactMs: 47.678,
		EffectiveImpactMs: 47.678, Score: 0.5, Confidence: 0.72,
		LineStart: 10, LineEnd: 20,
		Source:    "window_stats.cpu_constraints",
		Causality: "adjacent_to_wakeup_chain", ChainRelevance: "adjacent",
		DominantState: string(tracequery.StateRunnable), RunnableMs: 47.678,
		CPUConstraintKind:              "sched_switch_next_info",
		CPUConstraintCPUSet:            "background",
		CPUConstraintPolicy:            "next_info affinity=ffb restricted=true",
		CPUConstraintAllowedCPUs:       []int{0, 1, 3},
		CPUConstraintExcludedCPUs:      []int{2, 12, 13},
		CPUConstraintAllowedMaxTierKHz: 2270000,
		CPUConstraintGlobalMaxTierKHz:  2750000,
		Summary:                        "affinity constraint witness",
	}
	notes := traceQueryTypedRootCauseStateRichNotes(item)
	joined := strings.Join(notes, "\n")
	for _, want := range []string{
		"cpu_constraint_kind=sched_switch_next_info",
		"cpu_constraint_cpuset=background",
		"cpu_constraint_allowed_cpus=0,1,3",
		"cpu_constraint_excluded_cpus=2,12,13",
		"cpu_constraint_allowed_max_tier_khz=2270000",
		"cpu_constraint_global_max_tier_khz=2750000",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("note %q must ride the wire, got:\n%s", want, joined)
		}
	}
}

// --- 件3: 锚定 0.000 括注词面 -------------------------------------------------

// 件3: when anchored==0 the remainder 行2's ownership bracket must not claim a
// chain seat holds the (empty) anchored share — zh 「(无锚定段)」 / en "(no
// anchored share exists)"; the twin-invisible arm (RNB-1 D1) keeps handling
// the orthogonal 「席不可见」 form on anchored>0 rows.
func TestRNB2ZeroAnchoredBracketSpeaksNoAnchoredShare(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"workShark-6666", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rnb2-zero-anchored",
			Subject: "workShark-6666", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
			StateKind: "d_sleep", ChainRelevance: "adjacent",
			ImpactMS: 9.272, CumulativeImpactMS: 9.272,
			ChainAnchoredMS: 0, ChainAnchorFullMS: 9.272, ChainAnchorRemainderSeat: true,
			Rank: 1, Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "同源二分:全窗9.272ms=锚定0.000ms(无锚定段)+本行其余9.272ms(无链上凭证)") {
		t.Fatalf("the zero-anchored remainder must speak the 无锚定段 bracket:\n%s", fence)
	}
	if strings.Contains(fence, "(⛓链上席)") || strings.Contains(fence, "锚定席未上本榜") {
		t.Fatalf("a 0.000 anchored share has NO owner — neither ownership bracket may render:\n%s", fence)
	}
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !rspaFenceContains(fenceEN, "same-source split: full-window 9.272ms=0.000ms anchored (no anchored share exists) + this remainder 9.272ms (no chain credential)") {
		t.Fatalf("en mirror of the zero-anchored bracket missing:\n%s", fenceEN)
	}
}
