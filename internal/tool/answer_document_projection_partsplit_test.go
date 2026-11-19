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
		// RE-BASED (HMC377 E2, owner ruling 2026-11-18): reader wording of the wakeup credential.
		"等待内份披露(按口径不拆段入榜):其中等待内份 3.500ms(落在目标线程等待之内,该等待由本线程唤醒结束,最后一次 6.006000s·直接唤醒)",
		"等待外份 1.500ms(目标并未在等它,不入链上)",
		"等待内+等待外=本席 runnable 账 5.000ms 恒等",
		"等待内份与本席已发布值同段,不相加",
	} {
		if !strings.Contains(fence, partsplitSquash(want)) {
			t.Fatalf("zh 行2 sub-line must carry %q:\n%s", want, fence)
		}
	}
	// OMGCLEAN-1 件6+件9 (§29.175.10/.14). EVOLUTION RECORD — 涉既裁位移②:
	// the §29.150④/§29.156 five-line ◎ mention block compresses into the ONE
	// 未入榜最大 auxiliary row (subject + seat account + inline identity 括注);
	// the full identity stays on the 行2 分账句 (§29.156 双面) and the typed
	// observation record; the internal 候选池/席值/车道/序数 sentence retires
	// (§29.175.12 零内部术语).
	elim := partsplitSquash(runtimeTraceProjElimOverviewFence(proj, model, true))
	// 双复核修复 件2 (冷读 CR2, 2026-07-21): row value = the seat's own account
	// (AccountMS) with the inline pre/post identity 括注 right after it.
	for _, want := range []string{
		"· 未入榜最大 invseat-77 5.000ms(等待内 3.500 + 等待外 1.500)· 按口径不拆段入榜",
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
	if !strings.Contains(elim, partsplitSquash("最大估算潜力 2.000ms")) ||
		strings.Contains(elim, partsplitSquash("最大估算潜力 3.500ms")) {
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
		// RE-BASED (HMC377 E2, owner ruling 2026-11-18): reader wording of the wakeup credential.
		"inside-wait share disclosure (kept whole per its caliber; not split into a board row): 3.500ms inside the analysed thread's waits (ended by this thread's wakeup, last at 6.006000s, via=direct)",
		"1.500ms outside them (the analysed thread was not waiting for it — never on-chain)",
		"inside + outside == this seat's runnable account 5.000ms",
		"never additive",
	} {
		if !strings.Contains(fence, partsplitSquash(want)) {
			t.Fatalf("EN 行2 sub-line must carry %q:\n%s", want, fence)
		}
	}
	elim := partsplitSquash(runtimeTraceProjElimOverviewFence(proj, model, false))
	// OMGCLEAN-1 件6+件9 + 双复核 件2: the EN ◎ face carries the one
	// unranked-max aux row (AccountMS + inline identity, compressed clause —
	// 件3 EN 禁续行).
	for _, want := range []string{
		"· unranked max invseat-77 5.000ms (in 3.500 + out 1.500) · kept whole per caliber",
	} {
		if !strings.Contains(elim, partsplitSquash(want)) {
			t.Fatalf("EN ◎ unranked-max row must carry %q:\n%s", want, elim)
		}
	}
	if !strings.Contains(elim, partsplitSquash("max potential 2.000ms")) ||
		strings.Contains(elim, partsplitSquash("max potential 3.500ms")) {
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
		if strings.Contains(fence, "等待内份披露") || strings.Contains(elim, "等待内份披露") {
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
	// RE-BASED (HMC377 E2, owner ruling 2026-11-18 "同意继续"): old → new =
	// "· 未入榜最大 Binder:43397_19-23088 13.979ms(唤醒边前 13.959 + 边后
	// 0.020)" → no such row. 23088's one wakeup ended a 0.524ms wait of 59566
	// that holds none of its ready time (raw oracle: inside 0.000), so it has
	// no inside-wait share to disclose and the overview names nothing.
	if strings.Contains(elim, partsplitSquash("Binder:43397_19-23088")) || strings.Contains(elim, partsplitSquash("按口径不拆段入榜")) {
		t.Fatalf("tieba flag ◎ overview must not disclose a share 23088 never had:\n%s", elim)
	}
	if strings.Contains(fence, "等待内份披露") {
		t.Fatalf("tieba flag: the cap-dead refused seat renders no tree row, so the tree face must stay mention-free:\n%s", fence)
	}
}

// TestPartsplitDonghu2955Witness — live witness #2 (donghu 2955 board): the
// OS_FFRT_2_0-2614 refusal (since HMC377 E2: 0.018 + 19.545 == 19.563, below
// the floor) renders no overview row.
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
	// RE-BASED (HMC377 E2, owner ruling 2026-11-18 "同意继续"): old → new =
	// "· 未入榜最大 OS_FFRT_2_0-2614 19.563ms(唤醒边前 9.618 + 边后 9.945)" →
	// no row: only 0.018ms of 2614's ready time lies inside the 1.124ms wait
	// its wakeup ended (raw oracle), below the 2.332ms disclosure floor; the
	// typed record (0.018 + 19.545) stays on the pool seat
	// (TestPartsplitDonghu2955WitnessDisclosure).
	if strings.Contains(elim, partsplitSquash("OS_FFRT_2_0-2614")) && strings.Contains(elim, partsplitSquash("按口径不拆段入榜")) {
		t.Fatalf("donghu 2955 ◎ overview must not disclose 2614's below-floor share:\n%s", elim)
	}
}
