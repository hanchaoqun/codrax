package tool

// answer_document_projection_partsplit_test.go — PARTSPLIT-1 (§29.150④ user
// ruling, 2026-07-19) display pins: the R4-mirror-refused gated composite
// seat's pre-edge-share disclosure faces — 行2 分账 sub-line + ◎ non-seat
// mention block (fixture home for the representative-shapes bidirectional
// harness entry partsplit_refused_disclosure).

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// partsplitSquash — probe containment on the ALL-spaces-stripped line join:
// the token-aware width wrap may break a long CJK row at ANY point (inserting
// a break space the single-space squash cannot distinguish from prose), so
// both sides of every Contains drop spaces entirely (zh has no meaningful
// spaces; EN wants pass through the same transform on both sides).
func partsplitSquash(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(strings.TrimLeft(line, "│ \t"))
	}
	return strings.ReplaceAll(b.String(), " ", "")
}

// partsplitDisclosureProjection — representative shape: one on-chain seat
// (section max 2.000), one refused background inversion seat wearing the
// atomic quartet (3.5 pre + 1.5 post == 5.0 runnable account), and the
// matching NON-SEAT side-channel record (cap-dead honest bit exercised: the
// disclosure pre-share 3.5 EXCEEDS the section max 2.0, so any section-head
// absorption would be visible).
func partsplitDisclosureProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:              []string{"worker-9", "app-100"},
		WindowStartTs:           6.000,
		WindowEndTs:             6.010,
		RootCauseFamilyObserved: true,
		OnChainCauses: []types.TraceCausalProjectionNode{
			// A d_state chain seat (the elimv2 fixture family) so the tree
			// face lights the ⛓ state-icon mark the ◎ channel words emit.
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "d_state_or_io_wait", StateKind: "d_sleep", ChainRelevance: "on_chain",
				ImpactMS: 2.0, EffectiveImpactMS: 2.0, Rank: 1, Confidence: 0.8},
		},
		BackgroundCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "invseat-77",
				Object: "priority_inversion_runnable_wait", StateKind: "runnable",
				ChainRelevance: "background", ImpactMS: 5.0, EffectiveImpactMS: 0.8,
				RunnableMS: 5.0, Confidence: 0.76,
				GatedCompositeEdgePreShareMS:  3.5,
				GatedCompositeEdgePostShareMS: 1.5,
				GatedCompositeEdgeAnchorTS:    6.006,
				GatedCompositeEdgeAnchorVia:   "direct"},
		},
		GatedCompositeEdgeShareDisclosures: []types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure{
			{Subject: "invseat-77", PreMS: 3.5, PostMS: 1.5, AccountMS: 5.0,
				AnchorTS: 6.006, Via: "direct", SeatPublished: false},
		},
	}
}

// TestPartsplitSeatSubLineAndElimMentionZH — 面1+面2 zh: the seat-row 行2 分账
// sub-line (values + credential + identity + non-addition rider) and the ◎
// non-seat mention block (head + rider + row + off-board clause).
func TestPartsplitSeatSubLineAndElimMentionZH(t *testing.T) {
	proj := partsplitDisclosureProjection()
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := partsplitSquash(runtimeTraceProjTreeFence(model, true))
	// OMGCLEAN-1 件10 (§29.175.12): the 行2 face keeps the full split account
	// with the white-word head (rule numbers retired: R4拒转·整席不拆 →
	// 按口径不拆段入榜; R3 边凭证 → 唤醒边).
	for _, want := range []string{
		"边前份披露(按口径不拆段入榜):其中边前份 3.500ms(唤醒边前,凭证:唤醒边,最晚凭证边 6.006000s·直接裸边)",
		"边后份 1.500ms(边界后,不入链上)",
		"边前+边后=本席 runnable 账 5.000ms 恒等",
		"边前份与本席已发布值同段,不相加",
	} {
		if !strings.Contains(fence, partsplitSquash(want)) {
			t.Fatalf("zh 行2 sub-line must carry %q:\n%s", want, fence)
		}
	}
	// OMGCLEAN-1 件6+件9 (§29.175.10/.14). EVOLUTION RECORD — 涉既裁位移②:
	// the §29.150④/§29.156 five-line ◎ mention block compresses into the ONE
	// 未入榜最大 auxiliary row (subject + pre-edge value + credential words);
	// the full identity stays on the 行2 分账句 (§29.156 双面) and the typed
	// observation record; the internal 候选池/席值/车道/序数 sentence retires
	// (§29.175.12 零内部术语).
	elim := partsplitSquash(runtimeTraceProjElimOverviewFence(proj, model, true))
	for _, want := range []string{
		"· 未入榜最大 invseat-77 3.500ms · 有唤醒凭证,按口径不拆段入榜",
	} {
		if !strings.Contains(elim, partsplitSquash(want)) {
			t.Fatalf("zh ◎ 未入榜最大 row must carry %q:\n%s", want, elim)
		}
	}
	for _, banned := range []string{"R4", "R3", "候选池", "车道"} {
		if strings.Contains(elim, banned) {
			t.Fatalf("§29.175.12: internal word %q must stay off the ◎ face:\n%s", banned, elim)
		}
	}
	// 节头「最大可消」不吸披露行: the section head keeps its own member max
	// (2.000) even though the disclosure pre-share (3.500) is larger, and no
	// head ever speaks the disclosure value.
	if !strings.Contains(elim, partsplitSquash("最大可消 2.000ms")) ||
		strings.Contains(elim, partsplitSquash("最大可消 3.500ms")) {
		t.Fatalf("the ◎ section head must never absorb the non-seat disclosure:\n%s", elim)
	}
	if !model.Marks.has(runtimeTraceProjMarkGatedCompositeEdgeShare) {
		t.Fatalf("the disclosure mark must record at the emission sites")
	}
}

