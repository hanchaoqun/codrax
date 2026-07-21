package tool

// answer_document_projection_display_hyg2_test.go — DISPLAY-HYG 第二轮 pins
// (§29.104.18.1 catalog 遗留 + 各批 P3 备案群, 2026-07-17).
//
// Each test names its filing coordinate; pure display-layer pins only —
// values, seats and board order are asserted UNCHANGED wherever a fixture
// renders them.
//
// MUTATION self-checks (each VERIFIED red 2026-07-17; cp-copy recovery
// only; 复核件2② corrected the first draft's unverified M-claims):
//   - M-1 neutralize the breakLine "="-tail carry (eqTail := false) →
//     TestWrapNeverEndsLineWithEqualsTail red (the [E8]= hanging-tail form
//     re-appears in the sweep);
//   - M-2 neutralize the bare-"=" left-fusion pass (loop bound to 0) → the
//     naked-operator line-open arm red at width 57 (the EN spaced form).

import (
	"os"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEffectiveAttributionTagSpacingSingleForm — 复核件3 (P3-2 空格两档):
// the 有效归因 value tag spells ONE spacing form (值前空格, §29.104.18.1 B11
// 规范) — the emitter census is asserted at the SOURCE so a new unspaced
// site cannot land silently (three sites unified 2026-07-17; the spaced
// family already held the 行3 equation, the periodic tag and the inherited
// note).
func TestEffectiveAttributionTagSpacingSingleForm(t *testing.T) {
	src, err := os.ReadFile("answer_document_mutation_runtime_tree.go")
	if err != nil {
		t.Fatalf("read emitter source: %v", err)
	}
	if strings.Contains(string(src), `"有效归因%.`) {
		t.Fatalf("复核件3: an unspaced 有效归因 value emitter resurfaced (值前空格规范)")
	}
	if strings.Count(string(src), `"有效归因 %.`) < 4 {
		t.Fatalf("复核件3: the spaced 有效归因 emitter family census dropped — verify the sites before re-pinning")
	}
}

// TestWrapNeverEndsLineWithEqualsTail — §29.114 P3 「[E5]= 悬行尾」: the
// relation sentence 「…账目关系(见图例):本行=按本行发生段归账,[E8]=按链上聚合
// 归账(根因排序席)」 used to break right after "[E8]=" (witness
// 20260717-092738 L154 and four siblings), bisecting the assignment claim at
// its operator. The break engine now carries an "="-tailed atom down to the
// continuation, and a bare "=" fuses LEFT with its anchor (+ the single
// separating space) at atomization — so no emitted line ends with a hanging
// "=" AND no continuation opens with a naked "=". Sweep width ≥ 32: below the
// pathological lane (carry + next atom can't share any line) legitimately
// strands the operator rather than bisect a registered compound (GAPB
// cluster-word integrity outranks the strand rule); every realistic display
// width (row cap 68 / ◎ cap 100) sits far above the floor. Byte
// concatenation stays identical throughout.
func TestWrapNeverEndsLineWithEqualsTail(t *testing.T) {
	texts := []string{
		"与[E8]同线程同状态族·物理时间重叠(不可相加)·账目关系(见图例):本行=按本行发生段归账,[E8]=按链上聚合归账(根因排序席)",
		"与[E27]同线程同状态族·物理时间不相交·账目关系(见图例):本行=按状态类互斥归账(状态族合计),[E27]=按链上聚合归账(根因排序席)",
		// EN spaced form: the bare "=" fuses left with its anchor.
		"same-thread state family as [E8] · account relation (see the legend): this row = occurrence-segment account, [E8] = on-chain aggregate account (rank seat)",
	}
	for _, text := range texts {
		for width := 32; width <= 100; width++ {
			chunks := runtimeTraceProjWrapDisplay(text, width)
			if strings.Join(chunks, "") != text {
				t.Fatalf("wrap must stay lossless (width %d): %q", width, text)
			}
			for i, chunk := range chunks {
				if i != len(chunks)-1 &&
					strings.HasSuffix(strings.TrimRight(chunk, " "), "=") {
					t.Fatalf("§29.114 P3 [E#]= 悬行尾: no line may end with a hanging \"=\" (width %d):\n%q\nchunks: %q", width, text, chunks)
				}
				// 复核件2 (P2-2): the twin failure face — a continuation must
				// not OPEN with a naked "=" either (the EN spaced form used to
				// break between the anchor and the operator, e.g. width 57).
				if i != 0 && strings.HasPrefix(strings.TrimLeft(chunk, " "), "= ") {
					t.Fatalf("复核件2: no continuation may open with a naked \"=\" (width %d):\n%q\nchunks: %q", width, text, chunks)
				}
			}
		}
	}
}

// TestBarShareTextLessThanOnePercent — catalog B10: a nonzero share that
// %.0f-rounds to 0 prints `<1%` (the bar beside it always shows ≥1 filled
// cell for value>0 — 「█░░… 0%」 self-contradicted, witness ×11); a true
// zero keeps `0%` and every other share keeps its legacy print
// byte-identically. The 5-cell " NN%" layout holds on both arms (C3 错位
// discipline).
func TestBarShareTextLessThanOnePercent(t *testing.T) {
	cases := []struct {
		share float64
		want  string
	}{
		{0, "   0%"},
		{0.2, "  <1%"},
		{0.49, "  <1%"},
		{0.5, "  <1%"}, // %.0f rounds half-to-even → "0": still nonzero
		{0.51, "   1%"},
		{1, "   1%"},
		{7, "   7%"},
		{100, " 100%"},
	}
	for _, tc := range cases {
		got := runtimeTraceProjBarShareText(tc.share)
		if got != tc.want {
			t.Fatalf("share %v: got %q want %q", tc.share, got, tc.want)
		}
		if len([]rune(got)) != 5 {
			t.Fatalf("share %v: layout must stay 5 cells: %q", tc.share, got)
		}
	}
}

// TestFamilyMemberFullWindowChipBothFaces — catalog A5: a 同源二分
// re-anchored family seat (ChainAnchorFullMS > 0) wears the (全窗账) chip on
// BOTH member faces — the tree 行4+ roster rows AND the detail stanza's
// 成员 field key (one chip authority); plain family rows keep the bare
// 成员/members key byte-identically on both.
func TestFamilyMemberFullWindowChipBothFaces(t *testing.T) {
	node := scoreDerivClampedCountNode()
	node.ChainAnchorFullMS = 16.064
	for _, zh := range []bool{true, false} {
		rows := runtimeTraceProjFamilyRosterSubRows(node, zh)
		if len(rows) == 0 || !strings.Contains(rows[0], runtimeTraceProjFamilyMemberFullWindowChip(node, zh)) {
			t.Fatalf("tree roster rows must wear the chip (zh=%v): %q", zh, rows)
		}
	}
	if chip := runtimeTraceProjFamilyMemberFullWindowChip(node, true); chip != "(全窗账)" {
		t.Fatalf("zh chip: %q", chip)
	}
	projection := elimBoardProjection()
	projection.AdjacentCauses = append(projection.AdjacentCauses, node)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "- 成员(全窗账): ") {
		t.Fatalf("catalog A5: the detail stanza member field must wear the chip:\n%s", detail)
	}
	// Negative arm: a plain family row keeps the bare key.
	bare := elimBoardProjection()
	bare.AdjacentCauses = append(bare.AdjacentCauses, scoreDerivClampedCountNode())
	bareModel := buildRuntimeTraceProjTreeModel(bare, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	bareDetail := runtimeTraceProjDetailFullText(bareModel, true)
	if strings.Contains(bareDetail, "全窗账") {
		t.Fatalf("a plain family row must keep the bare 成员 key:\n%s", bareDetail)
	}
	if !strings.Contains(bareDetail, "- 成员: ") {
		t.Fatalf("the bare member field must render:\n%s", bareDetail)
	}
}

// TestMidTruncateKeepsHeadTailSegment — catalog B5 (witness L193
// 「CompThrea…-2955」 swallowed the report's only distinguishing `_0`): the
// pid-keeping middle cut preserves up to 4 cells of the head's TAIL beside
// the prefix; degenerate narrow budgets keep the legacy prefix-only forms
// byte-identically (the ptv4/lad pins).
func TestMidTruncateKeepsHeadTailSegment(t *testing.T) {
	if got := runtimeTraceProjMidTruncateKeepPid("CompThreadPool_0-2955", 16); got != "CompTh…ol_0-2955" {
		t.Fatalf("head-tail keep form: %q", got)
	}
	// Legacy narrow-budget forms stay byte-identical.
	if got := runtimeTraceProjMidTruncateKeepPid("CookieMonsterCl-59843", 7); got != "…-59843" {
		t.Fatalf("exact-fit pid lane must hold: %q", got)
	}
	if got := runtimeTraceProjMidTruncateKeepPid("RSUniRenderThre-1963", 8); got != "RS…-1963" {
		t.Fatalf("narrow prefix-only lane must hold: %q", got)
	}
}

// TestWindowRangeSeparatorUnified — catalog A7: the reader-facing window
// boundary prose speaks ONE separator (`X~Ys`, the chip form) — the former
// 「13762.792s → 13763.025s」 arrow was a third style beside 窗X~Ys chips and
// [X~Ys] evidence windows. The `..` form stays the audit/k=v wire convention
// (核对面 keeps full %.6f precision there). The arrow survives ONLY as a
// value-transition glyph (原始 X → 计入 Y), never as a window range.
func TestWindowRangeSeparatorUnified(t *testing.T) {
	projection := elimBoardProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "分析窗 2942.100~2942.300s") {
		t.Fatalf("the analysis-window line must wear the tilde form:\n%s", lead)
	}
	if strings.Contains(lead, "s → ") {
		t.Fatalf("catalog A7: no window range may keep the arrow separator:\n%s", lead)
	}
}

