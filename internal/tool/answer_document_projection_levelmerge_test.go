package tool

// answer_document_projection_levelmerge_test.go — LEVELMERGE-1 件2/件3 display
// pins (方案 P 区间分账 + 聚合席↔成员两向互指, user rulings 2026-07-18).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// levelmergeGatedShareProjection is the representative shape: the residual
// (B) aggregate seat + its demoted A constituent clone + the claiming
// inversion seat (claim-seat line interval 300..305 = the inversion row's own
// lines), two member occurrence VIEW rows of the same (subject, runnable)
// family, and a second thread wearing the fail-open 裁定④ overlap disclosure
// (its claim seat deliberately unresolvable → generic noun form).
func levelmergeGatedShareProjection() types.TraceCausalProjection {
	inv := elimChainNode("lm-inv", "dep_worker-200", "priority_inversion_candidate", "running", 2, 8.0, 300)
	inv.ChainDepth = 2
	inv.GatedRunnableMS = 6
	inv.GatedRunningDeficitMS = 2

	b := elimChainNode("lm-b", "dep_worker-200", "runnable_wait", "runnable", 3, 5.0, 100)
	b.GatedShareClaimedMS = 10
	b.GatedShareFullMS = 15
	b.GatedShareClaimSeats = []string{"300..305"}

	a := elimChainNode("lm-a", "dep_worker-200", "runnable_wait", "runnable", 0, 10.0, 100)
	a.Rank = 0
	a.ChainRelevance = "adjacent"
	a.Causality = "adjacent_to_wakeup_chain"
	a.GatedShareClaimedMS = 10
	a.GatedShareFullMS = 15
	a.GatedShareConstituentSeat = true
	a.GatedShareClaimSeats = []string{"300..305"}

	// Two member occurrence VIEW rows of the (dep_worker-200, runnable)
	// family — the ORD-A membership predicate shape (wakeup_causal_impact,
	// depth>0). Values differ from every seat value (no R1 same-fact merge).
	m1 := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "lm-m1",
		Subject: "dep_worker-200", Predicate: "wakeup_causal_impact",
		Object: "runnable_wait", TypeToken: "runnable_wait", StateKind: "runnable",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		ImpactMS: 8.2, CumulativeImpactMS: 8.2, EffectiveImpactMS: 8.2,
		LineStart: 110, LineEnd: 120, Confidence: 0.8,
	}
	m2 := m1
	m2.EvidenceID = "lm-m2"
	m2.ChainDepth = 2
	m2.ImpactMS, m2.CumulativeImpactMS, m2.EffectiveImpactMS = 6.8, 6.8, 6.8
	m2.LineStart, m2.LineEnd = 150, 160

	partial := elimChainNode("lm-partial", "dep_partial-201", "runnable_wait", "runnable", 4, 8.0, 400)
	partial.GatedShareOverlapDisclosureMS = 2.5
	partial.GatedShareClaimSeats = []string{"900..910"} // unresolvable → generic noun

	// A D-state chain row keeps the ⛓ glyph's icon legend lit beside the ◎
	// channel word (the representative-fixture convention — xlane2 同款).
	dio := elimChainNode("lm-dio", "worker-300", "d_state_or_io_wait", "d_sleep", 6, 3.0, 600)

	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"app_target-100"},
		WindowStartTs:           10.000,
		WindowEndTs:             10.100,
		OnChainCauses:           []types.TraceCausalProjectionNode{inv, b, m1, m2, partial, dio},
		AdjacentCauses:          []types.TraceCausalProjectionNode{a},
	}
}

func levelmergeFindRow(model *runtimeTraceProjTreeModel, id string) *runtimeTraceProjTreeRow {
	return xlane2FindRowByEvidenceID(model, id)
}

