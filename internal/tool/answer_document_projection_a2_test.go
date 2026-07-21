package tool

// answer_document_projection_a2_test.go — A2 batch pins (§29.174 处置⑤ 队列尾
// + §29.179 A 批委托 + §29.190, 2026-07-21).
//
// Mutation ledger (突变臂, cp 纪律, red logs in the batch workspace):
//   M-件1  reverting the direction lane to the template table reds
//          TestA2NextStepTemplateSentencesRetired / DirectionActionsFromSections;
//   M-件2  dropping the mini-legend emission reds TestA2TreeMiniLegendCoversUsedGlyphs;
//   M-件3  removing the main-line fold reds TestA2SeatRowFoldBudget;
//   M-件4① re-rooting the 构成 edge word reds TestA2MakeupEdgeWordFaces;
//   M-件4③ removing the flat-lane sort reds TestA2FlatLaneValueSorted.

import (
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// a2StripFenceMiniLegend removes the A2 件2 tree-head mini-legend lines (the
// "- 记号速览: "/"- mark key: " head note and its two-space continuations) so
// glyph-COUNT pins keep counting body emissions only — the key deliberately
// repeats every used glyph once (先用后释), which is not a body emission.
func a2StripFenceMiniLegend(fence string) string {
	var out []string
	inKey := false
	for _, line := range strings.Split(fence, "\n") {
		if strings.HasPrefix(line, "- 记号速览: ") || strings.HasPrefix(line, "- mark key: ") {
			inKey = true
			continue
		}
		if inKey && strings.HasPrefix(line, "  ") {
			continue
		}
		inKey = false
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// a2SeatRowFoldProjection is the 件3 fold-bearing shape for the bidirectional
// legend sweep: one background cross-thread aggregate whose stanza row's main
// line exceeds the row budget, so the ↳ fold continuation (and its legend
// entry) render.
func a2SeatRowFoldProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"tppmgr-300", "VSyncGenerator-2270"},
		WindowStartTs: 3679.899,
		WindowEndTs:   3681.129,
		OnChainCauses: []types.TraceCausalProjectionNode{
			elimChainNode("E-fold-run", "tppmgr-300", "runnable_wait", "runnable", 1, 26.392, 100),
		},
		BackgroundCauses: []types.TraceCausalProjectionNode{compareProjAggregateNode()},
	}
}

// TestA2TreeMiniLegendCoversUsedGlyphs — 件2 先用后释清零 pin: on a rich board
// every closed-table glyph the body actually stamped appears in the head key,
// the key sits BEFORE the first body row, and a board render carries the
// full-legend pointer. zh and EN.
func TestA2TreeMiniLegendCoversUsedGlyphs(t *testing.T) {
	for _, zh := range []bool{true, false} {
		projection := elimBoardProjection()
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		pointer := "(全释义见 树读法)"
		leadWord := "- 记号速览: "
		if !zh {
			pointer = "(full definitions: Tree reading)"
			leadWord = "- mark key: "
		}
		keyAt := strings.Index(fence, leadWord)
		if keyAt < 0 || !strings.Contains(fence, pointer) {
			t.Fatalf("zh=%v: mini legend + pointer must render on a glyph-bearing board:\n%s", zh, fence)
		}
		bodyAt := strings.Index(fence, "│")
		if bodyAt >= 0 && bodyAt < keyAt {
			t.Fatalf("zh=%v: mini legend must sit before the first body row (先用后释):\n%s", zh, fence)
		}
		// Coverage: every closed-table mark the render stamped has its word in
		// the key region (the region = key line plus its continuations).
		keyRegion := fence[keyAt:]
		if end := strings.Index(keyRegion, "\n│"); end >= 0 {
			keyRegion = keyRegion[:end]
		}
		for _, entry := range runtimeTraceProjTreeMiniLegendGlyphTable() {
			if !model.Marks.has(entry.mark) {
				continue
			}
			word := entry.zh
			if !zh {
				word = entry.en
			}
			if !strings.Contains(keyRegion, word) {
				t.Fatalf("zh=%v: used glyph %q missing from the mini legend key:\n%s", zh, word, fence)
			}
		}
	}
}