// TestDetailLegendGlyphLinesGateOnGlyphs — catalog B4: the table glossary's
// ⊘/⚠ lines were its only UNCONDITIONAL glyph entries (dead-entry witness
// 20260717-101345:381 — legend defines ⊘ while the body carries zero ⊘).
// They now gate on the same emission-site marks the tree legend consumes:
// a glyph-free render drops both lines; an undrillable row lights ⊘ and an
// analysis-window-crossing actual lights ⚠, exactly with their glyphs.
func TestDetailLegendGlyphLinesGateOnGlyphs(t *testing.T) {
	const dashLine = "- 「—」 = 该列对此节点无值。"
	bare := scoreDerivClusterText(t, elimBoardProjection(), "zh")
	if !strings.Contains(bare, dashLine) {
		t.Fatalf("fixture must render the table glossary:\n%s", bare)
	}
	if strings.Contains(bare, "- ⊘ = ") || strings.Contains(bare, "- ⚠ = ") {
		t.Fatalf("catalog B4: a glyph-free render must not define ⊘/⚠ in the glossary:\n%s", bare)
	}
	full := elimBoardProjection()
	undrill := elimChainNode("E-und", "waiter-9", "d_state_or_io_wait", "d_sleep", 6, 1.100, 300)
	undrill.UndrillableReason = "missing_wakeup"
	within := false
	cross := elimChainNode("E-crx", "crosser-9", "runnable_wait", "runnable", 7, 1.050, 310)
	cross.ActualImpactMS = 9.5
	cross.WithinRequestedWindow = &within
	full.OnChainCauses = append(full.OnChainCauses, undrill, cross)
	text := scoreDerivClusterText(t, full, "zh")
	if !strings.Contains(text, "⊘链止") || !strings.Contains(text, "⚠实际") {
		t.Fatalf("fixture must render both glyphs:\n%s", text)
	}
	if !strings.Contains(text, "- ⊘ = 窗口内无匹配唤醒事件(sched_wakeup),链止于此(同树内 ⊘链止)。") {
		t.Fatalf("catalog B4: the ⊘ glossary line must render with its glyph:\n%s", text)
	}
	if !strings.Contains(text, "- ⚠ = 实际状态区间确证跨出分析窗(同树内 ⚠实际Xms);") {
		t.Fatalf("catalog B4: the ⚠ glossary line must render with its glyph:\n%s", text)
	}
}

