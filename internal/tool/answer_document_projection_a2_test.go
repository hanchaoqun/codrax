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
	"unicode"

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
// line exceeds the row budget, so the ⤷ fold continuation (and its legend
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
// whitelist boundary into a ⤷ continuation; every emitted physical line holds
// the row cap; content is byte-whole across the fold (only boundary spaces
// trim); and the ⤷ legend/mini-key entries ride along (记号-图例双向).
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
	// Dual-review F3 (2026-07-21): the EN commitment line renders AND holds
	// the 100-cell head budget itself (the first form measured 101 and handed
	// five EN dumps a fresh over-budget line).
	// SMALL3-1 件2 (§29.196③, 2026-07-21): a ⌗-LESS flat lane keeps this A2
	// sentence byte-identically on both faces — the 「(计数当量行恒末)」
	// half-sentence forms render exactly when a ⌗ tail row is on the board
	// (词条-图例双向; pinned in TestA2FlatLaneCaliberSideSinksToTail).
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := runtimeTraceProjTreeFence(enModel, false)
	enLine := "- flat on-chain seats sort by displayed value desc (flat seats only; attached rows follow the chain)"
	if !strings.Contains(enFence, enLine) {
		t.Fatalf("件4③ F3: EN commitment sentence missing or drifted:\n%s", enFence)
	}
	if w := runewidth.StringWidth(enLine); w > runtimeTraceProjTreeRowMaxWidth {
		t.Fatalf("件4③ F3: EN commitment head line over the row budget (w=%d)", w)
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

// TestA2FrequencyNextStepDoesNotClaimUnprovedLimit — B949: the
// frequency_thermal seat proves a modeled supply-fold opportunity against an
// ideal basis. It does not, by itself, prove that a policy/thermal limit bound
// the target slice. Keep the subject, action and value, but never turn that
// typed headroom into a deterministic "lift the limit" claim.
func TestA2FrequencyNextStepDoesNotClaimUnprovedLimit(t *testing.T) {
	section := runtimeTraceProjElimSection{
		direction: "frequency_thermal",
		maxEff:    58.320,
		entries: []runtimeTraceProjElimEntry{{row: runtimeTraceProjTreeRow{
			Node: types.TraceCausalProjectionNode{
				Subject:           ".ugc.aweme.lite-17267",
				EffectiveImpactMS: 58.320,
			},
		}}},
	}
	zh, ok := runtimeTraceNextStepDirectionActionFor(section, true)
	if !ok || zh.text != "频率与热治理→评估提升 .ugc.aweme.lite-17267 的运行算力供给(升频/迁核)(按既定理想基准折算的可提升空间 58.320ms)" {
		t.Fatalf("B949 zh calibrated action drifted: ok=%t text=%q", ok, zh.text)
	}
	for _, banned := range []string{"解除", "运行频点限制"} {
		if strings.Contains(zh.text, banned) {
			t.Fatalf("B949 zh action overclaimed an unproved target limit via %q: %q", banned, zh.text)
		}
	}

	en, ok := runtimeTraceNextStepDirectionActionFor(section, false)
	if !ok || en.text != "frequency & thermal → evaluate improving .ugc.aweme.lite-17267's running compute supply (boost / migrate) (modeled headroom 58.320ms versus the stated ideal basis)" {
		t.Fatalf("B949 en calibrated action drifted: ok=%t text=%q", ok, en.text)
	}
	for _, banned := range []string{"lifting", "running-frequency limits", "eliminable"} {
		if strings.Contains(en.text, banned) {
			t.Fatalf("B949 en action overclaimed an unproved target limit via %q: %q", banned, en.text)
		}
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
// peer on its own 行2-region qualifier line (tree face only); an unresolved
// peer emits nothing (宁漏勿假); the ◎ seat row stays byte-compact (零动臂);
// a NON-self inversion seat never grows the line (self-lane composer only).
// Dual-review F1 (2026-07-21): the 「(低优先级已证)」/"proven lower priority"
// parenthetical is RETIRED (typed premise falsified — no hard-caliber
// relation-peer carrier exists on the wire; 宁漏勿假 word downgrade) and its
// absence is pinned.
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
	if !strings.Contains(fence, "· 对端 worker-7") {
		t.Fatalf("件13 正臂: the self inversion seat must name its resolved peer on 行2:\n%s", fence)
	}
	if strings.Contains(fence, "低优先级已证") {
		t.Fatalf("件13 F1 降词: the retired proven-lower-priority parenthetical re-rendered:\n%s", fence)
	}
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := runtimeTraceProjTreeFence(enModel, false)
	if !strings.Contains(enFence, "· peer worker-7") {
		t.Fatalf("件13 EN word face missing:\n%s", enFence)
	}
	if strings.Contains(enFence, "proven lower priority") {
		t.Fatalf("件13 F1 降词 EN: the retired parenthetical re-rendered:\n%s", enFence)
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
// pins: every engine-minted Rank>0 seat and the target's own supply-fold
// deficit seat survive the per-family wire position cap (tree-face 恒显
// swallow point #2); an unseated diagnostic at the same over-cap position
// stays cut. The ◎ face keeps
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
	if !traceQueryRootCauseRankRowCapExempt(tracequery.RootCauseRankItem{Rank: 33}) {
		t.Fatalf("件11(a): an engine-minted positive ordinal must survive the transport cap")
	}
	if traceQueryRootCauseRankRowCapExempt(tracequery.RootCauseRankItem{Tier: "tertiary"}) {
		t.Fatalf("件11(a) 负臂: an unseated diagnostic must retain the transport cap")
	}
	// Wire-level arm: a board whose self fold seat sits past the family cap
	// still publishes its record; an ordinary ranked seat also survives, while
	// an unseated over-cap neighbor stays cut.
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
			Thread: tracequery.ThreadRef{PID: 9000, Comm: "ranked-seat"}, Type: "runnable_wait",
			Rank: cap + 1, Tier: "secondary", EffectiveImpactMs: 0.5,
			Causality: "on_wakeup_chain", LineStart: 900, LineEnd: 905,
		},
		tracequery.RootCauseRankItem{
			Thread: tracequery.ThreadRef{PID: 42, Comm: "target"}, Type: "running",
			Rank: cap + 2, Tier: "tertiary", EffectiveImpactMs: 0.3,
			SupplyFoldDeficitMs: 0.3, SubjectIsAnalysisTarget: true,
			OnChainBasis: tracequery.RootCauseOnChainBasisSelfWallClockInterval,
			Causality:    "self_wall_clock", LineStart: 950, LineEnd: 955,
		},
		tracequery.RootCauseRankItem{
			Thread: tracequery.ThreadRef{PID: 9001, Comm: "cut-me"}, Type: "trace_gap",
			Tier: "data_gap", LineStart: 960, LineEnd: 965, Summary: "unseated diagnostic",
		})
	result := tracequery.Result{
		View: "root_cause_rank", SourcePath: "x.systrace",
		RootCauseRank: &tracequery.RootCauseRankResult{Items: items},
	}
	obs := traceQueryTypedObservations(result, "fixture", "p-rank", "r", "", time.Unix(1751600000, 0).UTC())
	sawSelf, sawRanked, sawCut := false, false, false
	for _, record := range obs {
		if strings.Contains(record.Subject, "target-42") {
			sawSelf = true
		}
		if strings.Contains(record.Subject, "cut-me") {
			sawCut = true
		}
		if strings.Contains(record.Subject, "ranked-seat") {
			sawRanked = true
		}
	}
	if !sawSelf {
		t.Fatalf("件11(a): the over-cap self fold seat must still publish its record")
	}
	if !sawRanked {
		t.Fatalf("件11(a): the over-cap engine-ranked seat must still publish its record")
	}
	if sawCut {
		t.Fatalf("件11(a) 负臂: the unseated over-cap diagnostic must stay cut")
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

// TestA2SeatRowFoldLongParenWhole — dual-review F2 pin (P2): a value's LONG
// caliber parenthetical never tears across a fold. The EN aggregate stanza row
// (the r2_dualboard_en / tieba_flagship_en witness shape) folds with the whole
// "value (cross-thread cumulative, not wall clock)" group on ONE physical
// line — the ⤷ legend promises breaks never land inside word+parenthetical
// units, and the 12-cell short-paren fusion alone let this 40-cell group
// break mid-parenthetical. A group wider than the fusion cap keeps its
// token-boundary breaks (the honest width bound of every capped fusion).
func TestA2SeatRowFoldLongParenWhole(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowBackground, HasData: true, Node: compareProjAggregateNode(),
	}
	marks := &runtimeTraceProjMarkSet{}
	row.marks = marks
	line := runtimeTraceProjStanzaRowLine(row, runtimeTraceProjTreeLabelWidth, 1230.0, true, false)
	physical := strings.Split(line, "\n")
	if len(physical) < 2 {
		t.Fatalf("F2: the over-budget EN aggregate row must still fold:\n%s", line)
	}
	group := "101084.884ms (cross-thread cumulative, not wall clock)"
	found := false
	for _, l := range physical {
		if w := runewidth.StringWidth(l); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("F2: folded physical line over budget (w=%d): %q", w, l)
		}
		if strings.Contains(l, group) {
			found = true
			continue
		}
		if strings.Contains(l, "(cross-thread") {
			t.Fatalf("F2: the value+parenthetical group tore across the fold: %q\n%s", l, line)
		}
	}
	if !found {
		t.Fatalf("F2: the whole value+parenthetical group must sit on one physical line:\n%s", line)
	}
	// Scope arm: the fusion is ASCII-content only — a CJK-content group keeps
	// the punct-aware lanes (fusing it would chain with the 词+数值 pass up to
	// exactly the line width and push a carried-`·` super-atom into the
	// rune-blind hard split, bisecting a CJK word; the displaywrap
	// never-bisects pin holds that arm).
	for _, chunk := range runtimeTraceProjWrapDisplay("链上L1(父节点未确认)·置信高", 20) {
		if w := runewidth.StringWidth(chunk); w > 20 {
			t.Fatalf("F2 scope: CJK-content group must keep token-boundary chunks (w=%d): %q", w, chunk)
		}
	}
	// 负臂 (honest bound): an over-cap group keeps token-boundary breaks —
	// fusion never mints an atom wider than the line.
	over := "1.0ms (" + strings.Repeat("wide ", 14) + "tail)"
	if chunks := runtimeTraceProjWrapDisplay(over, 40); len(chunks) < 2 {
		t.Fatalf("F2 负臂: an over-cap group must still break at token boundaries: %q", chunks)
	}
}

// TestDispfixLabelValueChipNeverSplits — DISPFIX-1 件2 (§29.213 排期件5): the
// zh 「·标签=值」 direction chip (·方向=IO/内核/依赖, io_dependency) is ONE
// unbreakable atom — the fold never lands a break between the CJK label and its
// "=<value>" (the witness broke "…·方向\n⤷ =IO/内核/依赖", contradicting the
// legend's 不拆「标签+值」 promise). Diagnosis was case (a): codrax's OWN
// atomizer split the chip (the value opens ASCII, so "=" bound to the value run
// and no pass fused the CJK label to it) — not a downstream viewer — so the fix
// is the label=value fusion in runtimeTraceProjWrapDisplay.
func TestDispfixLabelValueChipNeverSplits(t *testing.T) {
	// The live word table byte for io_dependency (single source, §29.187②).
	word, ok := tracefence.FixDirectionWord("io_dependency", true)
	if !ok || word != "IO/内核/依赖" {
		t.Fatalf("fixture drift: io_dependency zh word must be IO/内核/依赖, got %q ok=%v", word, ok)
	}
	chip := " ·方向=" + word
	prefix := "└─ worker-1234 runnable 12.345ms 有效归因 8.900ms ·板锚 app-100"
	line := prefix + chip
	// Widths where the pre-fix atomizer broke label|=value (empirically 40/45/
	// 70/75/80) plus the full budget.
	for _, w := range []int{40, 45, 55, 65, 70, 75, 80, 100} {
		chunks := runtimeTraceProjWrapDisplay(line, w)
		for i, c := range chunks {
			if runewidth.StringWidth(c) > w {
				t.Fatalf("width=%d: chunk over budget (w=%d): %q", w, runewidth.StringWidth(c), c)
			}
			if i > 0 && strings.HasPrefix(strings.TrimLeft(c, " "), "=") {
				t.Fatalf("width=%d: continuation opens with a naked =value (chip split): %q", w, c)
			}
			if strings.HasSuffix(c, "方向") {
				t.Fatalf("width=%d: a line ends with the bare label 方向 (chip split): %q", w, c)
			}
		}
		// Wrap is byte-identical (grouping only, never loss).
		if strings.Join(chunks, "") != line {
			t.Fatalf("width=%d: chunk concat not byte-identical to input", w)
		}
		// Wherever the chip lands, it lands WHOLE — any chunk mentioning the
		// label carries the full ·方向=值 contiguously (never a torn half).
		for _, c := range chunks {
			if strings.Contains(c, "方向") && !strings.Contains(c, "·方向="+word) {
				t.Fatalf("width=%d: the chip must appear whole (·方向=值) wherever it lands, got %q", w, c)
			}
		}
	}
	// 负臂: a fitting single line is byte-identical (no fold, no atom mangling).
	short := "◇ app-100 ·方向=" + word
	if chunks := runtimeTraceProjWrapDisplay(short, runtimeTraceProjTreeRowMaxWidth); len(chunks) != 1 || chunks[0] != short {
		t.Fatalf("负臂: a fitting chip line must stay single + byte-identical, got %v", chunks)
	}
	// 负臂: the existing /-enumeration fold pin never regresses — a bare over-
	// wide enum value still fuses whole (no label to bind, value stands alone).
	bareEnum := runtimeTraceProjWrapDisplay("worker "+word, 8)
	for _, c := range bareEnum {
		if strings.Contains(c, word) && strings.TrimLeft(c, " ") != word {
			// value packed with more than itself is fine; the point is it never
			// tore mid-enumeration.
		}
		if strings.HasPrefix(strings.TrimLeft(c, " "), "/") {
			t.Fatalf("负臂: /-enumeration must not open a line: %q", c)
		}
	}
}

// TestA2MiniLegendCoversGlyphLeadMarks — dual-review F4 structural pin: every
// full-legend MARK-group entry whose teaching clause opens with a backticked
// GLYPH (first rune non-ASCII, non-Han — a symbol a truncated reader cannot
// decode) must be decodable through the mini-legend closed table at the GLYPH
// level (some mini row's word face leads with that glyph). The used-glyph
// coverage pin walks only table rows, so an off-table glyph mark was
// structurally invisible to it (the ↳ gap this sweep closes). Exempt glyph
// leads, deliberately: ▸ (◎ direction section heads), · (annotation-line /
// separator grammar), — (◎ aux zone divider) — ◎-face structure taught on
// the ◎ face, not tree-fence one-off symbols; a NEW ◎-face glyph must be
// added here consciously.
func TestA2MiniLegendCoversGlyphLeadMarks(t *testing.T) {
	miniLeads := map[rune]bool{}
	for _, entry := range runtimeTraceProjTreeMiniLegendGlyphTable() {
		for _, r := range entry.zh {
			miniLeads[r] = true
			break
		}
	}
	exempt := map[rune]bool{'▸': true, '·': true, '—': true}
	for _, entry := range runtimeTraceProjLegendCatalog() {
		if entry.Group != runtimeTraceProjLegendGroupMark {
			continue
		}
		rest, ok := strings.CutPrefix(entry.ZH, "- `")
		if !ok || rest == "" {
			continue
		}
		lead := []rune(rest)[0]
		if lead < 0x80 || unicode.Is(unicode.Han, lead) || exempt[lead] {
			continue
		}
		if !miniLeads[lead] {
			t.Fatalf("F4: glyph-lead legend mark %d (%q) undecodable via the mini-legend closed table", entry.Mark, entry.ZH)
		}
	}
}

// TestA2SelfRowMainLineFoldBudget — dual-review F8 pin: the SELF row's MAIN
// line joins the same fold width budget as every seat row (UX-2① symmetry) —
// an over-budget self line folds at the whitelist into a ⤷ continuation under
// the self lead, every physical line holds the row cap, content is byte-whole,
// and the fold stamps the shared ⤷ mark (legend/mini-key wiring).
func TestA2SelfRowMainLineFoldBudget(t *testing.T) {
	marks := &runtimeTraceProjMarkSet{}
	row := runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowSelf, HasData: true, marks: marks,
		Node: types.TraceCausalProjectionNode{
			Subject:   "sysevent_store-47924",
			Object:    "BackgroundTaskDispatcherWorkerPoolVeryLongPeerThreadName-00042-987654321",
			TypeToken: "io_latency", Predicate: "critical_blocking",
			ImpactMS: 12.345, EffectiveImpactMS: 12.345,
		},
	}
	lines := runtimeTraceProjSelfRowLines(row, 100.0, true)
	if len(lines) < 2 {
		t.Fatalf("F8: the over-budget self main line must fold:\n%s", strings.Join(lines, "\n"))
	}
	for _, l := range lines {
		if w := runewidth.StringWidth(l); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("F8: folded self line still over budget (w=%d): %q", w, l)
		}
	}
	if !strings.Contains(lines[1], "⤷ ") {
		t.Fatalf("F8: the self continuation must open with the ⤷ marker: %q", lines[1])
	}
	if !marks.has(runtimeTraceProjMarkSeatRowWrapCont) {
		t.Fatalf("F8: the self fold must stamp the shared ⤷ mark")
	}
	// Byte-wholeness across the fold (marker + boundary spaces strip).
	var joined strings.Builder
	for i, l := range lines {
		part := l
		if i > 0 {
			part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "⤷"))
		}
		joined.WriteString(part)
	}
	for _, want := range []string{"BackgroundTaskDispatcherWorkerPoolVeryLongPeerThreadName-00042-987654321", "12.345ms"} {
		if !strings.Contains(joined.String(), want) {
			t.Fatalf("F8: fold lost content %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	// 负臂: a fits-in-budget self row stays one line, no mark.
	small := runtimeTraceProjTreeRow{
		Kind: runtimeTraceProjTreeRowSelf, HasData: true, marks: &runtimeTraceProjMarkSet{},
		Node: types.TraceCausalProjectionNode{
			Subject: "ui-1", Object: "worker-7",
			TypeToken: "io_latency", Predicate: "critical_blocking",
			ImpactMS: 1.0, EffectiveImpactMS: 1.0,
		},
	}
	if got := runtimeTraceProjSelfRowLines(small, 100.0, true); len(got) != 1 {
		t.Fatalf("F8 负臂: an in-budget self row must stay one line: %q", got)
	}
}

// TestA2BadgeCompoundTokenSpace — SMALL3-1 件1 (§29.196② settle of the A2R-3
// filing, 2026-07-21), the TestA2BadgeGlyphGeometry family's compound-token
// arm: the two prose-pointer mint points (runtimeTraceProjFourStateSemantic
// Pointer / SupplyPointer) compose 「badge+[E#]」 WITH exactly one space after
// the badge glyph (「➋ [E15]」, never the glyph-adjacent 「➋[E15]」) — the
// §29.191 one-space rule now covers the prose face too; no exempt compound
// token exists.
func TestA2BadgeCompoundTokenSpace(t *testing.T) {
	target := "jitter-17284"
	model := runtimeTraceProjTreeModel{
		Target: target,
		TreeRows: []runtimeTraceProjTreeRow{
			{
				Kind: runtimeTraceProjTreeRowSemantic, HasData: true,
				Badge: 2, EvidenceTag: "E15",
				Node: types.TraceCausalProjectionNode{
					Subject: target, EffectiveImpactMS: 2.388,
				},
			},
			{
				Kind: runtimeTraceProjTreeRowChain, HasData: true,
				Badge: 1, EvidenceTag: "E4",
				Node: types.TraceCausalProjectionNode{
					Subject: target, StateKind: "running",
					SupplyFoldComputed: true, SupplyFoldDeficitMS: 2.127,
				},
			},
		},
	}
	tag, count, best := runtimeTraceProjFourStateSemanticPointer(model)
	if tag != " ➋ [E15]" || count != 1 || best != 2.388 {
		t.Fatalf("件1: semantic pointer must mint the one-space compound 「 ➋ [E15]」, got %q (count=%d best=%.3f)", tag, count, best)
	}
	value, supplyTag, ok := runtimeTraceProjFourStateSupplyPointer(model)
	if !ok || supplyTag != " ➊ [E4]" || value != 2.127 {
		t.Fatalf("件1: supply pointer must mint the one-space compound 「 ➊ [E4]」, got %q (value=%.3f ok=%v)", supplyTag, value, ok)
	}
	// Geometry arm shared with 件10(a): in both minted tokens every badge
	// glyph's immediate successor is exactly one space.
	for _, minted := range []string{tag, supplyTag} {
		for i, r := range minted {
			if r != '➊' && r != '➋' && r != '➌' && r != '➍' && r != '➎' {
				continue
			}
			rest := minted[i+len(string(r)):]
			if !strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "  ") {
				t.Fatalf("件1: badge must be followed by exactly one space in %q", minted)
			}
		}
	}
}

// TestA2FlatLaneCaliberSideSinksToTail — SMALL3-1 件2 (§29.196③ settle of the
// A2R-5 filing), the TestA2FlatLaneValueSorted family's ⌗ arm: a caliber-side
// row in the flat on-chain lane sinks to the tail UNCONDITIONALLY — its
// non-wall-clock numeral (live witness donghu_17284 window 13762.845..
// 13762.900: 1.759ms → ⌗1.332 → 0.060ms) never competes in the value ordering
// (异单位禁比), even when it prints LARGER than every wall-clock value; the
// commitment sentence carries the ruled 「(计数当量行恒末)」 half-sentence on
// both faces.
func TestA2FlatLaneCaliberSideSinksToTail(t *testing.T) {
	caliber := elimChainNode("E-cal", "io-scorer-9", "block_io_by_inode", "", 7, 99.0, 400)
	caliber.Tier = types.TraceCausalTierCaliberSide
	caliber.Predicate = "root_cause_caliber_side"
	caliber.Rank = 0
	caliber.StateKind = ""
	projection := types.TraceCausalProjection{
		WakeupPath:              []string{"ui-1"},
		RootCauseFamilyObserved: true,
		OnChainCauses: []types.TraceCausalProjectionNode{
			elimChainNode("E-small", "worker-2", "runnable_wait", "runnable", 4, 3.0, 100),
			caliber,
			elimChainNode("E-big", "worker-3", "runnable_wait", "runnable", 5, 9.0, 200),
		},
	}
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		big := strings.Index(fence, "worker-3")
		small := strings.Index(fence, "worker-2")
		cal := strings.Index(fence, "io-scorer-9")
		if big < 0 || small < 0 || cal < 0 || !(big < small && small < cal) {
			t.Fatalf("件2 zh=%v: the ⌗ row must sink below every value row (9.0 at %d, 3.0 at %d, ⌗99.0 at %d):\n%s",
				zh, big, small, cal, fence)
		}
		half := "(计数当量行恒末)"
		line := "- 链上平铺席按显示值降序(仅平铺席;树位挂靠行按链结构)(计数当量行恒末)"
		if !zh {
			half = "(count-equivalent last)"
			line = "- flat on-chain seats: displayed value desc (attached rows follow the chain)(count-equivalent last)"
		}
		if !strings.Contains(fence, line) {
			t.Fatalf("件2 zh=%v: the commitment sentence must carry the %q half-sentence:\n%s", zh, half, fence)
		}
		if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("件2 zh=%v: tail-form commitment line over the row budget (w=%d)", zh, w)
		}
	}
	// 负臂 (词条-图例双向): a ⌗-less flat lane keeps the A2 sentence
	// byte-identically — the legend-taught 计数当量 term never renders
	// untaught.
	bare := projection
	bare.OnChainCauses = []types.TraceCausalProjectionNode{
		elimChainNode("E-small", "worker-2", "runnable_wait", "runnable", 4, 3.0, 100),
		elimChainNode("E-big", "worker-3", "runnable_wait", "runnable", 5, 9.0, 200),
	}
	bareModel := buildRuntimeTraceProjTreeModel(bare, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	bareFence := runtimeTraceProjTreeFence(bareModel, true)
	if strings.Contains(bareFence, "计数当量行恒末") {
		t.Fatalf("件2 负臂: a ⌗-less lane must not carry the half-sentence:\n%s", bareFence)
	}
}
