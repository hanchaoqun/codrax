package tool

// answer_document_projection_omgclean_test.go — OMGCLEAN-1 batch pins
// (§29.175 user ruling ② + .1-.17 补记 family, 2026-07-20): the ◎ five-zone
// re-layout, the 判词文法 verdict grammar, the — 辅助 — two-column grammar,
// the 件10 zero-internal-terms sweep, and the value-channel no-touch guard.
//
// MUTATION self-checks (cp-copy recovery only):
//   M-件1  flipping the tail word back to 方向未定/复合 reds the rename pins
//          (elim_test.go promise pins + the revisit76 probe);
//   M-件2  see internal/types/trace_causal_projection_omgclean_test.go;
//   M-件7  hoisting a MIXED section's anchor reds the xlane3 hoist pins;
//   M-件9  breaking the label padding reds TestOmgcleanAuxTwoColumnAligned;
//   M-件11 re-rooting a verdict word (换词根) reds TestOmgcleanVerdictWordTable;
//   M-rider3 see internal/tracequery/business_span_mention_test.go.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// TestOmgcleanVerdictWordTable — 件11 终版全表映射臂 (§29.175.17 verbatim
// table): one family one head-verdict root, ·限定 suffixes, zh/EN 同批. The
// mapping is display-only over the typed token (registry untouched).
func TestOmgcleanVerdictWordTable(t *testing.T) {
	node := types.TraceCausalProjectionNode{}
	for _, tc := range []struct {
		token string
		zh    string
		en    string
	}{
		{"scheduler_latency", "调度延迟", "scheduling latency"},
		{"runnable_wait", "调度延迟", "scheduling latency"},
		{"runnable", "调度延迟", "scheduling latency"},
		{"fragmented_runnable_wait", "调度延迟·碎片化", "scheduling latency·fragmented"},
		{"cpu_pressure", "调度延迟·CPU竞争", "scheduling latency·CPU contention"},
		{"io_wait", "IO阻塞", "IO blocking"},
		{"d_state_or_io_wait", "IO阻塞·不可中断(原因未证)", "IO blocking·uninterruptible (cause unproven)"},
		{"io_latency", "IO阻塞·设备延迟", "IO blocking·device latency"},
	} {
		zh, ok := runtimeTraceProjElimVerdictTokenWord(node, tc.token, true)
		if !ok || zh != tc.zh {
			t.Fatalf("件11 全表臂: %s → %q (want %q, ok=%v)", tc.token, zh, tc.zh, ok)
		}
		en, ok := runtimeTraceProjElimVerdictTokenWord(node, tc.token, false)
		if !ok || en != tc.en {
			t.Fatalf("件11 全表臂 EN: %s → %q (want %q, ok=%v)", tc.token, en, tc.en, ok)
		}
	}
	// 文法突变负臂: every mapped word keeps its ruled ROOT (换词根形=红).
	for _, tc := range []struct{ token, root string }{
		{"fragmented_runnable_wait", "调度延迟"},
		{"cpu_pressure", "调度延迟"},
		{"d_state_or_io_wait", "IO阻塞"},
		{"io_latency", "IO阻塞"},
	} {
		word, _ := runtimeTraceProjElimVerdictTokenWord(node, tc.token, true)
		if !strings.HasPrefix(word, tc.root) {
			t.Fatalf("件11 文法臂: %s must keep the %s root, got %q", tc.token, tc.root, word)
		}
	}
	// sleep 负臂 (§29.175.16: sleep has no generic diagnosis reading — the
	// family keeps its raw word by absence from the table).
	for _, token := range []string{"sleep_wait", "sleep", "fragmented_sleep_wait", "s_sleep"} {
		if word, ok := runtimeTraceProjElimVerdictTokenWord(node, token, true); ok {
			t.Fatalf("件11 sleep 负臂: %s must stay unmapped, got %q", token, word)
		}
	}
	// binder/优先级反转/语义类 维持 by absence.
	for _, token := range []string{"binder_wait", "priority_inversion_candidate", "class_verification"} {
		if word, ok := runtimeTraceProjElimVerdictTokenWord(node, token, true); ok {
			t.Fatalf("件11 维持臂: %s must stay unmapped, got %q", token, word)
		}
	}
	// refined-D 臂 — EVOLUTION RECORD (RULE3-1 件7, §29.182① verbatim
	// 「①已证非IO D 席铸独立判词「不可中断等待·非IO已证」(独立词根合文法=另一
	// 族病;榜面零裸态词,图例承诺变真;EN 同批)」, 2026-07-21): the OMGCLEAN-1
	// ok=false bare-word fallback is RETIRED — the typed non-IO proof now
	// mints its own verdict root on both faces.
	refined := types.TraceCausalProjectionNode{DStateRefinedNonIO: true}
	if word, ok := runtimeTraceProjElimVerdictTokenWord(refined, "d_state_or_io_wait", true); !ok || word != "不可中断等待·非IO已证" {
		t.Fatalf("件7 refined 臂: the proven non-IO seat must wear its own verdict root, got %q (ok=%v)", word, ok)
	}
	if word, ok := runtimeTraceProjElimVerdictTokenWord(refined, "d_state_or_io_wait", false); !ok || word != "uninterruptible wait·proven non-IO" {
		t.Fatalf("件7 refined 臂 EN: got %q (ok=%v)", word, ok)
	}
	// 件7 文法臂: the refined word must NOT wear the IO阻塞 root (独立词根).
	if word, _ := runtimeTraceProjElimVerdictTokenWord(refined, "d_state_or_io_wait", true); strings.HasPrefix(word, "IO阻塞") {
		t.Fatalf("件7 独立词根臂: the proven non-IO verdict must not ride the IO阻塞 root, got %q", word)
	}
	// running 折算门: an undiscounted running row keeps its raw word (absence
	// never wears a frequency claim); the discounted fold maps to the
	// 低频运行 root (·折算 completes on the caliber slot).
	if word, ok := runtimeTraceProjElimVerdictTokenWord(node, "running", true); ok {
		t.Fatalf("件11 running 门: undiscounted running must stay unmapped, got %q", word)
	}
	discounted := types.TraceCausalProjectionNode{EffectiveImpactMS: 9.0, ImpactMS: 12.0}
	if word, ok := runtimeTraceProjElimVerdictTokenWord(discounted, "running", true); !ok || word != "低频运行" {
		t.Fatalf("件11 折算席: the discounted running fold must map to 低频运行, got %q (ok=%v)", word, ok)
	}
}

