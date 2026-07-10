package preview

import (
	"strings"
	"testing"
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
	projection := "```text\n⊚ render-thread-42 ‹用户关注线程› 满格=窗口16.667ms\n├─下钻─ ❶⚙ worker-7 █████░░░░░ 8.000ms 48% [E1]\n│ · 算力供给候选·根因排序#1·置信中\n```\n"
	page, err := RenderStandaloneMarkdownHTML("trace", []byte(projection))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<pre class="trace-projection-tree" role="region" aria-label="Trace causal projection tree" tabindex="0"><code class="language-text"><span class="trace-cell trace-cell-1 trace-icon trace-icon-root"><span class="trace-icon-glyph">⊚</span></span>`,
		`--font-mono: "Sarasa Mono SC", "Noto Sans Mono CJK SC", "Source Han Mono SC"`,
		`pre.trace-projection-tree { font-size: .8125rem; line-height: 1.52; white-space: pre; overflow-x: auto;`,
		`font-variant-ligatures: none`,
		`font-variant-emoji: text`,
		`pre.trace-projection-tree .trace-cell { display: inline-block; width: 1ch; min-width: 1ch; height: 1em;`,
		`pre.trace-projection-tree .trace-cell-2 { width: 2ch; min-width: 2ch; }`,
		`<span class="trace-rank-chip trace-rank-1 trace-rank-width-1"><span class="trace-rank-glyph">❶</span></span><span class="trace-cell trace-cell-1 trace-icon trace-icon-running"><span class="trace-icon-glyph">⚙</span></span>`,
		`<span class="trace-rank-ordinal trace-rank-1 trace-rank-width-2">#1</span>`,
		`pre.trace-projection-tree .trace-rank-chip,`,
		`height: 1em; line-height: 1em;`,
		`pre.trace-projection-tree .trace-rank-width-1 { width: 1ch; min-width: 1ch; }`,
		`pre.trace-projection-tree .trace-rank-width-2 { width: 2ch; min-width: 2ch; }`,
		`--rank-1-fg: #7c2d12; --rank-1-bg: #ffedd5;`,
		`font-family: "Apple Symbols", "Segoe UI Symbol", "Noto Sans Symbols 2"`,
		`pre.trace-projection-tree .trace-icon { display: inline-grid; place-items: center; overflow: hidden;`,
		`transform: scale(.82) translateY(.01em);`,
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
}

func TestTraceProjectionIconsUseOneCellOpticalBoxes(t *testing.T) {
	body := "```text\n⊚ app ‹用户关注线程› 满格=窗口7.000ms\n│ ⊚ ☾ ⧖ ⚙ ⛓ ⊗ ⇅ ✦ ↯ ◌ ◦\n```\n"
	html, err := RenderMarkdownHTML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	for _, class := range []string{"root", "sleep", "runnable", "running", "io", "lock", "inversion", "optimization", "interrupt", "blind", "neutral"} {
		if !strings.Contains(html, `trace-icon trace-icon-`+class+`"><span class="trace-icon-glyph">`) {
			t.Fatalf("renderer-owned icon %q lacks its fixed optical box:\n%s", class, html)
		}
	}
	for _, glyph := range []string{"⊚", "☾", "⧖", "⚙", "⛓", "⊗", "⇅", "✦", "↯", "◌", "◦"} {
		if strings.Count(html, glyph) != 1 && glyph != "⊚" {
			t.Fatalf("icon text must remain lossless for %q:\n%s", glyph, html)
		}
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

	flat, err := RenderMarkdownHTML([]byte("```text\n(本报告未做唤醒链下钻,以下各行按层级平铺) 满格=窗口8.000ms\n◦ worker ███░ 3ms\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(flat, `class="trace-projection-tree"`) {
		t.Fatalf("flat causal projection must receive trace-tree styling:\n%s", flat)
	}
}

func TestTraceProjectionRankHighlightIsClosedToSystemRankTokens(t *testing.T) {
	body := "```text\n⊚ app ‹用户关注线程› 满格=窗口7.000ms\n├─下钻─ ❶⚙ worker R1 #1 根因排序#12 根因排序#2 [E1]\n```\n"
	html, err := RenderMarkdownHTML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(html, `class="trace-rank-chip trace-rank-1 trace-rank-width-1"`) != 1 ||
		strings.Count(html, `class="trace-rank-ordinal trace-rank-2 trace-rank-width-2"`) != 1 {
		t.Fatalf("closed rank tokens were not highlighted exactly once:\n%s", html)
	}
	for _, forbidden := range []string{
		`class="trace-rank-ordinal trace-rank-1">#1</span>`, // bare name/content token
		`class="trace-rank-ordinal trace-rank-1">#12`,       // multi-digit rank is not TOP-3 #1
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
