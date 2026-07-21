package tool

// answer_document_projection_mark70_test.go — LT-HYG mark70 pin (§29.79
// 观察续档, 2026-07-14): the ◎ overview footnote emits the ⌗ 口径旁栏 word
// family (计数当量 on count-class rows) through
// runtimeTraceProjCaliberSideWord WITHOUT lighting the marks, while the tree
// 行2 site lights them — under a tree form that skips the 行2 ⌗ arm (the
// witnessed ◇ folded/derived form carries the count-class REGISTRY token but
// not the typed caliber_side tier), the wordface and its legend entry
// decouple: the reader meets 计数当量 with no legend teaching it.
//
// Fix under pin: the footnote emission point lights the SAME marks as the
// tree site (runtimeTraceProjMarkCaliberSideRow + count-class →
// runtimeTraceProjMarkFamilyCountEquivalent, mark 70) — the NEW-7
// bidirectional legend sweep then auto-guards both directions for every
// future shape. Red without the footnote tick (verified by reverting it):
// the fence emits 计数当量 while mark 70 stays unlit and the legend entry
// never renders.

import (
	"strings"
	"testing"
)

func TestMark70FootnoteOnlyCountEquivalentKeepsLegendCoupled(t *testing.T) {
	for _, zh := range []bool{true, false} {
		// elimBoardProjection is the in-suite production form of the
		// decoupling: its ◇ count-class row (E-pgc page_cache_churn) carries
		// the REGISTRY count token under Tier "tertiary" — the tree 行2 ⌗
		// arm (typed caliber_side tier only) never runs, so the ◎ footnote
		// is the word's ONLY emitter.
		proj := elimBoardProjection()
		model := buildRuntimeTraceProjTreeModel(proj, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		elim := runtimeTraceProjElimOverviewFence(proj, model, zh)
		if elim == "" {
			t.Fatal("the ◎ overview fence must render for this shape")
		}
		surface := fence + "\n" + elim

		// The word family reaches the rendered surface through the ◎ aux row.
		// 双复核修复 件3 (冷读 CR6, 2026-07-21). EVOLUTION RECORD: the EN row
		// appended a bare 「· 计数当量」 zh residual — deleted (count-equivalent
		// already leads the EN value form), so the EN face couples through the
		// count-equivalent stem while zh keeps 计数当量 (the zh-en 同词裁定's
		// scope is the kernel state words, 冷读 CR6 判).
		wordFace := "计数当量"
		if !zh {
			wordFace = "count-equivalent"
		}
		if !strings.Contains(elim, wordFace) {
			t.Fatalf("footnote must emit the count-class caliber word %q:\n%s", wordFace, elim)
		}
		// OMGCLEAN-1 件9 (§29.175.8/.13, 2026-07-20). EVOLUTION RECORD: the
		// ⌗ seats ride plain 口径旁栏 aux rows — the ⌗ glyph is stripped from
		// the ◎ face (树/明细 keep it); the word coupling now rides the
		// 计数当量 value form + the 口径旁栏 label (NEW-7 probe coupling).
		if !strings.Contains(surface, "· 口径旁栏") &&
			!strings.Contains(surface, "· caliber sidebar") {
			t.Fatalf("the caliber aux row must carry the seat:\n%s", surface)
		}

		// 词条-图例双向: the emission lights mark 70 (count comparison-form
		// legend seat) and the ⌗ row seat, so the legend teaches what the
		// footnote speaks.
		if !model.Marks.has(runtimeTraceProjMarkFamilyCountEquivalent) {
			t.Fatal("mark 70 (计数当量 legend seat) must be lit by the footnote emission point")
		}
		if !model.Marks.has(runtimeTraceProjMarkCaliberSideRow) {
			t.Fatal("the ⌗ 口径旁栏 legend seat must be lit by the footnote emission point")
		}
		lang := "zh"
		if !zh {
			lang = "en"
		}
		lead := runtimeTraceProjLeadText(proj, model, lang, zh)
		// 件13 (2026-07-21): the legend token carries the 定稿 space.
		legendProbe := "计数当量 X(非墙钟)"
		if !strings.Contains(lead, legendProbe) {
			t.Fatalf("the mark-70 legend entry must render for the footnote-only form:\n%s", lead)
		}
	}
}
