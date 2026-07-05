package preview

import (
	"strings"
	"testing"
)

// Customer-shaped regression (RFH #66 residual ②): answer prose carries
// angle-bracket tokens that goldmark parses as inline raw HTML — JS
// stack frames ("<anonymous>"), generic instantiations ("Vec<int>") —
// and goldmark's safe mode replaced them with "<!-- raw HTML omitted -->",
// destroying load-bearing information.
//
// Ruling: escape-and-display, never drop. The anti-XSS guarantee is
// "never executed", not "never displayed": escaped literals carry zero
// execution surface while keeping every byte of content visible.
func TestRenderMarkdownHTMLEscapesRawHTMLInsteadOfDropping(t *testing.T) {
	body := "崩溃栈顶帧 <anonymous> 出现在 Vec<int> 的泛型实例化路径,随后 map<string, vector<Foo>> 也被展开。"
	out, err := RenderMarkdownHTML([]byte(body))
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	for _, want := range []string{
		"&lt;anonymous&gt;",
		"Vec&lt;int&gt;",
		"map&lt;string, vector&lt;Foo&gt;&gt;",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("raw-HTML-shaped token must survive as escaped literal %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "raw HTML omitted") {
		t.Fatalf("safe-mode placeholder must never swallow prose tokens:\n%s", out)
	}
	// Mutation guard: flipping to goldmark html.WithUnsafe (or removing
	// the escape) would emit the tokens as live markup — must stay red.
	for _, banned := range []string{"<anonymous>", "<int>", "<string,"} {
		if strings.Contains(out, banned) {
			t.Fatalf("raw HTML must never reach the page unescaped (%q):\n%s", banned, out)
		}
	}
}

// Block-level raw HTML follows the same ruling: a <script> block is
// displayed as escaped literal text (information preserved for the
// reader) with zero execution surface.
func TestRenderMarkdownHTMLEscapesHTMLBlockInsteadOfDropping(t *testing.T) {
	body := "前置说明。\n\n<script>alert(1)</script>\n\n<div class=\"x\">\n块内容 line2\n</div>\n\n后置说明。"
	out, err := RenderMarkdownHTML([]byte(body))
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	if !strings.Contains(out, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("script block must be displayed as escaped literal:\n%s", out)
	}
	if !strings.Contains(out, "&lt;div class=&#34;x&#34;&gt;") ||
		!strings.Contains(out, "块内容 line2") ||
		!strings.Contains(out, "&lt;/div&gt;") {
		t.Fatalf("html block content (including closure line) must survive escaped:\n%s", out)
	}
	if strings.Contains(out, "raw HTML omitted") {
		t.Fatalf("safe-mode placeholder must never appear:\n%s", out)
	}
	if strings.Contains(out, "<script>") || strings.Contains(out, `<div class="x">`) {
		t.Fatalf("raw HTML must never reach the page as live markup:\n%s", out)
	}
}