// TestPartsplitSeatSubLineAndElimMentionEN — EN parity (same values, EN word
// faces, closed-set via token verbatim).
func TestPartsplitSeatSubLineAndElimMentionEN(t *testing.T) {
	proj := partsplitDisclosureProjection()
	model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fence := partsplitSquash(runtimeTraceProjTreeFence(model, false))
	for _, want := range []string{
		"pre-edge share disclosure (kept whole per its caliber; not split into a board row): 3.500ms pre-edge (before the wakeup edge; credential: the wakeup edge, latest credential edge 6.006000s, via=direct)",
		"1.500ms post-edge (after the boundary — never on-chain)",
		"pre + post == this seat's runnable account 5.000ms",
		"never additive",
	} {
		if !strings.Contains(fence, partsplitSquash(want)) {
			t.Fatalf("EN 行2 sub-line must carry %q:\n%s", want, fence)
		}
	}
	elim := partsplitSquash(runtimeTraceProjElimOverviewFence(proj, model, false))
	// OMGCLEAN-1 件6+件9: the EN ◎ face carries the one unranked-max aux row.
	for _, want := range []string{
		"· unranked max invseat-77 3.500ms · wakeup-edge credential; kept whole per its caliber, not split into a board row",
	} {
		if !strings.Contains(elim, partsplitSquash(want)) {
			t.Fatalf("EN ◎ unranked-max row must carry %q:\n%s", want, elim)
		}
	}
	if !strings.Contains(elim, partsplitSquash("max eliminable 2.000ms")) ||
		strings.Contains(elim, partsplitSquash("max eliminable 3.500ms")) {
		t.Fatalf("the EN ◎ section head must never absorb the non-seat disclosure:\n%s", elim)
	}
}

// TestPartsplitAdmissionFailClosed — the typed admission gates: a broken X+Y
// identity, a missing quartet member, and a zero runnable base all render
// NOTHING on either face (absence silent, 宁漏勿假指).
func TestPartsplitAdmissionFailClosed(t *testing.T) {
	mutate := map[string]func(*types.TraceCausalProjection){
		"identity broken (X+Y != runnable account)": func(p *types.TraceCausalProjection) {
			p.BackgroundCauses[0].GatedCompositeEdgePreShareMS = 4.5
			p.GatedCompositeEdgeShareDisclosures[0].PreMS = 4.5
		},
		"missing anchor ts (partial quartet)": func(p *types.TraceCausalProjection) {
			p.BackgroundCauses[0].GatedCompositeEdgeAnchorTS = 0
			p.GatedCompositeEdgeShareDisclosures[0].AnchorTS = 0
		},
		"unknown via token (closed set)": func(p *types.TraceCausalProjection) {
			p.BackgroundCauses[0].GatedCompositeEdgeAnchorVia = "guessed"
			p.GatedCompositeEdgeShareDisclosures[0].Via = "guessed"
		},
		"zero runnable base on the node": func(p *types.TraceCausalProjection) {
			p.BackgroundCauses[0].RunnableMS = 0
			p.GatedCompositeEdgeShareDisclosures[0].AccountMS = 0
		},
	}
	for name, mut := range mutate {
		proj := partsplitDisclosureProjection()
		mut(&proj)
		model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		fence := runtimeTraceProjTreeFence(model, true)
		elim := runtimeTraceProjElimOverviewFence(proj, model, true)
		if strings.Contains(fence, "边前份披露") || strings.Contains(elim, "边前份披露") {
			t.Fatalf("%s: the disclosure must stay silent:\nfence:\n%s\nelim:\n%s", name, fence, elim)
		}
		if model.Marks.has(runtimeTraceProjMarkGatedCompositeEdgeShare) {
			t.Fatalf("%s: the mark must not record on a refused admission", name)
		}
	}
}

