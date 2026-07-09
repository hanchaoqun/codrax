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
		`"PingFang SC"`, `"HarmonyOS Sans SC"`, `"Microsoft YaHei"`,
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