// TestOmgcleanVerdictBoardFaceVsTreeFace — 树面原词臂 (§29.175.17: 素状态词退
// ◎ 诊断面,树状态面/明细保留): the SAME runnable seat renders 调度延迟 on the
// ◎ board face while the tree face keeps the raw runnable state word.
func TestOmgcleanVerdictBoardFaceVsTreeFace(t *testing.T) {
	projection := elimBoardProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tree := runtimeTraceProjTreeFence(model, true)
	elim := runtimeTraceProjElimOverviewFence(projection, model, true)
	if !strings.Contains(elim, "调度延迟") {
		t.Fatalf("◎ 诊断面 must speak the verdict word:\n%s", elim)
	}
	if !strings.Contains(tree, "runnable") {
		t.Fatalf("树状态面 must keep the raw runnable word:\n%s", tree)
	}
	if !model.Marks.has(runtimeTraceProjMarkElimVerdictGrammar) {
		t.Fatalf("the verdict grammar legend mark must light at the ◎ emission")
	}
}

// TestOmgcleanZoneOrderAndBlankSeparation — §29.175.6 区域序终裁: ⛓ 方向节区
// → ◈ 业务线索 → ◇ 邻近 → ▒ 背景 → — 辅助 —, zones separated by blank lines.
func TestOmgcleanZoneOrderAndBlankSeparation(t *testing.T) {
	projection := elimBoardProjection()
	projection.BusinessSpanMentions = spanvisMentions()
	projection.BusinessSpanMentionOmitted = 2
	_, fence := elimRenderOverview(t, projection, true)
	idx := func(marker string) int { return strings.Index(fence, marker) }
	section := idx("▸ ")
	business := idx("\n\n◈ ")
	adjacent := idx("\n\n◇ 邻近(")
	background := idx("\n\n▒ ")
	aux := idx("\n\n— 辅助 · 对账与另账(不占序数) —")
	if section < 0 || business < 0 || adjacent < 0 || background < 0 || aux < 0 {
		t.Fatalf("§29.175.6: all five zones must render blank-separated (▸=%d ◈=%d ◇=%d ▒=%d 辅助=%d):\n%s",
			section, business, adjacent, background, aux, fence)
	}
	if !(section < business && business < adjacent && adjacent < background && background < aux) {
		t.Fatalf("§29.175.6 区域序: ▸ < ◈ < ◇ < ▒ < 辅助 must hold (▸=%d ◈=%d ◇=%d ▒=%d 辅助=%d):\n%s",
			section, business, adjacent, background, aux, fence)
	}
}