// TestPartsplitMentionRefResolutionAllOrNothing — the [E#] pointer mints only
// on a typed anchor + canonical-subject match with a non-empty tag.
func TestPartsplitMentionRefResolutionAllOrNothing(t *testing.T) {
	node := partsplitDisclosureProjection().BackgroundCauses[0]
	d := partsplitDisclosureProjection().GatedCompositeEdgeShareDisclosures[0]
	model := runtimeTraceProjTreeModel{Marks: &runtimeTraceProjMarkSet{},
		Background: []runtimeTraceProjTreeRow{{Node: node, EvidenceTag: "E7"}}}
	if ref := runtimeTraceProjGatedCompositeEdgeShareResolveRef(model, d); ref != "E7" {
		t.Fatalf("matching row must resolve to its tag: %q", ref)
	}
	// Anchor mismatch → no pointer (a different boundary is a different fact).
	other := d
	other.AnchorTS = 6.007
	if ref := runtimeTraceProjGatedCompositeEdgeShareResolveRef(model, other); ref != "" {
		t.Fatalf("anchor mismatch must resolve nothing: %q", ref)
	}
	// Tag-less row → no pointer (all-or-nothing, never a fabricated [E#]).
	model.Background[0].EvidenceTag = ""
	if ref := runtimeTraceProjGatedCompositeEdgeShareResolveRef(model, d); ref != "" {
		t.Fatalf("tag-less row must resolve nothing: %q", ref)
	}
}

// TestPartsplitTiebaFlagWitness — live witness #1 (§29.150④ target, tieba
// flag board): the 23088 refusal renders as the ◎ non-seat mention with the
// production numbers (13.959 + 0.020 == 13.979) and the off-board clause;
// the refused seat itself renders NO tree row (cap-dead, 账在池), so the
// non-seat mention is the disclosure's only face.
func TestPartsplitTiebaFlagWitness(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	const trace = "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{PID: 59566, TimeStart: 34579.472865, TimeEnd: 34579.587805,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	at := time.Unix(1751600000, 0).UTC()
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := query
		q.View = view
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "fixture", "p-"+view, "r", "", at)...)
	}
	projection := types.TraceCausalProjectionFromObservationRecords(obs)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := partsplitSquash(runtimeTraceProjTreeFence(model, true))
	elim := partsplitSquash(runtimeTraceProjElimOverviewFence(projection, model, true))
	// OMGCLEAN-1 件6+件9 (§29.175.10/.14): the witness account rides the ONE
	// 未入榜最大 aux row — subject + pre-edge value + credential white words;
	// the full pre/post identity lives on the 分账 detail face.
	for _, want := range []string{
		"· 未入榜最大 Binder:43397_19-23088 13.959ms · 有唤醒凭证,按口径不拆段入榜",
	} {
		if !strings.Contains(elim, partsplitSquash(want)) {
			t.Fatalf("tieba flag ◎ 未入榜最大 row must carry %q:\n%s", want, elim)
		}
	}
	if strings.Contains(fence, "边前份披露") {
		t.Fatalf("tieba flag: the cap-dead refused seat renders no tree row, so the tree face must stay mention-free:\n%s", fence)
	}
}

// TestPartsplitDonghu2955Witness — live witness #2 (donghu 2955 board): the
// OS_FFRT_2_0-2614 refusal renders with 9.618 + 9.945 == 19.563.
func TestPartsplitDonghu2955Witness(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{PID: 2955, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	at := time.Unix(1751600000, 0).UTC()
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := query
		q.View = view
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "fixture", "p-"+view, "r", "", at)...)
	}
	projection := types.TraceCausalProjectionFromObservationRecords(obs)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	elim := partsplitSquash(runtimeTraceProjElimOverviewFence(projection, model, true))
	// OMGCLEAN-1 件6+件9: the aux row carries the pre-edge share (9.618); the
	// full 9.618 + 9.945 == 19.563 identity stays on the 分账 detail face.
	if !strings.Contains(elim, partsplitSquash("· 未入榜最大 OS_FFRT_2_0-2614 9.618ms · 有唤醒凭证,按口径不拆段入榜")) {
		t.Fatalf("donghu 2955 ◎ 未入榜最大 row must carry the 2614 witness value:\n%s", elim)
	}
}