// 件2 end to end: the split pair renders its two 行2 faces with the resolved
// inversion-seat [E#], the identity words carry the typed values, the
// fail-open row wears the 裁定④ clause with the generic noun (unresolvable
// claim span never guesses an E#), and every published value stays.
func TestLevelMergeGatedShareSplitDisplayFaces(t *testing.T) {
	for _, zh := range []bool{true, false} {
		t.Run(map[bool]string{true: "zh", false: "en"}[zh], func(t *testing.T) {
			model := buildRuntimeTraceProjTreeModel(levelmergeGatedShareProjection(),
				newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := runtimeTraceProjTreeFence(model, zh)
			inv := levelmergeFindRow(&model, "lm-inv")
			b := levelmergeFindRow(&model, "lm-b")
			a := levelmergeFindRow(&model, "lm-a")
			partial := levelmergeFindRow(&model, "lm-partial")
			if inv == nil || b == nil || a == nil || partial == nil {
				t.Fatalf("fixture drifted: all four rows must render")
			}
			invTag := strings.TrimSpace(inv.EvidenceTag)
			if invTag == "" {
				t.Fatalf("fixture drifted: the inversion seat must carry an evidence tag")
			}
			// The stamp resolved the claim-seat line interval to the [E#].
			if len(b.GatedShareClaimRefs) != 1 || b.GatedShareClaimRefs[0] != invTag ||
				len(a.GatedShareClaimRefs) != 1 || a.GatedShareClaimRefs[0] != invTag {
				t.Fatalf("claim refs must resolve to the inversion seat %q: b=%v a=%v", invTag, b.GatedShareClaimRefs, a.GatedShareClaimRefs)
			}
			residualWord := "分账残余席:全账 15.000ms = 已计入反转席[" + invTag + "]份 10.000ms + 本席残余 5.000ms"
			constituentWord := "分账构成份·归因已计入反转席[" + invTag + "]"
			overlapWord := "其中 2.500ms 与本线程反转席重叠(按现有真段区间测得"
			if !zh {
				residualWord = "split-account residual seat: full account 15.000ms = 10.000ms counted at the priority-inversion seat [" + invTag + "] + this seat's residual 5.000ms"
				constituentWord = "split-account constituent share · attribution already counted at the priority-inversion seat [" + invTag + "]"
				overlapWord = "2.500ms of this account overlaps this thread's priority-inversion seat (measured over the available real segments"
			}
			for _, want := range []string{residualWord, constituentWord, overlapWord} {
				if !rspaFenceContains(fence, want) {
					t.Fatalf("missing word face %q:\n%s", want, fence)
				}
			}
			if !model.Marks.has(runtimeTraceProjMarkGatedShareSplit) || !model.Marks.has(runtimeTraceProjMarkGatedShareOverlap) {
				t.Fatalf("the split/overlap legend marks must record at their emission sites")
			}
			// 值通道:已批面 = the published values ARE the split values; the
			// fail-open row's published value stays the pre-split full.
			for _, value := range []string{"5.000", "10.000", "8.000"} {
				if !rspaFenceContains(fence, value) {
					t.Fatalf("published value %s missing from the fence:\n%s", value, fence)
				}
			}
		})
	}
}

// 件3 end to end: the aggregate seat lists its constituent member rows and
// each member points back — both directions all-or-nothing; the demoted A
// clone never takes the seat role. (The occurrence census counts occurrences
// through MergedCount when the display fold merges member rows; this fixture
// keeps them un-folded — the folded shape is covered by the pass's
// occurrence-count arm.)
func TestLevelMergeAggregateMemberCrossRefTwoWay(t *testing.T) {
	for _, zh := range []bool{true, false} {
		t.Run(map[bool]string{true: "zh", false: "en"}[zh], func(t *testing.T) {
			model := buildRuntimeTraceProjTreeModel(levelmergeGatedShareProjection(),
				newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := runtimeTraceProjTreeFence(model, zh)
			b := levelmergeFindRow(&model, "lm-b")
			a := levelmergeFindRow(&model, "lm-a")
			m1 := levelmergeFindRow(&model, "lm-m1")
			m2 := levelmergeFindRow(&model, "lm-m2")
			if b == nil || a == nil || m1 == nil || m2 == nil {
				t.Fatalf("fixture drifted: seat and both member rows must render")
			}
			seatTag := strings.TrimSpace(b.EvidenceTag)
			if len(b.AggregateMemberRefs) != 2 {
				t.Fatalf("the seat must list both constituent member rows, got %v", b.AggregateMemberRefs)
			}
			if m1.AggregateSeatRef != seatTag || m2.AggregateSeatRef != seatTag {
				t.Fatalf("both members must point back at the seat %q: %q / %q", seatTag, m1.AggregateSeatRef, m2.AggregateSeatRef)
			}
			if a.AggregateMemberRefs != nil || a.AggregateSeatRef != "" {
				t.Fatalf("the demoted constituent clone must not take the seat role or a member pointer")
			}
			memberWord := "归因已计入[" + seatTag + "](聚合席),本行为构成段,不另计"
			seatWord := "构成段见["
			if !zh {
				memberWord = "attribution already counted at [" + seatTag + "] (the aggregate seat); this row is a constituent segment, not counted again"
				seatWord = "constituent segments at ["
			}
			squash := func(s string) string { return strings.ReplaceAll(s, " ", "") }
			if n := strings.Count(squash(rspaFenceJoined(fence)), squash(memberWord)); n != 2 {
				t.Fatalf("exactly the two member rows must wear the constituent word (got %d):\n%s", n, fence)
			}
			if !rspaFenceContains(fence, seatWord) {
				t.Fatalf("the seat must wear the member listing %q:\n%s", seatWord, fence)
			}
			if !model.Marks.has(runtimeTraceProjMarkAggregateMemberCrossRef) {
				t.Fatalf("the cross-ref legend mark must record at the emission site")
			}
		})
	}
}

// Negative pins: a single member never mints the pair (the R1 same-fact merge
// owns that shape), and an ambiguous seat population (two rank rows of one
// (subject, family)) skips both directions (宁漏勿假指).
func TestLevelMergeAggregateMemberCrossRefNegatives(t *testing.T) {
	base := levelmergeGatedShareProjection()
	single := base
	single.OnChainCauses = nil
	for _, node := range base.OnChainCauses {
		if node.EvidenceID == "lm-m2" {
			continue
		}
		single.OnChainCauses = append(single.OnChainCauses, node)
	}
	model := buildRuntimeTraceProjTreeModel(single, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if b := levelmergeFindRow(&model, "lm-b"); b == nil || b.AggregateMemberRefs != nil {
		t.Fatalf("a single member must not mint the seat listing")
	}
	if m1 := levelmergeFindRow(&model, "lm-m1"); m1 == nil || m1.AggregateSeatRef != "" {
		t.Fatalf("a single member must not point at a seat")
	}

	ambiguous := levelmergeGatedShareProjection()
	second := elimChainNode("lm-b2", "dep_worker-200", "runnable_wait", "runnable", 5, 4.0, 500)
	// A family-folded second seat (trunk-fold-exempt via its family fields)
	// so the ambiguity reaches the crossref pass instead of being display-
	// merged away.
	second.FamilyMemberCount = 2
	second.FamilyMemberMaxMS = 2.5
	second.FamilyMemberMinMS = 1.5
	second.FamilyFoldCaliber = "sum_disjoint"
	ambiguous.OnChainCauses = append(ambiguous.OnChainCauses, second)
	model2 := buildRuntimeTraceProjTreeModel(ambiguous, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, id := range []string{"lm-b", "lm-b2"} {
		if row := levelmergeFindRow(&model2, id); row != nil && row.AggregateMemberRefs != nil {
			t.Fatalf("an ambiguous seat population must not mint the listing (%s)", id)
		}
	}
	for _, id := range []string{"lm-m1", "lm-m2"} {
		if row := levelmergeFindRow(&model2, id); row != nil && row.AggregateSeatRef != "" {
			t.Fatalf("an ambiguous seat population must not mint member pointers (%s)", id)
		}
	}
}

// ◎ face: the demoted constituent row leaves the eliminable-overview
// population (its claimed share is already inside the inversion seat's gated
// composite — a full ◇ bar beside that seat is the visual double count) and
// the dedicated footnote disclosures it (排除≠消失); the residual B seat and
// the inversion seat keep participating.
func TestLevelMergeGatedConstituentLeavesElimPopulation(t *testing.T) {
	for _, zh := range []bool{true, false} {
		t.Run(map[bool]string{true: "zh", false: "en"}[zh], func(t *testing.T) {
			projection := levelmergeGatedShareProjection()
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := runtimeTraceProjElimOverviewFence(projection, model, zh)
			a := levelmergeFindRow(&model, "lm-a")
			if a == nil {
				t.Fatalf("fixture drifted: the constituent row must render")
			}
			if runtimeTraceProjElimEligible(*a) {
				t.Fatalf("the constituent row must leave the ◎ population")
			}
			if !runtimeTraceProjElimGatedConstituentExcluded(*a) {
				t.Fatalf("the exclusion census must claim exactly the constituent row")
			}
			footnote := "分账构成份(已计入反转席,降道):1 行,见明细 [" + strings.TrimSpace(a.EvidenceTag) + "]"
			if !zh {
				footnote = "split-account constituent share(s) (counted at the inversion seat, demoted): 1 row(s) — see the detail blocks [" + strings.TrimSpace(a.EvidenceTag) + "]"
			}
			if !rspaFenceContains(fence, footnote) {
				t.Fatalf("missing ◎ exclusion footnote %q:\n%s", footnote, fence)
			}
			// The residual seat's value stays on the ◎ face (5.000ms bar).
			if !rspaFenceContains(fence, "5.000ms") {
				t.Fatalf("the residual seat must keep its ◎ participation:\n%s", fence)
			}
		})
	}
}

// --- 修补轮 (双复核后, 2026-07-18): 件2 display/projection 加固 pin 六补 + 件4 ---

// levelmergeTrunkFoldProjection — 修补轮件2①: a genuinely foldable trunk
// pair. The trunk subject dep_worker-200 carries TWO same-(state,type)
// occurrence rows: the gated-share residual B seat and a plain occurrence.
// With the exclusion arm the pair never trunk-folds (the B seat keeps its
// carved account visible); strip the gated account (gated=false) and the SAME
// pair folds ×2 — the load-bearing foldability precondition, so a deleted
// exclusion arm reds the pin through the disease shape (B 席被 ×N 吞).
func levelmergeTrunkFoldProjection(gated bool) types.TraceCausalProjection {
	b := elimChainNode("lmtf-b", "dep_worker-200", "runnable_wait", "runnable", 2, 5.0, 100)
	if gated {
		b.GatedShareClaimedMS = 10
		b.GatedShareFullMS = 15
		b.GatedShareClaimSeats = []string{"300..305"}
	}
	plain := elimChainNode("lmtf-plain", "dep_worker-200", "runnable_wait", "runnable", 3, 2.0, 500)
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"dep_worker-200", "app_target-100"},
		WindowStartTs:           10.000,
		WindowEndTs:             10.100,
		OnChainCauses:           []types.TraceCausalProjectionNode{b, plain},
	}
}

// 件2① pin: the trunk ×N fold must never swallow a gated-share seat.
func TestLevelMergeTrunkFoldExclusionArm(t *testing.T) {
	// Disease-shape guard: the residual seat and its plain twin both render,
	// un-merged.
	model := buildRuntimeTraceProjTreeModel(levelmergeTrunkFoldProjection(true),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	b := levelmergeFindRow(&model, "lmtf-b")
	plain := levelmergeFindRow(&model, "lmtf-plain")
	if b == nil || plain == nil {
		t.Fatalf("the B seat and the plain occurrence must both render (B 席被 ×N 吞 = the disease shape)")
	}
	if b.Node.MergedCount > 1 || plain.Node.MergedCount > 1 {
		t.Fatalf("neither row may wear a ×N fold: b=%d plain=%d", b.Node.MergedCount, plain.Node.MergedCount)
	}
	// Positive control (load-bearing precondition): the SAME pair with the
	// gated account stripped IS foldable and folds ×2 — the exclusion arm is
	// the only thing standing between the B seat and the fold.
	control := buildRuntimeTraceProjTreeModel(levelmergeTrunkFoldProjection(false),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	folded := levelmergeFindRow(&control, "lmtf-b")
	if folded == nil || folded.Node.MergedCount != 2 {
		t.Fatalf("fixture drifted: the ungated pair must trunk-fold ×2 (got %+v)", folded)
	}
	if levelmergeFindRow(&control, "lmtf-plain") != nil {
		t.Fatalf("fixture drifted: the folded twin must be consumed by the ×2 row")
	}
}

// 件2② pin: the 件3 crossref same-board gate — a member on ANOTHER named
// rank board never joins the two-way pointer pair; the whole family renders
// zero pointer bytes (all-or-nothing, XLANE-3 让位红线).
func TestLevelMergeCrossRefSameBoardGate(t *testing.T) {
	projection := levelmergeGatedShareProjection()
	for i := range projection.OnChainCauses {
		node := &projection.OnChainCauses[i]
		switch node.EvidenceID {
		case "lm-m2":
			node.QueryWindowStartTs, node.QueryWindowEndTs = 20.000, 20.500
			node.RankBoardTarget = "other_app-999"
		case "lm-b", "lm-m1":
			node.QueryWindowStartTs, node.QueryWindowEndTs = 10.000, 10.100
		}
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	b := levelmergeFindRow(&model, "lm-b")
	m1 := levelmergeFindRow(&model, "lm-m1")
	m2 := levelmergeFindRow(&model, "lm-m2")
	if b == nil || m1 == nil || m2 == nil {
		t.Fatalf("fixture drifted: seat and both members must render")
	}
	if b.AggregateMemberRefs != nil {
		t.Fatalf("a cross-board member must veto the seat listing (all-or-nothing), got %v", b.AggregateMemberRefs)
	}
	if m1.AggregateSeatRef != "" || m2.AggregateSeatRef != "" {
		t.Fatalf("cross-board families must mint zero member pointers: %q / %q", m1.AggregateSeatRef, m2.AggregateSeatRef)
	}
}

// 件2③ pin: the 件2 claim-ref resolution is all-or-nothing — ONE
// unresolvable or malformed claim span zeroes the whole [E#] list and the
// sentence keeps the generic noun (宁漏勿假指); the untouched sibling row
// still resolves (per-row precision).
func TestLevelMergeClaimRefAllOrNothing(t *testing.T) {
	cases := []struct {
		name string
		span string
	}{
		{"unmatched span zeroes the list", "900..910"},
		{"malformed span zeroes the list", "not_a_span"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projection := levelmergeGatedShareProjection()
			for i := range projection.OnChainCauses {
				if projection.OnChainCauses[i].EvidenceID == "lm-b" {
					projection.OnChainCauses[i].GatedShareClaimSeats = []string{"300..305", tc.span}
				}
			}
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
			b := levelmergeFindRow(&model, "lm-b")
			a := levelmergeFindRow(&model, "lm-a")
			inv := levelmergeFindRow(&model, "lm-inv")
			if b == nil || a == nil || inv == nil {
				t.Fatalf("fixture drifted")
			}
			if b.GatedShareClaimRefs != nil {
				t.Fatalf("a partial span roster must zero the refs, got %v", b.GatedShareClaimRefs)
			}
			invTag := strings.TrimSpace(inv.EvidenceTag)
			if len(a.GatedShareClaimRefs) != 1 || a.GatedShareClaimRefs[0] != invTag {
				t.Fatalf("the untouched sibling must still resolve to %q, got %v", invTag, a.GatedShareClaimRefs)
			}
			// The generic-noun sentence is the honest fallback on the fence.
			fence := runtimeTraceProjTreeFence(model, true)
			if !rspaFenceContains(fence, "已计入本线程反转席份 10.000ms") {
				t.Fatalf("the residual sentence must keep the generic noun:\n%s", fence)
			}
		})
	}
}

// 件2④ pin: the 件3 member listing is all-or-nothing — a member row without
// an evidence tag (no line anchor → no E#) vetoes BOTH directions for the
// whole family; a partial listing must never mint.
func TestLevelMergeMemberRefsAllOrNothing(t *testing.T) {
	projection := levelmergeGatedShareProjection()
	for i := range projection.OnChainCauses {
		if projection.OnChainCauses[i].EvidenceID == "lm-m2" {
			projection.OnChainCauses[i].LineStart = 0
			projection.OnChainCauses[i].LineEnd = 0
		}
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	b := levelmergeFindRow(&model, "lm-b")
	m1 := levelmergeFindRow(&model, "lm-m1")
	m2 := levelmergeFindRow(&model, "lm-m2")
	if b == nil || m1 == nil || m2 == nil {
		t.Fatalf("fixture drifted: all three rows must render")
	}
	if strings.TrimSpace(m2.EvidenceTag) != "" {
		t.Fatalf("fixture drifted: the anchorless member must stay tagless")
	}
	if b.AggregateMemberRefs != nil {
		t.Fatalf("a tagless member must veto the seat listing (all-or-nothing), got %v", b.AggregateMemberRefs)
	}
	if m1.AggregateSeatRef != "" || m2.AggregateSeatRef != "" {
		t.Fatalf("a tagless member must veto every member pointer: %q / %q", m1.AggregateSeatRef, m2.AggregateSeatRef)
	}
}

// 件4 pin: the 件2 claim-ref stamp carries the SAME same-board gate as the
// 件3 crossref (XLANE-3 让位红线) — an inversion seat on ANOTHER named rank
// board never resolves the claim pointer even at an exact line-interval
// match; the sentence keeps the generic noun (跨具名板形→歧义跳).
func TestLevelMergeClaimRefSameBoardGate(t *testing.T) {
	projection := levelmergeGatedShareProjection()
	for i := range projection.OnChainCauses {
		node := &projection.OnChainCauses[i]
		switch node.EvidenceID {
		case "lm-inv":
			node.QueryWindowStartTs, node.QueryWindowEndTs = 20.000, 20.500
			node.RankBoardTarget = "other_app-999"
		case "lm-b":
			node.QueryWindowStartTs, node.QueryWindowEndTs = 10.000, 10.100
		}
	}
	for i := range projection.AdjacentCauses {
		if projection.AdjacentCauses[i].EvidenceID == "lm-a" {
			projection.AdjacentCauses[i].QueryWindowStartTs = 10.000
			projection.AdjacentCauses[i].QueryWindowEndTs = 10.100
		}
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	b := levelmergeFindRow(&model, "lm-b")
	a := levelmergeFindRow(&model, "lm-a")
	if b == nil || a == nil {
		t.Fatalf("fixture drifted: the split pair must render")
	}
	if b.GatedShareClaimRefs != nil || a.GatedShareClaimRefs != nil {
		t.Fatalf("a cross-board inversion seat must never resolve the claim pointer: b=%v a=%v",
			b.GatedShareClaimRefs, a.GatedShareClaimRefs)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !rspaFenceContains(fence, "已计入本线程反转席份 10.000ms") {
		t.Fatalf("the residual sentence must fall back to the generic noun:\n%s", fence)
	}
}

// 件2⑤ pin (display half): the ×N merged Σ constituent form — the per-seat
// ledger clear zeroed the claimed/full decomposition but the typed marker
// survives, the ◎ census still claims the row, and the row-2 face
// self-explains instead of going silent (◎ 脚注不再单面).
func TestLevelMergeMergedConstituentRowStillSelfExplains(t *testing.T) {
	for _, zh := range []bool{true, false} {
		t.Run(map[bool]string{true: "zh", false: "en"}[zh], func(t *testing.T) {
			projection := levelmergeGatedShareProjection()
			for i := range projection.AdjacentCauses {
				if projection.AdjacentCauses[i].EvidenceID != "lm-a" {
					continue
				}
				// The stored ×N Σ artifact form after the aggregate-side
				// per-seat-ledger clear (M8): marker kept, accounts zeroed.
				projection.AdjacentCauses[i].GatedShareClaimedMS = 0
				projection.AdjacentCauses[i].GatedShareFullMS = 0
				projection.AdjacentCauses[i].GatedShareClaimSeats = nil
				projection.AdjacentCauses[i].MergedCount = 2
			}
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := runtimeTraceProjTreeFence(model, zh)
			a := levelmergeFindRow(&model, "lm-a")
			if a == nil {
				t.Fatalf("fixture drifted: the merged constituent row must render")
			}
			want := "分账构成份(合并行)"
			if !zh {
				want = "split-account constituent share (merged row)"
			}
			if !rspaFenceContains(fence, want) {
				t.Fatalf("the merged zero-value constituent row must self-explain %q:\n%s", want, fence)
			}
			if !model.Marks.has(runtimeTraceProjMarkGatedShareSplit) {
				t.Fatalf("the split legend mark must record on the merged form")
			}
			// The ◎ census still claims the row and the footnote names it —
			// the marker bool is the census key, not the cleared floats.
			if runtimeTraceProjElimEligible(*a) {
				t.Fatalf("the merged constituent row must stay out of the ◎ population")
			}
			if !runtimeTraceProjElimGatedConstituentExcluded(*a) {
				t.Fatalf("the ◎ exclusion census must still claim the merged row")
			}
			elimFence := runtimeTraceProjElimOverviewFence(projection, model, zh)
			footnote := "分账构成份(已计入反转席,降道):1 行"
			if !zh {
				footnote = "split-account constituent share(s) (counted at the inversion seat, demoted): 1 row(s)"
			}
			if !rspaFenceContains(elimFence, footnote) {
				t.Fatalf("the ◎ footnote must keep naming the merged row:\n%s", elimFence)
			}
		})
	}
}