// TestOmgcleanAuxTwoColumnAligned — 件9 两列对齐 pin (§29.175.8): every aux
// row pads its label to ONE shared display width — the content column starts
// at the same cell on every `· ` row.
func TestOmgcleanAuxTwoColumnAligned(t *testing.T) {
	// The elimv2 direction board + a cut seat fields aux labels of DIFFERENT
	// display widths (∩ 重叠对 8 / 守恒 4 / 未入榜 6 cells) — the alignment
	// claim is only falsifiable across differing widths (a same-width roster
	// aligns for free).
	projection := elimv2DirectionBoardProjection()
	projection.OnChainCauses = append(projection.OnChainCauses,
		elimv2DirectionNode("Ev2-cut2", "worker-Y-33", "runnable_wait", "runnable", 6, 0.4, 800, "scheduling_supply", 1000.180, 1000.181))
	_, fence := elimRenderOverview(t, projection, true)
	inAux := false
	contentCol := -1
	rows := 0
	for _, line := range strings.Split(fence, "\n") {
		if line == "— 辅助 · 对账与另账(不占序数) —" {
			inAux = true
			continue
		}
		if !inAux || !strings.HasPrefix(line, "· ") {
			continue
		}
		rows++
		// content starts after the label padding: locate the first run of ≥2
		// spaces past the "· " lead and measure its END in display cells.
		rest := line[len("· "):]
		at := strings.Index(rest, "  ")
		if at < 0 {
			t.Fatalf("件9: aux row without a label/content boundary: %q", line)
		}
		content := strings.TrimLeft(rest[at:], " ")
		col := runewidth.StringWidth(line[:len(line)-len(content)])
		if contentCol == -1 {
			contentCol = col
		} else if col != contentCol {
			t.Fatalf("件9 两列对齐: content column %d != %d on %q:\n%s", col, contentCol, line, fence)
		}
	}
	if rows < 2 {
		t.Fatalf("fixture: expected ≥2 aux rows, got %d:\n%s", rows, fence)
	}
}