// TestA2SeatRowFoldBudget — 件3 pin: an over-budget stanza seat row folds at a
// whitelist boundary into a ↳ continuation; every emitted physical line holds
// the row cap; content is byte-whole across the fold (only boundary spaces
// trim); and the ↳ legend/mini-key entries ride along (记号-图例双向).
func TestA2SeatRowFoldBudget(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: compareProjAggregateNode(),
	}
	marks := &runtimeTraceProjMarkSet{}
	row.marks = marks
	line := runtimeTraceProjStanzaRowLine(row, runtimeTraceProjTreeLabelWidth, 1230.0, true, true)
	physical := strings.Split(line, "\n")
	if len(physical) < 2 {
		t.Fatalf("件3: the over-budget aggregate row must fold:\n%s", line)
	}
	for _, l := range physical {
		if w := runewidth.StringWidth(l); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("件3: folded physical line still over budget (w=%d): %q", w, l)
		}
	}
	for _, l := range physical[1:] {
		if !strings.Contains(l, "⤷ ") {
			t.Fatalf("件3: continuation must open with the ⤷ marker: %q", l)
		}
	}
	if !marks.has(runtimeTraceProjMarkSeatRowWrapCont) {
		t.Fatalf("件3: the fold must stamp the ⤷ mark for the legend/mini key")
	}
	// Byte-wholeness: joining the physical lines (marker + boundary spaces
	// stripped) reproduces the unfolded content.
	var joined strings.Builder
	for i, l := range physical {
		part := l
		if i > 0 {
			part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "⤷"))
		}
		joined.WriteString(part)
	}
	for _, want := range []string{"101084.884ms(跨线程累计,非墙钟)", "≈平均排队深度 82.2"} {
		if !strings.Contains(joined.String(), want) {
			t.Fatalf("件3: fold lost content %q:\n%s", want, line)
		}
	}
}

