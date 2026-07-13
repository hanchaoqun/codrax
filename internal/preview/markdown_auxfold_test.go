package preview

import (
	"os"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// §29.9 aux-reference appendix pins (UXAUX batch; revised user ruling
// 2026-07-09 — <details> fold form rejected, relocation form pinned).
//
// The fixture testdata/auxfold_opendir_792.md is a VERBATIM byte-copy of
// customer specimen cust_trace_opendir_792.txt lines 60-172 (engine-actual
// Markdown: the real 树读法 legend block, the causal-projection ```text
// fence, the real 各列口径 glossary block, and the metric table head).
// Do not hand-edit it — fixtures carry the engine-cast shape.

func auxFoldFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/auxfold_opendir_792.md")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

// mustIndex returns the byte offset of sub in s, failing the test when absent.
func mustIndex(t *testing.T, s, sub, what string) int {
	t.Helper()
	i := strings.Index(s, sub)
	if i < 0 {
		t.Fatalf("%s: output does not contain %q", what, sub)
	}
	return i
}

const (
	auxPointerHTML   = `<p class="aux">（树读法与各列口径见文末「阅读参考」）</p>`
	auxAppendixOpen  = "<section class=\"aux\">\n<h2>阅读参考</h2>"
	auxAppendixClose = "</section>"
)

// nthIndex returns the byte offset of the n-th (0-based) occurrence of
// sub in s, failing the test when there are not enough occurrences.
func nthIndex(t *testing.T, s, sub string, n int, what string) int {
	t.Helper()
	at := 0
	for i := 0; ; i++ {
		rel := strings.Index(s[at:], sub)
		if rel < 0 {
			t.Fatalf("%s: occurrence %d of %q not found", what, n, sub)
		}
		at += rel
		if i == n {
			return at
		}
		at += len(sub)
	}
}

// Pin ① — the two real opendir_792 aux blocks relocate, complete, to
// the document-end 「阅读参考」 appendix (heading verbatim; appendix
// after ALL body content) and no longer exist at their original sites;
// pin ② half — each original site carries exactly one verbatim pointer
// line; pin ④ half — the non-marker "分析窗 …" paragraph and its list
// stay in place; pin ⑤ — no <details> anywhere (anti-regression to the
// rejected fold form); and the L7/L8-adjacent static guard: the
// causal-projection ```text fence stays an ordinary escaped
// <pre><code> in the body, never a mermaid container.
func TestAuxAppendixRelocatesOpendir792ReferenceBlocks(t *testing.T) {
	html, err := RenderMarkdownHTML(auxFoldFixture(t))
	if err != nil {
		t.Fatal(err)
	}

	// Pin ⑤: the rejected <details> form must never come back.
	if strings.Contains(html, "<details") {
		t.Fatal("no <details> may be emitted (rejected §29.9 draft form)")
	}

	// Exactly one appendix, opened with the verbatim weakened heading.
	if got := strings.Count(html, "<section"); got != 1 {
		t.Fatalf("want exactly 1 appendix <section>, got %d", got)
	}
	appendixAt := mustIndex(t, html, auxAppendixOpen, "appendix opener")
	appendixEnd := appendixAt + strings.Index(html[appendixAt:], auxAppendixClose)
	if appendixEnd < appendixAt {
		t.Fatal("appendix never closes")
	}
	appendixBody := html[appendixAt:appendixEnd]

	// Appendix sits after ALL body content (the metric table is the
	// last body block in the fixture).
	if tableEnd := mustIndex(t, html, "</table>", "metric table close"); appendixAt < tableEnd {
		t.Fatal("appendix must render after all body content")
	}

	// Both blocks are complete inside the appendix: marker paragraphs,
	// first / deep-nested / last legend leaves, first / last glossary
	// entries. Every probe is unique to its block, so Count==1 also
	// proves the block was MOVED (gone from the original site), not
	// copied.
	for _, probe := range []string{
		"<p>树读法:</p>",
		"自上而下 = 从关注线程向上游追溯。",      // first legend item
		"<code>├─下钻─</code>",      // nested 边: sublist leaf
		"<code>计数当量Xms</code>",    // deep 口径: sublist leaf
		"满格 = 树头标注的长度(本报告为分析窗全长)", // 时长条 leaf
		"仅按频率比折算;真实缺口只多不少",        // last legend leaf
		"<p>各列口径:</p>",
		"窗口投影 = 该节点的状态落在分析窗内的时长",                    // first glossary item
		"N线程取最大(单项a~b) = 跨线程折叠,数值取成员最大(墙钟跨线程不可加和)。", // last glossary item
	} {
		if got := strings.Count(html, probe); got != 1 {
			t.Fatalf("probe %q: want exactly 1 occurrence (moved, not copied), got %d", probe, got)
		}
		if !strings.Contains(appendixBody, probe) {
			t.Fatalf("appendix missing relocated content %q", probe)
		}
	}
	// First-appearance order inside the appendix: legend before glossary.
	if strings.Index(appendixBody, "<p>树读法:</p>") > strings.Index(appendixBody, "<p>各列口径:</p>") {
		t.Fatal("appendix must keep first-appearance order (树读法 before 各列口径)")
	}

	// Source labels (single-projection natural form, pinned): both
	// blocks sit under the same "### Trace 因果投影" heading, so each
	// block carries the same label line, immediately before it.
	labelHTML := `<p class="aux-src">—— 来自: Trace 因果投影</p>`
	if got := strings.Count(html, labelHTML); got != 2 {
		t.Fatalf("want 2 source labels (one per relocated block), got %d", got)
	}
	lbl0 := nthIndex(t, appendixBody, labelHTML, 0, "first source label")
	lbl1 := nthIndex(t, appendixBody, labelHTML, 1, "second source label")
	treeAt := mustIndex(t, appendixBody, "<p>树读法:</p>", "appendix 树读法 paragraph")
	colsAt := mustIndex(t, appendixBody, "<p>各列口径:</p>", "appendix 各列口径 paragraph")
	if !(lbl0 < treeAt && treeAt < lbl1 && lbl1 < colsAt) {
		t.Fatalf("each source label must precede its block: lbl0=%d tree=%d lbl1=%d cols=%d", lbl0, treeAt, lbl1, colsAt)
	}

	// Pin ② half: exactly two verbatim pointer lines (the two original
	// sites are not adjacent), positioned at the original sites: after
	// the 分析窗 block, before the tree fence; and between the fence
	// and the metric table.
	if got := strings.Count(html, auxPointerHTML); got != 2 {
		t.Fatalf("want exactly 2 pointer lines, got %d", got)
	}
	ptr1 := mustIndex(t, html, auxPointerHTML, "first pointer")
	ptr2 := ptr1 + len(auxPointerHTML) + strings.Index(html[ptr1+len(auxPointerHTML):], auxPointerHTML)
	fence := mustIndex(t, html, `<pre class="trace-projection-tree"`, "projection tree fence")
	tableAt := mustIndex(t, html, "<table>", "metric table")
	winPara := mustIndex(t, html, "分析窗 33872.289s", "分析窗 paragraph")
	winItem := mustIndex(t, html, "链上已归因 112.175ms(94%)", "分析窗 list item")
	if !(winPara < ptr1 && winItem < ptr1 && ptr1 < fence && fence < ptr2 && ptr2 < tableAt && tableAt < appendixAt) {
		t.Fatalf("site order broken: winPara=%d winItem=%d ptr1=%d fence=%d ptr2=%d table=%d appendix=%d",
			winPara, winItem, ptr1, fence, ptr2, tableAt, appendixAt)
	}

	// The fence is untouched and never mis-detected as mermaid.
	if strings.Contains(html, `<div class="mermaid">`) {
		t.Fatal("fixture has no mermaid content; no mermaid container may appear")
	}
}

// Pin ② adjacency merge — two marker blocks that are ADJACENT at the
// same original site leave only ONE pointer line, and both blocks land
// in the appendix.
func TestAuxAppendixAdjacentBlocksShareOnePointer(t *testing.T) {
	md := "正文段。\n\n树读法:\n- 自上而下 = x\n\n各列口径:\n- 窗口投影 = y\n\n尾段。\n"
	html, err := RenderMarkdownHTML([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(html, auxPointerHTML); got != 1 {
		t.Fatalf("adjacent moved blocks must share one pointer line, got %d:\n%s", got, html)
	}
	appendixAt := mustIndex(t, html, auxAppendixOpen, "appendix opener")
	for _, probe := range []string{"<p>树读法:</p>", "自上而下 = x", "<p>各列口径:</p>", "窗口投影 = y"} {
		if at := mustIndex(t, html, probe, "relocated block content"); at < appendixAt {
			t.Fatalf("%q must live in the appendix only", probe)
		}
	}
	// Body prose around the site is untouched and stays before the appendix.
	ptr := mustIndex(t, html, auxPointerHTML, "pointer")
	head := mustIndex(t, html, "正文段。", "leading paragraph")
	tail := mustIndex(t, html, "尾段。", "trailing paragraph")
	if !(head < ptr && ptr < tail && tail < appendixAt) {
		t.Fatalf("order broken: head=%d ptr=%d tail=%d appendix=%d", head, ptr, tail, appendixAt)
	}
}

// Pin ③ — byte-identical duplicate blocks dedup to ONE appendix copy
// (first occurrence), while every original site still gets its pointer
// line.
func TestAuxAppendixDedupsByteIdenticalBlocks(t *testing.T) {
	md := "树读法:\n- 自上而下 = x\n\n中间正文。\n\n树读法:\n- 自上而下 = x\n"
	html, err := RenderMarkdownHTML([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(html, auxPointerHTML); got != 2 {
		t.Fatalf("both duplicate sites must keep a pointer, got %d:\n%s", got, html)
	}
	if got := strings.Count(html, "<p>树读法:</p>"); got != 1 {
		t.Fatalf("byte-identical blocks must dedup to one appendix copy, got %d:\n%s", got, html)
	}
	if got := strings.Count(html, "自上而下 = x"); got != 1 {
		t.Fatalf("deduped list content must appear once, got %d:\n%s", got, html)
	}
	mustIndex(t, html, auxAppendixOpen, "appendix opener")
}

// Pin ③ negative arm — byte-DIFFERENT variants each keep their own
// appendix copy. Includes the strongest false-merge shape: two blocks
// whose flattened plain text is identical and only the list STRUCTURE
// differs (nested vs flat) — the byte-span dedup key must keep both.
func TestAuxAppendixKeepsByteDifferentVariants(t *testing.T) {
	for name, md := range map[string]string{
		"value variant":          "树读法:\n- 自上而下 = x\n\n正文。\n\n树读法:\n- 自上而下 = y\n",
		"structure-only variant": "树读法:\n- 甲\n  - 乙\n\n正文。\n\n树读法:\n- 甲\n- 乙\n",
	} {
		html, err := RenderMarkdownHTML([]byte(md))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := strings.Count(html, "<p>树读法:</p>"); got != 2 {
			t.Fatalf("%s: variants must NOT dedup, want 2 appendix copies, got %d:\n%s", name, got, html)
		}
		if got := strings.Count(html, auxPointerHTML); got != 2 {
			t.Fatalf("%s: want 2 pointer lines, got %d", name, got)
		}
	}
}

// Pin ④ — the exact-equality gate: a marker word that is merely the
// PREFIX of a paragraph (content in the same paragraph, or a soft-broken
// second line) must not move anything, even when a list follows.
func TestAuxAppendixExactEqualityGateRejectsPrefixForms(t *testing.T) {
	for name, md := range map[string]string{
		"same-line content":       "树读法:自上而下追溯。\n- 自上而下 = x\n",
		"soft-broken second line": "各列口径:\n窗口投影说明。\n\n- 窗口投影 = x\n",
		"trailing word":           "各列口径:补\n- 窗口投影 = x\n",
	} {
		html, err := RenderMarkdownHTML([]byte(md))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(html, "<section") || strings.Contains(html, "阅读参考") {
			t.Fatalf("%s: prefix/mixed marker paragraph must never move, got:\n%s", name, html)
		}
	}
}

// Conservative arm — a lone marker paragraph with no immediately
// following List is left alone: no appendix, no pointer, paragraph
// rendered unchanged in place.
func TestAuxAppendixMarkerWithoutListIsUntouched(t *testing.T) {
	for name, md := range map[string]string{
		"followed by paragraph": "树读法:\n\n正文段落。\n",
		"at end of document":    "正文。\n\n各列口径:\n",
		"followed by fence":     "树读法:\n\n```text\nx\n```\n",
	} {
		html, err := RenderMarkdownHTML([]byte(md))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(html, "<section") || strings.Contains(html, "阅读参考") {
			t.Fatalf("%s: marker without a following list must not move, got:\n%s", name, html)
		}
	}
	html, err := RenderMarkdownHTML([]byte("树读法:\n\n正文段落。\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<p>树读法:</p>") {
		t.Fatalf("lone marker paragraph must render unchanged, got:\n%s", html)
	}
}

// The §29.9 ruling text writes the markers with a fullwidth colon; the
// engine emits the ASCII form. Both exact byte forms relocate (closed
// set of four strings — still no fuzzy matching).
func TestAuxAppendixAcceptsBothColonByteForms(t *testing.T) {
	for name, md := range map[string]string{
		"ascii colon":     "树读法:\n- 自上而下 = x\n",
		"fullwidth colon": "树读法：\n- 自上而下 = x\n",
	} {
		html, err := RenderMarkdownHTML([]byte(md))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(html, auxAppendixOpen) || strings.Count(html, auxPointerHTML) != 1 {
			t.Fatalf("%s: expected relocation with one pointer, got:\n%s", name, html)
		}
	}
}

// Pin ④ escaping — markdown-authored angle-bracket tokens in a
// relocated list still pass through the escaping pipeline inside the
// appendix (same anti-XSS ruling as rawHTMLLiteralRenderer / RFH #66);
// the pointer/appendix markup is renderer-emitted scaffolding, not a
// hole.
func TestAuxAppendixKeepsEscapingPipelineInsideAppendix(t *testing.T) {
	html, err := RenderMarkdownHTML([]byte("树读法:\n- 栈帧 <anonymous> 处等待\n- 泛型 Vec<int> 示例\n- <script>alert(1)</script>\n"))
	if err != nil {
		t.Fatal(err)
	}
	appendixAt := mustIndex(t, html, auxAppendixOpen, "appendix opener")
	body := html[appendixAt:]
	for _, want := range []string{"&lt;anonymous&gt;", "Vec&lt;int&gt;", "&lt;script&gt;"} {
		if !strings.Contains(body, want) {
			t.Fatalf("relocated list must keep escaped literal %q, got:\n%s", want, html)
		}
	}
	for _, forbid := range []string{"<anonymous>", "<script>alert"} {
		if strings.Contains(html, forbid) {
			t.Fatalf("raw markup %q leaked unescaped through the appendix", forbid)
		}
	}
}

// Pin CSS + surface coverage — the .aux / appendix CSS ships in the
// standalone page style block (same style as the §28.7 CJK font pin),
// the rejected details rules are gone, and the standalone .html sidecar
// (customer report) shares RenderMarkdownHTML so pointer + appendix
// appear there too.
func TestStandaloneHTMLPageAuxAppendixCSSAndBody(t *testing.T) {
	page, err := RenderStandaloneMarkdownHTML("t", auxFoldFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	style := page[strings.Index(page, "<style>"):strings.Index(page, "</style>")]
	for _, want := range []string{
		".aux {",
		"section.aux {",
		"section.aux h2 {",
		".aux-src {",
	} {
		if !strings.Contains(style, want) {
			t.Fatalf("page <style> missing %q", want)
		}
	}
	if strings.Contains(style, "details.aux") {
		t.Fatal("rejected details.aux CSS must be gone")
	}
	// Pointer lines and appendix render small and muted.
	auxRule := style[strings.Index(style, ".aux {"):]
	auxRule = auxRule[:strings.Index(auxRule, "}")]
	for _, want := range []string{"var(--muted)", "font-size"} {
		if !strings.Contains(auxRule, want) {
			t.Fatalf(".aux rule must keep the region small and muted, got: %s", auxRule)
		}
	}
	// Both surfaces: the standalone (customer report) page carries the
	// pointer lines and the appendix, and never a <details>.
	if strings.Count(page, auxPointerHTML) != 2 || !strings.Contains(page, auxAppendixOpen) {
		t.Fatal("standalone page missing pointer lines or appendix")
	}
	if strings.Contains(page, "<details") {
		t.Fatal("standalone page must not contain <details>")
	}
}

// Source-label pins — the double-projection cmp_792 form. The fixture
// testdata/auxfold_cmp_792.md is assembled from four VERBATIM slices of
// customer specimen cust_trace_cmp_792.txt (lines 206-255 / 366-382 /
// 674-722 / 791-806, blank-line separated): two projections, each with
// its own "### Trace 因果投影 — <artifact>" heading and its own 树读法 +
// 各列口径 blocks; the A/B copies are byte-DIFFERENT (hex-diff verified),
// so dedup keeps all four. Each of the four appendix blocks must carry
// the source label of ITS projection heading, immediately before it.
func TestAuxAppendixSourceLabelsCmp792(t *testing.T) {
	src, err := os.ReadFile("testdata/auxfold_cmp_792.md")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	html, err := RenderMarkdownHTML(src)
	if err != nil {
		t.Fatal(err)
	}
	labelA := `<p class="aux-src">—— 来自: Trace 因果投影 — 7.0B30SP22_7315.systrace</p>`
	labelB := `<p class="aux-src">—— 来自: Trace 因果投影 — 6.0B138_3900.sys.systrace</p>`

	// Four sites, none adjacent → four pointers; four byte-different
	// blocks → four appendix copies, two labels per projection.
	if got := strings.Count(html, auxPointerHTML); got != 4 {
		t.Fatalf("want 4 pointer lines, got %d", got)
	}
	for probe, want := range map[string]int{
		labelA:                   2,
		labelB:                   2,
		"<p>树读法:</p>":            2,
		"<p>各列口径:</p>":           2,
		"<code>⚙/running</code>": 1, // A-only legend leaf (variant witness)
		"<code>├─自身─</code>":     1, // B-only legend edge (variant witness)
	} {
		if got := strings.Count(html, probe); got != want {
			t.Fatalf("probe %q: want %d occurrences, got %d", probe, want, got)
		}
	}

	appendixAt := mustIndex(t, html, auxAppendixOpen, "appendix opener")
	appendixBody := html[appendixAt:]
	// Interleaving inside the appendix: labelA, 树读法A, labelA,
	// 各列口径A, labelB, 树读法B, labelB, 各列口径B.
	seq := []int{
		nthIndex(t, appendixBody, labelA, 0, "label A #1"),
		nthIndex(t, appendixBody, "<p>树读法:</p>", 0, "树读法 A"),
		nthIndex(t, appendixBody, labelA, 1, "label A #2"),
		nthIndex(t, appendixBody, "<p>各列口径:</p>", 0, "各列口径 A"),
		nthIndex(t, appendixBody, labelB, 0, "label B #1"),
		nthIndex(t, appendixBody, "<p>树读法:</p>", 1, "树读法 B"),
		nthIndex(t, appendixBody, labelB, 1, "label B #2"),
		nthIndex(t, appendixBody, "<p>各列口径:</p>", 1, "各列口径 B"),
	}
	for i := 1; i < len(seq); i++ {
		if seq[i-1] >= seq[i] {
			t.Fatalf("appendix label/block interleaving broken at step %d: %v", i, seq)
		}
	}
}

// Source-label conservative arm — a document with no heading before the
// marker block gets no label line (and dedup semantics are untouched by
// labels: the label never enters the dedup key).
func TestAuxAppendixSourceLabelOmittedWithoutHeading(t *testing.T) {
	html, err := RenderMarkdownHTML([]byte("树读法:\n- 自上而下 = x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "aux-src") || strings.Contains(html, "来自") {
		t.Fatalf("no preceding heading → no source label, got:\n%s", html)
	}
	mustIndex(t, html, auxAppendixOpen, "appendix opener")
}

// P3-1 — inlinePlainText shares the bounds guard of the
// auxBlockSourceKey main path: synthetic out-of-range / reversed /
// negative segments are skipped, never sliced (no panic), and valid
// segments still contribute.
func TestInlinePlainTextSkipsOutOfRangeSegments(t *testing.T) {
	source := []byte("树读法:")
	para := ast.NewParagraph()
	para.AppendChild(para, ast.NewTextSegment(text.NewSegment(0, len(source))))
	para.AppendChild(para, ast.NewTextSegment(text.NewSegment(2, len(source)+7))) // Stop beyond source
	para.AppendChild(para, ast.NewTextSegment(text.Segment{Start: 5, Stop: 2}))   // reversed
	para.AppendChild(para, ast.NewTextSegment(text.Segment{Start: -3, Stop: 2}))  // negative start
	if got := inlinePlainText(para, source); got != "树读法:" {
		t.Fatalf("corrupted segments must be skipped, valid kept; got %q", got)
	}
}

// Pin determinism — rendering the same real-report fixture twice is
// byte-equal on both surfaces (shared entry + standalone page).
func TestAuxAppendixRenderIsDeterministic(t *testing.T) {
	src := auxFoldFixture(t)
	first, err := RenderMarkdownHTML(src)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderMarkdownHTML(src)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("RenderMarkdownHTML is not byte-deterministic across runs")
	}
	pageA, err := RenderStandaloneMarkdownHTML("t", src)
	if err != nil {
		t.Fatal(err)
	}
	pageB, err := RenderStandaloneMarkdownHTML("t", src)
	if err != nil {
		t.Fatal(err)
	}
	if pageA != pageB {
		t.Fatal("RenderStandaloneMarkdownHTML is not byte-deterministic across runs")
	}
}