// TestOmgcleanAuxGroupOrderAndGlyphPolicy — 件9 组序臂 (§29.175.8: 对账组先,
// 另账组后) + §29.175.13 图标策略 (∩ 在场 / ⌗ 缺席) + §29.175.11 三分规则.
func TestOmgcleanAuxGroupOrderAndGlyphPolicy(t *testing.T) {
	projection := elimv2DirectionBoardProjection()
	// Force a value-cut chain seat so the 另账组 未入榜 row renders beside
	// the 对账组 rows (the base fixture fits TOP5 exactly).
	projection.OnChainCauses = append(projection.OnChainCauses,
		elimv2DirectionNode("Ev2-cut", "worker-X-22", "runnable_wait", "runnable", 6, 0.5, 700, "scheduling_supply", 1000.170, 1000.171))
	_, fence := elimRenderOverview(t, projection, true)
	auxAt := strings.Index(fence, "— 辅助 · 对账与另账(不占序数) —")
	if auxAt < 0 {
		t.Fatalf("the aux zone must render the 定稿 announcing head:\n%s", fence)
	}
	aux := fence[auxAt:]
	pair := strings.Index(aux, "· ∩ 重叠对")
	conservation := strings.Index(aux, "· 守恒")
	unranked := strings.Index(aux, "· 未入榜")
	if pair < 0 || conservation < 0 || unranked < 0 {
		t.Fatalf("件9: 对账组 (∩/守恒) and 另账组 (未入榜) rows must render:\n%s", aux)
	}
	if !(pair < conservation && conservation < unranked) {
		t.Fatalf("件9 组序: 对账组 (∩ %d, 守恒 %d) must precede 另账组 (未入榜 %d):\n%s",
			pair, conservation, unranked, aux)
	}
	// §29.175.13: only the functional ∩ glyph survives on labels; ⌗ is
	// stripped from the ◎ face entirely.
	if strings.Contains(fence, "⌗") {
		t.Fatalf("§29.175.13: the ⌗ glyph must stay off the ◎ face:\n%s", fence)
	}
	// §29.175.11 行首三分规则: value rows lead with the right-aligned value,
	// aux list rows with `· `, tail-note rows indent without a dot; nothing
	// else (two-level indentation, no third level). 双复核修复 件3 (冷读 CR3,
	// 2026-07-21): the width-governor continuation whitelist arm is DELETED —
	// a wrapped aux continuation is a real violation (禁续行; the legend
	// promises sibling-split-not-wrap), so it now lands in the value-row arm
	// and reds unless it happens to be a value row.
	for _, line := range strings.Split(fence, "\n") {
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		if strings.HasPrefix(line, "— 辅助 ") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "· "): // aux list row
		case strings.HasPrefix(line, "  另有") || strings.HasPrefix(line, "  …"): // tail note
		case strings.HasPrefix(line, " "): // value row (right-aligned %9.3f)
			if !strings.Contains(line, "ms ") {
				t.Fatalf("三分规则: an indented non-value line outside the closed forms: %q\n%s", line, fence)
			}
		default: // zone/section heads at column 0
		}
	}
}

// TestOmgcleanUnrankedSiblingRows — 件9补四 双行形臂 (§29.175.14): the 未入榜
// and 未入榜最大 accounts render as two SAME-LEVEL sibling rows (never a
// wrapped continuation of one row).
func TestOmgcleanUnrankedSiblingRows(t *testing.T) {
	projection := elimGapCutBoardProjection()
	projection.BackgroundCauses = append(projection.BackgroundCauses, types.TraceCausalProjectionNode{
		EvidenceID: "E-refused", Subject: "Binder:43397_19-23088",
		Predicate: "background_pressure", Object: "runnable", StateKind: "runnable",
		ChainRelevance: "background", LineStart: 900, LineEnd: 950,
	})
	projection.GatedCompositeEdgeShareDisclosures = []types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure{
		{Subject: "Binder:43397_19-23088", PreMS: 13.982, PostMS: 0.020, AccountMS: 14.002,
			AnchorTS: 34579.555890, Via: "direct", SeatPublished: false},
	}
	_, fence := elimRenderOverview(t, projection, true)
	if !strings.Contains(fence, "\n· 未入榜 ") || !strings.Contains(fence, "\n· 未入榜最大") {
		t.Fatalf("件9补四: 未入榜 and 未入榜最大 must render as sibling `· ` rows:\n%s", fence)
	}
	// 双复核修复 件2 (冷读 CR2, 2026-07-21): the row value is the seat's OWN
	// account (AccountMS, 定稿 14.002 形) and the pre/post identity 括注 sits
	// right after it — the former pin froze the pre-edge share (13.982) as the
	// row value with the typed AccountMS carried unused.
	if !strings.Contains(fence, "Binder:43397_19-23088 14.002ms(唤醒边前 13.982 + 边后 0.020)· 按口径不拆段入榜") {
		t.Fatalf("件2 定稿词形: the 未入榜最大 row must carry AccountMS + the inline identity 括注:\n%s", fence)
	}
}