// TestA2FlatLaneValueSorted — 件4③ pin: ≥2 flat on-chain sibling roots render
// value-descending and the tree head carries the commitment sentence; a
// single flat seat makes no ordering claim (negative arm).
func TestA2FlatLaneValueSorted(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:              []string{"ui-1"},
		RootCauseFamilyObserved: true,
		OnChainCauses: []types.TraceCausalProjectionNode{
			elimChainNode("E-small", "worker-2", "runnable_wait", "runnable", 4, 3.0, 100),
			elimChainNode("E-big", "worker-3", "runnable_wait", "runnable", 5, 9.0, 200),
			elimChainNode("E-mid", "worker-4", "runnable_wait", "runnable", 6, 6.0, 300),
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "- 链上平铺席按显示值降序(仅平铺席;树位挂靠行按链结构)") {
		t.Fatalf("件4③: ≥2 flat seats must declare the ordering commitment:\n%s", fence)
	}
	big := strings.Index(fence, "worker-3")
	mid := strings.Index(fence, "worker-4")
	small := strings.Index(fence, "worker-2")
	if big < 0 || mid < 0 || small < 0 || !(big < mid && mid < small) {
		t.Fatalf("件4③: flat seats must render value-descending (9.0 at %d, 6.0 at %d, 3.0 at %d):\n%s",
			big, mid, small, fence)
	}
	// Negative arm: one flat seat → no ordering claim.
	single := projection
	single.OnChainCauses = projection.OnChainCauses[:1]
	singleModel := buildRuntimeTraceProjTreeModel(single, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if strings.Contains(runtimeTraceProjTreeFence(singleModel, true), "链上平铺席按显示值降序") {
		t.Fatalf("件4③ 负臂: a single flat seat must make no ordering claim")
	}
}

// TestA2MakeupEdgeWordFaces — 件4① pin (§29.174 UX-16①): the state-makeup
// edge family speaks 构成/makeup on all four faces (edge label, 行2 chip,
// detail relation, legend), and the retired 成因 word face is gone from the
// edge family (词面单点; the ranked-row noun 成因行身份行 is a different word
// and deliberately stays).
func TestA2MakeupEdgeWordFaces(t *testing.T) {
	if got := runtimeTraceProjEdgeLabel(runtimeTraceProjTreeEdgeCause, true); got != "构成─" {
		t.Fatalf("件4① zh edge word: %q", got)
	}
	if got := runtimeTraceProjEdgeLabel(runtimeTraceProjTreeEdgeCause, false); got != "makeup─" {
		t.Fatalf("件4① en edge word: %q", got)
	}
	catalog := runtimeTraceProjLegendCatalog()
	found := false
	for _, entry := range catalog {
		if entry.Mark != runtimeTraceProjMarkEdgeCause {
			continue
		}
		found = true
		if !strings.Contains(entry.ZH, "├─构成─") || !strings.Contains(entry.ZH, "非因果声明") {
			t.Fatalf("件4① zh legend entry must teach 构成 as a non-causal state makeup: %q", entry.ZH)
		}
		if !strings.Contains(entry.EN, "├─makeup─") {
			t.Fatalf("件4① en legend entry: %q", entry.EN)
		}
		if strings.Contains(entry.ZH, "成因") {
			t.Fatalf("件4① 词面单点: the retired 成因 word must leave the edge legend: %q", entry.ZH)
		}
	}
	if !found {
		t.Fatalf("件4①: the makeup edge legend entry is missing")
	}
}

// TestA2NextStepTemplateSentencesRetired — 件1 禁模板句 pin: the retired
// per-record template sentences never render again — on a ledger carrying the
// exact witness-shaped next_step records, the next-step list holds none of
// the three runnable_2:503-505 sentences (nor the EN prose passthrough).
func TestA2NextStepTemplateSentencesRetired(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{AnswerContract: types.AnswerContract{Language: "zh"}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true,
		Observations: []types.ObservationRecord{
			{
				ID: "trace_query:r#1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", Predicate: "state_churn",
				RichNotes: []string{"next_step=investigate the peer threads that repeatedly wake it", "next_step_kind=s_sleep"},
			},
			{
				ID: "trace_query:r#2", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", Predicate: "state_churn",
				RichNotes: []string{"next_step=inspect low-priority dependency threads", "next_step_kind=priority_inversion"},
			},
		}}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	items := runtimeTraceNextStepItems(doc, bus)
	joined := ""
	for _, item := range items {
		joined += item.Text + "\n"
	}
	for _, banned := range []string{
		"排查相邻的调度与资源事件",
		"排查反复唤醒它的对端线程",
		"排查所依赖的低优先级线程的调度延迟",
		"排查同 CPU 竞争",
		"investigate the peer threads that repeatedly wake it",
		"inspect low-priority dependency threads",
	} {
		if strings.Contains(joined, banned) {
			t.Fatalf("件1 禁模板句: retired template sentence %q re-rendered:\n%s", banned, joined)
		}
	}
}

// TestA2NextStepDirectionActionsFromSections — 件1 正臂: a published ◎
// direction section synthesizes exactly one subject+value action row whose
// value is the section head's 最大可消 bytes; the unresolved tail section
// emits nothing (无席方向不发). zh and EN word faces.
func TestA2NextStepDirectionActionsFromSections(t *testing.T) {
	section := runtimeTraceProjElimSection{
		direction: "lock_priority",
		maxEff:    23.994,
		entries: []runtimeTraceProjElimEntry{{row: runtimeTraceProjTreeRow{
			Node: types.TraceCausalProjectionNode{Subject: "CookieMonsterCl-59843",
				EffectiveImpactMS: 23.994},
		}}},
	}
	action, ok := runtimeTraceNextStepDirectionActionFor(section, true)
	if !ok {
		t.Fatalf("件1: a resolved-direction section must synthesize an action")
	}
	if action.text != "锁与优先级→评估提升 CookieMonsterCl-59843 调度优先级或减少其唤醒往返依赖(23.994ms 可消)" {
		t.Fatalf("件1 zh action word face drifted: %q", action.text)
	}
	enAction, ok := runtimeTraceNextStepDirectionActionFor(section, false)
	if !ok || !strings.Contains(enAction.text, "lock & priority → ") ||
		!strings.Contains(enAction.text, "CookieMonsterCl-59843") ||
		!strings.Contains(enAction.text, "(23.994ms eliminable)") {
		t.Fatalf("件1 en action word face drifted: %q", enAction.text)
	}
	// 无席方向不发: the unresolved tail (direction "") synthesizes nothing.
	tail := section
	tail.direction = ""
	if _, ok := runtimeTraceNextStepDirectionActionFor(tail, true); ok {
		t.Fatalf("件1: the unresolved tail section must not synthesize an action")
	}
}

// TestA2BadgeGlyphGeometry — 件10 (§29.191 BADGEVIS) pins:
// (a) 格宽断言 — every badge glyph measures EXACTLY one cell under BOTH
//
//	go-runewidth east-asian conditions (the ❶ U+2776 family was ambiguous:
//	2 cells on a CJK terminal vs 1 counted — the occlusion root); and every
//	badge emission is followed by exactly one space (禁后字紧贴), including
//	the detail ordinal token (the former "❶#1" glyph-adjacent form).
//
// (b) 字形换族 — the family is the U+278A sans-serif block ➊..➎ (single mint
//
//	point tracefence.BadgeGlyphs).
func TestA2BadgeGlyphGeometry(t *testing.T) {
	badges := tracefence.BadgeGlyphs()
	want := []string{"➊", "➋", "➌", "➍", "➎"}
	if len(badges) != len(want) {
		t.Fatalf("件10(b): badge family drifted: %v", badges)
	}
	wide := runewidth.Condition{EastAsianWidth: true}
	narrow := runewidth.Condition{EastAsianWidth: false}
	for i, glyph := range badges {
		if glyph != want[i] {
			t.Fatalf("件10(b): badge %d must be %q (U+278A family), got %q", i+1, want[i], glyph)
		}
		r := []rune(glyph)[0]
		if wide.RuneWidth(r) != 1 || narrow.RuneWidth(r) != 1 {
			t.Fatalf("件10(a) 格宽: badge %q must measure 1 cell under BOTH east-asian conditions (wide=%d narrow=%d)",
				glyph, wide.RuneWidth(r), narrow.RuneWidth(r))
		}
	}
	// (a) one-space arm, detail ordinal token face: glyph + exactly one space.
	if got := runtimeTraceProjSeatOrdinalToken(2, 2); got != "➋ #2" {
		t.Fatalf("件10(a): the detail ordinal token must carry exactly one post-badge space, got %q", got)
	}
	if got := runtimeTraceProjSeatOrdinalToken(7, 0); got != "#7" {
		t.Fatalf("badge-less ordinal token keeps the bare chip form, got %q", got)
	}
	// (a) one-space arm, tree row face: on a badge-wearing rendered row the
	// glyph's immediate successor is exactly one space (never a second space,
	// never a glyph).
	model := buildRuntimeTraceProjTreeModel(elimBoardProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	sawBadge := false
	for _, line := range strings.Split(a2StripFenceMiniLegend(fence), "\n") {
		for i, r := range line {
			if r != '➊' && r != '➋' && r != '➌' && r != '➍' && r != '➎' {
				continue
			}
			sawBadge = true
			rest := line[i+len(string(r)):]
			if !strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "  ") {
				t.Fatalf("件10(a): badge must be followed by exactly one space: %q", line)
			}
		}
	}
	if !sawBadge {
		t.Fatalf("fixture drifted: no badge-wearing row rendered:\n%s", fence)
	}
}

// a2UnfoldSeatRows joins ⤷ fold continuations back onto their host line (no
// separator — the fold trims only boundary break spaces), so verbatim
// row-scoped pins can keep asserting logical-row content after A2 件3 brought
// seat rows into the fold width budget. Byte caveat: a break that consumed a
// boundary space reconstructs without it — callers assert space-insensitive
// needles across fold boundaries.
func a2UnfoldSeatRows(fence string) string {
	var out []string
	for _, line := range strings.Split(fence, "\n") {
		trimmed := strings.TrimLeft(line, "│ ")
		if strings.HasPrefix(trimmed, "⤷ ") && len(out) > 0 {
			out[len(out)-1] += strings.TrimPrefix(trimmed, "⤷ ")
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// TestA2VerdictFamilyLabelRenamed — 件8 (§29.190①) pin: the ◎ verdict-grammar
// legend entry speaks the renamed family label 「IO/内核/依赖族」/"IO / kernel /
// dependency family" (§29.187② 连带), and the retired label is gone (zh/EN
// 词面单点).
func TestA2VerdictFamilyLabelRenamed(t *testing.T) {
	for _, entry := range runtimeTraceProjLegendCatalog() {
		if !strings.Contains(entry.ZH, "◎ 判词文法") {
			continue
		}
		if !strings.Contains(entry.ZH, "IO/内核/依赖族=IO阻塞") || strings.Contains(entry.ZH, "IO与依赖族") {
			t.Fatalf("件8 zh family label must be 「IO/内核/依赖族」: %q", entry.ZH)
		}
		if !strings.Contains(entry.EN, "IO / kernel / dependency family = IO blocking") {
			t.Fatalf("件8 en family label must follow: %q", entry.EN)
		}
		return
	}
	t.Fatalf("verdict grammar legend entry missing")
}

// TestA2HeadlineBadgeSeatNote — 件9 (§29.190②) pins: (正臂) a crown that is
// not its board's ➊ seat wears the badge-arm parenthetical even when the ◎
// arm is silent; (负臂) a crown that IS a board-➊ seat stays silent; the
// existing ◎-arm wording is untouched (既有臂不回归 — pinned by the run2fixa
// tests).
func TestA2HeadlineBadgeSeatNote(t *testing.T) {
	crown := types.TraceCausalProjectionNode{
		EvidenceID: "E-crown", Subject: "worker-9", RankBoardTarget: "board-1",
		EffectiveImpactMS: 5.0,
	}
	badgeRow := runtimeTraceProjTreeRow{Badge: 1, Node: types.TraceCausalProjectionNode{
		EvidenceID: "E-badge", Subject: "worker-1", RankBoardTarget: "board-1",
		EffectiveImpactMS: 9.0,
	}}
	model := runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{badgeRow}}
	if got := runtimeTraceProjHeadlineBadgeSeatNote(model, &crown, true); got != "(主榜席;➊ 按板内发布序)" {
		t.Fatalf("件9 正臂 zh note drifted: %q", got)
	}
	if got := runtimeTraceProjHeadlineBadgeSeatNote(model, &crown, false); got != " (lead-board seat; ➊ follows the board's published order)" {
		t.Fatalf("件9 正臂 en note drifted: %q", got)
	}
	// 负臂1: the crown IS a board-➊ seat (twin ordinals: any match silences).
	self := crown
	self.EvidenceID = "E-badge"
	if got := runtimeTraceProjHeadlineBadgeSeatNote(model, &self, true); got != "" {
		t.Fatalf("件9 负臂: crown==➊ must stay silent, got %q", got)
	}
	// 负臂2: no board identity / no badge row on the board → no claim (宁漏).
	bare := crown
	bare.RankBoardTarget = ""
	if got := runtimeTraceProjHeadlineBadgeSeatNote(model, &bare, true); got != "" {
		t.Fatalf("件9: a board-less crown must claim nothing, got %q", got)
	}
	other := crown
	other.RankBoardTarget = "board-2"
	if got := runtimeTraceProjHeadlineBadgeSeatNote(model, &other, true); got != "" {
		t.Fatalf("件9: a board with no rendered ➊ must claim nothing, got %q", got)
	}
}

// TestA2WireFoldMaxMemberDisclosure — 件5② (§29.179 A 批委托) pins: a
// wire-fold record carrying the new folded_max_subject/folded_max_state_kind
// keys re-materializes the RUN2FIX-A 件2 max-member carriers and the fold row
// renders the 成员最大 clause; a record without the subject key keeps the
// range-only legacy line (宁漏勿假). Verbatim "key=value" literals are the
// deliberate wire double-write (trace_note_keys.go change protocol step 3).
func TestA2WireFoldMaxMemberDisclosure(t *testing.T) {
	record := types.ObservationRecord{
		ID: "trace_query:x#wakeup_causal_impact_fold", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", Predicate: "wakeup_causal_impact",
		GroundingPolicy: types.ClaimGroundingHard,
		Subject:         "", Value: "47.282", Unit: "ms", Confidence: 0.78,
		ClaimKey: "wakeup_causal_impact:folded_overflow",
		Span:     types.ObservationSpan{LineStart: 100, LineEnd: 200},
		RichNotes: []string{
			"causality=on_wakeup_chain", "chain_relevance=on_chain",
			"impact=47.282", "folded_rows=8", "folded_min_ms=0.858", "folded_max_ms=47.282",
			"folded_subjects=CookieMonsterCl-59843,ThreadPoolForeg-60555",
			"folded_max_subject=CookieMonsterCl-59843",
			"folded_max_state_kind=s_sleep",
		},
	}
	projection := types.TraceCausalProjectionFromObservationRecords([]types.ObservationRecord{record})
	var fold *types.TraceCausalProjectionNode
	for i := range projection.OnChainCauses {
		if projection.OnChainCauses[i].MergedCount == 8 {
			fold = &projection.OnChainCauses[i]
		}
	}
	if fold == nil {
		t.Fatalf("wire fold row did not re-materialize: %+v", projection)
	}
	if fold.MergedMaxSubject != "CookieMonsterCl-59843" || fold.MergedMaxStateKind != "s_sleep" {
		t.Fatalf("件5②: max-member carriers must re-materialize: %+v", fold)
	}
	clause := runtimeTraceProjFoldMaxMemberClause(*fold, true)
	if !strings.Contains(clause, "成员最大 CookieMonsterCl-59843") || !strings.Contains(clause, "47.282ms") {
		t.Fatalf("件5②: the fold row must render the max-member clause: %q", clause)
	}
	// 负臂: no subject key → carriers stay empty (state alone claims nothing).
	bare := record
	bare.RichNotes = []string{
		"causality=on_wakeup_chain", "chain_relevance=on_chain",
		"impact=47.282", "folded_rows=8", "folded_min_ms=0.858", "folded_max_ms=47.282",
		"folded_max_state_kind=s_sleep",
	}
	bareProj := types.TraceCausalProjectionFromObservationRecords([]types.ObservationRecord{bare})
	for _, node := range bareProj.OnChainCauses {
		if node.MergedCount == 8 && (node.MergedMaxSubject != "" || node.MergedMaxStateKind != "") {
			t.Fatalf("件5② 负臂: subject-less wire fold must not mint carriers: %+v", node)
		}
	}
}

// TestA2SelfInversionPeerQualifierLine — 件13 (§29.192.1 + §29.192.3 修正)
// pins: a SELF inversion seat with a typed resolved BlockingPeer names the
// peer on its own 行2 qualifier line (tree face only); an unresolved peer
// emits nothing (宁漏勿假); the ◎ seat row stays byte-compact (零动臂); a
// NON-self inversion seat never grows the line (self-lane composer only).
func TestA2SelfInversionPeerQualifierLine(t *testing.T) {
	seat := types.TraceCausalProjectionNode{
		EvidenceID: "E-selfinv", Subject: "ui-1", Object: "priority_inversion_candidate",
		TypeToken: "priority_inversion_candidate", PriorityInversionCandidate: true,
		Tier: "primary", Predicate: "root_cause_primary",
		ChainRelevance: "on_chain", Rank: 1, Confidence: 0.9,
		ImpactMS: 12.0, EffectiveImpactMS: 12.0,
		BlockingKind: "lock_contention", BlockingPeer: "worker-7",
	}
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"waker-2", "ui-1"},
		OnChainCauses:           []types.TraceCausalProjectionNode{seat},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "· 对端 worker-7(低优先级已证)") {
		t.Fatalf("件13 正臂: the self inversion seat must name its resolved peer on 行2:\n%s", fence)
	}
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	if enFence := runtimeTraceProjTreeFence(enModel, false); !strings.Contains(enFence, "· peer worker-7 (proven lower priority)") {
		t.Fatalf("件13 EN word face missing:\n%s", enFence)
	}
	// ◎ 零动臂: the overview seat row carries no peer wording.
	elim := runtimeTraceProjElimOverviewFence(projection, model, true)
	if strings.Contains(elim, "对端 worker-7") {
		t.Fatalf("件13 ◎ 零动: the overview row must stay compact:\n%s", elim)
	}
	// 负臂: unresolved peer emits nothing.
	unresolved := projection
	unresolved.OnChainCauses = []types.TraceCausalProjectionNode{seat}
	unresolved.OnChainCauses[0].BlockingPeer = ""
	uModel := buildRuntimeTraceProjTreeModel(unresolved, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if strings.Contains(runtimeTraceProjTreeFence(uModel, true), "对端 ") {
		t.Fatalf("件13 负臂: an unresolved peer must emit no qualifier line")
	}
}

// TestA2SelfSupplyFoldSeatWireCapExempt — 件11(a) (§29.192 + §29.192.2 修正)
// pins: the target's own supply-fold deficit seat survives the per-family
// wire position cap (tree-face 恒显 swallow point #2; the engine board cap is
// already closed by the §29.93.3 selfSide lane); an ordinary seat at the same
// over-cap position stays cut (值切其他席不受影响负臂). The ◎ face keeps
// ZERO exemption by construction (no ◎ code touched — its TOP5 slice still
// reads the board order; pinned via the roster helper's unchanged TopN math
// in TestOmgclean/elim suites).
func TestA2SelfSupplyFoldSeatWireCapExempt(t *testing.T) {
	if got := traceQuerySelfSupplyFoldSeatCapExempt(tracequery.RootCauseRankItem{
		SupplyFoldDeficitMs: 2.5, SubjectIsAnalysisTarget: true,
		OnChainBasis: tracequery.RootCauseOnChainBasisSelfWallClockInterval,
	}); !got {
		t.Fatalf("件11(a): the self supply-fold seat must ride the cap exemption")
	}
	for name, item := range map[string]tracequery.RootCauseRankItem{
		"no deficit":     {SubjectIsAnalysisTarget: true, OnChainBasis: tracequery.RootCauseOnChainBasisSelfWallClockInterval},
		"foreign seat":   {SupplyFoldDeficitMs: 2.5, OnChainBasis: tracequery.RootCauseOnChainBasisSelfWallClockInterval},
		"non-self basis": {SupplyFoldDeficitMs: 2.5, SubjectIsAnalysisTarget: true},
	} {
		if traceQuerySelfSupplyFoldSeatCapExempt(item) {
			t.Fatalf("件11(a) 负臂 %s: only the typed triple exempts", name)
		}
	}
	// Wire-level arm: a board whose self fold seat sits past the family cap
	// still publishes its record; the ordinary over-cap neighbor stays cut.
	cap := traceQueryWidthTypedFamilyRowCap()
	items := make([]tracequery.RootCauseRankItem, 0, cap+2)
	for i := 0; i < cap; i++ {
		items = append(items, tracequery.RootCauseRankItem{
			Thread: tracequery.ThreadRef{PID: 1000 + i, Comm: "w"}, Type: "runnable_wait",
			Rank: i + 1, Tier: "primary", EffectiveImpactMs: float64(100 - i),
			Causality: "on_wakeup_chain", LineStart: 10 * (i + 1), LineEnd: 10*(i+1) + 5,
		})
	}
	items = append(items,
		tracequery.RootCauseRankItem{
			Thread: tracequery.ThreadRef{PID: 9000, Comm: "cut-me"}, Type: "runnable_wait",
			Rank: cap + 1, Tier: "secondary", EffectiveImpactMs: 0.5,
			Causality: "on_wakeup_chain", LineStart: 900, LineEnd: 905,
		},
		tracequery.RootCauseRankItem{
			Thread: tracequery.ThreadRef{PID: 42, Comm: "target"}, Type: "running",
			Rank: cap + 2, Tier: "tertiary", EffectiveImpactMs: 0.3,
			SupplyFoldDeficitMs: 0.3, SubjectIsAnalysisTarget: true,
			OnChainBasis: tracequery.RootCauseOnChainBasisSelfWallClockInterval,
			Causality:    "self_wall_clock", LineStart: 950, LineEnd: 955,
		})
	result := tracequery.Result{
		View: "root_cause_rank", SourcePath: "x.systrace",
		RootCauseRank: &tracequery.RootCauseRankResult{Items: items},
	}
	obs := traceQueryTypedObservations(result, "fixture", "p-rank", "r", "", time.Unix(1751600000, 0).UTC())
	sawSelf, sawCut := false, false
	for _, record := range obs {
		if strings.Contains(record.Subject, "target-42") {
			sawSelf = true
		}
		if strings.Contains(record.Subject, "cut-me") {
			sawCut = true
		}
	}
	if !sawSelf {
		t.Fatalf("件11(a): the over-cap self fold seat must still publish its record")
	}
	if sawCut {
		t.Fatalf("件11(a) 负臂: the ordinary over-cap row must stay cut")
	}
}

// TestA2AuxProliferableFamiliesTop3 — 件12 (§29.192) pins: the ∩ overlap-pair
// family renders TOP3 by overlap value with an honest tail count; the 构成拆解
// family caps at 3 + tail; the single-row families (守恒/未入榜/未入榜最大)
// stay single rows (零动臂 — asserted structurally: one label, one row).
func TestA2AuxProliferableFamiliesTop3(t *testing.T) {
	entry := func(tag string, clauses ...runtimeTraceProjCrossDirectionClause) runtimeTraceProjElimEntry {
		row := runtimeTraceProjTreeRow{EvidenceTag: tag}
		row.CrossDirectionOverlapClauses = clauses
		return runtimeTraceProjElimEntry{row: row}
	}
	rendered := []runtimeTraceProjElimEntry{
		entry("E1", runtimeTraceProjCrossDirectionClause{Ref: "E2", OverlapMS: 1.0}),
		entry("E3", runtimeTraceProjCrossDirectionClause{Ref: "E4", OverlapMS: 9.0}),
		entry("E5", runtimeTraceProjCrossDirectionClause{Ref: "E6", OverlapMS: 5.0}),
		entry("E7", runtimeTraceProjCrossDirectionClause{Ref: "E8", OverlapMS: 3.0}),
		entry("E9", runtimeTraceProjCrossDirectionClause{Ref: "E10", OverlapMS: 7.0}),
	}
	rows := runtimeTraceProjElimCrossDirectionFootnote(rendered, &runtimeTraceProjMarkSet{}, true)
	if len(rows) != 4 {
		t.Fatalf("件12: 5 pairs must render TOP3 + one tail count row, got %d: %+v", len(rows), rows)
	}
	for i, want := range []string{"9.000ms", "7.000ms", "5.000ms"} {
		if !strings.Contains(rows[i].content, want) {
			t.Fatalf("件12: pair rows must sort by overlap desc (row %d wants %s): %+v", i, want, rows)
		}
	}
	if rows[3].content != "另有 2 对见树行" {
		t.Fatalf("件12: tail count row drifted: %+v", rows[3])
	}
	// ≤3 pairs: no tail row (负臂).
	small := runtimeTraceProjElimCrossDirectionFootnote(rendered[:2], &runtimeTraceProjMarkSet{}, true)
	if len(small) != 2 {
		t.Fatalf("件12 负臂: ≤3 pairs must render without a tail row: %+v", small)
	}
}
