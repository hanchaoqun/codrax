package preview

import (
	"os"
	"strings"
	"testing"
)

// Customer-shaped regression: prose uses a lone "~" as a range connector
// (Chinese numeric-interval convention). Two ranges in one paragraph used
// to make stock GFM strikethrough wrap everything between them in <del>
// (specimen: .codrax/output/20260705-181128.544-80798.html).
const tildeRangeProse = "com.baidu.tieba 59566 主线程在 34579.472865s~34579.587805s 的 114.940ms 帧窗口内共出现 4 个卡顿子窗口，唤醒链每层延迟 6~11ms。"

func TestRenderMarkdownHTMLSingleTildeRangeStaysLiteral(t *testing.T) {
	out, err := RenderMarkdownHTML([]byte(tildeRangeProse))
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	if strings.Contains(out, "<del>") || strings.Contains(out, "</del>") {
		t.Fatalf("single-tilde range connectors must never render as strikethrough:\n%s", out)
	}
	if !strings.Contains(out, "34579.472865s~34579.587805s") ||
		!strings.Contains(out, "6~11ms") {
		t.Fatalf("tilde ranges must survive literally in prose:\n%s", out)
	}
}

func TestRenderMarkdownHTMLDoubleTildeKeepsGFMStrikethrough(t *testing.T) {
	out, err := RenderMarkdownHTML([]byte("前一版结论 ~~全帧丢失~~ 已被修正。"))
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	if !strings.Contains(out, "<del>全帧丢失</del>") {
		t.Fatalf("double-tilde must keep standard GFM strikethrough:\n%s", out)
	}
}

func TestRenderMarkdownHTMLMixedSingleAndDoubleTilde(t *testing.T) {
	out, err := RenderMarkdownHTML([]byte("区间 3~5ms 与 ~~已废弃~~ 并存，另一区间 7~9ms。"))
	if err != nil {
		t.Fatalf("RenderMarkdownHTML: %v", err)
	}
	if !strings.Contains(out, "<del>已废弃</del>") {
		t.Fatalf("double-tilde span lost in mixed prose:\n%s", out)
	}
	if !strings.Contains(out, "3~5ms") || !strings.Contains(out, "7~9ms") {
		t.Fatalf("single-tilde ranges corrupted in mixed prose:\n%s", out)
	}
	if strings.Count(out, "<del>") != 1 {
		t.Fatalf("exactly one strikethrough span expected:\n%s", out)
	}
}

func TestRenderStandaloneMarkdownHTMLSingleTildeRangeNoDel(t *testing.T) {
	page, err := RenderStandaloneMarkdownHTML("t", []byte(tildeRangeProse))
	if err != nil {
		t.Fatalf("RenderStandaloneMarkdownHTML: %v", err)
	}
	// The embedded mermaid bundle legitimately contains "<del>" inside
	// its minified JS, so assert on the rendered prose itself: ranges
	// stay literal and no <del> opens on the range content.
	if !strings.Contains(page, "34579.472865s~34579.587805s") {
		t.Fatalf("standalone page lost literal tilde range")
	}
	if strings.Contains(page, "<del>34579") || strings.Contains(page, "<del>6") {
		t.Fatalf("standalone page struck through prose between tilde ranges")
	}
}

// Source guard: the single point that owns the ruling is the goldmark
// extension list in markdown.go. Reintroducing extension.GFM (or the
// stock extension.Strikethrough) silently restores single-tilde <del>,
// which the behavioral pins above would catch — this guard names the
// exact regression vector so the failure is self-explaining.
func TestMarkdownRendererDoesNotUseStockGFMStrikethrough(t *testing.T) {
	src, err := os.ReadFile("markdown.go")
	if err != nil {
		t.Fatalf("read markdown.go: %v", err)
	}
	// Match code only: comments may (and do) name extension.GFM when
	// explaining why it is banned.
	var code []string
	for _, line := range strings.Split(string(src), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		code = append(code, line)
	}
	text := strings.Join(code, "\n")
	if strings.Contains(text, "extension.GFM") {
		t.Fatalf("markdown.go must not use extension.GFM: its bundled Strikethrough opens <del> on a single tilde; use strikethroughDoubleTildeOnly (see strikethrough.go)")
	}
	if strings.Contains(text, "extension.Strikethrough") {
		t.Fatalf("markdown.go must not use stock extension.Strikethrough (single-tilde <del>); use strikethroughDoubleTildeOnly (see strikethrough.go)")
	}
	if !strings.Contains(text, "strikethroughDoubleTildeOnly") {
		t.Fatalf("markdown.go lost the strikethroughDoubleTildeOnly extension (ruling 2026-07-05: single ~ never strikes)")
	}
}