// TestOmgcleanInternalTermSweep — 件10 全仓用户面 grep 闭合臂 (§29.175.12:
// R3/R4/候选池 零命中于渲染字面): the whole rendered report (tree + ◎ +
// legend/lead) on representative boards carries no rule numbers and no
// internal-pool words, zh and EN alike.
func TestOmgcleanInternalTermSweep(t *testing.T) {
	for _, zh := range []bool{true, false} {
		projection := elimBoardProjection()
		projection.GatedCompositeEdgeShareDisclosures = []types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure{
			{Subject: "shadowhook-64305", PreMS: 3.5, PostMS: 1.5, AccountMS: 5.0,
				AnchorTS: 6.006, Via: "direct", SeatPublished: false},
		}
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		tree := runtimeTraceProjTreeFence(model, zh)
		elim := runtimeTraceProjElimOverviewFence(projection, model, zh)
		lang := "zh"
		if !zh {
			lang = "en"
		}
		lead := runtimeTraceProjLeadText(projection, model, lang, zh)
		surface := tree + "\n" + elim + "\n" + lead
		// 双复核修复 件5 (对抗 CR-2/CR-11, 2026-07-21): 「§」 (ledger section
		// numbers are internal vocabulary — the legend self-minted 「§29.175.6」
		// past the old list) and 「拒转」 join the banned closed set.
		for _, banned := range []string{"R3 ", "R4", "候选池", "R3边", "R3 边", "§", "拒转"} {
			if strings.Contains(surface, banned) {
				at := strings.Index(surface, banned)
				lo := at - 40
				if lo < 0 {
					lo = 0
				}
				t.Fatalf("件10 (%s): internal term %q on a user face near ...%s...", lang, banned, surface[lo:at+len(banned)])
			}
		}
	}
}

// TestOmgcleanValueChannelUntouchedByRender — 硬纪律1 guard. 双复核修复 件11
// (对抗 CR-5, spec 原令 strip-then-DeepEqual, 2026-07-21): the former 4-field
// subset compare could miss a mutation on ANY other node field — the guard is
// now a FULL-entry reflect.DeepEqual after stripping only the render-owned
// marks collector pointer (the one field the renderer legitimately stamps).
// A second render must additionally be byte-identical (deterministic display).
func TestOmgcleanValueChannelUntouchedByRender(t *testing.T) {
	projection := elimBoardProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	runtimeTraceProjTreeFence(model, true)
	strip := func(entries []runtimeTraceProjElimEntry) []runtimeTraceProjElimEntry {
		out := append([]runtimeTraceProjElimEntry(nil), entries...)
		for i := range out {
			out[i].row.marks = nil
		}
		return out
	}
	before := strip(runtimeTraceProjElimBoard(model))
	fence1 := runtimeTraceProjElimOverviewFence(projection, model, true)
	after := strip(runtimeTraceProjElimBoard(model))
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("the render must not touch ANY board entry field (strip-then-DeepEqual):\nbefore=%+v\nafter=%+v", before, after)
	}
	fence2 := runtimeTraceProjElimOverviewFence(projection, model, true)
	if fence1 != fence2 {
		t.Fatalf("the ◎ render must be deterministic:\n%s\nvs\n%s", fence1, fence2)
	}
}

// TestOmgcleanHoistMixedSectionKeepsPerRow — 件7 混板负臂 (§29.133 件G
// preserved): a section whose members carry DIFFERENT typed board targets
// hoists nothing — every row keeps its own ·板锚 chip (absence never hoists a
// claim). MUTATION self-check: making the hoist unconditional (first target
// wins) reds this pin.
func TestOmgcleanHoistMixedSectionKeepsPerRow(t *testing.T) {
	mk := func(id, subject, board string, rank int, eff float64, line int) types.TraceCausalProjectionNode {
		node := elimChainNode(id, subject, "runnable_wait", "runnable", rank, eff, line)
		node.FixDirection = "scheduling_supply"
		node.RankBoardTarget = board
		return node
	}
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"waker-1", "board-A-1"},
		WindowStartTs:           1000.000, WindowEndTs: 1000.200,
		OnChainCauses: []types.TraceCausalProjectionNode{
			mk("E-a", "worker-a-1", "board-A-1", 1, 6.0, 100),
			mk("E-b", "worker-b-2", "board-B-2", 2, 4.0, 200),
		},
	}
	_, fence := elimRenderOverview(t, projection, true)
	if !strings.Contains(fence, "尺=各板目标线程") {
		t.Fatalf("fixture: the mixed-board ruler must fire:\n%s", fence)
	}
	for _, line := range strings.Split(fence, "\n") {
		if strings.HasPrefix(line, "▸ ") && strings.Contains(line, "·板锚") {
			t.Fatalf("件7 混板负臂: a mixed section must not hoist an anchor:\n%s", fence)
		}
	}
	if !strings.Contains(fence, "·板锚 board-A-1 [E") || !strings.Contains(fence, "·板锚 board-B-2 [E") {
		t.Fatalf("件7 混板负臂: per-row anchors must survive on the mixed section:\n%s", fence)
	}
}

