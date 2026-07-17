package tool

// answer_document_projection_rspa_test.go — EVOLUTION RECORD (RSPA
// §29.61.10a/b/c, 2026-07-14): display pins for the on-chain seat-value
// re-anchoring bipartition, built on the donghu witness numbers — CompThread
// D 36.757 = 3.598 anchored + 33.159 remainder; JankManager runnable
// census-full 31.191 = 1.759 anchored + 29.432 remainder.
//
//	件a  the ◇ remainder half renders the 行2 同源二分 disclosure
//	     (全窗=锚定+其余) and its runnable-family 调度压力候选 category word
//	     on the adjacent channel (邻近影响#N chip, never 根因排序#N);
//	件b  the ⛓ clipped D/IO half renders its own half of the disclosure
//	     (本行锚定 + ◇余段席 tail);
//	件c  the WO-C1-family relation sentence renders on BOTH halves with the
//	     full-account value and the [E#] cross-references both ways (typed
//	     AccountRelSameSource* fields, the ONLY additive seat relation);
//	件d  the 同源二分 / 合计还原全窗账 legend entries render exactly when
//	     their marks fire;
//	件e  the M-IO completion-closure credential renders as a small demoted
//	     context word on the io_latency row (typed per-IO field).
//
// MUTATION self-checks:
//   - dropping the ChainAnchorFullMS arm in
//     runtimeTraceProjSameSegMirrorTagTexts reds TestRSPA the disclosure pins
//     (件a/件b) — no other pass emits 同源二分;
//   - dropping pair class (0) in runtimeTraceProjMarkAccountRelations reds
//     件c (the pair would fall to the generic 两套账目 template, whose
//     不可相加 verdict is FALSE on this pair — the 合计还原全窗账 token
//     vanishes);
//   - dropping the ResourceCompletionClosure arm reds 件e.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// rspaFenceJoined re-joins the fence's wrapped continuation lines (the PTV4 T3
// wrap is byte-identical — chunk concatenation reproduces the tag text, only
// the break position and the continuation indent are added), so full-sentence
// pins stay independent of where the width governor happens to break a line.
func rspaFenceJoined(fence string) string {
	var b strings.Builder
	for _, line := range strings.Split(fence, "\n") {
		b.WriteString(strings.TrimLeft(line, "│ \t"))
	}
	return b.String()
}

// rspaFenceContains reports whether the fence carries the pinned sentence,
// compared SPACE-INSENSITIVELY across wrapped continuation lines
// (DISPLAY-WRAP 件①(f), 2026-07-16: the emitters trim the invisible break
// space at EOL — catalog C1 — so a sentence's inner ASCII spaces are no
// longer reconstructable from the physical lines; word order and every
// non-space byte stay pinned).
func rspaFenceContains(fence, pin string) bool {
	squash := func(s string) string { return strings.ReplaceAll(s, " ", "") }
	return strings.Contains(squash(rspaFenceJoined(fence)), squash(pin))
}

// rspaSameSourceSplitProjection is the donghu-witness geometry: a case-B
// clipped D/IO chain seat with its ◇ remainder twin (same segment set, lines
// shared — the engine clone keeps the physical evidence identity), a second
// runnable-family ◇ remainder with NO rendered counterpart (mark-A-only
// shape), and an io_latency row carrying the M-IO completion-closure
// credential. Also the representative shape feeding the NEW-7 bidirectional
// legend sweep (answer_document_mutation_runtime_revisit76_test.go).
func rspaSameSourceSplitProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"CompThread-2955", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			// ⛓ case-B clipped D/IO seat — the anchored half keeps the chain
			// seat; the published value channels carry ONLY the credentialed
			// portion (engine rspaClipSeatToAnchored form).
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "rspa-dio-anchored",
			Subject: "CompThread-2955", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
			StateKind: "d_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 3.598, CumulativeImpactMS: 3.598, EffectiveImpactMS: 3.598,
			ChainAnchoredMS: 3.598, ChainAnchorFullMS: 36.757,
			Rank: 1, Tier: "primary", Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}, {
			// M-IO closure witness: the completion thread's io_latency row
			// wears the typed per-IO credential (件e).
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rspa-io-closure",
			Subject: "irq/143-ufs-1234", Object: "io_latency", TypeToken: "io_latency",
			ChainRelevance: "on_chain", ChainDepth: 2,
			ImpactMS: 1.4, CumulativeImpactMS: 1.4, EffectiveImpactMS: 1.4,
			ResourceCompletionClosure: true,
			Rank:                      2, Tier: "secondary", Confidence: 0.8, LineStart: 60, LineEnd: 70,
		}},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			// ◇ remainder twin of the clipped D/IO seat above (engine
			// rspaCloneAsRemainderSeat form: shared lines, remainder value).
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rspa-dio-remainder",
			Subject: "CompThread-2955", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
			StateKind: "d_sleep", ChainRelevance: "adjacent",
			ImpactMS: 33.159, CumulativeImpactMS: 33.159,
			ChainAnchoredMS: 3.598, ChainAnchorFullMS: 36.757, ChainAnchorRemainderSeat: true,
			Rank: 1, Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}, {
			// ◇ runnable remainder (JankManager census-full witness) — its
			// counterpart is rank-suppressed off the render, so only the 行2
			// disclosure fires (件a; no relation sentence, 宁漏勿假指).
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rspa-runnable-remainder",
			Subject: "JankManager-4321", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "adjacent",
			ImpactMS: 29.432, CumulativeImpactMS: 29.432,
			ChainAnchoredMS: 1.759, ChainAnchorFullMS: 31.191, ChainAnchorRemainderSeat: true,
			Rank: 2, Confidence: 0.8, LineStart: 80, LineEnd: 90,
		}},
	}
}

