package preview

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
)

// Customer ruling 2026-07-09 (79 系回访): the standalone HTML report renders
// zh-CN-heavy answer bodies; the page CSS must (a) name CJK faces explicitly
// so Windows browsers never fall back to SimSun for the zh text, (b) keep
// prose tracking loosened (line-height ≥1.7 + slight letter-spacing), and
// (c) keep letter-spacing at 0 inside pre/code where the causal-projection
// tree relies on the CJK 2:1 per-character grid for bar alignment.
func TestStandaloneHTMLPageCJKFontAndSpacing(t *testing.T) {
	page, err := RenderStandaloneMarkdownHTML("t", []byte("# 标题\n\n中文正文。\n\n```text\n⊚ 树 █░ 1ms\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	style := page[strings.Index(page, "<style>"):strings.Index(page, "</style>")]
	for _, want := range []string{
		// CJK faces in the body stack (Windows would otherwise pick SimSun).
		`"PingFang SC"`, `"HarmonyOS Sans SC"`, `"Microsoft YaHei"`, `"Noto Sans SC"`,
		// Loosened prose rhythm for dense zh reports.
		"16px/1.78", "letter-spacing: .02em",
	} {
		if !strings.Contains(style, want) {
			t.Fatalf("page <style> missing %q", want)
		}
	}
	// The tree grid guard: pre/code must pin letter-spacing back to 0 —
	// a constant per-char pad breaks CJK double-width bar alignment.
	preRule := style[strings.Index(style, "pre, code {"):]
	preRule = preRule[:strings.Index(preRule, "}")]
	if !strings.Contains(preRule, "letter-spacing: 0") {
		t.Fatalf("pre/code rule must reset letter-spacing to 0, got: %s", preRule)
	}
}

func TestStandaloneHTMLTraceProjectionTreePresentation(t *testing.T) {
	// v5 P0 (2026-07-11, causal_tree_v5_design_20260711.md 重-1/重-3/§C.3):
	// the fence opener carries the typed info token; the state glyph and its
	// UXG-0 D5 companion space fuse into one 2ch envelope slot; pure-ASCII
	// stretches collapse into width-pinned runs; the scale transform family
	// is retired. CAL-1 件⑥a (2026-07-12): the badge ❶ and ITS D5 companion
	// space now fuse into the same 2ch envelope (colored pill at full 2ch,
	// overflow visible, ink complete) — the 1ch chip + .95 shrink is retired.
	projection := "```text trace-causal-projection\n⊚ render-thread-42 ‹用户关注线程› 满格=窗口16.667ms\n├─下钻─ ❶ ⚙ worker-7 █████░░░░░ 8.000ms 48% [E1]\n│ · 算力供给候选·根因排序#1·置信中\n```\n"
	page, err := RenderStandaloneMarkdownHTML("trace", []byte(projection))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<pre class="trace-projection-tree" role="region" aria-label="Trace causal projection tree" tabindex="0"><code class="language-text"><span class="trace-line"><span class="trace-slot trace-icon trace-icon-root"><span class="trace-ink">⊚ </span></span>`,
		`--font-mono: "Sarasa Mono SC", "Noto Sans Mono CJK SC", "Source Han Mono SC"`,
		`pre.trace-projection-tree { font-size: .8125rem; line-height: 1.52; white-space: pre; overflow-x: auto;`,
		`font-variant-ligatures: none`,
		`font-variant-emoji: text`,
		`pre.trace-projection-tree .trace-cell { display: inline-block; width: 1ch; min-width: 1ch; height: 1em;`,
		`pre.trace-projection-tree .trace-cell-2 { width: 2ch; min-width: 2ch; }`,
		// CAL-1 件⑥a (T-6 fulfilled): the badge eats its D5 companion space
		// into a 2ch envelope pill — same slot geometry as the state glyph
		// next to it; textContent keeps both bytes.
		`<span class="trace-slot trace-rank-pill trace-rank-1"><span class="trace-ink">❶ </span></span><span class="trace-slot trace-icon trace-icon-running"><span class="trace-ink">⚙ </span></span>`,
		`<span class="trace-rank-ordinal trace-rank-1 trace-rank-width-2">#1</span>`,
		// Run segmentation: the ASCII name+bar-gap stretch is ONE pinned run.
		`<span class="trace-run" style="width:9ch">worker-7 </span>`,
		// Box-drawing rails stay per-rune 1ch cells with the rail class.
		`<span class="trace-cell trace-cell-1 trace-rail">├</span><span class="trace-cell trace-cell-1 trace-rail">─</span>`,
		// Bar blocks stay per-rune 1ch cells with the bar class.
		`<span class="trace-cell trace-cell-1 trace-bar">█</span>`,
		`pre.trace-projection-tree .trace-rank-ordinal {`,
		`pre.trace-projection-tree .trace-rank-pill { border-radius: .22em; font-weight: 750;`,
		`height: 1em; line-height: 1em;`,
		`pre.trace-projection-tree .trace-rank-width-1 { width: 1ch; min-width: 1ch; }`,
		`pre.trace-projection-tree .trace-rank-width-2 { width: 2ch; min-width: 2ch; }`,
		`--rank-1-fg: #7c2d12; --rank-1-bg: #ffedd5;`,
		// UXG-0 D2: neutral ◇ channel chip colors, three themes + the rule
		// (the print pin includes its one-line :root neighbor so the light
		// theme's standalone declaration cannot satisfy it).
		`--rank-adjacent-fg: #334155; --rank-adjacent-bg: #e2e8f0;`,
		`--rank-adjacent-fg: #cbd5e1; --rank-adjacent-bg: #334155;`,
		`--rank-5-bg: #fce7f3; --rank-adjacent-fg: #334155;`,
		`pre.trace-projection-tree .trace-rank-adjacent { color: var(--rank-adjacent-fg); background: var(--rank-adjacent-bg); }`,
		`--font-symbols: "Apple Symbols", "Segoe UI Symbol", "Noto Sans Symbols 2"`,
		// v5 P0 envelope geometry (重-1): 2ch inline-grid slot, overflow
		// visible, no explicit slot height; chips keep overflow hidden
		// (colored pill stays inside its cells).
		`pre.trace-projection-tree .trace-slot { display: inline-grid; width: 2ch; min-width: 2ch; place-items: center start; overflow: visible;`,
		// F4 (fix round 2026-07-11): place-items centers only within the grid
		// AREA; Chromium sizes the auto track to overflowing ink, so the 1ch
		// slot needs justify-content to center the wider-than-track ink
		// (真机 -0.78px per side).
		`pre.trace-projection-tree .trace-slot-1 { width: 1ch; min-width: 1ch; place-items: center; justify-content: center; }`,
		// CAL-1 件⑥b: the neutral mark's ink-only optical raise (envelope
		// treatment — no qualified bigger-ink glyph replacement exists under
		// the dual-context width-1 rule).
		`pre.trace-projection-tree .trace-icon-neutral .trace-ink { font-size: 1.2em; }`,
		`pre.trace-projection-tree .trace-ink {`,
		`pre.trace-projection-tree .trace-run { display: inline-block; height: 1em; line-height: 1em;`,
		`overflow: hidden; border-radius: .22em;`,
		// 档1 (T-5): line hover, stanza-head sticky, E# anchor styling.
		`pre.trace-projection-tree .trace-line:hover {`,
		`pre.trace-projection-tree .trace-stanza-head { position: sticky; left: 0;`,
		`pre.trace-projection-tree a.trace-eref {`,
		`--link: #79c0ff; --focus: #60a5fa;`,
		`@media (max-width: 640px)`,
		`pre.trace-projection-tree { font-size: 12px; line-height: 1.48; }`,
		`@media print`,
		`pre.trace-projection-tree { font-size: 7.5pt; line-height: 1.4; overflow: visible;`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("standalone trace report missing UX contract %q", want)
		}
	}
	// 重-1 negative pin (CAL-1 件⑥a tightened, 2026-07-12): the optical scale
	// family is FULLY retired — envelope geometry, never transform tuning.
	// The former last member (the badge chip's T-6 .95 ink shrink) retired
	// with the badge envelope: ANY transform scale, any translateY, any
	// resurrection of .trace-icon-glyph or the badge chip form
	// (.trace-rank-chip / .trace-rank-glyph) are regressions.
	style := page[strings.Index(page, "<style>"):strings.Index(page, "</style>")]
	if got := strings.Count(style, "transform: scale("); got != 0 {
		t.Fatalf("tree CSS must carry ZERO scale transforms (family retired), got %d:\n%s", got, style)
	}
	for _, banned := range []string{"translateY(", ".trace-icon-glyph", "transform-origin", ".trace-rank-chip", ".trace-rank-glyph"} {
		if strings.Contains(style, banned) {
			t.Fatalf("retired scale-family CSS resurfaced (%q):\n%s", banned, style)
		}
	}
}

func TestTraceProjectionIconsUseEnvelopeSlots(t *testing.T) {
	// UXG-0 D3 (2026-07-11): ⊘ (链止/flat banner head) joined the directory.
	// v5 P0 (重-1/T-6): a directory glyph followed by its companion space
	// renders as ONE 2ch envelope slot whose ink keeps BOTH bytes; a glyph
	// with no companion space (the line-final one here, and ⊘ inside ⊘链止)
	// keeps a centered 1ch slot — both overflow:visible, never clipped.
	//
	// UXG-1 M3 (2026-07-12): the glyph roster derives from the tracefence
	// single-source directory — the SAME table the tool-side §24.3 form
	// specs and this package's classifier consume — replacing the former
	// handwritten list. A directory extension is exercised here
	// automatically; a directory removal fails the build.
	marks := tracefence.StateMarks()
	glyphs := make([]string, 0, len(marks))
	for _, mark := range marks {
		glyphs = append(glyphs, mark.Glyph)
	}
	body := "```text\n⊚ app ‹用户关注线程› 满格=窗口7.000ms\n│ " + strings.Join(glyphs, " ") + "\n```\n"
	html, err := RenderMarkdownHTML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	for _, mark := range marks[:len(marks)-1] {
		want := `<span class="trace-slot trace-icon trace-icon-` + mark.Class + `"><span class="trace-ink">` + mark.Glyph + ` </span></span>`
		if !strings.Contains(html, want) {
			t.Fatalf("glyph %q lacks its 2ch envelope slot (want %q):\n%s", mark.Glyph, want, html)
		}
	}
	// The line-final glyph has no companion space → 1ch standalone slot.
	last := marks[len(marks)-1]
	if !strings.Contains(html, `<span class="trace-slot trace-slot-1 trace-icon trace-icon-`+last.Class+`"><span class="trace-ink">`+last.Glyph+`</span></span>`) {
		t.Fatalf("space-less glyph %q must keep a 1ch standalone slot:\n%s", last.Glyph, html)
	}
	for _, mark := range marks {
		if strings.Count(html, mark.Glyph) != 1 && mark.Glyph != tracefence.RootGlyph {
			t.Fatalf("icon text must remain lossless for %q:\n%s", mark.Glyph, html)
		}
	}
}

// TestTraceProjectionKeepMarkGlyphWithoutSpaceKeepsOneCellSlot pins the T-6
// boundary case named by the design (§C.3): ⊘ inside the ⊘链止 keep-mark has
// NO companion space — it must ride the 1ch slot, and the following CJK
// bytes must stay ordinary grid cells (textContent intact).
func TestTraceProjectionKeepMarkGlyphWithoutSpaceKeepsOneCellSlot(t *testing.T) {
	body := "```text trace-causal-projection\n⊚ app ‹用户关注线程› 满格=窗口7.000ms\n│ ◌ 自身 1.000ms 无唤醒记录·⊘链止 [E3]\n```\n"
	html, err := RenderMarkdownHTML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<span class="trace-slot trace-slot-1 trace-icon trace-icon-undrillable"><span class="trace-ink">⊘</span></span><span class="trace-cell trace-cell-2">链</span>`) {
		t.Fatalf("⊘链止 keep-mark must keep the 1ch slot + ordinary CJK cells:\n%s", html)
	}
}

func TestTraceProjectionOptimizationActionTokenKeepsGridWidth(t *testing.T) {
	body := "```text\n⊚ app ‹用户关注线程› 满格=窗口7.000ms\n└─语义─ ✦ VerifyClass · 优化点\n```\n"
	html, err := RenderMarkdownHTML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<span class="trace-action-token trace-action-width-6">优化点</span>`) {
		t.Fatalf("optimization action word must be emphasized at its original width:\n%s", html)
	}
}

func TestTraceProjectionTreeClassDoesNotLeakToOrdinaryTextFences(t *testing.T) {
	ordinary, err := RenderMarkdownHTML([]byte("```text\n⊚ a user-authored sketch\n└─ node\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ordinary, "trace-projection-tree") {
		t.Fatalf("ordinary text fence received trace-tree styling:\n%s", ordinary)
	}
	lookalike, err := RenderMarkdownHTML([]byte("```text\n⊚ sketch 满格=窗口1ms\n└─ node\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(lookalike, "trace-projection-tree") {
		t.Fatalf("user-authored root/scale lookalike received system projection styling:\n%s", lookalike)
	}

	// UXG-0 D1 F-2 (2026-07-11): the former handwritten retired-wording flat
	// fixture is REPLACED — current-generator positives are engine-minted in
	// the cross-package pin (internal/tool answer_document_projection_uxg0_test
	// TestUXG0FenceHeadsClassifiedByPreview); this package keeps lookalike
	// COUNTEREXAMPLES plus the archive arm below, quoted VERBATIM from a real
	// pre-UXR-1 archived report (customlogs codrax_output_archive_20260711/
	// 20260711-150505.628-34902.md) — archived reports keep their styling.
	archived, err := RenderMarkdownHTML([]byte("```text\n(唤醒链路径未解析;以下各行按层级平铺)  窗口起止未采集·满格=本报告最大0.285ms(回退尺度,不显示占窗%)\n\n▒ 背景压力\n    ✦ T7@ZeusThreadPo-61839 · VerifyClass… ▒▒▒▒▒▒▒▒▒▒     0.285ms  类校验 · [E1]\n      · 语义优化span·类校验\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(archived, `class="trace-projection-tree"`) {
		t.Fatalf("archived pre-UXR-1 flat projection must keep trace-tree styling:\n%s", archived)
	}
}

func TestTraceProjectionRankHighlightIsClosedToSystemRankTokens(t *testing.T) {
	body := "```text\n⊚ app ‹用户关注线程› 满格=窗口7.000ms\n├─下钻─ ❶ ⚙ worker R1 #1 根因排序#12 根因排序#2 [E1]\n│ 邻近影响#12 邻近影响#3 adjacent-impact #45 x#4\n```\n"
	html, err := RenderMarkdownHTML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(html, `class="trace-slot trace-rank-pill trace-rank-1"`) != 1 ||
		strings.Count(html, `class="trace-rank-ordinal trace-rank-2 trace-rank-width-2"`) != 1 {
		t.Fatalf("closed rank tokens were not highlighted exactly once:\n%s", html)
	}
	// UXG-0 D2: the ◇ channel ordinal arm is equally closed — channel word
	// required, #1..#5 single-digit only (multi-digit and bare #N stay plain).
	if strings.Count(html, `class="trace-rank-ordinal trace-rank-adjacent trace-rank-width-2"`) != 1 {
		t.Fatalf("adjacent channel ordinal must highlight exactly once (邻近影响#3):\n%s", html)
	}
	for _, forbidden := range []string{
		`class="trace-rank-ordinal trace-rank-1">#1</span>`, // bare name/content token
		`class="trace-rank-ordinal trace-rank-1">#12`,       // multi-digit rank is not TOP-3 #1
		`trace-rank-adjacent trace-rank-width-2">#12`,       // multi-digit adjacent ordinal
		`trace-rank-adjacent trace-rank-width-2">#45`,       // multi-digit adjacent ordinal (en)
		`trace-rank-adjacent trace-rank-width-2">#4</span>`, // bare #N without the channel word
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("non-system token was rank-highlighted as %q:\n%s", forbidden, html)
		}
	}
}

func TestStandaloneHTMLDocumentLanguageAndNavigationFollowReport(t *testing.T) {
	english, err := RenderStandaloneMarkdownHTML("Trace report", []byte("# Root cause\n\nDeterministic optimization points.\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<html lang="en">`, "Codrax Report Preview", "Self-contained HTML"} {
		if !strings.Contains(english, want) {
			t.Fatalf("English report missing %q", want)
		}
	}
	mixedEnglish, err := RenderStandaloneMarkdownHTML("Trace report", []byte("# Root cause\n\nThe VerifyClass span on 线程-worker is deterministic evidence, and the scheduler chain remains the primary explanation.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mixedEnglish, `<html lang="en">`) {
		t.Fatalf("one Chinese entity must not flip an English report to zh-CN")
	}

	chinese, err := RenderStandaloneMarkdownHTML("Trace 报告", []byte("# 根因\n\n确定性优化点。\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<html lang="zh-CN">`, "Codrax 报告预览", "自包含 HTML"} {
		if !strings.Contains(chinese, want) {
			t.Fatalf("Chinese report missing %q", want)
		}
	}
}