// TestFamilyStemRowMsTailAligns — 复核件1 / catalog C3 错位行 (witness
// 101345:306 「合计33.159ms」 pushed the % column right by 1): the
// family-stem value cell can exceed the legacy 11-cell " %9.3fms" slot, so
// the fence pre-computes ONE shared slot and every arm pads to it — the ms
// tails of a family row and a plain row land on the SAME column (两形:
// zh 合计 + EN total). Family-free fences keep the legacy slot by
// construction (the whole existing suite is that negative arm).
func TestFamilyStemRowMsTailAligns(t *testing.T) {
	msTailCol := func(fence, needle string) int {
		for _, line := range strings.Split(fence, "\n") {
			if !strings.Contains(line, needle) {
				continue
			}
			i := strings.Index(line, needle)
			return runewidth.StringWidth(line[:i+len(needle)])
		}
		return -1
	}
	for _, zh := range []bool{true, false} {
		projection := elimBoardProjection()
		family := elimChainNode("E-fam", "CompThread_0-2955", "d_state_or_io_wait", "d_sleep", 6, 33.159, 300)
		family.EffectiveImpactMS = 33.159
		family.FamilyMemberCount = 4
		family.FamilyFoldCaliber = tracequery.RootCauseMemberFoldCaliberSumDisjoint
		family.FamilyMemberMaxMS = 16.064
		family.FamilyMemberMinMS = 2.197
		family.FamilyMemberSumMS = 33.159
		projection.OnChainCauses = append(projection.OnChainCauses, family)
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		stem := "合计33.159ms"
		if !zh {
			stem = "total 33.159ms"
		}
		famCol := msTailCol(fence, stem)
		plainCol := msTailCol(fence, "26.392ms")
		if famCol < 0 || plainCol < 0 {
			t.Fatalf("zh=%v: fixture rows missing (fam=%d plain=%d):\n%s", zh, famCol, plainCol, fence)
		}
		if famCol != plainCol {
			t.Fatalf("复核件1 C3: family and plain ms tails must share one column (zh=%v: fam=%d plain=%d):\n%s", zh, famCol, plainCol, fence)
		}
	}
}

