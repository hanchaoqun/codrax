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
	// refined-D 负臂: the typed non-IO proof outranks the merged IO阻塞 root.
	refined := types.TraceCausalProjectionNode{DStateRefinedNonIO: true}
	if word, ok := runtimeTraceProjElimVerdictTokenWord(refined, "d_state_or_io_wait", true); ok {
		t.Fatalf("件11 refined 负臂: the proven non-IO row must keep its refined word, got %q", word)
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
	aux := idx("\n\n— 辅助 —")
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
		if line == "— 辅助 —" {
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
	auxAt := strings.Index(fence, "— 辅助 —")
	if auxAt < 0 {
		t.Fatalf("the aux zone must render:\n%s", fence)
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
	// else (two-level indentation, no third level).
	inAux := false
	for _, line := range strings.Split(fence, "\n") {
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		if line == "— 辅助 —" {
			inAux = true
			continue
		}
		switch {
		case strings.HasPrefix(line, "· "): // aux list row
		case strings.HasPrefix(line, "  另有") || strings.HasPrefix(line, "  …"): // tail note
		case strings.HasPrefix(line, "  ") && inAux: // width-governor continuation (safety net)
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
	if !strings.Contains(fence, "Binder:43397_19-23088 13.982ms · 有唤醒凭证,按口径不拆段入榜") {
		t.Fatalf("§29.175.14 词形: the 未入榜最大 row must carry the ruled form:\n%s", fence)
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
		for _, banned := range []string{"R3 ", "R4", "候选池", "R3边", "R3 边"} {
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

// TestOmgcleanValueChannelUntouchedByRender — 硬纪律1 guard (strip-then-
// compare): rendering the ◎ fence mutates NO value/ordinal channel — the
// board built from the same model before and after the render is deep-equal,
// and a second render is byte-identical (deterministic display, zero value
// side effects).
func TestOmgcleanValueChannelUntouchedByRender(t *testing.T) {
	projection := elimBoardProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	runtimeTraceProjTreeFence(model, true)
	before := runtimeTraceProjElimBoard(model)
	fence1 := runtimeTraceProjElimOverviewFence(projection, model, true)
	after := runtimeTraceProjElimBoard(model)
	if len(before) != len(after) {
		t.Fatalf("the render must not change the board population (%d → %d)", len(before), len(after))
	}
	for i := range before {
		a, b := before[i], after[i]
		if a.row.Node.EffectiveImpactMS != b.row.Node.EffectiveImpactMS ||
			a.row.Node.Rank != b.row.Node.Rank || a.channelRank != b.channelRank ||
			a.homeOrder != b.homeOrder {
			t.Fatalf("the render must not touch the value/ordinal channels (entry %d)", i)
		}
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