// --- 双复核修复轮 pins (2026-07-21) ------------------------------------------

// TestOmgcleanMergeDirectionAdoptionReachesBoard — 双复核 件10 (对抗 CR-4
// 活体 pin 缺席实证: reverting the aggregate.go adoption block kept the whole
// tool package green). Board-construction-level pin: raw observation records
// (a direction-BARE chain-view survivor + a direction-stamped rank-supplying
// seat + a bare peer, the runnable_2 E18(+8) carriage shape) run the REAL
// compile path (record → projection → R2 ×N merge → model → render), and the
// merged seat must surface INSIDE its adopted ▸ direction section with the
// tree face wearing the 修向 word. MUTATION self-check: no-op the 件2
// empty-slot adoption block (aggregate.go) — the seat falls back into the
// 其他方向 tail and this pin reds while every fixture-projection pin stays
// green (that gap is exactly what this pin closes).
func TestOmgcleanMergeDirectionAdoptionReachesBoard(t *testing.T) {
	mk := func(id, predicate, value string, notes []string, lineStart, lineEnd int) types.ObservationRecord {
		return types.ObservationRecord{
			ID:              id,
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       predicate,
			ClaimKey:        predicate + ":running:" + id,
			Subject:         "worker-77",
			Object:          "running",
			Value:           value,
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
			RichNotes:       notes,
			Confidence:      0.8,
		}
	}
	// Distinct root_cause_* predicates keep the three census publications
	// alive through the classified dedupe (same-predicate zero-board twins
	// fold there before R2 ever runs); the LARGEST row is direction- and
	// rank-bare, so the classified order makes it the group-first survivor —
	// exactly the runnable_2 carriage shape.
	records := []types.ObservationRecord{
		// Direction-bare census survivor (the group-first seed).
		mk("omg-seed", "root_cause_context", "10.000", []string{
			"tier=secondary", "impact_ms=10.000", "cumulative_impact_ms=10.000",
			"effective_impact_ms=10.000", "chain_relevance=on_chain",
			"causality=on_wakeup_chain", "dominant_state=running"}, 100, 200),
		// Direction-stamped rank-supplying seat (board/window/direction one
		// identity — the adoption donor; NON-first by value order).
		mk("omg-rank", "root_cause_secondary", "4.000", []string{
			"tier=secondary", "rank=2", "impact_ms=4.000", "cumulative_impact_ms=4.000",
			"effective_impact_ms=4.000", "chain_relevance=on_chain",
			"causality=on_wakeup_chain", "dominant_state=running",
			"fix_direction=frequency_thermal"}, 300, 400),
		// Bare peer — lifts the group to the ≥3 R2 threshold.
		mk("omg-peer", "root_cause_tertiary", "2.684", []string{
			"tier=secondary", "impact_ms=2.684", "cumulative_impact_ms=2.684",
			"effective_impact_ms=2.684", "chain_relevance=on_chain",
			"causality=on_wakeup_chain", "dominant_state=running"}, 500, 600),
	}
	projection := types.TraceCausalProjectionFromObservationRecords(records)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tree := runtimeTraceProjTreeFence(model, true)
	elim := runtimeTraceProjElimOverviewFence(projection, model, true)
	if elim == "" {
		t.Fatalf("fixture: the ◎ fence must render (rank family observed)")
	}
	// The compile path must actually have merged the group (×N carriage —
	// otherwise this pin would pass on unmerged rows and prove nothing; the
	// merged row wears the (+2) evidence fold and the 3次 range face).
	if !strings.Contains(tree, "(+2)]") || !strings.Contains(tree, "3次(") {
		t.Fatalf("fixture: the record compile must R2-merge the three same-kind rows:\n%s", tree)
	}
	// ◎ face: the merged seat sits INSIDE the adopted direction section; the
	// direction-less tail section never renders (the merged seat was the only
	// tail candidate).
	headAt := strings.Index(elim, "▸ 频率与热治理")
	rowAt := strings.Index(elim, "worker-77")
	if headAt < 0 || rowAt < headAt {
		t.Fatalf("件10: the merged seat must render inside the adopted ▸ 频率与热治理 section (head=%d row=%d):\n%s", headAt, rowAt, elim)
	}
	if strings.Contains(elim, "▸ 其他方向") {
		t.Fatalf("件10: the adopted direction must empty the 其他方向 tail:\n%s", elim)
	}
	// Tree face: the merged row wears the 修向 word (attribute-axis surface).
	if !strings.Contains(tree, "修向 频率与热治理") {
		t.Fatalf("件10: the tree face must wear the adopted 修向 word:\n%s", tree)
	}
}

