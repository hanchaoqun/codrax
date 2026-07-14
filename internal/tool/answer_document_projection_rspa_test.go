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
	if !strings.Contains(fence, "同源二分:全窗31.191ms=锚定1.759ms(⛓链上席)+本行其余29.432ms(无链上凭证)") {
		t.Fatalf("the ◇ runnable remainder must disclose the bipartition on 行2:\n%s", fence)
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
	if !strings.Contains(fenceEN, "same-source split: full-window 31.191ms=1.759ms anchored (⛓ chain seat) + this remainder 29.432ms (no chain credential)") {
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
	if !strings.Contains(fenceEN, "same-source split: full-window 36.757ms=this row 3.598ms anchored + remainder 33.159ms (◇ remainder seat)") {
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
	if !strings.Contains(fence, "⛓席=["+rem.AccountRelRef+"]凭证锚定段合计;◇席(本行)=同线程同状态窗内其余段(无链上凭证);两席同源不相交,合计还原全窗账 36.757ms") {
		t.Fatalf("the ◇ half must speak the ruling's relation sentence with the counterpart's E#:\n%s", fence)
	}
	if !strings.Contains(fence, "◇席=["+clip.AccountRelRef+"]同线程同状态窗内其余段(无链上凭证);⛓席(本行)=凭证锚定段合计;两席同源不相交,合计还原全窗账 36.757ms") {
		t.Fatalf("the ⛓ half must speak its mirrored relation sentence:\n%s", fence)
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