// 件a: the ◇ runnable remainder row — 行2 同源二分 disclosure with the donghu
// numbers plus the runnable family's 调度压力候选 category word on the
// adjacent channel (邻近影响#N, never a chain-channel chip on this row).
func TestRSPARemainderRowRendersSplitDisclosureAndCategory(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rspaSameSourceSplitProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	// EVOLUTION RECORD (RNB-1 D1 修复轮, 2026-07-14): this remainder's ⛓
	// counterpart is rank-suppressed off the render BY FIXTURE DESIGN — the
	// former 「(⛓链上席)」 ownership word was therefore a dangling pointer;
	// the twin-visibility backstop now downgrades it to the honest 未上榜
	// form. The visible pair keeps the ownership word (D-IO pin below).
	if !strings.Contains(fence, "同源二分:全窗31.191ms=锚定1.759ms(锚定席未上本榜,见明细)+本行其余29.432ms(无链上凭证)") {
		t.Fatalf("the ◇ runnable remainder must disclose the bipartition on 行2 (twin-invisible form):\n%s", fence)
	}
	if !strings.Contains(fence, "同源二分:全窗36.757ms=锚定3.598ms") &&
		!strings.Contains(fence, "(⛓链上席)") {
		t.Fatalf("the visible D-IO pair must keep the ownership word:\n%s", fence)
	}
	if !strings.Contains(fence, "调度压力候选·邻近影响#2") {
		t.Fatalf("the remainder keeps its runnable-family category word on the ADJACENT channel:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkChainAnchorSplit) {
		t.Fatalf("the 同源二分 mark must record at the emission site")
	}
	// The counterpart-less runnable remainder carries NO relation sentence
	// (宁漏勿假指 — nothing renders to point at).
	for i := range model.Adjacent {
		if model.Adjacent[i].Node.EvidenceID == "rspa-runnable-remainder" &&
			model.Adjacent[i].AccountRelRef != "" {
			t.Fatalf("a counterpart-less remainder must stay sentence-less: %+v", model.Adjacent[i].AccountRelRef)
		}
	}
	// en mirror of the disclosure.
	modelEN := buildRuntimeTraceProjTreeModel(rspaSameSourceSplitProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !rspaFenceContains(fenceEN, "same-source split: full-window 31.191ms=1.759ms anchored (anchored seat not on this board; see the detail index) + this remainder 29.432ms (no chain credential)") {
		t.Fatalf("en mirror of the remainder disclosure missing:\n%s", fenceEN)
	}
}

// 件b: the ⛓ clipped D/IO half discloses its own side of the split.
func TestRSPAClippedSeatRendersItsHalfOfDisclosure(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rspaSameSourceSplitProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "同源二分:全窗36.757ms=本行锚定3.598ms+其余33.159ms(◇余段席)") {
		t.Fatalf("the ⛓ clipped seat must disclose its half of the bipartition:\n%s", fence)
	}
	modelEN := buildRuntimeTraceProjTreeModel(rspaSameSourceSplitProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !rspaFenceContains(fenceEN, "same-source split: full-window 36.757ms=this row 3.598ms anchored + remainder 33.159ms (◇ remainder seat)") {
		t.Fatalf("en mirror of the clipped-seat disclosure missing:\n%s", fenceEN)
	}
}

// 件c: the relation sentence — full-account value + [E#] cross-references
// both ways, on the RSPA same-source template (never the generic 两套账目
// 不可相加 form, which would be FALSE on this pair).
func TestRSPARelationSentenceCarriesFullAccountAndRefs(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rspaSameSourceSplitProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var rem, clip *runtimeTraceProjTreeRow
	for i := range model.Adjacent {
		if model.Adjacent[i].Node.EvidenceID == "rspa-dio-remainder" {
			rem = &model.Adjacent[i]
		}
	}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows} {
		for i := range rows {
			if rows[i].Node.EvidenceID == "rspa-dio-anchored" {
				clip = &rows[i]
			}
		}
	}
	if rem == nil || clip == nil {
		t.Fatalf("fixture drifted: both halves must render (rem=%v clip=%v)", rem != nil, clip != nil)
	}
	if rem.AccountRelRef != strings.TrimSpace(clip.EvidenceTag) ||
		clip.AccountRelRef != strings.TrimSpace(rem.EvidenceTag) {
		t.Fatalf("the pair must cross-reference both ways: rem→%q (want %q), clip→%q (want %q)",
			rem.AccountRelRef, clip.EvidenceTag, clip.AccountRelRef, rem.EvidenceTag)
	}
	if rem.AccountRelSameSourceFullMS != 36.757 || clip.AccountRelSameSourceFullMS != 36.757 {
		t.Fatalf("both halves must carry the typed full-account value: rem=%.3f clip=%.3f",
			rem.AccountRelSameSourceFullMS, clip.AccountRelSameSourceFullMS)
	}
	if rem.AccountRelSameSourceAnchoredSide || !clip.AccountRelSameSourceAnchoredSide {
		t.Fatalf("side selectors must name the halves: rem anchored=%v clip anchored=%v",
			rem.AccountRelSameSourceAnchoredSide, clip.AccountRelSameSourceAnchoredSide)
	}
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "同源二分对席:⛓席=["+rem.AccountRelRef+"],◇席=本行;合计还原全窗账 36.757ms(规则见图例)") {
		t.Fatalf("the ◇ half must speak the collapsed relation pair with the counterpart's E# (DISPLAY-WRAP 件③(b): the rule clauses live in the legend):\n%s", fence)
	}
	if !strings.Contains(fence, "同源二分对席:◇席=["+clip.AccountRelRef+"],⛓席=本行;合计还原全窗账 36.757ms(规则见图例)") {
		t.Fatalf("the ⛓ half must speak its mirrored collapsed relation pair:\n%s", fence)
	}
	// Negative arm: the generic template's coverage-set wording must NOT tag
	// this pair (its 不可相加 verdict is false here — no row in this fixture
	// may wear the two-accounts sentence).
	if strings.Contains(fence, "两套账目覆盖集不同") {
		t.Fatalf("the same-source pair must never wear the generic two-accounts sentence:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkChainAnchorRelation) {
		t.Fatalf("the relation legend mark must record at the emission site")
	}
}

// 件c fallback arm (case A): with no clipped counterpart rendered, the ◇
// remainder pairs to the thread's chain-lane seat of the same state family.
func TestRSPARemainderPairsToChainLaneSeatWhenNoClippedRow(t *testing.T) {
	remainder := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "JankManager-4321", TypeToken: "runnable_wait", StateKind: "runnable",
			ChainRelevance: "adjacent", ImpactMS: 29.432, CumulativeImpactMS: 29.432,
			ChainAnchoredMS: 1.759, ChainAnchorFullMS: 31.191, ChainAnchorRemainderSeat: true,
		},
		Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E9",
	}
	chainSeat := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "JankManager-4321", TypeToken: "runnable_wait", StateKind: "runnable",
			ChainRelevance: "on_chain", ImpactMS: 1.759, CumulativeImpactMS: 1.759, Rank: 3,
		},
		Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: "E2",
	}
	model := runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{chainSeat},
		Adjacent: []runtimeTraceProjTreeRow{remainder},
	}
	runtimeTraceProjMarkAccountRelations(&model, true)
	if model.Adjacent[0].AccountRelRef != "E2" || model.Adjacent[0].AccountRelSameSourceFullMS != 31.191 {
		t.Fatalf("case-A fallback: the remainder must point at the chain-lane seat: ref=%q full=%.3f",
			model.Adjacent[0].AccountRelRef, model.Adjacent[0].AccountRelSameSourceFullMS)
	}
	if model.TreeRows[0].AccountRelRef != "E9" || !model.TreeRows[0].AccountRelSameSourceAnchoredSide {
		t.Fatalf("the chain seat must point back as the anchored side: ref=%q anchored=%v",
			model.TreeRows[0].AccountRelRef, model.TreeRows[0].AccountRelSameSourceAnchoredSide)
	}
}