// TestOmgcleanAuxNoContinuationEN — 双复核 件3 (冷读 CR3/对抗 CR-9): the EN
// auxiliary zone bans width-governor continuations — every aux line is a
// same-level `· ` row or a closed-form tail note, and every line fits the
// 100-cell row budget (the legend promises sibling-split-not-wrap; a wrapped
// continuation is the violation this pin keeps red). The fixture fields the
// wrap-prone families: unranked max (disclosure), caliber sidebar (count
// row) and the semantic-leads census.
func TestOmgcleanAuxNoContinuationEN(t *testing.T) {
	projection := elimBoardProjection()
	projection.GatedCompositeEdgeShareDisclosures = []types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure{
		{Subject: "Binder:43397_19-23088", PreMS: 13.982, PostMS: 0.020, AccountMS: 14.002,
			AnchorTS: 34579.555890, Via: "direct", SeatPublished: false},
	}
	_, fence := elimRenderOverview(t, projection, false)
	auxAt := strings.Index(fence, "— auxiliary · reconciliation & side accounts (no ordinal) —")
	if auxAt < 0 {
		t.Fatalf("件1 EN head: the aux zone must announce the two groups:\n%s", fence)
	}
	rows := 0
	for _, line := range strings.Split(fence[auxAt:], "\n") {
		if line == "" || strings.HasPrefix(line, "```") {
			break // zone ends at the fence tail
		}
		if strings.HasPrefix(line, "— auxiliary") {
			continue
		}
		if !strings.HasPrefix(line, "· ") {
			t.Fatalf("件3 禁续行: every EN aux zone line is a same-level `· ` row, got %q:\n%s", line, fence)
		}
		rows++
		if w := runewidth.StringWidth(line); w > 100 {
			t.Fatalf("件3: aux line exceeds the 100-cell budget (%d): %q", w, line)
		}
	}
	if rows < 2 {
		t.Fatalf("fixture: expected ≥2 EN aux rows, got %d:\n%s", rows, fence)
	}
	// The 件2 EN row form rides the same fixture (AccountMS + inline identity).
	if !strings.Contains(fence, "Binder:43397_19-23088 14.002ms (pre 13.982 + post 0.020) · kept whole per caliber") {
		t.Fatalf("件2 EN 词形: the unranked-max row must carry AccountMS + the inline identity:\n%s", fence)
	}
}

// TestOmgcleanBackgroundDegenerateSingleLine — 双复核 件9 (冷读 CR8 双计形):
// a background stanza whose every row lacks a displayable in-window projected
// value renders ONE merged head/tail line — never the head-plus-"另有 N 行"
// double-count form (donghu_2955 witness: the lone ⌗ caliber-side background
// row).
func TestOmgcleanBackgroundDegenerateSingleLine(t *testing.T) {
	base := elimBoardProjection()
	for _, zh := range []bool{true, false} {
		projection := base
		projection.BackgroundCauses = []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "bg-cal",
			Subject: ".ugc.aweme.lite-17267", Predicate: "root_cause_caliber_side",
			Object: "page_cache_churn", TypeToken: "page_cache_churn",
			ChainRelevance: "background", ImpactMS: 81.616, CumulativeImpactMS: 81.616,
			Tier: "caliber_side", Confidence: 0.72, LineStart: 200, LineEnd: 260,
		}}
		_, fence := elimRenderOverview(t, projection, zh)
		want := "▒ 背景压力 1 行(无窗内投影值)见背景段"
		banned := "另有 1 行见背景段"
		if !zh {
			want = "▒ 1 background-pressure row(s) (no in-window projected value) — see the background stanza"
			banned = "1 more row(s) — see the background stanza"
		}
		if !strings.Contains(fence, want) {
			t.Fatalf("件9 退化臂 (zh=%v): the degenerate zone must collapse to %q:\n%s", zh, want, fence)
		}
		if strings.Contains(fence, banned) {
			t.Fatalf("件9 退化臂 (zh=%v): the double-count tail must not render:\n%s", zh, fence)
		}
	}
}