// TestSemanticFamilyMemberVerbatimSpanChip — catalog C12: a semantic
// family's roster rows carry engine-verbatim span names (source-side
// unbalanced parens included) — the member word wears the (span原文) chip on
// both member faces; non-semantic families keep the bare word.
func TestSemanticFamilyMemberVerbatimSpanChip(t *testing.T) {
	node := scoreDerivClampedCountNode()
	node.SemanticClass = "jit_compile"
	node.FamilyMemberRoster = []string{"JIT compiling void a.b.readIntToBcd (int) (kind=Baseline) from x.jar!classes4.dex) 0.607ms"}
	rows := runtimeTraceProjFamilyRosterSubRows(node, true)
	if len(rows) == 0 || !strings.HasPrefix(rows[0], "成员(span原文) ") {
		t.Fatalf("semantic roster rows must wear the verbatim-span chip: %q", rows)
	}
	rowsEN := runtimeTraceProjFamilyRosterSubRows(node, false)
	if len(rowsEN) == 0 || !strings.HasPrefix(rowsEN[0], "member (verbatim span) ") {
		t.Fatalf("EN semantic roster rows must wear the chip: %q", rowsEN)
	}
	plain := runtimeTraceProjFamilyRosterSubRows(scoreDerivClampedCountNode(), true)
	if len(plain) == 0 || !strings.HasPrefix(plain[0], "成员 ") {
		t.Fatalf("non-semantic families keep the bare word: %q", plain)
	}
}

// TestAnonymousNodePreviewName — catalog B9: a subject/object-less node
// carrying a member preview names itself by its members instead of the
// opaque 「(未命名因果节点)」 placeholder; nodes without any preview keep the
// placeholder (absence never invents).
func TestAnonymousNodePreviewName(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		MergedSubjects: []string{"keva-3-17439"},
		MergedCount:    5,
	}
	if got := runtimeTraceProjDetailFullName(node, true); got != "keva-3-17439 等 5 线程" {
		t.Fatalf("zh preview name: %q", got)
	}
	if got := runtimeTraceProjDetailFullName(node, false); got != "keva-3-17439, … (5 threads)" {
		t.Fatalf("en preview name: %q", got)
	}
	if got := runtimeTraceCausalProjectionNodeSubjectCell(node, true); !strings.Contains(got, "keva-3-17439") {
		t.Fatalf("the (a) node cell rides the same preview: %q", got)
	}
	var bare types.TraceCausalProjectionNode
	if got := runtimeTraceProjDetailFullName(bare, true); got != "(未命名因果节点)" {
		t.Fatalf("no preview → placeholder stays: %q", got)
	}
	// RUN2FIX-A 复核 CR-4 (2026-07-20, 刻意更新非静默): parenthesized EN form.
	if got := runtimeTraceCausalProjectionNodeSubjectCell(bare, false); got != "(unnamed causal node)" {
		t.Fatalf("EN placeholder stays: %q", got)
	}
}