// 件d: the legend teaches both word families exactly when the marks fire.
func TestRSPALegendEntriesRenderWithMarks(t *testing.T) {
	projection := rspaSameSourceSplitProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	_ = runtimeTraceProjTreeFence(model, true) // marks record at render
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "`同源二分`") {
		t.Fatalf("the 同源二分 legend entry must render when the split discloses:\n%s", lead)
	}
	if !strings.Contains(lead, "`合计还原全窗账`") {
		t.Fatalf("the 合计还原全窗账 legend entry must render when the relation sentence fires:\n%s", lead)
	}
	if !strings.Contains(lead, "唯一可相加形") {
		t.Fatalf("the legend must teach the ONLY-additive-form boundary (W-A untouched elsewhere):\n%s", lead)
	}
}

// 件e: the M-IO completion-closure credential renders as a demoted context
// word on the io_latency row (typed per-IO field; soft note, no legend mark).
func TestRSPACompletionClosureWordRenders(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rspaSameSourceSplitProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "完成闭合凭证") {
		t.Fatalf("the io_latency closure row must wear the 完成闭合凭证 context word:\n%s", fence)
	}
	modelEN := buildRuntimeTraceProjTreeModel(rspaSameSourceSplitProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !strings.Contains(fenceEN, "completion-closure credential") {
		t.Fatalf("en mirror of the closure word missing:\n%s", fenceEN)
	}
}