// TestOmgcleanMentionNameFaceKeepsDistinguishingPrefix — 双复核 件12 (冷读
// CR9 截断名撞脸负臂): two DIFFERENT over-budget span names that differ only
// in their MIDDLE (past the old fixed 12-cell head keep, before the kept
// tail) must render DISTINCT ◈ faces — the mention cut keeps the
// distinguishing head prefix and floors the tail at the RUN2FIX-A 6-cell
// identity stub.
func TestOmgcleanMentionNameFaceKeepsDistinguishingPrefix(t *testing.T) {
	a := "H:CommitLayer name:AAAAWindowSurface SurfaceNode8005819039744,zorder: 2147483392,"
	b := "H:CommitLayer name:BBBBWindowSurface SurfaceNode8005819039744,zorder: 2147483392,"
	siblings := []types.TraceCausalProjectionBusinessSpanMention{{Name: a}, {Name: b}}
	// The DEFAULT tail-keeping cut collides on this pair (identical 12-cell
	// head + identical kept tail) — the pre-fix production face.
	if fa, _ := runtimeTraceProjFamilySpanTopNameFace(a, runtimeTraceProjBusinessSpanMentionNameBudget); true {
		if fb, _ := runtimeTraceProjFamilySpanTopNameFace(b, runtimeTraceProjBusinessSpanMentionNameBudget); fa != fb {
			t.Fatalf("fixture: the default cut must collide (pre-fix shape): %q vs %q", fa, fb)
		}
	}
	faceA := runtimeTraceProjBusinessSpanMentionNameFace(a, siblings)
	faceB := runtimeTraceProjBusinessSpanMentionNameFace(b, siblings)
	if faceA == faceB {
		t.Fatalf("件12 撞脸负臂: distinct names must not share one face: %q", faceA)
	}
	for _, face := range []string{faceA, faceB} {
		if !strings.Contains(face, "…") {
			t.Fatalf("fixture: the face must actually truncate: %q", face)
		}
		if w := runewidth.StringWidth(face); w > runtimeTraceProjBusinessSpanMentionNameBudget {
			t.Fatalf("件12: face exceeds the name budget (%d): %q", w, face)
		}
	}
	// The distinguishing middle must SURVIVE on the face (head-prefix keep).
	if !strings.Contains(faceA, "AAAA") || !strings.Contains(faceB, "BBBB") {
		t.Fatalf("件12: the distinguishing prefix must survive: %q / %q", faceA, faceB)
	}
	// B5b 维持臂: a truncated name with NO colliding sibling keeps the
	// tail-keeping default face byte-identically (prior ruling untouched).
	long := "H:void OHOS::AbilityRuntime::ContextImpl::InitResourceManager(const AppExecFwk::BundleInfo &, const std::shared_ptr<Context>)"
	def, _ := runtimeTraceProjFamilySpanTopNameFace(long, runtimeTraceProjBusinessSpanMentionNameBudget)
	if face := runtimeTraceProjBusinessSpanMentionNameFace(long, siblings); face != def {
		t.Fatalf("件12 B5b 维持臂: a non-colliding name must keep the default cut: %q vs %q", face, def)
	}
	// A fitting name stays verbatim (no cut, no stub).
	if face := runtimeTraceProjBusinessSpanMentionNameFace("Choreographer#doFrame 76795", siblings); face != "Choreographer#doFrame 76795" {
		t.Fatalf("件12: a fitting name must render verbatim: %q", face)
	}
}