// --- RNB-1 (§29.88 R2/R4, 2026-07-14) display pins ---------------------------

// rspaRNBDivergentDemotedProjection — the case-A' ownership-divergent ◇
// remainder (donghu keva-1 witness numbers: census 2.283 = 2.266 + 0.017,
// chain lane's own Σ 2.181) beside its untouched chain seat, plus an R4
// whole-seat lane-demoted affinity satellite (§29.88.5 witness shape:
// logd.writer dual seat). Also a representative shape for the NEW-7
// bidirectional legend sweep.
func rspaRNBDivergentDemotedProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"keva-1-17437", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			// The chain seat keeps its OWN account (链席自账不动).
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "rnb-chain-seat",
			Subject: "keva-1-17437", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 2.181, CumulativeImpactMS: 2.181, EffectiveImpactMS: 2.181,
			Rank: 1, Tier: "primary", Confidence: 0.9, LineStart: 10, LineEnd: 20,
		}},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			// Case A' ◇ remainder: divergence marker + double-Σ pair.
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rnb-divergent-remainder",
			Subject: "keva-1-17437", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "adjacent",
			ImpactMS: 0.017, CumulativeImpactMS: 0.017,
			ChainAnchoredMS: 2.266, ChainAnchorFullMS: 2.283, ChainAnchorRemainderSeat: true,
			ChainAnchorOwnershipDivergent: true, ChainAnchorChainLaneMS: 2.181, ChainAnchorCensusMS: 2.266,
			Rank: 1, Confidence: 0.8, LineStart: 30, LineEnd: 40,
		}, {
			// R4 whole-seat lane demotion (values untouched).
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rnb-demoted-affinity",
			Subject: "logd.writer-9163", Object: "cpu_affinity_or_cpuset", TypeToken: "cpu_affinity_or_cpuset",
			StateKind: "runnable", ChainRelevance: "adjacent",
			ImpactMS: 47.678, CumulativeImpactMS: 47.678,
			ChainCredentialLaneDemoted: true,
			Rank:                       2, Confidence: 0.72, LineStart: 50, LineEnd: 60,
		}},
	}
}

// RNB-件a: the case-A' remainder speaks the downgraded double-account 行2
// sentence (never the additive 同源二分 form) with both Σs and the delta.
func TestRSPARNBDivergentRemainderRendersDoubleAccountSentence(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rspaRNBDivergentDemotedProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !rspaFenceContains(fence, "账目关系(锚定权属失合):全窗2.283ms=锚定2.266ms+本行其余0.017ms(无链上凭证);链席自账Σ2.181ms 与锚定账Σ2.266ms 失合(差0.085ms),链席另列自账,两席不可相加") {
		t.Fatalf("the divergent remainder must speak the double-account sentence:\n%s", fence)
	}
	if strings.Contains(fence, "同源二分:全窗2.283ms") {
		t.Fatalf("the additive 同源二分 head is forbidden on a divergent remainder:\n%s", fence)
	}
	if strings.Contains(fence, "合计还原全窗账 2.283ms") {
		t.Fatalf("the additive relation sentence is forbidden on a divergent pair:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkChainAnchorDivergent) {
		t.Fatalf("the divergence legend mark must record at the emission site")
	}
	modelEN := buildRuntimeTraceProjTreeModel(rspaRNBDivergentDemotedProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !rspaFenceContains(fenceEN, "account relation (anchored-ownership divergence): whole-window 2.283ms=2.266ms anchored + this remainder 0.017ms (no chain credential); the chain seat's own Σ 2.181ms diverges from the anchored-ledger Σ 2.266ms (delta 0.085ms)") {
		t.Fatalf("en mirror of the double-account sentence missing:\n%s", fenceEN)
	}
}

// RNB-件b: the R4 lane-demoted seat renders the whole-seat demotion
// disclosure with its value untouched on 行1.
func TestRSPARNBDemotedSeatRendersDisclosure(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(rspaRNBDivergentDemotedProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "无链上凭证(整席降道,见图例)") {
		// DISPLAY-WRAP 件③(b): the sentence body lives in the legend entry;
		// the row keeps the legend-keyed chip word.
		t.Fatalf("the demoted seat must carry the whole-seat demotion chip word:\n%s", fence)
	}
	if !strings.Contains(fence, "47.678ms") {
		t.Fatalf("the demoted seat's value must stay untouched on the board:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkChainCredentialDemoted) {
		t.Fatalf("the demotion legend mark must record at the emission site")
	}
	modelEN := buildRuntimeTraceProjTreeModel(rspaRNBDivergentDemotedProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !rspaFenceContains(fenceEN, "no chain credential (whole-seat demotion; see legend)") {
		t.Fatalf("en mirror of the demotion disclosure missing:\n%s", fenceEN)
	}
}

// RNB-件c: the smr1 pair class (0) never claims the additive relation on a
// divergent remainder — the pairing arm skips it (its dedicated 行2 sentence
// owns the relation disclosure).
func TestRSPARNBDivergentRemainderSkipsAdditivePairing(t *testing.T) {
	remainder := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "keva-1-17437", TypeToken: "runnable_wait", StateKind: "runnable",
			ChainRelevance: "adjacent", ImpactMS: 0.017, CumulativeImpactMS: 0.017,
			ChainAnchoredMS: 2.266, ChainAnchorFullMS: 2.283, ChainAnchorRemainderSeat: true,
			ChainAnchorOwnershipDivergent: true, ChainAnchorChainLaneMS: 2.181, ChainAnchorCensusMS: 2.266,
		},
		Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, EvidenceTag: "E9",
	}
	chainSeat := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "keva-1-17437", TypeToken: "runnable_wait", StateKind: "runnable",
			ChainRelevance: "on_chain", ImpactMS: 2.181, CumulativeImpactMS: 2.181, Rank: 1,
		},
		Kind: runtimeTraceProjTreeRowChain, HasData: true, EvidenceTag: "E2",
	}
	model := runtimeTraceProjTreeModel{
		TreeRows: []runtimeTraceProjTreeRow{chainSeat},
		Adjacent: []runtimeTraceProjTreeRow{remainder},
	}
	runtimeTraceProjMarkAccountRelations(&model, true)
	if model.Adjacent[0].AccountRelRef != "" || model.TreeRows[0].AccountRelRef != "" {
		t.Fatalf("the divergent pair must never enter the additive pairing arm: rem=%q chain=%q",
			model.Adjacent[0].AccountRelRef, model.TreeRows[0].AccountRelRef)
	}
}
